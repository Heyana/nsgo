package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/harmony"
	"go.yaml.in/yaml/v3"
)

// widget is an app's home-screen card: what its 2x2 card says.
type widget struct {
	Title string `yaml:"title"`
}

// project is an app's nsgo.yaml and where it lives.
type project struct {
	Dir string `yaml:"-"`
	// resourceDir replaces Dir's resources directory for a generated preview host.
	resourceDir string
	// Release strips symbols and debug information for the smallest
	// binaries; nsgo build sets it, nsgo dev does not.
	Release bool `yaml:"-"`
	// idDefault records that nsgo.yaml gave no id, so it follows the name.
	idDefault bool
	// devURL is the dev server's address for targets that pass the app no environment, and devToken the run's token for it, which the build gives the app in a file it adds (devTokenFile).
	devURL, devToken string
	// devDebug retains DWARF and disables optimization for Delve.
	devDebug bool
	// testBinary builds the package's tests with the nsgotest tag in
	// place of the app, for nsgo test -target.
	testBinary bool
	// bare records that the directory has no nsgo.yaml: a library, or a
	// module's root, that nsgo test and nsgo check take as a tree of
	// packages.
	bare bool
	// steps, when set, are what nsgo test <entry> runs in place of the
	// package's tests: the steps of standard input on the app at entry.
	steps *steps
	// split, set while a web release builds, holds its split bundles.
	split *splitBundles
	// clipParent is the app's ID when this is its App Clip's project
	// (clipProject), and parentClip the app's app_clip.
	clipParent string
	parentClip *appClip
	// released lists the files nsgo build put in the release directory
	// for the target it builds.
	released []string
	// checkStorePrivacy validates the Apple manifests in a store package.
	checkStorePrivacy bool
	// buildParallel bounds Go's compile workers for one target while several
	// targets build together. Zero keeps Go's default.
	buildParallel int
	// packageLicenses is set by build for a standalone app package.
	packageLicenses bool
	// noticeArchitectures is set for a bundle with more than one Go binary.
	noticeArchitectures []string
	thirdPartyNotices   string
	// nativeLibraries are the declarations selected for each executable slice.
	nativeLibraries map[string][]nativeLibrary

	Name    string `yaml:"name"`
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
	// Game covers the app's game classification and system Game Mode metadata.
	Game bool `yaml:"game"`
	// Gamepad declares support for an optional extended game controller.
	Gamepad bool `yaml:"gamepad"`
	// Copyright is the app's copyright notice, such as "© 2026 Example": its Windows executables' LegalCopyright and its installer's, and the NSHumanReadableCopyright of its macOS Info.plist, which the About panel shows.
	Copyright string        `yaml:"copyright"`
	Licenses  licensePolicy `yaml:"licenses"`
	// BuildNumber numbers builds of a version, as stores want each upload
	// numbered: CFBundleVersion and Android's versionCode. Without it
	// they come from Version.
	BuildNumber int `yaml:"build_number"`
	// Appearance starts the app light or dark whatever the system's, its
	// launch screen and window included; empty follows the system.
	Appearance string `yaml:"appearance"`
	// LaunchScreen gives standalone iOS and Android apps their launch artwork.
	LaunchScreen *launchScreen `yaml:"launch_screen"`
	// Window is no longer nsgo.yaml's: Go code sizes windows.
	Window any `yaml:"window"`
	// Widget is the app's home-screen card on HarmonyOS: the title it shows.
	Widget *widget `yaml:"widget"`
	// Icon names the app's icons; resolveIcons fills in the defaults
	// under assets.
	Icon icons `yaml:"icon"`
	Dev  struct {
		Target string `yaml:"target"`
		// Icon replaces Icon in nsgo dev's builds, field by field.
		Icon icons `yaml:"icon"`
		// HotReload rebuilds and restarts the app when a watched file
		// changes; it defaults to true.
		HotReload *bool `yaml:"hot_reload"`
		// FPS is no longer nsgo.yaml's: apps take --nsgo-fps.
		FPS any `yaml:"fps"`
		// Client builds the dev client in, which connects to nsgo dev's
		// server: nsgo dev sets it. Trace has the client send the server
		// each commit: nsgo dev -trace sets it.
		Client bool `yaml:"-"`
		Trace  bool `yaml:"-"`
		// Profile enables a runtime profile capture from the dev server.
		Profile bool `yaml:"-"`
		// Highlight has the client outline what each commit changed: nsgo dev -highlight sets it.
		Highlight bool `yaml:"-"`
		// Devtools has the client serve DevTools: nsgo dev -devtools sets it.
		Devtools bool `yaml:"-"`
		// Layout is what DevTools' layout audit starts showing, as bounds,grid=8: nsgo dev -layout sets it.
		Layout string `yaml:"-"`
		// Automation links the agent nsgo dev -automation drives the app through (package nsgotest), with the nsgoautomation tag.
		Automation bool `yaml:"-"`
		// Limits runs a TSX app sandboxed, as tsx.View would, within
		// these limits (tsx.ParseLimits) and its capabilities: nsgo dev
		// -limits sets it.
		Limits string `yaml:"-"`
	} `yaml:"dev"`
	// Theme is a TSX app's theme file, relative to nsgo.yaml: a theme
	// core.ParseTheme reads, NSGo's or a shadcn/ui one, which the
	// compiler gives the app before it mounts.
	Theme string `yaml:"theme"`
	// Tailwind extends Tailwind's default theme for a TSX app's classes,
	// as core.ParseTailwind reads it; the compiler checks it.
	Tailwind yaml.Node `yaml:"tailwind"`
	// Capabilities are what a TSX app's code may reach once signed into a
	// package (nsgo publish), and under nsgo dev -limits.
	Capabilities yaml.Node `yaml:"capabilities"`
	// StatusBar is a TSX app's default status bar on iOS and Android,
	// under its pages' and dialogs': foreground (auto, light or dark) and
	// hidden.
	StatusBar yaml.Node `yaml:"status_bar"`
	// I18n is the app's catalogs: Dir, relative to nsgo.yaml, holds a
	// YAML file per language, Source's with every message. Their
	// languages are the ones the app shows.
	I18n struct {
		Dir    string `yaml:"dir"`
		Source string `yaml:"source"`
	} `yaml:"i18n"`
	// Web is the web target's: Host is a JS module, relative to
	// nsgo.yaml and inside its directory, whose createHost(player) gives
	// the app's page its host. Offline enables an offline launch of the
	// published site through a service worker.
	Web struct {
		Host    string `yaml:"host"`
		Offline bool   `yaml:"offline"`
		// Base fixes assets to an absolute deployment path for deep links.
		Base string `yaml:"base"`
	} `yaml:"web"`
	// MiniProgram is the mini program targets': AppID holds the mini
	// program's on each platform (wechat, qq, alipay, douyin), and
	// CustomNavigation hides the platform's navigation bar, so the app
	// fills the page and draws its own, clear of the capsule button.
	MiniProgram struct {
		AppID            map[string]string `yaml:"appid"`
		CustomNavigation bool              `yaml:"custom_navigation"`
	} `yaml:"miniprogram"`
	// Apple is the macOS and iOS targets': Info holds scalar and array entries for
	// their Info.plist, such as NSMicrophoneUsageDescription.
	Apple struct {
		Info map[string]any `yaml:"info"`
		// RequiresFullScreen is the iOS package's UIRequiresFullScreen, true unless set; false lets an iPad run the app in Split View, Slide Over and Stage Manager.
		RequiresFullScreen *bool `yaml:"requires_full_screen"`
		// MultipleScenes is the iOS package's UIApplicationSupportsMultipleScenes, false unless set; true lets the system create more than one scene session for the app.
		MultipleScenes *bool `yaml:"multiple_scenes"`
		// Orientations are the iPad's UISupportedInterfaceOrientations~ipad, of portrait, portrait_upside_down, landscape_left and landscape_right; all four when RequiresFullScreen is false, which needs them, and undeclared otherwise.
		Orientations []string `yaml:"orientations"`
		// AppGroup is the app group an iOS app shares with its widgets,
		// group.<id> by default. Setting it explicitly with app_clip also
		// shares persistent data and storage with the App Clip.
		AppGroup string `yaml:"app_group"`
		// SignInWithApple claims com.apple.developer.applesignin, which Sign in with Apple needs, for the iOS and tvOS packages and the Mac App Store's (sign.go).
		SignInWithApple bool `yaml:"sign_in_with_apple"`
		// Purchases puts StoreKit's library, which in-app purchases go through, into the macOS, iOS and tvOS packages (storekit.go).
		Purchases bool `yaml:"purchases"`
		// ICloud configures Apple iCloud key-value store and ubiquity document containers (cloud.go); top-level cloud: is its provider-neutral alias.
		ICloud cloudConfig `yaml:"icloud"`
		// Privacy adds the app's required-reason APIs, tracking settings and collected data to PrivacyInfo.xcprivacy.
		Privacy privacySettings `yaml:"privacy"`
	} `yaml:"apple"`
	// Cloud configures cloud storage entitlements, such as Apple iCloud key-value store and ubiquity containers (cloud.go).
	Cloud cloudConfig `yaml:"cloud"`
	// ICloud is a top-level alias for Cloud for Apple iCloud entitlements (cloud.go).
	ICloud cloudConfig `yaml:"icloud"`
	// FileTypes are the kinds of documents the app opens, which desktop
	// and full iOS packages register (filetypes.go).
	FileTypes []fileType `yaml:"file_types"`
	// URLSchemes are the schemes of the app's own links, which its desktop, iOS and Android packages register.
	URLSchemes []string `yaml:"url_schemes"`
	// AppLinks are the HTTPS hosts associated with the full app on iOS and Android; the app's server supplies their association files.
	AppLinks []string `yaml:"app_links"`
	// Commands are command-line programs the app's desktop packages ship beside its executable (commands.go).
	Commands []command `yaml:"commands"`
	// Widgets are the kinds of the app's widgets, which nsgo build puts
	// into its macOS and iOS packages as a widget extension (widgets.go).
	Widgets []widgetKind `yaml:"widgets"`
	// LiveActivities turns on the app's Live Activities, which the iOS
	// widget extension draws and Android shows as notifications
	// (widgets.go).
	LiveActivities liveActivities `yaml:"live_activities"`
	// AppClip is the app's App Clip, which nsgo build nests in its iOS
	// package (appclip.go).
	AppClip *appClip `yaml:"app_clip"`
	// Android is the Android target's: Permissions are the permissions its manifest asks for beyond the runtime's, such as REQUEST_INSTALL_PACKAGES, which a name without a dot is under android.permission. TargetSDK replaces the runtime's targetSdkVersion when nonzero, while zero keeps the runtime's API 36 default. Recording declares the runtime's recording service and its permissions, which microphone.RecordWAV needs. Classes lists precompiled DEX files and DEX JARs for standalone Android packages.
	Android struct {
		Permissions []string `yaml:"permissions"`
		TargetSDK   int      `yaml:"target_sdk"`
		Recording   bool     `yaml:"recording"`
		// BackgroundMotion declares the bounded shortService used by WatchBackgroundMotion.
		BackgroundMotion bool `yaml:"background_motion"`

		Classes []string `yaml:"classes"`
		// ShareTypes are the MIME types the launcher accepts from the sharesheet.
		ShareTypes []string `yaml:"share_types"`
		// BackgroundDownloads declares the runtime's dataSync download service.
		BackgroundDownloads bool `yaml:"background_downloads"`
		// LoopbackCleartext allows HTTP to loopback in standalone packages.
		LoopbackCleartext bool `yaml:"loopback_cleartext"`
		// UsesCleartextTraffic allows plain HTTP to any host in standalone packages.
		UsesCleartextTraffic bool `yaml:"uses_cleartext_traffic"`
	} `yaml:"android"`
	Compliance storeCompliance `yaml:"compliance"`
	// Harmonyos is the harmonyos target's: Permissions are the ones its
	// module.json asks for beyond the runtime's INTERNET, by their
	// ohos.permission names, such as MICROPHONE and CAMERA, which a prompt
	// only asks for once declared. Signing takes AppGallery Connect files.
	Harmonyos struct {
		Permissions []string         `yaml:"permissions"`
		Signing     *harmony.Signing `yaml:"signing"`
	} `yaml:"harmonyos"`
	// MacOS is the macOS target's disk image: DMGBackground, a PNG with
	// an optional @2x twin, is the picture behind its window, which takes
	// its size, else a built-in arrow; DMGIcon, an .icns or a square PNG,
	// is the volume's icon, else the app's. Category is the app's
	// LSApplicationCategoryType, such as public.app-category.productivity,
	// which the Mac App Store requires, and Entitlements are what a store
	// build's sandboxed app claims beyond its defaults (macstore.go); a
	// signed non-store build (such as Developer ID) also claims
	// keychain-access-groups from it (sign.go).
	MacOS struct {
		DMGBackground string         `yaml:"dmg_background"`
		DMGIcon       string         `yaml:"dmg_icon"`
		Category      string         `yaml:"category"`
		Entitlements  map[string]any `yaml:"entitlements"`
	} `yaml:"macos"`
	// Windows is the Windows target's options: CommandLine adds a .com console companion beside the GUI executable, and it or Path has the installer put the app's folder on the user's Path, so consoles find the companion and the app's commands, while InstallerIcon and UninstallerIcon, an .ico or a square PNG, are the icons of the setup program and the uninstaller, else the app's. DeviceCapabilities are the devices a store build's MSIX declares the app uses, such as microphone and webcam, which Windows asks the user for and keeps under the package's name. Publisher is who publishes the app, the CompanyName of its executables and its installer and the publisher of its uninstall entry, else Store's publisher name. Store is the Microsoft Store's package identity, as Partner Center lists it, which a store build's MSIX carries (msix.go): the package's name, its publisher (CN=...) and the publisher's display name.
	Windows struct {
		CommandLine        bool     `yaml:"command_line"`
		Path               bool     `yaml:"path"`
		Publisher          string   `yaml:"publisher"`
		InstallerIcon      string   `yaml:"installer_icon"`
		UninstallerIcon    string   `yaml:"uninstaller_icon"`
		DeviceCapabilities []string `yaml:"device_capabilities"`
		Store              struct {
			Identity      string `yaml:"identity"`
			Publisher     string `yaml:"publisher"`
			PublisherName string `yaml:"publisher_name"`
		} `yaml:"store"`
	} `yaml:"windows"`
	// Linux is the Linux target's: Categories are its desktop entry's freedesktop menu categories, which file it in application menus and need a main one (validateLinuxCategories). Maintainer is who its Debian and Arch packages name as theirs, "Name <email>", else the app's name. Depends and Recommends add to those packages' Depends and Recommends, and depend and optdepend lines, after NSGo's own (linuxpkg.go). Summary, License, an SPDX expression, and FlatpakRuntime, GNOME's runtime version, are what the AppStream metadata and a store build's Flatpak manifest say (flatpak.go). Share is a directory, relative to nsgo.yaml, whose files the Debian and Arch packages install under /usr/share and the AppImage under its AppDir's usr/share, such as metainfo/<id>.metainfo.xml, which replaces NSGo's (linuxShareFiles).
	Linux struct {
		Categories []string `yaml:"categories"`
		// AppImageUpdate overrides the external update locator; empty disables discovery.
		AppImageUpdate *string   `yaml:"appimage_update"`
		Maintainer     string    `yaml:"maintainer"`
		Summary        string    `yaml:"summary"`
		License        string    `yaml:"license"`
		FlatpakRuntime string    `yaml:"flatpak_runtime"`
		Depends        linuxDeps `yaml:"depends"`
		Recommends     linuxDeps `yaml:"recommends"`
		Share          string    `yaml:"share"`
	} `yaml:"linux"`
	// Releases is where the app is published: GitHub, a repository
	// ("owner/name") whose releases, tagged TagPrefix (v by default) and
	// the version, hold build.output's releases directory, which the
	// install scripts download from (release.go); by default the one the
	// Git remote origin names.
	Releases struct {
		GitHub    string  `yaml:"github"`
		TagPrefix *string `yaml:"tag_prefix"`
	} `yaml:"release"`
	// Updates has the app update the way its platform does (updates.go).
	Updates updatesConfig `yaml:"updates"`
	Steam   steamConfig   `yaml:"steam"`

	// Store names where the app's text-based store listings live (listings.go).
	Store storeConfig `yaml:"store"`
	Itch  itchConfig  `yaml:"itch"`
	// Mobile is the iOS and Android targets': ExcludeResources are paths in
	// resources/ their packages leave out, such as what only desktops run.
	Mobile struct {
		ExcludeResources []string          `yaml:"exclude_resources"`
		AssetPacks       []assetPackConfig `yaml:"asset_packs"`
	} `yaml:"mobile"`
	Sign  signing `yaml:"sign"`
	Build struct {
		Output  string   `yaml:"output"`
		Targets []string `yaml:"targets"`
		// LDFlags are the app's own linker flags, each as go build's -ldflags takes them, ahead of the tool's so that its -X values, such as the app's identity in core, win (ldflags).
		LDFlags []string `yaml:"ldflags"`
		// Tags are the app's Go build tags, which every go command the tool runs for the app takes, the tool's own added to them (goTags).
		Tags []string `yaml:"tags"`
		// Open shows the output directory once the build finishes; it
		// defaults to false.
		Open *bool `yaml:"open"`
		// FPS is no longer nsgo.yaml's: apps take --nsgo-fps.
		FPS any `yaml:"fps"`
		// Installers packages the desktop targets for users to install
		// (installer.go): a disk image on macOS, an NSIS installer
		// on Windows, Debian and Arch packages on Linux; it defaults to
		// true.
		Installers *bool `yaml:"installers"`
		// Jobs is how many targets build at once; 0, the default, is
		// half the CPUs.
		Jobs int `yaml:"jobs"`
		// HarmonyosBundles says how the harmonyos target names the bundle:
		// auto, the default, uses the app's id when the machine has the
		// OpenHarmony SDK and pads it otherwise; exact needs the SDK;
		// padded always pads.
		HarmonyosBundles string `yaml:"harmonyos_bundles"`
		// run starts the package built for this machine afterwards (-run),
		// on device (-device), an iPhone signed for team (-team);
		// embed packages the app for host apps of their own (-embed).
		run, embed bool
		// all records that the targets were given as all.
		all          bool
		dynamic      bool
		device, team string
	} `yaml:"build"`
}

