package main

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"cloud.google.com/go/compute/metadata"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	defaultGooglePlayBaseURL       = "https://androidpublisher.googleapis.com"
	defaultGooglePlayTokenURL      = "https://oauth2.googleapis.com/token"
	googlePlayPublisherScope       = "https://www.googleapis.com/auth/androidpublisher"
	googlePlayServiceAccountEnvVar = "NSGO_PLAY_SERVICE_ACCOUNT"
	googleAppCredentialsEnvVar     = "GOOGLE_APPLICATION_CREDENTIALS"
)

// findGoogleDefaultCredentials and googleMetadataEmail are seams for testing ADC fallback deterministically.
var (
	findGoogleDefaultCredentials = func(ctx context.Context, scopes ...string) (*google.Credentials, error) {
		return google.FindDefaultCredentials(ctx, scopes...)
	}
	googleMetadataEmail = func(ctx context.Context) string {
		if !metadata.OnGCE() {
			return ""
		}
		email, err := metadata.GetWithContext(ctx, "instance/service-accounts/default/email")
		if err != nil {
			return ""
		}
		return strings.TrimSpace(email)
	}
)

type googlePlayCredentialInfo struct {
	Type    string
	Account string
	Source  string
}

func (c googlePlayCredentialInfo) identity() string {
	switch {
	case c.Account != "" && c.Type != "":
		return fmt.Sprintf("%s (%s)", c.Account, c.Type)
	case c.Account != "":
		return c.Account
	case c.Type != "":
		return c.Type + " credentials"
	default:
		return ""
	}
}

func (c googlePlayCredentialInfo) summary() string {
	var parts []string
	if c.Type != "" {
		parts = append(parts, c.Type)
	}
	if c.Account != "" {
		parts = append(parts, c.Account)
	}
	label := strings.Join(parts, " ")
	if c.Source != "" {
		if label != "" {
			return fmt.Sprintf("%s (%s)", label, c.Source)
		}
		return c.Source
	}
	return label
}

type googlePlayClient struct {
	baseURL       string
	uploadBaseURL string
	httpClient    *http.Client
	tokenFunc     func(ctx context.Context) (string, error)
	credential    googlePlayCredentialInfo
}

func (c *googlePlayClient) credentialSummary() string {
	if c == nil {
		return ""
	}
	return c.credential.summary()
}

type googleServiceAccount struct {
	ClientEmail string `json:"client_email"`
	PrivateKey  string `json:"private_key"`
	TokenURI    string `json:"token_uri"`
}

func newGooglePlayClientFromEnv(ctx context.Context) (*googlePlayClient, error) {
	return googlePlayClientFromEnv(ctx, nil)
}

// googlePlayClientFromEnv authenticates to Google Play using NSGO_PLAY_SERVICE_ACCOUNT when set,
// or falls back to Application Default Credentials with the androidpublisher scope.
// Requests use httpClient, or a client with a 60-second timeout when it is nil.
func googlePlayClientFromEnv(ctx context.Context, httpClient *http.Client) (*googlePlayClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	if raw := strings.TrimSpace(os.Getenv(googlePlayServiceAccountEnvVar)); raw != "" {
		return newGooglePlayClientFromServiceAccount(raw, defaultGooglePlayBaseURL, httpClient)
	}
	return newGooglePlayClientFromADC(ctx, defaultGooglePlayBaseURL, httpClient)
}

func newGooglePlayClientFromADC(ctx context.Context, baseURL string, httpClient *http.Client) (*googlePlayClient, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	authCtx := context.WithValue(ctx, oauth2.HTTPClient, httpClient)
	creds, err := findGoogleDefaultCredentials(authCtx, googlePlayPublisherScope)
	if err != nil {
		return nil, formatGooglePlayMissingCredentialsError(err)
	}
	if creds == nil || creds.TokenSource == nil {
		return nil, formatGooglePlayMissingCredentialsError(errors.New("Application Default Credentials returned no token source"))
	}
	info := describeGoogleADCCredentials(ctx, creds)
	return newGooglePlayClientFromTokenSource(creds.TokenSource, info, baseURL, httpClient), nil
}

