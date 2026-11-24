package knownfolder

import (
	"path/filepath"
	"runtime"
	"testing"
)

func TestPath(t *testing.T) {
	const downloads = "374DE290-123F-4565-9164-39C4925E467B"
	got := Path(downloads)
	if runtime.GOOS != "windows" {
		if got != "" {
			t.Errorf("Path off Windows = %q, want none", got)
		}
		return
	}
	t.Logf("Downloads: %q", got)
	if !filepath.IsAbs(got) {
		t.Errorf("Path(Downloads) = %q, not absolute", got)
	}
	for _, bad := range []string{"", "not-a-guid", "374DE290-123F-4565-9164-39C4925E467", "ZZZZZZZZ-123F-4565-9164-39C4925E467B"} {
		if got := Path(bad); got != "" {
			t.Errorf("Path(%q) = %q, want none", bad, got)
		}
	}
}
