package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
)

// windowsSetupName is the installer's release file name.
func windowsSetupName(p *project, arch string) string {
	return p.releaseName("windows_"+arch, "-setup.exe")
}

// buildWindowsInstaller builds a per-user setup and its embedded uninstaller with Go alone. Both use native Windows controls and run without elevation.
func buildWindowsInstaller(ctx context.Context, p *project, dir, arch string) (string, error) {
	d, err := windowsInstallerPlan(ctx, p, dir, arch)
	if err != nil {
		return "", err
	}
	return buildGoWindowsInstaller(ctx, p, dir, arch, d)
}

type windowsInstallerData struct {
	Name, Version, Exe, Output, UninstallKey, Publisher string
	ARM64, Command                                      bool
	Files, Dirs, Excludes                               []string
	FileTypes                                           []windowsInstallerFileType
	URLSchemes                                          []string
}

// A type's stable ID determines its ProgID, independently of display names and order.
type windowsInstallerFileType struct {
	ProgID, Name string
	Extensions   []string
}

func windowsInstallerPlan(_ context.Context, p *project, dir, arch string) (*windowsInstallerData, error) {
	name, err := p.displayName()
	if err != nil {
		return nil, err
	}
	d := &windowsInstallerData{
		Name: name, Version: p.Version, Exe: p.Name + ".exe", Output: windowsSetupName(p, arch),
		UninstallKey: `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + p.ID,
		Publisher:    p.windowsPublisher(),
		ARM64:        arch == "arm64",
		Command:      p.windowsCommandLine() || p.windowsPath(),
		// An earlier build's NSIS script and this app's setups stay out; the app's own programs named *-setup.exe go in.
		Excludes: []string{p.Name + ".nsi", p.packageSlug() + "-*-windows-*-setup.exe"},
	}
	d.URLSchemes = p.URLSchemes
	for _, f := range p.FileTypes {
		sum := sha256.Sum256([]byte(f.ID))
		d.FileTypes = append(d.FileTypes, windowsInstallerFileType{
			ProgID: fmt.Sprintf("%s.File.%x", p.ID, sum[:8]), Name: f.Name, Extensions: f.Extensions,
		})
	}
	err = filepath.WalkDir(dir, func(path string, e fs.DirEntry, err error) error {
		if err != nil || path == dir {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if !strings.ContainsRune(rel, filepath.Separator) {
			for _, x := range d.Excludes {
				if ok, _ := filepath.Match(x, rel); ok {
					return nil
				}
			}
		}
		rel = strings.ReplaceAll(rel, "/", `\`)
		if e.IsDir() {
			d.Dirs = append(d.Dirs, rel)
		} else {
			d.Files = append(d.Files, rel)
		}
		return nil
	})
	// Deepest first, so each is empty when it is removed.
	slices.Reverse(d.Dirs)
	return d, err
}
