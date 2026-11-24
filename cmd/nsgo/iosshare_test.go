package main

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestIOSShareTypes(t *testing.T) {
	for _, tc := range []struct {
		types string
		valid bool
	}{
		{"[]", true}, {"[public.url, public.plain-text, com.adobe.pdf]", true},
		{"[public.url, public.url]", false}, {"[text/plain]", false}, {"[\"public.url' OR TRUEPREDICATE\"]", false},
	} {
		t.Run(tc.types, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Shares\napple:\n  share_types: " + tc.types + "\n"})
			_, err := load(dir)
			if (err == nil) != tc.valid {
				t.Fatalf("load: %v", err)
			}
		})
	}
}

func TestIOSShareMetadataAndAppGroup(t *testing.T) {
	p := &project{ID: "dev.example.app", Name: "Share App", Version: "1.2.3"}
	p.Apple.ShareTypes = []string{"public.url", "public.data"}
	info := shareInfo(p, map[string]any{"CFBundleSupportedPlatforms": []any{"iPhoneSimulator"}})
	decoded, err := decodePlist(encodePlist(info))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.(map[string]any)["CFBundleIdentifier"] != p.ID+".share" {
		t.Fatal("invalid extension Info.plist")
	}
	if info["CFBundleIdentifier"] != "dev.example.app.share" || info[shareGroupKey] != "group.dev.example.app" {
		t.Fatalf("info: %#v", info)
	}
	ext := info["NSExtension"].(map[string]any)
	if ext["NSExtensionPrincipalClass"] != "NSGoShareController" {
		t.Fatal(ext)
	}
	rule := ext["NSExtensionAttributes"].(map[string]any)["NSExtensionActivationRule"].(string)
	for _, part := range []string{"UTI-CONFORMS-TO 'public.url'", "UTI-CONFORMS-TO 'public.data'", "== $item.attachments.@count"} {
		if !strings.Contains(rule, part) {
			t.Fatalf("rule lacks %s: %s", part, rule)
		}
	}
	if p.simulatorEntitlements() == nil || !strings.Contains(p.sharePlist(), "group.dev.example.app") {
		t.Fatal("containing app lacks app group")
	}
	p.AppClip = &appClip{Name: "Clip", ID: "dev.example.app.Clip", Main: "."}
	if p.clipProject().iosShares() {
		t.Fatal("App Clip contains a share extension")
	}
}

func TestIOSShareRequiresDedicatedProvisioningProfile(t *testing.T) {
	p := &project{ID: "dev.example.app"}
	app := t.TempDir()
	if err := os.MkdirAll(iosSharePath(app), 0o755); err != nil {
		t.Fatal(err)
	}
	err := signIOSShare(context.Background(), p, app, "identity")
	if err == nil || !strings.Contains(err.Error(), "sign.share_profile") {
		t.Fatalf("sign: %v", err)
	}
	if _, err := os.Stat(filepath.Join(iosSharePath(app), "embedded.mobileprovision")); !os.IsNotExist(err) {
		t.Fatal("signed without an extension profile")
	}
}

func TestIOSShareAndWidgetsHaveIndependentSigningTargets(t *testing.T) {
	p := &project{ID: "dev.example.app", Name: "App", Widgets: []widgetKind{{Kind: "clock"}}}
	p.Apple.ShareTypes = []string{"public.url"}
	var project strings.Builder
	if err := iosProject.Execute(&project, iosProjectData(p, iOS, "TEAM")); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`PRODUCT_BUNDLE_IDENTIFIER = "dev.example.app.share"`,
		`PRODUCT_BUNDLE_IDENTIFIER = "dev.example.app.widgets"`,
		`AB0000000000000000000019 /* PBXTargetDependency */`,
		`AA0000000000000000000019 /* PBXTargetDependency */`,
		`CODE_SIGN_ENTITLEMENTS = NSGoShare.entitlements`,
		`unsigned/PlugIns/NSGoShare.appex`,
	} {
		if !strings.Contains(project.String(), want) {
			t.Errorf("project lacks %s", want)
		}
	}
	if runtime.GOOS == "darwin" {
		dir := filepath.Join(t.TempDir(), "Shares.xcodeproj")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "project.pbxproj"), []byte(project.String()), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := run(t.Context(), "xcodebuild", "-list", "-project", dir); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(iosSigningExtensions(p, tvOS)); got != 0 {
		t.Errorf("tvOS has %d iOS extensions", got)
	}
}

