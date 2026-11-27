package main

import (
	"bytes"
	"cmp"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
	"golang.org/x/mod/module"
)

// nsgo-copy.yaml sits in each copied component's directory (#2467, #2459).
// It records the source module, canonical version, component name, test choice,
// copied files and generated files deterministically with no timestamps or
// hashes, so loading and saving produces identical bytes. Every read and write
// driven by a record goes through an os.Root opened at the copy's directory,
// with paths validated on every read and symlinks refused.

const (
	copyRecordName     = "nsgo-copy.yaml"
	copyRecordVersion  = 1
	maxCopyRecordBytes = 64 << 10 // 64 KiB
	copyRecordHeader   = "# Written by nsgo add. nsgo diff compares this directory with the copy of this version.\n"
)

// copyRecord is version 1 of nsgo-copy.yaml.
type copyRecord struct {
	Version       int      `yaml:"version"`
	Module        string   `yaml:"module"`
	ModuleVersion string   `yaml:"module_version"`
	Component     string   `yaml:"component"`
	Tests         bool     `yaml:"tests"`
	Files         []string `yaml:"files"`
	Generated     []string `yaml:"generated,omitempty"`
}

// moduleCopy is an existing copied component discovered inside the app's module.
type moduleCopy struct {
	dir        string // absolute path to the copy's directory
	relDir     string // slash-separated path relative to the module root
	importPath string // <appModule>/<relDir>
	record     *copyRecord
}

// formatCopyRecord writes r in canonical deterministic YAML form.
func formatCopyRecord(r *copyRecord) ([]byte, error) {
	if err := validateCopyRecord(r, nil); err != nil {
		return nil, err
	}
	files := slices.Clone(r.Files)
	slices.Sort(files)
	generated := slices.Clone(r.Generated)
	slices.Sort(generated)

	var b bytes.Buffer
	b.WriteString(copyRecordHeader)
	fmt.Fprintf(&b, "version: %d\n", r.Version)
	fmt.Fprintf(&b, "module: %s\n", r.Module)
	fmt.Fprintf(&b, "module_version: %s\n", r.ModuleVersion)
	fmt.Fprintf(&b, "component: %s\n", r.Component)
	fmt.Fprintf(&b, "tests: %t\n", r.Tests)
	if len(files) == 0 {
		b.WriteString("files: []\n")
	} else {
		b.WriteString("files:\n")
		for _, f := range files {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	if len(generated) > 0 {
		b.WriteString("generated:\n")
		for _, f := range generated {
			fmt.Fprintf(&b, "  - %s\n", f)
		}
	}
	return b.Bytes(), nil
}

// parseCopyRecord strictly parses and validates nsgo-copy.yaml bytes.
func parseCopyRecord(data []byte) (*copyRecord, error) {
	if len(data) > maxCopyRecordBytes {
		return nil, fmt.Errorf("%s exceeds 64 KiB", copyRecordName)
	}
	if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		return nil, fmt.Errorf("%s: UTF-8 byte order mark (BOM) is not allowed", copyRecordName)
	}
	if !utf8.Valid(data) {
		return nil, fmt.Errorf("%s: not valid UTF-8", copyRecordName)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%s: empty YAML file", copyRecordName)
	}

	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", copyRecordName, err)
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("%s: multiple YAML documents are not allowed", copyRecordName)
		}
		return nil, fmt.Errorf("%s: %w", copyRecordName, err)
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: must be a YAML mapping", copyRecordName)
	}
	if err := rejectCopyYAMLFeatures(&doc); err != nil {
		return nil, fmt.Errorf("%s: %w", copyRecordName, err)
	}

	// Capture line numbers for files/generated entries so path refusals can name the line.
	pathLines := map[string]int{}
	rootMap := doc.Content[0]
	for i := 0; i+1 < len(rootMap.Content); i += 2 {
		k, v := rootMap.Content[i], rootMap.Content[i+1]
		if (k.Value == "files" || k.Value == "generated") && v.Kind == yaml.SequenceNode {
			for _, item := range v.Content {
				if item.Kind == yaml.ScalarNode {
					if _, exists := pathLines[item.Value]; !exists {
						pathLines[item.Value] = item.Line
					}
				}
			}
		}
	}

	var r copyRecord
	if err := decodeStrictYAML(data, &r); err != nil {
		return nil, fmt.Errorf("%s: %w", copyRecordName, err)
	}
	if err := validateCopyRecord(&r, pathLines); err != nil {
		return nil, fmt.Errorf("%s: %w", copyRecordName, err)
	}
	return &r, nil
}

