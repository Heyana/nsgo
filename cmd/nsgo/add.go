package main

import (
	"bufio"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"
	"unicode"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// nsgo add is go get with its plan shown first (#2460). It plans on copies of go.mod and go.sum, with the go command of the toolchain the module already selects, no toolchain switch, and module proxies for every module not marked private, so planning runs nothing but the go command. It shows what would change, refuses what nobody chose, such as moving the app onto an NSGo that is not a release, and writes both files only once the plan is confirmed.

// addOptions are nsgo add's flags.
type addOptions struct {
	dryRun    bool   // print the plan and write nothing
	yes       bool   // apply the plan without asking
	list      bool   // list libraries and components from NSGo's index and configured registries
	diff      bool   // print every copied component file in full
	dir       string // destination directory for the one copied component
	skipTests bool   // -tests=false leaves out _test.go files and testdata/
}

// addRequest is a module or component named on nsgo add's command line: its path, the version typed after @ (else the index entry's version or the query upgrade for libraries, or resolved from go.mod/index/upgrade for components), the optional component name, and the bare or @registry/name index name it was resolved from when named by one.
type addRequest struct {
	path, version   string
	typed           bool
	component       string
	fromName        string
	itemVersion     string // version declared in a registry entry for a component item
	registryLicense string // SPDX expression from a registry entry, when added by name
}

func (r addRequest) displayName() string {
	if r.fromName != "" {
		return r.fromName
	}
	if r.component != "" {
		return r.path + "#" + r.component
	}
	return r.path
}

func (r addRequest) displayArg() string {
	if r.fromName != "" {
		if r.typed {
			return r.fromName + "@" + r.version
		}
		return r.fromName
	}
	s := r.path
	if r.typed {
		s += "@" + r.version
	}
	if r.component != "" {
		s += "#" + r.component
	}
	return s
}

// addSpec is a parsed argument to nsgo add: a @registry/name[@version]
// reference, a bare name in NSGo's built-in index, or a direct module[@version][#component] path.
type addSpec struct {
	registry        string
	item            string
	path            string
	version         string
	typed           bool
	component       string
	fromName        string
	itemVersion     string
	registryLicense string
}

// parseAddRequests reads nsgo add's arguments, resolving bare names in NSGo's built-in index.
func parseAddRequests(args []string) ([]addRequest, error) {
	return parseAddRequestsWithIndex(args, builtinRegistry())
}

// parseAddRequestsWithIndex reads nsgo add's arguments, each a bare name in idx or a module[@version][#component].
func parseAddRequestsWithIndex(args []string, idx *registryIndex) ([]addRequest, error) {
	specs, err := parseAddSpecsWithIndex(args, idx)
	if err != nil {
		return nil, err
	}
	reqs := make([]addRequest, 0, len(specs))
	for _, s := range specs {
		if s.registry != "" {
			return nil, fmt.Errorf("add: %q is a registry reference, not a module path", s.registry+"/"+s.item)
		}
		reqs = append(reqs, addRequest{
			path:            s.path,
			version:         s.version,
			typed:           s.typed,
			component:       s.component,
			fromName:        s.fromName,
			itemVersion:     s.itemVersion,
			registryLicense: s.registryLicense,
		})
	}
	return reqs, nil
}

// parseAddSpecsWithIndex reads nsgo add's arguments: module[@version][#component],
// @registry/name[@version], or a bare name in NSGo's built-in index idx.
// Bare names resolve only in idx so custom registries can never shadow bare names (#2461, #2464).
func parseAddSpecsWithIndex(args []string, idx *registryIndex) ([]addSpec, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("add: name a module, library or component: %s add name|@registry/name|module[@version][#component]", nsgoCommand())
	}
	var specs []addSpec
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("add: flags go before the modules: %s add %s %s", nsgoCommand(), arg, args[0])
		}
		if strings.HasPrefix(arg, "@") {
			spec, err := parseRegistryAddSpec(arg)
			if err != nil {
				return nil, err
			}
			if slices.ContainsFunc(specs, func(s addSpec) bool {
				return s.registry == spec.registry && s.item == spec.item
			}) {
				return nil, fmt.Errorf("add: %s/%s is named twice", spec.registry, spec.item)
			}
			specs = append(specs, spec)
			continue
		}
		var s addSpec
		if strings.Contains(arg, "#") {
			spec, comp, _ := strings.Cut(arg, "#")
			if !validComponentName(comp) {
				return nil, fmt.Errorf("add: %q has an invalid component name %q", arg, comp)
			}
			path, version, hasVersion := strings.Cut(spec, "@")
			if hasVersion && version == "" {
				return nil, fmt.Errorf("add: %q is not a module path with an optional @version and #component", arg)
			}
			if err := module.CheckPath(path); err != nil {
				return nil, fmt.Errorf("add: %q: %w", arg, err)
			}
			if hasVersion && version == "none" {
				return nil, fmt.Errorf("add: %s@none would remove %s; add never removes a module", path, path)
			}
			s = addSpec{path: path, version: version, typed: hasVersion, component: comp}
		} else {
			spec, version, hasVersion := strings.Cut(arg, "@")
			if hasVersion && version == "" {
				return nil, fmt.Errorf("add: %q is not a module path or listed name with an optional @version", arg)
			}
			domain, _, _ := strings.Cut(spec, "/")
			if strings.Contains(domain, ".") {
				if err := module.CheckPath(spec); err != nil {
					return nil, fmt.Errorf("add: %q: %w", arg, err)
				}
				if !hasVersion {
					version = "upgrade"
				}
				if version == "none" {
					return nil, fmt.Errorf("add: %s@none would remove %s; add never removes a module", spec, spec)
				}
				s = addSpec{path: spec, version: version, typed: hasVersion}
			} else if isValidRegistryName(spec) {
				item, ok := idx.lookup(spec)
				if !ok {
					return nil, fmt.Errorf("add: unknown name %q; run %s add -list to see listed libraries and components", spec, nsgoCommand())
				}
				if hasVersion && version == "none" {
					return nil, fmt.Errorf("add: %s@none would remove %s; add never removes a module", spec, item.Module)
				}
				if item.Component != "" {
					s = addSpec{
						path:            item.Module,
						version:         version,
						typed:           hasVersion,
						component:       item.Component,
						fromName:        spec,
						itemVersion:     item.Version,
						registryLicense: item.License,
					}
				} else {
					if !hasVersion {
						version = cmp.Or(item.Version, "upgrade")
					}
					s = addSpec{path: item.Module, version: version, typed: hasVersion, fromName: spec, registryLicense: item.License}
				}
			} else {
				return nil, fmt.Errorf("add: %q is not a module path or listed name with an optional @version", arg)
			}
		}
		if slices.ContainsFunc(specs, func(existing addSpec) bool {
			return existing.registry == "" && existing.path == s.path && existing.component == s.component
		}) {
			dup := s.path
			if s.component != "" {
				dup += "#" + s.component
			}
			return nil, fmt.Errorf("add: %s is named twice", dup)
		}
		specs = append(specs, s)
	}
	return specs, nil
}

