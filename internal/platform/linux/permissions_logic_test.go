package linux

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestParseFlatpakInfoAndDetectSandbox(t *testing.T) {
	info := parseFlatpakInfo([]byte(`
# Flatpak metadata
[Application]
name=dev.nsgo.gallery
runtime=runtime/org.gnome.Platform/x86_64/51

[Context]
shared=network;ipc;
sockets=wayland;fallback-x11;pulseaudio;
devices=dri;
filesystems=xdg-run/pipewire-0;
`))
	if info.appID != "dev.nsgo.gallery" {
		t.Fatalf("appID = %q, want dev.nsgo.gallery", info.appID)
	}
	if !slices.Equal(info.sockets, []string{"wayland", "fallback-x11", "pulseaudio"}) {
		t.Fatalf("sockets = %v", info.sockets)
	}
	if !slices.Equal(info.devices, []string{"dri"}) {
		t.Fatalf("devices = %v", info.devices)
	}
	if !slices.Equal(info.filesystems, []string{"xdg-run/pipewire-0"}) {
		t.Fatalf("filesystems = %v", info.filesystems)
	}

	// Flatpak takes precedence and uses Application.name.
	sb := detectLinuxSandbox([]byte("[Application]\nname=org.example.App\n"), true, "", "", "fallback.id")
	if sb.kind != linuxSandboxFlatpak || sb.appID != "org.example.App" {
		t.Fatalf("flatpak sandbox = %+v", sb)
	}

	// Flatpak without Application.name falls back to appID.
	sb = detectLinuxSandbox(nil, true, "", "", "fallback.id")
	if sb.kind != linuxSandboxFlatpak || sb.appID != "fallback.id" {
		t.Fatalf("flatpak fallback sandbox = %+v", sb)
	}

	// Snap prefixes SNAP_NAME with "snap.", matching xdg-desktop-portal.
	sb = detectLinuxSandbox(nil, false, "/snap/gallery/42", "gallery", "dev.nsgo.gallery")
	if sb.kind != linuxSandboxSnap || sb.appID != "snap.gallery" {
		t.Fatalf("snap sandbox = %+v", sb)
	}

	// Snap without SNAP_NAME falls back to appID.
	sb = detectLinuxSandbox(nil, false, "/snap/gallery/42", "", "dev.nsgo.gallery")
	if sb.kind != linuxSandboxSnap || sb.appID != "dev.nsgo.gallery" {
		t.Fatalf("snap fallback sandbox = %+v", sb)
	}

	// Unsandboxed Linux.
	sb = detectLinuxSandbox(nil, false, "", "", "dev.nsgo.gallery")
	if sb.kind != linuxSandboxNone {
		t.Fatalf("unsandboxed = %+v", sb)
	}
}

