//go:build windows

package authscheme_test

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

func registerFixtureScheme(t *testing.T, scheme string) {
	t.Helper()
	// An isolated runner can warm its own browser profile before the test. Keep that profile outside TempDir: the runner closes its browser before removing it, after the test's provider and native app have stopped.
	dir := os.Getenv("NSGO_TEST_AUTH_SCHEME_DATA")
	if dir == "" {
		dir = t.TempDir()
	}
	t.Setenv("APPDATA", filepath.Join(dir, "config"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "local"))
	path := `Software\Classes\` + scheme
	key, exists, err := registry.CreateKey(registry.CURRENT_USER, path, registry.ALL_ACCESS)
	if err != nil {
		t.Fatal(err)
	}
	if exists {
		key.Close()
		t.Fatal("owned fixture scheme already exists")
	}
	t.Cleanup(func() {
		key.Close()
		for _, suffix := range []string{`\shell\open\command`, `\shell\open`, `\shell`, ""} {
			if err := registry.DeleteKey(registry.CURRENT_USER, path+suffix); err != nil {
				t.Error(err)
			}
		}
	})
	if err := key.SetStringValue("", "URL:NSGo synthetic scheme fixture"); err != nil {
		t.Fatal(err)
	}
	if err := key.SetStringValue("URL Protocol", ""); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command, _, err := registry.CreateKey(key, `shell\open\command`, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	defer command.Close()
	if err := command.SetStringValue("", fmt.Sprintf(`"%s" -test.run=^TestRegisteredSchemeAuthCallback$ "%%1"`, executable)); err != nil {
		t.Fatal(err)
	}
}

func launchFixtureURL(uri string) error {
	open, _ := syscall.UTF16PtrFromString("open")
	address, err := syscall.UTF16PtrFromString(uri)
	if err != nil {
		return err
	}
	shell := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteW")
	result, _, _ := shell.Call(0, uintptr(unsafe.Pointer(open)), uintptr(unsafe.Pointer(address)), 0, 0, 1)
	if result <= 32 {
		return fmt.Errorf("owned URL dispatcher returned %d", result)
	}
	return nil
}

// The default browser still opens the real local provider URL. The provider invokes the OS's private-URI dispatcher directly, avoiding a browser-specific external-protocol consent decision or any default-browser/policy change.
func fixtureProviderCallback(w http.ResponseWriter, _ *http.Request, callback string, failures chan<- error) {
	fmt.Fprint(w, "<!doctype html><title>NSGo Auth1399 fixture</title>Synthetic callback sent through the OS URL dispatcher.")
	go func() {
		if err := launchFixtureURL(callback); err != nil {
			failures <- err
		}
	}()
}
