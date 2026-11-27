package miniprogram

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

type fakePermissionHost struct {
	hasGet, hasAuth, hasOpen bool
	settings                 map[string]any
	getFail                  bool
	authResult               map[string]struct {
		ok     bool
		errMsg string
		mutate *bool
	}
	openOK    bool
	getCalls  []string
	authCalls []string
	openCalls int
}

func (f *fakePermissionHost) hasGetSetting() bool  { return f.hasGet }
func (f *fakePermissionHost) hasAuthorize() bool   { return f.hasAuth }
func (f *fakePermissionHost) hasOpenSetting() bool { return f.hasOpen }

func (f *fakePermissionHost) getSetting(scope string, done func(status int, ok bool)) {
	f.getCalls = append(f.getCalls, scope)
	if f.getFail {
		done(driver.PermissionUnknown, false)
		return
	}
	v, ok := f.settings[scope]
	if !ok {
		done(scopeStatus(false, false, false), true)
		return
	}
	b, isBool := v.(bool)
	done(scopeStatus(true, isBool, b), true)
}

func (f *fakePermissionHost) authorize(scope string, done func(ok bool, errMsg string)) {
	f.authCalls = append(f.authCalls, scope)
	res, ok := f.authResult[scope]
	if !ok {
		done(true, "")
		return
	}
	if res.mutate != nil {
		if f.settings == nil {
			f.settings = map[string]any{}
		}
		f.settings[scope] = *res.mutate
	}
	done(res.ok, res.errMsg)
}

func (f *fakePermissionHost) openSetting(done func(ok bool)) {
	f.openCalls++
	done(f.openOK)
}

func boolPtr(v bool) *bool { return &v }

func TestMiniProgramPermissionScopeMapping(t *testing.T) {
	want := map[string]string{
		driver.PermissionMicrophone: "scope.record",
		driver.PermissionCamera:     "scope.camera",
		driver.PermissionBluetooth:  "scope.bluetooth",
	}
	for name, wantScope := range want {
		scope, ok := permissionScope(name)
		if !ok || scope != wantScope {
			t.Errorf("permissionScope(%q) = (%q, %v), want (%q, true)", name, scope, ok, wantScope)
		}
	}
	for _, unsupported := range []string{
		driver.PermissionAccessibility,
		driver.PermissionScreenRecording,
		driver.PermissionInputMonitoring,
		driver.PermissionFullDiskAccess,
		driver.PermissionContacts,
		driver.PermissionCalendars,
		driver.PermissionReminders,
		driver.PermissionPhotos,
		driver.PermissionMediaLibrary,
		driver.PermissionLocalNetwork,
		driver.PermissionLocation,
		driver.PermissionBackgroundLocation,
		driver.PermissionMotion,
		driver.PermissionActivityRecognition,
		driver.PermissionNotifications,
		driver.PermissionAutomation,
		driver.PermissionAutomation + ":com.apple.finder",
		"unknown",
	} {
		if scope, ok := permissionScope(unsupported); ok || scope != "" {
			t.Errorf("permissionScope(%q) = (%q, %v), want (\"\", false)", unsupported, scope, ok)
		}
	}
}

