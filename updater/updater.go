//go:build !nsgo_steam

// Package updater keeps an app up to date the way its platform does. On
// direct macOS and Windows builds, Sparkle and WinSparkle, which nsgo build bundles when
// nsgo.yaml configures updates, read the app's appcast on their own
// schedule, download a newer release, verify its EdDSA signature,
// install it and restart the app. Linux and direct Android builds read the
// same appcast and install its verified package after confirmation. Apple
// App Store builds offer their store page; Google Play handles a manual
// check with its native update flow.
package updater

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/updater/locales"
)

// Check checks for a newer version now, as a Check for Updates… item
// does, and shows what it finds: on direct macOS and Windows in Sparkle's and
// WinSparkle's windows, on Linux and direct Android an alert that installs the
// signed package after confirmation, on Google Play its native update flow,
// elsewhere an alert that offers to open
// the newer version's page, says the app is up to date, or says the check
// failed.
func Check(cx *core.Scope) { check(cx, false, nil) }

// CheckInBackground checks without showing anything unless it finds a
// newer version, as the platforms' scheduled checks do.
func CheckInBackground(cx *core.Scope) { check(cx, true, nil) }

// CheckDaily's schedule: interval is how long it leaves between its
// checks, a day, as Sparkle and WinSparkle leave between theirs, and
// retry how long after a check that failed.
const (
	interval = 24 * time.Hour
	retry    = time.Hour
)

// CheckDaily keeps the time of its last check, in seconds since 1970, in
// a partition of the app's storage, which the app's own Storage neither
// lists nor clears.

// CheckDaily checks in the background, as CheckInBackground does, unless
// it has within the last day, so a newer version is offered once a day
// rather than at every start. Sparkle and WinSparkle keep such a schedule
// of their own; on Linux, direct Android and Apple App Store builds nsgo.Run calls CheckDaily as
// the app starts and as the user comes back to it when nsgo.yaml's
// updates.automatic is on. A check that failed, as one offline does, is
// tried again an hour later.
func CheckDaily(cx *core.Scope) {
	if !Status(cx).AutomaticChecks {
		return
	}
	now := cx.WallTime()
	var kept core.Storage
	due := true
	// The partition is a scope's of its own, so the app's storage stays
	// whole, and is read untracked.
	part := cx.Child()
	part.Run(func() {
		part.ProvideStorage(schedulePartition, 0)
		kept = part.Storage()
		if text, ok := kept.GetItem(checkedKey); ok {
			if sec, err := strconv.ParseInt(text, 10, 64); err == nil {
				// A time ahead of now is a clock set back since: check.
				since := now.Sub(time.Unix(sec, 0))
				due = since < 0 || since >= interval
			}
		}
	})
	part.Dispose()
	if !due {
		return
	}
	keep := func(checked time.Time) {
		if err := kept.SetItem(checkedKey, strconv.FormatInt(checked.Unix(), 10)); err != nil {
			log.Printf("updater: %v", err)
		}
	}
	// Kept before the answer, so a call meanwhile does not check too.
	keep(now)
	if err := kept.SetItem(lastCheckKey, strconv.FormatInt(now.Unix(), 10)); err != nil {
		log.Printf("updater: %v", err)
	}
	check(cx, true, func(c core.UpdateCheck) {
		if c.Err != nil {
			// Due again after retry rather than a day.
			keep(now.Add(retry - interval))
		}
	})
}