type privacySettings struct {
	AccessedAPIs    map[string][]string `yaml:"accessed_apis"`
	CollectedData   []map[string]any    `yaml:"collected_data"`
	Tracking        *bool               `yaml:"tracking"`
	TrackingDomains []string            `yaml:"tracking_domains"`
}

// icons are an app's icon files.
type icons struct {
	// Apple is an Icon Composer .icon for macOS, iOS and an Apple Watch
	// without an icon of its own, or an .icns for macOS.
	Apple string `yaml:"apple"`
	// PNG is the shared square-image fallback for targets without their own artwork, including Apple targets Apple does not cover.
	PNG string `yaml:"png"`
	// Windows and Linux replace PNG for their packages, store assets and installers.
	Windows string `yaml:"windows"`
	Linux   string `yaml:"linux"`
	// TVOS is the Apple TV's: Xcode's .brandassets with its layered icon
	// and top shelf images, or a wide image that fills them. Without it
	// the PNG layers as the app icon and top shelf images.
	TVOS string `yaml:"tvos"`
	// WatchOS is the Apple Watch's: an Icon Composer .icon drawn in
	// circles, or a square image. Without it Apple is used, then PNG.
	WatchOS string `yaml:"watchos"`
	// Android is the adaptive launcher icon's foreground, background and
	// optional themed monochrome layer.
	Android *androidIcon `yaml:"android"`
}

