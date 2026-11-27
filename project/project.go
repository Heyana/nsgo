// Package project loads and validates an NSGo app's nsgo.yaml configuration,
// applying the same schema, validation and defaults as the nsgo tool without
// making network or Git calls.
package project

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/targets"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
	"go.yaml.in/yaml/v3"
)

// Distribution channels accepted by updates.channel and returned by Config.Channel.
const (
	ChannelDirect = "direct"
	ChannelStore  = "store"
	ChannelSteam  = "steam"
	ChannelNone   = "none"
)

// Config is nsgo.yaml as the nsgo tool reads it: the same fields, YAML tags,
// validation and defaults.
type Config struct {
	// Dir is the absolute directory containing nsgo.yaml.
	Dir string `yaml:"-"`

	Name    string `yaml:"name"`
	ID      string `yaml:"id"`
	Version string `yaml:"version"`
	// Game covers the app's game classification and system Game Mode metadata.
	Game bool `yaml:"game"`
	// Gamepad declares support for an optional extended game controller.
	Gamepad bool `yaml:"gamepad"`
	// Copyright is the app's copyright notice, such as "© 2026 Example": its
	// Windows executables' LegalCopyright and its installer's, and the
	// NSHumanReadableCopyright of its macOS Info.plist, which the About panel shows.
	Copyright string        `yaml:"copyright"`
	Licenses  LicensePolicy `yaml:"licenses"`
	// BuildNumber numbers builds of a version, as stores want each upload
	// numbered: CFBundleVersion and Android's versionCode. Without it they come
	// from Version.
	BuildNumber int `yaml:"build_number"`
	// Appearance starts the app light or dark whatever the system's, its launch
	// screen and window included; empty follows the system.
	Appearance string `yaml:"appearance"`
	// LaunchScreen gives standalone iOS and Android apps their launch artwork.
	LaunchScreen *LaunchScreen `yaml:"launch_screen"`
	// Window is no longer nsgo.yaml's: Go code sizes windows.
	Window any `yaml:"window"`
	// Widget is the app's home-screen card on HarmonyOS: the title it shows.
	Widget *Widget `yaml:"widget"`
	// Icon names the app's icons; Load fills in the defaults under assets.
	Icon Icons     `yaml:"icon"`
	Dev  DevConfig `yaml:"dev"`
	// Theme is a TSX app's theme file, relative to nsgo.yaml: a theme
	// core.ParseTheme reads, NSGo's or a shadcn/ui one, which the compiler gives
	// the app before it mounts.
	Theme string `yaml:"theme"`
	// Tailwind extends Tailwind's default theme for a TSX app's classes, as
	// core.ParseTailwind reads it; the compiler checks it.
	Tailwind yaml.Node `yaml:"tailwind"`
	// Capabilities are what a TSX app's code may reach once signed into a
	// package (nsgo publish), and under nsgo dev -limits.
	Capabilities yaml.Node `yaml:"capabilities"`
	// Registries maps @name prefixes to custom registry YAML URLs or paths
	// relative to nsgo.yaml.
	Registries yaml.Node `yaml:"registries"`
	// StatusBar is a TSX app's default status bar on iOS and Android, under its
	// pages' and dialogs': foreground (auto, light or dark) and hidden.
	StatusBar yaml.Node `yaml:"status_bar"`
	// I18n is the app's catalogs: Dir, relative to nsgo.yaml, holds a YAML file
	// per language, Source's with every message. Their languages are the ones the
	// app shows.
	I18n I18nConfig `yaml:"i18n"`
	// Components configures where copied components are placed: Dir, relative to
	// nsgo.yaml, holds copies written by nsgo add (default: components).
	Components ComponentsConfig `yaml:"components"`
	// Web is the web target's configuration.
	Web WebConfig `yaml:"web"`
	// MiniProgram is the mini program targets' configuration.
	MiniProgram MiniProgramConfig `yaml:"miniprogram"`
	// Apple is the macOS and iOS targets' configuration.
	Apple AppleConfig `yaml:"apple"`
	// Cloud configures cloud storage entitlements, such as Apple iCloud key-value
	// store and ubiquity containers.
	Cloud CloudConfig `yaml:"cloud"`
	// ICloud is a top-level alias for Cloud for Apple iCloud entitlements.
	ICloud CloudConfig `yaml:"icloud"`
	// FileTypes are the kinds of documents the app opens, which desktop and full
	// iOS packages register.
	FileTypes []FileType `yaml:"file_types"`
	// URLSchemes are the schemes of the app's own links, which its desktop, iOS
	// and Android packages register.
	URLSchemes []string `yaml:"url_schemes"`
	// AppLinks are the HTTPS hosts associated with the full app on iOS and
	// Android; the app's server supplies their association files.
	AppLinks []string `yaml:"app_links"`
	// Commands are command-line programs the app's desktop packages ship beside
	// its executable.
	Commands []Command `yaml:"commands"`
	// Widgets are the kinds of the app's widgets, which nsgo build puts into its
	// macOS and iOS packages as a widget extension.
	Widgets []WidgetKind `yaml:"widgets"`
	// LiveActivities turns on the app's Live Activities, which the iOS widget
	// extension draws and Android shows as notifications.
	LiveActivities LiveActivities `yaml:"live_activities"`
	// AppClip is the app's App Clip, which nsgo build nests in its iOS package.
	AppClip *AppClip `yaml:"app_clip"`
	// QuickLook is the app's macOS QuickLook Preview Extension
	// (com.apple.quicklook.preview), embedded in Contents/PlugIns.
	QuickLook *QuickLook `yaml:"quicklook"`
	// Android is the Android target's configuration.
	Android    AndroidConfig   `yaml:"android"`
	Compliance StoreCompliance `yaml:"compliance"`
	// Harmonyos is the harmonyos target's configuration.
	Harmonyos HarmonyConfig `yaml:"harmonyos"`
	// MacOS is the macOS target's configuration.
	MacOS MacOSConfig `yaml:"macos"`
	// Windows is the Windows target's configuration.
	Windows WindowsConfig `yaml:"windows"`
	// Linux is the Linux target's configuration.
	Linux LinuxConfig `yaml:"linux"`
	// Releases is where the app is published.
	Releases ReleaseConfig `yaml:"release"`
	// Updates configures how the app updates.
	Updates UpdatesConfig `yaml:"updates"`
	Steam   SteamConfig   `yaml:"steam"`
	// Store names where the app's text-based store listings live.
	Store StoreConfig `yaml:"store"`
	Itch  ItchConfig  `yaml:"itch"`
	// Mobile is the iOS and Android targets' shared configuration.
	Mobile MobileConfig `yaml:"mobile"`
	Sign   Signing      `yaml:"sign"`
	Build  BuildConfig  `yaml:"build"`

	idDefault  bool
	allTargets bool
}

