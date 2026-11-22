package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/harmony"
	"github.com/nsgo-dev/nsgo/tsx/websrc"
)

// buildAll writes one directory per target: <output>/<name>-<version>-<target>.
func buildAll(ctx context.Context, p *project) error {
	p.Release = true
	p.packageLicenses = !p.Build.embed
	if err := p.readReleaseNotes(); err != nil {
		return err
	}
	buildTargets, err := expandTargets(p.Build.Targets)
	if err != nil {
		return err
	}
	if err := p.checkSteamTargets(buildTargets); err != nil {
		return err
	}
	if p.Build.embed {
		if buildTargets, err = embedOnly(buildTargets); err != nil {
			return err
		}
	} else if p.channel() == channelStore {
		buildTargets = universalStoreTargets(buildTargets)
	}
	if err := p.checkStoreCompliance(buildTargets); err != nil {
		return err
	}
	jobs, err := p.buildJobs(len(buildTargets))
	if err != nil {
		return err
	}
	started := time.Now()
	header(p.Name, p.Version, strings.Join(buildTargets, ", "))
	if p.Build.all {
		// Say what all left out that the build does not add.
		var out []string
		if !slices.Contains(buildTargets, "android_x64") && !slices.Contains(buildTargets, "android_universal") {
			out = append(out, "android_x64, which needs an NDK")
		}
		if !slices.ContainsFunc(buildTargets, func(t string) bool { return strings.HasPrefix(t, "miniprogram_") }) {
			out = append(out, "the mini programs, which take Go apps of 2 MB (miniprogram)")
		}
		if len(out) > 0 {
			detail("all leaves out %s", strings.Join(out, " and "))
		}
	}
	if err := p.checkNotarizing(); err != nil {
		return err
	}
	if err := p.checkSigningWindows(buildTargets); err != nil {
		return err
	}
	if err := p.prepareCatalogs(ctx, ".", false); err != nil {
		return err
	}
	if p.channel() != channelSteam {
		if err := p.cleanReleaseDir(); err != nil {
			return err
		}
	}
	total, err := buildPackages(ctx, p, buildTargets, jobs)
	if err != nil {
		return err
	}
	if err := p.writeInstallScripts(ctx, buildTargets); err != nil {
		return err
	}
	if err := p.writeFlatpakManifest(buildTargets); err != nil {
		return err
	}
	if err := p.writePackageManifests(ctx, buildTargets); err != nil {
		return err
	}
	buildSummary(p, len(buildTargets), total, time.Since(started))
	if p.openOutput() {
		openDir(p.Build.Output)
	}
	if p.Build.run {
		for _, target := range buildTargets {
			if runnable(target) {
				return runPackage(ctx, p, target, p.packageDir(target))
			}
		}
		return fmt.Errorf("-run: none of %s runs on this machine", strings.Join(buildTargets, ", "))
	}
	return nil
}

// packageDir is the output directory of one built target.
func (p *project) packageDir(target string) string {
	return filepath.Join(p.Build.Output, fmt.Sprintf("%s-%s-%s", p.Name, p.Version, target))
}

// buildPackages builds each target's package into its directory, up to
// jobs at once, and returns the space they take. Each builds from a
// copy of p, which holds what one target's build sets, such as what it
// released. A target that fails stops the others and leaves no directory.
func buildPackages(ctx context.Context, p *project, targets []string, jobs int) (int64, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var (
		// mu keeps a target's lines together and guards the results.
		mu       sync.Mutex
		total    int64
		firstErr error
		wg       sync.WaitGroup
	)
	slots := make(chan struct{}, jobs)
	for i, target := range targets {
		select {
		case slots <- struct{}{}:
		case <-ctx.Done():
		}
		if ctx.Err() != nil {
			break
		}
		// Several targets are counted: [2/5] building ios. Begun here,
		// their spinners stand in the targets' order.
		count := ""
		if len(targets) > 1 {
			count = fmt.Sprintf("[%d/%d] ", i+1, len(targets))
		}
		progress := begin("%sbuilding %s", count, target)
		// Beside other targets, its lines wait for its own.
		progress.hold = jobs > 1
		wg.Go(func() {
			defer func() { <-slots }()
			dir := p.packageDir(target)
			q := *p
			q.buildParallel = compileWorkers(jobs)
			q.released = nil
			err := os.RemoveAll(dir)
			if err == nil {
				err = os.MkdirAll(dir, 0o755)
			}
			if err == nil {
				err = buildTarget(withStep(ctx, progress), &q, target, dir)
			}
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				progress.fail()
				os.RemoveAll(dir)
				// The others fail as they stop: the first failure is
				// the build's.
				if firstErr == nil {
					firstErr = fmt.Errorf("%s: %w", target, err)
					cancel()
				}
				return
			}
			size := dirSize(dir)
			total += size
			progress.done(fmt.Sprintf("%sbuilt %s", count, target), formatSize(size))
			detail("→ %s", rel(dir))
			for _, file := range q.released {
				detail("→ %s", rel(file))
			}
		})
	}
	wg.Wait()
	if firstErr == nil {
		// Interrupted before any target failed.
		firstErr = ctx.Err()
	}
	return total, firstErr
}

// buildJobs is how many targets nsgo build builds at once: build.jobs, or
// half the CPUs by default, and no more than there are targets. Each target's
// Go compile workers are reduced to keep the total near the CPU count.
func (p *project) buildJobs(targets int) (int, error) {
	jobs := p.Build.Jobs
	switch {
	case jobs < 0:
		return 0, fmt.Errorf("build.jobs is %d; use 1 or more, or 0 for half the CPUs", jobs)
	case jobs == 0:
		jobs = max(1, runtime.NumCPU()/2)
	}
	jobs = min(jobs, max(1, runtime.NumCPU()))
	return max(1, min(jobs, targets)), nil
}

// compileWorkers is the -p of each go build while jobs targets build at once, so that their compilers together stay within the CPU count; a lone target keeps every CPU.
func compileWorkers(jobs int) int {
	return max(1, runtime.NumCPU()/max(1, jobs))
}

// buildSummary closes a build of several targets with their number, time
// and space; one target's line already says its own.
func buildSummary(p *project, targets int, size int64, took time.Duration) {
	if targets < 2 {
		return
	}
	done("built %d targets in %s (%s, %s)", targets, rel(p.Build.Output), formatDuration(took), formatSize(size))
}

// dirSize is how much space the files under dir take.
func dirSize(dir string) int64 {
	var total int64
	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

// openDir shows dir in the host's file manager. A build is complete
// without it, so failing to open is only a warning. It waits for the
// opener, which returns once the system has the request: when nsgo
// leads its terminal's session, the opener would otherwise be hung up
// as nsgo exits, before it hands dir on.
func openDir(dir string) {
	note("open %s", rel(dir))
	opener := "xdg-open"
	switch runtime.GOOS {
	case "darwin":
		opener = "open"
	case "windows":
		opener = "explorer"
	}
	err := exec.Command(opener, dir).Run()
	// explorer exits with 1 even when it opens dir.
	var exit *exec.ExitError
	if runtime.GOOS == "windows" && errors.As(err, &exit) {
		err = nil
	}
	if err != nil {
		warn("could not open %s: %v", rel(dir), err)
	}
}

// openCommand returns the command that shows a URL in the user's browser.
func openCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	}
	return "xdg-open", []string{url}
}