func parseRegistryAddSpec(arg string) (addSpec, error) {
	regKey, rest, ok := strings.Cut(arg, "/")
	if !ok || rest == "" {
		return addSpec{}, fmt.Errorf("add: %q must be @registry/name[@version]", arg)
	}
	if regKey == "@nsgo" {
		return addSpec{}, fmt.Errorf("add: %q is reserved for NSGo's built-in index", regKey)
	}
	if !isValidRegistryName(regKey[1:]) {
		return addSpec{}, fmt.Errorf("add: %q has invalid registry name %q", arg, regKey)
	}
	if strings.Contains(rest, "#") {
		return addSpec{}, fmt.Errorf("add: %q is not a valid @registry/name[@version]", arg)
	}
	itemName, version, hasVersion := strings.Cut(rest, "@")
	if !isValidRegistryName(itemName) {
		return addSpec{}, fmt.Errorf("add: %q has invalid item name %q", arg, itemName)
	}
	if hasVersion {
		if version == "" {
			return addSpec{}, fmt.Errorf("add: %q is missing a version after @", arg)
		}
		if version == "none" {
			return addSpec{}, fmt.Errorf("add: %s/%s@none would remove it; add never removes a module", regKey, itemName)
		}
	}
	return addSpec{
		registry: regKey,
		item:     itemName,
		version:  version,
		typed:    hasVersion,
	}, nil
}

// resolveAddSpecs resolves any @registry/name specs through nsgo.yaml's
// configured registries, refusing unconfigured registries without modifying
// nsgo.yaml (#2464).
func resolveAddSpecs(ctx context.Context, specs []addSpec, modDir string) ([]addRequest, []*loadedRegistry, error) {
	needsRegistry := slices.ContainsFunc(specs, func(s addSpec) bool { return s.registry != "" })
	if !needsRegistry {
		reqs := make([]addRequest, len(specs))
		for i, s := range specs {
			reqs[i] = addRequest{
				path:            s.path,
				version:         s.version,
				typed:           s.typed,
				component:       s.component,
				fromName:        s.fromName,
				itemVersion:     s.itemVersion,
				registryLicense: s.registryLicense,
			}
		}
		return reqs, nil, nil
	}

	regMap, _, err := loadProjectRegistries(".", modDir)
	if err != nil {
		return nil, nil, fmt.Errorf("add: %w", err)
	}

	loaded := map[string]*loadedRegistry{}
	var used []*loadedRegistry
	reqs := make([]addRequest, 0, len(specs))

	for _, s := range specs {
		if s.registry == "" {
			if slices.ContainsFunc(reqs, func(r addRequest) bool { return r.path == s.path && r.component == s.component }) {
				dup := s.path
				if s.component != "" {
					dup += "#" + s.component
				}
				return nil, nil, fmt.Errorf("add: %s is named twice", dup)
			}
			reqs = append(reqs, addRequest{
				path:            s.path,
				version:         s.version,
				typed:           s.typed,
				component:       s.component,
				fromName:        s.fromName,
				itemVersion:     s.itemVersion,
				registryLicense: s.registryLicense,
			})
			continue
		}
		rc, ok := regMap[s.registry]
		if !ok {
			return nil, nil, fmt.Errorf("add: registry %s is not configured in nsgo.yaml; add it under registries:\n  registries:\n    %q: https://...", s.registry, s.registry)
		}
		lr, ok := loaded[s.registry]
		if !ok {
			lr, err = loadRegistry(ctx, rc)
			if err != nil {
				return nil, nil, fmt.Errorf("add: %w", err)
			}
			loaded[s.registry] = lr
			used = append(used, lr)
		}
		item, ok := lr.Index.lookup(s.item)
		if !ok {
			return nil, nil, fmt.Errorf("add: %s/%s was not found in registry %s (%s); run %s add -list %s to see its items", s.registry, s.item, s.registry, rc.Raw, nsgoCommand(), s.registry)
		}
		if item.Component != "" {
			if slices.ContainsFunc(reqs, func(r addRequest) bool { return r.path == item.Module && r.component == item.Component }) {
				return nil, nil, fmt.Errorf("add: %s#%s is named twice", item.Module, item.Component)
			}
			reqs = append(reqs, addRequest{
				path:            item.Module,
				version:         s.version,
				typed:           s.typed,
				component:       item.Component,
				fromName:        s.registry + "/" + s.item,
				itemVersion:     item.Version,
				registryLicense: item.License,
			})
			continue
		}
		version := s.version
		if !s.typed {
			if item.Version != "" {
				version = item.Version
			} else {
				version = "upgrade"
			}
		}
		if slices.ContainsFunc(reqs, func(r addRequest) bool { return r.path == item.Module && r.component == "" }) {
			return nil, nil, fmt.Errorf("add: %s is named twice", item.Module)
		}
		reqs = append(reqs, addRequest{
			path:            item.Module,
			version:         version,
			typed:           s.typed,
			fromName:        s.registry + "/" + s.item,
			registryLicense: item.License,
		})
	}
	return reqs, used, nil
}

