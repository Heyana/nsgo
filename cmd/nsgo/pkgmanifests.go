package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"go.yaml.in/yaml/v3"
)

// Each direct release with desktop installers writes package manager
// manifests into the releases directory for the platforms that built
// installers:
//   - homebrew/<slug>.rb: a Homebrew cask with auto_updates true for the
//     release's macOS disk images (universal, dual-arch or single-arch).
//   - winget/<id>.yaml, <id>.installer.yaml and <id>.locale.en-US.yaml:
//     a winget multi-file manifest (schema 1.9.0) for the release's
//     Windows NSIS installers.
//
// Like flatpak/<id>.yml, each scans the releases directory for this
// version's installers so machines building separate architectures join
// into one manifest.

const (
	wingetSchemaVersion = "1.9.0"
	wingetDefaultLocale = "en-US"
	defaultManifestDesc = "Desktop application"
)

// writePackageManifests writes Homebrew cask and winget manifests for a
// direct release's macOS and Windows installers.
func (p *project) writePackageManifests(ctx context.Context, targets []string) error {
	if p.Build.embed || !p.updates() || !p.installers() {
		return nil
	}
	hasMac := slices.ContainsFunc(targets, func(t string) bool { return strings.HasPrefix(t, "macos_") })
	hasWin := slices.ContainsFunc(targets, func(t string) bool { return strings.HasPrefix(t, "windows_") })
	if !hasMac && !hasWin {
		return nil
	}
	base := p.releaseFilesURLContext(ctx)
	if base == "" {
		warn("no package manager manifests: set release.github or updates.feed to download installers from the release")
		return nil
	}
	if hasMac {
		if err := p.writeHomebrewCask(ctx, targets, base); err != nil {
			return err
		}
	}
	if hasWin {
		if err := p.writeWingetManifest(ctx, base); err != nil {
			return err
		}
	}
	return nil
}

// releaseFilesURLContext is where this version's release files are
// downloaded from: updates.feed, else its GitHub release; "" without
// either.
func (p *project) releaseFilesURLContext(ctx context.Context) string {
	if p.Updates.Feed != "" {
		return strings.TrimRight(p.Updates.Feed, "/") + "/"
	}
	if repo := p.releaseGitHubRepo(ctx); repo != "" {
		return "https://github.com/" + repo + "/releases/download/" + url.PathEscape(p.tagPrefix()+p.Version) + "/"
	}
	return ""
}

// releaseGitHubRepo is the GitHub repository ("owner/name") of this
// project's releases, reusing what validateUpdates resolved when
// release.github was not set explicitly.
func (p *project) releaseGitHubRepo(ctx context.Context) string {
	if p.Releases.GitHub != "" {
		return p.Releases.GitHub
	}
	if p.Updates.repo != "" {
		return p.Updates.repo
	}
	return p.githubRepository(ctx)
}

// manifestHomepage is the project's homepage URL for package manager
// manifests: its GitHub repository, else updates.feed (ensuring a slash
// after the domain as Homebrew's style audit requires).
func (p *project) manifestHomepage(ctx context.Context) string {
	if repo := p.releaseGitHubRepo(ctx); repo != "" {
		return "https://github.com/" + repo
	}
	if p.Updates.Feed != "" {
		u, err := url.Parse(p.Updates.Feed)
		if err == nil && u.Scheme != "" && u.Host != "" {
			if u.Path == "" {
				u.Path = "/"
			}
			return u.String()
		}
	}
	return ""
}

// manifestReleasePage is this version's GitHub release tag page, or ""
// when releases are hosted on a custom feed or without a GitHub repo.
func (p *project) manifestReleasePage(ctx context.Context) string {
	if p.Updates.Feed != "" {
		return ""
	}
	repo := p.releaseGitHubRepo(ctx)
	if repo == "" {
		return ""
	}
	return "https://github.com/" + repo + "/releases/tag/" + url.PathEscape(p.tagPrefix()+p.Version)
}

// releaseFileSHA256 returns the lowercase hex SHA-256 of path, or ok=false
// when path does not exist.
func releaseFileSHA256(path string) (sum string, ok bool, err error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:]), true, nil
}

