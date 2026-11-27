package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"golang.org/x/mod/modfile"
	modzip "golang.org/x/mod/zip"
)

// nsgo check integration for components declared in a module root's
// nsgo-module.yaml (#2466). Modules without nsgo-module.yaml or without a
// components map are checked unchanged.

// checkModuleComponents checks every component declared in p.Dir's root
// nsgo-module.yaml over the files x/mod's zip.CheckDir would put in the
// module's zip. A module without nsgo-module.yaml or without components returns
// nil with no output.
func checkModuleComponents(_ context.Context, p *project) error {
	if p == nil || p.Dir == "" {
		return nil
	}
	modPath, ok, err := readRootModulePath(p.Dir)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	manifest, err := readModuleManifest(p.Dir, modPath, moduleManifestNSGoVersion())
	if err != nil {
		return err
	}
	if manifest == nil || len(manifest.Components) == 0 {
		return nil
	}

	s := begin("checking components")
	cf, cfErr := modzip.CheckDir(p.Dir)
	if cfErr != nil && len(cf.Valid) == 0 && len(cf.Omitted) == 0 && len(cf.Invalid) == 0 && cf.SizeError == nil {
		s.fail()
		return fmt.Errorf("checking module files: %w", cfErr)
	}

	// Collect every valid and invalid file path CheckDir saw, relative to p.Dir.
	// Including cf.Invalid lets component rules diagnose case-insensitive
	// collisions and invalid path elements inside declared components directly.
	var modFiles []string
	addModPath := func(absPath string) {
		relPath, err := filepath.Rel(p.Dir, absPath)
		if err != nil {
			return
		}
		slash := filepath.ToSlash(relPath)
		if slash == "." || strings.HasPrefix(slash, "../") {
			return
		}
		modFiles = append(modFiles, slash)
	}
	for _, f := range cf.Valid {
		addModPath(f)
	}
	for _, f := range cf.Invalid {
		addModPath(f.Path)
	}

	// Track omitted nested modules (submodules) inside p.Dir.
	omittedDirs := make([]string, 0, len(cf.Omitted))
	for _, f := range cf.Omitted {
		relPath, err := filepath.Rel(p.Dir, f.Path)
		if err != nil {
			continue
		}
		slash := filepath.ToSlash(relPath)
		if slash != "." && !strings.HasPrefix(slash, "../") {
			omittedDirs = append(omittedDirs, slash)
		}
	}

	compDirs := make(map[string]bool, len(manifest.Components))
	for _, dir := range manifest.Components {
		compDirs[dir] = true
	}

	var errLines []string
	for _, name := range manifest.componentNames() {
		compDir := manifest.Components[name]
		if path.Base(compDir) == "main" {
			errLines = append(errLines, fmt.Sprintf("%s: component %q (%s) must not be a main package", compDir, name, compDir))
			continue
		}
		absCompDir := filepath.Join(p.Dir, filepath.FromSlash(compDir))
		info, statErr := os.Lstat(absCompDir)
		if statErr != nil {
			errLines = append(errLines, fmt.Sprintf("%s: component %q directory not found: %v", compDir, name, statErr))
			continue
		}
		if !info.IsDir() {
			errLines = append(errLines, fmt.Sprintf("%s: component %q must be a directory", compDir, name))
			continue
		}
		if nestedMod := findNestedGoMod(p.Dir, compDir); nestedMod != "" {
			errLines = append(errLines, fmt.Sprintf("%s: component %q is inside a nested module (%s)", compDir, name, nestedMod))
			continue
		}

		// Refuse any nested module directory inside the component too.
		prefix := compDir + "/"
		for _, omitted := range omittedDirs {
			if strings.HasPrefix(omitted, prefix) {
				goModPath := filepath.Join(p.Dir, filepath.FromSlash(omitted), "go.mod")
				if st, err := os.Lstat(goModPath); err == nil && !st.IsDir() {
					errLines = append(errLines, fmt.Sprintf("%s/go.mod: nested module file is not allowed in a component", omitted))
				}
			}
		}

		compRoot, err := os.OpenRoot(absCompDir)
		if err != nil {
			errLines = append(errLines, fmt.Sprintf("%s: %v", compDir, err))
			continue
		}

		var files []componentFile
		for _, mf := range modFiles {
			relPath, ok := strings.CutPrefix(mf, prefix)
			if !ok {
				continue
			}
			st, err := compRoot.Lstat(relPath)
			if err != nil {
				errLines = append(errLines, fmt.Sprintf("%s: %v", mf, err))
				continue
			}
			pRel := relPath
			files = append(files, componentFile{
				Path: pRel,
				Size: st.Size(),
				Open: func() (io.ReadCloser, error) {
					return compRoot.Open(pRel)
				},
			})
		}

		res := checkComponentRules(files, componentCheckOptions{
			ModulePath:    modPath,
			ComponentDir:  compDir,
			ComponentDirs: compDirs,
		})
		compRoot.Close()

		for _, flag := range res.Flags {
			warn("%s (nsgo add -y will refuse the copy)", formatComponentFinding(compDir, flag))
		}
		for _, imp := range res.ModuleImports {
			note("component %s (%s) imports %s, which is not a component; apps that copy it will require %s", name, compDir, imp, modPath)
		}
		for _, e := range res.Errors {
			errLines = append(errLines, formatComponentFinding(compDir, e))
		}
	}

	if len(errLines) > 0 {
		s.fail()
		return fmt.Errorf("components have errors\n%s", strings.Join(errLines, "\n"))
	}
	s.done("checked components", "")
	return nil
}

// readRootModulePath returns the module path declared by go.mod directly in
// dir, or ok == false when dir has no go.mod.
func readRootModulePath(dir string) (modPath string, ok bool, err error) {
	data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	mf, err := modfile.ParseLax("go.mod", data, nil)
	if err != nil {
		return "", false, fmt.Errorf("go.mod: %w", err)
	}
	if mf.Module == nil || mf.Module.Mod.Path == "" {
		return "", false, errors.New("go.mod has no module directive")
	}
	return mf.Module.Mod.Path, true, nil
}

// findNestedGoMod checks whether compDir or any directory strictly between
// modRoot and compDir contains a go.mod file.
func findNestedGoMod(modRoot, compDir string) string {
	for cur := compDir; cur != "." && cur != "" && cur != "/"; cur = path.Dir(cur) {
		goMod := filepath.Join(modRoot, filepath.FromSlash(cur), "go.mod")
		if info, err := os.Lstat(goMod); err == nil && !info.IsDir() {
			return cur + "/go.mod"
		}
	}
	return ""
}
