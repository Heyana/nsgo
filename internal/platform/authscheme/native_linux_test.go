//go:build linux && !android

package authscheme_test

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func registerFixtureScheme(t *testing.T, scheme string) {
	t.Helper()
	dir, err := os.MkdirTemp("", "auth1399-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	for _, name := range []string{"config", "data/applications", "cache", "state", "runtime"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for key, name := range map[string]string{"XDG_CONFIG_HOME": "config", "XDG_DATA_HOME": "data", "XDG_CACHE_HOME": "cache", "XDG_STATE_HOME": "state", "XDG_RUNTIME_DIR": "runtime"} {
		t.Setenv(key, filepath.Join(dir, name))
	}
	t.Setenv("GIO_USE_VFS", "local")
	t.Setenv("GTK_USE_PORTAL", "0")
	t.Setenv("GDK_DEBUG", "no-portals")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(dir, "launch.log")
	launcher := filepath.Join(dir, "launch.sh")
	quote := func(s string) string { return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'" }
	script := "#!/bin/sh\nexec " + quote(executable) + " -test.run=^TestRegisteredSchemeAuthCallback$ -test.v -- \"$@\" >> " + quote(log) + " 2>&1\n"
	if err := os.WriteFile(launcher, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if t.Failed() {
			if output, err := os.ReadFile(log); err == nil {
				t.Logf("owned handler output:\n%s", output)
			}
		}
	})
	entry := "[Desktop Entry]\nType=Application\nName=NSGo synthetic scheme fixture\nNoDisplay=true\n" +
		fmt.Sprintf("Exec=/bin/sh \"%s\" %%u\n", launcher) +
		"MimeType=x-scheme-handler/" + scheme + ";x-scheme-handler/http;x-scheme-handler/https;\n"
	if err := os.WriteFile(filepath.Join(dir, "data/applications/nsgo-auth1399.desktop"), []byte(entry), 0o600); err != nil {
		t.Fatal(err)
	}
	defaults := "[Default Applications]\nx-scheme-handler/" + scheme + "=nsgo-auth1399.desktop\nx-scheme-handler/http=nsgo-auth1399.desktop\nx-scheme-handler/https=nsgo-auth1399.desktop\n"
	if err := os.WriteFile(filepath.Join(dir, "config/mimeapps.list"), []byte(defaults), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("update-desktop-database", filepath.Join(dir, "data/applications")).CombinedOutput(); err != nil {
		t.Fatalf("owned desktop registration: %v\n%s", err, out)
	}
	if out, err := exec.Command("xdg-mime", "query", "default", "x-scheme-handler/http").CombinedOutput(); err != nil || strings.TrimSpace(string(out)) != "nsgo-auth1399.desktop" {
		t.Fatalf("owned HTTP handler was not selected: %v, %s", err, out)
	}
}

func launchFixtureURL(uri string) error {
	if out, err := exec.Command("gio", "open", uri).CombinedOutput(); err != nil {
		return fmt.Errorf("owned URL dispatcher: %w: %s", err, out)
	}
	return nil
}

func fixtureProviderCallback(w http.ResponseWriter, r *http.Request, callback string, _ chan<- error) {
	http.Redirect(w, r, callback, http.StatusFound)
}
