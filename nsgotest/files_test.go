package nsgotest_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
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
			app.SaveFile = func(r driver.SaveRequest) string {
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