// Registry is one custom registry entry declared under nsgo.yaml's registries map.
type Registry struct {
	Name string // prefixed with '@', e.g. "@acme"
	Raw  string // URL or relative path as written in nsgo.yaml
	URL  string // non-empty for remote registries
	Path string // absolute path on disk for local file registries
	Line int    // line in nsgo.yaml
}

// Widget is an app's home-screen card on HarmonyOS: what its 2x2 card says.
type Widget struct {
	Title string `yaml:"title"`
}

// LicensePolicy configures third-party license notice checks during builds.
type LicensePolicy struct {
	Unknown  string   `yaml:"unknown"`
	Copyleft string   `yaml:"copyleft"`
	Allow    []string `yaml:"allow"`
}

// LaunchScreen configures standalone iOS and Android launch screen artwork.
type LaunchScreen struct {
	LaunchAppearance `yaml:",inline"`
	Dark             LaunchAppearance `yaml:"dark"`
}

// LaunchAppearance is one appearance's launch screen background color and image.
type LaunchAppearance struct {
	Background string `yaml:"background"`
	Image      string `yaml:"image"`
}

// Color returns Background as an opaque ARGB uint32 (0xFFRRGGBB), or 0 when
// Background is unset or not a valid #RRGGBB color.
func (a LaunchAppearance) Color() uint32 {
	c, ok := parseLaunchColor(a.Background)
	if !ok {
		return 0
	}
	return c
}

func parseLaunchColor(bg string) (uint32, bool) {
	if !strings.HasPrefix(bg, "#") {
		return 0, false
	}
	rgb, err := hex.DecodeString(bg[1:])
	if err != nil || len(rgb) != 3 {
		return 0, false
	}
	return 0xff000000 | uint32(rgb[0])<<16 | uint32(rgb[1])<<8 | uint32(rgb[2]), true
}

