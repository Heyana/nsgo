//go:build darwin && !ios

package apple

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ebitengine/purego/objc"
)

func TestDirectoryBookmarks(t *testing.T) {
	dir := t.TempDir()
	track := filepath.Join(dir, "01 Prelude #1.flac")
	if err := os.WriteFile(track, []byte("fLaC-data"), 0o644); err != nil {
		t.Fatal(err)
	}
	url := objc.ID(Class("NSURL")).Send(sel("fileURLWithPath:isDirectory:"), NSString(dir), true)
	if url == 0 {
		t.Fatal("NSURL fileURLWithPath:isDirectory: returned nil")
	}
	token, ok := CreateDirectoryBookmark(url, true)
	if !ok || token == "" {
		t.Fatal("CreateDirectoryBookmark failed")
	}

	reopened, err := OpenDirectoryBookmark(token, true)
	if err != nil {
		t.Fatalf("OpenDirectoryBookmark: %v", err)
	}
	defer ReleaseDirectoryBookmark(reopened.Token)

	if reopened.Token == "" || reopened.Path == "" || reopened.Name == "" {
		t.Fatalf("reopened = %+v", reopened)
	}
	data, err := fs.ReadFile(reopened, "01 Prelude #1.flac")
	if err != nil || string(data) != "fLaC-data" {
		t.Fatalf("ReadFile from reopened bookmark = %q, %v", data, err)
	}
	uri, err := reopened.MediaURI("01 Prelude #1.flac")
	if err != nil || !strings.HasPrefix(uri, "file:///") || !strings.HasSuffix(uri, "01%20Prelude%20%231.flac") {
		t.Fatalf("MediaURI from reopened bookmark = %q, %v", uri, err)
	}

	// Releasing is idempotent.
	ReleaseDirectoryBookmark(reopened.Token)
	ReleaseDirectoryBookmark(reopened.Token)

	// Invalid token fails cleanly.
	if _, err := OpenDirectoryBookmark("not-a-valid-bookmark!!!", true); err == nil {
		t.Fatal("OpenDirectoryBookmark accepted invalid token")
	}
}
