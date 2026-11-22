//go:build windows && !nsgo_steam

package windows

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

// winSparkle is WinSparkle.dll from beside the executable, which nsgo
// build puts there when nsgo.yaml configures updates, once
// startWinSparkle started it. WinSparkle reads the app's appcast on its
// own schedule and shows its windows; it downloads a newer installer,
// checks its EdDSA signature, has the app quit and runs it, and the
// installer starts the new version.
var winSparkle *syscall.DLL

var updateQuitOwner atomic.Pointer[updateQuit]
var winUpdateConfig updates.Config
var winUpdateFailure string
var winUpdateOwner atomic.Pointer[window]
var winUpdateFound, winUpdateNotes string
var winUpdateGeneration uint64

var updateFoundCallback = syscall.NewCallbackCDecl(func() uintptr { notifyWinSparkle(true); return 0 })
var updateNotFoundCallback = syscall.NewCallbackCDecl(func() uintptr { notifyWinSparkle(false); return 0 })
var updateChangedCallback = syscall.NewCallbackCDecl(func() uintptr {
	if w := winUpdateOwner.Load(); w != nil {
		w.post(func() { reportWinSparkle() })
	}
	return 0
})

// WinSparkle's callbacks, made once: it may call them from any thread.
var (
	canShutdown = syscall.NewCallbackCDecl(func() uintptr {
		if owner := updateQuitOwner.Load(); owner != nil && owner.ask() {
			return 1
		}
		return 0
	})
	shutdownRequest = syscall.NewCallbackCDecl(func() uintptr {
		if owner := updateQuitOwner.Load(); owner != nil {
			owner.shutdown()
		}
		return 0
	})
)

