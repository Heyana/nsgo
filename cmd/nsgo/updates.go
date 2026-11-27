package main

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/yuin/goldmark"

	projectcfg "github.com/nsgo-dev/nsgo/project"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// nsgo.yaml's updates have the app update the way its platform and the
// channel it is handed out through do (package updater). On the direct
// channel, for each macOS, Windows, Linux and Android target it releases,
// nsgo build writes appcast-<target>.xml into the releases directory: an
// RSS feed in Sparkle's format whose one item is this version's
// installer, signed with the updates key. macOS packages carry
// Sparkle.framework and Windows ones WinSparkle.dll, which read that feed
// and install; Linux and Android read it themselves. iOS asks the App
// Store on the direct and store channels.

// The channels updates.channel names: the app's own downloads, which
// update themselves; a store, or a package manager, that updates them;
// Steam, which updates depot builds; and none, which checks nothing.
const (
	channelDirect = projectcfg.ChannelDirect
	channelStore  = projectcfg.ChannelStore
	channelSteam  = projectcfg.ChannelSteam
	channelNone   = projectcfg.ChannelNone
)

// readReleaseNotes renders -release-notes' Markdown, if any, into the HTML each appcast's item carries as its description, which Sparkle and WinSparkle show in their update dialogs and the other backends' updater shows as text.
func (p *project) readReleaseNotes() error {
	if p.updatesNotes == "" {
		return nil
	}
	data, err := os.ReadFile(p.updatesNotes)
	if err != nil {
		return fmt.Errorf("-release-notes: %w", err)
	}
	var b bytes.Buffer
	if err := goldmark.Convert(data, &b); err != nil {
		return fmt.Errorf("-release-notes: %w", err)
	}
	p.updatesNotesHTML = strings.TrimSpace(b.String())
	return nil
}

// resolveUpdatesRuntime decodes updates.key and resolves the GitHub repository
// fallback when updates.feed is unset. Keeping this in cmd/nsgo leaves
// project.Load free of Git and network calls.
func (p *project) resolveUpdatesRuntime(ctx context.Context) error {
	if p.Updates.Key == "" {
		return nil
	}
	key, err := pkg.DecodePublicKey(p.Updates.Key)
	if err != nil {
		return fmt.Errorf("updates: key: %w; set it to the public key nsgo keygen prints", err)
	}
	p.updatesPublic = key
	if p.Updates.Feed == "" {
		p.updatesRepo = p.githubRepository(ctx)
	}
	return nil
}

// checkChannel refuses a channel updates.channel or -channel cannot name.
func checkChannel(channel string) error {
	return projectcfg.ValidateChannel(channel)
}

// channel is the channel the builds are handed out through.
func (p *project) channel() string {
	return p.Config.Channel()
}

// updatesOn reports whether the app's release builds check for updates.
func (p *project) updatesOn() bool {
	channel := p.channel()
	return !p.devClient && (channel == channelDirect || channel == channelStore)
}

// updates reports whether the app's builds update themselves, on the
// direct channel: Sparkle and WinSparkle, and the appcasts.
func (p *project) updates() bool { return p.updatesOn() && p.channel() == channelDirect }

// updatesAutomatic reports whether the app checks on its own.
func (p *project) updatesAutomatic() bool { return p.Config.UpdatesAutomatic() }

// feedTarget reports whether target's releases have an appcast.
func feedTarget(target string) bool {
	for _, prefix := range []string{"macos_", "windows_", "linux_", "android_"} {
		if strings.HasPrefix(target, prefix) {
			return true
		}
	}
	return false
}

// appcastName is target's appcast's file name.
func appcastName(target string) string { return "appcast-" + target + ".xml" }

// feedBase is the URL the release's files are under, "" when neither
// updates.feed nor a GitHub repository names one.
func (p *project) feedBase() string {
	if p.Updates.Feed != "" {
		return p.Updates.Feed
	}
	if p.updatesRepo != "" {
		return "https://github.com/" + p.updatesRepo + "/releases/latest/download/"
	}
	return ""
}