func TestParsePermissionStoreValuesAndErrors(t *testing.T) {
	cases := []struct {
		in   []string
		want permissionStoreResult
	}{
		{nil, permissionStoreNotFound},
		{[]string{}, permissionStoreNotFound},
		{[]string{""}, permissionStoreNotFound},
		{[]string{"yes"}, permissionStoreAllowed},
		{[]string{" YES "}, permissionStoreAllowed},
		{[]string{"no"}, permissionStoreDenied},
		{[]string{"No"}, permissionStoreDenied},
		{[]string{"ask"}, permissionStoreNotFound},
		{[]string{"ASK"}, permissionStoreNotFound},
		{[]string{"custom-unknown"}, permissionStoreUnknown},
	}
	for _, tc := range cases {
		if got := parsePermissionStoreValues(tc.in); got != tc.want {
			t.Errorf("parsePermissionStoreValues(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}

	if !isPortalNotFoundError(errors.New("GDBus.Error:org.freedesktop.portal.Error.NotFound: No entry for devices")) {
		t.Error("expected NotFound error match")
	}
	if !isPortalNotAllowedError(errors.New("GDBus.Error:org.freedesktop.portal.Error.NotAllowed: Permission denied")) {
		t.Error("expected NotAllowed error match")
	}
	if !isPortalNotAllowedError(errors.New("GDBus.Error:org.freedesktop.DBus.Error.AccessDenied: blocked")) {
		t.Error("expected AccessDenied error match")
	}
	if !isPortalUnknownMethodError(errors.New("GDBus.Error:org.freedesktop.DBus.Error.UnknownMethod: GetPermission")) {
		t.Error("expected UnknownMethod error match")
	}
	if got := classifyCameraRemoteError(nil); got != cameraRemoteGranted {
		t.Errorf("nil err = %v, want granted", got)
	}
	if got := classifyCameraRemoteError(errors.New("org.freedesktop.portal.Error.NotAllowed")); got != cameraRemoteNotAllowed {
		t.Errorf("NotAllowed = %v, want not allowed", got)
	}
	if got := classifyCameraRemoteError(errors.New("org.freedesktop.DBus.Error.ServiceUnknown")); got != cameraRemoteUnavailable {
		t.Errorf("ServiceUnknown = %v, want unavailable", got)
	}
	if got := classifyCameraRemoteError(errors.New("org.freedesktop.portal.Error.Failed")); got != cameraRemoteOtherError {
		t.Errorf("Failed = %v, want other error", got)
	}
}

func TestPipeWireSocketPath(t *testing.T) {
	env := func(m map[string]string) func(string) string {
		return func(k string) string { return m[k] }
	}
	if got := pipeWireSocketPath(env(map[string]string{
		"XDG_RUNTIME_DIR": "/run/user/1000",
	})); got != filepath.Join("/run/user/1000", "pipewire-0") {
		t.Fatalf("default path = %q", got)
	}
	if got := pipeWireSocketPath(env(map[string]string{
		"PIPEWIRE_RUNTIME_DIR": "/run/pw",
		"XDG_RUNTIME_DIR":      "/run/user/1000",
		"PIPEWIRE_REMOTE":      "pipewire-custom",
	})); got != filepath.Join("/run/pw", "pipewire-custom") {
		t.Fatalf("custom runtime dir path = %q", got)
	}
	if got := pipeWireSocketPath(env(map[string]string{
		"PIPEWIRE_REMOTE": "/tmp/custom-pw.sock",
	})); got != "/tmp/custom-pw.sock" {
		t.Fatalf("abs remote = %q", got)
	}
	if got := pipeWireSocketPath(env(nil)); got != "" {
		t.Fatalf("empty env = %q, want empty", got)
	}
}

// buildFakePipeWireGlobal encodes a PipeWire Registry::Global event payload.
func buildFakePipeWireGlobal(globalID int32, perms uint32, ifaceType string, props map[string]string) []byte {
	var dictBody []byte
	dictBody = appendSPAInt(dictBody, int32(len(props)))
	for k, v := range props {
		dictBody = appendSPAString(dictBody, k)
		dictBody = appendSPAString(dictBody, v)
	}
	dictPod := encodeSPAStruct(dictBody)

	var body []byte
	body = appendSPAInt(body, globalID)
	body = appendSPAInt(body, int32(perms))
	body = appendSPAString(body, ifaceType)
	body = appendSPAInt(body, 3)
	body = append(body, dictPod...)
	return encodePipeWireMessage(pwRegistryID, pwRegistryEventGlobal, 0, encodeSPAStruct(body))
}

func buildFakePipeWireDone(id, seq int32) []byte {
	var body []byte
	body = appendSPAInt(body, id)
	body = appendSPAInt(body, seq)
	return encodePipeWireMessage(pwCoreID, pwCoreEventDone, 1, encodeSPAStruct(body))
}

func buildFakePipeWireError(id, seq, res int32, msg string) []byte {
	var body []byte
	body = appendSPAInt(body, id)
	body = appendSPAInt(body, seq)
	body = appendSPAInt(body, res)
	body = appendSPAString(body, msg)
	return encodePipeWireMessage(pwCoreID, pwCoreEventError, 1, encodeSPAStruct(body))
}

func runPipeWireProbeServer(t *testing.T, respond func(server net.Conn)) pipewireMicProbe {
	t.Helper()
	client, server := net.Pipe()
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer server.Close()
		// Consume the client's Hello, GetRegistry, and Sync messages.
		for i := range 3 {
			id, op, _, err := readPipeWireMessage(server)
			if err != nil {
				t.Errorf("server read message %d: %v", i, err)
				return
			}
			wantOp := []uint8{pwCoreMethodHello, pwCoreMethodGetRegistry, pwCoreMethodSync}[i]
			if id != pwCoreID || op != wantOp {
				t.Errorf("msg %d = (id=%d, op=%d), want (0, %d)", i, id, op, wantOp)
				return
			}
		}
		respond(server)
	}()
	got := probePipeWireMicrophoneConn(client, time.Second)
	client.Close()
	<-done
	return got
}

func TestProbePipeWireMicrophoneConn(t *testing.T) {
	t.Run("readable audio source grants access", func(t *testing.T) {
		got := runPipeWireProbeServer(t, func(s net.Conn) {
			// Include an unrelated Core::Info message and a speaker sink before the mic.
			_, _ = s.Write(encodePipeWireMessage(pwCoreID, 0, 0, encodeSPAStruct(appendSPAInt(nil, 1))))
			_, _ = s.Write(buildFakePipeWireGlobal(10, pwPermR, "PipeWire:Interface:Node", map[string]string{
				"media.class": "Audio/Sink",
			}))
			_, _ = s.Write(buildFakePipeWireGlobal(11, pwPermR|0o200, "PipeWire:Interface:Node", map[string]string{
				"media.class": "Audio/Source",
			}))
			_, _ = s.Write(buildFakePipeWireDone(0, pwSyncSeq))
		})
		if got != pipewireMicGranted {
			t.Fatalf("got %v, want pipewireMicGranted", got)
		}
	})

	t.Run("virtual source and duplex grant access", func(t *testing.T) {
		for _, class := range []string{"Audio/Source/Virtual", "Audio/Duplex"} {
			got := runPipeWireProbeServer(t, func(s net.Conn) {
				_, _ = s.Write(buildFakePipeWireGlobal(20, pwPermR, "PipeWire:Interface:Node", map[string]string{
					"media.class": class,
				}))
				_, _ = s.Write(buildFakePipeWireDone(0, pwSyncSeq))
			})
			if got != pipewireMicGranted {
				t.Fatalf("class %s: got %v, want pipewireMicGranted", class, got)
			}
		}
	})

	t.Run("audio source without PW_PERM_R is denied", func(t *testing.T) {
		got := runPipeWireProbeServer(t, func(s net.Conn) {
			_, _ = s.Write(buildFakePipeWireGlobal(12, 0, "PipeWire:Interface:Node", map[string]string{
				"media.class": "Audio/Source",
			}))
			_, _ = s.Write(buildFakePipeWireDone(0, pwSyncSeq))
		})
		if got != pipewireMicDenied {
			t.Fatalf("got %v, want pipewireMicDenied", got)
		}
	})

	t.Run("only playback sinks exposed reports no sources", func(t *testing.T) {
		got := runPipeWireProbeServer(t, func(s net.Conn) {
			_, _ = s.Write(buildFakePipeWireGlobal(10, pwPermR, "PipeWire:Interface:Node", map[string]string{
				"media.class": "Audio/Sink",
			}))
			_, _ = s.Write(buildFakePipeWireDone(0, pwSyncSeq))
		})
		if got != pipewireMicNoSources {
			t.Fatalf("got %v, want pipewireMicNoSources", got)
		}
	})

	t.Run("core error EACCES or EPERM is denied", func(t *testing.T) {
		for _, res := range []int32{pwErrAcces, pwErrPerm} {
			got := runPipeWireProbeServer(t, func(s net.Conn) {
				_, _ = s.Write(buildFakePipeWireError(0, 1, res, "permission denied"))
			})
			if got != pipewireMicDenied {
				t.Fatalf("res %d: got %v, want pipewireMicDenied", res, got)
			}
		}
	})

	t.Run("unexpected EOF before done is unavailable", func(t *testing.T) {
		got := runPipeWireProbeServer(t, func(s net.Conn) {
			// Close immediately without Done.
		})
		if got != pipewireMicUnavailable {
			t.Fatalf("got %v, want pipewireMicUnavailable", got)
		}
	})

	t.Run("missing socket path is unavailable", func(t *testing.T) {
		if got := probePipeWireMicrophonePath(filepath.Join(t.TempDir(), "missing.sock"), 200*time.Millisecond); got != pipewireMicUnavailable {
			t.Fatalf("missing socket = %v, want pipewireMicUnavailable", got)
		}
		if got := probePipeWireMicrophonePath("", 200*time.Millisecond); got != pipewireMicUnavailable {
			t.Fatalf("empty socket = %v, want pipewireMicUnavailable", got)
		}
	})

	t.Run("unreadable socket permission error is denied", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("root ignores unix socket permissions")
		}
		dir, err := os.MkdirTemp("", "pw-*")
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.RemoveAll(dir) })
		sockPath := filepath.Join(dir, "s")
		ln, err := net.Listen("unix", sockPath)
		if err != nil {
			t.Skipf("cannot create unix socket in temp dir: %v", err)
		}
		defer ln.Close()
		if err := os.Chmod(sockPath, 0o000); err != nil {
			t.Fatal(err)
		}
		defer os.Chmod(sockPath, 0o600)
		if got := probePipeWireMicrophonePath(sockPath, 200*time.Millisecond); got != pipewireMicDenied {
			t.Fatalf("chmod 000 socket = %v, want pipewireMicDenied", got)
		}
	})
}

