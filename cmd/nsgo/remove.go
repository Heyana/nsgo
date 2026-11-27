package main

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// nsgo remove drops libraries the module no longer imports, through a go mod tidy planned on copies of go.mod and go.sum (#2465). It refuses libraries still imported by any package of the module under any build tag, plans go mod tidy with add's environment, marks unrelated tidied lines, explains any library kept by another dependency or tool and mentions any replace left in place, never lowering another module's version.

// removeOptions are nsgo remove's flags.
type removeOptions struct {
	dryRun bool // print the plan and write nothing
	yes    bool // apply the plan without asking
}

// removeRequest is a module named on nsgo remove's command line, after resolving any bare built-in or @registry/name reference.
type removeRequest struct {
	arg  string // as typed on the command line
	path string // resolved module path
}

// removeSpec is a parsed argument to nsgo remove: a @registry/name reference, a bare name in NSGo's built-in index, or a direct module path.
type removeSpec struct {
	arg      string // as typed on the command line
	registry string // non-empty for @registry/name
	item     string // item name within registry
	path     string // resolved module path (empty until custom registry resolution)
}

// askToApplyRemove asks on stderr whether to apply remove's plan and reads the answer from a terminal.
func askToApplyRemove(ctx context.Context) (bool, error) {
	return askToApplyFor(ctx, "remove")
}

// parseRemoveSpecsWithIndex reads nsgo remove's arguments: module, @registry/name, or a bare name in NSGo's built-in index idx. Bare names resolve only in idx so custom registries can never shadow bare names (#2461, #2464, #2465); built-in components are refused before anything is planned.
func parseRemoveSpecsWithIndex(args []string, idx *registryIndex) ([]removeSpec, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("remove: name a module, library or @registry/name: %s remove name|@registry/name|module", nsgoCommand())
	}
	var specs []removeSpec
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			return nil, fmt.Errorf("remove: flags go before the modules: %s remove %s %s", nsgoCommand(), arg, args[0])
		}
		if strings.HasPrefix(arg, "@") {
			spec, err := parseRegistryRemoveSpec(arg)
			if err != nil {
				return nil, err
			}
			if slices.ContainsFunc(specs, func(s removeSpec) bool {
				return s.registry == spec.registry && s.item == spec.item
			}) {
				return nil, fmt.Errorf("remove: %s/%s is named twice", spec.registry, spec.item)
			}
			specs = append(specs, spec)
			continue
		}
		if strings.Contains(arg, "#") {
			return nil, fmt.Errorf("remove: %q is not a module path or listed name", arg)
		}
		if strings.Contains(arg, "@") {
			return nil, fmt.Errorf("remove: %q has a version; name the module without @version", arg)
		}
		domain, _, _ := strings.Cut(arg, "/")
		var s removeSpec
		if strings.Contains(domain, ".") {
			if err := module.CheckPath(arg); err != nil {
				return nil, fmt.Errorf("remove: %q: %w", arg, err)
			}
			s = removeSpec{arg: arg, path: arg}
		} else if isValidRegistryName(arg) {
			item, ok := idx.lookup(arg)
			if !ok {
				return nil, fmt.Errorf("remove: unknown name %q; run %s add -list to see listed libraries and components", arg, nsgoCommand())
			}
			if item.Component != "" {
				return nil, fmt.Errorf("remove: %s is a component, copied into this module; delete its copy's directory", arg)
			}
			s = removeSpec{arg: arg, path: item.Module}
		} else {
			return nil, fmt.Errorf("remove: %q is not a module path or listed name", arg)
		}
		if slices.ContainsFunc(specs, func(existing removeSpec) bool { return existing.registry == "" && existing.path == s.path }) {
			return nil, fmt.Errorf("remove: %s is named twice", s.path)
		}
		specs = append(specs, s)
	}
	return specs, nil
}

func parseRegistryRemoveSpec(arg string) (removeSpec, error) {
	regKey, rest, ok := strings.Cut(arg, "/")
	if !ok || rest == "" {
		return removeSpec{}, fmt.Errorf("remove: %q must be @registry/name", arg)
	}
	if regKey == "@nsgo" {
		return removeSpec{}, fmt.Errorf("remove: %q is reserved for NSGo's built-in index", regKey)
	}
	if !isValidRegistryName(regKey[1:]) {
		return removeSpec{}, fmt.Errorf("remove: %q has invalid registry name %q", arg, regKey)
	}
	if strings.Contains(rest, "#") {
		return removeSpec{}, fmt.Errorf("remove: %q is not a valid @registry/name", arg)
	}
	if strings.Contains(rest, "@") {
		return removeSpec{}, fmt.Errorf("remove: %q has a version; name the module without @version", arg)
	}
	if !isValidRegistryName(rest) {
		return removeSpec{}, fmt.Errorf("remove: %q has invalid item name %q", arg, rest)
	}
	return removeSpec{
		arg:      arg,
		registry: regKey,
		item:     rest,
	}, nil
}