// check asks the platform's updater and shows what it answers, which
// answered, if any, hears first.
func check(cx *core.Scope, background bool, answered func(core.UpdateCheck)) {
	cx.CheckForUpdates(background, func(c core.UpdateCheck) {
		if answered != nil {
			answered(c)
		}
		if c.Shown {
			return
		}
		if s := core.UpdaterStatus(cx); !s.NativePreferences && (s.Managed == ManagedSelf || s.Managed == ManagedPage) {
			if err := scheduleStorage(cx).SetItem(lastCheckKey, strconv.FormatInt(cx.WallTime().Unix(), 10)); err != nil {
				log.Printf("updater: %v", err)
			}
		}
		msgs := locales.For(cx)
		ok := []core.AlertAction{{Label: msgs.Ok(), Role: core.AlertDefault}}
		switch {
		case c.Err != nil:
			log.Printf("updater: %v", c.Err)
			if !background {
				cx.ShowAlert(core.AlertConfig{Title: msgs.UpdateError(), Message: msgs.UpdateErrorMessage(),
					Actions: ok, Cancelable: true}, func(int) {})
			}
		case c.Managed == ManagedDevelopment || c.Managed == ManagedChannel || c.Managed == ManagedStore:
			if !background {
				message := msgs.UpdateChannel()
				if c.Managed == ManagedDevelopment {
					message = msgs.UpdateDevelopment()
				}
				if c.Managed == ManagedStore {
					message = msgs.UpdateStore(c.Store)
				}
				cx.ShowAlert(core.AlertConfig{Title: msgs.UpdateError(), Message: message, Actions: ok, Cancelable: true}, func(int) {})
			}
		case c.Version == "" && !c.Restart:
			// The running version is the newest, so a package staged for an earlier update has been installed or is out of date. One still downloading or installing stays.
			if !cx.UpdateActive() {
				discardStaged(cx)
			}
			if !background {
				cx.ShowAlert(core.AlertConfig{Title: msgs.UpToDate(), Message: msgs.UpToDateMessage(c.App, c.Current),
					Actions: ok, Cancelable: true}, func(int) {})
			}
		default:
			offer := msgs.UpdateAvailableMessage(c.App, c.Version, c.Current)
			if c.Restart && (c.Version == "" || c.Version == c.Current) {
				offer = msgs.UpdateReadyMessage(c.App)
			}
			cx.ShowAlert(core.AlertConfig{
				Title:   msgs.UpdateAvailable(c.App),
				Message: updateMessage(offer, c.Notes),
				Actions: []core.AlertAction{{Label: msgs.Update(), Role: core.AlertDefault},
					{Label: msgs.Later(), Role: core.AlertCancel}},
				Cancelable: true,
			}, func(index int) {
				if index == 0 {
					if c.Restart {
						cx.RequestRestart()
					} else if c.Download != "" && installs(cx) {
						downloadAndInstall(cx, c)
					} else {
						cx.OpenURL(c.URL, nil)
					}
				}
			})
		}
	})
}

// maxNotes is how many characters of a release's notes the prompt shows; longer notes end in an ellipsis, as the release's page has them whole.
const maxNotes = 1500

// updateMessage is the prompt's message: the line offering the version, then its notes, if any, cut to maxNotes characters.
func updateMessage(offer, notes string) string {
	notes = strings.TrimSpace(notes)
	if notes == "" {
		return offer
	}
	if r := []rune(notes); len(r) > maxNotes {
		notes = strings.TrimSpace(string(r[:maxNotes])) + "…"
	}
	return offer + "\n\n" + notes
}

// stagingDir is the directory under the app's cache that holds the package downloadAndInstall stages, which only it writes.
const stagingDir = "nsgo.updater"

// staging is where downloadAndInstall stages packages: under the app's cache, else the system's temporary directory.
func staging(cx *core.Scope) string {
	dir := cx.CacheDir()
	if dir == "" {
		dir = os.TempDir()
	}
	return filepath.Join(dir, stagingDir)
}

// discardStaged removes the packages downloadAndInstall staged on Linux and Android. Android's installer reads one through a content URI after it starts, so a package stays until the next download or a check that finds the app up to date.
func discardStaged(cx *core.Scope) {
	if !installs(cx) {
		return
	}
	if err := os.RemoveAll(staging(cx)); err != nil {
		log.Printf("updater: %v", err)
	}
}

// installs reports whether the app downloads and installs packages itself: on Linux and Android, not a web build that runs as them (core.Env.Simulated).
func installs(cx *core.Scope) bool {
	sys := cx.Env().OS()
	return (sys == core.OSLinux || sys == core.OSAndroid) && !cx.Env().Simulated()
}

// downloadAndInstall downloads, verifies and installs c's package off the app's graph and main threads, after the user chose Update, which is their approval; package managers still ask for their own elevation or install permission. The work outlives the scope that offered the update. Linux restarts into the new version once its installer finished; a failure is logged and offers the release's page, where the user can get the package themselves. Update chosen again while one runs is ignored.
func downloadAndInstall(cx *core.Scope, c core.UpdateCheck) {
	if cx.UpdateActive() {
		log.Printf("updater: an update is already downloading or installing")
		return
	}
	dir := staging(cx)
	discardStaged(cx)
	core.InstallUpdateInBackground(cx, c, dir, func(app *core.Scope, path string, err error) {
		if err != nil {
			log.Printf("updater: %v", err)
			msgs := locales.For(app)
			app.ShowAlert(core.AlertConfig{
				Title:   msgs.InstallError(),
				Message: msgs.InstallErrorMessage(c.App, c.Version),
				Actions: []core.AlertAction{{Label: msgs.OpenPage(), Role: core.AlertDefault},
					{Label: msgs.Later(), Role: core.AlertCancel}},
				Cancelable: true,
			}, func(index int) {
				if index == 0 {
					app.OpenURL(c.URL, nil)
				}
			})
			return
		}
		if app.Env().OS() == core.OSLinux {
			// The package manager or AppImage replacement installed it into place.
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				log.Printf("updater: %v", err)
			}
			app.RequestRestart()
		}
	})
}