// mockLinuxPermissionBackend simulates PermissionStore, Camera portal, Device
// portal, and PipeWire for deterministic cross-platform tests.
type mockLinuxPermissionBackend struct {
	mu sync.Mutex

	store map[string]permissionStoreResult // key: table+"/"+id+"/"+appID

	cameraPresent bool
	cameraOK      bool
	cameraRemote  cameraRemoteProbe
	cameraAskFn   func(done func(code uint32, err error)) (cancel func())

	deviceOK    bool
	deviceAskFn func(device string, done func(code uint32, err error)) (cancel func())

	pipewireMic pipewireMicProbe

	storeCalls      int
	cameraAskCalls  int
	deviceAskCalls  int
	pipewireCalls   int
	remoteCalls     int
	lastDeviceAsked string
}

func (m *mockLinuxPermissionBackend) lookupPermissionStore(table, id, appID string) permissionStoreResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.storeCalls++
	if m.store == nil {
		return permissionStoreUnavailable
	}
	if res, ok := m.store[table+"/"+id+"/"+appID]; ok {
		return res
	}
	return permissionStoreUnavailable
}

func (m *mockLinuxPermissionBackend) cameraPortalAvailable() (bool, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cameraPresent, m.cameraOK
}

func (m *mockLinuxPermissionBackend) cameraOpenPipeWireRemote() cameraRemoteProbe {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.remoteCalls++
	return m.cameraRemote
}

