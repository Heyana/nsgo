package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// planRemove runs nsgo remove -dry-run with args in the module at dir and returns the plan it prints.
func planRemove(t *testing.T, dir string, args ...string) (string, error) {
	t.Helper()
	return runRemove(t, dir, removeOptions{dryRun: true}, nil, args...)
}

// runRemove runs nsgo remove with o and args in the module at dir, asking ask, and returns what it prints on standard output.
func runRemove(t *testing.T, dir string, o removeOptions, ask func(context.Context) (bool, error), args ...string) (string, error) {
	t.Helper()
	t.Chdir(dir)
	var out bytes.Buffer
	var err error
	captureStderr(t, func() { err = removeModules(t.Context(), o, args, &out, ask) })
	return out.String(), err
}

// goModTidyWrites copies the module at dir, runs go mod tidy in the copy outside any workspace, and returns the go.mod and go.sum it writes.
func goModTidyWrites(t *testing.T, dir string) (gomod, gosum []byte) {
	t.Helper()
	copied := t.TempDir()
	if err := os.CopyFS(copied, os.DirFS(dir)); err != nil {
		t.Fatal(err)
	}
	goIn(t, copied, "mod", "tidy")
	return moduleFiles(t, copied)
}

// After add, importing, deleting the import, then remove, go.mod and go.sum equal what go mod tidy writes, and the versions the add raised stay (#2465).
func TestRemoveAfterAddEqualsGoModTidyAndKeepsRaisedVersions(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart"); err != nil {
		t.Fatal(err)
	}
	// Import the library, tidy, then delete the import as a user does before removing it.
	writeFiles(t, dir, map[string]string{
		"main.go": "package main\n\nimport _ \"github.com/acme/chart\"\n\nfunc main() {}\n",
	})
	goIn(t, dir, "mod", "tidy")
	writeFiles(t, dir, map[string]string{
		"main.go": "package main\n\nfunc main() {}\n",
	})

	oldMod, oldSum := moduleFiles(t, dir)
	plan, err := planRemove(t, dir, "github.com/acme/chart")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	const want = `github.com/acme/chart
  module     remove github.com/acme/chart v1.2.0

example.com/app
  go.mod     - require github.com/acme/chart v1.2.0
             - require example.com/dep v1.0.0
  go.sum     - 4 lines
`
	if plan != want {
		t.Errorf("plan:\n%s\nwant:\n%s", plan, want)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Fatalf("-dry-run wrote go.mod or go.sum:\n%s\n%s", gomod, gosum)
	}

	wantMod, wantSum := goModTidyWrites(t, dir)
	if _, err := runRemove(t, dir, removeOptions{yes: true}, nil, "github.com/acme/chart"); err != nil {
		t.Fatal(err)
	}
	gomod, gosum := moduleFiles(t, dir)
	if !bytes.Equal(gomod, wantMod) || !bytes.Equal(gosum, wantSum) {
		t.Errorf("go.mod:\n%s\ngo.sum:\n%s\nwant what go mod tidy writes:\n%s\n%s", gomod, gosum, wantMod, wantSum)
	}
	// The NSGo version that add raised from v0.3.4 to v0.4.0 must stay at v0.4.0.
	parsed, err := modfile.Parse("go.mod", gomod, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := requiredVersion(parsed, nsgoModule); got != "v0.4.0" {
		t.Errorf("nsgo version after remove = %q, want v0.4.0", got)
	}
	if got := requiredVersion(parsed, "github.com/acme/chart"); got != "" {
		t.Errorf("chart still in go.mod at %q", got)
	}
}

