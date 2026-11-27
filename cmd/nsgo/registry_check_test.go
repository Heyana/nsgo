package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io"
	"io/fs"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/modfile"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
	modzip "golang.org/x/mod/zip"
)

// Online registry policy checks (#2472).
//
// TestRegistryOnline runs only when NSGO_REGISTRY_ONLINE=1 (invoked weekly and
// on registry.yaml pull requests by .github/workflows/registry.yml). The
// underlying registryChecker methods are exercised offline against a local
// file:// module proxy built with golang.org/x/mod/zip.CreateFromDir and an
// httptest.Server.

const (
	registryCheckEnvVar         = "NSGO_REGISTRY_ONLINE"
	registryCheckIndexFile      = "registry.yaml"
	registryCheckModuleManifest = "nsgo-module.yaml"
	registryCheckMaxManifest    = 1 << 20 // 1 MiB
	registryCheckMaxNotes       = 4 << 10 // 4 KiB
	registryCheckMaxIndex       = 4 << 20 // 4 MiB
)

// registryCheckIndex matches version 1 of cmd/nsgo/registry.yaml (#2461).
type registryCheckIndex struct {
	Version int                  `yaml:"version"`
	Items   []registryCheckEntry `yaml:"items"`
}

// registryCheckEntry is one library or component listed in registry.yaml.
type registryCheckEntry struct {
	Name        string   `yaml:"name"`
	Module      string   `yaml:"module"`
	Component   string   `yaml:"component,omitempty"`
	Version     string   `yaml:"version,omitempty"`
	Description string   `yaml:"description"`
	License     string   `yaml:"license"`
	Homepage    string   `yaml:"homepage,omitempty"`
	Tags        []string `yaml:"tags,omitempty"`
}

func (e registryCheckEntry) targetQuery() string {
	if e.Version != "" {
		return e.Version
	}
	return "upgrade"
}

// registryCheckManifest matches version 1 of nsgo-module.yaml (#2462).
type registryCheckManifest struct {
	Version    int               `yaml:"version"`
	Import     []string          `yaml:"import"`
	Targets    []string          `yaml:"targets"`
	Notes      string            `yaml:"notes"`
	Components map[string]string `yaml:"components"`

	hasImport bool
}

// parseRegistryCheckIndex parses a version 1 registry.yaml strictly.
func parseRegistryCheckIndex(data []byte) (*registryCheckIndex, error) {
	if len(data) > registryCheckMaxIndex {
		return nil, fmt.Errorf("registry index exceeds %d bytes", registryCheckMaxIndex)
	}
	if !utf8.Valid(data) {
		return nil, errors.New("registry index is not valid UTF-8")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, errors.New("empty YAML file")
	}
	if err := inspectStrictYAMLDoc(data); err != nil {
		return nil, err
	}
	var idx registryCheckIndex
	if err := decodeStrictYAML(data, &idx); err != nil {
		return nil, err
	}
	if idx.Version != 1 {
		return nil, fmt.Errorf("unsupported registry index version %d; expected 1", idx.Version)
	}
	return &idx, nil
}

// loadRegistryCheckFile reads and parses path if it exists. When the file is
// absent (before #2461 merges cmd/nsgo/registry.yaml), exists is false and err
// is nil.
func loadRegistryCheckFile(path string) (idx *registryCheckIndex, exists bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	idx, err = parseRegistryCheckIndex(data)
	if err != nil {
		return nil, true, err
	}
	return idx, true, nil
}

// inspectStrictYAMLDoc rejects multiple documents, non-mapping roots, YAML
// anchors, aliases, merge keys, and duplicate mapping keys before decoding.
func inspectStrictYAMLDoc(data []byte) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return err
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("multiple YAML documents are not allowed")
		}
		return err
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return errors.New("YAML root must be a mapping")
	}
	return walkStrictYAMLNode(&doc, 0)
}

