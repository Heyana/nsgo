package android

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// TestEditMenuProtocolMatchesDriver checks that Protocol.java's edit menu item,
// event and action bit constants match internal/driver.
func TestEditMenuProtocolMatchesDriver(t *testing.T) {
	testhost.RequireSource(t)

	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`static final int (\w+) = (\d+);`).FindAllStringSubmatch(string(java), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for name, want := range map[string]int{
		"ITEM_EDIT_MENU":    int(driver.ItemEditMenu),
		"EVENT_EDIT_ACTION": int(driver.EventEditAction),
		"EDIT_CUT":          int(driver.EditCut),
		"EDIT_COPY":         int(driver.EditCopy),
		"EDIT_PASTE":        int(driver.EditPaste),
		"EDIT_SELECT_ALL":   int(driver.EditSelectAll),
	} {
		if got, ok := values[name]; !ok || got != want {
			t.Errorf("Protocol.java %s = %d (found %v), want %d", name, got, ok, want)
		}
	}
}