func newGooglePlayClientFromTokenSource(ts oauth2.TokenSource, cred googlePlayCredentialInfo, baseURL string, httpClient *http.Client) *googlePlayClient {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	return &googlePlayClient{
		baseURL:       baseURL,
		uploadBaseURL: baseURL,
		httpClient:    httpClient,
		credential:    cred,
		tokenFunc: func(ctx context.Context) (string, error) {
			if err := ctx.Err(); err != nil {
				return "", err
			}
			tok, err := ts.Token()
			if err != nil {
				return "", err
			}
			if tok == nil || tok.AccessToken == "" {
				return "", errors.New("Google OAuth2 returned an empty access token")
			}
			return tok.AccessToken, nil
		},
	}
}

func describeGoogleADCCredentials(ctx context.Context, creds *google.Credentials) googlePlayCredentialInfo {
	info := googlePlayCredentialInfo{Source: "Application Default Credentials"}
	if creds == nil {
		return info
	}
	if len(creds.JSON) == 0 {
		info.Type = "metadata_server"
		info.Account = googleMetadataEmail(ctx)
		return info
	}
	var meta struct {
		Type                           string `json:"type"`
		ClientEmail                    string `json:"client_email"`
		Account                        string `json:"account"`
		ServiceAccountImpersonationURL string `json:"service_account_impersonation_url"`
	}
	if err := json.Unmarshal(creds.JSON, &meta); err != nil {
		return info
	}
	info.Type = strings.TrimSpace(meta.Type)
	switch {
	case strings.TrimSpace(meta.ClientEmail) != "":
		info.Account = strings.TrimSpace(meta.ClientEmail)
	case strings.TrimSpace(meta.ServiceAccountImpersonationURL) != "":
		info.Account = impersonatedServiceAccountEmail(meta.ServiceAccountImpersonationURL)
	case strings.TrimSpace(meta.Account) != "":
		info.Account = strings.TrimSpace(meta.Account)
	}
	return info
}

// impersonatedServiceAccountEmail extracts the service account email from an IAM Credentials
// generateAccessToken URL such as https://iamcredentials.googleapis.com/v1/projects/-/serviceAccounts/<email>:generateAccessToken.
func impersonatedServiceAccountEmail(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return ""
	}
	const marker = "/serviceAccounts/"
	idx := strings.Index(u.Path, marker)
	if idx < 0 {
		return ""
	}
	rest := u.Path[idx+len(marker):]
	if slash := strings.IndexByte(rest, '/'); slash >= 0 {
		rest = rest[:slash]
	}
	if colon := strings.IndexByte(rest, ':'); colon >= 0 {
		rest = rest[:colon]
	}
	rest = strings.TrimSpace(rest)
	if !strings.Contains(rest, "@") {
		return ""
	}
	return rest
}

func formatGooglePlayMissingCredentialsError(adcErr error) error {
	options := fmt.Sprintf("set %s to a service account JSON key or file path, or configure Application Default Credentials (for example, `gcloud auth application-default login --impersonate-service-account=<service-account>`, %s, or Workload Identity Federation)", googlePlayServiceAccountEnvVar, googleAppCredentialsEnvVar)
	msg := sanitizeGoogleCredentialsError(adcErr)
	if msg == "" || (os.Getenv(googleAppCredentialsEnvVar) == "" && strings.Contains(msg, "could not find default credentials")) {
		return fmt.Errorf("Google Play credentials missing: %s", options)
	}
	return fmt.Errorf("Google Play credentials: %s (%s)", msg, options)
}

func sanitizeGoogleCredentialsError(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if gac := os.Getenv(googleAppCredentialsEnvVar); gac != "" && strings.Contains(msg, gac) && looksLikeCredentialSecret(gac) {
		msg = strings.ReplaceAll(msg, gac, "<redacted>")
	}
	return msg
}

