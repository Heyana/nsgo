package harmony_test

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestHostSystemBarsAndCutout checks the ArkTS host's immersive handling, which no test runs: the cutout insets every side, since rotation can put a punch-hole at the bottom or a side, and the two system bar requests are made through their own BarRequest, so a device that refuses the navigation indicator does not make the bars again and hide them after an edge-swipe reveal.
func TestHostSystemBarsAndCutout(t *testing.T) {
	page := read(t, filepath.Join("templates", "entry", "src", "main", "ets", "pages", "Index.ets"))
	for _, side := range []string{"cut.topRect", "cut.leftRect", "cut.rightRect", "cut.bottomRect"} {
		if !strings.Contains(page, side) {
			t.Errorf("Index.ets no longer reads %s: that side of the cutout is no inset", side)
		}
	}
	for _, want := range []string{
		"this.bars.make(names.join(','), retry, () => w.setWindowSystemBarEnable(names))",
		"this.indicator.make(immersive ? 'hide' : 'show', retry, () => w.setSpecificSystemBarEnabled('navigationIndicator', !immersive))",
		"if (!this.tree.sbImmersive || this.bars.failed) {",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("Index.ets no longer has %q", want)
		}
	}
	if n := strings.Count(page, "setWindowSystemBarEnable("); n != 1 {
		t.Errorf("Index.ets calls setWindowSystemBarEnable %d times, want once, through its BarRequest", n)
	}
}
