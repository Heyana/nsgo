package main

import (
	"flag"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

// override is a flag that remembers whether the user gave it, so nsgo dev
// and nsgo build replace nsgo.yaml's values only with flags that were
// passed.
type override[T any] struct {
	value T
	set   bool
	parse func(string) (T, error)
}

func (o *override[T]) String() string { return fmt.Sprint(o.value) }

func (o *override[T]) Set(s string) error {
	v, err := o.parse(s)
	if err != nil {
		return err
	}
	o.value, o.set = v, true
	return nil
}

// replace stores the flag's value in dst when the user gave it.
func (o *override[T]) replace(dst *T) {
	if o.set {
		*dst = o.value
	}
}

// stringFlag and intFlag register a flag that replaces a value from
// nsgo.yaml.
func stringFlag(fs *flag.FlagSet, name, usage string) *override[string] {
	return bindOverride(fs, name, usage, func(s string) (string, error) { return s, nil })
}

func intFlag(fs *flag.FlagSet, name, usage string) *override[int] {
	return bindOverride(fs, name, usage, strconv.Atoi)
}

func bindOverride[T any](fs *flag.FlagSet, name, usage string, parse func(string) (T, error)) *override[T] {
	o := &override[T]{parse: parse}
	fs.Var(o, name, usage)
	return o
}

// stringListFlag collects a repeatable string flag's values in order, each as written: an -ldflags value keeps its spaces and quotes for go build to split.
func stringListFlag(fs *flag.FlagSet, name, usage string) *[]string {
	var values []string
	fs.Func(name, usage, func(value string) error {
		values = append(values, value)
		return nil
	})
	return &values
}

// boolOverride is an override[bool] that -flag alone sets to true.
type boolOverride struct{ override[bool] }

func (o *boolOverride) IsBoolFlag() bool { return true }

func boolFlag(fs *flag.FlagSet, name, usage string) *boolOverride {
	o := &boolOverride{override[bool]{parse: strconv.ParseBool}}
	fs.Var(o, name, usage)
	return o
}

// overrides are the app values nsgo.yaml gives, as flags shared by nsgo dev
// and nsgo build.
type overrides struct {
	name, id, version *override[string]
	buildNumber       *override[int]
}

func bindOverrides(fs *flag.FlagSet) *overrides {
	return &overrides{
		name:        stringFlag(fs, "name", "the app `name` (default: nsgo.yaml's name)"),
		id:          stringFlag(fs, "id", "the bundle `id` (default: nsgo.yaml's id)"),
		version:     stringFlag(fs, "version", "the app `version` (default: nsgo.yaml's version)"),
		buildNumber: intFlag(fs, "build-number", "the build's `number`, CFBundleVersion and Android's versionCode (default: nsgo.yaml's build_number)"),
	}
}

func (o *overrides) apply(p *project) {
	o.name.replace(&p.Name)
	// A generated ID follows the name, as it does when nsgo.yaml is read.
	if p.idDefault {
		p.ID = generatedID(p.Name)
	}
	o.id.replace(&p.ID)
	o.version.replace(&p.Version)
	o.buildNumber.replace(&p.BuildNumber)
}

// devFlags are nsgo dev's flags; those that name an nsgo.yaml value replace
// it when the user gives them.
type devFlags struct {
	*overrides
	target      *override[string]
	hotReload   *boolOverride
	fps         bool
	port        int
	host        string
	open        bool
	device      string
	androidHost bool
	team        string
	release     bool
	debug       bool
	trace       bool
	profile     devProfileOptions
	highlight   bool
	devtools    bool
	auto        bool
	lang        string
	clip        string
	limits      limitsFlag
	layout      layoutFlag
}

// layoutFlag is -layout: alone bounds, or what DevTools' layout audit starts showing, joined by commas: bounds, grid and grid=N, N from 4 to 1000000 points (-layout=bounds,grid=4). It holds the parts without spaces, as the dev client reads them (the root package's parseDevLayout).
type layoutFlag string

func (l *layoutFlag) String() string   { return string(*l) }
func (l *layoutFlag) IsBoolFlag() bool { return true }

func (l *layoutFlag) Set(v string) error {
	switch v {
	case "true", "":
		v = "bounds"
	case "false":
		*l = ""
		return nil
	}
	parts := strings.Split(v, ",")
	for i, part := range parts {
		part = strings.TrimSpace(part)
		name, n, hasN := strings.Cut(part, "=")
		switch {
		case (name == "bounds" || name == "grid") && !hasN:
		case name == "grid":
			// The audit's smallest step (core's minAuditGrid), written so that NaN fails it too.
			if step, err := strconv.ParseFloat(n, 32); err != nil || !(step >= 4 && step <= 1e6) {
				return fmt.Errorf("grid=%s: needs a step of 4 to 1000000 points", n)
			}
		default:
			return fmt.Errorf("%q: needs bounds, grid or grid=N", part)
		}
		parts[i] = part
	}
	*l = layoutFlag(strings.Join(parts, ","))
	return nil
}

// limitsFlag is -limits: alone the sandbox's default limits, or limits
// as tsx.ParseLimits reads them (-limits=nodes=1000,call=500ms).
type limitsFlag string

func (l *limitsFlag) String() string   { return string(*l) }
func (l *limitsFlag) IsBoolFlag() bool { return true }

func (l *limitsFlag) Set(v string) error {
	switch v {
	case "true":
		v = "default"
	case "false":
		v = ""
	}
	for _, kv := range strings.Split(v, ",") {
		if k, _, _ := strings.Cut(kv, "="); v != "" && v != "default" &&
			!slices.Contains([]string{"load", "call", "call-total", "stack", "nodes", "timers", "requests", "sounds", "body", "storage", "memory"}, k) {
			return fmt.Errorf("no limit %q: load, call, call-total, stack, nodes, timers, requests, sounds, body, storage or memory", k)
		}
	}
	*l = limitsFlag(v)
	return nil
}

func bindDevFlags(fs *flag.FlagSet) *devFlags {
	f := &devFlags{overrides: bindOverrides(fs)}
	f.target = stringFlag(fs, "target", "`target` to use: native, tui, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android (a device or emulator), miniprogram_wechat, miniprogram_qq, miniprogram_alipay or miniprogram_douyin (default: nsgo.yaml's dev.target)")
	f.hotReload = boolFlag(fs, "hot-reload", "rebuild and restart on changes (default: nsgo.yaml's dev.hot_reload, else true)")
	fs.BoolVar(&f.fps, "fps", false, "start the app with --nsgo-fps, showing the performance monitor")
	fs.IntVar(&f.port, "port", 8080, "the dev server's `port`, which serves the web target; the next free one when taken")
	fs.StringVar(&f.host, "host", "", "the dev server's bind `address` (default: all interfaces; use 127.0.0.1 for loopback only)")
	fs.BoolVar(&f.open, "open", true, "open the web target in the browser")
	fs.StringVar(&f.device, "device", "", "the `device` to run on: an iPhone's name, UDID, serial or ECID, a simulator's name or UDID, or an adb serial or emulator's AVD name (default: the only one connected, or the booted simulator)")
	fs.BoolVar(&f.androidHost, "android-host", false, "run Go on this machine and show the app on the Android device over adb")
	fs.BoolVar(&f.release, "release", false, "build stripped release binaries, as nsgo build does, for measuring with -fps")
	fs.BoolVar(&f.debug, "debug", false, "run the native desktop app under Delve, keeping its debugger address across reloads")
	fs.BoolVar(&f.trace, "trace", false, "print each commit's time, mount items and the writes that caused it from the running app")
	fs.StringVar(&f.profile.kind, "profile", "", "capture a running app's runtime profile: cpu, heap or trace")
	fs.DurationVar(&f.profile.duration, "profile-duration", 10*time.Second, "the capture duration for -profile cpu or trace")
	fs.StringVar(&f.profile.output, "profile-output", "", "the local profile file (default: .nsgo/profiles/cpu.pprof, heap.pprof or execution.trace)")
	fs.BoolVar(&f.highlight, "highlight", false, "outline for a moment each view a commit of the running app changed")
	fs.BoolVar(&f.devtools, "devtools", false, "open DevTools in the browser: the running app's views, state, profiler and a test recorder")
	fs.Var(&f.layout, "layout", "show DevTools' layout audit over the running app: alone every view's boxes, or what follows = joined by commas, bounds, grid (8 points) and grid=N, as -layout=bounds,grid=4")
	fs.BoolVar(&f.auto, "automation", false, "let agents drive the running app over nsgo mcp, on macOS, Windows, Linux, the web, mini programs and Apple devices and simulators: windows, snapshots, presses, typing, menus, alerts and screenshots")
	fs.StringVar(&f.lang, "lang", "", "start the app with --nsgo-lang, in this `locale`, as de or zh-Hant, or a pseudo locale: en-XA accents and lengthens its messages, ar-XB mirrors them")
	fs.StringVar(&f.team, "team", "", "the Apple developer `team` the iOS target signs with (default: the team Xcode last used)")
	fs.StringVar(&f.clip, "clip", "", "with -target ios or ios_simulator, run nsgo.yaml's App Clip alone, opened from this `url`, in place of the app")
	fs.Var(&f.limits, "limits", "run a TSX app sandboxed, as tsx.View runs a package: within the default limits, or those given (`load=10s,call=2s,call-total=0,stack=1000,nodes=200000,timers=10000,requests=32,sounds=256,body=67108864,storage=1048576`), and nsgo.yaml's capabilities")
	return f
}

// options are dev's options of the flags that name no nsgo.yaml value.
func (f *devFlags) options() devOptions {

	return devOptions{apply: f.apply, port: f.port, host: f.host, open: f.open, device: f.device, androidHost: f.androidHost, team: f.team, release: f.release, debug: f.debug, trace: f.trace,
		highlight: f.highlight, devtools: f.devtools, layout: string(f.layout), automation: f.auto, fps: f.fps, lang: f.lang, clip: f.clip, profile: f.profile}
}

func (f *devFlags) apply(p *project) {
	f.overrides.apply(p)
	p.Dev.Limits = string(f.limits)
	f.target.replace(&p.Dev.Target)
	if f.hotReload.set {
		p.Dev.HotReload = &f.hotReload.value
	}
}

// buildFlags are nsgo build's flags; those that name an nsgo.yaml value
// replace it when the user gives them.
type buildFlags struct {
	*overrides
	targets *override[string]
	output  *override[string]
	ldflags *[]string
	open    *boolOverride
	install *boolOverride
	jobs    *override[int]
	channel *override[string]
	notes   string
	run     bool
	embed   bool
	dynamic bool
	device  string
	team    string
	sign    signFlags
}

func bindBuildFlags(fs *flag.FlagSet) *buildFlags {
	f := &buildFlags{overrides: bindOverrides(fs)}
	f.targets = stringFlag(fs, "target", "comma-separated `targets`: all for every platform Go and Xcode build, miniprogram for every mini program, or their names; a bare OS such as windows builds both its architectures (default: nsgo.yaml's build.targets)")
	f.output = stringFlag(fs, "output", "the `output` directory (default: nsgo.yaml's build.output)")
	f.ldflags = stringListFlag(fs, "ldflags", "add app linker `flags` to nsgo.yaml's build.ldflags, as go build's -ldflags takes them; repeat for more")
	f.open = boolFlag(fs, "open", "open the output directory (default: nsgo.yaml's build.open)")
	f.install = boolFlag(fs, "installers", "package macOS, Windows and Linux targets for users to install: a disk image, an NSIS installer, Debian and Arch packages (default: nsgo.yaml's build.installers, else true)")
	f.jobs = intFlag(fs, "jobs", "how many targets build at once, sharing compile workers across them (default: nsgo.yaml's build.jobs, else half the CPUs)")
	f.channel = stringFlag(fs, "channel", "the `channel` the packages are handed out through, which decides how they update: direct, store, steam or none (default: nsgo.yaml's updates.channel)")
	fs.StringVar(&f.notes, "release-notes", "", "a Markdown `file` of the release's notes, which the appcasts carry for the update prompts to show")
	fs.BoolVar(&f.run, "run", false, "start the package built for this machine: the app, the iPhone's, simulator's or Android device's, or the site served")
	fs.StringVar(&f.device, "device", "", "the `device` -run starts the package on: an iPhone's name, UDID, serial or ECID, a simulator's name or UDID, or an adb serial or emulator's AVD name (default: the only one connected, or the booted simulator)")
	fs.StringVar(&f.team, "team", "", "the Apple developer `team` -run signs the iPhone's package with (default: the team Xcode last used)")
	fs.BoolVar(&f.embed, "embed", false, "package the app for iOS and Android apps of their own to embed: an XCFramework and an AAR")
	fs.BoolVar(&f.dynamic, "dynamic", false, "with -embed, make the XCFramework of dynamic frameworks, which the host embeds, in place of static libraries")
	f.sign.bind(fs)
	return f
}

func (f *buildFlags) apply(p *project) {
	f.overrides.apply(p)
	if f.targets.set {
		p.Build.Targets = strings.Split(f.targets.value, ",")
		p.Build.all = slices.Contains(p.Build.Targets, "all")
	}
	if f.output.set {
		p.Build.Output = p.resolve(f.output.value)
	}
	if f.open.set {
		p.Build.Open = &f.open.value
	}
	if f.install.set {
		p.Build.Installers = &f.install.value
	}
	f.jobs.replace(&p.Build.Jobs)
	f.channel.replace(&p.Updates.Channel)
	if f.notes != "" {
		p.Updates.notes, _ = filepath.Abs(f.notes)
	}
	p.Build.LDFlags = append(p.Build.LDFlags, (*f.ldflags)...)
	p.Build.run, p.Build.embed, p.Build.dynamic = f.run, f.embed, f.dynamic
	p.Build.device, p.Build.team = f.device, f.team
	f.sign.apply(p)
}