// An import only in an _ios.go file (or in tests, or in another app package in the same module) is refused, naming its package (#2465).
func TestRemoveRefusesStillImported(t *testing.T) {
	offlineAdd(t, libraryProxy(t))

	t.Run("ios build tag", func(t *testing.T) {
		dir := nsgoApp(t)
		if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart"); err != nil {
			t.Fatal(err)
		}
		writeFiles(t, dir, map[string]string{
			"main.go":              "package main\n\nfunc main() {}\n",
			"iosonly/chart_ios.go": "package iosonly\n\nimport _ \"github.com/acme/chart\"\n",
		})
		goIn(t, dir, "mod", "tidy")
		oldMod, oldSum := moduleFiles(t, dir)

		for _, o := range []removeOptions{{dryRun: true}, {yes: true}} {
			_, err := runRemove(t, dir, o, nil, "github.com/acme/chart")
			want := "github.com/acme/chart is still imported by example.com/app/iosonly"
			if err == nil || !strings.Contains(err.Error(), want) {
				t.Errorf("%+v: %v, want %q", o, err, want)
			}
			if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
				t.Fatalf("%+v: go.mod or go.sum changed", o)
			}
		}
	})

	t.Run("test file", func(t *testing.T) {
		dir := nsgoApp(t)
		if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "example.com/dep"); err != nil {
			t.Fatal(err)
		}
		writeFiles(t, dir, map[string]string{
			"pkg/pkg.go":      "package pkg\n",
			"pkg/pkg_test.go": "package pkg_test\n\nimport _ \"example.com/dep\"\n",
		})
		goIn(t, dir, "mod", "tidy")

		_, err := planRemove(t, dir, "example.com/dep")
		want := "example.com/dep is still imported by example.com/app/pkg (tests)"
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("got %v, want %q", err, want)
		}
	})
}

// A library another dependency needs stays, and the plan says why; a library whose tool go.mod declares also stays and says why (#2465).
func TestRemoveKeptByDependencyOrTool(t *testing.T) {
	offlineAdd(t, libraryProxy(t))

	t.Run("needed by another dependency", func(t *testing.T) {
		dir := nsgoApp(t)
		if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart", "example.com/dep"); err != nil {
			t.Fatal(err)
		}
		// Import both, tidy so dep is direct, then remove only the app's import of dep while keeping chart.
		writeFiles(t, dir, map[string]string{
			"main.go": "package main\n\nimport (\n\t_ \"github.com/acme/chart\"\n\t_ \"example.com/dep\"\n)\n\nfunc main() {}\n",
		})
		goIn(t, dir, "mod", "tidy")
		writeFiles(t, dir, map[string]string{
			"main.go": "package main\n\nimport _ \"github.com/acme/chart\"\n\nfunc main() {}\n",
		})

		plan, err := planRemove(t, dir, "example.com/dep")
		if err != nil {
			t.Fatalf("plan: %v\n%s", err, plan)
		}
		const want = `example.com/dep
  module     keep example.com/dep v1.0.0
  kept       needed by github.com/acme/chart

example.com/app
  go.mod     ~ require example.com/dep v1.0.0 // indirect, was direct
`
		if plan != want {
			t.Errorf("plan:\n%s\nwant:\n%s", plan, want)
		}

		// Apply updates dep to // indirect without dropping or downgrading anything.
		if _, err := runRemove(t, dir, removeOptions{yes: true}, nil, "example.com/dep"); err != nil {
			t.Fatal(err)
		}
		// Running remove again when already indirect changes nothing and still explains why dep stays.
		oldMod, oldSum := moduleFiles(t, dir)
		plan2, err := runRemove(t, dir, removeOptions{}, nil, "example.com/dep")
		if err != nil {
			t.Fatalf("second remove: %v\n%s", err, plan2)
		}
		const want2 = `example.com/dep
  module     keep example.com/dep v1.0.0
  kept       needed by github.com/acme/chart
`
		if plan2 != want2 {
			t.Errorf("second plan:\n%s\nwant:\n%s", plan2, want2)
		}
		if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
			t.Errorf("second remove wrote go.mod or go.sum")
		}
	})

	t.Run("declared tool", func(t *testing.T) {
		dir := nsgoApp(t)
		writeFiles(t, dir, map[string]string{"main.go": "package main\n\nfunc main() {}\n"})
		oldMod, oldSum := moduleFiles(t, dir)

		plan, err := runRemove(t, dir, removeOptions{}, nil, nsgoModule)
		if err != nil {
			t.Fatalf("plan: %v\n%s", err, plan)
		}
		const want = `github.com/nsgo-dev/nsgo
  module     keep github.com/nsgo-dev/nsgo v0.3.4
  kept       go.mod declares tool github.com/nsgo-dev/nsgo/cmd/nsgo
`
		if plan != want {
			t.Errorf("plan:\n%s\nwant:\n%s", plan, want)
		}
		if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
			t.Errorf("tool-kept remove wrote go.mod or go.sum")
		}
	})
}

