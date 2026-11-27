package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"strings"
	"unicode"
)

// privacyManifestName is the privacy manifest Apple reads at a bundle's resource root.
const privacyManifestName = "PrivacyInfo.xcprivacy"

const diskSpaceCategory = "NSPrivacyAccessedAPICategoryDiskSpace"

// privacyLdflags carries the project's declaration into the Apple binary,
// including a framework that the tool builds for an embedding host.
func (p *project) privacyLdflags(env []string) []string {
	target := envTarget(env)
	if !strings.HasPrefix(target, "macos_") && !strings.HasPrefix(target, "ios_") {
		return nil
	}
	if len(p.Apple.Privacy.AccessedAPIs[diskSpaceCategory]) == 0 {
		return nil
	}
	return []string{"-X github.com/nsgo-dev/nsgo/internal/platform/apple.diskSpaceDeclared=1"}
}

// nsgoPrivacyReasons are the required-reason APIs every NSGo app uses on Apple platforms, with their reasons: NSUserDefaults for the app's own settings (internal/platform/apple's language and notification keys, CA92.1), Go's os package's stat, fstat, fstatat and lstat on files in the app's containers (C617.1), and mach_absolute_time, Go's monotonic clock, and NSProcessInfo.systemUptime for elapsed time within the app (35F9.1). Disk space and active keyboards are left to apple.privacy. The disk-space service runs only when the project declares it.
var nsgoPrivacyReasons = map[string][]string{
	"NSPrivacyAccessedAPICategoryUserDefaults":   {"CA92.1"},
	"NSPrivacyAccessedAPICategoryFileTimestamp":  {"C617.1"},
	"NSPrivacyAccessedAPICategorySystemBootTime": {"35F9.1"},
}

// privacyManifest returns a bundle's PrivacyInfo.xcprivacy: existing, the manifest the app's resources put there or nil, with NSGo's declarations and apple.privacy's merged in. Categories and their reasons are sorted, so a build writes the same file each time.
func (p *project) privacyManifest(existing map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range existing {
		out[k] = v
	}
	reasons := map[string][]string{}
	add := func(name string, rs ...string) {
		for _, r := range rs {
			if !slices.Contains(reasons[name], r) {
				reasons[name] = append(reasons[name], r)
			}
		}
	}
	others := []any{}
	for _, e := range plistArray(existing["NSPrivacyAccessedAPITypes"]) {
		d, _ := e.(map[string]any)
		name, _ := d["NSPrivacyAccessedAPIType"].(string)
		if name == "" {
			others = append(others, e)
			continue
		}
		for _, r := range plistArray(d["NSPrivacyAccessedAPITypeReasons"]) {
			if s, ok := r.(string); ok {
				add(name, s)
			}
		}
	}
	for name, rs := range nsgoPrivacyReasons {
		add(name, rs...)
	}
	for name, rs := range p.Apple.Privacy.AccessedAPIs {
		add(name, rs...)
	}
	names := make([]string, 0, len(reasons))
	for name := range reasons {
		names = append(names, name)
	}
	sort.Strings(names)
	accessed := others
	for _, name := range names {
		rs := reasons[name]
		sort.Strings(rs)
		values := make([]any, len(rs))
		for i, r := range rs {
			values[i] = r
		}
		accessed = append(accessed, map[string]any{
			"NSPrivacyAccessedAPIType":        name,
			"NSPrivacyAccessedAPITypeReasons": values,
		})
	}
	out["NSPrivacyAccessedAPITypes"] = accessed

	tracking, _ := existing["NSPrivacyTracking"].(bool)
	if t := p.Apple.Privacy.Tracking; t != nil && *t {
		tracking = true
	}
	out["NSPrivacyTracking"] = tracking

	domains := plistArray(existing["NSPrivacyTrackingDomains"])
	for _, d := range p.Apple.Privacy.TrackingDomains {
		if !slices.Contains(domains, any(d)) {
			domains = append(domains, d)
		}
	}
	out["NSPrivacyTrackingDomains"] = domains

	data := plistArray(existing["NSPrivacyCollectedDataTypes"])
	for _, item := range p.Apple.Privacy.CollectedData {
		v := plistValue(item)
		if !slices.ContainsFunc(data, func(e any) bool { return reflect.DeepEqual(e, v) }) {
			data = append(data, v)
		}
	}
	out["NSPrivacyCollectedDataTypes"] = data
	return out
}

// plistArray is v as a property list's array, empty but not nil when it is none, so encodePlist writes an empty array.
func plistArray(v any) []any {
	a, _ := v.([]any)
	return append([]any{}, a...)
}

// plistValue converts what nsgo.yaml decodes into the values encodePlist writes, as int64 for YAML's ints.
func plistValue(v any) any {
	switch v := v.(type) {
	case map[string]any:
		m := make(map[string]any, len(v))
		for k, e := range v {
			m[k] = plistValue(e)
		}
		return m
	case []any:
		a := make([]any, len(v))
		for i, e := range v {
			a[i] = plistValue(e)
		}
		return a
	case int:
		return int64(v)
	}
	return v
}