// homebrewCask describes the rendered Homebrew cask file.
type homebrewCask struct {
	Token    string
	Version  string
	ArmSHA   string
	IntelSHA string
	Single   string // non-empty for universal or single-arch casks
	ArchDep  string // "", "arm64", or "x86_64" for single-arch casks
	URL      string // pre-formatted Ruby double-quoted string literal
	Name     string
	Desc     string
	Homepage string
	App      string
	ZapTrash []string
}

// writeHomebrewCask writes homebrew/<slug>.rb in the releases directory
// for the macOS disk images built for this version.
func (p *project) writeHomebrewCask(ctx context.Context, targets []string, base string) error {
	uniSHA, hasUni, err := releaseFileSHA256(filepath.Join(p.releaseDir(), dmgName(p, "universal")))
	if err != nil {
		return err
	}
	armSHA, hasArm, err := releaseFileSHA256(filepath.Join(p.releaseDir(), dmgName(p, "arm64")))
	if err != nil {
		return err
	}
	x64SHA, hasX64, err := releaseFileSHA256(filepath.Join(p.releaseDir(), dmgName(p, "x64")))
	if err != nil {
		return err
	}
	if !hasUni && !hasArm && !hasX64 {
		return nil
	}
	name, err := p.displayName()
	if err != nil {
		return err
	}
	token := p.packageSlug()
	cask := homebrewCask{
		Token:    token,
		Version:  p.Version,
		Name:     name,
		Desc:     homebrewDesc(p.Linux.Summary, token, name),
		Homepage: p.manifestHomepage(ctx),
		App:      p.Name + ".app",
		ZapTrash: homebrewZapTrash(p.ID),
	}
	preferUni := hasUni && (slices.Contains(targets, "macos_universal") || (!hasArm && !hasX64))
	switch {
	case preferUni:
		cask.Single = uniSHA
		cask.URL = p.homebrewURL(ctx, base, "universal", false)
	case hasArm && hasX64:
		cask.ArmSHA = armSHA
		cask.IntelSHA = x64SHA
		cask.URL = p.homebrewURL(ctx, base, "", true)
	case hasArm:
		cask.Single = armSHA
		cask.ArchDep = "arm64"
		cask.URL = p.homebrewURL(ctx, base, "arm64", false)
	default:
		cask.Single = x64SHA
		cask.ArchDep = "x86_64"
		cask.URL = p.homebrewURL(ctx, base, "x64", false)
	}
	dir := filepath.Join(p.releaseDir(), "homebrew")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, token+".rb")
	if err := os.WriteFile(path, renderHomebrewCask(cask), 0o644); err != nil {
		return err
	}
	detail("→ %s", rel(path))
	return nil
}

// homebrewURL formats the cask's url stanza string literal, using Ruby's
// #{version} interpolation when Version needs no URL percent-encoding and
// #{arch} when both macOS architectures are present.
func (p *project) homebrewURL(ctx context.Context, base, arch string, dual bool) string {
	useVerInterp := url.PathEscape(p.Version) == p.Version
	verPart := url.PathEscape(p.Version)
	if useVerInterp {
		verPart = "#{version}"
	}
	archPart := arch
	if dual {
		archPart = "#{arch}"
	}
	var prefix string
	if p.Updates.Feed != "" {
		prefix = rubyEscape(base)
	} else {
		repo := p.releaseGitHubRepo(ctx)
		tagPrefix := url.PathEscape(p.tagPrefix())
		prefix = rubyEscape("https://github.com/"+repo+"/releases/download/"+tagPrefix) + verPart + "/"
	}
	slug := rubyEscape(url.PathEscape(p.packageSlug()))
	return `"` + prefix + slug + "-" + verPart + "-macos-" + archPart + `.dmg"`
}

