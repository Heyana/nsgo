package main

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
)

// addAppConfig is what nsgo add reads from the app's nsgo.yaml (or defaults when there is no nsgo.yaml): the expanded build targets, the app's build tags and its license policy.
type addAppConfig struct {
	targets    []string // targets to inspect (Build.Targets, or hostTarget() without nsgo.yaml)
	appTargets []string // Build.Targets from nsgo.yaml, nil when no nsgo.yaml exists
	tags       []string
	licenses   licensePolicy
}

// loadAddAppConfig reads nsgo.yaml from cwd, else modDir; when neither has one, it falls back to the host's target and the default license policy.
func loadAddAppConfig(cwd, modDir string) (*addAppConfig, error) {
	dirs := []string{cwd}
	if modDir != "" && modDir != cwd {
		dirs = append(dirs, modDir)
	}
	for _, dir := range dirs {
		p, err := load(dir)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}
		return &addAppConfig{
			targets:    p.Build.Targets,
			appTargets: p.Build.Targets,
			tags:       p.Build.Tags,
			licenses:   p.Licenses,
		}, nil
	}
	return &addAppConfig{targets: []string{hostTarget()}}, nil
}

// hostTarget is the NSGo target name for the host machine.
func hostTarget() string {
	osName := runtime.GOOS
	if osName == "darwin" {
		osName = "macos"
	}
	return osName + "_" + goArchName(runtime.GOARCH)
}

// addTargetSlice is one go list invocation's environment, tags and nsgo-natives.yaml target key.
type addTargetSlice struct {
	nativeTarget string
	env          []string
	tags         []string
}

// addTargetSlices returns the listing slices for an expanded NSGo target without needing Xcode or an Android NDK, matching what nsgo build lists for that target.
func addTargetSlices(target string) ([]addTargetSlice, error) {
	switch target {
	case "macos_universal":
		return []addTargetSlice{
			{nativeTarget: "macos_arm64", env: []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}},
			{nativeTarget: "macos_x64", env: []string{"GOOS=darwin", "GOARCH=amd64", "CGO_ENABLED=0"}},
		}, nil
	case "android_universal":
		return []addTargetSlice{
			{env: []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}},
			{env: []string{"GOOS=android", "GOARCH=amd64", "CGO_ENABLED=1"}},
		}, nil
	}
	sys, arch, _ := strings.Cut(target, "_")
	switch sys {
	case "tui":
		env, err := tuiEnv(target)
		if err != nil {
			return nil, err
		}
		return []addTargetSlice{{nativeTarget: nativeTarget(target), env: env, tags: targetTags(target)}}, nil
	case "macos":
		return []addTargetSlice{{nativeTarget: nativeTarget(target), env: []string{"GOOS=darwin", "GOARCH=" + goArch(arch), "CGO_ENABLED=0"}}}, nil
	case "windows", "linux":
		return []addTargetSlice{{nativeTarget: nativeTarget(target), env: []string{"GOOS=" + sys, "GOARCH=" + goArch(arch), "CGO_ENABLED=0"}}}, nil
	case "web", "miniprogram", "harmonyos":
		return []addTargetSlice{{env: []string{"GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0"}, tags: targetTags(target)}}, nil
	case "ios", "tvos", "watchos":
		return []addTargetSlice{{nativeTarget: nativeTarget(target), env: []string{"GOOS=ios", "GOARCH=arm64", "CGO_ENABLED=1"}, tags: targetTags(target)}}, nil
	case "android":
		cgo := "0"
		if arch != "arm64" {
			cgo = "1"
		}
		return []addTargetSlice{{env: []string{"GOOS=android", "GOARCH=" + goArch(arch), "CGO_ENABLED=" + cgo}}}, nil
	default:
		// Fallback for a non-standard host OS when no nsgo.yaml exists.
		if target == hostTarget() {
			return []addTargetSlice{{env: []string{"GOOS=" + runtime.GOOS, "GOARCH=" + runtime.GOARCH, "CGO_ENABLED=0"}}}, nil
		}
		return nil, fmt.Errorf("unknown target %q", target)
	}
}

