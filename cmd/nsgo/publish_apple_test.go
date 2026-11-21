package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPublishApplePreflight(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the test stubs xcrun with a shell script")
	}
	dir := t.TempDir()
	xcrun := filepath.Join(dir, "xcrun")
	if err := os.WriteFile(xcrun, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$NSGO_TEST_ALTOOL_ARGS\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatal(err)
	}
	keyFile := filepath.Join(dir, "temporary key.p8")
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: secret}), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, platform, file, wantErr string
		unauthorized, duplicate       bool
		keyText                       bool
	}{
		{name: "iOS", platform: "IOS", file: "Example.ipa"},
		{name: "macOS", platform: "MAC_OS", file: "Example.pkg"},
		{name: "key given as text", platform: "IOS", file: "Example.ipa", keyText: true},
		{name: "reused iOS build", platform: "IOS", file: "Example.ipa", wantErr: "already has build 42", duplicate: true},
		{name: "reused Mac build", platform: "MAC_OS", file: "Example.pkg", wantErr: "already has build 42", duplicate: true},
		{name: "bad credentials", platform: "IOS", file: "Example.ipa", wantErr: "refused the API key", unauthorized: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			invocation := filepath.Join(t.TempDir(), "altool-args")
			t.Setenv("NSGO_TEST_ALTOOL_ARGS", invocation)
			for _, name := range append(appStoreKeyEnv[:], notaryKeyEnv[:]...) {
				t.Setenv(name, "")
			}
			if tc.keyText {
				pemText, err := os.ReadFile(keyFile)
				if err != nil {
					t.Fatal(err)
				}
				t.Setenv(appStoreKeyEnv[0], string(pemText))
				t.Setenv(appStoreKeyEnv[1], "TESTKEY000")
				t.Setenv(appStoreKeyEnv[2], "test-issuer")
			} else {
				t.Setenv(notaryKeyEnv[0], keyFile)
				t.Setenv(notaryKeyEnv[1], "TESTKEY000")
				t.Setenv(notaryKeyEnv[2], "test-issuer")
			}
			u, err := newAppleUploader()
			if err != nil {
				t.Fatal(err)
			}
			service := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				checkAppStoreToken(t, r.Header.Get("Authorization"), key)
				if r.Method != http.MethodGet {
					t.Errorf("method = %s", r.Method)
				}
				q := r.URL.Query()
				switch r.URL.Path {
				case "/v1/apps":
					if q.Get("filter[bundleId]") != "dev.nsgo.example" {
						t.Errorf("app lookup = %v", q)
					}
					if tc.unauthorized {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					fmt.Fprint(w, `{"data":[{"id":"1234","attributes":{"bundleId":"dev.nsgo.example"}}]}`)
				case "/v1/builds":
					version := ""
					if tc.platform == "IOS" {
						version = "1.2.3"
					}
					if tc.unauthorized || q.Get("filter[app]") != "1234" || q.Get("filter[version]") != "42" ||
						q.Get("filter[preReleaseVersion.platform]") != tc.platform || q.Get("filter[preReleaseVersion.version]") != version {
						t.Errorf("build lookup = %v", q)
					}
					if tc.duplicate {
						fmt.Fprint(w, `{"data":[{"id":"build"}]}`)
					} else {
						fmt.Fprint(w, `{"data":[]}`)
					}
				default:
					t.Errorf("unexpected request = %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer service.Close()
			u.store.baseURL, u.store.httpClient = service.URL, service.Client()
			err = u.publish(t.Context(), tc.file, appleUpload{bundle: "dev.nsgo.example", version: "1.2.3", build: "42", platform: tc.platform})
			u.close()
			if tc.keyText {
				if filepath.Base(u.keyFile) != "AuthKey_TESTKEY000.p8" {
					t.Errorf("altool key file = %s", u.keyFile)
				}
				if _, err := os.Stat(u.keyFile); !os.IsNotExist(err) {
					t.Errorf("temporary key file left behind, stat = %v", err)
				}
			}
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("publish error = %v, want %q", err, tc.wantErr)
				}
				if _, err := os.Stat(invocation); !os.IsNotExist(err) {
					t.Errorf("xcrun ran after preflight error, stat = %v", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(invocation)
			want := strings.Join([]string{"altool", "--upload-package", tc.file, "--wait", "--show-progress",
				"--api-key", "TESTKEY000", "--api-issuer", "test-issuer", "--p8-file-path", u.keyFile, ""}, "\n")
			if !tc.keyText && u.keyFile != keyFile {
				t.Errorf("altool key file = %s, want %s", u.keyFile, keyFile)
			}
			if err != nil || string(got) != want {
				t.Errorf("xcrun invocation = %q, %v, want %q", got, err, want)
			}
		})
	}
}

func checkAppStoreToken(t *testing.T, header string, key *ecdsa.PrivateKey) {
	t.Helper()
	token, ok := strings.CutPrefix(header, "Bearer ")
	parts := strings.Split(token, ".")
	if !ok || len(parts) != 3 {
		t.Error("missing App Store Connect bearer JWT")
		return
	}
	sign, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil || len(sign) != 64 {
		t.Error("invalid JWT signature encoding")
		return
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if !ecdsa.Verify(&key.PublicKey, digest[:], new(big.Int).SetBytes(sign[:32]), new(big.Int).SetBytes(sign[32:])) {
		t.Error("invalid JWT signature")
	}
}

func TestReadAppleUploadPackage(t *testing.T) {
	for _, platform := range []string{"IOS", "MAC_OS"} {
		t.Run(platform, func(t *testing.T) {
			dir := t.TempDir()
			app := filepath.Join(dir, "Example.app")
			contents := app
			if platform == "MAC_OS" {
				if runtime.GOOS != "darwin" {
					t.Skip("needs macOS productbuild and pkgutil")
				}
				contents = filepath.Join(app, "Contents")
				if err := os.MkdirAll(filepath.Join(contents, "MacOS"), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(contents, "MacOS", "Example"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
					t.Fatal(err)
				}
			} else if err := os.MkdirAll(contents, 0o755); err != nil {
				t.Fatal(err)
			}
			info := encodePlist(map[string]any{
				"CFBundleIdentifier": "dev.nsgo.example", "CFBundleShortVersionString": "1.2.3", "CFBundleVersion": "42",
				"CFBundleName": "Example", "CFBundleExecutable": "Example", "CFBundlePackageType": "APPL", "CFBundleSupportedPlatforms": []any{"iPhoneOS"},
			})
			if err := os.WriteFile(filepath.Join(contents, "Info.plist"), info, 0o644); err != nil {
				t.Fatal(err)
			}
			var got appleUpload
			var err error
			if platform == "IOS" {
				file := filepath.Join(dir, "Example.ipa")
				if err := writeIPA(app, file); err != nil {
					t.Fatal(err)
				}
				got, err = readAppleIPA(t.Context(), file)
			} else {
				file := filepath.Join(dir, "Example.pkg")
				if err := run(t.Context(), "productbuild", "--component", app, "/Applications", file); err != nil {
					t.Fatal(err)
				}
				got, err = readApplePkg(t.Context(), file)
			}
			want := appleUpload{bundle: "dev.nsgo.example", version: "1.2.3", build: "42", platform: platform}
			if got != want || err != nil {
				t.Fatalf("package metadata = %+v, %v, want %+v", got, err, want)
			}
		})
	}
}
