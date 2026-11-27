package main

import (
	"bytes"
	"errors"
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/scanner"
	"go/token"
	"io"
	"path"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
	"golang.org/x/text/language"
)

// Shared copy-component rules enforced by both nsgo check (#2466) and nsgo add
// (#2467), following the Copied components rules of RFC #2459.

const (
	maxComponentFiles      = 256
	maxComponentFileBytes  = 1 << 20 // 1 MiB
	maxComponentTotalBytes = 4 << 20 // 4 MiB
)

// componentFile is one file inside a component directory, supplied either from
// a directory scan (x/mod zip.CheckDir) or from a downloaded module zip archive.
type componentFile struct {
	// Path is the clean, slash-separated path relative to the component directory.
	Path string
	// Size is the uncompressed size in bytes, checked before Open is called.
	Size int64
	// Open opens the file's contents. It is only called for allowed files within
	// the per-file and total size limits.
	Open func() (io.ReadCloser, error)
}

// componentFinding is a rule error or flag warning reported by checkComponentRules.
type componentFinding struct {
	// File is the slash-separated path relative to the component directory, or
	// empty for a component-wide error such as total file count or total size.
	File    string
	Line    int // 1-based line number, or 0 when not line-specific
	Column  int // 1-based rune column, or 0 when not column-specific
	Message string
}

// formatComponentFinding formats f with an optional component directory prefix.
func formatComponentFinding(compDir string, f componentFinding) string {
	loc := compDir
	if f.File != "" {
		if compDir != "" {
			loc = compDir + "/" + f.File
		} else {
			loc = f.File
		}
	}
	switch {
	case loc != "" && f.Line > 0 && f.Column > 0:
		return fmt.Sprintf("%s:%d:%d: %s", loc, f.Line, f.Column, f.Message)
	case loc != "" && f.Line > 0:
		return fmt.Sprintf("%s:%d: %s", loc, f.Line, f.Message)
	case loc != "":
		return fmt.Sprintf("%s: %s", loc, f.Message)
	default:
		return f.Message
	}
}

// componentCheckOptions configures checkComponentRules.
type componentCheckOptions struct {
	// ModulePath is the module's path (from go.mod), used to identify imports of
	// other packages in the same module.
	ModulePath string
	// ComponentDir is the component's slash-separated directory relative to the
	// module root (e.g. "ui/chat"). Its last path element is the expected Go
	// package name.
	ComponentDir string
	// ComponentDirs maps every component directory declared by the module (relative
	// to the module root) to true. Imports of other packages in ModulePath not in
	// this set are recorded in componentCheckResult.ModuleImports.
	ComponentDirs map[string]bool
}

// componentCheckResult holds broken rules, flag warnings and non-component
// same-module imports found in a component.
type componentCheckResult struct {
	// Errors are broken rules that prevent copying under any flag.
	Errors []componentFinding
	// Flags are constructs shown for human confirmation and refused by nsgo add -y.
	Flags []componentFinding
	// ModuleImports are sorted, unique import paths of packages in the same module
	// that are not declared components (excluding the component itself and its
	// locales/ package).
	ModuleImports []string
}

