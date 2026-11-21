//go:build android

// Package android is NSGo's Android backend. The app is a plain Go executable that the prebuilt Java runtime in java/ starts as a child process; nsgo packages it as libnsgo.so in an APK. The two talk over the process's pipes: one batch per commit, with separate messages for pixel frames, and one message per event and per step of text. Java keeps the text the engine prepares, measures it and lays it out with StaticLayout, and mounts one Android view per node; text calls have correlated replies and deadlines. Go fetches images and Java decodes them, animated GIF and WebP included; Go rasterizes SVG with imagedecode first, as Android cannot. Java hosts web views, videos and native components' views, plays canvas drawings, runs style transitions, a port of core.Animator and layout animations on Choreographer frames, shows input accessory bars above the keyboard and reports the environment, back, keys, presses outside overlays and answers to alerts and clipboard reads. An overlay over other apps shows in a window of its own, an NsgoView the first view hands its surface's batches, images and events on to (overlay.go).
//
// Go builds Android shared libraries only with cgo and the NDK; a second
// process keeps every build an executable, so arm64 needs only Go. An
// x86_64 emulator needs cgo and the NDK's clang, as Go links android/amd64
// no other way.
package android

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"image"
	"io"
	"log"
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/fontfile"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// Message types, kept in sync with Protocol.java.
const (
	// Go to Java.
	msgBatch   = 1 // a MountBatch, applied on the UI thread
	msgRelease = 5 // frees a prepared text
	msgImage   = 6 // bytes of an image view's source or placeholder
	// msgCanvasImage is the bytes of an image canvases draw, by its key.
	msgCanvasImage = 8
	// msgFonts lists the app's font files, with each face's family,
	// weight and slant, before anything else.
	msgFonts = 9
	// msgTest is a test host's step for the runtime's test agent.
	msgTest = 10
	// msgHostCall is the app's call of an embedding host's handler, a URL
	// for its handler or a failure (host.go).
	msgHostCall = 11
	// msgWantFrame asks for msgFrame at the next Choreographer frame.
	msgWantFrame = 13
	// msgFontData is a font the app registers as it runs, its bytes with
	// its face's family, weight and slant, before the text that uses it.
	msgFontData = 14
	// msgFolder lists a folder of a picked tree or reads part of its file
	// (folders.go).
	msgFolder = 15
	// msgFetch starts or cancels an image's download (fetch.go).
	msgFetch = 16
	// msgLiveUpdate starts, updates or ends one of package widget's Live
	// Activities, JSON (driver.SendLiveUpdate).
	msgLiveUpdate = 17
	// msgRecording starts or stops package microphone's recording or
	// asks for the microphone, JSON (driver.SendRecording).
	msgRecording = 18
	// msgSecret reads, writes or deletes a secret in the runtime's keystore (secrets.go).
	msgSecret = 19
	// msgCookies sets, lists or clears the web views' CookieManager and storage (webcookie.go).
	msgCookies = 20
	// msgScan starts or stops the runtime's code scanner (scan.go).
	msgScan = 21
	// msgPixelFrame carries a bitmap for an item of the next batch on its surface.

	msgPixelFrame = 22
	// msgDiskSpace asks what the app can allocate on the volume containing its data directory.
	msgDiskSpace = 23
	// This slot is inside the prior runtime's accepted range. A stale
	// prebuilt runtime can ignore the optional monitor and keep running.
	msgPerfPanel   = 24
	msgTextRequest = 26
	// msgGamepadRumble starts or stops rumble on a connected game controller (gamepad.go, Gamepads.java).
	msgGamepadRumble = 27

	// Java to Go.
	msgStart       = 1 // the view's size and the folds measured at it (readSize), as msgResize
	msgResize      = 2 // as msgStart, then the environment as msgEnvironment's when it changed with the size
	msgEvent       = 3
	msgEnvironment = 5  // the environment, before msgStart and on change
	msgDNS         = 6  // the default network's DNS servers
	msgTestReply   = 9  // the test agent's answer to a msgTest
	msgHost        = 10 // the embedding host's part of the channel (host.go)
	msgFrame       = 12 // a Choreographer frame began, answering msgWantFrame: its frameTimeNanos, a u64
	msgFolderReply = 13 // a folder's entries or a file's bytes, answering msgFolder
	msgFetched     = 14 // a download's progress or response, answering msgFetch
	// msgLiveReport is what the runtime reports of Live Activities, JSON
	// (driver.ReportLiveUpdate).
	msgLiveReport = 15
	// msgSurface is what an overlay's view reports of its surface: its size, frame and environment, the first of which attaches its tree, or its end (overlay.go).
	msgSurface = 16
	// msgTimeZone is the device's time zone, after the process starts and as it changes (timezone.go).
	msgTimeZone = 17
	// msgRecordingReport is what the runtime's recording service reports, JSON (driver.ReportRecording).
	msgRecordingReport = 18
	// msgSecretReply answers a msgSecret.
	msgSecretReply = 19
	// msgSensorReport is a sensor session's report or coalesced packet (sensors.go).
	msgSensorReport = 20
	// msgCookiesSet answers a msgCookies (webcookie.go).
	msgCookiesSet = 21
	// msgCanvasRequest requests or cancels a canvas image: surface, key, request, cancel.
	msgCanvasRequest = 22
	// msgScanReport is what the code scanner reports: a frame of the camera, its close or its failure (scan.go).
	msgScanReport = 23

	// msgGamepad reports a game controller connecting, disconnecting or changing buttons and axes (gamepad.go).
	msgGamepad = 24
	// msgDiskSpaceReply answers msgDiskSpace, with byte counts or the reason it failed.
	msgDiskSpaceReply = 25
	msgTextReply      = 27
	msgPerfFrame      = 28 // a monitored Choreographer frame and UI work since the previous frame
)

// The parts of an image view's request msgImage carries.
const (
	imagePlaceholder = 0
	imageSource      = 1
)

type surface struct {
	sensorReports sensorReports

	mu      sync.Mutex // guards out and buf
	out     *os.File
	buf     []byte
	ids     atomic.Uint32 // the last prepared text or layout id
	textRPC *textRPC
	engine  *engine.Engine
	started chan struct{}      // closed once engine is set
	env     driver.Environment // the last reported, on the reader goroutine
	// omu guards overlays, the overlays over other apps the app opened, by their surface IDs (overlay.go).
	omu      sync.Mutex
	overlays map[uint32]*overlay
	// frameWanted is set while the app waits for a display frame, from
	// its request until the runtime's answer.
	frameWanted atomic.Bool
	// perf measures the surface when an app or test asks (-fps).
	perf *performance

	ratio atomic.Uint32 // the pixel ratio's float32 bits, for rasterizing SVG

	canvasRequests canvasRequests // mounts and cache misses, on separate goroutines

	// On the engine goroutine.
	// main is the activity's view, the first surface; requests is the last image request number, across surfaces.
	main     treeView
	requests uint32
	// programs are the shader programs translated, numbered from nextProgram.
	programs    map[*driver.ShaderProgram]*glProgram
	nextProgram uint32

	// The native components; cmu guards them, as the reader goroutine
	// hands them their views' events.
	cmu        sync.Mutex
	components driver.NativeComponents
	natives    map[nativeKey]*nativeView

	// staged are the directories of the files saves of data were written to for Java to copy, by save and its surface (answerKey), until it answers; smu guards them.
	smu    sync.Mutex
	staged map[uint64]string
	// folders are the folder picks and reads Java has yet to answer.
	folders folders
	// downloads are the image downloads Java has yet to finish.
	downloads downloads
	// secrets are the secret requests Java has yet to answer.
	secrets secretWaits

	// testReply takes the runtime's answers to a test host's steps.
	testReply func(r *reader)
	// host is the channel to the app embedding the view.
	host *embedHost
}

// Options configures Run.
type Options struct {
	// Components are the native components Native elements name.
	Components driver.NativeComponents
	// Fonts are font files under the app's resources, registered before
	// the first view.
	Fonts []string
	// Appearance is the appearance the app starts in.
	Appearance driver.Appearance
	// Perf shows the performance monitor (--nsgo-fps).
	Perf bool
	// Updates is how the app updates (updates.go).
	Updates updates.Config
}

// Run serves app to the Java runtime over standard input and output until
// the runtime closes them. Call it from main.
func Run(app engine.App, opts Options) {
	updateConfig = opts.Updates
	s := connect(opts)
	s.serve(app, opts, nil)
}

// connect keeps the pipe to the runtime to the surface, and sends the
// app's own prints to standard error, which the runtime logs.
func connect(opts Options) *surface {
	fd, err := syscall.Dup(1)
	if err != nil {
		panic(err)
	}
	if err := syscall.Dup3(2, 1, 0); err != nil {
		panic(err)
	}
	s := &surface{
		out:      os.NewFile(uintptr(fd), "nsgo-runtime"),
		started:  make(chan struct{}),
		env:      driver.Environment{OS: driver.OSAndroid, HasWebView: true, ScheduledNotifications: true, Gamepads: true},
		main:     treeView{images: map[driver.NodeID]*imageView{}},
		overlays: map[uint32]*overlay{},

		components: opts.Components,
		natives:    map[nativeKey]*nativeView{},
		host:       newEmbedHost(),
	}
	s.textRPC = newTextRPC(func(payload []byte, deadline time.Time) error {
		return s.sendBefore(msgTextRequest, deadline, func(w *writer) { w.buf = append(w.buf, payload...) })
	})
	useDNS()
	s.sendFonts(opts.Fonts)
	driver.ConnectLiveUpdates(func(msg []byte) {
		s.send(msgLiveUpdate, func(w *writer) { w.buf = append(w.buf, msg...) })
	})
	driver.ConnectRecordings(func(msg []byte) {
		s.send(msgRecording, func(w *writer) { w.buf = append(w.buf, msg...) })
	})
	driver.ConnectGamepadRumble(func(id int, lowFreq, highFreq float32, duration time.Duration) {
		var ms int32
		if duration > 0 {
			d := duration.Milliseconds()
			if d <= 0 {
				d = 1
			}
			if d > math.MaxInt32 {
				d = math.MaxInt32
			}
			ms = int32(d)
		}
		s.send(msgGamepadRumble, func(w *writer) {
			w.u32(uint32(int32(id)))
			w.float(lowFreq)
			w.float(highFreq)
			w.u32(uint32(ms))
		})
	})
	return s
}

