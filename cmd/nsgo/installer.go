package main

import (
	"os"
	"path/filepath"
	"strings"
)

// packageSlug is what the files of an app's installers and its Linux
// packages are named by: its name in lowercase ASCII words joined by
// hyphens, as release pages and package managers keep it whole, else the
// last part of its ID for a name with no such words.
func (p *project) packageSlug() string { return p.Config.PackageSlug() }

// releaseName names a release file of target's: <slug>-<version>-<system>-<arch>
// and then suffix, such as my-app-1.2.0-windows-x64-setup.exe, so a
// release page tells its downloads apart by system. Targets without an
// architecture, such as ios, give their name alone.
func (p *project) releaseName(target, suffix string) string {
	return p.packageSlug() + "-" + p.Version + "-" + strings.ReplaceAll(target, "_", "-") + suffix
}

// slug keeps s's ASCII letters and digits, in lowercase, joined by
// hyphens where anything else was.
func slug(s string) string {
	var b strings.Builder
	hyphen := false
	for _, r := range strings.ToLower(s) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			if hyphen && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			hyphen = false
		} else {
			hyphen = true
		}
	}
	return b.String()
}

// displayName is the app's name as users see it: its catalogs' appName in
// the source language, else nsgo.yaml's name.
func (p *project) displayName() (string, error) {
	n, err := p.appNames()
	if err != nil || n == nil {
		return p.Name, err
	}
	return n.names[n.source], nil
}

// iconFile is an installer's icon in the format ext names (.icns or
// .ico): path's own when it is one, else path's PNG or, without a path,
// the app's icon converted; nil without either.
func (p *project) iconFile(path, ext string) ([]byte, error) {
	if strings.EqualFold(filepath.Ext(path), ext) {
		return os.ReadFile(path)
	}
	img, err := p.iconImage()
	if ext == ".ico" {
		img, err = p.windowsIconImage()
	}
	if path != "" {
		img, err = readIcon(path)
	}
	if img == nil || err != nil {
		return nil, err
	}
	if ext == ".icns" {
		return icns(img), nil
	}
	return icoFile(img), nil
}
