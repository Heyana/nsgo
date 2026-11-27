package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"

	"go.yaml.in/yaml/v3"
)

func TestAppleInfoArrays(t *testing.T) {
	var p project
	if err := yaml.Unmarshal([]byte("name: PiP\nid: dev.example.pip\napple:\n  info:\n    UIBackgroundModes: [audio]\n    NSMicrophoneUsageDescription: 'A & B'\n    ExampleBoolean: true\n"), &p); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{"ios", "macos", "tvos", "watchos"} {
		t.Run(target, func(t *testing.T) {
			templates := map[string]*template.Template{"ios": iosPlist, "macos": macPlist, "tvos": tvosPlist, "watchos": watchosPlist}
			path := filepath.Join(t.TempDir(), "Info.plist")
			if err := writePlist(path, &p, templates[target], ""); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, want := range []string{"<key>UIBackgroundModes</key><array><string>audio</string></array>", "<string>A &amp; B</string>", "<key>ExampleBoolean</key><true/>"} {
				if !strings.Contains(string(b), want) {
					t.Fatalf("missing %s in %s", want, b)
				}
			}
		})
	}
	p.Apple.Info["Bad"] = make(chan int)
	if err := writePlist(filepath.Join(t.TempDir(), "Info.plist"), &p, iosPlist, ""); err == nil {
		t.Fatal("unsupported plist value was stringified")
	}
}

func TestSensorAppleInfoDictionaryAndLocationEntitlement(t *testing.T) {
	var p project
	if err := yaml.Unmarshal([]byte("apple:\n  info:\n    NSLocationWhenInUseUsageDescription: Nearby places\n    UIBackgroundModes: [location]\n    NSLocationTemporaryUsageDescriptionDictionary:\n      route: A precise route\n"), &p); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Info.plist")
	if err := writePlist(path, &p, iosPlist, ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "<key>NSLocationTemporaryUsageDescriptionDictionary</key><dict><key>route</key><string>A precise route</string></dict>") {
		t.Fatalf("dictionary missing: %s", b)
	}
	if p.hardenedEntitlements()["com.apple.security.personal-information.location"] != true {
		t.Fatal("location usage omitted hardened runtime entitlement")
	}
	if err := yaml.Unmarshal([]byte("apple:\n  info:\n    NSContactsUsageDescription: Find friends\n    NSPhotoLibraryUsageDescription: Pick photos\n    NSAppleEventsUsageDescription: Automate Finder\n"), &p); err != nil {
		t.Fatal(err)
	}
	ents := p.hardenedEntitlements()
	for _, key := range []string{
		"com.apple.security.personal-information.addressbook",
		"com.apple.security.personal-information.photos-library",
		"com.apple.security.automation.apple-events",
	} {
		if ents[key] != true {
			t.Errorf("missing hardened entitlement %s", key)
		}
	}
	p.Apple.Info = nil
	if len(p.hardenedEntitlements()) != 0 {
		t.Fatal("empty build claims an entitlement")
	}
}

// iosPlistFor writes the iOS Info.plist of the nsgo.yaml in src, as load validates it.
func iosPlistFor(t *testing.T, src string) string {
	t.Helper()
	var p project
	if err := yaml.Unmarshal([]byte(src), &p); err != nil {
		t.Fatal(err)
	}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Info.plist")
	if err := writePlist(path, &p, iosPlist, ""); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

const everyIPadOrientation = "<key>UISupportedInterfaceOrientations~ipad</key><array><string>UIInterfaceOrientationPortrait</string><string>UIInterfaceOrientationPortraitUpsideDown</string><string>UIInterfaceOrientationLandscapeLeft</string><string>UIInterfaceOrientationLandscapeRight</string></array>"

func TestIOSPlistKeepsFullScreenByDefault(t *testing.T) {
	plist := iosPlistFor(t, "name: Sheet\n")
	if !strings.Contains(plist, "<key>UIRequiresFullScreen</key><true/>") {
		t.Errorf("default plist is not full screen:\n%s", plist)
	}
	if strings.Contains(plist, "UISupportedInterfaceOrientations") {
		t.Errorf("default plist declares orientations:\n%s", plist)
	}
}

func TestIOSMultitaskingDeclaresEveryIPadOrientation(t *testing.T) {
	for _, src := range []string{
		"name: Sheet\napple:\n  requires_full_screen: false\n",
		"name: Sheet\napple:\n  requires_full_screen: false\n  orientations: [landscape_right, portrait, landscape_left, portrait_upside_down]\n",
	} {
		plist := iosPlistFor(t, src)
		for _, want := range []string{"<key>UIRequiresFullScreen</key><false/>", everyIPadOrientation} {
			if !strings.Contains(plist, want) {
				t.Errorf("%q: plist lacks %q:\n%s", src, want, plist)
			}
		}
	}
}

func TestIOSMultipleScenes(t *testing.T) {
	for _, test := range []struct {
		src, want string
	}{
		{"name: Sheet\n", "<key>UIApplicationSupportsMultipleScenes</key><false/>"},
		{"name: Sheet\napple:\n  multiple_scenes: true\n", "<key>UIApplicationSupportsMultipleScenes</key><true/>"},
	} {
		plist := iosPlistFor(t, test.src)
		if !strings.Contains(plist, test.want) {
			t.Errorf("%q: plist lacks %q:\n%s", test.src, test.want, plist)
		}
	}
}

func TestIOSFullScreenIPadOrientations(t *testing.T) {
	plist := iosPlistFor(t, "name: Sheet\napple:\n  orientations: [landscape_left, portrait]\n")
	for _, want := range []string{
		"<key>UIRequiresFullScreen</key><true/>",
		"<key>UISupportedInterfaceOrientations~ipad</key><array><string>UIInterfaceOrientationPortrait</string><string>UIInterfaceOrientationLandscapeLeft</string></array>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("plist lacks %q:\n%s", want, plist)
		}
	}
}

func TestIPadMultitaskingRejections(t *testing.T) {
	for src, want := range map[string]string{
		"apple:\n  orientations: [diagonal]\n":                                                           `apple.orientations has "diagonal"`,
		"apple:\n  orientations: [portrait, portrait]\n":                                                 `apple.orientations repeats "portrait"`,
		"apple:\n  requires_full_screen: false\n  orientations: [portrait, landscape_left]\n":            "must list all four",
		"apple:\n  info:\n    UIRequiresFullScreen: false\n":                                             "set apple.requires_full_screen",
		"apple:\n  info:\n    UISupportedInterfaceOrientations~ipad: [UIInterfaceOrientationPortrait]\n": "set apple.orientations",
		"apple:\n  info:\n    UIApplicationSupportsMultipleScenes: true\n":                               "set apple.multiple_scenes",
	} {
		var p project
		if err := yaml.Unmarshal([]byte(src), &p); err != nil {
			t.Fatal(err)
		}
		p.Name = "Sheet"
		if err := p.Validate(); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: error = %v, want %q", src, err, want)
		}
	}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: Sheet\napple:\n  orientations: [diagonal]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := load(dir); err == nil || !strings.Contains(err.Error(), "apple.orientations") {
		t.Errorf("load error = %v, want the orientation's", err)
	}
}
