package main

import (
	"archive/zip"
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	"golang.org/x/mod/sumdb/dirhash"
	modzip "golang.org/x/mod/zip"
)

// Copying components with nsgo add (#2467, #2459).
//
// A component is a Go package declared under components: in its module's
// nsgo-module.yaml. nsgo add downloads the module zip in an isolated directory
// using add's environment, verifies the zip structure and hash against the
// download's Sum and the app's go.sum, validates every file against the shared
// copied-component rule engine (component_rules.go), rewrites imports and
// whole-path comments, generates locales/catalog_gen.go in memory under the
// copy's final import path, checks licenses against the app's policy, plans
// go.mod and go.sum requirements, vets every target through -overlay and
// -mod=readonly, shows the full plan, and writes atomically through hidden
// staging directories under the module's check lock.

const copyStagingPrefix = ".nsgo-copy-"

// copyVetEnv resolves the build environment for vetting a target; tests replace
// it to simulate a host lacking a target's toolchain.
var copyVetEnv = vetEnv

// copyAppConfig holds the app-level settings loaded from nsgo.yaml when present.
type copyAppConfig struct {
	proj          *project // nil when no nsgo.yaml exists
	componentsDir string   // absolute base directory for copied components
	targets       []string // expanded build.targets, or empty to vet host only
	tags          []string // build.tags
	licenses      licensePolicy
	i18nDir       string // absolute path to app's catalogs directory, or empty
}

func loadCopyAppConfig(cwd, modDir string) (*copyAppConfig, error) {
	cfg := &copyAppConfig{
		componentsDir: filepath.Join(modDir, "components"),
	}
	var proj *project
	if p, err := load(cwd); err == nil {
		proj = p
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("add: %w", err)
	} else if absCwd, absErr := filepath.Abs(cwd); absErr == nil && absCwd != modDir {
		if p, err := load(modDir); err == nil {
			proj = p
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("add: %w", err)
		}
	}
	if proj != nil {
		cfg.proj = proj
		dir := proj.Components.Dir
		if dir == "" {
			dir = "components"
		}
		cfg.componentsDir = proj.resolve(dir)
		cfg.targets = proj.Build.Targets
		cfg.tags = proj.Build.Tags
		cfg.licenses = proj.Licenses
		cfg.i18nDir = proj.I18n.Dir
	}
	return cfg, nil
}

// modDownloadResult is the JSON output of `go mod download -json`.
type modDownloadResult struct {
	Path     string
	Version  string
	Info     string
	GoMod    string
	Zip      string
	Dir      string
	Sum      string
	GoModSum string
	Error    string
}

// downloadedModule is a verified module zip ready for reading component files.
type downloadedModule struct {
	path         string
	version      string
	zipPath      string
	sum          string
	goModRaw     []byte
	goMod        *modfile.File
	manifest     *moduleManifest
	zr           *zip.ReadCloser
	entries      map[string]*zip.File // slash-separated path relative to module root -> zip.File
	sortedPaths  []string
	existingReqs map[string]string
}

func (dm *downloadedModule) close() {
	if dm != nil && dm.zr != nil {
		dm.zr.Close()
		dm.zr = nil
	}
}

func (dm *downloadedModule) readFile(relPath string, limit int64) ([]byte, error) {
	zf, ok := dm.entries[relPath]
	if !ok {
		return nil, fs.ErrNotExist
	}
	if limit > 0 && int64(zf.UncompressedSize64) > limit {
		return nil, fmt.Errorf("%s exceeds %d bytes", relPath, limit)
	}
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	if limit > 0 {
		data, err := io.ReadAll(io.LimitReader(rc, limit+1))
		if err != nil {
			return nil, err
		}
		if int64(len(data)) > limit {
			return nil, fmt.Errorf("%s exceeds %d bytes", relPath, limit)
		}
		return data, nil
	}
	return io.ReadAll(rc)
}

// downloadVerifiedModule downloads modPath@query in an isolated directory using
// m's environment, checks the zip with x/mod's zip.CheckZip, verifies its
// dirhash.HashZip against the download's Sum and the app's go.sum (if present),
// and parses go.mod and nsgo-module.yaml directly from the zip.
func (m *addModule) downloadVerifiedModule(ctx context.Context, modPath, query string, appGoSum []byte) (*downloadedModule, error) {
	tmp, err := os.MkdirTemp("", "nsgo-copy-dl-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	cmd := exec.CommandContext(ctx, m.goCmd, "mod", "download", "-json", modPath+"@"+query)
	cmd.Dir = tmp
	cmd.Env = m.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, runErr := cmd.Output()

	var dl modDownloadResult
	decErr := json.Unmarshal(out, &dl)
	if runErr != nil {
		msg := dl.Error
		if msg == "" {
			msg = goMessages(stderr.String())
		}
		needed := ""
		for _, match := range tooNewGo.FindAllStringSubmatch(msg, -1) {
			if strings.Contains(match[1], "@") && (needed == "" || semver.Compare("v"+match[2], "v"+needed) > 0) {
				needed = match[2]
			}
		}
		if needed != "" {
			return nil, fmt.Errorf("download %s@%s: %s\nto move this module to Go %s on purpose, run go get go@%s first", modPath, query, msg, needed, needed)
		}
		if msg != "" {
			return nil, fmt.Errorf("download %s@%s: %s", modPath, query, msg)
		}
		return nil, fmt.Errorf("download %s@%s: %w", modPath, query, runErr)
	}
	if decErr != nil {
		return nil, fmt.Errorf("download %s@%s: %w", modPath, query, decErr)
	}
	if dl.Error != "" {
		return nil, fmt.Errorf("download %s@%s: %s", modPath, query, dl.Error)
	}
	if dl.Zip == "" {
		return nil, fmt.Errorf("download %s@%s returned no zip archive", modPath, query)
	}

	mv := module.Version{Path: dl.Path, Version: dl.Version}
	if _, err := modzip.CheckZip(mv, dl.Zip); err != nil {
		return nil, fmt.Errorf("%s@%s: invalid module zip: %w", dl.Path, dl.Version, err)
	}

	actualHash, err := dirhash.HashZip(dl.Zip, dirhash.Hash1)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: hash zip: %w", dl.Path, dl.Version, err)
	}
	if dl.Sum != "" && actualHash != dl.Sum {
		return nil, fmt.Errorf("%s@%s: zip hash mismatch: archive hashes to %s, download reported %s", dl.Path, dl.Version, actualHash, dl.Sum)
	}
	if sumHash := goSumEntry(appGoSum, dl.Path, dl.Version); sumHash != "" && actualHash != sumHash {
		return nil, fmt.Errorf("%s@%s: go.sum mismatch: archive hashes to %s, go.sum has %s", dl.Path, dl.Version, actualHash, sumHash)
	}

	zr, err := zip.OpenReader(dl.Zip)
	if err != nil {
		return nil, fmt.Errorf("%s@%s: open zip: %w", dl.Path, dl.Version, err)
	}
	dm := &downloadedModule{
		path:    dl.Path,
		version: dl.Version,
		zipPath: dl.Zip,
		sum:     actualHash,
		zr:      zr,
		entries: make(map[string]*zip.File, len(zr.File)),
	}
	prefix := dl.Path + "@" + dl.Version + "/"
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		relPath, ok := strings.CutPrefix(f.Name, prefix)
		if !ok || relPath == "" {
			continue
		}
		dm.entries[relPath] = f
		dm.sortedPaths = append(dm.sortedPaths, relPath)
	}
	slices.Sort(dm.sortedPaths)

	goModRaw, err := dm.readFile("go.mod", maxModuleManifestBytes)
	if err != nil {
		dm.close()
		return nil, fmt.Errorf("%s@%s: read go.mod from zip: %w", dl.Path, dl.Version, err)
	}
	mf, err := modfile.ParseLax("go.mod", goModRaw, nil)
	if err != nil {
		dm.close()
		return nil, fmt.Errorf("%s@%s: parse go.mod: %w", dl.Path, dl.Version, err)
	}
	dm.goModRaw = goModRaw
	dm.goMod = mf

	if modSum := goSumEntry(appGoSum, dl.Path, dl.Version+"/go.mod"); modSum != "" {
		actualModSum, err := dirhash.Hash1([]string{"go.mod"}, func(string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(goModRaw)), nil
		})
		if err == nil && actualModSum != modSum {
			dm.close()
			return nil, fmt.Errorf("%s@%s: go.sum mismatch for go.mod: archive hashes to %s, go.sum has %s", dl.Path, dl.Version, actualModSum, modSum)
		}
	}

	if manifestRaw, err := dm.readFile(moduleManifestName, maxModuleManifestBytes); err == nil {
		reqNSGo := requiredVersion(mf, nsgoModule)
		manifest, err := parseModuleManifest(manifestRaw, dl.Path, reqNSGo, moduleManifestNSGoVersion())
		if err != nil {
			dm.close()
			return nil, fmt.Errorf("%s@%s: %w", dl.Path, dl.Version, err)
		}
		dm.manifest = manifest
	} else if !errors.Is(err, fs.ErrNotExist) {
		dm.close()
		return nil, fmt.Errorf("%s@%s: %w", dl.Path, dl.Version, err)
	}

	return dm, nil
}

// listProxyVersions lists available release versions of modPath from the proxy
// in an isolated directory using m's environment.
func (m *addModule) listProxyVersions(ctx context.Context, modPath string) ([]string, error) {
	tmp, err := os.MkdirTemp("", "nsgo-copy-vers-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.invalid/nsgo-list\n\ngo 1.22\n"), 0o644); err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, m.goCmd, "list", "-m", "-versions", "-json", modPath)
	cmd.Dir = tmp
	cmd.Env = m.env
	out, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	var res struct {
		Versions []string
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return nil, err
	}
	return res.Versions, nil
}

// goSumEntry returns the recorded hash for modPath and version in goSum, or "".
func goSumEntry(goSum []byte, modPath, version string) string {
	for line := range strings.SplitSeq(string(goSum), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == modPath && fields[1] == version {
			return fields[2]
		}
	}
	return ""
}

// copiedFile is a file ready to be written into a copied component directory.
type copiedFile struct {
	relPath   string
	data      []byte
	isTest    bool
	isLicense bool
	isLocale  bool
	generated bool
	buildTag  string // build constraint expression for test files, when present
}

type importRewriteStat struct {
	oldPath  string
	newPath  string
	files    int
	comments int
}

type stringLiteralNotice struct {
	file    string
	line    int
	oldPath string
}

type catalogSupplyHint struct {
	file   string
	oldKey string
	newKey string
}

type copyVetResult struct {
	vetted    []string
	notVetted []struct {
		target string
		reason string
	}
	failures []struct {
		target string
		output string
	}
}

