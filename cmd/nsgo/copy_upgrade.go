package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/mod/modfile"
)

// Copied-component upgrade diagnostics (#2470, #2459):
//   - In nsgo add: when a plan moves any module imported by existing copies
//     (NSGo first among them), vet those copies for the host against the
//     planned go.mod, name each copy that stops compiling with its first error
//     and nsgo diff/add commands, and refuse the plan under -y.
//   - In nsgo check: before vetting, read every nsgo-copy.yaml, validate its
//     paths and confinement, and warn for each copy whose recorded module is
//     required by go.mod at a different version (leaving healthy copies quiet
//     and passing when copies still compile).
//   - On go vet / go build / nsgo dev compile failure: inspect Go's
//     "# <import path>" lines and append the copy diagnostic line after the go
//     command's output for each failing copied component.

// brokenCopy is an existing copied component that stops compiling under an
// nsgo add plan's planned go.mod.
type brokenCopy struct {
	copy          moduleCopy
	ref           string
	targetVersion string
	firstError    string
}

// copyTargetRef returns the name to pass to nsgo diff / nsgo add for c: the
// component name when unique among copies in the module, else its relative
// directory.
func copyTargetRef(c moduleCopy, all []moduleCopy) string {
	count := 0
	for _, other := range all {
		if other.record.Component == c.record.Component {
			count++
		}
	}
	if count <= 1 {
		return c.record.Component
	}
	return c.relDir
}

// copyMismatchLine formats the diagnostic line for a copy whose source module
// is required in go.mod at requiredVer (differing from c.record.ModuleVersion).
func copyMismatchLine(c moduleCopy, all []moduleCopy, requiredVer string) string {
	cmd := nsgoCommand()
	ref := copyTargetRef(c, all)
	return fmt.Sprintf("%s is %s from %s %s; this module requires %s: %s diff %s@%s, %s add %s@%s",
		c.relDir, c.record.Component, c.record.Module, c.record.ModuleVersion, requiredVer,
		cmd, ref, requiredVer, cmd, ref, requiredVer)
}

// copyBrokenLine formats the diagnostic line for a copy that failed to compile
// when its recorded source module did not move to another required version in
// go.mod (for example, another imported module such as NSGo moved).
func copyBrokenLine(c moduleCopy, all []moduleCopy) string {
	cmd := nsgoCommand()
	ref := copyTargetRef(c, all)
	return fmt.Sprintf("%s is %s from %s %s: %s diff %s@upgrade, %s add %s@upgrade",
		c.relDir, c.record.Component, c.record.Module, c.record.ModuleVersion,
		cmd, ref, cmd, ref)
}