func validateCopyRecord(r *copyRecord, pathLines map[string]int) error {
	switch {
	case r.Version == 0:
		return fmt.Errorf("version: %d is required", copyRecordVersion)
	case r.Version != copyRecordVersion:
		return fmt.Errorf("version is %d; this nsgo (%s) reads version %d", r.Version, displayVersion(toolVersion()), copyRecordVersion)
	}
	if err := module.CheckPath(r.Module); err != nil {
		return fmt.Errorf("invalid module %q: %w", r.Module, err)
	}
	if err := module.Check(r.Module, r.ModuleVersion); err != nil {
		return fmt.Errorf("invalid module_version %q: %w", r.ModuleVersion, err)
	}
	if !validComponentName(r.Component) {
		return fmt.Errorf("invalid component name %q", r.Component)
	}

	seen := map[string]bool{}
	seenLower := map[string]string{}
	checkList := func(field string, paths []string) error {
		for _, p := range paths {
			linePrefix := ""
			if line := pathLines[p]; line > 0 {
				linePrefix = fmt.Sprintf("line %d: ", line)
			}
			if err := validateCopyRecordPath(p); err != nil {
				return fmt.Errorf("%s%s entry %q: %w", linePrefix, field, p, err)
			}
			if seen[p] {
				return fmt.Errorf("%sduplicate path %q", linePrefix, p)
			}
			low := strings.ToLower(p)
			if prev, ok := seenLower[low]; ok {
				return fmt.Errorf("%spath %q collides case-insensitively with %q", linePrefix, p, prev)
			}
			seen[p] = true
			seenLower[low] = p
		}
		return nil
	}
	if err := checkList("files", r.Files); err != nil {
		return err
	}
	if err := checkList("generated", r.Generated); err != nil {
		return err
	}
	return nil
}

// validateCopyRecordPath verifies that p is a clean, local, slash-separated
// relative path allowed by the copied-component file rules.
func validateCopyRecordPath(p string) error {
	if p == "" || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") {
		return errors.New("must be a local slash-separated path")
	}
	if !filepath.IsLocal(filepath.FromSlash(p)) {
		return errors.New("path escapes the copy directory")
	}
	if err := module.CheckFilePath(p); err != nil {
		return err
	}
	return componentFileAllowed(p)
}

// readCopyRecord reads and strictly validates nsgo-copy.yaml inside copyDir
// through os.Root, refusing symlinks and non-regular files. Returns (nil, nil)
// when nsgo-copy.yaml does not exist in copyDir.
func readCopyRecord(copyDir string) (*copyRecord, error) {
	info, err := os.Lstat(copyDir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: symlinks are not allowed", copyDir)
	}
	if !info.IsDir() {
		return nil, nil
	}
	root, err := os.OpenRoot(copyDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()

	st, err := root.Lstat(copyRecordName)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: symlinks are not allowed", copyRecordName)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: must be a regular file", copyRecordName)
	}
	if st.Size() > maxCopyRecordBytes {
		return nil, fmt.Errorf("%s exceeds 64 KiB", copyRecordName)
	}
	f, err := root.Open(copyRecordName)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCopyRecordBytes+1))
	if err != nil {
		return nil, err
	}
	return parseCopyRecord(data)
}

