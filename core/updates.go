package core

import (
	"context"
	"errors"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// UpdateCheck is what the platform's updater answers CheckForUpdates
// with.
type UpdateCheck struct {
	// Shown says the platform's own updater showed what it found in its
	// windows, as Sparkle on macOS and WinSparkle on Windows do: they
	// offer the newer version, install it and restart the app. Version metadata arrives separately through updater.Status; capability is still set.
	Shown bool
	// Managed and Store are updater.CapabilityOf's synchronous capability.
	Managed UpdaterManaged
	Store   string
	// App and Current are the app's name and running version.
	App, Current string
	// Version is the newer version found, "" when Current is the newest,
	// and URL the page that gets it: the App Store's on iOS, the
	// release's elsewhere.
	Version, URL string
	// Notes are Version's release notes as plain text, "" when its feed has none.
	Notes string
	// Download is the signed package for Version, when this target can
	// install it itself. Signature and SHA256 authenticate its bytes; Key is
	// the public Ed25519 key from the app's update configuration.
	Download, Signature, SHA256, Key string
	Size                             int64
	// Err says why the updater could not check.
	Err error
}

// UpdateDownloadProgress reports bytes received by DownloadUpdate.
type UpdateDownloadProgress struct{ Loaded, Total int64 }

// DownloadUpdate stages an update through the platform's HTTP client.
func DownloadUpdate(ctx context.Context, update UpdateCheck, dir string, progress func(UpdateDownloadProgress)) (string, error) {
	if driver.UpdateDownload == nil {
		return "", errors.New("core: update download is unavailable")
	}
	d := driver.Update{App: update.App, Current: update.Current, Version: update.Version, URL: update.URL, Notes: update.Notes,
		Download: update.Download, Signature: update.Signature, SHA256: update.SHA256, Key: update.Key, Size: update.Size}
	path, err := driver.UpdateDownload(ctx, d, dir, func(p driver.UpdateDownloadProgress) {
		if progress != nil {
			progress(UpdateDownloadProgress(p))
		}
	})
	return path, err
}

// InstallUpdate is the core's part of updater.Install. It hands a verified
// package to the platform's installer and reports completion or handoff.
func InstallUpdate(cx *Scope, path string, done func(error)) {
	cx.installPackage(SharedFile{Path: path}, func(installed bool, err error) {
		if err == nil && !installed {
			err = errors.New("core: update installer did not start")
		}
		if done != nil {
			done(err)
		}
	})
}

// updateDownloadTimeout bounds InstallUpdateInBackground's download, which a stalled connection would otherwise hold forever.
const updateDownloadTimeout = 10 * time.Minute

// InstallUpdateInBackground is the core's part of the Update action of package updater's alert on Linux and Android: off the graph's goroutine it downloads and verifies update's package into dir (DownloadUpdate), within updateDownloadTimeout, then hands it to the platform's installer (InstallUpdate). The work belongs to the app's root scope rather than cx, so it finishes though the scope that offered the update is disposed meanwhile; done runs on the graph's goroutine under the root scope, which it is given, with the package's path and what failed, nil once Linux's package manager installed it or Android's installer started.
func InstallUpdateInBackground(cx *Scope, update UpdateCheck, dir string, done func(app *Scope, path string, err error)) {
	app := cx.rt.root
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), updateDownloadTimeout)
		path, err := DownloadUpdate(ctx, update, dir, nil)
		cancel()
		app.Post(func() {
			if err != nil {
				done(app, path, err)
				return
			}
			InstallUpdate(app, path, func(err error) { done(app, path, err) })
		})
	}()
}

// CheckForUpdates is the core's part of updater.Check and
// updater.CheckInBackground, which apps use: it asks the platform's
// updater for a newer version of the app, quietly when background, and
// done hears its answer. A platform with no updater, such as the web,
// never answers.
func (cx *Scope) CheckForUpdates(background bool, done func(UpdateCheck)) {
	t := cx.tree()
	if t == nil {
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemCheckForUpdates, On: background}, func(ev driver.Event) {
		c := UpdateCheck{Shown: ev.Finished}
		if u, ok := ev.Data.(*driver.Update); ok && u != nil {
			c.Managed, c.Store = UpdaterManaged(u.Managed), u.Store
			c.App, c.Current, c.Version, c.URL, c.Notes = u.App, u.Current, u.Version, u.URL, u.Notes
			c.Download, c.Signature, c.SHA256, c.Key, c.Size = u.Download, u.Signature, u.SHA256, u.Key, u.Size
			if u.Err != "" {
				c.Err = errors.New(u.Err)
			}
		}
		done(c)
	})
}
