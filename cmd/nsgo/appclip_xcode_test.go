package main

import (
	"bytes"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestIOSAppClipSigningProject(t *testing.T) {
	dir := t.TempDir()
	p := &project{Name: "Parent", ID: "dev.example.parent", Dir: dir,
		AppClip: &appClip{Name: "Quick Clip", ID: "dev.example.parent.Clip", Main: "clip", Minimum: "17.0", Domains: []string{"example.com"}},
		Widgets: []widgetKind{{Kind: "counter"}}}
	unsigned := filepath.Join(dir, "unsigned.app")
	for path, value := range map[string]map[string]any{
		"Info.plist":                         {"CFBundleIdentifier": p.ID},
		"AppClips/Quick Clip.app/Info.plist": {"CFBundleIdentifier": p.AppClip.ID, "NSAppClip": map[string]any{}},
		"PlugIns/Widgets.appex/Info.plist":   {"CFBundleIdentifier": p.ID + ".widgets"},
	} {
		file := filepath.Join(unsigned, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(file, encodePlist(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	work := filepath.Join(dir, "sign")
	project, err := writeIOSSigningProject(p, iOS, unsigned, work, "TEAM123")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(project, "project.pbxproj"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`name = "Embed App Clips";`, `dstPath = "$(CONTENTS_FOLDER_PATH)/AppClips";`,
		`productType = "com.apple.product-type.application.on-demand-install-capable";`,
		`PRODUCT_BUNDLE_IDENTIFIER = "dev.example.parent.Clip";`, `PRODUCT_NAME = "Quick Clip";`,
		`IPHONEOS_DEPLOYMENT_TARGET = 17.0;`, `CODE_SIGN_ENTITLEMENTS = Clip.entitlements;`,
		`AA0000000000000000000029 /* PBXTargetDependency */,`,
		`AA0000000000000000000019 /* PBXTargetDependency */,`,
		`productType = "com.apple.product-type.app-extension";`,
	} {
		if !strings.Contains(string(data), want) {
			t.Errorf("signing project lost %s", want)
		}
	}
	read := func(name string) map[string]any {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(work, name))
		if err != nil {
			t.Fatal(err)
		}
		value, err := decodePlist(data)
		if err != nil {
			t.Fatal(err)
		}
		return value.(map[string]any)
	}
	appEnts, clipEnts := read("App.entitlements"), read("Clip.entitlements")
	if got := appEnts["com.apple.developer.associated-appclip-app-identifiers"].([]any); len(got) != 1 || got[0] != "$(AppIdentifierPrefix)dev.example.parent.Clip" {
		t.Errorf("parent's clip association = %v", got)
	}
	if _, ok := appEnts["com.apple.security.application-groups"]; !ok {
		t.Error("parent lost the widgets' group")
	}
	if clipEnts["com.apple.developer.on-demand-install-capable"] != true || clipEnts["com.apple.developer.parent-application-identifiers"].([]any)[0] != "$(AppIdentifierPrefix)dev.example.parent" {
		t.Errorf("clip entitlements = %v", clipEnts)
	}
	if got := clipEnts["com.apple.developer.associated-domains"].([]any); len(got) != 1 || got[0] != "appclips:example.com" {
		t.Errorf("clip domains = %v", got)
	}
	if read("Clip-Info.plist")["CFBundleIdentifier"] != p.AppClip.ID {
		t.Error("clip target processes the parent's Info.plist")
	}
	plain := *p
	plain.AppClip = nil
	for name, value := range map[string]map[string]string{
		"plain": iosProjectData(&plain, iOS, "TEAM123"),
		"tv":    iosProjectData(p, tvOS, "TEAM123"),
		"clip":  iosProjectData(p.clipProject(), iOS, "TEAM123"),
	} {
		if value["Clip"] != "" {
			t.Errorf("%s recursively embeds an App Clip", name)
		}
	}
}

// TestIOSAppClipXcodeBuild builds real Go iOS products and passes them through
// Xcode's generated target graph with signing disabled. Provisioning and device
// installation require separate account-backed acceptance.
func TestIOSAppClipXcodeBuild(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("needs Xcode")
	}
	if err := exec.Command("xcrun", "--sdk", "iphoneos", "--show-sdk-path").Run(); err != nil {
		t.Skip("needs the iPhoneOS SDK")
	}
	for _, name := range []string{"Quick Clip", "Parent Fixture"} {
		t.Run(name, func(t *testing.T) { testIOSAppClipXcodeBuild(t, name) })
	}
}

func testIOSAppClipXcodeBuild(t *testing.T, clipName string) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":                  "module example.com/appclipxcode\n\ngo 1.27.3\n",
		"main.go":                 "package main\nfunc main() { println(\"parent\") }\n",
		"clip/main.go":            "package main\nfunc main() { println(\"clip\") }\n",
		"resources/parent.txt":    "parent resource",
		"clip/resources/clip.txt": "clip resource",
		"nsgo.yaml":               "name: Parent Fixture\nid: dev.example.appclipxcode\napp_clip:\n  name: " + clipName + "\n  main: ./clip\n",
	})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	p.Release = true
	unsigned, err := buildIOSApp(t.Context(), p, iOS, filepath.Join(dir, "unsigned"), false)
	if err != nil {
		t.Fatal(err)
	}
	work := filepath.Join(dir, "sign")
	project, err := writeIOSSigningProject(p, iOS, unsigned, work, "")
	if err != nil {
		t.Fatal(err)
	}
	products := filepath.Join(work, "products")
	if err := runXcodebuild(t.Context(), "-project", project, "-target", "Nsgo", "-configuration", "Release", "-sdk", "iphoneos",
		"CODE_SIGNING_ALLOWED=NO", "NSGO_SIGNED_PRODUCTS_DIR="+products, "SYMROOT="+filepath.Join(work, "build"), "OBJROOT="+filepath.Join(work, "obj"), "build"); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(products, p.Name+".app")
	text := func(path string) []byte {
		t.Helper()
		file, err := macho.Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		section := file.Section("__text")
		if section == nil {
			t.Fatalf("%s has no executable text", path)
		}
		data, err := section.Data()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	for from, to := range map[string]string{
		filepath.Join(unsigned, p.Name):                        filepath.Join(output, p.Name),
		filepath.Join(iosClipApp(p, unsigned), p.AppClip.Name): filepath.Join(iosClipApp(p, output), p.AppClip.Name),
	} {
		// Xcode may strip the copied clip's symbols in Release, but its
		// executable code must remain the Go product's.
		if !bytes.Equal(text(to), text(from)) {
			t.Fatalf("Xcode product %s changed Go's executable code", to)
		}
	}
	clip := iosClipApp(p, output)
	if got, err := os.ReadFile(filepath.Join(clip, "clip.txt")); err != nil || string(got) != "clip resource" {
		t.Fatalf("embedded clip lost its resources: %q, %v", got, err)
	}
	if _, err := os.Stat(filepath.Join(clip, "parent.txt")); !os.IsNotExist(err) {
		t.Fatal("embedded clip contains the parent's resources")
	}
}
