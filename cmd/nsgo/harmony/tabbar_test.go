package harmony_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHostWiresNativeTabBarAndClipping checks that Index.ets declares the
// native view kind, renders core.TabBar with ArkUI Tabs/TabContent/Badge,
// reports tab taps back as "tabselect", and applies Overflow: Hidden clipping.
func TestHostWiresNativeTabBarAndClipping(t *testing.T) {
	path := filepath.Join("..", "..", "..", "cmd", "nsgo", "harmony", "templates", "entry", "src", "main", "ets", "pages", "Index.ets")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	src := string(data)
	for _, want := range []string{
		"const KIND_NATIVE: number = 10;",
		"code === 'tb'",
		"struct TabIconView",
		"struct TabBarView",
		"Tabs({ barPosition: this.barPos(), index: this.tabsIndex(), controller: this.controller })",
		"TabContent()",
		"Badge({",
		"host.send('tabselect', this.vid, index, 0, '')",
		"wrapBuilder(tabBarBuilder)",
		"c.clip(v.clip)",
	} {
		if !strings.Contains(src, want) {
			t.Errorf("Index.ets missing %q", want)
		}
	}
}
