package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// nsgo check: the catalogs and their code, go vet, then TypeScript's checker on the app's TSX against
// the declarations its packages give (.nsgo/types). A directory without nsgo.yaml has no app's TSX
// to check; one with nsgo.yaml but no Go, as a package's TSX app that a host runs, has nothing to
// vet, and its TSX is checked against every module NSGo gives.

// tsgo is TypeScript 7's compiler, built from its Go sources and cached
// by Go, so checking needs only Go.
const tsgo = "github.com/microsoft/typescript-go/cmd/tsgo@v0.0.0-20260820064610-89d5d5b2849a"

// check checks p: its catalogs, go vet and its TSX. Without targets go vet runs for the host, and in NSGo's repository for the dev client too; with targets it runs for them alone, up to jobs at once (checkTargets). jobs above 0 also bounds each go vet's compile workers (-p); 0 vets one target at a time with go's own default. Checks of one module wait for each other (lockCheck).
func check(ctx context.Context, p *project, target string, jobs int) error {
	if jobs < 0 {
		return fmt.Errorf("-jobs is %d; use 1 or more, or 0 for one target at a time", jobs)
	}
	unlock, err := lockCheck(ctx, p.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	s := begin("checking catalogs")
	if err := p.prepareCatalogs(ctx, "./...", true); err != nil {
		s.fail()
		return err
	}
	s.done("checked catalogs", "")
	if !p.bare && !hasGo(p.Dir) {
		return checkTSX(ctx, p)
	}
	if target == "" {
		if err := checkHost(ctx, p, jobs); err != nil {
			return err
		}
	} else if err := checkTargets(ctx, p, target, jobs); err != nil {
		return err
	}
	if p.bare {
		return nil
	}
	return checkTSX(ctx, p)
}

// checkHost vets p for the host and, in NSGo's own repository, the dev client.
func checkHost(ctx context.Context, p *project, jobs int) error {
	s := begin("vetting")
	if err := goToolWithTags(ctx, p, nil, nil, vetArgs(jobs, "./...")...); err != nil {
		s.fail()
		return err
	}
	s.done("vetted", "")
	// The dev client builds only into nsgo dev's builds, which take the nsgodev tag, and the automation agent with nsgoautomation, so NSGo's own repository vets them as they build them.
	if path, err := importPathOf(p.Dir); err == nil && path == nsgoModule {
		s = begin("vetting the dev client")
		if err := goToolWithTags(ctx, p, nil, []string{"nsgodev"}, vetArgs(jobs, ".", "./internal/dev")...); err != nil {
			s.fail()
			return err
		}
		if err := vetAutomation(ctx, p, []string{"GOOS=darwin", "GOARCH=arm64", "CGO_ENABLED=0"}, jobs, "macos"); err != nil {
			s.fail()
			return err
		}
		if err := goToolWithTags(ctx, p, []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}, []string{"nsgodev", "nsgoautomation"}, vetArgs(jobs, ".", "./internal/dev", "./nsgotest", "./internal/platform/android")...); err != nil {
			s.fail()
			return err
		}
		if err := goToolWithTags(ctx, p, []string{"GOOS=windows", "GOARCH=arm64", "CGO_ENABLED=0"}, []string{"nsgodev", "nsgoautomation"}, vetArgs(jobs, ".", "./internal/dev", "./nsgotest", "./internal/platform/windows")...); err != nil {
			s.fail()
			return err
		}
		if runtime.GOOS == "linux" {
			if err := vetAutomation(ctx, p, nil, jobs, "linux"); err != nil {
				s.fail()
				return err
			}
		}
		s.done("vetted the dev client", "")
	}
	return nil
}

// vetAutomation checks the tagged agent and the platform surface it drives.
func vetAutomation(ctx context.Context, p *project, env []string, jobs int, platform string) error {
	return goToolWithTags(ctx, p, env, []string{"nsgodev", "nsgoautomation"}, vetArgs(jobs, ".", "./internal/dev", "./nsgotest", "./internal/platform/"+platform)...)
}

// vetArgs is go vet's arguments for patterns, with -p jobs when jobs is above 0.
func vetArgs(jobs int, patterns ...string) []string {
	args := []string{"vet"}
	if jobs > 0 {
		args = append(args, "-p", strconv.Itoa(jobs))
	}
	return append(args, patterns...)
}