// openPage returns the command that shows url, the address of a page nsgo dev serves with the run's token in it (devServer.pageURL), in the user's browser, keeping it off every command line, which every user of the machine may read, the browser's included: macOS's osascript reads open location from its standard input, and elsewhere the browser opens file, a page in dir only this user reads that sends it on to url.
func openPage(url, dir string) (cmd *exec.Cmd, file string, err error) {
	if runtime.GOOS == "darwin" {
		cmd = exec.Command("osascript")
		quoted := strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(url)
		cmd.Stdin = strings.NewReader(`open location "` + quoted + `"` + "\n")
		return cmd, "", nil
	}
	if dir, err = filepath.Abs(dir); err != nil {
		return nil, "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, "", err
	}
	f, err := os.CreateTemp(dir, "open-*.html")
	if err != nil {
		return nil, "", err
	}
	// JSON escapes <, > and &, so the address cannot end the script.
	target, _ := json.Marshal(url)
	_, err = f.WriteString(`<!doctype html><meta charset="utf-8"><title>nsgo dev</title><script>location.replace(` + string(target) + `)</script>` + "\n")
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return nil, "", err
	}
	name, args := openCommand(f.Name())
	return exec.Command(name, args...), f.Name(), nil
}

// targets are the names nsgo.yaml and -target accept, each one package.
var targets = []string{
	"macos_arm64", "macos_x64", "macos_universal",
	"windows_arm64", "windows_x64",
	"linux_arm64", "linux_x64",
	"ios", "ios_simulator",
	"tvos", "tvos_simulator",
	"watchos", "watchos_simulator",
	"android_arm64", "android_x64",
	"harmonyos",
	"web",
	"miniprogram_wechat", "miniprogram_qq", "miniprogram_alipay", "miniprogram_douyin",
	"tui_macos_arm64", "tui_macos_x64", "tui_windows_arm64", "tui_windows_x64", "tui_linux_arm64", "tui_linux_x64",
}

// miniProgramGroup names every mini program target at once.
const miniProgramGroup = "miniprogram"

// expandTargets replaces "all" with every target that needs only Go and Xcode, a bare OS name with its architectures, and "miniprogram" with every mini program, checks every name and drops duplicates, so each package is built once. android_x64 stays out of "all" and "android" because it needs an NDK, macos_universal out of "all" and "macos" because it builds both macOS architectures again to join them, and the mini programs because they take only Go apps that fit their packages.
func expandTargets(names []string) ([]string, error) {
	var out []string
	for _, name := range names {
		expanded := []string{name}
		switch {
		case name == "all":
			expanded = goAndXcodeTargets()
		case name == "tui":
			expanded = nil
			for _, t := range targets {
				if strings.HasPrefix(t, "tui_") {
					expanded = append(expanded, t)
				}
			}
		case name == miniProgramGroup:
			expanded = nil
			for _, t := range targets {
				if strings.HasPrefix(t, "miniprogram_") {
					expanded = append(expanded, t)
				}
			}
		case slices.Contains([]string{"macos", "windows", "linux", "android"}, name):
			// The bare OS, as nsgo test -target spells it (#589).
			expanded = nil
			for _, t := range targets {
				if goos, _, _ := strings.Cut(t, "_"); goos == name && t != "android_x64" && t != "macos_universal" {
					expanded = append(expanded, t)
				}
			}
		case !slices.Contains(targets, name):
			return nil, fmt.Errorf("unknown target %q; use all, miniprogram or one of %s", name, strings.Join(targets, ", "))
		}
		for _, target := range expanded {
			if !slices.Contains(out, target) {
				out = append(out, target)
			}
		}
	}
	return out, nil
}

// goAndXcodeTargets are the packages a Mac with Go and Xcode can build. android_x64 is omitted: Go links android/amd64 only with cgo, so it needs an NDK; macos_universal, which joins the two macOS targets' builds; the harmonyos preview, and the mini programs, which only some apps fit.
func goAndXcodeTargets() []string {
	var out []string
	for _, target := range targets {
		if target != "android_x64" && target != "macos_universal" && target != "harmonyos" && !strings.HasPrefix(target, "miniprogram_") {
			out = append(out, target)
		}
	}
	return out
}

// universalStoreTargets replaces the architectures of each combined store
// package in place in the build order, keeping the other targets in order.
func universalStoreTargets(targets []string) []string {
	mac := slices.Contains(targets, "macos_universal") ||
		slices.Contains(targets, "macos_arm64") && slices.Contains(targets, "macos_x64")
	android := slices.Contains(targets, "android_arm64") && slices.Contains(targets, "android_x64")
	var out []string
	for _, target := range targets {
		switch target {
		case "macos_arm64", "macos_x64":
			if mac {
				target = "macos_universal"
			}
		case "android_arm64", "android_x64":
			if android {
				target = "android_universal"
			}
		}
		if !slices.Contains(out, target) {
			out = append(out, target)
		}
	}
	return out
}

