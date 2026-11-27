package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
)

func publishPlay(ctx context.Context, track, path string) error {
	track = strings.TrimSpace(track)
	if track == "" {
		return errors.New("publish: -target android requires -track")
	}
	path, projectID, err := playBundlePath(path)
	if err != nil {
		return err
	}
	bundle, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open the App Bundle from nsgo build -target android -channel store: %w", err)
	}
	defer bundle.Close()
	info, err := bundle.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("expected an App Bundle file at %s", rel(path))
	}
	identity, err := apk.ReadBundleIdentity(bundle, info.Size())
	if err != nil {
		return fmt.Errorf("read App Bundle %s: %w", rel(path), err)
	}
	if projectID != "" && identity.Package != projectID {
		return fmt.Errorf("App Bundle package %q differs from the project's ID %q", identity.Package, projectID)
	}
	// An App Bundle can take minutes to upload, longer than store listings' 60-second requests.
	client, err := googlePlayClientFromEnv(ctx, &http.Client{
		Timeout: 5 * time.Minute,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	})
	if err != nil {
		return err
	}
	if cred := client.credentialSummary(); cred != "" {
		note("Google Play auth: %s", cred)
	}
	if err := uploadGooglePlayBundle(ctx, client, track, identity, bundle, info.Size()); err != nil {
		return err
	}
	done("committed %s version code %d to Google Play track %s", identity.Package, identity.VersionCode, track)
	return nil
}

func playBundlePath(path string) (bundle, projectID string, err error) {
	if strings.EqualFold(filepath.Ext(path), ".aab") {
		return path, "", nil
	}
	if strings.EqualFold(filepath.Ext(path), ".apk") {
		return "", "", errors.New("Google Play publishing takes an Android App Bundle (.aab)")
	}
	p, err := load(path)
	if err != nil {
		return "", "", err
	}
	var found string
	for _, target := range []string{"android_arm64", "android_x64", "android_universal"} {
		candidate := filepath.Join(p.releaseDir(), p.releaseName(target, ".aab"))
		info, err := os.Stat(candidate)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return "", "", err
		}
		if !info.Mode().IsRegular() {
			continue
		}
		if found != "" {
			return "", "", fmt.Errorf("multiple App Bundles in %s. Pass the .aab file to publish", rel(p.releaseDir()))
		}
		found = candidate
	}
	if found == "" {
		return "", "", fmt.Errorf("no App Bundle in %s for %s %s. Run nsgo build -target android -channel store", rel(p.releaseDir()), p.Name, p.Version)
	}
	return found, p.ID, nil
}

type playTrack struct {
	ID       string        `json:"track"`
	Releases []playRelease `json:"releases"`
}

type playRelease struct {
	VersionCodes []string `json:"versionCodes"`
	Status       string   `json:"status"`
}

// uploadGooglePlayBundle opens an edit, checks the bundle's version code against the track's, uploads the bundle, releases it to the track and commits the edit, deleting the edit when any step fails.
func uploadGooglePlayBundle(ctx context.Context, c *googlePlayClient, track string, app apk.BundleIdentity, bundle io.ReaderAt, size int64) (err error) {
	appPath := "/androidpublisher/v3/applications/" + url.PathEscape(app.Package) + "/edits"
	var edit struct {
		ID string `json:"id"`
	}
	if err := c.doJSON(ctx, http.MethodPost, appPath, struct{}{}, &edit); err != nil {
		return fmt.Errorf("open Google Play edit: %w", err)
	}
	if edit.ID == "" {
		return errors.New("Google Play returned an edit without an ID")
	}
	editPath := appPath + "/" + url.PathEscape(edit.ID)
	defer func() {
		if err != nil {
			cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
			defer cancel()
			c.doJSON(cleanup, http.MethodDelete, editPath, nil, nil)
		}
	}()
	current, err := readGooglePlayTrack(ctx, c, editPath, track)
	if err != nil {
		return fmt.Errorf("read Google Play track %q: %w", track, err)
	}
	var highest int64
	for _, release := range current.Releases {
		for _, code := range release.VersionCodes {
			n, err := strconv.ParseInt(code, 10, 64)
			if err != nil || n <= 0 {
				return fmt.Errorf("Google Play track %q returned an invalid version code %q", track, code)
			}
			highest = max(highest, n)
		}
	}
	if app.VersionCode <= highest {
		return fmt.Errorf("bundle version code %d must exceed Google Play track %q's highest version code %d", app.VersionCode, track, highest)
	}
	uploaded, err := c.uploadBundle(ctx, app.Package, edit.ID, bundle, size)
	if err != nil {
		return fmt.Errorf("upload App Bundle to Google Play: %w", err)
	}
	if uploaded != app.VersionCode {
		return fmt.Errorf("Google Play returned version code %d for bundle version code %d", uploaded, app.VersionCode)
	}
	change := playTrack{ID: current.ID, Releases: []playRelease{{VersionCodes: []string{strconv.FormatInt(uploaded, 10)}, Status: "completed"}}}
	if err := c.doJSON(ctx, http.MethodPut, editPath+"/tracks/"+url.PathEscape(current.ID), change, nil); err != nil {
		return fmt.Errorf("update Google Play track %q: %w", track, err)
	}
	if err := c.doJSON(ctx, http.MethodPost, editPath+":commit", nil, nil); err != nil {
		return fmt.Errorf("commit Google Play edit: %w", err)
	}
	return nil
}

func readGooglePlayTrack(ctx context.Context, c *googlePlayClient, editPath, track string) (playTrack, error) {
	var current playTrack
	path := editPath + "/tracks/"
	trackID := track
	err := c.doJSON(ctx, http.MethodGet, path+url.PathEscape(trackID), nil, &current)
	var response *googlePlayError
	if track == "internal" && errors.As(err, &response) && response.StatusCode == http.StatusNotFound {
		// Google also uses "qa" for internal testing in its track API.
		trackID = "qa"
		err = c.doJSON(ctx, http.MethodGet, path+trackID, nil, &current)
	}
	if err != nil {
		return playTrack{}, err
	}
	if current.ID == "" {
		current.ID = trackID
	}
	if current.ID != track && (track != "internal" || current.ID != "qa") {
		return playTrack{}, fmt.Errorf("Google Play returned a different track %q", current.ID)
	}
	return current, nil
}
