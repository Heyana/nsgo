package main

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// appleUploader checks a build against App Store Connect and delivers it with Xcode's altool, which takes the API key as a .p8 file.
type appleUploader struct {
	store          *appStoreClient
	keyID, issuer  string
	keyFile        string
	removeKeyFiles func()
}

// newAppleUploader reads the App Store Connect key that store listings read, NSGO_APP_STORE_KEY or else the notarization key. A key given as PEM text is written to a private temporary file for altool; close removes it.
func newAppleUploader() (appleUploader, error) {
	key, keyID, issuer, err := appStoreCredentialsFromEnv()
	if err != nil {
		return appleUploader{}, err
	}
	priv, err := loadECDSAPrivateKey(key)
	if err != nil {
		return appleUploader{}, fmt.Errorf("App Store Connect key: %w", err)
	}
	u := appleUploader{
		store: &appStoreClient{
			baseURL:    defaultAppStoreConnectBaseURL,
			httpClient: &http.Client{Timeout: 60 * time.Second},
			tokenFunc: func(context.Context) (string, error) {
				return signAppStoreJWT(priv, keyID, issuer, time.Now())
			},
		},
		keyID:          keyID,
		issuer:         issuer,
		removeKeyFiles: func() {},
	}
	if !strings.Contains(key, "-----BEGIN") {
		if u.keyFile, err = filepath.Abs(key); err != nil {
			return appleUploader{}, err
		}
		return u, nil
	}
	dir, err := os.MkdirTemp("", "nsgo-app-store-key-*")
	if err != nil {
		return appleUploader{}, err
	}
	u.keyFile = filepath.Join(dir, "AuthKey_"+filepath.Base(keyID)+".p8")
	if err := os.WriteFile(u.keyFile, []byte(key), 0o600); err != nil {
		os.RemoveAll(dir)
		return appleUploader{}, err
	}
	u.removeKeyFiles = func() { os.RemoveAll(dir) }
	return u, nil
}

func (u appleUploader) close() { u.removeKeyFiles() }

func (u appleUploader) publish(ctx context.Context, file string, build appleUpload) error {
	if err := u.checkBuild(ctx, build); err != nil {
		return err
	}
	event("uploading %s %s (%s) to App Store Connect and waiting for processing", build.bundle, build.version, build.build)
	cmd := exec.CommandContext(ctx, "xcrun", "altool", "--upload-package", file, "--wait", "--show-progress",
		"--api-key", u.keyID, "--api-issuer", u.issuer, "--p8-file-path", u.keyFile)
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("App Store Connect upload or processing failed: %w", err)
	}
	done("App Store Connect processed %s %s (%s)", build.bundle, build.version, build.build)
	return nil
}

func (u appleUploader) list(ctx context.Context, resource string, query url.Values) ([]ascResource, error) {
	var result ascListResponse
	if err := u.store.doJSON(ctx, http.MethodGet, "/v1/"+resource+"?"+query.Encode(), nil, &result); err != nil {
		return nil, err
	}
	if result.Data == nil {
		return nil, fmt.Errorf("App Store Connect returned no %s list", resource)
	}
	return result.Data, nil
}

func (u appleUploader) checkBuild(ctx context.Context, build appleUpload) error {
	apps, err := u.list(ctx, "apps", url.Values{
		"filter[bundleId]": {build.bundle},
		"fields[apps]":     {"bundleId"},
		"limit":            {"2"},
	})
	if err != nil {
		return err
	}
	if len(apps) != 1 || apps[0].ID == "" || apps[0].Attributes["bundleId"] != build.bundle {
		return fmt.Errorf("App Store Connect has no matching app for %s visible to the API key", build.bundle)
	}
	query := url.Values{
		"filter[app]":                        {apps[0].ID},
		"filter[version]":                    {build.build},
		"filter[preReleaseVersion.platform]": {build.platform},
		"filter[processingState]":            {"PROCESSING,VALID"},
		"fields[builds]":                     {"version"},
		"limit":                              {"1"},
	}
	// macOS build numbers are shared across all versions of an app. iOS starts a new sequence for each app version.
	if build.platform == "IOS" {
		query.Set("filter[preReleaseVersion.version]", build.version)
	}
	builds, err := u.list(ctx, "builds", query)
	if err != nil {
		return err
	}
	if len(builds) != 0 {
		return fmt.Errorf("App Store Connect already has build %s for %s (%s), increment build_number and build the package again", build.build, build.bundle, build.platform)
	}
	return nil
}
