// Package windowsinstaller is the Windows installer runtime embedded as source by the nsgo tool. It builds with Go alone, independently of the UI engine.
package windowsinstaller

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
)

// Manifest describes one per-user installation. The tool embeds it in both programs, so uninstall removes an explicit file list rather than a directory.
type Manifest struct {
	ID, Name, Version, Exe, Publisher string
	ARM64, Command, Uninstall         bool
	Files, Dirs                       []string
	FileTypes                         []FileType
	URLSchemes                        []string
	Names                             []Name
}

// FileType is an application-specific ProgID and its Open With extensions.
type FileType struct {
	ProgID, Name string
	Extensions   []string
}

// Name is the app's display name in a Windows UI language.
type Name struct {
	Language uint16
	Text     string
}

// UninstallKey is compatible with earlier NSIS installations of this app.
func (m Manifest) UninstallKey() string {
	return `Software\Microsoft\Windows\CurrentVersion\Uninstall\` + m.ID
}

// ValidPath accepts portable relative payload paths. Windows applies its own reserved-name checks as well when it opens them inside an os.Root.
func ValidPath(name string) bool {
	return fs.ValidPath(name) && name != "." && !strings.ContainsAny(name, "\\:\x00")
}

// Validate refuses a malformed embedded manifest before any filesystem or registry change. A directory chosen by the user never expands this file list.
func (m Manifest) Validate() error {
	if m.ID == "" || strings.ContainsAny(m.ID, "\\/\x00") || m.Name == "" || strings.ContainsRune(m.Name, 0) || !ValidPath(m.Exe) || strings.Contains(m.Exe, "/") {
		return errors.New("invalid installer identity or executable")
	}
	seen := map[string]bool{}
	for _, name := range m.Files {
		key := strings.ToLower(name)
		if !ValidPath(name) || seen[key] || key == "uninstall.exe" {
			return fmt.Errorf("invalid or duplicate installer file %q", name)
		}
		seen[key] = true
	}
	if !seen[strings.ToLower(m.Exe)] {
		return errors.New("installer payload has no application executable")
	}
	for _, name := range m.Dirs {
		if !ValidPath(name) {
			return fmt.Errorf("invalid installer directory %q", name)
		}
	}
	for _, typ := range m.FileTypes {
		if typ.ProgID == "" || strings.ContainsAny(typ.ProgID, "\\/\x00") {
			return errors.New("invalid installer ProgID")
		}
		for _, extension := range typ.Extensions {
			if extension == "" || strings.HasPrefix(extension, ".") || strings.ContainsAny(extension, "\\/\x00") {
				return errors.New("invalid installer extension")
			}
		}
	}
	for _, scheme := range m.URLSchemes {
		if scheme == "" || strings.ContainsAny(scheme, "\\/\x00") {
			return errors.New("invalid installer URL scheme")
		}
	}
	return nil
}
