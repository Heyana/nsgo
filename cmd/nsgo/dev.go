package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// session is how dev builds and shows one target. start runs after each successful build and returns the running app; close, when set, cleans up session-wide resources such as an automation browser when dev ends.
type session struct {
	build func(ctx context.Context, p *project) error
	start func(ctx context.Context, p *project) (app, error)
	watch func(*project) string // additional inputs the target packages
	close func()                // releases a target that lives across rebuilds
}

// app is one started dev app. stop ends it, and wait returns once it has
// exited, reporting a failure. The web target only serves, so its wait is
// nil.
type app struct {
	stop func()
	wait func(context.Context) error
}

// devOptions are nsgo dev's flags that do not name a value in the project.
// The rest of the flags have replaced nsgo.yaml's values already, and apply
// replaces them again after each reload.
type devOptions struct {
	apply       func(*project)
	port        int    // the dev server's, which the web target is served from
	host        string // the dev server's bind address; empty listens on every interface
	open        bool   // whether to open the web target in the browser
	androidHost bool   // runs the Go side of an Android app on this machine
	device      string // the iPhone, simulator or Android device to run on
	team        string // the Apple developer team the iOS target signs with
	release     bool   // build as nsgo build does, for measuring

	debug     bool // run a native desktop app under Delve
	debugger  *devDebugger
	trace     bool // print the running app's commits
	profile   devProfileOptions
	highlight bool // outline what the running app's commits changed
	devtools  bool // open DevTools for the running app
	// automation lets tools drive the running app (automation.go); hub relays their requests.

	automation bool
	hub        *automationHub
	layout     string // what DevTools' layout audit starts showing (layoutFlag)
	fps        bool   // show the performance monitor: --nsgo-fps
	lang       string // the locale to run the app in: --nsgo-lang
	clip       string // the URL the App Clip opens from, run in the app's place
	server     *devServer
	nativeDir  string // the native desktop session's window state and Windows build directories
	preview    *previewSession
}

// automationSupported reports whether nsgo dev -automation drives target on hostOS.
func automationSupported(target, hostOS string) bool {
	return automationTargetError(target, hostOS, false) == nil
}

