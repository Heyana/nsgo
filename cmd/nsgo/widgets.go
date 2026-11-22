package main

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Widgets (package widget) need a widget extension in the app's bundle,
// which WidgetKit runs to draw them. WidgetKit declares widgets in Swift
// alone, so the tool compiles one with Xcode's swiftc from
// widgets/Widgets.swift, which draws the timelines the app writes with
// SwiftUI, and Kinds.swift, which it writes from nsgo.yaml's widgets.
// Beside it in the bundle's frameworks goes a library of
// widgets/Reload.swift, through which package widget asks WidgetKit to
// reload them, as WidgetCenter has no Objective-C interface. The
// Info.plists of both say where the timelines are: on iOS in the container
// of an app group they share (widgetGroupKey), and on macOS in a directory
// of the user's home (widgetPathKey), which the app writes and the
// extension, in the App Sandbox, may read, since macOS opens a group's
// container to no app signed ad hoc.
//
// Live Activities (live_activities) are iOS's alone: the extension draws them with widgets/ActivityViews.swift, and the library starts, updates and ends them with widgets/Activities.swift, both sharing widgets/Activity.swift for their attributes. Widget and activity buttons share widgets/Actions.swift, whose intent runs in the app. App Intents finds intents by the metadata Xcode extracts as it builds (appintentsmetadataprocessor), which the tool extracts as Xcode does, into the extension and the app.

//go:embed widgets/Widgets.swift widgets/Reload.swift widgets/Actions.swift widgets/Activity.swift widgets/Activities.swift widgets/ActivityViews.swift
var widgetSources embed.FS

const (
	widgetsAppex = "Widgets.appex"
	widgetsExec  = "Widgets"
	widgetKit    = "NSGoWidgetKit"
	// widgetKitDylib is the reload library on macOS, a plain library among
	// the frameworks; iOS's is a framework, as the App Store takes no
	// other.
	widgetKitDylib = "libNSGoWidgetKit.dylib"
	widgetGroupKey = "NSGoAppGroup"
	widgetPathKey  = "NSGoWidgetsPath"
	// maxWidgets is how many kinds a WidgetBundle holds, Live Activities
	// counting as one.
	maxWidgets = 10
	// appIntentsMetadata is where App Intents reads a bundle's intents.
	appIntentsMetadata = "Metadata.appintents"
)

// liveActivities is nsgo.yaml's live_activities.
type liveActivities struct {
	// Enabled lets the app start Live Activities on iOS and Android.
	Enabled bool `yaml:"enabled"`
	// FrequentUpdates lets an iOS app update its activities more often
	// than the system's budget, as the user allows in its settings
	// (NSSupportsLiveActivitiesFrequentUpdates).
	FrequentUpdates bool `yaml:"frequent_updates"`
}

// iosExtension reports whether the iOS app has a widget extension: for
// its widgets or its Live Activities.
func (p *project) iosExtension() bool { return len(p.Widgets) > 0 || p.LiveActivities.Enabled }

// androidPermissions are the permissions the Android app asks for: android.permissions, with those of Live Updates when it has Live Activities, the foreground service's an overlay over other apps takes when it asks for SYSTEM_ALERT_WINDOW (androidOverlay), and REQUEST_INSTALL_PACKAGES on the direct channel, whose updater hands the next APK to the package installer. A store's build never asks for it, as stores review it.
func (p *project) androidPermissions() []string {
	perms := p.Android.Permissions
	var added []string
	if p.LiveActivities.Enabled {
		added = append(added, "android.permission.POST_NOTIFICATIONS", "android.permission.POST_PROMOTED_NOTIFICATIONS")
	}
	if p.androidOverlay() {
		added = append(added, "android.permission.FOREGROUND_SERVICE", "android.permission.FOREGROUND_SERVICE_SPECIAL_USE")
	}
	if p.updates() {
		added = append(added, "android.permission.REQUEST_INSTALL_PACKAGES")
	}
	for _, perm := range added {
		if !slices.Contains(perms, perm) {
			perms = append(slices.Clip(perms), perm)
		}
	}
	return perms
}

