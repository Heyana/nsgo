package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileTypesRegisterInThePlist(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{"nsgo.yaml": `name: Flow
id: me.oneo.flow
file_types:
  - name: Flow File
    extensions: [flow]
    mime: [application/x-me.oneo.flow.flow]
  - name: Markdown
    id: net.daringfireball.markdown
    extensions: [md, markdown]
    role: viewer
    conforms: [public.plain-text]
`})
	p, err := load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := p.FileTypes[0].ID; got != "me.oneo.flow.flow" {
		t.Errorf("the default type is %q", got)
	}
	path := filepath.Join(t.TempDir(), "Info.plist")
	if err := writePlist(path, p, macPlist, p.fileTypesPlist()); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	plist := string(data)
	exported, imported, _ := strings.Cut(plist[strings.Index(plist, "UTExportedTypeDeclarations"):], "UTImportedTypeDeclarations")
	for _, want := range []string{
		"<key>CFBundleTypeRole</key><string>Editor</string>",
		"<key>LSHandlerRank</key><string>Owner</string>",
		"<key>CFBundleTypeRole</key><string>Viewer</string>",
		"<key>LSHandlerRank</key><string>Alternate</string>",
		"<key>public.mime-type</key><array><string>application/x-me.oneo.flow.flow</string></array>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}
	if !strings.Contains(exported, "<string>me.oneo.flow.flow</string>") || !strings.Contains(exported, "<string>public.data</string>") {
		t.Errorf("the app's own type is not exported:\n%s", exported)
	}
	if !strings.Contains(imported, "<array><string>md</string><string>markdown</string></array>") ||
		strings.Contains(imported, "me.oneo.flow.flow") {
		t.Errorf("another's type is not imported:\n%s", imported)
	}
	if lint, err := exec.LookPath("plutil"); err == nil {
		if out, err := exec.Command(lint, "-lint", path).CombinedOutput(); err != nil {
			t.Errorf("plutil: %s", out)
		}
	}
}

func TestFileTypesAreChecked(t *testing.T) {
	for yaml, want := range map[string]string{
		"- name: Flow\n  extensions: [.flow]":                               "not .flow",
		"- name: Flow":                                                      "extensions is required",
		"- extensions: [flow]":                                              "name is required",
		"- name: Flow\n  extensions: [flow]\n  role: owner":                 "role is \"owner\"",
		"- name: Flow\n  extensions: [flow]\n  icon: flow.svg":              "neither an .icns nor a .png",
		"- name: A\n  extensions: [flow]\n- name: B\n  extensions: [flow]":  "listed twice",
		"- name: Flow\n  extensions: [flow]\n  mime: [video/*]":             "not a media type",
		"- name: Flow\n  extensions: [flow]\n  mime: ['video/mp4;foo=bar']": "not a media type",
		"- name: Flow\n  extensions: [flow]\n  mime: [video/mp4/extra]":     "not a media type",
	} {
		dir := t.TempDir()
		writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Flow\nfile_types:\n" + yamlItems(yaml)})
		if _, err := load(dir); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: %v, want %q", yaml, err, want)
		}
	}
}

func yamlItems(s string) string {
	return "  " + strings.ReplaceAll(s, "\n", "\n  ") + "\n"
}

func TestFileTypesRegisterInTheIOSPlist(t *testing.T) {
	p := &project{Name: "Documents", ID: "dev.nsgo.documents", FileTypes: []fileType{
		{Name: "Document & Notes", ID: "dev.nsgo.documents.note", Extensions: []string{"note"}, MIME: []string{"application/x-nsgo-note"}, Icon: "note.icns"},
		{Name: "Plain text", ID: "public.plain-text", Extensions: []string{"txt"}, Role: "viewer", Conforms: []string{"public.text"}},
	}}
	extra := p.iosFileTypesPlist(iOS)
	for _, want := range []string{
		"<key>CFBundleDocumentTypes</key>",
		"<key>UTExportedTypeDeclarations</key>",
		"<key>UTImportedTypeDeclarations</key>",
		"<key>LSItemContentTypes</key><array><string>dev.nsgo.documents.note</string></array>",
		"<key>CFBundleTypeName</key><string>Document &amp; Notes</string>",
		"<key>CFBundleTypeRole</key><string>Viewer</string>",
		"<key>public.mime-type</key><array><string>application/x-nsgo-note</string></array>",
	} {
		if !strings.Contains(extra, want) {
			t.Errorf("iOS document declaration lacks %q", want)
		}
	}
	if strings.Contains(extra, ".icns") || strings.Contains(extra, "CFBundleTypeIconFile") || strings.Contains(extra, "UTTypeIconFile") {
		t.Fatal("the iOS bundle declared macOS document icons")
	}
	if !strings.Contains(p.fileTypesPlist(), "dev.nsgo.documents.note.icns") {
		t.Fatal("the macOS document icon was lost")
	}
	if got := p.iosFileTypesPlist(tvOS); got != "" {
		t.Fatalf("an Apple TV declared documents: %s", got)
	}
	p.clipParent = "dev.nsgo.parent"
	if got := p.iosFileTypesPlist(iOS); got != "" {
		t.Fatalf("an App Clip declared documents: %s", got)
	}
	if got := (&project{}).iosFileTypesPlist(iOS); got != "" {
		t.Fatalf("an app without file types declared documents: %s", got)
	}
}
