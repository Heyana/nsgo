package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	"strings"
	"unicode"
	"unicode/utf8"

	buildtargets "github.com/nsgo-dev/nsgo/targets"
	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/semver"
)

// nsgo-module.yaml sits optionally beside go.mod at a library module's root (#2462).
// It declares which packages apps import, which build targets the module supports,
// plain-text author notes shown last in nsgo add's plan, and which packages the
// module offers for copying as components. nsgo build never reads it.

const (
	moduleManifestName     = "nsgo-module.yaml"
	maxModuleManifestBytes = 1 << 20 // 1 MiB
	maxModuleNotesBytes    = 4 << 10 // 4 KiB
	moduleManifestVersion  = 1
)

// moduleManifestNSGoVersion is this nsgo's version when comparing against a
// module's require github.com/nsgo-dev/nsgo line; tests replace it.
var moduleManifestNSGoVersion = toolVersion

// moduleManifest is the parsed form of a module's nsgo-module.yaml.
type moduleManifest struct {
	Version    int               `yaml:"version"`
	Import     []string          `yaml:"import"`
	Targets    []string          `yaml:"targets"`
	Notes      string            `yaml:"notes"`
	Components map[string]string `yaml:"components"`

	// hasImport records whether the YAML file declared an import key, so an
	// explicit empty list suppresses the root-package fallback while omitting
	// the key keeps it.
	hasImport bool
	// expandedTargets are Targets expanded through targets.Expand.
	expandedTargets []string
}

// readModuleManifest reads and parses nsgo-module.yaml from modDir through
// os.Root, returning (nil, nil) when the file is absent.
func readModuleManifest(modDir, modPath, currentNSGo string) (*moduleManifest, error) {
	root, err := os.OpenRoot(modDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	f, err := root.Open(moduleManifestName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: must be a regular file", moduleManifestName)
	}
	if info.Size() > maxModuleManifestBytes {
		return nil, fmt.Errorf("%s exceeds 1 MiB", moduleManifestName)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxModuleManifestBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}
	requiredNSGo := readModuleRequiredNSGo(root)
	return parseModuleManifest(data, modPath, requiredNSGo, currentNSGo)
}

// readModuleRequiredNSGo returns the module's require github.com/nsgo-dev/nsgo
// version from go.mod inside root, or empty when absent or unreadable.
func readModuleRequiredNSGo(root *os.Root) string {
	f, err := root.Open("go.mod")
	if err != nil {
		return ""
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxModuleManifestBytes))
	if err != nil {
		return ""
	}
	mf, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return ""
	}
	return requiredVersion(mf, nsgoModule)
}