func looksLikeCredentialSecret(s string) bool {
	trimmed := strings.TrimSpace(s)
	return strings.HasPrefix(trimmed, "{") ||
		strings.ContainsAny(trimmed, "\r\n") ||
		strings.Contains(trimmed, "PRIVATE KEY") ||
		strings.Contains(trimmed, "private_key") ||
		strings.Contains(trimmed, "refresh_token") ||
		strings.Contains(trimmed, "client_secret")
}

func newGooglePlayClientFromServiceAccount(jsonOrPath, baseURL string, httpClient *http.Client) (*googlePlayClient, error) {
	var data []byte
	if strings.HasPrefix(strings.TrimSpace(jsonOrPath), "{") {
		data = []byte(jsonOrPath)
	} else {
		var err error
		data, err = os.ReadFile(jsonOrPath)
		if err != nil {
			// The value may be a mangled key rather than a path, so leave it out of the error.
			var pathErr *os.PathError
			if errors.As(err, &pathErr) {
				err = pathErr.Err
			}
			return nil, fmt.Errorf("Google Play service account: read %s: %w", googlePlayServiceAccountEnvVar, err)
		}
	}
	var sa googleServiceAccount
	if err := json.Unmarshal(data, &sa); err != nil {
		return nil, fmt.Errorf("Google Play service account: %w", err)
	}
	if sa.ClientEmail == "" || sa.PrivateKey == "" {
		return nil, errors.New("Google Play service account JSON requires client_email and private_key")
	}
	if sa.TokenURI == "" {
		sa.TokenURI = defaultGooglePlayTokenURL
	}
	privKey, err := parseRSAPrivateKey(sa.PrivateKey)
	if err != nil {
		return nil, fmt.Errorf("Google Play service account private_key: %w", err)
	}
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 60 * time.Second}
	}
	var mu sync.Mutex
	var cachedToken string
	var cachedExpiry time.Time
	return &googlePlayClient{
		baseURL:       baseURL,
		uploadBaseURL: baseURL,
		httpClient:    httpClient,
		credential: googlePlayCredentialInfo{
			Type:    "service_account",
			Account: sa.ClientEmail,
			Source:  googlePlayServiceAccountEnvVar,
		},
		tokenFunc: func(ctx context.Context) (string, error) {
			mu.Lock()
			defer mu.Unlock()
			if cachedToken != "" && time.Now().Add(time.Minute).Before(cachedExpiry) {
				return cachedToken, nil
			}
			tok, expiresIn, err := fetchGooglePlayAccessToken(ctx, httpClient, sa.ClientEmail, sa.TokenURI, privKey, time.Now())
			if err != nil {
				return "", err
			}
			cachedToken = tok
			if expiresIn <= 0 {
				expiresIn = 3600
			}
			cachedExpiry = time.Now().Add(time.Duration(expiresIn) * time.Second)
			return cachedToken, nil
		},
	}, nil
}

func parseRSAPrivateKey(pemText string) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(pemText))
	if block == nil {
		return nil, errors.New("not a valid PEM private key")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		rsaKey, ok := k.(*rsa.PrivateKey)
		if !ok {
			return nil, errors.New("expected an RSA private key")
		}
		return rsaKey, nil
	}
	return x509.ParsePKCS1PrivateKey(block.Bytes)
}