// serve takes the runtime's messages until it closes the pipe. A test
// host's app is the test's: the runtime's size goes to test, and the
// test starts the engine.
func (s *surface) serve(app engine.App, opts Options, test *testOpen) {
	in := bufio.NewReader(os.Stdin)
	for {
		kind, payload, err := readMessage(in)
		if err != nil {
			// The runtime is gone, and the app with it.
			s.textRPC.close()
			s.out.Close()
			if s.engine != nil {
				s.engine.Close()
			}
			s.canvasRequests.close()
			s.closeNatives()
			return
		}
		r := reader{payload}
		switch kind {
		case msgStart:
			size, folds := readSize(&r)
			g := foldGeometry(size, folds)
			if test != nil {
				test.geometry = g
				test.size <- size
				<-s.started
				break
			}
			s.startEngine(app, size, g, opts, nil)
		case msgFrame:
			s.frameWanted.Store(false)
			s.engine.FrameAt(frameTime(&r))
		case msgPerfFrame:
			s.perfFrame(&r)
		case msgResize:
			size, g, env := readResize(&r)
			if env == nil || env.Equal(s.env) {
				s.engine.Resize(size, g)
				break
			}
			// The environment that changed with the size, as a rotation's safe area, goes with it.
			s.env = *env
			s.ratio.Store(math.Float32bits(env.PixelRatio))
			s.engine.ResizeWithEnvironment(size, g, *env)
		case msgEvent:
			from, ev, data := readEvent(&r)
			if ev.Kind == driver.EventNative {
				s.nativeEvent(from, ev.ID, ev.Text, data)
				break
			}
			target := s.target(from)
			if target == nil {
				// An overlay that ended, or never attached.
				break
			}
			if ev.Kind == driver.EventInputModality {
				target.InputModality(ev.Count)
				break
			}
			if ev.Kind == driver.EventTouch {
				ev.Touches, ev.TouchGeometry = readTouchEvent(data)
			}
			switch ev.Kind {
			case driver.EventOpenLinks:
				ev.Data, ev.Text = []string{ev.Text}, ""
			case driver.EventShared:
				var items []driver.SharedItem
				if err := json.Unmarshal([]byte(ev.Text), &items); err == nil {
					ev.Data = items
				} else {
					log.Printf("android share: %v", err)
				}
				ev.Text = ""
			case driver.EventPaste:
				ev.Data, ev.Text = readPaste(ev.Text, data), ""
			case driver.EventClipboard:
				readClipboard(&ev, data)
			case driver.EventPick:
				ev.Data = readPicked(data)
			case driver.EventSpeak:
				ev.Data, ev.Text, ev.Index = readSpeak(ev, data), "", 0
			case driver.EventFiles:
				if s.pickedFolders(answerKey(from, ev.Count)) {
					ev.Data = s.readFolders(data)
				} else if strings.HasPrefix(data, "content://") {
					ev.Data = data
				} else {
					ev.Data = readFiles(data)
				}
				s.unstage(answerKey(from, ev.Count))
			case driver.EventDownload:
				var download driver.WebDownload
				if json.Unmarshal([]byte(data), &download) == nil {
					ev.Data = &download
				}
			case driver.EventMessage:
				// WebNode takes only the top frame's, with the URL of its page.
				ev.Origin, ev.URL, ev.MainFrame = driver.WebOrigin(data), data, true
			}
			if ev.Kind == driver.EventWheel {
				// A wheel's delta comes as the velocity, and whether a
				// touchpad turned it as the value.
				ev.Delta, ev.Velocity, ev.Precise, ev.Value = ev.Velocity, driver.Point{}, ev.Value != 0, 0
			}
			if ev.Kind == driver.EventPan {
				// A pan's start and end carry its bound values as the size.
				ev.Values, ev.Size = driver.Point{X: ev.Size.Width, Y: ev.Size.Height}, driver.Size{}
			}
			if ev.Kind == driver.EventScroll && s.frameWanted.Load() {
				// The runtime reports the offsets the app's commits set as
				// it applies them: while the app waits for a frame, the
				// report rides with it, where it cost a pass of its own
				// every frame of an app scrolling in its frames.
				target.DispatchBeforeFrame(ev)
			} else {
				target.Dispatch(ev)
			}
		case msgTextReply:
			s.receiveText(&r)
		case msgEnvironment:
			env := readEnvironment(&r)
			if env.Equal(s.env) {
				break
			}
			s.env = env
			s.ratio.Store(math.Float32bits(env.PixelRatio))
			if s.engine != nil {
				s.engine.SetEnvironment(env)
			}
		case msgSurface:
			s.surfaceReport(&r)
		case msgFolderReply:
			s.folderReply(&r)
		case msgDiskSpaceReply:
			s.diskSpaceReply(&r)
		case msgSecretReply:
			s.secretReply(&r)
		case msgFetched:
			s.fetched(&r)
		case msgTestReply:
			if s.testReply != nil {
				s.testReply(&r)
			}
		case msgHost:
			s.hostMessage(&r)
		case msgSensorReport:
			s.sensorReports.receive(payload, s.engine.Dispatch)
		case msgLiveReport:
			driver.ReportLiveUpdate(slices.Clone(payload))
		case msgTimeZone:
			// Go has no other way to change the zone: the store races with readers of time.Local, but it is one pointer, which none sees torn.
			time.Local = readTimeZone(&r)
		case msgRecordingReport:
			driver.ReportRecording(slices.Clone(payload))
		case msgCanvasRequest:
			s.canvasRequest(&r)
		case msgCookiesSet:
			s.webCookiesSet(&r)
		case msgScanReport:
			s.scanReport(&r)
		case msgGamepad:
			readGamepad(&r)
		case msgDNS:
			servers := make([]string, r.u32())
			for i := range servers {
				servers[i] = r.str()
			}
			dnsServers.Store(&servers)
		}
	}
}

// init points os.TempDir at the app's cache, as Android gives an app's process no TMPDIR and its default, /data/local/tmp, is the shell's, and time.Local at the zone the runtime passes as TZ, which Go on Android does not read (timezone.go).
func init() {
	if dir := os.Getenv("NSGO_CACHE_DIR"); dir != "" && os.Getenv("TMPDIR") == "" {
		os.Setenv("TMPDIR", dir)
	}
	if tz := os.Getenv("TZ"); tz != "" {
		if loc, err := loadZone(tz); err == nil {
			time.Local = loc
		}
	}
}

// startEngine starts app's engine at size with the fold geometry g measured at it; test, when set, is what a test host opens it with.
func (s *surface) startEngine(app engine.App, size driver.Size, g driver.Geometry, opts Options, test *testOpen) {
	eopts := engine.Options{Size: size, Geometry: g, Environment: s.env, Appearance: opts.Appearance, Host: s.hostOptions(),
		Secrets: keystore{s}}
	// The runtime passes the app's own files and cache directories.
	data, logs := os.Getenv("NSGO_DATA_DIR"), ""
	if data != "" {
		logs = filepath.Join(data, "logs")
	}
	eopts.SetDirs(data, os.Getenv("NSGO_CACHE_DIR"), logs)
	if test != nil {
		eopts.UseTest(test.options, &test.pin)
		// A phone's screen is what it is.
		eopts.Size, eopts.Geometry = size, g
	}
	if opts.Perf || test != nil && test.options.FPS {
		s.perf = &performance{monitor: engine.NewPerfMonitor()}
		eopts.Perf = s.perf.monitor
		if test == nil {
			s.perf.showPanel = true
		}
		s.send(msgPerfPanel, func(w *writer) { w.flags(s.perf.showPanel) })
	}
	imageload.HTTP = s.httpGet
	imageload.UseDiskCache(eopts.CacheDir)
	s.engine = engine.Start(s, app, eopts)
	close(s.started)
}

// testOpen is what a test host opens the app with: the test's options,
// the environment it pins and the runtime's size, once it starts.
type testOpen struct {
	options testhost.Options
	pin     engine.TestPin
	size    chan driver.Size
	// geometry is the fold geometry measured at the runtime's size, set before it is sent.
	geometry driver.Geometry
}

// readClipboard fills in what an EventClipboard carries in its data:
// "image", the PNG's size and the PNG in base64, "toolarge" and its
// bytes or "error" and why for one Java could not give, or "uri" lines of
// a file's file: URL.
func readClipboard(ev *driver.Event, data string) {
	for _, line := range strings.Split(data, "\n") {
		kind, rest, _ := strings.Cut(line, "\t")
		switch kind {
		case "uri":
			ev.URIs = append(ev.URIs, rest)
		case "image":
			f := strings.SplitN(rest, "\t", 3)
			if len(f) != 3 {
				continue
			}
			w, _ := strconv.Atoi(f[0])
			h, _ := strconv.Atoi(f[1])
			b, err := base64.StdEncoding.DecodeString(f[2])
			ev.Data = driver.ReadClipboardImage(b, driver.Size{Width: driver.Points(w), Height: driver.Points(h)}, err)
		case "toolarge":
			n, _ := strconv.Atoi(rest)
			ev.Data = &driver.ClipboardImage{TooLarge: n}
		case "error":
			ev.Data = &driver.ClipboardImage{Err: rest}
		}
	}
}

