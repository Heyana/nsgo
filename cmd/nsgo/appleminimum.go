package main

import (
	"context"
	"fmt"
	"slices"
	"text/template"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

// Apple's native plist keys remain the home of package minimum versions, as in NSH. A package may require a newer system than its runtime's build floor; raising that installation requirement does not rebuild bundled frameworks. Go 1.27, which this module requires, runs on macOS 13 and later.
const macOSRuntimeMinimum = "13.0"

// Mac Catalyst 16 targets macOS 13, the Darwin Go runtime's floor.
const macCatalystRuntimeMinimum = "16.0"

func appleVersion(value string) ([3]uint32, bool) {
	return projectcfg.AppleVersion(value)
}

func (p *project) minimumVersion(key, floor string) (string, error) {
	value, exists := p.Apple.Info[key]
	if !exists {
		return floor, nil
	}
	text, ok := value.(string)
	parsed, valid := appleVersion(text)
	baseline, _ := appleVersion(floor)
	if !ok || !valid {
		return "", fmt.Errorf("apple.info.%s: write a version string of one to three numbers, from the runtime minimum %s up", key, floor)
	}
	if slices.Compare(parsed[:], baseline[:]) < 0 {
		return "", fmt.Errorf("apple.info.%s: %q is below the runtime minimum %s", key, text, floor)
	}
	return text, nil
}

func (p *project) appleMinimum(sys *appleOS) (string, error) {
	if sys == nil {
		return p.minimumVersion("LSMinimumSystemVersion", macOSRuntimeMinimum)
	}
	if sys == iOS && p.clipParent != "" {
		return p.parentClip.Minimum, nil
	}
	return p.minimumVersion("MinimumOSVersion", sys.minimum)
}

// IOSMinimum is the selected iOS package version after configuration validation.
func (p *project) IOSMinimum() string {
	minimum, _ := p.appleMinimum(iOS)
	return minimum
}

func (p *project) MacOSMinimum() (string, error)   { return p.appleMinimum(nil) }
func (p *project) TVOSMinimum() (string, error)    { return p.appleMinimum(tvOS) }
func (p *project) WatchOSMinimum() (string, error) { return p.appleMinimum(watchOS) }

func (p *project) validatePlistMinimum(t *template.Template) error {
	var sys *appleOS
	switch t {
	case macPlist:
	case iosPlist:
		sys = iOS
	case tvosPlist:
		sys = tvOS
	case watchosPlist:
		sys = watchOS
	default:
		return nil
	}
	_, err := p.appleMinimum(sys)
	return err
}

// iosProjectEnv gives this app's link step its declared deployment target; shared runtime helpers and check-only environments retain their own floors.
func iosProjectEnv(ctx context.Context, p *project, sys *appleOS, simulator bool) ([]string, error) {
	minimum, err := p.appleMinimum(sys)
	if err != nil {
		return nil, err
	}
	configured := *sys
	configured.minimum = minimum
	return iosEnv(ctx, &configured, simulator)
}

func frameworkMinimum(minimum string, catalyst bool) string {
	if catalyst {
		requested, _ := appleVersion(minimum)
		floor, _ := appleVersion(macCatalystRuntimeMinimum)
		if slices.Compare(requested[:], floor[:]) < 0 {
			return macCatalystRuntimeMinimum
		}
	}
	return minimum
}