// plannedComponent is one component planned for copying into the app module.
type plannedComponent struct {
	displayName string // bare name if added by registry name, else mod#comp
	fromName    string // bare registry name when added by name
	srcMod      string
	srcVersion  string
	srcDir      string // slash-separated relative to source module root
	compName    string // declared component name in nsgo-module.yaml
	pkgName     string // Go package name (last element of srcDir)
	explicit    bool   // named directly on the command line (vs pulled in as a dependency)

	// Relationship to the app's existing requirement of srcMod.
	appReqVer string // version of srcMod already required by app, or ""
	raisesMod bool   // true when srcVersion > appReqVer
	olderCopy bool   // true when typed srcVersion < appReqVer

	destAbs    string // absolute destination directory
	destRel    string // slash-separated relative to module root
	importPath string // <appMod>/<destRel>

	tests       bool
	files       []copiedFile // sorted by relPath (includes generated files, excludes nsgo-copy.yaml)
	record      *copyRecord
	recordBytes []byte

	rewrites       []importRewriteStat
	stringLiterals []stringLiteralNotice
	supplyHints    []catalogSupplyHint

	licenseIDs      []string // recognized SPDX/classifier license IDs
	licenseWarnings []string // policy warnings (e.g. copyleft under warn)
	licenseRefusals []string // policy refusals (e.g. copyleft under error, unknown)

	flags []componentFinding // #2466 flag warnings (refused under -y)
	notes []string           // author notes from nsgo-module.yaml (if any)

	extImports []string // external/non-component imports (after rewriting)
	dm         *downloadedModule
}

// copyPlan is the complete plan for copying one or more components.
type copyPlan struct {
	m          *addModule
	app        *copyAppConfig
	appModPath string
	registries []*loadedRegistry // custom registries resolved for this plan, in order

	components []*plannedComponent
	already    []moduleCopy // requested components already copied at the same version

	// Planned go.mod / go.sum changes.
	oldMod, newMod, oldSum, newSum []byte
	hadSum, hasSum                 bool
	before, after                  *modfile.File

	// Refusals collected during planning (always fail, -y or not).
	refusals []string
	// Vet results across targets.
	vet copyVetResult

	// Show full files in output (-diff).
	showDiff bool
	// Direct requirements needed by copied components.
	directMods map[string]bool
	// Modules that moved NSGo (for catalog regeneration notice).
	movedNSGo bool
}

