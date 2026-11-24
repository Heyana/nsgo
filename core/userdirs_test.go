package core

import (
	"errors"
	"testing"
)

func TestUserDir(t *testing.T) {
	none := NewRoot(RootOptions{})
	defer none.Dispose()
	if path, err := none.UserDir(DownloadsDir); path != "" || !errors.Is(err, ErrUserDirUnavailable) {
		t.Errorf("without folders UserDir = %q, %v; want unavailable", path, err)
	}
	asked := 0
	cx := NewRoot(RootOptions{UserDir: func(d StandardDir) string {
		asked++
		if d == MusicDir {
			return "/home/ada/Music"
		}
		return ""
	}})
	defer cx.Dispose()
	if path, err := cx.UserDir(MusicDir); path != "/home/ada/Music" || err != nil {
		t.Errorf("UserDir(MusicDir) = %q, %v", path, err)
	}
	if _, err := cx.UserDir(VideosDir); !errors.Is(err, ErrUserDirUnavailable) {
		t.Errorf("a folder the platform does not name: %v, want unavailable", err)
	}
	for _, d := range []StandardDir{0, VideosDir + 1} {
		if _, err := cx.UserDir(d); !errors.Is(err, ErrUserDirUnavailable) {
			t.Errorf("UserDir(%d) = %v, want unavailable", d, err)
		}
	}
	if asked != 2 {
		t.Errorf("the platform was asked %d times, want 2: never for an unknown folder", asked)
	}
}
