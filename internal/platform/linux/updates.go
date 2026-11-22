//go:build linux && !android && !nsgo_steam

package linux

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/appimage"
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

// installUpdate installs a verified Debian or Arch package through pkexec, or
// atomically replaces the running AppImage with the verified download.
func installUpdate(path string, done func(error)) {
	go func() {
		if updates.LinuxPackageFormat(path) == updates.LinuxPackageAppImage {
			exe, err := os.Executable()
			if err != nil {
				done(err)
				return
			}
			done(updates.ReplaceAppImage(path, appimage.Path(exe)))
			return
		}
		args, err := updates.LinuxInstallCommand(path)
		if err != nil {
			done(err)
			return
		}
		done(exec.Command(args[0], args[1:]...).Run())
	}()
}

// selfUpdatable reports whether download, a package an appcast offers, can
// replace the running app: an AppImage when running from one, or a Debian or
// Arch package whose package manager owns the app's executable.
func selfUpdatable(ctx context.Context, download string) bool {
	exe, err := os.Executable()
	if err != nil {
		return false
	}
	return updates.LinuxSelfUpdatable(ctx, exe, appimage.Path(exe), download)
}

// configureUpdates probes, once at launch and only in builds with a self-updater, whether a package can replace the app: updates.LinuxConfig asks dpkg, and an Arch package pacman owns or the AppImage the app runs from replaces it as well.
func configureUpdates(c updates.Config) updates.Config {
	c = updates.LinuxConfig(c)
	if c.PageOnly && c.Checks(false) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		c.PageOnly = !selfUpdatable(ctx, "update.pkg.tar.gz") && !selfUpdatable(ctx, "update.AppImage")
	}
	return c
}

// checkForUpdates reads the appcast off the main thread and selects the signed package matching this installation, or falls back to the release's page.
func (w *window) checkForUpdates(id uint32) {
	c := updateConfig
	updates.Answer(id, func(ctx context.Context) driver.Update {
		return updates.FindMatching(ctx, c, func(it updates.Item) bool {
			return it.Download != "" && selfUpdatable(ctx, it.Download)
		})
	}, w.engine.Dispatch)
}