func TestMiniProgramPermissionStatusAndGuide(t *testing.T) {
	host := &fakePermissionHost{
		hasGet: true, hasAuth: true, hasOpen: true,
		settings: map[string]any{
			"scope.record":    true,
			"scope.camera":    false,
			"scope.bluetooth": "unexpected",
		},
	}
	run := func(cmd driver.PermissionCommand, name string) driver.Event {
		var events []driver.Event
		handlePermission(42, driver.PermissionRequest{Command: cmd, Name: name}, host, func(ev driver.Event) {
			events = append(events, ev)
		})
		if len(events) != 1 {
			t.Fatalf("cmd=%v name=%q dispatched %d events, want 1", cmd, name, len(events))
		}
		return events[0]
	}

	if ev := run(driver.PermissionCheck, driver.PermissionMicrophone); ev.Kind != driver.EventPermission || ev.Count != 42 || ev.Index != driver.PermissionGranted {
		t.Errorf("microphone check = %+v, want granted", ev)
	}
	if ev := run(driver.PermissionCheck, driver.PermissionCamera); ev.Index != driver.PermissionDenied {
		t.Errorf("camera check = %+v, want denied", ev)
	}
	if ev := run(driver.PermissionCheck, driver.PermissionBluetooth); ev.Index != driver.PermissionUnknown {
		t.Errorf("non-boolean bluetooth check = %+v, want unknown", ev)
	}
	delete(host.settings, "scope.bluetooth")
	if ev := run(driver.PermissionCheck, driver.PermissionBluetooth); ev.Index != driver.PermissionNotAsked {
		t.Errorf("absent bluetooth check = %+v, want not-asked", ev)
	}
	if ev := run(driver.PermissionGuide, driver.PermissionMicrophone); ev.Index != driver.PermissionGranted {
		t.Errorf("microphone guide = %+v, want granted", ev)
	}
	if ev := run(driver.PermissionCheck, driver.PermissionAccessibility); ev.Index != driver.PermissionUnavailable {
		t.Errorf("accessibility check = %+v, want unavailable", ev)
	}

	// getSetting failure answers unknown.
	host.getFail = true
	if ev := run(driver.PermissionCheck, driver.PermissionCamera); ev.Index != driver.PermissionUnknown {
		t.Errorf("failed getSetting = %+v, want unknown", ev)
	}

	// Missing getSetting with authorize present answers unknown; missing both answers unavailable.
	host.getFail = false
	host.hasGet = false
	if ev := run(driver.PermissionCheck, driver.PermissionCamera); ev.Index != driver.PermissionUnknown {
		t.Errorf("missing getSetting with authorize = %+v, want unknown", ev)
	}
	host.hasAuth = false
	if ev := run(driver.PermissionCheck, driver.PermissionCamera); ev.Index != driver.PermissionUnavailable {
		t.Errorf("missing both APIs = %+v, want unavailable", ev)
	}

	// PermissionStopGuide dispatches no event.
	var stopEvents []driver.Event
	handlePermission(99, driver.PermissionRequest{Command: driver.PermissionStopGuide}, host, func(ev driver.Event) {
		stopEvents = append(stopEvents, ev)
	})
	if len(stopEvents) != 0 {
		t.Errorf("stop guide dispatched %+v, want none", stopEvents)
	}
}

