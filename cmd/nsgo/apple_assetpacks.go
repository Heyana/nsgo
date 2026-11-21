package main

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
)

//go:embed assetpacks/Downloader.swift
var assetDownloaderSwift []byte

const assetDownloaderName = "NSGoAssetDownloader"

func (p *project) iosAssetPacks() bool {
	return len(p.Mobile.AssetPacks) > 0 && p.Release && !p.Dev.Client && !p.testBinary && p.clipParent == ""
}

func (p *project) assetPacksPlist() string {
	return plistEntries(string(encodePlist(map[string]any{
		"BAAppGroupID": p.appGroup(), "BAHasManagedAssetPacks": true, "BAUsesAppleHosting": true,
	})))
}

func assetDownloaderAppex(app string) string {
	return filepath.Join(app, "Extensions", assetDownloaderName+".appex")
}

// Apple's managed downloader is an ExtensionKit extension. Xcode's
// Apple-hosted template uses StoreDownloaderExtension's Swift entry point.
// The app itself calls the Objective-C asset manager from Go.
func addIOSAssetDownloader(ctx context.Context, p *project, app string, simulator bool) error {
	appex := assetDownloaderAppex(app)
	if err := os.MkdirAll(appex, 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp("", "nsgo-asset-downloader-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	src := filepath.Join(tmp, "Downloader.swift")
	if err := os.WriteFile(src, assetDownloaderSwift, 0o600); err != nil {
		return err
	}
	sdkName := iOS.sdkName(simulator)
	sdk, err := output(ctx, "xcrun", "--sdk", sdkName, "--show-sdk-path")
	if err != nil {
		return err
	}
	triple := "arm64-apple-ios26.0"
	if simulator {
		triple += "-simulator"
	}
	if err := run(ctx, "xcrun", "--sdk", sdkName, "swiftc", "-target", triple, "-sdk", sdk, "-O",
		"-parse-as-library", "-application-extension", "-module-name", assetDownloaderName,
		"-Xlinker", "-rpath", "-Xlinker", "@executable_path/../../Frameworks",
		"-o", filepath.Join(appex, assetDownloaderName), src); err != nil {
		return fmt.Errorf("compile the Background Assets downloader: %w", err)
	}
	info, err := appleBuildInfo(ctx, iOS, simulator)
	if err != nil {
		return err
	}
	if info == nil {
		info = map[string]any{}
	}
	info["CFBundleExecutable"] = assetDownloaderName
	info["CFBundleIdentifier"] = p.ID + ".assetdownloader"
	info["CFBundleName"] = assetDownloaderName
	info["CFBundlePackageType"] = "XPC!"
	info["CFBundleShortVersionString"] = p.Version
	info["CFBundleVersion"] = p.BundleVersion()
	info["CFBundleSupportedPlatforms"] = []any{iOS.platformName(simulator)}
	info["MinimumOSVersion"] = "26.0"
	info["UIDeviceFamily"] = []any{1, 2}
	info["EXAppExtensionAttributes"] = map[string]any{"EXExtensionPointIdentifier": "com.apple.background-asset-downloader-extension"}
	if err := os.WriteFile(filepath.Join(appex, "Info.plist"), encodePlist(info), 0o644); err != nil {
		return err
	}
	if simulator {
		return signAssetDownloader(ctx, appex, "-", p.groupEntitlements())
	}
	return nil
}

func signIOSAssetDownloader(ctx context.Context, p *project, app, identity string) error {
	appex := assetDownloaderAppex(app)
	if _, err := os.Stat(appex); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	id := p.ID + ".assetdownloader"
	if p.Sign.AssetPacksProfile == "" {
		return fmt.Errorf("%s needs sign.asset_packs_profile, a provisioning profile granting %s", id, p.appGroup())
	}
	data, err := os.ReadFile(p.Sign.AssetPacksProfile)
	if err != nil {
		return err
	}
	profile, err := readProfile(ctx, data)
	if err != nil {
		return err
	}
	entitlements, _ := profile["Entitlements"].(map[string]any)
	appID, _ := entitlements["application-identifier"].(string)
	if !profileAllows(appID, id) {
		return fmt.Errorf("sign.asset_packs_profile is for %s, not %s", appID, id)
	}
	groups, _ := entitlements["com.apple.security.application-groups"].([]any)
	if !slices.Contains(groups, any(p.appGroup())) {
		return fmt.Errorf("sign.asset_packs_profile does not grant %s", p.appGroup())
	}
	entitlements["application-identifier"] = signedAppID(appID, id)
	entitlements["com.apple.security.application-groups"] = []any{p.appGroup()}
	if err := os.WriteFile(filepath.Join(appex, "embedded.mobileprovision"), data, 0o644); err != nil {
		return err
	}
	return signAssetDownloader(ctx, appex, identity, entitlements)
}

func signAssetDownloader(ctx context.Context, appex, identity string, entitlements map[string]any) error {
	path, err := writeEntitlements(entitlements)
	if err != nil {
		return err
	}
	defer os.Remove(path)
	return run(ctx, "codesign", "--force", "--sign", identity, "--entitlements", path, "--generate-entitlement-der", appex)
}

func packageIOSAssetPacks(ctx context.Context, p *project, dir string) error {
	if !p.iosAssetPacks() {
		return nil
	}
	stage(ctx, "packaging Background Assets")
	packs, err := p.storeAssetPacks()
	if err != nil {
		return err
	}
	assetDir := filepath.Join(dir, "asset-packs")
	if err := os.MkdirAll(assetDir, 0o755); err != nil {
		return err
	}
	for i, pack := range packs {
		policy := "prefetch"
		if pack.Delivery == "install-time" {
			policy = "essential"
		}
		selectors := make([]map[string]string, 0, len(pack.Files))
		for _, file := range pack.Files {
			selectors = append(selectors, map[string]string{"file": file.Name})
		}
		manifest := map[string]any{
			"assetPackID": pack.Name,
			"downloadPolicy": map[string]any{policy: map[string]any{
				"installationEventTypes": []string{"firstInstallation", "subsequentUpdate"},
			}},
			"fileSelectors": selectors,
			"platforms":     []string{"iOS"},
		}
		data, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			return err
		}
		manifestPath := filepath.Join(assetDir, pack.Name+".json")
		if err := os.WriteFile(manifestPath, data, 0o644); err != nil {
			return err
		}
		file := filepath.Join(assetDir, pack.Name+".aar")
		cmd := exec.CommandContext(ctx, "xcrun", "ba-package", "package", manifestPath, "--output-path", file, "--quiet")
		cmd.Dir = p.Mobile.AssetPacks[i].Dir
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("package Background Assets %s: %w\n%s", pack.Name, err, out)
		}
		if err := p.releaseCopy(file, p.releaseName("ios", "-"+pack.Name+".aar")); err != nil {
			return err
		}
	}
	return nil
}