func signGoogleServiceAccountJWT(email, tokenURI string, key *rsa.PrivateKey, now time.Time) (string, error) {
	headerBytes, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	claimsBytes, err := json.Marshal(map[string]any{
		"iss":   email,
		"scope": googlePlayPublisherScope,
		"aud":   tokenURI,
		"iat":   now.Unix(),
		"exp":   now.Add(time.Hour).Unix(),
	})
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(headerBytes) + "." + enc.EncodeToString(claimsBytes)
	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func fetchGooglePlayAccessToken(ctx context.Context, client *http.Client, email, tokenURI string, key *rsa.PrivateKey, now time.Time) (string, int, error) {
	assertion, err := signGoogleServiceAccountJWT(email, tokenURI, key, now)
	if err != nil {
		return "", 0, err
	}
	form := url.Values{
		"grant_type": {"urn:ietf:params:oauth:grant-type:jwt-bearer"},
		"assertion":  {assertion},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURI, strings.NewReader(form.Encode()))
	if err != nil {
		return "", 0, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("Google OAuth2 token exchange: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, fmt.Errorf("Google OAuth2 token exchange failed (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(raw, &tokenResp); err != nil {
		return "", 0, err
	}
	if tokenResp.AccessToken == "" {
		return "", 0, errors.New("Google OAuth2 token exchange returned empty access_token")
	}
	return tokenResp.AccessToken, tokenResp.ExpiresIn, nil
}

func (c *googlePlayClient) client() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return http.DefaultClient
}

func (c *googlePlayClient) base() string {
	if c.baseURL != "" {
		return strings.TrimRight(c.baseURL, "/")
	}
	return defaultGooglePlayBaseURL
}

func (c *googlePlayClient) uploadBase() string {
	if c.uploadBaseURL != "" {
		return strings.TrimRight(c.uploadBaseURL, "/")
	}
	return c.base()
}

func (c *googlePlayClient) authorizationHeader(ctx context.Context) (string, error) {
	if c.tokenFunc == nil {
		return "", nil
	}
	token, err := c.tokenFunc(ctx)
	if err != nil {
		if id := c.credential.identity(); id != "" {
			return "", fmt.Errorf("Google Play auth (%s): %w", id, err)
		}
		return "", fmt.Errorf("Google Play auth: %w", err)
	}
	return "Bearer " + token, nil
}

func (c *googlePlayClient) parseError(method, path string, status int, raw []byte) error {
	err := parseGooglePlayError(method, path, status, raw)
	var playErr *googlePlayError
	if errors.As(err, &playErr) {
		playErr.Credential = c.credential
	}
	return err
}

func (c *googlePlayClient) doJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	var bodyReader io.Reader
	if reqBody != nil {
		data, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base()+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if auth, err := c.authorizationHeader(ctx); err != nil {
		return err
	} else if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("Google Play %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("Google Play %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.parseError(method, path, resp.StatusCode, raw)
	}
	if respBody != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, respBody); err != nil {
			return fmt.Errorf("Google Play %s %s: decode response: %w", method, path, err)
		}
	}
	return nil
}

func (c *googlePlayClient) uploadImage(ctx context.Context, packageName, editID, language, imageType, filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/upload/androidpublisher/v3/applications/%s/edits/%s/listings/%s/%s?uploadType=media",
		url.PathEscape(packageName), url.PathEscape(editID), url.PathEscape(language), url.PathEscape(imageType))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.uploadBase()+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", imageMIMEType(filePath))
	req.Header.Set("Accept", "application/json")
	if auth, err := c.authorizationHeader(ctx); err != nil {
		return err
	} else if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("Google Play upload %s: %w", filepath.Base(filePath), err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return c.parseError(http.MethodPost, path, resp.StatusCode, raw)
	}
	return nil
}

// googlePlayError is a non-2xx Android Publisher response.
type googlePlayError struct {
	Method, Path string
	StatusCode   int
	// Status is Google's error status, else its first error reason.
	Status     string
	Message    string
	Body       string
	Credential googlePlayCredentialInfo
}

func (e *googlePlayError) Error() string {
	var msg string
	if e.Message == "" {
		msg = fmt.Sprintf("Google Play %s %s failed with HTTP %d: %s", e.Method, e.Path, e.StatusCode, e.Body)
	} else if e.Status != "" {
		msg = fmt.Sprintf("Google Play %s %s (HTTP %d, %s): %s", e.Method, e.Path, e.StatusCode, e.Status, e.Message)
	} else {
		msg = fmt.Sprintf("Google Play %s %s (HTTP %d): %s", e.Method, e.Path, e.StatusCode, e.Message)
	}
	if e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden {
		if id := e.Credential.identity(); id != "" {
			msg += fmt.Sprintf("; Google Play refused %s — grant it access to the app in Play Console › Users and permissions", id)
		} else {
			msg += "; grant the authenticated identity access to the app in Play Console › Users and permissions"
		}
	}
	return msg
}