// writePrivacyManifest writes dir's PrivacyInfo.xcprivacy, merging NSGo's and apple.privacy's declarations into one the app's resources already put there.
func (p *project) writePrivacyManifest(dir string) error {
	path := filepath.Join(dir, privacyManifestName)
	var existing map[string]any
	if data, err := os.ReadFile(path); err == nil {
		v, err := decodePlist(data)
		if err != nil {
			return fmt.Errorf("%s: %w", privacyManifestName, err)
		}
		var ok bool
		if existing, ok = v.(map[string]any); !ok {
			return fmt.Errorf("%s: not a dictionary", privacyManifestName)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if p.checkStorePrivacy {
		if err := validateResourcePrivacy(existing); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		for i, item := range p.Apple.Privacy.CollectedData {
			if err := validateCollectedPrivacyData(item); err != nil {
				return fmt.Errorf("apple.privacy.collected_data[%d]: %w", i, err)
			}
		}
	}
	manifest := p.privacyManifest(existing)
	if p.checkStorePrivacy {
		if err := validatePrivacyTracking(manifest); err != nil {
			return fmt.Errorf("%s (including apple.privacy): %w", path, err)
		}
		if manifest["NSPrivacyTracking"] == false {
			delete(manifest, "NSPrivacyTrackingDomains")
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, encodePlist(manifest), 0o644)
}

// validateResourcePrivacy checks fields before merging can discard malformed values.
func validateResourcePrivacy(manifest map[string]any) error {
	if tracking, exists := manifest["NSPrivacyTracking"]; exists {
		if _, ok := tracking.(bool); !ok {
			return errors.New("NSPrivacyTracking must be a boolean")
		}
	}
	if domains, exists := manifest["NSPrivacyTrackingDomains"]; exists {
		if _, ok := domains.([]any); !ok {
			return errors.New("NSPrivacyTrackingDomains must be an array of domain names")
		}
	}
	if err := validatePrivacyEntries(manifest, "NSPrivacyAccessedAPITypes", validatePrivacyAPI); err != nil {
		return err
	}
	return validatePrivacyEntries(manifest, "NSPrivacyCollectedDataTypes", validateCollectedPrivacyData)
}

func validatePrivacyEntries(manifest map[string]any, key string, validate func(map[string]any) error) error {
	value, exists := manifest[key]
	if !exists {
		return nil
	}
	items, ok := value.([]any)
	if !ok {
		return fmt.Errorf("%s must be an array of dictionaries", key)
	}
	for i, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("%s[%d] must be a dictionary", key, i)
		}
		if err := validate(entry); err != nil {
			return fmt.Errorf("%s[%d]: %w", key, i, err)
		}
	}
	return nil
}

func validatePrivacyAPI(entry map[string]any) error {
	const prefix = "NSPrivacyAccessedAPICategory"
	if category, _ := entry["NSPrivacyAccessedAPIType"].(string); !strings.HasPrefix(category, prefix) || category == prefix {
		return errors.New("NSPrivacyAccessedAPIType must name a required-reason API category")
	}
	return validatePrivacyStrings(entry, "NSPrivacyAccessedAPITypeReasons")
}

func validateCollectedPrivacyData(entry map[string]any) error {
	const prefix = "NSPrivacyCollectedDataType"
	if category, _ := entry[prefix].(string); !strings.HasPrefix(category, prefix) || category == prefix {
		return errors.New("NSPrivacyCollectedDataType must name a collected data type")
	}
	for _, key := range []string{"NSPrivacyCollectedDataTypeLinked", "NSPrivacyCollectedDataTypeTracking"} {
		if _, ok := entry[key].(bool); !ok {
			return fmt.Errorf("%s must be a boolean", key)
		}
	}
	return validatePrivacyStrings(entry, "NSPrivacyCollectedDataTypePurposes")
}

func validatePrivacyStrings(entry map[string]any, key string) error {
	values, _ := entry[key].([]any)
	if len(values) == 0 {
		return fmt.Errorf("%s must be a nonempty array of strings", key)
	}
	for i, v := range values {
		if value, ok := v.(string); !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("%s[%d] must be a nonempty string", key, i)
		}
	}
	return nil
}

func validatePrivacyTracking(manifest map[string]any) error {
	tracking, _ := manifest["NSPrivacyTracking"].(bool)
	domains := plistArray(manifest["NSPrivacyTrackingDomains"])
	if tracking && len(domains) == 0 {
		return errors.New("NSPrivacyTracking is true; list the tracking domains in NSPrivacyTrackingDomains or apple.privacy.tracking_domains")
	}
	if !tracking && len(domains) > 0 {
		return errors.New("tracking domains require NSPrivacyTracking or apple.privacy.tracking to be true")
	}
	for i, value := range domains {
		domain, _ := value.(string)
		if !strings.Contains(domain, ".") || strings.ContainsAny(domain, ":/?#") || strings.ContainsFunc(domain, unicode.IsSpace) {
			return fmt.Errorf("NSPrivacyTrackingDomains[%d] must be a domain name without a scheme, port, path or query", i)
		}
	}
	return nil
}