// readSpeak reads what Speaker.java's EventSpeak reports: its error as the text, whether it stopped as the index, and the voices it lists as JSON in its data.
func readSpeak(ev driver.Event, data string) *driver.SpeakEvent {
	e := &driver.SpeakEvent{Error: ev.Text, Stopped: ev.Index != 0}
	if data != "" {
		json.Unmarshal([]byte(data), &e.Voices)
	}
	return e
}

// readPicked reads the files an EventPick carries in its data, one per
// line: the name, the MIME type, the width, the height, the time it was
// last changed in milliseconds and the bytes in base64.
func readPicked(data string) []driver.PickedFile {
	var files []driver.PickedFile
	for _, line := range strings.Split(data, "\n") {
		f := strings.Split(line, "\t")
		if len(f) != 6 {
			continue
		}
		b, err := base64.StdEncoding.DecodeString(f[5])
		if err != nil {
			continue
		}
		w, _ := strconv.Atoi(f[2])
		h, _ := strconv.Atoi(f[3])
		file := driver.PickedFile{Name: f[0], MIME: f[1], Data: b, Size: driver.Size{Width: driver.Points(w), Height: driver.Points(h)}}
		if ms, _ := strconv.ParseInt(f[4], 10, 64); ms > 0 {
			file.Modified = time.UnixMilli(ms)
		}
		files = append(files, file)
	}
	return files
}

// readFiles reads the files an EventFiles carries in its data, one per
// line: the name, the MIME type and the path of the copy Java made.
func readFiles(data string) []driver.File {
	var files []driver.File
	for _, line := range strings.Split(data, "\n") {
		if f := strings.Split(line, "\t"); len(f) == 3 {
			files = append(files, driver.File{Name: f[0], MIME: f[1], Path: f[2], Temporary: true})
		}
	}
	return files
}

// stage writes the data of the batch's saves and byte-backed shares that
// have no file on disk to the cache, for Java to read. A save that cannot
// be written fails at once, and Java, seeing no path, leaves it.
func (s *surface) stage(id uint32, b *driver.MountBatch) {
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemSaveFile:
			if it.Save.Path != "" {
				continue
			}
			r := *it.Save
			path, err := r.Stage(filepath.Join(os.Getenv("NSGO_CACHE_DIR"), "nsgo-save"))
			if err != nil {
				ev := driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: err.Error()}
				go s.dispatchTo(id, ev)
			} else {
				r.Path = path
				s.smu.Lock()
				if s.staged == nil {
					s.staged = map[uint64]string{}
				}
				s.staged[answerKey(id, it.Count)] = filepath.Dir(path)
				s.smu.Unlock()
			}
			it.Save = &r
		case driver.ItemShare:
			r := *it.Share
			var files []driver.SharedFile
			for j, f := range r.Files {
				if f.Path != "" {
					continue
				}
				if files == nil {
					files = append([]driver.SharedFile(nil), r.Files...)
				}
				path, err := f.Stage(filepath.Join(os.Getenv("NSGO_CACHE_DIR"), "nsgo-share"))
				if err != nil {
					continue
				}
				files[j].Path = path
			}
			if files != nil {
				r.Files = files
				it.Share = &r
			}
		}
	}
}

// unstage removes what stage wrote for save, an answerKey, once Java answered it.
func (s *surface) unstage(save uint64) {
	s.smu.Lock()
	dir, ok := s.staged[save]
	delete(s.staged, save)
	s.smu.Unlock()
	if ok {
		os.RemoveAll(dir)
	}
}

// dnsServers are the DNS servers the runtime last reported.
var dnsServers atomic.Pointer[[]string]

// useDNS sends the app's DNS queries to the servers the runtime reports.
// Android has no resolv.conf, so Go without cgo would ask localhost.
func useDNS() {
	net.DefaultResolver = &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			servers := dnsServers.Load()
			if servers == nil || len(*servers) == 0 {
				return nil, errors.New("android: no DNS servers")
			}
			// Go tries its default servers in turn, 127.0.0.1 first:
			// map that one to our first and the rest to our second.
			i := 0
			if address != "127.0.0.1:53" {
				i = 1
			}
			var d net.Dialer
			return d.DialContext(ctx, network, net.JoinHostPort((*servers)[i%len(*servers)], "53"))
		},
	}
}

func (s *surface) requestText(kind byte, body func(*writer)) ([]byte, bool) {
	var start time.Time
	if s.perf != nil {
		start = time.Now()
	}
	id, reply := s.textRPC.call(kind, textTimeout, body)
	if !start.IsZero() {
		s.perf.monitor.RecordTextRoundTrip(time.Since(start))
	}
	if reply.err != nil {
		log.Printf("android text %s %d: %v", textOperation(kind), id, reply.err)
		return nil, false
	}
	return reply.data, true
}

func (s *surface) receiveText(r *reader) {
	if len(r.buf) < 10 {
		log.Print("android text: truncated reply")
		return
	}
	id, kind, failed := r.u64(), r.byte(), r.byte()
	reply := textReply{data: r.buf}
	if failed != 0 {
		if failed == 1 && len(r.buf) >= 4 && uint32(len(r.buf)-4) == binary.BigEndian.Uint32(r.buf) {
			reply = textReply{err: errors.New(r.str())}
		} else {
			reply = textReply{err: errors.New("malformed error reply")}
		}
	}
	if !s.textRPC.complete(id, kind, reply) {
		log.Printf("android text: unexpected %s reply %d", textOperation(kind), id)
	}
}

// Prepare implements core.TextEngine: Java keeps the text and its paint
// under a new id, so measuring and laying it out send only the id.
func (s *surface) Prepare(t *driver.AttributedText) driver.PreparedText {
	p := preparedText{s: s, id: s.ids.Add(1), text: *t}
	// StaticLayout ellipsizes a head or a middle on one line only.
	p.cut = (t.Ellipsize == driver.EllipsizeHead || t.Ellipsize == driver.EllipsizeMiddle) && t.MaxLines > 1
	for _, a := range t.Attachments {
		p.attachments = append(p.attachments, a.Size)
	}
	_, p.ready = s.requestText(textPrepare, func(w *writer) {
		w.u32(p.id)
		w.str(t.Text)
		writeStyle(w, &t.Style)
		w.u32(uint32(t.MaxLines))
		w.u32(uint32(len(t.Spans)))
		for _, sp := range t.Spans {
			w.u32(uint32(sp.Start)) // UTF-8 offsets; Java converts them
			w.u32(uint32(sp.End))
			writeStyle(w, &sp.Style)
			w.u32(uint32(sp.Background.Color)) // 0: none
			w.points(sp.Background.Radius)
			writeBackgroundInset(w, sp.Background.Top)
			writeBackgroundInset(w, sp.Background.Bottom)
		}
		// Java sizes each placeholder and places it in its line as
		// core.TextAttachment.Ascent and Extent say.
		w.u32(uint32(len(t.Attachments)))
		for _, a := range t.Attachments {
			w.u32(uint32(a.Offset))
			w.points(a.Size.Width)
			w.points(a.Size.Height)
			w.byte(byte(a.Align))
			w.points(a.Descent)
		}
		// Where the line limit cuts, the shadow, then the decorations'
		// styles, colors, thicknesses and underline offsets (NaN: the
		// font's) of the text and each span.
		w.byte(byte(t.Ellipsize))
		w.u32(uint32(t.Shadow.Color)) // 0: none
		w.points(t.Shadow.Offset.X)
		w.points(t.Shadow.Offset.Y)
		w.points(t.Shadow.Radius)
		writeDecoration(w, &t.Style)
		for _, sp := range t.Spans {
			writeDecoration(w, &sp.Style)
		}
	})
	return p
}

// preparedText is text Java keeps under id until Release, and the sizes
// of its attachments.
type preparedText struct {
	s           *surface
	id          uint32
	attachments []driver.Size
	text        driver.AttributedText
	ready       bool
	// cut means Layout cuts the last line at its head or middle.
	cut bool
}

// Measure asks the runtime's StaticLayout and estimates the size on failure.
func (p preparedText) Measure(maxWidth driver.Points) driver.Size {
	if p.ready {
		if data, ok := p.s.requestText(textMeasure, func(w *writer) {
			w.u32(p.id)
			w.points(maxWidth) // Auto, a NaN, does not wrap
		}); ok {
			if len(data) == 8 {
				r := reader{data}
				return driver.Size{Width: r.points(), Height: r.points()}
			}
			log.Print("android text measure: malformed size")
		}
	}
	return engine.ApproximateTextSize(&p.text, maxWidth)
}

// Layout asks Java to keep the final StaticLayout for the upcoming mount.
func (p preparedText) Layout(size driver.Size) driver.TextLayout {
	if p.cut && p.ready {
		if l := p.layoutCut(size); l != nil {
			return l
		}
	}
	l := &textLayout{s: p.s}
	l.carets.Set(l.caretLines)
	if !p.ready {
		return l
	}
	l.id = p.s.ids.Add(1)
	data, ok := p.s.requestText(textLayoutOp, func(w *writer) {
		w.u32(p.id)
		w.u32(l.id)
		w.points(size.Width)
	})
	if !ok {
		return l
	}
	if len(data) < 4 {
		log.Print("android text layout: missing attachments count")
		return l
	}
	r := reader{data}
	n := int(r.u32())
	if n > len(r.buf)/8 || len(r.buf) != n*8 {
		log.Print("android text layout: malformed attachment boxes")
		return l
	}
	l.native = true
	l.attachments = make([]attachmentBox, n)
	l.sizes = p.attachments
	for i := range l.attachments {
		x, y := r.points(), r.points()
		l.attachments[i] = attachmentBox{driver.Point{X: x, Y: y}, !math.IsNaN(float64(x))}
	}
	return l
}