// checkComponentRules validates all files of one component against the copied
// component rules: file allowlist, path restrictions, count/size limits, UTF-8
// without BOM, Go syntax/package/import/build/generate/embed rules, and flag
// warnings (control/format characters, non-ASCII identifiers, risky imports and
// compiler directives).
func checkComponentRules(files []componentFile, opts componentCheckOptions) componentCheckResult {
	var res componentCheckResult

	sorted := slices.Clone(files)
	slices.SortFunc(sorted, func(a, b componentFile) int {
		return strings.Compare(a.Path, b.Path)
	})

	// Check count and size limits before reading any file contents.
	if len(sorted) > maxComponentFiles {
		res.Errors = append(res.Errors, componentFinding{
			Message: fmt.Sprintf("component has %d files; maximum is %d", len(sorted), maxComponentFiles),
		})
	}
	var totalBytes int64
	oversize := make(map[string]bool)
	for _, f := range sorted {
		if f.Size > maxComponentFileBytes {
			oversize[f.Path] = true
			res.Errors = append(res.Errors, componentFinding{
				File:    f.Path,
				Message: fmt.Sprintf("file is %d bytes; maximum is 1 MiB (%d bytes)", f.Size, maxComponentFileBytes),
			})
		}
		if f.Size > 0 {
			totalBytes += f.Size
		}
	}
	if totalBytes > maxComponentTotalBytes {
		res.Errors = append(res.Errors, componentFinding{
			Message: fmt.Sprintf("component is %d bytes in total; maximum is 4 MiB (%d bytes)", totalBytes, maxComponentTotalBytes),
		})
	}

	// Pass 1: path syntax, case-insensitive collisions, file allow/refuse list,
	// and directory inventory (used by //go:embed checks in Pass 2).
	seenLower := make(map[string]string, len(sorted))
	seenDirLower := make(map[string]string)
	refusedFiles := make(map[string]string)
	allDirs := make(map[string]bool)
	var allowedFiles []componentFile

	for _, f := range sorted {
		for dir := path.Dir(f.Path); dir != "." && dir != "" && dir != "/"; dir = path.Dir(dir) {
			allDirs[dir] = true
			lowDir := strings.ToLower(dir)
			if prev, ok := seenDirLower[lowDir]; ok && prev != dir {
				res.Errors = append(res.Errors, componentFinding{
					File:    f.Path,
					Message: fmt.Sprintf("directory %q collides case-insensitively with %q", dir, prev),
				})
				refusedFiles[f.Path] = "case-insensitive collision"
			} else if !ok {
				seenDirLower[lowDir] = dir
			}
		}

		low := strings.ToLower(f.Path)
		if prev, ok := seenLower[low]; ok {
			res.Errors = append(res.Errors, componentFinding{
				File:    f.Path,
				Message: fmt.Sprintf("path collides case-insensitively with %s", prev),
			})
			refusedFiles[f.Path] = "case-insensitive collision"
		} else {
			seenLower[low] = f.Path
		}

		if err := componentFileAllowed(f.Path); err != nil {
			res.Errors = append(res.Errors, componentFinding{
				File:    f.Path,
				Message: err.Error(),
			})
			refusedFiles[f.Path] = err.Error()
			continue
		}
		allowedFiles = append(allowedFiles, f)
	}

	// Build set of declared component import paths in this module.
	wantPkg := path.Base(opts.ComponentDir)
	if opts.ComponentDir == "" || opts.ComponentDir == "." {
		wantPkg = ""
	}
	var compImportPath string
	if opts.ModulePath != "" && opts.ComponentDir != "" {
		compImportPath = opts.ModulePath + "/" + opts.ComponentDir
	}
	compImports := make(map[string]bool, len(opts.ComponentDirs))
	if opts.ModulePath != "" {
		for dir := range opts.ComponentDirs {
			compImports[opts.ModulePath+"/"+dir] = true
		}
	}

	// Pass 2: read and inspect allowed text and Go files (within size limits).
	var bytesRead int64
	var hasRootPkgFile bool
	var skippedRootGo bool
	moduleImportSet := make(map[string]bool)

	for _, f := range allowedFiles {
		isGo := strings.EqualFold(path.Ext(f.Path), ".go")
		isRootGo := isGo && path.Dir(f.Path) == "." && !strings.HasSuffix(strings.ToLower(path.Base(f.Path)), "_test.go")

		if !isComponentTextFile(f.Path) {
			continue
		}
		if oversize[f.Path] || totalBytes > maxComponentTotalBytes || len(sorted) > maxComponentFiles {
			if isRootGo {
				skippedRootGo = true
			}
			continue
		}
		if f.Open == nil {
			if isRootGo {
				skippedRootGo = true
			}
			continue
		}

		data, err := readBoundedFile(f.Open, maxComponentFileBytes)
		if err != nil {
			res.Errors = append(res.Errors, componentFinding{
				File:    f.Path,
				Message: err.Error(),
			})
			if isRootGo {
				skippedRootGo = true
			}
			continue
		}
		bytesRead += int64(len(data))
		if bytesRead > maxComponentTotalBytes {
			res.Errors = append(res.Errors, componentFinding{
				File:    f.Path,
				Message: fmt.Sprintf("component exceeds 4 MiB (%d bytes) total size limit", maxComponentTotalBytes),
			})
			if isRootGo {
				skippedRootGo = true
			}
			break
		}

		// UTF-8 without BOM.
		if bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
			res.Errors = append(res.Errors, componentFinding{
				File:    f.Path,
				Line:    1,
				Column:  1,
				Message: "UTF-8 byte order mark (BOM) is not allowed",
			})
			if isRootGo {
				skippedRootGo = true
			}
			continue
		}
		if line, col, ok := firstInvalidUTF8(data); !ok {
			res.Errors = append(res.Errors, componentFinding{
				File:    f.Path,
				Line:    line,
				Column:  col,
				Message: "text file is not valid UTF-8",
			})
			if isRootGo {
				skippedRootGo = true
			}
			continue
		}

		// Scan text files for control and format characters (flags).
		res.Flags = append(res.Flags, checkTextControlChars(f.Path, data)...)

		if !isGo {
			continue
		}

		goErrs, goFlags, pkgOK, modImps := checkComponentGoFile(f.Path, data, goFileCheckContext{
			wantPkg:        wantPkg,
			modulePath:     opts.ModulePath,
			compImportPath: compImportPath,
			compImports:    compImports,
			refusedFiles:   refusedFiles,
			allDirs:        allDirs,
		})
		res.Errors = append(res.Errors, goErrs...)
		res.Flags = append(res.Flags, goFlags...)
		if pkgOK && isRootGo {
			hasRootPkgFile = true
		}
		if len(goErrs) > 0 && isRootGo {
			skippedRootGo = true
		}
		for _, imp := range modImps {
			moduleImportSet[imp] = true
		}
	}

	if wantPkg != "" && !hasRootPkgFile && !skippedRootGo {
		res.Errors = append(res.Errors, componentFinding{
			Message: fmt.Sprintf("component has no non-test Go file declaring package %q", wantPkg),
		})
	}

	if len(moduleImportSet) > 0 {
		res.ModuleImports = make([]string, 0, len(moduleImportSet))
		for imp := range moduleImportSet {
			res.ModuleImports = append(res.ModuleImports, imp)
		}
		slices.Sort(res.ModuleImports)
	}

	return res
}