func parseGooglePlayError(method, path string, status int, raw []byte) error {
	e := &googlePlayError{Method: method, Path: path, StatusCode: status, Body: strings.TrimSpace(string(raw))}
	var payload struct {
		Error struct {
			Message string `json:"message"`
			Status  string `json:"status"`
			Errors  []struct {
				Reason string `json:"reason"`
			} `json:"errors"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &payload) == nil && payload.Error.Message != "" {
		e.Message = payload.Error.Message
		e.Status = payload.Error.Status
		if e.Status == "" && len(payload.Error.Errors) > 0 {
			e.Status = payload.Error.Errors[0].Reason
		}
	}
	return e
}

// uploadBundle streams an App Bundle of size bytes into the edit and returns the version code Google Play read from it.
func (c *googlePlayClient) uploadBundle(ctx context.Context, packageName, editID string, bundle io.ReaderAt, size int64) (int64, error) {
	path := fmt.Sprintf("/upload/androidpublisher/v3/applications/%s/edits/%s/bundles?uploadType=media", url.PathEscape(packageName), url.PathEscape(editID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.uploadBase()+path, io.NewSectionReader(bundle, 0, size))
	if err != nil {
		return 0, err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Accept", "application/json")
	if auth, err := c.authorizationHeader(ctx); err != nil {
		return 0, err
	} else if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return 0, fmt.Errorf("Google Play upload App Bundle: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return 0, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return 0, c.parseError(http.MethodPost, path, resp.StatusCode, raw)
	}
	var uploaded struct {
		VersionCode int64 `json:"versionCode"`
	}
	if err := json.Unmarshal(raw, &uploaded); err != nil {
		return 0, fmt.Errorf("Google Play upload App Bundle: decode response: %w", err)
	}
	return uploaded.VersionCode, nil
}

func imageMIMEType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	default:
		return "image/png"
	}
}

// uploadGooglePlayListings creates an edit, uploads all localized listings,
// screenshots, graphics and optional track release notes, validates, and commits.
func uploadGooglePlayListings(ctx context.Context, listings *storeListings, client *googlePlayClient) error {
	if client == nil {
		var err error
		client, err = newGooglePlayClientFromEnv(ctx)
		if err != nil {
			return err
		}
	}
	if cred := client.credentialSummary(); cred != "" {
		note("Google Play auth: %s", cred)
	}
	pkg := listings.Manifest.GooglePlay.PackageName
	pkgEsc := url.PathEscape(pkg)

	// 1. Open edit.
	var edit struct {
		ID string `json:"id"`
	}
	if err := client.doJSON(ctx, http.MethodPost, fmt.Sprintf("/androidpublisher/v3/applications/%s/edits", pkgEsc), map[string]any{}, &edit); err != nil {
		return err
	}
	if edit.ID == "" {
		return errors.New("Google Play: edits.insert returned an empty edit id")
	}
	editBase := fmt.Sprintf("/androidpublisher/v3/applications/%s/edits/%s", pkgEsc, url.PathEscape(edit.ID))

	// 2. Update app details if configured.
	details := map[string]any{}
	defaultLang := strings.TrimSpace(listings.Manifest.GooglePlay.DefaultLanguage)
	if defaultLang == "" && listings.Manifest.DefaultLocale != "" {
		for _, loc := range listings.Locales {
			if loc.Locale == listings.Manifest.DefaultLocale {
				defaultLang = loc.GooglePlay.Locale
				break
			}
		}
		if defaultLang == "" {
			defaultLang = mapToGooglePlayLocale(listings.Manifest.DefaultLocale)
		}
	} else if defaultLang != "" {
		defaultLang = mapToGooglePlayLocale(defaultLang)
	}
	if defaultLang != "" {
		details["defaultLanguage"] = defaultLang
	}
	if listings.Manifest.GooglePlay.ContactEmail != "" {
		details["contactEmail"] = listings.Manifest.GooglePlay.ContactEmail
	}
	if listings.Manifest.GooglePlay.ContactWebsite != "" {
		details["contactWebsite"] = listings.Manifest.GooglePlay.ContactWebsite
	}
	if listings.Manifest.GooglePlay.ContactPhone != "" {
		details["contactPhone"] = listings.Manifest.GooglePlay.ContactPhone
	}
	if len(details) > 0 {
		if err := client.doJSON(ctx, http.MethodPatch, editBase+"/details", details, nil); err != nil {
			return err
		}
	}

	// 3. Update localized store listings and upload images.
	type playReleaseNote struct {
		Language string `json:"language"`
		Text     string `json:"text"`
	}
	var releaseNotes []playReleaseNote

	for _, loc := range listings.Locales {
		g := loc.GooglePlay
		langEsc := url.PathEscape(g.Locale)
		body := map[string]any{
			"language":         g.Locale,
			"title":            g.Title,
			"shortDescription": g.ShortDescription,
			"fullDescription":  g.Description,
		}
		if g.VideoURL != "" {
			body["video"] = g.VideoURL
		}
		if err := client.doJSON(ctx, http.MethodPut, editBase+"/listings/"+langEsc, body, nil); err != nil {
			return err
		}

		for _, imgType := range sortedMapKeys(g.Screenshots) {
			files := g.Screenshots[imgType]
			if len(files) == 0 {
				continue
			}
			imgTypeEsc := url.PathEscape(imgType)
			if err := client.doJSON(ctx, http.MethodDelete, fmt.Sprintf("%s/listings/%s/%s", editBase, langEsc, imgTypeEsc), nil, nil); err != nil {
				return err
			}
			for _, f := range files {
				if err := client.uploadImage(ctx, pkg, edit.ID, g.Locale, imgType, f); err != nil {
					return err
				}
			}
		}
		if g.FeatureGraphic != "" {
			if err := client.doJSON(ctx, http.MethodDelete, fmt.Sprintf("%s/listings/%s/featureGraphic", editBase, langEsc), nil, nil); err != nil {
				return err
			}
			if err := client.uploadImage(ctx, pkg, edit.ID, g.Locale, "featureGraphic", g.FeatureGraphic); err != nil {
				return err
			}
		}
		if g.Icon != "" {
			if err := client.doJSON(ctx, http.MethodDelete, fmt.Sprintf("%s/listings/%s/icon", editBase, langEsc), nil, nil); err != nil {
				return err
			}
			if err := client.uploadImage(ctx, pkg, edit.ID, g.Locale, "icon", g.Icon); err != nil {
				return err
			}
		}
		if g.ReleaseNotes != "" {
			releaseNotes = append(releaseNotes, playReleaseNote{
				Language: g.Locale,
				Text:     g.ReleaseNotes,
			})
		}
	}

	// 4. Update track release notes when a track is named and notes are present.
	if track := strings.TrimSpace(listings.Manifest.GooglePlay.Track); track != "" && len(releaseNotes) > 0 {
		trackPath := editBase + "/tracks/" + url.PathEscape(track)
		var trackResp struct {
			Track    string           `json:"track"`
			Releases []map[string]any `json:"releases"`
		}
		if err := client.doJSON(ctx, http.MethodGet, trackPath, nil, &trackResp); err != nil {
			return err
		}
		if trackResp.Track == "" {
			trackResp.Track = track
		}
		if len(trackResp.Releases) == 0 {
			trackResp.Releases = []map[string]any{{"status": "draft"}}
		}
		trackResp.Releases[0]["releaseNotes"] = releaseNotes
		if err := client.doJSON(ctx, http.MethodPut, trackPath, trackResp, nil); err != nil {
			return err
		}
	}

	// 5. Validate and commit edit.
	if err := client.doJSON(ctx, http.MethodPost, editBase+":validate", nil, nil); err != nil {
		return err
	}
	return client.doJSON(ctx, http.MethodPost, editBase+":commit", nil, nil)
}
