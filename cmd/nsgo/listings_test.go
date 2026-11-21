package main

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func writeValidTwoLanguageListings(t *testing.T, root string) string {
	t.Helper()
	writeFiles(t, root, map[string]string{
		"nsgo.yaml": `name: Orbit Notes
id: dev.example.orbit
version: 1.4.0
copyright: "© 2026 Example Studio"
store:
  dir: store
`,
		"store/listing.yaml": `version: 1
default_locale: en-US
app_store:
  platform: ios
  primary_category: PRODUCTIVITY
  secondary_category: UTILITIES
google_play:
  contact_email: support@example.com
  contact_website: https://example.com
  contact_phone: "+1-555-0100"
  track: production
`,
		"store/en-US/listing.yaml": `version: 1
title: Orbit Notes
subtitle: Fast local-first notes
short_description: Fast local-first notes built with NSGo.
keywords:
  - notes
  - markdown
  - offline
support_url: https://example.com/support
marketing_url: https://example.com
privacy_policy_url: https://example.com/privacy
video_url: https://www.youtube.com/watch?v=dQw4w9WgXcQ
app_store:
  screenshots:
    APP_IPHONE_67:
      - screenshots/iphone-1.png
  previews:
    IPHONE_67:
      - previews/demo.mp4
google_play:
  screenshots:
    phoneScreenshots:
      - screenshots/phone-1.png
  feature_graphic: screenshots/feature.png
  icon: screenshots/icon.png
`,
		"store/en-US/description.md":      "Orbit Notes keeps your ideas fast and local.\n\nSync when you want, work offline always.\n",
		"store/en-US/release_notes.md":    "Improved search speed and fixed dark mode borders.\n",
		"store/en-US/promotional_text.md": "New: instant Markdown previews!\n",
		"store/zh-Hans/listing.yaml": `version: 1
title: 轨道笔记
subtitle: 快速本地优先笔记
short_description: 用 NSGo 构建的快速本地优先笔记应用。
keywords: "笔记,Markdown,离线"
support_url: https://example.com/zh/support
privacy_policy_url: https://example.com/zh/privacy
app_store:
  screenshots:
    APP_IPHONE_67:
      - screenshots/iphone-1.png
google_play:
  screenshots:
    phoneScreenshots:
      - screenshots/phone-1.png
`,
		"store/zh-Hans/description.md":   "轨道笔记让灵感随时记录。\n\n支持离线编辑与快速检索。\n",
		"store/zh-Hans/release_notes.md": "优化搜索速度并修复深色模式边框。\n",
	})
	mediaFiles := map[string][]byte{
		"store/en-US/screenshots/iphone-1.png":   []byte("\x89PNG\r\n\x1a\nen-iphone-1"),
		"store/en-US/previews/demo.mp4":          []byte("ftypmp42-en-preview-bytes"),
		"store/en-US/screenshots/phone-1.png":    []byte("\x89PNG\r\n\x1a\nen-phone-1"),
		"store/en-US/screenshots/feature.png":    []byte("\x89PNG\r\n\x1a\nen-feature"),
		"store/en-US/screenshots/icon.png":       []byte("\x89PNG\r\n\x1a\nen-icon"),
		"store/zh-Hans/screenshots/iphone-1.png": []byte("\x89PNG\r\n\x1a\nzh-iphone-1"),
		"store/zh-Hans/screenshots/phone-1.png":  []byte("\x89PNG\r\n\x1a\nzh-phone-1"),
	}
	for relPath, content := range mediaFiles {
		full := filepath.Join(root, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, content, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func verifyES256JWT(t *testing.T, authHeader string, pub *ecdsa.PublicKey, wantKeyID, wantIssuer string) {
	t.Helper()
	tok, ok := strings.CutPrefix(authHeader, "Bearer ")
	if !ok {
		t.Fatalf("missing Bearer token in Authorization header %q", authHeader)
	}
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed JWT: %q", tok)
	}
	hdrRaw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		t.Fatalf("decode JWT header: %v", err)
	}
	var hdr map[string]string
	if err := json.Unmarshal(hdrRaw, &hdr); err != nil {
		t.Fatalf("unmarshal JWT header: %v", err)
	}
	if hdr["alg"] != "ES256" || hdr["kid"] != wantKeyID {
		t.Fatalf("unexpected JWT header: %+v", hdr)
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode JWT claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		t.Fatalf("unmarshal JWT claims: %v", err)
	}
	if claims["iss"] != wantIssuer || claims["aud"] != "appstoreconnect-v1" {
		t.Fatalf("unexpected JWT claims: %+v", claims)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sig) != 64 {
		t.Fatalf("unexpected ES256 signature len=%d err=%v", len(sig), err)
	}
	r := new(big.Int).SetBytes(sig[:32])
	s := new(big.Int).SetBytes(sig[32:])
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(pub, digest[:], r, s) {
		t.Fatalf("invalid ES256 JWT signature")
	}
}

func verifyRS256JWT(t *testing.T, assertion string, pub *rsa.PublicKey, wantEmail string) {
	t.Helper()
	parts := strings.Split(assertion, ".")
	if len(parts) != 3 {
		t.Fatalf("malformed RS256 JWT: %q", assertion)
	}
	claimsRaw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		t.Fatalf("decode claims: %v", err)
	}
	var claims map[string]any
	if err := json.Unmarshal(claimsRaw, &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	if claims["iss"] != wantEmail || claims["scope"] != googlePlayPublisherScope {
		t.Fatalf("unexpected Google JWT claims: %+v", claims)
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		t.Fatalf("decode RS256 sig: %v", err)
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, digest[:], sig); err != nil {
		t.Fatalf("invalid RS256 JWT signature: %v", err)
	}
}

func TestStoreListingsUploadTwoLanguagesEndToEnd(t *testing.T) {
	appDir := writeValidTwoLanguageListings(t, t.TempDir())

	// 1. Mock App Store Connect server with real ES256 JWT verification.
	ecKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const ascKeyID = "KEY1234567"
	const ascIssuer = "issuer-uuid-0001"

	var ascMu sync.Mutex
	ascInfoLocs := map[string]map[string]any{}
	ascVerLocs := map[string]map[string]any{}
	ascPrimaryCategory := ""
	ascSecondaryCategory := ""
	ascDeletedScreenshots := []string{}
	ascVersionCopyright := ""
	ascUploadedBytes := map[string][]byte{}
	ascCommittedMD5 := map[string]string{}
	ascAssetCount := 0

	var ascServer *httptest.Server
	ascServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ascMu.Lock()
		defer ascMu.Unlock()

		// Upload chunk URLs do not require API Bearer auth (pre-signed storage URLs).
		if strings.HasPrefix(r.URL.Path, "/upload/") {
			data, _ := io.ReadAll(r.Body)
			assetID := strings.TrimPrefix(r.URL.Path, "/upload/")
			ascUploadedBytes[assetID] = append(ascUploadedBytes[assetID], data...)
			w.WriteHeader(http.StatusOK)
			return
		}

		verifyES256JWT(t, r.Header.Get("Authorization"), &ecKey.PublicKey, ascKeyID, ascIssuer)
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/apps":
			if got := r.URL.Query().Get("filter[bundleId]"); got != "dev.example.orbit" {
				t.Errorf("filter[bundleId] = %q, want dev.example.orbit", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "app-1", "type": "apps"}},
			})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/apps/app-1/appInfos":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "info-1", "type": "appInfos", "attributes": map[string]any{"appStoreState": "PREPARE_FOR_SUBMISSION"}},
				},
			})

		case r.Method == http.MethodPatch && r.URL.Path == "/v1/appInfos/info-1":
			var body struct {
				Data struct {
					Relationships struct {
						PrimaryCategory struct {
							Data struct {
								ID string `json:"id"`
							} `json:"data"`
						} `json:"primaryCategory"`
						SecondaryCategory struct {
							Data struct {
								ID string `json:"id"`
							} `json:"data"`
						} `json:"secondaryCategory"`
					} `json:"relationships"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ascPrimaryCategory = body.Data.Relationships.PrimaryCategory.Data.ID
			ascSecondaryCategory = body.Data.Relationships.SecondaryCategory.Data.ID
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "info-1", "type": "appInfos"}})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/appInfos/info-1/appInfoLocalizations":
			// en-US already exists (tests PATCH); zh-Hans does not (tests POST).
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "infoloc-en", "type": "appInfoLocalizations", "attributes": map[string]any{"locale": "en-US"}},
				},
			})

		case r.Method == http.MethodPatch && r.URL.Path == "/v1/appInfoLocalizations/infoloc-en":
			var body struct {
				Data struct {
					Attributes map[string]any `json:"attributes"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ascInfoLocs["en-US"] = body.Data.Attributes
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "infoloc-en", "type": "appInfoLocalizations"}})

		case r.Method == http.MethodPost && r.URL.Path == "/v1/appInfoLocalizations":
			var body struct {
				Data struct {
					Attributes map[string]any `json:"attributes"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			locale, _ := body.Data.Attributes["locale"].(string)
			ascInfoLocs[locale] = body.Data.Attributes
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "infoloc-" + locale, "type": "appInfoLocalizations"}})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/apps/app-1/appStoreVersions":
			if r.URL.Query().Get("filter[platform]") != "IOS" || r.URL.Query().Get("filter[versionString]") != "1.4.0" {
				t.Errorf("unexpected appStoreVersions query: %s", r.URL.RawQuery)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "ver-1", "type": "appStoreVersions"}},
			})

		case r.Method == http.MethodPatch && r.URL.Path == "/v1/appStoreVersions/ver-1":
			var body struct {
				Data struct {
					Attributes map[string]any `json:"attributes"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ascVersionCopyright, _ = body.Data.Attributes["copyright"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "ver-1", "type": "appStoreVersions"}})

		case r.Method == http.MethodGet && r.URL.Path == "/v1/appStoreVersions/ver-1/appStoreVersionLocalizations":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{
					{"id": "verloc-en", "type": "appStoreVersionLocalizations", "attributes": map[string]any{"locale": "en-US"}},
				},
			})

		case r.Method == http.MethodPatch && r.URL.Path == "/v1/appStoreVersionLocalizations/verloc-en":
			var body struct {
				Data struct {
					Attributes map[string]any `json:"attributes"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ascVerLocs["en-US"] = body.Data.Attributes
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "verloc-en", "type": "appStoreVersionLocalizations"}})

		case r.Method == http.MethodPost && r.URL.Path == "/v1/appStoreVersionLocalizations":
			var body struct {
				Data struct {
					Attributes map[string]any `json:"attributes"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			locale, _ := body.Data.Attributes["locale"].(string)
			ascVerLocs[locale] = body.Data.Attributes
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "verloc-" + locale, "type": "appStoreVersionLocalizations"}})

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/appScreenshotSets"):
			// Return an existing set for en-US (to verify old screenshots are replaced) and empty for zh-Hans (to verify creation).
			if strings.Contains(r.URL.Path, "/verloc-en/") {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": []map[string]any{
						{"id": "sset-en", "type": "appScreenshotSets", "attributes": map[string]any{"screenshotDisplayType": "APP_IPHONE_67"}},
					},
				})
			} else {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})
			}

		case r.Method == http.MethodGet && r.URL.Path == "/v1/appScreenshotSets/sset-en/appScreenshots":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": []map[string]any{{"id": "old-shot-1", "type": "appScreenshots"}},
			})

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/v1/appScreenshots/"):
			ascDeletedScreenshots = append(ascDeletedScreenshots, filepath.Base(r.URL.Path))
			w.WriteHeader(http.StatusNoContent)

		case r.Method == http.MethodPost && r.URL.Path == "/v1/appScreenshotSets":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "sset-new", "type": "appScreenshotSets"}})

		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/appPreviewSets"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []any{}})

		case r.Method == http.MethodPost && r.URL.Path == "/v1/appPreviewSets":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": "pset-1", "type": "appPreviewSets"}})

		case r.Method == http.MethodPost && (r.URL.Path == "/v1/appScreenshots" || r.URL.Path == "/v1/appPreviews"):
			var body struct {
				Data struct {
					Type       string `json:"type"`
					Attributes struct {
						FileName string `json:"fileName"`
						FileSize int64  `json:"fileSize"`
					} `json:"attributes"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			ascAssetCount++
			assetID := body.Data.Type + "-" + strconv.Itoa(ascAssetCount)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"data": map[string]any{
					"id":   assetID,
					"type": body.Data.Type,
					"attributes": map[string]any{
						"uploadOperations": []map[string]any{
							{
								"method": "PUT",
								"url":    ascServer.URL + "/upload/" + assetID,
								"offset": 0,
								"length": body.Data.Attributes.FileSize,
								"requestHeaders": []map[string]string{
									{"name": "Content-Type", "value": "application/octet-stream"},
								},
							},
						},
					},
				},
			})

		case r.Method == http.MethodPatch && (strings.HasPrefix(r.URL.Path, "/v1/appScreenshots/") || strings.HasPrefix(r.URL.Path, "/v1/appPreviews/")):
			assetID := filepath.Base(r.URL.Path)
			var body struct {
				Data struct {
					Attributes struct {
						Uploaded           bool   `json:"uploaded"`
						SourceFileChecksum string `json:"sourceFileChecksum"`
					} `json:"attributes"`
				} `json:"data"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if !body.Data.Attributes.Uploaded {
				t.Errorf("asset %s committed with uploaded=false", assetID)
			}
			ascCommittedMD5[assetID] = body.Data.Attributes.SourceFileChecksum
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"id": assetID}})

		default:
			t.Errorf("unexpected App Store Connect request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer ascServer.Close()

	// 2. Mock Google Play + OAuth2 server with real RS256 service-account JWT verification.
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(rsaKey),
	})
	const serviceAccountEmail = "publisher@example-project.iam.gserviceaccount.com"
	const playAccessToken = "ya29.mock-access-token"

	var playMu sync.Mutex
	playDetails := map[string]any{}
	playListings := map[string]map[string]any{}
	playImages := map[string][][]byte{}
	playDeletedImages := map[string]int{}
	var playTrackReleaseNotes []map[string]any
	playValidated := false
	playCommitted := false

	playServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		playMu.Lock()
		defer playMu.Unlock()
		w.Header().Set("Content-Type", "application/json")

		if r.Method == http.MethodPost && r.URL.Path == "/oauth2/token" {
			_ = r.ParseForm()
			if r.FormValue("grant_type") != "urn:ietf:params:oauth:grant-type:jwt-bearer" {
				t.Errorf("unexpected grant_type: %q", r.FormValue("grant_type"))
			}
			verifyRS256JWT(t, r.FormValue("assertion"), &rsaKey.PublicKey, serviceAccountEmail)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": playAccessToken,
				"expires_in":   3600,
				"token_type":   "Bearer",
			})
			return
		}

		if r.Header.Get("Authorization") != "Bearer "+playAccessToken {
			t.Errorf("Google Play request %s %s missing expected Bearer token: %q", r.Method, r.URL.Path, r.Header.Get("Authorization"))
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		const editPrefix = "/androidpublisher/v3/applications/dev.example.orbit/edits"
		const uploadPrefix = "/upload/androidpublisher/v3/applications/dev.example.orbit/edits/edit-42/listings/"

		switch {
		case r.Method == http.MethodPost && r.URL.Path == editPrefix:
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "edit-42"})

		case r.Method == http.MethodPatch && r.URL.Path == editPrefix+"/edit-42/details":
			_ = json.NewDecoder(r.Body).Decode(&playDetails)
			_ = json.NewEncoder(w).Encode(playDetails)

		case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, editPrefix+"/edit-42/listings/"):
			lang := strings.TrimPrefix(r.URL.Path, editPrefix+"/edit-42/listings/")
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			playListings[lang] = body
			_ = json.NewEncoder(w).Encode(body)

		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, editPrefix+"/edit-42/listings/"):
			key := strings.TrimPrefix(r.URL.Path, editPrefix+"/edit-42/listings/")
			playDeletedImages[key]++
			_ = json.NewEncoder(w).Encode(map[string]any{})

		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, uploadPrefix):
			key := strings.TrimPrefix(r.URL.Path, uploadPrefix)
			data, _ := io.ReadAll(r.Body)
			playImages[key] = append(playImages[key], data)
			_ = json.NewEncoder(w).Encode(map[string]any{"image": map[string]any{"id": "img-1"}})

		case r.Method == http.MethodGet && r.URL.Path == editPrefix+"/edit-42/tracks/production":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"track":    "production",
				"releases": []map[string]any{{"status": "completed", "versionCodes": []string{"140"}}},
			})

		case r.Method == http.MethodPut && r.URL.Path == editPrefix+"/edit-42/tracks/production":
			var body struct {
				Releases []struct {
					ReleaseNotes []map[string]any `json:"releaseNotes"`
				} `json:"releases"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Releases) > 0 {
				playTrackReleaseNotes = body.Releases[0].ReleaseNotes
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"track": "production"})

		case r.Method == http.MethodPost && r.URL.Path == editPrefix+"/edit-42:validate":
			playValidated = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "edit-42"})

		case r.Method == http.MethodPost && r.URL.Path == editPrefix+"/edit-42:commit":
			playCommitted = true
			_ = json.NewEncoder(w).Encode(map[string]any{"id": "edit-42"})

		default:
			t.Errorf("unexpected Google Play request: %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer playServer.Close()

	// Build real clients pointing to the mock servers.
	ascClient := &appStoreClient{
		baseURL:    ascServer.URL,
		httpClient: ascServer.Client(),
		tokenFunc: func(context.Context) (string, error) {
			return signAppStoreJWT(ecKey, ascKeyID, ascIssuer, time.Now())
		},
	}
	saJSON, err := json.Marshal(googleServiceAccount{
		ClientEmail: serviceAccountEmail,
		PrivateKey:  string(rsaPEM),
		TokenURI:    playServer.URL + "/oauth2/token",
	})
	if err != nil {
		t.Fatal(err)
	}
	playClient, err := newGooglePlayClientFromServiceAccount(string(saJSON), playServer.URL, playServer.Client())
	if err != nil {
		t.Fatal(err)
	}

	// Execute end-to-end publish to BOTH stores.
	err = publishWithContext(t.Context(), publishOptions{
		store:            "all",
		appStoreClient:   ascClient,
		googlePlayClient: playClient,
	}, appDir)
	if err != nil {
		t.Fatalf("publishWithContext failed: %v", err)
	}

	// Verify App Store Connect received both en-US and zh-Hans.
	if ascPrimaryCategory != "PRODUCTIVITY" || ascSecondaryCategory != "UTILITIES" {
		t.Errorf("App Store categories = (%q, %q), want (PRODUCTIVITY, UTILITIES)", ascPrimaryCategory, ascSecondaryCategory)
	}
	if len(ascDeletedScreenshots) != 1 || ascDeletedScreenshots[0] != "old-shot-1" {
		t.Errorf("expected old App Store screenshot deleted before re-upload, got %v", ascDeletedScreenshots)
	}
	if ascVersionCopyright != "© 2026 Example Studio" {
		t.Errorf("App Store copyright = %q, want %q", ascVersionCopyright, "© 2026 Example Studio")
	}
	if len(ascInfoLocs) != 2 || ascInfoLocs["en-US"]["name"] != "Orbit Notes" || ascInfoLocs["zh-Hans"]["name"] != "轨道笔记" {
		t.Fatalf("unexpected App Store appInfoLocalizations: %+v", ascInfoLocs)
	}
	if ascInfoLocs["en-US"]["subtitle"] != "Fast local-first notes" || ascInfoLocs["zh-Hans"]["subtitle"] != "快速本地优先笔记" {
		t.Fatalf("unexpected App Store subtitles: %+v", ascInfoLocs)
	}
	if ascVerLocs["en-US"]["keywords"] != "notes,markdown,offline" || ascVerLocs["zh-Hans"]["keywords"] != "笔记,Markdown,离线" {
		t.Fatalf("unexpected App Store keywords: %+v", ascVerLocs)
	}
	if ascVerLocs["en-US"]["description"] != "Orbit Notes keeps your ideas fast and local.\n\nSync when you want, work offline always." {
		t.Fatalf("unexpected App Store en-US description: %q", ascVerLocs["en-US"]["description"])
	}
	if ascVerLocs["en-US"]["promotionalText"] != "New: instant Markdown previews!" {
		t.Fatalf("unexpected App Store en-US promotionalText: %q", ascVerLocs["en-US"]["promotionalText"])
	}
	if ascVerLocs["zh-Hans"]["whatsNew"] != "优化搜索速度并修复深色模式边框。" {
		t.Fatalf("unexpected App Store zh-Hans whatsNew: %q", ascVerLocs["zh-Hans"]["whatsNew"])
	}
	// Verify all 3 App Store media assets (2 screenshots + 1 preview) uploaded & committed with valid MD5.
	if len(ascUploadedBytes) != 3 || len(ascCommittedMD5) != 3 {
		t.Fatalf("expected 3 App Store media assets uploaded & committed, got uploaded=%d committed=%d", len(ascUploadedBytes), len(ascCommittedMD5))
	}
	for id, data := range ascUploadedBytes {
		sum := md5.Sum(data)
		wantMD5 := hex.EncodeToString(sum[:])
		if ascCommittedMD5[id] != wantMD5 {
			t.Errorf("asset %s MD5 = %q, want %q", id, ascCommittedMD5[id], wantMD5)
		}
	}

	// Verify Google Play received both en-US and zh-CN (mapped from zh-Hans), images, release notes, validate & commit.
	if !playValidated || !playCommitted {
		t.Fatalf("Google Play edit validated=%v committed=%v, want both true", playValidated, playCommitted)
	}
	if playDetails["defaultLanguage"] != "en-US" || playDetails["contactEmail"] != "support@example.com" {
		t.Fatalf("unexpected Google Play details: %+v", playDetails)
	}
	if len(playListings) != 2 {
		t.Fatalf("expected 2 Google Play listings, got %d: %+v", len(playListings), playListings)
	}
	if playListings["en-US"]["title"] != "Orbit Notes" || playListings["en-US"]["shortDescription"] != "Fast local-first notes built with NSGo." {
		t.Fatalf("unexpected Google Play en-US listing: %+v", playListings["en-US"])
	}
	if playListings["zh-CN"]["title"] != "轨道笔记" || playListings["zh-CN"]["shortDescription"] != "用 NSGo 构建的快速本地优先笔记应用。" {
		t.Fatalf("unexpected Google Play zh-CN listing: %+v", playListings["zh-CN"])
	}
	if len(playImages["en-US/phoneScreenshots"]) != 1 || len(playImages["en-US/featureGraphic"]) != 1 || len(playImages["en-US/icon"]) != 1 || len(playImages["zh-CN/phoneScreenshots"]) != 1 {
		t.Fatalf("unexpected Google Play uploaded images: %v", playImages)
	}
	if playDeletedImages["en-US/phoneScreenshots"] != 1 || playDeletedImages["zh-CN/phoneScreenshots"] != 1 {
		t.Fatalf("expected Google Play old screenshots deleted before upload: %+v", playDeletedImages)
	}
	if len(playTrackReleaseNotes) != 2 {
		t.Fatalf("expected 2 Google Play track release notes, got %+v", playTrackReleaseNotes)
	}
}

func TestStoreListingsValidationFailsBeforeUpload(t *testing.T) {
	cases := []struct {
		name    string
		store   string
		mutate  func(t *testing.T, dir string)
		wantErr string
	}{
		{
			name:  "appstore title over 30 ASCII chars",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"title: Orbit Notes", "title: "+strings.Repeat("A", 31))
			},
			wantErr: "app_store title is 31 characters (max 30)",
		},
		{
			name:  "appstore title over 30 CJK runes",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/zh-Hans/listing.yaml"),
					"title: 轨道笔记", "title: "+strings.Repeat("字", 31))
			},
			wantErr: "app_store title is 31 characters (max 30)",
		},
		{
			name:  "appstore subtitle over 30 chars",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"subtitle: Fast local-first notes", "subtitle: "+strings.Repeat("S", 31))
			},
			wantErr: "app_store subtitle is 31 characters (max 30)",
		},
		{
			name:  "appstore keywords over 100 chars",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/zh-Hans/listing.yaml"),
					`keywords: "笔记,Markdown,离线"`, `keywords: "`+strings.Repeat("k", 101)+`"`)
			},
			wantErr: "app_store keywords is 101 characters (max 100)",
		},
		{
			name:  "appstore promotional_text over 170 chars",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "store/en-US/promotional_text.md"), []byte(strings.Repeat("p", 171)), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "app_store promotional_text is 171 characters (max 170)",
		},
		{
			name:  "appstore description over 4000 chars",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "store/en-US/description.md"), []byte(strings.Repeat("d", 4001)), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "app_store description is 4001 characters (max 4000)",
		},
		{
			name:  "appstore missing support_url",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"support_url: https://example.com/support", "")
			},
			wantErr: "app_store support_url is required",
		},
		{
			name:  "appstore invalid support_url scheme",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"support_url: https://example.com/support", "support_url: ftp://example.com")
			},
			wantErr: "must use http or https",
		},
		{
			name:  "google play title over 30 chars",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"title: Orbit Notes", "title: "+strings.Repeat("P", 31))
			},
			wantErr: "google_play title is 31 characters (max 30)",
		},
		{
			name:  "google play short_description over 80 chars",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"short_description: Fast local-first notes built with NSGo.",
					"short_description: "+strings.Repeat("x", 81))
			},
			wantErr: "google_play short_description is 81 characters (max 80)",
		},
		{
			name:  "google play full description over 4000 chars",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "store/en-US/description.md"), []byte(strings.Repeat("g", 4001)), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "google_play description is 4001 characters (max 4000)",
		},
		{
			name:  "google play release_notes over 500 chars",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				if err := os.WriteFile(filepath.Join(dir, "store/en-US/release_notes.md"), []byte(strings.Repeat("r", 501)), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "google_play release_notes is 501 characters (max 500)",
		},
		{
			name:  "unknown YAML key in top-level listing.yaml",
			store: "all",
			mutate: func(t *testing.T, dir string) {
				appendToFile(t, filepath.Join(dir, "store/listing.yaml"), "\nextra_unknown_field: true\n")
			},
			wantErr: "extra_unknown_field",
		},
		{
			name:  "unknown YAML key in locale listing.yaml",
			store: "all",
			mutate: func(t *testing.T, dir string) {
				appendToFile(t, filepath.Join(dir, "store/en-US/listing.yaml"), "\nmade_up_key: oops\n")
			},
			wantErr: "made_up_key",
		},
		{
			name:  "missing version in top-level listing.yaml",
			store: "all",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/listing.yaml"), "version: 1", "")
			},
			wantErr: "version must be 1",
		},
		{
			name:  "unsupported version in locale listing.yaml",
			store: "all",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"), "version: 1", "version: 2")
			},
			wantErr: "version must be 1",
		},
		{
			name:  "missing required short_description for Play",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"short_description: Fast local-first notes built with NSGo.", "")
			},
			wantErr: "google_play short_description is required",
		},
		{
			name:  "missing required description",
			store: "all",
			mutate: func(t *testing.T, dir string) {
				if err := os.Remove(filepath.Join(dir, "store/en-US/description.md")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "description is required",
		},
		{
			name:  "both inline YAML description and description.md",
			store: "all",
			mutate: func(t *testing.T, dir string) {
				appendToFile(t, filepath.Join(dir, "store/en-US/listing.yaml"), "\ndescription: inline conflict\n")
			},
			wantErr: "cannot specify both description in listing.yaml and description.md",
		},
		{
			name:  "unsupported locale directory",
			store: "all",
			mutate: func(t *testing.T, dir string) {
				badDir := filepath.Join(dir, "store/xx-ZZ")
				if err := os.MkdirAll(badDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(badDir, "listing.yaml"), []byte("version: 1\ntitle: X\nshort_description: Y\ndescription: Z\nsupport_url: https://example.com\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: `unsupported App Store locale "xx-ZZ"`,
		},
		{
			name:  "duplicate mapped Google Play locale",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				dupDir := filepath.Join(dir, "store/zh-CN")
				if err := os.MkdirAll(dupDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dupDir, "listing.yaml"), []byte("version: 1\ntitle: 重复\nshort_description: 重复\ndescription: 重复\n"), 0o644); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: `Google Play locale "zh-CN" duplicates`,
		},
		{
			name:  "missing screenshot file",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				if err := os.Remove(filepath.Join(dir, "store/en-US/screenshots/iphone-1.png")); err != nil {
					t.Fatal(err)
				}
			},
			wantErr: "no such file or directory",
		},
		{
			name:  "path traversal escaping listings dir",
			store: "appstore",
			mutate: func(t *testing.T, dir string) {
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"- screenshots/iphone-1.png", "- ../../../nsgo.yaml")
			},
			wantErr: "escapes the listings directory",
		},
		{
			name:  "too many Google Play screenshots",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				var items []string
				for range 9 {
					items = append(items, "      - screenshots/phone-1.png")
				}
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"      - screenshots/phone-1.png", strings.Join(items, "\n"))
			},
			wantErr: "has 9 files (max 8)",
		},
		{
			name:  "unsupported screenshot extension",
			store: "play",
			mutate: func(t *testing.T, dir string) {
				gifPath := filepath.Join(dir, "store/en-US/screenshots/anim.gif")
				if err := os.WriteFile(gifPath, []byte("GIF89a"), 0o644); err != nil {
					t.Fatal(err)
				}
				replaceInFile(t, filepath.Join(dir, "store/en-US/listing.yaml"),
					"- screenshots/phone-1.png", "- screenshots/anim.gif")
			},
			wantErr: `unsupported extension ".gif"`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			appDir := writeValidTwoLanguageListings(t, t.TempDir())
			tc.mutate(t, appDir)

			var ascCalls, playCalls atomic.Int64
			ascServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				ascCalls.Add(1)
				t.Errorf("unexpected App Store HTTP request %s %s after validation should have failed", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer ascServer.Close()
			playServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				playCalls.Add(1)
				t.Errorf("unexpected Google Play HTTP request %s %s after validation should have failed", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer playServer.Close()

			err := publishWithContext(t.Context(), publishOptions{
				store:            tc.store,
				appStoreClient:   &appStoreClient{baseURL: ascServer.URL, httpClient: ascServer.Client()},
				googlePlayClient: &googlePlayClient{baseURL: playServer.URL, httpClient: playServer.Client()},
			}, appDir)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
			}
			if ascCalls.Load() != 0 || playCalls.Load() != 0 {
				t.Fatalf("upload requests were sent despite validation failure: asc=%d play=%d", ascCalls.Load(), playCalls.Load())
			}
		})
	}
}

func TestStoreListingsDryRunNeedsNoCredentials(t *testing.T) {
	appDir := writeValidTwoLanguageListings(t, t.TempDir())
	t.Setenv("NSGO_APP_STORE_KEY", "")
	t.Setenv("NSGO_APP_STORE_KEY_ID", "")
	t.Setenv("NSGO_APP_STORE_ISSUER", "")
	t.Setenv("NSGO_NOTARY_KEY", "")
	t.Setenv("NSGO_NOTARY_KEY_ID", "")
	t.Setenv("NSGO_NOTARY_ISSUER", "")
	t.Setenv("NSGO_PLAY_SERVICE_ACCOUNT", "")

	if err := publishWithContext(t.Context(), publishOptions{
		store:  "all",
		dryRun: true,
	}, appDir); err != nil {
		t.Fatalf("dry-run failed: %v", err)
	}
}

func TestStoreListingsCredentialsAndAPIErrors(t *testing.T) {
	appDir := writeValidTwoLanguageListings(t, t.TempDir())

	t.Run("flag conflict between cert and store", func(t *testing.T) {
		err := publishWithContext(t.Context(), publishOptions{cert: "release.cert", store: "appstore"}, appDir)
		if err == nil || !strings.Contains(err.Error(), "cannot be combined") {
			t.Fatalf("got %v, want flag conflict error", err)
		}
	})

	t.Run("unknown store target", func(t *testing.T) {
		err := publishWithContext(t.Context(), publishOptions{store: "steam"}, appDir)
		if err == nil || !strings.Contains(err.Error(), "unknown -store") {
			t.Fatalf("got %v, want unknown -store error", err)
		}
	})

	t.Run("missing App Store credentials", func(t *testing.T) {
		t.Setenv("NSGO_APP_STORE_KEY", "")
		t.Setenv("NSGO_NOTARY_KEY", "")
		err := publishWithContext(t.Context(), publishOptions{store: "appstore"}, appDir)
		if err == nil || !strings.Contains(err.Error(), "NSGO_APP_STORE_KEY") {
			t.Fatalf("got %v, want missing App Store credentials error", err)
		}
	})

	t.Run("fallback to NSGO_NOTARY env vars", func(t *testing.T) {
		t.Setenv("NSGO_APP_STORE_KEY", "")
		t.Setenv("NSGO_NOTARY_KEY", "/tmp/key.p8")
		t.Setenv("NSGO_NOTARY_KEY_ID", "ABC123")
		t.Setenv("NSGO_NOTARY_ISSUER", "ISS456")
		k, id, iss, err := appStoreCredentialsFromEnv()
		if err != nil || k != "/tmp/key.p8" || id != "ABC123" || iss != "ISS456" {
			t.Fatalf("got (%q, %q, %q, %v)", k, id, iss, err)
		}
	})

	t.Run("missing Google Play credentials", func(t *testing.T) {
		t.Setenv("NSGO_PLAY_SERVICE_ACCOUNT", "")
		err := publishWithContext(t.Context(), publishOptions{store: "play"}, appDir)
		if err == nil || !strings.Contains(err.Error(), "NSGO_PLAY_SERVICE_ACCOUNT") {
			t.Fatalf("got %v, want missing Play credentials error", err)
		}
	})

	t.Run("App Store API error surfaces detail", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"errors":[{"status":"409","code":"STATE_ERROR","title":"Cannot edit","detail":"Version is locked for review"}]}`))
		}))
		defer srv.Close()
		err := publishWithContext(t.Context(), publishOptions{
			store:          "appstore",
			appStoreClient: &appStoreClient{baseURL: srv.URL, httpClient: srv.Client()},
		}, appDir)
		if err == nil || !strings.Contains(err.Error(), "Version is locked for review") {
			t.Fatalf("got %v, want App Store API error detail", err)
		}
	})

	t.Run("Google Play API error surfaces message", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":{"code":403,"status":"PERMISSION_DENIED","message":"The caller does not have permission"}}`))
		}))
		defer srv.Close()
		err := publishWithContext(t.Context(), publishOptions{
			store:            "play",
			googlePlayClient: &googlePlayClient{baseURL: srv.URL, httpClient: srv.Client()},
		}, appDir)
		if err == nil || !strings.Contains(err.Error(), "The caller does not have permission") {
			t.Fatalf("got %v, want Google Play API error message", err)
		}
	})
}

func replaceInFile(t *testing.T, path, oldStr, newStr string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), oldStr) {
		t.Fatalf("%s does not contain %q", path, oldStr)
	}
	updated := strings.Replace(string(data), oldStr, newStr, 1)
	if err := os.WriteFile(path, []byte(updated), 0o644); err != nil {
		t.Fatal(err)
	}
}

func appendToFile(t *testing.T, path, suffix string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, []byte(suffix)...), 0o644); err != nil {
		t.Fatal(err)
	}
}