func walkStrictYAMLNode(n *yaml.Node, depth int) error {
	if depth > 32 {
		return errors.New("YAML nesting depth limit exceeded")
	}
	if n.Anchor != "" {
		return fmt.Errorf("line %d: YAML anchors are not allowed", n.Line)
	}
	if n.Kind == yaml.AliasNode || n.Alias != nil {
		return fmt.Errorf("line %d: YAML aliases are not allowed", n.Line)
	}
	if n.Tag == "!!merge" {
		return fmt.Errorf("line %d: YAML merge keys are not allowed", n.Line)
	}
	if n.Kind == yaml.MappingNode {
		seen := make(map[string]bool, len(n.Content)/2)
		for i := 0; i+1 < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Tag == "!!merge" {
				return fmt.Errorf("line %d: YAML merge keys are not allowed", k.Line)
			}
			if k.Kind != yaml.ScalarNode {
				return fmt.Errorf("line %d: YAML mapping keys must be scalars", k.Line)
			}
			if seen[k.Value] {
				return fmt.Errorf("line %d: duplicate YAML key %q", k.Line, k.Value)
			}
			seen[k.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := walkStrictYAMLNode(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}

// readRegistryCheckModuleManifest parses and validates nsgo-module.yaml at
// modDir through os.Root, returning (nil, nil) when the file is absent.
func readRegistryCheckModuleManifest(modDir string) (*registryCheckManifest, error) {
	root, err := os.OpenRoot(modDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	f, err := root.Open(registryCheckModuleManifest)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", registryCheckModuleManifest, err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, fmt.Errorf("%s: %w", registryCheckModuleManifest, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: must be a regular file", registryCheckModuleManifest)
	}
	if info.Size() > registryCheckMaxManifest {
		return nil, fmt.Errorf("%s exceeds 1 MiB", registryCheckModuleManifest)
	}
	data, err := io.ReadAll(io.LimitReader(f, registryCheckMaxManifest+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", registryCheckModuleManifest, err)
	}
	return parseRegistryCheckModuleManifest(data)
}

func parseRegistryCheckModuleManifest(data []byte) (*registryCheckManifest, error) {
	if len(data) > registryCheckMaxManifest {
		return nil, fmt.Errorf("%s exceeds 1 MiB", registryCheckModuleManifest)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s: invalid UTF-8", registryCheckModuleManifest)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: empty YAML file", registryCheckModuleManifest)
	}
	if err := inspectStrictYAMLDoc(data); err != nil {
		return nil, fmt.Errorf("%s: %w", registryCheckModuleManifest, err)
	}

	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", registryCheckModuleManifest, err)
	}
	hasImport := false
	if len(doc.Content) == 1 && doc.Content[0].Kind == yaml.MappingNode {
		for i := 0; i+1 < len(doc.Content[0].Content); i += 2 {
			if doc.Content[0].Content[i].Value == "import" {
				hasImport = true
				break
			}
		}
	}

	var m registryCheckManifest
	if err := decodeStrictYAML(data, &m); err != nil {
		return nil, fmt.Errorf("%s: %w", registryCheckModuleManifest, err)
	}
	m.hasImport = hasImport
	if m.Version != 1 {
		return nil, fmt.Errorf("%s: unsupported version %d; expected 1", registryCheckModuleManifest, m.Version)
	}

	seenImport := make(map[string]bool, len(m.Import))
	for _, dir := range m.Import {
		if !validRegistryCheckRelPath(dir, true) {
			return nil, fmt.Errorf("%s: import path %q must be '.' or a clean relative directory", registryCheckModuleManifest, dir)
		}
		if seenImport[dir] {
			return nil, fmt.Errorf("%s: duplicate import path %q", registryCheckModuleManifest, dir)
		}
		seenImport[dir] = true
	}

	if len(m.Targets) > 0 {
		if _, err := expandTargets(m.Targets); err != nil {
			return nil, fmt.Errorf("%s: targets: %w", registryCheckModuleManifest, err)
		}
	}

	if len(m.Notes) > registryCheckMaxNotes {
		return nil, fmt.Errorf("%s: notes exceed 4 KiB", registryCheckModuleManifest)
	}
	for i, r := range m.Notes {
		if r == '\n' {
			continue
		}
		if r == '\r' && i+1 < len(m.Notes) && m.Notes[i+1] == '\n' {
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return nil, fmt.Errorf("%s: notes contain disallowed character %U", registryCheckModuleManifest, r)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(m.Components)) {
		if !validRegistryCheckName(name) {
			return nil, fmt.Errorf("%s: invalid component name %q", registryCheckModuleManifest, name)
		}
		dir := m.Components[name]
		if !validRegistryCheckRelPath(dir, false) {
			return nil, fmt.Errorf("%s: component %q path %q must be a clean relative directory below the module root", registryCheckModuleManifest, name, dir)
		}
	}
	return &m, nil
}

func validRegistryCheckName(s string) bool {
	if len(s) == 0 || len(s) > 64 || s[0] == '-' || s[len(s)-1] == '-' {
		return false
	}
	for i := range len(s) {
		c := s[i]
		if !('a' <= c && c <= 'z' || '0' <= c && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

func validRegistryCheckRelPath(p string, allowRoot bool) bool {
	if p == "." {
		return allowRoot
	}
	if p == "" || !fs.ValidPath(p) || strings.Contains(p, "\\") || !utf8.ValidString(p) {
		return false
	}
	for _, r := range p {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
			return false
		}
	}
	_, err := filepath.Localize(p)
	return err == nil
}

// isReleaseTag reports whether v is a canonical semantic release tag (never a
// pseudo-version or pre-release).
func isReleaseTag(v string) bool {
	return semver.IsValid(v) && !module.IsPseudoVersion(v) && semver.Prerelease(v) == ""
}

// moduleRepoURL maps a Go module path to the HTTPS URL checked for repository
// redirects. Two-segment code hosts map to https://<host>/<owner>/<repo>,
// gitlab.com strips a trailing major-version segment, and vanity domains query
// https://<modulePath>?go-get=1.
func moduleRepoURL(modPath string) (string, error) {
	if err := module.CheckPath(modPath); err != nil {
		return "", fmt.Errorf("invalid module path %q: %w", modPath, err)
	}
	parts := strings.Split(modPath, "/")
	host := parts[0]
	switch host {
	case "github.com", "bitbucket.org", "codeberg.org", "gitee.com", "git.sr.ht":
		if len(parts) < 3 {
			return "", fmt.Errorf("module path %q on %s lacks owner/repo", modPath, host)
		}
		return "https://" + strings.Join(parts[:3], "/"), nil
	case "gitlab.com":
		if len(parts) < 3 {
			return "", fmt.Errorf("module path %q on %s lacks group/project", modPath, host)
		}
		trimmed := parts
		if isMajorVersionSegment(trimmed[len(trimmed)-1]) {
			trimmed = trimmed[:len(trimmed)-1]
		}
		return "https://" + strings.Join(trimmed, "/"), nil
	default:
		return "https://" + modPath + "?go-get=1", nil
	}
}

func isMajorVersionSegment(seg string) bool {
	if len(seg) < 2 || seg[0] != 'v' {
		return false
	}
	n, err := strconv.Atoi(seg[1:])
	return err == nil && n >= 2
}

// registryChecker verifies every online policy requirement for a registry item.
type registryChecker struct {
	goCmd        string
	env          []string
	goVersion    string
	nsgoVersion  string
	nsgoLocalDir string
	httpClient   *http.Client
	repoURL      func(modPath string) (string, error)
	runCheck     func(ctx context.Context, dir string, env []string) error
}

func newOnlineRegistryChecker(ctx context.Context) (*registryChecker, error) {
	goCmd, err := exec.LookPath("go")
	if err != nil {
		return nil, fmt.Errorf("go not found on PATH: %w", err)
	}
	vars, err := goEnvValues(ctx, goCmd, ".", append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=local"), "GOPROXY")
	if err != nil {
		return nil, err
	}
	env := mergeEnv(os.Environ(),
		"GO111MODULE=on",
		"GOWORK=off",
		"GOTOOLCHAIN=local",
		"GOPROXY="+initModuleProxy(vars["GOPROXY"]),
	)
	nsgoLocalDir := ""
	if root, err := filepath.Abs(filepath.Join("..", "..")); err == nil {
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err == nil {
			nsgoLocalDir = root
		}
	}
	return &registryChecker{
		goCmd:        goCmd,
		env:          env,
		goVersion:    repoGoVersion(),
		nsgoLocalDir: nsgoLocalDir,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		repoURL:  moduleRepoURL,
		runCheck: runNsgoCheckLinuxX64,
	}, nil
}

func repoGoVersion() string {
	data, err := os.ReadFile(filepath.Join("..", "..", "go.mod"))
	if err != nil {
		return "1.27.3"
	}
	mf, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil || mf.Go == nil || mf.Go.Version == "" {
		return "1.27.3"
	}
	return mf.Go.Version
}

// mergeEnv returns a copy of base with each KEY=VALUE override replacing any
// existing entry with the same key.
func mergeEnv(base []string, overrides ...string) []string {
	out := slices.Clone(base)
	for _, kv := range overrides {
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			continue
		}
		replaced := false
		for i, existing := range out {
			if ek, _, _ := strings.Cut(existing, "="); strings.EqualFold(ek, key) {
				out[i] = kv
				replaced = true
				break
			}
		}
		if !replaced {
			out = append(out, kv)
		}
	}
	return out
}

type downloadedRegistryModule struct {
	Path    string
	Version string
	Dir     string
	GoMod   string
	Zip     string
	Error   string
}

// checkEntry runs all listing policy checks for one registry item:
//  1. @upgrade (or pinned version) resolves to a semantic release tag, and go.mod declares the listed module path;
//  2. the repository URL does not redirect (HTTP 3xx fails);
//  3. adding the item to a scratch module requiring NSGo's latest release keeps NSGo on a release;
//  4. nsgo-module.yaml (if present) and each linked nsgo-natives.yaml parse cleanly;
//  5. libraries vet cleanly under `nsgo check -target linux_x64` importing their declared `import` packages;
//  6. components are declared in nsgo-module.yaml at @upgrade and (unless module is NSGo) copy into a scratch app and vet under `nsgo check -target linux_x64`.
func (c *registryChecker) checkEntry(ctx context.Context, entry registryCheckEntry) error {
	if err := module.CheckPath(entry.Module); err != nil {
		return fmt.Errorf("%s: invalid module path %q: %w", entry.Name, entry.Module, err)
	}

	// 1. Resolve version and verify go.mod declares the listed path.
	dl, err := c.resolveAndVerifyModule(ctx, entry)
	if err != nil {
		return fmt.Errorf("%s: %w", entry.Name, err)
	}

	// 2. Verify repository URL does not redirect.
	if err := c.checkRepoNoRedirect(ctx, entry.Module); err != nil {
		return fmt.Errorf("%s: %w", entry.Name, err)
	}

	// 3. Plan in a scratch module requiring NSGo's latest release and ensure NSGo stays on a release.
	nsgoVer, err := c.latestNSGoRelease(ctx)
	if err != nil {
		return fmt.Errorf("%s: %w", entry.Name, err)
	}
	scratchDir, err := os.MkdirTemp("", "nsgo-regcheck-scratch-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(scratchDir)

	if err := c.planAgainstNSGoRelease(ctx, scratchDir, nsgoVer, entry, dl.Version); err != nil {
		return fmt.Errorf("%s: %w", entry.Name, err)
	}

	// 4. Parse nsgo-module.yaml (if present) and any nsgo-natives.yaml in the module's packages.
	manifest, err := readRegistryCheckModuleManifest(dl.Dir)
	if err != nil {
		return fmt.Errorf("%s (%s@%s): %w", entry.Name, entry.Module, dl.Version, err)
	}
	if err := validateModuleNativeManifests(dl.Dir); err != nil {
		return fmt.Errorf("%s (%s@%s): %w", entry.Name, entry.Module, dl.Version, err)
	}

	// 5 & 6. Library vs component checks.
	if entry.Component == "" {
		if err := c.checkLibrary(ctx, scratchDir, entry, dl, manifest); err != nil {
			return fmt.Errorf("%s: %w", entry.Name, err)
		}
		return nil
	}
	if err := c.checkComponent(ctx, nsgoVer, entry, dl, manifest); err != nil {
		return fmt.Errorf("%s: %w", entry.Name, err)
	}
	return nil
}

func (c *registryChecker) resolveAndVerifyModule(ctx context.Context, entry registryCheckEntry) (*downloadedRegistryModule, error) {
	dir, err := os.MkdirTemp("", "nsgo-regcheck-dl-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	// @upgrade requires a module context; write an empty scratch go.mod so
	// Go resolves @upgrade against GOPROXY without inheriting any caller module.
	probeMod := fmt.Sprintf("module example.com/regcheckdl\n\ngo %s\n", c.goVersion)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(probeMod), 0o644); err != nil {
		return nil, err
	}

	query := entry.Module + "@" + entry.targetQuery()
	cmd := exec.CommandContext(ctx, c.goCmd, "mod", "download", "-json", query)
	cmd.Dir = dir
	cmd.Env = c.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()

	var dl downloadedRegistryModule
	decErr := json.Unmarshal(out, &dl)
	if err != nil {
		if decErr == nil && dl.Error != "" {
			return nil, fmt.Errorf("resolve %s: %s", query, dl.Error)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			return nil, fmt.Errorf("resolve %s: %w\n%s", query, err, msg)
		}
		return nil, fmt.Errorf("resolve %s: %w", query, err)
	}
	if decErr != nil {
		return nil, fmt.Errorf("decode go mod download for %s: %w", query, decErr)
	}
	if dl.Error != "" {
		return nil, fmt.Errorf("resolve %s: %s", query, dl.Error)
	}
	if module.IsPseudoVersion(dl.Version) {
		return nil, fmt.Errorf("%s resolved to pseudo-version %s (no semantic release tag)", query, dl.Version)
	}
	if !isReleaseTag(dl.Version) {
		return nil, fmt.Errorf("%s resolved to %q, which is not a semantic release tag", query, dl.Version)
	}
	if dl.Dir == "" {
		return nil, fmt.Errorf("%s returned no module directory", query)
	}

	if err := verifyDownloadedGoMod(dl.Dir, entry.Module); err != nil {
		return nil, fmt.Errorf("%s@%s: %w", entry.Module, dl.Version, err)
	}
	return &dl, nil
}

func verifyDownloadedGoMod(modDir, wantPath string) error {
	root, err := os.OpenRoot(modDir)
	if err != nil {
		return err
	}
	defer root.Close()

	f, err := root.Open("go.mod")
	if err != nil {
		return fmt.Errorf("read go.mod: %w", err)
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, registryCheckMaxManifest))
	if err != nil {
		return fmt.Errorf("read go.mod: %w", err)
	}
	mf, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return fmt.Errorf("parse go.mod: %w", err)
	}
	if mf.Module == nil {
		return errors.New("go.mod has no module directive")
	}
	if mf.Module.Mod.Path != wantPath {
		return fmt.Errorf("go.mod declares module path %q, want %q", mf.Module.Mod.Path, wantPath)
	}
	return nil
}

func (c *registryChecker) checkRepoNoRedirect(ctx context.Context, modPath string) error {
	urlFn := c.repoURL
	if urlFn == nil {
		urlFn = moduleRepoURL
	}
	rawURL, err := urlFn(modPath)
	if err != nil {
		return err
	}
	client := c.httpClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	// Ensure redirects are never followed even if caller passed a custom client.
	clientCopy := *client
	clientCopy.CheckRedirect = func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return fmt.Errorf("build request for %s: %w", rawURL, err)
	}
	req.Header.Set("User-Agent", "nsgo-registry-check/1")
	resp, err := clientCopy.Do(req)
	if err != nil {
		return fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		loc := resp.Header.Get("Location")
		return fmt.Errorf("repository %s redirected with HTTP %d to %q", rawURL, resp.StatusCode, loc)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("repository %s returned HTTP %d", rawURL, resp.StatusCode)
	}
	return nil
}

func (c *registryChecker) latestNSGoRelease(ctx context.Context) (string, error) {
	if c.nsgoVersion != "" {
		return c.nsgoVersion, nil
	}
	dir, err := os.MkdirTemp("", "nsgo-regcheck-latest-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	gomod := fmt.Sprintf("module example.com/nsgoprobe\n\ngo %s\n", c.goVersion)
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return "", err
	}
	cmd := exec.CommandContext(ctx, c.goCmd, "list", "-m", "-versions", "-json", nsgoModule)
	cmd.Dir = dir
	cmd.Env = c.env
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("list %s versions: %w\n%s", nsgoModule, err, strings.TrimSpace(stderr.String()))
	}
	var listed struct {
		Versions []string
	}
	if err := json.Unmarshal(out, &listed); err != nil {
		return "", fmt.Errorf("decode %s versions: %w", nsgoModule, err)
	}
	best := ""
	for _, v := range listed.Versions {
		if !isReleaseTag(v) {
			continue
		}
		if best == "" || semver.Compare(v, best) > 0 {
			best = v
		}
	}
	if best == "" {
		return "", fmt.Errorf("%s has no released version", nsgoModule)
	}
	c.nsgoVersion = best
	return best, nil
}

func (c *registryChecker) planAgainstNSGoRelease(ctx context.Context, scratchDir, nsgoVer string, entry registryCheckEntry, resolvedVer string) error {
	gomod := fmt.Sprintf("module example.com/registrycheck\n\ngo %s\n\nrequire %s %s\n", c.goVersion, nsgoModule, nsgoVer)
	if err := os.WriteFile(filepath.Join(scratchDir, "go.mod"), []byte(gomod), 0o644); err != nil {
		return err
	}
	m := &addModule{
		dir:     scratchDir,
		cwd:     scratchDir,
		goCmd:   c.goCmd,
		env:     c.env,
		listEnv: c.env,
	}
	req := addRequest{
		path:            entry.Module,
		version:         entry.targetQuery(),
		typed:           entry.Version != "",
		registryLicense: entry.License,
	}
	plan, err := m.plan(ctx, []addRequest{req})
	if err != nil {
		return fmt.Errorf("plan %s@%s against %s %s: %w", entry.Module, resolvedVer, nsgoModule, nsgoVer, err)
	}
	if err := plan.refusal(); err != nil {
		return fmt.Errorf("plan refused for %s@%s: %w", entry.Module, resolvedVer, err)
	}
	nsgoAfter := requiredVersion(plan.after, nsgoModule)
	if !isReleaseTag(nsgoAfter) {
		return fmt.Errorf("adding %s@%s moved %s to %q, which is not a release", entry.Module, resolvedVer, nsgoModule, nsgoAfter)
	}
	if err := plan.apply(ctx); err != nil {
		return fmt.Errorf("apply plan for %s@%s: %w", entry.Module, resolvedVer, err)
	}
	return nil
}

// validateModuleNativeManifests walks Go package directories inside modDir
// (skipping nested modules, vendor, testdata, and hidden/_ directories) and
// validates every nsgo-natives.yaml with readNativeManifest.
func validateModuleNativeManifests(modDir string) error {
	return filepath.WalkDir(modDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			return nil
		}
		if p != modDir {
			name := d.Name()
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return fs.SkipDir
			}
			// Skip nested modules.
			if _, statErr := os.Stat(filepath.Join(p, "go.mod")); statErr == nil {
				return fs.SkipDir
			}
		}
		if _, err := readNativeManifest(p); err != nil {
			rel, relErr := filepath.Rel(modDir, p)
			if relErr != nil {
				rel = p
			}
			return fmt.Errorf("%s/%s: %w", filepath.ToSlash(rel), nativeManifestName, err)
		}
		return nil
	})
}