// resolveRemoveSpecs resolves any @registry/name specs through nsgo.yaml's configured registries, refusing unconfigured registries and copied components without modifying nsgo.yaml (#2464, #2465).
func resolveRemoveSpecs(ctx context.Context, specs []removeSpec, modDir string) ([]removeRequest, []*loadedRegistry, error) {
	needsRegistry := slices.ContainsFunc(specs, func(s removeSpec) bool { return s.registry != "" })
	if !needsRegistry {
		reqs := make([]removeRequest, len(specs))
		for i, s := range specs {
			reqs[i] = removeRequest{arg: s.arg, path: s.path}
		}
		return reqs, nil, nil
	}

	regMap, _, err := loadProjectRegistries(".", modDir)
	if err != nil {
		return nil, nil, fmt.Errorf("remove: %w", err)
	}

	loaded := map[string]*loadedRegistry{}
	var used []*loadedRegistry
	reqs := make([]removeRequest, 0, len(specs))

	for _, s := range specs {
		if s.registry == "" {
			if slices.ContainsFunc(reqs, func(r removeRequest) bool { return r.path == s.path }) {
				return nil, nil, fmt.Errorf("remove: %s is named twice", s.path)
			}
			reqs = append(reqs, removeRequest{arg: s.arg, path: s.path})
			continue
		}
		rc, ok := regMap[s.registry]
		if !ok {
			return nil, nil, fmt.Errorf("remove: registry %s is not configured in nsgo.yaml; add it under registries:\n  registries:\n    %q: https://...", s.registry, s.registry)
		}
		lr, ok := loaded[s.registry]
		if !ok {
			lr, err = loadRegistry(ctx, rc)
			if err != nil {
				return nil, nil, fmt.Errorf("remove: %w", err)
			}
			loaded[s.registry] = lr
			used = append(used, lr)
		}
		item, ok := lr.Index.lookup(s.item)
		if !ok {
			return nil, nil, fmt.Errorf("remove: %s/%s was not found in registry %s (%s); run %s add -list %s to see its items", s.registry, s.item, s.registry, rc.Raw, nsgoCommand(), s.registry)
		}
		if item.Component != "" {
			return nil, nil, fmt.Errorf("remove: %s/%s is a component, copied into this module; delete its copy's directory", s.registry, s.item)
		}
		if slices.ContainsFunc(reqs, func(r removeRequest) bool { return r.path == item.Module }) {
			return nil, nil, fmt.Errorf("remove: %s is named twice", item.Module)
		}
		reqs = append(reqs, removeRequest{arg: s.arg, path: item.Module})
	}
	return reqs, used, nil
}

// removeModules runs nsgo remove using NSGo's built-in index and any registries configured in nsgo.yaml: it refuses components and still-imported libraries, plans go mod tidy on copies of go.mod and go.sum, writes the plan to out, and applies it with -y, or once ask, which is nil when nobody can be asked, says yes.
func removeModules(ctx context.Context, o removeOptions, args []string, out io.Writer, ask func(context.Context) (bool, error)) error {
	return removeModulesWithIndex(ctx, o, args, builtinRegistry(), out, ask)
}

func removeModulesWithIndex(ctx context.Context, o removeOptions, args []string, idx *registryIndex, out io.Writer, ask func(context.Context) (bool, error)) error {
	specs, err := parseRemoveSpecsWithIndex(args, idx)
	if err != nil {
		return err
	}
	planning := func(err error) error {
		if ctx.Err() != nil {
			return fmt.Errorf("remove: interrupted while planning, so nothing was written: %w", ctx.Err())
		}
		return err
	}
	m, err := findModule(ctx, "remove")
	if err != nil {
		return planning(err)
	}
	s := begin("planning %s", strings.Join(args, " "))
	reqs, usedRegistries, err := resolveRemoveSpecs(withStep(ctx, s), specs, m.dir)
	if err != nil {
		s.remove()
		return planning(err)
	}
	p, err := m.planRemove(withStep(ctx, s), reqs)
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
			return fmt.Errorf("remove: the plan was not applied, as nobody can confirm it without a terminal: run %s remove -y %s to apply it", nsgoCommand(), strings.Join(args, " "))
		}
		ok, err := ask(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("remove: the plan was not applied")
		}
	}
	if err := p.apply(ctx); err != nil {
		return err
	}
	for _, item := range p.items {
		if item.after == "" {
			done("removed %s %s", item.path, item.before)
		} else {
			detail("kept %s %s", item.path, item.after)
		}
	}
	if p.m.vendored() {
		detail("next: go mod vendor")
	}
	return nil
}

