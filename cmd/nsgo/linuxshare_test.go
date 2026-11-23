package main

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The Debian and Arch packages install NSGo's AppStream metadata and linux.share's files under /usr/share, where software centers and desktops read them, rather than in the app's directory, and the AppImage holds them in its AppDir's usr/share; the app's own metadata replaces NSGo's.
func TestLinuxShare(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"App": "binary", "dev.example.app.png": "icon", "dev.example.app.desktop": "entry"})
	os.Chmod(filepath.Join(dir, "App"), 0o755)
	share := t.TempDir()
	writeFiles(t, share, map[string]string{
		"metainfo/dev.example.app.metainfo.xml":    "<component>own</component>",
		"applications/dev.example.app-cli.desktop": "entry",
		"bash-completion/completions/app":          "complete",
	})
	p := &project{Dir: dir, Name: "App", ID: "dev.example.app", Version: "1.0.0"}

	// Without linux.share, NSGo's own metadata.
	files, err := linuxPackageFiles(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	deb, err := debPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if got := tarFiles(t, arMembers(t, deb)["data.tar.gz"])["./usr/share/metainfo/dev.example.app.metainfo.xml"]; !strings.Contains(got, "<id>dev.example.app</id>") ||
		!strings.Contains(got, "<launchable type=\"desktop-id\">dev.example.app.desktop</launchable>") {
		t.Errorf("the .deb's metadata is %q", got)
	}
	entries, err := appDirEntries(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(entries, func(e linuxPackageEntry) bool { return e.name == "usr/share/metainfo/dev.example.app.metainfo.xml" }) {
		t.Error("the AppImage lacks the metadata")
	}
	// The metadata's launchable is the desktop entry in usr/share/applications, which links to the one at the AppDir's top.
	if i := slices.IndexFunc(entries, func(e linuxPackageEntry) bool { return e.name == "usr/share/applications/dev.example.app.desktop" }); i < 0 || entries[i].link != "../../../dev.example.app.desktop" {
		t.Errorf("the AppImage's usr/share/applications/dev.example.app.desktop is %+v", entries)
	} else if resolved := path.Join(path.Dir(entries[i].name), entries[i].link); resolved != "dev.example.app.desktop" ||
		!slices.ContainsFunc(entries, func(e linuxPackageEntry) bool { return e.name == resolved && e.link == "" && !e.dir }) {
		t.Errorf("the desktop entry's link resolves to %s, which the AppDir does not hold", resolved)
	}
	if _, err := sqfsTree(entries); err != nil {
		t.Fatal(err)
	}

	p.Linux.Share = share
	files, err = linuxPackageFiles(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	if deb, err = debPackage(p, files, "amd64"); err != nil {
		t.Fatal(err)
	}
	pkg, err := archPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	for format, members := range map[string]map[string]string{
		"deb":  tarFiles(t, arMembers(t, deb)["data.tar.gz"]),
		"arch": tarFiles(t, pkg),
	} {
		prefix := ""
		if format == "deb" {
			prefix = "./"
		}
		for name, want := range map[string]string{
			"usr/share/metainfo/dev.example.app.metainfo.xml":    "<component>own</component>",
			"usr/share/applications/dev.example.app-cli.desktop": "entry",
			"usr/share/bash-completion/completions/app":          "complete",
		} {
			if got := members[prefix+name]; got != want {
				t.Errorf("the %s package's %s%s is %q, want %q", format, prefix, name, got, want)
			}
		}
		if _, ok := members[prefix+"usr/share/applications/dev.example.app.desktop"]; !ok {
			t.Errorf("the %s package lacks NSGo's desktop entry", format)
		}
		for name := range members {
			if strings.HasPrefix(name, prefix+"opt/app/usr/") {
				t.Errorf("the %s package installs %s in the app's directory", format, name)
			}
		}
	}
	entries, err = appDirEntries(p, dir)
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"usr/share/metainfo/dev.example.app.metainfo.xml": "<component>own</component>",
		"usr/share/bash-completion/completions/app":       "complete",
	} {
		i := slices.IndexFunc(entries, func(e linuxPackageEntry) bool { return e.name == name })
		if i < 0 || string(entries[i].data) != want {
			t.Errorf("the AppImage's %s is missing or not %q", name, want)
		}
	}
	if n := len(slices.DeleteFunc(slices.Clone(entries), func(e linuxPackageEntry) bool { return e.name != "usr/share/metainfo/dev.example.app.metainfo.xml" })); n != 1 {
		t.Errorf("the AppImage holds %d metadata files", n)
	}

	if _, err := sqfsTree(entries); err != nil {
		t.Fatal(err)
	}

	// Metadata among the resources is the app's own in the AppImage, where resources/usr/share is the AppDir's.
	p.Linux.Share = ""
	writeFiles(t, dir, map[string]string{"usr/share/metainfo/dev.example.app.metainfo.xml": "<component>resource</component>"})
	if entries, err = appDirEntries(p, dir); err != nil {
		t.Fatal(err)
	}
	if n := len(slices.DeleteFunc(entries, func(e linuxPackageEntry) bool { return e.name != "usr/share/metainfo/dev.example.app.metainfo.xml" })); n != 1 {
		t.Errorf("the AppImage holds %d metadata files beside the resources' own", n)
	}
	os.RemoveAll(filepath.Join(dir, "usr"))
	p.Linux.Share = share

	// A file NSGo writes itself is refused, as is a linux.share that is no directory.
	writeFiles(t, share, map[string]string{"applications/dev.example.app.desktop": "mine"})
	if _, err := linuxPackageFiles(p, dir); err == nil || !strings.Contains(err.Error(), "usr/share/applications/dev.example.app.desktop, from linux.share or NSGo, collides") {
		t.Errorf("a desktop entry of the app's own: %v", err)
	}
	p.Linux.Share = filepath.Join(share, "bash-completion", "completions", "app")
	if _, err := linuxPackageFiles(p, dir); err == nil || !strings.Contains(err.Error(), "is not a directory") {
		t.Errorf("a file as linux.share: %v", err)
	}

	// A file where the package needs a directory is refused too.
	clash := t.TempDir()
	p.Linux.Share = clash
	for _, name := range []string{"applications", "metainfo"} {
		os.RemoveAll(clash)
		writeFiles(t, clash, map[string]string{name: "not a directory"})
		if _, err := linuxPackageFiles(p, dir); err == nil || !strings.Contains(err.Error(), "usr/share/"+name+", from linux.share or NSGo, collides") {
			t.Errorf("a file named %s: %v", name, err)
		}
	}

	// The AppImage's link to its desktop entry is refused beside a resource at its path.
	p.Linux.Share = ""
	writeFiles(t, dir, map[string]string{"usr/share/applications/dev.example.app.desktop": "resource"})
	if _, err := appDirEntries(p, dir); err == nil || !strings.Contains(err.Error(), "usr/share/applications/dev.example.app.desktop, from linux.share or NSGo, collides") {
		t.Errorf("a desktop entry among the resources: %v", err)
	}
	os.RemoveAll(filepath.Join(dir, "usr"))
	writeFiles(t, dir, map[string]string{"usr": "a file where usr/share is needed"})
	if _, err := appDirEntries(p, dir); err == nil || !strings.Contains(err.Error(), "collides") {
		t.Errorf("a file named usr among the resources: %v", err)
	}
}

// The Linux packages' metadata names a license only when linux.license gives one, no content rating, and Utility when linux.categories names no category, which AppStream needs; a Flatpak's keeps its defaults.
func TestDirectMetainfo(t *testing.T) {
	p := &project{Dir: t.TempDir(), Name: "App", ID: "dev.example.app", Version: "1.0.0"}
	direct := func() string {
		t.Helper()
		data, err := p.metainfo(true)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}
	got := direct()
	for _, unwanted := range []string{"project_license", "content_rating"} {
		if strings.Contains(got, unwanted) {
			t.Errorf("the metadata names %s:\n%s", unwanted, got)
		}
	}
	if !strings.Contains(got, "<categories>\n    <category>Utility</category>\n  </categories>") {
		t.Errorf("the metadata lacks the Utility category:\n%s", got)
	}
	p.Linux.License, p.Linux.Categories = "MIT", []string{"Office"}
	got = direct()
	if !strings.Contains(got, "<project_license>MIT</project_license>") || strings.Contains(got, "<categories>") {
		t.Errorf("the metadata with linux.license and linux.categories:\n%s", got)
	}
	p.Linux.License, p.Linux.Categories = "", nil
	flatpak, err := p.metainfo(false)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(flatpak); !strings.Contains(s, "<project_license>LicenseRef-proprietary</project_license>") || !strings.Contains(s, `<content_rating type="oars-1.1"/>`) || strings.Contains(s, "<categories>") {
		t.Errorf("the Flatpak's metadata changed:\n%s", s)
	}
}

// linux.share is relative to nsgo.yaml.
func TestLinuxShareConfig(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: App\nlinux: { share: packaging/share }\n"})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "packaging", "share"); p.Linux.Share != want {
		t.Errorf("linux.share is %s, want %s", p.Linux.Share, want)
	}
}