func buildTarget(ctx context.Context, p *project, target, dir string) error {
	if strings.HasPrefix(target, "tui_") && !p.Build.embed {
		return buildTUI(ctx, p, dir, target)
	}
	_, arch, _ := strings.Cut(target, "_")
	if p.Build.embed {
		switch target {
		case "ios", "ios_simulator":
			// One framework holds both.
			return buildIOSFramework(ctx, p, dir)
		case "android_arm64", "android_x64":
			return buildAndroidAAR(ctx, p, dir, goArch(arch))
		}
		return fmt.Errorf("-embed builds for %s; desktop hosts embed apps with nsgo.NewView", strings.Join(embedTargets, ", "))
	}
	p.checkStorePrivacy = p.channel() == channelStore && appleStoreTarget(target)
	if err := p.checkUpdateFeed(target); err != nil {
		return err
	}
	switch target {
	case "macos_universal":
		return buildMacUniversal(ctx, p, dir)
	case "macos_arm64", "macos_x64":
		app, err := buildMacApp(ctx, p, dir, goArch(arch))
		if err != nil {
			return err
		}
		if p.channel() == channelStore {
			if !p.installers() {
				_, err := signMacStoreApp(ctx, p, app)
				return err
			}
			return releaseMacStorePackage(ctx, p, app, dir, arch)
		}
		if err := signMacApp(ctx, p, app); err != nil {
			return err
		}
		dmg := ""
		if p.installers() {
			if dmg, err = buildDMG(ctx, p, app, dir, arch); err != nil {
				return err
			}
		}
		if err := notarizeMac(ctx, p, app, dmg); err != nil {
			return err
		}
		return p.releaseInstaller(target, dmg)
	case "windows_arm64", "windows_x64":
		if err := buildWindows(ctx, p, dir, goArch(arch)); err != nil {
			return err
		}
		install := p.installers()
		signer := windowsSignerOf(p)
		if signer != nil {
			if err := signer.signWindowsPackage(ctx, p, dir); err != nil {
				return err
			}
		}
		if !install {
			return nil
		}
		if p.channel() == channelStore {
			msix, err := buildMSIX(ctx, p, dir, goArch(arch))
			if err != nil {
				return err
			}
			if signer != nil {
				if err := signer.signFile(ctx, msix, false); err != nil {
					return err
				}
			}
			return p.releaseAssets(msix)
		}
		setup, err := buildNSIS(ctx, p, dir, arch)
		if err != nil {
			return err
		}
		if signer != nil && setup != "" {
			if err := signer.signFile(ctx, setup, true); err != nil {
				return err
			}
			detailIn(ctx, "signed %s", filepath.Base(setup))
		}
		return p.releaseInstaller(target, setup)
	case "watchos":
		_, err := buildWatchApp(ctx, p, dir, false)
		return err
	case "ios", "tvos":
		sys, _, _ := appleOSOf(target)
		app, err := buildIOSApp(ctx, p, sys, dir, false)
		if err != nil {
			return err
		}
		if err := signForStore(ctx, p, sys, app); err != nil {
			return err
		}
		stage(ctx, "packaging the IPA")
		ipa := filepath.Join(dir, p.Name+".ipa")
		if err := writeIPA(app, ipa); err != nil {
			return err
		}
		return p.releaseCopy(ipa, p.releaseName(target, ".ipa"))
	case "ios_simulator", "tvos_simulator", "watchos_simulator":
		sys, _, _ := appleOSOf(target)
		_, err := buildIOSApp(ctx, p, sys, dir, true)
		return err
	case "web":
		if err := buildWeb(ctx, p, dir, false); err != nil {
			return err
		}
		return p.releaseZip(dir, p.releaseName(target, ".zip"))
	case "harmonyos":
		// The engine runs in WebAssembly in ArkWeb under an ArkTS host,
		// which draws what it mounts with ArkUI
		// (docs/architecture/harmonyos.md).
		site := filepath.Join(dir, "web")
		if err := buildHarmony(ctx, p, site); err != nil {
			return err
		}
		code := p.BuildNumber
		if code == 0 {
			code = 1
		}
		hap, err := buildHarmonyHAP(ctx, p, dir, site, code)
		if err != nil {
			return err
		}
		return p.releaseCopy(hap, p.releaseName(target, ".hap"))
	case "linux_arm64", "linux_x64":
		if err := buildLinux(ctx, p, dir, goArch(arch)); err != nil {
			return err
		}
		if p.bundlesSteamLinuxRuntime(target) {
			stage(ctx, "bundling the Steam GTK runtime")
			return addSteamLinuxRuntime(p, dir)
		}
		if !p.installers() {
			return nil
		}
		if p.channel() == channelStore {
			archive, err := buildLinuxArchive(ctx, p, dir, goArch(arch))
			if err != nil {
				return err
			}
			return p.releaseAssets(archive)
		}
		deb, pkg, err := buildLinuxPackages(ctx, p, dir, goArch(arch))
		if err != nil {
			return err
		}
		appImage, err := buildAppImage(ctx, p, dir, goArch(arch))
		if err != nil {
			return err
		}
		return p.releaseInstaller(target, deb, pkg, appImage)
	case "android_universal":
		return releaseAndroidBundle(ctx, p, dir, target, "arm64", "amd64")
	case "android_arm64", "android_x64":
		if p.channel() == channelStore {
			return releaseAndroidBundle(ctx, p, dir, target, goArch(arch))
		}
		apk, err := buildAndroidAPK(ctx, p, dir, goArch(arch))
		if err != nil {
			return err
		}
		name := p.releaseName(target, ".apk")
		if err := p.releaseCopy(apk, name); err != nil {
			return err
		}
		return p.writeAppcast(target, filepath.Join(p.releaseDir(), name))
	}
	if mp, ok := miniProgramPlatformOf(target); ok {
		if err := buildMiniProgram(ctx, p, mp, dir); err != nil {
			return err
		}
		return p.releaseTree(dir, "miniprogram/"+mp.name)
	}
	return fmt.Errorf("unknown target %q; use all or one of %s", target, strings.Join(targets, ", "))
}

// buildMacUniversal joins the two macOS app builds before the bundle is signed and packaged. Each architecture is built through the normal pipeline first, so resources, widgets and optional libraries keep the same inputs as their single-architecture counterparts.
func buildMacUniversal(ctx context.Context, p *project, dir string) error {
	p.noticeArchitectures = []string{"arm64", "amd64"}
	work, err := os.MkdirTemp(dir, ".universal-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	arm, err := buildMacApp(ctx, p, filepath.Join(work, "arm64"), "arm64")
	if err != nil {
		return err
	}
	x64, err := buildMacApp(ctx, p, filepath.Join(work, "x64"), "amd64")
	if err != nil {
		return err
	}
	if err := matchMacLibraries(arm, x64); err != nil {
		return err
	}
	// Moving the arm64 bundle keeps its framework links, executable modes and the signatures of what lipo leaves alone, such as Sparkle's.
	app := filepath.Join(dir, p.Name+".app")
	if err := os.Rename(arm, app); err != nil {
		return err
	}
	if err := lipoMacBinaries(ctx, app, x64); err != nil {
		return err
	}
	if p.channel() == channelStore {
		if !p.installers() {
			_, err := signMacStoreApp(ctx, p, app)
			return err
		}
		return releaseMacStorePackage(ctx, p, app, dir, "universal")
	}
	if p.updates() {
		if err := rewriteUniversalFeed(p, app); err != nil {
			return err
		}
	}
	if p.Sign.Identity != "" {
		if err := signMacApp(ctx, p, app); err != nil {
			return err
		}
	} else {
		if err := signUniversalAdHoc(ctx, p, app); err != nil {
			return err
		}
	}
	dmg := ""
	if p.installers() {
		dmg, err = buildDMG(ctx, p, app, dir, "universal")
		if err != nil {
			return err
		}
	}
	if err := notarizeMac(ctx, p, app, dmg); err != nil {
		return err
	}
	return p.releaseInstaller("macos_universal", dmg)
}

// lipoMacBinaries replaces every executable or dylib that differs between the two architecture bundles with a universal binary. One that is the same in both, such as Sparkle's helpers or a library already universal, stays as it is, signature included, as lipo refuses to join two files of the same architectures. Resource files remain those from the arm64 bundle, which are identical inputs from buildMacApp.
func lipoMacBinaries(ctx context.Context, app, other string) error {
	return filepath.WalkDir(app, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		rel, err := filepath.Rel(app, path)
		if err != nil {
			return err
		}
		relSlash := filepath.ToSlash(rel)
		if !strings.HasSuffix(relSlash, ".dylib") && !strings.HasPrefix(relSlash, "Contents/MacOS/") && !strings.Contains(relSlash, "/Contents/MacOS/") {
			return nil
		}
		otherPath := filepath.Join(other, rel)
		theirs, err := os.ReadFile(otherPath)
		if err != nil {
			return nil
		}
		ours, err := os.ReadFile(path)
		if err != nil || bytes.Equal(ours, theirs) {
			return err
		}
		return run(ctx, "lipo", "-create", path, otherPath, "-output", path)
	})
}