// appcastURL is target's appcast, which its builds read; "" for a target
// with none.
func (p *project) appcastURL(target string) string {
	if base := p.feedBase(); base != "" && feedTarget(target) {
		return base + appcastName(target)
	}
	return ""
}

// checkUpdateFeed refuses to build target's updating package without a
// channel nsgo knows, the key the direct one signs with, or a feed for it
// to read.
func (p *project) checkUpdateFeed(target string) error {
	if err := checkChannel(p.Updates.Channel); err != nil {
		return err
	}
	if p.updates() && p.updatesPublic == nil {
		return errors.New("updates: the direct channel signs its installers: set updates.key to the public key nsgo keygen prints")
	}
	if p.updates() && feedTarget(target) && p.feedBase() == "" {
		return errors.New("updates: no feed for the appcasts: set updates.feed, or release.github to the repository whose releases hold them")
	}
	return nil
}

// downloadURL is where a release file of this version is: under the feed,
// or in this version's GitHub release.
func (p *project) downloadURL(file string) string {
	if p.Updates.Feed != "" {
		return p.Updates.Feed + url.PathEscape(file)
	}
	return "https://github.com/" + p.updatesRepo + "/releases/download/" + url.PathEscape(p.tagPrefix()+p.Version) + "/" + url.PathEscape(file)
}

// releasePage is this version's GitHub release, "" with a feed of its own.
func (p *project) releasePage() string {
	if p.Updates.Feed != "" || p.updatesRepo == "" {
		return ""
	}
	return "https://github.com/" + p.updatesRepo + "/releases/tag/" + url.PathEscape(p.tagPrefix()+p.Version)
}

// envTarget names the target a go build's environment builds for.
func envTarget(env []string) string {
	goos, goarch := runtime.GOOS, runtime.GOARCH
	for _, e := range env {
		if v, ok := strings.CutPrefix(e, "GOOS="); ok {
			goos = v
		}
		if v, ok := strings.CutPrefix(e, "GOARCH="); ok {
			goarch = v
		}
	}
	if goos == "darwin" {
		goos = "macos"
	}
	return goos + "_" + goArchName(goarch)
}

// updateLdflags give nsgo.Run the updates of a build for env's target:
// on the direct channel its appcast and key, where the backend reads one
// rather than Sparkle, and whether it checks on its own.
func (p *project) updateLdflags(env []string) []string {
	if p.devClient {
		return nil
	}
	// The backend, which nsgo.Run runs the app with, reads them.
	const pkgPath = "github.com/nsgo-dev/nsgo/internal/backend"
	flags := []string{fmt.Sprintf("-X %s.updateChannel=%s", pkgPath, p.channel())}
	if !p.updatesOn() {
		return flags
	}
	if feed := p.appcastURL(envTarget(env)); feed != "" && p.updates() {
		flags = append(flags, fmt.Sprintf("-X '%s.updateFeed=%s'", pkgPath, feed),
			fmt.Sprintf("-X %s.updateKey=%s", pkgPath, pkg.EncodePublicKey(p.updatesPublic)))
	}
	if p.updatesAutomatic() {
		flags = append(flags, fmt.Sprintf("-X %s.updateAutomatic=1", pkgPath))
	}
	return flags
}

// sparklePlist is what a macOS app's Info.plist tells Sparkle: its
// appcast, the key its downloads are signed with and whether it checks on
// its own. A user's choice in Sparkle's settings wins.
func (p *project) sparklePlist(target string) string {
	if !p.updates() {
		return ""
	}
	automatic := "<false/>"
	if p.updatesAutomatic() {
		automatic = "<true/>"
	}
	downloads := "<false/>"
	if p.Updates.AutomaticDownloads {
		downloads = "<true/>"
	}
	return fmt.Sprintf("\t<key>SUAutomaticallyUpdate</key>%s\n\t<key>SUFeedURL</key><string>%s</string>\n\t<key>SUPublicEDKey</key><string>%s</string>\n\t<key>SUEnableAutomaticChecks</key>%s",
		downloads, xmlText(p.appcastURL(target)), pkg.EncodePublicKey(p.updatesPublic), automatic)
}

