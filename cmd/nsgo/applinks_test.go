package main

import (
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestAppLinksConfig(t *testing.T) {
	for _, tc := range []struct{ hosts, wantError string }{
		{"[EXAMPLE.com, shop.example.com]", ""},
		{"[example.com, EXAMPLE.COM]", "listed twice"},
		{"['https://example.com']", "explicit DNS host"},
		{"['example.com/path']", "explicit DNS host"},
		{"['example.com:443']", "explicit DNS host"},
		{"['*.example.com']", "explicit DNS host"},
		{"['127.0.0.1']", "explicit DNS host"},
		{"['localhost']", "explicit DNS host"},
		{"['" + strings.Repeat("x", 64) + ".example.com']", "longer than 63"},
	} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte("name: App\nid: dev.example.app\napp_links: "+tc.hosts+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if tc.wantError != "" {
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("%s: %v", tc.hosts, err)
			}
			continue
		}
		if err != nil || !slices.Equal(p.AppLinks, []string{"example.com", "shop.example.com"}) {
			t.Fatalf("normalized hosts=%v, error=%v", p, err)
		}
	}
}

func TestAppLinksAppleEntitlements(t *testing.T) {
	p := &project{Config: projectcfg.Config{ID: "dev.example.app", AppLinks: []string{"example.com", "shop.example.com"}, Widgets: []widgetKind{{Kind: "counter"}}}}
	want := []any{"applinks:example.com", "applinks:shop.example.com"}
	ents := p.xcodeEntitlements(true, false)
	if got, _ := ents[associatedDomainsEntitlement].([]any); !slices.Equal(got, want) {
		t.Fatalf("Xcode Associated Domains=%v", got)
	}
	if groups, _ := ents["com.apple.security.application-groups"].([]any); !slices.Equal(groups, []any{p.appGroup()}) {
		t.Fatalf("Xcode lost widget groups=%v", groups)
	}
	for _, grant := range [][]any{nil, {"*"}, {"applinks:example.com"}, want} {
		profile := map[string]any{associatedDomainsEntitlement: grant}
		claims := map[string]any{}
		err := p.claimAppLinks(claims, profile)
		allowed := slices.Contains(grant, any("*")) || slices.Equal(grant, want)
		if allowed {
			if got, _ := claims[associatedDomainsEntitlement].([]any); err != nil || !slices.Equal(got, want) {
				t.Fatalf("claims=%v error=%v", claims, err)
			}
		} else if err == nil {
			t.Fatalf("insufficient profile grant accepted: %v", grant)
		}
	}
	// Store signing reads and narrows the same map; capture the grant before replacing its wildcard with the declared domains.
	profile := map[string]any{associatedDomainsEntitlement: []any{"*"}}
	if err := p.claimAppLinks(profile, profile); err != nil {
		t.Fatal(err)
	}
	if got, _ := profile[associatedDomainsEntitlement].([]any); !slices.Equal(got, want) {
		t.Fatalf("profile wildcard was claimed unchanged: %v", got)
	}
	p.AppClip = &appClip{Name: "Clip", ID: p.ID + ".Clip", Main: ".", Domains: []string{"example.com"}}
	p.Apple.AppGroup = "group.dev.example.shared"
	ents = p.xcodeEntitlements(false, true)
	if groups, _ := ents["com.apple.security.application-groups"].([]any); !slices.Equal(groups, []any{p.Apple.AppGroup}) {
		t.Fatalf("Xcode lost App Clip storage group=%v", groups)
	}
	if got, _ := ents[associatedDomainsEntitlement].([]any); !slices.Equal(got, want) {
		t.Fatalf("App Clip group replaced full-app Associated Domains=%v", got)
	}
	if got, _ := ents["com.apple.developer.associated-appclip-app-identifiers"].([]any); !slices.Equal(got, []any{"$(AppIdentifierPrefix)" + p.AppClip.ID}) {
		t.Fatalf("Associated Domains replaced the parent-clip tie=%v", got)
	}
	clip := p.clipProject()
	if len(clip.AppLinks) != 0 || len(clip.appLinkDomains()) != 0 {
		t.Fatal("App Clip inherited the full app's applinks claims")
	}
	if _, ok := p.simulatorEntitlements()[associatedDomainsEntitlement]; ok {
		t.Fatal("ad hoc simulator claimed a developer entitlement")
	}
}
