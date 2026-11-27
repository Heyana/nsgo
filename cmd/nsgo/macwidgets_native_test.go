package main

import (
	"bytes"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// This compiles and signs the real WidgetKit bundle. Profile authorization and a WidgetKit host reading the timeline need a provisioned fixture.
func TestMacStoreWidgetBundleUsesSharedContainer(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs macOS and Xcode")
	}
	if err := exec.Command("xcrun", "--find", "swiftc").Run(); err != nil {
		t.Skip("needs Xcode's Swift compiler")
	}
	p := &project{Config: projectcfg.Config{ID: "dev.example.sandboxwidgets", Version: "1.0", Widgets: []widgetKind{{Kind: "counter"}}}, Release: true}
	p.Updates.Channel = channelStore
	p.Apple.AppGroup = "group.dev.example.sharedwidgets"
	app := filepath.Join(t.TempDir(), "Fixture.app")
	if err := addMacWidgets(t.Context(), p, app, runtime.GOARCH); err != nil {
		t.Fatal(err)
	}
	appex := macWidgetsAppex(app)
	data, err := os.ReadFile(filepath.Join(appex, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	v, err := decodePlist(data)
	if err != nil {
		t.Fatal(err)
	}
	info := v.(map[string]any)
	if info[widgetGroupKey] != p.appGroup() || info[widgetPathKey] != nil {
		t.Fatalf("extension timeline location=%#v", info)
	}
	data, err = exec.Command("codesign", "--display", "--entitlements", ":-", appex).CombinedOutput()
	if err != nil {
		t.Fatal(err)
	}
	start, end := bytes.Index(data, []byte("<?xml")), bytes.Index(data, []byte("</plist>"))
	if start < 0 || end < start {
		t.Fatal("codesign did not return an XML entitlement dictionary")
	}
	data = data[start : end+len("</plist>")]
	v, err = decodePlist(data)
	if err != nil {
		t.Fatal(err)
	}
	ents := v.(map[string]any)
	groups, _ := ents["com.apple.security.application-groups"].([]any)
	if !slices.Equal(groups, []any{p.appGroup()}) || ents["com.apple.security.app-sandbox"] != true {
		t.Fatalf("extension code signature=%#v", ents)
	}
	if _, ok := ents["com.apple.security.temporary-exception.files.home-relative-path.read-only"]; ok {
		t.Fatal("store extension retained the direct-build home exception")
	}
}
