package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
)

func parseModuleTemplate(kind string) (module, version, name string, ok bool) {
	spec, rawName, hasName := strings.Cut(kind, "#")
	module, version, hasVersion := strings.Cut(spec, "@")
	domain, _, _ := strings.Cut(module, "/")
	if !strings.Contains(domain, ".") || strings.HasPrefix(module, "-") || hasVersion && version == "" {
		return "", "", "", false
	}
	if hasName {
		localName, err := filepath.Localize(rawName)
		if err != nil || localName == "" || localName == "." {
			return "", "", "", false
		}
		name = localName
	}
	if !hasVersion {
		version = "latest"
	}
	return module, version, name, true
}

func openModuleTemplateRoot(moduleDir, module, name string) (*os.Root, error) {
	root, err := os.OpenRoot(filepath.Join(moduleDir, "template"))
	if err != nil {
		return nil, fmt.Errorf("init: module %s has no readable template directory: %w", module, err)
	}
	if name == "" {
		return root, nil
	}
	sub, err := root.OpenRoot(name)
	root.Close()
	if err != nil {
		return nil, fmt.Errorf("init: module %s has no readable template %q: %w", module, filepath.ToSlash(name), err)
	}
	return sub, nil
}

// initModuleTemplate writes the template kind names into dir. Its go.mod.tmpl, or else the go.mod init writes as for a built-in template outside any module or with -module, makes dir a new module, which completeModule completes as it does a built-in template's, keeping the template's own lines; otherwise the module enclosing dir, at enclosing, gets the lines it lacks.
func initModuleTemplate(ctx context.Context, dir, kind string, data initData, enclosing string) error {
	module, version, name, ok := parseModuleTemplate(kind)
	if !ok {
		return fmt.Errorf("init: no template %q; use %s or a module path with optional @version and #name", kind, strings.Join(templates, ", "))
	}
	moduleDir, err := downloadInitModule(ctx, module+"@"+version)
	if err != nil {
		return err
	}
	src, err := openModuleTemplateRoot(moduleDir, module, name)
	if err != nil {
		return err
	}
	defer src.Close()

	if name == "" && !isTemplateDir(src.FS(), ".") {
		if names, _, err := moduleTemplateNames(src.FS()); err == nil && len(names) > 0 {
			return fmt.Errorf("init: module %s has no default template; select one with -template %s#<name> from: %s", module, kind, strings.Join(names, ", "))
		}
	}
	files, err := moduleTemplateFiles(src.FS(), data, data.Module != "" || enclosing == "")
	if err != nil {
		return fmt.Errorf("init: template %s: %w", kind, err)
	}
	if err := writeModuleTemplate(dir, src, files); err != nil {
		return err
	}
	newModule := slices.ContainsFunc(files, func(f moduleTemplateFile) bool { return f.name == "go.mod" })
	return completeModule(ctx, dir, enclosing, newModule)
}

func listInitTemplates(ctx context.Context, w io.Writer, kind string) error {
	if slices.Contains(templates, kind) {
		for _, name := range templates {
			if _, err := fmt.Fprintln(w, name); err != nil {
				return err
			}
		}
		return nil
	}
	module, version, name, ok := parseModuleTemplate(kind)
	if !ok {
		return fmt.Errorf("init: no template %q; use %s or a module path with optional @version and #name", kind, strings.Join(templates, ", "))
	}
	if name != "" {
		return errors.New("init: -list does not take a #template name")
	}
	moduleDir, err := downloadInitModule(ctx, module+"@"+version)
	if err != nil {
		return err
	}
	root, err := openModuleTemplateRoot(moduleDir, module, "")
	if err != nil {
		return err
	}
	defer root.Close()

	names, hasRoot, err := moduleTemplateNames(root.FS())
	if err != nil {
		return fmt.Errorf("init: list templates in %s: %w", module, err)
	}
	if len(names) == 0 && !hasRoot {
		return fmt.Errorf("init: module %s has no templates", module)
	}
	if hasRoot {
		if _, err := fmt.Fprintln(w, "(default)"); err != nil {
			return err
		}
	}
	for _, n := range names {
		if _, err := fmt.Fprintln(w, n); err != nil {
			return err
		}
	}
	return nil
}