func readBoundedFile(open func() (io.ReadCloser, error), limit int64) ([]byte, error) {
	rc, err := open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(io.LimitReader(rc, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds 1 MiB (%d bytes)", limit)
	}
	return data, nil
}

// validComponentRelPath checks that relPath is a clean slash-separated relative
// path whose elements consist only of ASCII letters, digits, '-', '_' and '.',
// with no element starting with '.' or '_'.
func validComponentRelPath(relPath string) error {
	if relPath == "" || strings.HasPrefix(relPath, "/") || strings.Contains(relPath, "\\") {
		return fmt.Errorf("invalid component path %q", relPath)
	}
	for elem := range strings.SplitSeq(relPath, "/") {
		if elem == "" {
			return fmt.Errorf("path %q has an empty element", relPath)
		}
		if elem[0] == '.' || elem[0] == '_' {
			return fmt.Errorf("path element %q must not start with '.' or '_'", elem)
		}
		for i := range len(elem) {
			c := elem[i]
			ok := 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9' || c == '-' || c == '_' || c == '.'
			if !ok {
				return fmt.Errorf("path element %q contains disallowed character %q; only ASCII letters, digits, '-', '_' and '.' are allowed", elem, c)
			}
		}
	}
	return nil
}

// refusedExactNames are filenames refused anywhere inside a component,
// compared case-insensitively.
var refusedExactNames = map[string]string{
	"go.mod":            "nested module file is not allowed in a component",
	"go.sum":            "go.sum is not allowed in a component",
	"go.work":           "go.work is not allowed in a component",
	"go.work.sum":       "go.work.sum is not allowed in a component",
	"nsgo.yaml":         "nsgo.yaml is not allowed in a component",
	"nsgo-module.yaml":  "nsgo-module.yaml is not allowed in a component",
	"nsgo-natives.yaml": "nsgo-natives.yaml is not allowed in a component",
	"nsgo-copy.yaml":    "nsgo-copy.yaml is reserved for nsgo add and not allowed in a component",
	"package.json":      "package.json is not allowed in a component",
	"tsconfig.json":     "tsconfig.json is not allowed in a component",
	"jsconfig.json":     "jsconfig.json is not allowed in a component",
	"deno.json":         "deno.json is not allowed in a component",
	"composer.json":     "composer.json is not allowed in a component",
}

// refusedSourceExtensions are native/assembly/binary extensions explicitly
// refused anywhere inside a component, compared case-insensitively.
var refusedSourceExtensions = map[string]bool{
	".s":       true,
	".sx":      true,
	".syso":    true,
	".c":       true,
	".h":       true,
	".m":       true,
	".mm":      true,
	".cc":      true,
	".cpp":     true,
	".cxx":     true,
	".hh":      true,
	".hpp":     true,
	".f":       true,
	".f90":     true,
	".swig":    true,
	".swigcxx": true,
	".wasm":    true,
}

// allowedAssetExtensions are image, font, sound and plain-text extensions
// allowed anywhere inside a component, compared case-insensitively.
var allowedAssetExtensions = map[string]bool{
	// Images
	".svg":  true,
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".webp": true,
	".gif":  true,
	".avif": true,
	".ico":  true,
	// Fonts
	".ttf":   true,
	".otf":   true,
	".woff2": true,
	// Sounds
	".wav": true,
	".mp3": true,
	".ogg": true,
	// Plain text
	".txt": true,
}

// componentFileAllowed reports whether relPath (slash-separated, relative to
// the component root) is permitted by the copied-component path and file rules.
func componentFileAllowed(relPath string) error {
	if err := validComponentRelPath(relPath); err != nil {
		return err
	}

	low := strings.ToLower(relPath)
	dir := path.Dir(low)
	base := path.Base(low)
	ext := path.Ext(base)
	topDir := ""
	if dir != "." {
		topDir, _, _ = strings.Cut(low, "/")
	}

	// Refused exact names (checked first at any depth, case-insensitively).
	if reason, bad := refusedExactNames[base]; bad {
		return errors.New(reason)
	}

	// Refused native/assembly/Wasm extensions (checked before license documents
	// so names like LICENSE.syso or LICENSE.wasm can never bypass the rule).
	if refusedSourceExtensions[ext] {
		return fmt.Errorf("%s files are not allowed in a component", ext)
	}

	// Go files: allowed only at the component root and directly in locales/.
	if ext == ".go" {
		if dir == "." || dir == "locales" {
			return nil
		}
		return fmt.Errorf("Go files are only allowed at the component root and in locales/; %q would be a second package", path.Dir(relPath))
	}

	// License documents (same matcher as license notices in licenses.go).
	if _, ok := isLicenseDocument(path.Base(relPath)); ok {
		return nil
	}

	// YAML: only in locales/ (directly) and testdata/ (at any depth).
	if ext == ".yaml" || ext == ".yml" {
		if dir == "locales" || topDir == "testdata" {
			return nil
		}
		return fmt.Errorf("YAML files are only allowed in locales/ and testdata/")
	}

	// README.md at the component root only.
	if low == "readme.md" {
		return nil
	}

	// JSON: only in testdata/ and asset subdirectories (never top-level, never
	// locales/, and never package/tsconfig/jsconfig/deno/composer.json).
	if ext == ".json" {
		if dir == "." || topDir == "locales" {
			return fmt.Errorf("JSON files are only allowed in testdata/ and asset subdirectories")
		}
		return nil
	}

	// Images, fonts, sounds and .txt.
	if allowedAssetExtensions[ext] {
		return nil
	}

	return fmt.Errorf("file type is not allowed in a component")
}

// isComponentTextFile reports whether an allowed component file is treated as
// UTF-8 text (checked for BOM, valid UTF-8 and control/format characters)
// rather than a binary asset.
func isComponentTextFile(relPath string) bool {
	low := strings.ToLower(relPath)
	ext := path.Ext(low)
	switch ext {
	case ".go", ".yaml", ".yml", ".txt", ".md", ".json", ".svg":
		return true
	case ".png", ".jpg", ".jpeg", ".webp", ".gif", ".avif", ".ico",
		".ttf", ".otf", ".woff2",
		".wav", ".mp3", ".ogg":
		return false
	}
	_, license := isLicenseDocument(path.Base(relPath))
	return license
}

// firstInvalidUTF8 returns the 1-based line and column of the first invalid
// UTF-8 sequence in data, or (0, 0, true) when data is valid UTF-8.
func firstInvalidUTF8(data []byte) (line, col int, ok bool) {
	line, col = 1, 1
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r == utf8.RuneError && size == 1 {
			return line, col, false
		}
		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		data = data[size:]
	}
	return 0, 0, true
}

