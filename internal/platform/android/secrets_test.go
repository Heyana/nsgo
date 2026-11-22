package android

import (
	"io/fs"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestSecretsJava exercises Secrets.java on the JVM across native NSGo
// AES-GCM secrets and expo-secure-store migration (modern AES-GCM, legacy
// unprefixed AES-GCM, hybrid RSA+AES-GCM, isolation, cleanup and errors),
// and decodes a wire reply through Go's reader.
func TestSecretsJava(t *testing.T) {
	javac, java := androidJDK(t)
	classes := t.TempDir()
	args := []string{
		"-d", classes,
		filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"),
		filepath.Join("java", "dev", "oneo", "nsgo", "Secrets.java"),
	}
	if err := filepath.WalkDir(filepath.Join("testdata", "secrets"), func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(path, ".java") {
			args = append(args, path)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.SecretsCheck").CombinedOutput(); err != nil {
		t.Fatalf("SecretsCheck: %v\n%s", err, out)
	}

	out, err := exec.CommandContext(t.Context(), java, "-cp", classes, "dev.oneo.nsgo.SecretsCheck", "wire-expo").Output()
	if err != nil {
		t.Fatalf("SecretsCheck wire-expo: %v", err)
	}
	r := reader{out}
	id := r.u32()
	errText := r.str()
	found := r.byte() != 0
	value := string(r.bytes())
	if len(r.buf) != 0 {
		t.Fatalf("%d trailing bytes in secret reply", len(r.buf))
	}
	if id != 42 || errText != "" || !found || value != "wire-token-日本語" {
		t.Fatalf("wire reply = (id=%d, err=%q, found=%v, value=%q), want (42, \"\", true, \"wire-token-日本語\")", id, errText, found, value)
	}
}