func (p preparedText) Release() {
	p.s.send(msgRelease, func(w *writer) { w.u32(p.id) })
}

// layoutCut lays p out with its last line's rest cut at its head or
// middle, as driver.AttributedText.CutRest does, from where StaticLayout
// breaks its lines at size's width; nil when the lines fit.
func (p preparedText) layoutCut(size driver.Size) driver.TextLayout {
	whole := p.text
	whole.MaxLines = 0
	q := p.s.Prepare(&whole).(preparedText)
	ql := q.Layout(size).(*textLayout)
	lines := ql.TextLines()
	ql.Release()
	q.Release()
	if len(lines) <= p.text.MaxLines {
		return nil
	}
	fits := func(c *driver.AttributedText) bool {
		r := p.s.Prepare(c)
		defer r.Release()
		return r.Measure(driver.Auto).Width <= size.Width+0.01
	}
	cut := p.text.CutRest(lines[p.text.MaxLines-1].Start, fits)
	r := p.s.Prepare(&cut).(preparedText)
	defer r.Release()
	l := r.Layout(size).(*textLayout)
	// Attachments by the app's indices: those cut out are hidden.
	if n := len(p.text.Attachments); n > 0 {
		boxes, sizes := make([]attachmentBox, n), make([]driver.Size, n)
		for k, o := range cut.Origin {
			if k < len(l.attachments) && o < n {
				boxes[o], sizes[o] = l.attachments[k], l.sizes[k]
			}
		}
		l.attachments, l.sizes = boxes, sizes
	}
	return l
}

// textLayout names a StaticLayout Java built, and where it put the
// text's attachments. The runtime drops the layout itself when a later
// ItemText replaces it or its view is deleted.
type textLayout struct {
	native      bool
	id          uint32
	s           *surface
	attachments []attachmentBox
	sizes       []driver.Size
	// carets hit-tests the lines, which Java reads when first asked.
	carets driver.LazyCarets
}

// HitTest implements driver.TextHitTester.
func (l *textLayout) HitTest(p driver.Point) driver.Caret { return l.carets.HitTest(p) }

// CaretRect implements driver.TextHitTester.
func (l *textLayout) CaretRect(c driver.Caret) (driver.Bounds, bool) { return l.carets.CaretRect(c) }

// RangeRects implements driver.TextHitTester.
func (l *textLayout) RangeRects(start, end int, out []driver.Bounds) []driver.Bounds {
	return l.carets.RangeRects(start, end, out)
}

// caretLines asks Java where the StaticLayout's characters are, which it
// keeps while a text view shows it, and waits for the answer.
func (l *textLayout) caretLines() driver.CaretLayout {
	if !l.native {
		return nil
	}
	data, ok := l.s.requestText(textCarets, func(w *writer) { w.u32(l.id) })
	if !ok {
		return nil
	}
	if out, ok := readCarets(data); ok {
		return out
	}
	log.Print("android text carets: malformed lines")
	return nil
}

// readCarets reads each line's UTF-16 range, box and direction, then its clusters.
func readCarets(data []byte) (driver.CaretLayout, bool) {
	if len(data) < 4 {
		return nil, false
	}
	r := reader{data}
	n := int(r.u32())
	if n > len(r.buf)/29 {
		return nil, false
	}
	out := make(driver.CaretLayout, n)
	for i := range out {
		if len(r.buf) < 29 {
			return nil, false
		}
		l := &out[i]
		l.Start, l.End = int(r.u32()), int(r.u32())
		l.Box = driver.Bounds{Origin: driver.Point{X: r.points(), Y: r.points()},
			Size: driver.Size{Width: r.points(), Height: r.points()}}
		l.RTL = r.byte() != 0
		n := int(r.u32())
		if n > len(r.buf)/17 {
			return nil, false
		}
		l.Clusters = make([]driver.CaretCluster, n)
		for k := range l.Clusters {
			c := &l.Clusters[k]
			c.Start, c.End = int(r.u32()), int(r.u32())
			c.Left, c.Right = r.points(), r.points()
			c.RTL = r.byte() != 0
		}
	}
	return out, len(r.buf) == 0
}

// TextLines implements core.LineLayout, through the runtime's StaticLayout.
func (l *textLayout) TextLines() []driver.TextLine {
	if !l.native {
		return nil
	}
	data, ok := l.s.requestText(textLines, func(w *writer) { w.u32(l.id) })
	if !ok {
		return nil
	}
	if len(data) < 4 {
		log.Print("android text lines: missing line count")
		return nil
	}
	r := reader{data}
	n := int(r.u32())
	if n > len(r.buf)/40 || len(r.buf) != n*40 {
		log.Print("android text lines: malformed lines")
		return nil
	}
	lines := make([]driver.TextLine, n)
	for i := range lines {
		l := &lines[i]
		l.Start, l.End = int(r.u32()), int(r.u32()) // UTF-8 offsets
		l.Bounds = driver.Bounds{Origin: driver.Point{X: r.points(), Y: r.points()},
			Size: driver.Size{Width: r.points(), Height: r.points()}}
		l.Ascender, l.Descender, l.CapHeight, l.XHeight = r.points(), r.points(), r.points(), r.points()
	}
	return lines
}

// attachmentBox is the top left of an attachment in the content box.
type attachmentBox struct {
	origin driver.Point
	shown  bool
}

// Release frees a StaticLayout that no mount batch will name.
func (l *textLayout) Release() {
	if l.id != 0 {
		l.s.send(msgRelease, func(w *writer) { w.u32(l.id) })
	}
}

// Attachment implements core.AttachmentLayout.
func (l *textLayout) Attachment(i int) (driver.Bounds, bool) {
	if i < 0 || i >= len(l.attachments) || i >= len(l.sizes) || !l.attachments[i].shown {
		return driver.Bounds{}, false
	}
	return driver.Bounds{Origin: l.attachments[i].origin, Size: l.sizes[i]}, true
}

// RequestFrame implements engine.Platform: Java answers with msgFrame
// when its next Choreographer frame begins, before it steps the native
// animations.
func (s *surface) RequestFrame() {
	s.frameWanted.Store(true)
	s.send(msgWantFrame, func(*writer) {})
}

func (s *surface) SubmitGPU(driver.GPUFrame) {}

// Mount implements engine.Platform: the commit and any follow-up shader
// pixel batches, then the loads of the image sources it changed.
func (s *surface) Mount(b *driver.MountBatch) {
	s.openOverlays(b)
	s.mount(&s.main, b)
}

// mount sends the batch of v's tree to Java, which names its surface, and loads the image sources it changed.
func (s *surface) mount(v *treeView, b *driver.MountBatch) {
	b = withoutOffscreenReleases(s.withoutScans(v.id, s.withoutWebCookies(v.id, s.withoutAuthSessions(v.id, s.withoutPermissions(v.id, s.withoutUpdateChecks(v.id, withoutDesktopItems(withoutEffects(b))))))))
	// Disk space uses a separate message and runs on a worker. Android has no file manager an app can reveal in or trash through. A test's observer may hold the batch, so filter these items from a copy.
	outsideBatch := func(it driver.MountItem) bool {
		return it.Kind == driver.ItemRevealFile || it.Kind == driver.ItemTrashFile || it.Kind == driver.ItemDiskSpace
	}
	if slices.ContainsFunc(b.Items, outsideBatch) {
		for _, it := range b.Items {
			switch it.Kind {
			case driver.ItemRevealFile, driver.ItemTrashFile:
				s.dispatchTo(v.id, driver.Event{Kind: driver.EventFiles, Count: it.Count, Text: driver.NoFileRevealer})
			case driver.ItemDiskSpace:
				s.diskSpace(v.id, it.Count, it.Value)
			}
		}
		out := *b
		out.Items = slices.DeleteFunc(slices.Clone(b.Items), outsideBatch)
		b = &out
	}
	s.canvasRequests.update(v.id, b)
	var frames []pixelFrame
	requests, loads := s.trackImages(v, b, &frames)
	canvases := v.trackCanvasPixels(b, &frames)
	s.stage(v.id, b)
	for i := range b.Items {
		it := &b.Items[i]
		switch it.Kind {
		case driver.ItemPickDirectories:
			s.pickFolders(answerKey(v.id, it.Count))
		case driver.ItemText:
			if l, ok := it.Text.Layout.(*textLayout); ok && !l.native {
				// A timed-out layout may have reached Java. Release its ID
				// before the batch, which sends an empty layout instead.
				l.Release()
			}
		}
	}
	natives := s.trackNatives(v.id, b)
	first := !v.defaultsSent
	v.defaultsSent = true
	for batch := b; ; {
		shaders, pending := s.trackShaders(v, batch)
		err := s.sendBatch(frames, func(w *writer) {
			w.u32(v.id)
			encode(w, batch, first, requests, natives, shaders, canvases)
		})
		if err != nil || len(pending) == 0 {
			break
		}
		// A fresh budget can send at least one deferred copy per batch.
		// Keep these batches ahead of the next commit to this tree.
		batch = &driver.MountBatch{Root: b.Root, Items: pending}
		first, requests, natives, frames, canvases = false, nil, nil, nil, nil
	}
	for _, l := range loads {
		go s.load(l)
	}
	// Go loads the images Java shows, so it answers ImageSize too.
	for i := range b.Items {
		if it := b.Items[i]; it.Kind == driver.ItemImageInfo {
			go s.dispatchTo(v.id, imageload.Info(&it, imagedecode.Size))
		}
	}
}

