package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestPrivacyManifestIncludesNSGoAndAppDeclarations(t *testing.T) {
	tracking := true
	p := &project{}
	p.Apple.Privacy.AccessedAPIs = map[string][]string{"NSPrivacyAccessedAPICategoryDiskSpace": {"E174.1"}, "NSPrivacyAccessedAPICategoryUserDefaults": {"CA92.1", "1C8F.1"}}
	p.Apple.Privacy.Tracking = &tracking
	p.Apple.Privacy.TrackingDomains = []string{"example.com"}
	p.Apple.Privacy.CollectedData = []map[string]any{{"NSPrivacyCollectedDataType": "NSPrivacyCollectedDataTypeEmailAddress", "NSPrivacyCollectedDataTypeLinked": true, "NSPrivacyCollectedDataTypePurposes": []any{"NSPrivacyCollectedDataTypePurposeAppFunctionality"}}}
	got, err := decodePlist(p.privacyManifest(nil))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"NSPrivacyAccessedAPITypes": []any{
			map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategoryDiskSpace", "NSPrivacyAccessedAPITypeReasons": []any{"E174.1"}},
			map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategoryFileTimestamp", "NSPrivacyAccessedAPITypeReasons": []any{"C617.1"}},
			map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategorySystemBootTime", "NSPrivacyAccessedAPITypeReasons": []any{"35F9.1"}},
			map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategoryUserDefaults", "NSPrivacyAccessedAPITypeReasons": []any{"1C8F.1", "CA92.1"}},
		},
		"NSPrivacyCollectedDataTypes": []any{map[string]any{"NSPrivacyCollectedDataType": "NSPrivacyCollectedDataTypeEmailAddress", "NSPrivacyCollectedDataTypeLinked": true, "NSPrivacyCollectedDataTypePurposes": []any{"NSPrivacyCollectedDataTypePurposeAppFunctionality"}}},
		"NSPrivacyTracking":           true,
		"NSPrivacyTrackingDomains":    []any{"example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("manifest\n%#v\nwant\n%#v", got, want)
	}
	if a, b := p.privacyManifest(nil), p.privacyManifest(nil); !bytes.Equal(a, b) {
		t.Fatal("two manifests of one project differ")
	}
}

// An app that puts a PrivacyInfo.xcprivacy in its resources keeps its declarations, NSGo's and apple.privacy's merged in, and writing it again changes nothing.
func TestPrivacyManifestMergesTheAppsOwn(t *testing.T) {
	dir := t.TempDir()
	own := encodePlist(map[string]any{
		"NSPrivacyAccessedAPITypes": []any{
			map[string]any{"NSPrivacyAccessedAPIType": "NSPrivacyAccessedAPICategoryFileTimestamp", "NSPrivacyAccessedAPITypeReasons": []any{"3B52.1"}},
		},
		"NSPrivacyTracking":        true,
		"NSPrivacyTrackingDomains": []any{"ads.example.com"},
	})
	path := filepath.Join(dir, privacyManifestName)
	if err := os.WriteFile(path, own, 0o644); err != nil {
		t.Fatal(err)
	}
	p := &project{}
	p.Apple.Privacy.TrackingDomains = []string{"ads.example.com", "metrics.example.com"}
	if err := p.writePrivacyManifest(dir); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)
	if err := p.writePrivacyManifest(dir); err != nil {
		t.Fatal(err)
	}
	second, _ := os.ReadFile(path)
	if !bytes.Equal(first, second) {
		t.Fatalf("writing again changed the manifest:\n%s\n%s", first, second)
	}
	got, err := decodePlist(first)
	if err != nil {
		t.Fatal(err)
	}
	m := got.(map[string]any)
	if m["NSPrivacyTracking"] != true || !reflect.DeepEqual(m["NSPrivacyTrackingDomains"], []any{"ads.example.com", "metrics.example.com"}) {
		t.Fatalf("tracking %v %v", m["NSPrivacyTracking"], m["NSPrivacyTrackingDomains"])
	}
	types := m["NSPrivacyAccessedAPITypes"].([]any)
	timestamps := types[0].(map[string]any)
	if timestamps["NSPrivacyAccessedAPIType"] != "NSPrivacyAccessedAPICategoryFileTimestamp" || !reflect.DeepEqual(timestamps["NSPrivacyAccessedAPITypeReasons"], []any{"3B52.1", "C617.1"}) {
		t.Fatalf("file timestamps %v", timestamps)
	}
	if len(types) != 3 {
		t.Fatalf("accessed APIs %v", types)
	}
}

func TestPrivacyValidation(t *testing.T) {
	for _, c := range []struct {
		apis map[string][]string
		data []map[string]any
		err  string
	}{
		{apis: map[string][]string{"NSPrivacyAccessedAPICategoryDiskSpace": {"E174.1"}}},
		{apis: map[string][]string{"DiskSpace": {"E174.1"}}, err: "not an NSPrivacyAccessedAPICategory"},
		{apis: map[string][]string{"NSPrivacyAccessedAPICategoryDiskSpace": nil}, err: "needs its reasons"},
		{data: []map[string]any{{"NSPrivacyCollectedDataTypeLinked": false}}, err: "no NSPrivacyCollectedDataType"},
	} {
		p := &project{}
		p.Apple.Privacy.AccessedAPIs, p.Apple.Privacy.CollectedData = c.apis, c.data
		err := p.validatePrivacy()
		if c.err == "" && err != nil || c.err != "" && (err == nil || !strings.Contains(err.Error(), c.err)) {
			t.Errorf("%v %v: %v, want %q", c.apis, c.data, err, c.err)
		}
	}
}

func TestDiskSpaceLinkerDeclaration(t *testing.T) {
	p := &project{}
	macos := []string{"GOOS=darwin", "GOARCH=arm64"}
	if got := p.privacyLdflags(macos); len(got) != 0 {
		t.Fatalf("macOS with no reason: %q", got)
	}
	p.Apple.Privacy.AccessedAPIs = map[string][]string{diskSpaceCategory: {"85F4.1"}}
	for _, goos := range []string{"darwin", "ios"} {
		if got := p.privacyLdflags([]string{"GOOS=" + goos, "GOARCH=arm64"}); len(got) != 1 || !strings.HasSuffix(got[0], ".diskSpaceDeclared=1") {
			t.Fatalf("%s linker declaration: %q", goos, got)
		}
	}
	if got := p.privacyLdflags([]string{"GOOS=linux", "GOARCH=amd64"}); len(got) != 0 {
		t.Fatalf("Linux with a reason: %q", got)
	}
}
