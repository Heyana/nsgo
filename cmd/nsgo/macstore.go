package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A store build of a macOS target is the Mac App Store's package,
// <slug>-<version>-macos-<arch>.pkg: the app in the App Sandbox, with
// sign.macos_profile embedded and its entitlements, signed with the
// profile's Apple Distribution identity, in an installer package
// productbuild, which ships with macOS, signs with the team's Mac
// Installer Distribution identity. Without a profile the app is signed ad
// hoc and the package unsigned, which runs sandboxed here but uploads
// nowhere.

// sandboxEntitlements are what a store build's app claims: the sandbox, with the network and the files the user picks, the microphone, the camera, the calendars and Bluetooth when apple.info explains why, and macos.entitlements on top.
func (p *project) sandboxEntitlements() map[string]any {
	ents := map[string]any{
		"com.apple.security.app-sandbox":                    true,
		"com.apple.security.network.client":                 true,
		"com.apple.security.files.user-selected.read-write": true,
	}
	maps.Copy(ents, p.hardenedEntitlements())
	if p.appleInfoDescribes("NSBluetoothAlwaysUsageDescription") {
		ents["com.apple.security.device.bluetooth"] = true
	}
	maps.Copy(ents, p.MacOS.Entitlements)
	if len(p.Widgets) > 0 {
		groups, _ := ents["com.apple.security.application-groups"].([]any)
		groups = slices.Clone(groups)
		if !slices.Contains(groups, any(p.appGroup())) {
			groups = append(groups, p.appGroup())
		}
		ents["com.apple.security.application-groups"] = groups
	}
	return ents
}

// releaseMacStorePackage packages the signed app and gathers it for release.
func releaseMacStorePackage(ctx context.Context, p *project, app, dir, arch string) error {
	pkg, err := buildMacStorePackage(ctx, p, app, dir, arch)
	if err != nil {
		return err
	}
	return p.releaseAssets(pkg)
}

// buildMacStorePackage signs app for the Mac App Store and writes its
// installer package into dir, returning the package's path.
func buildMacStorePackage(ctx context.Context, p *project, app, dir, arch string) (string, error) {
	team, err := signMacStoreApp(ctx, p, app)
	if err != nil {
		return "", err
	}
	installer := ""
	if team != "" {
		if installer, err = p.installerIdentity(ctx, team); err != nil {
			return "", err
		}
	}
	stage(ctx, "packaging for the Mac App Store")
	pkg := filepath.Join(dir, p.releaseName("macos_"+arch, ".pkg"))
	args := []string{"--component", app, "/Applications"}
	if installer != "" {
		args = append(args, "--sign", installer)
	}
	return pkg, run(ctx, "productbuild", append(args, pkg)...)
}

// signMacStoreApp applies sandbox and profile signatures even when an app build omits its installer. It returns the team for installer signing.
func signMacStoreApp(ctx context.Context, p *project, app string) (string, error) {
	if len(p.Widgets) > 0 && (p.Sign.MacOSProfile == "" || p.Sign.WidgetsProfile == "") {
		return "", fmt.Errorf("sandboxed widgets need sign.macos_profile and sign.widgets_profile granting app group %s", p.appGroup())
	}
	if !p.Game && p.MacOSCategory() == "" {
		detailIn(ctx, "set macos.category, which the Mac App Store requires, such as public.app-category.productivity")
	}
	ents := p.sandboxEntitlements()
	identity, team := "-", ""
	if p.Sign.MacOSProfile == "" {
		detailIn(ctx, "unsigned: set sign.macos_profile to upload it to the Mac App Store")
		if p.Apple.SignInWithApple {
			warnIn(ctx, "Sign in with Apple is left out: macOS grants it only with sign.macos_profile")
		}
		if p.cloudEnabled() {
			warnIn(ctx, "iCloud is left out: macOS grants it only with sign.macos_profile")
		}
	} else {
		data, err := os.ReadFile(p.Sign.MacOSProfile)
		if err != nil {
			return "", err
		}
		profile, err := readProfile(ctx, data)
		if err != nil {
			return "", fmt.Errorf("%s: %w", rel(p.Sign.MacOSProfile), err)
		}
		granted, _ := profile["Entitlements"].(map[string]any)
		appID, _ := granted["com.apple.application-identifier"].(string)
		if !profileAllows(appID, p.ID) {
			return "", fmt.Errorf("the provisioning profile is for %s, not %s", appID, p.ID)
		}
		groups, _ := granted["com.apple.security.application-groups"].([]any)
		if len(p.Widgets) > 0 && !slices.Contains(groups, any(p.appGroup())) {
			return "", fmt.Errorf("the provisioning profile does not grant widget app group %s", p.appGroup())
		}
		team, _ = granted["com.apple.developer.team-identifier"].(string)
		ents["com.apple.application-identifier"] = signedAppID(appID, p.ID)
		ents["com.apple.developer.team-identifier"] = team
		if err := p.claimSignInWithApple(ents, granted); err != nil {
			return "", err
		}
		if err := p.claimCloudEntitlements(ents, granted, team); err != nil {
			return "", err
		}
		if err := os.WriteFile(filepath.Join(app, "Contents", "embedded.provisionprofile"), data, 0o644); err != nil {
			return "", err
		}
		if identity, err = profileIdentity(ctx, profile); err != nil {
			return "", err
		}
		if len(p.Widgets) > 0 {
			if err := signMacStoreWidgets(ctx, p, app, team); err != nil {
				return "", err
			}
		}
	}
	path, err := writeEntitlements(ents)
	if err != nil {
		return "", err
	}
	defer os.Remove(path)
	stage(ctx, "signing for the Mac App Store")
	libs, _ := filepath.Glob(filepath.Join(app, "Contents", "Frameworks", "*.dylib"))
	for _, lib := range libs {
		if err := run(ctx, "codesign", "--force", "--sign", identity, lib); err != nil {
			return "", err
		}
	}
	if err := run(ctx, "codesign", "--force", "--sign", identity, "--entitlements", path, app); err != nil {
		return "", err
	}
	return team, nil
}

// installerIdentity is the identity that signs the store's installer
// package: sign.installer, else the keychain's Mac Installer
// Distribution identity of team.
func (p *project) installerIdentity(ctx context.Context, team string) (string, error) {
	if p.Sign.Installer != "" {
		return p.Sign.Installer, nil
	}
	out, err := output(ctx, "security", "find-identity", "-v", "-p", "basic")
	if err != nil {
		return "", err
	}
	for line := range strings.SplitSeq(out, "\n") {
		if strings.Contains(line, "Installer") && strings.Contains(line, "("+team+")") {
			if _, rest, ok := strings.Cut(line, ") "); ok {
				if sha, _, ok := strings.Cut(strings.TrimSpace(rest), " "); ok {
					return sha, nil
				}
			}
		}
	}
	return "", fmt.Errorf("the keychain has no Mac Installer Distribution identity of team %s: install it, or set sign.installer", team)
}