// loadModuleCopies parses go.mod at dir's module root and discovers all copied
// components in the module. Returns (nil, nil, nil) when dir is outside a Go
// module.
func loadModuleCopies(dir string) (*modfile.File, []moduleCopy, error) {
	modRoot := moduleRootOf(dir)
	modPath := filepath.Join(modRoot, "go.mod")
	raw, err := os.ReadFile(modPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	mf, err := modfile.Parse(modPath, raw, nil)
	if err != nil {
		return nil, nil, err
	}
	if mf.Module == nil {
		return mf, nil, nil
	}
	copies, err := findModuleCopies(modRoot, mf.Module.Mod.Path)
	if err != nil {
		return nil, nil, err
	}
	return mf, copies, nil
}

// checkCopies validates every nsgo-copy.yaml in dir's module, checks that
// recorded paths inside each copy are confined and contain no symlinks, and
// warns for each copy whose module is required by go.mod at another version.
// Up-to-date copies are not reported.
func checkCopies(dir string) error {
	mf, copies, err := loadModuleCopies(dir)
	if err != nil {
		return err
	}
	if len(copies) == 0 {
		return nil
	}
	for _, c := range copies {
		if err := checkCopyRecordSymlinks(c); err != nil {
			return err
		}
	}
	for _, c := range copies {
		reqVer := requiredVersion(mf, c.record.Module)
		if reqVer != "" && reqVer != c.record.ModuleVersion {
			warn("%s", copyMismatchLine(c, copies, reqVer))
		}
	}
	return nil
}

// checkCopyRecordSymlinks opens copy's directory via os.Root and refuses any
// existing recorded path (or parent directory along it) that is a symlink.
// Files that the user intentionally deleted from their copy are ignored.
func checkCopyRecordSymlinks(c moduleCopy) error {
	root, err := os.OpenRoot(c.dir)
	if err != nil {
		return fmt.Errorf("%s/%s: %w", c.relDir, copyRecordName, err)
	}
	defer root.Close()

	for _, relPath := range slices.Concat(c.record.Files, c.record.Generated) {
		if _, err := checkRootNoSymlinks(root, relPath); err != nil {
			return fmt.Errorf("%s/%s: %w", c.relDir, copyRecordName, err)
		}
	}
	return nil
}

// failingPackagesFromGoOutput extracts unique package import paths from Go's
// "# <import path>" header lines in go vet / go build output.
func failingPackagesFromGoOutput(output string) []string {
	var pkgs []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(output, "\n") {
		pkg, ok := parseGoPackageHeader(line)
		if !ok || seen[pkg] {
			continue
		}
		seen[pkg] = true
		pkgs = append(pkgs, pkg)
	}
	return pkgs
}

// parseGoPackageHeader parses a Go compiler/vet "# <import path>" header line
// and returns the normalized package import path (stripping optional test
// annotations like "[pkg.test]" or "_test"/".test" suffixes).
func parseGoPackageHeader(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	rest, ok := strings.CutPrefix(trimmed, "# ")
	if !ok {
		return "", false
	}
	pkg, _, _ := strings.Cut(strings.TrimSpace(rest), " ")
	pkg = strings.TrimSuffix(strings.TrimPrefix(pkg, "["), "]")
	pkg = strings.TrimSuffix(pkg, "_test")
	pkg = strings.TrimSuffix(pkg, ".test")
	if pkg == "" {
		return "", false
	}
	return pkg, true
}

// annotateCopyFailure matches failing package headers in goOutput against
// copied components in dir's module and returns one diagnostic line per
// failing copy, or empty when no copied component failed.
func annotateCopyFailure(dir, goOutput string) string {
	pkgs := failingPackagesFromGoOutput(goOutput)
	if len(pkgs) == 0 {
		return ""
	}
	modRoot := moduleRootOf(dir)
	modPath := filepath.Join(modRoot, "go.mod")
	raw, err := os.ReadFile(modPath)
	if err != nil {
		return ""
	}
	mf, err := modfile.Parse(modPath, raw, nil)
	if err != nil || mf.Module == nil {
		return ""
	}
	appModPath := mf.Module.Mod.Path
	// Fast path: skip walking the module when no failing package is inside it.
	hasLocal := false
	for _, pkg := range pkgs {
		if strings.HasPrefix(pkg, appModPath+"/") {
			hasLocal = true
			break
		}
	}
	if !hasLocal {
		return ""
	}
	copies, err := findModuleCopies(modRoot, appModPath)
	if err != nil || len(copies) == 0 {
		return ""
	}

	var notes []string
	seen := map[string]bool{}
	for _, pkg := range pkgs {
		for _, c := range copies {
			if pkg != c.importPath && !strings.HasPrefix(pkg, c.importPath+"/") {
				continue
			}
			if seen[c.relDir] {
				break
			}
			seen[c.relDir] = true
			reqVer := requiredVersion(mf, c.record.Module)
			if reqVer != "" && reqVer != c.record.ModuleVersion {
				notes = append(notes, copyMismatchLine(c, copies, reqVer))
			} else {
				notes = append(notes, copyBrokenLine(c, copies))
			}
			break
		}
	}
	return strings.Join(notes, "\n")
}

// movedModuleVersions returns every module whose required version in after
// differs from before (including newly required modules).
func movedModuleVersions(before, after *modfile.File) map[string]string {
	oldReqs := requirements(before)
	newReqs := requirements(after)
	moved := map[string]string{}
	for path, nr := range newReqs {
		if or, had := oldReqs[path]; !had || or.version != nr.version {
			moved[path] = nr.version
		}
	}
	return moved
}

// copyImportsAnyModule reports whether c imports any module in moved (directly,
// via its locales/ subpackage, or transitively through other packages in the
// same module), or whether c's recorded source module itself moved.
func copyImportsAnyModule(modRoot, appModPath string, c moduleCopy, moved map[string]string, after *modfile.File) bool {
	if _, ok := moved[c.record.Module]; ok {
		return true
	}
	visited := map[string]bool{}
	imports := map[string]bool{}
	collectLocalImports(modRoot, appModPath, c.dir, visited, imports)
	localesDir := filepath.Join(c.dir, "locales")
	if st, err := os.Lstat(localesDir); err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
		collectLocalImports(modRoot, appModPath, localesDir, visited, imports)
	}
	for imp := range imports {
		if mod := providingModule(imp, after); mod != "" {
			if _, ok := moved[mod]; ok {
				return true
			}
		}
		for mod := range moved {
			if imp == mod || strings.HasPrefix(imp, mod+"/") {
				return true
			}
		}
	}
	return false
}

