package main

import (
	"encoding/xml"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestURLSchemesRegisterInThePlist(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Umapp\nid: me.oneo.umapp\nurl_schemes: [Umapp, umapp-dev]\n"})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(p.URLSchemes, ","); got != "umapp,umapp-dev" {
		t.Errorf("schemes %q", got)
	}
	path := filepath.Join(t.TempDir(), "Info.plist")
	if err := writePlist(path, p, macPlist, p.urlSchemesPlist()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	plist := string(data)
	for _, want := range []string{
		"<key>CFBundleURLTypes</key>",
		"<key>CFBundleURLName</key><string>me.oneo.umapp</string>",
		"<key>CFBundleURLSchemes</key><array><string>umapp</string><string>umapp-dev</string></array>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}
	if lint, err := exec.LookPath("plutil"); err == nil {
		if out, err := exec.Command(lint, "-lint", path).CombinedOutput(); err != nil {
			t.Errorf("plutil: %s", out)
		}
	}
}

// The iOS app registers the schemes too, its App Clip and an Apple TV's app none (#909).
func TestURLSchemesRegisterInTheIOSPlist(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "Umapp", ID: "me.oneo.umapp", URLSchemes: []string{"umapp"}}}
	if got := p.iosURLSchemesPlist(iOS); got != p.urlSchemesPlist() || got == "" {
		t.Errorf("the iOS app registers %q", got)
	}
	if got := p.iosURLSchemesPlist(tvOS); got != "" {
		t.Errorf("the Apple TV app registers %q", got)
	}
	clip := &project{Config: projectcfg.Config{Name: "Umapp Clip", ID: "me.oneo.umapp.Clip", URLSchemes: []string{"umapp"}}, clipParent: "me.oneo.umapp"}
	if got := clip.iosURLSchemesPlist(iOS); got != "" {
		t.Errorf("the App Clip registers %q", got)
	}
	path := filepath.Join(t.TempDir(), "Info.plist")
	if err := writePlist(path, p, iosPlist, p.iosURLSchemesPlist(iOS)); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "<key>CFBundleURLSchemes</key><array><string>umapp</string></array>") {
		t.Errorf("the iOS Info.plist lacks the scheme:\n%s", data)
	}
	if lint, err := exec.LookPath("plutil"); err == nil {
		if out, err := exec.Command(lint, "-lint", path).CombinedOutput(); err != nil {
			t.Errorf("plutil: %s", out)
		}
	}
}

func TestNoURLSchemesWriteNothing(t *testing.T) {
	if got := (&project{Config: projectcfg.Config{ID: "me.oneo.umapp"}}).urlSchemesPlist(); got != "" {
		t.Errorf("an app without schemes registers %q", got)
	}
}

func TestURLSchemesAreChecked(t *testing.T) {
	for schemes, want := range map[string]string{
		"[umapp://]":     "not a URL scheme",
		"[1app]":         "not a URL scheme",
		"[https]":        "belongs to the system",
		"[MailTo]":       "belongs to the system",
		"[umapp, UMAPP]": "listed twice",
		"[u]":            "drive",
	} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Umapp\nurl_schemes: " + schemes + "\n"})
		if _, err := load(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", schemes, err, want)
		}
	}
}

// The Windows installer registers each scheme under HKCU\Software\Classes, opening the app with the link after "--", records it with the uninstall entry, and removes what it recorded, unless another app took the scheme since.
func TestURLSchemesRegisterInTheInstaller(t *testing.T) {
	dir := t.TempDir()
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "My App", ID: "me.oneo.umapp", Version: "1.0.0", URLSchemes: []string{"umapp", "umapp-dev"}}}
	writeFiles(t, dir, map[string]string{p.Name + ".exe": "exe"})
	d, err := windowsInstallerPlan(t.Context(), p, dir, "x64")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(d.URLSchemes, ",") != "umapp,umapp-dev" {
		t.Fatalf("installer schemes: %v", d.URLSchemes)
	}
}

func TestURLSchemesRegisterInTheMSIX(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "My App", ID: "me.oneo.umapp", Version: "1.0.0", URLSchemes: []string{"umapp"}}}
	data, err := msixManifest(p, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		Extensions []struct {
			Category string `xml:"Category,attr"`
			Protocol struct {
				Name       string `xml:"Name,attr"`
				Parameters string `xml:"Parameters,attr"`
			} `xml:"Protocol"`
		} `xml:"Applications>Application>Extensions>Extension"`
	}
	if err := xml.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	// Every MSIX also declares its startup task (storepkg_test.go); only the protocols count here.
	var protocols []string
	for _, e := range m.Extensions {
		if e.Category == "windows.protocol" {
			protocols = append(protocols, e.Protocol.Name+" "+e.Protocol.Parameters)
		}
	}
	if len(protocols) != 1 || protocols[0] != `umapp -- "%1"` {
		t.Errorf("the protocols are %+v:\n%s", m.Extensions, data)
	}
	p.URLSchemes = []string{strings.Repeat("u", 40)}
	if _, err := msixManifest(p, "amd64"); err == nil {
		t.Error("a 40-character scheme went into the manifest")
	}
}

// The Linux desktop entry registers each scheme as an x-scheme-handler type beside the file types, with %U, which passes links and files alike, and the packages refresh the desktop database for them.
func TestURLSchemesRegisterInTheDesktopEntry(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "Umapp", ID: "me.oneo.umapp", Version: "1.0.0", URLSchemes: []string{"umapp", "umapp-dev"}}}
	stage := t.TempDir()
	writeFiles(t, stage, map[string]string{p.Name: "binary"})
	if err := writeLinuxIcon(p, stage); err != nil {
		t.Fatal(err)
	}
	entry, err := os.ReadFile(filepath.Join(stage, p.ID+".desktop"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`Exec="Umapp" %U` + "\n", "MimeType=x-scheme-handler/umapp;x-scheme-handler/umapp-dev;\n"} {
		if !strings.Contains(string(entry), want) {
			t.Errorf("desktop entry lacks %q:\n%s", want, entry)
		}
	}
	files, err := linuxPackageFiles(p, stage)
	if err != nil {
		t.Fatal(err)
	}
	deb, err := debPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if data := tarFiles(t, arMembers(t, deb)["data.tar.gz"])["./usr/share/applications/"+p.ID+".desktop"]; !strings.Contains(data, "x-scheme-handler/umapp;") || !strings.Contains(data, " %U\n") {
		t.Errorf("the Debian package's desktop entry:\n%s", data)
	}
	if !strings.Contains(tarFiles(t, arMembers(t, deb)["control.tar.gz"])["./postinst"], "update-desktop-database") {
		t.Error("the Debian package does not refresh the desktop database")
	}
	arch, err := archPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(tarFiles(t, arch)[".INSTALL"], "update-desktop-database") {
		t.Error("the Arch package does not refresh the desktop database")
	}

	// With file types too, %U passes their files as file URLs, which the backend takes as files.
	p.FileTypes = []fileType{{Name: "Flow", ID: p.ID + ".flow", Extensions: []string{"flow"}}}
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := writeLinuxIcon(p, stage); err != nil {
		t.Fatal(err)
	}
	entry, _ = os.ReadFile(filepath.Join(stage, p.ID+".desktop"))
	if want := "MimeType=application/x-me.oneo.umapp.flow;x-scheme-handler/umapp;x-scheme-handler/umapp-dev;\n"; !strings.Contains(string(entry), want) || strings.Contains(string(entry), "%F") {
		t.Errorf("desktop entry with file types, want %q and %%U:\n%s", want, entry)
	}
}