// listAddItems lists items from NSGo's built-in index and every registry
// configured in nsgo.yaml (or only the @registry prefixes named in args),
// filtered by any search words across name, description and tags. It works
// inside or outside a Go module.
func listAddItems(ctx context.Context, idx *registryIndex, args []string, out io.Writer) error {
	var regFilters []string
	var needles []string
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return fmt.Errorf("add: flags go before the filter words: %s add -list %s", nsgoCommand(), arg)
		}
		if strings.HasPrefix(arg, "@") && !strings.Contains(arg, "/") {
			if arg == "@nsgo" {
				return fmt.Errorf("add: %q is reserved for NSGo's built-in index", arg)
			}
			if !isValidRegistryName(arg[1:]) {
				return fmt.Errorf("add: invalid registry name %q", arg)
			}
			if !slices.Contains(regFilters, arg) {
				regFilters = append(regFilters, arg)
			}
			continue
		}
		needles = append(needles, strings.ToLower(arg))
	}

	var modDir string
	if gomod, err := enclosingGoMod("."); err == nil && gomod != "" {
		modDir = filepath.Dir(gomod)
	}
	regMap, regList, err := loadProjectRegistries(".", modDir)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	var targets []registryConfig
	if len(regFilters) > 0 {
		for _, name := range regFilters {
			rc, ok := regMap[name]
			if !ok {
				return fmt.Errorf("add: registry %s is not configured in nsgo.yaml; add it under registries:\n  registries:\n    %q: https://...", name, name)
			}
			targets = append(targets, rc)
		}
	} else {
		targets = regList
	}

	type row struct {
		name, kind, mod, desc string
	}
	var rows []row
	if len(regFilters) == 0 && idx != nil {
		for _, item := range idx.Items {
			if !item.matchesWords(item.Name, needles) {
				continue
			}
			rows = append(rows, row{
				name: item.Name,
				kind: item.kind(),
				mod:  item.Module,
				desc: item.Description,
			})
		}
	}
	for _, rc := range targets {
		lr, err := loadRegistry(ctx, rc)
		if err != nil {
			return fmt.Errorf("add: %w", err)
		}
		if lr.FromCache {
			warn("registry %s (%s): offline; using cached copy (%s ago)", rc.Name, rc.Raw, formatCacheAge(lr.CacheAge))
		}
		for _, item := range lr.Index.Items {
			display := rc.Name + "/" + item.Name
			if !item.matchesWords(display, needles) {
				continue
			}
			rows = append(rows, row{
				name: display,
				kind: item.kind(),
				mod:  item.Module,
				desc: item.Description,
			})
		}
	}
	slices.SortFunc(rows, func(a, b row) int {
		return cmp.Compare(a.name, b.name)
	})
	if len(rows) == 0 {
		return nil
	}
	var wName, wKind, wMod int
	for _, r := range rows {
		wName = max(wName, len(r.name))
		wKind = max(wKind, len(r.kind))
		wMod = max(wMod, len(r.mod))
	}
	var b bytes.Buffer
	for _, r := range rows {
		fmt.Fprintf(&b, "%-*s  %-*s  %-*s  %s\n", wName, r.name, wKind, r.kind, wMod, r.mod, r.desc)
	}
	_, err = out.Write(b.Bytes())
	return err
}

// addModules runs nsgo add using NSGo's built-in index and any registries configured in nsgo.yaml: -list prints matching items without needing a module; otherwise it plans reqs in the module holding the working directory, writes the plan to out, and applies it with -y, or once ask, which is nil when nobody can be asked, says yes.
func addModules(ctx context.Context, o addOptions, args []string, out io.Writer, ask func(context.Context) (bool, error)) error {
	return addModulesWithIndex(ctx, o, args, builtinRegistry(), out, ask)
}

func addModulesWithIndex(ctx context.Context, o addOptions, args []string, idx *registryIndex, out io.Writer, ask func(context.Context) (bool, error)) error {
	if o.list {
		if o.dryRun || o.yes || o.diff || o.dir != "" || o.skipTests {
			return errors.New("add: -list cannot be combined with -dry-run, -y, -diff, -dir or -tests=false")
		}
		return listAddItems(ctx, idx, args, out)
	}
	specs, err := parseAddSpecsWithIndex(args, idx)
	if err != nil {
		return err
	}
	if o.dir != "" && len(specs) != 1 {
		return errors.New("add: -dir applies to one component")
	}
	// An interrupt kills the go commands of planning, whose errors would say only that they were killed.
	planning := func(err error) error {
		if ctx.Err() != nil {
			return fmt.Errorf("add: interrupted while planning, so nothing was written: %w", ctx.Err())
		}
		return err
	}
	m, err := findAddModule(ctx)
	if err != nil {
		return planning(err)
	}
	// The plan says what planning found, so the step leaves no line of its own.
	s := begin("planning %s", strings.Join(args, " "))
	reqs, usedRegistries, err := resolveAddSpecs(withStep(ctx, s), specs, m.dir)
	if err != nil {
		s.remove()
		return planning(err)
	}
	hasComp := slices.ContainsFunc(reqs, func(r addRequest) bool { return r.component != "" })
	if hasComp {
		return addCopyComponentsWithModule(ctx, m, o, reqs, usedRegistries, s, out, ask)
	}
	if o.dir != "" || o.diff || o.skipTests {
		s.remove()
		return errors.New("add: -dir, -diff and -tests=false apply only when copying a component")
	}
	p, err := m.plan(withStep(ctx, s), reqs)
	s.remove()
	if err != nil {
		return planning(err)
	}
	p.registries = usedRegistries
	if err := p.write(out); err != nil {
		return err
	}
	if err := p.refusal(); err != nil {
		return err
	}
	if !p.changes() || o.dryRun {
		return nil
	}
	if !o.yes {
		if ask == nil {
			return fmt.Errorf("add: the plan was not applied, as nobody can confirm it without a terminal: run %s add -y %s to apply it", nsgoCommand(), strings.Join(args, " "))
		}
		ok, err := ask(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("add: the plan was not applied")
		}
	}
	if err := p.apply(ctx); err != nil {
		return err
	}
	for _, item := range p.items {
		done("require %s %s", item.path, item.selected)
	}
	if p.m.vendored() {
		detail("next: go mod vendor")
	}
	for _, item := range p.items {
		for _, imp := range item.importPaths() {
			detail("next: import %q", imp)
		}
	}
	return nil
}

// askToApply asks on stderr whether to apply the plan and reads the answer, y or yes, from standard input, a terminal.
func askToApply(ctx context.Context) (bool, error) {
	return askToApplyFor(ctx, "add")
}

// askToApplyFor asks on stderr whether to apply cmdName's plan and reads the answer, y or yes, from standard input, a terminal.
func askToApplyFor(ctx context.Context, cmdName string) (bool, error) {
	fmt.Fprint(os.Stderr, "Apply these changes? [y/N] ")
	answer := make(chan string, 1)
	in := os.Stdin
	go func() {
		line, _ := bufio.NewReader(in).ReadString('\n')
		answer <- line
	}()
	select {
	case <-ctx.Done():
		fmt.Fprintln(os.Stderr)
		return false, fmt.Errorf("%s: interrupted, so the plan was not applied: %w", cmdName, ctx.Err())
	case line := <-answer:
		a := strings.ToLower(strings.TrimSpace(line))
		return a == "y" || a == "yes", nil
	}
}

// addModule is the module nsgo add and nsgo remove plan in, and how their go commands run.
type addModule struct {
	dir string // the module's root, which holds go.mod
	cwd string // the working directory where nsgo add was invoked
	// goCmd is the go command of the toolchain the module selects, which runs with GOTOOLCHAIN=local, so planning downloads and runs no other toolchain.
	goCmd string
	// env is every planning go command's environment: the user's, with GOWORK=off, GOTOOLCHAIN=local, the toolchain's GOROOT and GOPROXY up to its first direct, which becomes off, so only modules GOPRIVATE or GONOPROXY marks are fetched from their own hosts.
	env []string
	// listEnv is env without forcing GOWORK=off when the module is under a go.work, so target listing through -overlay runs in the workspace as nsgo build does.
	listEnv []string
	// goFlags is the module's GOFLAGS from go env.
	goFlags string
	// workspace is the go.work the module is under, which planning leaves out, as GOWORK=off go get does.
	workspace string
	// sumDB, noSumDB and noProxy are GOSUMDB, GONOSUMDB and GONOPROXY, the last two GOPRIVATE when unset.
	sumDB, noSumDB, noProxy string
}

