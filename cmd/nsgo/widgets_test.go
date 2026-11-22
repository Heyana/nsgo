package main

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestWidgetsConfig(t *testing.T) {
	for _, c := range []struct {
		yaml, err string
	}{
		{"widgets: [{kind: counter, families: [small, extra_large, inline]}]", ""},
		{"widgets: [{kind: a/b}]", "is not a kind"},
		{"widgets: [{kind: a}, {kind: a}]", "listed twice"},
		{"widgets: [{kind: a, families: [huge]}]", "is none of"},
		{"apple: {app_group: shared}\nwidgets: [{kind: a}]", "is not an app group"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: app\n"+c.yaml+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := load(dir)
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%s: %v, want %q", c.yaml, err, c.err)
		}
	}
}

func TestKindsSwift(t *testing.T) {
	p := &project{ID: "dev.example.app", Widgets: []widgetKind{
		{Kind: "counter", Name: `The "count"`, Families: []string{"small", "circular"}},
		{Kind: "agenda"},
	}}
	src, err := kindsSwift(p, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`StaticConfiguration(kind: "counter", provider: NSGoProvider(kind: "counter"))`,
		`.configurationDisplayName(Text(verbatim: "The \"count\""))`,
		`.supportedFamilies(families(["small", "circular"]))`,
		`.supportedFamilies(families(["small", "medium", "large"]))`,
		"        NSGoWidget0()\n        NSGoWidget1()\n",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("Kinds.swift lacks %s:\n%s", want, src)
		}
	}
	if got := p.widgetPlist(true); got != "<key>NSGoAppGroup</key><string>group.dev.example.app</string>" {
		t.Errorf("iOS plist = %s", got)
	}
	if got := p.widgetPlist(false); got != "<key>NSGoWidgetsPath</key><string>Library/Application Support/dev.example.app/Widgets</string>" {
		t.Errorf("macOS plist = %s", got)
	}
	p.Updates.Channel = channelStore
	p.Release = true
	if got := p.widgetPlist(false); got != "<key>NSGoAppGroup</key><string>group.dev.example.app</string>" {
		t.Errorf("store macOS plist = %s", got)
	}
	p.Release = false
	if got := p.widgetPlist(false); !strings.Contains(got, "NSGoWidgetsPath") || strings.Contains(got, "NSGoAppGroup") {
		t.Errorf("development macOS plist needs its unprovisioned path: %s", got)
	}
	if (&project{}).widgetPlist(true) != "" {
		t.Error("an app without widgets says where their timelines are")
	}
}