// renderHomebrewCask formats cask following Homebrew's cask stanza order,
// stanza grouping, alignment and RuboCop style rules.
func renderHomebrewCask(c homebrewCask) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "cask %s do\n", rubyString(c.Token))
	if c.ArmSHA != "" && c.IntelSHA != "" {
		b.WriteString("  arch arm: \"arm64\", intel: \"x64\"\n\n")
		fmt.Fprintf(&b, "  version %s\n", rubyString(c.Version))
		fmt.Fprintf(&b, "  sha256 arm:   %s,\n", rubyString(c.ArmSHA))
		fmt.Fprintf(&b, "         intel: %s\n\n", rubyString(c.IntelSHA))
	} else {
		fmt.Fprintf(&b, "  version %s\n", rubyString(c.Version))
		fmt.Fprintf(&b, "  sha256 %s\n\n", rubyString(c.Single))
	}
	fmt.Fprintf(&b, "  url %s\n", c.URL)
	fmt.Fprintf(&b, "  name %s\n", rubyString(c.Name))
	fmt.Fprintf(&b, "  desc %s\n", rubyString(c.Desc))
	fmt.Fprintf(&b, "  homepage %s\n\n", rubyString(c.Homepage))
	b.WriteString("  auto_updates true\n")
	if c.ArchDep != "" {
		fmt.Fprintf(&b, "  depends_on arch: :%s\n", c.ArchDep)
	}
	b.WriteString("  depends_on :macos\n\n")
	fmt.Fprintf(&b, "  app %s\n", rubyString(c.App))
	if len(c.ZapTrash) > 0 {
		b.WriteString("\n  zap trash: [\n")
		for _, path := range c.ZapTrash {
			fmt.Fprintf(&b, "    %s,\n", rubyString(path))
		}
		b.WriteString("  ]\n")
	}
	b.WriteString("end\n")
	return b.Bytes()
}

// homebrewZapTrash returns the macOS data, cache, log and preference paths
// an NSGo app writes for id, sorted case-insensitively as Homebrew's
// Cask/ArrayAlphabetization cop requires.
func homebrewZapTrash(id string) []string {
	if id == "" {
		return nil
	}
	paths := []string{
		"~/Library/Application Support/" + id,
		"~/Library/Caches/" + id,
		"~/Library/Logs/" + id,
		"~/Library/Preferences/" + id + ".plist",
	}
	slices.SortFunc(paths, func(a, b string) int {
		return strings.Compare(strings.ToLower(a), strings.ToLower(b))
	})
	return paths
}

var (
	brewCommandLineRE = regexp.MustCompile(`(?i)\bcommand ?line\b`)
	brewArticleRE     = regexp.MustCompile(`(?i)^(?:the|an?)\s+`)
	brewPlatformRE    = regexp.MustCompile(`(?i)\b(?:macos|mac(?: ?os(?: ?x)?)?|os ?x)\b`)
)

// homebrewDesc cleans summary to satisfy Homebrew's Cask/Desc rules:
// non-empty, <= 80 characters, no leading article, starts with a capital
// letter, does not start with the cask token/name, does not mention macOS,
// does not end with a period (except "etc.") and contains no Unicode
// symbols. Falls back to defaultManifestDesc when summary is empty or
// cannot satisfy those constraints.
func homebrewDesc(summary, token, appName string) string {
	s := strings.Map(func(r rune) rune {
		if unicode.Is(unicode.So, r) {
			return -1
		}
		return r
	}, summary)
	s = singleLine(s)
	s = brewCommandLineRE.ReplaceAllStringFunc(s, func(m string) string {
		if len(m) > 0 && m[0] == 'C' {
			return "Command-line"
		}
		return "command-line"
	})
	for brewArticleRE.MatchString(s) {
		s = brewArticleRE.ReplaceAllString(s, "")
	}
	if strings.HasSuffix(s, ".") && !strings.HasSuffix(s, "etc.") {
		s = strings.TrimRight(s, ".")
		s = strings.TrimSpace(s)
	}
	if s == "" {
		return defaultManifestDesc
	}
	// Capitalize first letter unless it is one of Homebrew's valid lowercase starts.
	firstWord, _, _ := strings.Cut(s, " ")
	if firstWord != "iOS" && firstWord != "iPhone" {
		r, size := utf8.DecodeRuneInString(s)
		if r != utf8.RuneError && unicode.IsLower(r) {
			s = string(unicode.ToUpper(r)) + s[size:]
		}
	}
	if startsWithCaskName(s, token) || (appName != "" && startsWithCaskName(s, appName)) {
		return defaultManifestDesc
	}
	for _, m := range brewPlatformRE.FindAllString(s, -1) {
		if m != "MAC" {
			return defaultManifestDesc
		}
	}
	if utf8.RuneCountInString(s) > 80 {
		s = truncateRunes(s, 80)
		s = strings.TrimRight(s, " .,;:-")
		if s == "" {
			return defaultManifestDesc
		}
	}
	return s
}