// findAddModule finds the module holding the working directory, its toolchain's go command, and the settings planning keeps.
func findAddModule(ctx context.Context) (*addModule, error) {
	return findModule(ctx, "add")
}

// findModule finds the module holding the working directory, its toolchain's go command, and the settings planning keeps, prefixing errors with cmdName.
func findModule(ctx context.Context, cmdName string) (*addModule, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cmdName, err)
	}
	// The one go command without GOWORK=off, which reports where the module and its workspace are and, with the local toolchain, switches to none a go.work names.
	found, err := goEnvValues(ctx, "go", ".", append(os.Environ(), "GOTOOLCHAIN=local"), "GOMOD", "GOWORK")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cmdName, err)
	}
	if found["GOMOD"] == "" || found["GOMOD"] == os.DevNull {
		return nil, fmt.Errorf("%s: not in a module: run %s %s in the module that takes the library", cmdName, nsgoCommand(), cmdName)
	}
	m := &addModule{dir: filepath.Dir(found["GOMOD"]), cwd: cwd}
	if w := found["GOWORK"]; w != "off" {
		m.workspace = w
	}
	alone := append(os.Environ(), "GOWORK=off")
	// The go found on PATH reports the GOROOT of the toolchain the module selects, switching to it as any command in the module would.
	vars, err := goEnvValues(ctx, "go", m.dir, alone, "GOROOT", "GOPROXY", "GOSUMDB", "GONOSUMDB", "GONOPROXY", "GOFLAGS")
	if err != nil {
		return nil, fmt.Errorf("%s: %w", cmdName, err)
	}
	m.goCmd = filepath.Join(vars["GOROOT"], "bin", "go")
	if runtime.GOOS == "windows" {
		m.goCmd += ".exe"
	}
	if _, err := os.Stat(m.goCmd); err != nil {
		return nil, fmt.Errorf("%s: the module's toolchain has no go command: %w", cmdName, err)
	}
	// GOROOT is the toolchain's own, as the go command sets it for a toolchain it switches to: a GOROOT the user exported, such as another toolchain's, would have this go command read that tree's standard library.
	common := []string{"GOTOOLCHAIN=local", "GOROOT=" + vars["GOROOT"], "GOPROXY=" + initModuleProxy(vars["GOPROXY"])}
	m.env = append(alone, common...)
	if m.workspace != "" {
		m.listEnv = append(os.Environ(), common...)
	} else {
		m.listEnv = m.env
	}
	m.goFlags = vars["GOFLAGS"]
	m.sumDB, m.noSumDB, m.noProxy = vars["GOSUMDB"], vars["GONOSUMDB"], vars["GONOPROXY"]
	return m, nil
}

// goEnvValues is what go env -json prints of names, run as goCmd in dir with env.
func goEnvValues(ctx context.Context, goCmd, dir string, env []string, names ...string) (map[string]string, error) {
	cmd := exec.CommandContext(ctx, goCmd, append([]string{"env", "-json"}, names...)...)
	cmd.Dir, cmd.Env = dir, env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go env: %w\n%s", err, goMessages(stderr.String()))
	}
	values := map[string]string{}
	if err := json.Unmarshal(out, &values); err != nil {
		return nil, fmt.Errorf("go env: %w", err)
	}
	return values, nil
}

// vendored reports whether the module vendors its dependencies, which go mod vendor then updates.
func (m *addModule) vendored() bool {
	info, err := os.Stat(filepath.Join(m.dir, "vendor"))
	return err == nil && info.IsDir()
}

// goOutput runs the module's go command with args in its root and returns its standard output, or an error holding what it printed on standard error.
func (m *addModule) goOutput(ctx context.Context, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, m.goCmd, args...)
	cmd.Dir, cmd.Env = m.dir, m.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, &addGoError{args: args, err: err, messages: goMessages(stderr.String())}
	}
	return out, nil
}

// addGoError is a go command of planning that failed, with the messages it printed.
type addGoError struct {
	args     []string
	err      error
	messages string
}

func (e *addGoError) Error() string {
	return fmt.Sprintf("go %s: %v\n%s", e.args[0], e.err, e.messages)
}

// goMessages is what a go command printed on standard error, without its progress lines and its go: prefixes.
func goMessages(stderr string) string {
	var lines []string
	for _, l := range strings.Split(strings.TrimSpace(stderr), "\n") {
		if strings.HasPrefix(l, "go: downloading ") || strings.HasPrefix(l, "go: finding ") {
			continue
		}
		lines = append(lines, strings.TrimPrefix(l, "go: "))
	}
	return strings.Join(lines, "\n")
}

// goListModule is what go list -m -json prints of a module, as far as the plan reads it.
type goListModule struct {
	Path, Version, Dir string
	Time               *time.Time
	Update             *goListModule
	Replace            *goListModule
	Retracted          []string
	Deprecated         string
	Error              *struct{ Err string }
}

// listModules runs go list -m -e -json on paths with flags, and returns what it printed of each path.
func (m *addModule) listModules(ctx context.Context, flags []string, paths []string) (map[string]goListModule, error) {
	out, err := m.goOutput(ctx, append(append([]string{"list", "-m", "-e", "-json", "-mod=readonly"}, flags...), paths...)...)
	if err != nil {
		return nil, err
	}
	listed := map[string]goListModule{}
	for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
		var mod goListModule
		if err := dec.Decode(&mod); err != nil {
			return nil, fmt.Errorf("go list -m: %w", err)
		}
		listed[mod.Path] = mod
	}
	return listed, nil
}

// packageNames are the names of the packages at paths, the root packages of the modules there, with -find, which loads none of their imports; a path with no package has none.
func (m *addModule) packageNames(ctx context.Context, flags []string, paths []string) (map[string]string, error) {
	out, err := m.goOutput(ctx, append(append([]string{"list", "-find", "-e", "-json=ImportPath,Name", "-mod=readonly"}, flags...), paths...)...)
	if err != nil {
		return nil, err
	}
	names := map[string]string{}
	for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
		var pkg struct{ ImportPath, Name string }
		if err := dec.Decode(&pkg); err != nil {
			return nil, fmt.Errorf("go list: %w", err)
		}
		names[pkg.ImportPath] = pkg.Name
	}
	return names, nil
}