// treeView is what Go keeps of a tree a view of the runtime shows, on the engine goroutine: the activity's, the first surface, or an overlay's.
type treeView struct {
	// id is the surface's, 0 for the activity's view.
	id uint32
	// defaultsSent is set once a batch carried the defaults.
	defaultsSent bool
	images       map[driver.NodeID]*imageView
	// canvasPixels keeps the copies Java's mounted drawings hold, preventing their address-based keys from being reused.
	canvasPixels map[driver.NodeID]map[string]*image.RGBA
	// shaderPixels are the copies of pixels each shader view was sent last, by image, which Java keeps.
	shaderPixels map[driver.NodeID][]*image.RGBA
}

// target is the Engine of surface id, for the reader goroutine: the first's, else an attached overlay's that has not ended, else nil.
func (s *surface) target(id uint32) *engine.Engine {
	if id == 0 {
		return s.engine
	}
	s.omu.Lock()
	defer s.omu.Unlock()
	if o := s.overlays[id]; o != nil && !o.ended {
		return o.engine
	}
	return nil
}

// dispatchTo reports ev to surface id from any goroutine, once the first surface's engine started; an overlay gone takes nothing.
func (s *surface) dispatchTo(id uint32, ev driver.Event) {
	<-s.started
	if e := s.target(id); e != nil {
		e.Dispatch(ev)
	}
}

// answerKey keys a service request Java answers by the surface that asked and its count, which each tree numbers on its own.
func answerKey(surface, count uint32) uint64 { return uint64(surface)<<32 | uint64(count) }

// imageView is what Go keeps of an image view: the sources of its
// current request and how to stop loading them.
type imageView struct {
	source, placeholder driver.ImageSource
	sampling            driver.ImageSampling
	events              driver.EventMask
	request             uint32
	cancel              context.CancelFunc
	size                driver.Size // the view's, which an SVG is drawn for
	// A pixel source keeps the same request across frames. A style update with the same copy sends no pixels.
	pixels *image.RGBA
}

func (v *imageView) stop() {
	if v.cancel != nil {
		v.cancel()
	}
}

// imageLoad fetches the sources of one request of an image view, node id of surface's tree.
type imageLoad struct {
	ctx                 context.Context
	surface             uint32
	id                  driver.NodeID
	request             uint32
	source, placeholder driver.ImageSource
	events              driver.EventMask
	size                driver.Size
}

// trackImages follows the image views of b. A view whose sources change
// gets a new request number, which its ItemImage carries and its bytes
// name, so Java drops the bytes of requests it has moved past. It returns
// the request of each ItemImage in b, in order, and the loads to start.
func (s *surface) trackImages(tv *treeView, b *driver.MountBatch, frames *[]pixelFrame) ([]uint32, []imageLoad) {
	var requests []uint32
	var loads []imageLoad
	images := tv.images
	for i := range b.Items {
		it := &b.Items[i]
		v := images[it.ID]
		switch it.Kind {
		case driver.ItemCreate, driver.ItemDelete:
			if v != nil {
				v.stop()
				delete(images, it.ID)
			}
			if it.Kind == driver.ItemCreate && it.View == driver.KindImage {
				images[it.ID] = &imageView{}
			}
		case driver.ItemEvents:
			if v != nil {
				v.events = it.Events
			}
		case driver.ItemFrame:
			if v != nil {
				v.size = it.Frame.Size
			}
		case driver.ItemImage:
			if v == nil {
				v = &imageView{}
				images[it.ID] = v
			}
			c := it.Image
			px := c.Source.Pixels
			source, placeholder := c.Source, c.Placeholder
			if px != nil {
				// A pixel source needs no placeholder, loader or load events.
				source, placeholder = driver.ImageSource{}, driver.ImageSource{}
				if px != v.pixels {
					*frames = append(*frames, pixelFrame{item: i, pixels: px})
				}
			}
			// A decoded source turning nearest reloads at its full size; pixels are never downsampled.
			if v.request == 0 || (v.pixels == nil) != (px == nil) || !sameSource(v.source, source) || !sameSource(v.placeholder, placeholder) ||
				(px == nil && v.sampling != c.Sampling && c.Sampling == driver.ImageSamplingNearest) {
				v.stop()
				s.requests++
				v.source, v.placeholder, v.request = source, placeholder, s.requests
				v.cancel = nil
				if px == nil {
					ctx, cancel := context.WithCancel(context.Background())
					v.cancel = cancel
					loads = append(loads, imageLoad{ctx, tv.id, it.ID, v.request, source, placeholder, v.events, v.size})
				}
			}
			v.pixels = px
			v.sampling = c.Sampling
			requests = append(requests, v.request)
		}
	}
	// A frame may follow its view's ItemImage.
	for i := range loads {
		if v := images[loads[i].id]; v != nil {
			loads[i].size = v.size
		}
	}
	return requests, loads
}

func sameSource(a, b driver.ImageSource) bool {
	return a.URI == b.URI && len(a.Data) == len(b.Data) && (len(a.Data) == 0 || &a.Data[0] == &b.Data[0]) &&
		a.Pixels == b.Pixels && a.Offscreen.Same(b.Offscreen) && a.Version == b.Version
}

func withoutOffscreenReleases(b *driver.MountBatch) *driver.MountBatch {
	isRelease := func(it driver.MountItem) bool { return it.Kind == driver.ItemReleaseOffscreen }
	if !slices.ContainsFunc(b.Items, isRelease) {
		return b
	}
	out := *b
	out.Items = slices.DeleteFunc(slices.Clone(b.Items), isRelease)
	return &out
}

func isEmpty(src driver.ImageSource) bool { return src.URI == "" && len(src.Data) == 0 }

// load fetches l's placeholder and source side by side and hands their
// bytes to Java, which decodes them and reports EventLoad, or EventError
// for bytes that are no image.
func (s *surface) load(l imageLoad) {
	if !isEmpty(l.placeholder) {
		go func() {
			if data, err := imageload.Load(l.ctx, l.placeholder, nil); err == nil && l.ctx.Err() == nil {
				if img, err := s.prepareImage(l.ctx, data, l.placeholder.URI, l.size); err == nil {
					s.sendImage(l, imagePlaceholder, img)
				}
			}
		}()
	}
	if isEmpty(l.source) {
		return
	}
	s.report(l, driver.Event{Kind: driver.EventLoadStart})
	var last time.Time
	data, err := imageload.Load(l.ctx, l.source, func(loaded, total int64) {
		if now := time.Now(); l.ctx.Err() == nil && now.Sub(last) >= 100*time.Millisecond {
			last = now
			s.report(l, driver.Event{Kind: driver.EventProgress, Loaded: loaded, Total: total})
		}
	})
	switch {
	case l.ctx.Err() != nil:
		// The view is gone or shows other sources.
	case err != nil:
		s.report(l, driver.Event{Kind: driver.EventError, Text: err.Error()})
		s.report(l, driver.Event{Kind: driver.EventLoadEnd})
	default:
		img, err := s.prepareImage(l.ctx, data, l.source.URI, l.size)
		if err != nil {
			s.report(l, driver.Event{Kind: driver.EventError, Text: err.Error()})
			s.report(l, driver.Event{Kind: driver.EventLoadEnd})
			return
		}
		if err := s.sendImage(l, imageSource, img); err != nil {
			s.report(l, driver.Event{Kind: driver.EventError, Text: err.Error()})
			s.report(l, driver.Event{Kind: driver.EventLoadEnd})
		}
	}
}

// report dispatches ev about l's view if the view reports its kind.
func (s *surface) report(l imageLoad, ev driver.Event) {
	if l.events.Has(ev.Kind) {
		ev.ID = l.id
		s.dispatchTo(l.surface, ev)
	}
}

func (s *surface) sendImage(l imageLoad, part byte, img decodedImage) error {
	return s.send(msgImage, func(w *writer) {
		w.u32(l.surface)
		w.u32(uint32(l.id))
		w.u32(l.request)
		w.byte(part)
		w.float(img.scale)
		w.points(img.size.Width) // 0: the bitmap's own pixels
		w.points(img.size.Height)
		w.u32(uint32(len(img.data)))
		w.buf = append(w.buf, img.data...)
	})
}

// scaleSuffix matches the "@2x" core names an asset variant with.
var scaleSuffix = regexp.MustCompile(`@(\d+(?:\.\d+)?)x(?:\.\w+)?$`)

// imageScale is the pixels per point a file name declares, as "@2x".
func imageScale(uri string) float32 {
	if strings.HasPrefix(uri, "data:") {
		return 1
	}
	m := scaleSuffix.FindStringSubmatch(uri)
	if m == nil {
		return 1
	}
	f, err := strconv.ParseFloat(m[1], 32)
	if err != nil || f <= 0 {
		return 1
	}
	return float32(f)
}

// send writes one message whose payload body writes, or none and an error when the payload is longer than Java reads, as an image's or a font's bytes can be.
func (s *surface) send(kind byte, body func(w *writer)) error {
	return s.sendBefore(kind, time.Time{}, body)
}

func (s *surface) sendBefore(kind byte, deadline time.Time, body func(w *writer)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !deadline.IsZero() && !time.Now().Before(deadline) {
		return errTextTimeout
	}
	w := writer{append(s.buf[:0], kind, 0, 0, 0, 0)}
	body(&w)
	if err := checkMessage(kind, len(w.buf)-5); err != nil {
		return err
	}
	binary.BigEndian.PutUint32(w.buf[1:], uint32(len(w.buf)-5))
	_, err := s.out.Write(w.buf)
	if cap(w.buf) <= 1<<20 { // let an image's buffer go
		s.buf = w.buf
	}
	return err
}

