package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/google/licenseclassifier/v2/assets"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

const noticesName = "THIRD_PARTY_NOTICES.txt"

type licensePolicy struct {
	Unknown  string   `yaml:"unknown"`
	Copyleft string   `yaml:"copyleft"`
	Allow    []string `yaml:"allow"`
}

func (policy licensePolicy) validate() error {
	for _, rule := range []struct{ name, value string }{
		{"unknown", policy.Unknown}, {"copyleft", policy.Copyleft},
	} {
		switch rule.value {
		case "", "error", "warn":
		default:
			return fmt.Errorf("licenses.%s is %q; use error or warn", rule.name, rule.value)
		}
	}
	for _, module := range policy.Allow {
		if module == "" || len(strings.Fields(module)) != 1 || strings.TrimSpace(module) != module {
			return fmt.Errorf("licenses.allow needs exact module paths, got %q", module)
		}
	}
	return nil
}

type licenseModule struct {
	Path, Version, Dir string
	Main               bool
	Replace            *licenseModule
}

type licensePackage struct {
	ImportPath, Dir, Root string
	DepOnly, Standard     bool
	Module                *licenseModule
	EmbedFiles            []string
}

type licenseSource struct {
	name, label, root string
	firstParty        bool
	dirs              map[string]bool
	files             map[string]bool // explicit native license paths; true for primary texts
}

var licenseClassifier = sync.OnceValues(assets.DefaultClassifier)

func (p *project) licenseNoticesFile(ctx context.Context, env, tags, flags []string, overlay string) (string, error) {
	sources := map[string]*licenseSource{}
	p.nativeLibraries = map[string][]nativeLibrary{}
	var hasCore bool
	architectures := p.noticeArchitectures
	if len(architectures) == 0 {
		architectures = []string{""}
	}
	for _, arch := range architectures {
		targetEnv := slices.Clone(env)
		if arch != "" {
			targetEnv = append(targetEnv, "GOARCH="+arch)
		}
		core, err := p.listLicenseSources(ctx, targetEnv, tags, flags, overlay, sources)
		if err != nil {
			return "", err
		}
		hasCore = hasCore || core
	}
	notices, err := p.renderLicenseNotices(ctx, sources, env)
	if err != nil {
		return "", err
	}
	p.thirdPartyNotices = notices
	if !hasCore {
		return "", nil
	}
	source := fmt.Sprintf("// Written by nsgo: the app's third-party notices.\n\npackage main\n\nimport %q\n\nfunc init() { core.SetBundledThirdPartyNotices(%q) }\n",
		nsgoModule+"/core", notices)
	return p.cacheFile("notices", ".go", source)
}

func (p *project) listLicenseSources(ctx context.Context, env, tags, flags []string, overlay string, sources map[string]*licenseSource) (bool, error) {
	args := append([]string{"list", "-deps", "-json=ImportPath,Dir,Root,DepOnly,Standard,Module,EmbedFiles"}, flags...)
	if p.testBinary {
		args = append(args, "-test")
	}
	args = append(args, tagsArgs(tags)...)
	if overlay != "" {
		args = append(args, "-overlay", overlay)
	}
	cmd := exec.CommandContext(ctx, "go", append(args, ".")...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return false, fmt.Errorf("list linked packages for license notices: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}

	var hasCore bool
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var pkg licensePackage
		if err := dec.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				return hasCore, nil
			}
			return false, fmt.Errorf("decode linked packages for license notices: %w", err)
		}
		if pkg.Standard && pkg.Root != "" {
			if sources["Go"] == nil {
				label := "Go runtime and standard library"
				if version, err := os.ReadFile(filepath.Join(pkg.Root, "VERSION")); err == nil {
					line, _, _ := strings.Cut(string(version), "\n")
					label += " (" + line + ")"
				}
				root := pkg.Root
				// Homebrew installs Go's LICENSE beside libexec, its GOROOT.
				if filepath.Base(root) == "libexec" {
					parent := filepath.Dir(root)
					if data, err := os.ReadFile(filepath.Join(parent, "LICENSE")); err == nil && bytes.Contains(data, []byte("The Go Authors")) {
						root = parent
					}
				}
				sources["Go"] = &licenseSource{name: "Go", label: label, root: root, dirs: map[string]bool{root: true, pkg.Root: true}}
			}
			if strings.HasPrefix(pkg.ImportPath, "vendor/") {
				sources["Go"].addDirs(pkg.Dir)
			}
			continue
		}
		if pkg.Module == nil {
			continue
		}
		if pkg.ImportPath == nsgoModule+"/core" {
			hasCore = true
		}
		addLicensePackage(sources, pkg)
		if err := p.addPackageNatives(pkg, nativeTarget(env), sources); err != nil {
			return false, err
		}
	}
}