// androidOverlay reports whether the Android app shows overlays over other apps (mobile.OpenOverlay): it lists SYSTEM_ALERT_WINDOW in android.permissions, and its manifest declares the runtime's overlay service, which no other app does.
func (p *project) androidOverlay() bool {
	return slices.Contains(p.Android.Permissions, "android.permission.SYSTEM_ALERT_WINDOW")
}

// androidLocation reports whether the Android app runs location streams in the background (core.LocationOptions.Background): it lists FOREGROUND_SERVICE_LOCATION in android.permissions, and its APK's manifest declares the runtime's location service, which no other app does; an -embed library leaves the service to the host's manifest.
func (p *project) androidLocation() bool {
	return slices.Contains(p.Android.Permissions, "android.permission.FOREGROUND_SERVICE_LOCATION")
}

// widgetKind is a kind of the app's widgets in nsgo.yaml.
type widgetKind struct {
	// Kind names it for widget.Update; Name and Description show in the
	// system's gallery of widgets.
	Kind        string `yaml:"kind"`
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	// Families are the sizes it comes in: small, medium and large by
	// default.
	Families []string `yaml:"families"`
}

// widgetLocalizedText is either a catalog key or a literal widget label. The widget extension looks a key up at run time, and shows a literal verbatim.
type widgetLocalizedText struct {
	key, value string
}

// widgetLocalizations holds the catalog strings a widget extension uses. Every language has the source language's text for the keys it does not translate.
type widgetLocalizations struct {
	source string                       // the source language, the extension's development region
	texts  map[string]map[string]string // by language, then key
}

// widgetLocalizations reads the app's catalog messages that widget names and descriptions name. A widget field is a key only when it names a message of the source catalog, which must take no arguments. An empty name shows the kind and is not looked up.
func (p *project) widgetLocalizations() (*widgetLocalizations, error) {
	if p.I18n.Dir == "" || !hasCatalogs(p.I18n.Dir) {
		return nil, nil
	}
	c, ds := loadCatalog(p.I18n.Dir, p.I18n.Source)
	if err := ds.err(); err != nil {
		return nil, err
	}
	src := c.files[c.source]
	keys := map[string]bool{}
	for _, w := range p.Widgets {
		for _, value := range []string{w.Name, w.Description} {
			if value != "" && src.msgs[value] != nil {
				if len(c.params[value]) > 0 {
					return nil, fmt.Errorf("%s: %s names a widget label, so it takes no arguments", filepath.Base(src.path), value)
				}
				keys[value] = true
			}
		}
	}
	if len(keys) == 0 {
		return nil, nil
	}
	localizations := &widgetLocalizations{source: c.source, texts: map[string]map[string]string{}}
	for lang, f := range c.files {
		texts := map[string]string{}
		for key := range keys {
			m := f.msgs[key]
			if m == nil {
				m = src.msgs[key]
			}
			texts[key] = constantText(m.parts)
		}
		localizations.texts[lang] = texts
	}
	return localizations, nil
}

func (l *widgetLocalizations) resolve(value string) widgetLocalizedText {
	if l == nil || value == "" {
		return widgetLocalizedText{value: value}
	}
	for _, texts := range l.texts {
		if _, ok := texts[value]; ok {
			return widgetLocalizedText{key: value}
		}
	}
	return widgetLocalizedText{value: value}
}

func (t widgetLocalizedText) swiftText() string {
	if t.key != "" {
		return "Text(LocalizedStringKey(" + swiftString(t.key) + "))"
	}
	return "Text(verbatim: " + swiftString(t.value) + ")"
}

// writeWidgetLocalizations writes the extension's Localizable.strings files into resources. SwiftUI's LocalizedStringKey looks them up in the extension's bundle, not the app's.
func (p *project) writeWidgetLocalizations(resources string) error {
	l, err := p.widgetLocalizations()
	if err != nil || l == nil {
		return err
	}
	for lang, texts := range l.texts {
		var b strings.Builder
		for _, key := range slices.Sorted(maps.Keys(texts)) {
			fmt.Fprintf(&b, "%s = %s;\n", appleString(key), appleString(texts[key]))
		}
		dir := filepath.Join(resources, lang+".lproj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, "Localizable.strings"), []byte(b.String()), 0o644); err != nil {
			return err
		}
	}
	return nil
}