func readMessage(r *bufio.Reader) (byte, []byte, error) {
	var head [5]byte
	if _, err := io.ReadFull(r, head[:]); err != nil {
		return 0, nil, err
	}
	payload := make([]byte, binary.BigEndian.Uint32(head[1:]))
	if _, err := io.ReadFull(r, payload); err != nil {
		return 0, nil, err
	}
	return head[0], payload, nil
}

// defaults are a new view's style and props as writeViewStyle and
// writeViewProps write them, which a surface's first batch carries:
// NsgoView starts each view it creates in them, with no events, so the
// core sends neither for a view that keeps them.
var defaults = sync.OnceValue(func() []byte {
	style, props := driver.DefaultViewStyle, driver.ViewProps{}
	var sw, pw writer
	writeViewStyle(&sw, &style)
	writeViewProps(&pw, &props)
	var w writer
	w.u32(uint32(len(sw.buf)))
	w.buf = append(w.buf, sw.buf...)
	w.u32(uint32(len(pw.buf)))
	w.buf = append(w.buf, pw.buf...)
	return w.buf
})

// encode writes a batch in the format NsgoView.mount reads, after the surface's ID mount writes: each item is its kind, its ID and the fields of that kind; requests numbers its ItemImage items in order, and natives says what its ItemNative items send. Kinds it does not know, ItemBackUnhandled and the desktop-only ItemMenu, which Java ignores, have no fields, and ItemWindow only an overlay's (writeWindow). The batch's layout animation comes before its items, and the first batch's defaults after it.
func encode(w *writer, b *driver.MountBatch, first bool, requests []uint32, natives []nativeItem, shaders []shaderItem, canvases map[int][]string) {
	w.u32(uint32(b.Root))
	writeLayoutAnimation(w, b.LayoutAnimation)
	w.flags(first)
	if first {
		w.buf = append(w.buf, defaults()...)
	}
	w.u32(uint32(len(b.Items)))
	for i := range b.Items {
		it := &b.Items[i]
		kind := it.Kind
		if kind == driver.ItemSaveFile && it.Save.Preflight {
			kind = driver.ItemSaveFilePreflight
		}
		w.byte(byte(kind))
		w.u32(uint32(it.ID))
		switch it.Kind {
		case driver.ItemCreate:
			w.byte(byte(it.View))
		case driver.ItemInsert:
			w.u32(uint32(it.Parent))
			w.u32(uint32(it.Before))
		case driver.ItemFrame:
			w.bounds(it.Frame)
			writeLayoutTransition(w, it.Transition)
		case driver.ItemMatch:
			w.u32(uint32(it.Before))
		case driver.ItemViewStyle:
			writeViewStyle(w, it.ViewStyle)
		case driver.ItemText:
			t := it.Text
			var layout uint32
			if l, ok := t.Layout.(*textLayout); ok && l.native {
				layout = l.id
			}
			w.str(t.Text)
			w.u32(layout)
			w.points(t.Content.Origin.X)
			w.points(t.Content.Origin.Y)
		case driver.ItemInput:
			in := it.Input
			writeStyle(w, &in.Style)
			w.str(in.Placeholder)
			w.byte(byte(in.KeyboardType))
			w.flags(in.Masked, in.Readonly, in.Disabled)
			w.bounds(in.Content)
			w.u32(uint32(in.Accessory))
			writeEditing(w, in)
		case driver.ItemInputValue:
			w.str(it.Value)
			w.u32(it.Count)
		case driver.ItemSelection:
			w.u32(uint32(max(it.Selection.Start, 0)))
			w.u32(uint32(max(it.Selection.End, 0)))
		case driver.ItemScrollBy:
			w.points(it.Point.X)
			w.points(it.Point.Y)
		case driver.ItemStickyCells:
			w.u32(uint32(len(it.Sticky)))
			for _, c := range it.Sticky {
				w.u32(uint32(c.Cell))
				w.points(c.Start)
				w.points(c.End)
				w.u32(c.Node)
				w.points(c.Inset)
			}
		case driver.ItemEvents:
			w.u64(uint64(it.Events))
		case driver.ItemViewProps:
			writeViewProps(w, it.ViewProps)
		case driver.ItemScrollConfig:
			c := it.Scroll
			b := &c.Behavior
			w.flags(c.Horizontal, c.HideIndicator)
			w.flags(b.Paging, b.DisableIntervalMomentum, b.NoScroll, b.NoBounces, c.Refreshable, c.Refreshing,
				c.NoBouncesZoom, c.NoPinchZoom)
			w.flags(c.CenterContent)
			w.points(b.SnapToInterval)
			w.byte(byte(b.SnapToAlignment))
			w.u32(uint32(len(b.SnapToOffsets)))
			for _, o := range b.SnapToOffsets {
				w.points(o)
			}
			w.float(b.Deceleration())
			in := b.ContentInset
			w.points(in.Top)
			w.points(in.Right)
			w.points(in.Bottom)
			w.points(in.Left)
			w.byte(byte(b.KeyboardDismissMode))
			w.byte(byte(b.KeyboardShouldPersistTaps))
			w.float(c.MinZoomScale)
			w.float(c.MaxZoomScale)
			w.float(c.ZoomScale)
			w.points(b.RefreshIndicator)
			w.points(c.HeaderInset)
		case driver.ItemScrollContent:
			w.points(it.Size.Width)
			w.points(it.Size.Height)
		case driver.ItemImage:
			c := it.Image
			w.u32(requests[0])
			requests = requests[1:]
			w.byte(byte(c.ResizeMode - driver.Cover)) // Java's are 0-based
			w.points(c.BlurRadius)
			w.u32(uint32(max(c.FadeDuration.Milliseconds(), 0))) // 0: the platform's
			w.flags(c.Sampling == driver.ImageSamplingNearest)
			// Each axis of the position as a fraction of the free space
			// plus points, which is what any position resolves to.
			for _, a := range [...]driver.PositionAxis{c.Position.X, c.Position.Y} {
				at := a.Resolve(0)
				w.float(float32(a.Resolve(1) - at))
				w.points(at)
			}
		case driver.ItemModal:
			w.byte(byte(it.Modal.Animation))
		case driver.ItemAlert:
			a := it.Alert
			w.u32(a.ID)
			w.str(a.Title)
			w.str(a.Message)
			w.u32(uint32(len(a.Actions)))
			for _, act := range a.Actions {
				w.str(act.Label)
				w.byte(byte(act.Role))
			}
			w.flags(a.Sheet)
			w.flags(a.Cancelable)
		case driver.ItemScrollTo:
			w.points(it.Point.X)
			w.points(it.Point.Y)
			w.flags(it.Animated)
		case driver.ItemZoomTo:
			z := it.Zoom
			w.float(z.Scale)
			var center driver.Point
			if z.Center != nil {
				center = *z.Center
			}
			w.flags(z.Center != nil)
			w.points(center.X)
			w.points(center.Y)
			w.flags(it.Animated)
		case driver.ItemCanvas:
			// The ops' length first: Java keeps them as they are and plays
			// them on each draw.
			writeDrawing(w, it.Drawing)
			keys := canvases[i]
			w.u32(uint32(len(keys)))
			for _, key := range keys {
				w.str(key)
			}
		case driver.ItemShader:
			if it.Shader != nil && len(shaders) > 0 {
				writeShader(w, it.Shader, shaders[0])
				shaders = shaders[1:]
			} else {
				w.u32(0) // no program
				w.flags(false)
				w.u32(0)
				w.u32(0)
				w.u32(0)
			}
		case driver.ItemSetClipboard, driver.ItemAnnounce, driver.ItemTitle, driver.ItemAccessibilityLabel,
			driver.ItemKeepImage, driver.ItemReleaseImage:
			w.str(it.Value)
		case driver.ItemReadClipboard:
			w.u32(it.Count)
			w.byte(byte(it.Clipboard))
		case driver.ItemSetClipboardImage:
			data, _ := it.Data.([]byte)
			w.str(string(data))
		case driver.ItemOpenURL:
			w.u32(it.Count)
			w.str(it.Value)
		case driver.ItemSystemGestures:
			w.u32(uint32(len(it.SystemGestures.AndroidRects)))
			for _, rect := range it.SystemGestures.AndroidRects {
				w.bounds(rect)
			}
		case driver.ItemHaptic, driver.ItemBadge, driver.ItemReleaseAwake:
			w.u32(it.Count)
		case driver.ItemOrientation:
			w.byte(byte(it.Count))
		case driver.ItemKeepAwake:
			w.u32(it.Count)
			w.flags(it.Awake != nil && it.Awake.Display)
		case driver.ItemSpeak:
			r := it.Speak
			w.u32(it.Count)
			w.byte(byte(r.Command))
			w.str(r.Text)
			w.str(r.Lang)
			w.str(r.Voice)
			w.float(r.Rate)
			w.float(r.Pitch)
		case driver.ItemBiometricAvailability:
			w.u32(it.Count)
		case driver.ItemBiometricAuthenticate:
			w.u32(it.Count)
			w.str(it.Value)
		case driver.ItemSensor:
			writeSensorRequest(w, it.Count, *it.Sensor)
		case driver.ItemNotificationPermission, driver.ItemOverlayPermission:
			w.u32(it.Count)
			w.flags(it.On)
		case driver.ItemOpenSurface:
			writeSurface(w, it.Surface)
		case driver.ItemWindow:
			writeWindow(w, it.Window)
		case driver.ItemShowNotification:
			n := it.Notification
			w.u32(it.Count)
			w.str(n.ID)
			w.str(n.Title)
			w.str(n.Body)
			w.flags(n.Silent)
			// When it shows, in Unix milliseconds, 0 for now, and how it repeats.
			var at int64
			if !n.At.IsZero() {
				at = n.At.UnixMilli()
			}
			w.u64(uint64(at))
			w.byte(byte(n.Repeat))
		case driver.ItemCloseNotification:
			w.str(it.Value)
		case driver.ItemPickImages, driver.ItemTakePhoto:
			// The request, as NsgoView reads it: the limit, the longest
			// side (0: any), the JPEG quality (0: the default, keeping
			// a pick's JPEGs) and whether the stored format stays.
			r := it.Pick
			w.u32(it.Count)
			w.u32(uint32(r.Limit))
			w.u32(uint32(r.MaxDimension))
			w.float(r.Quality)
			w.flags(r.Format == driver.PickCurrent)
		case driver.ItemPickFiles:
			// The limit and the media types offered, none for every file.
			types, _ := it.Files.MIMETypes()
			w.u32(it.Count)
			w.u32(uint32(it.Files.Limit))
			w.u32(uint32(len(types)))
			for _, t := range types {
				w.str(t)
			}
		case driver.ItemPickDirectories:
			w.u32(it.Count)
			w.u32(uint32(it.Directories.Limit))
		case driver.ItemSaveFile:
			// The file Java copies to where the user chose, "" for a save
			// that failed already, then the name and type suggested.
			w.u32(it.Count)
			w.str(it.Save.Path)
			w.str(it.Save.Name)
			w.str(it.Save.MIME)
		case driver.ItemSavePreflightAnswer:
			w.u32(it.Count)
			w.flags(it.On)
			w.str(it.Value)
		case driver.ItemOpenFile:
			// The file, the name the app opening it sees and its type.
			w.u32(it.Count)
			w.str(it.Open.Path)
			w.str(it.Open.Name)
			w.str(it.Open.MIME)
		case driver.ItemInstallPackage:
			// The APK, under its name.
			w.u32(it.Count)
			w.str(it.Open.Path)
			w.str(it.Open.Name)
		case driver.ItemCanInstallPackages, driver.ItemInstallSettings:
			w.u32(it.Count)
		case driver.ItemShare:
			// The text, then each file as ItemOpenFile's.
			w.u32(it.Count)
			w.str(it.Share.Text)
			w.u32(uint32(len(it.Share.Files)))
			for _, f := range it.Share.Files {
				w.str(f.Path)
				w.str(f.Name)
				w.str(f.MIME)
			}
		case driver.ItemAppearance:
			w.byte(byte(it.Appearance))
		case driver.ItemDirection:
			writeDirection(w, it)
		case driver.ItemKeymap:
			w.u32(uint32(len(it.Keys)))
			for _, k := range it.Keys {
				w.str(k.Key)
				// InText and Repeat, then from bit 2 on the keys it yields (Protocol.java's KEY_USE_*).
				var flags byte
				if k.InText {
					flags |= 1
				}
				if k.Repeat {
					flags |= 2
				}
				w.byte(flags | byte(k.Yields)<<2)
				// The ID EventKey reports when the binding fires.
				w.u32(k.ID)
			}
		case driver.ItemStatusBar:
			c := it.StatusBar
			w.byte(byte(c.Style))
			w.flags(c.Hidden, c.Animated)
		case driver.ItemOverflow:
			w.bounds(it.Frame)
		case driver.ItemScreen:
			c := it.Screen
			w.str(c.Title)
			w.flags(c.HeaderHidden, c.HeaderTransparent, c.LargeTitle, c.CanGoBack, c.BackHidden, c.NoPopOnSwipe, c.NoPopOnBack)
			w.str(c.BackTitle)
			w.u32(uint32(c.Background))
			w.u32(uint32(c.TitleColor))
			w.u32(uint32(c.TintColor))
			w.u32(uint32(c.Left))
			w.u32(uint32(c.Right))
			w.u32(uint32(c.TitleView))
			w.points(c.HeaderHeight)
		case driver.ItemWeb:
			c := it.Web
			w.str(c.Source.Src)
			w.str(c.Source.HTML)
			w.str(c.Source.BaseURL)
			w.str(c.InjectedJavaScript)
			// WebNode runs its bridge first, so the app's script can post.
			w.str(c.InjectedJavaScriptBeforeContent)
			w.str(c.UserAgent)
			w.str(c.UserAgentSuffix)
			// Android has no back and forward swipes.
			w.flags(c.NoScroll, c.NoJavaScript, c.Restricted, c.Incognito, c.Media&driver.WebMicrophone != 0, c.Media&driver.WebCamera != 0, c.PinchZoom)
			w.str(c.Hosts)
			w.str(c.Schemes)
			w.str(c.DownloadDir)
			w.u32(uint32(len(c.Headers)))
			for name, value := range c.Headers {
				w.str(name)
				w.str(value)
			}
		case driver.ItemVideo:
			c := it.Video
			w.str(c.Source)
			w.flags(c.Paused, c.Muted, c.Repeat, c.Controls)
			w.float(c.Volume)
			w.float(c.Rate)
			w.byte(byte(c.ResizeMode - driver.Cover)) // Java's are 0-based
			w.u32(uint32(c.ProgressInterval.Milliseconds()))
			w.byte(byte(c.AudioSession))
			w.u32(uint32(c.Headers.Len()))
			for name, value := range c.Headers.All {
				w.str(name)
				w.str(value)
			}
		case driver.ItemVideoSeek:
			w.u64(uint64(it.Time.Milliseconds()))
		case driver.ItemNative:
			n := natives[0]
			natives = natives[1:]
			w.str(n.class)
			w.u32(uint32(len(n.props)))
			for _, p := range n.props {
				w.str(p.Name)
				w.str(jsonValue(p.Value))
			}
			// The props animated nodes drive, when they changed: Java
			// hands the view their values on the frames they change.
			w.flags(it.Native.Bind)
			if it.Native.Bind {
				w.u32(uint32(len(it.Native.Bindings)))
				for _, b := range it.Native.Bindings {
					w.str(b.Name)
					w.u32(b.Node)
					w.flags(b.Color)
				}
			}
		case driver.ItemNativeCommand:
			w.str(it.Value)
			w.str(jsonValue(it.Data))
		case driver.ItemAnimation:
			writeAnimation(w, it.Animation)
		case driver.ItemWebCommand:
			w.byte(byte(it.WebCommand))
			if it.WebCommand == driver.WebPostMessage {
				w.str(driver.WebMessageScript(it.Value))
			} else if it.WebCommand == driver.WebLoad {
				if src, ok := it.Data.(*driver.WebSource); ok && src != nil {
					w.str(src.Src)
					w.str(src.HTML)
					w.str(src.BaseURL)
				} else {
					w.str("")
					w.str("")
					w.str("")
				}
			} else {
				w.str(it.Value)
			}
		}
	}
}