// dev builds and runs the app, with a dev server its builds connect to: a failed build shows over the app, and Command or Control+R in the app, outside the browser, builds and starts it again unless the app binds the key itself, with OnKey or an enabled item of its application menu (internal/dev's Wrap). It exits once the app does. With nsgo.yaml's hot reload it rebuilds and restarts the app whenever a Go file it compiles from, go.mod, nsgo.yaml or a catalog changes, and keeps the running app when a build fails. After each Go build of an app with a tsconfig.json it writes the TSX's types to .nsgo/types, for editors.
func dev(ctx context.Context, p *project, opts devOptions) error {
	if opts.debug {
		if err := checkDevDebug(p, opts); err != nil {
			return err
		}
		d, err := newDevDebugger(p)
		if err != nil {
			return err
		}
		opts.debugger = d
		defer os.RemoveAll(d.root)
	}
	if opts.automation {
		if err := automationTargetError(p.Dev.Target, runtime.GOOS, opts.release); err != nil {
			return err
		}
		if p.Dev.Target == "web" {
			if _, err := findBrowser(); err != nil {
				return err
			}
		}
	}
	appProject := p
	if opts.preview != nil {
		config := *p
		config.Dir = opts.preview.projectDir
		appProject = &config
	}
	profile, err := opts.profile.forProject(appProject, opts.release)
	if err != nil {
		return err
	}
	p.prepareDev(opts)
	if p.Dev.Target == "native" {
		captureFrame := !opts.release && (runtime.GOOS == "darwin" || runtime.GOOS == "windows" || runtime.GOOS == "linux")
		stageWindows := runtime.GOOS == "windows" && opts.debugger == nil
		if captureFrame || stageWindows {
			dir, err := os.MkdirTemp("", "nsgo-dev-native-")
			if err != nil && stageWindows {
				return fmt.Errorf("prepare Windows dev builds: %w", err)
			}
			if err == nil {
				opts.nativeDir = dir
				defer os.RemoveAll(dir)
			}
		}
	}
	srv, err := startDevServer(opts.host, opts.port)
	if err != nil {
		return err
	}
	defer srv.close()
	srv.dir = appProject.Dir
	opts.server = srv
	if profile.kind != "" {
		srv.mu.Lock()
		srv.profile = &devProfile{opts: profile}
		srv.mu.Unlock()
	}
	if opts.automation {
		if ip := srv.addr.IP; !ip.IsUnspecified() && !ip.IsLoopback() {
			return errors.New("-automation needs a loopback listener; omit -host or use a loopback address")
		}
		hub, err := newAutomationHub(srv, p.Dev.Target)
		if err != nil {
			return err
		}
		if err := hub.writeSession(p.Dir); err != nil {
			return err
		}
		defer hub.removeSession()
		appConsole.teeTo(hub.log)
		defer appConsole.teeTo(nil)
		opts.hub = hub
	}
	s, err := sessionFor(p, opts)
	if err != nil {
		return err
	}
	if s.close != nil {
		defer s.close()
	}

	stop := func() {}
	defer func() { stop() }()
	header(p.Name, p.Version, "dev "+p.Dev.Target)
	if opts.devtools {
		url := srv.pageURL(srv.url(), "/_nsgo/devtools/")
		note("DevTools %s", link(url))
		if opts.open {
			srv.open(url, p.cache("dev"))
		}
	}
	var exited chan error
	first, started, hinted := true, false, false
	for {
		progress := begin("building %s", p.Dev.Target)
		if opts.hub != nil {
			opts.hub.setBuilding(true)
		}
		// The catalogs' code first, which the build and the files watched
		// may need. One listing serves both, listed again when the
		// catalogs' new code may import more.
		stage(ctx, "generating catalogs")
		var pkgs []goPackage
		var catalogErr error
		if opts.preview != nil {
			catalogErr = opts.preview.writeHost(ctx, p, opts.lang)
		}
		if catalogErr == nil {
			pkgs, catalogErr = p.appPackages(ctx, ".")
		}
		if catalogErr == nil {
			var wrote bool
			if wrote, catalogErr = p.writeCatalogs(ctx, pkgs, false); wrote {
				pkgs, _ = p.appPackages(ctx, ".")
			}
		}
		stage(ctx, "finding sources")
		dirs := sourceDirs(p.Dir, pkgs)

		if p.Dev.Target == "android" {
			for _, path := range p.Android.Classes {
				dirs = append(dirs, p.resolve(path))
			}
		}
		if opts.preview != nil {
			dirs = append(dirs, opts.preview.source, opts.preview.projectDir, moduleRootOf(opts.preview.source))
			if p.Theme != "" {
				dirs = append(dirs, p.Theme)
			}
		}
		if p.Web.Host != "" {
			dirs = append(dirs, p.resolve(p.Web.Host))
		}
		if hasCatalogs(p.I18n.Dir) {
			dirs = append(dirs, p.I18n.Dir)
		}
		dirs, sources := splitSources(dirs)
		scanBuild := func() string {
			files := scan(dirs)
			if s.watch != nil {
				files += s.watch(p)
			}
			return files
		}
		fingerprint := scanBuild()
		err = catalogErr
		if opts.debugger != nil {
			opts.debugger.buildDir = ""
			if err == nil {
				err = opts.debugger.nextBuild()
			}
		}
		if err == nil {
			err = s.build(ctx, p)
		}
		if err != nil && opts.debugger != nil {
			os.RemoveAll(opts.debugger.buildDir)
		}
		if opts.hub != nil {
			opts.hub.setBuilding(false)
		}
		switch {
		case ctx.Err() != nil:
			progress.fail()
			return nil
		case err != nil && first && !p.hotReload():
			progress.fail()
			return err
		case err != nil:
			progress.fail()
			srv.send("error", strings.TrimSpace(err.Error()))
			report(err)
			detail("waiting for changes")
		default:
			progress.done(fmt.Sprintf("built %s", p.Dev.Target), "")
			stop()
			stop, exited = func() {}, nil
			// The web target only serves unless -automation starts its own browser; the others start an app, which may boot, install and launch on a device first.
			var starting *Step
			if p.Dev.Target != "web" || (opts.automation && !started) {
				starting = begin("starting the app")
			}
			reloadNow := started
			if next, err := s.start(ctx, p); err != nil {
				if starting != nil {
					starting.fail()
				}
				if opts.debugger != nil {
					return err
				}
				fail("run failed: %v", err)
			} else {
				if starting != nil {
					starting.done("started the app", "")
				}
				stop = next.stop
				if next.wait != nil {
					exited = make(chan error, 1)
					go func(wait func(context.Context) error, done chan error) { done <- wait(ctx) }(next.wait, exited)
				}
				started = true
				if !hinted && p.hotReload() {
					detail("watching for changes · Control+C stops")
					hinted = true
				}
			}
			srv.send("ok", "")
			if reloadNow {
				srv.send("reload", "")
			}
			// Editors read the TSX's types from .nsgo/types; a Go build
			// may have registered new ones.
			if p.hasTSConfig() {
				if err := writeTypes(ctx, p); err != nil && ctx.Err() == nil {
					warn("%v", err)
				}
			}
		}
		first = false
		if reason := waitForChange(ctx, p, srv, scanBuild, sources, fingerprint, exited); reason != nil {
			return *reason
		}
		// The flags and the rest of nsgo.yaml may have changed too.
		loadNext := func() (*project, error) { return load(p.Dir) }
		if opts.preview != nil {
			loadNext = opts.preview.project
		}
		if next, err := loadNext(); err == nil {
			opts.apply(next)
			if err := checkDevDebug(next, opts); err != nil {
				warn("%v", err)
			} else {
				next.prepareDev(opts)
				*p = *next
			}
		} else {
			warn("%v", err)
		}
	}
}

