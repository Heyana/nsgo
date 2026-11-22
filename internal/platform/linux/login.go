//go:build linux && !android

package linux

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/platform/appimage"
)

// loginEntry is the per-user XDG autostart file for an app.
type loginEntry struct {
	path, name, executable string
}

// errLoginPortal directs Flatpak commands to the asynchronous Background portal.
var errLoginPortal = errors.New("Flatpak login registration requires the Background portal")

// appLoginEntry is the entry for the app with ID id, or the executable's name for an app without one.
func appLoginEntry(id, name, executable string) (loginEntry, error) {
	if id == "" {
		id = filepath.Base(executable)
	}
	if id == "" || id == "." || strings.ContainsAny(id, "/\\") {
		return loginEntry{}, errors.New("the app ID is not a valid autostart file name")
	}
	if strings.ContainsRune(executable, '%') {
		// GLib looks the Exec's program up before it expands field codes, so the %% a percent sign takes there names no file and GNOME starts nothing.
		return loginEntry{}, errors.New("an executable whose path holds a percent sign cannot start at login")
	}
	if name == "" {
		name = id
	}
	config := os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(config) {
		// The XDG Base Directory Specification ignores a relative path, as it ignores an unset one.
		home, err := os.UserHomeDir()
		if err != nil {
			return loginEntry{}, err
		}
		config = filepath.Join(home, ".config")
	}
	return loginEntry{path: filepath.Join(config, "autostart", id+".desktop"), name: name, executable: executable}, nil
}

// loginExecutable is the file the session starts: the AppImage itself for an app in one, as its runtime mounts the app at a path of the run's own, else the running executable.
func loginExecutable() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", err
	}
	if image := appimage.Path(executable); image != "" {
		return image, nil
	}
	return executable, nil
}

// desktopExecArg writes one argument of an Exec key as the Desktop Entry Specification reads it: quoted when it holds a reserved character, with ", `, $ and \ escaped inside the quotes, and % doubled, as field codes take it.
func desktopExecArg(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	if s != "" && !strings.ContainsAny(s, " \t\n\"'\\><~|&;$*?#()`") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if strings.ContainsRune("\"`$\\", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// desktopValue escapes a string value of a desktop entry. An Exec value is unescaped this way before its quoting is read, so a backslash inside quotes is written four times.
func desktopValue(s string) string {
	return strings.NewReplacer("\\", "\\\\", "\n", "\\n", "\t", "\\t", "\r", "\\r").Replace(s)
}

func (e loginEntry) data() []byte {
	exec := desktopExecArg(e.executable) + " " + desktopExecArg("--nsgo-launched-at-login")
	return []byte("[Desktop Entry]\nType=Application\nName=" + desktopValue(e.name) + "\nExec=" + desktopValue(exec) + "\nX-GNOME-Autostart-enabled=true\n")
}

func (e loginEntry) set(on bool) error {
	if !on {
		err := os.Remove(e.path)
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if err := os.MkdirAll(filepath.Dir(e.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(e.path), ".nsgo-login-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(e.data()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), e.path)
}

// enabled reports whether the entry starts this executable: one the user turned off, edited or left behind by a copy of the app elsewhere does not.
func (e loginEntry) enabled() bool {
	b, err := os.ReadFile(e.path)
	return err == nil && bytes.Equal(b, e.data())
}

// currentLoginEntry is the running app's entry.
func currentLoginEntry(id, name string) (loginEntry, error) {
	if InstalledBy() == "Flatpak" {
		return loginEntry{}, errLoginPortal
	}
	executable, err := loginExecutable()
	if err != nil {
		return loginEntry{}, err
	}
	if InstalledBy() == "Snap" {
		label, _ := os.ReadFile("/proc/self/attr/current")
		instance := os.Getenv("SNAP_INSTANCE_NAME")
		if instance == "" {
			instance = os.Getenv("SNAP_NAME")
		}
		return snapLoginEntry(id, name, executable, os.Getenv("SNAP"), os.Getenv("SNAP_USER_DATA"), instance, string(label))
	}
	return appLoginEntry(id, name, executable)
}

func setLaunchAtLogin(on bool, id, name string) {
	e, err := currentLoginEntry(id, name)
	if err == nil {
		err = e.set(on)
	}
	if err != nil {
		log.Printf("nsgo: launch at login: %v", err)
	}
}

func launchesAtLogin(id, name string) bool {
	e, err := currentLoginEntry(id, name)
	return err == nil && e.enabled()
}