// writeAddOverlay writes a go -overlay file mapping the module's go.mod and go.sum to the planned copies in tmp.
func writeAddOverlay(tmp, modDir string, hasSum, hadSum bool) (string, error) {
	replace := map[string]string{
		filepath.Join(modDir, "go.mod"): filepath.Join(tmp, "go.mod"),
	}
	if hasSum {
		replace[filepath.Join(modDir, "go.sum")] = filepath.Join(tmp, "go.sum")
	} else if hadSum {
		replace[filepath.Join(modDir, "go.sum")] = ""
	}
	data, err := json.Marshal(struct {
		Replace map[string]string
	}{Replace: replace})
	if err != nil {
		return "", err
	}
	path := filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// addItemInspection holds what planning learned about an item's targets, native libraries, licenses and policy outcomes.
type addItemInspection struct {
	modules  []addModuleLicense
	natives  []addNativePolicySummary
	targets  []addTargetReport
	refusals []string
}

type addModuleLicense struct {
	path             string
	types            []string
	copyleft         []string
	unknown          []string
	registryLicense  string
	registryMismatch bool
	allowed          bool
	refusedUnknown   bool
	warnUnknown      bool
	refusedCopyleft  bool
	warnCopyleft     bool
}

type addNativePolicySummary struct {
	modulePath      string
	name            string
	targets         []string
	types           []string
	copyleft        []string
	unknown         []string
	allowed         bool
	refusedUnknown  bool
	warnUnknown     bool
	refusedCopyleft bool
	warnCopyleft    bool
}

type addTargetReport struct {
	target   string
	natives  []addTargetNative
	excluded []addExcludedPackage
	errors   []string
}

type addTargetNative struct {
	name       string
	modulePath string
	host       string
	inPackage  bool
	types      []string
	unknown    []string
}

type addExcludedPackage struct {
	importPath string
	reason     string
}

// inspect runs the build's own per-target package listing and license classification over each planned item, using an overlay of the planned go.mod and go.sum.
func (p *addPlan) inspect(ctx context.Context, tmp string) error {
	if !p.changes() || p.app == nil {
		return nil
	}
	overlay, err := writeAddOverlay(tmp, p.m.dir, p.hasSum, p.hadSum)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	for i := range p.items {
		if p.items[i].listed.Error != nil {
			continue
		}
		stage(ctx, "inspecting targets and licenses for %s", p.items[i].path)
		insp, err := p.inspectItem(ctx, overlay, &p.items[i])
		if err != nil {
			return fmt.Errorf("add: %w", err)
		}
		p.items[i].inspection = insp
	}
	return nil
}

// hasNonTestGo reports whether dir contains at least one non-test Go source file.
func hasNonTestGo(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !e.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			return true
		}
	}
	return false
}

