package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAndroidHostTracksPackagingInputs(t *testing.T) {
	p := &project{Dir: t.TempDir()}
	p.Android.Classes = []string{"native/views.dex"}
	write := func(name, text string) {
		t.Helper()
		path := filepath.Join(p.Dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("nsgo.yaml", "name: Demo\n")
	write("resources/nested/asset.txt", "first")
	write("native/views.dex", "first classes")
	fingerprint := androidHostInputs(p)
	write("main.go", "package main\n")
	if got := androidHostInputs(p); got != fingerprint {
		t.Fatal("Go-only edit changed the APK inputs")
	}
	for _, change := range []struct{ path, contents string }{
		{"resources/nested/asset.txt", "second resource"},
		{"native/views.dex", "second classes"},
		{"assets/Icon-dev.png", "new launcher icon"},
		{"nsgo.yaml", "name: Demo\nandroid:\n  permissions: [ACCESS_WIFI_STATE]\n"},
	} {
		write(change.path, change.contents)
		next := androidHostInputs(p)
		if next == fingerprint {
			t.Errorf("change to %s was not noticed", change.path)
		}
		fingerprint = next
	}
	if err := os.Remove(filepath.Join(p.Dir, "resources/nested/asset.txt")); err != nil {
		t.Fatal(err)
	}
	if androidHostInputs(p) == fingerprint {
		t.Fatal("removing a packaged resource was not noticed")
	}
}