// rewriteUniversalFeed points the joined bundle's Sparkle feed, the arm64 build's, at macos_universal's appcast.
func rewriteUniversalFeed(p *project, app string) error {
	path := filepath.Join(app, "Contents", "Info.plist")
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	for _, arch := range []string{"macos_arm64", "macos_x64"} {
		if feed := p.appcastURL(arch); feed != "" {
			data = bytes.ReplaceAll(data, []byte(feed), []byte(p.appcastURL("macos_universal")))
		}
	}
	return os.WriteFile(path, data, 0o644)
}

// signUniversalAdHoc signs the joined bundle ad hoc again without sign.identity, as buildMacApp signed each architecture's, since lipo broke the signatures of what it joined.
func signUniversalAdHoc(ctx context.Context, p *project, app string) error {
	if appex := macWidgetsAppex(app); len(p.Widgets) > 0 {
		if err := signMacWidgets(ctx, p, appex, "-"); err != nil {
			return err
		}
	}
	err := filepath.WalkDir(filepath.Join(app, "Contents"), func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() || !strings.HasSuffix(path, ".dylib") {
			return walkErr
		}
		return run(ctx, "codesign", "--force", "--sign", "-", path)
	})
	if err != nil {
		return err
	}
	return run(ctx, "codesign", "--force", "--sign", "-", app)
}

// goArch maps x64, the name packages use, to Go's amd64.
func goArch(arch string) string {
	if arch == "x64" {
		return "amd64"
	}
	return arch
}

// goBuild builds the project's main package to out.
func goBuild(ctx context.Context, p *project, out string, env []string, ldflags ...string) error {
	return goBuildWith(ctx, p, out, env, buildExtra{}, ldflags...)
}

// buildExtra is what a build adds to go build: flags, build tags and
// files the overlay adds to the main package, by name.
type buildExtra struct {
	flags []string
	tags  []string
	files map[string]string
}

// goBuildWith is goBuild with extra's flags, tags and files. A TSX app's
// bundles, compiled ahead, go in through the overlay too, and it builds
// without the compiler.
func goBuildWith(ctx context.Context, p *project, out string, env []string, extra buildExtra, ldflags ...string) error {
	cmd, done, err := goBuildCommand(ctx, p, out, env, extra, ldflags...)
	if err != nil {
		return err
	}
	defer done()
	// The tool's chatter only matters when it fails: the step's line says
	// what it did, and a failure shows the whole output under its error.
	var output bytes.Buffer
	cmd.Stdout, cmd.Stderr = &output, &output
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go %s failed\n%s", cmd.Args[1], strings.TrimSpace(output.String()))
	}
	return nil
}