func TestMiniProgramPermissionAsk(t *testing.T) {
	t.Run("not asked then granted", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: true,
			settings: map[string]any{},
			authResult: map[string]struct {
				ok     bool
				errMsg string
				mutate *bool
			}{
				"scope.record": {ok: true, mutate: boolPtr(true)},
			},
		}
		var got driver.Event
		handlePermission(1, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionMicrophone}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionGranted {
			t.Fatalf("got %+v, want granted", got)
		}
		if !slices.Equal(host.authCalls, []string{"scope.record"}) {
			t.Errorf("authCalls = %v, want [scope.record]", host.authCalls)
		}
	})

	t.Run("already granted skips authorize", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: true,
			settings: map[string]any{"scope.camera": true},
		}
		var got driver.Event
		handlePermission(2, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCamera}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionGranted || len(host.authCalls) != 0 {
			t.Fatalf("got %+v authCalls=%v, want granted without authorize", got, host.authCalls)
		}
	})

	t.Run("already denied skips authorize", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: true,
			settings: map[string]any{"scope.bluetooth": false},
		}
		var got driver.Event
		handlePermission(3, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionBluetooth}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionDenied || len(host.authCalls) != 0 {
			t.Fatalf("got %+v authCalls=%v, want denied without authorize", got, host.authCalls)
		}
	})

	t.Run("authorize auth deny without setting mutation still answers denied", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: true,
			settings: map[string]any{},
			authResult: map[string]struct {
				ok     bool
				errMsg string
				mutate *bool
			}{
				"scope.camera": {ok: false, errMsg: "authorize:fail auth deny"},
			},
		}
		var got driver.Event
		handlePermission(4, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCamera}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionDenied {
			t.Fatalf("got %+v, want denied", got)
		}
	})

	t.Run("authorize deny updates authSetting to false", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: true,
			settings: map[string]any{},
			authResult: map[string]struct {
				ok     bool
				errMsg string
				mutate *bool
			}{
				"scope.record": {ok: false, errMsg: "authorize:fail", mutate: boolPtr(false)},
			},
		}
		var got driver.Event
		handlePermission(5, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionMicrophone}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionDenied {
			t.Fatalf("got %+v, want denied", got)
		}
	})

	t.Run("authorize invalid scope answers unavailable", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: true,
			settings: map[string]any{},
			authResult: map[string]struct {
				ok     bool
				errMsg string
				mutate *bool
			}{
				"scope.bluetooth": {ok: false, errMsg: "authorize:fail invalid scope"},
			},
		}
		var got driver.Event
		handlePermission(6, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionBluetooth}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionUnavailable {
			t.Fatalf("got %+v, want unavailable", got)
		}
	})

	t.Run("authorize transient failure keeps not-asked", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: true,
			settings: map[string]any{},
			authResult: map[string]struct {
				ok     bool
				errMsg string
				mutate *bool
			}{
				"scope.camera": {ok: false, errMsg: "authorize:fail system error"},
			},
		}
		var got driver.Event
		handlePermission(7, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCamera}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionNotAsked {
			t.Fatalf("got %+v, want not-asked", got)
		}
	})

	t.Run("alipay without authorize reports current setting", func(t *testing.T) {
		host := &fakePermissionHost{
			hasGet: true, hasAuth: false, hasOpen: true,
			settings: map[string]any{
				"scope.record": true,
				"scope.camera": false,
			},
		}
		for _, tc := range []struct {
			name string
			want int
		}{
			{driver.PermissionMicrophone, driver.PermissionGranted},
			{driver.PermissionCamera, driver.PermissionDenied},
			{driver.PermissionBluetooth, driver.PermissionNotAsked},
		} {
			var got driver.Event
			handlePermission(8, driver.PermissionRequest{Command: driver.PermissionAsk, Name: tc.name}, host, func(ev driver.Event) { got = ev })
			if got.Index != tc.want {
				t.Errorf("%s: got %d, want %d", tc.name, got.Index, tc.want)
			}
		}
		host.getFail = true
		var got driver.Event
		handlePermission(9, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCamera}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionUnknown {
			t.Errorf("failed getSetting without authorize = %+v, want unknown", got)
		}
	})

	t.Run("missing both APIs answers unavailable", func(t *testing.T) {
		host := &fakePermissionHost{}
		var got driver.Event
		handlePermission(10, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionCamera}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionUnavailable {
			t.Fatalf("got %+v, want unavailable", got)
		}
	})

	t.Run("missing getSetting calls authorize directly", func(t *testing.T) {
		host := &fakePermissionHost{
			hasAuth: true,
			authResult: map[string]struct {
				ok     bool
				errMsg string
				mutate *bool
			}{
				"scope.record":    {ok: true},
				"scope.camera":    {ok: false, errMsg: "authorize:fail auth deny"},
				"scope.bluetooth": {ok: false, errMsg: "authorize:fail system error"},
			},
		}
		for _, tc := range []struct {
			name string
			want int
		}{
			{driver.PermissionMicrophone, driver.PermissionGranted},
			{driver.PermissionCamera, driver.PermissionDenied},
			{driver.PermissionBluetooth, driver.PermissionUnknown},
		} {
			var got driver.Event
			handlePermission(11, driver.PermissionRequest{Command: driver.PermissionAsk, Name: tc.name}, host, func(ev driver.Event) { got = ev })
			if got.Index != tc.want {
				t.Errorf("%s: got %d, want %d", tc.name, got.Index, tc.want)
			}
		}
	})

	t.Run("unsupported permission answers unavailable", func(t *testing.T) {
		host := &fakePermissionHost{hasGet: true, hasAuth: true, hasOpen: true}
		var got driver.Event
		handlePermission(12, driver.PermissionRequest{Command: driver.PermissionAsk, Name: driver.PermissionContacts}, host, func(ev driver.Event) { got = ev })
		if got.Index != driver.PermissionUnavailable || len(host.getCalls) != 0 || len(host.authCalls) != 0 {
			t.Fatalf("got %+v calls=(%v,%v), want unavailable with no host calls", got, host.getCalls, host.authCalls)
		}
	})
}

func TestMiniProgramPermissionOpenSettings(t *testing.T) {
	host := &fakePermissionHost{hasOpen: true, openOK: true}
	var got driver.Event
	handlePermission(20, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionCamera}, host, func(ev driver.Event) { got = ev })
	if got.Kind != driver.EventPermission || got.Count != 20 || !got.Finished || host.openCalls != 1 {
		t.Fatalf("open camera settings = %+v calls=%d, want Finished=true calls=1", got, host.openCalls)
	}

	host.openOK = false
	handlePermission(21, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionBluetooth}, host, func(ev driver.Event) { got = ev })
	if got.Finished || host.openCalls != 2 {
		t.Fatalf("failed open settings = %+v calls=%d, want Finished=false calls=2", got, host.openCalls)
	}

	host.openOK = true
	handlePermission(22, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionAccessibility}, host, func(ev driver.Event) { got = ev })
	if got.Finished || host.openCalls != 2 {
		t.Fatalf("unsupported open settings = %+v calls=%d, want Finished=false without host call", got, host.openCalls)
	}

	host.hasOpen = false
	handlePermission(23, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionMicrophone}, host, func(ev driver.Event) { got = ev })
	if got.Finished || host.openCalls != 2 {
		t.Fatalf("missing openSetting = %+v calls=%d, want Finished=false without host call", got, host.openCalls)
	}
}
