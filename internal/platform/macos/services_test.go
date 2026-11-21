//go:build darwin && !ios

package macos

import (
	"os"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestSavePanelStartFolder(t *testing.T) {
	dir := t.TempDir()
	var initial string
	onMain(func() {
		loadAppKit()
		objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		panel := savePanel(driver.SaveRequest{Name: "report.txt", Start: dir})
		initial = apple.GoString(panel.Send(sel("directoryURL")).Send(sel("path")))
	})
	got, err := os.Stat(initial)
	want, wantErr := os.Stat(dir)
	if err != nil || wantErr != nil || !os.SameFile(got, want) {
		t.Fatalf("save panel starts in %q, want %q", initial, dir)
	}
}