// goBuildCommand is the go command goBuildWith runs, and done removes the files it needs no longer than that.
func goBuildCommand(ctx context.Context, p *project, out string, env []string, extra buildExtra, ldflags ...string) (cmd *exec.Cmd, done func(), err error) {
	done = func() {}
	args := append([]string{"build"}, extra.flags...)
	if p.devDebug {
		args = append(args, "-gcflags=all=-N -l")
		ldflags = append(ldflags, "-s=false -w=false")
	}
	if p.buildParallel > 0 {
		args = append(args, "-p", strconv.Itoa(p.buildParallel))
	}
	ldflags = append(append([]string(nil), ldflags...), p.privacyLdflags(env)...)
	args = append(args, "-ldflags", p.ldflags(append(append([]string(nil), ldflags...), p.updateLdflags(env)...)...), "-o", out)
	tags, err := goTags(ctx, p, env, extra.tags...)
	if err != nil {
		return nil, nil, err
	}
	files := map[string]string{}
	for k, v := range extra.files {
		files[k] = v
	}
	if p.testBinary {
		// nsgo test -target: the package's tests, with the backend's test
		// host, in place of the app.
		args = []string{"test", "-c", "-ldflags", p.ldflags(ldflags...), "-o", out}
		tags = append(tags, "nsgotest")
		tsxTest, err := p.tsxTestFile()
		if err != nil {
			return nil, nil, err
		}
		if tsxTest != "" {
			files["zz_nsgo_tsx_test.go"] = tsxTest
		}
		// The app starts from bundles compiled ahead, as its packages do,
		// rather than compiling as it starts, which a phone or a page
		// takes seconds for; the tests still link the compiler for their
		// own files.
		stage(ctx, "compiling TSX")
		bundle, err := p.bundleFile()
		if err != nil {
			return nil, nil, err
		}
		if bundle != "" {
			files["zz_nsgo_bundle.go"] = bundle
		}
	} else {
		stage(ctx, "compiling TSX")
		bundle, err := p.bundleFile()
		if err != nil {
			return nil, nil, err
		}
		if bundle != "" {
			files["zz_nsgo_bundle.go"] = bundle
			tags = append(tags, "nsgo_bundled")
		}
	}
	if p.Release {
		args = append(args, "-trimpath", "-buildvcs=false")
	}
	if p.Dev.Client {
		tags = append(tags, "nsgodev")
		if p.Dev.Automation {
			tags = append(tags, "nsgoautomation")
		}
	}
	tags = uniqueTags(tags)
	args = append(args, tagsArgs(tags)...)
	env, err = smallJSON(ctx, p, env, tags)
	if err != nil {
		return nil, nil, err
	}
	if p.devToken != "" {
		token, err := p.devTokenFile()
		if err != nil {
			return nil, nil, err
		}
		done = func() { os.Remove(token) }
		files["zz_nsgo_devtoken.go"] = token
	}
	overlay, err := p.buildOverlay(ctx, env, tags, extra.flags, files)
	if err != nil {
		done()
		return nil, nil, err
	}
	if overlay != "" {
		args = append(args, "-overlay", overlay)
	}
	stage(ctx, "compiling Go")
	cmd = exec.CommandContext(ctx, "go", append(args, ".")...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	return cmd, done, nil
}

func (p *project) buildOverlay(ctx context.Context, env, tags, flags []string, files map[string]string) (string, error) {
	var overlay string
	if len(files) > 0 {
		var err error
		overlay, err = p.overlay(files)
		if err != nil {
			return "", err
		}
	}
	if !p.packageLicenses || p.testBinary {
		p.nativeLibraries = map[string][]nativeLibrary{}
		if nativeTarget(env) != "" {
			_, err := p.listLicenseSources(ctx, env, tags, flags, overlay, map[string]*licenseSource{})
			return overlay, err
		}
		return overlay, nil
	}
	stage(ctx, "collecting license notices")
	noticeFile, err := p.licenseNoticesFile(ctx, env, tags, flags, overlay)
	if err != nil {
		return "", err
	}
	if noticeFile == "" {
		return overlay, nil
	}
	files["zz_nsgo_notices.go"] = noticeFile
	return p.overlay(files)
}

// devTokenFile writes the file a build for a target that passes the app no environment adds to its main package: it gives the app the run's token in NSGO_DEV_TOKEN as main initializes (devtools.Token). With -ldflags -X, the token would be on go build's command line and the linker's, which every user of the machine may read, and in the binary's build information; this file only its user reads, and goBuildCommand's done removes it.
func (p *project) devTokenFile() (string, error) {
	if err := os.MkdirAll(p.cache(), 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(p.cache(), "devtoken-*.go")
	if err != nil {
		return "", err
	}
	_, err = fmt.Fprintf(f, "// Written by nsgo: nsgo dev's token for this run's server.\n\npackage main\n\nimport \"os\"\n\nfunc init() { os.Setenv(\"NSGO_DEV_TOKEN\", %q) }\n", p.devToken)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

// smallJSON returns env with GOEXPERIMENT=nojsonv2 added: encoding/json's first implementation, which from Go 1.27, where Go builds encoding/json on encoding/json/v2 by default, links into a page about 1 MB smaller, into an Android app about 450 KB and into a desktop one 100 to 450 KB; Go 1.26 builds the first implementation already, and the experiment changes nothing. An app whose packages or dependencies import encoding/json/v2 or jsontext keeps them, as does one whose GOEXPERIMENT, in the environment or set with go env -w, names the experiment either way.
func smallJSON(ctx context.Context, p *project, env, tags []string) ([]string, error) {
	cur, err := goExperiment(ctx, p.Dir, env)
	if err != nil {
		return nil, err
	}
	if strings.Contains(cur, "jsonv2") {
		return env, nil
	}
	args := []string{"list", "-deps", "-f", `{{if not .Standard}}{{join .Imports "\n"}}{{end}}`}
	if p.testBinary {
		args = append(args, "-test")
	}
	args = append(args, tagsArgs(tags)...)
	cmd := exec.CommandContext(ctx, "go", append(args, ".")...)
	cmd.Dir = p.Dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("go list: %w\n%s", err, stderr.Bytes())
	}
	for line := range strings.SplitSeq(string(out), "\n") {
		if line == "encoding/json/v2" || line == "encoding/json/jsontext" {
			return env, nil
		}
	}
	return append(env, "GOEXPERIMENT="+experiments(cur, "nojsonv2")), nil
}

// goExperiment returns the GOEXPERIMENT the go command takes in dir under env, which counts what go env -w set, as a GOEXPERIMENT the tool adds to the environment replaces it.
func goExperiment(ctx context.Context, dir string, env []string) (string, error) {
	cmd := exec.CommandContext(ctx, "go", "env", "GOEXPERIMENT")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("go env GOEXPERIMENT: %w\n%s", err, stderr.Bytes())
	}
	return strings.TrimSpace(string(out)), nil
}

// experiments returns the experiments cur names with extra added.
func experiments(cur, extra string) string {
	if cur != "" {
		return cur + "," + extra
	}
	return extra
}

// buildMacApp writes <dir>/<name>.app for one architecture.
func buildMacApp(ctx context.Context, p *project, dir, arch string) (string, error) {
	minimum, err := p.appleMinimum(nil)
	if err != nil {
		return "", err
	}
	app := filepath.Join(dir, p.Name+".app")
	bin := filepath.Join(app, "Contents", "MacOS", p.Name)
	if err := goBuild(ctx, p, bin, []string{"GOOS=darwin", "GOARCH=" + arch, "CGO_ENABLED=0"}, "-macos="+minimum); err != nil {
		return "", err
	}
	stage(ctx, "bundling the app")
	resources := filepath.Join(app, "Contents", "Resources")
	if err := copyResources(p, resources); err != nil {
		return "", err
	}
	if err := os.MkdirAll(resources, 0o755); err != nil {
		return "", err
	}
	icon, err := writeMacIcon(ctx, p, resources)
	if err != nil {
		return "", err
	}
	names, err := writeAppleNames(p, resources)
	if err != nil {
		return "", err
	}
	if err := p.writeFileTypeIcons(resources); err != nil {
		return "", err
	}
	extra := icon + "\n" + names + "\n" + p.fileTypesPlist() + "\n" + p.urlSchemesPlist() + "\n" + p.widgetPlist(false) + "\n" + p.sparklePlist("macos_"+goArchName(arch))
	if game := p.gamePlist(true, false); game != "" {
		extra += "\n" + game
	}
	if err := writePlist(filepath.Join(app, "Contents", "Info.plist"), p, macPlist, extra); err != nil {
		return "", err
	}
	if err := p.writePrivacyManifest(resources); err != nil {
		return "", err
	}
	if err := addMacWidgets(ctx, p, app, arch); err != nil {
		return "", err
	}
	if err := addMacStoreKit(ctx, p, app, arch); err != nil {
		return "", err
	}
	if err := addSparkle(ctx, p, app); err != nil {
		return "", err
	}
	stage(ctx, "signing")
	if err := addMacLibraries(ctx, p, app, arch); err != nil {
		return "", err
	}
	return app, run(ctx, "codesign", "--force", "--sign", "-", app)
}

// buildWindows writes <dir>/<name>.exe with the icon and version linked in, and the resources beside it with WinSparkle.dll where the app updates.
func buildWindows(ctx context.Context, p *project, dir, arch string) error {
	syso, removeResources, err := writeWindowsResources(ctx, p, arch)
	defer removeResources()
	if err != nil {
		return err
	}
	if err := goBuild(ctx, p, filepath.Join(dir, p.Name+".exe"),
		[]string{"GOOS=windows", "GOARCH=" + arch, "CGO_ENABLED=0"}, "-H=windowsgui"); err != nil {
		return err
	}
	if p.windowsCommandLine() {
		stage(ctx, "compiling the command companion")
		if err := buildWindowsCommand(ctx, p, dir, arch, syso); err != nil {
			return err
		}
	} else if p.Windows.CommandLine {
		detailIn(ctx, "the Microsoft Store's package leaves out the command companion, which no console finds there")
	}
	stage(ctx, "copying resources")
	if err := copyResources(p, dir); err != nil {
		return err
	}
	if err := addWinSparkle(ctx, p, dir, arch); err != nil {
		return err
	}
	return p.addNativeLibraries(ctx, "windows_"+goArchName(arch), dir)
}

// buildLinux writes <dir>/<name> with the resources and icon beside it.
func buildLinux(ctx context.Context, p *project, dir, arch string) error {
	if err := goBuild(ctx, p, filepath.Join(dir, p.Name),
		[]string{"GOOS=linux", "GOARCH=" + arch, "CGO_ENABLED=0"}); err != nil {
		return err
	}
	stage(ctx, "copying resources")
	if err := copyResources(p, dir); err != nil {
		return err
	}
	if err := writeLinuxIcon(p, dir); err != nil {
		return err
	}
	return p.addNativeLibraries(ctx, "linux_"+goArchName(arch), dir)
}

// buildFPS is -fps: the site starts its app with --nsgo-fps, which a page
// can only carry from the build.
var buildFPS bool

// harmonyFlags are the engine's flags -run passes to the HAP's ability as
// Want parameters, so the same HAP can start with the monitor on or off.
// The site still writes perf.json for a launch that carries no parameters.
func harmonyFlags() []string {
	if buildFPS {
		return []string{"--nsgo-fps"}
	}
	return nil
}

// buildHarmony writes the site the HAP carries to dir: the engine compiled
// for ArkWeb with the harmonyos backend, the Go runtime's wasm_exec.js, and
// the page that starts them under the host's bridge.
func buildHarmony(ctx context.Context, p *project, dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := goBuildWith(ctx, p, filepath.Join(dir, "app.wasm"),
		[]string{"GOOS=js", "GOARCH=wasm"}, buildExtra{tags: []string{"nsgoharmony"}}); err != nil {
		return err
	}
	goroot, err := output(ctx, "go", "env", "GOROOT")
	if err != nil {
		return err
	}
	js, err := os.ReadFile(filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "wasm_exec.js"), js, 0o644); err != nil {
		return err
	}
	// The site is reused between builds, so an earlier -fps build's flag
	// would start the monitor in a later build that did not ask for it.
	// A Want's parameters are the runtime way to ask (runHarmony); this
	// file is only the fallback for a launch that carries none.
	if err := os.Remove(filepath.Join(dir, "perf.json")); err != nil && !os.IsNotExist(err) {
		return err
	}
	if buildFPS {
		if err := os.WriteFile(filepath.Join(dir, "perf.json"), []byte("{}"), 0o644); err != nil {
			return err
		}
	}
	// The card's page reads this from the HAP's rawfile directory: its data
	// comes from the app, since a card has no Web view for the engine.
	title := p.Name
	if p.Widget != nil && p.Widget.Title != "" {
		title = p.Widget.Title
	}
	widget, err := json.Marshal(map[string]string{"title": title})
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "widget.json"), widget, 0o644); err != nil {
		return err
	}
	// The app's resources travel beside the page, so an "asset:" image
	// reads its bytes from the HAP's rawfile directory: the page cannot
	// open a file, and ArkUI's Image cannot name a path inside the HAP.
	res := filepath.Join(dir, "resources")
	if err := os.RemoveAll(res); err != nil {
		return err
	}
	if err := copyMobileResources(p, res); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "index.html"), harmony.Boot(), 0o644)
}