// A template/ directory that is itself a template is copied whole, so its subdirectories are part of it rather than named templates.
func moduleTemplateNames(src fs.FS) ([]string, bool, error) {
	if isTemplateDir(src, ".") {
		return nil, true, nil
	}
	var names []string
	err := fs.WalkDir(src, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || name == "." || !entry.IsDir() {
			return err
		}
		if isTemplateDir(src, name) {
			names = append(names, name)
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	slices.Sort(names)
	return names, false, nil
}

func isTemplateDir(src fs.FS, dir string) bool {
	return hasTemplateEntry(src, dir, "nsgo.yaml") && hasTemplateEntry(src, dir, "main.go")
}

func hasTemplateEntry(src fs.FS, dir, base string) bool {
	for _, name := range []string{base, base + ".tmpl"} {
		p := name
		if dir != "." {
			p = dir + "/" + name
		}
		info, err := fs.Stat(src, p)
		if err == nil && info.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// Use an isolated directory so a caller's module, workspace and replace
// directives cannot influence the download or receive its checksums.
func downloadInitModule(ctx context.Context, query string) (string, error) {
	dir, err := os.MkdirTemp("", "nsgo-init-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	env := append(os.Environ(), "GO111MODULE=on", "GOWORK=off", "GOTOOLCHAIN=local", "GONOPROXY=none")
	goCommand := func(args ...string) *exec.Cmd {
		cmd := exec.CommandContext(ctx, "go", args...)
		cmd.Dir, cmd.Env = dir, env
		return cmd
	}
	proxy, err := goCommand("env", "GOPROXY").Output()
	if err != nil {
		return "", fmt.Errorf("init: read Go module proxy: %w", err)
	}
	env = append(env, "GOPROXY="+initModuleProxy(strings.TrimSpace(string(proxy))))

	out, err := goCommand("mod", "download", "-json", query).Output()
	var result struct{ Dir, Error string }
	decodeErr := json.Unmarshal(out, &result)
	if err != nil {
		if result.Error != "" {
			return "", fmt.Errorf("init: download template %s: %s", query, result.Error)
		}
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) != 0 {
			return "", fmt.Errorf("init: download template %s: %s", query, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return "", fmt.Errorf("init: download template %s: %w", query, err)
	}
	if decodeErr != nil {
		return "", fmt.Errorf("init: read download result for %s: %w", query, decodeErr)
	}
	if result.Dir == "" {
		return "", fmt.Errorf("init: download template %s returned no module directory", query)
	}
	return result.Dir, nil
}

func initModuleProxy(proxies string) string {
	start := 0
	for {
		end := strings.IndexAny(proxies[start:], ",|")
		if end < 0 {
			end = len(proxies) - start
		}
		value := strings.TrimSpace(proxies[start : start+end])
		if value == "direct" {
			return proxies[:start] + "off"
		}
		if start+end == len(proxies) {
			return proxies
		}
		start += end + 1
	}
}

type moduleTemplateFile struct {
	name, source string
	dir          bool
	content      []byte
}

// Read and render every template before opening any destination file. writeModule writes a go.mod, as for a built-in template, when the template has none.
func moduleTemplateFiles(src fs.FS, data initData, writeModule bool) ([]moduleTemplateFile, error) {
	if data.Module == "" {
		data.Module = defaultModule(data.Slug)
	}
	var files []moduleTemplateFile
	seen := make(map[string]bool)
	err := fs.WalkDir(src, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || name == "." {
			return err
		}
		file := moduleTemplateFile{source: name, dir: entry.IsDir()}
		target := name
		if !file.dir {
			if !entry.Type().IsRegular() {
				return fmt.Errorf("%s is not a regular file", name)
			}
			var marked bool
			target, marked = strings.CutSuffix(name, ".tmpl")
			if marked || target == "nsgo.yaml" || target == "main.go" {
				var err error
				file.content, err = renderInitTemplate(src, name, data)
				if err != nil {
					return err
				}
				file.source = ""
			}
		}
		if _, exists := seen[target]; target == "." || exists {
			return fmt.Errorf("%s conflicts with another template entry", target)
		}
		file.name, err = filepath.Localize(target)
		if err != nil {
			return fmt.Errorf("invalid output path %q: %w", target, err)
		}
		files = append(files, file)
		seen[target] = !file.dir
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, name := range []string{"nsgo.yaml", "main.go"} {
		if !seen[name] {
			return nil, fmt.Errorf("missing %s", name)
		}
	}
	hasModuleFile, hasModuleEntry := seen["go.mod"]
	if hasModuleEntry && !hasModuleFile {
		return nil, fmt.Errorf("go.mod is a directory")
	}
	if writeModule && !hasModuleEntry {
		var content bytes.Buffer
		if err := initMod.Execute(&content, data); err != nil {
			return nil, err
		}
		files = append(files, moduleTemplateFile{name: "go.mod", content: content.Bytes()})
	}
	return files, nil
}

func renderInitTemplate(src fs.FS, name string, data initData) ([]byte, error) {
	content, err := fs.ReadFile(src, name)
	if err != nil {
		return nil, err
	}
	t, err := template.New(name).Option("missingkey=error").Parse(string(content))
	if err != nil {
		return nil, err
	}
	var rendered bytes.Buffer
	if err := t.Execute(&rendered, data); err != nil {
		return nil, err
	}
	return rendered.Bytes(), nil
}

func writeModuleTemplate(dir string, src *os.Root, files []moduleTemplateFile) (err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst, err := os.OpenRoot(dir)
	if err != nil {
		return err
	}
	defer dst.Close()

	for _, file := range files {
		info, err := dst.Lstat(file.name)
		switch {
		case err == nil && !(file.dir && info.IsDir()):
			return fmt.Errorf("init: %s exists", rel(filepath.Join(dir, file.name)))
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			return err
		}
	}
	var created []string
	defer func() {
		if err != nil {
			for i := len(created) - 1; i >= 0; i-- {
				err = errors.Join(err, dst.Remove(created[i]))
			}
		}
	}()
	for _, file := range files {
		if file.dir {
			if err := dst.Mkdir(file.name, 0o755); err != nil {
				if info, statErr := dst.Lstat(file.name); !errors.Is(err, fs.ErrExist) || statErr != nil || !info.IsDir() {
					return err
				}
				continue
			}
		} else {
			if err := copyModuleTemplateFile(src, dst, file); err != nil {
				return err
			}
		}
		created = append(created, file.name)
	}
	for _, file := range files {
		if !file.dir {
			done("create %s", rel(filepath.Join(dir, file.name)))
		}
	}
	return nil
}

func copyModuleTemplateFile(src, dst *os.Root, file moduleTemplateFile) error {
	var content io.Reader = bytes.NewReader(file.content)
	if file.source != "" {
		f, err := src.Open(file.source)
		if err != nil {
			return err
		}
		defer f.Close()
		content = f
	}
	f, err := dst.OpenFile(file.name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(f, content)
	if err := errors.Join(copyErr, f.Close()); err != nil {
		return errors.Join(err, dst.Remove(file.name))
	}
	return nil
}