// addPlan is what nsgo add would write: go.mod and go.sum as go get writes them for the requests, with what each request resolved to, and why any of it is refused.
type addPlan struct {
	m          *addModule
	app        *addAppConfig
	registries []*loadedRegistry // custom registries resolved for this plan, in order
	items      []addItem
	already    []module.Version // requests go.mod required already, which change nothing

	oldMod, newMod, oldSum, newSum []byte
	hadSum, hasSum                 bool
	before, after                  *modfile.File
}

// addItem is a request as planned: the version it takes, the one the module had selected before, what go list says of it and its optional nsgo-module.yaml.
type addItem struct {
	addRequest
	selected, initial string
	listed            goListModule
	modDir            string
	packageName       string
	manifest          *moduleManifest
	inspection        *addItemInspection
}

// importable reports whether the item's module has a root package an app imports.
func (item addItem) importable() bool {
	return item.packageName != "" && item.packageName != "main"
}

// importPaths returns the Go package import paths the plan recommends: those
// declared by nsgo-module.yaml's import key when present, else the root package
// when it is importable.
func (item addItem) importPaths() []string {
	if item.manifest != nil && item.manifest.hasImport {
		return item.manifest.importPaths(item.path)
	}
	if item.importable() {
		return []string{item.path}
	}
	return nil
}

// tooNewGo finds what the go command's message says needs a newer Go, and the version it needs.
var tooNewGo = regexp.MustCompile(`(\S+) requires go >= (\S+) \(running go`)

// plan works out what go get writes for reqs, on copies of go.mod and go.sum in a temporary directory: the files are never written.
func (m *addModule) plan(ctx context.Context, reqs []addRequest) (*addPlan, error) {
	p := &addPlan{m: m}
	var err error
	if p.app, err = loadAddAppConfig(m.cwd, m.dir); err != nil {
		return nil, err
	}
	if p.oldMod, err = os.ReadFile(filepath.Join(m.dir, "go.mod")); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	p.oldSum, err = os.ReadFile(filepath.Join(m.dir, "go.sum"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("add: %w", err)
	}
	p.hadSum = err == nil
	if p.before, err = modfile.Parse(filepath.Join(m.dir, "go.mod"), p.oldMod, nil); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	if p.before.Module == nil {
		return nil, fmt.Errorf("add: %s has no module line", filepath.Join(m.dir, "go.mod"))
	}
	p.newMod, p.newSum, p.hasSum, p.after = p.oldMod, p.oldSum, p.hadSum, p.before

	var get []addRequest
	for _, r := range reqs {
		if v := requiredVersion(p.before, r.path); v != "" && !r.typed {
			p.already = append(p.already, module.Version{Path: r.path, Version: v})
			continue
		}
		get = append(get, r)
	}
	if len(get) == 0 {
		return p, nil
	}

	tmp, err := os.MkdirTemp("", "nsgo-add-")
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	defer os.RemoveAll(tmp)
	planned := filepath.Join(tmp, "go.mod")
	if err := os.WriteFile(planned, p.oldMod, 0o644); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	if p.hadSum {
		if err := os.WriteFile(filepath.Join(tmp, "go.sum"), p.oldSum, 0o644); err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}
	}
	// -modfile reads and writes the copies, go.sum the one beside go.mod's copy, and the module's own files stay as they are.
	flags := []string{"-modfile=" + planned}
	paths := make([]string, len(get))
	args := append([]string{"get"}, flags...)
	for i, r := range get {
		paths[i] = r.path
		args = append(args, r.path+"@"+r.version)
	}

	stage(ctx, "reading the module's requirements")
	// The versions the module selects now tell a downgrade; a module whose requirements do not load yet leaves them unknown, and go get reports why.
	initial, _ := m.listModules(ctx, flags, paths)

	stage(ctx, "go get")
	if _, err := m.goOutput(ctx, args...); err != nil {
		var goErr *addGoError
		if !errors.As(err, &goErr) {
			return nil, fmt.Errorf("add: %w", err)
		}
		// Modules, named module@version, that need a newer Go, the newest of which go get go@ raises the module to; the module's own go line is its toolchain's to meet.
		needed := ""
		for _, match := range tooNewGo.FindAllStringSubmatch(goErr.messages, -1) {
			if strings.Contains(match[1], "@") && (needed == "" || semver.Compare("v"+match[2], "v"+needed) > 0) {
				needed = match[2]
			}
		}
		if needed != "" {
			return nil, fmt.Errorf("add: %s\nto move this module to Go %s on purpose, run go get go@%s first", goErr.messages, needed, needed)
		}
		if strings.Contains(goErr.messages, "GOPROXY=off") {
			return nil, fmt.Errorf("add: go get failed\n%s\nnsgo add fetches modules through the proxies GOPROXY lists before direct, and from their own hosts only those GOPRIVATE or GONOPROXY marks", goErr.messages)
		}
		return nil, fmt.Errorf("add: go get failed\n%s", goErr.messages)
	}
	if p.newMod, err = os.ReadFile(planned); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	p.newSum, err = os.ReadFile(filepath.Join(tmp, "go.sum"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("add: %w", err)
	}
	p.hasSum = err == nil
	if p.after, err = modfile.Parse(filepath.Join(m.dir, "go.mod"), p.newMod, nil); err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}

	stage(ctx, "reading what go get chose")
	// List root packages first so the module cache holds each selected module's
	// files before go list -m reports its Dir.
	names, err := m.packageNames(ctx, flags, paths)
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	listed, err := m.listModules(ctx, append(flags, "-u", "-retracted"), paths)
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}
	currentNSGo := moduleManifestNSGoVersion()
	for _, r := range get {
		item := addItem{addRequest: r, listed: listed[r.path], packageName: names[r.path]}
		item.selected = item.listed.Version
		item.initial = requiredVersion(p.before, r.path)
		if before, ok := initial[r.path]; ok && before.Error == nil {
			item.initial = before.Version
		}
		if item.listed.Error == nil {
			modDir := item.listed.Dir
			if item.listed.Replace != nil && item.listed.Replace.Dir != "" {
				modDir = item.listed.Replace.Dir
			}
			if modDir == "" && item.selected != "" {
				if out, err := m.goOutput(ctx, "mod", "download", "-json", item.path+"@"+item.selected); err == nil {
					var dl struct{ Dir string }
					if json.Unmarshal(out, &dl) == nil {
						modDir = dl.Dir
					}
				}
			}
			item.modDir = modDir
			if modDir != "" {
				manifest, err := readModuleManifest(modDir, item.path, currentNSGo)
				if err != nil {
					if strings.HasPrefix(err.Error(), item.path+" ") {
						return nil, fmt.Errorf("add: %w", err)
					}
					return nil, fmt.Errorf("add: %s: %w", item.path, err)
				}
				if manifest != nil {
					if err := m.validateModuleManifestPackages(ctx, flags, item.path, modDir, manifest); err != nil {
						return nil, fmt.Errorf("add: %s: %w", item.path, err)
					}
					item.manifest = manifest
				}
			}
		}
		p.items = append(p.items, item)
	}
	if err := p.inspect(ctx, tmp); err != nil {
		return nil, err
	}
	return p, nil
}