func (m *mockLinuxPermissionBackend) cameraAccess(done func(code uint32, err error)) (cancel func()) {
	m.mu.Lock()
	m.cameraAskCalls++
	fn := m.cameraAskFn
	m.mu.Unlock()
	if fn != nil {
		return fn(done)
	}
	done(portalResponseOther, errors.New("org.freedesktop.DBus.Error.ServiceUnknown"))
	return func() {}
}

func (m *mockLinuxPermissionBackend) devicePortalAvailable() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.deviceOK
}

func (m *mockLinuxPermissionBackend) deviceAccess(device string, done func(code uint32, err error)) (cancel func()) {
	m.mu.Lock()
	m.deviceAskCalls++
	m.lastDeviceAsked = device
	fn := m.deviceAskFn
	m.mu.Unlock()
	if fn != nil {
		return fn(device, done)
	}
	done(portalResponseOther, errors.New("org.freedesktop.DBus.Error.ServiceUnknown"))
	return func() {}
}

func (m *mockLinuxPermissionBackend) probePipeWireMicrophone() pipewireMicProbe {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pipewireCalls++
	return m.pipewireMic
}

func TestUnsandboxedLinuxPermissionsNotGated(t *testing.T) {
	b := &mockLinuxPermissionBackend{}
	ctrl := newLinuxPermissionController(linuxSandbox{kind: linuxSandboxNone, appID: "dev.nsgo.demo"}, b)

	for _, name := range []string{driver.PermissionCamera, driver.PermissionMicrophone, driver.PermissionAccessibility} {
		if got := ctrl.check(name); got != driver.PermissionNotGated {
			t.Fatalf("unsandboxed check(%s) = %d, want NotGated", name, got)
		}
		var events []driver.Event
		ctrl.handle(1, driver.PermissionRequest{Command: driver.PermissionAsk, Name: name}, func(ev driver.Event) {
			events = append(events, ev)
		})
		if len(events) != 1 || events[0].Index != driver.PermissionNotGated {
			t.Fatalf("unsandboxed ask(%s) events = %+v", name, events)
		}
	}

	var openEvents []driver.Event
	ctrl.handle(2, driver.PermissionRequest{Command: driver.PermissionOpenSettings, Name: driver.PermissionCamera}, func(ev driver.Event) {
		openEvents = append(openEvents, ev)
	})
	if len(openEvents) != 1 || openEvents[0].Finished {
		t.Fatalf("open settings = %+v", openEvents)
	}
	ctrl.handle(3, driver.PermissionRequest{Command: driver.PermissionStopGuide}, func(ev driver.Event) {
		t.Fatalf("stop guide unexpectedly dispatched %+v", ev)
	})

	if b.storeCalls != 0 || b.cameraAskCalls != 0 || b.deviceAskCalls != 0 || b.pipewireCalls != 0 || b.remoteCalls != 0 {
		t.Fatalf("unsandboxed Linux touched backend: %+v", b)
	}
}