// startsWithCaskName reports whether desc starts with name (ignoring
// hyphens and spaces, case-insensitively) at a word boundary, matching
// Homebrew's DescHelper check.
func startsWithCaskName(desc, name string) bool {
	compact := strings.Map(func(r rune) rune {
		if r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToLower(r)
	}, name)
	if compact == "" {
		return false
	}
	// Walk desc matching each rune of compact with optional spaces or hyphens between them.
	d := strings.ToLower(desc)
	di := 0
	for ci, want := range compact {
		if ci > 0 && di < len(d) {
			r, sz := utf8.DecodeRuneInString(d[di:])
			if r == '-' || unicode.IsSpace(r) {
				di += sz
			}
		}
		if di >= len(d) {
			return false
		}
		got, sz := utf8.DecodeRuneInString(d[di:])
		if got != want {
			return false
		}
		di += sz
	}
	if di == len(d) {
		return true
	}
	next, _ := utf8.DecodeRuneInString(d[di:])
	return !(unicode.IsLetter(next) || unicode.IsDigit(next) || next == '_')
}

// rubyEscape escapes s for inclusion inside a Ruby double-quoted string
// literal, preventing backslash, quote and interpolation injection.
func rubyEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			b.WriteString(`\\`)
		case '"':
			b.WriteString(`\"`)
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '#':
			if i+1 < len(s) && (s[i+1] == '{' || s[i+1] == '@' || s[i+1] == '$') {
				b.WriteString(`\#`)
			} else {
				b.WriteByte('#')
			}
		default:
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

// rubyString returns s as a Ruby double-quoted string literal.
func rubyString(s string) string {
	return `"` + rubyEscape(s) + `"`
}

// Winget multi-file manifest structures (winget-pkgs schema v1.9.0).
// Field order matches wingetcreate / winget-pkgs canonical order and is
// deterministic across runs.

type wingetVersionManifest struct {
	PackageIdentifier string `yaml:"PackageIdentifier"`
	PackageVersion    string `yaml:"PackageVersion"`
	DefaultLocale     string `yaml:"DefaultLocale"`
	ManifestType      string `yaml:"ManifestType"`
	ManifestVersion   string `yaml:"ManifestVersion"`
}

type wingetInstallerSwitches struct {
	Silent             string `yaml:"Silent,omitempty"`
	SilentWithProgress string `yaml:"SilentWithProgress,omitempty"`
}

type wingetAppsAndFeaturesEntry struct {
	DisplayName    string `yaml:"DisplayName,omitempty"`
	Publisher      string `yaml:"Publisher,omitempty"`
	DisplayVersion string `yaml:"DisplayVersion,omitempty"`
	InstallerType  string `yaml:"InstallerType,omitempty"`
}

type wingetInstallerEntry struct {
	Architecture    string `yaml:"Architecture"`
	InstallerUrl    string `yaml:"InstallerUrl"`
	InstallerSha256 string `yaml:"InstallerSha256"`
}

type wingetInstallerManifest struct {
	PackageIdentifier      string                       `yaml:"PackageIdentifier"`
	PackageVersion         string                       `yaml:"PackageVersion"`
	InstallerLocale        string                       `yaml:"InstallerLocale,omitempty"`
	InstallerType          string                       `yaml:"InstallerType"`
	Scope                  string                       `yaml:"Scope,omitempty"`
	UpgradeBehavior        string                       `yaml:"UpgradeBehavior,omitempty"`
	InstallerSwitches      *wingetInstallerSwitches     `yaml:"InstallerSwitches,omitempty"`
	AppsAndFeaturesEntries []wingetAppsAndFeaturesEntry `yaml:"AppsAndFeaturesEntries,omitempty"`
	Installers             []wingetInstallerEntry       `yaml:"Installers"`
	ManifestType           string                       `yaml:"ManifestType"`
	ManifestVersion        string                       `yaml:"ManifestVersion"`
}

