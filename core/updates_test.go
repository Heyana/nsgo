package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestInstallUpdate(t *testing.T) {
	for _, tc := range []struct {
		name    string
		os      OS
		file    string
		reply   driver.Event
		wantErr string
	}{
		{"linux installs", OSLinux, "hello-1.1.deb", driver.Event{Finished: true}, ""},
		{"linux install fails", OSLinux, "hello-1.1.deb", driver.Event{Text: "exit status 1"}, "exit status 1"},
		{"android opens installer", OSAndroid, "hello-1.1.apk", driver.Event{Finished: true}, ""},
		{"android installer does not start", OSAndroid, "hello-1.1.apk", driver.Event{}, "core: update installer did not start"},
		{"unavailable", OSMacOS, "hello-1.1.pkg", driver.NoPackageInstaller(0), driver.NoInstaller},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var cx *Scope
			h := newHost(t, Size{Width: 100, Height: 100}, func(c *Scope) Node { cx = c; return View{} })
			h.tree.SetEnvironment(Environment{OS: tc.os})
			h.commit()
			pkg := filepath.Join(t.TempDir(), tc.file)
			if err := os.WriteFile(pkg, []byte("package"), 0o600); err != nil {
				t.Fatal(err)
			}

			calls := 0
			var err error
			InstallUpdate(cx, pkg, func(e error) { calls++; err = e })
			it := command(h.commit(), driver.ItemInstallPackage)
			if it == nil || it.Open.Path != pkg || it.Open.Name != tc.file {
				t.Fatalf("install request = %+v", it)
			}
			if calls != 0 {
				t.Fatalf("callback ran before the backend answered")
			}
			reply := tc.reply
			reply.Kind, reply.Count = driver.EventFiles, it.Count
			h.tree.Dispatch(reply)
			h.tree.Dispatch(reply)
			if calls != 1 {
				t.Fatalf("callback ran %d times, want 1", calls)
			}
			if tc.wantErr == "" && err != nil || tc.wantErr != "" && (err == nil || err.Error() != tc.wantErr) {
				t.Errorf("install reply = %v, want %q", err, tc.wantErr)
			}
		})
	}
}
