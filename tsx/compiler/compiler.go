// Package compiler compiles TSX apps into the scripts the tsx package
// runs, as NSH's bundler does (nsh-bundler): esbuild strips TypeScript,
// lowers syntax goja lacks and bundles the app's modules into one script,
// and jsx.go compiles the JSX into calls of the renderer, as NSH's
// compiler does. Imports of NSGo's modules (nsgo, nsgo/*, nsgo:rt) stay,
// as require calls the runtime answers. No Node is needed.
package compiler

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"strings"
	"unicode"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/go-sourcemap/sourcemap"

	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/tsx"
)

// App returns the app whose TSX entry is entry in fsys, for nsgo.Run and
// nsgotest.Start: it compiles the app as it starts, then runs it.
//
//	//go:embed *.tsx
//	var src embed.FS
//
//	func main() { nsgo.Run(compiler.App(src, "App.tsx")) }
func App(fsys fs.FS, entry string) func(cx *core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		code, err := Compile(fsys, entry)
		if err != nil {
			panic(err)
		}
		return tsx.App(entry, code)(cx)
	}
}

// Compile compiles the app whose entry is entry in fsys, and the modules
// it imports, into one script with its source map inline.
func Compile(fsys fs.FS, entry string) (string, error) {
	entry = path.Clean(entry)
	result := api.Build(api.BuildOptions{
		EntryPoints: []string{entry},
		Bundle:      true,
		Write:       false,
		Format:      api.FormatIIFE,
		Platform:    api.PlatformNeutral,
		Target:      api.ES2020,
		Sourcemap:   api.SourceMapInline,
		Outfile:     "bundle.js",
		Charset:     api.CharsetUTF8,
		LogLevel:    api.LogLevelSilent,
		Plugins:     []api.Plugin{appPlugin(fsys)},
	})
	if len(result.Errors) > 0 {
		return "", messages(result.Errors)
	}
	for _, f := range result.OutputFiles {
		if strings.HasSuffix(f.Path, ".js") {
			return string(f.Contents), nil
		}
	}
	return "", errors.New("compiler: esbuild wrote no script")
}

// isNative reports whether an import names one of NSGo's modules, which
// the runtime serves.
func isNative(p string) bool {
	return p == "nsgo" || strings.HasPrefix(p, "nsgo/") || strings.HasPrefix(p, "nsgo:")
}

// sources are the extensions a module's import may leave out, in order.
var sources = []string{".tsx", ".ts", ".jsx", ".js", ".mjs"}

// appPlugin resolves and loads the app's modules from fsys, compiling
// each one's JSX.
func appPlugin(fsys fs.FS) api.Plugin {
	return api.Plugin{Name: "nsgo", Setup: func(b api.PluginBuild) {
		b.OnResolve(api.OnResolveOptions{Filter: ".*"}, func(args api.OnResolveArgs) (api.OnResolveResult, error) {
			if isNative(args.Path) {
				return api.OnResolveResult{Path: args.Path, External: true}, nil
			}
			var p string
			switch {
			case args.Kind == api.ResolveEntryPoint:
				p = path.Clean(args.Path)
			case strings.HasPrefix(args.Path, "./") || strings.HasPrefix(args.Path, "../"):
				p = path.Join(path.Dir(args.Importer), args.Path)
			default:
				return api.OnResolveResult{}, fmt.Errorf("cannot import %q: an app imports its own files and NSGo's modules (nsgo, nsgo/base, ...)", args.Path)
			}
			if resolved, ok := resolve(fsys, p); ok {
				return api.OnResolveResult{Path: resolved, Namespace: "app"}, nil
			}
			return api.OnResolveResult{}, fmt.Errorf("cannot find %q", args.Path)
		})
		b.OnLoad(api.OnLoadOptions{Filter: ".*", Namespace: "app"}, func(args api.OnLoadArgs) (api.OnLoadResult, error) {
			data, err := fs.ReadFile(fsys, args.Path)
			if err != nil {
				return api.OnLoadResult{}, err
			}
			src := string(data)
			if strings.HasSuffix(args.Path, ".json") {
				return api.OnLoadResult{Contents: &src, Loader: api.LoaderJSON}, nil
			}
			code, err := Transform(args.Path, src)
			if err != nil {
				var list []api.Message
				for _, e := range unwrap(err) {
					var ce *Error
					if errors.As(e, &ce) {
						list = append(list, api.Message{Text: ce.Message,
							Location: &api.Location{File: ce.File, Line: ce.Line, Column: ce.Column - 1}})
					} else {
						list = append(list, api.Message{Text: e.Error()})
					}
				}
				return api.OnLoadResult{Errors: list}, nil
			}
			return api.OnLoadResult{Contents: &code, Loader: api.LoaderJS}, nil
		})
	}}
}

