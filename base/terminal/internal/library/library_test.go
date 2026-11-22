package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFindInEmbeddedHostBundle(t *testing.T) {
	for _, layout := range []struct{ name, executable, library string }{
		{"ios", "Host.app/Host", "Host.app/Frameworks/GhosttyVT.framework/GhosttyVT"},
		{"catalyst", "Host.app/Contents/MacOS/Host", "Host.app/Contents/Frameworks/GhosttyVT.framework/GhosttyVT"},
	} {
		t.Run(layout.name, func(t *testing.T) {
			root := t.TempDir()
			library := filepath.Join(root, layout.library)
			if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(library, []byte("fixture"), 0o755); err != nil {
				t.Fatal(err)
			}
			if got := findInApp(filepath.Join(root, layout.executable)); got != library {
				t.Errorf("findInApp = %q, want %q", got, library)
			}
		})
	}
}