// checkTextControlChars flags Unicode Cc (other than tab, LF, and CR before LF),
// Cf, U+2028 and U+2029 in a valid UTF-8 text file. In an RTL language's
// catalog under locales/, U+200E, U+200F, U+061C and U+2066-U+2069 inside
// message values are exempted.
func checkTextControlChars(relPath string, data []byte) []componentFinding {
	normalized := strings.ReplaceAll(string(data), "\r\n", "\n")

	var rtlSpans []textSpan
	if isRTLCatalogFile(relPath) {
		rtlSpans = yamlCatalogMessageSpans(normalized)
	}

	var flags []componentFinding
	line, col := 1, 1
	for _, r := range normalized {
		if r == '\n' {
			line++
			col = 1
			continue
		}
		if r != '\t' && isFlaggedControlOrFormat(r) {
			if !(len(rtlSpans) > 0 && isExemptRTLBidiRune(r) && inTextSpans(rtlSpans, line, col)) {
				flags = append(flags, componentFinding{
					File:    relPath,
					Line:    line,
					Column:  col,
					Message: fmt.Sprintf("contains control or format character %U", r),
				})
			}
		}
		col++
	}
	return flags
}

func isFlaggedControlOrFormat(r rune) bool {
	return unicode.Is(unicode.Cc, r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029'
}

func isExemptRTLBidiRune(r rune) bool {
	return r == '\u200E' || r == '\u200F' || r == '\u061C' || (r >= '\u2066' && r <= '\u2069')
}

func isRTLCatalogFile(relPath string) bool {
	low := strings.ToLower(relPath)
	if path.Dir(low) != "locales" {
		return false
	}
	ext := path.Ext(low)
	if ext != ".yaml" && ext != ".yml" {
		return false
	}
	lang := strings.TrimSuffix(path.Base(relPath), path.Ext(relPath))
	return isRTLLanguage(lang)
}

// isRTLLanguage matches core/env.go's rightToLeft language and script table.
func isRTLLanguage(lang string) bool {
	parts := strings.Split(strings.ReplaceAll(lang, "_", "-"), "-")
	for _, p := range parts[1:] {
		if len(p) == 4 {
			switch strings.ToLower(p) {
			case "arab", "hebr", "syrc", "thaa", "nkoo", "adlm", "rohg", "mand", "samr", "mend", "yezi":
				return true
			}
			return false
		}
	}
	switch strings.ToLower(parts[0]) {
	case "ar", "arc", "ckb", "dv", "fa", "he", "iw", "ks", "ps", "sd", "syr", "ug", "ur", "yi", "lrc", "mzn", "glk":
		return true
	}
	return false
}

// textSpan is an inclusive [start, end] range of 1-based (line, col) positions.
type textSpan struct {
	startLine, startCol int
	endLine, endCol     int
}

func inTextSpans(spans []textSpan, line, col int) bool {
	for _, sp := range spans {
		afterStart := line > sp.startLine || (line == sp.startLine && col >= sp.startCol)
		beforeEnd := line < sp.endLine || (line == sp.endLine && col <= sp.endCol)
		if afterStart && beforeEnd {
			return true
		}
	}
	return false
}

// yamlCatalogMessageSpans parses normalized (LF-only) catalog YAML and returns
// the source spans of message value scalars, excluding keys, descriptions and
// comments.
func yamlCatalogMessageSpans(src string) []textSpan {
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
		return nil
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	lines := strings.Split(src, "\n")
	var spans []textSpan

	var collectMapping func(m *yaml.Node, allowSupply bool)
	collectMapping = func(m *yaml.Node, allowSupply bool) {
		for i := 0; i+1 < len(m.Content); i += 2 {
			k, v := m.Content[i], m.Content[i+1]
			if allowSupply && strings.ContainsAny(k.Value, "/.") && v.Kind == yaml.MappingNode {
				collectMapping(v, false)
				continue
			}
			switch v.Kind {
			case yaml.ScalarNode:
				if sp, ok := yamlScalarSpan(lines, k, v); ok {
					spans = append(spans, sp)
				}
			case yaml.MappingNode:
				for j := 0; j+1 < len(v.Content); j += 2 {
					fk, fv := v.Content[j], v.Content[j+1]
					if fk.Value == "message" && fv.Kind == yaml.ScalarNode {
						if sp, ok := yamlScalarSpan(lines, fk, fv); ok {
							spans = append(spans, sp)
						}
					}
				}
			}
		}
	}
	collectMapping(doc.Content[0], true)
	return spans
}

// yamlScalarSpan computes the rune-column span of scalar node v in LF-normalized lines.
func yamlScalarSpan(lines []string, keyNode, v *yaml.Node) (textSpan, bool) {
	if v.Line < 1 || v.Line > len(lines) || v.Column < 1 {
		return textSpan{}, false
	}
	switch v.Style {
	case yaml.DoubleQuotedStyle:
		return scanQuotedSpan(lines, v.Line, v.Column, '"', true)
	case yaml.SingleQuotedStyle:
		return scanQuotedSpan(lines, v.Line, v.Column, '\'', false)
	case yaml.LiteralStyle, yaml.FoldedStyle:
		keyIndent := keyNode.Column - 1
		startLine := v.Line + 1
		if startLine > len(lines) {
			return textSpan{}, false
		}
		blockIndent := -1
		endLine := startLine - 1
		for ln := startLine; ln <= len(lines); ln++ {
			text := lines[ln-1]
			trimmed := strings.TrimLeft(text, " ")
			if trimmed == "" {
				if blockIndent >= 0 {
					endLine = ln
				}
				continue
			}
			indent := len(text) - len(trimmed)
			if blockIndent < 0 {
				if indent <= keyIndent {
					break
				}
				blockIndent = indent
			} else if indent < blockIndent {
				break
			}
			endLine = ln
		}
		if endLine < startLine {
			return textSpan{}, false
		}
		return textSpan{
			startLine: startLine,
			startCol:  1,
			endLine:   endLine,
			endCol:    utf8.RuneCountInString(lines[endLine-1]) + 1,
		}, true
	default:
		// Plain scalar: starts at (v.Line, v.Column) and continues on subsequent
		// lines indented past keyNode.Column until a YAML comment or dedent.
		keyIndent := keyNode.Column - 1
		endLine := v.Line
		endCol := plainScalarLineEndCol(lines[v.Line-1], v.Column)
		for ln := v.Line + 1; ln <= len(lines); ln++ {
			text := lines[ln-1]
			trimmed := strings.TrimLeft(text, " ")
			if trimmed == "" {
				continue
			}
			indent := len(text) - len(trimmed)
			if indent <= keyIndent || strings.HasPrefix(trimmed, "#") {
				break
			}
			// Stop if a new mapping key appears.
			if strings.Contains(trimmed, ": ") || strings.HasSuffix(trimmed, ":") {
				break
			}
			endLine = ln
			endCol = plainScalarLineEndCol(text, indent+1)
		}
		return textSpan{
			startLine: v.Line,
			startCol:  v.Column,
			endLine:   endLine,
			endCol:    endCol,
		}, true
	}
}

func scanQuotedSpan(lines []string, startLine, startCol int, quote rune, backslashEscape bool) (textSpan, bool) {
	opened := false
	escaped := false
	for ln := startLine; ln <= len(lines); ln++ {
		runes := []rune(lines[ln-1])
		c := 0
		if ln == startLine {
			c = startCol - 1
		}
		for ; c < len(runes); c++ {
			r := runes[c]
			if !opened {
				if r == quote {
					opened = true
				}
				continue
			}
			if backslashEscape {
				if escaped {
					escaped = false
					continue
				}
				if r == '\\' {
					escaped = true
					continue
				}
				if r == quote {
					return textSpan{startLine: startLine, startCol: startCol + 1, endLine: ln, endCol: c}, true
				}
			} else {
				if r == quote {
					if c+1 < len(runes) && runes[c+1] == quote {
						c++
						continue
					}
					return textSpan{startLine: startLine, startCol: startCol + 1, endLine: ln, endCol: c}, true
				}
			}
		}
	}
	return textSpan{}, false
}

func plainScalarLineEndCol(line string, startCol int) int {
	runes := []rune(line)
	for i := max(0, startCol-1); i < len(runes); i++ {
		if runes[i] == '#' && (i == 0 || runes[i-1] == ' ' || runes[i-1] == '\t') {
			return i
		}
	}
	return len(runes)
}

type goFileCheckContext struct {
	wantPkg        string
	modulePath     string
	compImportPath string
	compImports    map[string]bool
	refusedFiles   map[string]string
	allDirs        map[string]bool
}

func checkComponentGoFile(relPath string, data []byte, gctx goFileCheckContext) (errs []componentFinding, flags []componentFinding, pkgOK bool, modImps []string) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, relPath, data, parser.ParseComments)
	if err != nil {
		var list scanner.ErrorList
		if errors.As(err, &list) {
			for _, e := range list {
				errs = append(errs, componentFinding{
					File:    relPath,
					Line:    e.Pos.Line,
					Column:  e.Pos.Column,
					Message: "Go parse error: " + e.Msg,
				})
			}
		} else {
			errs = append(errs, componentFinding{
				File:    relPath,
				Message: "Go parse error: " + err.Error(),
			})
		}
		return errs, flags, false, nil
	}

	dir := strings.ToLower(path.Dir(relPath))
	base := strings.ToLower(path.Base(relPath))
	isTest := strings.HasSuffix(base, "_test.go")

	// Package declaration check.
	expectedPkg := gctx.wantPkg
	if dir == "locales" {
		expectedPkg = "locales"
	}
	if expectedPkg != "" {
		gotPkg := file.Name.Name
		if gotPkg == "main" {
			pos := fset.Position(file.Name.Pos())
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    pos.Line,
				Message: `declares package "main"; a component cannot be a main package`,
			})
		} else {
			validPkg := gotPkg == expectedPkg || (isTest && gotPkg == expectedPkg+"_test")
			if !validPkg {
				pos := fset.Position(file.Name.Pos())
				errs = append(errs, componentFinding{
					File:    relPath,
					Line:    pos.Line,
					Message: fmt.Sprintf("declares package %q; want %q", gotPkg, expectedPkg),
				})
			} else {
				pkgOK = true
			}
		}
	}

	// Comments: build constraints, //go:generate, //go:embed and flagged directives.
	for _, cg := range file.Comments {
		for _, c := range cg.List {
			pos := fset.Position(c.Pos())
			text := c.Text

			// Build constraints (only before the package clause).
			if c.Pos() < file.Package {
				if constraint.IsGoBuild(text) || constraint.IsPlusBuild(text) {
					expr, perr := constraint.Parse(text)
					if perr != nil {
						errs = append(errs, componentFinding{
							File:    relPath,
							Line:    pos.Line,
							Message: fmt.Sprintf("invalid build constraint: %v", perr),
						})
					} else if constraintRequiresIgnore(expr) {
						errs = append(errs, componentFinding{
							File:    relPath,
							Line:    pos.Line,
							Message: "build constraint ignores this file (//go:build ignore is not allowed)",
						})
					}
				}
			}

			// //go:generate directive.
			if rest, ok := cutGoDirective(text, "//go:generate"); ok {
				if dir != "locales" || !isAllowedGenerateCommand(rest) {
					errs = append(errs, componentFinding{
						File:    relPath,
						Line:    pos.Line,
						Message: "only //go:generate go tool nsgo i18n (with optional -source <tag>) in locales/ is allowed",
					})
				}
				continue
			}

			// //go:embed directive.
			if rest, ok := cutGoDirective(text, "//go:embed"); ok {
				embedErrs := checkGoEmbedDirective(relPath, pos.Line, rest, gctx.refusedFiles, gctx.allDirs)
				errs = append(errs, embedErrs...)
				continue
			}

			// Flagged compiler directives.
			if directive := flaggedGoDirective(text); directive != "" {
				flags = append(flags, componentFinding{
					File:    relPath,
					Line:    pos.Line,
					Message: fmt.Sprintf("uses directive %s", directive),
				})
			}
		}
	}

	// Imports: C, internal, tsx outside tests, flagged imports, same-module imports.
	for _, imp := range file.Imports {
		impPath, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		pos := fset.Position(imp.Pos())
		switch {
		case impPath == "C":
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    pos.Line,
				Message: `import "C" (cgo) is not allowed in a component`,
			})
		case hasInternalSegment(impPath):
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    pos.Line,
				Message: fmt.Sprintf("imports internal package %q, which a copy outside the module cannot reach", impPath),
			})
		case !isTest && impPath == nsgoModule+"/tsx":
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    pos.Line,
				Message: fmt.Sprintf("imports %q outside tests; apps register copied components for TSX themselves", impPath),
			})
		default:
			if isFlaggedImport(impPath) {
				flags = append(flags, componentFinding{
					File:    relPath,
					Line:    pos.Line,
					Message: fmt.Sprintf("imports %q", impPath),
				})
			}
			if gctx.modulePath != "" && (impPath == gctx.modulePath || strings.HasPrefix(impPath, gctx.modulePath+"/")) {
				if impPath != gctx.compImportPath && impPath != gctx.compImportPath+"/locales" && !gctx.compImports[impPath] {
					modImps = append(modImps, impPath)
				}
			}
		}
	}

	// Non-ASCII identifiers (flags).
	ast.Inspect(file, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || ident.Name == "" {
			return true
		}
		if hasNonASCII(ident.Name) {
			pos := fset.Position(ident.Pos())
			flags = append(flags, componentFinding{
				File:    relPath,
				Line:    pos.Line,
				Column:  pos.Column,
				Message: fmt.Sprintf("identifier %q contains non-ASCII characters", ident.Name),
			})
		}
		return true
	})

	return errs, flags, pkgOK, modImps
}

