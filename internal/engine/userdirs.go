package engine

import (
	"bufio"
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nsgo-dev/nsgo/core"
)

// SystemUserDir answers core.Scope.UserDir as the desktop the app runs on
// does at the time of the call, so a folder the user moves is followed:
// Foundation's folders in the home directory on macOS, the known folders
// on Windows, which knownFolder looks up by FOLDERID
// (knownfolder.Path), and the XDG user directories elsewhere. A folder
// is "" where the system names no such folder, and on phones and the web.
func SystemUserDir(knownFolder func(id string) string) func(core.StandardDir) string {
	return func(dir core.StandardDir) string {
		if runtime.GOOS == "windows" {
			if id := knownFolders[dir]; id != "" && knownFolder != nil {
				return knownFolder(id)
			}
			return ""
		}
		return userDir(runtime.GOOS, dir, os.Getenv, os.ReadFile)
	}
}

// knownFolders are the FOLDERIDs of the user's folders.
var knownFolders = map[core.StandardDir]string{
	core.DocumentsDir: "FDD39AD0-238F-46AF-ADB4-6C85480369C7", core.DownloadsDir: "374DE290-123F-4565-9164-39C4925E467B",
	core.DesktopDir: "B4BFCC3A-DB2C-424C-B029-7FE99A87C641", core.PicturesDir: "33E28130-4E1E-4676-835A-98395C3BC3BB",
	core.MusicDir: "4BD8D571-6D19-48D3-BE97-422220080E43", core.VideosDir: "18989B1D-99B5-455B-841C-AB7C74E4DDFC",
}

// appleUserDirs are the user domain's folders NSSearchPathForDirectoriesInDomains names under the home directory.
var appleUserDirs = map[core.StandardDir]string{
	core.DocumentsDir: "Documents", core.DownloadsDir: "Downloads", core.DesktopDir: "Desktop",
	core.PicturesDir: "Pictures", core.MusicDir: "Music", core.VideosDir: "Movies",
}

// xdgUserDirs are the keys of user-dirs.dirs.
var xdgUserDirs = map[core.StandardDir]string{
	core.DocumentsDir: "XDG_DOCUMENTS_DIR", core.DownloadsDir: "XDG_DOWNLOAD_DIR", core.DesktopDir: "XDG_DESKTOP_DIR",
	core.PicturesDir: "XDG_PICTURES_DIR", core.MusicDir: "XDG_MUSIC_DIR", core.VideosDir: "XDG_VIDEOS_DIR",
}

func userDir(goos string, dir core.StandardDir, env func(string) string, read func(string) ([]byte, error)) string {
	switch goos {
	case "android", "ios", "js", "wasip1", "windows":
		return ""
	}
	home := env("HOME")
	if home == "" || !filepath.IsAbs(home) {
		return ""
	}
	if goos == "darwin" {
		if name := appleUserDirs[dir]; name != "" {
			return filepath.Join(home, name)
		}
		return ""
	}
	key := xdgUserDirs[dir]
	if key == "" {
		return ""
	}
	config := env("XDG_CONFIG_HOME")
	if config == "" || !filepath.IsAbs(config) {
		config = filepath.Join(home, ".config")
	}
	if data, err := read(filepath.Join(config, "user-dirs.dirs")); err == nil {
		if path, ok := xdgUserDir(data, key, home); ok {
			// A folder set to the home directory is disabled.
			if filepath.Clean(path) == filepath.Clean(home) {
				return ""
			}
			return filepath.Clean(path)
		}
	}
	// As GLib's g_get_user_special_dir, only the desktop has a default.
	if dir == core.DesktopDir {
		return filepath.Join(home, "Desktop")
	}
	return ""
}

// xdgUserDir reads key's folder from user-dirs.dirs, whose values are
// quoted absolute paths or paths starting with $HOME; the last line
// setting key wins.
func xdgUserDir(data []byte, key, home string) (path string, ok bool) {
	lines := bufio.NewScanner(bytes.NewReader(data))
	for lines.Scan() {
		line := strings.TrimSpace(lines.Text())
		value, found := strings.CutPrefix(line, key)
		if !found {
			continue
		}
		value, found = strings.CutPrefix(strings.TrimLeft(value, " \t"), "=")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		if len(value) < 2 || value[0] != '"' {
			continue
		}
		value = value[1:]
		end := closingQuote(value)
		if end < 0 {
			continue
		}
		value = unescape(value[:end])
		switch {
		case value == "$HOME":
			path, ok = home, true
		case strings.HasPrefix(value, "$HOME/"):
			path, ok = filepath.Join(home, value[len("$HOME/"):]), true
		case strings.HasPrefix(value, "/"):
			path, ok = value, true
		}
	}
	return path, ok
}

// closingQuote is the index of s's first unescaped double quote, -1 if none.
func closingQuote(s string) int {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return -1
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
