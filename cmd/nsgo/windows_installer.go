package main

import (
	"archive/zip"
	"context"
	"embed"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/windowsinstaller"
)

//go:embed windowsinstaller/*.go
var windowsInstallerSources embed.FS

// buildGoWindowsInstaller builds both programs with the same Go toolchain as the app. The signed uninstaller is an ordinary payload member, so no host-side Windows compiler or post-link uninstaller hook is needed.
func buildGoWindowsInstaller(ctx context.Context, p *project, dir, arch string, d *windowsInstallerData) (string, error) {
	m := windowsinstaller.Manifest{ID: p.ID, Name: d.Name, Version: p.Version, Exe: d.Exe, Publisher: d.Publisher, ARM64: d.ARM64, Command: d.Command, URLSchemes: d.URLSchemes}
	for _, name := range d.Files {
		m.Files = append(m.Files, strings.ReplaceAll(name, `\`, "/"))
	}
	for _, name := range d.Dirs {
		m.Dirs = append(m.Dirs, strings.ReplaceAll(name, `\`, "/"))
	}
	for _, typ := range d.FileTypes {
		m.FileTypes = append(m.FileTypes, windowsinstaller.FileType{ProgID: typ.ProgID, Name: typ.Name, Extensions: typ.Extensions})
	}
	version, err := p.windowsVersion()
	if err != nil {
		return "", err
	}
	for _, name := range version.Tables {
		m.Names = append(m.Names, windowsinstaller.Name{Language: name.Language, Text: name.Name})
	}
	if err := m.Validate(); err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp("", "nsgo-windows-installer-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(temp)
	entries, err := windowsInstallerSources.ReadDir("windowsinstaller")
	if err != nil {
		return "", err
	}
	for _, entry := range entries {
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := windowsInstallerSources.ReadFile("windowsinstaller/" + entry.Name())
		if err != nil {
			return "", err
		}
		data = []byte(strings.Replace(string(data), "package windowsinstaller", "package main", 1))
		if err := os.WriteFile(filepath.Join(temp, entry.Name()), data, 0600); err != nil {
			return "", err
		}
	}
	main := `package main
import(_ "embed"; "encoding/json"; "fmt"; "os")
//go:embed manifest.json
var manifestBytes []byte
func main(){var manifest Manifest;if err:=json.Unmarshal(manifestBytes,&manifest);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)};os.Exit(Run(manifest))}
`

	if err := os.WriteFile(filepath.Join(temp, "main.go"), []byte(main), 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(temp, "go.mod"), []byte("module nsgo-windows-installer\n\ngo 1.27.3\n"), 0600); err != nil {
		return "", err
	}
	goarch := goArch(arch)
	compile := func(output, icon, suffix string, uninstall bool) error {
		m.Uninstall = uninstall
		data, err := json.Marshal(m)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(temp, "manifest.json"), data, 0600); err != nil {
			return err
		}
		syso, err := windowsInstallerResource(p, goarch, icon, filepath.Base(output), suffix)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(temp, windowsResourceObject(goarch)), syso, 0600); err != nil {
			return err
		}
		command := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-ldflags=-s -w -H=windowsgui", "-o", output, ".")
		command.Dir = temp
		command.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+goarch, "CGO_ENABLED=0", "GOWORK=off")
		if out, err := command.CombinedOutput(); err != nil {
			return fmt.Errorf("building Windows installer: %w\n%s", err, out)
		}
		return nil
	}
	stage(ctx, "building the uninstaller")
	uninstaller := filepath.Join(temp, "Uninstall.exe")
	if err := compile(uninstaller, p.Windows.UninstallerIcon, " Uninstall", true); err != nil {
		return "", err
	}
	if signer := windowsSignerOf(p); signer != nil {
		if err := signer.load(p); err != nil {
			return "", err
		}
		if err := signer.signFile(ctx, uninstaller, true); err != nil {
			return "", err
		}
	}
	payloadPath := filepath.Join(temp, "payload.zip")
	payload, err := os.Create(payloadPath)
	if err != nil {
		return "", err
	}
	archive := zip.NewWriter(payload)
	add := func(name, source string) error {
		file, err := os.Open(source)
		if err != nil {
			return err
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("installer payload %s is not a regular file", name)
		}
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = name
		header.Method = zip.Deflate
		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}
		_, err = io.Copy(writer, file)
		return err
	}
	for _, name := range m.Files {
		if err := add(name, filepath.Join(dir, filepath.FromSlash(name))); err != nil {
			archive.Close()
			payload.Close()
			return "", err
		}
	}
	if err := add("Uninstall.exe", uninstaller); err != nil {
		archive.Close()
		payload.Close()
		return "", err
	}
	if err := archive.Close(); err != nil {
		payload.Close()
		return "", err
	}
	if err := payload.Close(); err != nil {
		return "", err
	}
	stage(ctx, "building the installer")
	output := filepath.Join(dir, d.Output)
	if err := compile(output, p.Windows.InstallerIcon, " Setup", false); err != nil {
		return "", err
	}
	input, err := os.Open(payloadPath)
	if err != nil {
		return "", err
	}
	defer input.Close()
	if err := windowsinstaller.AppendPayload(output, input); err != nil {
		return "", err
	}
	return output, nil
}

// windowsInstallerResource retains each configured ICO image, the publisher and localized version tables, plus a manifest selecting native v6 controls and an asInvoker token. A file named setup.exe must not trigger UAC heuristics.
func windowsInstallerResource(p *project, arch, icon, exe, suffix string) ([]byte, error) {
	data, err := p.iconFile(icon, ".ico")
	if err != nil {
		return nil, err
	}
	var images [][]byte
	var sizes []int
	var iconEntries []windowsIconEntry
	if len(data) != 0 {
		if len(data) < 6 || binary.LittleEndian.Uint16(data[:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != 1 {
			return nil, fmt.Errorf("invalid installer ICO")
		}
		count := int(binary.LittleEndian.Uint16(data[4:6]))
		if count == 0 || 6+count*16 > len(data) {
			return nil, fmt.Errorf("invalid installer ICO directory")
		}
		for i := range count {
			entry := data[6+i*16:]
			n, offset := uint64(binary.LittleEndian.Uint32(entry[8:])), uint64(binary.LittleEndian.Uint32(entry[12:]))
			if offset < uint64(6+count*16) || offset+n > uint64(len(data)) || n == 0 {
				return nil, fmt.Errorf("invalid installer ICO image")
			}
			side := int(entry[0])
			if side == 0 {
				side = 256
			}
			images = append(images, data[offset:offset+n])
			sizes = append(sizes, side)
			iconEntries = append(iconEntries, windowsIconEntry{Shape: [4]uint8{entry[0], entry[1], entry[2], entry[3]}, Format: [2]uint16{binary.LittleEndian.Uint16(entry[4:]), binary.LittleEndian.Uint16(entry[6:])}})
		}
	}
	version, err := p.windowsVersion()
	if err != nil {
		return nil, err
	}
	version.Exe = exe
	for i := range version.Tables {
		version.Tables[i].Name += suffix
	}
	return windowsResourceWithManifest(arch, images, sizes, version, windowsInstallerManifest, iconEntries)
}

const windowsInstallerManifest = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly xmlns="urn:schemas-microsoft-com:asm.v1" manifestVersion="1.0">
<assemblyIdentity name="NSGo.Installer" processorArchitecture="*" version="1.0.0.0" type="win32"/>
<trustInfo xmlns="urn:schemas-microsoft-com:asm.v3"><security><requestedPrivileges><requestedExecutionLevel level="asInvoker" uiAccess="false"/></requestedPrivileges></security></trustInfo>
<dependency><dependentAssembly><assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/></dependentAssembly></dependency>
<application xmlns="urn:schemas-microsoft-com:asm.v3"><windowsSettings><dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware><longPathAware xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">true</longPathAware></windowsSettings></application>
<compatibility xmlns="urn:schemas-microsoft-com:compatibility.v1"><application><supportedOS Id="{8e0f7a12-bfb3-4fe8-b9a5-48fd50a15a9a}"/></application></compatibility>
</assembly>`
