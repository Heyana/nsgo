package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAndroidClassFiles(t *testing.T) {
	p := &project{Dir: t.TempDir()}
	first, second, third, raw := classTestDex(1), classTestDex(2), classTestDex(3), classTestDex(4)
	jar := filepath.Join(p.Dir, "widget.jar")
	writeClassTestJar(t, jar, []classTestEntry{
		{"classes10.dex", third},
		{"classes.dex", first},
		{"resources/classes.dex", []byte("not an APK shard")},
		{"classes2.dex", second},
	})
	dex := filepath.Join(p.Dir, "helper.dex")
	if err := os.WriteFile(dex, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	p.Android.Classes = []string{"widget.jar", dex}
	files, err := androidClassFiles(p)
	if err != nil {
		t.Fatal(err)
	}
	want := []classTestEntry{{"classes2.dex", first}, {"classes3.dex", second}, {"classes4.dex", third}, {"classes5.dex", raw}}
	if len(files) != len(want) {
		t.Fatalf("got %d DEX files, want %d", len(files), len(want))
	}
	for i, f := range files {
		if f.Name != want[i].name || !bytes.Equal(f.Data, want[i].data) {
			t.Errorf("DEX %d: got %q and data %x, want %q and data %x", i, f.Name, f.Data, want[i].name, want[i].data)
		}
	}
}

func TestAndroidClassFilesRejectInvalidInputs(t *testing.T) {
	p := &project{Dir: t.TempDir()}
	writeClassTestJar(t, filepath.Join(p.Dir, "java.jar"), []classTestEntry{{"Badge.class", []byte{0xca, 0xfe, 0xba, 0xbe}}})
	writeClassTestJar(t, filepath.Join(p.Dir, "duplicate.jar"), []classTestEntry{
		{"classes.dex", classTestDex(1)}, {"classes.dex", classTestDex(2)},
	})
	writeClassTestJar(t, filepath.Join(p.Dir, "malformed.jar"), []classTestEntry{{"classes.dex", []byte("bad")}})
	newer := classTestDex(0)
	copy(newer[4:7], "040")
	if err := os.WriteFile(filepath.Join(p.Dir, "recent.dex"), newer, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ file, message string }{
		{"java.jar", "compile Java or Kotlin bytecode with D8"},
		{"duplicate.jar", "duplicate classes.dex"},
		{"malformed.jar", "classes.dex: invalid DEX header"},
		{"recent.dex", "unsupported on Android API 28"},

		{"widget.aar", "want a precompiled .dex"},
	} {
		t.Run(tc.file, func(t *testing.T) {
			p.Android.Classes = []string{tc.file}
			_, err := androidClassFiles(p)
			if err == nil || !strings.Contains(err.Error(), tc.message) || !strings.Contains(err.Error(), "android.classes") {
				t.Fatalf("got %v, want an android.classes error containing %q", err, tc.message)
			}
		})
	}
	p.Android.Classes = []string{"missing.dex"}
	if _, err := androidClassFiles(p); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("missing DEX file returned %v, want os.ErrNotExist", err)
	}
}

type classTestEntry struct {
	name string
	data []byte
}

func writeClassTestJar(t *testing.T, path string, entries []classTestEntry) {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	for _, entry := range entries {
		w, err := z.Create(entry.name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write(entry.data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
}

func classTestDex(tag byte) []byte {
	data := make([]byte, 0x71)
	copy(data, "dex\n039\x00")
	binary.LittleEndian.PutUint32(data[32:36], uint32(len(data)))
	binary.LittleEndian.PutUint32(data[36:40], 0x70)
	binary.LittleEndian.PutUint32(data[40:44], 0x12345678)
	data[len(data)-1] = tag
	return data
}