// validateLinkedNativeManifests lists all linked packages in dir for linux_x64
// and validates every nsgo-natives.yaml in those packages.
func (c *registryChecker) validateLinkedNativeManifests(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, c.goCmd, "list", "-deps", "-json", "-mod=readonly", "./...")
	cmd.Dir = dir
	cmd.Env = mergeEnv(c.env, "GOOS=linux", "GOARCH=amd64", "CGO_ENABLED=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("list linked packages for linux_x64: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	checked := make(map[string]bool)
	dec := json.NewDecoder(bytes.NewReader(out))
	for dec.More() {
		var pkg struct {
			ImportPath string
			Dir        string
			Standard   bool
		}
		if err := dec.Decode(&pkg); err != nil {
			return fmt.Errorf("decode go list -deps: %w", err)
		}
		if pkg.Standard || pkg.Dir == "" || checked[pkg.Dir] {
			continue
		}
		checked[pkg.Dir] = true
		if _, err := readNativeManifest(pkg.Dir); err != nil {
			return fmt.Errorf("linked package %s/%s: %w", pkg.ImportPath, nativeManifestName, err)
		}
	}
	return nil
}

func (c *registryChecker) checkLibrary(ctx context.Context, scratchDir string, entry registryCheckEntry, dl *downloadedRegistryModule, manifest *registryCheckManifest) error {
	var imports []string
	if manifest != nil && manifest.hasImport {
		for _, rel := range manifest.Import {
			if rel == "." {
				imports = append(imports, entry.Module)
			} else {
				imports = append(imports, entry.Module+"/"+rel)
			}
		}
	} else {
		imports = []string{entry.Module}
	}
	if len(imports) == 0 {
		return fmt.Errorf("library %s@%s has no import packages declared", entry.Module, dl.Version)
	}

	var src bytes.Buffer
	src.WriteString("package scratch\n\nimport (\n")
	for _, imp := range imports {
		fmt.Fprintf(&src, "\t_ %q\n", imp)
	}
	src.WriteString(")\n")
	if err := os.WriteFile(filepath.Join(scratchDir, "check.go"), src.Bytes(), 0o644); err != nil {
		return err
	}

	if err := c.goModTidyAndVerifyNSGo(ctx, scratchDir); err != nil {
		return err
	}
	if err := c.validateLinkedNativeManifests(ctx, scratchDir); err != nil {
		return err
	}
	return c.execCheck(ctx, scratchDir)
}