// buildHarmonyHAP assembles the HAP, naming the installed bundle from the
// app's id when the machine can build a host for it (harmony.CompileHost)
// and the configuration asks for it. Signing always uses the app's exact
// id. Without the SDK an unsigned auto build falls back to the
// committed host's padded name, which harmony.Package swaps in the bytecode.
// Exact mode fails when the SDK cannot build a host; auto keeps the app
// building and says why.
func buildHarmonyHAP(ctx context.Context, p *project, dir, site string, code int) (string, error) {
	mode := p.Build.HarmonyosBundles
	if mode == "" {
		mode = "auto"
	}
	exact := func(output string) (string, error) {
		hostDir, err := harmony.CompileHost(ctx, p.ID)
		if err != nil {
			return "", err
		}
		return harmony.PackageWithHost(output, site, hostDir, p.Name, p.ID, p.Version, code, p.Harmonyos.Permissions)
	}
	padded := func() (string, error) {
		return harmony.Package(dir, site, p.Name, p.ID, p.Version, code, p.Harmonyos.Permissions)
	}
	report := func(name string) {
		if name != "" {
			stage(ctx, "harmonyos: bundle %s", name)
		}
	}
	if p.Harmonyos.Signing != nil {
		signer, err := p.harmonySigner(ctx)
		if err != nil {
			return "", err
		}
		work, err := os.MkdirTemp(dir, ".hap-signing-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(work)
		unsigned, err := exact(work)
		if err != nil {
			return "", fmt.Errorf("HarmonyOS signing needs an exact bundle name: %w", err)
		}
		stage(ctx, "signing the HarmonyOS HAP")
		signed := filepath.Join(work, "signed-"+filepath.Base(unsigned))
		if err := signer.Sign(ctx, unsigned, signed); err != nil {
			return "", err
		}
		hap := filepath.Join(dir, filepath.Base(unsigned))
		if err := os.Rename(signed, hap); err != nil {
			return "", err
		}
		report(p.ID)
		return hap, nil
	}
	switch mode {
	case "exact":
		hap, err := exact(dir)
		if err != nil {
			return "", fmt.Errorf("build.harmonyos_bundles is exact: %w", err)
		}
		report(p.ID)
		return hap, nil
	case "padded":
		hap, err := padded()
		if err != nil {
			return "", err
		}
		name, _ := harmony.BundleName(p.ID)
		report(name)
		return hap, nil
	case "auto":
		hap, err := exact(dir)
		if err == nil {
			report(p.ID)
			return hap, nil
		}
		if !errors.Is(err, harmony.ErrNoSDK) {
			stage(ctx, "harmonyos: %v; using a padded bundle name", err)
		}
		hap, err = padded()
		if err != nil {
			return "", err
		}
		name, _ := harmony.BundleName(p.ID)
		report(name)
		return hap, nil
	}
	return "", fmt.Errorf("harmonyos: unknown build.harmonyos_bundles %q: use auto, exact or padded", mode)
}

func (p *project) harmonySigner(ctx context.Context) (*harmony.Signer, error) {
	switch p.Build.HarmonyosBundles {
	case "", "auto", "exact":
	case "padded":
		return nil, fmt.Errorf("harmonyos.signing needs the app's exact id: use auto or exact for build.harmonyos_bundles")
	default:
		return nil, fmt.Errorf("harmonyos: unknown build.harmonyos_bundles %q: use auto or exact for signing", p.Build.HarmonyosBundles)
	}
	signer, err := harmony.OpenSigner(p.Dir, *p.Harmonyos.Signing)
	if err != nil {
		return nil, err
	}
	if err := signer.CheckProfile(ctx, p.ID); err != nil {
		return nil, err
	}
	return signer, nil
}

// buildWeb writes the site to dir: index.html, nsgo.json, nsgo.js (the
// loader), wasm_exec.js, app.wasm, nsgo.yaml's host module and the
// resources beside them. A release names every file but the page,
// nsgo.json and manifest.webmanifest by its content, so hosts cache them
// for good, and tells the runtime the name each resource got; a dev build
// keeps the names and follows nsgo dev's server.
func buildWeb(ctx context.Context, p *project, dir string, dev bool) error {
	base, err := p.webBase()
	if err != nil {
		return err
	}
	wasm := filepath.Join(dir, "app.wasm")
	offline := !dev && p.Web.Offline
	var published []string
	publish := func(name string) {
		if offline {
			published = append(published, name)
		}
	}
	if !dev {
		// A release's TSX is split: the page fetches each module an
		// import() names when the import runs. Its files hold the scripts
		// the page's realm runs, wrapped so the runtime finds them by name.
		stage(ctx, "compiling TSX")
		split, err := compileSplit(p.Dir, func(file, name string, data []byte) (string, error) {
			data = []byte(websrc.Bundle(name, string(data)))
			file = hashedName(file, data)
			publish(file)
			if err := os.MkdirAll(filepath.Join(dir, filepath.Dir(file)), 0o755); err != nil {
				return "", err
			}
			return file, os.WriteFile(filepath.Join(dir, file), data, 0o644)
		})
		if err != nil {
			return err
		}
		// The app links none of them.
		split.shipped = map[string]bool{}
		for e := range split.main {
			split.shipped[e] = true
		}
		p.split = split
		defer func() { p.split = nil }()
	}
	if err := goBuild(ctx, p, wasm, []string{"GOOS=js", "GOARCH=wasm"}); err != nil {
		return err
	}
	stage(ctx, "writing the site")
	goroot, err := output(ctx, "go", "env", "GOROOT")
	if err != nil {
		return err
	}
	js, err := os.ReadFile(filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"))
	if err != nil {
		return err
	}
	img, err := p.iconImage()
	if err != nil {
		return err
	}
	page := map[string]any{"Name": p.Name, "Reload": dev, "Base": html.EscapeString(base),
		"Color": appearanceColor(p.Appearance), "Appearance": p.Appearance}
	// app is nsgo.json: what the loader needs to show the app.
	info, err := os.Stat(wasm)
	if err != nil {
		return err
	}
	app := webApp{Wasm: "app.wasm", WasmSize: info.Size(), Base: base}
	// write puts data in dir, by its content's name in a release, and
	// returns the name.
	write := func(name string, data []byte) (string, error) {
		if !dev {
			name = hashedName(name, data)
			publish(name)
		}
		return name, os.WriteFile(filepath.Join(dir, name), data, 0o644)
	}
	if app.Exec, err = write("wasm_exec.js", js); err != nil {
		return err
	}
	if p.split != nil {
		// The realm's programs and the app's bundles are the page's files,
		// so a Content-Security-Policy needs no 'unsafe-eval'
		// (docs/architecture/tsx.md).
		if len(p.split.main) > 0 {
			if app.Runtime, err = write("nsgo-runtime.js", []byte(websrc.Runtime())); err != nil {
				return err
			}
			if offline {
				if app.Realm, err = write("realm.html", []byte("<!doctype html>\n<meta charset=\"utf-8\">\n")); err != nil {
					return err
				}
			}
		}
		for _, e := range slices.Sorted(maps.Keys(p.split.main)) {
			file := strings.TrimSuffix(filepath.Base(e), filepath.Ext(e)) + ".js"
			name, err := write(file, []byte(websrc.Bundle(e, p.split.main[e])))
			if err != nil {
				return err
			}
			app.Scripts = append(app.Scripts, name)
		}
	}
	if page["Loader"], err = write("nsgo.js", loaderJS); err != nil {
		return err
	}
	app.Loader = page["Loader"].(string)
	if p.Web.Host != "" {
		data, err := p.webHost()
		if err != nil {
			return err
		}
		if app.Host, err = write("host.js", data); err != nil {
			return err
		}
	}
	if dev {
		if err := copyResources(p, dir); err != nil {
			return err
		}
	} else {
		data, err := os.ReadFile(wasm)
		if err != nil {
			return err
		}
		app.Wasm = hashedName("app.wasm", data)
		path := filepath.Join(dir, app.Wasm)
		if err := os.Rename(wasm, path); err != nil {
			return err
		}
		stage(ctx, "compressing the web module")
		if err := writeWebWasmCopies(path, data); err != nil {
			return err
		}
		publish(app.Wasm)
		if app.Resources, err = copyHashedResources(p, dir); err != nil {
			return err
		}
		for _, name := range app.Resources {
			publish(name)
		}
	}
	if img != nil {
		if page["Icon"], err = write("icon.png", scaledPNG(img, 256)); err != nil {
			return err
		}
		if page["TouchIcon"], err = write("apple-touch-icon.png", scaledPNG(img, 180)); err != nil {
			return err
		}
	}
	if !dev {
		var icons []webIcon
		for _, size := range []int{192, 512} {
			if img == nil {
				break
			}
			name, err := write(fmt.Sprintf("icon-%d.png", size), scaledPNG(img, size))
			if err != nil {
				return err
			}
			icons = append(icons, webIcon{Src: name, Sizes: fmt.Sprintf("%dx%d", size, size), Type: "image/png"})
		}
		if err := writeWebManifest(p, dir, icons); err != nil {
			return err
		}
		page["Manifest"] = "manifest.webmanifest"
		if offline {
			data, err := os.ReadFile(filepath.Join(dir, "manifest.webmanifest"))
			if err != nil {
				return err
			}
			if page["Manifest"], err = write("manifest.webmanifest", data); err != nil {
				return err
			}
		}
	}
	data, err := json.MarshalIndent(app, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "nsgo.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if data, err = json.Marshal(app); err != nil {
		return err
	}
	// The page's own files: its style, and the module that runs the app, so
	// a policy that runs none of its scripts inline still runs it.
	page["Exec"] = app.Exec
	if page["Style"], err = write("page.css", pageCSS(p)); err != nil {
		return err
	}
	if page["Module"], err = write("mount.js", mountJS(app.Loader, string(data), offline)); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "index.html"))
	if err != nil {
		return err
	}
	err = indexHTML.Execute(f, page)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil || !offline {
		return err
	}
	return writeOfflineWorker(dir, append(published, "index.html"))
}