func cutGoDirective(comment, directive string) (string, bool) {
	if !strings.HasPrefix(comment, directive) {
		return "", false
	}
	rest := comment[len(directive):]
	if rest == "" {
		return "", true
	}
	if rest[0] == ' ' || rest[0] == '\t' {
		return rest, true
	}
	return "", false
}

func flaggedGoDirective(comment string) string {
	if !strings.HasPrefix(comment, "//go:") {
		return ""
	}
	end := len(comment)
	for i := len("//go:"); i < len(comment); i++ {
		if comment[i] == ' ' || comment[i] == '\t' {
			end = i
			break
		}
	}
	name := comment[:end]
	switch name {
	case "//go:linkname", "//go:wasmimport", "//go:wasmexport", "//go:debug":
		return name
	}
	if strings.HasPrefix(name, "//go:cgo_") {
		return name
	}
	return ""
}

func isFlaggedImport(impPath string) bool {
	switch impPath {
	case "unsafe", "syscall", "os/exec", "plugin":
		return true
	}
	return impPath == "golang.org/x/sys" || strings.HasPrefix(impPath, "golang.org/x/sys/") ||
		impPath == "github.com/ebitengine/purego" || strings.HasPrefix(impPath, "github.com/ebitengine/purego/")
}

func hasInternalSegment(impPath string) bool {
	return impPath == "internal" ||
		strings.HasPrefix(impPath, "internal/") ||
		strings.HasSuffix(impPath, "/internal") ||
		strings.Contains(impPath, "/internal/")
}