// parseModuleManifest parses and validates nsgo-module.yaml bytes. When the
// module requires a newer NSGo than currentNSGo and the file contains an
// unknown field (or newer schema version), the error names the required version.
func parseModuleManifest(data []byte, modPath, requiredNSGo, currentNSGo string) (*moduleManifest, error) {
	if len(data) > maxModuleManifestBytes {
		return nil, fmt.Errorf("%s exceeds 1 MiB", moduleManifestName)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: empty YAML file", moduleManifestName)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%s: empty YAML file", moduleManifestName)
		}
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%s: multiple YAML documents are not allowed", moduleManifestName)
		}
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: must be a YAML mapping", moduleManifestName)
	}
	if err := rejectYAMLAliasesAndMerges(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}

	newer := requiresNewerNSGo(requiredNSGo, currentNSGo)
	root := doc.Content[0]
	var hasImport, hasTargets, hasComponents bool
	for i := 0; i+1 < len(root.Content); i += 2 {
		k := root.Content[i]
		if k.Kind != yaml.ScalarNode || k.Tag != "!!str" {
			return nil, fmt.Errorf("%s: keys must be strings", moduleManifestName)
		}
		switch k.Value {
		case "version", "notes":
		case "import":
			hasImport = true
		case "targets":
			hasTargets = true
		case "components":
			hasComponents = true
		default:
			if newer && modPath != "" {
				return nil, fmt.Errorf("%s needs NSGo %s: go get %s@%s", modPath, requiredNSGo, nsgoModule, requiredNSGo)
			}
			return nil, fmt.Errorf("%s: unknown field %q", moduleManifestName, k.Value)
		}
	}

	var m moduleManifest
	strict := yaml.NewDecoder(bytes.NewReader(data))
	strict.KnownFields(true)
	if err := strict.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}
	m.hasImport = hasImport

	switch {
	case m.Version == 0:
		return nil, fmt.Errorf("%s: version: %d is required", moduleManifestName, moduleManifestVersion)
	case m.Version > moduleManifestVersion && newer && modPath != "":
		return nil, fmt.Errorf("%s needs NSGo %s: go get %s@%s", modPath, requiredNSGo, nsgoModule, requiredNSGo)
	case m.Version != moduleManifestVersion:
		return nil, fmt.Errorf("%s: version is %d; this nsgo reads version %d", moduleManifestName, m.Version, moduleManifestVersion)
	}

	seenImport := make(map[string]bool, len(m.Import))
	for _, dir := range m.Import {
		if !validManifestRelPath(dir, true) {
			return nil, fmt.Errorf("%s: import %q must be a clean relative directory path in the module (use \".\" for the root)", moduleManifestName, dir)
		}
		if seenImport[dir] {
			return nil, fmt.Errorf("%s: import repeats %q", moduleManifestName, dir)
		}
		seenImport[dir] = true
	}

	if hasTargets {
		if len(m.Targets) == 0 {
			return nil, fmt.Errorf("%s: targets must not be empty", moduleManifestName)
		}
		expanded, err := buildtargets.Expand(m.Targets)
		if err != nil {
			return nil, fmt.Errorf("%s: targets: %w", moduleManifestName, err)
		}
		m.expandedTargets = expanded
	}

	if err := validateModuleNotes(m.Notes); err != nil {
		return nil, fmt.Errorf("%s: %w", moduleManifestName, err)
	}
	m.Notes = strings.ReplaceAll(m.Notes, "\r\n", "\n")

	if hasComponents {
		for _, name := range slices.Sorted(maps.Keys(m.Components)) {
			if !validComponentName(name) {
				return nil, fmt.Errorf("%s: component name %q must be 1 to 64 characters of [a-z0-9-], not starting or ending with '-'", moduleManifestName, name)
			}
			dir := m.Components[name]
			if !validManifestRelPath(dir, false) {
				return nil, fmt.Errorf("%s: component %q path %q must be a clean relative directory below the module root", moduleManifestName, name, dir)
			}
		}
	}

	return &m, nil
}

// requiresNewerNSGo reports whether required is a valid semantic version newer
// than current.
func requiresNewerNSGo(required, current string) bool {
	return semver.IsValid(required) && semver.IsValid(current) && semver.Compare(required, current) > 0
}

// rejectYAMLAliasesAndMerges walks n and refuses anchors, aliases, merge keys
// and duplicate mapping keys.
func rejectYAMLAliasesAndMerges(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode {
		return errors.New("YAML aliases are not allowed")
	}
	if n.Anchor != "" {
		return errors.New("YAML anchors are not allowed")
	}
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag == "!!merge" {
				return errors.New("YAML merge keys are not allowed")
			}
			if k.Kind == yaml.ScalarNode {
				if seen[k.Value] {
					return fmt.Errorf("duplicate key %q", k.Value)
				}
				seen[k.Value] = true
			}
		}
	}
	for _, child := range n.Content {
		if err := rejectYAMLAliasesAndMerges(child); err != nil {
			return err
		}
	}
	return nil
}

