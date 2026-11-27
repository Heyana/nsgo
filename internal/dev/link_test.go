package dev

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// devClientSymbols are functions only the dev client has, which nsgo dev's builds link with the nsgodev tag; small ones the compiler inlines are left out, as they leave no symbol.
var devClientSymbols = []string{"internal/dev.startDevTools", "internal/dev.runDevCommand", "internal/dev.listenDevTools", "internal/dev.traceTo", "internal/dev.parseDevLayout", "internal/dev.receiveRuntimeProfile", "internal/dev.startDevState", "internal/dev.startDevReady"}

// devOverlaySymbols are functions of DevTools' overlay in core, the layout audit's among them, which only the dev client mounts (core.UpdateHighlights).
var devOverlaySymbols = []string{"core.(*Tree).mountHighlights", "core.(*Tree).auditBoxes", "core.paintAudit", "core.(*layoutAudit).readout", "(*Tree).keepAudit"}

// devCausalSymbols are causal-chain diagnostic functions in core, compiled only under nsgodev so release builds link none of them.
var devCausalSymbols = []string{"core.(*runtime).beginSignalWrite", "core.(*runtime).endCompRun", "core.(*runtime).beginExplicitDisposal", "core.(*runtime).endDisposal", "core.(*devSurface).Causes", "core.(*devSurface).Triggers", "core.(*devSurface).Disposals"}

// TestReleaseBuildsLinkNoDevClient builds an app as nsgo build does and as nsgo dev does and reads their symbols. DevTools' client, traces, runtime profiles and overlay link only into dev builds. Core's recording for DevTools links into both, behind devtools.Enabled.
func TestReleaseBuildsLinkNoDevClient(t *testing.T) {
	if testing.Short() {
		t.Skip("builds an app twice")
	}
	dir := t.TempDir()
	symbols := func(tags string) string {
		t.Helper()
		bin := filepath.Join(dir, "hello"+tags)
		args := []string{"build", "-o", bin}
		if tags != "" {
			args = append(args, "-tags", tags)
		}
		build := exec.Command("go", append(args, "../../examples/hello")...)
		build.Env = append(os.Environ(), "GOFLAGS=")
		if out, err := build.CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", tags, err, out)
		}
		out, err := exec.Command("go", "tool", "nm", bin).Output()
		if err != nil {
			t.Fatalf("go tool nm: %v", err)
		}
		return string(out)
	}
	release, dev := symbols(""), symbols("nsgodev")
	allSymbols := append(append(slices.Clone(devClientSymbols), devOverlaySymbols...), devCausalSymbols...)
	for _, s := range allSymbols {
		if strings.Contains(release, s) {
			t.Errorf("a release build links %s", s)
		}
		if !strings.Contains(dev, s) {
			t.Errorf("a dev build lacks %s, so this test reads the wrong names", s)
		}
	}
}