type androidIcon struct {
	// PNG replaces icon.png for Android bitmap icons; adaptive layers remain optional.
	PNG        string `yaml:"png"`
	Foreground string `yaml:"foreground"`
	Background string `yaml:"background"`
	Monochrome string `yaml:"monochrome"`
}

// openOutput reports whether nsgo build opens its output directory.
func (p *project) openOutput() bool {
	return p.Build.Open != nil && *p.Build.Open
}

// installers reports whether nsgo build packages desktop targets for
// users to install, unless build.installers turns it off.
func (p *project) installers() bool {
	return p.channel() != channelSteam && (p.Build.Installers == nil || *p.Build.Installers)
}

// hotReload reports whether nsgo dev rebuilds the app on changes, unless
// dev.hot_reload turns it off; without it, dev runs the app once and exits
// when the app does.
func (p *project) hotReload() bool {
	return p.Dev.HotReload == nil || *p.Dev.HotReload
}

// loadOrDir is load, or for a directory without nsgo.yaml a bare project
// named after it.
func loadOrDir(dir string) (*project, error) {
	p, err := load(dir)
	if !errors.Is(err, os.ErrNotExist) {
		return p, err
	}
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("no directory %s", dir)
	}
	name := filepath.Base(abs)
	return &project{Dir: abs, Name: name, ID: generatedID(name), idDefault: true, bare: true}, nil
}