// Lines unrelated to the removed library are marked as also tidied, and any replace of the removed library is mentioned as left in place (#2465).
func TestRemoveMarksAlsoTidiedAndMentionsReplace(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	local := filepath.Join(t.TempDir(), "localdep")
	writeFiles(t, local, map[string]string{
		"go.mod": "module example.com/localdep\n\ngo 1.26\n",
		"dep.go": "package localdep\n",
	})
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart", "example.com/std"); err != nil {
		t.Fatal(err)
	}
	// Add a replace for a local module as well as an unrelated stray require (example.com/std).
	gomod, _ := moduleFiles(t, dir)
	extra := "\nrequire example.com/localdep v0.0.0\n\nreplace example.com/localdep => " + filepath.ToSlash(local) + "\n"
	writeFiles(t, dir, map[string]string{
		"go.mod":  string(gomod) + extra,
		"main.go": "package main\n\nfunc main() {}\n",
	})

	plan, err := planRemove(t, dir, "github.com/acme/chart", "example.com/localdep")
	if err != nil {
		t.Fatalf("plan: %v\n%s", err, plan)
	}
	for _, want := range []string{
		"  module     remove github.com/acme/chart v1.2.0\n",
		"  module     remove example.com/localdep v0.0.0\n",
		"  replaced   go.mod still replaces it with " + filepath.ToSlash(local) + ", left in place\n",
		"- require github.com/acme/chart v1.2.0\n",
		"- require example.com/localdep v0.0.0\n",
		"- require example.com/dep v1.0.0\n",
		"- require example.com/std v1.0.0, also tidied\n",
	} {
		if !strings.Contains(plan, want) {
			t.Errorf("plan lacks %q:\n%s", want, plan)
		}
	}
	// example.com/dep was a dependency of chart, so it must NOT be marked as also tidied.
	if strings.Contains(plan, "example.com/dep v1.0.0, also tidied") {
		t.Errorf("chart's own dependency was marked also tidied:\n%s", plan)
	}
}