// removePlan is what nsgo remove would write: go.mod and go.sum as go mod tidy writes them, with each target's outcome and which diff lines belong to the targets versus other tidied lines.
type removePlan struct {
	m           *addModule
	registries  []*loadedRegistry // custom registries resolved for this plan, in order
	items       []removeItem
	notRequired []removeNotRequired
	related     map[string]bool // target modules and their pre-tidy transitive dependencies

	oldMod, newMod, oldSum, newSum []byte
	hadSum, hasSum                 bool
	before, after                  *modfile.File
}

// removeItem is a required module targeted by nsgo remove.
type removeItem struct {
	removeRequest
	before      string   // version required before tidying
	after       string   // version still required after tidying, or empty when dropped
	keptReasons []string // why the module stays after tidying
	replaces    []string // replace targets left in go.mod for this module
}

// removeNotRequired is a requested module that go.mod did not require.
type removeNotRequired struct {
	path     string
	replaces []string
}

// planRemove verifies no target is directly imported, then runs go mod tidy on copies of go.mod and go.sum in a temporary directory: the module's files are never written.
func (m *addModule) planRemove(ctx context.Context, reqs []removeRequest) (*removePlan, error) {
	p := &removePlan{m: m, related: map[string]bool{}}
	var err error
	if p.oldMod, err = os.ReadFile(filepath.Join(m.dir, "go.mod")); err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}
	p.oldSum, err = os.ReadFile(filepath.Join(m.dir, "go.sum"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("remove: %w", err)
	}
	p.hadSum = err == nil
	if p.before, err = modfile.Parse(filepath.Join(m.dir, "go.mod"), p.oldMod, nil); err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}
	if p.before.Module == nil {
		return nil, fmt.Errorf("remove: %s has no module line", filepath.Join(m.dir, "go.mod"))
	}
	p.newMod, p.newSum, p.hasSum, p.after = p.oldMod, p.oldSum, p.hadSum, p.before
	mainMod := p.before.Module.Mod.Path

	var targets []removeRequest
	for _, r := range reqs {
		if r.path == mainMod {
			return nil, fmt.Errorf("remove: %s is this module", r.path)
		}
		if requiredVersion(p.before, r.path) == "" {
			if owner := providingModule(r.path, p.before); owner != "" && owner != r.path {
				return nil, fmt.Errorf("remove: %s is a package of the module %s, not a module: %s remove %s", r.path, owner, nsgoCommand(), owner)
			}
			p.notRequired = append(p.notRequired, removeNotRequired{
				path:     r.path,
				replaces: moduleReplaces(p.before, r.path),
			})
			continue
		}
		targets = append(targets, r)
	}
	if len(targets) == 0 {
		return p, nil
	}

	tmp, err := os.MkdirTemp("", "nsgo-remove-")
	if err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}
	defer os.RemoveAll(tmp)
	planned := filepath.Join(tmp, "go.mod")
	if err := os.WriteFile(planned, p.oldMod, 0o644); err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}
	if p.hadSum {
		if err := os.WriteFile(filepath.Join(tmp, "go.sum"), p.oldSum, 0o644); err != nil {
			return nil, fmt.Errorf("remove: %w", err)
		}
	}
	modFlag := "-modfile=" + planned
	paths := make([]string, len(targets))
	for i, r := range targets {
		paths[i] = r.path
	}

	stage(ctx, "checking imports")
	whyOut, err := m.goOutput(ctx, append([]string{"mod", "why", "-m", modFlag}, paths...)...)
	if err != nil {
		var goErr *addGoError
		if errors.As(err, &goErr) {
			return nil, fmt.Errorf("remove: go mod why failed\n%s", goErr.messages)
		}
		return nil, fmt.Errorf("remove: %w", err)
	}
	why := parseModWhy(whyOut)
	var imported []string
	for _, r := range targets {
		chain := why[r.path]
		if len(chain) < 2 {
			continue
		}
		importer := chain[len(chain)-2]
		if !isMainModulePackage(importer, mainMod, p.before) {
			continue
		}
		pkg := importer
		if base, isTest := strings.CutSuffix(pkg, ".test"); isTest {
			pkg = base + " (tests)"
		}
		msg := fmt.Sprintf("%s is still imported by %s", r.path, pkg)
		if targetPkg := chain[len(chain)-1]; targetPkg != r.path {
			msg += fmt.Sprintf(" (%s)", targetPkg)
		}
		imported = append(imported, msg)
	}
	if len(imported) > 0 {
		return nil, errors.New("remove: " + strings.Join(imported, "\n"))
	}

	// Record the target modules and their pre-tidy transitive dependencies so the plan can distinguish them from unrelated tidied lines.
	for _, path := range paths {
		p.related[path] = true
	}
	if graphOut, err := m.goOutput(ctx, "mod", "graph", modFlag); err == nil {
		for path := range reachableFrom(parseModGraph(graphOut), paths) {
			p.related[path] = true
		}
	}

	stage(ctx, "go mod tidy")
	if _, err := m.goOutput(ctx, "mod", "tidy", modFlag); err != nil {
		var goErr *addGoError
		if !errors.As(err, &goErr) {
			return nil, fmt.Errorf("remove: %w", err)
		}
		needed := ""
		for _, match := range tooNewGo.FindAllStringSubmatch(goErr.messages, -1) {
			if strings.Contains(match[1], "@") && (needed == "" || semver.Compare("v"+match[2], "v"+needed) > 0) {
				needed = match[2]
			}
		}
		if needed != "" {
			return nil, fmt.Errorf("remove: %s\nto move this module to Go %s on purpose, run go get go@%s first", goErr.messages, needed, needed)
		}
		if strings.Contains(goErr.messages, "GOPROXY=off") {
			return nil, fmt.Errorf("remove: go mod tidy failed\n%s\nnsgo remove fetches modules through the proxies GOPROXY lists before direct, and from their own hosts only those GOPRIVATE or GONOPROXY marks", goErr.messages)
		}
		return nil, fmt.Errorf("remove: go mod tidy failed\n%s", goErr.messages)
	}

	if p.newMod, err = os.ReadFile(planned); err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}
	p.newSum, err = os.ReadFile(filepath.Join(tmp, "go.sum"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("remove: %w", err)
	}
	p.hasSum = err == nil
	if p.after, err = modfile.Parse(filepath.Join(m.dir, "go.mod"), p.newMod, nil); err != nil {
		return nil, fmt.Errorf("remove: %w", err)
	}

	var stayed []string
	for _, r := range targets {
		if requiredVersion(p.after, r.path) != "" {
			stayed = append(stayed, r.path)
		}
	}
	var stayedWhy map[string][]string
	var stayedGraph map[string][]string
	if len(stayed) > 0 {
		stage(ctx, "reading why kept modules stay")
		if out, err := m.goOutput(ctx, append([]string{"mod", "why", "-m", modFlag}, stayed...)...); err == nil {
			stayedWhy = parseModWhy(out)
		}
		if out, err := m.goOutput(ctx, "mod", "graph", modFlag); err == nil {
			stayedGraph = parseModGraph(out)
		}
	}

	for _, r := range targets {
		item := removeItem{
			removeRequest: r,
			before:        requiredVersion(p.before, r.path),
			after:         requiredVersion(p.after, r.path),
			replaces:      moduleReplaces(p.after, r.path),
		}
		if item.after != "" {
			item.keptReasons = keptReasonsFor(r.path, mainMod, p.after, stayedWhy[r.path], stayedGraph)
		}
		p.items = append(p.items, item)
	}
	return p, nil
}