// validComponentName reports whether name is 1 to 64 characters of [a-z0-9-],
// not starting or ending with '-'.
func validComponentName(name string) bool {
	if len(name) < 1 || len(name) > 64 || name[0] == '-' || name[len(name)-1] == '-' {
		return false
	}
	for i := range len(name) {
		c := name[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// validManifestRelPath reports whether p is a clean slash-separated relative
// directory inside the module root, allowing "." only when allowRoot is true.
func validManifestRelPath(p string, allowRoot bool) bool {
	if p == "." {
		return allowRoot
	}
	if p == "" || !fs.ValidPath(p) || strings.Contains(p, "\\") || hasControlOrFormat(p) {
		return false
	}
	_, err := filepath.Localize(p)
	return err == nil
}

// hasControlOrFormat reports whether s contains any control or Unicode format
// character or invalid UTF-8.
func hasControlOrFormat(s string) bool {
	if !utf8.ValidString(s) {
		return true
	}
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}

// validateModuleNotes checks that notes is at most 4 KiB of valid UTF-8 plain
// text containing no control or Unicode format characters other than line
// breaks (\n or \r\n).
func validateModuleNotes(s string) error {
	if len(s) > maxModuleNotesBytes {
		return errors.New("notes exceed 4 KiB")
	}
	if !utf8.ValidString(s) {
		return errors.New("notes must be valid UTF-8")
	}
	for i, r := range s {
		if r == '\n' {
			continue
		}
		if r == '\r' && i+1 < len(s) && s[i+1] == '\n' {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return fmt.Errorf("notes contain disallowed character %U", r)
		}
	}
	return nil
}

// supportsTarget reports whether m's targets list covers target, or true when m
// declares no targets list. macos_universal is covered when listed explicitly
// or when both macos_arm64 and macos_x64 are covered.
func (m *moduleManifest) supportsTarget(target string) bool {
	if m == nil || len(m.expandedTargets) == 0 {
		return true
	}
	if slices.Contains(m.expandedTargets, target) {
		return true
	}
	if target == "macos_universal" {
		return slices.Contains(m.expandedTargets, "macos_arm64") && slices.Contains(m.expandedTargets, "macos_x64")
	}
	return false
}

// missingTargets returns the targets from appTargets that m's targets list
// lacks, preserving appTargets' order.
func (m *moduleManifest) missingTargets(appTargets []string) []string {
	if m == nil || len(m.expandedTargets) == 0 || len(appTargets) == 0 {
		return nil
	}
	var missing []string
	for _, t := range appTargets {
		if !m.supportsTarget(t) {
			missing = append(missing, t)
		}
	}
	return missing
}

// importPaths returns the Go package import paths declared by m.Import for modPath.
func (m *moduleManifest) importPaths(modPath string) []string {
	if m == nil || len(m.Import) == 0 {
		return nil
	}
	out := make([]string, len(m.Import))
	for i, dir := range m.Import {
		out[i] = manifestPackagePath(modPath, dir)
	}
	return out
}

// componentNames returns m's declared component names in sorted order.
func (m *moduleManifest) componentNames() []string {
	if m == nil || len(m.Components) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(m.Components))
}

// formattedNotes returns the lines of m.Notes with trailing blank newlines
// trimmed, or nil when notes has no non-whitespace content.
func (m *moduleManifest) formattedNotes() []string {
	if m == nil || strings.TrimSpace(m.Notes) == "" {
		return nil
	}
	trimmed := strings.TrimRight(m.Notes, "\n")
	return strings.Split(trimmed, "\n")
}

// manifestPackagePath joins a module path with a clean relative directory ("."
// for the module root).
func manifestPackagePath(modPath, relDir string) string {
	if relDir == "." {
		return modPath
	}
	return modPath + "/" + relDir
}

// goListManifestPackage is what go list -find -e -json prints when validating
// the packages named by nsgo-module.yaml.
type goListManifestPackage struct {
	ImportPath     string
	Name           string
	Dir            string
	IgnoredGoFiles []string
	Module         *struct{ Path, Dir string }
	Error          *struct{ Err string }
}

// validateModuleManifestPackages checks every package listed in manifest.Import
// and manifest.Components against go list and the module root: each must be a
// package belonging to modPath (never a nested module), never a main package,
// and each component's last path element must equal its Go package name.
func (m *addModule) validateModuleManifestPackages(ctx context.Context, flags []string, modPath, modDir string, manifest *moduleManifest) error {
	if manifest == nil || (len(manifest.Import) == 0 && len(manifest.Components) == 0) {
		return nil
	}
	root, err := os.OpenRoot(modDir)
	if err != nil {
		return err
	}
	defer root.Close()

	var pkgPaths []string
	seen := map[string]bool{}
	addPath := func(relDir string) {
		p := manifestPackagePath(modPath, relDir)
		if !seen[p] {
			seen[p] = true
			pkgPaths = append(pkgPaths, p)
		}
	}
	for _, dir := range manifest.Import {
		addPath(dir)
	}
	for _, name := range manifest.componentNames() {
		addPath(manifest.Components[name])
	}

	out, err := m.goOutput(ctx, append(append([]string{"list", "-find", "-e", "-json=ImportPath,Name,Dir,IgnoredGoFiles,Module,Error", "-mod=readonly"}, flags...), pkgPaths...)...)
	if err != nil {
		return err
	}
	pkgs := map[string]goListManifestPackage{}
	for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
		var pkg goListManifestPackage
		if err := dec.Decode(&pkg); err != nil {
			return fmt.Errorf("go list: %w", err)
		}
		pkgs[pkg.ImportPath] = pkg
	}

	resolvePkg := func(kind, label, relDir string) (string, error) {
		if hasNestedGoMod(root, relDir) {
			return "", fmt.Errorf("%s: %s %s (%s) is inside a nested module, not %s", moduleManifestName, kind, label, relDir, modPath)
		}
		imp := manifestPackagePath(modPath, relDir)
		pkg, ok := pkgs[imp]
		if !ok {
			return "", fmt.Errorf("%s: %s %s (%s) is not a package of %s", moduleManifestName, kind, label, relDir, modPath)
		}
		if pkg.Module == nil || pkg.Module.Path != modPath {
			if pkg.Module != nil {
				return "", fmt.Errorf("%s: %s %s (%s) belongs to module %s, not %s", moduleManifestName, kind, label, relDir, pkg.Module.Path, modPath)
			}
			if pkg.Error != nil && pkg.Error.Err != "" {
				return "", fmt.Errorf("%s: %s %s (%s) is not a package of %s: %s", moduleManifestName, kind, label, relDir, modPath, pkg.Error.Err)
			}
			return "", fmt.Errorf("%s: %s %s (%s) is not a package of %s", moduleManifestName, kind, label, relDir, modPath)
		}
		pkgName := pkg.Name
		if pkg.Error != nil {
			if !strings.Contains(pkg.Error.Err, "build constraints exclude all Go files") || len(pkg.IgnoredGoFiles) == 0 {
				return "", fmt.Errorf("%s: %s %s (%s): %s", moduleManifestName, kind, label, relDir, pkg.Error.Err)
			}
			pkgName, err = ignoredPackageName(root, relDir, pkg.IgnoredGoFiles)
			if err != nil {
				return "", fmt.Errorf("%s: %s %s (%s): %w", moduleManifestName, kind, label, relDir, err)
			}
		}
		if pkgName == "" {
			return "", fmt.Errorf("%s: %s %s (%s) has no Go package", moduleManifestName, kind, label, relDir)
		}
		return pkgName, nil
	}

	for _, dir := range manifest.Import {
		pkgName, err := resolvePkg("import", fmt.Sprintf("%q", dir), dir)
		if err != nil {
			return err
		}
		if pkgName == "main" {
			return fmt.Errorf("%s: import %q is a main package, which cannot be imported", moduleManifestName, dir)
		}
	}

	for _, name := range manifest.componentNames() {
		dir := manifest.Components[name]
		pkgName, err := resolvePkg("component", fmt.Sprintf("%q", name), dir)
		if err != nil {
			return err
		}
		if pkgName == "main" {
			return fmt.Errorf("%s: component %q (%s) is a main package", moduleManifestName, name, dir)
		}
		want := path.Base(dir)
		if pkgName != want {
			return fmt.Errorf("%s: component %q (%s) has package name %q; its directory must end in %q", moduleManifestName, name, dir, pkgName, pkgName)
		}
	}

	return nil
}

// hasNestedGoMod reports whether relDir (when below the module root) or any of
// its ancestor directories below the module root contains a go.mod file.
func hasNestedGoMod(root *os.Root, relDir string) bool {
	if relDir == "." {
		return false
	}
	parts := strings.Split(relDir, "/")
	cur := ""
	for _, part := range parts {
		if cur == "" {
			cur = part
		} else {
			cur += "/" + part
		}
		if _, err := root.Stat(cur + "/go.mod"); err == nil {
			return true
		}
	}
	return false
}

// ignoredPackageName reads the package clause of non-test Go files listed in
// ignored inside relDir under root when build constraints excluded them on the
// host OS/architecture.
func ignoredPackageName(root *os.Root, relDir string, ignored []string) (string, error) {
	fset := token.NewFileSet()
	pkgName := ""
	for _, name := range ignored {
		if strings.HasSuffix(name, "_test.go") || !strings.HasSuffix(name, ".go") {
			continue
		}
		filePath := name
		if relDir != "." {
			filePath = relDir + "/" + name
		}
		f, err := root.Open(filePath)
		if err != nil {
			return "", err
		}
		src, readErr := io.ReadAll(io.LimitReader(f, maxModuleManifestBytes))
		f.Close()
		if readErr != nil {
			return "", readErr
		}
		parsed, err := parser.ParseFile(fset, name, src, parser.PackageClauseOnly)
		if err != nil {
			return "", err
		}
		if pkgName == "" {
			pkgName = parsed.Name.Name
		} else if parsed.Name.Name != pkgName {
			return "", fmt.Errorf("found multiple packages %q and %q", pkgName, parsed.Name.Name)
		}
	}
	if pkgName == "" {
		return "", errors.New("no non-test Go files")
	}
	return pkgName, nil
}

// hostTargetName returns NSGo's target name for the current host, or empty on
// unsupported hosts.
func hostTargetName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos_" + goArchName(runtime.GOARCH)
	case "windows", "linux":
		return runtime.GOOS + "_" + goArchName(runtime.GOARCH)
	default:
		return ""
	}
}