func (c *registryChecker) checkComponent(ctx context.Context, nsgoVer string, entry registryCheckEntry, dl *downloadedRegistryModule, manifest *registryCheckManifest) error {
	compDir := dl.Dir
	if entry.Module == nsgoModule && c.nsgoLocalDir != "" && (manifest == nil || manifest.Components[entry.Component] == "") {
		// Before the next NSGo release is tagged, components added to this
		// repository's nsgo-module.yaml are not yet in the previous published
		// release on proxy.golang.org; validate them against this checkout.
		localManifest, err := readRegistryCheckModuleManifest(c.nsgoLocalDir)
		if err != nil {
			return fmt.Errorf("local %s: %w", registryCheckModuleManifest, err)
		}
		if localManifest != nil && localManifest.Components[entry.Component] != "" {
			manifest = localManifest
			compDir = c.nsgoLocalDir
		}
	}
	if manifest == nil {
		return fmt.Errorf("component %q is not declared: %s@%s has no %s", entry.Component, entry.Module, dl.Version, registryCheckModuleManifest)
	}
	compRel, ok := manifest.Components[entry.Component]
	if !ok || compRel == "" {
		return fmt.Errorf("component %q is not declared in %s@%s %s", entry.Component, entry.Module, dl.Version, registryCheckModuleManifest)
	}

	pkgName, err := validateComponentPackage(compDir, compRel)
	if err != nil {
		return fmt.Errorf("component %q (%s in %s@%s): %w", entry.Component, compRel, entry.Module, dl.Version, err)
	}
	if pkgName != path.Base(compRel) {
		return fmt.Errorf("component %q package name %q does not match directory %q", entry.Component, pkgName, path.Base(compRel))
	}

	// NSGo's own check.yml vets its components in place (AGENTS.md).
	if entry.Module == nsgoModule {
		return nil
	}

	appDir, err := os.MkdirTemp("", "nsgo-regcheck-comp-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(appDir)

	const appMod = "example.com/registrycheckapp"
	destRel := path.Join("components", path.Base(compRel))
	oldPrefix := entry.Module + "/" + compRel
	newPrefix := appMod + "/" + destRel

	if err := copyComponentTree(dl.Dir, compRel, filepath.Join(appDir, filepath.FromSlash(destRel)), oldPrefix, newPrefix); err != nil {
		return fmt.Errorf("copy component %q: %w", entry.Component, err)
	}

	// Build scratch app go.mod requiring NSGo's latest release plus non-NSGo
	// dependencies from the source module's go.mod so MVS resolves copied
	// component imports cleanly.
	appGoMod, err := buildScratchComponentGoMod(appMod, c.goVersion, nsgoVer, dl.Dir)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(appDir, "go.mod"), appGoMod, 0o644); err != nil {
		return err
	}
	useGo := fmt.Sprintf("package scratchapp\n\nimport _ %q\n", newPrefix)
	if err := os.WriteFile(filepath.Join(appDir, "use.go"), []byte(useGo), 0o644); err != nil {
		return err
	}

	if err := c.goModTidyAndVerifyNSGo(ctx, appDir); err != nil {
		return err
	}
	if err := c.validateLinkedNativeManifests(ctx, appDir); err != nil {
		return err
	}
	return c.execCheck(ctx, appDir)
}

func validateComponentPackage(modDir, compRel string) (string, error) {
	// Refuse nested modules along compRel.
	parts := strings.Split(compRel, "/")
	cur := modDir
	for _, seg := range parts {
		cur = filepath.Join(cur, seg)
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return "", fmt.Errorf("component directory %q is inside a nested module", compRel)
		}
	}

	root, err := os.OpenRoot(modDir)
	if err != nil {
		return "", err
	}
	defer root.Close()

	compRoot, err := root.OpenRoot(filepath.FromSlash(compRel))
	if err != nil {
		return "", fmt.Errorf("open component directory %q: %w", compRel, err)
	}
	defer compRoot.Close()

	entries, err := fs.ReadDir(compRoot.FS(), ".")
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	pkgName := ""
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		src, err := fs.ReadFile(compRoot.FS(), e.Name())
		if err != nil {
			return "", err
		}
		f, err := parser.ParseFile(fset, e.Name(), src, parser.PackageClauseOnly)
		if err != nil {
			return "", fmt.Errorf("parse %s: %w", e.Name(), err)
		}
		if f.Name.Name == "main" {
			return "", errors.New("component cannot be a main package")
		}
		if pkgName == "" {
			pkgName = f.Name.Name
		} else if pkgName != f.Name.Name {
			return "", fmt.Errorf("multiple package names (%q and %q)", pkgName, f.Name.Name)
		}
	}
	if pkgName == "" {
		return "", fmt.Errorf("no non-test Go files in %q", compRel)
	}
	return pkgName, nil
}