type wingetDefaultLocaleManifest struct {
	PackageIdentifier string `yaml:"PackageIdentifier"`
	PackageVersion    string `yaml:"PackageVersion"`
	PackageLocale     string `yaml:"PackageLocale"`
	Publisher         string `yaml:"Publisher"`
	PublisherUrl      string `yaml:"PublisherUrl,omitempty"`
	PackageName       string `yaml:"PackageName"`
	PackageUrl        string `yaml:"PackageUrl,omitempty"`
	License           string `yaml:"License"`
	Copyright         string `yaml:"Copyright,omitempty"`
	ShortDescription  string `yaml:"ShortDescription"`
	ReleaseNotesUrl   string `yaml:"ReleaseNotesUrl,omitempty"`
	ManifestType      string `yaml:"ManifestType"`
	ManifestVersion   string `yaml:"ManifestVersion"`
}

// writeWingetManifest writes the version, installer and defaultLocale
// manifests under winget/ in the releases directory for the Windows NSIS
// installers built for this version.
func (p *project) writeWingetManifest(ctx context.Context, base string) error {
	var installers []wingetInstallerEntry
	for _, arch := range []string{"x64", "arm64"} {
		file := nsisSetupName(p, arch)
		sum, ok, err := releaseFileSHA256(filepath.Join(p.releaseDir(), file))
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		installers = append(installers, wingetInstallerEntry{
			Architecture:    arch,
			InstallerUrl:    base + url.PathEscape(file),
			InstallerSha256: strings.ToUpper(sum),
		})
	}
	if len(installers) == 0 {
		return nil
	}
	name, err := p.displayName()
	if err != nil {
		return err
	}
	pkgID := p.wingetPackageID(ctx)
	pkgName := wingetText(name, pkgID, 2, 256)
	publisher := wingetText(p.wingetPublisher(ctx, name), pkgID, 2, 256)
	homepage := p.manifestHomepage(ctx)
	license := wingetText(p.Linux.License, "Proprietary", 3, 512)
	shortDesc := wingetText(p.Linux.Summary, defaultManifestDesc, 3, 256)
	copyright := ""
	if c := singleLine(p.Copyright); utf8.RuneCountInString(c) >= 3 {
		copyright = truncateRunes(c, 512)
	}

	versionManifest := wingetVersionManifest{
		PackageIdentifier: pkgID,
		PackageVersion:    p.Version,
		DefaultLocale:     wingetDefaultLocale,
		ManifestType:      "version",
		ManifestVersion:   wingetSchemaVersion,
	}
	installerManifest := wingetInstallerManifest{
		PackageIdentifier: pkgID,
		PackageVersion:    p.Version,
		InstallerLocale:   wingetDefaultLocale,
		InstallerType:     "nullsoft",
		Scope:             "user",
		UpgradeBehavior:   "install",
		InstallerSwitches: &wingetInstallerSwitches{
			Silent:             "/S",
			SilentWithProgress: "/S",
		},
		AppsAndFeaturesEntries: []wingetAppsAndFeaturesEntry{{
			DisplayName:    truncateRunes(singleLine(name), 256),
			Publisher:      truncateRunes(singleLine(p.windowsPublisher()), 256),
			DisplayVersion: truncateRunes(p.Version, 128),
			InstallerType:  "nullsoft",
		}},
		Installers:      installers,
		ManifestType:    "installer",
		ManifestVersion: wingetSchemaVersion,
	}
	localeManifest := wingetDefaultLocaleManifest{
		PackageIdentifier: pkgID,
		PackageVersion:    p.Version,
		PackageLocale:     wingetDefaultLocale,
		Publisher:         publisher,
		PublisherUrl:      homepage,
		PackageName:       pkgName,
		PackageUrl:        homepage,
		License:           license,
		Copyright:         copyright,
		ShortDescription:  shortDesc,
		ReleaseNotesUrl:   p.manifestReleasePage(ctx),
		ManifestType:      "defaultLocale",
		ManifestVersion:   wingetSchemaVersion,
	}

	dir := filepath.Join(p.releaseDir(), "winget")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	files := []struct {
		name   string
		schema string
		data   any
	}{
		{pkgID + ".yaml", "version", versionManifest},
		{pkgID + ".installer.yaml", "installer", installerManifest},
		{pkgID + ".locale." + wingetDefaultLocale + ".yaml", "defaultLocale", localeManifest},
	}
	for _, f := range files {
		out, err := marshalWingetYAML(f.schema, f.data)
		if err != nil {
			return err
		}
		path := filepath.Join(dir, f.name)
		if err := os.WriteFile(path, out, 0o644); err != nil {
			return err
		}
	}
	detail("→ %s", rel(filepath.Join(dir, pkgID+".yaml")))
	return nil
}