// addCopyComponentsWithModule runs the component-copying path of nsgo add using an already-discovered module and resolved requests.
func addCopyComponentsWithModule(ctx context.Context, m *addModule, o addOptions, reqs []addRequest, usedRegistries []*loadedRegistry, s *Step, out io.Writer, ask func(context.Context) (bool, error)) error {
	if o.dir != "" && len(reqs) != 1 {
		s.remove()
		return errors.New("add: -dir applies to one component")
	}
	for _, r := range reqs {
		if r.component == "" {
			s.remove()
			return fmt.Errorf("add: cannot mix library %s and component requests in one command; run them separately", r.path)
		}
	}
	planning := func(err error) error {
		if ctx.Err() != nil {
			return fmt.Errorf("add: interrupted while planning, so nothing was written: %w", ctx.Err())
		}
		return err
	}
	p, err := m.planCopy(withStep(ctx, s), o, reqs)
	s.remove()
	if err != nil {
		return planning(err)
	}
	defer p.close()
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
	if flagErr := p.flagError(); flagErr != nil && o.yes {
		return flagErr
	}
	if !o.yes {
		if ask == nil {
			return fmt.Errorf("add: the plan was not applied, as nobody can confirm it without a terminal: run %s add -y %s to apply it", nsgoCommand(), formatAddRequestLabels(reqs))
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
	for _, c := range p.components {
		done("copied %s (%s %s) → %s", c.compName, c.srcMod, c.srcVersion, c.destRel)
	}
	if p.m.vendored() {
		detail("next: go mod vendor")
	}
	for _, c := range p.components {
		if c.explicit {
			detail("next: import %q", c.importPath)
		}
	}
	return nil
}

func formatAddRequestLabels(reqs []addRequest) string {
	parts := make([]string, len(reqs))
	for i, r := range reqs {
		parts[i] = r.displayArg()
	}
	return strings.Join(parts, " ")
}

func (p *copyPlan) close() {
	seen := map[*downloadedModule]bool{}
	for _, c := range p.components {
		if c.dm != nil && !seen[c.dm] {
			seen[c.dm] = true
			c.dm.close()
		}
	}
}

func (p *copyPlan) changes() bool {
	return len(p.components) > 0 || !bytes.Equal(p.oldMod, p.newMod) || p.hadSum != p.hasSum || !bytes.Equal(p.oldSum, p.newSum)
}

func (p *copyPlan) refusal() error {
	var reasons []string
	reasons = append(reasons, p.refusals...)
	for _, c := range p.components {
		reasons = append(reasons, c.licenseRefusals...)
	}
	// Check NSGo moving to non-release (add's rule).
	oldNSGo, newNSGo := requiredVersion(p.before, nsgoModule), requiredVersion(p.after, nsgoModule)
	if newNSGo != oldNSGo && semver.Prerelease(newNSGo) != "" {
		kind := "a pre-release"
		if module.IsPseudoVersion(newNSGo) {
			kind = "a commit with no release"
		}
		reasons = append(reasons, fmt.Sprintf("the plan moves %s from %s to %s, %s: to move to it on purpose, run go get %s@%s first", nsgoModule, cmp.Or(oldNSGo, "none"), newNSGo, kind, nsgoModule, newNSGo))
	}
	if len(reasons) == 0 {
		return nil
	}
	return errors.New("add: " + strings.Join(reasons, "\n"))
}

func (p *copyPlan) flagError() error {
	var lines []string
	for _, c := range p.components {
		for _, f := range c.flags {
			lines = append(lines, formatComponentFinding(c.destRel, f))
		}
	}
	for _, vf := range p.vet.failures {
		lines = append(lines, fmt.Sprintf("go vet failed for %s:\n%s", vf.target, vf.output))
	}
	if len(lines) == 0 {
		return nil
	}
	return fmt.Errorf("add: refused under -y because the copy has flagged items (review and confirm interactively without -y):\n%s", strings.Join(lines, "\n"))
}

// planCopy prepares the full copy plan without modifying any file in the module.
func (m *addModule) planCopy(ctx context.Context, o addOptions, reqs []addRequest) (plan *copyPlan, retErr error) {
	app, err := loadCopyAppConfig(m.cwd, m.dir)
	if err != nil {
		return nil, err
	}
	p := &copyPlan{
		m:          m,
		app:        app,
		showDiff:   o.diff,
		directMods: map[string]bool{},
	}
	defer func() {
		if retErr != nil {
			p.close()
		}
	}()

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
	p.appModPath = p.before.Module.Mod.Path
	p.newMod, p.newSum, p.hasSum, p.after = p.oldMod, p.oldSum, p.hadSum, p.before

	existingCopies, err := findModuleCopies(m.dir, p.appModPath)
	if err != nil {
		return nil, fmt.Errorf("add: %w", err)
	}

	// Track downloaded modules by (modPath, version) so transitive components
	// reuse the same verified zip reader.
	downloads := map[string]*downloadedModule{}
	defer func() {
		// Any download not attached to p.components gets closed here.
		attached := map[*downloadedModule]bool{}
		if retErr == nil && p != nil {
			for _, c := range p.components {
				attached[c.dm] = true
			}
		}
		for _, dm := range downloads {
			if !attached[dm] {
				dm.close()
			}
		}
	}()

	type compKey struct{ mod, comp string }
	existingByKey := map[compKey]moduleCopy{}
	for _, ec := range existingCopies {
		existingByKey[compKey{ec.record.Module, ec.record.Component}] = ec
	}

	// Map of (srcMod, srcDir) -> plannedComponent for components being copied in this plan.
	plannedBySrc := map[compKey]*plannedComponent{}
	// Map of lower-case destination relDir -> component name, to catch destination collisions inside one plan.
	claimedDestLower := map[string]string{}

	var queue []struct {
		req         addRequest
		explicit    bool
		explicitDir string
	}
	for _, r := range reqs {
		queue = append(queue, struct {
			req         addRequest
			explicit    bool
			explicitDir string
		}{req: r, explicit: true, explicitDir: o.dir})
	}

	includeTests := !o.skipTests

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		r := cur.req

		// Refuse a component from a module that go.mod replaces.
		if hasModuleReplace(p.before, r.path) {
			return nil, fmt.Errorf("add: %s is replaced in go.mod; copying components from a replaced module is not supported", r.path)
		}

		appReqVer := requiredVersion(p.before, r.path)
		resolvedQuery, finalAppReq, err := resolveComponentVersionQuery(r, appReqVer)
		if err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}

		stage(ctx, "downloading %s@%s", r.path, resolvedQuery)
		dm, err := m.getOrDownloadModule(ctx, downloads, r.path, resolvedQuery, appReqVer, r.typed, p.oldSum)
		if err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}
		if r.fromName != "" && !r.typed && appReqVer == "" && module.IsPseudoVersion(dm.version) {
			return nil, fmt.Errorf("add: %s has no release; add its module by path with a version: %s add %s@%s#%s", r.fromName, nsgoCommand(), dm.path, dm.version, r.component)
		}

		// Verify the module's nsgo-module.yaml declares r.component.
		if dm.manifest == nil || dm.manifest.Components[r.component] == "" {
			if finalAppReq {
				hint := m.suggestVersionWithComponent(ctx, r, dm.version, p.oldSum)
				return nil, fmt.Errorf("add: %s %s declares no component %s; %s", dm.path, dm.version, r.component, hint)
			}
			return nil, fmt.Errorf("add: %s %s declares no component %s", dm.path, dm.version, r.component)
		}
		srcDir := dm.manifest.Components[r.component]
		pkgName := path.Base(srcDir)
		if pkgName == "main" {
			return nil, fmt.Errorf("add: %s %s component %q (%s) is a main package", dm.path, dm.version, r.component, srcDir)
		}

		// Check if already planned in this batch.
		key := compKey{dm.path, r.component}
		if _, alreadyPlanned := plannedBySrc[key]; alreadyPlanned {
			continue
		}

		// Check existing copy of this (module, component).
		if ec, exists := existingByKey[key]; exists {
			if cur.explicitDir != "" {
				destAbs, _, err := resolveCopyDestination(m.dir, m.cwd, app.componentsDir, cur.explicitDir, pkgName, true)
				if err != nil {
					return nil, fmt.Errorf("add: %w", err)
				}
				if filepath.Clean(destAbs) != filepath.Clean(ec.dir) {
					return nil, fmt.Errorf("add: %s (%s) is already copied at %s; a second copy in the same module is not allowed", r.component, dm.path, ec.relDir)
				}
			}
			if ec.record.ModuleVersion == dm.version {
				if cur.explicit {
					p.already = append(p.already, ec)
				}
				continue
			}
			nameForDiff := r.fromName
			if nameForDiff == "" {
				nameForDiff = r.component
			}
			return nil, fmt.Errorf("add: %s is already copied at %s (%s); updating an existing copy to %s is not supported yet (compare with %s diff %s@%s)", r.component, ec.relDir, ec.record.ModuleVersion, dm.version, nsgoCommand(), nameForDiff, dm.version)
		}

		// Determine and validate destination directory.
		destAbs, destRel, err := resolveCopyDestination(m.dir, m.cwd, app.componentsDir, cur.explicitDir, pkgName, cur.explicitDir != "")
		if err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}
		if err := validateCopyDestination(m.dir, destAbs, destRel, r.component, cur.explicit); err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}
		lowDest := strings.ToLower(destRel)
		if prev, dup := claimedDestLower[lowDest]; dup {
			return nil, fmt.Errorf("add: %s and %s both map to %s; copy one first with %s add -dir <dir> <name>", prev, r.component, destRel, nsgoCommand())
		}
		claimedDestLower[lowDest] = r.component

		// Collect zip entries inside srcDir and validate against #2466 rules.
		compFiles, zipRawFiles, err := collectComponentZipFiles(dm, srcDir, includeTests)
		if err != nil {
			return nil, fmt.Errorf("add: %s#%s: %w", dm.path, r.component, err)
		}

		compDirs := make(map[string]bool, len(dm.manifest.Components))
		for _, d := range dm.manifest.Components {
			compDirs[d] = true
		}
		ruleRes := checkComponentRules(compFiles, componentCheckOptions{
			ModulePath:    dm.path,
			ComponentDir:  srcDir,
			ComponentDirs: compDirs,
		})
		if len(ruleRes.Errors) > 0 {
			var lines []string
			for _, e := range ruleRes.Errors {
				lines = append(lines, formatComponentFinding(srcDir, e))
			}
			return nil, fmt.Errorf("add: %s#%s breaks copied-component rules:\n%s", dm.path, r.component, strings.Join(lines, "\n"))
		}

		// Collect ancestor license documents from module root down to parent(srcDir).
		ancestorLicenses, err := collectAncestorLicenses(dm, srcDir, zipRawFiles)
		if err != nil {
			return nil, fmt.Errorf("add: %s#%s: %w", dm.path, r.component, err)
		}

		// Discover any same-module component imports and enqueue them.
		sameModCompByImport := map[string]string{}
		for name, d := range dm.manifest.Components {
			sameModCompByImport[dm.path+"/"+d] = name
		}
		for _, f := range zipRawFiles {
			if !strings.HasSuffix(strings.ToLower(f.relPath), ".go") {
				continue
			}
			// Skip generated catalog_gen.go from zip.
			if strings.EqualFold(f.relPath, "locales/"+genFile) {
				continue
			}
			imps := parseGoFileImports(f.data)
			for _, imp := range imps {
				if depComp, ok := sameModCompByImport[imp]; ok && depComp != r.component {
					if _, done := plannedBySrc[compKey{dm.path, depComp}]; !done {
						queue = append(queue, struct {
							req         addRequest
							explicit    bool
							explicitDir string
						}{
							req: addRequest{
								path:      dm.path,
								version:   dm.version,
								typed:     true,
								component: depComp,
							},
							explicit: false,
						})
					}
				}
			}
		}

		pc := &plannedComponent{
			displayName: r.displayName(),
			fromName:    r.fromName,
			srcMod:      dm.path,
			srcVersion:  dm.version,
			srcDir:      srcDir,
			compName:    r.component,
			pkgName:     pkgName,
			explicit:    cur.explicit,
			appReqVer:   appReqVer,
			raisesMod:   appReqVer != "" && semver.Compare(dm.version, appReqVer) > 0,
			olderCopy:   appReqVer != "" && r.typed && semver.Compare(dm.version, appReqVer) < 0,
			destAbs:     destAbs,
			destRel:     destRel,
			importPath:  p.appModPath + "/" + destRel,
			tests:       includeTests,
			flags:       ruleRes.Flags,
			dm:          dm,
		}
		pc.notes = dm.manifest.formattedNotes()

		// Store raw files + ancestor licenses temporarily on pc.files; we rewrite
		// Go files once all transitive components' destinations are known.
		for _, rf := range zipRawFiles {
			if strings.EqualFold(rf.relPath, "locales/"+genFile) {
				continue // regenerated from catalogs under final import path
			}
			data := rf.data
			if isComponentTextFile(rf.relPath) {
				data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))
			}
			_, isLic := isLicenseDocument(path.Base(rf.relPath))
			isTest := isComponentTestFile(rf.relPath)
			isLoc := strings.EqualFold(path.Dir(rf.relPath), "locales")
			pc.files = append(pc.files, copiedFile{
				relPath:   rf.relPath,
				data:      data,
				isTest:    isTest,
				isLicense: isLic,
				isLocale:  isLoc,
			})
		}
		for _, al := range ancestorLicenses {
			pc.files = append(pc.files, copiedFile{
				relPath:   al.relPath,
				data:      bytes.ReplaceAll(al.data, []byte("\r\n"), []byte("\n")),
				isLicense: true,
			})
		}
		slices.SortFunc(pc.files, func(a, b copiedFile) int {
			return cmp.Compare(a.relPath, b.relPath)
		})

		plannedBySrc[key] = pc
		p.components = append(p.components, pc)
	}

	if len(p.components) == 0 {
		return p, nil
	}

	// Build import rewrite map per source module (covering newly planned copies
	// AND any existing copies from the same source module).
	rewritesByMod := map[string]map[string]string{}
	addRewrite := func(srcMod, srcDir, copyImportPath string) {
		if rewritesByMod[srcMod] == nil {
			rewritesByMod[srcMod] = map[string]string{}
		}
		rewritesByMod[srcMod][srcMod+"/"+srcDir] = copyImportPath
		rewritesByMod[srcMod][srcMod+"/"+srcDir+"/locales"] = copyImportPath + "/locales"
	}
	for _, c := range p.components {
		addRewrite(c.srcMod, c.srcDir, c.importPath)
	}
	// Also map existing copies from any source module involved in this plan.
	for _, ec := range existingCopies {
		for _, c := range p.components {
			if ec.record.Module == c.srcMod && c.dm.manifest != nil {
				if srcDir := c.dm.manifest.Components[ec.record.Component]; srcDir != "" {
					addRewrite(ec.record.Module, srcDir, ec.importPath)
				}
			}
		}
	}

	// Rewrite Go files, extract test build tags, detect string literals, check
	// licenses against app policy, and collect external imports.
	for _, c := range p.components {
		rwMap := rewritesByMod[c.srcMod]
		statsMap := map[string]*importRewriteStat{}
		extSet := map[string]bool{}

		for i := range c.files {
			f := &c.files[i]
			if !strings.HasSuffix(strings.ToLower(f.relPath), ".go") {
				continue
			}
			formatted, fileRewrites, strLits, imps, buildExpr, err := rewriteComponentGoFile(f.relPath, f.data, rwMap)
			if err != nil {
				return nil, fmt.Errorf("add: %s/%s: %w", c.srcDir, f.relPath, err)
			}
			f.data = formatted
			if f.isTest {
				f.buildTag = buildExpr
			}
			for oldP, rw := range fileRewrites {
				st := statsMap[oldP]
				if st == nil {
					st = &importRewriteStat{oldPath: oldP, newPath: rwMap[oldP]}
					statsMap[oldP] = st
				}
				if rw.inImport {
					st.files++
				}
				st.comments += rw.comments
			}
			c.stringLiterals = append(c.stringLiterals, strLits...)
			for _, imp := range imps {
				if !isStdlibImport(imp) && imp != p.appModPath && !strings.HasPrefix(imp, p.appModPath+"/") {
					extSet[imp] = true
				}
			}
		}
		for _, oldP := range slices.Sorted(maps.Keys(statsMap)) {
			c.rewrites = append(c.rewrites, *statsMap[oldP])
		}
		c.extImports = slices.Sorted(maps.Keys(extSet))

		// Classify licenses and apply app's license policy.
		if err := evaluateCopyLicenses(c, p.app.licenses); err != nil {
			return nil, fmt.Errorf("add: %w", err)
		}

		// Check if any app catalog supplies this component's original locales.
		if hasComponentLocales(c) && p.app.i18nDir != "" && hasCatalogs(p.app.i18nDir) {
			origLocales := c.srcMod + "/" + c.srcDir + "/locales"
			copyLocales := c.importPath + "/locales"
			c.supplyHints = findAppCatalogSupplies(m.dir, p.app.i18nDir, origLocales, copyLocales)
		}
	}

	// Plan go.mod and go.sum requirements needed by the copied components.
	stage(ctx, "planning go.mod requirements")
	if err := p.planRequirements(ctx); err != nil {
		return nil, err
	}

	// Check source modules' go language version against the app's planned go line.
	appGoAfter := ""
	if p.after.Go != nil {
		appGoAfter = p.after.Go.Version
	}
	for _, c := range p.components {
		srcGo := ""
		if c.dm.goMod.Go != nil {
			srcGo = c.dm.goMod.Go.Version
		}
		if srcGo == "" || semver.Compare("v"+srcGo, "v1.22") < 0 {
			return nil, fmt.Errorf("add: %s %s has go %s, below Go 1.22 (loop variable semantics would change when copied)", c.srcMod, c.srcVersion, cmp.Or(srcGo, "none"))
		}
		if appGoAfter != "" && semver.Compare("v"+srcGo, "v"+appGoAfter) > 0 {
			return nil, fmt.Errorf("add: %s %s requires go %s, newer than this module's go %s: run go get go@%s first", c.srcMod, c.srcVersion, srcGo, appGoAfter, srcGo)
		}
	}

	// Generate locales/catalog_gen.go in memory for components with catalogs,
	// resolving any external supplies with the planned go.mod through -overlay.
	stage(ctx, "generating catalogs")
	for _, c := range p.components {
		if !hasComponentLocales(c) {
			continue
		}
		genBytes, err := p.generateComponentCatalog(ctx, c)
		if err != nil {
			return nil, fmt.Errorf("add: %s: %w", c.destRel, err)
		}
		c.files = append(c.files, copiedFile{
			relPath:   "locales/" + genFile,
			data:      genBytes,
			isLocale:  true,
			generated: true,
		})
		slices.SortFunc(c.files, func(a, b copiedFile) int {
			return cmp.Compare(a.relPath, b.relPath)
		})
	}

	// Build each component's deterministic nsgo-copy.yaml record.
	for _, c := range p.components {
		var recFiles, recGen []string
		for _, f := range c.files {
			if f.generated {
				recGen = append(recGen, f.relPath)
			} else {
				recFiles = append(recFiles, f.relPath)
			}
		}
		slices.Sort(recFiles)
		slices.Sort(recGen)
		c.record = &copyRecord{
			Version:       copyRecordVersion,
			Module:        c.srcMod,
			ModuleVersion: c.srcVersion,
			Component:     c.compName,
			Tests:         c.tests,
			Files:         recFiles,
			Generated:     recGen,
		}
		recBytes, err := formatCopyRecord(c.record)
		if err != nil {
			return nil, fmt.Errorf("add: %s: %w", c.destRel, err)
		}
		c.recordBytes = recBytes
	}

	// Vet the planned copies across target(s) using -overlay and -mod=readonly
	// (skip vetting if there are already hard refusals that prevent a valid build).
	if p.refusal() == nil {
		stage(ctx, "vetting copied components")
		if err := p.vetCopies(ctx); err != nil {
			return nil, err
		}
	}

	return p, nil
}