// copyComponentTree copies a declared component directory from modDir/compRel
// into dstDir through os.OpenRoot and rewrites Go imports starting with
// oldPrefix to newPrefix.
func copyComponentTree(modDir, compRel, dstDir, oldPrefix, newPrefix string) error {
	root, err := os.OpenRoot(modDir)
	if err != nil {
		return err
	}
	defer root.Close()

	compRoot, err := root.OpenRoot(filepath.FromSlash(compRel))
	if err != nil {
		return err
	}
	defer compRoot.Close()

	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	return fs.WalkDir(compRoot.FS(), ".", func(rel string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
				return fs.SkipDir
			}
			return os.MkdirAll(filepath.Join(dstDir, filepath.FromSlash(rel)), 0o755)
		}
		if !d.Type().IsRegular() {
			return fmt.Errorf("component file %q is not a regular file", rel)
		}
		if name == nativeManifestName || strings.HasSuffix(name, ".syso") || name == "go.mod" || name == "go.sum" {
			return fmt.Errorf("component contains disallowed file %q", rel)
		}
		data, err := fs.ReadFile(compRoot.FS(), rel)
		if err != nil {
			return err
		}
		if strings.HasSuffix(name, ".go") {
			data, err = rewriteComponentImports(rel, data, oldPrefix, newPrefix)
			if err != nil {
				return err
			}
		}
		return os.WriteFile(filepath.Join(dstDir, filepath.FromSlash(rel)), data, 0o644)
	})
}

func rewriteComponentImports(filename string, src []byte, oldPrefix, newPrefix string) ([]byte, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filename, err)
	}
	changed := false
	for _, imp := range file.Imports {
		val, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		if val == oldPrefix || strings.HasPrefix(val, oldPrefix+"/") {
			rewritten := newPrefix + strings.TrimPrefix(val, oldPrefix)
			imp.Path.Value = strconv.Quote(rewritten)
			changed = true
		}
	}
	if !changed {
		return src, nil
	}
	ast.SortImports(fset, file)
	var buf bytes.Buffer
	if err := format.Node(&buf, fset, file); err != nil {
		return nil, fmt.Errorf("format %s: %w", filename, err)
	}
	return buf.Bytes(), nil
}

func buildScratchComponentGoMod(appMod, goVer, nsgoVer, srcModDir string) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "module %s\n\ngo %s\n\nrequire (\n\t%s %s\n", appMod, goVer, nsgoModule, nsgoVer)
	if data, err := os.ReadFile(filepath.Join(srcModDir, "go.mod")); err == nil {
		if mf, err := modfile.ParseLax("go.mod", data, nil); err == nil {
			for _, req := range mf.Require {
				if req.Mod.Path == nsgoModule {
					continue
				}
				fmt.Fprintf(&b, "\t%s %s\n", req.Mod.Path, req.Mod.Version)
			}
		}
	}
	b.WriteString(")\n")
	return b.Bytes(), nil
}

func (c *registryChecker) goModTidyAndVerifyNSGo(ctx context.Context, dir string) error {
	cmd := exec.CommandContext(ctx, c.goCmd, "mod", "tidy")
	cmd.Dir = dir
	cmd.Env = c.env
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go mod tidy: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if err != nil {
		return err
	}
	mf, err := modfile.Parse("go.mod", data, nil)
	if err != nil {
		return err
	}
	if v := requiredVersion(mf, nsgoModule); v != "" && !isReleaseTag(v) {
		return fmt.Errorf("after go mod tidy, %s resolved to %q (not a release)", nsgoModule, v)
	}
	return nil
}

func (c *registryChecker) execCheck(ctx context.Context, dir string) error {
	fn := c.runCheck
	if fn == nil {
		fn = runNsgoCheckLinuxX64
	}
	return fn(ctx, dir, c.env)
}