// marshalWingetYAML encodes a winget manifest struct as deterministic YAML
// headed by its schema language-server comment.
func marshalWingetYAML(schema string, v any) ([]byte, error) {
	var b bytes.Buffer
	fmt.Fprintf(&b, "# yaml-language-server: $schema=https://aka.ms/winget-manifest.%s.%s.schema.json\n\n", schema, wingetSchemaVersion)
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// wingetPackageID returns a schema-valid winget PackageIdentifier: 2 to 8
// dot-separated segments of 1..32 valid characters, up to 128 characters
// total. A dot-separated p.ID (such as dev.example.hello) is preserved
// verbatim when it already satisfies the schema.
func (p *project) wingetPackageID(ctx context.Context) string {
	var segs []string
	for part := range strings.SplitSeq(p.ID, ".") {
		if s := sanitizeWingetIDSegment(part); s != "" {
			segs = append(segs, s)
		}
	}
	if len(segs) < 2 {
		pub := sanitizeWingetIDSegment(p.wingetPublisher(ctx, ""))
		if pub == "" {
			pub = "NSGo"
		}
		pkg := ""
		if len(segs) == 1 {
			pkg = segs[0]
		}
		if pkg == "" {
			pkg = sanitizeWingetIDSegment(p.Name)
		}
		if pkg == "" {
			pkg = sanitizeWingetIDSegment(p.packageSlug())
		}
		if pkg == "" {
			pkg = "App"
		}
		segs = []string{pub, pkg}
	}
	if len(segs) > 8 {
		segs = segs[:8]
	}
	for len(segs) > 2 && utf8.RuneCountInString(strings.Join(segs, ".")) > 128 {
		segs = segs[:len(segs)-1]
	}
	return strings.Join(segs, ".")
}

// sanitizeWingetIDSegment strips characters the winget PackageIdentifier
// schema forbids and caps the segment to 32 runes.
func sanitizeWingetIDSegment(s string) string {
	cleaned := strings.Map(func(r rune) rune {
		if r <= 0x1f || r == 0x7f || unicode.IsSpace(r) {
			return -1
		}
		switch r {
		case '.', '\\', '/', ':', '*', '?', '"', '<', '>', '|':
			return -1
		}
		return r
	}, s)
	return truncateRunes(cleaned, 32)
}

// wingetPublisher returns the publisher name for the default locale
// manifest: windows.publisher, else windows.store.publisher_name, else
// linux.maintainer's name, else the GitHub repository owner, else name.
func (p *project) wingetPublisher(ctx context.Context, name string) string {
	if pub := singleLine(p.windowsPublisher()); pub != "" {
		return pub
	}
	if m := p.Linux.Maintainer; m != "" {
		if before, _, ok := strings.Cut(m, "<"); ok {
			m = before
		}
		if pub := singleLine(m); pub != "" {
			return pub
		}
	}
	if repo := p.releaseGitHubRepo(ctx); repo != "" {
		if owner, _, ok := strings.Cut(repo, "/"); ok && owner != "" {
			return owner
		}
	}
	return singleLine(name)
}

// wingetText normalizes s to one line within [minLen, maxLen] runes,
// falling back to fallback when s is shorter than minLen.
func wingetText(s, fallback string, minLen, maxLen int) string {
	v := singleLine(s)
	if utf8.RuneCountInString(v) < minLen {
		v = singleLine(fallback)
	}
	return truncateRunes(v, maxLen)
}

// singleLine collapses whitespace runs and line breaks into single spaces.
func singleLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// truncateRunes returns at most maxLen runes of s.
func truncateRunes(s string, maxLen int) string {
	if maxLen <= 0 || utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	count := 0
	for i := range s {
		if count == maxLen {
			return s[:i]
		}
		count++
	}
	return s
}
