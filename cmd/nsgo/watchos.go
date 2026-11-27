package main

import (
	"archive/zip"
	"context"
	_ "embed"
	"encoding/binary"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"
)

// An Apple Watch app is a SwiftUI app: watchOS has no public UIKit, so
// its views are SwiftUI's, which the host in watch/Host.swift shows as
// internal/platform/watchos tells it. The tool compiles the host with
// Xcode's swiftc and links it into the app's Go executable, whose main
// runs the host's app as an iPhone app's runs UIKit's. Go builds arm64
// alone, not arm64_32, so the app runs on the watches watchOS 26 runs as
// arm64: Series 9, Ultra 2 and later (decided by the user, 2026-11-09).

//go:embed watch/Host.swift
var watchHost []byte

var watchOS = &appleOS{name: "watchOS", target: "watchos", sdk: "watchos", minimum: "26.0", device: "Apple Watch",
	families: []int{4}, idioms: []string{"watch"}, platform: "Watch",
	deployment: "WATCHOS_DEPLOYMENT_TARGET", plist: watchosPlist}

// buildWatchApp writes <dir>/<name>.app for watchOS's simulator or
// devices: the app's Go code with the host linked in, its resources, icon
// and Info.plist. Device builds are left unsigned for the developer's own
// identity, as iOS's are.
func buildWatchApp(ctx context.Context, p *project, dir string, simulator bool) (string, error) {
	sys := watchOS
	if _, err := p.appleMinimum(sys); err != nil {
		return "", err
	}
	env, err := iosProjectEnv(ctx, p, sys, simulator)
	if err != nil {
		return "", err
	}
	host, err := compileWatchHost(ctx, simulator)
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(filepath.Dir(host))
	sdkName := sys.sdkName(simulator)
	sdk, err := output(ctx, "xcrun", "--sdk", sdkName, "--show-sdk-path")
	if err != nil {
		return "", err
	}
	app := filepath.Join(dir, p.Name+".app")
	if err := os.RemoveAll(app); err != nil {
		return "", err
	}
	// Go's runtime objects target a newer OS than the minimum; the
	// linker's warning about it is noise. The host's object names the
	// Swift libraries it needs, which the system holds.
	link := fmt.Sprintf("-extldflags '-Wl,-w %s -L%s'", host, filepath.Join(sdk, "usr/lib/swift"))
	if err := goBuildWith(ctx, p, filepath.Join(app, p.Name), env, buildExtra{tags: []string{"nsgowatch"}}, link); err != nil {
		return "", err
	}
	stage(ctx, "bundling the app")
	if err := copyMobileResources(p, app); err != nil {
		return "", err
	}
	icon, err := writeWatchIcon(ctx, p, app, simulator)
	if err != nil {
		return "", err
	}
	names, err := writeAppleNames(p, app)
	if err != nil {
		return "", err
	}
	if p.iosExtension() {
		detailIn(ctx, "no widgets or Live Activities on watchOS: they are left out")
	}
	if p.Apple.Purchases {
		detailIn(ctx, "no in-app purchases on watchOS: StoreKit's library is left out")
	}
	extra := icon + "\n" + names + "\n\t<key>CFBundleSupportedPlatforms</key><array><string>" + sys.platformName(simulator) + "</string></array>"
	buildInfo, err := appleBuildPlist(ctx, p, sys, simulator)
	if err != nil {
		return "", err
	}
	extra += "\n" + buildInfo
	if err := writePlist(filepath.Join(app, "Info.plist"), p, sys.plist, extra); err != nil {
		return "", err
	}
	if err := p.writePrivacyManifest(app); err != nil {
		return "", err
	}
	if !simulator {
		return app, nil
	}
	stage(ctx, "signing")
	return app, run(ctx, "codesign", "--force", "--sign", "-", app)
}