// automationTargetError reports why nsgo dev -automation cannot drive target on goos, or nil when it can.
func automationTargetError(target, goos string, release bool) error {
	if release {
		return errors.New("-automation drives dev builds; -release builds what users run")
	}
	if target == "web" {
		return nil
	}
	if target == "android" {
		return nil
	}
	if _, ok := miniProgramPlatformOf(target); ok {
		return nil
	}
	if goos == "windows" && target == "native" {
		return nil
	}
	switch goos {
	case "darwin":
		switch target {
		case "native", "ios", "tvos", "watchos", "ios_simulator", "tvos_simulator", "watchos_simulator":
			return nil
		}
	case "linux":
		if target == "native" {
			return nil
		}
	}
	return errors.New("-automation drives macOS, Windows, Linux, Android, the web, mini programs and Apple devices and simulators so far: use -target android, web, miniprogram_wechat, miniprogram_qq, miniprogram_alipay or miniprogram_douyin, run it on Windows or Linux with -target native, or on a Mac with -target native, ios, tvos, watchos, ios_simulator, tvos_simulator or watchos_simulator (docs/roadmap.md, Automation)")
}

// checkDevAutomation reports why nsgo dev -automation cannot drive target on goos, or nil when it can.
func checkDevAutomation(target, goos string, release bool) error {
	return automationTargetError(target, goos, release)
}

// prepareDev sets what dev's builds carry: the dev client, -trace's commits, -highlight's outlines, -layout's audit, -automation's agent and the dev icons; -release strips them as nsgo build does.
func (p *project) prepareDev(opts devOptions) {
	p.Release, p.Dev.Client, p.Dev.Trace, p.Dev.Highlight, p.Dev.Devtools, p.Dev.Automation = opts.release, true, opts.trace, opts.highlight, opts.devtools, opts.automation
	p.Dev.Layout = opts.layout

	p.devDebug = opts.debug
	p.Dev.Profile = opts.profile.kind != ""
	p.useDevIcons()
}

// appFlags are the engine's flags dev starts the app with: -fps's
// --nsgo-fps and -lang's --nsgo-lang.
func (o devOptions) appFlags() []string {
	var flags []string
	if o.fps {
		flags = append(flags, "--nsgo-fps")
	}
	// The preview uses -lang as its initial locale, then restores the
	// locale picked in its own controls on subsequent starts.
	if o.lang != "" && o.preview == nil {
		flags = append(flags, "--nsgo-lang="+o.lang)
	}
	return flags
}

// waitForChange returns nil once dev should build again: a reload asked
// for, or with hot reload a watched file changed. It returns the error
// dev ends with when the run is over: ctx done, or the app exited.
//
// A change of the app's TSX alone builds no Go: its bundles are compiled
// again and the running apps load them (compiler.App under nsgodev).
func waitForChange(ctx context.Context, p *project, srv *devServer, scanBuild func() string, sources []string, fingerprint string, exited chan error) *error {
	tick := time.NewTicker(300 * time.Millisecond)
	defer tick.Stop()
	tsx := scan(sources)
	for {
		select {
		case <-ctx.Done():
			var none error
			return &none
		case <-srv.reloads:
			return nil
		case err := <-exited:
			return &err
		case <-tick.C:
			if !p.hotReload() {
				continue
			}
			if scanBuild() != fingerprint {
				// Let the editor finish writing.
				time.Sleep(100 * time.Millisecond)
				return nil
			}
			if now := scan(sources); now != tsx {
				tsx = settled(sources, now)
				reloadBundles(p, srv)
			}
		}
	}
}

// settled waits until the files listed stop changing, as an editor or a
// copy writes them, and returns their fingerprint then.
func settled(files []string, last string) string {
	for range 20 {
		time.Sleep(100 * time.Millisecond)
		now := scan(files)
		if now == last {
			break
		}
		last = now
	}
	return last
}