// collectLocalImports parses imports from every regular .go file in pkgDir and
// recursively follows imports of other packages in the same module.
func collectLocalImports(modRoot, appModPath, pkgDir string, visited, imports map[string]bool) {
	clean := filepath.Clean(pkgDir)
	if visited[clean] {
		return
	}
	visited[clean] = true

	entries, err := os.ReadDir(clean)
	if err != nil {
		return
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(clean, name), nil, parser.ImportsOnly)
		if err != nil {
			continue
		}
		for _, spec := range f.Imports {
			imp, err := strconv.Unquote(spec.Path.Value)
			if err != nil || imp == "" {
				continue
			}
			imports[imp] = true
			if appModPath != "" && strings.HasPrefix(imp, appModPath+"/") {
				relSub := strings.TrimPrefix(imp, appModPath+"/")
				if filepath.IsLocal(filepath.FromSlash(relSub)) {
					subDir := filepath.Join(modRoot, filepath.FromSlash(relSub))
					if st, err := os.Lstat(subDir); err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0 {
						collectLocalImports(modRoot, appModPath, subDir, visited, imports)
					}
				}
			}
		}
	}
}

// checkCopiedComponents vets existing copied components that import any module
// moved by p against the planned go.mod and go.sum for the host, recording any
// copies that stop compiling in p.brokenCopies.
func (p *addPlan) checkCopiedComponents(ctx context.Context, tmp string) error {
	moved := movedModuleVersions(p.before, p.after)
	if len(moved) == 0 {
		return nil
	}
	appModPath := p.before.Module.Mod.Path
	copies, err := findModuleCopies(p.m.dir, appModPath)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	if len(copies) == 0 {
		return nil
	}

	var affected []moduleCopy
	for _, c := range copies {
		if copyImportsAnyModule(p.m.dir, appModPath, c, moved, p.after) {
			affected = append(affected, c)
		}
	}
	if len(affected) == 0 {
		return nil
	}

	stage(ctx, "vetting copied components")

	// Prepare an isolated copy of the planned go.mod and go.sum so vetting
	// never mutates p.newMod, p.newSum or the module's own files.
	vetTmp := filepath.Join(tmp, "vet")
	if err := os.MkdirAll(vetTmp, 0o755); err != nil {
		return fmt.Errorf("add: %w", err)
	}
	vetModBytes, err := absolutizeModFileReplaces(p.newMod, p.m.dir)
	if err != nil {
		return fmt.Errorf("add: %w", err)
	}
	vetMod := filepath.Join(vetTmp, "go.mod")
	if err := os.WriteFile(vetMod, vetModBytes, 0o644); err != nil {
		return fmt.Errorf("add: %w", err)
	}

	vetSum := slices.Clone(p.newSum)
	vetSum = p.ensureMovedModuleSums(ctx, vetTmp, moved, vetSum)
	if len(vetSum) > 0 || p.hasSum {
		if err := os.WriteFile(filepath.Join(vetTmp, "go.sum"), vetSum, 0o644); err != nil {
			return fmt.Errorf("add: %w", err)
		}
	}

	var patterns []string
	for _, c := range affected {
		patterns = append(patterns, "./"+c.relDir)
		localesDir := filepath.Join(c.dir, "locales")
		if st, err := os.Lstat(localesDir); err == nil && st.IsDir() && st.Mode()&os.ModeSymlink == 0 && hasGo(localesDir) {
			patterns = append(patterns, "./"+c.relDir+"/locales")
		}
	}

	args := []string{"vet", "-modfile=" + vetMod, "-mod=readonly"}
	if tags := p.hostVetTags(ctx); len(tags) > 0 {
		args = append(args, "-tags", strings.Join(tags, ","))
	}
	args = append(args, patterns...)

	cmd := exec.CommandContext(ctx, p.m.goCmd, args...)
	cmd.Dir = p.m.dir
	cmd.Env = p.m.env
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	msg := strings.TrimSpace(string(out))
	if strings.Contains(msg, "no packages to vet") && !strings.Contains(msg, "# ") {
		return nil
	}
	p.brokenCopies = parseVetCopyFailures(msg, affected, copies, p.after)
	return nil
}

