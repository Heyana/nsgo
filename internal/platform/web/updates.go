//go:build js && wasm

package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A release compares its immutable file descriptor with the current nsgo.json. Reloading fetches the new index without forcing a waiting offline worker to activate underneath other live pages of the preceding release.
type webUpdates struct {
	app           string
	current       []byte
	version       string
	url           string
	ready, closed bool
	blocked       bool
	abort         js.Value
	last          time.Time
	pending       []uint32
	checking      bool
}

func webRelease(text string) (canonical []byte, version string, err error) {
	var config map[string]any
	if err = json.Unmarshal([]byte(text), &config); err != nil {
		return nil, "", err
	}
	wasm, _ := config["wasm"].(string)
	if config["release"] != true || wasm == "" {
		return nil, "", fmt.Errorf("not a published NSGo release descriptor")
	}
	version, _ = config["version"].(string)
	canonical, err = json.Marshal(config)
	return
}

func (s *surface) initUpdates(opts Options) driver.UpdaterState {
	if opts.Mount != "" || opts.Host != "" {
		return driver.UpdaterState{NativePreferences: true}
	}
	data, version, err := webRelease(opts.UpdateConfig)
	if err != nil {
		return driver.UpdaterState{NativePreferences: true}
	}
	base := resourceBase
	if base == "" {
		base = s.doc.Get("baseURI").String()
	}
	s.updates = webUpdates{app: opts.Name, current: data, version: version, url: js.Global().Get("URL").New("nsgo.json", base).Get("href").String()}
	var policy struct {
		Channel string `json:"updateChannel"`
	}
	_ = json.Unmarshal(data, &policy)
	s.updates.blocked = policy.Channel == "none" || policy.Channel == "steam"
	return s.updateState()
}

func (s *surface) updateState() driver.UpdaterState {
	managed := driver.UpdateManagedUnavailable
	if s.updates.current != nil {
		managed = driver.UpdateManagedSelf
	}
	if s.updates.blocked {
		managed = driver.UpdateManagedChannel
	}
	return driver.UpdaterState{Managed: managed, NativePreferences: true, LastCheck: s.updates.last}
}

func (s *surface) checkForUpdates(it *driver.MountItem) {
	if s.updates.closed {
		return
	}
	if it.UpdatePreference != nil {
		state := s.updateState()
		s.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Data: &state})
		return
	}
	id := it.Count
	u := driver.Update{App: s.updates.app, Current: s.updates.version, Managed: s.updateState().Managed}
	if s.updates.blocked {
		s.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Count: id, Data: &u})
		return
	}
	if s.updates.current == nil {
		u.Err = "updates: this page has no standalone release descriptor"
		s.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Count: id, Data: &u})
		return
	}
	s.updates.pending = append(s.updates.pending, id)
	if s.updates.checking {
		return
	}
	s.updates.checking = true
	go func() {
		controller := js.Global().Get("AbortController").New()
		s.updates.abort = controller
		timer := time.AfterFunc(30*time.Second, func() { controller.Call("abort") })
		defer timer.Stop()
		finish := func(err error) {
			s.updates.checking = false
			ids := s.updates.pending
			s.updates.pending = nil
			s.updates.abort = js.Undefined()
			if s.updates.closed {
				return
			}
			if err != nil {
				u.Err = "updates: " + err.Error()
			}
			s.updates.last = time.Now()
			state := s.updateState()
			state.Available = u.Version
			s.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Data: &state})
			for _, id := range ids {
				s.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Count: id, Data: &u})
			}
		}
		defer func() {
			if r := recover(); r != nil {
				finish(fmt.Errorf("release check failed: %v", r))
			}
		}()
		response, err := await(js.Global().Call("fetch", s.updates.url, map[string]any{"cache": "no-store", "signal": controller.Get("signal")}))
		if err != nil {
			finish(err)
			return
		}
		if !response.Get("ok").Truthy() {
			finish(fmt.Errorf("release descriptor returned HTTP %d", response.Get("status").Int()))
			return
		}
		body, err := await(response.Call("text"))
		if err != nil {
			finish(err)
			return
		}
		text := body.String()
		if len(text) > 1<<20 {
			finish(fmt.Errorf("release descriptor is too large"))
			return
		}
		latest, version, err := webRelease(text)
		if err != nil {
			finish(err)
			return
		}
		s.updates.ready = !bytes.Equal(latest, s.updates.current)
		if s.updates.ready {
			u.Restart, u.Version = true, version
		}
		finish(nil)
	}()
}

func (s *surface) restartUpdatedPage() {
	if s.updates.ready && !s.updates.closed {
		reloadWebPage()
	}
}

var reloadWebPage = func() { js.Global().Get("location").Call("reload") }

func (s *surface) closeUpdates() {
	s.updates.closed = true
	if s.updates.abort.Truthy() {
		s.updates.abort.Call("abort")
	}
}