func TestSandboxedNonMediaPermissionsRemainNotGated(t *testing.T) {
	b := &mockLinuxPermissionBackend{}
	for _, sb := range []linuxSandbox{
		{kind: linuxSandboxFlatpak, appID: "org.example.App"},
		{kind: linuxSandboxSnap, appID: "snap.example"},
	} {
		ctrl := newLinuxPermissionController(sb, b)
		for _, name := range []string{
			driver.PermissionAccessibility,
			driver.PermissionBluetooth,
			driver.PermissionFullDiskAccess,
			driver.PermissionScreenRecording,
		} {
			if got := ctrl.check(name); got != driver.PermissionNotGated {
				t.Fatalf("sandbox %v check(%s) = %d, want NotGated", sb.kind, name, got)
			}
		}
	}
	if b.storeCalls != 0 || b.pipewireCalls != 0 || b.remoteCalls != 0 {
		t.Fatalf("non-media permissions touched backend")
	}
}

func TestSandboxedCameraPermissionsWithPermissionStore(t *testing.T) {
	for _, sb := range []linuxSandbox{
		{kind: linuxSandboxFlatpak, appID: "org.example.Cam"},
		{kind: linuxSandboxSnap, appID: "snap.cam"},
	} {
		t.Run(sb.appID, func(t *testing.T) {
			key := "devices/camera/" + sb.appID
			b := &mockLinuxPermissionBackend{
				store:         map[string]permissionStoreResult{key: permissionStoreNotFound},
				cameraPresent: true,
				cameraOK:      true,
			}
			ctrl := newLinuxPermissionController(sb, b)

			// Not asked yet -> PermissionNotAsked.
			if got := ctrl.check(driver.PermissionCamera); got != driver.PermissionNotAsked {
				t.Fatalf("initial check = %d, want NotAsked", got)
			}

			// User dismisses prompt without saving "no" (response=1, store still NotFound) -> PermissionNotAsked.
			b.cameraAskFn = func(done func(uint32, error)) func() {
				done(portalResponseCancelled, nil)
				return func() {}
			}
			var got int
			ctrl.ask(driver.PermissionCamera, func(s int) { got = s })
			if got != driver.PermissionNotAsked {
				t.Fatalf("dismissed without denial = %d, want NotAsked", got)
			}

			// User denies prompt (response=1, store updated to "no") -> PermissionDenied.
			b.cameraAskFn = func(done func(uint32, error)) func() {
				b.mu.Lock()
				b.store[key] = permissionStoreDenied
				b.mu.Unlock()
				done(portalResponseCancelled, nil)
				return func() {}
			}
			ctrl.ask(driver.PermissionCamera, func(s int) { got = s })
			if got != driver.PermissionDenied {
				t.Fatalf("denied ask = %d, want Denied", got)
			}
			if got := ctrl.check(driver.PermissionCamera); got != driver.PermissionDenied {
				t.Fatalf("check after deny = %d, want Denied", got)
			}

			// User allows prompt (response=0, store updated to "yes") -> PermissionGranted.
			b.cameraAskFn = func(done func(uint32, error)) func() {
				b.mu.Lock()
				b.store[key] = permissionStoreAllowed
				b.mu.Unlock()
				done(portalResponseSuccess, nil)
				return func() {}
			}
			ctrl.ask(driver.PermissionCamera, func(s int) { got = s })
			if got != driver.PermissionGranted {
				t.Fatalf("allowed ask = %d, want Granted", got)
			}
			if got := ctrl.check(driver.PermissionCamera); got != driver.PermissionGranted {
				t.Fatalf("check after allow = %d, want Granted", got)
			}

			// Guide answers immediately with checked status.
			var guideEv driver.Event
			ctrl.handle(9, driver.PermissionRequest{Command: driver.PermissionGuide, Name: driver.PermissionCamera}, func(ev driver.Event) {
				guideEv = ev
			})
			if guideEv.Count != 9 || guideEv.Index != driver.PermissionGranted {
				t.Fatalf("guide event = %+v", guideEv)
			}
		})
	}
}