// ensureMovedModuleSums downloads zip checksums for moved non-replaced modules
// that go get only recorded a /go.mod entry for, so go vet -mod=readonly can
// load packages from them without failing on missing go.sum entries.
func (p *addPlan) ensureMovedModuleSums(ctx context.Context, scratchDir string, moved map[string]string, sumData []byte) []byte {
	locallyReplaced := map[string]bool{}
	for _, r := range p.after.Replace {
		if r.New.Version == "" {
			locallyReplaced[r.Old.Path] = true
		}
	}
	var toDownload []string
	for modPath, ver := range moved {
		if ver == "" || locallyReplaced[modPath] {
			continue
		}
		prefix := modPath + " " + ver + " "
		if bytes.Contains(sumData, []byte(prefix)) {
			continue
		}
		toDownload = append(toDownload, modPath+"@"+ver)
	}
	if len(toDownload) == 0 {
		return sumData
	}
	slices.Sort(toDownload)
	cmd := exec.CommandContext(ctx, p.m.goCmd, append([]string{"mod", "download", "-json"}, toDownload...)...)
	cmd.Dir = scratchDir
	cmd.Env = p.m.env
	out, err := cmd.Output()
	if err != nil {
		return sumData
	}
	var buf bytes.Buffer
	buf.Write(sumData)
	if len(sumData) > 0 && sumData[len(sumData)-1] != '\n' {
		buf.WriteByte('\n')
	}
	for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
		var dl struct {
			Path     string
			Version  string
			Sum      string
			GoModSum string
		}
		if err := dec.Decode(&dl); err != nil {
			break
		}
		if dl.Path != "" && dl.Version != "" {
			if dl.Sum != "" {
				line := dl.Path + " " + dl.Version + " " + dl.Sum + "\n"
				if !bytes.Contains(buf.Bytes(), []byte(line)) {
					buf.WriteString(line)
				}
			}
			if dl.GoModSum != "" {
				line := dl.Path + " " + dl.Version + "/go.mod " + dl.GoModSum + "\n"
				if !bytes.Contains(buf.Bytes(), []byte(line)) {
					buf.WriteString(line)
				}
			}
		}
	}
	return buf.Bytes()
}

// hostVetTags returns build tags for host vetting in p.m.dir if nsgo.yaml
// exists and configures build.tags (merged with GOFLAGS).
func (p *addPlan) hostVetTags(ctx context.Context) []string {
	if _, err := os.Stat(filepath.Join(p.m.dir, "nsgo.yaml")); err != nil {
		return nil
	}
	proj, err := load(p.m.dir)
	if err != nil {
		return nil
	}
	tags, err := goTags(ctx, proj, p.m.env)
	if err != nil {
		return proj.Build.Tags
	}
	return tags
}