// parseModWhy parses the output of go mod why -m into each module's shortest import chain, or nil when unneeded.
func parseModWhy(out []byte) map[string][]string {
	chains := map[string][]string{}
	var current string
	for _, raw := range strings.Split(string(out), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if mod, ok := strings.CutPrefix(line, "# "); ok {
			current = strings.TrimSpace(mod)
			chains[current] = nil
			continue
		}
		if current == "" || strings.HasPrefix(line, "(") {
			continue
		}
		chains[current] = append(chains[current], line)
	}
	return chains
}

// isMainModulePackage reports whether pkg (which may carry go mod why's .test suffix) belongs to mainMod rather than a required dependency.
func isMainModulePackage(pkg, mainMod string, f *modfile.File) bool {
	base := strings.TrimSuffix(pkg, ".test")
	if base != mainMod && !strings.HasPrefix(base, mainMod+"/") {
		return false
	}
	if owner := providingModule(base, f); len(owner) > len(mainMod) {
		return false
	}
	return true
}

// parseModGraph parses go mod graph lines into an adjacency list keyed by parent module path (without @version).
func parseModGraph(out []byte) map[string][]string {
	edges := map[string][]string{}
	for _, raw := range strings.Split(string(out), "\n") {
		fields := strings.Fields(raw)
		if len(fields) != 2 {
			continue
		}
		parent, _, _ := strings.Cut(fields[0], "@")
		child, _, _ := strings.Cut(fields[1], "@")
		if parent == "" || child == "" {
			continue
		}
		if !slices.Contains(edges[parent], child) {
			edges[parent] = append(edges[parent], child)
		}
	}
	return edges
}

