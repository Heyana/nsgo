package main

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/authenticode"
	"github.com/nsgo-dev/nsgo/cmd/nsgo/windowsinstaller"
	projectcfg "github.com/nsgo-dev/nsgo/project"
)

// The payload reader must continue to locate the ZIP after Authenticode adds alignment padding and its certificate table. The uninstaller is signed before embedding; checking only the outer setup would miss an unsigned uninstall.
func TestWindowsInstallerSignedPayload(t *testing.T) {
	key := testWindowsKey(t)
	certificate := filepath.Join(t.TempDir(), "test.pfx")
	writeTestCertificate(t, certificate, key)
	dir := t.TempDir()
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "App", ID: "dev.nsgo.installersigned", Version: "1.2.0"}, Release: true}
	p.Sign.Certificate = certificate
	p.Sign.TimestampURL = "none"
	writeFiles(t, dir, map[string]string{"App.exe": "synthetic app payload", "resources/data.txt": "data"})
	setup, err := buildWindowsInstaller(t.Context(), p, dir, "x64")
	if err != nil {
		t.Fatal(err)
	}
	signer := windowsSignerOf(p)
	if err := signer.load(p); err != nil {
		t.Fatal(err)
	}
	if err := signer.signFile(t.Context(), setup, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(setup)
	if err != nil {
		t.Fatal(err)
	}
	if !authenticode.Signed(data) {
		t.Fatal("outer installer is unsigned")
	}
	archive, closer, err := windowsinstaller.OpenPayload(setup)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	found := false
	for _, file := range archive.File {
		if file.Name != "Uninstall.exe" {
			continue
		}
		found = true
		in, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(in)
		in.Close()
		if err != nil {
			t.Fatal(err)
		}
		if !authenticode.Signed(data) {
			t.Fatal("embedded uninstaller is unsigned")
		}
		verifyInstallerPE(t, data)
	}
	if !found {
		t.Fatal("installer has no uninstaller")
	}
	verifyInstallerPE(t, data)
}
func verifyInstallerPE(t *testing.T, data []byte) {
	t.Helper()
	image, err := pe.NewFile(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	header, ok := image.OptionalHeader.(*pe.OptionalHeader64)
	if !ok || header.Subsystem != 2 {
		t.Fatal("installer is not a GUI executable")
	}
	_, resources := decodeResources(t, image, image.Section(".rsrc").VirtualAddress)
	manifest := string(resources[resourceKey{24, 1, 0}])
	for _, required := range []string{`level="asInvoker"`, `Microsoft.Windows.Common-Controls`, `version="6.0.0.0"`, `longPathAware`} {
		if !strings.Contains(manifest, required) {
			t.Errorf("installer manifest lacks %s", required)
		}
	}
}
func verifyInstallerPayload(t *testing.T, setup string) {
	t.Helper()
	data, err := os.ReadFile(setup)
	if err != nil {
		t.Fatal(err)
	}
	verifyInstallerPE(t, data)
	archive, closer, err := windowsinstaller.OpenPayload(setup)
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	names := map[string]bool{}
	for _, file := range archive.File {
		names[file.Name] = true
		if strings.HasSuffix(file.Name, ".nsi") || strings.HasSuffix(file.Name, "-setup.exe") || strings.HasSuffix(file.Name, ".ico") {
			t.Errorf("installer embedded its build-only artifact %q", file.Name)
		}
		if file.Name == "Uninstall.exe" {
			in, err := file.Open()
			if err != nil {
				t.Fatal(err)
			}
			data, err := io.ReadAll(in)
			in.Close()
			if err != nil {
				t.Fatal(err)
			}
			verifyInstallerPE(t, data)
		}
	}
	for _, name := range []string{"App.exe", "fonts/a.ttf", "Uninstall.exe"} {
		if !names[name] {
			t.Errorf("installer omitted %s", name)
		}
	}
}

// ICO group attributes describe DIB images as well as PNGs; deriving every image's format from a square 32-bit default corrupts supplied ICO entries.
func TestWindowsInstallerPreservesICOAttributes(t *testing.T) {
	dir := t.TempDir()
	var icon bytes.Buffer
	binary.Write(&icon, binary.LittleEndian, [3]uint16{0, 1, 1})
	binary.Write(&icon, binary.LittleEndian, [4]uint8{16, 32, 16, 0})
	binary.Write(&icon, binary.LittleEndian, [2]uint16{1, 8})
	binary.Write(&icon, binary.LittleEndian, [2]uint32{40, 22})
	// This resource test checks the ICO header and opaque image preservation.
	icon.Write(make([]byte, 40))
	path := filepath.Join(dir, "custom.ico")
	if err := os.WriteFile(path, icon.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	resource, err := windowsInstallerResource(&project{Config: projectcfg.Config{Name: "App", Version: "1.0.0"}}, "amd64", path, "Setup.exe", " Setup")
	if err != nil {
		t.Fatal(err)
	}
	image, err := pe.NewFile(bytes.NewReader(resource))
	if err != nil {
		t.Fatal(err)
	}
	defer image.Close()
	_, data := decodeResources(t, image, 0)
	group := data[resourceKey{rtGroupIcon, iconGroupID, langEnUS}]
	if len(group) != 20 || !bytes.Equal(group[6:14], icon.Bytes()[6:14]) {
		t.Fatalf("ICO attributes changed: %x", group)
	}
}