func TestSandboxedCameraPermissionsWhenStoreBlockedByProxy(t *testing.T) {
	// Simulates a strict Flatpak sandbox where xdg-dbus-proxy blocks
	// org.freedesktop.impl.portal.PermissionStore, so Check uses
	// OpenPipeWireRemote and Ask uses AccessCamera.
	b := &mockLinuxPermissionBackend{
		cameraRemote: cameraRemoteNotAllowed,
	}
	ctrl := newLinuxPermissionController(linuxSandbox{kind: linuxSandboxFlatpak, appID: "org.example.Strict"}, b)

	// Before asking, OpenPipeWireRemote returns NotAllowed -> PermissionNotAsked.
	if got := ctrl.check(driver.PermissionCamera); got != driver.PermissionNotAsked {
		t.Fatalf("initial check = %d, want NotAsked", got)
	}

	// User denies AccessCamera (response=1) -> PermissionDenied, cached for next Check.
	b.cameraAskFn = func(done func(uint32, error)) func() {
		done(portalResponseCancelled, nil)
		return func() {}
	}
	var got int
	ctrl.ask(driver.PermissionCamera, func(s int) { got = s })
	if got != driver.PermissionDenied {
		t.Fatalf("denied ask = %d, want Denied", got)
	}
	if got := ctrl.check(driver.PermissionCamera); got != driver.PermissionDenied {
		t.Fatalf("check after cached denial = %d, want Denied", got)
	}

	// User grants AccessCamera (response=0) and OpenPipeWireRemote now succeeds -> PermissionGranted.
	b.cameraAskFn = func(done func(uint32, error)) func() {
		b.mu.Lock()
		b.cameraRemote = cameraRemoteGranted
		b.mu.Unlock()
		done(portalResponseSuccess, nil)
		return func() {}
	}
	ctrl.ask(driver.PermissionCamera, func(s int) { got = s })
	if got != driver.PermissionGranted {
		t.Fatalf("allowed ask = %d, want Granted", got)
	}
	if got := ctrl.check(driver.PermissionCamera); got != driver.PermissionGranted {
		t.Fatalf("check after grant = %d, want Granted", got)
	}

	// No camera portal at all -> PermissionUnknown.
	b2 := &mockLinuxPermissionBackend{cameraRemote: cameraRemoteUnavailable}
	ctrl2 := newLinuxPermissionController(linuxSandbox{kind: linuxSandboxFlatpak, appID: "org.example.NoPortal"}, b2)
	if got := ctrl2.check(driver.PermissionCamera); got != driver.PermissionUnknown {
		t.Fatalf("no portal check = %d, want Unknown", got)
	}
}