// checkTargets vets every requested target, up to jobs at once (one for 0). Only Apple mobile targets included through all may be skipped for an unavailable SDK; explicit names stay strict. A target's failure does not prevent checking the independent targets beside and after it; cancellation stops starting new ones.
func checkTargets(ctx context.Context, p *project, target string, jobs int) error {
	requested := strings.Split(target, ",")
	names, err := expandTargets(requested)
	if err != nil {
		return err
	}
	repository := false
	if p != nil {
		path, err := importPathOf(p.Dir)
		repository = err == nil && path == nsgoModule
	}
	// Each target's outcome stands at its index, so the skips and failures are reported in the targets' order whichever finishes first.
	type outcome struct {
		err error
		// why is set when the target was skipped.
		why string
	}
	outcomes := make([]outcome, len(names))
	slots := make(chan struct{}, max(1, min(jobs, len(names))))
	var wg sync.WaitGroup
	var interrupted error
	for i, name := range names {
		if ctx.Err() != nil {
			interrupted = ctx.Err()
			break
		}
		_, _, apple := appleOSOf(name)
		optional := apple && slices.Contains(requested, "all") && !slices.Contains(requested, name)
		if optional && runtime.GOOS != "darwin" {
			outcomes[i].why = "they need macOS with Xcode"
			continue
		}
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			interrupted = ctx.Err()
			break
		}
		wg.Go(func() {
			defer func() { <-slots }()
			env, err := vetEnv(ctx, name)
			if err != nil && optional && ctx.Err() == nil {
				outcomes[i].why = err.Error()
				return
			}
			s := begin("vetting for %s", name)
			if err == nil {
				err = goToolWithTags(ctx, p, env, targetTags(name), vetArgs(jobs, "./...")...)
			}
			if err == nil && repository {
				// The test hosts build only with nsgotest, which no app build sets, and each must implement testhost.Surface, so the repository vets them for every target it checks.
				err = goToolWithTags(ctx, p, env, slices.Concat(targetTags(name), []string{"nsgotest"}), vetArgs(jobs, "./internal/platform/...")...)
				if err != nil {
					err = fmt.Errorf("test hosts: %w", err)
				}
				if err == nil && (strings.HasPrefix(name, "macos_") || strings.HasPrefix(name, "windows_")) {
					err = goToolWithTags(ctx, p, env, slices.Concat(targetTags(name), []string{"nsgoautomation"}), vetArgs(jobs, "./internal/platform/...")...)
					if err != nil {
						err = fmt.Errorf("automation hosts: %w", err)
					}
				}
			}
			if err == nil && repository && targetSupportsAutomation(name) {
				err = goToolWithTags(ctx, p, env, slices.Concat(targetTags(name), []string{"nsgodev", "nsgoautomation"}), vetArgs(jobs, ".", "./internal/dev", "./nsgotest", "./internal/platform/...")...)
				if err != nil {
					err = fmt.Errorf("automation: %w", err)
				}
			}
			if err == nil && repository && strings.HasPrefix(name, "miniprogram_") {
				// Mini programs build their own nsgodev transport without net/http.
				err = goToolWithTags(ctx, p, env, slices.Concat(targetTags(name), []string{"nsgodev"}), vetArgs(jobs, ".", "./internal/dev", "./internal/devtools", "./internal/backend")...)
				if err != nil {
					err = fmt.Errorf("dev client: %w", err)
				}
			}
			if err != nil {
				s.fail()
				outcomes[i].err = fmt.Errorf("%s: %w", name, err)
				return
			}
			s.done("vetted for "+name, "")
		})
	}
	wg.Wait()
	var skipped []string
	// why is the reason the first target was skipped for, so a Mac whose Xcode lacks an SDK, or fails for another reason, says which.
	var why string
	var failures []error
	for i, o := range outcomes {
		if o.why != "" {
			skipped = append(skipped, names[i])
			if why == "" {
				why = o.why
			}
		}
		if o.err != nil {
			failures = append(failures, o.err)
		}
	}
	if interrupted != nil {
		failures = append(failures, interrupted)
	}
	if len(skipped) > 0 {
		warn("skipped %s: %s", strings.Join(skipped, ", "), why)
	}
	return errors.Join(failures...)
}

// checkTSX writes the app's types and type-checks its TSX. An app
// without a tsconfig.json has no TSX, so no types are written.
func checkTSX(ctx context.Context, p *project) error {
	if !p.hasTSConfig() {
		note("no tsconfig.json: nothing to type-check (a TSX app's extends .nsgo/tsconfig.json)")
		return nil
	}
	if err := writeTypes(ctx, p); err != nil {
		return err
	}
	config := filepath.Join(p.Dir, "tsconfig.json")
	s := begin("type-checking")
	if err := goTool(ctx, p.Dir, nil, "run", tsgo, "-p", config, "--pretty", "false"); err != nil {
		s.fail()
		return err
	}
	s.done("type-checked", "")
	return nil
}

