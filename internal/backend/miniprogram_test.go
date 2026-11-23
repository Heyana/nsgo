package backend

import (
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"testing"
)

// TestMiniProgramBackendBuildGating verifies that mini-program builds select run_miniprogram.go and link the automation agent and live surface only with nsgodev and nsgoautomation, never in release builds.
func TestMiniProgramBackendBuildGating(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("needs the go command")
	}
	list := func(pkg, tags string) (files, deps []string) {
		t.Helper()
		cmd := exec.Command("go", "list", "-tags="+tags, "-json=GoFiles,Deps", pkg)
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0", "GOFLAGS=")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("go list -tags=%s %s: %v\n%s", tags, pkg, err, out)
		}
		var info struct {
			GoFiles []string
			Deps    []string
		}
		if err := json.Unmarshal(out, &info); err != nil {
			t.Fatal(err)
		}
		return info.GoFiles, info.Deps
	}

	// Release mini-program build: run_miniprogram.go selected, without the automation agent, net/http or nsgotest.
	files, deps := list(".", "nsgominiprogram,nsgo_noshader")
	if !slices.Contains(files, "run_miniprogram.go") {
		t.Fatalf("release miniprogram backend files = %v, missing run_miniprogram.go", files)
	}
	for _, forbidden := range []string{
		"github.com/nsgo-dev/nsgo/nsgotest",
		"github.com/nsgo-dev/nsgo/internal/automation",
		"net/http",
	} {
		if slices.Contains(deps, forbidden) {
			t.Errorf("release miniprogram backend unexpectedly depends on %s", forbidden)
		}
	}

	// Automation mini-program build: run_miniprogram.go selected, links the dev client and the automation agent.
	files, deps = list(".", "nsgominiprogram,nsgo_noshader,nsgodev,nsgoautomation")
	if !slices.Contains(files, "run_miniprogram.go") {
		t.Fatalf("automation miniprogram backend files = %v, missing run_miniprogram.go", files)
	}
	for _, required := range []string{
		"github.com/nsgo-dev/nsgo/internal/platform/miniprogram",
		"github.com/nsgo-dev/nsgo/internal/devtools",
		"github.com/nsgo-dev/nsgo/internal/automation",
		"github.com/nsgo-dev/nsgo/nsgotest",
	} {
		if !slices.Contains(deps, required) {
			t.Errorf("automation miniprogram backend missing dependency %s", required)
		}
	}
}
