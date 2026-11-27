package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// fileTypeOwned reports whether the app declares f as its own type.
func fileTypeOwned(f fileType, appID string) bool {
	return strings.HasPrefix(f.ID, appID+".")
}

// fileTypeIconName is the name of f's icon in the package's resources.
func fileTypeIconName(f fileType) string {
	if f.Icon == "" {
		return ""
	}
	return f.ID + ".icns"
}

// writeFileTypeIcons writes the file types' icons into a macOS package's
// resources.
func (p *project) writeFileTypeIcons(resources string) error {
	for _, f := range p.FileTypes {
		if f.Icon == "" {
			continue
		}
		var data []byte
		if filepath.Ext(f.Icon) == ".icns" {
			var err error
			if data, err = os.ReadFile(f.Icon); err != nil {
				return err
			}
		} else {
			img, err := readIcon(f.Icon)
			if err != nil {
				return err
			}
			data = icns(img)
		}
		if err := os.WriteFile(filepath.Join(resources, fileTypeIconName(f)), data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

// fileTypesPlist is the Info.plist entries that register the file types:
// the documents the app opens, and the types it declares or imports.
func (p *project) fileTypesPlist() string { return p.documentTypesPlist(true) }

// iosFileTypesPlist registers documents for a full iOS app. The tool
// leaves these declarations out of Apple TV and App Clip bundles.
// Document icons currently use the app icon on iOS; .icns keys are macOS's.
func (p *project) iosFileTypesPlist(sys *appleOS) string {
	if sys != iOS || p.clipParent != "" {
		return ""
	}
	docs := p.documentTypesPlist(false)
	if docs == "" {
		return ""
	}
	// Opened documents arrive as copies (internal/platform/ios/openfiles.go), not in place; App Store Connect warns when an app declaring document types leaves this unset.
	return docs + "\n\t<key>LSSupportsOpeningDocumentsInPlace</key><false/>"
}

func (p *project) documentTypesPlist(macIcons bool) string {
	if len(p.FileTypes) == 0 {
		return ""
	}
	var docs, exported, imported strings.Builder
	for _, f := range p.FileTypes {
		role, rank := "Editor", "Owner"
		if f.Role == "viewer" {
			role = "Viewer"
		}
		decls := &exported
		if !fileTypeOwned(f, p.ID) {
			rank, decls = "Alternate", &imported
		}
		docs.WriteString("\t\t<dict>\n")
		fmt.Fprintf(&docs, "\t\t\t<key>CFBundleTypeName</key><string>%s</string>\n", xmlText(f.Name))
		fmt.Fprintf(&docs, "\t\t\t<key>CFBundleTypeRole</key><string>%s</string>\n", role)
		fmt.Fprintf(&docs, "\t\t\t<key>LSHandlerRank</key><string>%s</string>\n", rank)
		fmt.Fprintf(&docs, "\t\t\t<key>LSItemContentTypes</key><array><string>%s</string></array>\n", xmlText(f.ID))
		if name := fileTypeIconName(f); macIcons && name != "" {
			fmt.Fprintf(&docs, "\t\t\t<key>CFBundleTypeIconFile</key><string>%s</string>\n", xmlText(name))
		}
		docs.WriteString("\t\t</dict>\n")

		conforms := f.Conforms
		if len(conforms) == 0 {
			conforms = []string{"public.data"}
		}
		decls.WriteString("\t\t<dict>\n")
		fmt.Fprintf(decls, "\t\t\t<key>UTTypeIdentifier</key><string>%s</string>\n", xmlText(f.ID))
		fmt.Fprintf(decls, "\t\t\t<key>UTTypeDescription</key><string>%s</string>\n", xmlText(f.Name))
		fmt.Fprintf(decls, "\t\t\t<key>UTTypeConformsTo</key>%s\n", plistStrings(conforms))
		if name := fileTypeIconName(f); macIcons && name != "" {
			fmt.Fprintf(decls, "\t\t\t<key>UTTypeIconFile</key><string>%s</string>\n", xmlText(name))
		}
		fmt.Fprintf(decls, "\t\t\t<key>UTTypeTagSpecification</key><dict><key>public.filename-extension</key>%s",
			plistStrings(f.Extensions))
		if len(f.MIME) > 0 {
			fmt.Fprintf(decls, "<key>public.mime-type</key>%s", plistStrings(f.MIME))
		}
		decls.WriteString("</dict>\n")
		decls.WriteString("\t\t</dict>\n")
	}
	out := "\t<key>CFBundleDocumentTypes</key>\n\t<array>\n" + docs.String() + "\t</array>"
	if exported.Len() > 0 {
		out += "\n\t<key>UTExportedTypeDeclarations</key>\n\t<array>\n" + exported.String() + "\t</array>"
	}
	if imported.Len() > 0 {
		out += "\n\t<key>UTImportedTypeDeclarations</key>\n\t<array>\n" + imported.String() + "\t</array>"
	}
	return out
}

func plistStrings(values []string) string {
	var b strings.Builder
	b.WriteString("<array>")
	for _, v := range values {
		fmt.Fprintf(&b, "<string>%s</string>", xmlText(v))
	}
	b.WriteString("</array>")
	return b.String()
}