// unreadable matches a compile's failure to open a source, which on
// Windows a file still held by what writes it gives.
var unreadable = regexp.MustCompile(`: open [^:\n]+: `)

// reloadBundles compiles the app's TSX again and sends the running apps
// the bundles, or the failure to show over them. A source that cannot be
// opened yet is read again a few times first.
func reloadBundles(p *project, srv *devServer) {
	progress := begin("compiling TSX")
	bundles, err := compileBundles(p.Dir, true)
	for try := 0; err != nil && try < 5 && unreadable.MatchString(err.Error()); try++ {
		time.Sleep(200 * time.Millisecond)
		bundles, err = compileBundles(p.Dir, true)
	}
	if err != nil {
		progress.fail()
		srv.send("error", strings.TrimSpace(err.Error()))
		report(err)
		return
	}
	srv.setBundles(bundles)
	progress.done("reloaded TSX", "")
	srv.send("ok", "")
	for entry := range bundles {
		srv.send("bundle", entry)
	}
}

// bundleSource reports whether a file is one a TSX bundle compiles from.
func bundleSource(name string) bool {
	switch filepath.Ext(name) {
	case ".tsx", ".ts", ".jsx", ".js", ".mjs", ".json":
		return true
	}
	return false
}

// splitSources takes the files a TSX bundle compiles from out of dirs.
func splitSources(dirs []string) (rest, sources []string) {
	for _, d := range dirs {
		if info, err := os.Stat(d); err == nil && !info.IsDir() && bundleSource(d) {
			sources = append(sources, d)
		} else {
			rest = append(rest, d)
		}
	}
	return rest, sources
}

// devServer is what dev's builds connect to: it streams build results to the apps and pages it started, takes their reload requests and the commits -trace prints, and serves the web target. It answers only requests that name it by an address or this machine's name (ownHost), and, but for the pages' files, carry the run's token (handle).
type devServer struct {
	addr    *net.TCPAddr
	srv     *http.Server
	mux     *http.ServeMux
	reloads chan struct{}
	// token is the run's, which the apps get with their dev server's address and the pages in their address's fragment (pageURL).
	token string

	mu      sync.Mutex
	clients map[chan string]bool
	// pages are the DevTools pages following the app's messages, and dir the app's directory, which their relative sites (a TSX file's) are in.
	pages   map[chan string]bool
	dir     string
	last    string // the last error event, for apps that connect later
	files   string
	bundles map[string]string // the latest TSX bundles, by entry
	// profile is the active runtime capture, set before the app starts.
	profile *devProfile
	// hub relays nsgo dev -automation's requests over the app's DevTools stream, nil without -automation.
	hub *automationHub
	// opened are the files open wrote, which close removes.
	opened []string
}

// setBundles keeps the latest TSX bundles, which /_nsgo/bundle serves.
func (s *devServer) setBundles(b map[string]string) {
	s.mu.Lock()
	s.bundles = b
	s.mu.Unlock()
}

// startDevServer listens on host and port, or the next free port on that
// host; 0 takes any port and an empty host listens on all interfaces.
func startDevServer(host string, port int) (*devServer, error) {
	bind := net.JoinHostPort(host, strconv.Itoa(port))
	addr, err := net.ResolveTCPAddr("tcp", bind)
	if err != nil {
		return nil, fmt.Errorf("resolve dev server address: %w", err)
	}
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	var ln *net.TCPListener
	for ; addr.Port <= min(port+100, 65535); addr.Port++ {
		if ln, err = net.ListenTCP("tcp", addr); err == nil || port == 0 {
			break
		}
	}
	if err != nil {
		return nil, fmt.Errorf("listen for the dev server at %s: %w", bind, err)
	}
	s := &devServer{addr: ln.Addr().(*net.TCPAddr), mux: http.NewServeMux(), reloads: make(chan struct{}, 1), token: token,
		clients: map[chan string]bool{}, pages: map[chan string]bool{}}
	s.handle("/_nsgo/events", s.events)
	s.handle("/_nsgo/profile", s.serveProfile)
	s.serveDevTools()
	s.handle("/_nsgo/bundle", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		code, ok := s.bundles[r.URL.Query().Get("entry")]
		s.mu.Unlock()
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/javascript; charset=utf-8")
		io.WriteString(w, code)
	})
	s.handle("/_nsgo/reload", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST to reload", http.StatusMethodNotAllowed)
			return
		}
		s.reload()
	})
	// A traced app's commits, printed as the app's output is.
	s.handle("/_nsgo/trace", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "POST a trace", http.StatusMethodNotAllowed)
			return
		}
		io.Copy(&appConsole, r.Body)
	})
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		files := s.files
		s.mu.Unlock()
		if files == "" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		webFiles(files).ServeHTTP(w, r)
	})
	s.srv = &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !ownHost(r.Host) {
			http.Error(w, "nsgo dev answers requests for this machine's addresses and name only", http.StatusForbidden)
			return
		}
		s.mux.ServeHTTP(w, r)
	})}
	go s.srv.Serve(ln)
	return s, nil
}