// Icons names an app's icon files across platforms.
type Icons struct {
	// Apple is an Icon Composer .icon for macOS, iOS and an Apple Watch without
	// an icon of its own, or an .icns for macOS.
	Apple string `yaml:"apple"`
	// PNG is the shared square-image fallback for targets without their own
	// artwork, including Apple targets Apple does not cover.
	PNG string `yaml:"png"`
	// Windows and Linux replace PNG for their packages, store assets and installers.
	Windows string `yaml:"windows"`
	Linux   string `yaml:"linux"`
	// TVOS is the Apple TV's: Xcode's .brandassets with its layered icon and top
	// shelf images, or a wide image that fills them. Without it the PNG layers as
	// the app icon and top shelf images.
	TVOS string `yaml:"tvos"`
	// WatchOS is the Apple Watch's: an Icon Composer .icon drawn in circles, or a
	// square image. Without it Apple is used, then PNG.
	WatchOS string `yaml:"watchos"`
	// Android is the adaptive launcher icon's foreground, background and optional
	// themed monochrome layer.
	Android *AndroidIcon `yaml:"android"`
}

// AndroidIcon configures an Android bitmap or adaptive launcher icon.
type AndroidIcon struct {
	// PNG replaces icon.png for Android bitmap icons; adaptive layers remain optional.
	PNG        string `yaml:"png"`
	Foreground string `yaml:"foreground"`
	Background string `yaml:"background"`
	Monochrome string `yaml:"monochrome"`
}

// DevConfig is nsgo.yaml's dev section.
type DevConfig struct {
	Target string `yaml:"target"`
	// Icon replaces Icon in nsgo dev's builds, field by field.
	Icon Icons `yaml:"icon"`
	// HotReload rebuilds and restarts the app when a watched file changes; it
	// defaults to true.
	HotReload *bool `yaml:"hot_reload"`
	// FPS is no longer nsgo.yaml's: apps take --nsgo-fps.
	FPS any `yaml:"fps"`
}

// I18nConfig is nsgo.yaml's i18n section.
type I18nConfig struct {
	Dir    string `yaml:"dir"`
	Source string `yaml:"source"`
	// Coverage is warn (default) or off for dependency translations.
	Coverage string `yaml:"coverage"`
}

// ComponentsConfig is nsgo.yaml's components section.
type ComponentsConfig struct {
	Dir string `yaml:"dir"`
}

// WebConfig is nsgo.yaml's web section.
type WebConfig struct {
	Host    string `yaml:"host"`
	Offline bool   `yaml:"offline"`
	// Base fixes assets to an absolute deployment path for deep links.
	Base string `yaml:"base"`
}

// MiniProgramConfig is nsgo.yaml's miniprogram section.
type MiniProgramConfig struct {
	AppID              map[string]string `yaml:"appid"`
	CustomNavigation   bool              `yaml:"custom_navigation"`
	WeChatSpeechPlugin string            `yaml:"wechat_speech_plugin"`
}

// AppleConfig is nsgo.yaml's apple section.
type AppleConfig struct {
	Info map[string]any `yaml:"info"`
	// ShareTypes are the UTIs the iOS share extension accepts.
	ShareTypes []string `yaml:"share_types"`
	// RequiresFullScreen is the iOS package's UIRequiresFullScreen, true unless
	// set; false lets an iPad run the app in Split View, Slide Over and Stage Manager.
	RequiresFullScreen *bool `yaml:"requires_full_screen"`
	// MultipleScenes is the iOS package's UIApplicationSupportsMultipleScenes,
	// false unless set; true lets the system create more than one scene session.
	MultipleScenes *bool `yaml:"multiple_scenes"`
	// Orientations are the iPad's UISupportedInterfaceOrientations~ipad, of
	// portrait, portrait_upside_down, landscape_left and landscape_right.
	Orientations []string `yaml:"orientations"`
	// AppGroup is the app group an iOS app shares with its widgets, group.<id> by
	// default. Setting it explicitly with app_clip also shares persistent data
	// and storage with the App Clip.
	AppGroup string `yaml:"app_group"`
	// SignInWithApple claims com.apple.developer.applesignin for iOS, tvOS and
	// Mac App Store packages.
	SignInWithApple bool `yaml:"sign_in_with_apple"`
	// Purchases puts StoreKit's library into macOS, iOS and tvOS packages.
	Purchases bool `yaml:"purchases"`
	// ICloud configures Apple iCloud key-value store and ubiquity document
	// containers; top-level cloud: is its provider-neutral alias.
	ICloud CloudConfig `yaml:"icloud"`
	// Privacy adds the app's required-reason APIs, tracking settings and
	// collected data to PrivacyInfo.xcprivacy.
	Privacy PrivacySettings `yaml:"privacy"`
}

