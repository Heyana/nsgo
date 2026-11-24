package core

import "errors"

// StandardDir names one of the user's standard folders (desktop.StandardDir).
type StandardDir uint8

const (
	DocumentsDir StandardDir = iota + 1
	DownloadsDir
	DesktopDir
	PicturesDir
	MusicDir
	// VideosDir is macOS's Movies.
	VideosDir
)

// ErrUserDirUnavailable means the platform has no such folder of the user's
// that an app reaches by path.
var ErrUserDirUnavailable = errors.New("core: the user's folder is unavailable")

// UserDir returns the absolute path of the user's folder dir, as the system
// names it now; the folder may not exist yet (desktop.UserDir).
func (cx *Scope) UserDir(dir StandardDir) (string, error) {
	if dir < DocumentsDir || dir > VideosDir || cx.rt.userDir == nil {
		return "", ErrUserDirUnavailable
	}
	if path := cx.rt.userDir(dir); path != "" {
		return path, nil
	}
	return "", ErrUserDirUnavailable
}