// tokenHeader carries the run's token on the apps' requests; pages, whose EventSource sends no header, put it in the query as nsgo-token.
const tokenHeader = "X-NSGo-Token"

// newToken returns 32 random bytes in hex.
func newToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// tokenIs reports whether got is want, in constant time.
func tokenIs(got, want string) bool {
	return want != "" && subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

// handle serves pattern with h for requests carrying the run's token alone: whatever reads the app's state, the build's or the project's, or changes them.
func (s *devServer) handle(pattern string, h http.HandlerFunc) {
	s.mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get(tokenHeader)
		if token == "" {
			token = r.URL.Query().Get("nsgo-token")
		}
		if !tokenIs(token, s.token) {
			http.Error(w, "nsgo dev's token is needed: open the address nsgo dev prints", http.StatusUnauthorized)
			return
		}
		h(w, r)
	})
}

// ownHost reports whether a request's Host names this server: an IP address, which no site can point its own name at, localhost, or this machine's name, as Bonjour gives it with .local too. Any other name may be a site's whose address was pointed at this machine to reach the server from its pages.
func ownHost(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if ip, _, _ := strings.Cut(strings.Trim(host, "[]"), "%"); net.ParseIP(ip) != nil || host == "localhost" {
		return true
	}
	name, err := os.Hostname()
	if err != nil || name == "" {
		return false
	}
	name = strings.TrimSuffix(strings.ToLower(name), ".local")
	return host == name || host == name+".local"
}

func (s *devServer) close() {
	s.srv.Close()
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, f := range s.opened {
		os.Remove(f)
	}
}

// open shows the page at url, one of pageURL's, in the user's browser (openPage), writing what it may need to in dir.
func (s *devServer) open(url, dir string) {
	cmd, file, err := openPage(url, dir)
	if err == nil {
		if file != "" {
			s.mu.Lock()
			s.opened = append(s.opened, file)
			s.mu.Unlock()
		}
		err = cmd.Start()
	}
	if err != nil {
		warn("could not open the browser: %v", err)
		return
	}
	go cmd.Wait()
}

func (s *devServer) reload() {
	select {
	case s.reloads <- struct{}{}:
	default:
	}
}

// serve serves dir at the server's root, for the web target.
func (s *devServer) serve(dir string) {
	s.mu.Lock()
	s.files = dir
	s.mu.Unlock()
}

// url is the server's address on this machine.
func (s *devServer) url() string {
	if s.addr.IP.IsUnspecified() {
		return s.urlAt("127.0.0.1")
	}
	return (&neturl.URL{Scheme: "http", Host: s.addr.String()}).String()
}

func (s *devServer) urlAt(host string) string {
	return "http://" + net.JoinHostPort(host, strconv.Itoa(s.addr.Port))
}

// pageURL is the address of a page the server serves at base, as url or one of lanURLs, and path, with the run's token in its fragment, which the page reads and no request carries.
func (s *devServer) pageURL(base, path string) string {
	return base + path + "#nsgo-token=" + s.token
}

// lanURLs are the addresses phones can reach. A specific bind address excludes all other addresses; a wildcard bind gives the outbound IPv4 address first.
func (s *devServer) lanURLs() []string {
	switch {
	case s.addr.IP.IsLoopback():
		return nil
	case !s.addr.IP.IsUnspecified():
		return []string{s.url()}
	}
	var ips []net.IP
	addrs, _ := net.InterfaceAddrs()
	for _, a := range addrs {
		if ip, ok := a.(*net.IPNet); ok && !ip.IP.IsLoopback() && ip.IP.To4() != nil && ip.IP.IsPrivate() {
			ips = append(ips, ip.IP)
		}
	}
	if out := outboundIP(); out != nil {
		if i := slices.IndexFunc(ips, out.Equal); i > 0 {
			ips = append(append([]net.IP{ips[i]}, ips[:i]...), ips[i+1:]...)
		}
	}
	out := make([]string, len(ips))
	for i, ip := range ips {
		out[i] = s.urlAt(ip.String())
	}
	return out
}

// outboundIP is the address the machine sends from to other hosts, as its default route goes; a UDP socket connects without sending anything. Nil without a route.
func outboundIP() net.IP {
	c, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return nil
	}
	defer c.Close()
	if a, ok := c.LocalAddr().(*net.UDPAddr); ok {
		return a.IP
	}
	return nil
}

