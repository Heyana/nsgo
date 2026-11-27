package main

import (
	"encoding/json"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestAppNames names an app in its catalogs' languages in its Apple
// bundles, its web manifest and its desktop entry.
func TestAppNames(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"locales/en.yaml":      "appName: Gallery\nhello: Hi\nNSCameraUsageDescription: Camera access\n",
		"locales/zh-Hans.yaml": "appName: 画廊\nNSCameraUsageDescription: 使用相机\n",
		"locales/pt-BR.yaml":   "appName: \"Galeria \\\"Nova\\\"\"\n",
	})
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "gallery", ID: "dev.oneo.gallery"}}
	p.I18n.Dir, p.I18n.Source = filepath.Join(dir, "locales"), "en"
	p.Apple.Info = map[string]any{
		"NSCameraUsageDescription": "Camera access",
		"UIBackgroundModes":        []string{"audio"},
	}

	res := filepath.Join(dir, "Resources")
	extra, err := writeAppleNames(p, res)
	if err != nil {
		t.Fatal(err)
	}
	if want := "<key>CFBundleDisplayName</key><string>Gallery</string>"; !strings.Contains(extra, want) {
		t.Errorf("plist entries %q lack %q", extra, want)
	}
	if want := "<key>CFBundleLocalizations</key><array><string>en</string><string>pt-BR</string><string>zh-Hans</string></array>"; !strings.Contains(extra, want) {
		t.Errorf("plist entries %q lack %q", extra, want)
	}
	for lang, want := range map[string]string{"zh-Hans": `"CFBundleDisplayName" = "画廊";`, "pt-BR": `"CFBundleName" = "Galeria \"Nova\"";`} {
		data, _ := os.ReadFile(filepath.Join(res, lang+".lproj", "InfoPlist.strings"))
		if !strings.Contains(string(data), want) {
			t.Errorf("%s.lproj: %q lacks %q", lang, data, want)
		}
	}
	data, _ := os.ReadFile(filepath.Join(res, "zh-Hans.lproj", "InfoPlist.strings"))
	if want := `"NSCameraUsageDescription" = "使用相机";`; !strings.Contains(string(data), want) {
		t.Errorf("zh-Hans.lproj: %q lacks %q", data, want)
	}
	data, _ = os.ReadFile(filepath.Join(res, "en.lproj", "InfoPlist.strings"))
	if want := `"NSCameraUsageDescription" = "Camera access";`; !strings.Contains(string(data), want) {
		t.Errorf("en.lproj: %q lacks %q", data, want)
	}
	// pt-BR does not translate the camera's reason, so it says the source's, as the catalogs' functions do, rather than the system falling back to Info.plist's.
	data, _ = os.ReadFile(filepath.Join(res, "pt-BR.lproj", "InfoPlist.strings"))
	if want := `"NSCameraUsageDescription" = "Camera access";`; !strings.Contains(string(data), want) {
		t.Errorf("pt-BR.lproj: %q lacks %q", data, want)
	}
	if strings.Contains(string(data), "UIBackgroundModes") {
		t.Errorf("pt-BR.lproj localizes an array: %q", data)
	}

	web := filepath.Join(dir, "web")
	os.MkdirAll(web, 0o755)
	if err := writeWebManifest(p, web, nil); err != nil {
		t.Fatal(err)
	}
	var m webManifest
	data, _ = os.ReadFile(filepath.Join(web, "manifest.webmanifest"))
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Name != "Gallery" || m.NameLocalized["zh-Hans"] != "画廊" || m.ShortNameLocalized["pt-BR"] != `Galeria "Nova"` {
		t.Errorf("manifest %s", data)
	}

	if got := desktopLocale("zh-Hant"); got != "zh_TW" {
		t.Errorf("desktop locale of zh-Hant: %q", got)
	}
	if got := desktopLocale("pt-BR"); got != "pt_BR" {
		t.Errorf("desktop locale of pt-BR: %q", got)
	}

	// An app named with an argument fails.
	writeFiles(t, dir, map[string]string{
		"locales/en.yaml":      "appName: \"{n} apps\"\n",
		"locales/zh-Hans.yaml": "appName: 画廊\n",
		"locales/pt-BR.yaml":   "appName: \"Galeria Nova\"\n",
	})
	if _, err := p.appNames(); err == nil || !strings.Contains(err.Error(), "takes no arguments") {
		t.Errorf("appName with an argument: %v", err)
	}
}

