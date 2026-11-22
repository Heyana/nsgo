//go:build windows

package windows

import (
	"errors"
	"log"
	"sync"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/appstartup"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

const (
	startupTaskClass       = "Windows.ApplicationModel.StartupTask"
	startupGetAsync        = 7
	startupRequestEnable   = 6
	startupDisable         = 7
	startupState           = 8
	startupEnabled         = 2
	startupEnabledByPolicy = 4
)

var (
	iidStartupStatics = parseGUID("ee5b60bd-a148-41a7-b26e-e8b88a1e62f8")
	// AsyncOperationCompletedHandler<StartupTask> and <StartupTaskState>,
	// derived from their WinRT parameterized signatures.
	iidStartupLoaded      = parseGUID("741f7697-2452-5c80-83c6-3b6f822b904c")
	iidStartupEnabled     = parseGUID("70a0bf67-19e8-5a86-a32e-3c9863825a04")
	iidAppInstanceStatics = parseGUID("9d11e77f-9ea6-47af-a6ec-46784c5ba254")
)

type startupRequest struct {
	owner   *window
	id      uint32
	set, on bool
}

// packagedStartup serializes app-wide reads and writes, so a read waits for an
// earlier asynchronous enable. Every read loads the actual StartupTask state.
type packagedStartup struct {
	root     *window
	requests []startupRequest
	busy     bool
	// Only a GetAsync completion owns a native result between threads. close
	// releases one queued after the main loop stopped, and later completions
	// release their own result without posting it.
	mu     sync.Mutex
	closed bool
	result *comObject
}

func (w *window) launchAtLoginItem(it *driver.MountItem) {
	if !packaged() {
		if it.Kind == driver.ItemSetLaunchAtLogin {
			setLaunchAtLogin(it.On, w.opts.ID)
		} else {
			w.engine.Dispatch(driver.Event{Kind: driver.EventLaunchAtLogin, Count: it.Count, Finished: launchesAtLogin(w.opts.ID)})
		}
		return
	}
	root := w.appWindow()
	if root.startup == nil {
		root.startup = &packagedStartup{root: root}
	}
	c := root.startup
	c.requests = append(c.requests, startupRequest{owner: w, id: it.Count, set: it.Kind == driver.ItemSetLaunchAtLogin, on: it.On})
	c.next()
}

func (c *packagedStartup) next() {
	if c.busy || len(c.requests) == 0 {
		return
	}
	c.busy = true
	statics := activationFactory(startupTaskClass, &iidStartupStatics)
	if statics == nil {
		c.finish(false, errors.New("StartupTask is unavailable"))
		return
	}
	defer statics.release()
	id := hstring(appstartup.WindowsTaskID)
	defer deleteHString(id)
	var op *comObject
	hr := statics.call(startupGetAsync, id, uintptr(unsafe.Pointer(&op)))
	if failed(hr) || op == nil {
		c.finish(false, hresultError("StartupTask.GetAsync", hr))
		return
	}
	completed := newHandler(&iidStartupLoaded, func(_ uintptr, status unsafe.Pointer) {
		var task *comObject
		var err error
		if uintptr(status) != asyncStatusCompleted {
			err = errors.New("StartupTask.GetAsync did not complete")
		} else {
			if hr := op.call(asyncGetResults, uintptr(unsafe.Pointer(&task))); failed(hr) || task == nil {
				err = hresultError("StartupTask.GetResults", hr)
			}
		}
		op.release()
		c.mu.Lock()
		if c.closed {
			c.mu.Unlock()
			task.release()
			return
		}
		c.result = task
		c.mu.Unlock()
		c.root.post(func() { c.loaded(err) })
	})
	completed.agile = true
	hr = op.call(asyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) {
		op.release()
		c.finish(false, hresultError("StartupTask.put_Completed", hr))
	}
}

func (c *packagedStartup) loaded(err error) {
	c.mu.Lock()
	task, closed := c.result, c.closed
	c.result = nil
	c.mu.Unlock()
	defer task.release()
	if closed {
		return
	}
	if err != nil || task == nil {
		c.finish(false, err)
		return
	}
	r := c.requests[0]
	var state int32
	if hr := task.call(startupState, uintptr(unsafe.Pointer(&state))); failed(hr) {
		c.finish(false, hresultError("StartupTask.State", hr))
		return
	}
	if !r.set || r.on && startupIsEnabled(state) {
		c.finish(startupIsEnabled(state), nil)
		return
	}
	if !r.on {
		hr := task.call(startupDisable)
		if failed(hr) {
			c.finish(false, hresultError("StartupTask.Disable", hr))
		} else {
			c.finish(false, nil)
		}
		return
	}
	var op *comObject
	hr := task.call(startupRequestEnable, uintptr(unsafe.Pointer(&op)))
	if failed(hr) || op == nil {
		c.finish(false, hresultError("StartupTask.RequestEnableAsync", hr))
		return
	}
	completed := newHandler(&iidStartupEnabled, func(_ uintptr, status unsafe.Pointer) {
		var result int32
		var err error
		if uintptr(status) != asyncStatusCompleted {
			err = errors.New("StartupTask enable did not complete")
		} else {
			if hr := op.call(asyncGetResults, uintptr(unsafe.Pointer(&result))); failed(hr) {
				err = hresultError("StartupTask enable GetResults", hr)
			}
		}
		op.release()
		c.root.post(func() { c.finish(startupIsEnabled(result), err) })
	})
	completed.agile = true
	hr = op.call(asyncPutCompleted, completed.ptr())
	completed.release()
	if failed(hr) {
		op.release()
		c.finish(false, hresultError("StartupTask enable put_Completed", hr))
	}
}

func startupIsEnabled(state int32) bool {
	return state == startupEnabled || state == startupEnabledByPolicy
}

func (c *packagedStartup) finish(enabled bool, err error) {
	if len(c.requests) == 0 {
		return
	}
	r := c.requests[0]
	c.requests = c.requests[1:]
	c.busy = false
	if err != nil {
		log.Printf("nsgo: launch at login: %v", err)
	}
	if r.set && r.on && !enabled && err == nil {
		log.Print("nsgo: launch at login: Windows did not enable the startup task; check Startup apps and policy")
	}
	if !r.set && !r.owner.destroying && r.owner.engine != nil {
		r.owner.engine.Dispatch(driver.Event{Kind: driver.EventLaunchAtLogin, Count: r.id, Finished: enabled})
	}
	c.next()
}

func (c *packagedStartup) close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.closed = true
	task := c.result
	c.result = nil
	c.mu.Unlock()
	task.release()
	c.requests = nil
}

// packagedLoginLaunch reads packaged activation before another consumer can
// take it; StartupTask activations carry no command-line login marker.
func packagedLoginLaunch() bool {
	if !packaged() {
		return false
	}
	statics := activationFactory("Windows.ApplicationModel.AppInstance", &iidAppInstanceStatics)
	if statics == nil {
		return false
	}
	defer statics.release()
	var args *comObject
	if failed(statics.call(7, uintptr(unsafe.Pointer(&args)))) || args == nil {
		return false
	}
	defer args.release()
	var kind int32
	return !failed(args.call(6, uintptr(unsafe.Pointer(&kind)))) && kind == 1020 // ActivationKind.StartupTask
}
