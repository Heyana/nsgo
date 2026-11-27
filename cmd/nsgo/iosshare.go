package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	// Keep the target-specific extension in apps' vendored tool dependencies.
	_ "github.com/nsgo-dev/nsgo/cmd/nsgo/shareextension"
)

const shareAppex = "NSGoShare.appex"
const shareExecutable = "NSGoShare"
const shareGroupKey = "NSGoShareAppGroup"
const shareTypesKey = "NSGoShareTypes"
const shareNotificationKey = "NSGoShareNotification"

func (p *project) iosShares() bool { return len(p.Apple.ShareTypes) > 0 && p.clipParent == "" }
func (p *project) sharePlist() string {
	if !p.iosShares() {
		return ""
	}
	return "<key>" + shareTypesKey + "</key>" + plistStrings(p.Apple.ShareTypes) + "<key>" + shareGroupKey + "</key><string>" + xmlText(p.appGroup()) + "</string><key>" + shareNotificationKey + "</key><string>" + xmlText(p.ID+".nsgo-shared") + "</string>"
}
func (p *project) shareActivation() string {
	var tests []string
	for _, uti := range p.Apple.ShareTypes {
		tests = append(tests, "ANY $attachment.registeredTypeIdentifiers UTI-CONFORMS-TO '"+uti+"'")
	}
	return "extensionItems.@count > 0 AND SUBQUERY(extensionItems, $item, $item.attachments.@count > 0 AND SUBQUERY($item.attachments, $attachment, " + strings.Join(tests, " OR ") + ").@count == $item.attachments.@count).@count == extensionItems.@count"
}
func iosSharePath(app string) string { return filepath.Join(app, "PlugIns", shareAppex) }

func shareInfo(p *project, platform map[string]any) map[string]any {
	types := make([]any, len(p.Apple.ShareTypes))
	for i, uti := range p.Apple.ShareTypes {
		types[i] = uti
	}
	out := map[string]any{
		"CFBundleIdentifier":            p.ID + ".share",
		"CFBundleName":                  p.Name,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleDisplayName":           p.Name,
		"CFBundleExecutable":            shareExecutable,
		"CFBundlePackageType":           "XPC!",
		"CFBundleShortVersionString":    p.Version,
		"CFBundleVersion":               p.BundleVersion(),
		"MinimumOSVersion":              iOS.minimum,
		"UIDeviceFamily":                []any{int64(1), int64(2)},
		"NSExtension": map[string]any{
			"NSExtensionPointIdentifier": "com.apple.share-services",
			"NSExtensionPrincipalClass":  "NSGoShareController",
			"NSExtensionAttributes":      map[string]any{"NSExtensionActivationRule": p.shareActivation()},
		},
		shareGroupKey:        p.appGroup(),
		shareTypesKey:        types,
		shareNotificationKey: p.ID + ".nsgo-shared",
	}
	for k, v := range platform {
		out[k] = v
	}
	return out
}

// addIOSShare builds a small Go host using only extension-safe Foundation and UIKit APIs. Its generated entry point exists only during this build.
func addIOSShare(ctx context.Context, p *project, app string, env []string, simulator bool) error {
	if !p.iosShares() {
		return nil
	}
	stage(ctx, "building the share extension")
	dir, err := os.MkdirTemp("", "nsgo-share-extension-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte(`package main
import (
 "runtime"
 "github.com/nsgo-dev/nsgo/cmd/nsgo/shareextension"
)
func init(){ runtime.LockOSThread() }
func main(){ shareextension.Run() }
`), 0o600); err != nil {
		return err
	}
	appex := iosSharePath(app)
	if err := os.MkdirAll(appex, 0o755); err != nil {
		return err
	}
	if err := goTool(ctx, p.Dir, env, "build", "-trimpath", "-buildvcs=false", "-ldflags", "-s -w -extldflags=-Wl,-w,-application_extension", "-o", filepath.Join(appex, shareExecutable), source); err != nil {
		return err
	}
	platform, err := appleBuildInfo(ctx, iOS, simulator)
	if err != nil {
		return err
	}
	if platform == nil {
		platform = map[string]any{}
	}
	platform["CFBundleSupportedPlatforms"] = []any{iOS.platformName(simulator)}
	if simulator {
		platform["DTPlatformName"] = "iphonesimulator"
	}
	if err := os.WriteFile(filepath.Join(appex, "Info.plist"), encodePlist(shareInfo(p, platform)), 0o644); err != nil {
		return err
	}
	if _, err := writeAppleNames(p, appex); err != nil {
		return err
	}
	if err := p.writePrivacyManifest(appex); err != nil {
		return err
	}
	if !simulator {
		return nil
	}
	path, err := writeEntitlements(p.groupEntitlements())
	if err != nil {
		return err
	}
	defer os.Remove(path)
	return run(ctx, "codesign", "--force", "--sign", "-", "--entitlements", path, appex)
}

func signIOSShare(ctx context.Context, p *project, app, identity string) error {
	appex := iosSharePath(app)
	if _, err := os.Stat(appex); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	if p.Sign.ShareProfile == "" {
		return fmt.Errorf("the share extension %s.share needs sign.share_profile, its own provisioning profile granting %s", p.ID, p.appGroup())
	}
	data, err := os.ReadFile(p.Sign.ShareProfile)
	if err != nil {
		return err
	}
	profile, err := readProfile(ctx, data)
	if err != nil {
		return err
	}
	entitlements, ok := profile["Entitlements"].(map[string]any)
	if !ok {
		return fmt.Errorf("sign.share_profile has no entitlements")
	}
	id := p.ID + ".share"
	appID, _ := entitlements["application-identifier"].(string)
	if !profileAllows(appID, id) {
		return fmt.Errorf("sign.share_profile is for %s, not %s", appID, id)
	}
	if groups, _ := entitlements["com.apple.security.application-groups"].([]any); !slices.Contains(groups, any(p.appGroup())) {
		return fmt.Errorf("sign.share_profile does not grant the app group %s", p.appGroup())
	}
	entitlements["application-identifier"] = signedAppID(appID, id)
	entitlements["com.apple.security.application-groups"] = []any{p.appGroup()}
	if err := os.WriteFile(filepath.Join(appex, "embedded.mobileprovision"), data, 0o644); err != nil {
		return err
	}
	ents, err := writeEntitlements(entitlements)
	if err != nil {
		return err
	}
	defer os.Remove(ents)
	return run(ctx, "codesign", "--force", "--sign", identity, "--entitlements", ents, "--generate-entitlement-der", appex)
}

// iosSigningExtension describes an already-built extension Xcode signs and embeds as a separate target when running an app on a physical device.
type iosSigningExtension struct{ Name, ID, Prefix, Script string }

func iosSigningExtensions(p *project, sys *appleOS) []iosSigningExtension {
	var out []iosSigningExtension
	if sys != iOS || p.clipParent != "" {
		return out
	}
	if p.iosExtension() {
		out = append(out, iosSigningExtension{"Widgets", p.ID + ".widgets", "AA", iosWidgetsScript})
	}
	if p.iosShares() {
		out = append(out, iosSigningExtension{shareExecutable, p.ID + ".share", "AB", strings.ReplaceAll(iosWidgetsScript, widgetsAppex, shareAppex)})
	}
	return out
}
