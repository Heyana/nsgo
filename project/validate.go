package project

import (
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

const (
	macOSRuntimeMinimum = "13.0"
	iosRuntimeMinimum   = "15.0"
	assetsDir           = "assets"
	maxWidgets          = 10
)

var (
	assetPackName     = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	extensionPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	utiPattern        = regexp.MustCompile(`^[A-Za-z0-9-]+(\.[A-Za-z0-9-]+)+$`)
	mimePattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]*/[a-z0-9][a-z0-9!#$&^_.+-]*$`)
	schemePattern     = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9+.-]*$`)
	domainPattern     = regexp.MustCompile(`^(\*\.)?[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?(\.[A-Za-z0-9]([A-Za-z0-9-]*[A-Za-z0-9])?)+$`)
	widgetKindPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	appGroupPattern   = regexp.MustCompile(`^group\.[A-Za-z0-9][A-Za-z0-9.-]*$`)

	widgetFamilies = []string{"small", "medium", "large", "extra_large", "circular", "rectangular", "inline"}

	systemSchemes = map[string]bool{
		"http": true, "https": true, "file": true, "ftp": true, "mailto": true, "tel": true, "sms": true, "facetime": true,
		"data": true, "about": true, "blob": true, "javascript": true, "asset": true, "x-apple-helpviewer": true,
	}

	iosOrientations = []string{"portrait", "portrait_upside_down", "landscape_left", "landscape_right"}

	linuxMainCategories = []string{"AudioVideo", "Audio", "Video", "Development", "Education", "Game", "Graphics", "Network", "Office", "Science", "Settings", "System", "Utility"}

	linuxAdditionalCategories = []string{
		"Building", "Debugger", "IDE", "GUIDesigner", "Profiling", "RevisionControl", "Translation",
		"Calendar", "ContactManagement", "Database", "Dictionary", "Chart", "Email", "Finance", "FlowChart", "PDA", "ProjectManagement", "Presentation", "Spreadsheet", "WordProcessor",
		"2DGraphics", "VectorGraphics", "RasterGraphics", "3DGraphics", "Scanning", "OCR", "Photography", "Publishing", "Viewer",
		"TextTools", "DesktopSettings", "HardwareSettings", "Printing", "PackageManager",
		"Dialup", "InstantMessaging", "Chat", "IRCClient", "Feed", "FileTransfer", "HamRadio", "News", "P2P", "RemoteAccess", "Telephony", "TelephonyTools", "VideoConference", "WebBrowser", "WebDevelopment",
		"Midi", "Mixer", "Sequencer", "Tuner", "TV", "AudioVideoEditing", "Player", "Recorder", "DiscBurning",
		"ActionGame", "AdventureGame", "ArcadeGame", "BoardGame", "BlocksGame", "CardGame", "KidsGame", "LogicGame", "RolePlaying", "Shooter", "Simulation", "SportsGame", "StrategyGame",
		"Art", "Construction", "Music", "Languages", "ArtificialIntelligence", "Astronomy", "Biology", "Chemistry", "ComputerScience", "DataVisualization", "Economy", "Electricity", "Geography", "Geology", "Geoscience", "History", "Humanities", "ImageProcessing", "Literature", "Maps", "Math", "NumericalAnalysis", "MedicalSoftware", "Physics", "Robotics", "Spirituality", "Sports", "ParallelComputing",
		"Amusement", "Archiving", "Compression", "Electronics", "Emulator", "Engineering", "FileTools", "FileManager", "TerminalEmulator", "Filesystem", "Monitor", "Security", "Accessibility", "Calculator", "Clock", "TextEditor", "Documentation", "Adult",
		"Core", "KDE", "GNOME", "XFCE", "DDE", "LXQt", "GTK", "Qt", "Motif", "Java", "ConsoleOnly",
	}

	linuxReservedCategories = []string{"Screensaver", "TrayIcon", "Applet", "Shell"}
)

// AppleVersion parses a 1-to-3-part numeric Apple OS version string.
func AppleVersion(value string) ([3]uint32, bool) {
	var numbers [3]uint32
	parts := strings.Split(value, ".")
	if len(parts) > len(numbers) {
		return numbers, false
	}
	for i, part := range parts {
		if part == "" {
			return numbers, false
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return numbers, false
			}
		}
		n, err := strconv.ParseUint(part, 10, 32)
		if err != nil {
			return numbers, false
		}
		numbers[i] = uint32(n)
	}
	return numbers, true
}

// ValidateChannel refuses a channel updates.channel or -channel cannot name.
func ValidateChannel(channel string) error {
	switch channel {
	case "", ChannelDirect, ChannelStore, ChannelSteam, ChannelNone:
		return nil
	}
	return fmt.Errorf("updates: channel %q is not direct, store, steam or none", channel)
}

// ValidateSkipInstallers checks desktop system names for build.skip_installers.
func ValidateSkipInstallers(systems []string) error {
	for _, system := range systems {
		switch system {
		case "macos", "windows", "linux":
		default:
			return fmt.Errorf("unknown desktop system %q; use macos, windows or linux", system)
		}
	}
	return nil
}

// ValidTag reports whether tag is a valid Go build tag: letters, digits,
// underscores and dots.
func ValidTag(tag string) bool {
	if tag == "" {
		return false
	}
	for _, r := range tag {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '.') {
			return false
		}
	}
	return true
}

// ValidateAppImageUpdate checks an AppImage update locator string.
func ValidateAppImageUpdate(info string) error {
	if info == "" {
		return nil
	}
	if strings.ContainsAny(info, "\x00\r\n") {
		return errors.New("contains a NUL or line break")
	}
	parts := strings.Split(info, "|")
	switch parts[0] {
	case "zsync":
		if len(parts) == 2 {
			u, err := url.Parse(parts[1])
			if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" && u.Fragment == "" {
				return nil
			}
		}
		return errors.New("use zsync| followed by an absolute HTTP or HTTPS control-file URL without a fragment")
	case "gh-releases-zsync":
		if len(parts) == 5 && parts[1] != "" && parts[2] != "" && parts[3] != "" && parts[4] != "" && !strings.ContainsAny(parts[1]+parts[2], "/\\") {
			return nil
		}
		return errors.New("use gh-releases-zsync|owner|repository|tag|filename-pattern")
	default:
		return errors.New("transport must be zsync or gh-releases-zsync")
	}
}

// ValidateLinuxDeps rejects blank entries and line breaks in Linux package
// dependency lists.
func ValidateLinuxDeps(field string, depends, recommends LinuxDeps) error {
	for _, key := range []struct {
		name   string
		values []string
	}{
		{"depends.deb", depends.Deb},
		{"depends.arch", depends.Arch},
		{"recommends.deb", recommends.Deb},
		{"recommends.arch", recommends.Arch},
	} {
		for i, value := range key.values {
			if strings.TrimSpace(value) == "" {
				return fmt.Errorf("%s.%s entry %d is empty", field, key.name, i+1)
			}
			if strings.ContainsAny(value, "\r\n") {
				return fmt.Errorf("%s.%s entry %d contains a line break", field, key.name, i+1)
			}
		}
	}
	return nil
}

func (c *Config) minimumVersion(key, floor string) (string, error) {
	value, exists := c.Apple.Info[key]
	if !exists {
		return floor, nil
	}
	text, ok := value.(string)
	parsed, valid := AppleVersion(text)
	baseline, _ := AppleVersion(floor)
	if !ok || !valid {
		return "", fmt.Errorf("apple.info.%s: write a version string of one to three numbers, from the runtime minimum %s up", key, floor)
	}
	if slices.Compare(parsed[:], baseline[:]) < 0 {
		return "", fmt.Errorf("apple.info.%s: %q is below the runtime minimum %s", key, text, floor)
	}
	return text, nil
}

func (c *Config) validateAppleMinimums() error {
	for _, item := range [][2]string{
		{"LSMinimumSystemVersion", macOSRuntimeMinimum},
		{"MinimumOSVersion", iosRuntimeMinimum},
	} {
		if _, err := c.minimumVersion(item[0], item[1]); err != nil {
			return err
		}
	}
	return nil
}

func (c *Config) validateIPadMultitasking() error {
	seen := make(map[string]bool, len(c.Apple.Orientations))
	for _, orientation := range c.Apple.Orientations {
		if !slices.Contains(iosOrientations, orientation) {
			return fmt.Errorf("nsgo.yaml: apple.orientations has %q; use portrait, portrait_upside_down, landscape_left or landscape_right", orientation)
		}
		if seen[orientation] {
			return fmt.Errorf("nsgo.yaml: apple.orientations repeats %q", orientation)
		}
		seen[orientation] = true
	}
	if !c.IOSRequiresFullScreen() && len(c.Apple.Orientations) > 0 && len(c.Apple.Orientations) < len(iosOrientations) {
		return fmt.Errorf("nsgo.yaml: apple.requires_full_screen is false, so apple.orientations must list all four orientations, as an iPad app that multitasks supports each; leave it out to declare them all")
	}
	for _, k := range [][2]string{
		{"UIRequiresFullScreen", "apple.requires_full_screen"},
		{"UISupportedInterfaceOrientations~ipad", "apple.orientations"},
		{"UIApplicationSupportsMultipleScenes", "apple.multiple_scenes"},
	} {
		if _, ok := c.Apple.Info[k[0]]; ok {
			return fmt.Errorf("nsgo.yaml: apple.info sets %s, which the iOS package writes; set %s instead", k[0], k[1])
		}
	}
	return nil
}

func (c *Config) validateAssetPacks() error {
	names := map[string]bool{}
	for i := range c.Mobile.AssetPacks {
		pack := &c.Mobile.AssetPacks[i]
		if pack.Name == "base" || !assetPackName.MatchString(pack.Name) || names[pack.Name] {
			return fmt.Errorf("mobile.asset_packs has invalid or duplicate name %q", pack.Name)
		}
		names[pack.Name] = true
		switch pack.Delivery {
		case "install-time", "fast-follow":
		default:
			return fmt.Errorf("mobile.asset_packs %s: delivery must be install-time or fast-follow", pack.Name)
		}
		if pack.Dir == "" {
			return fmt.Errorf("mobile.asset_packs %s needs a dir", pack.Name)
		}
		pack.Dir = c.Resolve(pack.Dir)
		info, err := os.Lstat(pack.Dir)
		if err != nil {
			return fmt.Errorf("mobile.asset_packs %s: %w", pack.Name, err)
		}
		if !info.IsDir() {
			return fmt.Errorf("mobile.asset_packs %s: %s must be a directory", pack.Name, rel(pack.Dir))
		}
	}
	return nil
}

func (c *Config) validateLicenses() error {
	for _, rule := range []struct{ name, value string }{
		{"unknown", c.Licenses.Unknown},
		{"copyleft", c.Licenses.Copyleft},
	} {
		switch rule.value {
		case "", "error", "warn":
		default:
			return fmt.Errorf("licenses.%s is %q; use error or warn", rule.name, rule.value)
		}
	}
	for _, module := range c.Licenses.Allow {
		if module == "" || len(strings.Fields(module)) != 1 || strings.TrimSpace(module) != module {
			return fmt.Errorf("licenses.allow needs exact module paths, got %q", module)
		}
	}
	return nil
}

func (c *Config) validateLaunchScreen() error {
	s := c.LaunchScreen
	if s == nil {
		return nil
	}
	if s.Background == "" && s.Image == "" {
		return fmt.Errorf("launch_screen needs a background or image")
	}
	if (s.Dark.Background != "" && s.Background == "") || (s.Dark.Image != "" && s.Image == "") {
		return fmt.Errorf("launch_screen.dark overrides a background or image set in launch_screen")
	}
	if _, ok := c.Apple.Info["UILaunchScreen"]; ok {
		return fmt.Errorf("launch_screen and apple.info.UILaunchScreen both set the iOS launch screen")
	}
	if err := c.resolveLaunchAppearance("launch_screen", &s.LaunchAppearance); err != nil {
		return err
	}
	return c.resolveLaunchAppearance("launch_screen.dark", &s.Dark)
}

func (c *Config) resolveLaunchAppearance(key string, a *LaunchAppearance) error {
	if a.Background != "" {
		if _, ok := parseLaunchColor(a.Background); !ok {
			return fmt.Errorf("%s.background: %q is not a #RRGGBB color", key, a.Background)
		}
	}
	if a.Image != "" {
		a.Image = c.Resolve(a.Image)
	}
	return nil
}

func (c *Config) validateFileTypes() error {
	seen := map[string]bool{}
	for i := range c.FileTypes {
		f := &c.FileTypes[i]
		key := fmt.Sprintf("file_types[%d]", i)
		if f.Name == "" {
			return fmt.Errorf("%s.name is required", key)
		}
		if len(f.Extensions) == 0 {
			return fmt.Errorf("%s.extensions is required", key)
		}
		for _, ext := range f.Extensions {
			if !extensionPattern.MatchString(ext) {
				return fmt.Errorf("%s.extensions: %q is not an extension; write flow, not .flow", key, ext)
			}
		}
		for _, m := range f.MIME {
			if !mimePattern.MatchString(m) {
				return fmt.Errorf("%s.mime: %q is not a media type, as video/mp4", key, m)
			}
		}
		if f.ID == "" {
			f.ID = c.ID + "." + strings.ToLower(f.Extensions[0])
		}
		if !utiPattern.MatchString(f.ID) {
			return fmt.Errorf("%s.id %q is not a type identifier, as com.example.app.doc", key, f.ID)
		}
		if seen[f.ID] {
			return fmt.Errorf("%s: the type %q is listed twice", key, f.ID)
		}
		seen[f.ID] = true
		switch f.Role {
		case "", "editor", "viewer":
		default:
			return fmt.Errorf("%s.role is %q; use editor, viewer or leave it out", key, f.Role)
		}
		for _, conf := range f.Conforms {
			if !utiPattern.MatchString(conf) {
				return fmt.Errorf("%s.conforms: %q is not a type identifier, as public.data", key, conf)
			}
		}
		if f.Icon != "" {
			f.Icon = c.Resolve(f.Icon)
			if ext := filepath.Ext(f.Icon); ext != ".icns" && ext != ".png" {
				return fmt.Errorf("%s.icon: %s is neither an .icns nor a .png", key, rel(f.Icon))
			}
			if _, err := os.Stat(f.Icon); err != nil {
				return fmt.Errorf("%s.icon: %w", key, err)
			}
		}
	}
	return nil
}

func (c *Config) validateURLSchemes() error {
	seen := map[string]bool{}
	for i, s := range c.URLSchemes {
		key := fmt.Sprintf("url_schemes[%d]", i)
		if !schemePattern.MatchString(s) {
			return fmt.Errorf("%s: %q is not a URL scheme; write myapp, not myapp://", key, s)
		}
		if len(s) < 2 {
			return fmt.Errorf("%s: %q is a Windows drive's letter; choose a longer scheme", key, s)
		}
		s = strings.ToLower(s)
		if systemSchemes[s] {
			return fmt.Errorf("%s: %q belongs to the system; choose a scheme of the app's own, as its name", key, s)
		}
		if seen[s] {
			return fmt.Errorf("%s: the scheme %q is listed twice", key, s)
		}
		seen[s] = true
		c.URLSchemes[i] = s
	}
	return nil
}

func (c *Config) validateAppLinks() error {
	seen := map[string]bool{}
	for i, raw := range c.AppLinks {
		domain := strings.ToLower(raw)
		if strings.HasPrefix(domain, "*.") || !domainPattern.MatchString(domain) || len(domain) > 253 || net.ParseIP(domain) != nil {
			return fmt.Errorf("app_links[%d] %q is not an explicit DNS host; write example.com without a scheme, path, port or wildcard", i, raw)
		}
		for _, label := range strings.Split(domain, ".") {
			if len(label) > 63 {
				return fmt.Errorf("app_links[%d] %q has a DNS label longer than 63 bytes", i, raw)
			}
		}
		if seen[domain] {
			return fmt.Errorf("app_links[%d]: the host %q is listed twice", i, domain)
		}
		seen[domain] = true
		c.AppLinks[i] = domain
	}
	return nil
}

func (c *Config) validateCommands() error {
	pkgSlug := c.PackageSlug()
	for i, cmd := range c.Commands {
		if !ValidCommandName(cmd.Name) {
			return fmt.Errorf("commands entry %d: name %q must be letters, digits, '-' and '_', starting with a letter or a digit", i+1, cmd.Name)
		}
		base := strings.ToUpper(cmd.Name)
		if slices.Contains([]string{"CON", "PRN", "AUX", "NUL"}, base) || len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return fmt.Errorf("commands: %s is a reserved Windows file name", cmd.Name)
		}
		for _, taken := range []string{c.Name, pkgSlug, "Uninstall", "AppRun"} {
			if strings.EqualFold(cmd.Name, taken) {
				return fmt.Errorf("commands: %s is the name of the app's own %s; name the command another way", cmd.Name, takenFile(c.Name, pkgSlug, taken))
			}
		}
		for _, other := range c.Commands[:i] {
			if strings.EqualFold(cmd.Name, other.Name) {
				return fmt.Errorf("commands: %s and %s name the same file", other.Name, cmd.Name)
			}
		}
		if cmd.Package == "" || strings.HasPrefix(cmd.Package, "-") || strings.Contains(cmd.Package, "...") || strings.ContainsAny(cmd.Package, " \t\r\n") {
			return fmt.Errorf("commands: %s's package %q is not one Go package's path", cmd.Name, cmd.Package)
		}
	}
	return nil
}

// ValidCommandName reports whether name is letters, digits, '-' and '_' and
// starts with a letter or a digit, so it is a safe executable or extension name
// on every platform.
func ValidCommandName(name string) bool {
	for i, r := range name {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || i > 0 && (r == '-' || r == '_')) {
			return false
		}
	}
	return name != ""
}

func takenFile(name, pkgSlug, taken string) string {
	switch taken {
	case name:
		return "executable"
	case pkgSlug:
		return "command in /usr/bin"
	case "Uninstall":
		return "Windows uninstaller"
	}
	return "AppImage entry point"
}

func (c *Config) validateAndroidShareTypes() error {
	seen := make(map[string]bool, len(c.Android.ShareTypes))
	for i, value := range c.Android.ShareTypes {
		media, params, err := mime.ParseMediaType(value)
		major, minor, pair := strings.Cut(media, "/")
		wildcard := media == "*/*" || !strings.Contains(major, "*") &&
			(minor == "*" || !strings.Contains(minor, "*"))
		if err != nil || !pair || major == "" || minor == "" || len(params) != 0 || !wildcard {
			return fmt.Errorf("android.share_types: %q needs a MIME type, such as text/plain or image/*", value)
		}
		if seen[media] {
			return fmt.Errorf("android.share_types: duplicate MIME type %q", media)
		}
		seen[media] = true
		c.Android.ShareTypes[i] = media
	}
	return nil
}

func (c *Config) validateIOSShareTypes() error {
	seen := map[string]bool{}
	for _, uti := range c.Apple.ShareTypes {
		if !utiPattern.MatchString(uti) {
			return fmt.Errorf("apple.share_types: %q needs a uniform type identifier, as public.url", uti)
		}
		if seen[uti] {
			return fmt.Errorf("apple.share_types: duplicate type %q", uti)
		}
		seen[uti] = true
	}
	return nil
}

func (c *Config) validateWidgets() error {
	limit := maxWidgets
	if c.LiveActivities.Enabled {
		limit = maxWidgets - 1
	}
	if len(c.Widgets) > limit {
		return fmt.Errorf("widgets lists %d kinds, more than the %d a widget extension holds", len(c.Widgets), limit)
	}
	seen := map[string]bool{}
	for i, w := range c.Widgets {
		if !widgetKindPattern.MatchString(w.Kind) {
			return fmt.Errorf("widgets[%d].kind %q is not a kind: letters, digits, '.', '_' and '-', up to 64", i, w.Kind)
		}
		if seen[w.Kind] {
			return fmt.Errorf("widgets[%d]: the kind %q is listed twice", i, w.Kind)
		}
		seen[w.Kind] = true
		for _, f := range w.Families {
			if !slices.Contains(widgetFamilies, f) {
				return fmt.Errorf("widgets[%d].families: %q is none of %s", i, f, strings.Join(widgetFamilies, ", "))
			}
		}
	}
	if g := c.Apple.AppGroup; g != "" && !appGroupPattern.MatchString(g) {
		return fmt.Errorf("apple.app_group %q is not an app group, as group.com.example.app", g)
	}
	return nil
}

func (c *Config) validateAppClip() error {
	clip := c.AppClip
	if clip == nil {
		return nil
	}
	if clip.Name == "" {
		clip.Name = c.Name + " Clip"
	}
	if clip.ID == "" {
		clip.ID = c.ID + ".Clip"
	}
	if !strings.HasPrefix(clip.ID, c.ID+".") || !utiPattern.MatchString(clip.ID) {
		return fmt.Errorf("app_clip.id %q is not under the app's ID, as %s.Clip", clip.ID, c.ID)
	}
	if clip.Main == "" {
		clip.Main = "."
	}
	if clip.Minimum == "" {
		clip.Minimum = "16.0"
	}
	if version, ok := AppleVersion(clip.Minimum); !ok || version[0] < 16 {
		return fmt.Errorf("app_clip.minimum %q is not an iOS version from 16.0", clip.Minimum)
	} else if !strings.Contains(clip.Minimum, ".") {
		clip.Minimum += ".0"
	}
	for i, d := range clip.Domains {
		if !domainPattern.MatchString(d) {
			return fmt.Errorf("app_clip.domains[%d] %q is not a domain; write example.com, not https://example.com", i, d)
		}
	}
	return nil
}

func (c *Config) validatePrivacy() error {
	for name, reasons := range c.Apple.Privacy.AccessedAPIs {
		if !strings.HasPrefix(name, "NSPrivacyAccessedAPICategory") {
			return fmt.Errorf("apple.privacy.accessed_apis: %q is not an NSPrivacyAccessedAPICategory", name)
		}
		if len(reasons) == 0 || slices.Contains(reasons, "") {
			return fmt.Errorf("apple.privacy.accessed_apis: %s needs its reasons, such as E174.1", name)
		}
	}
	for i, item := range c.Apple.Privacy.CollectedData {
		if s, _ := item["NSPrivacyCollectedDataType"].(string); s == "" {
			return fmt.Errorf("apple.privacy.collected_data[%d] has no NSPrivacyCollectedDataType", i)
		}
	}
	return nil
}

func (c *Config) validateCloud() error {
	for _, section := range []struct {
		name string
		list []string
	}{
		{"cloud.containers", c.Cloud.Containers},
		{"icloud.containers", c.ICloud.Containers},
		{"apple.icloud.containers", c.Apple.ICloud.Containers},
	} {
		seen := map[string]bool{}
		for _, item := range section.list {
			if item == "" || strings.ContainsAny(item, " \t\r\n") {
				return fmt.Errorf("%s has invalid container %q", section.name, item)
			}
			if !strings.HasPrefix(item, "iCloud.") || len(item) == len("iCloud.") {
				return fmt.Errorf("%s container %q must start with \"iCloud.\"", section.name, item)
			}
			if seen[item] {
				return fmt.Errorf("%s repeats container %q", section.name, item)
			}
			seen[item] = true
		}
	}
	return nil
}

func (c *Config) validateUpdates() error {
	u := &c.Updates
	if err := ValidateChannel(u.Channel); err != nil {
		return err
	}
	if u.Key == "" {
		if u.Channel == "" && (u.Feed != "" || u.Automatic != nil || u.AutomaticDownloads) {
			return errors.New("updates: key holds no public key; make one with nsgo keygen, or set channel to store")
		}
		return nil
	}
	if _, err := pkg.DecodePublicKey(u.Key); err != nil {
		return fmt.Errorf("updates: key: %w; set it to the public key nsgo keygen prints", err)
	}
	if u.Feed == "" {
		return nil
	}
	f, err := url.Parse(u.Feed)
	if err != nil || f.Scheme != "https" || f.Host == "" || strings.ContainsAny(u.Feed, "'\"") {
		return fmt.Errorf("updates: feed %q is not an https URL", u.Feed)
	}
	if !strings.HasSuffix(u.Feed, "/") {
		u.Feed += "/"
	}
	return nil
}

func (c *Config) validateLinuxCategories() error {
	categories := c.LinuxCategories()
	if len(categories) == 0 {
		return nil
	}
	main := false
	for i, cat := range categories {
		switch {
		case slices.Contains(linuxMainCategories, cat):
			main = true
		case slices.Contains(linuxAdditionalCategories, cat):
		case slices.Contains(linuxReservedCategories, cat):
			return fmt.Errorf("linux.categories entry %d, %q, is reserved for desktop components", i+1, cat)
		case strings.HasPrefix(cat, "X-") && len(cat) > 2 && strings.Trim(cat[2:], "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_.") == "":
		default:
			return fmt.Errorf("linux.categories entry %d, %q, is not a freedesktop category; an extension starts with X- and holds letters, digits, '-', '_' and '.'", i+1, cat)
		}
		if slices.Index(categories, cat) < i {
			return fmt.Errorf("linux.categories names %q twice", cat)
		}
	}
	for _, cat := range []string{"Audio", "Video"} {
		if slices.Contains(categories, cat) && !slices.Contains(categories, "AudioVideo") {
			return fmt.Errorf("linux.categories names %s without AudioVideo, which it belongs to", cat)
		}
	}
	if !main {
		return fmt.Errorf("linux.categories names no main category, so menus would file the app under Other; add one of %s", strings.Join(linuxMainCategories, ", "))
	}
	return nil
}

func (c *Config) resolveIcons() error {
	if err := c.resolveIconSet(&c.Icon, "icon", ""); err != nil {
		return err
	}
	return c.resolveIconSet(&c.Dev.Icon, "dev.icon", "-dev")
}

func (c *Config) resolveIconSet(set *Icons, key, suffix string) error {
	var err error
	if set.Apple, err = c.iconPath(key, set.Apple, "AppIcon"+suffix+".icon", "AppIcon"+suffix+".icns"); err != nil {
		return err
	}
	if ext := filepath.Ext(set.Apple); set.Apple != "" && ext != ".icon" && ext != ".icns" {
		return fmt.Errorf("%s.apple: %s is neither an .icon nor an .icns", key, rel(set.Apple))
	}
	if set.PNG, err = c.iconPath(key, set.PNG, "Icon"+suffix+".png"); err != nil {
		return err
	}
	if set.TVOS, err = c.iconPath(key, set.TVOS, "AppIconTV"+suffix+".brandassets", "AppIconTV"+suffix+".png"); err != nil {
		return err
	}
	switch ext := filepath.Ext(set.TVOS); ext {
	case "", ".png":
	case ".brandassets":
		if info, err := os.Stat(set.TVOS); err != nil || !info.IsDir() {
			return fmt.Errorf("%s.tvos: %s is not a .brandassets folder", key, rel(set.TVOS))
		}
	default:
		return fmt.Errorf("%s.tvos: %s is neither a .brandassets nor a .png", key, rel(set.TVOS))
	}
	if set.WatchOS, err = c.iconPath(key, set.WatchOS, "AppIconWatch"+suffix+".icon", "AppIconWatch"+suffix+".png"); err != nil {
		return err
	}
	if ext := filepath.Ext(set.WatchOS); set.WatchOS != "" && ext != ".icon" && ext != ".png" {
		return fmt.Errorf("%s.watchos: %s is neither an .icon nor a .png", key, rel(set.WatchOS))
	}
	if set.Windows, err = c.iconPath(key+".windows", set.Windows, "IconWindows"+suffix+".png"); err != nil {
		return err
	}
	if set.Linux, err = c.iconPath(key+".linux", set.Linux, "IconLinux"+suffix+".png"); err != nil {
		return err
	}
	if set.Android != nil {
		if set.Android.PNG != "" {
			if set.Android.PNG, err = c.iconPath(key+".android.png", set.Android.PNG); err != nil {
				return err
			}
		}
		adaptive := set.Android.Foreground != "" || set.Android.Background != "" || set.Android.Monochrome != ""
		if adaptive && (set.Android.Foreground == "" || set.Android.Background == "") {
			return fmt.Errorf("%s.android needs foreground and background", key)
		}
		if !adaptive {
			if set.Android.PNG == "" {
				return fmt.Errorf("%s.android needs png or foreground and background", key)
			}
			return nil
		}
		if strings.HasPrefix(set.Android.Background, "#") {
			rgba, err := hex.DecodeString(set.Android.Background[1:])
			if err != nil || len(rgba) != 3 && len(rgba) != 4 {
				return fmt.Errorf("icon.android.background: %q is not a #RRGGBB or #RRGGBBAA color", set.Android.Background)
			}
		} else {
			resolved := c.Resolve(set.Android.Background)
			if _, err := os.Stat(resolved); err != nil {
				return fmt.Errorf("%s.android.background: %w", key, err)
			}
			set.Android.Background = resolved
		}
		for name, path := range map[string]*string{"foreground": &set.Android.Foreground, "monochrome": &set.Android.Monochrome} {
			if *path == "" {
				continue
			}
			resolved := c.Resolve(*path)
			if _, err := os.Stat(resolved); err != nil {
				return fmt.Errorf("%s.android.%s: %w", key, name, err)
			}
			*path = resolved
		}
	}
	return nil
}

func (c *Config) iconPath(key, path string, defaults ...string) (string, error) {
	if path != "" {
		path = c.Resolve(path)
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("%s: %w", key, err)
		}
		return path, nil
	}
	for _, name := range defaults {
		path := filepath.Join(c.Dir, assetsDir, name)
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	return "", nil
}

func (c *Config) resolveInstallerFiles() error {
	for _, f := range []struct {
		key  string
		path *string
		exts []string
	}{
		{"macos.dmg_background", &c.MacOS.DMGBackground, []string{".png"}},
		{"macos.dmg_icon", &c.MacOS.DMGIcon, []string{".icns", ".png"}},
		{"windows.installer_icon", &c.Windows.InstallerIcon, []string{".ico", ".png"}},
		{"windows.uninstaller_icon", &c.Windows.UninstallerIcon, []string{".ico", ".png"}},
	} {
		if *f.path == "" {
			continue
		}
		*f.path = c.Resolve(*f.path)
		if _, err := os.Stat(*f.path); err != nil {
			return fmt.Errorf("%s: %w", f.key, err)
		}
		if ext := strings.ToLower(filepath.Ext(*f.path)); !slices.Contains(f.exts, ext) {
			return fmt.Errorf("%s: %s is not %s", f.key, rel(*f.path), strings.Join(f.exts, " or "))
		}
	}
	return nil
}

func rel(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return path
}