// send tells every client kind: "error" with a failed build's output,
// "ok" once a build succeeds, "reload" for web pages to load it,
// "bundle" with a TSX entry whose new bundle /_nsgo/bundle serves, and
// "devtools" with a DevTools page's command for the app.
func (s *devServer) send(kind, text string) {
	msg := kind
	switch kind {
	case "error":
		msg += " " + strconv.Quote(text)
	case "bundle", "devtools":
		msg += " " + text
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	switch kind {
	case "error":
		s.last = msg
	case "ok":
		s.last = ""
	}
	for ch := range s.clients {
		select {
		case ch <- msg:
		default:
		}
	}
}

// events streams the server's events to an app or a page. The app's DevTools stream, ?devtools, carries the tools' requests as automation events too under -automation (automation.go).
func (s *devServer) events(w http.ResponseWriter, r *http.Request) {
	var requests chan []byte
	s.mu.Lock()
	hub := s.hub
	s.mu.Unlock()
	if hub != nil && r.URL.Query().Has("devtools") {
		var done func()
		var status int
		if requests, done, status = hub.connect(); status != 0 {
			http.Error(w, http.StatusText(status), status)
			return
		}
		defer done()
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	ch := make(chan string, 8)
	s.mu.Lock()
	s.clients[ch] = true
	last := s.last
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.clients, ch)
		s.mu.Unlock()
	}()
	flusher, _ := w.(http.Flusher)
	write := func(msg string) bool {
		if _, err := fmt.Fprintf(w, "data: %s\n\n", msg); err != nil {
			return false
		}
		if flusher != nil {
			flusher.Flush()
		}
		return true
	}
	if !write("hello") || last != "" && !write(last) {
		return
	}
	for {
		select {
		case msg := <-ch:
			if !write(msg) {
				return
			}
		case req := <-requests:
			if !write("automation " + string(req)) {
				return
			}
		case <-r.Context().Done():
			return
		}
	}
}

// sessionFor picks how dev builds and shows nsgo.yaml's target.
func sessionFor(p *project, opts devOptions) (session, error) {
	if opts.androidHost && (p.Dev.Target != "android" || opts.release) {
		return session{}, errors.New("-android-host needs -target android and a development build")
	}
	if opts.clip != "" {
		switch {
		case p.AppClip == nil:
			return session{}, fmt.Errorf("-clip: nsgo.yaml has no app_clip")
		case p.Dev.Target != "ios_simulator" && p.Dev.Target != "ios":
			return session{}, fmt.Errorf("-clip runs the App Clip on an iPhone or its simulator: use -target ios or ios_simulator")
		}
	}
	switch p.Dev.Target {
	case "native":
		switch runtime.GOOS {
		case "darwin":
			return macSession(opts), nil
		case "windows":
			return windowsSession(opts), nil
		case "linux":
			return linuxSession(opts), nil
		}
		return session{}, fmt.Errorf("native development runs on macOS, Windows and Linux; use -target web")
	case "tui":
		return tuiSession(opts), nil
	case "web":
		return webSession(opts), nil
	case "ios", "tvos", "watchos":
		sys, _, _ := appleOSOf(p.Dev.Target)
		return iosDeviceSession(sys, opts), nil
	case "ios_simulator", "tvos_simulator", "watchos_simulator":
		sys, _, _ := appleOSOf(p.Dev.Target)
		return iosSession(sys, opts), nil
	case "android":
		if opts.androidHost {
			return androidHostSession(opts)
		}
		return androidSession(opts), nil
	default:
		if mp, ok := miniProgramPlatformOf(p.Dev.Target); ok {
			return miniprogramSession(mp, opts), nil
		}
		return session{}, fmt.Errorf("unknown dev target %q; use native, tui, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android or %s",
			p.Dev.Target, strings.Join(miniProgramTargets(), ", "))
	}
}

// sourceDirs lists the project's directory and those of every non-standard
// package it compiles, NSGo's own included, and the files they embed: a
// TSX app's sources.
func sourceDirs(dir string, pkgs []goPackage) []string {
	dirs := []string{dir}
	for _, g := range pkgs {
		if g.standard || g.dir == "" {
			continue
		}
		dirs = append(dirs, g.dir)
		for _, e := range g.embeds {
			dirs = append(dirs, filepath.Join(g.dir, e))
		}
	}
	return dirs
}