// sendFonts tells Java the app's font files, paths in the APK's assets,
// with their faces' families, which Java loads as Typefaces before it
// prepares any text.
func (s *surface) sendFonts(paths []string) {
	type font struct {
		path string
		face fontfile.Face
	}
	var fonts []font
	for _, p := range paths {
		data, err := imageload.ReadResource(p)
		if err != nil {
			log.Printf("nsgo: font %s: %v", p, err)
			continue
		}
		faces, err := fontfile.Describe(data)
		if err != nil || len(faces) == 0 {
			log.Printf("nsgo: font %s: %v", p, err)
			continue
		}
		// Typeface reads a collection's first face.
		fonts = append(fonts, font{p, faces[0]})
	}
	if len(fonts) == 0 {
		return
	}
	s.send(msgFonts, func(w *writer) {
		w.u32(uint32(len(fonts)))
		for _, f := range fonts {
			w.str(f.path)
			w.str(f.face.Family)
			w.u16(uint16(f.face.Weight))
			w.flags(f.face.Italic)
		}
	})
}

// RegisterFont implements driver.FontRegistrar: Java makes a Typeface of
// it before it prepares the text sent after it.
func (s *surface) RegisterFont(data []byte) error {
	faces, err := fontfile.Describe(data)
	if err != nil {
		return err
	}
	if len(faces) == 0 {
		return errors.New("no faces")
	}
	// Typeface reads a collection's first face.
	f := faces[0]
	return s.send(msgFontData, func(w *writer) {
		w.str(f.Family)
		w.u16(uint16(f.Weight))
		w.flags(f.Italic)
		w.u32(uint32(len(data)))
		w.buf = append(w.buf, data...)
	})
}

// canvasRequest handles a cache miss or cancellation from one Java surface.
func (s *surface) canvasRequest(r *reader) {
	surface, key, request, stop := r.u32(), r.str(), r.u32(), r.byte() != 0
	if stop {
		s.canvasRequests.cancel(surface, key, request)
		return
	}
	ctx, src, load, known := s.canvasRequests.start(surface, key, request)
	if load == nil {
		if !known {
			s.sendCanvasImage(surface, key, request, decodedImage{}, true)
		}
		return
	}
	if src.Offscreen != nil {
		// Android's canvases draw no offscreen drawings yet: a failed entry, with no log line for each version.
		s.canvasRequests.finish(surface, key, load)
		s.sendCanvasImage(surface, key, request, decodedImage{}, false)
		return
	}
	go func() {
		data, err := imageload.Load(ctx, src, nil)
		var img decodedImage
		if err == nil {
			img, err = s.prepareImage(ctx, data, src.URI, driver.Size{})
		}
		if !s.canvasRequests.finish(surface, key, load) {
			return
		}
		if err != nil {
			log.Printf("nsgo: canvas: image %s: %v", key, err)
		}
		// An empty response records a bounded failed entry, avoiding a retry loop on every redraw. Java rejects obsolete request generations.
		s.sendCanvasImage(surface, key, request, img, false)
	}()
}