// pinned is a release of an updater nsgo bundles, by its SHA-256.
type pinned struct{ name, url, sha256 string }

// The updaters nsgo bundles: Sparkle in macOS packages and WinSparkle in Windows ones, under their MIT licenses. Moving WinSparkle to another release means checking its translations against internal/platform/updates' WinSparkleLanguage.
var (
	sparkleRelease = pinned{"Sparkle-2.10.1.tar.xz",
		"https://github.com/sparkle-project/Sparkle/releases/download/2.10.1/Sparkle-2.10.1.tar.xz",
		"1a856d2a9edb8e628de88121d7be04cf0e2167f8641650b78fca00b14244cf27"}
	winSparkleRelease = pinned{"WinSparkle-0.9.4.zip",
		"https://github.com/vslavik/winsparkle/releases/download/v0.9.4/WinSparkle-0.9.4.zip",
		"6037df37fc263bd1650a1c4949681a9d40ffe991d01f35892a406cb5d103c976"}
)

// The targets build at once (buildPackages), so one at a time downloads
// the updaters and unpacks Sparkle into the user's cache.
var fetching, unpacking sync.Mutex

// fetch returns the release's path in the user's cache, downloading it
// the first time.
func (r pinned) fetch(ctx context.Context) (string, error) {
	fetching.Lock()
	defer fetching.Unlock()
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(cache, "nsgo", "downloads")
	path := filepath.Join(dir, r.name)
	if data, err := os.ReadFile(path); err == nil && r.matches(data) {
		return path, nil
	}
	stage(ctx, "downloading %s", r.name)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%s answered %s", r.url, resp.Status)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if !r.matches(data) {
		return "", fmt.Errorf("%s does not have the SHA-256 nsgo pins", r.url)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, r.name+".*")
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	return path, os.Rename(tmp.Name(), path)
}

func (r pinned) matches(data []byte) bool {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]) == r.sha256
}

// sparkleFramework returns Sparkle.framework, unpacked once from its
// release in the user's cache. Unpacking takes macOS's tar, as the
// framework's links and signatures must stay whole.
func sparkleFramework(ctx context.Context) (string, error) {
	archive, err := sparkleRelease.fetch(ctx)
	if err != nil {
		return "", err
	}
	unpacking.Lock()
	defer unpacking.Unlock()
	dir := strings.TrimSuffix(archive, ".tar.xz")
	framework := filepath.Join(dir, "Sparkle.framework")
	if _, err := os.Stat(filepath.Join(framework, "Sparkle")); err == nil {
		return framework, nil
	}
	tmp, err := os.MkdirTemp(filepath.Dir(archive), "sparkle-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := run(ctx, "tar", "-xJf", archive, "-C", tmp, "./Sparkle.framework"); err != nil {
		return "", err
	}
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, dir); err != nil {
		// Another nsgo unpacked it meanwhile.
		if _, statErr := os.Stat(filepath.Join(framework, "Sparkle")); statErr == nil {
			return framework, nil
		}
		return "", err
	}
	return framework, nil
}

// addSparkle puts Sparkle.framework into an updating macOS app's
// Frameworks, signed ad hoc until signMacApp signs it with the app's
// identity.
func addSparkle(ctx context.Context, p *project, app string) error {
	if !p.updates() {
		return nil
	}
	framework, err := sparkleFramework(ctx)
	if err != nil {
		return fmt.Errorf("updates: Sparkle: %w", err)
	}
	dst := filepath.Join(app, "Contents", "Frameworks", "Sparkle.framework")
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := run(ctx, "ditto", framework, dst); err != nil {
		return err
	}
	return signSparkle(ctx, app, func(path string) []string { return []string{"--force", "--sign", "-", path} })
}