// runNsgoCheckLinuxX64 executes this repository's `nsgo check -target linux_x64`
// against dir via TestCLIProcess with delegation disabled, so offline tests
// using a stub NSGo proxy and online runs share the exact same check path.
func runNsgoCheckLinuxX64(ctx context.Context, dir string, env []string) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestCLIProcess$", "--", "check", "-target", "linux_x64", dir)
	cmd.Dir = dir
	filtered := make([]string, 0, len(env)+4)
	for _, kv := range env {
		k, _, _ := strings.Cut(kv, "=")
		if slices.Contains([]string{"NSGO_EXTENSION", "NSGO_DELEGATED", "NSGO_DELEGATED_FROM", "NSGO_NO_DELEGATE", "NSGO_CLI_PROCESS"}, k) {
			continue
		}
		filtered = append(filtered, kv)
	}
	cmd.Env = append(filtered, "NSGO_CLI_PROCESS=1", "NSGO_NO_DELEGATE=1", "GOWORK=off", "GOTOOLCHAIN=local")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("nsgo check -target linux_x64 failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// TestRegistryOnline verifies every entry in cmd/nsgo/registry.yaml against
// live module proxies and repositories. It skips unless NSGO_REGISTRY_ONLINE=1
// so ordinary offline test runs never touch the network (#2472).
func TestRegistryOnline(t *testing.T) {
	if os.Getenv(registryCheckEnvVar) != "1" {
		t.Skipf("set %s=1 to check registry entries online", registryCheckEnvVar)
	}
	idx, exists, err := loadRegistryCheckFile(registryCheckIndexFile)
	if err != nil {
		t.Fatalf("load %s: %v", registryCheckIndexFile, err)
	}
	if !exists {
		t.Logf("%s is not present yet; nothing to check", registryCheckIndexFile)
		return
	}
	if len(idx.Items) == 0 {
		t.Logf("%s lists no items; nothing to check", registryCheckIndexFile)
		return
	}
	checker, err := newOnlineRegistryChecker(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range idx.Items {
		t.Run(item.Name, func(t *testing.T) {
			if err := checker.checkEntry(t.Context(), item); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// --- Offline unit tests ---

// addZipProxyModule adds modPath@version to a local file:// module proxy using
// golang.org/x/mod/zip.CreateFromDir. If proxyMod is non-empty, it is written
// as the proxy's .mod file while zipGoMod is placed inside the zip's go.mod
// (allowing tests to exercise go.mod path mismatches).
func addZipProxyModule(t *testing.T, proxyDir, modPath, version, zipGoMod, proxyMod string, files map[string]string) {
	t.Helper()
	if proxyMod == "" {
		proxyMod = zipGoMod
	}
	srcDir := t.TempDir()
	allFiles := maps.Clone(files)
	if allFiles == nil {
		allFiles = map[string]string{}
	}
	allFiles["go.mod"] = zipGoMod
	if _, ok := allFiles["LICENSE"]; !ok {
		allFiles["LICENSE"] = licenseFixtureText(t, "MIT")
	}
	writeFiles(t, srcDir, allFiles)

	escPath, err := module.EscapePath(modPath)
	if err != nil {
		t.Fatalf("escape path %q: %v", modPath, err)
	}
	escVer, err := module.EscapeVersion(version)
	if err != nil {
		t.Fatalf("escape version %q: %v", version, err)
	}
	vDir := filepath.Join(proxyDir, filepath.FromSlash(escPath), "@v")
	if err := os.MkdirAll(vDir, 0o755); err != nil {
		t.Fatal(err)
	}

	info := fmt.Sprintf(`{"Version":%q,"Time":"2026-11-01T00:00:00Z"}`, version)
	writeFiles(t, vDir, map[string]string{
		escVer + ".info": info,
		escVer + ".mod":  proxyMod,
	})

	zipPath := filepath.Join(vDir, escVer+".zip")
	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zipErr := modzip.CreateFromDir(zf, module.Version{Path: modPath, Version: version}, srcDir)
	closeErr := zf.Close()
	if err := errors.Join(zipErr, closeErr); err != nil {
		t.Fatalf("CreateFromDir %s@%s: %v", modPath, version, err)
	}

	listPath := filepath.Join(vDir, "list")
	if module.IsPseudoVersion(version) {
		writeFiles(t, filepath.Dir(vDir), map[string]string{"@latest": info})
		if _, err := os.Stat(listPath); errors.Is(err, fs.ErrNotExist) {
			if err := os.WriteFile(listPath, nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return
	}
	f, err := os.OpenFile(listPath, os.O_WRONLY|os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	_, wErr := f.WriteString(version + "\n")
	if err := errors.Join(wErr, f.Close()); err != nil {
		t.Fatal(err)
	}
}

// newOfflineRegistryChecker builds a registryChecker wired to proxyDir (file://)
// and repoServer (httptest.Server).
func newOfflineRegistryChecker(t *testing.T, proxyDir string, repoServer *httptest.Server) *registryChecker {
	t.Helper()
	goCmd, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go is not on PATH")
	}
	env := mergeEnv(os.Environ(),
		"GO111MODULE=on",
		"GOPROXY=file://"+filepath.ToSlash(proxyDir),
		"GOSUMDB=off",
		"GOTOOLCHAIN=local",
		"GOWORK=off",
		"GOMODCACHE="+t.TempDir(),
		"GOFLAGS=-modcacherw",
		"GOPRIVATE=",
		"GONOPROXY=",
		"GONOSUMDB=",
	)
	return &registryChecker{
		goCmd:      goCmd,
		env:        env,
		goVersion:  repoGoVersion(),
		httpClient: repoServer.Client(),
		repoURL: func(modPath string) (string, error) {
			canonical, err := moduleRepoURL(modPath)
			if err != nil {
				return "", err
			}
			u, err := url.Parse(canonical)
			if err != nil {
				return "", err
			}
			// Route https://<host><path> to the local httptest.Server preserving host/path/query.
			target := repoServer.URL + "/" + u.Host + u.Path
			if u.RawQuery != "" {
				target += "?" + u.RawQuery
			}
			return target, nil
		},
		runCheck: runNsgoCheckLinuxX64,
	}
}

func seedOfflineNSGoProxy(t *testing.T, proxyDir string, nsgoManifest string) {
	t.Helper()
	goVer := repoGoVersion()
	nsgoMod := fmt.Sprintf("module %s\n\ngo %s\n", nsgoModule, goVer)
	baseFiles := map[string]string{
		"nsgo.go":         "package nsgo\n",
		"core/core.go":    "package core\n\ntype Node any\n",
		"ui/chat/chat.go": "package chat\n\nimport \"github.com/nsgo-dev/nsgo/core\"\n\nfunc View() core.Node { return nil }\n",
	}
	if nsgoManifest != "" {
		baseFiles[registryCheckModuleManifest] = nsgoManifest
	}
	addZipProxyModule(t, proxyDir, nsgoModule, "v0.3.4", nsgoMod, "", baseFiles)
	addZipProxyModule(t, proxyDir, nsgoModule, "v0.4.0", nsgoMod, "", baseFiles)
	addZipProxyModule(t, proxyDir, nsgoModule, "v0.4.1-0.20261125111955-8d716d9b939c", nsgoMod, "", baseFiles)
	addZipProxyModule(t, proxyDir, nsgoModule, "v0.5.0-rc.1", nsgoMod, "", baseFiles)
}

func okRepoServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestRegistryOnlineSkipsWhenUnset verifies TestRegistryOnline does nothing and
// skips cleanly when NSGO_REGISTRY_ONLINE is unset (#2472).
func TestRegistryOnlineSkipsWhenUnset(t *testing.T) {
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(t.Context(), exe, "-test.v", "-test.run=^TestRegistryOnline$")
	cmd.Env = mergeEnv(os.Environ(), registryCheckEnvVar+"=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("TestRegistryOnline with %s unset failed: %v\n%s", registryCheckEnvVar, err, out)
	}
	if !strings.Contains(string(out), "--- SKIP: TestRegistryOnline") {
		t.Fatalf("expected TestRegistryOnline to skip when %s is unset; output:\n%s", registryCheckEnvVar, out)
	}
}

func TestRegistryCheckModuleRepoURL(t *testing.T) {
	cases := map[string]string{
		"github.com/oneo-me/nsgo-media":          "https://github.com/oneo-me/nsgo-media",
		"github.com/nsgo-dev/nsgo/v2":            "https://github.com/nsgo-dev/nsgo",
		"github.com/nsgo-dev/game/packages/game": "https://github.com/nsgo-dev/game",
		"gitlab.com/group/sub/project":           "https://gitlab.com/group/sub/project",
		"gitlab.com/group/sub/project/v3":        "https://gitlab.com/group/sub/project",
		"codeberg.org/acme/widget":               "https://codeberg.org/acme/widget",
		"bitbucket.org/acme/widget/v2":           "https://bitbucket.org/acme/widget",
		"gitee.com/acme/widget":                  "https://gitee.com/acme/widget",
		"git.sr.ht/~alice/widget":                "https://git.sr.ht/~alice/widget",
		"example.com/chart":                      "https://example.com/chart?go-get=1",
		"nsgo.acme.example/libraries/chart/v2":   "https://nsgo.acme.example/libraries/chart/v2?go-get=1",
		"golang.org/x/mod":                       "https://golang.org/x/mod?go-get=1",
	}
	for modPath, want := range cases {
		got, err := moduleRepoURL(modPath)
		if err != nil {
			t.Errorf("moduleRepoURL(%q) error: %v", modPath, err)
			continue
		}
		if got != want {
			t.Errorf("moduleRepoURL(%q) = %q, want %q", modPath, got, want)
		}
	}
}

func TestRegistryCheckRepoRedirects(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/github.com/nsgo-dev/nsgo-media":
			w.WriteHeader(http.StatusOK)
		case "/github.com/oneo-me/nsgo-media":
			http.Redirect(w, r, "https://github.com/nsgo-dev/nsgo-media", http.StatusMovedPermanently)
		case "/github.com/acme/found302":
			http.Redirect(w, r, "https://github.com/other/found302", http.StatusFound)
		case "/github.com/acme/temp307":
			http.Redirect(w, r, "https://github.com/other/temp307", http.StatusTemporaryRedirect)
		case "/github.com/acme/perm308":
			http.Redirect(w, r, "https://github.com/other/perm308", http.StatusPermanentRedirect)
		case "/github.com/acme/gone":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	checker := newOfflineRegistryChecker(t, t.TempDir(), srv)

	if err := checker.checkRepoNoRedirect(t.Context(), "github.com/nsgo-dev/nsgo-media"); err != nil {
		t.Fatalf("200 OK repo failed: %v", err)
	}

	for _, tc := range []struct {
		modPath string
		want    string
	}{
		{"github.com/oneo-me/nsgo-media", "HTTP 301"},
		{"github.com/acme/found302", "HTTP 302"},
		{"github.com/acme/temp307", "HTTP 307"},
		{"github.com/acme/perm308", "HTTP 308"},
		{"github.com/acme/gone", "HTTP 404"},
	} {
		err := checker.checkRepoNoRedirect(t.Context(), tc.modPath)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("checkRepoNoRedirect(%q) = %v, want error containing %q", tc.modPath, err, tc.want)
		}
	}
}

func TestRegistryCheckValidLibrary(t *testing.T) {
	proxy := t.TempDir()
	seedOfflineNSGoProxy(t, proxy, "")
	goVer := repoGoVersion()

	validNatives := `version: 2
libraries:
  linux_x64:
    - name: libchart.so.1
      url: https://example.com/libchart.so.1
      sha256: 0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef
      licenses:
        - LICENSE
linux:
  linux_x64:
    depends:
      deb: [libchart1]
`
	// Library with nsgo-module.yaml (declaring root + subpackage) and valid nsgo-natives.yaml.
	addZipProxyModule(t, proxy, "github.com/acme/chart", "v1.2.0",
		fmt.Sprintf("module github.com/acme/chart\n\ngo %s\n\nrequire github.com/nsgo-dev/nsgo v0.3.4\n", goVer),
		"",
		map[string]string{
			"nsgo-module.yaml":      "version: 1\nimport:\n  - .\n  - sub\ntargets:\n  - macos_arm64\n  - linux_x64\n  - windows_x64\nnotes: Works on desktop targets.\n",
			"chart.go":              "package chart\n\nimport \"github.com/nsgo-dev/nsgo/core\"\n\nfunc Node() core.Node { return nil }\n",
			"sub/sub.go":            "package sub\n\nfunc Name() string { return \"sub\" }\n",
			"sub/LICENSE":           licenseFixtureText(t, "MIT"),
			"sub/nsgo-natives.yaml": validNatives,
		},
	)

	// Library without nsgo-module.yaml (falls back to importing root package).
	addZipProxyModule(t, proxy, "github.com/acme/plain", "v1.0.0",
		fmt.Sprintf("module github.com/acme/plain\n\ngo %s\n\nrequire github.com/nsgo-dev/nsgo v0.4.0\n", goVer),
		"",
		map[string]string{
			"plain.go": "package plain\n\nimport \"github.com/nsgo-dev/nsgo/core\"\n\nfunc Value() core.Node { return nil }\n",
		},
	)

	checker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))

	for _, entry := range []registryCheckEntry{
		{Name: "chart", Module: "github.com/acme/chart", Description: "Charts", License: "MIT"},
		{Name: "plain", Module: "github.com/acme/plain", Description: "Plain", License: "MIT"},
	} {
		if err := checker.checkEntry(t.Context(), entry); err != nil {
			t.Errorf("valid library %q failed: %v", entry.Name, err)
		}
	}
}

func TestRegistryCheckRefusesPseudoVersionAndPrerelease(t *testing.T) {
	proxy := t.TempDir()
	seedOfflineNSGoProxy(t, proxy, "")
	goVer := repoGoVersion()

	addZipProxyModule(t, proxy, "github.com/acme/untagged", "v0.0.0-20261101000000-abcdef123456",
		fmt.Sprintf("module github.com/acme/untagged\n\ngo %s\n", goVer),
		"",
		map[string]string{"untagged.go": "package untagged\n"},
	)
	addZipProxyModule(t, proxy, "github.com/acme/prerelease", "v1.0.0-rc.1",
		fmt.Sprintf("module github.com/acme/prerelease\n\ngo %s\n", goVer),
		"",
		map[string]string{"pre.go": "package prerelease\n"},
	)

	checker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))

	err := checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "untagged", Module: "github.com/acme/untagged", Description: "No tag", License: "MIT",
	})
	if err == nil || !strings.Contains(err.Error(), "pseudo-version") {
		t.Errorf("pseudo-version entry = %v, want pseudo-version failure", err)
	}

	err = checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "prerelease", Module: "github.com/acme/prerelease", Description: "Pre-release", License: "MIT",
	})
	if err == nil || !strings.Contains(err.Error(), "not a semantic release tag") {
		t.Errorf("pre-release entry = %v, want semantic release tag failure", err)
	}
}

