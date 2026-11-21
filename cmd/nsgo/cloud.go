package main

import (
	"fmt"
	"slices"
	"strings"
)

// cloudConfig is nsgo.yaml's cloud (or icloud / apple.icloud) section: Apple iCloud key-value store and ubiquity document container identifiers written into generated .entitlements plists by nsgo build.
type cloudConfig struct {
	KVStore    bool     `yaml:"kv_store"`
	Containers []string `yaml:"containers"`
}

const (
	kvStoreEntitlement          = "com.apple.developer.ubiquity-kvstore-identifier"
	icloudContainersEntitlement = "com.apple.developer.icloud-container-identifiers"
	icloudServicesEntitlement   = "com.apple.developer.icloud-services"
)

// cloudSettings merges top-level cloud:, icloud: and apple.icloud: into one deterministic configuration, preserving container order without duplicates.
func (p *project) cloudSettings() cloudConfig {
	cfg := cloudConfig{
		KVStore: p.Cloud.KVStore || p.ICloud.KVStore || p.Apple.ICloud.KVStore,
	}
	seen := map[string]bool{}
	for _, list := range [][]string{p.Cloud.Containers, p.ICloud.Containers, p.Apple.ICloud.Containers} {
		for _, c := range list {
			if !seen[c] {
				seen[c] = true
				cfg.Containers = append(cfg.Containers, c)
			}
		}
	}
	return cfg
}

// cloudEnabled reports whether nsgo.yaml asks for any Apple iCloud entitlement.
func (p *project) cloudEnabled() bool {
	cfg := p.cloudSettings()
	return cfg.KVStore || len(cfg.Containers) > 0
}

// validateCloud checks that every configured iCloud container identifier is non-empty, has no whitespace, is not duplicated within a single section, and starts with Apple's required "iCloud." prefix.
func (p *project) validateCloud() error {
	for _, section := range []struct {
		name string
		list []string
	}{
		{"cloud.containers", p.Cloud.Containers},
		{"icloud.containers", p.ICloud.Containers},
		{"apple.icloud.containers", p.Apple.ICloud.Containers},
	} {
		seen := map[string]bool{}
		for _, c := range section.list {
			if c == "" || strings.ContainsAny(c, " \t\r\n") {
				return fmt.Errorf("%s has invalid container %q", section.name, c)
			}
			if !strings.HasPrefix(c, "iCloud.") || len(c) == len("iCloud.") {
				return fmt.Errorf("%s container %q must start with \"iCloud.\"", section.name, c)
			}
			if seen[c] {
				return fmt.Errorf("%s repeats container %q", section.name, c)
			}
			seen[c] = true
		}
	}
	return nil
}

// cloudEntitlements returns the Apple iCloud entitlements requested by nsgo.yaml. When team is non-empty, the key-value store identifier uses "<team>.<id>"; otherwise it uses Xcode's "$(TeamIdentifierPrefix)<id>" placeholder.
func (p *project) cloudEntitlements(team string) map[string]any {
	cfg := p.cloudSettings()
	if !cfg.KVStore && len(cfg.Containers) == 0 {
		return nil
	}
	ents := map[string]any{}
	if cfg.KVStore {
		if team != "" {
			ents[kvStoreEntitlement] = team + "." + p.ID
		} else {
			ents[kvStoreEntitlement] = "$(TeamIdentifierPrefix)" + p.ID
		}
	}
	if len(cfg.Containers) > 0 {
		containers := make([]any, len(cfg.Containers))
		for i, c := range cfg.Containers {
			containers[i] = c
		}
		ents[icloudContainersEntitlement] = containers
		ents[icloudServicesEntitlement] = []any{"CloudDocuments"}
	}
	return ents
}

// claimCloudEntitlements puts the requested iCloud entitlements into ents after checking that granted (a provisioning profile's entitlements) allows them, or removes iCloud entitlements from ents when nsgo.yaml does not ask for them.
func (p *project) claimCloudEntitlements(ents, granted map[string]any, team string) error {
	cfg := p.cloudSettings()
	if !cfg.KVStore {
		delete(ents, kvStoreEntitlement)
	}
	if len(cfg.Containers) == 0 {
		delete(ents, icloudContainersEntitlement)
		delete(ents, icloudServicesEntitlement)
	}
	if !cfg.KVStore && len(cfg.Containers) == 0 {
		return nil
	}
	if cfg.KVStore {
		if _, ok := granted[kvStoreEntitlement]; !ok {
			return fmt.Errorf("the provisioning profile does not grant iCloud Key-Value Store (%s) for %s: enable iCloud on the App ID and download the profile again", kvStoreEntitlement, p.ID)
		}
		if team != "" {
			ents[kvStoreEntitlement] = team + "." + p.ID
		} else if v, ok := granted[kvStoreEntitlement].(string); ok && v != "" {
			ents[kvStoreEntitlement] = signedAppID(v, p.ID)
		} else {
			ents[kvStoreEntitlement] = "$(TeamIdentifierPrefix)" + p.ID
		}
	}
	if len(cfg.Containers) > 0 {
		allowed, _ := granted[icloudContainersEntitlement].([]any)
		for _, want := range cfg.Containers {
			if !slices.Contains(allowed, any(want)) && !slices.Contains(allowed, any("*")) {
				return fmt.Errorf("the provisioning profile does not grant iCloud container %s for %s: assign the container to the App ID and download the profile again", want, p.ID)
			}
		}
		containers := make([]any, len(cfg.Containers))
		for i, c := range cfg.Containers {
			containers[i] = c
		}
		ents[icloudContainersEntitlement] = containers
		ents[icloudServicesEntitlement] = []any{"CloudDocuments"}
	}
	return nil
}

// teamIDFromEntitlements returns the Apple team ID from a provisioning profile's entitlements dictionary, checking com.apple.developer.team-identifier and falling back to the prefix of application-identifier.
func teamIDFromEntitlements(ents map[string]any) string {
	if team, _ := ents["com.apple.developer.team-identifier"].(string); team != "" {
		return team
	}
	for _, key := range []string{"application-identifier", "com.apple.application-identifier"} {
		if appID, _ := ents[key].(string); appID != "" {
			if team, _, ok := strings.Cut(appID, "."); ok && team != "" {
				return team
			}
		}
	}
	return ""
}