func addLicensePackage(sources map[string]*licenseSource, pkg licensePackage) {
	module := pkg.Module
	source := sources[module.Path]
	if source == nil {
		label, root := moduleLabel(module), module.Dir
		if module.Replace != nil {
			root = module.Replace.Dir
			if module.Replace.Version == "" {
				label += " (local replacement)"
			} else {
				label += " => " + moduleLabel(module.Replace)
			}
		}
		rel, relErr := filepath.Rel(root, pkg.Dir)
		if root == "" || relErr != nil || !filepath.IsLocal(rel) {
			// A vendored package can still report its original replacement's
			// Module.Dir. Its selected files and notices belong to vendor.
			root = pkg.Dir
			if rel, ok := strings.CutPrefix(pkg.ImportPath, module.Path+"/"); ok {
				for range strings.SplitSeq(rel, "/") {
					root = filepath.Dir(root)
				}
			}
		}
		source = &licenseSource{name: module.Path, label: label, root: root, dirs: map[string]bool{root: true}}
		sources[module.Path] = source
	}
	source.firstParty = source.firstParty || !pkg.DepOnly || module.Main
	source.addDirs(pkg.Dir)
	for _, file := range pkg.EmbedFiles {
		source.addDirs(filepath.Join(pkg.Dir, filepath.Dir(file)))
	}
}

func moduleLabel(module *licenseModule) string {
	if module.Version == "" {
		return module.Path
	}
	return module.Path + " " + module.Version
}

func (source *licenseSource) addDirs(dir string) {
	rel, err := filepath.Rel(source.root, dir)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return
	}
	for dir != source.root && !source.dirs[dir] {
		source.dirs[dir] = true
		dir = filepath.Dir(dir)
	}
}