// lockCheck waits until no other nsgo check runs in dir's module, saying once which process it waits for, and returns what ends its own turn. The lock is the operating system's on .nsgo/check.lock at the module's root, so a check that crashed or was killed holds it no longer; the file stays, as removing it would let a check that opened it before take a lock no later one sees.
func lockCheck(ctx context.Context, dir string) (unlock func(), err error) {
	root := moduleRootOf(dir)
	if err := os.MkdirAll(filepath.Join(root, ".nsgo"), 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(root, ".nsgo", "check.lock")
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, err
	}
	var tick *time.Ticker
	for {
		locked, err := tryLockFile(f)
		if err != nil {
			f.Close()
			return nil, fmt.Errorf("locking %s: %w", rel(path), err)
		}
		if locked {
			break
		}
		if tick == nil {
			holder := "another nsgo check"
			if data, err := os.ReadFile(path); err == nil {
				if pid := strings.TrimSpace(string(data)); pid != "" {
					holder += " (pid " + pid + ")"
				}
			}
			note("%s is checking %s; waiting for it", holder, rel(root))
			tick = time.NewTicker(250 * time.Millisecond)
			defer tick.Stop()
		}
		select {
		case <-ctx.Done():
			f.Close()
			return nil, ctx.Err()
		case <-tick.C:
		}
	}
	// The pid is only for the message above; a write that fails leaves it out.
	if f.Truncate(0) == nil {
		f.WriteAt([]byte(strconv.Itoa(os.Getpid())+"\n"), 0)
	}
	return func() {
		f.Truncate(0)
		unlockFile(f)
		f.Close()
	}, nil
}

// moduleRootOf is the directory of the go.mod dir is in, or dir outside any module.
func moduleRootOf(dir string) string {
	for mod := dir; ; mod = filepath.Dir(mod) {
		if _, err := os.Stat(filepath.Join(mod, "go.mod")); err == nil {
			return mod
		}
		if filepath.Dir(mod) == mod {
			return dir
		}
	}
}

// targetSupportsAutomation reports whether checkTargets vets target's live automation build too.
func targetSupportsAutomation(name string) bool {
	if strings.HasPrefix(name, "android_") {
		return true
	}
	switch name {
	case "macos_arm64", "macos_x64", "macos_universal", "linux_arm64", "linux_x64", "ios", "tvos", "watchos", "ios_simulator", "tvos_simulator", "watchos_simulator":
		return true
	}
	return false
}

// vetEnv is the environment go vets a target's build in, as the target's build sets it; the tags its build adds are targetTags'.
func vetEnv(ctx context.Context, target string) ([]string, error) {
	os, arch, _ := strings.Cut(target, "_")
	switch os {
	case "macos":
		if target == "macos_universal" {
			// The universal package builds the same Go source for both architectures and joins the binaries, so vet it once as arm64.
			arch = "arm64"
		}
		return []string{"GOOS=darwin", "GOARCH=" + goArch(arch), "CGO_ENABLED=0"}, nil
	case "windows", "linux":
		return []string{"GOOS=" + os, "GOARCH=" + goArch(arch), "CGO_ENABLED=0"}, nil
	case "web", "miniprogram":
		return []string{"GOOS=js", "GOARCH=wasm"}, nil
	case "harmonyos":
		return []string{"GOOS=js", "GOARCH=wasm"}, nil
	case "ios", "tvos":
		sys, _, _ := appleOSOf(os)
		return iosEnv(ctx, sys, arch == "simulator")
	case "watchos":
		return iosEnv(ctx, watchOS, arch == "simulator")
	case "android":
		if arch == "arm64" {
			return []string{"GOOS=android", "GOARCH=arm64", "CGO_ENABLED=0"}, nil
		}
		cc, err := androidClang(goArch(arch))
		if err != nil {
			return nil, err
		}
		return []string{"GOOS=android", "GOARCH=" + goArch(arch), "CGO_ENABLED=1", "CC=" + cc}, nil
	}
	return nil, fmt.Errorf("unknown target %q", target)
}

// typesInit is the file the tool adds to the app's main package, through
// go's -overlay, to write the declarations of the modules its TSX
// imports: run last of the package's init functions, after every
// tsx.Register, it writes them, with the names token() reads from the
// app's theme file (nsgo.yaml's theme) over the default theme's, and ends
// the program before main, so the tool links no tsx.
const typesInit = `package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

func init() {
	dir := os.Getenv("NSGO_TYPES")
	if dir == "" {
		return
	}
	write := func() error {
		if err := tsx.WriteDeclarations(dir); err != nil {
			return err
		}
		file := os.Getenv("NSGO_THEME")
		if file == "" {
			return nil
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("nsgo.yaml: theme: %w", err)
		}
		theme, err := core.ParseTheme(data)
		if err != nil {
			return fmt.Errorf("nsgo.yaml: theme: %w", err)
		}
		return os.WriteFile(filepath.Join(dir, "types", "nsgo", "tokens.d.ts"), []byte(tsx.TokenDeclarations(theme)), 0o644)
	}
	if err := write(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Exit(0)
}
`

