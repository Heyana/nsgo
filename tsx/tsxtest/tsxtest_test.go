package tsxtest

import (
	"os"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/tsx"
)

func TestRunsTestFiles(t *testing.T) { Run(t, os.DirFS("testdata/app")) }

func TestRunsSteps(t *testing.T) {
	RunSteps(t, os.DirFS("testdata/app"), "App.tsx", `await app.press("id:count");
expect(app.shows("Pressed 1")).toBe(true);`)
}

func TestTestDeclarationsIncludeCameraPick(t *testing.T) {
	decl := tsx.Declarations()["types/nsgo/test.d.ts"]
	for _, want := range []string{
		"camera?: boolean | null;",
		"camera?: boolean",
		"onPick(handler: ((pick: Pick) => PickFile | PickFile[] | null) | null): void;",
		"updater?: UpdaterTestOptions | null;",
		"onUpdateCheck(handler: ((background: boolean) => UpdateCheckResult | null | undefined) | null): void;",
		"updateChecks(): UpdateCheckRecord[];",
		"restarts(): number;",
	} {
		if !strings.Contains(decl, want) {
			t.Errorf("test.d.ts missing %q", want)
		}
	}
}