// scan returns a fingerprint of the watched files' names, sizes and times:
// the Go and YAML files of each directory (nsgo.yaml and catalogs), and
// each file listed.
func scan(dirs []string) string {
	var b strings.Builder
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && !info.IsDir() {
			fmt.Fprintf(&b, "%s %d %d\n", dir, info.Size(), info.ModTime().UnixNano())
			continue
		}
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !(strings.HasSuffix(name, ".go") || strings.HasSuffix(name, ".yaml") || name == "go.mod" || name == "go.sum") {
				continue
			}
			if info, err := e.Info(); err == nil {
				fmt.Fprintf(&b, "%s/%s %d %d\n", dir, name, info.Size(), info.ModTime().UnixNano())
			}
		}
	}
	return b.String()
}

// devEnv is the environment of a dev app this machine starts: where its dev server is, and the run's token for it.
func devEnv(opts devOptions) []string {
	env := []string{"NSGO_DEV_SERVER=" + opts.server.url(), "NSGO_DEV_TOKEN=" + opts.server.token}
	if opts.nativeDir != "" && !opts.release {
		env = append(env, "NSGO_DEV_WINDOW_FRAME_FILE="+filepath.Join(opts.nativeDir, "frame"))
	}
	return env
}

// simulatorEnv is devEnv for an app the simulator starts, which passes it the variables named with SIMCTL_CHILD_.
func simulatorEnv(opts devOptions) []string {
	var env []string
	for _, e := range devEnv(opts) {
		env = append(env, "SIMCTL_CHILD_"+e)
	}
	return env
}

// macSession runs the app's bundle binary in the foreground, its output
// in this terminal.
func macSession(opts devOptions) session {
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := opts.desktopBuildDir(p, "macos")
			os.RemoveAll(dir)
			app, err := buildMacApp(ctx, p, dir, runtime.GOARCH)
			if err != nil || len(p.Widgets) == 0 {
				return err
			}
			// The app starts from its binary, which registers no extension
			// with the system: register its widgets.
			return run(ctx, "pluginkit", "-a", macWidgetsAppex(app))
		},
		start: func(ctx context.Context, p *project) (app, error) {
			dir := opts.desktopBuildDir(p, "macos")
			bin := filepath.Join(dir, p.Name+".app", "Contents", "MacOS", p.Name)
			return startDesktopDev(p, opts, dir, bin)
		},
	}
}

// windowsSession builds the app as a Windows executable and runs it in
// the foreground.
func windowsSession(opts devOptions) session {
	var built string
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := opts.desktopBuildDir(p, "windows")
			if opts.debugger == nil {
				// The running Windows app has the previous executable open.
				var err error
				dir, err = os.MkdirTemp(opts.nativeDir, "app-")
				if err != nil {
					return err
				}
			} else {
				// A debugged build reuses the project build directory; start it empty so no stale files remain.
				os.RemoveAll(dir)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return err
				}
			}
			if err := buildWindows(ctx, p, dir, runtime.GOARCH); err != nil {
				os.RemoveAll(dir)
				return err
			}
			built = dir
			return nil
		},
		start: func(ctx context.Context, p *project) (app, error) {
			dir := built
			bin := filepath.Join(dir, p.Name+".exe")
			a, err := startDesktopDev(p, opts, dir, bin)
			if err != nil {
				os.RemoveAll(dir)
				return app{}, err
			}
			if opts.debugger == nil {
				stop := a.stop
				a.stop = func() {
					stop()
					os.RemoveAll(dir)
				}
			}
			return a, nil
		},
	}
}

// linuxSession builds the app as a Linux executable and runs it in the
// foreground.
func linuxSession(opts devOptions) session {
	return session{
		build: func(ctx context.Context, p *project) error {
			dir := opts.desktopBuildDir(p, "linux")
			os.RemoveAll(dir)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return err
			}
			return buildLinux(ctx, p, dir, runtime.GOARCH)
		},
		start: func(ctx context.Context, p *project) (app, error) {
			dir := opts.desktopBuildDir(p, "linux")
			bin := filepath.Join(dir, p.Name)
			return startDesktopDev(p, opts, dir, bin)
		},
	}
}

// webSession serves the build from the dev server, whose page shows a failed build and loads each new one. Under -automation it opens its own headless browser over the DevTools protocol instead of the user's.
func webSession(opts devOptions) session {
	if opts.automation {
		return webAutomationSession(opts)
	}
	var once sync.Once
	return session{
		build: func(ctx context.Context, p *project) error {
			return buildWeb(ctx, p, p.cache("dev", "web"), true)
		},
		start: func(ctx context.Context, p *project) (app, error) {
			once.Do(func() {
				opts.server.serve(p.cache("dev", "web"))
				query := webQuery(opts.appFlags())
				base := opts.server.url()
				if opts.server.addr.IP.IsUnspecified() {
					base = opts.server.urlAt("localhost")
				}
				url := opts.server.pageURL(base, "/"+query)
				note("%s", link(url))
				for _, lan := range opts.server.lanURLs() {
					if lan == base {
						continue
					}
					detail("%s", link(opts.server.pageURL(lan, "/"+query)))
				}
				if opts.open {
					opts.server.open(url, p.cache("dev"))
				}
			})
			return app{stop: func() {}}, nil
		},
	}
}