func (m *addModule) getOrDownloadModule(ctx context.Context, cache map[string]*downloadedModule, modPath, query, appReqVer string, typed bool, appGoSum []byte) (*downloadedModule, error) {
	// Resolve @upgrade: if not typed or typed as "upgrade", download @latest and
	// never drop below appReqVer.
	dlQuery := query
	if query == "upgrade" {
		dlQuery = "latest"
	}
	cacheKey := modPath + "@" + dlQuery
	if dm, ok := cache[cacheKey]; ok {
		return dm, nil
	}
	dm, err := m.downloadVerifiedModule(ctx, modPath, dlQuery, appGoSum)
	if err != nil {
		return nil, err
	}
	if query == "upgrade" && appReqVer != "" && semver.Compare(appReqVer, dm.version) > 0 {
		dm.close()
		return m.getOrDownloadModule(ctx, cache, modPath, appReqVer, appReqVer, typed, appGoSum)
	}
	cache[cacheKey] = dm
	cache[modPath+"@"+dm.version] = dm
	return dm, nil
}

func resolveComponentVersionQuery(r addRequest, appReqVer string) (query string, finalAppReq bool, err error) {
	switch {
	case r.typed:
		if r.version == "none" {
			return "", false, fmt.Errorf("%s@none would remove %s; add never removes a module", r.path, r.path)
		}
		return r.version, false, nil
	case appReqVer != "":
		return appReqVer, true, nil
	case r.itemVersion != "":
		return r.itemVersion, false, nil
	default:
		return "upgrade", false, nil
	}
}

func (m *addModule) suggestVersionWithComponent(ctx context.Context, r addRequest, curVer string, appGoSum []byte) string {
	cmdName := nsgoCommand()
	formatCmd := func(ver string) string {
		if r.fromName != "" {
			return fmt.Sprintf("%s add %s@%s", cmdName, r.fromName, ver)
		}
		return fmt.Sprintf("%s add %s@%s#%s", cmdName, r.path, ver, r.component)
	}
	versions, err := m.listProxyVersions(ctx, r.path)
	if err == nil {
		// Check newer release versions from newest downward for one that declares r.component.
		var newer []string
		for _, v := range versions {
			if semver.Prerelease(v) == "" && semver.Compare(v, curVer) > 0 {
				newer = append(newer, v)
			}
		}
		semver.Sort(newer)
		for i := len(newer) - 1; i >= 0; i-- {
			v := newer[i]
			dm, err := m.downloadVerifiedModule(ctx, r.path, v, appGoSum)
			if err != nil {
				continue
			}
			has := dm.manifest != nil && dm.manifest.Components[r.component] != ""
			dm.close()
			if has {
				return fmt.Sprintf("%s raises it to %s", formatCmd(v), v)
			}
		}
	}
	return fmt.Sprintf("run %s to take a version that does", formatCmd("<version>"))
}

func hasModuleReplace(f *modfile.File, modPath string) bool {
	if f == nil {
		return false
	}
	for _, r := range f.Replace {
		if r.Old.Path == modPath {
			return true
		}
	}
	return false
}

func hasLocalDirReplace(f *modfile.File, modPath string) bool {
	if f == nil {
		return false
	}
	for _, r := range f.Replace {
		if r.Old.Path == modPath && r.Old.Version == "" && r.New.Version == "" {
			return true
		}
	}
	return false
}

func resolveCopyDestination(modDir, cwd, baseDir, explicitDir, pkgName string, hasExplicit bool) (destAbs, destRel string, err error) {
	if hasExplicit {
		if filepath.IsAbs(explicitDir) {
			destAbs = filepath.Clean(explicitDir)
		} else {
			destAbs = filepath.Clean(filepath.Join(cwd, explicitDir))
		}
		if filepath.Base(destAbs) != pkgName {
			return "", "", fmt.Errorf("-dir %q must end in the package's name %q", explicitDir, pkgName)
		}
	} else {
		destAbs = filepath.Clean(filepath.Join(baseDir, pkgName))
	}
	relPath, err := filepath.Rel(modDir, destAbs)
	if err != nil || relPath == "." || !filepath.IsLocal(relPath) {
		return "", "", fmt.Errorf("destination %q is outside the module %s", destAbs, modDir)
	}
	return destAbs, filepath.ToSlash(relPath), nil
}

func validateCopyDestination(modDir, destAbs, destRel, compName string, explicit bool) error {
	// Refuse vendor, testdata, and any segment starting with '.' or '_'.
	for seg := range strings.SplitSeq(destRel, "/") {
		if seg == "" || seg == "." || seg == ".." {
			return fmt.Errorf("invalid destination %q", destRel)
		}
		if seg == "vendor" || seg == "testdata" || seg[0] == '.' || seg[0] == '_' {
			return fmt.Errorf("destination %q is not allowed: directory %q is skipped by Go and nsgo", destRel, seg)
		}
	}

	// Walk every ancestor from modDir down to destAbs: refuse symlinks and nested modules.
	cur := modDir
	segs := strings.Split(destRel, "/")
	for i, seg := range segs {
		parent := cur
		cur = filepath.Join(cur, seg)
		info, err := os.Lstat(cur)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("destination %q passes through a symlink (%s)", destRel, relTo(modDir, cur))
			}
			if i == len(segs)-1 {
				if !explicit {
					return fmt.Errorf("destination %s already exists for required component %s; copy it elsewhere first with %s add -dir <dir> %s", destRel, compName, nsgoCommand(), compName)
				}
				return fmt.Errorf("destination %s already exists and is not a copy of %s", destRel, compName)
			}
			if !info.IsDir() {
				return fmt.Errorf("destination %q: %s is not a directory", destRel, relTo(modDir, cur))
			}
			if st, err := os.Lstat(filepath.Join(cur, "go.mod")); err == nil && !st.IsDir() {
				return fmt.Errorf("destination %q is inside a nested module (%s/go.mod)", destRel, relTo(modDir, cur))
			}
		} else if errors.Is(err, fs.ErrNotExist) {
			// Check parent directory for a case-insensitive collision with seg!
			if entries, readErr := os.ReadDir(parent); readErr == nil {
				for _, e := range entries {
					if strings.EqualFold(e.Name(), seg) {
						existingPath := filepath.ToSlash(filepath.Join(relTo(modDir, parent), e.Name()))
						if existingPath == "./"+e.Name() {
							existingPath = e.Name()
						}
						if i == len(segs)-1 {
							if !explicit {
								return fmt.Errorf("destination %s collides case-insensitively with existing %s for required component %s; copy it elsewhere first with %s add -dir <dir> %s", destRel, existingPath, compName, nsgoCommand(), compName)
							}
							return fmt.Errorf("destination %s collides case-insensitively with existing %s", destRel, existingPath)
						}
						return fmt.Errorf("destination %q collides case-insensitively with existing %s", destRel, existingPath)
					}
				}
			}
			// Remaining deeper segments don't exist yet, so no nested go.mod or symlinks below.
			break
		} else {
			return err
		}
	}
	return nil
}

type rawZipFile struct {
	relPath string
	data    []byte
}

func isComponentTestFile(relPath string) bool {
	low := strings.ToLower(relPath)
	if strings.HasSuffix(low, "_test.go") {
		return true
	}
	return low == "testdata" || strings.HasPrefix(low, "testdata/")
}

func collectComponentZipFiles(dm *downloadedModule, srcDir string, includeTests bool) ([]componentFile, []rawZipFile, error) {
	prefix := srcDir + "/"
	var files []componentFile
	var raws []rawZipFile

	for _, p := range dm.sortedPaths {
		relPath, ok := strings.CutPrefix(p, prefix)
		if !ok || relPath == "" {
			continue
		}
		if !includeTests && isComponentTestFile(relPath) {
			continue
		}
		zf := dm.entries[p]
		size := int64(zf.UncompressedSize64)
		zfile := zf
		files = append(files, componentFile{
			Path: relPath,
			Size: size,
			Open: func() (io.ReadCloser, error) {
				return zfile.Open()
			},
		})
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("component directory %q has no files", srcDir)
	}

	// Read allowed files within limits into raws (checkComponentRules will report
	// detailed errors for disallowed/oversized files first, so skip reading
	// oversized files here and let checkComponentRules report them).
	var total int64
	for _, f := range files {
		if f.Size > maxComponentFileBytes {
			continue
		}
		total += f.Size
		if total > maxComponentTotalBytes {
			break
		}
		if componentFileAllowed(f.Path) != nil {
			continue
		}
		data, err := readBoundedFile(f.Open, maxComponentFileBytes)
		if err != nil {
			continue
		}
		raws = append(raws, rawZipFile{relPath: f.Path, data: data})
	}
	return files, raws, nil
}

// collectAncestorLicenses collects license documents from the source module
// root and every directory strictly between the module root and srcDir. Two
// documents with the same name (case-insensitively) across those directories
// and the component root are refused.
func collectAncestorLicenses(dm *downloadedModule, srcDir string, compFiles []rawZipFile) ([]rawZipFile, error) {
	seenLower := map[string]string{}
	for _, cf := range compFiles {
		if path.Dir(cf.relPath) == "." {
			seenLower[strings.ToLower(cf.relPath)] = srcDir + "/" + cf.relPath
		}
	}

	// Build ancestor directory list: "", then each prefix strictly above srcDir.
	var ancestors []string
	ancestors = append(ancestors, "")
	parts := strings.Split(srcDir, "/")
	for i := 1; i < len(parts); i++ {
		ancestors = append(ancestors, strings.Join(parts[:i], "/"))
	}

	var result []rawZipFile
	for _, dir := range ancestors {
		for _, p := range dm.sortedPaths {
			parent := path.Dir(p)
			if parent == "." {
				parent = ""
			}
			if parent != dir {
				continue
			}
			base := path.Base(p)
			_, include := isLicenseDocument(base)
			if !include {
				continue
			}
			if err := componentFileAllowed(base); err != nil {
				return nil, fmt.Errorf("ancestor license %s: %w", p, err)
			}
			low := strings.ToLower(base)
			if prev, dup := seenLower[low]; dup {
				return nil, fmt.Errorf("duplicate license document name %q in %s and %s", base, prev, p)
			}
			seenLower[low] = p
			data, err := dm.readFile(p, maxComponentFileBytes)
			if err != nil {
				return nil, fmt.Errorf("read %s: %w", p, err)
			}
			if !utf8.Valid(data) {
				return nil, fmt.Errorf("%s is not valid UTF-8", p)
			}
			result = append(result, rawZipFile{relPath: base, data: data})
		}
	}
	return result, nil
}

