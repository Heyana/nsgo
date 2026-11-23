package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"
	"time"
)

// A Linux store build writes an installable .flatpak bundle in Go, plus the source archive and flatpak/<id>.yml for Flathub or flatpak-builder. Both use GNOME's runtime, which supplies GTK 4 and WebKitGTK.

// flatpakRuntime is the GNOME runtime's version the manifest names,
// unless linux.flatpak_runtime names another.
const flatpakRuntime = "51"

// flatpakArch is Flatpak's name of each architecture of the targets.
var flatpakArch = map[string]string{"x64": "x86_64", "arm64": "aarch64"}

// buildLinuxArchive writes the store's archive of the Linux package in
// dir, built for arch, Go's, beside it, with the AppStream metadata
// added, and returns its path.
func buildLinuxArchive(ctx context.Context, p *project, dir, arch string) (string, error) {
	stage(ctx, "writing the Flatpak's archive")
	if len(p.linuxCategories()) == 0 {
		// AppStream requires a category. Apps can replace this store-package fallback with linux.categories; ordinary desktop entries stay as set.
		entry := filepath.Join(dir, p.ID+".desktop")
		data, err := os.ReadFile(entry)
		if err != nil {
			return "", err
		}
		if !strings.Contains(string(data), "\nCategories=") {
			data = append(data, []byte("Categories=Utility;\n")...)
		}
		if err := os.WriteFile(entry, data, 0o644); err != nil {
			return "", err
		}
	}
	// AppStream composition requires the desktop entry's icon to exist even when the app has no artwork. Match MSIX's transparent package fallback.
	icon := filepath.Join(dir, p.ID+".png")
	if _, err := os.Stat(icon); os.IsNotExist(err) {
		if err := os.WriteFile(icon, scaledPNG(image.NewNRGBA(image.Rect(0, 0, 1, 1)), 512), 0o644); err != nil {
			return "", err
		}
	} else if err != nil {
		return "", err
	}
	metainfo, err := p.metainfo(false)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, p.ID+".metainfo.xml"), metainfo, 0o644); err != nil {
		return "", err
	}
	entries, err := linuxDirEntries(p, dir)
	if err != nil {
		return "", err
	}
	data, err := tarGz(entries, p.packageSlug()+"/")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, p.releaseName("linux_"+goArchName(arch), ".tar.gz"))
	return path, os.WriteFile(path, data, 0o644)
}

// metainfo is the app's AppStream metadata, which software centers and
// Flathub list it by. A Flatpak's names a proprietary license unless
// linux.license gives one, and an empty content rating, as Flathub asks;
// the Linux packages' (direct) name only a license linux.license gives,
// and Utility as the category, which AppStream needs, when
// linux.categories names none, as a Flatpak's desktop entry does.
func (p *project) metainfo(direct bool) ([]byte, error) {
	name, err := p.displayName()
	if err != nil {
		return nil, err
	}
	n, err := p.appNames()
	if err != nil {
		return nil, err
	}
	var names [][2]string
	if n != nil {
		for _, lang := range n.languages()[1:] {
			names = append(names, [2]string{desktopLocale(lang), n.names[lang]})
		}
	}
	summary := p.Linux.Summary
	if summary == "" {
		summary = name
	}
	license := p.Linux.License
	if license == "" && !direct {
		license = "LicenseRef-proprietary"
	}
	var categories []string
	if direct && len(p.linuxCategories()) == 0 {
		categories = []string{"Utility"}
	}
	developer := p.ID
	if i := strings.LastIndexByte(developer, '.'); i > 0 {
		developer = developer[:i]
	}
	homepage := ""
	if repo := p.githubRepository(context.Background()); repo != "" {
		homepage = "https://github.com/" + repo
	}
	var out bytes.Buffer
	err = metainfoTemplate.Execute(&out, map[string]any{
		"ID": p.ID, "Name": name, "Names": names, "Summary": summary, "License": license,
		"Developer": developer, "DeveloperName": p.maintainer(), "Homepage": homepage,
		"Version": p.Version, "Date": time.Now().Format(time.DateOnly),
		"Categories": categories, "Rating": !direct,
	})
	return out.Bytes(), err
}

