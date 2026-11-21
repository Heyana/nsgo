package main

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
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
	"time"
)

const defaultAppStoreConnectBaseURL = "https://api.appstoreconnect.apple.com"

var (
	appStoreKeyEnv = [3]string{"NSGO_APP_STORE_KEY", "NSGO_APP_STORE_KEY_ID", "NSGO_APP_STORE_ISSUER"}
)

type appStoreClient struct {
	baseURL    string
	httpClient *http.Client
	tokenFunc  func(ctx context.Context) (string, error)
}

func newAppStoreClientFromEnv() (*appStoreClient, error) {
	key, keyID, issuer, err := appStoreCredentialsFromEnv()
	if err != nil {
		return nil, err
	}
	privKey, err := loadECDSAPrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("app store connect key: %w", err)
	}
	return &appStoreClient{
		baseURL:    defaultAppStoreConnectBaseURL,
		httpClient: &http.Client{Timeout: 60 * time.Second},
		tokenFunc: func(context.Context) (string, error) {
			return signAppStoreJWT(privKey, keyID, issuer, time.Now())
		},
	}, nil
}

func appStoreCredentialsFromEnv() (key, keyID, issuer string, err error) {
	if v := strings.TrimSpace(os.Getenv(appStoreKeyEnv[0])); v != "" {
		keyID = strings.TrimSpace(os.Getenv(appStoreKeyEnv[1]))
		issuer = strings.TrimSpace(os.Getenv(appStoreKeyEnv[2]))
		if keyID == "" || issuer == "" {
			return "", "", "", fmt.Errorf("%s requires %s and %s", appStoreKeyEnv[0], appStoreKeyEnv[1], appStoreKeyEnv[2])
		}
		return v, keyID, issuer, nil
	}
	if v := strings.TrimSpace(os.Getenv(notaryKeyEnv[0])); v != "" {
		keyID = strings.TrimSpace(os.Getenv(notaryKeyEnv[1]))
		issuer = strings.TrimSpace(os.Getenv(notaryKeyEnv[2]))
		if keyID == "" || issuer == "" {
			return "", "", "", fmt.Errorf("%s requires %s and %s", notaryKeyEnv[0], notaryKeyEnv[1], notaryKeyEnv[2])
		}
		return v, keyID, issuer, nil
	}
	return "", "", "", fmt.Errorf("App Store Connect credentials missing: set %s, %s and %s (or %s, %s and %s)",
		appStoreKeyEnv[0], appStoreKeyEnv[1], appStoreKeyEnv[2],
		notaryKeyEnv[0], notaryKeyEnv[1], notaryKeyEnv[2])
}

func loadECDSAPrivateKey(keyOrPath string) (*ecdsa.PrivateKey, error) {
	var pemBytes []byte
	if strings.Contains(keyOrPath, "-----BEGIN") {
		pemBytes = []byte(keyOrPath)
	} else {
		var err error
		pemBytes, err = os.ReadFile(keyOrPath)
		if err != nil {
			return nil, err
		}
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		return nil, errors.New("not a valid PEM private key")
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		ec, ok := k.(*ecdsa.PrivateKey)
		if !ok {
			return nil, errors.New("expected an ECDSA P-256 private key")
		}
		if ec.Curve != elliptic.P256() {
			return nil, errors.New("expected an ECDSA P-256 private key")
		}
		return ec, nil
	}
	ec, err := x509.ParseECPrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	if ec.Curve != elliptic.P256() {
		return nil, errors.New("expected an ECDSA P-256 private key")
	}
	return ec, nil
}