func hasNonASCII(s string) bool {
	for i := range len(s) {
		if s[i] >= 0x80 {
			return true
		}
	}
	return false
}

// isAllowedGenerateCommand reports whether rest (the arguments after
// "//go:generate") is "go tool nsgo i18n" with an optional "-source <tag>".
func isAllowedGenerateCommand(rest string) bool {
	if strings.ContainsAny(rest, "\"'`$;|&<>()\\") {
		return false
	}
	fields := strings.Fields(rest)
	if len(fields) < 4 || fields[0] != "go" || fields[1] != "tool" || fields[2] != "nsgo" || fields[3] != "i18n" {
		return false
	}
	switch len(fields) {
	case 4:
		return true
	case 5:
		tag, ok := strings.CutPrefix(fields[4], "-source=")
		return ok && isValidBCP47Tag(tag)
	case 6:
		return fields[4] == "-source" && isValidBCP47Tag(fields[5])
	default:
		return false
	}
}

func isValidBCP47Tag(tag string) bool {
	if tag == "" {
		return false
	}
	t, err := language.Parse(tag)
	return err == nil && !t.IsRoot() && t.String() == tag
}

// constraintRequiresIgnore reports whether expr can never be satisfied unless
// the "ignore" build tag is set.
func constraintRequiresIgnore(expr constraint.Expr) bool {
	tags := map[string]bool{}
	var collect func(constraint.Expr)
	collect = func(e constraint.Expr) {
		switch e := e.(type) {
		case *constraint.TagExpr:
			tags[e.Tag] = true
		case *constraint.NotExpr:
			collect(e.X)
		case *constraint.AndExpr:
			collect(e.X)
			collect(e.Y)
		case *constraint.OrExpr:
			collect(e.X)
			collect(e.Y)
		}
	}
	collect(expr)
	if !tags["ignore"] {
		return false
	}
	delete(tags, "ignore")
	others := make([]string, 0, len(tags))
	for t := range tags {
		others = append(others, t)
	}
	if len(others) <= 16 {
		total := 1 << len(others)
		for mask := range total {
			if expr.Eval(func(tag string) bool {
				if tag == "ignore" {
					return false
				}
				i := slices.Index(others, tag)
				return i >= 0 && (mask&(1<<i)) != 0
			}) {
				return false
			}
		}
		return true
	}
	// Structural fallback for expressions with many tags.
	var structural func(constraint.Expr) bool
	structural = func(e constraint.Expr) bool {
		switch e := e.(type) {
		case *constraint.TagExpr:
			return e.Tag == "ignore"
		case *constraint.AndExpr:
			return structural(e.X) || structural(e.Y)
		case *constraint.OrExpr:
			return structural(e.X) && structural(e.Y)
		default:
			return false
		}
	}
	return structural(expr)
}