// compileWatchHost compiles the host into an object in a directory of its
// own, which the caller removes, and returns the object's path.
func compileWatchHost(ctx context.Context, simulator bool) (string, error) {
	stage(ctx, "compiling the watch host")
	tmp, err := os.MkdirTemp("", "nsgo-watch-")
	if err != nil {
		return "", err
	}
	src, obj := filepath.Join(tmp, "Host.swift"), filepath.Join(tmp, "Host.o")
	if err := os.WriteFile(src, watchHost, 0o644); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	sdkName := watchOS.sdkName(simulator)
	sdk, err := output(ctx, "xcrun", "--sdk", sdkName, "--show-sdk-path")
	if err != nil {
		os.RemoveAll(tmp)
		return "", fmt.Errorf("the watchOS SDK needs Xcode: %w", err)
	}
	if err := run(ctx, "xcrun", "--sdk", sdkName, "swiftc", "-c", "-target", watchOS.triple(simulator), "-sdk", sdk,
		"-O", "-parse-as-library", "-module-name", "NSGoWatch", src, "-o", obj); err != nil {
		os.RemoveAll(tmp)
		return "", err
	}
	return obj, nil
}

// writeWatchIcon compiles the app's icon into a watch bundle: Icon.WatchOS
// when the project has one, else the Icon Composer .icon macOS and iOS
// use, else the PNG through an asset catalog, which watchOS masks to a
// circle.
func writeWatchIcon(ctx context.Context, p *project, app string, simulator bool) (string, error) {
	minimum, err := p.appleMinimum(watchOS)
	if err != nil {
		return "", err
	}
	sys := watchOS
	sdk := sys.sdkName(simulator)
	icon := p.Icon.WatchOS
	if icon == "" {
		if filepath.Ext(p.Icon.Apple) == ".icon" {
			icon = p.Icon.Apple
		} else {
			icon = p.Icon.PNG
		}
	}
	switch filepath.Ext(icon) {
	case "":
		return "", nil
	case ".icon":
		return compileIcon(ctx, icon, app, sdk, minimum, sys.idioms)
	}
	img, err := readSquareIcon(icon)
	if err != nil {
		return "", err
	}
	tmp, err := os.MkdirTemp("", "nsgo-icon-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	catalog := filepath.Join(tmp, "Assets.xcassets")
	if err := writeDir(filepath.Join(catalog, "AppIcon.appiconset"), map[string][]byte{
		"Icon.png":      scaledPNG(img, 1024),
		"Contents.json": []byte(`{"images":[{"filename":"Icon.png","idiom":"universal","platform":"watchos","size":"1024x1024"}],` + catalogInfo + `}`),
	}); err != nil {
		return "", err
	}
	return compileIcon(ctx, catalog, app, sdk, minimum, sys.idioms)
}

// watchosPlist is a watch-only app's: watchOS shows it dark whatever the
// app asks for.
var watchosPlist = template.Must(template.New("watchos").Parse(plistHead + `	<key>MinimumOSVersion</key><string>{{.WatchOSMinimum}}</string>
	<key>UIDeviceFamily</key><array><integer>4</integer></array>
	<key>WKApplication</key><true/>
	<key>WKWatchOnly</key><true/>
</dict>
</plist>
`))

const (
	// watchAppBundleSuffix is the nested bundle ID suffix Apple's watch-only
	// template uses under the iOS stub container's bundle ID.
	watchAppBundleSuffix = ".watchkitapp"
	// watchStubBinaryRelPath and watchStubAssetsRelPath are Xcode's official
	// watchapp2-container stub executable and icon catalog under iPhoneOS.platform.
	watchStubBinaryRelPath = "Library/Application Support/MessagesApplicationStub/MessagesApplicationStub"
	watchStubAssetsRelPath = "Library/Application Support/MessagesApplicationStub/MessagesApplicationStub.xcassets"
	// watchKitWKRelPath is Apple's WatchKitSupport2/WK binary inside WatchOS.sdk.
	watchKitWKRelPath = "Library/Application Support/WatchKit/WK"
)

// watchAppBundleID returns the nested watchOS bundle identifier inside a
// watch-only App Store IPA (<id>.watchkitapp).
func watchAppBundleID(p *project) string {
	return p.ID + watchAppBundleSuffix
}

// packageWatchStoreIPA wraps the built watchOS app bundle in Xcode's iOS
// watch-only stub (com.apple.product-type.application.watchapp2-container),
// signs both bundles inside-out when provisioning profiles are configured, and
// packs <dir>/<name>.ipa with Payload/<name>.app and WatchKitSupport2/WK.
// The standalone watchOS <dir>/<name>.app stays untouched for -run installs.
func packageWatchStoreIPA(ctx context.Context, p *project, watchApp, dir string) (string, error) {
	work, err := os.MkdirTemp("", "nsgo-watch-ipa-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(work)
	stubApp, nestedWatchApp, err := buildWatchStubApp(ctx, p, watchApp, work)
	if err != nil {
		return "", err
	}
	if err := signWatchStore(ctx, p, stubApp, nestedWatchApp); err != nil {
		return "", err
	}
	stage(ctx, "packaging the watchOS IPA")
	wkPath, err := watchKitSupportWK(ctx)
	if err != nil {
		return "", err
	}
	ipa := filepath.Join(dir, p.Name+".ipa")
	if err := writeWatchIPA(stubApp, wkPath, ipa); err != nil {
		return "", err
	}
	return ipa, nil
}

// buildWatchStubApp builds the iOS watch-only stub wrapper bundle in dir and
// nests watchApp under Watch/<name>.app with bundle ID <id>.watchkitapp.
func buildWatchStubApp(ctx context.Context, p *project, watchApp, dir string) (stubApp, nestedWatchApp string, err error) {
	stage(ctx, "bundling the watchOS App Store stub")
	iphonePlatform, err := output(ctx, "xcrun", "--sdk", iOS.sdk, "--show-sdk-platform-path")
	if err != nil {
		return "", "", fmt.Errorf("the iOS platform stub needs Xcode: %w", err)
	}
	stubSrc := filepath.Join(iphonePlatform, filepath.FromSlash(watchStubBinaryRelPath))
	if _, err := os.Stat(stubSrc); err != nil {
		return "", "", fmt.Errorf("Xcode is missing the watch-only iOS stub at %s: %w", stubSrc, err)
	}
	stubApp = filepath.Join(dir, p.Name+".app")
	if err := os.MkdirAll(stubApp, 0o755); err != nil {
		return "", "", err
	}
	if err := copyWatchStubBinary(ctx, stubSrc, filepath.Join(stubApp, p.Name)); err != nil {
		return "", "", err
	}
	var iconPlist string
	stubAssets := filepath.Join(iphonePlatform, filepath.FromSlash(watchStubAssetsRelPath))
	if info, statErr := os.Stat(stubAssets); statErr == nil && info.IsDir() {
		iconPlist, err = compileAssets(ctx, []string{stubAssets}, "MessagesApplicationStub", stubApp, iOS.sdk, iOS.minimum, iOS.idioms)
		if err != nil {
			return "", "", err
		}
	}
	namesPlist, err := writeAppleNames(p, stubApp)
	if err != nil {
		return "", "", err
	}
	if err := writeWatchStubPlist(ctx, p, stubApp, iconPlist, namesPlist); err != nil {
		return "", "", err
	}
	if err := p.writePrivacyManifest(stubApp); err != nil {
		return "", "", err
	}
	nestedWatchApp = filepath.Join(stubApp, "Watch", filepath.Base(watchApp))
	if err := copyBundleDir(watchApp, nestedWatchApp); err != nil {
		return "", "", err
	}
	if err := rewriteNestedWatchPlist(nestedWatchApp, watchAppBundleID(p)); err != nil {
		return "", "", err
	}
	return stubApp, nestedWatchApp, nil
}

// copyWatchStubBinary writes the arm64 slice of Xcode's universal stub binary
// to dst, matching Xcode's THIN_PRODUCT_STUB_BINARY / EXCLUDED_ARCHS=arm64e.
// A single-architecture binary is copied verbatim.
func copyWatchStubBinary(ctx context.Context, src, dst string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	var magic [4]byte
	n, readErr := f.Read(magic[:])
	f.Close()
	if readErr != nil && n == 0 {
		return readErr
	}
	if n == 4 && isMachOFatMagic(magic) {
		if err := run(ctx, "lipo", "-thin", "arm64", src, "-output", dst); err != nil {
			return err
		}
		return os.Chmod(dst, 0o755)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o755)
}

func isMachOFatMagic(b [4]byte) bool {
	v := binary.BigEndian.Uint32(b[:])
	return v == 0xcafebabe || v == 0xbebafeca || v == 0xcafebabf || v == 0xbfbabeca
}

// writeWatchStubPlist writes the iOS watch-only container's Info.plist,
// matching Xcode's com.apple.product-type.application.watchapp2-container.
func writeWatchStubPlist(ctx context.Context, p *project, stubApp, iconPlist, namesPlist string) error {
	info := map[string]any{
		"CFBundleExecutable":            p.Name,
		"CFBundleIdentifier":            p.ID,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleName":                  p.Name,
		"CFBundleDisplayName":           p.Name,
		"CFBundlePackageType":           "APPL",
		"CFBundleShortVersionString":    p.Version,
		"CFBundleVersion":               p.BundleVersion(),
		"CFBundleSupportedPlatforms":    []any{iOS.platformName(false)},
		"ITSWatchOnlyContainer":         true,
		"LSApplicationLaunchProhibited": true,
		"LSRequiresIPhoneOS":            true,
		"MinimumOSVersion":              iOS.minimum,
		"UIDeviceFamily":                []any{int64(1), int64(2)},
	}
	buildInfo, err := appleBuildInfo(ctx, iOS, false)
	if err != nil {
		return err
	}
	maps.Copy(info, buildInfo)
	for _, partial := range []string{iconPlist, namesPlist} {
		entries, err := decodePartialPlist(partial)
		if err != nil {
			return err
		}
		maps.Copy(info, entries)
	}
	// App Store Connect reads export compliance keys from the top-level bundle.
	for _, key := range []string{"ITSAppUsesNonExemptEncryption", "ITSEncryptionExportComplianceCode"} {
		if v, ok := p.Apple.Info[key]; ok {
			info[key] = plistValue(v)
		}
	}
	return os.WriteFile(filepath.Join(stubApp, "Info.plist"), encodePlist(info), 0o644)
}

func decodePartialPlist(entries string) (map[string]any, error) {
	entries = strings.TrimSpace(entries)
	if entries == "" {
		return nil, nil
	}
	doc := `<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>` + entries + `</dict></plist>`
	v, err := decodePlist([]byte(doc))
	if err != nil {
		return nil, err
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("partial plist is not a dictionary")
	}
	return m, nil
}

// rewriteNestedWatchPlist updates the nested watchOS bundle's Info.plist for
// the watch-only App Store container: <id>.watchkitapp with WKApplication and
// WKWatchOnly set and no companion app bundle identifier.
func rewriteNestedWatchPlist(watchApp, bundleID string) error {
	plistPath := filepath.Join(watchApp, "Info.plist")
	data, err := os.ReadFile(plistPath)
	if err != nil {
		return err
	}
	v, err := decodePlist(data)
	if err != nil {
		return fmt.Errorf("watch Info.plist: %w", err)
	}
	dict, ok := v.(map[string]any)
	if !ok {
		return errors.New("watch Info.plist is not a dictionary")
	}
	dict["CFBundleIdentifier"] = bundleID
	dict["WKApplication"] = true
	dict["WKWatchOnly"] = true
	delete(dict, "WKCompanionAppBundleIdentifier")
	return os.WriteFile(plistPath, encodePlist(dict), 0o644)
}

// signWatchStore validates provisioning profiles and signs a watch-only App
// Store bundle inside-out: the nested watchOS app first, then the outer iOS
// stub wrapper. Without profiles, both bundles stay unsigned.
func signWatchStore(ctx context.Context, p *project, stubApp, watchApp string) error {
	if p.Sign.Profile == "" && p.Sign.WatchProfile == "" {
		detailIn(ctx, "unsigned: set sign.profile to upload it to App Store Connect")
		return nil
	}
	if p.Sign.Profile == "" {
		return fmt.Errorf("watchOS store signing needs sign.profile for the iOS stub %s alongside sign.watch_profile", p.ID)
	}
	stubData, err := os.ReadFile(p.Sign.Profile)
	if err != nil {
		return err
	}
	stubProfile, err := readProfile(ctx, stubData)
	if err != nil {
		return fmt.Errorf("%s: %w", rel(p.Sign.Profile), err)
	}
	stubEnts, ok := stubProfile["Entitlements"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s has no entitlements", rel(p.Sign.Profile))
	}
	stubAppID, _ := stubEnts["application-identifier"].(string)
	if !profileAllows(stubAppID, p.ID) {
		return fmt.Errorf("sign.profile is for %s, not %s", stubAppID, p.ID)
	}
	stubTeam := teamIDFromEntitlements(stubEnts)
	if stubTeam == "" {
		return fmt.Errorf("%s has no team identifier", rel(p.Sign.Profile))
	}

	watchID := watchAppBundleID(p)
	var (
		watchData    []byte
		watchProfile map[string]any
		watchEnts    map[string]any
		watchAppID   string
		watchTeam    string
	)
	if p.Sign.WatchProfile != "" {
		watchData, err = os.ReadFile(p.Sign.WatchProfile)
		if err != nil {
			return err
		}
		watchProfile, err = readProfile(ctx, watchData)
		if err != nil {
			return fmt.Errorf("%s: %w", rel(p.Sign.WatchProfile), err)
		}
		watchEnts, ok = watchProfile["Entitlements"].(map[string]any)
		if !ok {
			return fmt.Errorf("%s has no entitlements", rel(p.Sign.WatchProfile))
		}
		watchAppID, _ = watchEnts["application-identifier"].(string)
		if !profileAllows(watchAppID, watchID) {
			return fmt.Errorf("sign.watch_profile is for %s, not %s", watchAppID, watchID)
		}
		watchTeam = teamIDFromEntitlements(watchEnts)
		if watchTeam == "" {
			return fmt.Errorf("%s has no team identifier", rel(p.Sign.WatchProfile))
		}
		if watchTeam != stubTeam {
			return fmt.Errorf("sign.watch_profile (team %s) must belong to sign.profile's team %s", watchTeam, stubTeam)
		}
	} else {
		if !profileAllows(stubAppID, watchID) {
			return fmt.Errorf("the watchOS app %s needs sign.watch_profile, its own provisioning profile (sign.profile only covers %s)", watchID, stubAppID)
		}
		watchData = stubData
		watchProfile = stubProfile
		watchEnts = maps.Clone(stubEnts)
		watchAppID = stubAppID
		watchTeam = stubTeam
	}

	watchEnts["application-identifier"] = signedAppID(watchAppID, watchID)
	watchProject := *p
	watchProject.ID = watchID
	if err := watchProject.claimSignInWithApple(watchEnts, watchEnts); err != nil {
		return err
	}
	if err := watchProject.claimAppLinks(watchEnts, watchEnts); err != nil {
		return err
	}
	if err := watchProject.claimCloudEntitlements(watchEnts, watchEnts, watchTeam); err != nil {
		return err
	}

	// The iOS watch-only stub (watchapp2-container) declares no capabilities of
	// its own; strip capability entitlements that belong to the watch app.
	stubEnts["application-identifier"] = signedAppID(stubAppID, p.ID)
	for _, key := range []string{
		signInWithAppleEntitlement,
		associatedDomainsEntitlement,
		kvStoreEntitlement,
		icloudContainersEntitlement,
		icloudServicesEntitlement,
		"com.apple.security.application-groups",
	} {
		delete(stubEnts, key)
	}

	identity := p.Sign.Identity
	if identity == "" {
		if p.Sign.WatchProfile != "" {
			identity, err = profileSharedIdentity(ctx, stubProfile, watchProfile)
		} else {
			identity, err = profileIdentity(ctx, stubProfile)
		}
		if err != nil {
			return err
		}
	}

	if err := os.WriteFile(filepath.Join(watchApp, "embedded.mobileprovision"), watchData, 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(stubApp, "embedded.mobileprovision"), stubData, 0o644); err != nil {
		return err
	}
	watchEntsPath, err := writeEntitlements(watchEnts)
	if err != nil {
		return err
	}
	defer os.Remove(watchEntsPath)
	stubEntsPath, err := writeEntitlements(stubEnts)
	if err != nil {
		return err
	}
	defer os.Remove(stubEntsPath)

	detailIn(ctx, "sign with %s", identity)
	stage(ctx, "signing")
	// Sign inside-out: nested watch app frameworks and bundle before the outer iOS stub.
	if err := signFrameworks(ctx, watchApp, identity); err != nil {
		return err
	}
	if err := run(ctx, "codesign", "--force", "--sign", identity, "--entitlements", watchEntsPath, "--generate-entitlement-der", watchApp); err != nil {
		return err
	}
	return run(ctx, "codesign", "--force", "--sign", identity, "--entitlements", stubEntsPath, "--generate-entitlement-der", stubApp)
}

// watchKitSupportWK returns Xcode's WatchKitSupport2/WK binary path when
// present in the WatchOS SDK, or "" when absent.
func watchKitSupportWK(ctx context.Context) (string, error) {
	sdk, err := output(ctx, "xcrun", "--sdk", watchOS.sdk, "--show-sdk-path")
	if err != nil {
		return "", fmt.Errorf("the watchOS SDK needs Xcode: %w", err)
	}
	wk := filepath.Join(sdk, filepath.FromSlash(watchKitWKRelPath))
	if info, err := os.Stat(wk); err == nil && !info.IsDir() {
		return wk, nil
	}
	return "", nil
}

// writeWatchIPA packs stubApp under Payload/<name>.app and, when watchKitWK is
// non-empty, copies it verbatim to WatchKitSupport2/WK at the IPA root.
func writeWatchIPA(stubApp, watchKitWK, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	prefix := "Payload/" + filepath.Base(stubApp)
	if err := addZipDir(zw, stubApp, prefix); err != nil {
		return err
	}
	if watchKitWK != "" {
		info, err := os.Stat(watchKitWK)
		if err != nil {
			return err
		}
		dirHeader := &zip.FileHeader{Name: "WatchKitSupport2/", Method: zip.Store}
		dirHeader.SetMode(os.ModeDir | 0o755)
		if _, err := zw.CreateHeader(dirHeader); err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = "WatchKitSupport2/WK"
		h.Method = zip.Deflate
		h.SetMode(0o755)
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(watchKitWK)
		if err != nil {
			return err
		}
		if _, err := w.Write(data); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

func addZipDir(zw *zip.Writer, dir, prefix string) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		name := path.Join(prefix, filepath.ToSlash(relPath))
		if name == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = name
		if d.IsDir() {
			h.Name += "/"
			_, err = zw.CreateHeader(h)
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err != nil {
				return err
			}
			w, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			_, err = w.Write([]byte(target))
			return err
		}
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
}

// copyBundleDir copies a bundle tree preserving executable modes and symlinks.
func copyBundleDir(src, dst string) error {
	return filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, relPath)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}