// tsxModules are the packages registering every TSX module NSGo gives,
// which a directory of TSX alone is checked against: a host decides
// which its packages may import.
var tsxModules = []string{"diff", "editor", "richeditor", "terminal", "textview", "ui", "ui/chat", "ui/richeditor"}

// hasGo reports whether dir holds Go files.
func hasGo(dir string) bool {
	files, _ := filepath.Glob(filepath.Join(dir, "*.go"))
	return len(files) > 0
}

// hasTSConfig reports whether the app has a tsconfig.json, as a TSX
// app's does, extending .nsgo/tsconfig.json.
func (p *project) hasTSConfig() bool {
	_, err := os.Stat(filepath.Join(p.Dir, "tsconfig.json"))
	return err == nil
}

// writeTypes writes the declarations of the modules the app's TSX can
// import to .nsgo/types, running its main package with typesInit. It
// first checks that go sees typesInit in the package, so the app itself
// never runs.
func writeTypes(ctx context.Context, p *project) error {
	s := begin("writing types")
	if err := generateTypes(ctx, p); err != nil {
		s.fail()
		return err
	}
	s.done("wrote types", "→ "+filepath.Join(".nsgo", "types"))
	return nil
}

func generateTypes(ctx context.Context, p *project) error {
	dir := p.cache()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if !hasGo(p.Dir) {
		return generateModuleTypes(ctx, p)
	}
	file, err := p.cacheFile("types", ".go", typesInit)
	if err != nil {
		return err
	}
	ov, err := p.overlay(map[string]string{"zz_nsgo_types.go": file})
	if err != nil {
		return err
	}
	listTypes := func() ([]byte, error) {
		tags, err := goTags(ctx, p, nil)
		if err != nil {
			return nil, err
		}
		args := append([]string{"list", "-overlay", ov}, tagsArgs(tags)...)
		list := exec.CommandContext(ctx, "go", append(args, "-f", "{{.Name}} {{.GoFiles}}", ".")...)
		list.Dir = p.Dir
		return list.CombinedOutput()
	}
	out, err := listTypes()
	// The types import tsx, which go mod tidy, not seeing them, may have
	// left out what it needs.
	if err != nil && lacksModule(string(out)) {
		if err := p.tidy(ctx); err != nil {
			return err
		}
		out, err = listTypes()
	}
	if err != nil {
		return fmt.Errorf("go list failed\n%s", strings.TrimSpace(string(out)))
	}
	if name, files, _ := strings.Cut(strings.TrimSpace(string(out)), " "); name != "main" ||
		!strings.Contains(files, "zz_nsgo_types.go") {
		return fmt.Errorf("cannot write types: %s is not a main package go adds zz_nsgo_types.go to (%s)", p.Dir, out)
	}
	return runTypes(ctx, p, "-overlay", ov, ".")
}

// generateModuleTypes writes the types of every module NSGo gives, for a
// directory of TSX alone, from a main package of typesInit the tool makes
// in its cache.
func generateModuleTypes(ctx context.Context, p *project) error {
	imports := ""
	for _, m := range tsxModules {
		imports += "\t_ \"github.com/nsgo-dev/nsgo/tsx/" + m + "\"\n"
	}
	src := strings.Replace(typesInit, "import (\n", "import (\n"+imports, 1) + "\nfunc main() {}\n"
	pkgDir := p.cache("typesmain")
	if err := os.MkdirAll(pkgDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(pkgDir, "main.go"), []byte(src), 0o644); err != nil {
		return err
	}
	return runTypes(ctx, p, "./"+filepath.ToSlash(filepath.Join(".nsgo", "typesmain")))
}

// runTypes builds the package of buildArgs with typesInit and runs it.
func runTypes(ctx context.Context, p *project, buildArgs ...string) error {
	dir := p.cache()
	bin := filepath.Join(dir, "nsgo-types")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if err := goToolWithTags(ctx, p, nil, nil, append([]string{"build", "-o", bin}, buildArgs...)...); err != nil {
		return err
	}
	// typesInit ends the program before main; the timeout guards the
	// machine should it not.
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), "NSGO_TYPES="+dir)
	if p.Theme != "" {
		cmd.Env = append(cmd.Env, "NSGO_THEME="+filepath.Join(p.Dir, p.Theme))
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("writing types failed: %w\n%s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// goToolWithTags is goTool in p's directory with goTags' tags, extra the tool's, after args[0], the go command's subcommand; GOFLAGS's other flags still apply.
func goToolWithTags(ctx context.Context, p *project, env, extra []string, args ...string) error {
	tags, err := goTags(ctx, p, env, extra...)
	if err != nil {
		return err
	}
	args = append(append([]string{args[0]}, tagsArgs(tags)...), args[1:]...)
	return goTool(ctx, p.Dir, env, args...)
}