func TestApplePlistTakesTheProjectsEntries(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "App", ID: "dev.oneo.app"}}
	p.Apple.Info = map[string]any{"NSMicrophoneUsageDescription": "Dictate & send"}
	path := filepath.Join(t.TempDir(), "Info.plist")
	if err := writePlist(path, p, macPlist, ""); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if want := "<key>NSMicrophoneUsageDescription</key><string>Dictate &amp; send</string>"; !strings.Contains(string(data), want) {
		t.Errorf("Info.plist lacks %q:\n%s", want, data)
	}
	if strings.Contains(string(data), "NSHumanReadableCopyright") {
		t.Errorf("Info.plist without copyright holds one:\n%s", data)
	}

	// copyright is the About panel's, unless apple.info gives its own.
	p.Copyright = "© 2026 Oneo & co"
	if err := writePlist(path, p, macPlist, ""); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if want := "<key>NSHumanReadableCopyright</key><string>© 2026 Oneo &amp; co</string>"; !strings.Contains(string(data), want) {
		t.Errorf("Info.plist lacks %q:\n%s", want, data)
	}
	p.Apple.Info["NSHumanReadableCopyright"] = "Oneo"
	if err := writePlist(path, p, macPlist, ""); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(path)
	if n := strings.Count(string(data), "NSHumanReadableCopyright"); n != 1 || !strings.Contains(string(data), "<string>Oneo</string>") {
		t.Errorf("apple.info's copyright: %d entries:\n%s", n, data)
	}
}

// TestAppleInfoLocalizationWithoutAppName localizes apple.info's strings in every language of the catalogs of an app they do not name, from the source's development region.
func TestAppleInfoLocalizationWithoutAppName(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"locales/ja.yaml":      "NSCameraUsageDescription:\n  message: カメラでコードを読み取ります\n  description: The camera's permission prompt.\nNSMicrophoneUsageDescription: 声を録音します\nhello: こんにちは\n",
		"locales/zh-Hans.yaml": "NSCameraUsageDescription: 用相机扫码\nNSMicrophoneUsageDescription: 录制你的声音\n",
		"locales/de.yaml":      "hello: Hallo\n",
	})
	p := &project{Config: projectcfg.Config{Dir: dir}}
	p.I18n.Dir, p.I18n.Source = filepath.Join(dir, "locales"), "ja"
	p.Apple.Info = map[string]any{
		"NSCameraUsageDescription":     "Scans codes",
		"NSMicrophoneUsageDescription": []any{"not", "a", "string"},
		"NSFaceIDUsageDescription":     "Unlocks the app",
	}
	res := filepath.Join(dir, "Resources")
	extra, err := writeAppleNames(p, res)
	if err != nil {
		t.Fatal(err)
	}
	if want := "<key>CFBundleDevelopmentRegion</key><string>ja</string>"; !strings.Contains(extra, want) {
		t.Errorf("plist entries %q lack %q", extra, want)
	}
	if strings.Contains(extra, "CFBundleDisplayName") {
		t.Errorf("plist entries %q name an app without appName", extra)
	}
	for lang, want := range map[string]string{
		"ja":      `"NSCameraUsageDescription" = "カメラでコードを読み取ります";` + "\n",
		"zh-Hans": `"NSCameraUsageDescription" = "用相机扫码";` + "\n",
		"de":      `"NSCameraUsageDescription" = "カメラでコードを読み取ります";` + "\n",
	} {
		data, err := os.ReadFile(filepath.Join(res, lang+".lproj", "InfoPlist.strings"))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != want {
			t.Errorf("%s.lproj: %q, want %q", lang, data, want)
		}
	}

	// Without a message for any string entry, nothing is written and the development region stays the system's.
	p.Apple.Info = map[string]any{"NSFaceIDUsageDescription": "Unlocks the app"}
	res = filepath.Join(dir, "Other")
	extra, err = writeAppleNames(p, res)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(extra, "CFBundleDevelopmentRegion") {
		t.Errorf("plist entries %q set a development region without localized entries", extra)
	}
	if _, err := os.Stat(res); !os.IsNotExist(err) {
		t.Errorf("resources written without localized entries: %v", err)
	}
}

func TestAppleInfoLocalizationRejectsArguments(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"locales/en.yaml": "NSCameraUsageDescription: \"Camera for {name}\"\n",
	})
	p := &project{Config: projectcfg.Config{Dir: dir}}
	p.I18n.Dir, p.I18n.Source = filepath.Join(dir, "locales"), "en"
	p.Apple.Info = map[string]any{"NSCameraUsageDescription": "Camera access"}
	if _, err := writeAppleNames(p, filepath.Join(dir, "Resources")); err == nil || !strings.Contains(err.Error(), "takes no arguments") {
		t.Fatalf("apple.info message with an argument: %v", err)
	}
}
