//go:build linux && !android

package linux

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestLinuxSharePlan(t *testing.T) {
	dir := t.TempDir()
	stage := filepath.Join(dir, "stage")

	// Standalone web or mailto URI goes through OpenURI.OpenURI.
	for _, u := range []string{"https://example.com/path?q=1", "  http://example.com  ", "mailto:hi@example.com"} {
		plan, err := prepareLinuxShare(driver.ShareRequest{Text: u}, stage)
		if err != nil {
			t.Fatalf("prepareLinuxShare(%q): %v", u, err)
		}
		if plan.kind != linuxShareURI || plan.uri != linuxShareURIForText(u) || len(plan.paths) != 0 {
			t.Fatalf("plan(%q) = %+v", u, plan)
		}
	}

	// Plain text is staged as share.txt for OpenURI.OpenFile / FileManager1 fallback.
	plan, err := prepareLinuxShare(driver.ShareRequest{Text: "hello linux share"}, stage)
	if err != nil {
		t.Fatalf("prepareLinuxShare plain text: %v", err)
	}
	if plan.kind != linuxShareSingleFile || len(plan.paths) != 1 || filepath.Base(plan.paths[0]) != "share.txt" {
		t.Fatalf("plain text plan = %+v", plan)
	}
	if b, err := os.ReadFile(plan.paths[0]); err != nil || string(b) != "hello linux share" {
		t.Fatalf("staged text = %q, %v", b, err)
	}

	// Single file (disk or in-memory) chooses linuxShareSingleFile.
	a := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(a, []byte("A"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err = prepareLinuxShare(driver.ShareRequest{
		Files: []driver.SharedFile{{Path: a, Name: "Notes.txt", MIME: "text/plain"}},
	}, stage)
	if err != nil {
		t.Fatalf("single file: %v", err)
	}
	if plan.kind != linuxShareSingleFile || len(plan.paths) != 1 || filepath.Base(plan.paths[0]) != "Notes.txt" {
		t.Fatalf("single file plan = %+v", plan)
	}

	// Multiple files choose linuxShareMultiFiles for FileManager1.ShowItems.
	plan, err = prepareLinuxShare(driver.ShareRequest{
		Files: []driver.SharedFile{
			{Path: a, Name: "a.txt", MIME: "text/plain"},
			{Data: []byte("B"), Name: "b.txt", MIME: "text/plain"},
		},
	}, stage)
	if err != nil {
		t.Fatalf("multi file: %v", err)
	}
	if plan.kind != linuxShareMultiFiles || len(plan.paths) != 2 || plan.paths[0] != a || filepath.Base(plan.paths[1]) != "b.txt" {
		t.Fatalf("multi file plan = %+v", plan)
	}
}

func TestLinuxShareURIsAndOutcomes(t *testing.T) {
	for _, tc := range []struct {
		in, want string
	}{
		{"https://example.com", "https://example.com"},
		{"mailto:user@example.com", "mailto:user@example.com"},
		{"file:///etc/passwd", ""},
		{"javascript:alert(1)", ""},
		{"hello https://example.com", ""},
		{"mailto:", ""},
		{"", ""},
	} {
		if got := linuxShareURIForText(tc.in); got != tc.want {
			t.Errorf("linuxShareURIForText(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}

	if got := linuxFileURI("/tmp/hello world/a#1.txt"); got != "file:///tmp/hello%20world/a%231.txt" {
		t.Fatalf("linuxFileURI = %q", got)
	}

	if got := linuxSharePortalOutcome(0, true); got != shareOutcomeShown {
		t.Errorf("code 0 = %v, want shown", got)
	}
	if got := linuxSharePortalOutcome(1, true); got != shareOutcomeCancelled {
		t.Errorf("code 1 = %v, want cancelled", got)
	}
	if got := linuxSharePortalOutcome(2, true); got != shareOutcomeFallback {
		t.Errorf("code 2 with files = %v, want fallback", got)
	}
	if got := linuxSharePortalOutcome(2, false); got != shareOutcomeUnavailable {
		t.Errorf("code 2 without files = %v, want unavailable", got)
	}
}
