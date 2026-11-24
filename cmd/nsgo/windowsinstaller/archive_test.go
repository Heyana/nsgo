package windowsinstaller

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testManifest() Manifest {
	return Manifest{ID: "dev.nsgo.installer.test", Name: "Test App", Exe: "App.exe", Files: []string{"App.exe", "resources/日本語.txt"}, Dirs: []string{"resources"}}
}
func testArchive(t *testing.T, names []string) (*zip.Reader, []byte) {
	t.Helper()
	var data bytes.Buffer
	writer := zip.NewWriter(&data)
	for _, name := range names {
		file, err := writer.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(file, "payload "+name)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	raw := data.Bytes()
	archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	return archive, raw
}
func TestExtractAndUninstallKeepUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	m := testManifest()
	names := append(append([]string{}, m.Files...), "Uninstall.exe")
	archive, _ := testArchive(t, names)
	foreign := filepath.Join(root, "user settings.txt")
	if err := os.WriteFile(foreign, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := Extract(root, m, archive, nil); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(name)))
		if err != nil || string(data) != "payload "+name {
			t.Fatalf("extracted %s: %q, %v", name, data, err)
		}
	}
	if err := RemoveFiles(root, m, false); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(foreign); err != nil || string(data) != "keep" {
		t.Fatalf("uninstall changed another file: %q, %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(root, "resources")); !os.IsNotExist(err) {
		t.Fatalf("owned empty directory remains: %v", err)
	}
}
func TestExtractValidatesAllEntriesBeforeWriting(t *testing.T) {
	for _, bad := range []string{"../outside", "/outside", `dir\outside`, `C:/outside`, "App.exe"} {
		t.Run(bad, func(t *testing.T) {
			m := testManifest()
			archive, _ := testArchive(t, append(append(append([]string{}, m.Files...), "Uninstall.exe"), bad))
			destination := filepath.Join(t.TempDir(), "not-created")
			if err := Extract(destination, m, archive, nil); err == nil {
				t.Fatal("invalid archive was accepted")
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatal("archive validation wrote its destination")
			}
		})
	}
}
func TestFailedChecksumDoesNotReplaceAnExistingFile(t *testing.T) {
	m := testManifest()
	archive, data := testArchive(t, append(append([]string{}, m.Files...), "Uninstall.exe"))
	offset, err := archive.File[0].DataOffset()
	if err != nil {
		t.Fatal(err)
	}
	data[offset] ^= 1
	archive, err = zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	original := filepath.Join(root, m.Exe)
	os.WriteFile(original, []byte("old version"), 0600)
	if err := Extract(root, m, archive, nil); err == nil {
		t.Fatal("corrupted payload was accepted")
	}
	got, err := os.ReadFile(original)
	if err != nil || string(got) != "old version" {
		t.Fatalf("failed extraction replaced the existing app: %q, %v", got, err)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.Contains(entry.Name(), ".nsgo-") {
			t.Error("failed extraction left a staging file")
		}
	}
}
func TestExtractDoesNotFollowADirectoryLinkOutsideTheRoot(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	if err := os.Symlink(outside, filepath.Join(root, "resources")); err != nil {
		t.Skipf("directory symlinks unavailable: %v", err)
	}
	m := testManifest()
	archive, _ := testArchive(t, append(append([]string{}, m.Files...), "Uninstall.exe"))
	if err := Extract(root, m, archive, nil); err == nil {
		t.Fatal("extraction escaped through a directory link")
	}
	entries, err := os.ReadDir(outside)
	if err != nil || len(entries) != 0 {
		t.Fatalf("extraction wrote outside the root: %v, %v", entries, err)
	}
}
func TestManifestRejectsAmbiguousAndReservedFiles(t *testing.T) {
	for _, name := range []string{"app.EXE", "Uninstall.exe", "../outside", `C:\outside`} {
		m := testManifest()
		m.Files = append(m.Files, name)
		if err := m.Validate(); err == nil {
			t.Errorf("accepted manifest file %q", name)
		}
	}
	m := testManifest()
	m.FileTypes = []FileType{{ProgID: m.ID + ".Document", Extensions: []string{"tar.gz"}}}
	if err := m.Validate(); err != nil {
		t.Fatalf("valid dotted extension: %v", err)
	}
}