func (p *project) renderLicenseNotices(ctx context.Context, sources map[string]*licenseSource, env []string) (string, error) {
	classifier, err := licenseClassifier()
	if err != nil {
		return "", err
	}
	var out strings.Builder
	out.WriteString("Third-party license notices\n")
	for _, setting := range env {
		switch setting {
		case "GOOS=android":
			if sources[nsgoModule] != nil {
				out.WriteString("NSGo's license below covers its own Android Java classes.\n")
				if !p.Build.embed {
					notices, err := androidReviewNotices(androidruntime.ClassesJar)
					if err != nil {
						return "", err
					}
					out.WriteString(notices)
				}
			}
		case "GOOS=js":
			out.WriteString("The Go license below covers wasm_exec.js.\n")
			if sources[nsgoModule] != nil {
				out.WriteString("NSGo's license covers its bundled JavaScript runtime.\n")
			}
		}
	}
	var violations []error
	for _, name := range slices.Sorted(maps.Keys(sources)) {
		source := sources[name]
		var section strings.Builder
		section.WriteString("\n" + source.label + "\n" + strings.Repeat("-", len(source.label)) + "\n")
		types := map[string]bool{}
		var unknown []string
		var documents int
		files, err := source.licenseFiles()
		if err != nil {
			return "", fmt.Errorf("read licenses for %s: %w", source.name, err)
		}
		for _, path := range slices.Sorted(maps.Keys(files)) {
			data, err := os.ReadFile(path)
			if err != nil {
				return "", fmt.Errorf("read license for %s: %w", source.name, err)
			}
			rel, _ := filepath.Rel(source.root, path)
			filename := filepath.ToSlash(rel)
			known := false
			for _, match := range classifier.Match(data).Matches {
				if match.MatchType == "License" {
					types[match.Name] = true
					known = true
				}
			}
			if files[path] && !known {
				unknown = append(unknown, filename)
			}
			fmt.Fprintf(&section, "\n%s\n\n", filename)
			section.Write(data)
			documents++
			if !bytes.HasSuffix(data, []byte("\n")) {
				section.WriteByte('\n')
			}
		}
		if source.firstParty && documents == 0 {
			continue
		}
		if !source.firstParty && (len(types) == 0 || len(unknown) > 0) {
			reason := "has no recognized license text"
			if len(unknown) > 0 {
				reason = "has unrecognized license text in " + strings.Join(unknown, ", ")
			}
			if err := p.Licenses.report(ctx, source.name, "unknown", reason); err != nil {
				violations = append(violations, err)
			}
			section.WriteString("License identification incomplete.\n")
		}
		var copyleft []string
		for _, id := range slices.Sorted(maps.Keys(types)) {
			if copyleftLicense(id) {
				copyleft = append(copyleft, id)
			}
		}
		if len(copyleft) > 0 && !source.firstParty {
			if err := p.Licenses.report(ctx, source.name, "copyleft", "has copyleft license text ("+strings.Join(copyleft, ", ")+")"); err != nil {
				violations = append(violations, err)
			}
		}
		out.WriteString(section.String())
	}
	return out.String(), errors.Join(violations...)
}

// licenseFiles merges discovered notices and explicit native-license paths.
func (source *licenseSource) licenseFiles() (map[string]bool, error) {
	files := maps.Clone(source.files)
	if files == nil {
		files = map[string]bool{}
	}
	for _, dir := range slices.Sorted(maps.Keys(source.dirs)) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			primary, include := isLicenseDocument(entry.Name())
			if !entry.IsDir() && include {
				files[filepath.Join(dir, entry.Name())] = primary
			}
		}
	}
	return files, nil
}

func isLicenseDocument(name string) (primary, include bool) {
	name = strings.ToUpper(name)
	prefix, _, _ := strings.Cut(name, ".")
	prefix, _, _ = strings.Cut(prefix, "-")
	switch prefix {
	case "LICENSE", "LICENCE", "COPYING", "UNLICENSE", "OFL":
		return true, true
	case "NOTICE", "NOTICES", "COPYRIGHT", "PATENTS":
		return false, true
	}
	return false, false
}

func copyleftLicense(id string) bool {
	family, _, _ := strings.Cut(id, "-")
	switch family {
	case "AGPL", "APSL", "CDDL", "CPL", "EPL", "EUPL", "GPL", "LGPL", "MPL", "OSL", "SSPL":
		return true
	}
	return false
}

func (policy licensePolicy) report(ctx context.Context, module, kind, reason string) error {
	if slices.Contains(policy.Allow, module) {
		return nil
	}
	if kind == "unknown" && policy.Unknown != "warn" || kind == "copyleft" && policy.Copyleft == "error" {
		return fmt.Errorf("%s %s; set licenses.%s to warn or add the module to licenses.allow", module, reason, kind)
	}
	warnIn(ctx, "%s %s", module, reason)
	return nil
}

// androidReviewNotices reads the SDK notices kept with the rebuilt runtime. Older committed runtimes have no SDK and therefore no notice entry.
func androidReviewNotices(data []byte) (string, error) {
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return "", fmt.Errorf("Android runtime notices: %w", err)
	}
	for _, file := range z.File {
		if file.Name != "META-INF/nsgo-play-review-NOTICE.txt" {
			continue
		}
		r, err := file.Open()
		if err != nil {
			return "", err
		}
		defer r.Close()
		text, err := io.ReadAll(r)
		return "\n" + string(text) + "\n", err
	}
	return "", nil
}
