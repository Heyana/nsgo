//go:build linux && !android && !nsgo_steam

package linux

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

func (w *window) installPackage(id uint32, path string) {
	dispatch := w.engine.Dispatch
	installUpdate(path, func(err error) {
		ev := driver.Event{Kind: driver.EventFiles, Count: id, Finished: err == nil}
		if err != nil {
			ev.Text = err.Error()
		}
		dispatch(ev)
	})
}

func installUpdate(path string, done func(error)) {
	go func() {
		var cmd *exec.Cmd
		switch {
		case strings.HasSuffix(path, ".deb"):
			cmd = exec.Command("pkexec", "dpkg", "--install", path)
		case strings.HasSuffix(path, ".pkg.tar.gz"):
			cmd = exec.Command("pkexec", "pacman", "--upgrade", "--noconfirm", path)
		default:
			done(fmt.Errorf("updater: unsupported Linux package %q", filepath.Base(path)))
			return
		}
		err := cmd.Run()
		done(err)
	}()
}

// dpkgPattern escapes what dpkg-query --search reads as wildcards in a path.
var dpkgPattern = strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`)

// selfUpdatable reports whether download, the package an appcast offers, can replace the running app: the package manager of its format, dpkg for a Debian package and pacman for an Arch one, owns the app's executable. An AppImage, a package of the other format or a copy outside any package gets the release's page instead, as no package manager would replace it.
func selfUpdatable(ctx context.Context, download string) bool {
	u, err := url.Parse(download)
	if err != nil {
		return false
	}
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	var cmd *exec.Cmd
	switch name := path.Base(u.Path); {
	case strings.HasSuffix(name, ".deb"):
		cmd = exec.CommandContext(ctx, "dpkg-query", "--search", dpkgPattern.Replace(exe))
	case strings.HasSuffix(name, ".pkg.tar.gz"):
		cmd = exec.CommandContext(ctx, "pacman", "--query", "--owns", exe)
	default:
		return false
	}
	return cmd.Run() == nil
}

// configureUpdates probes package ownership only in builds with a self-updater.
func configureUpdates(c updates.Config) updates.Config { return updates.LinuxConfig(c) }

// checkForUpdates reads the appcast off the main thread and answers with
// what it found.
func (w *window) checkForUpdates(id uint32) {
	c := updateConfig
	updates.Answer(id, func(ctx context.Context) driver.Update {
		u := updates.Find(ctx, c)
		// Ownership was resolved at launch, before any appcast request.
		if c.PageOnly {
			u.Download = ""
		}
		return u
	}, w.engine.Dispatch)
}