// With widgets, a share extension and an App Clip together, every object the signing project refers to is defined once, and the project-level products directory the Clip's build directory names stays set.
func TestIOSSigningProjectDefinesEveryReferencedObject(t *testing.T) {
	p := &project{ID: "dev.example.app", Name: "App", Widgets: []widgetKind{{Kind: "clock"}}}
	p.Apple.ShareTypes = []string{"public.url"}
	p.AppClip = &appClip{Name: "Quick Clip", ID: "dev.example.app.Clip", Main: "clip", Minimum: "17.0", Domains: []string{"example.com"}}
	var b strings.Builder
	if err := iosProject.Execute(&b, iosProjectData(p, iOS, "TEAM")); err != nil {
		t.Fatal(err)
	}
	text := b.String()
	defined := map[string]int{}
	for _, m := range regexp.MustCompile(`(?m)^\t\t([0-9A-F]{24}) (?:/\*[^*]*\*/ )?= \{`).FindAllStringSubmatch(text, -1) {
		defined[m[1]]++
	}
	for id, n := range defined {
		if n != 1 {
			t.Errorf("%s is defined %d times", id, n)
		}
	}
	for _, id := range regexp.MustCompile(`\b[0-9A-F]{24}\b`).FindAllString(text, -1) {
		if defined[id] == 0 {
			t.Errorf("%s is referenced but never defined", id)
		}
	}
	for _, id := range []string{"AA0000000000000000000025", "AA0000000000000000000013", "AB0000000000000000000013", "AA0000000000000000000023"} {
		if defined[id] == 0 {
			t.Errorf("%s is missing", id)
		}
	}
	if n := strings.Count(text, `NSGO_SIGNED_PRODUCTS_DIR = "$(SYMROOT)/products";`); n != 2 {
		t.Errorf("the project's Debug and Release set NSGO_SIGNED_PRODUCTS_DIR %d times, want 2", n)
	}
}

func TestIOSShareSourcesRemainAvailableWithVendoredTools(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module example.com/sharefixture\n\ngo 1.27.3\n\nrequire github.com/nsgo-dev/nsgo v0.0.0\nreplace github.com/nsgo-dev/nsgo => " + filepath.ToSlash(root) + "\ntool github.com/nsgo-dev/nsgo/cmd/nsgo\n",
		"main.go": "package main\nfunc main() {}\n",
	})
	if err := goTool(t.Context(), dir, nil, "mod", "tidy"); err != nil {
		t.Fatal(err)
	}
	if err := goTool(t.Context(), dir, nil, "mod", "vendor"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cmd/nsgo/shareextension/main.go", "internal/platform/shareinbox/inbox.go", "internal/platform/shareinbox/darwin.go", "internal/platform/objcblock/block_darwin.go"} {
		if _, err := os.Stat(filepath.Join(dir, "vendor", nsgoModule, name)); err != nil {
			t.Error(err)
		}
	}
	if runtime.GOOS == "darwin" {
		env, err := iosEnv(t.Context(), iOS, true)
		if err != nil {
			t.Fatal(err)
		}
		p := &project{Dir: dir, ID: "dev.nsgo.test.vendoredshare", Name: "VendoredShare", Version: "1.0.0"}
		p.Apple.ShareTypes = []string{"public.url", "public.data"}
		app := filepath.Join(t.TempDir(), "VendoredShare.app")
		if err := addIOSShare(t.Context(), p, app, env, true); err != nil {
			t.Fatal(err)
		}
		if err := run(t.Context(), "codesign", "--verify", "--strict", iosSharePath(app)); err != nil {
			t.Fatal(err)
		}
	}
}

func TestIOSShareProfileMustGrantItsBundleAndGroup(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		groups   []any
		valid    bool
	}{
		{"matching", "TEAM.dev.example.app.share", []any{"group.dev.example.app", "group.unrelated"}, true},
		{"wrong bundle", "TEAM.dev.example.other.share", []any{"group.dev.example.app"}, false},
		{"missing group", "TEAM.dev.example.app.share", []any{"group.unrelated"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := t.TempDir()
			if err := os.MkdirAll(iosSharePath(app), 0o755); err != nil {
				t.Fatal(err)
			}
			p := &project{ID: "dev.example.app"}
			p.Sign.ShareProfile = filepath.Join(t.TempDir(), "Share.mobileprovision")
			if err := os.WriteFile(p.Sign.ShareProfile, []byte("fixture profile"), 0o600); err != nil {
				t.Fatal(err)
			}
			signed := false
			ctx := withProcessRunner(t, func(_ context.Context, c processCommand) ([]byte, error) {
				switch c.name {
				case "security":
					return encodePlist(map[string]any{"Entitlements": map[string]any{"application-identifier": tc.id, "com.apple.security.application-groups": tc.groups}}), nil
				case "codesign":
					signed = true
					if !tc.valid {
						t.Fatal("invalid extension profile was signed")
					}
					at := slices.Index(c.args, "--entitlements")
					if at < 0 || at+1 >= len(c.args) {
						t.Fatal("missing signing entitlements")
					}
					data, err := os.ReadFile(c.args[at+1])
					if err != nil {
						t.Fatal(err)
					}
					decoded, err := decodePlist(data)
					if err != nil {
						t.Fatal(err)
					}
					ents := decoded.(map[string]any)
					groups, _ := ents["com.apple.security.application-groups"].([]any)
					if ents["application-identifier"] != "TEAM.dev.example.app.share" || len(groups) != 1 || groups[0] != "group.dev.example.app" {
						t.Fatalf("signing entitlements: %#v", ents)
					}
					return nil, nil
				default:
					t.Fatalf("unexpected command: %s", c.name)
					return nil, nil
				}
			})
			err := signIOSShare(ctx, p, app, "identity")
			if (err == nil) != tc.valid || signed != tc.valid {
				t.Fatalf("sign = %v, signed %v", err, signed)
			}
		})
	}
}
