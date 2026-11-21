package harmony

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommittedHostMatchesTemplates(t *testing.T) {
	stamp, err := os.ReadFile("host-templates.sha256")
	if err != nil {
		t.Fatal(err)
	}
	files, err := walk(templates, "templates")
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.New()
	for _, name := range files {
		file := path.Join("templates", name)
		data, err := templates.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		io.WriteString(hash, file+"\x00")
		hash.Write(data)
		io.WriteString(hash, "\x00")
	}
	if got, want := strings.TrimSpace(string(stamp)), hex.EncodeToString(hash.Sum(nil)); got != want {
		t.Fatalf("HarmonyOS templates changed since the committed host was built. Run cmd/nsgo/harmony/host/build.sh with the API 26 SDK.")
	}
}

// sdkAvailable says the machine lacks the SDK when hvigorw is missing or no
// SDK path is set.
func TestSDKAvailableNeedsSDK(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("OHOS_BASE_SDK_HOME", "")
	err := sdkAvailable()
	if !errors.Is(err, ErrNoSDK) {
		t.Fatalf("sdkAvailable() = %v, want ErrNoSDK", err)
	}
}

// hostKey names a host by its bundle and the templates, so two ids differ and
// one id repeats.
func TestHostKey(t *testing.T) {
	a, err := hostKey("com.example.one")
	if err != nil {
		t.Fatal(err)
	}
	b, err := hostKey("com.example.one")
	if err != nil {
		t.Fatal(err)
	}
	c, err := hostKey("com.example.two")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("hostKey is not stable: %s != %s", a, b)
	}
	if a == c {
		t.Error("two bundles share a host key")
	}
	if len(a) != 32 {
		t.Errorf("hostKey length %d, want 32 hex digits", len(a))
	}
}

// materialize fills the host's placeholders and adds the page hvigor needs.
func TestMaterialize(t *testing.T) {
	dir := t.TempDir()
	if err := materialize(dir, "com.example.test"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"build-profile.json5", "entry/src/main/module.json5", "AppScope/app.json5"} {
		data, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(string(data), "{{") {
			t.Errorf("%s keeps a placeholder: %s", name, data)
		}
	}
	app, err := os.ReadFile(filepath.Join(dir, "AppScope", "app.json5"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(app), "com.example.test") {
		t.Errorf("app.json5 lacks the bundle: %s", app)
	}
	if _, err := os.Stat(filepath.Join(dir, "entry", "src", "main", "resources", "rawfile", "web", "index.html")); err != nil {
		t.Errorf("materialize did not write the page: %v", err)
	}
}

// A cache directory counts as a host only when PackageWithHost can read it.
func TestHostComplete(t *testing.T) {
	dir := t.TempDir()
	if hostComplete(dir) {
		t.Fatal("an empty directory counts as a host")
	}
	for _, name := range []string{"module.json", "pack.info", "ets/modules.abc"} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if !hostComplete(dir) {
		t.Fatal("a complete directory is not a host")
	}
}

// CompileHost reports ErrNoSDK before it builds anything when the machine has
// no SDK.
func TestCompileHostWithoutSDK(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("OHOS_BASE_SDK_HOME", "")
	if _, err := CompileHost(context.Background(), "com.example.none"); !errors.Is(err, ErrNoSDK) {
		t.Fatalf("CompileHost() = %v, want ErrNoSDK", err)
	}
}