var (
	widgetKindPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	widgetFamilies    = []string{"small", "medium", "large", "extra_large", "circular", "rectangular", "inline"}
	appGroupPattern   = regexp.MustCompile(`^group\.[A-Za-z0-9][A-Za-z0-9.-]*$`)
)

func (p *project) validateWidgets() error {
	if limit := p.widgetLimit(); len(p.Widgets) > limit {
		return fmt.Errorf("widgets lists %d kinds, more than the %d a widget extension holds", len(p.Widgets), limit)
	}
	seen := map[string]bool{}
	for i, w := range p.Widgets {
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
	if g := p.Apple.AppGroup; g != "" && !appGroupPattern.MatchString(g) {
		return fmt.Errorf("apple.app_group %q is not an app group, as group.com.example.app", g)
	}
	return nil
}

// widgetLimit is how many kinds of widgets the extension holds beside the
// Live Activities.
func (p *project) widgetLimit() int {
	if p.LiveActivities.Enabled {
		return maxWidgets - 1
	}
	return maxWidgets
}

func (w widgetKind) name() string {
	if w.Name == "" {
		return w.Kind
	}
	return w.Name
}

func (w widgetKind) families() []string {
	if len(w.Families) == 0 {
		return []string{"small", "medium", "large"}
	}
	return w.Families
}

// appGroup is the app group iOS and provisioned Mac App Store apps share with their widgets, and the iOS app with its App Clip.
func (p *project) appGroup() string {
	if p.Apple.AppGroup != "" {
		return p.Apple.AppGroup
	}
	return "group." + p.ID
}

// macWidgetsPath is where the macOS app keeps what its widgets show, from
// the user's home.
func (p *project) macWidgetsPath() string {
	return filepath.Join("Library", "Application Support", p.ID, "Widgets")
}

// macWidgetsUseAppGroup selects the shared container required by sandboxed Mac App Store builds. Direct and development builds retain their existing home-directory path, which needs no provisioning profile.
func (p *project) macWidgetsUseAppGroup() bool {
	return p.Release && p.channel() == channelStore
}

// widgetPlist returns the entries of the app's Info.plist that tell
// package widget where the timelines are and whether the app has Live
// Activities, "" for an app without widgets.
func (p *project) widgetPlist(ios bool) string {
	switch {
	case ios && p.iosExtension():
		s := "<key>" + widgetGroupKey + "</key><string>" + xmlText(p.appGroup()) + "</string>"
		if p.LiveActivities.Enabled {
			s += "\n\t<key>NSSupportsLiveActivities</key><true/>"
			if p.LiveActivities.FrequentUpdates {
				s += "\n\t<key>NSSupportsLiveActivitiesFrequentUpdates</key><true/>"
			}
		}
		return s
	case ios || len(p.Widgets) == 0:
		return ""
	}
	if p.macWidgetsUseAppGroup() {
		return "<key>" + widgetGroupKey + "</key><string>" + xmlText(p.appGroup()) + "</string>"
	}
	return "<key>" + widgetPathKey + "</key><string>" + xmlText(p.macWidgetsPath()) + "</string>"
}

// xmlText escapes s for XML's text and attributes.
func xmlText(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// kindsSwift returns Kinds.swift, the declaration of the kinds of widgets
// and, for iOS, of the Live Activities.
func kindsSwift(p *project, ios bool) (string, error) {
	localizations, err := p.widgetLocalizations()
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("// Code generated by nsgo from nsgo.yaml's widgets. DO NOT EDIT.\n\nimport SwiftUI\nimport WidgetKit\n")
	for i, w := range p.Widgets {
		name := widgetLocalizedText{value: w.name()}
		if w.Name != "" {
			name = localizations.resolve(w.Name)
		}
		description := localizations.resolve(w.Description)
		var fams []string
		for _, f := range w.families() {
			fams = append(fams, swiftString(f))
		}
		fmt.Fprintf(&b, `
struct NSGoWidget%d: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: %s, provider: NSGoProvider(kind: %[2]s)) { entry in
            NSGoEntryView(entry: entry, name: %[3]s, description: %[4]s, hasDescription: %[5]t)
        }
        .configurationDisplayName(%[3]s)
        .description(%[4]s)
        .supportedFamilies(families([%[6]s]))
    }
}
`, i, swiftString(w.Kind), name.swiftText(), description.swiftText(), w.Description != "", strings.Join(fams, ", "))
	}
	b.WriteString("\n@main\nstruct NSGoWidgets: WidgetBundle {\n    var body: some Widget {\n")
	for i := range p.Widgets {
		fmt.Fprintf(&b, "        NSGoWidget%d()\n", i)
	}
	if ios && p.LiveActivities.Enabled {
		b.WriteString("        if #available(iOS 16.1, *) {\n            NSGoActivityWidget()\n        }\n")
	}
	b.WriteString("    }\n}\n")
	return b.String(), nil
}

// swiftString quotes s as a Swift string literal.
func swiftString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '\\':
			b.WriteByte('\\')
			b.WriteRune(r)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			if r < 0x20 || r == 0x7f {
				fmt.Fprintf(&b, `\u{%x}`, r)
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return b.String()
}

// widgetTarget is what the widget extension is compiled for.
type widgetTarget struct {
	sdk, triple string
	ios         bool
}

func macWidgetTarget(arch string) widgetTarget {
	if arch == "amd64" {
		arch = "x86_64"
	}
	return widgetTarget{sdk: "macosx", triple: arch + "-apple-macos11.0"}
}

func iosWidgetTarget(simulator bool) widgetTarget {
	if simulator {
		return widgetTarget{sdk: "iphonesimulator", triple: "arm64-apple-ios15.0-simulator", ios: true}
	}
	return widgetTarget{sdk: "iphoneos", triple: "arm64-apple-ios15.0", ios: true}
}

// compiledWidgets are the widget extension's executable and the library of Swift that compileWidgets made, with the App Intents metadata of each.
type compiledWidgets struct {
	exe, lib string
	// exeIntents and libIntents are the Metadata.appintents of the
	// extension and of the app, which runs the library's intents; "" for
	// none.
	exeIntents, libIntents string
}

// compileWidgets compiles the widget extension's executable and the
// library for t. They are kept in the user's cache by their sources, the
// target and the toolchain, as actool's icons are, so builds compile them
// once.
func compileWidgets(ctx context.Context, p *project, t widgetTarget) (compiledWidgets, error) {
	read := func(name string) string {
		data, _ := widgetSources.ReadFile("widgets/" + name)
		return string(data)
	}
	kinds, err := kindsSwift(p, t.ios)
	if err != nil {
		return compiledWidgets{}, err
	}
	ext := map[string]string{"Widgets.swift": read("Widgets.swift"), "Kinds.swift": kinds, "Actions.swift": read("Actions.swift")}
	lib := map[string]string{"Reload.swift": read("Reload.swift"), "Actions.swift": read("Actions.swift")}
	if t.ios {
		// The extension's buttons name the intent on iOS, with or without
		// Live Activities.
		ext["Activity.swift"], ext["ActivityViews.swift"] = read("Activity.swift"), read("ActivityViews.swift")
		lib["Activity.swift"], lib["Activities.swift"] = read("Activity.swift"), read("Activities.swift")
	}
	version, err := output(ctx, "xcrun", "--sdk", t.sdk, "swiftc", "--version")
	if err != nil {
		return compiledWidgets{}, fmt.Errorf("widgets need Xcode's swiftc: %w", err)
	}
	sdk, err := output(ctx, "xcrun", "--sdk", t.sdk, "--show-sdk-path")
	if err != nil {
		return compiledWidgets{}, err
	}
	sum := sha256.New()
	parts := []string{version, sdk, t.triple}
	for _, files := range []map[string]string{ext, lib} {
		for _, name := range slices.Sorted(maps.Keys(files)) {
			parts = append(parts, name, files[name])
		}
		parts = append(parts, "|")
	}
	for _, part := range parts {
		fmt.Fprintf(sum, "%d:%s", len(part), part)
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return compiledWidgets{}, err
	}
	dir := filepath.Join(cache, "nsgo", "widgets", hex.EncodeToString(sum.Sum(nil)[:16]))
	out := compiledWidgets{exe: filepath.Join(dir, widgetsExec), lib: filepath.Join(dir, widgetKit)}
	out.exeIntents = filepath.Join(dir, "extension", appIntentsMetadata)
	out.libIntents = filepath.Join(dir, "app", appIntentsMetadata)
	if _, err := os.Stat(out.lib); err == nil {
		return out, nil
	}
	stage(ctx, "compiling the widgets")
	if err := os.MkdirAll(filepath.Dir(dir), 0o755); err != nil {
		return compiledWidgets{}, err
	}
	tmp, err := os.MkdirTemp(filepath.Dir(dir), "tmp-")
	if err != nil {
		return compiledWidgets{}, err
	}
	defer os.RemoveAll(tmp)
	write := func(sub string, files map[string]string) ([]string, error) {
		if err := os.MkdirAll(filepath.Join(tmp, sub), 0o755); err != nil {
			return nil, err
		}
		var paths []string
		for _, name := range slices.Sorted(maps.Keys(files)) {
			path := filepath.Join(tmp, sub, name)
			if err := os.WriteFile(path, []byte(files[name]), 0o644); err != nil {
				return nil, err
			}
			paths = append(paths, path)
		}
		return paths, nil
	}
	extFiles, err := write("ext", ext)
	if err != nil {
		return compiledWidgets{}, err
	}
	libFiles, err := write("lib", lib)
	if err != nil {
		return compiledWidgets{}, err
	}
	installName := "@rpath/" + widgetKitDylib
	if t.ios {
		installName = "@rpath/" + widgetKit + ".framework/" + widgetKit
	}
	if err := os.Mkdir(filepath.Join(tmp, "out"), 0o755); err != nil {
		return compiledWidgets{}, err
	}
	protocols, err := writeIntentProtocols(ctx, tmp)
	if err != nil {
		return compiledWidgets{}, err
	}
	swiftc := func(out, module string, files []string, args ...string) error {
		args = append([]string{"--sdk", t.sdk, "swiftc", "-target", t.triple, "-sdk", sdk, "-O", "-swift-version", "5",
			"-parse-as-library", "-module-name", module, "-o", filepath.Join(tmp, "out", out)}, args...)
		if module == "NSGoWidgets" {
			args = append(args, "-application-extension")
		}
		if t.ios {
			// ActivityKit and App Intents are younger than the extension's
			// iOS 15; App Intents reads the intents' constant values.
			args = append(args, "-Xlinker", "-weak_framework", "-Xlinker", "ActivityKit")
		}
		args = append(args, "-Xlinker", "-weak_framework", "-Xlinker", "AppIntents",
			"-wmo", "-emit-const-values-path", filepath.Join(tmp, module+".swiftconstvalues"),
			"-Xfrontend", "-const-gather-protocols-file", "-Xfrontend", protocols)
		if err := run(ctx, "xcrun", append(args, files...)...); err != nil {
			return fmt.Errorf("compiling the widgets: %w", err)
		}
		return nil
	}
	// App extensions start in NSExtensionMain, as Xcode links them, which
	// runs the WidgetBundle of @main.
	if err := swiftc(widgetsExec, "NSGoWidgets", extFiles, "-Xlinker", "-e", "-Xlinker", "_NSExtensionMain", "-Xlinker", "-dead_strip"); err != nil {
		return compiledWidgets{}, err
	}
	libArgs := []string{"-emit-library", "-Xlinker", "-install_name", "-Xlinker", installName, "-DNSGO_APP"}
	if err := swiftc(widgetKit, widgetKit, libFiles, libArgs...); err != nil {
		return compiledWidgets{}, err
	}
	for _, m := range []struct {
		module, sub string
		files       []string
	}{{"NSGoWidgets", "extension", extFiles}, {widgetKit, "app", libFiles}} {
		if err := extractIntents(ctx, t, sdk, tmp, m.module, m.files, filepath.Join(tmp, "out", m.sub)); err != nil {
			return compiledWidgets{}, err
		}
	}
	if err := os.Rename(filepath.Join(tmp, "out"), dir); err != nil {
		if _, statErr := os.Stat(out.lib); statErr != nil {
			return compiledWidgets{}, err
		}
	}
	return out, nil
}

// writeIntentProtocols writes the protocols whose conformances swiftc
// records the constant values of, which App Intents reads: those the
// toolchain lists, as Xcode passes them.
func writeIntentProtocols(ctx context.Context, dir string) (string, error) {
	protocols := []string{"AppIntent", "AppEntity", "AppEnum", "EntityQuery", "TransientEntity", "AppShortcutsProvider", "DynamicOptionsProvider", "AppIntentsPackage"}
	if swiftc, err := output(ctx, "xcrun", "--find", "swiftc"); err == nil {
		data, err := os.ReadFile(filepath.Join(filepath.Dir(filepath.Dir(swiftc)), "share", "swift", "SwiftConstantValues", "AppIntents.json"))
		var list struct {
			Protocols []string `json:"constValueProtocols"`
		}
		if err == nil && json.Unmarshal(data, &list) == nil && len(list.Protocols) > 0 {
			protocols = list.Protocols
		}
	}
	data, _ := json.Marshal(protocols)
	path := filepath.Join(dir, "protocols.json")
	return path, os.WriteFile(path, data, 0o644)
}

// extractIntents writes the App Intents metadata of a module compiled from
// files into out, as Xcode's ExtractAppIntentsMetadata does.
func extractIntents(ctx context.Context, t widgetTarget, sdk, tmp, module string, files []string, out string) error {
	swiftc, err := output(ctx, "xcrun", "--find", "swiftc")
	if err != nil {
		return err
	}
	xcode, err := output(ctx, "xcodebuild", "-version")
	if err != nil {
		return err
	}
	build := xcode[strings.LastIndex(xcode, " ")+1:]
	list := func(name string, lines []string) (string, error) {
		path := filepath.Join(tmp, module+"."+name)
		return path, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644)
	}
	sources, err := list("SwiftFileList", files)
	if err != nil {
		return err
	}
	consts, err := list("SwiftConstValuesFileList", []string{filepath.Join(tmp, module+".swiftconstvalues")})
	if err != nil {
		return err
	}
	empty, err := list("MetadataFileList", nil)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	platform, suffix := "macOS", "-apple-macos"
	if t.ios {
		platform, suffix = "iOS", "-apple-ios"
	}
	_, deployment, _ := strings.Cut(strings.TrimSuffix(t.triple, "-simulator"), suffix)
	err = run(ctx, "xcrun", "appintentsmetadataprocessor",
		"--toolchain-dir", filepath.Dir(filepath.Dir(filepath.Dir(swiftc))),
		"--module-name", module, "--sdk-root", sdk, "--xcode-version", build,
		"--platform-family", platform, "--deployment-target", deployment, "--target-triple", t.triple,
		"--output", out, "--source-file-list", sources, "--swift-const-vals-list", consts,
		"--metadata-file-list", empty, "--static-metadata-file-list", empty,
		"--compile-time-extraction", "--deployment-aware-processing", "--no-app-shortcuts-localization", "--force")
	if err != nil {
		return fmt.Errorf("extracting the App Intents of the widgets: %w", err)
	}
	if _, err := os.Stat(filepath.Join(out, appIntentsMetadata)); err != nil {
		return fmt.Errorf("extracting the App Intents of the widgets wrote no %s", appIntentsMetadata)
	}
	return nil
}

// widgetsInfo returns the widget extension's Info.plist, with the platform's entries.
func widgetsInfo(p *project, platform map[string]any) ([]byte, error) {
	d := map[string]any{
		"CFBundleName":                  p.Name,
		"CFBundleDisplayName":           p.Name,
		"CFBundleIdentifier":            p.ID + ".widgets",
		"CFBundleVersion":               p.BundleVersion(),
		"CFBundleShortVersionString":    p.Version,
		"CFBundleExecutable":            widgetsExec,
		"CFBundlePackageType":           "XPC!",
		"CFBundleInfoDictionaryVersion": "6.0",
		"NSExtension":                   map[string]any{"NSExtensionPointIdentifier": "com.apple.widgetkit-extension"},
	}
	l, err := p.widgetLocalizations()
	if err != nil {
		return nil, err
	}
	if l != nil {
		// Without it a language the catalogs lack falls back to English's .lproj, which an app written in another language does not have, and the widget shows its keys.
		d["CFBundleDevelopmentRegion"] = l.source
	}
	for k, v := range platform {
		d[k] = v
	}
	return encodePlist(d), nil
}

// addMacWidgets puts the widget extension and the reload library into a
// macOS bundle built for arch, signed ad hoc as the app is; signMacApp
// signs them again with an identity.
func addMacWidgets(ctx context.Context, p *project, app, arch string) error {
	if len(p.Widgets) == 0 {
		return nil
	}
	w, err := compileWidgets(ctx, p, macWidgetTarget(arch))
	if err != nil {
		return err
	}
	appex := macWidgetsAppex(app)
	contents := filepath.Join(appex, "Contents")
	for _, m := range []struct{ from, to string }{{w.exeIntents, filepath.Join(contents, "Resources")}, {w.libIntents, filepath.Join(app, "Contents", "Resources")}} {
		if err := copyDir(m.from, filepath.Join(m.to, appIntentsMetadata)); err != nil {
			return err
		}
	}
	if err := copyLibrary(w.exe, filepath.Join(contents, "MacOS", widgetsExec)); err != nil {
		return err
	}
	platform := map[string]any{
		"CFBundleSupportedPlatforms": []any{"MacOSX"},
		"LSMinimumSystemVersion":     "11.0",
	}
	if p.macWidgetsUseAppGroup() {
		platform[widgetGroupKey] = p.appGroup()
	} else {
		platform[widgetPathKey] = p.macWidgetsPath()
	}
	info, err := widgetsInfo(p, platform)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), info, 0o644); err != nil {
		return err
	}
	if err := p.writePrivacyManifest(filepath.Join(contents, "Resources")); err != nil {
		return err
	}
	if err := p.writeWidgetLocalizations(filepath.Join(contents, "Resources")); err != nil {
		return err
	}
	dylib := filepath.Join(app, "Contents", "Frameworks", widgetKitDylib)
	if err := copyLibrary(w.lib, dylib); err != nil {
		return err
	}
	if err := run(ctx, "codesign", "--force", "--sign", "-", dylib); err != nil {
		return err
	}
	return signMacWidgets(ctx, p, appex, "-")
}

