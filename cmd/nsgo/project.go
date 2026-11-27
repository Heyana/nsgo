package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	projectcfg "github.com/nsgo-dev/nsgo/project"
)

// Type aliases for nsgo.yaml's nested config types, which live in package project.
type (
	widget           = projectcfg.Widget
	licensePolicy    = projectcfg.LicensePolicy
	launchScreen     = projectcfg.LaunchScreen
	launchAppearance = projectcfg.LaunchAppearance
	icons            = projectcfg.Icons
	androidIcon      = projectcfg.AndroidIcon
	cloudConfig      = projectcfg.CloudConfig
	privacySettings  = projectcfg.PrivacySettings
	fileType         = projectcfg.FileType
	command          = projectcfg.Command
	widgetKind       = projectcfg.WidgetKind
	liveActivities   = projectcfg.LiveActivities
	appClip          = projectcfg.AppClip
	storeCompliance  = projectcfg.StoreCompliance
	linuxDeps        = projectcfg.LinuxDeps
	updatesConfig    = projectcfg.UpdatesConfig
	steamConfig      = projectcfg.SteamConfig
	storeConfig      = projectcfg.StoreConfig
	itchConfig       = projectcfg.ItchConfig
	assetPackConfig  = projectcfg.AssetPack
	signing          = projectcfg.Signing
)

// project is an app's nsgo.yaml configuration beside the tool's run-time state.
type project struct {
	projectcfg.Config `yaml:",inline"`

	// resourceDir replaces Dir's resources directory for a generated preview host.
	resourceDir string
	// Release strips symbols and debug information for the smallest
	// binaries; nsgo build sets it, nsgo dev does not.
	Release bool `yaml:"-"`
	// idDefault records that nsgo.yaml gave no id, so it follows the name.
	idDefault bool
	// devURL is the dev server's address for targets that pass the app no environment, and devToken the run's token for it, which the build gives the app in a file it adds (devTokenFile).
	devURL, devToken string
	// devDebug retains DWARF and disables optimization for Delve.
	devDebug bool
	// devClient builds the dev client in, which connects to nsgo dev's server:
	// nsgo dev sets it. devTrace has the client send the server each commit:
	// nsgo dev -trace sets it.
	devClient bool
	devTrace  bool
	// devProfile enables a runtime profile capture from the dev server.
	devProfile bool
	// devHighlight has the client outline what each commit changed: nsgo dev -highlight sets it.
	devHighlight bool
	// devDevtools has the client serve DevTools: nsgo dev -devtools sets it.
	devDevtools bool
	// devLayout is what DevTools' layout audit starts showing, as bounds,grid=8: nsgo dev -layout sets it.
	devLayout string
	// devAutomation links the agent nsgo dev -automation drives the app through (package nsgotest), with the nsgoautomation tag.
	devAutomation bool
	// devLimits runs a TSX app sandboxed, as tsx.View would, within these limits
	// (tsx.ParseLimits) and its capabilities: nsgo dev -limits sets it.
	devLimits string
	// buildRun starts the package built for this machine afterwards (-run),
	// on buildDevice (-device), an iPhone signed for buildTeam (-team);
	// buildEmbed packages the app for host apps of their own (-embed).
	buildRun, buildEmbed bool
	// buildAll records that the targets were given as all.
	buildAll               bool
	buildDynamic           bool
	buildDevice, buildTeam string
	// updatesPublic is Updates.Key's decoded public key, and updatesRepo the
	// GitHub repository the feed is in without an explicit Updates.Feed.
	updatesPublic ed25519.PublicKey
	updatesRepo   string
	// updatesNotes is build's -release-notes file, and updatesNotesHTML its
	// Markdown as HTML for the appcasts.
	updatesNotes     string
	updatesNotesHTML string
	// testBinary builds the package's tests with the nsgotest tag in
	// place of the app, for nsgo test -target.
	testBinary bool
	// bare records that the directory has no nsgo.yaml: a library, or a
	// module's root, that nsgo test and nsgo check take as a tree of
	// packages.
	bare bool
	// steps, when set, are what nsgo test <entry> runs in place of the
	// package's tests: the steps of standard input on the app at entry.
	steps *steps
	// split, set while a web release builds, holds its split bundles.
	split *splitBundles
	// clipParent is the app's ID when this is its App Clip's project
	// (clipProject), and parentClip the app's app_clip.
	clipParent string
	parentClip *appClip
	// released lists the files nsgo build put in the release directory
	// for the target it builds.
	released []string
	// checkStorePrivacy validates the Apple manifests in a store package.
	checkStorePrivacy bool
	// buildParallel bounds Go's compile workers for one target while several
	// targets build together. Zero keeps Go's default.
	buildParallel int
	// packageLicenses is set by build for a standalone app package.
	packageLicenses bool
	// noticeArchitectures is set for a bundle with more than one Go binary.
	noticeArchitectures []string
	thirdPartyNotices   string
	// nativeLibraries are the declarations selected for each executable slice.
	nativeLibraries map[string][]nativeLibrary
	// nativeLinuxDeps are, for each Linux target, the linux entries of the selected packages, in the order go list gives the packages (linuxRequirements).
	nativeLinuxDeps map[string][]nativeLinuxDeps
}