// PrivacySettings holds Apple PrivacyInfo.xcprivacy declarations.
type PrivacySettings struct {
	AccessedAPIs    map[string][]string `yaml:"accessed_apis"`
	CollectedData   []map[string]any    `yaml:"collected_data"`
	Tracking        *bool               `yaml:"tracking"`
	TrackingDomains []string            `yaml:"tracking_domains"`
}

// CloudConfig configures cloud storage entitlements, such as Apple iCloud
// key-value store and ubiquity document containers.
type CloudConfig struct {
	KVStore    bool     `yaml:"kv_store"`
	Containers []string `yaml:"containers"`
}

// FileType is a kind of document the app opens, registered by desktop and iOS
// packages.
type FileType struct {
	// Name is what the Finder, Explorer and Linux file managers call such a file.
	Name       string   `yaml:"name"`
	Extensions []string `yaml:"extensions"`
	// MIME names existing media types.
	MIME []string `yaml:"mime"`
	// ID is its uniform type identifier: <app id>.<first extension> by default.
	ID string `yaml:"id"`
	// Icon is an .icns or a square PNG for its macOS files, relative to nsgo.yaml.
	Icon string `yaml:"icon"`
	// Role is editor, the default, or viewer.
	Role string `yaml:"role"`
	// Conforms are the types it is a kind of: public.data by default.
	Conforms []string `yaml:"conforms"`
}

// Command is one of nsgo.yaml's commands shipped beside desktop executables.
type Command struct {
	Name    string   `yaml:"name"`
	Package string   `yaml:"package"`
	LDFlags []string `yaml:"ldflags"`
}