func TestSandboxedMicrophonePermissionsPortalAndPipeWire(t *testing.T) {
	t.Run("permission store and device portal flow", func(t *testing.T) {
		sb := linuxSandbox{kind: linuxSandboxSnap, appID: "snap.voice"}
		key := "devices/microphone/" + sb.appID
		b := &mockLinuxPermissionBackend{
			store:       map[string]permissionStoreResult{key: permissionStoreNotFound},
			pipewireMic: pipewireMicNoSources,
			deviceOK:    true,
		}
		ctrl := newLinuxPermissionController(sb, b)

		// Store NotFound + PipeWire hides sources -> PermissionNotAsked.
		if got := ctrl.check(driver.PermissionMicrophone); got != driver.PermissionNotAsked {
			t.Fatalf("initial mic check = %d, want NotAsked", got)
		}

		// AccessDevice prompt denied (store updated to "no") -> PermissionDenied.
		b.deviceAskFn = func(dev string, done func(uint32, error)) func() {
			if dev != "microphone" {
				t.Errorf("device = %q, want microphone", dev)
			}
			b.mu.Lock()
			b.store[key] = permissionStoreDenied
			b.mu.Unlock()
			done(portalResponseCancelled, nil)
			return func() {}
		}
		var got int
		ctrl.ask(driver.PermissionMicrophone, func(s int) { got = s })
		if got != driver.PermissionDenied {
			t.Fatalf("denied mic ask = %d, want Denied", got)
		}
		if got := ctrl.check(driver.PermissionMicrophone); got != driver.PermissionDenied {
			t.Fatalf("mic check after deny = %d, want Denied", got)
		}

		// AccessDevice prompt granted -> PermissionGranted, and a subsequent ask skips the prompt.
		b.deviceAskFn = func(dev string, done func(uint32, error)) func() {
			b.mu.Lock()
			b.store[key] = permissionStoreAllowed
			b.mu.Unlock()
			done(portalResponseSuccess, nil)
			return func() {}
		}
		ctrl.ask(driver.PermissionMicrophone, func(s int) { got = s })
		if got != driver.PermissionGranted {
			t.Fatalf("allowed mic ask = %d, want Granted", got)
		}
		beforeCalls := b.deviceAskCalls
		ctrl.ask(driver.PermissionMicrophone, func(s int) { got = s })
		if got != driver.PermissionGranted || b.deviceAskCalls != beforeCalls {
			t.Fatalf("repeat mic ask = %d (calls %d -> %d), want Granted without re-prompting", got, beforeCalls, b.deviceAskCalls)
		}
	})

	t.Run("strict flatpak fallback to PipeWire when AccessDevice restricted", func(t *testing.T) {
		sb := linuxSandbox{kind: linuxSandboxFlatpak, appID: "org.example.Audio"}
		b := &mockLinuxPermissionBackend{
			pipewireMic: pipewireMicGranted,
			deviceAskFn: func(dev string, done func(uint32, error)) func() {
				done(portalResponseOther, errors.New("org.freedesktop.portal.Error.NotAllowed: host only"))
				return func() {}
			},
		}
		ctrl := newLinuxPermissionController(sb, b)

		// PipeWire exposes readable Audio/Source -> PermissionGranted for both check and ask.
		if got := ctrl.check(driver.PermissionMicrophone); got != driver.PermissionGranted {
			t.Fatalf("check = %d, want Granted", got)
		}
		var got int
		ctrl.ask(driver.PermissionMicrophone, func(s int) { got = s })
		if got != driver.PermissionGranted {
			t.Fatalf("ask fallback = %d, want Granted", got)
		}

		// PipeWire explicitly denies (EACCES/EPERM or source without PW_PERM_R) -> PermissionDenied.
		b.pipewireMic = pipewireMicDenied
		if got := ctrl.check(driver.PermissionMicrophone); got != driver.PermissionDenied {
			t.Fatalf("denied pipewire check = %d, want Denied", got)
		}
		ctrl.ask(driver.PermissionMicrophone, func(s int) { got = s })
		if got != driver.PermissionDenied {
			t.Fatalf("denied pipewire ask = %d, want Denied", got)
		}

		// Neither PermissionStore nor PipeWire nor Device portal reachable -> PermissionUnknown.
		bNone := &mockLinuxPermissionBackend{pipewireMic: pipewireMicUnavailable, deviceOK: false}
		ctrlNone := newLinuxPermissionController(sb, bNone)
		if got := ctrlNone.check(driver.PermissionMicrophone); got != driver.PermissionUnknown {
			t.Fatalf("unreachable check = %d, want Unknown", got)
		}
		ctrlNone.ask(driver.PermissionMicrophone, func(s int) { got = s })
		if got != driver.PermissionUnknown {
			t.Fatalf("unreachable ask = %d, want Unknown", got)
		}
	})
}

func TestSandboxedPermissionAskCancelSuppressesCallback(t *testing.T) {
	sb := linuxSandbox{kind: linuxSandboxFlatpak, appID: "org.example.Cancel"}
	var savedDone func(uint32, error)
	var remoteCancelled bool
	b := &mockLinuxPermissionBackend{
		cameraAskFn: func(done func(uint32, error)) func() {
			savedDone = done
			return func() { remoteCancelled = true }
		},
	}
	ctrl := newLinuxPermissionController(sb, b)
	called := false
	cancel := ctrl.ask(driver.PermissionCamera, func(int) { called = true })
	cancel()
	if !remoteCancelled {
		t.Fatal("cancel did not cancel underlying portal request")
	}
	if savedDone != nil {
		savedDone(portalResponseSuccess, nil)
	}
	if called {
		t.Fatal("callback ran after cancel")
	}
}