// openOutput reports whether nsgo build opens its output directory.
func (p *project) openOutput() bool { return p.Config.OpenOutput() }

// installers reports whether to make an installer for a desktop system
// or an expanded target of that system.
func (p *project) installers(target string) bool {
	if p.channel() == channelSteam || (p.Build.Installers != nil && !*p.Build.Installers) {
		return false
	}
	system, _, _ := strings.Cut(target, "_")
	_, desktop := desktopSystems[system]
	return desktop && !slices.Contains(p.Build.SkipInstallers, system)
}

func validateSkipInstallers(systems []string) error {
	return projectcfg.ValidateSkipInstallers(systems)
}

// hotReload reports whether nsgo dev rebuilds the app on changes, unless
// dev.hot_reload turns it off; without it, dev runs the app once and exits
// when the app does.
func (p *project) hotReload() bool { return p.Config.HotReload() }

// loadOrDir is load, or for a directory without nsgo.yaml a bare project
// named after it.
func loadOrDir(dir string) (*project, error) {
	p, err := load(dir)
	if !errors.Is(err, os.ErrNotExist) {
		return p, err
	}
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if fi, err := os.Stat(abs); err != nil || !fi.IsDir() {
		return nil, fmt.Errorf("no directory %s", dir)
	}
	name := filepath.Base(abs)
	return &project{
		Config:    projectcfg.Config{Dir: abs, Name: name, ID: generatedID(name)},
		idDefault: true,
		bare:      true,
	}, nil
}

func load(dir string) (*project, error) {
	cfg, err := projectcfg.Load(dir)
	if err != nil {
		return nil, err
	}
	p := &project{
		Config:    *cfg,
		idDefault: cfg.DefaultID(),
		buildAll:  cfg.AllTargets(),
	}
	if err := p.resolveUpdatesRuntime(context.Background()); err != nil {
		return nil, fmt.Errorf("nsgo.yaml: %w", err)
	}
	return p, nil
}

// generatedID is the bundle ID of a project whose nsgo.yaml gives none, or of a bare project, made from its name.
func generatedID(name string) string { return projectcfg.GeneratedID(name) }

// resolve makes a path from nsgo.yaml or a flag absolute against the app's
// directory.
func (p *project) resolve(path string) string { return p.Config.Resolve(path) }

// cache is the project's directory for development builds.
func (p *project) cache(parts ...string) string {
	return filepath.Join(append([]string{p.Dir, ".nsgo"}, parts...)...)
}

// xflag is the -X flag setting the string variable name to value, quoted as go build splits -ldflags: in single quotes, or double quotes for a value with a single quote, as an app named Bob's Notes has. Nothing escapes inside either, so load refuses a name with both.
func xflag(name, value string) string {
	if strings.Contains(value, "'") {
		return `-X "` + name + "=" + value + `"`
	}
	return "-X '" + name + "=" + value + "'"
}

