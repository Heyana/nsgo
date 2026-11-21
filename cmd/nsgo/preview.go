package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"
)

const previewTag = "nsgopreview"

//go:embed previewhost/main.go.txt
var previewHostSource string

// previewSession keeps one generated command under the component's module.
// The hidden directory is skipped by the Go tool's ordinary package patterns.
type previewSession struct {
	source, projectDir, host string
}

func devPreview(ctx context.Context, flags *devFlags, path, explicitProject string) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(explicitProject, path)
	}
	source, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if info, err := os.Stat(source); err != nil || !info.IsDir() {
		return fmt.Errorf("preview package directory %q does not exist", source)
	}
	module := moduleRootOf(source)
	if _, err := os.Stat(filepath.Join(module, "go.mod")); err != nil {
		return fmt.Errorf("-preview needs a Go package inside a module: %w", err)
	}
	projectDir := source
	if explicitProject != "" {
		projectDir, err = filepath.Abs(explicitProject)
		if err != nil {
			return err
		}
		if moduleRootOf(projectDir) != module {
			return errors.New("the app and preview package must be in the same Go module")
		}
	} else {
		projectDir = previewProjectDir(source, module)
	}

	host, err := os.MkdirTemp(source, ".nsgo-preview-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(host)
	ps := &previewSession{source: source, projectDir: projectDir, host: host}
	p, err := ps.project()
	if err != nil {
		return err
	}
	flags.apply(p)
	opts := flags.options()
	opts.preview = ps
	return dev(ctx, p, opts)
}

// previewProjectDir takes the closest enclosing app's configuration, when
// the component belongs to an app. A module can preview a library on its own.
func previewProjectDir(dir, module string) string {
	for at := dir; ; at = filepath.Dir(at) {
		if _, err := os.Stat(filepath.Join(at, "nsgo.yaml")); err == nil {
			return at
		}
		if at == module {
			return dir
		}
	}
}

func (ps *previewSession) project() (*project, error) {
	config, err := loadOrDir(ps.projectDir)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(ps.source))
	p := &project{
		Dir: ps.host, resourceDir: filepath.Join(config.Dir, resourcesDir),
		Name: "Component Previews", ID: fmt.Sprintf("dev.nsgo.preview.p%x", sum[:8]),
		Version: "0.0.0", Appearance: config.Appearance, I18n: config.I18n,
	}
	p.Dev.Target, p.Dev.HotReload = config.Dev.Target, config.Dev.HotReload
	if p.Dev.Target == "" {
		p.Dev.Target = "native"
	}
	p.Build.Tags = append(slices.Clone(config.Build.Tags), previewTag)
	p.Mobile = config.Mobile
	if config.Theme != "" {
		p.Theme = config.resolve(config.Theme)
	}
	return p, nil
}

type previewPackage struct {
	Name, ImportPath string
	GoFiles          []string
	Error            *struct{ Err string }
}

func listPreviewPackage(ctx context.Context, dir string, env, tags []string) (previewPackage, error) {
	args := append([]string{"list", "-e", "-json=Name,ImportPath,GoFiles,Error"}, tagsArgs(tags)...)
	cmd := exec.CommandContext(ctx, "go", append(args, ".")...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return previewPackage{}, fmt.Errorf("go list preview package: %w\n%s", err, strings.TrimSpace(stderr.String()))
	}
	var info previewPackage
	err = json.Unmarshal(out, &info)
	return info, err
}

// previewFunctions reads fixtures from files enabled by nsgopreview.
// The host's typed slice checks the exact signature when it compiles.
func previewFunctions(dir string, previewFiles, ordinaryFiles []string) ([]string, error) {
	var names []string
	for _, file := range previewFiles {
		if slices.Contains(ordinaryFiles, file) {
			continue
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, file), nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, err
		}
		for _, decl := range parsed.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if ok && fn.Recv == nil && strings.HasPrefix(fn.Name.Name, "Preview") && len(fn.Name.Name) > len("Preview") {
				names = append(names, fn.Name.Name)
			}
		}
	}
	slices.Sort(names)
	return slices.Compact(names), nil
}

func (ps *previewSession) writeHost(ctx context.Context, p *project, initialLocale string) error {
	target := p.Dev.Target
	if target == "native" {
		system := runtime.GOOS
		if system == "darwin" {
			system = "macos"
		}
		target = system + "_" + goArchName(runtime.GOARCH)
	} else if target == "android" {
		target = "android_arm64"
	}
	env, err := vetEnv(ctx, target)
	if err != nil {
		return err
	}
	tags, err := goTags(ctx, p, env, "nsgodev")
	if err != nil {
		return err
	}
	previewTags := append(tags, targetTags(target)...)
	plainTags := slices.DeleteFunc(slices.Clone(previewTags), func(tag string) bool { return tag == previewTag })
	plain, err := listPreviewPackage(ctx, ps.source, env, plainTags)
	if err != nil {
		return err
	}
	selected, err := listPreviewPackage(ctx, ps.source, env, previewTags)
	if err != nil {
		return err
	}
	if selected.Error != nil {
		return fmt.Errorf("-preview: %s", selected.Error.Err)
	}
	if selected.Name == "main" {
		return errors.New("-preview needs an importable package; put fixtures beside components in a library package")
	}
	names, err := previewFunctions(ps.source, selected.GoFiles, plain.GoFiles)
	if err != nil {
		return err
	}
	if len(names) == 0 {
		return errors.New("-preview needs PreviewXxx functions in Go files guarded by //go:build nsgopreview")
	}

	data := struct {
		ImportPath, AppCatalog, Theme, InitialLocale string
		Names, Languages                             []string
	}{ImportPath: selected.ImportPath, Names: names, InitialLocale: initialLocale}
	if p.I18n.Dir != "" && hasCatalogs(p.I18n.Dir) {
		// A generated host imports the app's catalog to use all its locales
		// even if the isolated component happens not to import it.
		data.AppCatalog, err = importPathOf(p.I18n.Dir)
		if err != nil {
			return err
		}
		if _, _, exists := generatedCatalog(p.I18n.Dir); !exists {
			if _, err := p.writeCatalogs(ctx, []goPackage{{importPath: data.AppCatalog}}, false); err != nil {
				return err
			}
		}
		catalog, _ := loadCatalog(p.I18n.Dir, p.I18n.Source)
		data.Languages = catalog.languages()
		if data.AppCatalog == data.ImportPath {
			data.AppCatalog = ""
		}
	}
	if p.Theme != "" {
		theme, err := os.ReadFile(p.Theme)
		if err != nil {
			return fmt.Errorf("preview theme: %w", err)
		}
		data.Theme = string(theme)
	}
	host, err := template.New("preview host").Delims("/*%", "%*/").Parse(previewHostSource)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	if err := host.Execute(&buf, data); err != nil {
		return err
	}
	code, err := format.Source(buf.Bytes())
	if err != nil {
		return fmt.Errorf("formatting preview host: %w", err)
	}
	path := filepath.Join(ps.host, "main.go")
	if old, err := os.ReadFile(path); err == nil && bytes.Equal(old, code) {
		return nil
	}
	return os.WriteFile(path, code, 0o600)
}