// resolve finds the module p names: the file, or it with an extension,
// or its index.
func resolve(fsys fs.FS, p string) (string, bool) {
	var candidates []string
	if fi, err := fs.Stat(fsys, p); err == nil && !fi.IsDir() {
		return p, true
	}
	for _, ext := range append(sources, ".json") {
		candidates = append(candidates, p+ext)
	}
	for _, ext := range sources {
		candidates = append(candidates, path.Join(p, "index"+ext))
	}
	for _, c := range candidates {
		if fi, err := fs.Stat(fsys, c); err == nil && !fi.IsDir() {
			return c, true
		}
	}
	return "", false
}

// Transform compiles one module: its TypeScript stripped and its JSX
// compiled, as CommonJS with its source map inline.
func Transform(file, src string) (string, error) {
	loader := api.LoaderTS
	switch path.Ext(file) {
	case ".tsx":
		loader = api.LoaderTSX
	case ".jsx":
		loader = api.LoaderJSX
	case ".js", ".mjs":
		loader = api.LoaderJS
	}
	r := api.Transform(src, api.TransformOptions{
		Loader:      loader,
		JSX:         api.JSXTransform,
		JSXFactory:  jsxFactory,
		JSXFragment: jsxFragment,
		Format:      api.FormatCommonJS,
		Target:      api.ES2020,
		Sourcemap:   api.SourceMapExternal,
		Sourcefile:  file,
		Charset:     api.CharsetUTF8,
		LogLevel:    api.LogLevelSilent,
	})
	if len(r.Errors) > 0 {
		return "", messages(r.Errors)
	}
	code := string(r.Code)
	if !strings.Contains(code, jsxFactory) {
		return inline(code, string(r.Map)), nil
	}
	prev, err := sourcemap.Parse(file, r.Map)
	if err != nil {
		return "", fmt.Errorf("%s: %w", file, err)
	}
	pos := newPositions(code)
	original := newPositions(src)
	at := func(off int) (int, int) {
		l, c := pos.at(off)
		if _, _, ol, oc, ok := prev.Source(l+1, c); ok {
			// What it points at, past the spaces before it.
			l, c := original.at(skipSpace(src, original.offset(ol-1, oc)))
			return l + 1, c + 1
		}
		return l + 1, c + 1
	}
	f, err := lower(file, code, at, ambient(src))
	if err != nil {
		return "", err
	}
	out, m := f.mapped(pos, prev, file, src)
	return inline(out, m), nil
}

// skipSpace returns the offset of the first character from i on that is
// not a space.
func skipSpace(s string, i int) int {
	for i < len(s) && isSpace(s[i]) {
		i++
	}
	return i
}

// declareRE finds TypeScript's ambient declarations, which the transform
// removes but which name what the module's code uses.
var declareRE = regexp.MustCompile(`\bdeclare\s+(?:const|let|var|function|class)\s+([^;\n]*)`)

// ambient returns the names src declares with declare const, let, var,
// function and class.
func ambient(src string) []string {
	var names []string
	for _, m := range declareRE.FindAllStringSubmatch(src, -1) {
		depth := 0
		start := true
		for i, r := range m[1] {
			switch {
			case r == '(' || r == '<' || r == '{' || r == '[':
				depth++
			case r == ')' || r == '>' || r == '}' || r == ']':
				depth--
			case r == ',' && depth == 0:
				start = true
			case start && (r == '_' || r == '$' || unicode.IsLetter(r)):
				end := i
				for end < len(m[1]) && (m[1][end] == '_' || m[1][end] == '$' || unicode.IsLetter(rune(m[1][end])) || unicode.IsDigit(rune(m[1][end]))) {
					end++
				}
				names = append(names, m[1][i:end])
				start = false
			case start && !unicode.IsSpace(r):
				start = false
			}
		}
	}
	return names
}

// messages makes esbuild's messages one error.
func messages(ms []api.Message) error {
	errs := make([]error, len(ms))
	for i, m := range ms {
		if loc := m.Location; loc != nil {
			errs[i] = &Error{File: loc.File, Line: loc.Line, Column: loc.Column + 1, Message: m.Text}
		} else {
			errs[i] = errors.New(m.Text)
		}
	}
	return joinErrors(errs)
}

func joinErrors(errs []error) error {
	if len(errs) == 1 {
		return errs[0]
	}
	return errors.Join(errs...)
}

// unwrap returns the errors err joins, or err.
func unwrap(err error) []error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		return j.Unwrap()
	}
	return []error{err}
}