func parseGoFileImports(data []byte) []string {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", data, parser.ImportsOnly)
	if err != nil {
		return nil
	}
	var imps []string
	for _, imp := range file.Imports {
		if v, err := strconv.Unquote(imp.Path.Value); err == nil {
			imps = append(imps, v)
		}
	}
	return imps
}

type fileRewriteHit struct {
	inImport bool
	comments int
}

// rewriteComponentGoFile rewrites import paths and whole-path occurrences in
// comments according to rewrites, lists any string literals containing a
// rewritten path (leaving them untouched), extracts build constraints, and
// formats the result with go/format.
func rewriteComponentGoFile(relPath string, data []byte, rewrites map[string]string) (
	formatted []byte,
	hits map[string]fileRewriteHit,
	strLits []stringLiteralNotice,
	finalImports []string,
	buildExpr string,
	err error,
) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, data, parser.ParseComments)
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	hits = map[string]fileRewriteHit{}

	// Extract //go:build constraint expression before package clause.
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			if c.Pos() >= file.Package {
				continue
			}
			if constraint.IsGoBuild(c.Text) {
				if expr, err := constraint.Parse(c.Text); err == nil {
					buildExpr = expr.String()
				}
			} else if buildExpr == "" && constraint.IsPlusBuild(c.Text) {
				if expr, err := constraint.Parse(c.Text); err == nil {
					buildExpr = expr.String()
				}
			}
		}
	}

	// Sort rewrite keys longest-first so subpackages (e.g. .../locales) match
	// before their parent package in comments and string literals.
	oldPaths := slices.SortedFunc(maps.Keys(rewrites), func(a, b string) int {
		if len(a) != len(b) {
			return cmp.Compare(len(b), len(a))
		}
		return cmp.Compare(a, b)
	})

	// Rewrite imports, preserving any explicit import name.
	importLits := map[*ast.BasicLit]bool{}
	for _, imp := range file.Imports {
		importLits[imp.Path] = true
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if newP, ok := rewrites[p]; ok {
			imp.Path.Value = strconv.Quote(newP)
			h := hits[p]
			h.inImport = true
			hits[p] = h
			p = newP
		}
		finalImports = append(finalImports, p)
	}

	// Rewrite whole-path occurrences in comments.
	if len(oldPaths) > 0 {
		for _, cg := range file.Comments {
			for _, c := range cg.List {
				updated, matched := rewriteCommentWholePaths(c.Text, oldPaths, rewrites)
				if updated != c.Text {
					c.Text = updated
					for _, oldP := range matched {
						h := hits[oldP]
						h.comments++
						hits[oldP] = h
					}
				}
			}
		}
	}

	// List string literals containing any rewritten path (leaving them untouched).
	if len(oldPaths) > 0 {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || importLits[lit] {
				return true
			}
			val, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, oldP := range oldPaths {
				if containsWholeImportPath(val, oldP) {
					pos := fset.Position(lit.Pos())
					strLits = append(strLits, stringLiteralNotice{
						file:    relPath,
						line:    pos.Line,
						oldPath: oldP,
					})
					break
				}
			}
			return true
		})
	}

	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, nil, nil, nil, "", err
	}
	formatted, err = format.Source(buf.Bytes())
	if err != nil {
		return nil, nil, nil, nil, "", err
	}
	return formatted, hits, strLits, finalImports, buildExpr, nil
}

func isImportPathChar(b byte) bool {
	return 'a' <= b && b <= 'z' || 'A' <= b && b <= 'Z' || '0' <= b && b <= '9' || b == '.' || b == '_' || b == '-' || b == '/'
}

func isTrailingImportPathChar(s string, pos int) bool {
	if pos >= len(s) {
		return false
	}
	// A period followed by whitespace, punctuation or end-of-string is sentence
	// punctuation (e.g. "// Package locales holds translations for ...ui/chat."),
	// not part of an import path.
	if s[pos] == '.' {
		return pos+1 < len(s) && isImportPathChar(s[pos+1]) && s[pos+1] != '.'
	}
	return isImportPathChar(s[pos])
}

func containsWholeImportPath(s, target string) bool {
	if target == "" {
		return false
	}
	for start := 0; start < len(s); {
		idx := strings.Index(s[start:], target)
		if idx < 0 {
			return false
		}
		pos := start + idx
		end := pos + len(target)
		leftOK := pos == 0 || !isImportPathChar(s[pos-1])
		rightOK := !isTrailingImportPathChar(s, end)
		if leftOK && rightOK {
			return true
		}
		start = pos + 1
	}
	return false
}

func rewriteCommentWholePaths(text string, oldPaths []string, rewrites map[string]string) (string, []string) {
	var b strings.Builder
	var matched []string
	seen := map[string]bool{}
	i := 0
	for i < len(text) {
		replaced := false
		for _, oldP := range oldPaths {
			if strings.HasPrefix(text[i:], oldP) {
				end := i + len(oldP)
				leftOK := i == 0 || !isImportPathChar(text[i-1])
				rightOK := !isTrailingImportPathChar(text, end)
				if leftOK && rightOK {
					b.WriteString(rewrites[oldP])
					if !seen[oldP] {
						seen[oldP] = true
						matched = append(matched, oldP)
					}
					i = end
					replaced = true
					break
				}
			}
		}
		if !replaced {
			b.WriteByte(text[i])
			i++
		}
	}
	if len(matched) == 0 {
		return text, nil
	}
	return b.String(), matched
}

// isStdlibImport reports whether imp is a Go standard library package path
// (first path element has no dot).
func isStdlibImport(imp string) bool {
	first, _, _ := strings.Cut(imp, "/")
	return !strings.Contains(first, ".")
}

// evaluateCopyLicenses classifies every license document carried by c and
// enforces the app's licenses policy.
func evaluateCopyLicenses(c *plannedComponent, policy licensePolicy) error {
	classifier, err := licenseClassifier()
	if err != nil {
		return err
	}
	types := map[string]bool{}
	var unknown []string
	var docCount int
	for _, f := range c.files {
		primary, include := isLicenseDocument(path.Base(f.relPath))
		if !include {
			continue
		}
		docCount++
		known := false
		for _, match := range classifier.Match(f.data).Matches {
			if match.MatchType == "License" {
				types[match.Name] = true
				known = true
			}
		}
		if primary && !known {
			unknown = append(unknown, f.relPath)
		}
	}
	c.licenseIDs = slices.Sorted(maps.Keys(types))

	allowed := slices.Contains(policy.Allow, c.srcMod) || slices.Contains(policy.Allow, c.importPath)
	if !allowed {
		if docCount == 0 || len(types) == 0 || len(unknown) > 0 {
			reason := "has no recognized license text"
			if len(unknown) > 0 {
				reason = "has unrecognized license text in " + strings.Join(unknown, ", ")
			}
			if policy.Unknown != "warn" {
				c.licenseRefusals = append(c.licenseRefusals, fmt.Sprintf("%s %s; add %q to licenses.allow in nsgo.yaml or set licenses.unknown: warn", c.srcMod, reason, c.srcMod))
			} else {
				c.licenseWarnings = append(c.licenseWarnings, fmt.Sprintf("%s %s (licenses.unknown: warn)", c.srcMod, reason))
			}
		}
		var copyleft []string
		for _, id := range c.licenseIDs {
			if copyleftLicense(id) {
				copyleft = append(copyleft, id)
			}
		}
		if len(copyleft) > 0 {
			reason := "has copyleft license text (" + strings.Join(copyleft, ", ") + ")"
			if policy.Copyleft == "error" {
				c.licenseRefusals = append(c.licenseRefusals, fmt.Sprintf("%s %s; add %q to licenses.allow in nsgo.yaml (licenses.copyleft: error)", c.srcMod, reason, c.srcMod))
			} else {
				c.licenseWarnings = append(c.licenseWarnings, fmt.Sprintf("copyleft %s (%s)", strings.Join(copyleft, ", "), cmp.Or(policy.Copyleft, "warn")))
			}
		}
	}
	return nil
}

func hasComponentLocales(c *plannedComponent) bool {
	for _, f := range c.files {
		if f.isLocale && strings.HasSuffix(strings.ToLower(f.relPath), ".yaml") {
			return true
		}
	}
	return false
}

// findAppCatalogSupplies scans the app's catalogs directory for any YAML file
// supplying origLocales.
func findAppCatalogSupplies(modDir, i18nDir, origLocales, copyLocales string) []catalogSupplyHint {
	paths, _ := filepath.Glob(filepath.Join(i18nDir, "*.yaml"))
	slices.Sort(paths)
	var hints []catalogSupplyHint
	for _, p := range paths {
		var ds diags
		f := readCatalogFile(p, strings.TrimSuffix(filepath.Base(p), ".yaml"), &ds)
		if f == nil {
			continue
		}
		for _, s := range f.supplies {
			if s.importPath == origLocales {
				hints = append(hints, catalogSupplyHint{
					file:   relTo(modDir, p),
					oldKey: origLocales,
					newKey: copyLocales,
				})
				break
			}
		}
	}
	return hints
}

