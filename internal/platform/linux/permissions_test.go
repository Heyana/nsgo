//go:build linux && !android

package linux

import (
	"os"
	"testing"
)

func TestCurrentLinuxSandboxEnv(t *testing.T) {
	if _, err := os.Stat("/.flatpak-info"); err == nil {
		sb := currentLinuxSandbox("dev.nsgo.test")
		if sb.kind != linuxSandboxFlatpak {
			t.Fatalf("running in Flatpak, got kind %v", sb.kind)
		}
		return
	}
	t.Setenv("SNAP", "")
	t.Setenv("SNAP_NAME", "")
	if sb := currentLinuxSandbox("dev.nsgo.test"); sb.kind != linuxSandboxNone {
		t.Fatalf("unsandboxed got %+v", sb)
	}
	t.Setenv("SNAP", "/snap/gallery/12")
	t.Setenv("SNAP_NAME", "gallery")
	sb := currentLinuxSandbox("dev.nsgo.test")
	if sb.kind != linuxSandboxSnap || sb.appID != "snap.gallery" {
		t.Fatalf("snap got %+v", sb)
	}
}
