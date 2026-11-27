//go:build !js

package library

import (
	"debug/elf"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestFindInEmbeddedHostBundle(t *testing.T) {
	for _, layout := range []struct{ name, executable, library string }{
		{"ios", "Host.app/Host", "Host.app/Frameworks/GhosttyVT.framework/GhosttyVT"},
		{"tvos", "TVHost.app/TVHost", "TVHost.app/Frameworks/GhosttyVT.framework/GhosttyVT"},
		{"catalyst", "Host.app/Contents/MacOS/Host", "Host.app/Contents/Frameworks/GhosttyVT.framework/GhosttyVT"},
	} {
		t.Run(layout.name, func(t *testing.T) {
			root := t.TempDir()
			library := filepath.Join(root, layout.library)
			if err := os.MkdirAll(filepath.Dir(library), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(library, []byte("fixture"), 0o755); err != nil {
				t.Fatal(err)
			}
			if got := findInApp(filepath.Join(root, layout.executable)); got != library {
				t.Errorf("findInApp = %q, want %q", got, library)
			}
		})
	}
}

func TestFindInAndroidNativeLibraryDir(t *testing.T) {
	root := t.TempDir()
	libDir := filepath.Join(root, "lib", "arm64")
	exe := filepath.Join(libDir, "libnsgo.so")
	lib := filepath.Join(libDir, "libghostty-vt.so")
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lib, []byte("fixture"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findInAppWithName(exe, "libghostty-vt.so"); got != lib {
		t.Fatalf("findInAppWithName = %q, want %q", got, lib)
	}
	if got := findInAppWithName(exe, ""); got != "" {
		t.Fatalf("findInAppWithName with empty name = %q, want empty", got)
	}
}

func TestAndroidArm64PrebuildELF(t *testing.T) {
	path := filepath.Join("lib", "android-arm64", "libghostty-vt.so")
	f, err := elf.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	if f.Class != elf.ELFCLASS64 || f.Data != elf.ELFDATA2LSB || f.Machine != elf.EM_AARCH64 || f.Type != elf.ET_DYN {
		t.Fatalf("unexpected ELF header: class=%v data=%v machine=%v type=%v", f.Class, f.Data, f.Machine, f.Type)
	}
	for _, p := range f.Progs {
		if p.Type == elf.PT_INTERP {
			t.Fatal("shared library unexpectedly carries PT_INTERP")
		}
	}
	needed, err := f.DynString(elf.DT_NEEDED)
	if err != nil {
		t.Fatalf("DT_NEEDED: %v", err)
	}
	allowed := []string{"libc.so", "libm.so", "libdl.so"}
	for _, lib := range needed {
		if !slices.Contains(allowed, lib) {
			t.Errorf("unexpected DT_NEEDED %q (allowed bionic libs: %v)", lib, allowed)
		}
	}
	if !slices.Contains(needed, "libc.so") {
		t.Errorf("DT_NEEDED %v missing libc.so", needed)
	}
	syms, err := f.DynamicSymbols()
	if err != nil {
		t.Fatalf("DynamicSymbols: %v", err)
	}
	exported := map[string]bool{}
	for _, s := range syms {
		if s.Section != elf.SHN_UNDEF && s.Name != "" {
			exported[s.Name] = true
		}
	}
	for _, want := range []string{
		"ghostty_terminal_new",
		"ghostty_terminal_free",
		"ghostty_terminal_vt_write",
		"ghostty_render_state_new",
		"ghostty_type_json",
	} {
		if !exported[want] {
			t.Errorf("android-arm64 libghostty-vt.so does not export %s", want)
		}
	}
}
