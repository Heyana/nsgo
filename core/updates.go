package core

import (
	"context"
	"errors"
	"sync"
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

// UpdatePhase is what an in-app update is doing on Linux and Android
// (package updater).
type UpdatePhase uint8

const (
	// UpdateIdle says no update is downloading or installing.
	UpdateIdle UpdatePhase = iota
	// UpdateDownloading says an update package is streaming to disk.
	UpdateDownloading
	// UpdateInstalling says the verified package has been handed to the
	// platform installer.
	UpdateInstalling
)

// UpdateState is the live state of package updater's download and install on
// Linux and Android.
type UpdateState struct {
	Phase   UpdatePhase
	Version string
	Loaded  int64
	Total   int64
}

// Fraction is the download progress from 0 to 1, or 0 when Total is unknown or
// no bytes have arrived yet.
func (s UpdateState) Fraction() float64 {
	if s.Total <= 0 || s.Loaded <= 0 {
		return 0
	}
	if s.Loaded >= s.Total {
		return 1
	}
	return float64(s.Loaded) / float64(s.Total)
}

// UpdateState is the core's part of updater.Status: it reads the reactive
// state of an in-progress update download or installation, subscribing the
// calling view or effect to progress changes.
func (cx *Scope) UpdateState() UpdateState {
	rt := cx.rt
	if rt.updateState == nil {
		rt.updateState = NewSignal(rt.root, UpdateState{})
	}
	return rt.updateState.Get()
}

// UpdateActive reports whether an in-app update download or install is
// currently running for this app (package updater), without subscribing.
func (cx *Scope) UpdateActive() bool {
	return cx.rt.updateActive.Load()
}

func (cx *Scope) setUpdateState(s UpdateState) {
	rt := cx.rt
	if rt.updateState == nil {
		rt.updateState = NewSignal(rt.root, s)
		return
	}
	rt.updateState.Set(s)
}

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

// InstallUpdateInBackground is the core's part of the Update action of package updater's alert on Linux and Android: off the graph's goroutine it streams and verifies update's package into dir (DownloadUpdate), within updateDownloadTimeout, reporting download progress to UpdateState and SetTaskProgress, then hands it to the platform's installer (InstallUpdate). The work belongs to the app's root scope rather than cx, so it finishes though the scope that offered the update is disposed meanwhile; done runs on the graph's goroutine under the root scope, which it is given, with the package's path and what failed, nil once Linux's installer finished or Android's installer started.
func InstallUpdateInBackground(cx *Scope, update UpdateCheck, dir string, done func(app *Scope, path string, err error)) {
	app := cx.rt.root
	app.rt.updateActive.Store(true)
	initialTotal := max(update.Size, 0)
	app.setUpdateState(UpdateState{Phase: UpdateDownloading, Version: update.Version, Total: initialTotal})
	app.SetTaskProgress(TaskProgress{State: TaskProgressIndeterminate})

	go func() {
		var (
			mu       sync.Mutex
			latest   UpdateDownloadProgress
			posted   bool
			finished bool
		)
		onProgress := func(p UpdateDownloadProgress) {
			mu.Lock()
			if finished {
				mu.Unlock()
				return
			}
			latest = p
			if posted {
				mu.Unlock()
				return
			}
			posted = true
			mu.Unlock()
			app.Post(func() {
				mu.Lock()
				doneNow := finished
				cur := latest
				posted = false
				mu.Unlock()
				if doneNow {
					return
				}
				loaded := max(cur.Loaded, 0)
				total := cur.Total
				if total <= 0 {
					total = initialTotal
				}
				state := UpdateState{Phase: UpdateDownloading, Version: update.Version, Loaded: loaded, Total: max(total, 0)}
				app.setUpdateState(state)
				if state.Total > 0 {
					app.SetTaskProgress(TaskProgress{State: TaskProgressNormal, Value: state.Fraction()})
				} else {
					app.SetTaskProgress(TaskProgress{State: TaskProgressIndeterminate})
				}
			})
		}

		ctx, cancel := context.WithTimeout(context.Background(), updateDownloadTimeout)
		path, err := DownloadUpdate(ctx, update, dir, onProgress)
		cancel()

		mu.Lock()
		finished = true
		cur := latest
		mu.Unlock()

		app.Post(func() {
			if err != nil {
				app.rt.updateActive.Store(false)
				app.setUpdateState(UpdateState{})
				app.SetTaskProgress(TaskProgress{State: TaskProgressNone})
				done(app, path, err)
				return
			}
			total := max(cur.Total, initialTotal)
			loaded := max(cur.Loaded, total)
			app.setUpdateState(UpdateState{Phase: UpdateInstalling, Version: update.Version, Loaded: loaded, Total: total})
			app.SetTaskProgress(TaskProgress{State: TaskProgressIndeterminate})
			InstallUpdate(app, path, func(err error) {
				app.rt.updateActive.Store(false)
				app.setUpdateState(UpdateState{})
				app.SetTaskProgress(TaskProgress{State: TaskProgressNone})
				done(app, path, err)
			})
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