// readCopyFile reads relPath from copyDir through os.Root after validating
// relPath and refusing any symlink along the path or at the file itself.
func readCopyFile(copyDir, relPath string) ([]byte, error) {
	if err := validateCopyRecordPath(relPath); err != nil {
		return nil, fmt.Errorf("%s: %w", relPath, err)
	}
	root, err := os.OpenRoot(copyDir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	local := filepath.FromSlash(relPath)
	st, err := root.Lstat(local)
	if err != nil {
		return nil, err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("%s: symlinks are not allowed", relPath)
	}
	if !st.Mode().IsRegular() {
		return nil, fmt.Errorf("%s: not a regular file", relPath)
	}
	f, err := root.Open(local)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

// findModuleCopies walks modRoot to locate every directory holding an
// nsgo-copy.yaml record, skipping vendor, testdata, nested modules, and
// directories starting with '.' or '_'.
func findModuleCopies(modRoot, appModPath string) ([]moduleCopy, error) {
	var copies []moduleCopy
	err := filepath.WalkDir(modRoot, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if p == modRoot {
			return nil
		}
		name := d.Name()
		if d.Type()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if name == "vendor" || name == "testdata" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			return fs.SkipDir
		}
		if st, err := os.Lstat(filepath.Join(p, "go.mod")); err == nil && !st.IsDir() {
			return fs.SkipDir
		}
		rec, err := readCopyRecord(p)
		if err != nil {
			relPath, _ := filepath.Rel(modRoot, p)
			return fmt.Errorf("%s/%s: %w", filepath.ToSlash(relPath), copyRecordName, err)
		}
		if rec != nil {
			relPath, err := filepath.Rel(modRoot, p)
			if err != nil {
				return err
			}
			slash := filepath.ToSlash(relPath)
			impPath := slash
			if appModPath != "" {
				impPath = appModPath + "/" + slash
			}
			copies = append(copies, moduleCopy{
				dir:        p,
				relDir:     slash,
				importPath: impPath,
				record:     rec,
			})
			// A copied component cannot hold another copied component.
			return fs.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(copies, func(a, b moduleCopy) int {
		return cmp.Compare(a.relDir, b.relDir)
	})
	return copies, nil
}

// findEnclosingCopy checks whether pkgDir or an ancestor directory strictly
// below modRoot holds an nsgo-copy.yaml record. Used by license notice
// collection so a copied component gets its own THIRD_PARTY_NOTICES.txt section.
func findEnclosingCopy(modRoot, pkgDir string) (copyDir string, rec *copyRecord, ok bool) {
	if modRoot == "" || pkgDir == "" {
		return "", nil, false
	}
	absMod, err := filepath.Abs(modRoot)
	if err != nil {
		return "", nil, false
	}
	absPkg, err := filepath.Abs(pkgDir)
	if err != nil {
		return "", nil, false
	}
	relPath, err := filepath.Rel(absMod, absPkg)
	if err != nil || relPath == "." || !filepath.IsLocal(relPath) {
		return "", nil, false
	}
	for cur := absPkg; cur != absMod && cur != filepath.Dir(cur); cur = filepath.Dir(cur) {
		r, err := readCopyRecord(cur)
		if err == nil && r != nil {
			return cur, r, true
		}
	}
	return "", nil, false
}

// removeCopyRefusal checks whether arg names a copied component in modRoot
// (either by component name or by copy directory) and returns the refusal error
// naming the copy's directory to delete, or nil when arg does not match a copy.
func removeCopyRefusal(modRoot, appModPath, arg string) error {
	copies, err := findModuleCopies(modRoot, appModPath)
	if err != nil || len(copies) == 0 {
		return nil
	}
	cleanArg := filepath.ToSlash(filepath.Clean(arg))
	var matches []moduleCopy
	for _, c := range copies {
		if c.relDir == cleanArg || c.record.Component == arg || c.record.Module+"#"+c.record.Component == arg {
			matches = append(matches, c)
		}
	}
	if len(matches) == 0 {
		return nil
	}
	if len(matches) == 1 {
		return fmt.Errorf("remove: %s is a component, copied into this module; delete %s", arg, matches[0].relDir)
	}
	dirs := make([]string, len(matches))
	for i, m := range matches {
		dirs[i] = m.relDir
	}
	return fmt.Errorf("remove: %s matches copied components at %s; delete the copy's directory", arg, strings.Join(dirs, ", "))
}

// rejectCopyYAMLFeatures refuses YAML anchors, aliases, merge keys and
// duplicate mapping keys before struct decoding.
func rejectCopyYAMLFeatures(n *yaml.Node) error {
	if n == nil {
		return nil
	}
	if n.Kind == yaml.AliasNode || n.Alias != nil {
		return fmt.Errorf("line %d: YAML aliases are not allowed", n.Line)
	}
	if n.Anchor != "" {
		return fmt.Errorf("line %d: YAML anchors are not allowed", n.Line)
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
				return fmt.Errorf("line %d: duplicate key %q", k.Line, k.Value)
			}
			seen[k.Value] = true
		}
	}
	for _, child := range n.Content {
		if err := rejectCopyYAMLFeatures(child); err != nil {
			return err
		}
	}
	return nil
}