// parseVetCopyFailures groups go vet output by "# <import path>" section and
// matches each failing package back to its copied component, keeping each
// broken copy's first diagnostic line.
func parseVetCopyFailures(output string, affected, allCopies []moduleCopy, after *modfile.File) []brokenCopy {
	firstErrByDir := map[string]string{}
	var order []string
	recordErr := func(c moduleCopy, errLine string) {
		if _, exists := firstErrByDir[c.relDir]; exists {
			return
		}
		firstErrByDir[c.relDir] = errLine
		order = append(order, c.relDir)
	}
	matchCopy := func(pkg string) (moduleCopy, bool) {
		for _, c := range affected {
			if pkg == c.importPath || strings.HasPrefix(pkg, c.importPath+"/") {
				return c, true
			}
		}
		return moduleCopy{}, false
	}

	var curCopy moduleCopy
	var hasCur bool
	for line := range strings.SplitSeq(output, "\n") {
		if pkg, ok := parseGoPackageHeader(line); ok {
			curCopy, hasCur = matchCopy(pkg)
			continue
		}
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "vet: ") && hasCur && firstErrByDir[curCopy.relDir] != "" {
			continue
		}
		if hasCur {
			recordErr(curCopy, trimmed)
		}
	}

	// Fallback if go vet failed without emitting a matching "# <pkg>" header
	// (for example, a package load or syntax error).
	if len(order) == 0 && len(affected) > 0 {
		for line := range strings.SplitSeq(output, "\n") {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			slashed := filepath.ToSlash(trimmed)
			for _, c := range affected {
				if strings.HasPrefix(slashed, c.relDir+"/") || strings.HasPrefix(slashed, "./"+c.relDir+"/") || strings.Contains(slashed, c.importPath) {
					recordErr(c, trimmed)
				}
			}
		}
	}
	if len(order) == 0 && len(affected) > 0 {
		firstLine := output
		if line, _, ok := strings.Cut(output, "\n"); ok {
			firstLine = strings.TrimSpace(line)
		}
		if firstLine == "" {
			firstLine = "go vet failed"
		}
		recordErr(affected[0], firstLine)
	}

	byDir := make(map[string]moduleCopy, len(affected))
	for _, c := range affected {
		byDir[c.relDir] = c
	}
	broken := make([]brokenCopy, 0, len(order))
	for _, relDir := range order {
		c := byDir[relDir]
		targetVer := "upgrade"
		if v := requiredVersion(after, c.record.Module); v != "" && v != c.record.ModuleVersion {
			targetVer = v
		}
		broken = append(broken, brokenCopy{
			copy:          c,
			ref:           copyTargetRef(c, allCopies),
			targetVersion: targetVer,
			firstError:    firstErrByDir[relDir],
		})
	}
	return broken
}

// brokenCopyLines formats broken copies for nsgo add's plan output.
func (p *addPlan) brokenCopyLines() []string {
	if len(p.brokenCopies) == 0 {
		return nil
	}
	cmd := nsgoCommand()
	lines := make([]string, 0, len(p.brokenCopies)*2)
	for _, b := range p.brokenCopies {
		lines = append(lines, fmt.Sprintf("%s is %s from %s %s: %s",
			b.copy.relDir, b.copy.record.Component, b.copy.record.Module, b.copy.record.ModuleVersion, b.firstError))
		lines = append(lines, fmt.Sprintf("%s diff %s@%s, %s add %s@%s",
			cmd, b.ref, b.targetVersion, cmd, b.ref, b.targetVersion))
	}
	return lines
}

// yesRefusal returns an error refusing -y when the plan breaks one or more
// copied components.
func (p *addPlan) yesRefusal() error {
	if len(p.brokenCopies) == 0 {
		return nil
	}
	lines := make([]string, 0, len(p.brokenCopies))
	for _, b := range p.brokenCopies {
		lines = append(lines, fmt.Sprintf("  %s: %s", b.copy.relDir, b.firstError))
	}
	return fmt.Errorf("add: refused under -y because the plan breaks copied components (review and confirm interactively without -y):\n%s", strings.Join(lines, "\n"))
}
