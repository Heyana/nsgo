package main

import (
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCloudConfigParsingAndValidation(t *testing.T) {
	// Top-level cloud: section.
	dir := t.TempDir()
	yaml := `name: Quest
id: com.example.quest
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.quest
`
	if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := load(dir)
	if err != nil {
		t.Fatalf("load cloud config: %v", err)
	}
	cfg := p.cloudSettings()
	if !cfg.KVStore || !slices.Equal(cfg.Containers, []string{"iCloud.com.example.quest"}) {
		t.Fatalf("cloudSettings = %+v", cfg)
	}

	// Top-level icloud: and apple.icloud: aliases also parse cleanly.
	dir2 := t.TempDir()
	yaml2 := `name: Quest
id: com.example.quest
icloud:
  kv_store: true
apple:
  icloud:
    containers:
      - iCloud.com.example.quest
      - iCloud.com.example.shared
`
	if err := os.WriteFile(filepath.Join(dir2, "nsgo.yaml"), []byte(yaml2), 0o644); err != nil {
		t.Fatal(err)
	}
	p2, err := load(dir2)
	if err != nil {
		t.Fatalf("load icloud aliases: %v", err)
	}
	cfg2 := p2.cloudSettings()
	if !cfg2.KVStore || !slices.Equal(cfg2.Containers, []string{"iCloud.com.example.quest", "iCloud.com.example.shared"}) {
		t.Fatalf("merged cloudSettings = %+v", cfg2)
	}

	// Invalid containers are rejected.
	for _, bad := range []string{
		"cloud:\n  containers: [com.example.missing-prefix]\n",
		"cloud:\n  containers: [iCloud.]\n",
		"cloud:\n  containers: [\"iCloud.has space\"]\n",
		"cloud:\n  containers: [iCloud.dup, iCloud.dup]\n",
	} {
		badDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(badDir, "nsgo.yaml"), []byte("name: Bad\n"+bad), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := load(badDir); err == nil {
			t.Errorf("load accepted invalid cloud config:\n%s", bad)
		}
	}
}

func TestCloudEntitlementsPlistGeneration(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "Quest", ID: "com.example.quest"}}
	p.Cloud = cloudConfig{
		KVStore:    true,
		Containers: []string{"iCloud.com.example.quest"},
	}

	// 1. Xcode's automatic signing claims iCloud with its team placeholder; a Mac App Store build claims it only from its provisioning profile (step 2), so its sandbox entitlements leave it out.
	for key := range p.sandboxEntitlements() {
		if key == kvStoreEntitlement || key == icloudContainersEntitlement || key == icloudServicesEntitlement {
			t.Errorf("sandbox entitlements claim %s without a provisioning profile", key)
		}
	}
	for name, ents := range map[string]map[string]any{
		"xcode": p.xcodeEntitlements(false, false),
	} {
		if ents[kvStoreEntitlement] != "$(TeamIdentifierPrefix)com.example.quest" {
			t.Errorf("%s: %s = %#v", name, kvStoreEntitlement, ents[kvStoreEntitlement])
		}
		gotContainers, _ := ents[icloudContainersEntitlement].([]any)
		if len(gotContainers) != 1 || gotContainers[0] != "iCloud.com.example.quest" {
			t.Errorf("%s: %s = %#v", name, icloudContainersEntitlement, ents[icloudContainersEntitlement])
		}
		gotServices, _ := ents[icloudServicesEntitlement].([]any)
		if len(gotServices) != 1 || gotServices[0] != "CloudDocuments" {
			t.Errorf("%s: %s = %#v", name, icloudServicesEntitlement, ents[icloudServicesEntitlement])
		}
	}

	// 2. Provisioning profile signing resolves team prefix and round-trips through encodePlist / decodePlist.
	granted := map[string]any{
		"application-identifier":              "ABCDE12345.com.example.quest",
		"com.apple.developer.team-identifier": "ABCDE12345",
		kvStoreEntitlement:                    "ABCDE12345.*",
		icloudContainersEntitlement:           []any{"iCloud.com.example.quest"},
		icloudServicesEntitlement:             []any{"CloudDocuments"},
	}
	signed := map[string]any{"application-identifier": "ABCDE12345.com.example.quest"}
	if err := p.claimCloudEntitlements(signed, granted, teamIDFromEntitlements(granted)); err != nil {
		t.Fatalf("claimCloudEntitlements: %v", err)
	}
	plist := encodePlist(signed)
	decoded, err := decodePlist(plist)
	if err != nil {
		t.Fatalf("decodePlist: %v", err)
	}
	dict := decoded.(map[string]any)
	if dict[kvStoreEntitlement] != "ABCDE12345.com.example.quest" {
		t.Errorf("decoded %s = %#v", kvStoreEntitlement, dict[kvStoreEntitlement])
	}
	if containers, _ := dict[icloudContainersEntitlement].([]any); len(containers) != 1 || containers[0] != "iCloud.com.example.quest" {
		t.Errorf("decoded %s = %#v", icloudContainersEntitlement, dict[icloudContainersEntitlement])
	}
	if services, _ := dict[icloudServicesEntitlement].([]any); len(services) != 1 || services[0] != "CloudDocuments" {
		t.Errorf("decoded %s = %#v", icloudServicesEntitlement, dict[icloudServicesEntitlement])
	}

	// 3. Profile missing KV or container fails with a clear message.
	if err := p.claimCloudEntitlements(map[string]any{}, map[string]any{}, "ABCDE12345"); err == nil || !strings.Contains(err.Error(), kvStoreEntitlement) {
		t.Errorf("missing KV grant returned %v", err)
	}
	if err := p.claimCloudEntitlements(map[string]any{}, map[string]any{kvStoreEntitlement: "ABCDE12345.*"}, "ABCDE12345"); err == nil || !strings.Contains(err.Error(), "iCloud.com.example.quest") {
		t.Errorf("missing container grant returned %v", err)
	}

	// 4. Project without cloud config strips iCloud entitlements from profile.
	plain := &project{Config: projectcfg.Config{Name: "Plain", ID: "com.example.plain"}}
	ents := map[string]any{
		kvStoreEntitlement:          "ABCDE12345.*",
		icloudContainersEntitlement: []any{"iCloud.com.example.plain"},
		icloudServicesEntitlement:   []any{"CloudDocuments"},
	}
	if err := plain.claimCloudEntitlements(ents, ents, "ABCDE12345"); err != nil {
		t.Fatalf("plain claimCloudEntitlements: %v", err)
	}
	for _, key := range []string{kvStoreEntitlement, icloudContainersEntitlement, icloudServicesEntitlement} {
		if _, ok := ents[key]; ok {
			t.Errorf("plain project kept unrequested entitlement %s", key)
		}
	}
}