// macWidgetsAppex is the widget extension of a macOS bundle.
func macWidgetsAppex(app string) string {
	return filepath.Join(app, "Contents", "PlugIns", widgetsAppex)
}

// signMacWidgets signs a macOS bundle's widget extension, which WidgetKit runs in the App Sandbox, using a home-directory read exception for direct/development builds or the shared group of a store build, which signMacStoreWidgets later authorizes with a profile. It signs before the app, whose signature seals it, and the app is signed without --deep, which would sign it again without these entitlements.
func signMacWidgets(ctx context.Context, p *project, appex, identity string) error {
	values := map[string]any{"com.apple.security.app-sandbox": true}
	if p.macWidgetsUseAppGroup() {
		values["com.apple.security.application-groups"] = []any{p.appGroup()}
	} else {
		values["com.apple.security.temporary-exception.files.home-relative-path.read-only"] = []any{"/" + filepath.ToSlash(p.macWidgetsPath()) + "/"}
	}
	ents, err := writeEntitlements(values)
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

// addIOSWidgets puts the widget extension and the framework of Swift
// into an iOS bundle, with their App Intents metadata. For the simulator
// it signs them ad hoc, the extension with the app group, which the
// simulator grants from the entitlements of ad hoc signatures; device
// builds sign them with the app (signIOSWidgets).
func addIOSWidgets(ctx context.Context, p *project, app string, simulator bool) error {
	if !p.iosExtension() {
		return nil
	}
	w, err := compileWidgets(ctx, p, iosWidgetTarget(simulator))
	if err != nil {
		return err
	}
	exe, lib := w.exe, w.lib
	for _, m := range []struct{ from, to string }{{w.exeIntents, iosWidgetsAppex(app)}, {w.libIntents, app}} {
		if m.from == "" {
			continue
		}
		if err := copyDir(m.from, filepath.Join(m.to, appIntentsMetadata)); err != nil {
			return err
		}
	}
	platform, sdk := "iPhoneOS", "iphoneos"
	if simulator {
		platform, sdk = "iPhoneSimulator", "iphonesimulator"
	}
	appex := iosWidgetsAppex(app)
	if err := copyLibrary(exe, filepath.Join(appex, widgetsExec)); err != nil {
		return err
	}
	platformInfo := map[string]any{
		widgetGroupKey:               p.appGroup(),
		"CFBundleSupportedPlatforms": []any{platform},
		"MinimumOSVersion":           "15.0",
		"UIDeviceFamily":             []any{int64(1), int64(2)},
	}
	if simulator {
		// The simulator's extension names its platform alone: the toolchain metadata is for the store's.
		platformInfo["DTPlatformName"] = sdk
	} else {
		buildInfo, err := appleBuildInfo(ctx, iOS, false)
		if err != nil {
			return err
		}
		for key, value := range buildInfo {
			platformInfo[key] = value
		}
	}
	info, err := widgetsInfo(p, platformInfo)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(appex, "Info.plist"), info, 0o644); err != nil {
		return err
	}
	if err := p.writePrivacyManifest(appex); err != nil {
		return err
	}
	if err := p.writeWidgetLocalizations(appex); err != nil {
		return err
	}
	fw := filepath.Join(app, "Frameworks", widgetKit+".framework")
	if err := copyLibrary(lib, filepath.Join(fw, widgetKit)); err != nil {
		return err
	}
	err = os.WriteFile(filepath.Join(fw, "Info.plist"), encodePlist(map[string]any{
		"CFBundleIdentifier":            p.ID + ".widgetkit",
		"CFBundleExecutable":            widgetKit,
		"CFBundleName":                  widgetKit,
		"CFBundlePackageType":           "FMWK",
		"CFBundleVersion":               p.BundleVersion(),
		"CFBundleShortVersionString":    p.Version,
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleSupportedPlatforms":    []any{platform},
		"DTPlatformName":                sdk,
		"MinimumOSVersion":              "15.0",
	}), 0o644)
	if err != nil || !simulator {
		return err
	}
	if err := run(ctx, "codesign", "--force", "--sign", "-", fw); err != nil {
		return err
	}
	ents, err := writeEntitlements(p.groupEntitlements())
	if err != nil {
		return err
	}
	defer os.Remove(ents)
	return run(ctx, "codesign", "--force", "--sign", "-", "--entitlements", ents, appex)
}