func TestWidgetLocalizations(t *testing.T) {
	dir := t.TempDir()
	locales := filepath.Join(dir, "locales")
	writeFiles(t, locales, map[string]string{
		"en.yaml":      "widgetName: Now playing\nwidgetDescription: What is playing\nplain: Plain\n",
		"zh-Hans.yaml": "widgetName: 正在播放\n",
	})
	p := &project{Dir: dir}
	p.I18n.Dir, p.I18n.Source = locales, "en"
	p.Widgets = []widgetKind{{Kind: "now", Name: "widgetName", Description: "widgetDescription"}, {Kind: "plain", Name: "Plain"}}
	src, err := kindsSwift(p, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Text(LocalizedStringKey(\"widgetName\"))",
		"Text(LocalizedStringKey(\"widgetDescription\"))",
		"Text(verbatim: \"Plain\")",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("Kinds.swift lacks %s:\n%s", want, src)
		}
	}
	resources := filepath.Join(dir, "Resources")
	if err := p.writeWidgetLocalizations(resources); err != nil {
		t.Fatal(err)
	}
	for lang, want := range map[string]string{
		"en":      `"widgetName" = "Now playing";`,
		"zh-Hans": `"widgetName" = "正在播放";`,
	} {
		data, err := os.ReadFile(filepath.Join(resources, lang+".lproj", "Localizable.strings"))
		if err != nil || !strings.Contains(string(data), want) {
			t.Errorf("%s localization = %q, want %q, error %v", lang, data, want, err)
		}
	}
	data, err := os.ReadFile(filepath.Join(resources, "zh-Hans.lproj", "Localizable.strings"))
	if err != nil || !strings.Contains(string(data), `"widgetDescription" = "What is playing";`) {
		t.Errorf("zh-Hans missing source fallback = %q, error %v", data, err)
	}
	info, err := widgetsInfo(p, nil)
	if err != nil || !strings.Contains(string(info), "<key>CFBundleDevelopmentRegion</key>\n\t<string>en</string>") {
		t.Errorf("the extension's Info.plist = %s, error %v, want the source language as its development region", info, err)
	}
	writeFiles(t, locales, map[string]string{"en.yaml": "widgetName: Now playing\nwithArgs:\n  message: '{n}'\n  placeholders:\n    n: {type: int}\n"})
	p.Widgets = []widgetKind{{Kind: "bad", Name: "withArgs"}}
	if _, err := kindsSwift(p, false); err == nil || !strings.Contains(err.Error(), "takes no arguments") {
		t.Fatalf("parameterized widget key error = %v", err)
	}
}

func TestLiveActivitiesConfig(t *testing.T) {
	p := &project{ID: "dev.example.app", LiveActivities: liveActivities{Enabled: true, FrequentUpdates: true}}
	p.Android.Permissions = []string{"android.permission.CAMERA", "android.permission.POST_NOTIFICATIONS"}
	if !p.iosExtension() {
		t.Error("an app with Live Activities has no extension")
	}
	if got := p.widgetPlist(true); !strings.Contains(got, "<key>NSGoAppGroup</key>") || !strings.Contains(got, "<key>NSSupportsLiveActivities</key><true/>") ||
		!strings.Contains(got, "<key>NSSupportsLiveActivitiesFrequentUpdates</key><true/>") {
		t.Errorf("iOS plist = %s", got)
	}
	if got := p.widgetPlist(false); got != "" {
		t.Errorf("macOS plist of an app without widgets = %s", got)
	}
	if src, err := kindsSwift(p, true); err != nil || !strings.Contains(src, "NSGoActivityWidget()") {
		t.Errorf("Kinds.swift lacks the activities:\n%s", src)
	}
	if src, err := kindsSwift(p, false); err != nil || strings.Contains(src, "NSGoActivityWidget") {
		t.Errorf("macOS's Kinds.swift has the activities:\n%s", src)
	}
	want := []string{"android.permission.CAMERA", "android.permission.POST_NOTIFICATIONS", "android.permission.POST_PROMOTED_NOTIFICATIONS"}
	if got := p.androidPermissions(); !slices.Equal(got, want) {
		t.Errorf("Android permissions = %v, want %v", got, want)
	}
	if !slices.Equal(p.Android.Permissions, want[:2]) {
		t.Errorf("android.permissions changed to %v", p.Android.Permissions)
	}
	p.Widgets = make([]widgetKind, maxWidgets)
	for i := range p.Widgets {
		p.Widgets[i].Kind = fmt.Sprint("w", i)
	}
	if err := p.validateWidgets(); err == nil {
		t.Errorf("%d kinds of widgets and Live Activities fit a bundle", maxWidgets)
	}
}

// TestWidgetsCompile compiles the extension and the library for this Mac,
// and for the iOS simulator with Live Activities and their intents' App
// Intents metadata, which the Swift sources need to keep doing.
func TestWidgetsCompile(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs Xcode")
	}
	if err := exec.Command("xcrun", "--find", "swiftc").Run(); err != nil {
		t.Skip("needs Xcode's swiftc")
	}
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("HOME", t.TempDir()) // os.UserCacheDir reads it on macOS
	p := &project{ID: "dev.example.app", Widgets: []widgetKind{{Kind: "counter"}}, LiveActivities: liveActivities{Enabled: true}}
	for _, c := range []struct {
		target     widgetTarget
		activities bool
		symbols    []string
	}{
		{macWidgetTarget(runtime.GOARCH), false, []string{"_nsgo_widgets_reload", "_nsgo_activity_listen"}},
		{iosWidgetTarget(true), false, []string{"_nsgo_widgets_reload", "_nsgo_activity_listen"}},
		{iosWidgetTarget(true), true, []string{"_nsgo_widgets_reload", "_nsgo_activity_start", "_nsgo_activity_listen"}},
	} {
		p.LiveActivities.Enabled = c.activities
		w, err := compileWidgets(t.Context(), p, c.target)
		if err != nil {
			t.Fatal(err)
		}
		files := []string{w.exe, w.lib, filepath.Join(w.exeIntents, "extract.actionsdata"), filepath.Join(w.libIntents, "extract.actionsdata")}
		for _, f := range files {
			if _, err := os.Stat(f); err != nil {
				t.Error(err)
			}
		}
		out, err := exec.Command("nm", "-gU", w.lib).Output()
		for _, sym := range c.symbols {
			if err != nil || !strings.Contains(string(out), sym) {
				t.Errorf("the %s library exports no %s: %v", c.target.sdk, sym, err)
			}
		}
		for _, f := range []string{w.exeIntents, w.libIntents} {
			data, err := os.ReadFile(filepath.Join(f, "extract.actionsdata"))
			if err != nil || !strings.Contains(string(data), `"NSGoActionIntent"`) {
				t.Errorf("%s lacks NSGoActionIntent: %v", f, err)
			}
			if !c.target.ios && f == w.libIntents && !strings.Contains(string(data), "com.apple.link.systemProtocol.ForegroundContinuable") {
				t.Error("macOS widget actions have no app-process protocol")
			}
		}
		if !c.target.ios {
			continue
		}
		// iOS 15 runs the widgets, without ActivityKit and App Intents.
		out, _ = exec.Command("otool", "-l", w.exe).Output()
		if !strings.Contains(string(out), "LC_LOAD_WEAK_DYLIB") {
			t.Error("the extension links ActivityKit and App Intents strongly")
		}
	}
}