// checkGoEmbedDirective parses patterns in a //go:embed directive and refuses
// any pattern that escapes the package directory, matches nsgo-copy.yaml,
// matches any refused file in the component, or explicitly names a disallowed file.
func checkGoEmbedDirective(relPath string, line int, rest string, refusedFiles map[string]string, allDirs map[string]bool) []componentFinding {
	patterns, err := parseGoEmbedPatterns(rest)
	if err != nil {
		return []componentFinding{{
			File:    relPath,
			Line:    line,
			Message: fmt.Sprintf("invalid //go:embed directive: %v", err),
		}}
	}
	pkgDir := path.Dir(relPath) // "." or "locales"
	var errs []componentFinding

	// Sort refusedFiles keys so diagnostics are deterministic.
	refusedList := make([]string, 0, len(refusedFiles))
	for rf := range refusedFiles {
		refusedList = append(refusedList, rf)
	}
	slices.Sort(refusedList)

	for _, rawPat := range patterns {
		includeHidden := false
		pat := rawPat
		if trimmed, ok := strings.CutPrefix(pat, "all:"); ok {
			includeHidden = true
			pat = trimmed
		}
		if pat == "" || strings.HasPrefix(pat, "/") || strings.Contains(pat, "\\") {
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    line,
				Message: fmt.Sprintf("//go:embed pattern %q is invalid or escapes the package directory", rawPat),
			})
			continue
		}
		if slices.Contains(strings.Split(pat, "/"), "..") {
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    line,
				Message: fmt.Sprintf("//go:embed pattern %q escapes the package directory", rawPat),
			})
			continue
		}
		cleaned := path.Clean(pat)
		if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    line,
				Message: fmt.Sprintf("//go:embed pattern %q matches the package directory (including nsgo-copy.yaml)", rawPat),
			})
			continue
		}
		if _, err := path.Match(cleaned, ""); err != nil {
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    line,
				Message: fmt.Sprintf("invalid //go:embed pattern %q: %v", rawPat, err),
			})
			continue
		}

		// Does it match nsgo-copy.yaml at the component root?
		if pkgDir == "." && embedPatternMatches(cleaned, includeHidden, "nsgo-copy.yaml") {
			errs = append(errs, componentFinding{
				File:    relPath,
				Line:    line,
				Message: fmt.Sprintf("//go:embed pattern %q matches nsgo-copy.yaml", rawPat),
			})
			continue
		}

		// Does it match any refused file currently in the component?
		matchedRefused := false
		for _, rf := range refusedList {
			target := rf
			if pkgDir != "." {
				sub, ok := strings.CutPrefix(rf, pkgDir+"/")
				if !ok {
					continue
				}
				target = sub
			}
			if embedPatternMatches(cleaned, includeHidden, target) {
				errs = append(errs, componentFinding{
					File:    relPath,
					Line:    line,
					Message: fmt.Sprintf("//go:embed pattern %q matches refused file %s", rawPat, rf),
				})
				matchedRefused = true
			}
		}
		if matchedRefused {
			continue
		}

		// Synthetic check when the pattern itself explicitly names a disallowed file
		// or disallowed extension even if no such file is present yet.
		compRelPat := cleaned
		if pkgDir != "." {
			compRelPat = pkgDir + "/" + cleaned
		}
		if !hasGlobMeta(compRelPat) {
			if !allDirs[compRelPat] {
				if err := componentFileAllowed(compRelPat); err != nil {
					errs = append(errs, componentFinding{
						File:    relPath,
						Line:    line,
						Message: fmt.Sprintf("//go:embed pattern %q targets a refused file (%v)", rawPat, err),
					})
				}
			}
		} else if ext := path.Ext(path.Base(compRelPat)); ext != "" && !hasGlobMeta(ext) {
			synth := synthesizeGlobPath(compRelPat)
			if err := componentFileAllowed(synth); err != nil {
				errs = append(errs, componentFinding{
					File:    relPath,
					Line:    line,
					Message: fmt.Sprintf("//go:embed pattern %q matches refused files (%v)", rawPat, err),
				})
			}
		}
	}
	return errs
}

