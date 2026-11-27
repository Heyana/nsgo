package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	quickLookAppex          = "QuickLookPreview.appex"
	quickLookExec           = "QuickLookPreview"
	quickLookPrincipalClass = "NSGoQuickLookController"
)

// macQuickLookAppex is the QuickLook Preview Extension of a macOS bundle.
func macQuickLookAppex(app string) string {
	return filepath.Join(app, "Contents", "PlugIns", quickLookAppex)
}

// quickLookInfo returns Contents/Info.plist for QuickLookPreview.appex.
func quickLookInfo(p *project, minimum string) []byte {
	types := make([]any, len(p.QuickLook.Types))
	for i, uti := range p.QuickLook.Types {
		types[i] = uti
	}
	return encodePlist(map[string]any{
		"CFBundleName":                  p.Name,
		"CFBundleDisplayName":           p.Name,
		"CFBundleIdentifier":            p.QuickLook.ID,
		"CFBundleVersion":               p.BundleVersion(),
		"CFBundleShortVersionString":    p.Version,
		"CFBundleExecutable":            quickLookExec,
		"CFBundlePackageType":           "XPC!",
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleSupportedPlatforms":    []any{"MacOSX"},
		"LSMinimumSystemVersion":        minimum,
		"NSExtension": map[string]any{
			"NSExtensionPointIdentifier": "com.apple.quicklook.preview",
			"NSExtensionPrincipalClass":  quickLookPrincipalClass,
			"NSExtensionAttributes": map[string]any{
				"QLSupportedContentTypes": types,
				"QLIsDataBasedPreview":    false,
			},
		},
	})
}

// addMacQuickLook builds and embeds the app's macOS QuickLook Preview
// Extension (Contents/PlugIns/QuickLookPreview.appex) for arch, signed ad hoc
// with the mandatory App Sandbox entitlement. signMacApp and signMacStoreApp
// re-sign it with the release identity or Mac App Store profile before signing
// the host app.
func addMacQuickLook(ctx context.Context, p *project, app, arch string) error {
	if p.QuickLook == nil {
		return nil
	}
	minimum, err := p.appleMinimum(nil)
	if err != nil {
		return err
	}
	stage(ctx, "building the QuickLook extension")
	appex := macQuickLookAppex(app)
	contents := filepath.Join(appex, "Contents")
	bin := filepath.Join(contents, "MacOS", quickLookExec)
	if err := os.MkdirAll(filepath.Dir(bin), 0o755); err != nil {
		return err
	}
	if err := p.buildQuickLook(ctx, arch, minimum, bin); err != nil {
		return err
	}
	// go build -o writes a package archive for a package that is not main.
	if data, err := os.ReadFile(bin); err != nil || bytes.HasPrefix(data, []byte("!<arch>\n")) {
		return fmt.Errorf("quicklook: %s built no executable; is it a main package?", p.QuickLook.Main)
	}
	resources := filepath.Join(contents, "Resources")
	if err := copyResources(p, resources); err != nil {
		return err
	}
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return err
	}
	if err := p.writePrivacyManifest(resources); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), quickLookInfo(p, minimum), 0o644); err != nil {
		return err
	}
	return signMacQuickLook(ctx, p, appex, "-")
}

func (p *project) buildQuickLook(ctx context.Context, arch, minimum, out string) error {
	env := []string{"GOOS=darwin", "GOARCH=" + arch, "CGO_ENABLED=0"}
	tags, err := goTags(ctx, p, env)
	if err != nil {
		return err
	}
	args := append([]string{"build"}, tagsArgs(tags)...)
	var ldflags []string
	if p.Release {
		args = append(args, "-trimpath", "-buildvcs=false")
		ldflags = append(ldflags, "-s", "-w")
	}
	ldflags = append(ldflags, "-macos="+minimum)
	allFlags := append([]string{}, p.Build.LDFlags...)
	allFlags = append(allFlags, p.identityLdflags())
	if p.Release {
		allFlags = append(allFlags, "-X "+corePath+".appRelease=1")
	}
	if p.Appearance != "" {
		allFlags = append(allFlags, "-X "+nsgoModule+".appearance="+p.Appearance)
	}
	allFlags = append(allFlags, ldflags...)
	args = append(args, "-ldflags", strings.Join(allFlags, " "))
	if err := goTool(ctx, p.Dir, env, append(args, "-o", out, "--", p.QuickLook.Main)...); err != nil {
		return fmt.Errorf("quicklook: %s: %w", p.QuickLook.Main, err)
	}
	return nil
}

// signMacQuickLook signs a macOS bundle's QuickLook Preview Extension with the
// mandatory App Sandbox entitlement. It signs before the host app so the app's
// signature seals it without overwriting its entitlements.
func signMacQuickLook(ctx context.Context, p *project, appex, identity string) error {
	ents, err := writeEntitlements(map[string]any{
		"com.apple.security.app-sandbox": true,
	})
	if err != nil {
		return err
	}
	defer os.Remove(ents)
	args := []string{"--force", "--sign", identity, "--entitlements", ents}
	if identity != "-" && p.notarizing() {
		args = append(args, "--options", "runtime", "--timestamp")
	}
	return run(ctx, "codesign", append(args, appex)...)
}

// signMacStoreQuickLook provisions and signs the QuickLook extension for the
// Mac App Store using sign.quicklook_profile from the same team as the app.
func signMacStoreQuickLook(ctx context.Context, p *project, app, team string) error {
	data, err := os.ReadFile(p.Sign.QuickLookProfile)
	if err != nil {
		return err
	}
	profile, err := readProfile(ctx, data)
	if err != nil {
		return fmt.Errorf("sign.quicklook_profile: %w", err)
	}
	granted, _ := profile["Entitlements"].(map[string]any)
	id := p.QuickLook.ID
	appID, _ := granted["com.apple.application-identifier"].(string)
	if !profileAllows(appID, id) {
		return fmt.Errorf("sign.quicklook_profile is for %s, not %s", appID, id)
	}
	extTeam, _ := granted["com.apple.developer.team-identifier"].(string)
	if extTeam == "" || extTeam != team {
		return fmt.Errorf("sign.quicklook_profile must belong to the app profile's team %s", team)
	}
	identity, err := profileIdentity(ctx, profile)
	if err != nil {
		return err
	}
	ents, err := writeEntitlements(map[string]any{
		"com.apple.security.app-sandbox":      true,
		"com.apple.application-identifier":    signedAppID(appID, id),
		"com.apple.developer.team-identifier": team,
	})
	if err != nil {
		return err
	}
	defer os.Remove(ents)
	appex := macQuickLookAppex(app)
	if err := os.WriteFile(filepath.Join(appex, "Contents", "embedded.provisionprofile"), data, 0o644); err != nil {
		return err
	}
	return run(ctx, "codesign", "--force", "--sign", identity, "--entitlements", ents, "--generate-entitlement-der", appex)
}