// checkLinkedModuleTargets lists the modules linked by p's packages for target
// and returns warnings for any linked module whose nsgo-module.yaml declares
// targets lacking target. Modules without nsgo-module.yaml (or without targets)
// produce no warning.
func checkLinkedModuleTargets(ctx context.Context, p *project, target string, env, extraTags []string) []string {
	if p == nil || target == "" {
		return nil
	}
	tags, err := goTags(ctx, p, env, extraTags...)
	if err != nil {
		return nil
	}
	args := append([]string{"list", "-deps", "-e", "-json=Standard,Module"}, tagsArgs(tags)...)
	args = append(args, "./...")
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil || len(out) == 0 {
		return nil
	}

	type listedMod struct {
		Path, Dir string
		Main      bool
		Replace   *struct{ Path, Dir string }
	}
	modules := map[string]string{}
	for dec := json.NewDecoder(bytes.NewReader(out)); dec.More(); {
		var pkg struct {
			Standard bool
			Module   *listedMod
		}
		if err := dec.Decode(&pkg); err != nil {
			break
		}
		if pkg.Standard || pkg.Module == nil || pkg.Module.Main {
			continue
		}
		dir := pkg.Module.Dir
		if pkg.Module.Replace != nil && pkg.Module.Replace.Dir != "" {
			dir = pkg.Module.Replace.Dir
		}
		if dir != "" {
			modules[pkg.Module.Path] = dir
		}
	}

	currentNSGo := moduleManifestNSGoVersion()
	var warnings []string
	for _, modPath := range slices.Sorted(maps.Keys(modules)) {
		manifest, err := readModuleManifest(modules[modPath], modPath, currentNSGo)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s: %v", modPath, err))
			continue
		}
		if manifest != nil && !manifest.supportsTarget(target) {
			warnings = append(warnings, fmt.Sprintf("%s does not list %s in %s targets", modPath, target, moduleManifestName))
		}
	}
	return warnings
}
