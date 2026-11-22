//go:build js && wasm

package miniprogram

import (
	"syscall/js"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func updateSnapshotOf(value js.Value) updateSnapshot {
	if !value.Truthy() {
		return updateSnapshot{}
	}
	s := updateSnapshot{supported: value.Get("supported").Truthy(), checked: value.Get("checked").Truthy(), available: value.Get("available").Truthy(), ready: value.Get("ready").Truthy()}
	if err := value.Get("error"); err.Type() == js.TypeString {
		s.err = err.String()
	}
	if at := value.Get("checkedAt"); at.Type() == js.TypeNumber && at.Float() > 0 {
		s.checkedAt = time.UnixMilli(int64(at.Float()))
	}
	return s
}

func (s *surface) initUpdates(opts Options) driver.UpdaterState {
	value := s.bridge.Get("updates")
	s.updates = newMiniUpdater(opts.Name, opts.Version, updateSnapshotOf(value), func(ev driver.Event) {
		if s.engine != nil {
			s.engine.Dispatch(ev)
		}
	})
	if opts.Development {
		s.updates.blocked = driver.UpdateManagedDevelopment
	} else if opts.UpdateChannel == "none" || opts.UpdateChannel == "steam" {
		s.updates.blocked = driver.UpdateManagedChannel
	}
	if value.Truthy() && value.Get("changed").Type() == js.TypeFunction {
		s.updateChanged = js.FuncOf(func(js.Value, []js.Value) any { s.updates.publish(updateSnapshotOf(value)); return nil })
		s.updateUnsub = value.Call("changed", s.updateChanged)
	}
	s.updateClose = js.FuncOf(func(js.Value, []js.Value) any { s.closeUpdates(); return nil })
	s.bridge.Set("closeUpdates", s.updateClose)
	return s.updates.state()
}

func (s *surface) checkForUpdates(it *driver.MountItem) {
	if it.UpdatePreference != nil {
		state := s.updates.state()
		s.engine.Dispatch(driver.Event{Kind: driver.EventUpdate, Data: &state})
		return
	}
	s.updates.check(it.Count)
}

func (s *surface) applyUpdate() {
	if s.updates.state().Managed != driver.UpdateManagedSelf {
		return
	}
	value := s.bridge.Get("updates")
	if value.Truthy() && updateSnapshotOf(value).ready && value.Get("apply").Type() == js.TypeFunction {
		value.Call("apply")
	}
}

func (s *surface) closeUpdates() {
	s.updates.close()
	if s.updateUnsub.Type() == js.TypeFunction {
		s.updateUnsub.Invoke()
		s.updateUnsub = js.Undefined()
	}
	if s.updateChanged.Value.Type() == js.TypeFunction {
		s.updateChanged.Release()
		s.updateChanged = js.Func{}
	}
	s.bridge.Set("closeUpdates", js.Undefined())
	if s.updateClose.Value.Type() == js.TypeFunction {
		s.updateClose.Release()
		s.updateClose = js.Func{}
	}
	if s.engine != nil {
		go s.engine.Close()
	}
}