func TestRegistryCheckRefusesModulePathMismatch(t *testing.T) {
	proxy := t.TempDir()
	seedOfflineNSGoProxy(t, proxy, "")
	goVer := repoGoVersion()

	// Case 1: proxy .mod and zip go.mod both declare a different path (normal proxy behavior).
	addZipProxyModule(t, proxy, "github.com/acme/mismatch-both", "v1.0.0",
		fmt.Sprintf("module github.com/other/mismatch-both\n\ngo %s\n", goVer),
		"",
		map[string]string{"pkg.go": "package mismatch\n"},
	)
	// Case 2: proxy .mod matches, but go.mod inside the zip declares a different path.
	addZipProxyModule(t, proxy, "github.com/acme/mismatch-zip", "v1.0.0",
		fmt.Sprintf("module github.com/other/mismatch-zip\n\ngo %s\n", goVer),
		fmt.Sprintf("module github.com/acme/mismatch-zip\n\ngo %s\n", goVer),
		map[string]string{"pkg.go": "package mismatch\n"},
	)

	checker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))

	if err := checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "mismatch-both", Module: "github.com/acme/mismatch-both", Description: "Wrong mod", License: "MIT",
	}); err == nil {
		t.Errorf("mismatch-both unexpectedly passed")
	}

	err := checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "mismatch-zip", Module: "github.com/acme/mismatch-zip", Description: "Wrong zip mod", License: "MIT",
	})
	if err == nil || !strings.Contains(err.Error(), "declares module path") {
		t.Errorf("mismatch-zip = %v, want go.mod module path mismatch error", err)
	}
}

func TestRegistryCheckRefusesNSGoOffReleases(t *testing.T) {
	proxy := t.TempDir()
	seedOfflineNSGoProxy(t, proxy, "")
	goVer := repoGoVersion()

	addZipProxyModule(t, proxy, "github.com/acme/media-pseudo", "v0.1.0",
		fmt.Sprintf("module github.com/acme/media-pseudo\n\ngo %s\n\nrequire %s v0.4.1-0.20261125111955-8d716d9b939c\n", goVer, nsgoModule),
		"",
		map[string]string{"media.go": "package media\n"},
	)
	addZipProxyModule(t, proxy, "github.com/acme/media-pre", "v0.1.0",
		fmt.Sprintf("module github.com/acme/media-pre\n\ngo %s\n\nrequire %s v0.5.0-rc.1\n", goVer, nsgoModule),
		"",
		map[string]string{"media.go": "package media\n"},
	)

	checker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))

	for _, tc := range []struct {
		name   string
		module string
		want   string
	}{
		{"media-pseudo", "github.com/acme/media-pseudo", "a commit with no release"},
		{"media-pre", "github.com/acme/media-pre", "a pre-release"},
	} {
		err := checker.checkEntry(t.Context(), registryCheckEntry{
			Name: tc.name, Module: tc.module, Description: "Moves NSGo off release", License: "MIT",
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want substring %q", tc.name, err, tc.want)
		}
	}
}