// webAutomationSession builds the web target and opens it in a headless browser over the DevTools protocol with the nsgoTest binding installed, forwarding console output to appConsole and keeping the browser across rebuilds.
func webAutomationSession(opts devOptions) session {
	var (
		mu          sync.Mutex
		browser     *cdp
		cleanupPage func()
	)
	closeBrowser := func() {
		mu.Lock()
		b, cleanup := browser, cleanupPage
		browser, cleanupPage = nil, nil
		mu.Unlock()
		if cleanup != nil {
			cleanup()
		}
		if b != nil {
			b.close()
		}
	}
	return session{
		build: func(ctx context.Context, p *project) error {
			return buildWeb(ctx, p, p.cache("dev", "web"), true)
		},
		start: func(ctx context.Context, p *project) (app, error) {
			opts.server.serve(p.cache("dev", "web"))
			mu.Lock()
			b := browser
			mu.Unlock()
			if b == nil {
				query := webQuery(opts.appFlags())
				url := opts.server.pageURL(opts.server.urlAt("localhost"), "/"+query)
				note("%s", link(url))
				var err error
				b, err = startBrowser(ctx)
				if err != nil {
					return app{}, err
				}
				_, cleanup, err := openWebAutomation(b, url, &appConsole)
				if err != nil {
					b.close()
					return app{}, err
				}
				mu.Lock()
				browser = b
				cleanupPage = cleanup
				mu.Unlock()
			}
			cancelWait := make(chan struct{})
			var stopOnce sync.Once
			return app{
				stop: func() { stopOnce.Do(func() { close(cancelWait) }) },
				wait: func(ctx context.Context) error {
					select {
					case <-cancelWait:
						return nil
					case <-ctx.Done():
						return nil
					case <-b.done:
						if ctx.Err() != nil {
							return nil
						}
						if b.exitErr != nil {
							return fmt.Errorf("the automation browser exited: %w", b.exitErr)
						}
						return errors.New("the automation browser exited")
					}
				},
			}, nil
		},
		close: closeBrowser,
	}
}

// webQuery passes the engine's flags in the page's query, which nsgo.js
// gives the app as flags again: ?nsgo-fps&nsgo-lang=de.
func webQuery(flags []string) string {
	var params []string
	for _, f := range flags {
		name, value, ok := strings.Cut(strings.TrimPrefix(f, "--"), "=")
		if ok {
			name += "=" + neturl.QueryEscape(value)
		}
		params = append(params, name)
	}
	if len(params) == 0 {
		return ""
	}
	return "?" + strings.Join(params, "&")
}

// startProcess runs a command with its output above the tool's lines. stop
// ends the process; wait returns its error once it exits, or nil when stop
// or ctx ended it.
func startProcess(name string, args ...string) (stop func(), wait func(context.Context) error, err error) {
	return startProcessEnv(nil, name, args...)
}

// startProcessEnv is startProcess with env added to this process's.
func startProcessEnv(env []string, name string, args ...string) (stop func(), wait func(context.Context) error, err error) {
	return startProcessIn("", env, name, args...)
}

func startProcessIn(dir string, env []string, name string, args ...string) (stop func(), wait func(context.Context) error, err error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout, cmd.Stderr = &appConsole, &appConsole
	if env != nil {
		cmd.Env = append(os.Environ(), env...)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, err
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	var once sync.Once
	var stopped atomic.Bool
	stop = func() {
		once.Do(func() {
			select {
			case <-done:
				return
			default:
			}
			stopped.Store(true)
			if runtime.GOOS == "windows" {
				// Ask the app's windows to close, so it saves and exits;
				// a process without one is killed below.
				exec.Command("taskkill", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
			} else {
				cmd.Process.Signal(os.Interrupt)
			}
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				cmd.Process.Kill()
				<-done
			}
		})
	}
	wait = func(ctx context.Context) error {
		select {
		case <-done:
			if stopped.Load() {
				return nil
			}
			var exit *exec.ExitError
			if errors.As(waitErr, &exit) && exit.ExitCode() < 0 {
				return nil
			}
			return waitErr
		case <-ctx.Done():
			stop()
			return nil
		}
	}
	return stop, wait, nil
}