// sendCanvasImage returns bytes, a failed decode (empty bytes), or an obsolete source (unavailable). The latter removes the pending entry so a later drawing of the source can request it after its mount has registered it again.
func (s *surface) sendCanvasImage(surface uint32, key string, request uint32, img decodedImage, unavailable bool) {
	if err := s.send(msgCanvasImage, func(w *writer) {
		w.u32(surface)
		w.str(key)
		w.u32(request)
		w.flags(unavailable)
		w.float(img.scale)
		w.u32(uint32(len(img.data)))
		w.buf = append(w.buf, img.data...)
	}); err != nil {
		log.Printf("nsgo: canvas: image %s: %v", key, err)
	}
}

// writeDecor writes what a view style draws past React Native's styles,
// after the rest, in the order NsgoView.style reads it.
func writeDecor(w *writer, v *driver.ViewStyle) {
	c := v.Corners
	w.points(c.TopLeft)
	w.points(c.TopRight)
	w.points(c.BottomRight)
	w.points(c.BottomLeft)
	w.byte(byte(v.BorderStyle))
	o := v.Outline
	w.points(o.Width)
	w.points(o.Offset)
	w.u32(uint32(o.Color))
	w.byte(byte(o.Style))
	tr := v.Transform
	w.floats(tr.RotateX, tr.RotateY)
	w.points(tr.Perspective)
	w.points(v.Origin.X)
	w.points(v.Origin.Y)
	w.points(v.ChildPerspective)
	// Android has no plus-darker, which stays normal.
	blend := v.BlendMode
	if blend == driver.BlendPlusDarker {
		blend = driver.BlendNormal
	}
	w.byte(byte(blend))
	e := v.Effects
	if e == nil {
		e = &driver.Effects{}
	}
	w.byte(byte(min(len(e.Shadows), 255)))
	for _, sh := range e.Shadows[:min(len(e.Shadows), 255)] {
		w.points(sh.Offset.X)
		w.points(sh.Offset.Y)
		w.points(sh.Blur)
		w.points(sh.Spread)
		w.u32(uint32(sh.Color))
		w.flags(sh.Inset)
	}
	w.byte(byte(min(len(e.Gradients), 255)))
	for _, g := range e.Gradients[:min(len(e.Gradients), 255)] {
		w.byte(byte(g.Kind))
		w.floats(g.X0, g.Y0, g.X1, g.Y1, g.CX, g.CY, g.RX, g.RY, g.Angle)
		w.u32(uint32(len(g.Stops)))
		for _, s := range g.Stops {
			w.float(s.Offset)
			w.u32(uint32(s.Color))
		}
	}
	// Filters: their color matrix, blur and widest drop shadow, the last
	// of equals, which Android draws from Android 12.
	m, color, blur, shadows := driver.SplitFilters(e.Filters)
	w.flags(color)
	if color {
		w.floats(m[:]...)
	}
	w.points(blur)
	w.byte(byte(min(len(shadows), 1)))
	if len(shadows) > 0 {
		sh := shadows[0]
		for _, s := range shadows[1:] {
			if s.Blur >= sh.Blur {
				sh = s
			}
		}
		w.points(sh.Offset.X)
		w.points(sh.Offset.Y)
		w.points(sh.Blur)
		w.u32(uint32(sh.Color))
	}
	writeKeyframes(w, v.Animation)
}

// writeStyle writes a text style in the format Text.Style reads.
func writeStyle(w *writer, t *driver.TextStyle) {
	w.str(firstFamily(t.Font.Family))
	w.str(t.Font.Language)
	w.points(t.Font.Size)
	w.u16(uint16(t.Font.Weight))
	w.u32(uint32(t.Color))
	w.points(t.LineHeight)
	w.points(t.LetterSpacing)
	w.byte(byte(t.Align))
	w.flags(t.Font.Italic,
		t.Decoration&driver.TextDecorationUnderline != 0,
		t.Decoration&driver.TextDecorationLineThrough != 0, t.RTL)
	w.byte(byte(t.Font.Variant))
}

// writeBackgroundInset writes either points or a fraction of the box's height.
func writeBackgroundInset(w *writer, inset driver.Length) {
	if percent, ok := inset.Percent(); ok {
		w.byte(1)
		w.float(percent / 100)
		return
	}
	points, _ := inset.Points()
	w.byte(0)
	w.points(points)
}

// writeDecoration writes how a text style's decoration lines are drawn,
// which Text.Style.readDecoration reads.
func writeDecoration(w *writer, t *driver.TextStyle) {
	w.byte(byte(t.DecorationStyle))
	w.u32(uint32(t.DecorationColor))
	w.points(t.DecorationThickness)
	offset := driver.Auto
	if o, ok := t.UnderlineOffset.Points(); ok {
		offset = o
	}
	w.points(offset)
}

// writeViewStyle writes a view's style as NsgoView.style reads it.
func writeViewStyle(w *writer, v *driver.ViewStyle) {
	w.u32(uint32(v.Background))
	w.points(v.CornerRadius)
	w.points(v.BorderWidth)
	w.u32(uint32(v.BorderColor))
	// Each side's border, all 0 unless they differ.
	b, c := v.Borders, v.BorderColors
	w.points(b.Top)
	w.points(b.Right)
	w.points(b.Bottom)
	w.points(b.Left)
	w.u32(uint32(c.Top))
	w.u32(uint32(c.Right))
	w.u32(uint32(c.Bottom))
	w.u32(uint32(c.Left))
	w.float(v.Opacity)
	// Android draws shadows from elevation, which has no offset:
	// the view's own, cast in the system's black, else its style's.
	shadow, radius := v.ShadowColor, v.ShadowRadius
	if v.Elevation > 0 && (shadow&0xff == 0 || radius <= 0) {
		shadow, radius = driver.RGB(0), v.Elevation
	}
	w.u32(uint32(shadow))
	w.points(radius)
	w.u32(uint32(v.TintColor))
	// The scales as they are, 0 unset, for Java to combine with
	// animated ones as core.Transform.Scales does.
	tr := v.Transform.InPlane()
	w.points(tr.TranslateX)
	w.points(tr.TranslateY)
	w.floats(tr.Scale, tr.ScaleX, tr.ScaleY)
	// Android turns a view in its plane once: RotateZ joins Rotate,
	// which is exact unless RotateX or RotateY tilt it between.
	w.float(tr.Rotate + tr.RotateZ)
	tn := v.Transition
	w.u32(uint32(max(tn.Duration.Milliseconds(), 0)))
	w.u32(uint32(max(tn.Delay.Milliseconds(), 0)))
	w.float(tn.Easing.X1)
	w.float(tn.Easing.Y1)
	w.float(tn.Easing.X2)
	w.float(tn.Easing.Y2)
	w.byte(byte(tn.Properties))
	w.flags(v.Clip, v.Hidden, v.ContinuousCorners, v.BackfaceHidden)
	writeDecor(w, v)
}

// writeViewProps writes a view's props as NsgoView.props reads it.
func writeViewProps(w *writer, p *driver.ViewProps) {
	role := p.Role
	if role == driver.RoleAlertDialog {
		// The runtime knows a dialog, which names its pane alike.
		role = driver.RoleDialog
	}
	w.byte(byte(role))
	w.flags(p.Disabled, p.Selected, p.Checked, p.Indeterminate)
	w.str(p.Label)
	w.str(p.TestID)
	// Java decides when a drag activates the pan.
	a := p.PanActivation
	w.points(a.ActiveOffsetX)
	w.points(a.ActiveOffsetY)
	w.points(a.FailOffsetX)
	w.points(a.FailOffsetY)
	// Accessibility beyond the role, label and states.
	w.str(p.HelpText())
	w.flags(p.Expandable, p.Expanded, p.ElementsHidden, p.Value.IsRange())
	w.byte(byte(p.Accessible))
	w.byte(byte(p.LiveRegion))
	w.float(p.Value.Min)
	w.float(p.Value.Max)
	w.float(p.Value.Now)
	w.str(p.Value.Text)
	actions := p.ActionList()
	w.u32(uint32(len(actions)))
	for _, act := range actions {
		w.str(act.Name)
		w.str(act.Label)
	}
	// The runtime routes touches by them and times long presses.
	w.byte(byte(p.PointerEvents))
	w.points(p.HitSlop.Top)
	w.points(p.HitSlop.Right)
	w.points(p.HitSlop.Bottom)
	w.points(p.HitSlop.Left)
	w.u32(uint32(max(p.DelayLongPress.Milliseconds(), 0)))
	if r := p.Ripple; r != nil {
		w.flags(true, r.Borderless, r.Foreground)
		w.u32(uint32(r.Color))
		w.points(r.Radius)
	} else {
		w.byte(0)
	}
	// Selections run on the UI thread (Selection.java).
	w.flags(p.SelectionArea)
	w.byte(byte(p.Selectable))
	w.u32(uint32(p.SelectionColor))
	w.flags(p.Invalid, p.LabelPress)
	w.u32(uint32(p.LabelFor))
	var labels []driver.NodeID
	if p.LabelledBy != nil {
		labels = *p.LabelledBy
	}
	w.u32(uint32(len(labels)))
	for _, id := range labels {
		w.u32(uint32(id))
	}
}
