package main

import (
	"flag"
	"fmt"
	"strconv"
	"strings"
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
	width, height     *override[int]
}

func bindOverrides(fs *flag.FlagSet) *overrides {
	return &overrides{
		name:    stringFlag(fs, "name", "the app `name` (default: nsgo.yaml's name)"),
		id:      stringFlag(fs, "id", "the bundle `id` (default: nsgo.yaml's id)"),
		version: stringFlag(fs, "version", "the app `version` (default: nsgo.yaml's version)"),
		width:   intFlag(fs, "width", "the window `width` (default: nsgo.yaml's window.width)"),
		height:  intFlag(fs, "height", "the window `height` (default: nsgo.yaml's window.height)"),
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
	o.width.replace(&p.Window.Width)
	o.height.replace(&p.Window.Height)
}

// devFlags are nsgo dev's flags; those that name an nsgo.yaml value replace
// it when the user gives them.
type devFlags struct {
	*overrides
	target    *override[string]
	hotReload *boolOverride
	port      int
	open      bool
	device    string
	team      string
	fps       bool
}

func bindDevFlags(fs *flag.FlagSet) *devFlags {
	f := &devFlags{overrides: bindOverrides(fs)}
	f.target = stringFlag(fs, "target", "`target` to use: native, web, ios, ios_simulator or android (a device or emulator; default: nsgo.yaml's dev.target)")
	f.hotReload = boolFlag(fs, "hot-reload", "rebuild and restart on changes (default: nsgo.yaml's dev.hot_reload)")
	fs.IntVar(&f.port, "port", 8080, "the web target's port")
	fs.BoolVar(&f.open, "open", true, "open the web target in the browser")
	fs.StringVar(&f.device, "device", "", "the `device` the iOS target runs on: a connected iPhone's name, UDID, serial or ECID (default: the only one connected)")
	fs.StringVar(&f.team, "team", "", "the Apple developer `team` the iOS target signs with (default: the team Xcode last used)")
	fs.BoolVar(&f.fps, "fps", false, "show the performance monitor over the app (macOS)")
	return f
}

func (f *devFlags) apply(p *project) {
	f.overrides.apply(p)
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
	open    *boolOverride
}

func bindBuildFlags(fs *flag.FlagSet) *buildFlags {
	f := &buildFlags{overrides: bindOverrides(fs)}
	f.targets = stringFlag(fs, "target", "comma-separated `targets`, or all for every platform Go and Xcode build (default: nsgo.yaml's build.targets)")
	f.output = stringFlag(fs, "output", "the `output` directory (default: nsgo.yaml's build.output)")
	f.open = boolFlag(fs, "open", "open the output directory (default: nsgo.yaml's build.open)")
	return f
}

func (f *buildFlags) apply(p *project) {
	f.overrides.apply(p)
	if f.targets.set {
		p.Build.Targets = strings.Split(f.targets.value, ",")
	}
	if f.output.set {
		p.Build.Output = p.resolve(f.output.value)
	}
	if f.open.set {
		p.Build.Open = &f.open.value
	}
}