// signAppStoreJWT signs an ES256 JWT for App Store Connect API v1.
func signAppStoreJWT(key *ecdsa.PrivateKey, keyID, issuer string, now time.Time) (string, error) {
	headerBytes, err := json.Marshal(map[string]string{
		"alg": "ES256",
		"kid": keyID,
		"typ": "JWT",
	})
	if err != nil {
		return "", err
	}
	claimsBytes, err := json.Marshal(map[string]any{
		"iss": issuer,
		"iat": now.Unix(),
		"exp": now.Add(15 * time.Minute).Unix(),
		"aud": "appstoreconnect-v1",
	})
	if err != nil {
		return "", err
	}
	enc := base64.RawURLEncoding
	signingInput := enc.EncodeToString(headerBytes) + "." + enc.EncodeToString(claimsBytes)
	digest := sha256.Sum256([]byte(signingInput))
	r, s, err := ecdsa.Sign(rand.Reader, key, digest[:])
	if err != nil {
		return "", err
	}
	sig := make([]byte, 64)
	r.FillBytes(sig[:32])
	s.FillBytes(sig[32:])
	return signingInput + "." + enc.EncodeToString(sig), nil
}

func (c *appStoreClient) client() *http.Client {
	if c.httpClient != nil {
		return c.httpClient
	}
	return http.DefaultClient
}

func (c *appStoreClient) base() string {
	if c.baseURL != "" {
		return strings.TrimRight(c.baseURL, "/")
	}
	return defaultAppStoreConnectBaseURL
}