// requiredVersion is the version f's require line for path names, or empty.
func requiredVersion(f *modfile.File, path string) string {
	version := ""
	for _, r := range f.Require {
		if r.Mod.Path == path {
			version = r.Mod.Version
		}
	}
	return version
}

// changes reports whether applying the plan writes anything.
func (p *addPlan) changes() bool {
	return !bytes.Equal(p.oldMod, p.newMod) || p.hadSum != p.hasSum || !bytes.Equal(p.oldSum, p.newSum)
}

// refusal is why the plan cannot be applied, -y or not, naming for each reason the command that makes the change on purpose, or nil.
func (p *addPlan) refusal() error {
	var reasons []string
	for _, item := range p.items {
		if item.listed.Error != nil {
			reason := item.listed.Error.Err
			if owner := providingModule(item.path, p.after); owner != "" && owner != item.path {
				reason = fmt.Sprintf("%s is a package of the module %s, not a module: %s add %s", item.path, owner, nsgoCommand(), owner)
			}
			reasons = append(reasons, reason)
			continue
		}
		if item.fromName != "" && !item.typed && module.IsPseudoVersion(item.selected) {
			reasons = append(reasons, fmt.Sprintf("%s has no release; add its module by path with a version: %s add %s@%s", item.fromName, nsgoCommand(), item.path, item.selected))
			continue
		}
		if item.initial != "" && semver.Compare(item.selected, item.initial) < 0 && !(item.typed && item.version == item.selected) {
			reasons = append(reasons, fmt.Sprintf("%s@%s is %s, below the %s this module has: %s add %s@%s lowers it on purpose", item.path, item.version, item.selected, item.initial, nsgoCommand(), item.path, item.selected))
		}
		if item.inspection != nil {
			for _, reason := range item.inspection.refusals {
				if !slices.Contains(reasons, reason) {
					reasons = append(reasons, reason)
				}
			}
		}
	}
	old, moved := requiredVersion(p.before, nsgoModule), requiredVersion(p.after, nsgoModule)
	if moved != old && semver.Prerelease(moved) != "" {
		kind := "a pre-release"
		if module.IsPseudoVersion(moved) {
			kind = "a commit with no release"
		}
		reasons = append(reasons, fmt.Sprintf("the plan moves %s from %s to %s, %s: to move to it on purpose, run go get %s@%s first", nsgoModule, cmp.Or(old, "none"), moved, kind, nsgoModule, moved))
	}
	if len(reasons) == 0 {
		return nil
	}
	return errors.New("add: " + strings.Join(reasons, "\n"))
}

// providingModule is the module of f's requirements whose path is the longest prefix of the package path pkg, or empty.
func providingModule(pkg string, f *modfile.File) string {
	best := ""
	for _, r := range f.Require {
		if (pkg == r.Mod.Path || strings.HasPrefix(pkg, r.Mod.Path+"/")) && len(r.Mod.Path) > len(best) {
			best = r.Mod.Path
		}
	}
	return best
}

// codeHosts are the hosts whose module paths name an account after the host, as github.com/<account>/<repository> does.
var codeHosts = []string{"bitbucket.org", "codeberg.org", "git.sr.ht", "gitee.com", "github.com", "gitlab.com"}

// moduleOwner is who publishes the module at path, as its path says: the host, and on a code host the account there too.
func moduleOwner(path string) string {
	host, rest, _ := strings.Cut(path, "/")
	if account, _, _ := strings.Cut(rest, "/"); account != "" && slices.Contains(codeHosts, host) {
		return host + "/" + account
	}
	return host
}

// write prints the plan: any custom registries used, the requests go.mod required already, then a block for each module planned and one for the changes to this module's go.mod and go.sum.
func (p *addPlan) write(w io.Writer) error {
	var b bytes.Buffer
	for _, lr := range p.registries {
		fmt.Fprintf(&b, "%s\n", lr.headerLine())
	}
	if len(p.registries) > 0 && p.changes() {
		b.WriteString("\n")
	}
	for _, v := range p.already {
		fmt.Fprintf(&b, "%s %s is already in go.mod\n", v.Path, v.Version)
	}
	if !p.changes() {
		for _, item := range p.items {
			if item.listed.Error == nil {
				fmt.Fprintf(&b, "%s %s is already in go.mod\n", item.path, item.selected)
			}
		}
		_, err := w.Write(b.Bytes())
		return err
	}
	if len(p.already) > 0 {
		b.WriteString("\n")
	}
	row := func(label, value string) { fmt.Fprintf(&b, "  %-10s %s\n", label, value) }
	rows := func(label string, values []string) {
		for i, v := range values {
			if i > 0 {
				label = ""
			}
			row(label, v)
		}
	}
	owners := map[string]bool{moduleOwner(p.before.Module.Mod.Path): true}
	for _, r := range p.before.Require {
		owners[moduleOwner(r.Mod.Path)] = true
	}
	for _, item := range p.items {
		fmt.Fprintf(&b, "%s\n", item.path)
		if item.listed.Error != nil {
			row("error", item.listed.Error.Err)
			b.WriteString("\n")
			continue
		}
		version := item.selected
		if item.listed.Time != nil {
			version += ", published " + item.listed.Time.UTC().Format(time.DateOnly)
		}
		if item.initial != "" && semver.Compare(item.selected, item.initial) < 0 {
			version += ", a downgrade from " + item.initial
		}
		if item.listed.Update != nil {
			version += "; " + item.listed.Update.Version + " is newer"
		}
		row("module", item.path+" "+version)
		owner := moduleOwner(item.path)
		if !owners[owner] {
			owner += ", the first module from it in go.mod"
		}
		row("owner", owner)
		if module.IsPseudoVersion(item.selected) {
			row("untagged", item.selected+" is a commit with no release tag")
		}
		for _, why := range item.listed.Retracted {
			row("retracted", shortMessage(why, "retracted by module author"))
		}
		if item.listed.Deprecated != "" {
			row("deprecated", shortMessage(item.listed.Deprecated, "deprecated by module author"))
		}
		if r := item.listed.Replace; r != nil {
			row("replaced", "go.mod replaces it with "+strings.TrimSpace(r.Path+" "+r.Version)+", which builds use instead")
		}
		var targetLines []string
		if p.app != nil {
			if missing := item.manifest.missingTargets(p.app.appTargets); len(missing) > 0 {
				targetLines = append(targetLines, "lacks "+strings.Join(missing, ", "))
			}
		}
		if insp := item.inspection; insp != nil {
			var licenseLines []string
			for _, m := range insp.modules {
				licenseLines = append(licenseLines, formatModuleLicenseLine(m))
			}
			for _, n := range insp.natives {
				if line := formatNativePolicyLine(n); line != "" {
					licenseLines = append(licenseLines, line)
				}
			}
			rows("licenses", licenseLines)
			for _, tr := range insp.targets {
				targetLines = append(targetLines, formatTargetLines(tr)...)
			}
		}
		rows("targets", targetLines)
		if comps := item.manifest.componentNames(); len(comps) > 0 {
			lines := make([]string, len(comps))
			cmd := nsgoCommand()
			for i, name := range comps {
				lines[i] = fmt.Sprintf("%s: %s add %s#%s", name, cmd, item.path, name)
			}
			rows("components", lines)
		}
		if imports := item.importPaths(); len(imports) > 0 {
			lines := make([]string, len(imports))
			for i, imp := range imports {
				lines[i] = fmt.Sprintf("import %q", imp)
			}
			rows("next", lines)
		}
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "%s\n", p.before.Module.Mod.Path)
	rows("go.mod", p.goModChanges())
	if sum := p.goSumChange(); sum != "" {
		row("go.sum", sum)
	}
	rows("tools", p.toolChanges())
	if line := p.private(); line != "" {
		row("private", line)
	}
	if line := p.checksums(); line != "" {
		row("checksums", line)
	}
	if p.m.workspace != "" {
		row("workspace", "go.mod is planned for this module alone, as GOWORK=off go get writes it, not with "+relTo(p.m.dir, p.m.workspace))
	}
	if p.m.vendored() {
		row("next", "go mod vendor")
	}
	for _, item := range p.items {
		if lines := item.manifest.formattedNotes(); len(lines) > 0 {
			fmt.Fprintf(&b, "\nFrom %s, the author's notes:\n", item.path)
			for _, l := range lines {
				if l == "" {
					b.WriteString("  |\n")
				} else {
					fmt.Fprintf(&b, "  | %s\n", l)
				}
			}
		}
	}
	_, err := w.Write(b.Bytes())
	return err
}