// webApp is a site's nsgo.json, which names the files of the app for the
// loader, relative to itself.
type webApp struct {
	Base     string `json:"base,omitempty"`
	Wasm     string `json:"wasm"`
	WasmSize int64  `json:"wasmSize"`
	Exec     string `json:"exec"`
	Loader   string `json:"loader"`
	Host     string `json:"host,omitempty"`
	// Runtime is the script that gives the page's realm the runtime's
	// programs, and Scripts the app's bundles: the loader runs them in the
	// realm before the app starts, so a Content-Security-Policy needs no
	// 'unsafe-eval' (docs/architecture/tsx.md).
	Runtime string   `json:"runtime,omitempty"`
	Scripts []string `json:"scripts,omitempty"`
	// Realm is the empty page an offline release's realm frame shows, so its service worker serves the realm's scripts and lazy chunks.
	Realm string `json:"realm,omitempty"`
	// A published site lists its assets even when empty. A dev build serves names directly.
	Resources map[string]string `json:"resources,omitzero"`
}

// loaderJS is nsgo.js, which runs an app in a page.
//
//go:embed web/nsgo.js
var loaderJS []byte

// webHost reads nsgo.yaml's host module, which must be inside the app's
// directory once symbolic links are resolved.
func (p *project) webHost() ([]byte, error) {
	dir, err := filepath.EvalSymlinks(p.Dir)
	if err != nil {
		return nil, err
	}
	path, err := filepath.EvalSymlinks(p.resolve(p.Web.Host))
	if err != nil {
		return nil, fmt.Errorf("nsgo.yaml: web.host: %w", err)
	}
	if rel, err := filepath.Rel(dir, path); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, fmt.Errorf("nsgo.yaml: web.host %s is outside the app's directory", p.Web.Host)
	}
	return os.ReadFile(path)
}

// pageCSS is the page's style: the app fills the window, and the page
// never zooms, as an app's window does not; views that zoom take pinches
// themselves.
func pageCSS(p *project) []byte {
	css := "* { touch-action: pan-x pan-y; }\nhtml, body { margin: 0; height: 100%;"
	if color := appearanceColor(p.Appearance); color != "" {
		css += " background: " + color + ";"
	}
	return []byte(css + ` }
body > .nsgo-load-progress { position: absolute; inset: 0; margin: auto; width: min(16rem, 70%); }
`)
}

