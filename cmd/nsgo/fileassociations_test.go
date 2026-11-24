package main

import (
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func associationProject(t *testing.T) *project {
	t.Helper()
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"nsgo.yaml": `name: My App
id: dev.example.documents
version: 1.0.0
file_types:
  - name: Movie
    id: public.movie
    extensions: [MP4, mov, mkv, webm, avi, flv]
    role: viewer
  - name: Flow & Notes
    extensions: [flow, flownote]
  - name: External
    id: org.example.external
    extensions: [external]
    mime: [application/vnd.example.external]
`})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLinuxFileAssociations(t *testing.T) {
	p := associationProject(t)
	types, definitions, err := p.linuxFileTypes()
	if err != nil {
		t.Fatal(err)
	}
	wantTypes := []string{"video/mp4", "video/quicktime", "video/x-matroska", "video/webm", "video/x-msvideo", "video/x-flv",
		"application/x-dev.example.documents.flow", "application/vnd.example.external"}
	if !slices.Equal(types, wantTypes) {
		t.Fatalf("media types = %v; want %v", types, wantTypes)
	}
	var info struct {
		XMLName xml.Name
		Types   []struct {
			Type    string `xml:"type,attr"`
			Comment string `xml:"comment"`
			Globs   []struct {
				Pattern string `xml:"pattern,attr"`
			} `xml:"glob"`
		} `xml:"mime-type"`
	}
	if err := xml.Unmarshal(definitions, &info); err != nil {
		t.Fatal(err)
	}
	if info.XMLName.Space != "http://www.freedesktop.org/standards/shared-mime-info" || len(info.Types) != 1 {
		t.Fatalf("MIME definitions declare imported or known types: %s", definitions)
	}
	custom := info.Types[0]
	if custom.Type != wantTypes[6] || custom.Comment != "Flow & Notes" || len(custom.Globs) != 2 || custom.Globs[0].Pattern != "*.flow" || custom.Globs[1].Pattern != "*.flownote" {
		t.Fatalf("custom type = %+v", custom)
	}

	// The portable/AppImage and Flatpak payload must have a desktop entry even when an app has no icon. Both use the staged entry unchanged.
	stage := t.TempDir()
	// The executable must keep its Linux mode even on a Windows build host, whose filesystem reports no Unix executable permission bits.
	writeFiles(t, stage, map[string]string{p.Name: "binary"})
	if err := writeLinuxIcon(p, stage); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(stage, p.ID+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	check := func(entry string, executable string) {
		t.Helper()
		for _, want := range []string{`Exec="` + executable + `" %F` + "\n", "MimeType=" + strings.Join(wantTypes, ";") + ";\n"} {
			if !strings.Contains(entry, want) {
				t.Errorf("desktop entry lacks %q:\n%s", want, entry)
			}
		}
	}
	check(string(entry), p.Name)
	files, err := linuxPackageFiles(p, stage)
	if err != nil {
		t.Fatal(err)
	}
	deb, err := debPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	data := tarFiles(t, arMembers(t, deb)["data.tar.gz"])
	check(data["./usr/share/applications/"+p.ID+".desktop"], "/opt/my-app/"+p.Name)
	if got := data["./usr/share/mime/packages/"+p.ID+".xml"]; got != string(definitions) {
		t.Errorf("Debian package MIME definition = %q", got)
	}
	control := tarFiles(t, arMembers(t, deb)["control.tar.gz"])
	for _, script := range []string{"postinst", "postrm"} {
		if !strings.Contains(control["./"+script], "update-mime-database /usr/share/mime") || !strings.Contains(control["./"+script], "update-desktop-database -q /usr/share/applications") {
			t.Errorf("Debian package lacks cache updates in %s", script)
		}
	}
	arch, err := archPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	data = tarFiles(t, arch)
	check(data["usr/share/applications/"+p.ID+".desktop"], "/opt/my-app/"+p.Name)
	if data["usr/share/mime/packages/"+p.ID+".xml"] != string(definitions) || !strings.Contains(data[".INSTALL"], "post_remove()") {
		t.Error("Arch package lacks its MIME definition or removal cache hook")
	}
	entries, err := appDirEntries(p, stage)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]string{}
	for _, e := range entries {
		found[e.name] = string(e.data)
		if e.name == p.Name && e.mode&0o111 == 0 {
			t.Error("AppImage/store payload lost the main executable's mode")
		}
	}
	check(found[p.ID+".desktop"], p.Name)
	if found["usr/share/mime/packages/"+p.ID+".xml"] != string(definitions) {
		t.Error("AppImage lacks its MIME definition")
	}
	archive, err := buildLinuxArchive(t.Context(), p, stage, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(archive)
	if err != nil {
		t.Fatal(err)
	}
	data = tarFiles(t, b)
	check(data["my-app/"+p.ID+".desktop"], p.Name)
	if data["my-app/"+p.ID+".mime.xml"] != string(definitions) {
		t.Error("Flatpak archive lacks its MIME definition")
	}
}

func TestFileAssociationsWithoutDeclarations(t *testing.T) {
	p := associationProject(t)
	p.FileTypes = nil
	dir := t.TempDir()
	if err := writeLinuxIcon(p, dir); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(dir, p.ID+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(entry), "%F") || strings.Contains(string(entry), "MimeType=") {
		t.Errorf("an app without declarations registers types: %s", entry)
	}
	types, definitions, err := p.linuxFileTypes()
	if err != nil || len(types) != 0 || len(definitions) != 0 {
		t.Fatalf("no declarations = %v, %s, %v", types, definitions, err)
	}
	d, err := windowsInstallerPlan(t.Context(), p, dir, "x64")
	if err != nil {
		t.Fatal(err)
	}
	if len(d.FileTypes) != 0 {
		t.Error("an app without declarations registers Windows types")
	}
}

func TestLinuxFileAssociationsRequireImportedMIME(t *testing.T) {
	p := associationProject(t)
	p.FileTypes[2].MIME = nil
	if _, _, err := p.linuxFileTypes(); err == nil || !strings.Contains(err.Error(), "needs mime") {
		t.Fatalf("unknown imported type = %v", err)
	}
	// An explicit MIME replaces inference, including a known extension; duplicate declarations produce one desktop association.
	p.FileTypes[2].MIME = []string{"video/mp4", "video/mp4"}
	types, _, err := p.linuxFileTypes()
	if err != nil || len(types) != 7 {
		t.Fatalf("explicit media types = %v, %v", types, err)
	}
}

func TestWindowsInstallerFileAssociations(t *testing.T) {
	p := associationProject(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{p.Name + ".exe": "exe"})
	d, err := windowsInstallerPlan(t.Context(), p, dir, "x64")
	if err != nil {
		t.Fatal(err)
	}
	for i, f := range d.FileTypes {
		if !strings.HasPrefix(f.ProgID, p.ID+".") || f.Name != p.FileTypes[i].Name || !slices.Equal(f.Extensions, p.FileTypes[i].Extensions) {
			t.Fatalf("invalid type: %+v", f)
		}
	}
	slices.Reverse(p.FileTypes)
	p.FileTypes[0].Name = "Translated"
	newer, err := windowsInstallerPlan(t.Context(), p, dir, "x64")
	if err != nil {
		t.Fatal(err)
	}
	if newer.FileTypes[0].ProgID != d.FileTypes[2].ProgID {
		t.Error("ProgID changed when reordered or renamed")
	}
}
