package flatpak

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestVariantSerialization(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    value
		hex  string
	}{
		{"fixed tuple alignment", tuple(byteValue(1), uint32Value(2), byteValue(3)), "010000000200000003000000"},
		{"string tuple", tuple(stringValue("a"), stringValue("bc")), "610062630002"},
		{"string array", array(stringValue(""), stringValue("a"), stringValue("bc")), "61006263000205"},
		{"variant", variant(uint32Value(500)), "f40100000075"},
		{"dictionary", dictionary(dictionaryEntry("width", uint32Value(500))), "7769647468000000f40100000075060f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := hex.EncodeToString(tc.v.data); got != tc.hex {
				t.Fatalf("got %s, want %s", got, tc.hex)
			}
		})
	}
}

func TestBundleRejectsAmbiguousPaths(t *testing.T) {
	b := Bundle{ID: "dev.nsgo.Fixture", Arch: "x86_64", Branch: "stable", Metadata: "[Application]\nname=dev.nsgo.Fixture\n"}
	for _, entries := range [][]Entry{
		{{Name: "../outside"}}, {{Name: "files/../outside"}}, {{Name: "files/thing", Link: "../../outside"}},
		{{Name: "files/a"}, {Name: "files/a"}}, {{Name: "files/a"}, {Name: "files/a/b"}},
		{{Name: "metadata", Data: []byte(b.Metadata)}}, {{Name: "files/a", Dir: true}, {Name: "files/a"}},
	} {
		if _, err := Write(b, entries); err == nil {
			t.Fatalf("accepted %+v", entries)
		}
	}
}

func TestBundleIsDeterministicAndSelfContained(t *testing.T) {
	b, entries := fixtureBundle()
	data, err := Write(b, entries)
	if err != nil {
		t.Fatal(err)
	}
	again, err := Write(b, entries)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data, again) {
		t.Fatal("bundle is not deterministic")
	}
	if !bytes.HasPrefix(data, []byte("flatpak\x00")) || !bytes.Contains(data, []byte("NSGo Flatpak fixture")) {
		t.Fatal("missing Flatpak header or inline file content")
	}
	if dir := os.Getenv("NSGO_FLATPAK_FIXTURE_DIR"); dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fixture.flatpak"), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func fixtureBundle() (Bundle, []Entry) {
	b := Bundle{ID: "dev.nsgo.Fixture", Arch: "x86_64", Branch: "stable", Timestamp: 1,
		Metadata: "[Application]\nname=dev.nsgo.Fixture\nruntime=org.gnome.Platform/x86_64/51\ncommand=fixture\n\n[Context]\nshared=network;ipc;\nsockets=wayland;fallback-x11;pulseaudio;\ndevices=dri;\n"}
	entries := []Entry{
		{Name: "files/bin/fixture", Link: "../lib/fixture/fixture", Mode: 0o777},
		{Name: "files/lib/fixture/fixture", Data: []byte("#!/bin/sh\nprintf 'NSGo Flatpak fixture\\n'\n"), Mode: 0o755},
		{Name: "files/lib/fixture/empty", Mode: 0o644},
		{Name: "export/share/applications/dev.nsgo.Fixture.desktop", Data: []byte("[Desktop Entry]\nType=Application\nName=NSGo Fixture\nExec=fixture\nIcon=dev.nsgo.Fixture\n"), Mode: 0o644},
	}
	return b, entries
}