// mountJS is the module that runs the app, and says so when the app cannot
// start. It keeps Safari, which ignores the viewport's user-scalable, from
// zooming the page under a pinch.
func mountJS(loader, config string, offline bool) []byte {
	var register string
	if offline {
		register = `.then(() => {
  navigator.serviceWorker?.register(new URL(` + strconv.Quote("./"+offlineWorkerName) + `, import.meta.url), { updateViaCache: "none" })
    .catch((e) => console.warn("The app could not be made available offline:", e));
})`
	}
	return []byte(`// Written by nsgo: the page runs the app.
import { mount } from ` + strconv.Quote("./"+loader) + `;
document.addEventListener("gesturestart", (e) => e.preventDefault());
mount(null, null, { config: ` + config + ` })` + register + `.catch((e) => {
  console.error(e);
  const message = document.createElement("pre");
  message.textContent = "The app could not be loaded: " + e.message;
  message.style.cssText = "margin:20px;white-space:pre-wrap;font:13px ui-monospace,monospace";
  document.body.append(message);
});
`)
}

var indexHTML = template.Must(template.New("index").Parse(`<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1, maximum-scale=1, user-scalable=no, viewport-fit=cover">
<title>{{.Name}}</title>
{{- if .Base}}
<base href="{{.Base}}">
{{- end}}
{{- if .Appearance}}
<meta name="color-scheme" content="{{.Appearance}}">
{{- end}}
{{- if .Color}}
<meta name="theme-color" content="{{.Color}}">
{{- end}}
{{- if .Icon}}
<link rel="icon" href="{{.Icon}}">
<link rel="apple-touch-icon" href="{{.TouchIcon}}">
{{- end}}
{{- if .Manifest}}
<link rel="manifest" href="{{.Manifest}}">
{{- end}}
<link rel="stylesheet" href="{{.Style}}">
<script src="{{.Exec}}"></script>
</head>
<body>
<script type="module" src="{{.Module}}"></script>
{{- if .Reload}}
<script type="module">
// nsgo dev's server: each new build loads, a failed one shows over the app. Its address for the page carries the run's token in the fragment, which the page moves to the tab's sessionStorage, out of the address bar, the history and whatever copies the address, and where a reload and the app's Go (devtools.Token) take it from; a browser that keeps no storage for the page leaves it in the address.
const token = (() => {
  const hash = new URLSearchParams(location.hash.slice(1));
  const given = hash.get("nsgo-token");
  try {
    if (given === null) return sessionStorage.getItem("nsgo-token") ?? "";
    sessionStorage.setItem("nsgo-token", given);
  } catch {
    return given ?? "";
  }
  hash.delete("nsgo-token");
  const rest = hash.toString();
  history.replaceState(history.state, "", location.pathname + location.search + (rest ? "#" + rest : ""));
  return given;
})();
const events = new EventSource("/_nsgo/events?nsgo-token=" + encodeURIComponent(token));
let overlay;
events.onmessage = (e) => {
  const [kind, ...rest] = e.data.split(" ");
  if (kind === "reload") location.reload();
  if (kind === "ok" && overlay) { overlay.remove(); overlay = null; }
  if (kind !== "error") return;
  overlay?.remove();
  overlay = document.createElement("div");
  overlay.style.cssText = "position:fixed;inset:0;z-index:2147483647;background:rgba(17,17,17,.6);padding:48px 24px;display:flex;align-items:center;justify-content:center;box-sizing:border-box;font:13px system-ui;color:#f2f2f2";
  const panel = document.createElement("div");
  panel.style.cssText = "width:100%;max-width:720px;max-height:100%;box-sizing:border-box;display:flex;flex-direction:column;gap:16px;padding:20px;border-radius:12px;background:#1c1c1c;border:1px solid rgba(255,255,255,.1)";
  const say = (key, source) => globalThis.nsgoDevMessages?.[key] ?? source;
  const title = document.createElement("div");
  title.textContent = say("buildFailed", "Build failed");
  title.style.cssText = "color:#ff5c5c;font:bold 18px system-ui";
  const text = document.createElement("pre");
  text.textContent = JSON.parse(rest.join(" "));
  text.style.cssText = "margin:0;padding:12px;max-height:360px;overflow:auto;border-radius:8px;background:rgba(0,0,0,.35);white-space:pre-wrap;font:12px ui-monospace,monospace";
  const button = (label, onclick) => {
    const b = document.createElement("button");
    b.textContent = label;
    b.onclick = onclick;
    b.style.cssText = "padding:6px 14px;border:0;border-radius:6px;background:rgba(255,255,255,.12);color:inherit;font:inherit";
    return b;
  };
  const buttons = document.createElement("div");
  buttons.style.cssText = "display:flex;gap:8px;justify-content:flex-end";
  buttons.append(
    button(say("copy", "Copy"), () => navigator.clipboard.writeText(text.textContent)),
    button(say("reload", "Reload"), () => fetch("/_nsgo/reload", { method: "POST", headers: { "X-NSGo-Token": token } })),
  );
  panel.append(title, text, buttons);
  overlay.append(panel);
  document.body.append(overlay);
};
</script>
{{- end}}
</body>
</html>
`))

// writePlist writes t's plist for p, with extra entries, such as the
// icon's, at the end of its top dictionary.
func writePlist(path string, p *project, t *template.Template, extra string) error {
	if err := p.validatePlistMinimum(t); err != nil {
		return err
	}
	var out bytes.Buffer
	if err := t.Execute(&out, p); err != nil {
		return err
	}
	plist := out.String()
	if extra = strings.TrimSpace(extra); extra != "" {
		end := strings.LastIndex(plist, "</dict>")
		plist = plist[:end] + "\t" + extra + "\n" + plist[end:]
	}
	return os.WriteFile(path, []byte(plist), 0o644)
}

const plistHead = `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key><string>{{.Name}}</string>
	<key>CFBundleIdentifier</key><string>{{.ID}}</string>
	<key>CFBundleName</key><string>{{.Name}}</string>
	<key>CFBundlePackageType</key><string>APPL</string>
	<key>CFBundleShortVersionString</key><string>{{.Version}}</string>
	<key>CFBundleVersion</key><string>{{.BundleVersion}}</string>
{{- range $key, $value := .Apple.Info}}
{{- if and (ne $key "MinimumOSVersion") (ne $key "LSMinimumSystemVersion")}}
	<key>{{html $key}}</key>{{$.InfoValue $value}}
{{- end}}
{{- end}}
`

var macPlist = template.Must(template.New("mac").Parse(plistHead + `	<key>LSMinimumSystemVersion</key><string>{{.MacOSMinimum}}</string>
	<key>NSHighResolutionCapable</key><true/>
{{- if and .Copyright (not (index .Apple.Info "NSHumanReadableCopyright"))}}
	<key>NSHumanReadableCopyright</key><string>{{html .Copyright}}</string>
{{- end}}
{{- if .MacOSCategory}}
	<key>LSApplicationCategoryType</key><string>{{html .MacOSCategory}}</string>
{{- end}}
{{- if eq .Appearance "light"}}
	<key>NSRequiresAquaSystemAppearance</key><true/>
{{- end}}
</dict>
</plist>
`))

func rel(path string) string {
	if wd, err := os.Getwd(); err == nil {
		if r, err := filepath.Rel(wd, path); err == nil && !strings.HasPrefix(r, "..") {
			return r
		}
	}
	return path
}