// appLdflags joins the app's linker flags, nsgo.yaml's build.ldflags and those of -ldflags, into one -ldflags value.
func (p *project) appLdflags() string {
	return strings.Join(p.Build.LDFlags, " ")
}

// corePath is package core's, which holds the app's identity (core.App).
const corePath = "github.com/nsgo-dev/nsgo/core"

// identityLdflags are the -X flags building nsgo.yaml's name, ID, version and build number into core.App, which the app and its commands take.
func (p *project) identityLdflags() string {
	return strings.Join([]string{xflag(corePath+".appName", p.Name), xflag(corePath+".appID", p.ID),
		xflag(corePath+".appVersion", p.Version), xflag(corePath+".appBuild", p.BundleVersion())}, " ")
}

// ldflags builds nsgo.yaml's name, ID, version and build number into
// core.App, and passes the rest of what nsgo.Run takes from it.
func (p *project) ldflags(extra ...string) string {
	const pkg = "github.com/nsgo-dev/nsgo"
	// The dev client, which nsgo.Run starts in dev builds.
	const devPkg = pkg + "/internal/dev"
	flags := p.identityLdflags()
	// The linker keeps the last -X of a variable, so the app's flags go first and cannot replace the tool's.
	if app := p.appLdflags(); app != "" {
		flags = app + " " + flags
	}
	if p.Appearance != "" {
		flags += fmt.Sprintf(" -X %s.appearance=%s", pkg, p.Appearance)
	}
	if p.devURL != "" {
		flags += fmt.Sprintf(" -X '%s.devServer=%s' -X '%s.devServer=%s'", devPkg, p.devURL, compilerPath, p.devURL)
	}
	if p.devClient && p.devTrace {
		flags += fmt.Sprintf(" -X %s.devTrace=1", devPkg)
	}
	if p.devClient && p.devProfile {
		flags += fmt.Sprintf(" -X %s.devProfile=1", devPkg)
	}
	if p.devClient && p.devHighlight {
		flags += fmt.Sprintf(" -X %s.devHighlight=1", devPkg)
	}
	if p.devClient && p.devDevtools {
		flags += fmt.Sprintf(" -X %s.devDevtools=1", devPkg)
	}
	if p.devClient && p.devLayout != "" {
		flags += fmt.Sprintf(" -X %s.devLayout=%s", devPkg, p.devLayout)
	}
	if p.devClient && p.devLimits != "" {
		flags += fmt.Sprintf(" -X '%s.devLimits=%s'", compilerPath, p.devLimits)
		if caps, _ := p.capabilities(); caps != nil {
			data, _ := json.Marshal(caps)
			flags += fmt.Sprintf(" -X %s.devCapabilities=%s", compilerPath, base64.RawURLEncoding.EncodeToString(data))
		}
	}
	if p.Release {
		flags += fmt.Sprintf(" -X %s.appRelease=1 -s -w", corePath)
	}
	for _, e := range extra {
		flags += " " + e
	}
	return flags
}

// goArchName maps Go's amd64 to x64, the name packages use.
func goArchName(arch string) string {
	if arch == "amd64" {
		return "x64"
	}
	return arch
}

// iosOrientations are apple.orientations' names, in the order a default list declares them, with the UIKit orientations they stand for.
var iosOrientations = []struct{ name, plist string }{
	{"portrait", "UIInterfaceOrientationPortrait"},
	{"portrait_upside_down", "UIInterfaceOrientationPortraitUpsideDown"},
	{"landscape_left", "UIInterfaceOrientationLandscapeLeft"},
	{"landscape_right", "UIInterfaceOrientationLandscapeRight"},
}

// IOSIPadOrientationsPlist writes the iPad orientation array for Info.plist.
func (p *project) IOSIPadOrientationsPlist() string {
	var b strings.Builder
	b.WriteString("<array>")
	for _, o := range iosOrientations {
		if len(p.Apple.Orientations) == 0 || slices.Contains(p.Apple.Orientations, o.name) {
			b.WriteString("<string>" + o.plist + "</string>")
		}
	}
	b.WriteString("</array>")
	return b.String()
}