func (c *appStoreClient) doJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
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
	if c.tokenFunc != nil {
		token, err := c.tokenFunc(ctx)
		if err != nil {
			return fmt.Errorf("App Store Connect auth: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := c.client().Do(req)
	if err != nil {
		return fmt.Errorf("App Store Connect %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("App Store Connect %s %s: %w", method, path, err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseAppStoreError(method, path, resp.StatusCode, raw)
	}
	if respBody != nil && len(bytes.TrimSpace(raw)) > 0 {
		if err := json.Unmarshal(raw, respBody); err != nil {
			return fmt.Errorf("App Store Connect %s %s: decode response: %w", method, path, err)
		}
	}
	return nil
}

func parseAppStoreError(method, path string, status int, raw []byte) error {
	what := fmt.Sprintf("App Store Connect %s %s (HTTP %d)", method, path, status)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		what = fmt.Sprintf("App Store Connect refused the API key for %s %s (HTTP %d), check the key ID, issuer and the key's access to the app", method, path, status)
	}
	var payload struct {
		Errors []struct {
			Status string `json:"status"`
			Code   string `json:"code"`
			Title  string `json:"title"`
			Detail string `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &payload); err == nil && len(payload.Errors) > 0 {
		parts := make([]string, 0, len(payload.Errors))
		for _, e := range payload.Errors {
			msg := strings.TrimSpace(strings.Join([]string{e.Code, e.Title, e.Detail}, ": "))
			msg = strings.Trim(msg, ": ")
			if msg != "" {
				parts = append(parts, msg)
			}
		}
		if len(parts) > 0 {
			return fmt.Errorf("%s: %s", what, strings.Join(parts, "; "))
		}
	}
	if body := strings.TrimSpace(string(raw)); body != "" {
		return fmt.Errorf("%s: %s", what, body)
	}
	return errors.New(what)
}

type ascResource struct {
	Type       string         `json:"type"`
	ID         string         `json:"id,omitempty"`
	Attributes map[string]any `json:"attributes,omitempty"`
}

type ascListResponse struct {
	Data []ascResource `json:"data"`
}

type ascSingleResponse struct {
	Data ascResource `json:"data"`
}

type ascUploadOperation struct {
	Method         string `json:"method"`
	URL            string `json:"url"`
	Length         int64  `json:"length"`
	Offset         int64  `json:"offset"`
	RequestHeaders []struct {
		Name  string `json:"name"`
		Value string `json:"value"`
	} `json:"requestHeaders"`
}

// uploadAppStoreListings uploads all localized metadata, screenshots and previews
// to App Store Connect API v1.
func uploadAppStoreListings(ctx context.Context, listings *storeListings, client *appStoreClient) error {
	if client == nil {
		var err error
		client, err = newAppStoreClientFromEnv()
		if err != nil {
			return err
		}
	}
	platform, ok := normalizeAppStorePlatform(listings.Manifest.AppStore.Platform)
	if !ok {
		return fmt.Errorf("unsupported App Store platform %q", listings.Manifest.AppStore.Platform)
	}

	appID := strings.TrimSpace(listings.Manifest.AppStore.AppID)
	if appID == "" {
		var apps ascListResponse
		q := "/v1/apps?filter[bundleId]=" + url.QueryEscape(listings.Manifest.AppStore.BundleID)
		if err := client.doJSON(ctx, http.MethodGet, q, nil, &apps); err != nil {
			return err
		}
		if len(apps.Data) == 0 {
			return fmt.Errorf("App Store Connect: no app found for bundle ID %q", listings.Manifest.AppStore.BundleID)
		}
		appID = apps.Data[0].ID
	}

	// 1. Update appInfo categories (when configured) and appInfoLocalizations (name, subtitle, privacyPolicyUrl).
	appInfoID, err := client.resolveAppInfoID(ctx, appID)
	if err != nil {
		return err
	}
	if err := client.updateAppInfoCategories(ctx, appInfoID, listings.Manifest.AppStore.PrimaryCategory, listings.Manifest.AppStore.SecondaryCategory); err != nil {
		return err
	}
	existingInfoLocs, err := client.listLocalizations(ctx, fmt.Sprintf("/v1/appInfos/%s/appInfoLocalizations", url.PathEscape(appInfoID)))
	if err != nil {
		return err
	}
	for _, loc := range listings.Locales {
		a := loc.AppStore
		attrs := map[string]any{
			"name": a.Title,
		}
		if a.Subtitle != "" {
			attrs["subtitle"] = a.Subtitle
		}
		if a.PrivacyPolicyURL != "" {
			attrs["privacyPolicyUrl"] = a.PrivacyPolicyURL
		}
		if locID, ok := existingInfoLocs[a.Locale]; ok {
			body := map[string]any{
				"data": map[string]any{
					"type":       "appInfoLocalizations",
					"id":         locID,
					"attributes": attrs,
				},
			}
			if err := client.doJSON(ctx, http.MethodPatch, "/v1/appInfoLocalizations/"+url.PathEscape(locID), body, nil); err != nil {
				return err
			}
		} else {
			attrs["locale"] = a.Locale
			body := map[string]any{
				"data": map[string]any{
					"type":       "appInfoLocalizations",
					"attributes": attrs,
					"relationships": map[string]any{
						"appInfo": map[string]any{
							"data": map[string]string{"type": "appInfos", "id": appInfoID},
						},
					},
				},
			}
			if err := client.doJSON(ctx, http.MethodPost, "/v1/appInfoLocalizations", body, nil); err != nil {
				return err
			}
		}
	}

	// 2. Resolve or create appStoreVersion.
	versionID, err := client.resolveOrCreateVersion(ctx, appID, platform, listings.Manifest.AppStore.Version, listings.Manifest.AppStore.Copyright)
	if err != nil {
		return err
	}

	// 3. Update appStoreVersionLocalizations (description, keywords, whatsNew, promotionalText, supportUrl, marketingUrl).
	existingVerLocs, err := client.listLocalizations(ctx, fmt.Sprintf("/v1/appStoreVersions/%s/appStoreVersionLocalizations", url.PathEscape(versionID)))
	if err != nil {
		return err
	}
	for _, loc := range listings.Locales {
		a := loc.AppStore
		attrs := map[string]any{
			"description": a.Description,
			"supportUrl":  a.SupportURL,
		}
		if a.Keywords != "" {
			attrs["keywords"] = a.Keywords
		}
		if a.ReleaseNotes != "" {
			attrs["whatsNew"] = a.ReleaseNotes
		}
		if a.PromotionalText != "" {
			attrs["promotionalText"] = a.PromotionalText
		}
		if a.MarketingURL != "" {
			attrs["marketingUrl"] = a.MarketingURL
		}

		var verLocID string
		if id, ok := existingVerLocs[a.Locale]; ok {
			verLocID = id
			body := map[string]any{
				"data": map[string]any{
					"type":       "appStoreVersionLocalizations",
					"id":         verLocID,
					"attributes": attrs,
				},
			}
			if err := client.doJSON(ctx, http.MethodPatch, "/v1/appStoreVersionLocalizations/"+url.PathEscape(verLocID), body, nil); err != nil {
				return err
			}
		} else {
			attrs["locale"] = a.Locale
			body := map[string]any{
				"data": map[string]any{
					"type":       "appStoreVersionLocalizations",
					"attributes": attrs,
					"relationships": map[string]any{
						"appStoreVersion": map[string]any{
							"data": map[string]string{"type": "appStoreVersions", "id": versionID},
						},
					},
				},
			}
			var created ascSingleResponse
			if err := client.doJSON(ctx, http.MethodPost, "/v1/appStoreVersionLocalizations", body, &created); err != nil {
				return err
			}
			verLocID = created.Data.ID
		}

		// 4. Upload screenshots for this localization.
		if len(a.Screenshots) > 0 {
			if err := client.uploadScreenshots(ctx, verLocID, a.Screenshots); err != nil {
				return fmt.Errorf("%s screenshots: %w", a.Locale, err)
			}
		}
		// 5. Upload app previews for this localization.
		if len(a.Previews) > 0 {
			if err := client.uploadPreviews(ctx, verLocID, a.Previews); err != nil {
				return fmt.Errorf("%s previews: %w", a.Locale, err)
			}
		}
	}
	return nil
}

func (c *appStoreClient) resolveAppInfoID(ctx context.Context, appID string) (string, error) {
	var resp ascListResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/apps/%s/appInfos", url.PathEscape(appID)), nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", fmt.Errorf("App Store Connect: app %s has no appInfos", appID)
	}
	for _, info := range resp.Data {
		if state, _ := info.Attributes["appStoreState"].(string); state == "PREPARE_FOR_SUBMISSION" {
			return info.ID, nil
		}
	}
	return resp.Data[0].ID, nil
}

func (c *appStoreClient) updateAppInfoCategories(ctx context.Context, appInfoID, primary, secondary string) error {
	primary = strings.TrimSpace(primary)
	secondary = strings.TrimSpace(secondary)
	if primary == "" && secondary == "" {
		return nil
	}
	rels := map[string]any{}
	if primary != "" {
		rels["primaryCategory"] = map[string]any{
			"data": map[string]string{"type": "appCategories", "id": primary},
		}
	}
	if secondary != "" {
		rels["secondaryCategory"] = map[string]any{
			"data": map[string]string{"type": "appCategories", "id": secondary},
		}
	}
	body := map[string]any{
		"data": map[string]any{
			"type":          "appInfos",
			"id":            appInfoID,
			"relationships": rels,
		},
	}
	return c.doJSON(ctx, http.MethodPatch, "/v1/appInfos/"+url.PathEscape(appInfoID), body, nil)
}

func (c *appStoreClient) listLocalizations(ctx context.Context, path string) (map[string]string, error) {
	var resp ascListResponse
	if err := c.doJSON(ctx, http.MethodGet, path, nil, &resp); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(resp.Data))
	for _, item := range resp.Data {
		if locale, _ := item.Attributes["locale"].(string); locale != "" {
			out[locale] = item.ID
		}
	}
	return out, nil
}

func (c *appStoreClient) resolveOrCreateVersion(ctx context.Context, appID, platform, version, copyright string) (string, error) {
	q := fmt.Sprintf("/v1/apps/%s/appStoreVersions?filter[platform]=%s&filter[versionString]=%s",
		url.PathEscape(appID), url.QueryEscape(platform), url.QueryEscape(version))
	var resp ascListResponse
	if err := c.doJSON(ctx, http.MethodGet, q, nil, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) > 0 {
		id := resp.Data[0].ID
		if copyright != "" {
			body := map[string]any{
				"data": map[string]any{
					"type":       "appStoreVersions",
					"id":         id,
					"attributes": map[string]any{"copyright": copyright},
				},
			}
			if err := c.doJSON(ctx, http.MethodPatch, "/v1/appStoreVersions/"+url.PathEscape(id), body, nil); err != nil {
				return "", err
			}
		}
		return id, nil
	}
	attrs := map[string]any{
		"platform":      platform,
		"versionString": version,
	}
	if copyright != "" {
		attrs["copyright"] = copyright
	}
	body := map[string]any{
		"data": map[string]any{
			"type":       "appStoreVersions",
			"attributes": attrs,
			"relationships": map[string]any{
				"app": map[string]any{
					"data": map[string]string{"type": "apps", "id": appID},
				},
			},
		},
	}
	var created ascSingleResponse
	if err := c.doJSON(ctx, http.MethodPost, "/v1/appStoreVersions", body, &created); err != nil {
		return "", err
	}
	return created.Data.ID, nil
}

func (c *appStoreClient) uploadScreenshots(ctx context.Context, verLocID string, screenshots map[string][]string) error {
	var setsResp ascListResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/appStoreVersionLocalizations/%s/appScreenshotSets", url.PathEscape(verLocID)), nil, &setsResp); err != nil {
		return err
	}
	sets := map[string]string{}
	for _, s := range setsResp.Data {
		if dt, _ := s.Attributes["screenshotDisplayType"].(string); dt != "" {
			sets[dt] = s.ID
		}
	}
	for _, displayType := range sortedMapKeys(screenshots) {
		files := screenshots[displayType]
		if len(files) == 0 {
			continue
		}
		setID, ok := sets[displayType]
		if !ok {
			body := map[string]any{
				"data": map[string]any{
					"type": "appScreenshotSets",
					"attributes": map[string]any{
						"screenshotDisplayType": displayType,
					},
					"relationships": map[string]any{
						"appStoreVersionLocalization": map[string]any{
							"data": map[string]string{"type": "appStoreVersionLocalizations", "id": verLocID},
						},
					},
				},
			}
			var created ascSingleResponse
			if err := c.doJSON(ctx, http.MethodPost, "/v1/appScreenshotSets", body, &created); err != nil {
				return err
			}
			setID = created.Data.ID
		} else {
			if err := c.deleteExistingMedia(ctx, fmt.Sprintf("/v1/appScreenshotSets/%s/appScreenshots", url.PathEscape(setID)), "/v1/appScreenshots"); err != nil {
				return err
			}
		}
		for _, filePath := range files {
			if err := c.uploadMediaAsset(ctx, "appScreenshots", "/v1/appScreenshots", "appScreenshotSet", "appScreenshotSets", setID, filePath, ""); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *appStoreClient) uploadPreviews(ctx context.Context, verLocID string, previews map[string][]string) error {
	var setsResp ascListResponse
	if err := c.doJSON(ctx, http.MethodGet, fmt.Sprintf("/v1/appStoreVersionLocalizations/%s/appPreviewSets", url.PathEscape(verLocID)), nil, &setsResp); err != nil {
		return err
	}
	sets := map[string]string{}
	for _, s := range setsResp.Data {
		if pt, _ := s.Attributes["previewType"].(string); pt != "" {
			sets[pt] = s.ID
		}
	}
	for _, previewType := range sortedMapKeys(previews) {
		files := previews[previewType]
		if len(files) == 0 {
			continue
		}
		setID, ok := sets[previewType]
		if !ok {
			body := map[string]any{
				"data": map[string]any{
					"type": "appPreviewSets",
					"attributes": map[string]any{
						"previewType": previewType,
					},
					"relationships": map[string]any{
						"appStoreVersionLocalization": map[string]any{
							"data": map[string]string{"type": "appStoreVersionLocalizations", "id": verLocID},
						},
					},
				},
			}
			var created ascSingleResponse
			if err := c.doJSON(ctx, http.MethodPost, "/v1/appPreviewSets", body, &created); err != nil {
				return err
			}
			setID = created.Data.ID
		} else {
			if err := c.deleteExistingMedia(ctx, fmt.Sprintf("/v1/appPreviewSets/%s/appPreviews", url.PathEscape(setID)), "/v1/appPreviews"); err != nil {
				return err
			}
		}
		for _, filePath := range files {
			mimeType := previewMIMEType(filePath)
			if err := c.uploadMediaAsset(ctx, "appPreviews", "/v1/appPreviews", "appPreviewSet", "appPreviewSets", setID, filePath, mimeType); err != nil {
				return err
			}
		}
	}
	return nil
}

func (c *appStoreClient) deleteExistingMedia(ctx context.Context, listPath, itemBasePath string) error {
	var existing ascListResponse
	if err := c.doJSON(ctx, http.MethodGet, listPath, nil, &existing); err != nil {
		return err
	}
	for _, item := range existing.Data {
		if item.ID == "" {
			continue
		}
		if err := c.doJSON(ctx, http.MethodDelete, itemBasePath+"/"+url.PathEscape(item.ID), nil, nil); err != nil {
			return err
		}
	}
	return nil
}

func (c *appStoreClient) uploadMediaAsset(ctx context.Context, resType, endpoint, relName, relType, setID, filePath, mimeType string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	attrs := map[string]any{
		"fileName": filepath.Base(filePath),
		"fileSize": len(data),
	}
	if mimeType != "" {
		attrs["mimeType"] = mimeType
	}
	reserveBody := map[string]any{
		"data": map[string]any{
			"type":       resType,
			"attributes": attrs,
			"relationships": map[string]any{
				relName: map[string]any{
					"data": map[string]string{"type": relType, "id": setID},
				},
			},
		},
	}
	var reserved struct {
		Data struct {
			ID         string `json:"id"`
			Attributes struct {
				UploadOperations []ascUploadOperation `json:"uploadOperations"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := c.doJSON(ctx, http.MethodPost, endpoint, reserveBody, &reserved); err != nil {
		return err
	}
	for _, op := range reserved.Data.Attributes.UploadOperations {
		method := op.Method
		if method == "" {
			method = http.MethodPut
		}
		end := op.Offset + op.Length
		if op.Offset < 0 || op.Length < 0 || end > int64(len(data)) {
			return fmt.Errorf("App Store Connect returned out-of-range upload chunk [%d:%d] for %s (%d bytes)", op.Offset, end, filepath.Base(filePath), len(data))
		}
		chunk := data[op.Offset:end]
		req, err := http.NewRequestWithContext(ctx, method, op.URL, bytes.NewReader(chunk))
		if err != nil {
			return err
		}
		for _, h := range op.RequestHeaders {
			req.Header.Set(h.Name, h.Value)
		}
		resp, err := c.client().Do(req)
		if err != nil {
			return fmt.Errorf("upload %s: %w", filepath.Base(filePath), err)
		}
		respBytes, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("upload %s failed with HTTP %d: %s", filepath.Base(filePath), resp.StatusCode, strings.TrimSpace(string(respBytes)))
		}
	}

	sum := md5.Sum(data)
	commitBody := map[string]any{
		"data": map[string]any{
			"type": resType,
			"id":   reserved.Data.ID,
			"attributes": map[string]any{
				"uploaded":           true,
				"sourceFileChecksum": hex.EncodeToString(sum[:]),
			},
		},
	}
	return c.doJSON(ctx, http.MethodPatch, endpoint+"/"+url.PathEscape(reserved.Data.ID), commitBody, nil)
}

func previewMIMEType(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mov":
		return "video/quicktime"
	case ".m4v":
		return "video/x-m4v"
	default:
		return "video/mp4"
	}
}
