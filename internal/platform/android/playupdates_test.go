package android

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestPlayUpdatesJava(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	sources := []string{"java/dev/oneo/nsgo/PlayUpdates.java"}
	err := filepath.WalkDir("testdata/playupdates", func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && filepath.Ext(path) == ".java" {
			sources = append(sources, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), javac, append([]string{"-d", dir}, sources...)...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.PlayUpdatesCheck").CombinedOutput(); err != nil {
		t.Fatalf("Play updates: %v\n%s", err, out)
	}
}