func load(dir string) (*project, error) {
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(abs, "nsgo.yaml"))
	if err != nil {
		return nil, err
	}
	p := &project{Dir: abs}
	if err := yaml.Unmarshal(data, p); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if p.Name == "" {
		return nil, fmt.Errorf("nsgo.yaml: name is required")
	}
	if err := p.validateAppleMinimums(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateIPadMultitasking(); err != nil {
		return nil, err
	}
	if err := p.validateAssetPacks(); err != nil {
		return nil, err
	}
	if strings.Contains(p.Name, "'") && strings.Contains(p.Name, `"`) {
		return nil, fmt.Errorf("nsgo.yaml: name %q has both a single and a double quote, which go build cannot pass to the app; use one kind", p.Name)
	}
	if _, err := p.capabilities(); err != nil {
		return nil, err
	}
	if err := p.Licenses.validate(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if p.ID == "" {
		p.ID = generatedID(p.Name)
		p.idDefault = true
	}
	if p.Version == "" {
		p.Version = "0.0.0"
	}
	switch p.Appearance {
	case "", "light", "dark":
	default:
		return nil, fmt.Errorf("nsgo.yaml: appearance is %q; use light, dark or leave it out", p.Appearance)
	}
	if err := p.validateLaunchScreen(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if p.Window != nil {
		return nil, fmt.Errorf("nsgo.yaml: window is set in Go now: nsgo.Run(Root, nsgo.WindowSize(width, height)), or desktop.WindowOptions")
	}
	if p.Dev.FPS != nil || p.Build.FPS != nil {
		return nil, fmt.Errorf("nsgo.yaml: fps is gone: start the app with --nsgo-fps, or run nsgo dev -fps")
	}
	if owner, name, ok := strings.Cut(p.Releases.GitHub, "/"); p.Releases.GitHub != "" && (!ok || owner == "" || name == "" || strings.Contains(name, "/")) {
		return nil, fmt.Errorf("nsgo.yaml: release.github %q is not owner/name", p.Releases.GitHub)
	}
	if p.Linux.AppImageUpdate != nil {
		if err := validateAppImageUpdate(*p.Linux.AppImageUpdate); err != nil {
			return nil, fmt.Errorf("nsgo.yaml: linux.appimage_update: %w", err)
		}
	}
	for _, tag := range p.Build.Tags {
		if !validTag(tag) {
			return nil, fmt.Errorf("nsgo.yaml: build.tags has %q; a build tag is letters, digits, underscores and dots, one to an item", tag)
		}
	}
	if p.Dev.Target == "" {
		p.Dev.Target = "native"
	}
	if p.Build.Output == "" {
		p.Build.Output = "dist"
	}
	p.Build.Output = p.resolve(p.Build.Output)
	for i, perm := range p.Android.Permissions {
		if !strings.Contains(perm, ".") {
			p.Android.Permissions[i] = "android.permission." + perm
		}
	}
	for i, perm := range p.Harmonyos.Permissions {
		if !strings.Contains(perm, ".") {
			p.Harmonyos.Permissions[i] = "ohos.permission." + perm
		}
	}
	if p.I18n.Dir == "" {
		p.I18n.Dir = "locales"
	}
	p.I18n.Dir = p.resolve(p.I18n.Dir)
	if p.Store.Dir != "" {
		p.Store.Dir = p.resolve(p.Store.Dir)
	}
	if p.I18n.Source == "" {
		p.I18n.Source = "en"
	}
	if err := p.validateFileTypes(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateURLSchemes(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateAppLinks(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateCommands(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateAndroidShareTypes(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateWidgets(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateAppClip(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validatePrivacy(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateCloud(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateUpdates(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateLinuxDeps(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.validateLinuxCategories(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if p.Linux.Share != "" {
		p.Linux.Share = p.resolve(p.Linux.Share)
	}
	for _, path := range []*string{&p.Sign.Profile, &p.Sign.WidgetsProfile, &p.Sign.AssetPacksProfile, &p.Sign.ClipProfile, &p.Sign.MacOSProfile, &p.Sign.Keystore, &p.Sign.Certificate} {
		if *path != "" {
			*path = p.resolve(*path)
		}
	}
	if len(p.Build.Targets) == 0 {
		p.Build.Targets = []string{"macos_" + goArchName(runtime.GOARCH)}
	}
	p.Build.all = slices.Contains(p.Build.Targets, "all")
	expanded, err := expandTargets(p.Build.Targets)
	if err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	p.Build.Targets = expanded
	if err := p.resolveIcons(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := p.resolveInstallerFiles(); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	return p, nil
}

// generatedID is the bundle ID of a project whose nsgo.yaml gives none, or of a bare project, made from its name: "dev.nsgo." and the name's ASCII letters and digits, after "app" when they are none or start with a digit, so that a phone takes it as an Android package and an Apple bundle ID whatever the name, as My App, my-lib, ui_kit or 2d. Android's packages refuse spaces and hyphens, and Apple's bundle IDs spaces and underscores.
func generatedID(name string) string {
	part := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, name)
	if part == "" || part[0] >= '0' && part[0] <= '9' {
		part = "app" + part
	}
	return "dev.nsgo." + part
}

// resolve makes a path from nsgo.yaml or a flag absolute against the app's
// directory.
func (p *project) resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(p.Dir, path)
}

// cache is the project's directory for development builds.
func (p *project) cache(parts ...string) string {
	return filepath.Join(append([]string{p.Dir, ".nsgo"}, parts...)...)
}

// xflag is the -X flag setting the string variable name to value, quoted as go build splits -ldflags: in single quotes, or double quotes for a value with a single quote, as an app named Bob's Notes has. Nothing escapes inside either, so load refuses a name with both.
func xflag(name, value string) string {
	if strings.Contains(value, "'") {
		return `-X "` + name + "=" + value + `"`
	}
	return "-X '" + name + "=" + value + "'"
}

// appLdflags joins the app's linker flags, nsgo.yaml's build.ldflags and those of -ldflags, into one -ldflags value.
func (p *project) appLdflags() string {
	return strings.Join(p.Build.LDFlags, " ")
}

// corePath is package core's, which holds the app's identity (core.App).
const corePath = "github.com/nsgo-dev/nsgo/core"

// identityLdflags are the -X flags building nsgo.yaml's name, ID, version and build number into core.App, which the app and its commands take.
func (p *project) identityLdflags() string {
	return strings.Join([]string{xflag(corePath+".appName", p.Name), xflag(corePath+".appID", p.ID),
		xflag(corePath+".appVersion", p.Version), xflag(corePath+".appBuild", p.BundleVersion())}, " ")
}

// ldflags builds nsgo.yaml's name, ID, version and build number into
// core.App, and passes the rest of what nsgo.Run takes from it.
func (p *project) ldflags(extra ...string) string {
	const pkg = "github.com/nsgo-dev/nsgo"
	// The dev client, which nsgo.Run starts in dev builds.
	const devPkg = pkg + "/internal/dev"
	flags := p.identityLdflags()
	// The linker keeps the last -X of a variable, so the app's flags go first and cannot replace the tool's.
	if app := p.appLdflags(); app != "" {
		flags = app + " " + flags
	}
	if p.Appearance != "" {
		flags += fmt.Sprintf(" -X %s.appearance=%s", pkg, p.Appearance)
	}
	if p.devURL != "" {
		flags += fmt.Sprintf(" -X '%s.devServer=%s' -X '%s.devServer=%s'", devPkg, p.devURL, compilerPath, p.devURL)
	}
	if p.Dev.Client && p.Dev.Trace {
		flags += fmt.Sprintf(" -X %s.devTrace=1", devPkg)
	}
	if p.Dev.Client && p.Dev.Profile {
		flags += fmt.Sprintf(" -X %s.devProfile=1", devPkg)
	}
	if p.Dev.Client && p.Dev.Highlight {
		flags += fmt.Sprintf(" -X %s.devHighlight=1", devPkg)
	}
	if p.Dev.Client && p.Dev.Devtools {
		flags += fmt.Sprintf(" -X %s.devDevtools=1", devPkg)
	}
	if p.Dev.Client && p.Dev.Layout != "" {
		flags += fmt.Sprintf(" -X %s.devLayout=%s", devPkg, p.Dev.Layout)
	}
	if p.Dev.Client && p.Dev.Limits != "" {
		flags += fmt.Sprintf(" -X '%s.devLimits=%s'", compilerPath, p.Dev.Limits)
		if caps, _ := p.capabilities(); caps != nil {
			data, _ := json.Marshal(caps)
			flags += fmt.Sprintf(" -X %s.devCapabilities=%s", compilerPath, base64.RawURLEncoding.EncodeToString(data))
		}
	}
	if p.Release {
		flags += fmt.Sprintf(" -X %s.appRelease=1 -s -w", corePath)
	}
	for _, e := range extra {
		flags += " " + e
	}
	return flags
}

// BundleVersion is the build's number: BuildNumber, else Version.
func (p *project) BundleVersion() string {
	if p.BuildNumber > 0 {
		return strconv.Itoa(p.BuildNumber)
	}
	return p.Version
}

// goArchName maps Go's amd64 to x64, the name packages use.
func goArchName(arch string) string {
	if arch == "amd64" {
		return "x64"
	}
	return arch
}

// iosOrientations are apple.orientations' names, in the order a default list declares them, with the UIKit orientations they stand for.
var iosOrientations = []struct{ name, plist string }{
	{"portrait", "UIInterfaceOrientationPortrait"},
	{"portrait_upside_down", "UIInterfaceOrientationPortraitUpsideDown"},
	{"landscape_left", "UIInterfaceOrientationLandscapeLeft"},
	{"landscape_right", "UIInterfaceOrientationLandscapeRight"},
}

// validateIPadMultitasking checks apple.orientations and apple.requires_full_screen: known orientations, none twice, all four for an iPad app that multitasks, which App Store Connect refuses otherwise, and no apple.info entry for the keys they write.
func (p *project) validateIPadMultitasking() error {
	seen := make(map[string]bool, len(p.Apple.Orientations))
	for _, orientation := range p.Apple.Orientations {
		known := false
		for _, o := range iosOrientations {
			known = known || o.name == orientation
		}
		if !known {
			return fmt.Errorf("nsgo.yaml: apple.orientations has %q; use portrait, portrait_upside_down, landscape_left or landscape_right", orientation)
		}
		if seen[orientation] {
			return fmt.Errorf("nsgo.yaml: apple.orientations repeats %q", orientation)
		}
		seen[orientation] = true
	}
	if !p.IOSRequiresFullScreen() && len(p.Apple.Orientations) > 0 && len(p.Apple.Orientations) < len(iosOrientations) {
		return fmt.Errorf("nsgo.yaml: apple.requires_full_screen is false, so apple.orientations must list all four orientations, as an iPad app that multitasks supports each; leave it out to declare them all")
	}
	for _, k := range [][2]string{{"UIRequiresFullScreen", "apple.requires_full_screen"}, {"UISupportedInterfaceOrientations~ipad", "apple.orientations"}, {"UIApplicationSupportsMultipleScenes", "apple.multiple_scenes"}} {
		if _, ok := p.Apple.Info[k[0]]; ok {
			return fmt.Errorf("nsgo.yaml: apple.info sets %s, which the iOS package writes; set %s instead", k[0], k[1])
		}
	}
	return nil
}

// IOSRequiresFullScreen is the iOS package's UIRequiresFullScreen value.
func (p *project) IOSRequiresFullScreen() bool {
	return p.Apple.RequiresFullScreen == nil || *p.Apple.RequiresFullScreen
}

// IOSSupportsMultipleScenes is the iOS package's UIApplicationSupportsMultipleScenes value.
func (p *project) IOSSupportsMultipleScenes() bool {
	return p.Apple.MultipleScenes != nil && *p.Apple.MultipleScenes
}

// IOSHasIPadOrientations says whether the iOS package declares iPad orientations.
func (p *project) IOSHasIPadOrientations() bool {
	return len(p.Apple.Orientations) > 0 || !p.IOSRequiresFullScreen()
}

// IOSIPadOrientationsPlist writes the iPad orientation array for Info.plist.
func (p *project) IOSIPadOrientationsPlist() string {
	var b strings.Builder
	b.WriteString("<array>")
	for _, o := range iosOrientations {
		if len(p.Apple.Orientations) == 0 || slices.Contains(p.Apple.Orientations, o.name) {
			b.WriteString("<string>" + o.plist + "</string>")
		}
	}
	b.WriteString("</array>")
	return b.String()
}