// planRequirements works out the go.mod and go.sum updates needed by p.components.
func (p *copyPlan) planRequirements(ctx context.Context) error {
	needVersion := map[string]string{}
	addNeed := func(modPath, ver string) {
		if modPath == p.appModPath {
			return
		}
		p.directMods[modPath] = true
		if cur, ok := needVersion[modPath]; !ok || (ver != "" && (cur == "" || semver.Compare(ver, cur) > 0)) {
			needVersion[modPath] = ver
		}
	}

	for _, c := range p.components {
		// If the user explicitly typed a version raising an already-required source
		// module, raise that requirement in go.mod too.
		if c.raisesMod {
			addNeed(c.srcMod, c.srcVersion)
		}
		for _, imp := range c.extImports {
			if imp == c.srcMod || strings.HasPrefix(imp, c.srcMod+"/") {
				if c.olderCopy {
					// Keep the app's existing higher requirement of srcMod.
					p.directMods[c.srcMod] = true
				} else {
					addNeed(c.srcMod, c.srcVersion)
				}
				continue
			}
			// Match against the source module's go.mod requirements.
			if depMod := providingModule(imp, c.dm.goMod); depMod != "" {
				depVer := requiredVersion(c.dm.goMod, depMod)
				addNeed(depMod, depVer)
			} else if depMod := providingModule(imp, p.before); depMod != "" {
				p.directMods[depMod] = true
			} else {
				// External import not listed in source go.mod: resolve the package via go get.
				addNeed(imp, "upgrade")
			}
		}
	}

	// Determine which modules actually need `go get` (missing or below needed version).
	var getArgs []string
	var needDirectMark bool
	existingReqs := requirements(p.before)
	for _, modPath := range slices.Sorted(maps.Keys(needVersion)) {
		wantVer := needVersion[modPath]
		cur, has := existingReqs[modPath]
		if has && hasLocalDirReplace(p.before, modPath) {
			if cur.indirect {
				needDirectMark = true
			}
			continue
		}
		if has && wantVer != "upgrade" && semver.Compare(cur.version, wantVer) >= 0 {
			if cur.indirect {
				needDirectMark = true
			}
			continue
		}
		if wantVer == "" {
			wantVer = "upgrade"
		}
		getArgs = append(getArgs, modPath+"@"+wantVer)
	}
	for modPath := range p.directMods {
		if cur, has := existingReqs[modPath]; has && cur.indirect {
			needDirectMark = true
		}
	}

	if len(getArgs) == 0 && !needDirectMark {
		return nil
	}

	tmp, err := os.MkdirTemp("", "nsgo-copy-mod-")
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	defer os.RemoveAll(tmp)

	planned := filepath.Join(tmp, "go.mod")
	if err := os.WriteFile(planned, p.oldMod, 0o644); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if p.hadSum {
		if err := os.WriteFile(filepath.Join(tmp, "go.sum"), p.oldSum, 0o644); err != nil {
			return fmt.Errorf("add: %w", err)
		}
	}

	if len(getArgs) > 0 {
		args := append([]string{"get", "-modfile=" + planned}, getArgs...)
		if _, err := p.m.goOutput(ctx, args...); err != nil {
			var goErr *addGoError
			if !errors.As(err, &goErr) {
				return fmt.Errorf("add: %w", err)
			}
			needed := ""
			for _, match := range tooNewGo.FindAllStringSubmatch(goErr.messages, -1) {
				if strings.Contains(match[1], "@") && (needed == "" || semver.Compare("v"+match[2], "v"+needed) > 0) {
					needed = match[2]
				}
			}
			if needed != "" {
				return fmt.Errorf("add: %s\nto move this module to Go %s on purpose, run go get go@%s first", goErr.messages, needed, needed)
			}
			return fmt.Errorf("add: go get failed\n%s", goErr.messages)
		}
	}

	newMod, err := os.ReadFile(planned)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	after, err := modfile.Parse(filepath.Join(p.m.dir, "go.mod"), newMod, nil)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}

	// Mark every module directly imported by a copied component as direct.
	changedDirect := false
	for _, r := range after.Require {
		if p.directMods[r.Mod.Path] && r.Indirect {
			r.Indirect = false
			changedDirect = true
		}
	}
	if changedDirect {
		after.SetRequire(after.Require)
		after.Cleanup()
		formatted, err := after.Format()
		if err != nil {
			return fmt.Errorf("add: %w", err)
		}
		newMod = formatted
		after, err = modfile.Parse(filepath.Join(p.m.dir, "go.mod"), newMod, nil)
		if err != nil {
			return fmt.Errorf("add: %w", err)
		}
	}

	newSum, err := os.ReadFile(filepath.Join(tmp, "go.sum"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("add: %w", err)
	}
	p.newMod = newMod
	p.newSum = newSum
	p.hasSum = err == nil
	p.after = after

	oldNSGo, newNSGo := requiredVersion(p.before, nsgoModule), requiredVersion(p.after, nsgoModule)
	p.movedNSGo = newNSGo != "" && newNSGo != oldNSGo
	return nil
}

// generateComponentCatalog loads c's locales/*.yaml under c's final import path
// and generates locales/catalog_gen.go in memory using nsgo i18n's generator.
func (p *copyPlan) generateComponentCatalog(ctx context.Context, c *plannedComponent) ([]byte, error) {
	tmp, err := os.MkdirTemp("", "nsgo-copy-i18n-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)

	locDir := filepath.Join(tmp, "locales")
	if err := os.MkdirAll(locDir, 0o755); err != nil {
		return nil, err
	}
	for _, f := range c.files {
		if !f.isLocale || f.generated {
			continue
		}
		if err := os.WriteFile(filepath.Join(locDir, path.Base(f.relPath)), f.data, 0o644); err != nil {
			return nil, err
		}
	}

	source := detectComponentCatalogSource(c)
	cs := newCatalogSet(ctx)
	cat := cs.add(locDir, c.importPath+"/locales", source, false)
	cat.pkg = "locales"

	// Resolve any supplies using the planned go.mod through -overlay and -mod=readonly.
	var hasSupplies bool
	for _, f := range cat.files {
		if len(f.supplies) > 0 {
			hasSupplies = true
			break
		}
	}
	if hasSupplies {
		overlayPath, cleanup, err := p.writePlanOverlay(false)
		if err != nil {
			return nil, err
		}
		defer cleanup()
		p.resolveCopyCatalogSupplies(ctx, cs, overlayPath)
	}
	if err := cs.ds.err(); err != nil {
		return nil, err
	}
	return cat.generate()
}

func detectComponentCatalogSource(c *plannedComponent) string {
	// 1. Check the zip's existing locales/catalog_gen.go if present.
	if raw, err := c.dm.readFile(c.srcDir+"/locales/"+genFile, maxComponentFileBytes); err == nil {
		if src, _, ok := generatedCatalogBytes(raw); ok && src != "" {
			return src
		}
	}
	// 2. Check //go:generate go tool nsgo i18n -source <tag> in locales/*.go.
	for _, f := range c.files {
		if !f.isLocale || !strings.HasSuffix(strings.ToLower(f.relPath), ".go") {
			continue
		}
		for line := range strings.SplitSeq(string(f.data), "\n") {
			if rest, ok := cutGoDirective(strings.TrimSpace(line), "//go:generate"); ok {
				fields := strings.Fields(rest)
				if len(fields) == 5 {
					if tag, ok := strings.CutPrefix(fields[4], "-source="); ok && isValidBCP47Tag(tag) {
						return tag
					}
				} else if len(fields) == 6 && fields[4] == "-source" && isValidBCP47Tag(fields[5]) {
					return fields[5]
				}
			}
		}
	}
	return "en"
}

func generatedCatalogBytes(raw []byte) (source string, app, ok bool) {
	data := strings.ReplaceAll(string(raw), "\r\n", "\n")
	if !strings.HasPrefix(data, genHeader) {
		return "", false, false
	}
	_, rest, found := strings.Cut(data, "\nconst Source = ")
	if !found {
		return "", false, false
	}
	line, _, _ := strings.Cut(rest, "\n")
	source, err := strconv.Unquote(strings.TrimSpace(line))
	if err != nil {
		return "", false, false
	}
	return source, strings.Contains(data, "\n\ti18n.SetApp(Catalog)\n"), true
}

func (p *copyPlan) resolveCopyCatalogSupplies(ctx context.Context, cs *catalogSet, overlayPath string) {
	for i := 0; i < len(cs.order); i++ {
		c := cs.order[i]
		for _, f := range c.files {
			for _, s := range f.supplies {
				if s.importPath == c.importPath {
					cs.ds.errorf(f.path, s.line, "supplies its own catalog; write the messages at the top level")
					continue
				}
				s.target = cs.byPath[s.importPath]
				if s.target == nil {
					cmd := exec.CommandContext(ctx, p.m.goCmd, "list", "-e", "-overlay="+overlayPath, "-mod=readonly", "-f", "{{.Dir}}", s.importPath)
					cmd.Dir = p.m.dir
					cmd.Env = p.m.listEnv
					out, err := cmd.Output()
					dir := strings.TrimSpace(string(out))
					if err != nil || dir == "" {
						cs.ds.errorf(f.path, s.line, "%s: no such package", s.importPath)
						continue
					}
					source, app, ok := generatedCatalog(dir)
					if !ok {
						cs.ds.errorf(f.path, s.line, "%s has no catalogs nsgo wrote", s.importPath)
						continue
					}
					s.target = cs.add(dir, s.importPath, source, app)
				}
				cs.checkSupply(f, s)
			}
		}
	}
}

// writePlanOverlay writes the planned go.mod, go.sum and (when includeFiles is
// true) all copied component files into a temporary directory and returns the
// path to the Go -overlay JSON file and a cleanup function.
func (p *copyPlan) writePlanOverlay(includeFiles bool) (overlayPath string, cleanup func(), err error) {
	tmp, err := os.MkdirTemp("", "nsgo-copy-overlay-")
	if err != nil {
		return "", nil, err
	}
	cleanup = func() { os.RemoveAll(tmp) }

	replace := map[string]string{}
	addMapping := func(origRel, stagedAbs string) {
		origAbs := filepath.Join(p.m.dir, filepath.FromSlash(origRel))
		replace[origAbs] = stagedAbs
		if realMod, err := filepath.EvalSymlinks(p.m.dir); err == nil && realMod != p.m.dir {
			replace[filepath.Join(realMod, filepath.FromSlash(origRel))] = stagedAbs
		}
	}

	modStage := filepath.Join(tmp, "go.mod")
	if err := os.WriteFile(modStage, p.newMod, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	addMapping("go.mod", modStage)

	if p.hasSum {
		sumStage := filepath.Join(tmp, "go.sum")
		if err := os.WriteFile(sumStage, p.newSum, 0o644); err != nil {
			cleanup()
			return "", nil, err
		}
		addMapping("go.sum", sumStage)
	}

	if includeFiles {
		filesRoot := filepath.Join(tmp, "files")
		for _, c := range p.components {
			for _, f := range c.files {
				relFromMod := c.destRel + "/" + f.relPath
				staged := filepath.Join(filesRoot, filepath.FromSlash(relFromMod))
				if err := os.MkdirAll(filepath.Dir(staged), 0o755); err != nil {
					cleanup()
					return "", nil, err
				}
				if err := os.WriteFile(staged, f.data, 0o644); err != nil {
					cleanup()
					return "", nil, err
				}
				addMapping(relFromMod, staged)
			}
		}
	}

	raw, err := json.Marshal(struct {
		Replace map[string]string
	}{Replace: replace})
	if err != nil {
		cleanup()
		return "", nil, err
	}
	overlayPath = filepath.Join(tmp, "overlay.json")
	if err := os.WriteFile(overlayPath, raw, 0o644); err != nil {
		cleanup()
		return "", nil, err
	}
	return overlayPath, cleanup, nil
}

// vetCopies runs go vet on the copied packages in a temporary shadow module
// directory (holding the planned go.mod, go.sum and copied files, plus a shadow
// go.work when inside a workspace) with -mod=readonly, so planning never writes
// into the user's module, works in workspaces and vendored modules, and lets
// go's vet tool chdir into each copied package directory.
func (p *copyPlan) vetCopies(ctx context.Context) error {
	vetParent, err := os.MkdirTemp("", "nsgo-copy-vet-")
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	defer os.RemoveAll(vetParent)

	if realParent, err := filepath.EvalSymlinks(vetParent); err == nil {
		vetParent = realParent
	}
	vetModDir := filepath.Join(vetParent, "mod")
	if err := os.MkdirAll(vetModDir, 0o755); err != nil {
		return fmt.Errorf("add: %w", err)
	}

	// Write planned go.mod with any relative local replace paths resolved against p.m.dir.
	vetModBytes, err := absolutizeModFileReplaces(p.newMod, p.m.dir)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if err := os.WriteFile(filepath.Join(vetModDir, "go.mod"), vetModBytes, 0o644); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if p.hasSum {
		if err := os.WriteFile(filepath.Join(vetModDir, "go.sum"), p.newSum, 0o644); err != nil {
			return fmt.Errorf("add: %w", err)
		}
	}

	// Materialize all planned copied files into vetModDir.
	for _, c := range p.components {
		for _, f := range c.files {
			dst := filepath.Join(vetModDir, filepath.FromSlash(c.destRel+"/"+f.relPath))
			if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
				return fmt.Errorf("add: %w", err)
			}
			if err := os.WriteFile(dst, f.data, 0o644); err != nil {
				return fmt.Errorf("add: %w", err)
			}
		}
	}

	// Also materialize any existing copies from the module so a new copy that
	// imports an already-copied component compiles during vetting.
	if existing, err := findModuleCopies(p.m.dir, p.appModPath); err == nil {
		for _, ec := range existing {
			for _, relFile := range slices.Concat(ec.record.Files, ec.record.Generated) {
				data, err := readCopyFile(ec.dir, relFile)
				if err != nil {
					continue
				}
				dst := filepath.Join(vetModDir, filepath.FromSlash(ec.relDir+"/"+relFile))
				if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
					return fmt.Errorf("add: %w", err)
				}
				if err := os.WriteFile(dst, data, 0o644); err != nil {
					return fmt.Errorf("add: %w", err)
				}
			}
		}
	}

	baseEnv := slices.Clone(p.m.listEnv)
	if p.m.workspace != "" {
		vetWork, err := writeShadowGoWork(vetParent, p.m.workspace, p.m.dir, vetModDir)
		if err != nil {
			return fmt.Errorf("add: %w", err)
		}
		baseEnv = append(baseEnv, "GOWORK="+vetWork)
	}

	patterns := make([]string, 0, len(p.components)*2)
	for _, c := range p.components {
		patterns = append(patterns, "./"+c.destRel)
		if hasComponentLocales(c) {
			patterns = append(patterns, "./"+c.destRel+"/locales")
		}
	}

	targets := p.app.targets
	hostOnly := len(targets) == 0
	if hostOnly {
		targets = []string{hostVetTargetLabel()}
	}

	for _, target := range targets {
		var env []string
		var extraTags []string
		if !hostOnly {
			if _, _, apple := appleOSOf(target); apple && runtime.GOOS != "darwin" {
				p.vet.notVetted = append(p.vet.notVetted, struct{ target, reason string }{target, "needs macOS with Xcode"})
				continue
			}
			tEnv, err := copyVetEnv(ctx, target)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				p.vet.notVetted = append(p.vet.notVetted, struct{ target, reason string }{target, err.Error()})
				continue
			}
			env = tEnv
			extraTags = targetTags(target)
		}

		args := []string{"vet", "-mod=readonly"}
		allTags := slices.Concat(p.app.tags, extraTags)
		if len(allTags) > 0 {
			args = append(args, "-tags", strings.Join(allTags, ","))
		}
		args = append(args, patterns...)

		cmd := exec.CommandContext(ctx, p.m.goCmd, args...)
		cmd.Dir = vetModDir
		cmd.Env = append(slices.Clone(baseEnv), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			msg := strings.TrimSpace(string(out))
			if msg == "" {
				msg = err.Error()
			}
			p.vet.failures = append(p.vet.failures, struct{ target, output string }{target, msg})
			continue
		}
		p.vet.vetted = append(p.vet.vetted, target)
	}
	return nil
}