func TestRegistryCheckRefusesInvalidManifests(t *testing.T) {
	proxy := t.TempDir()
	seedOfflineNSGoProxy(t, proxy, "")
	goVer := repoGoVersion()

	// Invalid nsgo-module.yaml (unknown field).
	addZipProxyModule(t, proxy, "github.com/acme/bad-modmanifest", "v1.0.0",
		fmt.Sprintf("module github.com/acme/bad-modmanifest\n\ngo %s\n", goVer),
		"",
		map[string]string{
			"nsgo-module.yaml": "version: 1\nunknown_key: true\n",
			"pkg.go":           "package bad\n",
		},
	)

	// Invalid nsgo-module.yaml (YAML anchor).
	addZipProxyModule(t, proxy, "github.com/acme/anchor-modmanifest", "v1.0.0",
		fmt.Sprintf("module github.com/acme/anchor-modmanifest\n\ngo %s\n", goVer),
		"",
		map[string]string{
			"nsgo-module.yaml": "version: 1\nimport: &a\n  - .\n",
			"pkg.go":           "package bad\n",
		},
	)

	// Invalid nsgo-module.yaml (unknown target).
	addZipProxyModule(t, proxy, "github.com/acme/bad-target", "v1.0.0",
		fmt.Sprintf("module github.com/acme/bad-target\n\ngo %s\n", goVer),
		"",
		map[string]string{
			"nsgo-module.yaml": "version: 1\ntargets:\n  - not_a_target\n",
			"pkg.go":           "package bad\n",
		},
	)

	// Invalid nsgo-natives.yaml (bad sha256) in a linked subpackage.
	addZipProxyModule(t, proxy, "github.com/acme/bad-natives", "v1.0.0",
		fmt.Sprintf("module github.com/acme/bad-natives\n\ngo %s\n", goVer),
		"",
		map[string]string{
			"pkg.go":                   "package bad\n\nimport _ \"github.com/acme/bad-natives/native\"\n",
			"native/native.go":         "package native\n",
			"native/nsgo-natives.yaml": "version: 2\nlibraries:\n  linux_x64:\n    - name: libbad.so\n      url: https://example.com/libbad.so\n      sha256: not-a-sha\n      licenses: [LICENSE]\n",
		},
	)

	checker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))

	for _, tc := range []struct {
		name   string
		module string
		want   string
	}{
		{"bad-modmanifest", "github.com/acme/bad-modmanifest", "nsgo-module.yaml"},
		{"anchor-modmanifest", "github.com/acme/anchor-modmanifest", "anchors are not allowed"},
		{"bad-target", "github.com/acme/bad-target", "unknown target"},
		{"bad-natives", "github.com/acme/bad-natives", "nsgo-natives.yaml"},
	} {
		err := checker.checkEntry(t.Context(), registryCheckEntry{
			Name: tc.name, Module: tc.module, Description: "Invalid manifest", License: "MIT",
		})
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("%s: err = %v, want substring %q", tc.name, err, tc.want)
		}
	}
}

func TestRegistryCheckLibraryCompileFailure(t *testing.T) {
	proxy := t.TempDir()
	seedOfflineNSGoProxy(t, proxy, "")
	goVer := repoGoVersion()

	// Library that fails to compile on linux_x64.
	addZipProxyModule(t, proxy, "github.com/acme/brokenlib", "v1.0.0",
		fmt.Sprintf("module github.com/acme/brokenlib\n\ngo %s\n\nrequire %s v0.4.0\n", goVer, nsgoModule),
		"",
		map[string]string{
			"broken.go": "package brokenlib\n\nfunc Broken() int { return undefinedSymbol }\n",
		},
	)

	checker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))
	err := checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "brokenlib", Module: "github.com/acme/brokenlib", Description: "Broken library", License: "MIT",
	})
	if err == nil || !strings.Contains(err.Error(), "nsgo check -target linux_x64 failed") {
		t.Errorf("broken library = %v, want linux_x64 check failure", err)
	}
}

func TestRegistryCheckComponents(t *testing.T) {
	proxy := t.TempDir()
	seedOfflineNSGoProxy(t, proxy, "version: 1\ncomponents:\n  chat: ui/chat\n")
	goVer := repoGoVersion()

	// 1. Third-party fixture component that copies, rewrites internal subpackage imports, and passes linux_x64 check.
	addZipProxyModule(t, proxy, "github.com/acme/widgets", "v1.0.0",
		fmt.Sprintf("module github.com/acme/widgets\n\ngo %s\n\nrequire %s v0.4.0\n", goVer, nsgoModule),
		"",
		map[string]string{
			"nsgo-module.yaml":         "version: 1\ncomponents:\n  badge: badge\n",
			"badge/badge.go":           "package badge\n\nimport (\n\t\"github.com/acme/widgets/badge/locales\"\n\t\"github.com/nsgo-dev/nsgo/core\"\n)\n\nfunc View() (core.Node, string) { return nil, locales.Title() }\n",
			"badge/locales/locales.go": "package locales\n\nfunc Title() string { return \"Badge\" }\n",
		},
	)

	// 2. Fixture component declared in v1.0.0, dropped in its next release v1.1.0 (@upgrade must fail!).
	addZipProxyModule(t, proxy, "github.com/acme/dropped", "v1.0.0",
		fmt.Sprintf("module github.com/acme/dropped\n\ngo %s\n\nrequire %s v0.4.0\n", goVer, nsgoModule),
		"",
		map[string]string{
			"nsgo-module.yaml": "version: 1\ncomponents:\n  pill: pill\n",
			"pill/pill.go":     "package pill\n\nfunc Label() string { return \"pill\" }\n",
		},
	)
	addZipProxyModule(t, proxy, "github.com/acme/dropped", "v1.1.0",
		fmt.Sprintf("module github.com/acme/dropped\n\ngo %s\n\nrequire %s v0.4.0\n", goVer, nsgoModule),
		"",
		map[string]string{
			"nsgo-module.yaml": "version: 1\ncomponents: {}\n",
			"pill/pill.go":     "package pill\n\nfunc Label() string { return \"pill\" }\n",
		},
	)

	// 3. Third-party fixture component that declares a broken component failing linux_x64 check.
	addZipProxyModule(t, proxy, "github.com/acme/brokencomp", "v1.0.0",
		fmt.Sprintf("module github.com/acme/brokencomp\n\ngo %s\n\nrequire %s v0.4.0\n", goVer, nsgoModule),
		"",
		map[string]string{
			"nsgo-module.yaml": "version: 1\ncomponents:\n  card: card\n",
			"card/card.go":     "package card\n\nfunc Broken() int { return doesNotExist }\n",
		},
	)

	checker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))

	// Third-party component that copies and checks passes.
	if err := checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "badge", Module: "github.com/acme/widgets", Component: "badge", Description: "Badge component", License: "MIT",
	}); err != nil {
		t.Fatalf("valid third-party component failed: %v", err)
	}

	// Component dropped in its next release fails at @upgrade.
	err := checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "pill", Module: "github.com/acme/dropped", Component: "pill", Description: "Dropped component", License: "MIT",
	})
	if err == nil || !strings.Contains(err.Error(), `component "pill" is not declared`) || !strings.Contains(err.Error(), "v1.1.0") {
		t.Errorf("dropped component = %v, want not declared error at v1.1.0", err)
	}

	// Broken third-party component fails linux_x64 check after copy.
	err = checker.checkEntry(t.Context(), registryCheckEntry{
		Name: "card", Module: "github.com/acme/brokencomp", Component: "card", Description: "Broken component", License: "MIT",
	})
	if err == nil || !strings.Contains(err.Error(), "nsgo check -target linux_x64 failed") {
		t.Errorf("broken component = %v, want linux_x64 check failure", err)
	}

	// NSGo's own component verifies declaration at @upgrade without copying or invoking runCheck.
	nsgoCheckCalled := false
	nsgoChecker := newOfflineRegistryChecker(t, proxy, okRepoServer(t))
	nsgoChecker.runCheck = func(ctx context.Context, dir string, env []string) error {
		nsgoCheckCalled = true
		return errors.New("runCheck must not be called for NSGo's own components")
	}
	if err := nsgoChecker.checkEntry(t.Context(), registryCheckEntry{
		Name: "chat", Module: nsgoModule, Component: "chat", Description: "Chat component", License: "Apache-2.0",
	}); err != nil {
		t.Fatalf("NSGo component failed: %v", err)
	}
	if nsgoCheckCalled {
		t.Error("runCheck was called for NSGo's own component, want check.yml in-place exemption")
	}

	// Undeclared NSGo component still fails.
	err = nsgoChecker.checkEntry(t.Context(), registryCheckEntry{
		Name: "missing-comp", Module: nsgoModule, Component: "missing-comp", Description: "Missing NSGo component", License: "Apache-2.0",
	})
	if err == nil || !strings.Contains(err.Error(), "is not declared") {
		t.Errorf("undeclared NSGo component = %v, want not declared error", err)
	}
}