func (p *addPlan) inspectItem(ctx context.Context, overlay string, item *addItem) (*addItemInspection, error) {
	modDir := item.modDir
	if modDir == "" {
		modDir = item.listed.Dir
		if item.listed.Replace != nil && item.listed.Replace.Dir != "" {
			modDir = item.listed.Replace.Dir
		}
	}
	var pkgs []string
	explicit := false
	if item.manifest != nil && item.manifest.hasImport {
		pkgs = item.manifest.importPaths(item.path)
		explicit = true
	} else if modDir == "" || hasNonTestGo(modDir) {
		// A module with no nsgo-module.yaml import list and no root Go files (for example a cmd-only module) has no root package to list; classify its own license files directly.
		pkgs = []string{item.path}
	}

	// Collect each target's slices and deduplicate go list calls with identical (env, tags).
	type sliceKey struct {
		env  string
		tags string
	}
	type sliceJob struct {
		env  []string
		tags []string
	}
	flagTags := goFlagTags(p.m.goFlags)
	targetSlices := make([][]addTargetSlice, len(p.app.targets))
	jobMap := map[sliceKey]sliceJob{}
	var jobKeys []sliceKey
	if len(pkgs) > 0 {
		for i, target := range p.app.targets {
			slicesForTarget, err := addTargetSlices(target)
			if err != nil {
				return nil, err
			}
			targetSlices[i] = slicesForTarget
			for _, sl := range slicesForTarget {
				tags := uniqueTags(slices.Concat(flagTags, p.app.tags, sl.tags))
				key := sliceKey{env: strings.Join(sl.env, "\x00"), tags: strings.Join(tags, ",")}
				if _, ok := jobMap[key]; !ok {
					jobMap[key] = sliceJob{env: sl.env, tags: tags}
					jobKeys = append(jobKeys, key)
				}
			}
		}
	}

	type sliceResult struct {
		pkgs []licensePackage
		err  error
	}
	results := make(map[sliceKey]sliceResult, len(jobKeys))
	var mu sync.Mutex
	var wg sync.WaitGroup
	workers := min(max(1, runtime.NumCPU()/2), max(1, len(jobKeys)))
	slots := make(chan struct{}, workers)
	for _, key := range jobKeys {
		if ctx.Err() != nil {
			break
		}
		slots <- struct{}{}
		job := jobMap[key]
		wg.Go(func() {
			defer func() { <-slots }()
			baseEnv := p.m.listEnv
			if len(baseEnv) == 0 {
				baseEnv = p.m.env
			}
			fullEnv := append(slices.Clone(baseEnv), job.env...)
			listed, err := listLicensePackages(ctx, p.m.goCmd, p.m.dir, fullEnv, job.tags, []string{"-e", "-mod=readonly"}, overlay, pkgs)
			mu.Lock()
			results[key] = sliceResult{pkgs: listed, err: err}
			mu.Unlock()
		})
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	for _, key := range jobKeys {
		if res := results[key]; res.err != nil {
			return nil, res.err
		}
	}

	insp := &addItemInspection{}
	moduleSources := map[string]*licenseSource{}
	type nativeKey struct {
		modulePath, name string
	}
	nativeSummaries := map[nativeKey]*addNativePolicySummary{}
	var nativeOrder []nativeKey

	for i, target := range p.app.targets {
		report := addTargetReport{target: target}
		scratch := &project{
			Dir:             p.m.dir,
			nativeLibraries: map[string][]nativeLibrary{},
			nativeLinuxDeps: map[string][]nativeLinuxDeps{},
		}
		targetSources := map[string]*licenseSource{}
		seenExcluded := map[string]bool{}
		seenErrors := map[string]bool{}
		seenNatives := map[string]bool{}

		for _, sl := range targetSlices[i] {
			tags := uniqueTags(slices.Concat(flagTags, p.app.tags, sl.tags))
			key := sliceKey{env: strings.Join(sl.env, "\x00"), tags: strings.Join(tags, ",")}
			listed := results[key].pkgs
			for _, pkg := range listed {
				if pkg.Error != nil {
					if !explicit && pkg.ImportPath == item.path && strings.Contains(pkg.Error.Err, "no Go files in ") {
						continue
					}
					reason := cleanPackageError(pkg.Error.Err)
					if !seenExcluded[pkg.ImportPath] {
						seenExcluded[pkg.ImportPath] = true
						report.excluded = append(report.excluded, addExcludedPackage{
							importPath: pkg.ImportPath,
							reason:     reason,
						})
					}
					continue
				}
				if pkg.Standard || pkg.Module == nil {
					continue
				}
				addLicensePackageWith(moduleSources, pkg, false)
				if err := scratch.addPackageNatives(pkg, sl.nativeTarget, targetSources); err != nil {
					msg := err.Error()
					if !seenErrors[msg] {
						seenErrors[msg] = true
						report.errors = append(report.errors, msg)
						insp.refusals = append(insp.refusals, fmt.Sprintf("%s: %s", target, msg))
					}
				}
			}
			for _, lib := range scratch.nativeLibraries[sl.nativeTarget] {
				dedup := lib.owner + "\x00" + lib.Name
				if seenNatives[dedup] {
					continue
				}
				seenNatives[dedup] = true
				srcKey := "native:" + lib.owner + "/" + lib.Name
				src := targetSources[srcKey]
				var cl licenseClassification
				if src != nil {
					var err error
					cl, err = classifyLicenseSource(src)
					if err != nil {
						return nil, err
					}
				}
				modPath := item.path
				if src != nil && src.name != "" {
					modPath = src.name
				}
				host := ""
				if lib.URL != "" {
					if u, err := url.Parse(lib.URL); err == nil {
						host = u.Host
					}
				}
				report.natives = append(report.natives, addTargetNative{
					name:       lib.Name,
					modulePath: modPath,
					host:       host,
					inPackage:  lib.Path != "",
					types:      cl.types,
					unknown:    cl.unknown,
				})
				nk := nativeKey{modulePath: modPath, name: lib.Name}
				sum := nativeSummaries[nk]
				if sum == nil {
					sum = &addNativePolicySummary{
						modulePath: modPath,
						name:       lib.Name,
						types:      slices.Clone(cl.types),
						copyleft:   slices.Clone(cl.copyleft),
						unknown:    slices.Clone(cl.unknown),
					}
					nativeSummaries[nk] = sum
					nativeOrder = append(nativeOrder, nk)
				} else {
					sum.types = mergeSortedStrings(sum.types, cl.types)
					sum.copyleft = mergeSortedStrings(sum.copyleft, cl.copyleft)
					sum.unknown = mergeSortedStrings(sum.unknown, cl.unknown)
				}
				if !slices.Contains(sum.targets, target) {
					sum.targets = append(sum.targets, target)
				}
			}
		}
		if len(report.natives) > 0 || len(report.excluded) > 0 || len(report.errors) > 0 {
			insp.targets = append(insp.targets, report)
		}
	}

	// Ensure the item's own module is always classified, even if it has no root package or was excluded on every target.
	if moduleSources[item.path] == nil && modDir != "" {
		moduleSources[item.path] = &licenseSource{
			name:       item.path,
			label:      item.path + " " + item.selected,
			root:       modDir,
			firstParty: item.path == p.before.Module.Mod.Path,
			dirs:       map[string]bool{modDir: true},
		}
	}

	// Sort modules with the item's own module first, then alphabetically.
	modNames := slices.SortedFunc(maps.Keys(moduleSources), func(a, b string) int {
		switch {
		case a == item.path && b != item.path:
			return -1
		case b == item.path && a != item.path:
			return 1
		}
		return cmp.Compare(a, b)
	})

	policy := p.app.licenses
	for _, name := range modNames {
		src := moduleSources[name]
		if src.firstParty {
			continue
		}
		cl, err := classifyLicenseSource(src)
		if err != nil {
			return nil, err
		}
		ml := addModuleLicense{
			path:     name,
			types:    cl.types,
			copyleft: cl.copyleft,
			unknown:  cl.unknown,
			allowed:  slices.Contains(policy.Allow, name),
		}
		if name == item.path && item.registryLicense != "" {
			ml.registryLicense = item.registryLicense
			ml.registryMismatch = !licenseMatchesRegistry(cl.types, item.registryLicense)
		}
		if len(cl.types) == 0 || len(cl.unknown) > 0 {
			reason := "has no recognized license text"
			if len(cl.unknown) > 0 {
				reason = "has unrecognized license text in " + strings.Join(cl.unknown, ", ")
			}
			if !ml.allowed {
				if policy.Unknown != "warn" {
					ml.refusedUnknown = true
					insp.refusals = append(insp.refusals, licenseRefusalMessage(name, "", reason, "unknown"))
				} else {
					ml.warnUnknown = true
				}
			}
		}
		if len(cl.copyleft) > 0 && !ml.allowed {
			reason := "has copyleft license text (" + strings.Join(cl.copyleft, ", ") + ")"
			if policy.Copyleft == "error" {
				ml.refusedCopyleft = true
				insp.refusals = append(insp.refusals, licenseRefusalMessage(name, "", reason, "copyleft"))
			} else {
				ml.warnCopyleft = true
			}
		}
		insp.modules = append(insp.modules, ml)
	}

	for _, nk := range nativeOrder {
		sum := nativeSummaries[nk]
		sum.allowed = slices.Contains(policy.Allow, sum.modulePath)
		where := fmt.Sprintf("native library %s on %s", sum.name, strings.Join(sum.targets, ", "))
		if len(sum.types) == 0 || len(sum.unknown) > 0 {
			reason := "has no recognized license text"
			if len(sum.unknown) > 0 {
				reason = "has unrecognized license text in " + strings.Join(sum.unknown, ", ")
			}
			if !sum.allowed {
				if policy.Unknown != "warn" {
					sum.refusedUnknown = true
					insp.refusals = append(insp.refusals, licenseRefusalMessage(sum.modulePath, where, reason, "unknown"))
				} else {
					sum.warnUnknown = true
				}
			}
		}
		if len(sum.copyleft) > 0 && !sum.allowed {
			reason := "has copyleft license text (" + strings.Join(sum.copyleft, ", ") + ")"
			if policy.Copyleft == "error" {
				sum.refusedCopyleft = true
				insp.refusals = append(insp.refusals, licenseRefusalMessage(sum.modulePath, where, reason, "copyleft"))
			} else {
				sum.warnCopyleft = true
			}
		}
		insp.natives = append(insp.natives, *sum)
	}

	return insp, nil
}

// licenseRefusalMessage formats a refusal naming the module, what triggered it, and the nsgo.yaml setting that would admit it.
func licenseRefusalMessage(modulePath, where, reason, kind string) string {
	subject := modulePath
	if where != "" {
		subject += " (" + where + ")"
	}
	return fmt.Sprintf("%s %s: add licenses.allow: [%s] or set licenses.%s: warn in nsgo.yaml", subject, reason, modulePath, kind)
}

// cleanPackageError strips local filesystem paths from common go list package errors so plan output stays portable and readable.
func cleanPackageError(msg string) string {
	for _, prefix := range []string{
		"build constraints exclude all Go files",
		"no Go files",
	} {
		if strings.HasPrefix(msg, prefix+" in ") {
			return prefix
		}
	}
	return shortMessage(msg, "does not build")
}

// mergeSortedStrings returns the sorted union of a and b.
func mergeSortedStrings(a, b []string) []string {
	seen := map[string]bool{}
	for _, s := range a {
		seen[s] = true
	}
	for _, s := range b {
		seen[s] = true
	}
	return slices.Sorted(maps.Keys(seen))
}

// licenseMatchesRegistry reports whether the classifier's detected licenses match a registry item's declared SPDX expression.
func licenseMatchesRegistry(detected []string, registrySPDX string) bool {
	want := spdxLicenseIDs(registrySPDX)
	if len(want) == 0 || len(detected) == 0 {
		return false
	}
	norm := func(id string) string {
		id = strings.ToLower(strings.TrimSpace(id))
		id = strings.TrimSuffix(id, "-or-later")
		id = strings.TrimSuffix(id, "-only")
		id = strings.TrimSuffix(id, "+")
		return id
	}
	detSet := map[string]bool{}
	for _, id := range detected {
		detSet[norm(id)] = true
	}
	wantSet := map[string]bool{}
	for _, id := range want {
		wantSet[norm(id)] = true
	}
	return maps.Equal(detSet, wantSet)
}

// spdxLicenseIDs extracts the individual license identifiers from an SPDX expression such as "MIT OR Apache-2.0".
func spdxLicenseIDs(expr string) []string {
	replacer := strings.NewReplacer("(", " ", ")", " ")
	var ids []string
	for _, tok := range strings.Fields(replacer.Replace(expr)) {
		switch strings.ToUpper(tok) {
		case "AND", "OR", "WITH":
			continue
		}
		if !slices.Contains(ids, tok) {
			ids = append(ids, tok)
		}
	}
	return ids
}

// formatModuleLicenseLine formats one module's classified license line for the plan.
func formatModuleLicenseLine(m addModuleLicense) string {
	text := formatLicenseIDs(m.types, m.unknown)
	if m.registryMismatch {
		text += " (registry says " + m.registryLicense + ")"
	}
	var notes []string
	if len(m.copyleft) > 0 {
		switch {
		case m.allowed:
			notes = append(notes, "copyleft; allowed by licenses.allow")
		case m.refusedCopyleft:
			notes = append(notes, "copyleft; refused by licenses.copyleft: error")
		case m.warnCopyleft:
			notes = append(notes, "copyleft; licenses.copyleft: warn")
		}
	}
	if len(m.types) == 0 || len(m.unknown) > 0 {
		switch {
		case m.allowed:
			notes = append(notes, "allowed by licenses.allow")
		case m.refusedUnknown:
			notes = append(notes, "refused by licenses.unknown: error")
		case m.warnUnknown:
			notes = append(notes, "licenses.unknown: warn")
		}
	}
	if len(notes) > 0 {
		text += " (" + strings.Join(notes, "; ") + ")"
	}
	return m.path + ": " + text
}

// formatNativePolicyLine formats a native library's license policy note when it is copyleft or unknown.
func formatNativePolicyLine(n addNativePolicySummary) string {
	hasUnknown := len(n.types) == 0 || len(n.unknown) > 0
	hasCopyleft := len(n.copyleft) > 0
	if !hasUnknown && !hasCopyleft {
		return ""
	}
	text := formatLicenseIDs(n.types, n.unknown)
	var notes []string
	if hasCopyleft {
		switch {
		case n.allowed:
			notes = append(notes, "copyleft; allowed by licenses.allow")
		case n.refusedCopyleft:
			notes = append(notes, "copyleft; refused by licenses.copyleft: error")
		case n.warnCopyleft:
			notes = append(notes, "copyleft; licenses.copyleft: warn")
		}
	}
	if hasUnknown {
		switch {
		case n.allowed:
			notes = append(notes, "allowed by licenses.allow")
		case n.refusedUnknown:
			notes = append(notes, "refused by licenses.unknown: error")
		case n.warnUnknown:
			notes = append(notes, "licenses.unknown: warn")
		}
	}
	return fmt.Sprintf("%s native library %s: %s (%s)", n.modulePath, n.name, text, strings.Join(notes, "; "))
}

// formatLicenseIDs formats detected license IDs and any unrecognized primary license file names.
func formatLicenseIDs(types, unknown []string) string {
	switch {
	case len(types) == 0 && len(unknown) == 0:
		return "unknown"
	case len(types) == 0:
		return "unknown (unrecognized in " + strings.Join(unknown, ", ") + ")"
	case len(unknown) > 0:
		return strings.Join(types, ", ") + " (unrecognized in " + strings.Join(unknown, ", ") + ")"
	default:
		return strings.Join(types, ", ")
	}
}

// formatTargetLines formats a target report's native libraries, excluded packages and manifest errors.
func formatTargetLines(r addTargetReport) []string {
	var lines []string
	if len(r.natives) > 0 {
		parts := make([]string, len(r.natives))
		for i, n := range r.natives {
			detail := formatLicenseIDs(n.types, n.unknown)
			switch {
			case n.host != "":
				detail += ", from " + n.host
			case n.inPackage:
				detail += ", in package"
			}
			parts[i] = fmt.Sprintf("%s (%s)", n.name, detail)
		}
		lines = append(lines, r.target+": "+strings.Join(parts, ", "))
	}
	for _, ex := range r.excluded {
		lines = append(lines, fmt.Sprintf("%s: %s does not build (%s)", r.target, ex.importPath, ex.reason))
	}
	for _, errMsg := range r.errors {
		lines = append(lines, fmt.Sprintf("%s: %s", r.target, errMsg))
	}
	return lines
}