// -dry-run, -y, interactive confirmation, and the case without a terminal behave as they do for add (#2465).
func TestRemoveConfirmationDryRunAndNoTerminal(t *testing.T) {
	proxy := libraryProxy(t)
	path, env := goEnv(t, "file://"+filepath.ToSlash(proxy))
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"go.mod":  "module example.com/tool\n\ngo 1.26\n",
		"main.go": "package main\n\nfunc main() {}\n",
	})
	if out, code := runNsgo(t, dir, path, env, "add", "-y", "example.com/dep"); code != 0 {
		t.Fatalf("add: exit %d\n%s", code, out)
	}
	oldMod, oldSum := moduleFiles(t, dir)

	// Without a terminal and without -y, nothing is written and exit status is 1.
	out, code := runNsgo(t, dir, path, env, "remove", "example.com/dep")
	if code != 1 || !strings.Contains(out, "- require example.com/dep v1.0.0") || !strings.Contains(out, "the plan was not applied") || !strings.Contains(out, "remove -y example.com/dep") {
		t.Errorf("no terminal: exit %d\n%s", code, out)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("no terminal wrote go.mod or go.sum")
	}

	// -dry-run prints the plan, writes nothing and exits 0.
	if out, code := runNsgo(t, dir, path, env, "remove", "-dry-run", "example.com/dep"); code != 0 || !strings.Contains(out, "- require example.com/dep v1.0.0") {
		t.Errorf("-dry-run: exit %d\n%s", code, out)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("-dry-run wrote go.mod or go.sum")
	}

	// Interactive decline leaves files untouched; interactive confirm applies.
	offlineAdd(t, proxy)
	no := func(context.Context) (bool, error) { return false, nil }
	if _, err := runRemove(t, dir, removeOptions{}, no, "example.com/dep"); err == nil || !strings.Contains(err.Error(), "the plan was not applied") {
		t.Errorf("declined: %v", err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("declined wrote go.mod or go.sum")
	}

	wantMod, wantSum := goModTidyWrites(t, dir)
	yes := func(context.Context) (bool, error) { return true, nil }
	if _, err := runRemove(t, dir, removeOptions{}, yes, "example.com/dep"); err != nil {
		t.Fatal(err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, wantMod) || !bytes.Equal(gosum, wantSum) {
		t.Errorf("applied:\n%s\n%s\nwant:\n%s\n%s", gomod, gosum, wantMod, wantSum)
	}
}

// A component's name is refused before anything is planned, and bare built-in and @registry/name library names resolve and remove their modules (#2461, #2464, #2465).
func TestRemoveRefusesComponentAndResolvesName(t *testing.T) {
	idx := &registryIndex{
		Version: registryIndexVersion,
		Items: []registryItem{
			{Name: "chat", Module: nsgoModule, Component: "chat", Description: "Chat component.", License: "Apache-2.0"},
			{Name: "chart", Module: "github.com/acme/chart", Description: "Chart library.", License: "MIT"},
		},
	}

	runWithIdx := func(dir string, o removeOptions, args ...string) (string, error) {
		t.Helper()
		t.Chdir(dir)
		var out bytes.Buffer
		var err error
		captureStderr(t, func() { err = removeModulesWithIndex(t.Context(), o, args, idx, &out, nil) })
		return out.String(), err
	}

	// Even outside a module, a built-in component name is refused immediately without running go.
	dir := t.TempDir()
	plan, err := runWithIdx(dir, removeOptions{yes: true}, "chat")
	const wantErr = "chat is a component, copied into this module; delete its copy's directory"
	if err == nil || !strings.Contains(err.Error(), wantErr) {
		t.Fatalf("component refusal: %v, want %q", err, wantErr)
	}
	if plan != "" {
		t.Errorf("component refusal planned output:\n%s", plan)
	}

	// A bare built-in library name plans and removes its module.
	offlineAdd(t, libraryProxy(t))
	appDir := nsgoApp(t)
	if _, err := runAdd(t, appDir, addOptions{yes: true}, nil, "github.com/acme/chart"); err != nil {
		t.Fatal(err)
	}
	writeFiles(t, appDir, map[string]string{"main.go": "package main\n\nfunc main() {}\n"})
	plan, err = runWithIdx(appDir, removeOptions{dryRun: true}, "chart")
	if err != nil || !strings.Contains(plan, "- require github.com/acme/chart v1.2.0") {
		t.Fatalf("resolved library plan: %v\n%s", err, plan)
	}

	// Custom registries in nsgo.yaml resolve @registry/name, print their registry header line, refuse components, and never shadow bare names.
	nsgoYAML := "name: App\nregistries:\n  \"@acme\": tools/registry.yaml\n"
	writeFiles(t, appDir, map[string]string{
		"nsgo.yaml": nsgoYAML,
		"tools/registry.yaml": `version: 1
items:
  - name: chart
    module: github.com/acme/chart
    description: Chart library.
    license: MIT
  - name: chat
    module: github.com/nsgo-dev/nsgo
    component: chat
    description: Chat component.
    license: Apache-2.0
`,
	})
	regPlan, err := planRemove(t, appDir, "@acme/chart")
	if err != nil {
		t.Fatalf("@acme/chart plan: %v\n%s", err, regPlan)
	}
	firstLine, _, _ := strings.Cut(regPlan, "\n")
	if want := "registry @acme (tools/registry.yaml)"; firstLine != want {
		t.Errorf("@acme/chart first line = %q, want %q\n%s", firstLine, want, regPlan)
	}
	if !strings.Contains(regPlan, "- require github.com/acme/chart v1.2.0") {
		t.Errorf("@acme/chart plan missing chart removal:\n%s", regPlan)
	}

	// Custom registry component is refused before planning.
	if _, err := planRemove(t, appDir, "@acme/chat"); err == nil || !strings.Contains(err.Error(), "@acme/chat is a component, copied into this module") {
		t.Errorf("@acme/chat = %v, want component refusal", err)
	}

	// Bare name does not fall back to custom registry.
	if _, err := planRemove(t, appDir, "chart"); err == nil || !strings.Contains(err.Error(), `unknown name "chart"`) {
		t.Errorf("bare chart with custom registry = %v, want unknown name error", err)
	}

	// Unconfigured registry fails clearly without modifying nsgo.yaml.
	if _, err := planRemove(t, appDir, "@missing/chart"); err == nil || !strings.Contains(err.Error(), "registry @missing is not configured in nsgo.yaml") {
		t.Errorf("@missing/chart = %v, want unconfigured registry error", err)
	}
	if gotYAML, err := os.ReadFile(filepath.Join(appDir, "nsgo.yaml")); err != nil || string(gotYAML) != nsgoYAML {
		t.Errorf("nsgo.yaml changed after unconfigured registry error:\n%s", gotYAML)
	}

	// Naming the same module via @acme/chart and its module path is refused as duplicate.
	if _, err := planRemove(t, appDir, "@acme/chart", "github.com/acme/chart"); err == nil || !strings.Contains(err.Error(), "github.com/acme/chart is named twice") {
		t.Errorf("duplicate via @acme/chart and path = %v, want named twice error", err)
	}

	// Applying @acme/chart drops github.com/acme/chart.
	if _, err := runRemove(t, appDir, removeOptions{yes: true}, nil, "@acme/chart"); err != nil {
		t.Fatalf("apply @acme/chart: %v", err)
	}
	gomod, _ := moduleFiles(t, appDir)
	parsed, err := modfile.Parse("go.mod", gomod, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := requiredVersion(parsed, "github.com/acme/chart"); got != "" {
		t.Errorf("chart still in go.mod at %q after @acme/chart remove", got)
	}
}

// A module not in go.mod writes nothing and exits 0; a go.mod changed between planning and applying is refused (#2465).
func TestRemoveNotInGoModAndChangedFiles(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	oldMod, oldSum := moduleFiles(t, dir)

	plan, err := runRemove(t, dir, removeOptions{}, nil, "example.com/dep")
	if err != nil || plan != "example.com/dep is not in go.mod\n" {
		t.Fatalf("not in go.mod: %q, %v", plan, err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("not in go.mod changed files")
	}

	// A package path of a required module is refused, naming its module.
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart"); err != nil {
		t.Fatal(err)
	}
	_, err = planRemove(t, dir, "github.com/acme/chart/sub")
	if want := "github.com/acme/chart/sub is a package of the module github.com/acme/chart, not a module: nsgo remove github.com/acme/chart"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("package path: %v, want %q", err, want)
	}

	// Refusal when go.mod changes between planning and applying.
	edit := func(context.Context) (bool, error) {
		data, err := os.ReadFile(filepath.Join(dir, "go.mod"))
		if err != nil {
			return false, err
		}
		return true, os.WriteFile(filepath.Join(dir, "go.mod"), append(data, []byte("// edited\n")...), 0o644)
	}
	if _, err := runRemove(t, dir, removeOptions{}, edit, "github.com/acme/chart"); err == nil || !strings.Contains(err.Error(), "go.mod changed since the plan was made") {
		t.Errorf("edited go.mod: %v", err)
	}
}

// A plan that lowers any remaining module is refused, -y or not (#2465).
func TestRemoveRefusesLoweringAnotherModule(t *testing.T) {
	before, err := modfile.Parse("go.mod", []byte("module example.com/app\n\ngo 1.26\n\nrequire example.com/other v1.2.0\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	after, err := modfile.Parse("go.mod", []byte("module example.com/app\n\ngo 1.26\n\nrequire example.com/other v1.1.0\n"), nil)
	if err != nil {
		t.Fatal(err)
	}
	p := &removePlan{before: before, after: after}
	if err := p.refusal(); err == nil || !strings.Contains(err.Error(), "lowers example.com/other from v1.2.0 to v1.1.0") {
		t.Errorf("refusal() = %v, want downgrade refusal", err)
	}
}

// An interrupt during remove leaves go.mod and go.sum as they were (#2465).
func TestRemoveInterruptedWritesNothing(t *testing.T) {
	offlineAdd(t, libraryProxy(t))
	dir := nsgoApp(t)
	if _, err := runAdd(t, dir, addOptions{yes: true}, nil, "github.com/acme/chart"); err != nil {
		t.Fatal(err)
	}
	oldMod, oldSum := moduleFiles(t, dir)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	t.Chdir(dir)
	var err error
	captureStderr(t, func() {
		err = removeModules(ctx, removeOptions{yes: true}, []string{"github.com/acme/chart"}, io.Discard, nil)
	})
	if err == nil || !strings.Contains(err.Error(), "interrupted while planning, so nothing was written") {
		t.Errorf("interrupted: %v", err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("interrupted remove modified go.mod or go.sum")
	}

	// Failed write restores go.mod and go.sum.
	failed := false
	addRename = func(from, to string) error {
		if filepath.Base(to) == "go.sum" && !failed {
			failed = true
			return errors.New("disk full")
		}
		return os.Rename(from, to)
	}
	t.Cleanup(func() { addRename = os.Rename })
	_, err = runRemove(t, dir, removeOptions{yes: true}, nil, "github.com/acme/chart")
	if err == nil || !strings.Contains(err.Error(), "disk full") || !failed {
		t.Fatalf("failed write: %v, want injected error", err)
	}
	if gomod, gosum := moduleFiles(t, dir); !bytes.Equal(gomod, oldMod) || !bytes.Equal(gosum, oldSum) {
		t.Errorf("failed write left go.mod or go.sum changed")
	}
}

func TestParseRemoveRequests(t *testing.T) {
	idx := &registryIndex{
		Version: registryIndexVersion,
		Items: []registryItem{
			{Name: "chart", Module: "github.com/acme/chart", Description: "Chart library.", License: "MIT"},
		},
	}
	got, err := parseRemoveSpecsWithIndex([]string{"example.com/a", "chart", "@acme/dep"}, idx)
	if err != nil || len(got) != 3 || got[0].path != "example.com/a" || got[1].path != "github.com/acme/chart" || got[2].registry != "@acme" || got[2].item != "dep" {
		t.Fatalf("parseRemoveSpecsWithIndex = %+v, %v", got, err)
	}
	for _, args := range [][]string{
		nil,
		{"unknown"},
		{"example.com/a@v1.0.0"},
		{"example.com/a#chat"},
		{"@nsgo/chart"},
		{"@acme"},
		{"@acme/"},
		{"@acme/chart@v1.0.0"},
		{"@acme/chart#chat"},
		{"@acme/chart", "@acme/chart"},
		{"chart", "github.com/acme/chart"},
		{"example.com/a", "-y"},
		{"example.com/a", "example.com/a"},
		{"Example.com/a"},
	} {
		if _, err := parseRemoveSpecsWithIndex(args, idx); err == nil {
			t.Errorf("parseRemoveSpecsWithIndex(%q) succeeded", args)
		}
	}
}

// Delegation refuses remove in a module pinning a release before v0.3.5 (#2465).
func TestDelegationOfRemove(t *testing.T) {
	path, env := goEnv(t, nsgoProxy(t, "v0.9.0"))
	out, code := runNsgo(t, pinnedModule(t, "v0.3.4", ""), path, env, "remove", "example.com/lib")
	if want := "nsgo remove needs NSGo v0.3.5 or later; this module pins v0.3.4: go get github.com/nsgo-dev/nsgo@v0.3.5\n"; code != 1 || !strings.Contains(out, want) || strings.Contains(out, "fake nsgo") {
		t.Fatalf("exit %d, output %s; want %q and exit 1", code, out, want)
	}
}