// shortMessage is a retraction's rationale or a deprecation notice, text from a go.mod nobody vetted, as the go command prints it (modload.ShortMessage): its first line, trimmed, else def, or a placeholder when it is longer than 500 bytes or holds a character a terminal does not print, such as an escape that would hide the rest of the plan. go list -json, which the plan reads, gives the text as go.mod has it.
func shortMessage(msg, def string) string {
	msg, _, _ = strings.Cut(msg, "\n")
	if msg = strings.TrimSpace(msg); msg == "" {
		return def
	}
	if len(msg) > 500 {
		return "(message omitted: too long)"
	}
	for _, r := range msg {
		if !unicode.IsGraphic(r) && !unicode.IsSpace(r) {
			return "(message omitted: contains non-printable characters)"
		}
	}
	return msg
}

// relTo is path relative to dir when it can be, else path.
func relTo(dir, path string) string {
	if r, err := filepath.Rel(dir, path); err == nil {
		return filepath.ToSlash(r)
	}
	return path
}

// goModChanges are the plan's lines of go.mod that change: the go and toolchain lines, then each requirement added, moved or removed, the requests' first.
func (p *addPlan) goModChanges() []string {
	var lines []string
	directive := func(name, old, new string) {
		switch {
		case old == new:
		case old == "":
			lines = append(lines, "+ "+name+" "+new)
		case new == "":
			lines = append(lines, "- "+name+" "+old)
		default:
			lines = append(lines, "~ "+name+" "+old+" => "+new)
		}
	}
	goLine := func(f *modfile.File) string {
		if f.Go == nil {
			return ""
		}
		return f.Go.Version
	}
	toolchain := func(f *modfile.File) string {
		if f.Toolchain == nil {
			return ""
		}
		return f.Toolchain.Name
	}
	directive("go", goLine(p.before), goLine(p.after))
	directive("toolchain", toolchain(p.before), toolchain(p.after))

	before, after := requirements(p.before), requirements(p.after)
	var paths []string
	for path := range before {
		paths = append(paths, path)
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			paths = append(paths, path)
		}
	}
	requested := func(path string) int {
		return slices.IndexFunc(p.items, func(item addItem) bool { return item.path == path })
	}
	slices.SortFunc(paths, func(a, b string) int {
		ra, rb := requested(a), requested(b)
		switch {
		case ra >= 0 && rb >= 0:
			return cmp.Compare(ra, rb)
		case ra >= 0:
			return -1
		case rb >= 0:
			return 1
		}
		return cmp.Compare(a, b)
	})
	for _, path := range paths {
		old, hadOld := before[path]
		new, hasNew := after[path]
		switch {
		case !hadOld:
			lines = append(lines, "+ require "+path+" "+new.version+new.comment())
		case !hasNew:
			lines = append(lines, "- require "+path+" "+old.version)
		case old.version != new.version:
			line := "~ require " + path + " " + old.version + " => " + new.version
			switch {
			case semver.Compare(new.version, old.version) < 0:
				line += ", a downgrade"
			case path == nsgoModule && semver.Prerelease(new.version) == "":
				line += ", a release"
			}
			lines = append(lines, line)
		case old.indirect != new.indirect:
			lines = append(lines, "~ require "+path+" "+new.version+new.comment()+", was"+old.comment())
		}
	}
	return lines
}

// requirement is a require line's version and whether it is marked // indirect.
type requirement struct {
	version  string
	indirect bool
}

// comment is the comment go.mod writes after the requirement.
func (r requirement) comment() string {
	if r.indirect {
		return " // indirect"
	}
	return ""
}

// requirements are f's require lines by module path.
func requirements(f *modfile.File) map[string]requirement {
	reqs := map[string]requirement{}
	for _, r := range f.Require {
		reqs[r.Mod.Path] = requirement{version: r.Mod.Version, indirect: r.Indirect}
	}
	return reqs
}

// goSumChange counts the lines the plan adds to go.sum and removes from it.
func (p *addPlan) goSumChange() string {
	return formatGoSumChange(p.oldSum, p.newSum)
}

// formatGoSumChange counts the lines added to go.sum and removed from it.
func formatGoSumChange(oldSum, newSum []byte) string {
	added, removed := goSumDiff(oldSum, newSum)
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "+ "+lineCount(len(added)))
	}
	if len(removed) > 0 {
		parts = append(parts, "- "+lineCount(len(removed)))
	}
	return strings.Join(parts, ", ")
}

// lineCount is n lines, as the plan counts them.
func lineCount(n int) string {
	if n == 1 {
		return "1 line"
	}
	return fmt.Sprintf("%d lines", n)
}