// TestAndroidOverlayDeclarations checks that only an app listing SYSTEM_ALERT_WINDOW asks for the overlay's foreground service, in its APK's manifest and in its library's.
func TestAndroidOverlayDeclarations(t *testing.T) {
	p := &project{ID: "dev.example.app"}
	p.Android.Permissions = []string{"android.permission.CAMERA"}
	if p.androidOverlay() || slices.ContainsFunc(p.androidPermissions(), func(s string) bool { return strings.Contains(s, "FOREGROUND_SERVICE") }) {
		t.Errorf("an app without overlays asks for %v", p.androidPermissions())
	}
	if m := aarManifest(false, nil); strings.Contains(m, "FOREGROUND_SERVICE") || strings.Contains(m, "OverlayService") || strings.Contains(m, "SYSTEM_ALERT_WINDOW") {
		t.Errorf("a library without overlays declares them:\n%s", m)
	}
	p.Android.Permissions = append(p.Android.Permissions, "android.permission.SYSTEM_ALERT_WINDOW")
	want := []string{"android.permission.CAMERA", "android.permission.SYSTEM_ALERT_WINDOW", "android.permission.FOREGROUND_SERVICE", "android.permission.FOREGROUND_SERVICE_SPECIAL_USE"}
	if got := p.androidPermissions(); !p.androidOverlay() || !slices.Equal(got, want) {
		t.Errorf("an app with overlays asks for %v, want %v", got, want)
	}
	m := aarManifest(true, nil)
	if err := xml.Unmarshal([]byte(m), new(struct{})); err != nil {
		t.Fatalf("the library's manifest is no XML: %v\n%s", err, m)
	}
	for _, s := range []string{"SYSTEM_ALERT_WINDOW", "FOREGROUND_SERVICE_SPECIAL_USE", `android:name="dev.oneo.nsgo.OverlayService"`, `android:foregroundServiceType="specialUse"`} {
		if !strings.Contains(m, s) {
			t.Errorf("a library with overlays lacks %s:\n%s", s, m)
		}
	}
}

func TestAndroidWidgetLocalizations(t *testing.T) {
	dir := t.TempDir()
	locales := filepath.Join(dir, "locales")
	writeFiles(t, locales, map[string]string{"en.yaml": "widgetName: Counter\nwidgetDescription: The current count\nplain: Translated kind\n", "zh.yaml": "widgetName: 计数\n"})
	p := &project{Dir: dir, Widgets: []widgetKind{{Kind: "counter", Name: "widgetName", Description: "widgetDescription"}, {Kind: "plain"}}}
	p.I18n.Dir, p.I18n.Source = locales, "en"
	widgets, err := p.androidWidgets()
	if err != nil {
		t.Fatal(err)
	}
	if widgets[0].Names[""] != "Counter" || widgets[0].Names["zh"] != "计数" || widgets[0].Descriptions["zh"] != "The current count" {
		t.Fatalf("localizations %+v", widgets[0])
	}
	if widgets[1].Names[""] != "plain" || len(widgets[1].Names) != 1 {
		t.Fatalf("empty name translated its kind: %+v", widgets[1])
	}
	if !slices.Equal(widgets[1].Families, []string{"small", "medium", "large"}) {
		t.Fatalf("families %v", widgets[1].Families)
	}
}