// absolutizeModFileReplaces rewrites relative filesystem paths in go.mod
// replace directives so go.mod resolves the same local directories when placed
// in a temporary shadow directory.
func absolutizeModFileReplaces(raw []byte, modDir string) ([]byte, error) {
	mf, err := modfile.Parse("go.mod", raw, nil)
	if err != nil {
		return nil, err
	}
	changed := false
	for _, r := range mf.Replace {
		if r.New.Version == "" && !filepath.IsAbs(r.New.Path) {
			abs := filepath.Join(modDir, filepath.FromSlash(r.New.Path))
			if err := mf.AddReplace(r.Old.Path, r.Old.Version, filepath.ToSlash(abs), ""); err != nil {
				return nil, err
			}
			changed = true
		}
	}
	if !changed {
		return raw, nil
	}
	return mf.Format()
}

// writeShadowGoWork writes a temporary go.work replacing modDir's use entry
// with vetModDir and resolving all other relative paths against the original
// go.work's directory.
func writeShadowGoWork(vetParent, origWork, modDir, vetModDir string) (string, error) {
	raw, err := os.ReadFile(origWork)
	if err != nil {
		return "", err
	}
	wf, err := modfile.ParseWork(origWork, raw, nil)
	if err != nil {
		return "", err
	}
	workDir := filepath.Dir(origWork)
	realMod, _ := filepath.EvalSymlinks(modDir)
	if realMod == "" {
		realMod = filepath.Clean(modDir)
	}

	var newUses []string
	replacedSelf := false
	for _, u := range wf.Use {
		abs := u.Path
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(workDir, filepath.FromSlash(abs))
		}
		realU, _ := filepath.EvalSymlinks(abs)
		if realU == "" {
			realU = filepath.Clean(abs)
		}
		if realU == realMod {
			newUses = append(newUses, "./mod")
			replacedSelf = true
		} else {
			newUses = append(newUses, filepath.ToSlash(realU))
		}
	}
	if !replacedSelf {
		newUses = append(newUses, "./mod")
	}

	shadow, err := modfile.ParseWork("go.work", nil, nil)
	if err != nil {
		return "", err
	}
	if wf.Go != nil {
		_ = shadow.AddGoStmt(wf.Go.Version)
	}
	for _, u := range newUses {
		if err := shadow.AddUse(u, ""); err != nil {
			return "", err
		}
	}
	for _, r := range wf.Replace {
		newPath := r.New.Path
		if r.New.Version == "" && !filepath.IsAbs(newPath) {
			newPath = filepath.ToSlash(filepath.Join(workDir, filepath.FromSlash(newPath)))
		}
		if err := shadow.AddReplace(r.Old.Path, r.Old.Version, newPath, r.New.Version); err != nil {
			return "", err
		}
	}
	data := modfile.Format(shadow.Syntax)
	dst := filepath.Join(vetParent, "go.work")
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		return "", err
	}
	return dst, nil
}

func hostVetTargetLabel() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos_" + goArchName(runtime.GOARCH)
	case "windows", "linux":
		return runtime.GOOS + "_" + goArchName(runtime.GOARCH)
	default:
		return runtime.GOOS + "_" + runtime.GOARCH
	}
}