// reachableFrom returns all module paths reachable from roots in edges, including roots.
func reachableFrom(edges map[string][]string, roots []string) map[string]bool {
	seen := map[string]bool{}
	queue := make([]string, 0, len(roots))
	for _, r := range roots {
		if !seen[r] {
			seen[r] = true
			queue = append(queue, r)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, next := range edges[cur] {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return seen
}

// moduleReplaces returns formatted replacement targets for mod in f.
func moduleReplaces(f *modfile.File, mod string) []string {
	var out []string
	for _, r := range f.Replace {
		if r.Old.Path != mod {
			continue
		}
		target := strings.TrimSpace(r.New.Path + " " + r.New.Version)
		if r.Old.Version != "" {
			target = r.Old.Version + " => " + target
		}
		out = append(out, target)
	}
	return out
}

// keptReasonsFor explains why mod stayed in after after go mod tidy: declared tools, an import chain through another dependency, or a requiring module in the module graph.
func keptReasonsFor(mod, mainMod string, after *modfile.File, chain []string, graph map[string][]string) []string {
	var reasons []string
	var tools []string
	for _, t := range after.Tool {
		if providingModule(t.Path, after) == mod {
			tools = append(tools, t.Path)
		}
	}
	if len(tools) == 1 {
		reasons = append(reasons, "go.mod declares tool "+tools[0])
	} else if len(tools) > 1 {
		reasons = append(reasons, "go.mod declares tools "+strings.Join(tools, ", "))
	}

	if len(chain) >= 2 {
		neederPkg := chain[len(chain)-2]
		if !isMainModulePackage(neederPkg, mainMod, after) {
			needer := cmp.Or(providingModule(neederPkg, after), neederPkg)
			if len(chain) == 3 {
				reasons = append(reasons, "needed by "+needer)
			} else {
				reasons = append(reasons, fmt.Sprintf("needed by %s (%s)", needer, strings.Join(chain[1:], " -> ")))
			}
		}
	}

	if len(reasons) == 0 && len(graph) > 0 {
		var parents []string
		for parent, children := range graph {
			if parent == mainMod || parent == mod {
				continue
			}
			if slices.Contains(children, mod) && requiredVersion(after, parent) != "" {
				parents = append(parents, parent)
			}
		}
		slices.Sort(parents)
		if len(parents) > 0 {
			reasons = append(reasons, "needed by "+strings.Join(parents, ", "))
		}
	}

	if len(reasons) == 0 {
		reasons = append(reasons, "still required by another dependency")
	}
	return reasons
}

// changes reports whether applying the plan writes or removes anything.
func (p *removePlan) changes() bool {
	return !bytes.Equal(p.oldMod, p.newMod) || p.hadSum != p.hasSum || !bytes.Equal(p.oldSum, p.newSum)
}

// isTarget reports whether path was named on nsgo remove's command line.
func (p *removePlan) isTarget(path string) bool {
	return slices.ContainsFunc(p.items, func(item removeItem) bool { return item.path == path })
}

// refusal is why the plan cannot be applied, -y or not, or nil.
func (p *removePlan) refusal() error {
	before, after := requirements(p.before), requirements(p.after)
	var reasons []string
	var paths []string
	for path := range after {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		old, hadOld := before[path]
		if !hadOld {
			continue
		}
		new := after[path]
		if semver.Compare(new.version, old.version) < 0 {
			reasons = append(reasons, fmt.Sprintf("the plan lowers %s from %s to %s; remove never lowers another module", path, old.version, new.version))
		}
	}
	if len(reasons) == 0 {
		return nil
	}
	return errors.New("remove: " + strings.Join(reasons, "\n"))
}

// write prints the plan: any custom registries used, any targets not in go.mod, a block per targeted module, and the changes to this module's go.mod and go.sum.
func (p *removePlan) write(w io.Writer) error {
	var b bytes.Buffer
	for _, lr := range p.registries {
		fmt.Fprintf(&b, "%s\n", lr.headerLine())
	}
	if len(p.registries) > 0 && len(p.items) > 0 {
		b.WriteString("\n")
	}
	for _, nr := range p.notRequired {
		if len(nr.replaces) > 0 {
			fmt.Fprintf(&b, "%s is not in go.mod (replace with %s is left in place)\n", nr.path, strings.Join(nr.replaces, ", "))
		} else {
			fmt.Fprintf(&b, "%s is not in go.mod\n", nr.path)
		}
	}
	if len(p.items) == 0 {
		_, err := w.Write(b.Bytes())
		return err
	}
	if len(p.notRequired) > 0 {
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
	for i, item := range p.items {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s\n", item.path)
		if item.after == "" {
			row("module", "remove "+item.path+" "+item.before)
		} else {
			row("module", "keep "+item.path+" "+item.after)
			rows("kept", item.keptReasons)
		}
		for _, rep := range item.replaces {
			row("replaced", "go.mod still replaces it with "+rep+", left in place")
		}
	}
	if !p.changes() {
		_, err := w.Write(b.Bytes())
		return err
	}
	b.WriteString("\n")
	fmt.Fprintf(&b, "%s\n", p.before.Module.Mod.Path)
	rows("go.mod", p.goModChanges())
	if sum := formatGoSumChange(p.oldSum, p.newSum); sum != "" {
		row("go.sum", sum)
	}
	rows("tools", modToolChanges(p.before, p.after))
	fetched := fetchedModules(p.oldSum, p.newSum)
	if line := privateModules(p.m.noProxy, fetched); line != "" {
		row("private", line)
	}
	if line := checksumNotice(p.m.sumDB, p.m.noSumDB, fetched); line != "" {
		row("checksums", line)
	}
	if p.m.workspace != "" {
		row("workspace", "go.mod is planned for this module alone, as GOWORK=off go mod tidy writes it, not with "+relTo(p.m.dir, p.m.workspace))
	}
	if p.m.vendored() {
		row("next", "go mod vendor")
	}
	_, err := w.Write(b.Bytes())
	return err
}

// goModChanges are the plan's lines of go.mod that change, marking lines unrelated to the removed libraries as also tidied.
func (p *removePlan) goModChanges() []string {
	var lines []string
	directive := func(name, old, new string) {
		switch {
		case old == new:
		case old == "":
			lines = append(lines, "+ "+name+" "+new+", also tidied")
		case new == "":
			lines = append(lines, "- "+name+" "+old+", also tidied")
		default:
			lines = append(lines, "~ "+name+" "+old+" => "+new+", also tidied")
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
		return slices.IndexFunc(p.items, func(item removeItem) bool { return item.path == path })
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
			lines = append(lines, "+ require "+path+" "+new.version+new.comment()+", also tidied")
		case !hasNew:
			line := "- require " + path + " " + old.version
			if !p.related[path] {
				line += ", also tidied"
			}
			lines = append(lines, line)
		case old.version != new.version:
			line := "~ require " + path + " " + old.version + " => " + new.version
			if semver.Compare(new.version, old.version) < 0 {
				line += ", a downgrade"
			}
			if !p.isTarget(path) {
				line += ", also tidied"
			}
			lines = append(lines, line)
		case old.indirect != new.indirect:
			was := "direct"
			if old.indirect {
				was = "// indirect"
			}
			line := "~ require " + path + " " + new.version + new.comment() + ", was " + was
			if !p.isTarget(path) {
				line += ", also tidied"
			}
			lines = append(lines, line)
		}
	}
	return lines
}

// apply writes the planned go.mod and go.sum once both are as the plan found them.
func (p *removePlan) apply(ctx context.Context) error {
	return applyModFiles(ctx, "remove", p.m.dir, p.oldMod, p.newMod, p.oldSum, p.newSum, p.hadSum, p.hasSum)
}
