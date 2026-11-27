//go:build linux && !android

package linux

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

// snapLoginEntry follows snap userd's contract: the filename must match an
// app's autostart metadata and lives under SNAP_USER_DATA, irrespective of the
// sandbox's XDG_CONFIG_HOME. snapd substitutes that app's wrapper for argv[0].
func snapLoginEntry(id, name, executable, snapRoot, userData, instance, label string) (loginEntry, error) {
	if !filepath.IsAbs(snapRoot) || !filepath.IsAbs(userData) {
		return loginEntry{}, errors.New("Snap login registration needs SNAP and SNAP_USER_DATA")
	}
	relative, err := filepath.Rel(snapRoot, executable)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return loginEntry{}, errors.New("the executable is outside the Snap package")
	}
	data, err := os.ReadFile(filepath.Join(snapRoot, "meta", "snap.yaml"))
	if err != nil {
		return loginEntry{}, err
	}
	var metadata struct {
		Apps map[string]struct {
			Command   string `yaml:"command"`
			Autostart string `yaml:"autostart"`
		} `yaml:"apps"`
	}
	if err := yaml.Unmarshal(data, &metadata); err != nil {
		return loginEntry{}, err
	}
	appName := ""
	if prefix := "snap." + instance + "."; instance != "" && strings.HasPrefix(label, prefix) {
		if words := strings.Fields(strings.TrimPrefix(label, prefix)); len(words) > 0 {
			appName = words[0]
		}
	}
	if appName == "" {
		for candidate, app := range metadata.Apps {
			words := strings.Fields(app.Command)
			if len(words) == 0 || filepath.Clean(words[0]) != filepath.Clean(relative) {
				continue
			}
			if appName != "" {
				return loginEntry{}, errors.New("Snap apps sharing an executable need an AppArmor app identity")
			}
			appName = candidate
		}
		if appName == "" && len(metadata.Apps) == 1 {
			for candidate := range metadata.Apps {
				appName = candidate
			}
		}
	}
	app, ok := metadata.Apps[appName]
	if !ok || app.Autostart == "" {
		return loginEntry{}, errors.New("the Snap app must declare autostart in snap.yaml")
	}
	if filepath.Base(app.Autostart) != app.Autostart || !strings.HasSuffix(app.Autostart, ".desktop") || strings.ContainsAny(app.Autostart, "\\\x00") {
		return loginEntry{}, fmt.Errorf("invalid Snap autostart filename %q", app.Autostart)
	}
	if appName == "" || strings.ContainsRune(appName, '%') {
		return loginEntry{}, fmt.Errorf("invalid Snap app name %q", appName)
	}
	if name == "" {
		name = id
	}
	if name == "" {
		name = appName
	}
	// Use the revision-independent Snap app name in Exec: snap userd replaces
	// argv[0] with the app's wrapper and preserves the login marker, so a Snap
	// refresh copying SNAP_USER_DATA keeps LaunchAtLogin accurate.
	return loginEntry{path: filepath.Join(userData, ".config", "autostart", app.Autostart), name: name, executable: appName}, nil
}
