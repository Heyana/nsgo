package engine

import (
	"io/fs"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestUserDirFollowsEachDesktop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the paths are Unix paths; Windows asks SHGetKnownFolderPath")
	}
	const dirs = `# This file is written by xdg-user-dirs-update
XDG_DESKTOP_DIR="$HOME/Schreibtisch"
XDG_DOWNLOAD_DIR="$HOME/Downloads"
XDG_DOCUMENTS_DIR="/data/My \"Docs\""
XDG_MUSIC_DIR="$HOME"
XDG_PICTURES_DIR=relative
XDG_PICTURES_DIR = "$HOME/Bilder"
`
	env := func(vars map[string]string) func(string) string {
		return func(name string) string { return vars[name] }
	}
	read := func(files map[string]string) func(string) ([]byte, error) {
		return func(name string) ([]byte, error) {
			if data, ok := files[name]; ok {
				return []byte(data), nil
			}
			return nil, fs.ErrNotExist
		}
	}
	home := map[string]string{"HOME": "/home/ada"}
	custom := map[string]string{"HOME": "/home/ada", "XDG_CONFIG_HOME": "/cfg"}
	for _, c := range []struct {
		name  string
		goos  string
		env   map[string]string
		files map[string]string
		dir   core.StandardDir
		want  string
	}{
		{"macOS documents", "darwin", map[string]string{"HOME": "/Users/ada"}, nil, core.DocumentsDir, "/Users/ada/Documents"},
		{"macOS videos are Movies", "darwin", map[string]string{"HOME": "/Users/ada"}, nil, core.VideosDir, "/Users/ada/Movies"},
		{"macOS without a home", "darwin", nil, nil, core.DownloadsDir, ""},
		{"an unknown folder", "darwin", map[string]string{"HOME": "/Users/ada"}, nil, 0, ""},
		{"Linux reads user-dirs.dirs", "linux", home, map[string]string{"/home/ada/.config/user-dirs.dirs": dirs}, core.DesktopDir, "/home/ada/Schreibtisch"},
		{"XDG_CONFIG_HOME holds it", "linux", custom, map[string]string{"/cfg/user-dirs.dirs": dirs}, core.DownloadsDir, "/home/ada/Downloads"},
		{"an absolute path with escapes", "linux", home, map[string]string{"/home/ada/.config/user-dirs.dirs": dirs}, core.DocumentsDir, `/data/My "Docs"`},
		{"the home directory disables it", "linux", home, map[string]string{"/home/ada/.config/user-dirs.dirs": dirs}, core.MusicDir, ""},
		{"the last valid line wins", "linux", home, map[string]string{"/home/ada/.config/user-dirs.dirs": dirs}, core.PicturesDir, "/home/ada/Bilder"},
		{"an unset folder", "linux", home, map[string]string{"/home/ada/.config/user-dirs.dirs": dirs}, core.VideosDir, ""},
		{"the desktop's default", "linux", home, nil, core.DesktopDir, "/home/ada/Desktop"},
		{"no other defaults", "linux", home, nil, core.DownloadsDir, ""},
		{"FreeBSD as Linux", "freebsd", home, nil, core.DesktopDir, "/home/ada/Desktop"},
		{"Android has none", "android", home, nil, core.DesktopDir, ""},
		{"iOS has none", "ios", home, nil, core.DocumentsDir, ""},
		{"the web has none", "js", home, nil, core.DocumentsDir, ""},
	} {
		if got := userDir(c.goos, c.dir, env(c.env), read(c.files)); got != c.want {
			t.Errorf("%s: userDir = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestSystemUserDir(t *testing.T) {
	var asked []string
	lookup := SystemUserDir(func(id string) string {
		asked = append(asked, id)
		return `C:\Users\ada\` + id
	})
	for dir := core.DocumentsDir; dir <= core.VideosDir; dir++ {
		path := lookup(dir)
		t.Logf("folder %d: %q", dir, path)
		if path != "" && !filepath.IsAbs(path) && runtime.GOOS != "windows" {
			t.Errorf("SystemUserDir(%d) = %q, not absolute", dir, path)
		}
	}
	if lookup(0) != "" {
		t.Error("an unknown folder has a path")
	}
	if runtime.GOOS == "windows" {
		if len(asked) != 6 || asked[0] != "FDD39AD0-238F-46AF-ADB4-6C85480369C7" || asked[5] != "18989B1D-99B5-455B-841C-AB7C74E4DDFC" {
			t.Errorf("Windows asked for %q", asked)
		}
	} else if len(asked) != 0 {
		t.Errorf("%s asked for known folders %q", runtime.GOOS, asked)
	}
}