var metainfoTemplate = template.Must(template.New("metainfo").Funcs(template.FuncMap{"x": xmlText}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<component type="desktop-application">
  <id>{{x .ID}}</id>
  <name>{{x .Name}}</name>
{{- range .Names}}
  <name xml:lang="{{x (index . 0)}}">{{x (index . 1)}}</name>
{{- end}}
  <summary>{{x .Summary}}</summary>
  <metadata_license>CC0-1.0</metadata_license>
{{- if .License}}
  <project_license>{{x .License}}</project_license>
{{- end}}
  <developer id="{{x .Developer}}">
    <name>{{x .DeveloperName}}</name>
  </developer>
  <description>
    <p>{{x .Summary}}</p>
  </description>
  <launchable type="desktop-id">{{x .ID}}.desktop</launchable>
{{- if .Categories}}
  <categories>
{{- range .Categories}}
    <category>{{x .}}</category>
{{- end}}
  </categories>
{{- end}}
{{- if .Homepage}}
  <url type="homepage">{{x .Homepage}}</url>
{{- end}}
{{- if .Rating}}
  <content_rating type="oars-1.1"/>
{{- end}}
  <releases>
    <release version="{{x .Version}}" date="{{.Date}}"/>
  </releases>
</component>
`))

// writeFlatpakManifest writes flatpak/<id>.yml into the releases
// directory for a store build of Linux targets: the manifest installing
// the archives there, each by its download in the release, else by its
// path beside the manifest, for flatpak-builder to build here.
func (p *project) writeFlatpakManifest(targets []string) error {
	if p.channel() != channelStore || !p.installers() || !slices.ContainsFunc(targets, func(t string) bool { return strings.HasPrefix(t, "linux_") }) {
		return nil
	}
	base := p.releaseFilesURL()
	type source struct{ URL, Path, SHA256, Arch string }
	var sources []source
	// Archives other machines built join this build's.
	for _, arch := range []string{"x64", "arm64"} {
		file := p.releaseName("linux_"+arch, ".tar.gz")
		data, err := os.ReadFile(filepath.Join(p.releaseDir(), file))
		if err != nil {
			continue
		}
		sum := sha256.Sum256(data)
		s := source{SHA256: hex.EncodeToString(sum[:]), Arch: flatpakArch[arch]}
		if base != "" {
			s.URL = base + url.PathEscape(file)
		} else {
			s.Path = "../" + file
		}
		sources = append(sources, s)
	}
	if len(sources) == 0 {
		return nil
	}
	runtime := p.Linux.FlatpakRuntime
	if runtime == "" {
		runtime = flatpakRuntime
	}
	var out bytes.Buffer
	err := flatpakManifest.Execute(&out, map[string]any{
		"ID": p.ID, "Slug": p.packageSlug(), "Command": p.Name, "Runtime": runtime, "Sources": sources,
	})
	if err != nil {
		return err
	}
	dir := filepath.Join(p.releaseDir(), "flatpak")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, p.ID+".yml")
	if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil {
		return err
	}
	detail("→ %s", rel(path))
	if base == "" {
		warn("%s builds from the archives beside it: set release.github or updates.feed to download them from the release", rel(path))
	}
	return nil
}

// releaseFilesURL is where this version's release files are downloaded
// from: updates.feed, else its GitHub release; "" without either.
func (p *project) releaseFilesURL() string {
	return p.releaseFilesURLContext(context.Background())
}

// flatpakManifest installs the app in /app/lib/<slug>, its command, desktop
// entry, icon and metadata where Flatpak exports them, with the
// permissions a desktop app on Wayland or X11 needs: its display, the GPU,
// the network and sound.
var flatpakManifest = template.Must(template.New("flatpak").Funcs(template.FuncMap{"q": yamlQuote}).Parse(`id: {{.ID}}
runtime: org.gnome.Platform
runtime-version: {{q .Runtime}}
sdk: org.gnome.Sdk
command: {{q .Command}}
finish-args:
  - --share=ipc
  - --socket=wayland
  - --socket=fallback-x11
  - --device=dri
  - --share=network
  - --socket=pulseaudio
modules:
  - name: {{.Slug}}
    buildsystem: simple
    build-commands:
      - mkdir -p /app/lib/{{.Slug}} /app/bin
      - cp -a . /app/lib/{{.Slug}}/
      - ln -s {{q (printf "../lib/%s/%s" .Slug .Command)}} {{q (printf "/app/bin/%s" .Command)}}
      - install -Dm644 {{.ID}}.desktop /app/share/applications/{{.ID}}.desktop
      - if [ -f {{.ID}}.png ]; then install -Dm644 {{.ID}}.png /app/share/icons/hicolor/512x512/apps/{{.ID}}.png; fi
      - install -Dm644 {{.ID}}.metainfo.xml /app/share/metainfo/{{.ID}}.metainfo.xml
      - if [ -f {{.ID}}.mime.xml ]; then install -Dm644 {{.ID}}.mime.xml /app/share/mime/packages/{{.ID}}.xml; update-mime-database /app/share/mime; fi
    sources:
{{- range .Sources}}
      - type: archive
{{- if .URL}}
        url: {{q .URL}}
{{- else}}
        path: {{q .Path}}
{{- end}}
        sha256: {{.SHA256}}
        only-arches: [{{.Arch}}]
{{- end}}
`))

// yamlQuote is s as a YAML string in double quotes.
func yamlQuote(s string) string {
	return fmt.Sprintf("%q", s)
}