// prepareWinSparkle loads the bundled updater and persisted preferences before the first view. startWinSparkle initializes it once the app's language is known.
func prepareWinSparkle(c updates.Config, hwnd uintptr) {
	winUpdateConfig = c
	winUpdateFailure = ""
	winUpdateFound, winUpdateNotes = "", ""
	winUpdateOwner.Store(windows[hwnd])
	if !c.Checks(false) {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		return
	}
	dll, err := syscall.LoadDLL(filepath.Join(filepath.Dir(exe), "WinSparkle.dll"))
	if err != nil {
		winUpdateFailure = "updates: this direct build's WinSparkle.dll could not be loaded: " + err.Error()
		log.Printf("nsgo: %s", winUpdateFailure)
		return
	}
	var keep []any
	cstr := func(s string) uintptr {
		p, _ := syscall.BytePtrFromString(s)
		keep = append(keep, p)
		return uintptr(unsafe.Pointer(p))
	}
	wstr := func(s string) uintptr {
		p := wide(s)
		keep = append(keep, p)
		return uintptr(unsafe.Pointer(p))
	}
	call := func(name string, args ...uintptr) uintptr {
		p, err := dll.FindProc(name)
		if err != nil {
			log.Printf("nsgo: WinSparkle.dll: %v", err)
			return 0
		}
		r, _, _ := p.Call(args...)
		return r
	}
	registry := `Software\` + c.ID + `\WinSparkle`
	call("win_sparkle_set_appcast_url", cstr(c.Feed))
	if call("win_sparkle_set_eddsa_public_key", cstr(c.Key)) == 0 {
		log.Printf("nsgo: WinSparkle refuses updates.key's public key: it installs no update")
	}
	call("win_sparkle_set_app_details", wstr(c.App), wstr(c.App), wstr(c.Version))
	if c.Build != "" && c.Build != c.Version {
		call("win_sparkle_set_app_build_version", wstr(c.Build))
	}
	// Unset, WinSparkle follows the Windows UI language; set to one it lacks, it would say English instead.
	call("win_sparkle_set_registry_path", cstr(registry))
	// nsgo.yaml's choice is the default; the user's, in WinSparkle's
	// settings, stays.
	if !registryValue(registry, "CheckForUpdates") {
		automatic := uintptr(0)
		if c.Automatic {
			automatic = 1
		}
		call("win_sparkle_set_automatic_check_for_updates", automatic)
	}
	call("win_sparkle_set_can_shutdown_callback", canShutdown)
	call("win_sparkle_set_shutdown_request_callback", shutdownRequest)
	call("win_sparkle_set_did_find_update_callback", updateFoundCallback)
	call("win_sparkle_set_did_not_find_update_callback", updateNotFoundCallback)
	call("win_sparkle_set_update_dismissed_callback", updateChangedCallback)
	call("win_sparkle_set_error_callback", updateChangedCallback)
	runtime.KeepAlive(keep)
	winSparkle = dll
}

// stopWinSparkle stops WinSparkle's checks and downloads as the app ends.
func stopWinSparkle() {
	// A callback may be waiting for a deferred app decision. Release it
	// before cleanup joins WinSparkle's worker threads.
	if owner := updateQuitOwner.Swap(nil); owner != nil {
		owner.close()
	}
	if winSparkle == nil {
		return
	}
	if p, err := winSparkle.FindProc("win_sparkle_cleanup"); err == nil {
		p.Call()
	}
	winSparkle = nil
	winUpdateOwner.Store(nil)
}

// registryValue reports whether the current user's key path holds a value
// named name.
func registryValue(path, name string) bool {
	var key syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, wide(path), 0, syscall.KEY_READ, &key) != nil {
		return false
	}
	defer syscall.RegCloseKey(key)
	return syscall.RegQueryValueEx(key, wide(name), nil, nil, nil, nil) == nil
}

// startWinSparkle waits until the mounted app can supply its language.
func startWinSparkle(_ updates.Config, w *window, lang func() string) {
	if winSparkle == nil {
		return
	}
	if name := updates.WinSparkleLanguage(lang()); name != "" {
		if proc, err := winSparkle.FindProc("win_sparkle_set_lang"); err == nil {
			p, _ := syscall.BytePtrFromString(name)
			proc.Call(uintptr(unsafe.Pointer(p)))
			runtime.KeepAlive(p)
		}
	}
	updateQuitOwner.Store(newUpdateQuit(w))
	winUpdateCall("win_sparkle_init")
	reportWinSparkle()
}

func winUpdateCall(name string, args ...uintptr) uintptr {
	if winSparkle != nil {
		if proc, err := winSparkle.FindProc(name); err == nil {
			v, _, _ := proc.Call(args...)
			return v
		}
	}
	return 0
}

func winSparkleStatus() driver.UpdaterState {
	state := winUpdateConfig.State(false)
	state.NativePreferences = true
	if winUpdateFailure != "" {
		state.Managed = driver.UpdateManagedUnavailable
	}
	if winSparkle == nil {
		return state
	}
	state.AutomaticChecks = winUpdateCall("win_sparkle_get_automatic_check_for_updates") != 0
	if seconds := int64(winUpdateCall("win_sparkle_get_last_check_time")); seconds > 0 {
		state.LastCheck = time.Unix(seconds, 0)
	}
	state.Available, state.Notes = winUpdateFound, winUpdateNotes
	return state
}

func reportWinSparkle() {
	state := winSparkleStatus()
	if w := winUpdateOwner.Load(); w != nil && w.engine != nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Data: &state})
	}
}

// WinSparkle's public callbacks have no version payload. A confirmed find reads only status metadata from the configured appcast; WinSparkle still installs it.
func notifyWinSparkle(found bool) {
	w := winUpdateOwner.Load()
	if w == nil {
		return
	}
	w.post(func() {
		winUpdateGeneration++
		generation := winUpdateGeneration
		if !found {
			winUpdateFound, winUpdateNotes = "", ""
			reportWinSparkle()
			return
		}
		config := winUpdateConfig
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			update := updates.Find(ctx, config)
			w.post(func() {
				if winUpdateOwner.Load() != w || generation != winUpdateGeneration {
					return
				}
				if update.Err == "" {
					winUpdateFound, winUpdateNotes = update.Version, update.Notes
				}
				reportWinSparkle()
			})
		}()
	})
}

func (w *window) setUpdatePreference(p *driver.UpdatePreference) {
	if p.AutomaticChecks != nil && winSparkle != nil {
		on := uintptr(0)
		if *p.AutomaticChecks {
			on = 1
		}
		winUpdateCall("win_sparkle_set_automatic_check_for_updates", on)
	}
	reportWinSparkle()
}

func (w *window) checkForUpdates(background bool, id uint32) {
	state := winSparkleStatus()
	update := driver.Update{App: winUpdateConfig.App, Current: winUpdateConfig.Version, Managed: state.Managed, Store: state.Store, Err: winUpdateFailure}
	ev := driver.Event{Kind: driver.EventUpdate, Count: id, Data: &update, Finished: winSparkle != nil}
	if winSparkle != nil {
		name := "win_sparkle_check_update_with_ui"
		if background {
			name = "win_sparkle_check_update_without_ui"
		}
		winUpdateCall(name)
	}
	w.engine.Dispatch(ev)
}
