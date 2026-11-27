package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

const (
	testPlayApp    = "dev.example.play"
	testPlayEdits  = "/androidpublisher/v3/applications/" + testPlayApp + "/edits"
	testPlayEdit   = testPlayEdits + "/edit1"
	testPlayUpload = "/upload" + testPlayEdit + "/bundles?uploadType=media"
	testPlayTrack  = testPlayEdit + "/tracks/internal"
)

func TestPlayPublishCommitsTheRequestedTrack(t *testing.T) {
	for _, tc := range []struct{ flag, id string }{
		{"internal", "internal"},
		{"internal", "qa"},
		{"closed-testers", "closed-testers"},
		{"production", "production"},
	} {
		t.Run(tc.flag+"/"+tc.id, func(t *testing.T) {
			const bundle = "example aab data"
			track := testPlayEdit + "/tracks/" + tc.id
			steps := []playExchange{{http.MethodPost, testPlayEdits, `{}`, `{"id":"edit1"}`, 0}}
			if tc.flag != tc.id {
				steps = append(steps, playExchange{http.MethodGet, testPlayTrack, "", `{"error":{"message":"track not found"}}`, http.StatusNotFound})
			}
			steps = append(steps,
				playExchange{http.MethodGet, track, "", `{"track":"` + tc.id + `","releases":[{"versionCodes":["18","9"]},{"versionCodes":["17"]}]}`, 0},
				playExchange{http.MethodPost, testPlayUpload, bundle, `{"versionCode":19}`, 0},
				playExchange{http.MethodPut, track, `{"track":"` + tc.id + `","releases":[{"versionCodes":["19"],"status":"completed"}]}`, `{"track":"` + tc.id + `"}`, 0},
				playExchange{http.MethodPost, testPlayEdit + ":commit", "", `{"id":"edit1"}`, 0},
			)
			c := mockPlay(t, steps)
			app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
			if err := uploadGooglePlayBundle(t.Context(), c, tc.flag, app, strings.NewReader(bundle), int64(len(bundle))); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPlayPublishDiscardsFailedEdits(t *testing.T) {
	const bundle = "bundle"
	const noReleases = `{"track":"internal"}`
	for _, tc := range []struct {
		name, want string
		steps      []playExchange
	}{
		{"equal version", "highest version code 20", []playExchange{{http.MethodGet, testPlayTrack, "", `{"releases":[{"versionCodes":["12","20"]},{"versionCodes":["15"]}]}`, 0}}},
		{"older version", "highest version code 21", []playExchange{{http.MethodGet, testPlayTrack, "", `{"releases":[{"versionCodes":["21"]}]}`, 0}}},
		{"wrong track", `different track "production"`, []playExchange{{http.MethodGet, testPlayTrack, "", `{"track":"production"}`, 0}}},
		{"track API error", "PERMISSION_DENIED): app access denied", []playExchange{{http.MethodGet, testPlayTrack, "", `{"error":{"status":"PERMISSION_DENIED","message":"app access denied"}}`, 403}}},
		{"upload API error", "versionCodeAlreadyUsed): version exists", []playExchange{
			{http.MethodGet, testPlayTrack, "", noReleases, 0},
			{http.MethodPost, testPlayUpload, bundle, `{"error":{"errors":[{"reason":"versionCodeAlreadyUsed"}],"message":"version exists"}}`, 403},
		}},
		{"wrong uploaded version", "returned version code 31 for bundle version code 20", []playExchange{
			{http.MethodGet, testPlayTrack, "", noReleases, 0},
			{http.MethodPost, testPlayUpload, bundle, `{"versionCode":31}`, 0},
		}},
		{"commit API error", "commit Google Play edit: Google Play POST " + testPlayEdit + ":commit (HTTP 409, ABORTED): edit changed", []playExchange{
			{http.MethodGet, testPlayTrack, "", noReleases, 0},
			{http.MethodPost, testPlayUpload, bundle, `{"versionCode":20}`, 0},
			{http.MethodPut, testPlayTrack, `{"track":"internal","releases":[{"versionCodes":["20"],"status":"completed"}]}`, noReleases, 0},
			{http.MethodPost, testPlayEdit + ":commit", "", `{"error":{"status":"ABORTED","message":"edit changed"}}`, 409},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := append([]playExchange{{http.MethodPost, testPlayEdits, `{}`, `{"id":"edit1"}`, 0}}, tc.steps...)
			steps = append(steps, playExchange{http.MethodDelete, testPlayEdit, "", "", http.StatusNoContent})
			c := mockPlay(t, steps)
			app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 20}
			err := uploadGooglePlayBundle(t.Context(), c, "internal", app, strings.NewReader(bundle), int64(len(bundle)))
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("upload returned %v, want %q", err, tc.want)
			}
		})
	}
}

type playExchange struct {
	method, path, body, response string
	status                       int
}

func mockPlay(t *testing.T, steps []playExchange) *googlePlayClient {
	t.Helper()
	var count atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		i := int(count.Add(1)) - 1
		if i >= len(steps) {
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		want := steps[i]
		if r.Method != want.method || r.URL.RequestURI() != want.path || r.Header.Get("Authorization") != "Bearer test-access-token" {
			t.Errorf("request %d = %s %s, want authorized %s %s", i, r.Method, r.URL, want.method, want.path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil || string(body) != want.body {
			t.Errorf("request %d body = %q, %v; want %q", i, body, err, want.body)
		}
		if want.path == testPlayUpload && (r.ContentLength != int64(len(want.body)) || r.Header.Get("Content-Type") != "application/octet-stream") {
			t.Errorf("bundle upload did not send its binary length and content type")
		}
		if want.status != 0 {
			w.WriteHeader(want.status)
		}
		io.WriteString(w, want.response)
	}))
	t.Cleanup(func() {
		server.Close()
		if got := int(count.Load()); got != len(steps) {
			t.Errorf("Google Play received %d requests, want %d", got, len(steps))
		}
	})
	return &googlePlayClient{baseURL: server.URL, httpClient: server.Client(), tokenFunc: func(context.Context) (string, error) { return "test-access-token", nil }}
}

func TestPlayCredentialsLeaveTheirValueOutOfErrors(t *testing.T) {
	// A key pasted without its JSON braces is read as a path; the error must not echo it.
	const secret = "-----BEGIN PRIVATE KEY-----secret-material"
	t.Setenv(googlePlayServiceAccountEnvVar, secret)
	_, err := googlePlayClientFromEnv(t.Context(), nil)
	if err == nil || strings.Contains(err.Error(), "secret-material") || !strings.Contains(err.Error(), googlePlayServiceAccountEnvVar) {
		t.Fatalf("credential error = %v", err)
	}

	// Secret material mistakenly placed in GOOGLE_APPLICATION_CREDENTIALS must also stay out of errors.
	t.Setenv(googlePlayServiceAccountEnvVar, "")
	t.Setenv(googleAppCredentialsEnvVar, secret)
	_, err = googlePlayClientFromEnv(t.Context(), nil)
	if err == nil || strings.Contains(err.Error(), "secret-material") || !strings.Contains(err.Error(), googlePlayServiceAccountEnvVar) || !strings.Contains(err.Error(), "Application Default Credentials") {
		t.Fatalf("ADC credential error = %v", err)
	}
}

func testPlayServiceAccountJSON(t *testing.T, email, tokenURI string) ([]byte, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	data, err := json.Marshal(map[string]string{
		"type":         "service_account",
		"client_email": email,
		"private_key":  pemText,
		"token_uri":    tokenURI,
	})
	if err != nil {
		t.Fatal(err)
	}
	return data, pemText
}

func isolatePlayADCEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv(googlePlayServiceAccountEnvVar, "")
	t.Setenv(googleAppCredentialsEnvVar, "")
	t.Setenv("HOME", home)
	t.Setenv("APPDATA", home)
	return home
}

func newPlayUploadMockServer(t *testing.T, wantToken string) *httptest.Server {
	t.Helper()
	var committed atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer "+wantToken {
			t.Errorf("Play %s %s Authorization = %q, want %q", r.Method, r.URL.Path, got, "Bearer "+wantToken)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"status":"UNAUTHENTICATED","message":"bad token"}}`)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == testPlayEdits:
			_, _ = io.WriteString(w, `{"id":"edit1"}`)
		case r.Method == http.MethodGet && r.URL.Path == testPlayTrack:
			_, _ = io.WriteString(w, `{"track":"internal","releases":[{"versionCodes":["18"]}]}`)
		case r.Method == http.MethodPost && r.URL.RequestURI() == testPlayUpload:
			_, _ = io.WriteString(w, `{"versionCode":19}`)
		case r.Method == http.MethodPut && r.URL.Path == testPlayTrack:
			_, _ = io.WriteString(w, `{"track":"internal"}`)
		case r.Method == http.MethodPost && r.URL.Path == testPlayEdit+":commit":
			committed.Store(true)
			_, _ = io.WriteString(w, `{"id":"edit1"}`)
		default:
			t.Errorf("unexpected Play request %s %s", r.Method, r.URL.RequestURI())
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(func() {
		srv.Close()
		if !committed.Load() {
			t.Errorf("expected Play edit to be committed")
		}
	})
	return srv
}

func TestPlayAuthPrecedence(t *testing.T) {
	const saEmail = "sa-override@example.iam.gserviceaccount.com"

	t.Run("NSGO_PLAY_SERVICE_ACCOUNT JSON and file path beat ADC", func(t *testing.T) {
		for _, asFile := range []bool{false, true} {
			name := "inline_json"
			if asFile {
				name = "file_path"
			}
			t.Run(name, func(t *testing.T) {
				isolatePlayADCEnv(t)
				var tokenCalls atomic.Int32
				authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					tokenCalls.Add(1)
					if err := r.ParseForm(); err != nil {
						t.Errorf("parse token form: %v", err)
					}
					if got := r.FormValue("grant_type"); got != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
						t.Errorf("grant_type = %q", got)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"sa-access-token","expires_in":3600}`)
				}))
				defer authSrv.Close()

				saJSON, pemText := testPlayServiceAccountJSON(t, saEmail, authSrv.URL)
				if asFile {
					p := filepath.Join(t.TempDir(), "sa.json")
					if err := os.WriteFile(p, saJSON, 0o600); err != nil {
						t.Fatal(err)
					}
					t.Setenv(googlePlayServiceAccountEnvVar, p)
				} else {
					t.Setenv(googlePlayServiceAccountEnvVar, string(saJSON))
				}

				// Also populate GOOGLE_APPLICATION_CREDENTIALS and trap ADC lookup to verify it is never consulted.
				t.Setenv(googleAppCredentialsEnvVar, filepath.Join(t.TempDir(), "unused-adc.json"))
				origFind := findGoogleDefaultCredentials
				findGoogleDefaultCredentials = func(context.Context, ...string) (*google.Credentials, error) {
					t.Error("ADC must not be called when NSGO_PLAY_SERVICE_ACCOUNT is set")
					return nil, errors.New("unexpected ADC lookup")
				}
				t.Cleanup(func() { findGoogleDefaultCredentials = origFind })

				client, err := googlePlayClientFromEnv(t.Context(), authSrv.Client())
				if err != nil {
					t.Fatal(err)
				}
				summary := client.credentialSummary()
				if !strings.Contains(summary, "service_account") || !strings.Contains(summary, saEmail) || !strings.Contains(summary, googlePlayServiceAccountEnvVar) {
					t.Fatalf("credentialSummary = %q", summary)
				}
				if strings.Contains(summary, pemText) || strings.Contains(summary, "PRIVATE KEY") {
					t.Fatalf("credentialSummary leaked secret: %q", summary)
				}

				playSrv := newPlayUploadMockServer(t, "sa-access-token")
				client.baseURL = playSrv.URL
				client.uploadBaseURL = playSrv.URL
				app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
				if err := uploadGooglePlayBundle(t.Context(), client, "internal", app, strings.NewReader("aab"), 3); err != nil {
					t.Fatal(err)
				}
				if tokenCalls.Load() != 1 {
					t.Fatalf("token endpoint calls = %d, want 1 (cached)", tokenCalls.Load())
				}
			})
		}
	})

	t.Run("invalid NSGO_PLAY_SERVICE_ACCOUNT fails without falling back to ADC", func(t *testing.T) {
		isolatePlayADCEnv(t)
		t.Setenv(googlePlayServiceAccountEnvVar, `{"client_email":"broken@example.com"}`)
		origFind := findGoogleDefaultCredentials
		findGoogleDefaultCredentials = func(context.Context, ...string) (*google.Credentials, error) {
			t.Error("ADC must not be called when NSGO_PLAY_SERVICE_ACCOUNT is set to invalid JSON")
			return &google.Credentials{TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "adc-tok"})}, nil
		}
		t.Cleanup(func() { findGoogleDefaultCredentials = origFind })

		_, err := googlePlayClientFromEnv(t.Context(), nil)
		if err == nil || !strings.Contains(err.Error(), "client_email and private_key") {
			t.Fatalf("got %v, want service account JSON validation error", err)
		}
	})
}