// WidgetKind is a kind of the app's widgets in nsgo.yaml.
type WidgetKind struct {
	Kind        string   `yaml:"kind"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Families    []string `yaml:"families"`
}

// LiveActivities is nsgo.yaml's live_activities section.
type LiveActivities struct {
	Enabled         bool `yaml:"enabled"`
	FrequentUpdates bool `yaml:"frequent_updates"`
}

// AppClip is nsgo.yaml's app_clip section.
type AppClip struct {
	Name          string   `yaml:"name"`
	ID            string   `yaml:"id"`
	Main          string   `yaml:"main"`
	Minimum       string   `yaml:"minimum"`
	Domains       []string `yaml:"domains"`
	Notifications bool     `yaml:"notifications"`
	Location      bool     `yaml:"location"`
}

// QuickLook is nsgo.yaml's quicklook section: a macOS QuickLook Preview
// Extension (com.apple.quicklook.preview) embedded in Contents/PlugIns.
type QuickLook struct {
	// Main is the Go main package path of the QuickLook preview extension,
	// such as ./cmd/quicklook.
	Main string `yaml:"main"`
	// ID is the extension's bundle identifier: <app id>.quicklook by default.
	ID string `yaml:"id"`
	// Types are the UTIs the extension previews (QLSupportedContentTypes).
	// When omitted, all UTIs declared in file_types are used.
	Types []string `yaml:"types"`
}

// AndroidConfig is nsgo.yaml's android section.
type AndroidConfig struct {
	Permissions         []string `yaml:"permissions"`
	TargetSDK           int      `yaml:"target_sdk"`
	Recording           bool     `yaml:"recording"`
	BackgroundMotion    bool     `yaml:"background_motion"`
	Classes             []string `yaml:"classes"`
	ShareTypes          []string `yaml:"share_types"`
	BackgroundDownloads bool     `yaml:"background_downloads"`
	// DataSync declares the runtime's generic dataSync foreground service (mobile.StartDataSync) and its permissions.
	DataSync             bool `yaml:"data_sync"`
	LoopbackCleartext    bool `yaml:"loopback_cleartext"`
	UsesCleartextTraffic bool `yaml:"uses_cleartext_traffic"`
}

// StoreCompliance records whether the app's store questionnaires are complete.
type StoreCompliance struct {
	AppStore   AppStoreCompliance   `yaml:"app_store"`
	GooglePlay GooglePlayCompliance `yaml:"google_play"`
	Steam      SteamCompliance      `yaml:"steam"`
}

// AppStoreCompliance holds App Store questionnaire progress.
type AppStoreCompliance struct {
	AgeRating *bool `yaml:"age_rating"`
}

// GooglePlayCompliance holds Google Play questionnaire progress.
type GooglePlayCompliance struct {
	DataSafety *bool `yaml:"data_safety"`
}

// SteamCompliance holds Steamworks questionnaire progress.
type SteamCompliance struct {
	ContentSurvey *bool `yaml:"content_survey"`
}

// HarmonyConfig is nsgo.yaml's harmonyos section.
type HarmonyConfig struct {
	Permissions []string        `yaml:"permissions"`
	Signing     *HarmonySigning `yaml:"signing"`
}

// HarmonySigning names the AppGallery Connect materials for a HarmonyOS HAP.
// Paths are relative to the app's nsgo.yaml unless absolute.
type HarmonySigning struct {
	Keystore    string `yaml:"keystore"`
	KeyAlias    string `yaml:"key_alias"`
	Certificate string `yaml:"certificate"`
	Profile     string `yaml:"profile"`
}

// MacOSConfig is nsgo.yaml's macos section.
type MacOSConfig struct {
	DMGBackground string         `yaml:"dmg_background"`
	DMGIcon       string         `yaml:"dmg_icon"`
	Category      string         `yaml:"category"`
	Entitlements  map[string]any `yaml:"entitlements"`
}

// WindowsConfig is nsgo.yaml's windows section.
type WindowsConfig struct {
	CommandLine        bool               `yaml:"command_line"`
	Path               bool               `yaml:"path"`
	Publisher          string             `yaml:"publisher"`
	InstallerIcon      string             `yaml:"installer_icon"`
	UninstallerIcon    string             `yaml:"uninstaller_icon"`
	DeviceCapabilities []string           `yaml:"device_capabilities"`
	Store              WindowsStoreConfig `yaml:"store"`
}

// WindowsStoreConfig is the Microsoft Store package identity in windows.store.
type WindowsStoreConfig struct {
	Identity      string `yaml:"identity"`
	Publisher     string `yaml:"publisher"`
	PublisherName string `yaml:"publisher_name"`
}

// LinuxConfig is nsgo.yaml's linux section.
type LinuxConfig struct {
	Categories     []string  `yaml:"categories"`
	AppImageUpdate *string   `yaml:"appimage_update"`
	Maintainer     string    `yaml:"maintainer"`
	Summary        string    `yaml:"summary"`
	License        string    `yaml:"license"`
	FlatpakRuntime string    `yaml:"flatpak_runtime"`
	Depends        LinuxDeps `yaml:"depends"`
	Recommends     LinuxDeps `yaml:"recommends"`
	Share          string    `yaml:"share"`
}

// LinuxDeps lists packages for each Linux package format.
type LinuxDeps struct {
	Deb  []string `yaml:"deb"`
	Arch []string `yaml:"arch"`
}

// ReleaseConfig is nsgo.yaml's release section.
type ReleaseConfig struct {
	GitHub    string  `yaml:"github"`
	TagPrefix *string `yaml:"tag_prefix"`
}

// UpdatesConfig is nsgo.yaml's updates section.
type UpdatesConfig struct {
	Key                string `yaml:"key"`
	Feed               string `yaml:"feed"`
	Automatic          *bool  `yaml:"automatic"`
	AutomaticDownloads bool   `yaml:"automatic_downloads"`
	Channel            string `yaml:"channel"`
}

// SteamConfig is nsgo.yaml's steam section.
type SteamConfig struct {
	// AppID is the Steamworks App ID for native development outside Steam.
	AppID uint32 `yaml:"app_id"`
}

// StoreConfig is nsgo.yaml's store section: where store listing files live.
type StoreConfig struct {
	// Dir is the directory holding listing.yaml and per-locale directories,
	// relative to nsgo.yaml (default: "store" in publish commands).
	Dir string `yaml:"dir"`
}

// ItchConfig is nsgo.yaml's itch section.
type ItchConfig struct {
	Project  string            `yaml:"project"`
	Channels map[string]string `yaml:"channels"`
}

// MobileConfig is nsgo.yaml's mobile section.
type MobileConfig struct {
	ExcludeResources []string    `yaml:"exclude_resources"`
	AssetPacks       []AssetPack `yaml:"asset_packs"`
}

// AssetPack is one entry in mobile.asset_packs.
type AssetPack struct {
	Name     string `yaml:"name"`
	Dir      string `yaml:"dir"`
	Delivery string `yaml:"delivery"`
}

// Signing is nsgo.yaml's sign section: what release packages are signed with.
type Signing struct {
	Identity          string `yaml:"identity"`
	Profile           string `yaml:"profile"`
	WatchProfile      string `yaml:"watch_profile"`
	WidgetsProfile    string `yaml:"widgets_profile"`
	ShareProfile      string `yaml:"share_profile"`
	AssetPacksProfile string `yaml:"asset_packs_profile"`
	ClipProfile       string `yaml:"clip_profile"`
	QuickLookProfile  string `yaml:"quicklook_profile"`
	MacOSProfile      string `yaml:"macos_profile"`
	Installer         string `yaml:"installer"`
	Keystore          string `yaml:"keystore"`
	KeyAlias          string `yaml:"key_alias"`
	Notarize          string `yaml:"notarize"`
	Certificate       string `yaml:"certificate"`
	SignCommand       string `yaml:"sign_command"`
	TimestampURL      string `yaml:"timestamp_url"`
}

// BuildConfig is nsgo.yaml's build section.
type BuildConfig struct {
	Output           string   `yaml:"output"`
	Targets          []string `yaml:"targets"`
	LDFlags          []string `yaml:"ldflags"`
	Tags             []string `yaml:"tags"`
	Open             *bool    `yaml:"open"`
	FPS              any      `yaml:"fps"`
	Installers       *bool    `yaml:"installers"`
	SkipInstallers   []string `yaml:"skip_installers"`
	Jobs             int      `yaml:"jobs"`
	HarmonyosBundles string   `yaml:"harmonyos_bundles"`
}

// Load reads dir's nsgo.yaml strictly, refuses what nsgo build refuses with
// the same errors, and fills the documented defaults. Load makes no network or
// Git calls.
func Load(dir string) (*Config, error) {
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
	c := &Config{Dir: abs}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(c); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.Validate(); err != nil {
		return nil, err
	}
	return c, nil
}

// Validate checks c against nsgo.yaml's rules and fills defaults and resolved
// paths without making network or Git calls.
func (c *Config) Validate() error {
	if c.Name == "" {
		return fmt.Errorf("nsgo.yaml: name is required")
	}
	if err := c.validateAppleMinimums(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateIPadMultitasking(); err != nil {
		return err
	}
	if err := c.validateAssetPacks(); err != nil {
		return err
	}
	if strings.Contains(c.Name, "'") && strings.Contains(c.Name, `"`) {
		return fmt.Errorf("nsgo.yaml: name %q has both a single and a double quote, which go build cannot pass to the app; use one kind", c.Name)
	}
	if _, err := c.ParseCapabilities(); err != nil {
		return err
	}
	if _, err := c.ParseRegistries(); err != nil {
		return err
	}
	if err := c.validateLicenses(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if c.ID == "" {
		c.ID = GeneratedID(c.Name)
		c.idDefault = true
	}
	if c.Version == "" {
		c.Version = "0.0.0"
	}
	switch c.Appearance {
	case "", "light", "dark":
	default:
		return fmt.Errorf("nsgo.yaml: appearance is %q; use light, dark or leave it out", c.Appearance)
	}
	if err := c.validateLaunchScreen(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if c.Window != nil {
		return fmt.Errorf("nsgo.yaml: window is set in Go now: nsgo.Run(Root, nsgo.WindowSize(width, height)), or desktop.WindowOptions")
	}
	if c.Dev.FPS != nil || c.Build.FPS != nil {
		return fmt.Errorf("nsgo.yaml: fps is gone: start the app with --nsgo-fps, or run nsgo dev -fps")
	}
	if owner, name, ok := strings.Cut(c.Releases.GitHub, "/"); c.Releases.GitHub != "" && (!ok || owner == "" || name == "" || strings.Contains(name, "/")) {
		return fmt.Errorf("nsgo.yaml: release.github %q is not owner/name", c.Releases.GitHub)
	}
	if c.Linux.AppImageUpdate != nil {
		if err := ValidateAppImageUpdate(*c.Linux.AppImageUpdate); err != nil {
			return fmt.Errorf("nsgo.yaml: linux.appimage_update: %w", err)
		}
	}
	if err := ValidateSkipInstallers(c.Build.SkipInstallers); err != nil {
		return fmt.Errorf("nsgo.yaml: build.skip_installers: %w", err)
	}
	if c.MiniProgram.WeChatSpeechPlugin != "" {
		v := strings.TrimSpace(c.MiniProgram.WeChatSpeechPlugin)
		if v == "" || strings.ContainsAny(v, " \t\r\n") {
			return fmt.Errorf("nsgo.yaml: miniprogram.wechat_speech_plugin %q must be a plugin version such as 0.3.5", c.MiniProgram.WeChatSpeechPlugin)
		}
		c.MiniProgram.WeChatSpeechPlugin = v
	}
	for _, tag := range c.Build.Tags {
		if !ValidTag(tag) {
			return fmt.Errorf("nsgo.yaml: build.tags has %q; a build tag is letters, digits, underscores and dots, one to an item", tag)
		}
	}
	if c.Dev.Target == "" {
		c.Dev.Target = "native"
	}
	if c.Build.Output == "" {
		c.Build.Output = "dist"
	}
	c.Build.Output = c.Resolve(c.Build.Output)
	for i, perm := range c.Android.Permissions {
		if !strings.Contains(perm, ".") {
			c.Android.Permissions[i] = "android.permission." + perm
		}
	}
	for i, perm := range c.Harmonyos.Permissions {
		if !strings.Contains(perm, ".") {
			c.Harmonyos.Permissions[i] = "ohos.permission." + perm
		}
	}
	if c.I18n.Dir == "" {
		c.I18n.Dir = "locales"
	}
	c.I18n.Dir = c.Resolve(c.I18n.Dir)
	if c.Store.Dir != "" {
		c.Store.Dir = c.Resolve(c.Store.Dir)
	}
	if c.I18n.Source == "" {
		c.I18n.Source = "en"
	}
	switch c.I18n.Coverage {
	case "", "warn", "off":
	default:
		return fmt.Errorf("nsgo.yaml: i18n.coverage is %q; use warn or off", c.I18n.Coverage)
	}
	if err := c.validateFileTypes(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateURLSchemes(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateAppLinks(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateCommands(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateAndroidShareTypes(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateIOSShareTypes(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateWidgets(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateAppClip(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateQuickLook(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validatePrivacy(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateCloud(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateUpdates(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := ValidateLinuxDeps("linux", c.Linux.Depends, c.Linux.Recommends); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.validateLinuxCategories(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if c.Linux.Share != "" {
		c.Linux.Share = c.Resolve(c.Linux.Share)
	}
	for _, path := range []*string{
		&c.Sign.Profile, &c.Sign.WatchProfile, &c.Sign.WidgetsProfile, &c.Sign.ShareProfile,
		&c.Sign.AssetPacksProfile, &c.Sign.ClipProfile, &c.Sign.QuickLookProfile, &c.Sign.MacOSProfile,
		&c.Sign.Keystore, &c.Sign.Certificate,
	} {
		if *path != "" {
			*path = c.Resolve(*path)
		}
	}
	if len(c.Build.Targets) == 0 {
		c.Build.Targets = []string{"macos_" + goArchName(runtime.GOARCH)}
	}
	if slices.Contains(c.Build.Targets, "all") {
		c.allTargets = true
	}
	expanded, err := targets.Expand(c.Build.Targets)
	if err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	c.Build.Targets = expanded
	if err := c.resolveIcons(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	if err := c.resolveInstallerFiles(); err != nil {
		return fmt.Errorf("nsgo.yaml: %w", err)
	}
	return nil
}

// Channel is the channel the builds are handed out through: updates.channel,
// else "direct" when updates.key is set, else "none".
func (c *Config) Channel() string {
	switch {
	case c.Updates.Channel != "":
		return c.Updates.Channel
	case c.Updates.Key != "":
		return ChannelDirect
	}
	return ChannelNone
}

// BundleVersion is the build's number: BuildNumber when positive, else Version.
func (c *Config) BundleVersion() string {
	if c.BuildNumber > 0 {
		return strconv.Itoa(c.BuildNumber)
	}
	return c.Version
}

// Resolve makes a path from nsgo.yaml absolute against Dir.
func (c *Config) Resolve(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(c.Dir, path)
}

// DefaultID reports whether ID was generated from Name because nsgo.yaml gave
// no id.
func (c *Config) DefaultID() bool { return c.idDefault }

// AllTargets reports whether nsgo.yaml's build.targets listed "all" before
// target expansion.
func (c *Config) AllTargets() bool { return c.allTargets }

// OpenOutput reports whether nsgo build opens its output directory.
func (c *Config) OpenOutput() bool { return c.Build.Open != nil && *c.Build.Open }

// HotReload reports whether nsgo dev rebuilds the app on changes (true by default).
func (c *Config) HotReload() bool { return c.Dev.HotReload == nil || *c.Dev.HotReload }

// UpdatesAutomatic reports whether the app checks for updates on its own (true by default).
func (c *Config) UpdatesAutomatic() bool {
	return c.Updates.Automatic == nil || *c.Updates.Automatic
}

// TagPrefix is what a release's tag adds before the version, "v" by default.
func (c *Config) TagPrefix() string {
	if c.Releases.TagPrefix != nil {
		return *c.Releases.TagPrefix
	}
	return "v"
}

// IOSRequiresFullScreen is the iOS package's UIRequiresFullScreen value.
func (c *Config) IOSRequiresFullScreen() bool {
	return c.Apple.RequiresFullScreen == nil || *c.Apple.RequiresFullScreen
}

// IOSSupportsMultipleScenes is the iOS package's UIApplicationSupportsMultipleScenes value.
func (c *Config) IOSSupportsMultipleScenes() bool {
	return c.Apple.MultipleScenes != nil && *c.Apple.MultipleScenes
}

// IOSHasIPadOrientations says whether the iOS package declares iPad orientations.
func (c *Config) IOSHasIPadOrientations() bool {
	return len(c.Apple.Orientations) > 0 || !c.IOSRequiresFullScreen()
}

// MacOSCategory returns the bundle's category, with Games as the default for a
// project marked as a game when neither macos.category nor
// apple.info.LSApplicationCategoryType is set.
func (c *Config) MacOSCategory() string {
	_, custom := c.Apple.Info["LSApplicationCategoryType"]
	if c.MacOS.Category == "" && c.Game && !custom {
		return "public.app-category.games"
	}
	return c.MacOS.Category
}

// LinuxCategories returns Linux.Categories, adding "Game" when Game is true.
func (c *Config) LinuxCategories() []string {
	categories := c.Linux.Categories
	if c.Game && !slices.Contains(categories, "Game") {
		return append(slices.Clone(categories), "Game")
	}
	return categories
}

// PackageSlug is what the files of an app's installers and its Linux packages
// are named by: its name in lowercase ASCII words joined by hyphens, else the
// last part of its ID for a name with no such words, else "app".
func (c *Config) PackageSlug() string {
	if s := slug(c.Name); s != "" {
		return s
	}
	id := c.ID
	if i := strings.LastIndexByte(id, '.'); i >= 0 {
		id = id[i+1:]
	}
	if s := slug(id); s != "" {
		return s
	}
	return "app"
}

// ParseCapabilities reads nsgo.yaml's capabilities strictly, returning nil when
// it has none. YAML's keys are snake_case, the package's camelCase.
func (c *Config) ParseCapabilities() (*pkg.Capabilities, error) {
	if c.Capabilities.IsZero() {
		return nil, nil
	}
	var v any
	if err := c.Capabilities.Decode(&v); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: capabilities: %w", err)
	}
	data, err := json.Marshal(camelKeys(v))
	if err != nil {
		return nil, fmt.Errorf("nsgo.yaml: capabilities: %w", err)
	}
	caps, _, err := pkg.ParseCapabilities(data, true)
	if err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	return &caps, nil
}

// ParseRegistries reads and validates nsgo.yaml's registries mapping strictly,
// returning entries sorted by Name, or nil when none are configured.
func (c *Config) ParseRegistries() ([]Registry, error) {
	return parseRegistries(&c.Registries, c.Dir)
}

// GeneratedID is the bundle ID of a project whose nsgo.yaml gives none, made
// from its name: "dev.nsgo." and the name's ASCII letters and digits, after
// "app" when they are none or start with a digit.
func GeneratedID(name string) string {
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

func slug(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			hyphen = false
		} else {
			hyphen = true
		}
	}
	return b.String()
}

func goArchName(arch string) string {
	if arch == "amd64" {
		return "x64"
	}
	return arch
}

func camelKeys(v any) any {
	switch v := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for k, x := range v {
			parts := strings.Split(k, "_")
			for i := 1; i < len(parts); i++ {
				if parts[i] != "" {
					parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
				}
			}
			out[strings.Join(parts, "")] = camelKeys(x)
		}
		return out
	case []any:
		for i, x := range v {
			v[i] = camelKeys(x)
		}
	}
	return v
}
