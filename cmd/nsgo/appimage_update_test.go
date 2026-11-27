package main

import (
	"bytes"
	"context"
	"debug/elf"
	"encoding/binary"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppImageUpdateLocator(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "Hello World", Version: "1.2.0", Dir: t.TempDir()}}
	p.Releases.GitHub = "example/hello"
	for _, arch := range []string{"amd64", "arm64"} {
		got, err := p.appImageUpdate(context.Background(), arch)
		want := "gh-releases-zsync|example|hello|latest|hello-world-*-linux-" + goArchName(arch) + ".AppImage.zsync"
		if err != nil || got != want {
			t.Fatalf("%s locator = %q, %v; want %q", arch, got, err, want)
		}
		p.Version = "2.0.0"
		if next, _ := p.appImageUpdate(context.Background(), arch); next != got {
			t.Errorf("version changed the update locator: %q", next)
		}
	}
	custom := "zsync|https://example.invalid/latest.AppImage.zsync"
	p.Linux.AppImageUpdate = &custom
	if got, err := p.appImageUpdate(context.Background(), "amd64"); err != nil || got != custom {
		t.Fatalf("custom locator = %q, %v", got, err)
	}
	custom = ""
	if got, err := p.appImageUpdate(context.Background(), "amd64"); err != nil || got != "" {
		t.Fatalf("disabled locator = %q, %v", got, err)
	}
	p.Linux.AppImageUpdate, p.Releases.GitHub = nil, ""
	if got, err := p.appImageUpdate(context.Background(), "amd64"); err != nil || got != "" {
		t.Fatalf("locator without a release repository = %q, %v", got, err)
	}
}

func TestAppImageUpdateConfiguration(t *testing.T) {
	dir := t.TempDir()
	for _, value := range []string{
		"", "zsync|https://example.invalid/app.zsync",
		"gh-releases-zsync|owner|repo|latest-pre|hello-*-linux-x64.AppImage.zsync",
	} {
		body := "name: Hello\nlinux:\n  appimage_update: '" + value + "'\n"
		if err := os.WriteFile(filepath.Join(dir, "nsgo.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		p, err := load(dir)
		if err != nil || p.Linux.AppImageUpdate == nil || *p.Linux.AppImageUpdate != value {
			t.Fatalf("load update locator %q: project=%+v, error=%v", value, p, err)
		}
	}
	for _, value := range []string{
		"guess", "zsync|file:///tmp/app.zsync", "zsync|relative.zsync",
		"zsync|https://example.invalid/app.zsync#fragment",
		"zsync|https://example.invalid/a\nb", "zsync|https://example.invalid/a\x00b",
		"gh-releases-zsync|owner/repo|repo|latest|file", "gh-releases-zsync|owner|repo||file",
	} {
		if err := validateAppImageUpdate(value); err == nil {
			t.Errorf("accepted invalid locator %q", value)
		}
	}
}

// appImageELFFixture is an ELF64 file with a reserved section, generated here rather than importing an upstream runtime into a unit test.
func appImageELFFixture(machine elf.Machine, size int) []byte {
	names := []byte("\x00.shstrtab\x00.upd_info\x00")
	dataOffset := 64 + len(names)
	sections := dataOffset + size
	data := make([]byte, sections+3*64)
	copy(data, "\x7fELF\x02\x01\x01")
	copy(data[8:], "AI\x02")
	le := binary.LittleEndian
	le.PutUint16(data[16:], uint16(elf.ET_EXEC))
	le.PutUint16(data[18:], uint16(machine))
	le.PutUint32(data[20:], uint32(elf.EV_CURRENT))
	le.PutUint64(data[40:], uint64(sections))
	le.PutUint16(data[52:], 64)
	le.PutUint16(data[58:], 64)
	le.PutUint16(data[60:], 3)
	le.PutUint16(data[62:], 1)
	copy(data[64:], names)
	for i, section := range []struct {
		name, kind   uint32
		offset, size uint64
	}{
		{1, uint32(elf.SHT_STRTAB), 64, uint64(len(names))},
		{11, uint32(elf.SHT_PROGBITS), uint64(dataOffset), uint64(size)},
	} {
		header := data[sections+(i+1)*64:]
		le.PutUint32(header, section.name)
		le.PutUint32(header[4:], section.kind)
		le.PutUint64(header[24:], section.offset)
		le.PutUint64(header[32:], section.size)
		le.PutUint64(header[48:], 1)
	}
	return data
}

func TestAppImageUpdateELFSection(t *testing.T) {
	const info = "zsync|https://example.invalid/hello.AppImage.zsync"
	for _, machine := range []elf.Machine{elf.EM_X86_64, elf.EM_AARCH64} {
		runtime := appImageELFFixture(machine, 1024)
		before := bytes.Clone(runtime)
		if err := setAppImageUpdate(runtime, info); err != nil {
			t.Fatal(err)
		}
		f, err := elf.NewFile(bytes.NewReader(runtime))
		if err != nil {
			t.Fatal(err)
		}
		section := f.Section(".upd_info")
		data, err := section.Data()
		if err != nil || string(bytes.TrimRight(data, "\x00")) != info || data[len(info)] != 0 {
			t.Fatalf("section = %q, %v", data, err)
		}
		if !bytes.Equal(runtime[:section.Offset], before[:section.Offset]) || !bytes.Equal(runtime[section.Offset+section.Size:], before[section.Offset+section.Size:]) {
			t.Error("embedding metadata changed ELF layout or another section")
		}
		if err := setAppImageUpdate(runtime, ""); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(runtime, before) {
			t.Error("disabling update information left stale metadata")
		}
	}
	runtime := appImageELFFixture(elf.EM_X86_64, len(info))
	before := bytes.Clone(runtime)
	if err := setAppImageUpdate(runtime, info); err == nil || !strings.Contains(err.Error(), ".upd_info") || !bytes.Equal(runtime, before) {
		t.Fatalf("oversized metadata: error=%v, unchanged=%t", err, bytes.Equal(runtime, before))
	}
	if err := setAppImageUpdate([]byte("not an ELF"), info); err == nil {
		t.Error("accepted a malformed runtime")
	}
}