// write prints the copy plan to w.
func (p *copyPlan) write(w io.Writer) error {
	var b bytes.Buffer
	for _, lr := range p.registries {
		fmt.Fprintf(&b, "%s\n", lr.headerLine())
	}
	if len(p.registries) > 0 && (len(p.components) > 0 || len(p.already) > 0) {
		b.WriteString("\n")
	}
	for _, ec := range p.already {
		fmt.Fprintf(&b, "%s (%s %s) is already copied at %s\n", ec.record.Component, ec.record.Module, ec.record.ModuleVersion, ec.relDir)
	}
	if len(p.components) == 0 {
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

	for _, c := range p.components {
		// First line: name, origin, and whether this raises the app's module requirement.
		header := c.displayName
		switch {
		case strings.HasPrefix(c.fromName, "@"):
			regName, _, _ := strings.Cut(c.fromName, "/")
			header += fmt.Sprintf("  from registry %s: a component, copied into this module", regName)
		case c.fromName != "":
			header += "  from NSGo's registry: a component, copied into this module"
		default:
			header += "  a component, copied into this module"
		}
		if c.raisesMod {
			header += fmt.Sprintf("; raises %s from %s to %s", c.srcMod, c.appReqVer, c.srcVersion)
		}
		b.WriteString(header + "\n")

		fromLine := fmt.Sprintf("%s %s %s", c.srcMod, c.srcVersion, c.srcDir)
		switch {
		case c.appReqVer != "" && c.appReqVer == c.srcVersion:
			fromLine += ", the version this module requires"
		case c.olderCopy:
			fromLine += fmt.Sprintf(", older than %s, which this module requires", c.appReqVer)
		}
		if len(c.licenseIDs) > 0 {
			fromLine += "; " + strings.Join(c.licenseIDs, ", ")
		}
		row("from", fromLine)
		row("into", fmt.Sprintf("%s: import %q", c.destRel, c.importPath))

		rows("files", c.fileSummaryLines())

		if len(c.rewrites) > 0 {
			var rwLines []string
			for _, st := range c.rewrites {
				var parts []string
				if st.files > 0 {
					parts = append(parts, countNoun(st.files, "file"))
				}
				if st.comments > 0 {
					parts = append(parts, countNoun(st.comments, "comment"))
				}
				rwLines = append(rwLines, fmt.Sprintf("%s => %s, in %s", st.oldPath, st.newPath, strings.Join(parts, " and ")))
			}
			rows("imports", rwLines)
		}

		if len(c.stringLiterals) > 0 {
			var strLines []string
			for _, sl := range c.stringLiterals {
				strLines = append(strLines, fmt.Sprintf("%s:%d: string literal contains %q (left as written)", sl.file, sl.line, sl.oldPath))
			}
			rows("strings", strLines)
		}

		if len(c.licenseWarnings) > 0 {
			rows("licenses", c.licenseWarnings)
		}
		if len(c.licenseRefusals) > 0 {
			rows("licenses", c.licenseRefusals)
		}

		if len(c.flags) > 0 {
			flagLines := make([]string, len(c.flags))
			for i, fl := range c.flags {
				flagLines[i] = formatComponentFinding(c.destRel, fl)
			}
			rows("flags", flagLines)
		}

		b.WriteString("\n")
	}

	// Module-level summary: go.mod, go.sum, tools, vet results, next steps.
	fmt.Fprintf(&b, "%s\n", p.appModPath)
	goModLines := p.goModChanges()
	if len(goModLines) == 0 {
		row("go.mod", "unchanged")
	} else {
		rows("go.mod", goModLines)
	}
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

	if len(p.vet.vetted) > 0 {
		row("vetted", strings.Join(p.vet.vetted, " "))
	}
	if len(p.vet.notVetted) > 0 {
		lines := make([]string, len(p.vet.notVetted))
		for i, nv := range p.vet.notVetted {
			lines[i] = nv.target + ": " + nv.reason
		}
		rows("not vetted", lines)
	}
	if len(p.vet.failures) > 0 {
		lines := make([]string, len(p.vet.failures))
		for i, vf := range p.vet.failures {
			lines[i] = vf.target + ": " + strings.ReplaceAll(vf.output, "\n", "\n             ")
		}
		rows("vet failed", lines)
	}

	var nextLines []string
	for _, c := range p.components {
		if c.explicit {
			nextLines = append(nextLines, fmt.Sprintf("import %q", c.importPath))
		}
	}
	if p.m.vendored() {
		nextLines = append(nextLines, "go mod vendor")
	}
	for _, c := range p.components {
		for _, h := range c.supplyHints {
			nextLines = append(nextLines, fmt.Sprintf("%s supplies %s; supply %s once the copy is imported", h.file, h.oldKey, h.newKey))
		}
		if p.movedNSGo && hasComponentLocales(c) {
			nextLines = append(nextLines, fmt.Sprintf("%s dev, build or i18n then rewrites %s/locales/%s", nsgoCommand(), c.destRel, genFile))
		}
	}
	rows("next", nextLines)

	// Author notes (labelled as the author's, printed last).
	seenNotes := map[string]bool{}
	for _, c := range p.components {
		if len(c.notes) == 0 || seenNotes[c.srcMod] {
			continue
		}
		seenNotes[c.srcMod] = true
		fmt.Fprintf(&b, "\nFrom %s, the author's notes:\n", c.srcMod)
		for _, l := range c.notes {
			if l == "" {
				b.WriteString("  |\n")
			} else {
				fmt.Fprintf(&b, "  | %s\n", l)
			}
		}
	}

	// -diff: print every copied file in full.
	if p.showDiff {
		for _, c := range p.components {
			for _, f := range c.files {
				fmt.Fprintf(&b, "\n=== %s/%s ===\n", c.destRel, f.relPath)
				if isComponentTextFile(f.relPath) {
					b.Write(f.data)
					if len(f.data) > 0 && f.data[len(f.data)-1] != '\n' {
						b.WriteByte('\n')
					}
				} else {
					fmt.Fprintf(&b, "(binary file, %d bytes)\n", len(f.data))
				}
			}
			fmt.Fprintf(&b, "\n=== %s/%s ===\n", c.destRel, copyRecordName)
			b.Write(c.recordBytes)
		}
	}

	_, err := w.Write(b.Bytes())
	return err
}

func (c *plannedComponent) fileSummaryLines() []string {
	var (
		goSources   int
		assets      []string
		locales     []string
		tests       int
		testTags    []string
		testdataCnt int
		generated   []string
		licenses    []string
	)
	for _, f := range c.files {
		switch {
		case f.generated:
			generated = append(generated, f.relPath)
		case f.isLicense:
			licenses = append(licenses, f.relPath)
		case f.isTest:
			if strings.HasSuffix(strings.ToLower(f.relPath), "_test.go") {
				tests++
				if f.buildTag != "" {
					testTags = append(testTags, fmt.Sprintf("%s: %s", f.relPath, f.buildTag))
				}
			} else {
				testdataCnt++
			}
		case f.isLocale:
			locales = append(locales, f.relPath)
		case strings.HasSuffix(strings.ToLower(f.relPath), ".go"):
			goSources++
		default:
			assets = append(assets, f.relPath)
		}
	}

	var lines []string
	var firstParts []string
	if goSources > 0 {
		firstParts = append(firstParts, countNoun(goSources, "source file"))
	}
	if len(locales) > 0 {
		firstParts = append(firstParts, strings.Join(locales, ", "))
	}
	if len(assets) > 0 {
		firstParts = append(firstParts, strings.Join(assets, ", "))
	}
	if len(firstParts) > 0 {
		lines = append(lines, "+ "+strings.Join(firstParts, "; "))
	}
	if tests > 0 || testdataCnt > 0 {
		var testParts []string
		if tests > 0 {
			s := countNoun(tests, "test")
			if len(testTags) > 0 {
				s += " (" + strings.Join(testTags, "; ") + ")"
			}
			testParts = append(testParts, s)
		}
		if testdataCnt > 0 {
			testParts = append(testParts, countNoun(testdataCnt, "testdata file"))
		}
		lines = append(lines, "+ "+strings.Join(testParts, "; "))
	}
	for _, g := range generated {
		lines = append(lines, fmt.Sprintf("+ %s, written by this nsgo", g))
	}
	if len(licenses) > 0 {
		lines = append(lines, fmt.Sprintf("+ %s, under their own heading in %s", strings.Join(licenses, ", "), noticesName))
	}
	lines = append(lines, "+ "+copyRecordName)
	return lines
}

func countNoun(n int, singular string) string {
	if n == 1 {
		return "1 " + singular
	}
	return fmt.Sprintf("%d %ss", n, singular)
}

func (p *copyPlan) goModChanges() []string {
	ap := &addPlan{before: p.before, after: p.after}
	return ap.goModChanges()
}

func (p *copyPlan) goSumChange() string {
	ap := &addPlan{oldSum: p.oldSum, newSum: p.newSum}
	return ap.goSumChange()
}

func (p *copyPlan) toolChanges() []string {
	ap := &addPlan{before: p.before, after: p.after}
	return ap.toolChanges()
}

func (p *copyPlan) private() string {
	ap := &addPlan{m: p.m, oldSum: p.oldSum, newSum: p.newSum}
	return ap.private()
}

func (p *copyPlan) checksums() string {
	ap := &addPlan{m: p.m, oldSum: p.oldSum, newSum: p.newSum}
	return ap.checksums()
}

// apply writes all planned copied components and go.mod/go.sum atomically under
// the module's check lock:
//  1. removes any stale .nsgo-copy-* staging directories left by an interrupted run;
//  2. stages each component into a hidden .nsgo-copy-* directory beside its destination;
//  3. writes go.mod and go.sum (checking neither changed since planning);
//  4. renames each staging directory into place using addRename;
//  5. on any error or interrupt, removes staging/created copy directories and restores go.mod/go.sum.
func (p *copyPlan) apply(ctx context.Context) (retErr error) {
	unlock, err := lockCheck(ctx, p.m.dir)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	defer unlock()

	// Check go.mod and go.sum have not changed since planning.
	type modFileState struct {
		name               string
		old, new           []byte
		existed, nowExists bool
	}
	modFiles := []modFileState{
		{"go.mod", p.oldMod, p.newMod, true, true},
		{"go.sum", p.oldSum, p.newSum, p.hadSum, p.hasSum},
	}
	for _, f := range modFiles {
		data, err := os.ReadFile(filepath.Join(p.m.dir, f.name))
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return fmt.Errorf("add: %w", err)
		}
		if (err == nil) != f.existed || !bytes.Equal(data, f.old) {
			return fmt.Errorf("add: %s changed since the plan was made, so nothing was written: run %s add again", f.name, nsgoCommand())
		}
	}

	// Re-check destination availability and clean up stale .nsgo-copy-* dirs.
	var createdParents []string
	var staged []struct {
		stageDir string
		destAbs  string
		moved    bool
	}
	var writtenMod []modFileState

	defer func() {
		if retErr == nil {
			return
		}
		for i := len(staged) - 1; i >= 0; i-- {
			if staged[i].moved {
				retErr = errors.Join(retErr, os.RemoveAll(staged[i].destAbs))
			} else {
				retErr = errors.Join(retErr, os.RemoveAll(staged[i].stageDir))
			}
		}
		for i := len(createdParents) - 1; i >= 0; i-- {
			_ = os.Remove(createdParents[i]) // remove only if empty
		}
		for i := len(writtenMod) - 1; i >= 0; i-- {
			f := writtenMod[i]
			path := filepath.Join(p.m.dir, f.name)
			if f.existed {
				retErr = errors.Join(retErr, replaceFile(path, f.old))
			} else {
				retErr = errors.Join(retErr, os.Remove(path))
			}
		}
	}()

	cleanedParents := map[string]bool{}
	for _, c := range p.components {
		if err := validateCopyDestination(p.m.dir, c.destAbs, c.destRel, c.compName, c.explicit); err != nil {
			return fmt.Errorf("add: %w", err)
		}
		parent := filepath.Dir(c.destAbs)
		newParents, err := mkdirAllTracking(p.m.dir, parent)
		if err != nil {
			return fmt.Errorf("add: %w", err)
		}
		createdParents = append(createdParents, newParents...)

		// Remove any leftover staging directory from an earlier interrupted run
		// once per parent directory, before creating this run's staging dirs.
		if !cleanedParents[parent] {
			cleanStaleCopyStaging(parent)
			cleanedParents[parent] = true
		}

		stageDir, err := os.MkdirTemp(parent, copyStagingPrefix)
		if err != nil {
			return fmt.Errorf("add: %w", err)
		}
		staged = append(staged, struct {
			stageDir string
			destAbs  string
			moved    bool
		}{stageDir: stageDir, destAbs: c.destAbs})

		if err := writeComponentToStage(stageDir, c); err != nil {
			return fmt.Errorf("add: staging %s: %w", c.destRel, err)
		}
	}

	// Write go.mod and go.sum if changed.
	for _, f := range modFiles {
		if f.existed == f.nowExists && bytes.Equal(f.old, f.new) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("add: interrupted, so nothing was written: %w", err)
		}
		if err := replaceFile(filepath.Join(p.m.dir, f.name), f.new); err != nil {
			return fmt.Errorf("add: writing %s failed, so nothing was written: %w", f.name, err)
		}
		writtenMod = append(writtenMod, f)
	}

	// Move each staged component into its final destination.
	for i := range staged {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("add: interrupted, so the module is as it was: %w", err)
		}
		if err := addRename(staged[i].stageDir, staged[i].destAbs); err != nil {
			return fmt.Errorf("add: placing %s failed, so the module is as it was: %w", relTo(p.m.dir, staged[i].destAbs), err)
		}
		staged[i].moved = true
	}

	return nil
}

func writeComponentToStage(stageDir string, c *plannedComponent) error {
	root, err := os.OpenRoot(stageDir)
	if err != nil {
		return err
	}
	defer root.Close()

	writeOne := func(relPath string, data []byte) error {
		local := filepath.FromSlash(relPath)
		if dir := filepath.Dir(local); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		f, err := root.OpenFile(local, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil {
			return err
		}
		_, werr := f.Write(data)
		return errors.Join(werr, f.Close())
	}

	for _, cf := range c.files {
		if err := writeOne(cf.relPath, cf.data); err != nil {
			return err
		}
	}
	return writeOne(copyRecordName, c.recordBytes)
}

func mkdirAllTracking(stopAt, target string) ([]string, error) {
	var missing []string
	for cur := filepath.Clean(target); cur != filepath.Clean(stopAt) && cur != filepath.Dir(cur); cur = filepath.Dir(cur) {
		if _, err := os.Lstat(cur); err == nil {
			break
		}
		missing = append(missing, cur)
	}
	slices.Reverse(missing)
	var created []string
	for _, dir := range missing {
		if err := os.Mkdir(dir, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
			return created, err
		}
		created = append(created, dir)
	}
	return created, nil
}

func cleanStaleCopyStaging(parent string) {
	entries, err := os.ReadDir(parent)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), copyStagingPrefix) {
			_ = os.RemoveAll(filepath.Join(parent, e.Name()))
		}
	}
}