// signSparkle signs the Sparkle.framework in app, if any, with args'
// codesign arguments for each path: its helpers inside out, then the
// framework, as Sparkle's documentation orders them, the downloader
// keeping its entitlements.
func signSparkle(ctx context.Context, app string, args func(path string) []string) error {
	framework := filepath.Join(app, "Contents", "Frameworks", "Sparkle.framework")
	if _, err := os.Stat(framework); err != nil {
		return nil
	}
	version, err := filepath.EvalSymlinks(filepath.Join(framework, "Versions", "Current"))
	if err != nil {
		return err
	}
	for _, part := range []string{"XPCServices/Installer.xpc", "XPCServices/Downloader.xpc", "Autoupdate", "Updater.app"} {
		path := filepath.Join(version, filepath.FromSlash(part))
		if _, err := os.Stat(path); err != nil {
			continue
		}
		a := args(path)
		if part == "XPCServices/Downloader.xpc" {
			a = append(a[:len(a)-1:len(a)-1], "--preserve-metadata=entitlements", path)
		}
		if err := run(ctx, "codesign", a...); err != nil {
			return err
		}
	}
	return run(ctx, "codesign", args(framework)...)
}

// addWinSparkle puts WinSparkle.dll for arch, Go's, beside an updating
// Windows app's executable, from its release in the user's cache.
func addWinSparkle(ctx context.Context, p *project, dir, arch string) error {
	if !p.updates() {
		return nil
	}
	archive, err := winSparkleRelease.fetch(ctx)
	if err != nil {
		return fmt.Errorf("updates: WinSparkle: %w", err)
	}
	name := strings.TrimSuffix(winSparkleRelease.name, ".zip") + "/" + map[string]string{"amd64": "x64", "arm64": "ARM64"}[arch] + "/Release/WinSparkle.dll"
	z, err := zip.OpenReader(archive)
	if err != nil {
		return err
	}
	defer z.Close()
	f, err := z.Open(name)
	if err != nil {
		return fmt.Errorf("updates: %s holds no %s", winSparkleRelease.name, name)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "WinSparkle.dll"), data, 0o644)
}

// updatesSecretKey is the secret key a release's installers are signed
// with, NSGO_UPDATES_KEY's; nil without one. One of another public key
// than updates.key is refused.
func (p *project) updatesSecretKey() (ed25519.PrivateKey, error) {
	text := os.Getenv("NSGO_UPDATES_KEY")
	if text == "" {
		return nil, nil
	}
	key, err := pkg.DecodeSecretKey(text)
	if err != nil {
		return nil, fmt.Errorf("NSGO_UPDATES_KEY: %w", err)
	}
	if !key.Public().(ed25519.PublicKey).Equal(p.updatesPublic) {
		return nil, errors.New("NSGO_UPDATES_KEY: the secret key is not the one of updates.key")
	}
	return key, nil
}

// releaseInstaller moves target's installers, if any, into the releases
// directory with their checksums, and writes the appcast offering them.
func (p *project) releaseInstaller(target string, paths ...string) error {
	if err := p.releaseAssets(paths...); err != nil {
		return err
	}
	var released []string
	for _, path := range paths {
		// AppImage's delta control ships beside it, but is no installer.
		if path != "" && !strings.HasSuffix(path, ".AppImage.zsync") {
			released = append(released, filepath.Join(p.releaseDir(), filepath.Base(path)))
		}
	}
	if len(released) == 0 {
		return nil
	}
	return p.writeAppcast(target, released...)
}