// parseGoEmbedPatterns splits the arguments of a //go:embed line following Go's
// cmd/go embed pattern syntax (unquoted, double-quoted, or backtick-quoted).
func parseGoEmbedPatterns(s string) ([]string, error) {
	var args []string
	for {
		s = strings.TrimLeft(s, " \t")
		if s == "" {
			break
		}
		switch s[0] {
		case '"', '`':
			quote := s[0]
			i := 1
			for i < len(s) {
				if quote == '"' && s[i] == '\\' {
					i += 2
					continue
				}
				if s[i] == quote {
					break
				}
				i++
			}
			if i >= len(s) {
				return nil, errors.New("unterminated quoted pattern")
			}
			unq, err := strconv.Unquote(s[:i+1])
			if err != nil {
				return nil, err
			}
			s = s[i+1:]
			if s != "" && s[0] != ' ' && s[0] != '\t' {
				return nil, errors.New("missing space after quoted pattern")
			}
			args = append(args, unq)
		default:
			i := 0
			for i < len(s) && s[i] != ' ' && s[i] != '\t' {
				i++
			}
			args = append(args, s[:i])
			s = s[i:]
		}
	}
	if len(args) == 0 {
		return nil, errors.New("missing pattern")
	}
	return args, nil
}

// embedPatternMatches reports whether a Go //go:embed pattern (with "all:"
// stripped into includeHidden) matches target (clean slash-separated path
// relative to the package directory).
func embedPatternMatches(pat string, includeHidden bool, target string) bool {
	if !includeHidden && hasHiddenEmbedSegment(target) {
		// Note: Go allows a pattern that explicitly names a leading '.' or '_'
		// segment, e.g. //go:embed .hidden, so direct match still applies below
		// when pat itself matches target directly.
		if matched, _ := path.Match(pat, target); matched {
			return !globMatchedHidden(pat, target)
		}
		return false
	}
	if matched, _ := path.Match(pat, target); matched {
		return true
	}
	// If pat matches any parent directory of target, embedding that directory
	// recursively embeds target (skipping '.'/'_' segments unless includeHidden).
	for dir := path.Dir(target); dir != "." && dir != "" && dir != "/"; dir = path.Dir(dir) {
		if matched, _ := path.Match(pat, dir); matched {
			if includeHidden {
				return true
			}
			tail := strings.TrimPrefix(target, dir+"/")
			if !hasHiddenEmbedSegment(tail) && !globMatchedHidden(pat, dir) {
				return true
			}
		}
	}
	return false
}

func hasHiddenEmbedSegment(p string) bool {
	for seg := range strings.SplitSeq(p, "/") {
		if seg != "" && (seg[0] == '.' || seg[0] == '_') {
			return true
		}
	}
	return false
}

// globMatchedHidden reports whether a wildcard segment in pat matched a '.'- or
// '_'-prefixed segment in target (which Go skips without "all:").
func globMatchedHidden(pat, target string) bool {
	pSegs := strings.Split(pat, "/")
	tSegs := strings.Split(target, "/")
	if len(pSegs) != len(tSegs) {
		return hasHiddenEmbedSegment(target)
	}
	for i := range pSegs {
		if tSegs[i] != "" && (tSegs[i][0] == '.' || tSegs[i][0] == '_') {
			if pSegs[i] == "" || pSegs[i][0] != tSegs[i][0] {
				return true
			}
		}
	}
	return false
}

func hasGlobMeta(s string) bool {
	return strings.ContainsAny(s, "*?[")
}

func synthesizeGlobPath(pat string) string {
	var b strings.Builder
	for i := 0; i < len(pat); i++ {
		switch pat[i] {
		case '*', '?':
			b.WriteByte('x')
		case '[':
			end := strings.IndexByte(pat[i+1:], ']')
			if end >= 0 {
				b.WriteByte('x')
				i += end + 1
			} else {
				b.WriteByte('x')
			}
		default:
			b.WriteByte(pat[i])
		}
	}
	return b.String()
}