// iosWidgetsAppex is the widget extension of an iOS bundle.
func iosWidgetsAppex(app string) string {
	return filepath.Join(app, "PlugIns", widgetsAppex)
}

// groupEntitlements are the entitlements that share the app group.
func (p *project) groupEntitlements() map[string]any {
	return map[string]any{"com.apple.security.application-groups": []any{p.appGroup()}}
}

// signIOSWidgets signs an iOS device build's widget extension with
// sign.widgets_profile, which it embeds and takes the entitlements from,
// before the app is signed. The reload framework is signed with the app's
// other frameworks.
func signIOSWidgets(ctx context.Context, p *project, app, identity string) error {
	appex := iosWidgetsAppex(app)
	if _, err := os.Stat(appex); err != nil {
		return nil
	}
	if p.Sign.WidgetsProfile == "" {
		return fmt.Errorf("the widget extension %s.widgets needs sign.widgets_profile, its own provisioning profile granting %s", p.ID, p.appGroup())
	}
	data, err := os.ReadFile(p.Sign.WidgetsProfile)
	if err != nil {
		return err
	}
	profile, err := readProfile(ctx, data)
	if err != nil {
		return fmt.Errorf("%s: %w", rel(p.Sign.WidgetsProfile), err)
	}
	entitlements, ok := profile["Entitlements"].(map[string]any)
	if !ok {
		return fmt.Errorf("%s has no entitlements", rel(p.Sign.WidgetsProfile))
	}
	id := p.ID + ".widgets"
	appID, _ := entitlements["application-identifier"].(string)
	if !profileAllows(appID, id) {
		return fmt.Errorf("sign.widgets_profile is for %s, not %s", appID, id)
	}
	entitlements["application-identifier"] = signedAppID(appID, id)
	if groups, _ := entitlements["com.apple.security.application-groups"].([]any); !slices.Contains(groups, any(p.appGroup())) {
		return fmt.Errorf("sign.widgets_profile does not grant the app group %s", p.appGroup())
	}
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

// writeEntitlements writes a property list of entitlements to a temporary
// file, which the caller removes.
func writeEntitlements(d map[string]any) (string, error) {
	f, err := os.CreateTemp("", "nsgo-*.entitlements")
	if err != nil {
		return "", err
	}
	_, err = f.Write(encodePlist(d))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", errors.Join(errors.New("writing entitlements"), err)
	}
	return f.Name(), nil
}