// writeAppcast writes target's appcast into the releases directory, listing
// each installer in order. macOS and Windows install only a signed installer,
// so without the secret key their appcast is left out.
func (p *project) writeAppcast(target string, installers ...string) error {
	if !p.updates() || len(installers) == 0 || !feedTarget(target) {
		return nil
	}
	key, err := p.updatesSecretKey()
	if err != nil {
		return err
	}
	system, _, _ := strings.Cut(target, "_")
	if key == nil && (system == "macos" || system == "windows") {
		warn("no %s: sign the installers with updates.key's secret key, in NSGO_UPDATES_KEY", appcastName(target))
		return nil
	}
	name, err := p.displayName()
	if err != nil {
		return err
	}
	now := time.Now()
	items := make([]appcastItem, 0, len(installers))
	for _, installer := range installers {
		if installer == "" {
			continue
		}
		data, err := os.ReadFile(installer)
		if err != nil {
			return err
		}
		item := appcastItem{name: name, version: p.Version, build: p.BundleVersion(), page: p.releasePage(), notes: p.updatesNotesHTML,
			url: p.downloadURL(filepath.Base(installer)), os: system, length: len(data), sha256: fmt.Sprintf("%x", sha256.Sum256(data)), published: now}
		if key != nil {
			item.signature = base64.StdEncoding.EncodeToString(ed25519.Sign(key, data))
		}
		if system == "windows" {
			// The installer quits the app, installs quietly and starts the new version (windowsinstaller/runtime_windows.go).
			item.arguments = "/S /RELAUNCH"
		}
		items = append(items, item)
	}
	if len(items) == 0 {
		return nil
	}
	path := filepath.Join(p.releaseDir(), appcastName(target))
	if err := writeFile(path, appcastFeed(name, items)); err != nil {
		return err
	}
	return p.releaseAssets(path)
}

// appcastItem is one release package an appcast lists.
type appcastItem struct {
	name, version, build string
	// page is the release's page, url its installer's download and os
	// Sparkle's name of its system.
	page, url, os string
	length        int
	sha256        string
	// signature is the installer's EdDSA signature, in base64; "" unsigned.
	signature string
	// arguments are what WinSparkle runs the installer with.
	arguments string
	// notes is the release's notes in HTML, "" without any.
	notes     string
	published time.Time
}

// appcastFeed writes the Sparkle RSS appcast listing items. Each item's
// versions are elements and the enclosure's attributes both, which Sparkle's
// older readers take.
func appcastFeed(title string, items []appcastItem) []byte {
	var b bytes.Buffer
	b.WriteString("<?xml version=\"1.0\" encoding=\"utf-8\"?>\n")
	b.WriteString("<rss version=\"2.0\" xmlns:sparkle=\"http://www.andymatuschak.org/xml-namespaces/sparkle\">\n  <channel>\n")
	fmt.Fprintf(&b, "    <title>%s</title>\n", xmlText(title))
	for _, it := range items {
		b.WriteString("    <item>\n")
		fmt.Fprintf(&b, "      <title>%s</title>\n", xmlText(it.name+" "+it.version))
		if it.page != "" {
			fmt.Fprintf(&b, "      <link>%s</link>\n", xmlText(it.page))
		}
		if it.notes != "" {
			// CDATA cannot hold its own end, so one in the notes ends a section and starts the next.
			fmt.Fprintf(&b, "      <description><![CDATA[%s]]></description>\n", strings.ReplaceAll(it.notes, "]]>", "]]]]><![CDATA[>"))
		}
		fmt.Fprintf(&b, "      <pubDate>%s</pubDate>\n", it.published.UTC().Format(time.RFC1123Z))
		fmt.Fprintf(&b, "      <sparkle:version>%s</sparkle:version>\n", xmlText(it.build))
		fmt.Fprintf(&b, "      <sparkle:shortVersionString>%s</sparkle:shortVersionString>\n", xmlText(it.version))
		fmt.Fprintf(&b, "      <enclosure url=\"%s\" length=\"%d\" type=\"application/octet-stream\" sparkle:os=\"%s\" sparkle:version=\"%s\" sparkle:shortVersionString=\"%s\" sparkle:sha256=\"%s\"",
			xmlText(it.url), it.length, xmlText(it.os), xmlText(it.build), xmlText(it.version), xmlText(it.sha256))
		if it.signature != "" {
			fmt.Fprintf(&b, " sparkle:edSignature=\"%s\"", it.signature)
		}
		if it.arguments != "" {
			fmt.Fprintf(&b, " sparkle:installerArguments=\"%s\"", xmlText(it.arguments))
		}
		b.WriteString("/>\n    </item>\n")
	}
	b.WriteString("  </channel>\n</rss>\n")
	return b.Bytes()
}