func TestPlayAuthADCMissingNamesBothOptions(t *testing.T) {
	isolatePlayADCEnv(t)
	origFind := findGoogleDefaultCredentials
	findGoogleDefaultCredentials = func(context.Context, ...string) (*google.Credentials, error) {
		return nil, errors.New("google: could not find default credentials")
	}
	t.Cleanup(func() { findGoogleDefaultCredentials = origFind })

	_, err := googlePlayClientFromEnv(t.Context(), nil)
	if err == nil {
		t.Fatal("expected error when no credentials are configured")
	}
	for _, want := range []string{
		googlePlayServiceAccountEnvVar,
		"Application Default Credentials",
		"gcloud auth application-default login --impersonate-service-account",
		googleAppCredentialsEnvVar,
		"Workload Identity Federation",
	} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("missing-credentials error %q does not contain %q", err.Error(), want)
		}
	}
}

func TestPlayAuthADCCredentialTypes(t *testing.T) {
	t.Run("service_account via GOOGLE_APPLICATION_CREDENTIALS", func(t *testing.T) {
		isolatePlayADCEnv(t)
		const saEmail = "adc-sa@example.iam.gserviceaccount.com"
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}
			if got := r.FormValue("grant_type"); got != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Errorf("grant_type = %q", got)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"access_token":"adc-sa-token","token_type":"Bearer","expires_in":3600}`)
		}))
		defer authSrv.Close()

		saJSON, _ := testPlayServiceAccountJSON(t, saEmail, authSrv.URL)
		credPath := filepath.Join(t.TempDir(), "adc-sa.json")
		if err := os.WriteFile(credPath, saJSON, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(googleAppCredentialsEnvVar, credPath)

		client, err := googlePlayClientFromEnv(t.Context(), authSrv.Client())
		if err != nil {
			t.Fatal(err)
		}
		summary := client.credentialSummary()
		if summary != "service_account "+saEmail+" (Application Default Credentials)" {
			t.Fatalf("credentialSummary = %q", summary)
		}

		playSrv := newPlayUploadMockServer(t, "adc-sa-token")
		client.baseURL = playSrv.URL
		client.uploadBaseURL = playSrv.URL
		app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
		if err := uploadGooglePlayBundle(t.Context(), client, "internal", app, strings.NewReader("aab"), 3); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("authorized_user via GOOGLE_APPLICATION_CREDENTIALS and well-known gcloud path", func(t *testing.T) {
		for _, wellKnown := range []bool{false, true} {
			name := "env_var"
			if wellKnown {
				name = "well_known_gcloud_path"
			}
			t.Run(name, func(t *testing.T) {
				home := isolatePlayADCEnv(t)
				const (
					userEmail     = "dev@example.com"
					clientSecret  = "top-secret-oauth-client-secret"
					refreshSecret = "top-secret-refresh-token"
				)
				authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if err := r.ParseForm(); err != nil {
						t.Errorf("parse form: %v", err)
					}
					if r.FormValue("grant_type") != "refresh_token" ||
						r.FormValue("refresh_token") != refreshSecret ||
						r.FormValue("client_secret") != clientSecret {
						t.Errorf("unexpected refresh request: %v", r.Form)
					}
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"access_token":"adc-user-token","token_type":"Bearer","expires_in":3600}`)
				}))
				defer authSrv.Close()

				credJSON, err := json.Marshal(map[string]string{
					"type":          "authorized_user",
					"account":       userEmail,
					"client_id":     "test-client-id",
					"client_secret": clientSecret,
					"refresh_token": refreshSecret,
					"token_uri":     authSrv.URL,
				})
				if err != nil {
					t.Fatal(err)
				}
				if wellKnown {
					var p string
					if runtime.GOOS == "windows" {
						p = filepath.Join(home, "gcloud", "application_default_credentials.json")
					} else {
						p = filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
					}
					if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(p, credJSON, 0o600); err != nil {
						t.Fatal(err)
					}
				} else {
					p := filepath.Join(t.TempDir(), "authorized_user.json")
					if err := os.WriteFile(p, credJSON, 0o600); err != nil {
						t.Fatal(err)
					}
					t.Setenv(googleAppCredentialsEnvVar, p)
				}

				client, err := googlePlayClientFromEnv(t.Context(), authSrv.Client())
				if err != nil {
					t.Fatal(err)
				}
				summary := client.credentialSummary()
				if summary != "authorized_user "+userEmail+" (Application Default Credentials)" {
					t.Fatalf("credentialSummary = %q", summary)
				}
				if strings.Contains(summary, clientSecret) || strings.Contains(summary, refreshSecret) {
					t.Fatalf("credentialSummary leaked secret: %q", summary)
				}

				playSrv := newPlayUploadMockServer(t, "adc-user-token")
				client.baseURL = playSrv.URL
				client.uploadBaseURL = playSrv.URL
				app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
				if err := uploadGooglePlayBundle(t.Context(), client, "internal", app, strings.NewReader("aab"), 3); err != nil {
					t.Fatal(err)
				}
			})
		}
	})

	t.Run("impersonated_service_account from gcloud ADC", func(t *testing.T) {
		home := isolatePlayADCEnv(t)
		const (
			targetSA      = "play-publisher@example.iam.gserviceaccount.com"
			clientSecret  = "gcloud-client-secret-value"
			refreshSecret = "gcloud-refresh-token-value"
		)
		var sawSourceRefresh, sawImpersonate atomic.Bool
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/token":
				sawSourceRefresh.Store(true)
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse source token form: %v", err)
				}
				if r.FormValue("refresh_token") != refreshSecret {
					t.Errorf("unexpected refresh_token %q", r.FormValue("refresh_token"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"source-user-token","token_type":"Bearer","expires_in":3600}`)
			case "/v1/projects/-/serviceAccounts/" + targetSA + ":generateAccessToken":
				sawImpersonate.Store(true)
				if got := r.Header.Get("Authorization"); got != "Bearer source-user-token" {
					t.Errorf("impersonate Authorization = %q, want Bearer source-user-token", got)
				}
				var reqBody struct {
					Scope []string `json:"scope"`
				}
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Errorf("decode impersonate request: %v", err)
				}
				if !slices.Contains(reqBody.Scope, googlePlayPublisherScope) {
					t.Errorf("impersonate scopes = %v, want %q", reqBody.Scope, googlePlayPublisherScope)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"accessToken":"impersonated-play-token","expireTime":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
			default:
				t.Errorf("unexpected auth request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer authSrv.Close()

		credJSON, err := json.Marshal(map[string]any{
			"type":                              "impersonated_service_account",
			"delegates":                         []string{},
			"service_account_impersonation_url": authSrv.URL + "/v1/projects/-/serviceAccounts/" + targetSA + ":generateAccessToken",
			"source_credentials": map[string]string{
				"type":          "authorized_user",
				"client_id":     "gcloud-client-id",
				"client_secret": clientSecret,
				"refresh_token": refreshSecret,
				"token_uri":     authSrv.URL + "/token",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		var wellKnown string
		if runtime.GOOS == "windows" {
			wellKnown = filepath.Join(home, "gcloud", "application_default_credentials.json")
		} else {
			wellKnown = filepath.Join(home, ".config", "gcloud", "application_default_credentials.json")
		}
		if err := os.MkdirAll(filepath.Dir(wellKnown), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(wellKnown, credJSON, 0o600); err != nil {
			t.Fatal(err)
		}

		client, err := googlePlayClientFromEnv(t.Context(), authSrv.Client())
		if err != nil {
			t.Fatal(err)
		}
		summary := client.credentialSummary()
		if summary != "impersonated_service_account "+targetSA+" (Application Default Credentials)" {
			t.Fatalf("credentialSummary = %q", summary)
		}
		if strings.Contains(summary, clientSecret) || strings.Contains(summary, refreshSecret) {
			t.Fatalf("credentialSummary leaked secret: %q", summary)
		}

		playSrv := newPlayUploadMockServer(t, "impersonated-play-token")
		client.baseURL = playSrv.URL
		client.uploadBaseURL = playSrv.URL
		app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
		if err := uploadGooglePlayBundle(t.Context(), client, "internal", app, strings.NewReader("aab"), 3); err != nil {
			t.Fatal(err)
		}
		if !sawSourceRefresh.Load() || !sawImpersonate.Load() {
			t.Fatalf("impersonation steps missing: sourceRefresh=%v impersonate=%v", sawSourceRefresh.Load(), sawImpersonate.Load())
		}
	})

	t.Run("external_account WIF with service account impersonation (google-github-actions/auth)", func(t *testing.T) {
		isolatePlayADCEnv(t)
		const (
			targetSA  = "github-play@example.iam.gserviceaccount.com"
			audience  = "//iam.googleapis.com/projects/123456/locations/global/workloadIdentityPools/github/providers/nsgo"
			oidcToken = "fake-github-oidc-jwt-token"
		)
		var sawSTS, sawImpersonate atomic.Bool
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/token":
				sawSTS.Store(true)
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse STS form: %v", err)
				}
				if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:token-exchange" ||
					r.FormValue("subject_token") != oidcToken ||
					r.FormValue("audience") != audience {
					t.Errorf("unexpected STS exchange form: %v", r.Form)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"sts-federated-token","issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer","expires_in":3600}`)
			case "/v1/projects/-/serviceAccounts/" + targetSA + ":generateAccessToken":
				sawImpersonate.Store(true)
				if got := r.Header.Get("Authorization"); got != "Bearer sts-federated-token" {
					t.Errorf("WIF impersonate Authorization = %q, want Bearer sts-federated-token", got)
				}
				var reqBody struct {
					Scope []string `json:"scope"`
				}
				if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
					t.Errorf("decode impersonate request: %v", err)
				}
				if !slices.Contains(reqBody.Scope, googlePlayPublisherScope) {
					t.Errorf("impersonate scopes = %v, want %q", reqBody.Scope, googlePlayPublisherScope)
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprintf(w, `{"accessToken":"wif-play-token","expireTime":%q}`, time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
			default:
				t.Errorf("unexpected WIF request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer authSrv.Close()

		oidcFile := filepath.Join(t.TempDir(), "oidc.jwt")
		if err := os.WriteFile(oidcFile, []byte(oidcToken), 0o600); err != nil {
			t.Fatal(err)
		}
		credJSON, err := json.Marshal(map[string]any{
			"type":                              "external_account",
			"audience":                          audience,
			"subject_token_type":                "urn:ietf:params:oauth:token-type:jwt",
			"token_url":                         authSrv.URL + "/v1/token",
			"service_account_impersonation_url": authSrv.URL + "/v1/projects/-/serviceAccounts/" + targetSA + ":generateAccessToken",
			"credential_source": map[string]any{
				"file": oidcFile,
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		credPath := filepath.Join(t.TempDir(), "wif.json")
		if err := os.WriteFile(credPath, credJSON, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(googleAppCredentialsEnvVar, credPath)

		client, err := googlePlayClientFromEnv(t.Context(), authSrv.Client())
		if err != nil {
			t.Fatal(err)
		}
		summary := client.credentialSummary()
		if summary != "external_account "+targetSA+" (Application Default Credentials)" {
			t.Fatalf("credentialSummary = %q", summary)
		}

		playSrv := newPlayUploadMockServer(t, "wif-play-token")
		client.baseURL = playSrv.URL
		client.uploadBaseURL = playSrv.URL
		app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
		if err := uploadGooglePlayBundle(t.Context(), client, "internal", app, strings.NewReader("aab"), 3); err != nil {
			t.Fatal(err)
		}
		if !sawSTS.Load() || !sawImpersonate.Load() {
			t.Fatalf("WIF steps missing: sts=%v impersonate=%v", sawSTS.Load(), sawImpersonate.Load())
		}
	})

	t.Run("external_account direct URL credential source without impersonation", func(t *testing.T) {
		isolatePlayADCEnv(t)
		const audience = "//iam.googleapis.com/projects/123456/locations/global/workloadIdentityPools/ci/providers/oidc"
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/oidc":
				if r.Header.Get("Authorization") != "Bearer actions-req-tok" {
					t.Errorf("OIDC request Authorization = %q", r.Header.Get("Authorization"))
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"value":"url-subject-token"}`)
			case "/v1/token":
				if err := r.ParseForm(); err != nil {
					t.Errorf("parse STS form: %v", err)
				}
				if r.FormValue("subject_token") != "url-subject-token" || r.FormValue("scope") != googlePlayPublisherScope {
					t.Errorf("unexpected direct STS exchange: %v", r.Form)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"direct-wif-token","issued_token_type":"urn:ietf:params:oauth:token-type:access_token","token_type":"Bearer","expires_in":3600}`)
			default:
				t.Errorf("unexpected direct WIF request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer authSrv.Close()

		credJSON, err := json.Marshal(map[string]any{
			"type":               "external_account",
			"audience":           audience,
			"subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
			"token_url":          authSrv.URL + "/v1/token",
			"credential_source": map[string]any{
				"url":     authSrv.URL + "/oidc",
				"headers": map[string]string{"Authorization": "Bearer actions-req-tok"},
				"format": map[string]string{
					"type":                     "json",
					"subject_token_field_name": "value",
				},
			},
		})
		if err != nil {
			t.Fatal(err)
		}
		credPath := filepath.Join(t.TempDir(), "wif-direct.json")
		if err := os.WriteFile(credPath, credJSON, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv(googleAppCredentialsEnvVar, credPath)

		client, err := googlePlayClientFromEnv(t.Context(), authSrv.Client())
		if err != nil {
			t.Fatal(err)
		}
		if summary := client.credentialSummary(); summary != "external_account (Application Default Credentials)" {
			t.Fatalf("credentialSummary = %q", summary)
		}

		playSrv := newPlayUploadMockServer(t, "direct-wif-token")
		client.baseURL = playSrv.URL
		client.uploadBaseURL = playSrv.URL
		app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
		if err := uploadGooglePlayBundle(t.Context(), client, "internal", app, strings.NewReader("aab"), 3); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPlayAuthADCMetadataServer(t *testing.T) {
	t.Run("fake metadata token source reports metadata_server and email", func(t *testing.T) {
		isolatePlayADCEnv(t)
		const metaEmail = "123456-compute@developer.gserviceaccount.com"
		origFind := findGoogleDefaultCredentials
		origEmail := googleMetadataEmail
		findGoogleDefaultCredentials = func(_ context.Context, scopes ...string) (*google.Credentials, error) {
			if !slices.Contains(scopes, googlePlayPublisherScope) {
				t.Errorf("scopes = %v, want %q", scopes, googlePlayPublisherScope)
			}
			return &google.Credentials{
				TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "gce-meta-token"}),
			}, nil
		}
		googleMetadataEmail = func(context.Context) string { return metaEmail }
		t.Cleanup(func() {
			findGoogleDefaultCredentials = origFind
			googleMetadataEmail = origEmail
		})

		client, err := googlePlayClientFromEnv(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		if summary := client.credentialSummary(); summary != "metadata_server "+metaEmail+" (Application Default Credentials)" {
			t.Fatalf("credentialSummary = %q", summary)
		}

		playSrv := newPlayUploadMockServer(t, "gce-meta-token")
		client.baseURL = playSrv.URL
		client.uploadBaseURL = playSrv.URL
		app := apk.BundleIdentity{Package: testPlayApp, VersionCode: 19}
		if err := uploadGooglePlayBundle(t.Context(), client, "internal", app, strings.NewReader("aab"), 3); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("real GCE_METADATA_HOST end-to-end", func(t *testing.T) {
		const metaEmail = "cloudrun-play@example.iam.gserviceaccount.com"
		var sawToken, sawEmail atomic.Bool
		metaSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Metadata-Flavor") != "Google" {
				t.Errorf("missing Metadata-Flavor: Google header on %s", r.URL.RequestURI())
				w.WriteHeader(http.StatusForbidden)
				return
			}
			switch r.URL.Path {
			case "/computeMetadata/v1/project/project-id":
				_, _ = io.WriteString(w, "example-project")
			case "/computeMetadata/v1/instance/service-accounts/default/email":
				sawEmail.Store(true)
				_, _ = io.WriteString(w, metaEmail)
			case "/computeMetadata/v1/instance/service-accounts/default/token":
				sawToken.Store(true)
				if got := r.URL.Query().Get("scopes"); got != googlePlayPublisherScope {
					t.Errorf("metadata token scopes = %q, want %q", got, googlePlayPublisherScope)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"access_token":"real-gce-token","expires_in":3600,"token_type":"Bearer"}`)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
		defer metaSrv.Close()

		u, err := url.Parse(metaSrv.URL)
		if err != nil {
			t.Fatal(err)
		}
		exe, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		home := t.TempDir()
		cmd := exec.CommandContext(t.Context(), exe, "-test.run=^TestPlayAuthADCMetadataServerHelper$")
		cmd.Env = append(os.Environ(),
			"NSGO_TEST_GCE_METADATA=1",
			"GCE_METADATA_HOST="+u.Host,
			googlePlayServiceAccountEnvVar+"=",
			googleAppCredentialsEnvVar+"=",
			"HOME="+home,
			"APPDATA="+home,
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("GCE metadata helper failed: %v\n%s", err, out)
		}
		if !sawEmail.Load() || !sawToken.Load() {
			t.Fatalf("metadata server calls missing: sawEmail=%v sawToken=%v\n%s", sawEmail.Load(), sawToken.Load(), out)
		}
	})
}

func TestPlayAuthADCMetadataServerHelper(t *testing.T) {
	if os.Getenv("NSGO_TEST_GCE_METADATA") != "1" {
		return
	}
	client, err := googlePlayClientFromEnv(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if want := "metadata_server cloudrun-play@example.iam.gserviceaccount.com (Application Default Credentials)"; client.credentialSummary() != want {
		t.Fatalf("credentialSummary = %q, want %q", client.credentialSummary(), want)
	}
	tok, err := client.tokenFunc(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if tok != "real-gce-token" {
		t.Fatalf("token = %q, want real-gce-token", tok)
	}
}

type rotatingTokenSource struct {
	calls  atomic.Int32
	tokens []*oauth2.Token
}

func (r *rotatingTokenSource) Token() (*oauth2.Token, error) {
	i := int(r.calls.Add(1)) - 1
	if i >= len(r.tokens) {
		return r.tokens[len(r.tokens)-1], nil
	}
	return r.tokens[i], nil
}

func TestPlayAuthTokenRefresh(t *testing.T) {
	t.Run("ADC ReuseTokenSource refreshes expired token and caches valid token", func(t *testing.T) {
		isolatePlayADCEnv(t)
		src := &rotatingTokenSource{
			tokens: []*oauth2.Token{
				{AccessToken: "short-lived-token", Expiry: time.Now().Add(-time.Second)},
				{AccessToken: "refreshed-token", Expiry: time.Now().Add(time.Hour)},
			},
		}
		origFind := findGoogleDefaultCredentials
		findGoogleDefaultCredentials = func(context.Context, ...string) (*google.Credentials, error) {
			return &google.Credentials{
				JSON:        []byte(`{"type":"authorized_user","account":"dev@example.com"}`),
				TokenSource: oauth2.ReuseTokenSource(nil, src),
			}, nil
		}
		t.Cleanup(func() { findGoogleDefaultCredentials = origFind })

		client, err := googlePlayClientFromEnv(t.Context(), nil)
		if err != nil {
			t.Fatal(err)
		}
		tok1, err := client.tokenFunc(t.Context())
		if err != nil || tok1 != "short-lived-token" {
			t.Fatalf("first token = (%q, %v)", tok1, err)
		}
		tok2, err := client.tokenFunc(t.Context())
		if err != nil || tok2 != "refreshed-token" {
			t.Fatalf("second token = (%q, %v)", tok2, err)
		}
		tok3, err := client.tokenFunc(t.Context())
		if err != nil || tok3 != "refreshed-token" {
			t.Fatalf("third token = (%q, %v)", tok3, err)
		}
		if got := src.calls.Load(); got != 2 {
			t.Fatalf("underlying token source called %d times, want 2", got)
		}
	})

	t.Run("service account client refreshes expiring token and caches long-lived token", func(t *testing.T) {
		var calls atomic.Int32
		authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			n := calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			if n == 1 {
				// Expires within the 1-minute refresh window, so next call must refresh.
				_, _ = io.WriteString(w, `{"access_token":"sa-tok-1","expires_in":10}`)
				return
			}
			_, _ = io.WriteString(w, `{"access_token":"sa-tok-2","expires_in":3600}`)
		}))
		defer authSrv.Close()

		saJSON, _ := testPlayServiceAccountJSON(t, "refresh@example.iam.gserviceaccount.com", authSrv.URL)
		client, err := newGooglePlayClientFromServiceAccount(string(saJSON), "https://androidpublisher.googleapis.com", authSrv.Client())
		if err != nil {
			t.Fatal(err)
		}
		tok1, err := client.tokenFunc(t.Context())
		if err != nil || tok1 != "sa-tok-1" {
			t.Fatalf("tok1 = (%q, %v)", tok1, err)
		}
		tok2, err := client.tokenFunc(t.Context())
		if err != nil || tok2 != "sa-tok-2" {
			t.Fatalf("tok2 = (%q, %v)", tok2, err)
		}
		tok3, err := client.tokenFunc(t.Context())
		if err != nil || tok3 != "sa-tok-2" {
			t.Fatalf("tok3 = (%q, %v)", tok3, err)
		}
		if calls.Load() != 2 {
			t.Fatalf("token endpoint calls = %d, want 2", calls.Load())
		}
	})
}

func TestPlayAuth401And403ReportRefusedIdentity(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		body     string
		cred     googlePlayCredentialInfo
		wantSubs []string
	}{
		{
			name:   "403 impersonated service account",
			status: http.StatusForbidden,
			body:   `{"error":{"status":"PERMISSION_DENIED","message":"The caller does not have permission"}}`,
			cred: googlePlayCredentialInfo{
				Type:    "impersonated_service_account",
				Account: "play-publisher@example.iam.gserviceaccount.com",
				Source:  "Application Default Credentials",
			},
			wantSubs: []string{
				"HTTP 403",
				"PERMISSION_DENIED",
				"play-publisher@example.iam.gserviceaccount.com (impersonated_service_account)",
				"Play Console › Users and permissions",
			},
		},
		{
			name:   "401 external account WIF",
			status: http.StatusUnauthorized,
			body:   `{"error":{"status":"UNAUTHENTICATED","message":"Invalid credentials"}}`,
			cred: googlePlayCredentialInfo{
				Type:    "external_account",
				Account: "ci-play@example.iam.gserviceaccount.com",
				Source:  "Application Default Credentials",
			},
			wantSubs: []string{
				"HTTP 401",
				"UNAUTHENTICATED",
				"ci-play@example.iam.gserviceaccount.com (external_account)",
				"Play Console › Users and permissions",
			},
		},
		{
			name:   "403 authorized user without email",
			status: http.StatusForbidden,
			body:   `{"error":{"status":"PERMISSION_DENIED","message":"Forbidden"}}`,
			cred: googlePlayCredentialInfo{
				Type:   "authorized_user",
				Source: "Application Default Credentials",
			},
			wantSubs: []string{
				"HTTP 403",
				"authorized_user credentials",
				"Play Console › Users and permissions",
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()

			const secretToken = "super-secret-bearer-token"
			c := newGooglePlayClientFromTokenSource(
				oauth2.StaticTokenSource(&oauth2.Token{AccessToken: secretToken}),
				tc.cred,
				srv.URL,
				srv.Client(),
			)

			// Verify doJSON error.
			err := c.doJSON(t.Context(), http.MethodPost, testPlayEdits, struct{}{}, nil)
			if err == nil {
				t.Fatal("expected error from doJSON")
			}
			for _, sub := range tc.wantSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("doJSON error %q missing %q", err.Error(), sub)
				}
			}
			if strings.Contains(err.Error(), secretToken) {
				t.Errorf("doJSON error leaked access token: %q", err.Error())
			}

			// Verify uploadBundle error.
			_, err = c.uploadBundle(t.Context(), testPlayApp, "edit1", strings.NewReader("aab"), 3)
			if err == nil {
				t.Fatal("expected error from uploadBundle")
			}
			for _, sub := range tc.wantSubs {
				if !strings.Contains(err.Error(), sub) {
					t.Errorf("uploadBundle error %q missing %q", err.Error(), sub)
				}
			}
			if strings.Contains(err.Error(), secretToken) {
				t.Errorf("uploadBundle error leaked access token: %q", err.Error())
			}
		})
	}
}

func TestPlayFindsTheProjectsStoreBundle(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Example App\nid: dev.example.play\nversion: 1.2.3\nbuild:\n  output: artifacts\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := playBundlePath(dir); err == nil || !strings.Contains(err.Error(), "no App Bundle") {
		t.Fatalf("missing bundle returned %v", err)
	}
	releases := filepath.Join(dir, "artifacts", "releases")
	if err := os.MkdirAll(releases, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(releases, "example-app-1.2.3-android-universal.aab")
	if err := os.WriteFile(want, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	path, id, err := playBundlePath(dir)
	if err != nil || path != want || id != testPlayApp {
		t.Fatalf("bundle = %q, app = %q, err = %v; want %q", path, id, err, want)
	}
	if _, _, err := playBundlePath(filepath.Join(dir, "example.apk")); err == nil {
		t.Fatal("publish accepted an APK")
	}
	if err := os.WriteFile(filepath.Join(releases, "example-app-1.2.3-android-arm64.aab"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := playBundlePath(dir); err == nil || !strings.Contains(err.Error(), "multiple App Bundles") {
		t.Fatalf("ambiguous bundle lookup returned %v", err)
	}
}

func TestPlayPublishCommand(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"-target", "android", "--track", "internal", "missing.aab"}, "open the App Bundle"},
		{[]string{"-target", "android"}, "requires -track"},
		{[]string{"-target", "android", "-track", "internal", "-cert", "package.cert"}, "-target android takes only -track"},
		{[]string{"-target", "android", "-track", "internal", "-store", "play"}, "-target android takes only -track"},
		{[]string{"-target", "ios", "-track", "internal"}, "unknown target"},
		{nil, "-cert is required"},
	} {
		cmd := exec.CommandContext(t.Context(), exe, append([]string{"-test.run=^TestCLIProcess$", "--", "publish"}, tc.args...)...)
		cmd.Dir = t.TempDir()
		cmd.Env = append(os.Environ(), "NSGO_CLI_PROCESS=1", googlePlayServiceAccountEnvVar+"=")
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), tc.want) {
			t.Errorf("publish %v exited with %v and %s, want %q", tc.args, err, out, tc.want)
		}
	}
}
