package nsgotest_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestSaveFilePreflight(t *testing.T) {
	refusal := errors.New("the app owns this file")
	for _, test := range []struct {
		name  string
		err   error
		bytes string
	}{
		{name: "approved", bytes: "new file"},
		{name: "refused", err: refusal, bytes: "old file"},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "report.txt")
			if err := os.WriteFile(path, []byte("old file"), 0o600); err != nil {
				t.Fatal(err)
			}
			var scope *core.Scope
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				scope = cx
				return core.View{}
			})
			app.SaveFile = func(r nsgotest.FileSave) string {
				if !r.Preflight {
					t.Error("the save request did not ask for a preflight check")
				}
				return path
			}
			checks, completions := 0, 0
			app.Post(func() {
				scope.SaveFile(core.SaveOptions{
					Name: "report.txt", Data: []byte("new file"),
					Accept: func(chosen string) error {
						checks++
						if b, err := os.ReadFile(chosen); chosen != path || err != nil || string(b) != "old file" {
							t.Errorf("preflight target %q holds %q, %v", chosen, b, err)
						}
						return test.err
					},
				}, func(saved bool, chosen string, err error) {
					completions++
					if saved != (test.err == nil) || chosen != path || err != test.err {
						t.Errorf("save answered %v, %q, %v", saved, chosen, err)
					}
				})
			})
			app.Settle()
			if checks != 1 || completions != 1 {
				t.Errorf("ran %d checks and answered %d times", checks, completions)
			}
			if b, err := os.ReadFile(path); err != nil || string(b) != test.bytes {
				t.Errorf("saved file holds %q, %v", b, err)
			}
		})
	}
}

func TestSaveFilePreflightCancellationAndWriteFailure(t *testing.T) {
	for _, test := range []struct {
		name      string
		choose    func(*testing.T) string
		checks    int
		wantError bool
	}{
		{name: "cancelled", choose: func(*testing.T) string { return "" }},
		{
			name: "write failure",
			choose: func(t *testing.T) string {
				return filepath.Join(t.TempDir(), "missing", "report.txt")
			},
			checks: 1, wantError: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := test.choose(t)
			var scope *core.Scope
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				scope = cx
				return core.View{}
			})
			app.SaveFile = func(nsgotest.FileSave) string { return path }
			checks, completions := 0, 0
			app.Post(func() {
				scope.SaveFile(core.SaveOptions{
					Name: "report.txt", Data: []byte("new file"),
					Accept: func(chosen string) error {
						checks++
						if chosen != path {
							t.Errorf("preflight target %q, want %q", chosen, path)
						}
						return nil
					},
				}, func(saved bool, chosen string, err error) {
					completions++
					if saved || chosen != "" || (err != nil) != test.wantError {
						t.Errorf("save answered %v, %q, %v", saved, chosen, err)
					}
				})
			})
			app.Settle()
			if checks != test.checks || completions != 1 {
				t.Errorf("ran %d checks and answered %d times, want %d checks and one answer", checks, completions, test.checks)
			}
		})
	}
}