// goSumDiff is the lines of go.sum's new content that its old one lacks, and the other way round.
func goSumDiff(old, new []byte) (added, removed []string) {
	lines := func(data []byte) map[string]bool {
		set := map[string]bool{}
		for _, l := range strings.Split(string(data), "\n") {
			if l = strings.TrimSpace(l); l != "" {
				set[l] = true
			}
		}
		return set
	}
	before, after := lines(old), lines(new)
	for l := range after {
		if !before[l] {
			added = append(added, l)
		}
	}
	for l := range before {
		if !after[l] {
			removed = append(removed, l)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	return added, removed
}

// toolChanges says, for each tool line whose module the plan moves, which version go tool then runs.
func (p *addPlan) toolChanges() []string {
	return modToolChanges(p.before, p.after)
}

// modToolChanges says, for each tool line in after whose module moved between before and after, which version go tool then runs.
func modToolChanges(before, after *modfile.File) []string {
	var lines []string
	for _, t := range after.Tool {
		mod := providingModule(t.Path, after)
		if mod == "" {
			continue
		}
		old, new := toolVersionOf(before, mod), toolVersionOf(after, mod)
		if old == new {
			continue
		}
		line := fmt.Sprintf("go tool %s then runs %s", toolCommandName(t.Path), new)
		if t.Path == nsgoTool {
			line += ", and so does an installed nsgo here"
		}
		lines = append(lines, line)
	}
	return lines
}

// toolVersionOf is the version of mod that a tool of f's runs, as moduleNSGoVersion reads it: a replacement's, its directory for one by a directory, else the required one.
func toolVersionOf(f *modfile.File, mod string) string {
	required := requiredVersion(f, mod)
	for _, r := range f.Replace {
		if r.Old.Path == mod && (r.Old.Version == "" || r.Old.Version == required) {
			return cmp.Or(r.New.Version, r.New.Path)
		}
	}
	return required
}

// fetched are the modules whose hashes the plan adds to go.sum, the ones planning fetched.
func (p *addPlan) fetched() []string {
	return fetchedModules(p.oldSum, p.newSum)
}

// fetchedModules are the module paths whose hashes newSum adds over oldSum.
func fetchedModules(oldSum, newSum []byte) []string {
	added, _ := goSumDiff(oldSum, newSum)
	var paths []string
	for _, l := range added {
		if path, _, _ := strings.Cut(l, " "); !slices.Contains(paths, path) {
			paths = append(paths, path)
		}
	}
	return paths
}

// private names the fetched modules that came from their own hosts rather than a module proxy, as GONOPROXY, or GOPRIVATE, marks them.
func (p *addPlan) private() string {
	return privateModules(p.m.noProxy, p.fetched())
}

func privateModules(noProxy string, fetched []string) string {
	var paths []string
	for _, path := range fetched {
		if module.MatchPrefixPatterns(noProxy, path) {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return ""
	}
	return strings.Join(paths, ", ") + ": from their own hosts, as GONOPROXY or GOPRIVATE says"
}

// checksums names the fetched modules whose hashes go.sum takes without the checksum database checking them.
func (p *addPlan) checksums() string {
	return checksumNotice(p.m.sumDB, p.m.noSumDB, p.fetched())
}

func checksumNotice(sumDB, noSumDB string, fetched []string) string {
	if len(fetched) == 0 {
		return ""
	}
	if sumDB == "off" {
		return "GOSUMDB=off: go.sum takes the new hashes unchecked by the checksum database"
	}
	var paths []string
	for _, path := range fetched {
		if module.MatchPrefixPatterns(noSumDB, path) {
			paths = append(paths, path)
		}
	}
	if len(paths) == 0 {
		return ""
	}
	return strings.Join(paths, ", ") + ": unchecked by the checksum database, as GONOSUMDB or GOPRIVATE says"
}

// addRename is os.Rename, which tests replace to fail a write.
var addRename = os.Rename

// apply writes the planned go.mod and go.sum once both are as the plan found them, each through a temporary file renamed over it, and puts back what it replaced on any error or an interrupt before both are written.
func (p *addPlan) apply(ctx context.Context) error {
	return applyModFiles(ctx, "add", p.m.dir, p.oldMod, p.newMod, p.oldSum, p.newSum, p.hadSum, p.hasSum)
}

// applyModFiles writes the planned go.mod and go.sum in dir once both are as planning found them, renaming a temporary file over each (or removing go.sum when the plan dropped it), and restoring both on any error or an interrupt before both are written.
func applyModFiles(ctx context.Context, cmdName, dir string, oldMod, newMod, oldSum, newSum []byte, hadSum, hasSum bool) (err error) {
	type file struct {
		name               string
		old, new           []byte
		existed, nowExists bool
	}
	files := []file{
		{"go.mod", oldMod, newMod, true, true},
		{"go.sum", oldSum, newSum, hadSum, hasSum},
	}
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(dir, f.name))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("%s: %w", cmdName, err)
		}
		if (err == nil) != f.existed || !bytes.Equal(data, f.old) {
			return fmt.Errorf("%s: %s changed since the plan was made, so nothing was written: run %s %s again", cmdName, f.name, nsgoCommand(), cmdName)
		}
	}
	var written []file
	defer func() {
		if err == nil {
			return
		}
		for i := len(written) - 1; i >= 0; i-- {
			f := written[i]
			path := filepath.Join(dir, f.name)
			if f.existed {
				err = errors.Join(err, replaceFile(path, f.old))
			} else {
				err = errors.Join(err, os.Remove(path))
			}
		}
	}()
	for _, f := range files {
		if f.existed == f.nowExists && bytes.Equal(f.old, f.new) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("%s: interrupted, so go.mod and go.sum are as they were: %w", cmdName, err)
		}
		path := filepath.Join(dir, f.name)
		if !f.nowExists {
			if err := os.Remove(path); err != nil {
				return fmt.Errorf("%s: removing %s failed, so go.mod and go.sum are as they were: %w", cmdName, f.name, err)
			}
		} else if err := replaceFile(path, f.new); err != nil {
			// replaceFile renames last, so a file it fails to write is as it was.
			return fmt.Errorf("%s: writing %s failed, so go.mod and go.sum are as they were: %w", cmdName, f.name, err)
		}
		written = append(written, f)
	}
	return nil
}

// replaceFile writes data to path through a temporary file beside it, renamed over it, so path holds its old content or the new one, never a part; it keeps path's mode, 0644 for a new file.
func replaceFile(path string, data []byte) error {
	mode := fs.FileMode(0o644)
	if info, err := os.Stat(path); err == nil {
		mode = info.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".nsgo-add-*")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	err = errors.Join(err, tmp.Chmod(mode), tmp.Close())
	if err == nil {
		err = addRename(tmp.Name(), path)
	}
	if err != nil {
		os.Remove(tmp.Name())
	}
	return err
}
