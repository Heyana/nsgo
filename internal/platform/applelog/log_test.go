//go:build darwin

package applelog

import (
	"bytes"
	"errors"
	"log"
	"os"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestWriterPreservesOutputAndText(t *testing.T) {
	var stderr bytes.Buffer
	var entries []string
	w := &writer{stderr: &stderr, mirror: true, emit: func(text string) { entries = append(entries, text) }}
	input := []byte("WARN 中文\x00after\xff\n")
	if n, err := w.Write(input); n != len(input) || err != nil {
		t.Fatalf("write = %d, %v", n, err)
	}
	if !bytes.Equal(stderr.Bytes(), input) || len(entries) != 1 || entries[0] != "WARN 中文\\0after\uFFFD" {
		t.Fatalf("stderr = %q, entries = %q", stderr.Bytes(), entries)
	}
	stderr.Reset()
	w.mirror = false
	w.Write([]byte("ERROR second\n"))
	if stderr.Len() != 0 || entries[1] != "ERROR second" {
		t.Fatalf("null stderr = %q, entries = %q", stderr.Bytes(), entries)
	}
}

func TestLongEntriesKeepUnicodeAndLines(t *testing.T) {
	for _, text := range []string{strings.Repeat("界", 800), strings.Repeat("x", 970), "first\n" + strings.Repeat("x", 970), ""} {
		var entries []string
		parts(text, func(part string) { entries = append(entries, part) })
		for _, entry := range entries {
			if len(entry) > entryBytes || !utf8.ValidString(entry) {
				t.Fatalf("invalid entry: %d bytes, valid UTF-8 %v", len(entry), utf8.ValidString(entry))
			}
		}
		if strings.Contains(text, "\n") {
			if entries[0] != "first" || strings.Join(entries[1:], "") != strings.TrimPrefix(text, "first\n") {
				t.Fatalf("line splitting lost text: %q", entries)
			}
		} else if strings.Join(entries, "") != text || len(entries) == 0 {
			t.Fatalf("splitting lost text: %q", entries)
		}
	}
}

func TestStderrDestinations(t *testing.T) {
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if goesSomewhere(null) {
		t.Fatal("/dev/null is a stderr destination")
	}
	file, err := os.CreateTemp(t.TempDir(), "logs")
	if err != nil {
		t.Fatal(err)
	}
	if !goesSomewhere(file) {
		t.Error("a file is not a stderr destination")
	}
	file.Close()
	if goesSomewhere(file) {
		t.Error("a closed file is a stderr destination")
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer read.Close()
	defer write.Close()
	if !goesSomewhere(write) {
		t.Error("a pipe is not a stderr destination")
	}
	for _, test := range []struct{ dev, dest, debugger, disabled, want bool }{
		{true, false, true, false, true}, {false, true, false, false, true},
		{false, false, false, false, false}, {false, true, true, false, false},
		{false, true, true, true, true},
	} {
		if mirrorStderr(test.dev, test.dest, test.debugger, test.disabled) != test.want {
			t.Errorf("stderr policy: %+v", test)
		}
	}
}

func TestCustomLoggerKeepsItsWriter(t *testing.T) {
	var out bytes.Buffer
	logger := log.New(&out, "custom ", 0)
	if install(logger, false) {
		t.Fatal("replaced a custom logger")
	}
	logger.Print("kept")
	if out.String() != "custom kept\n" {
		t.Fatalf("custom output = %q", out.String())
	}
}

func TestUnavailableNativeKeepsStderr(t *testing.T) {
	previous := system
	system = func() (*systemAPI, error) { return nil, errors.New("unavailable native formatter") }
	t.Cleanup(func() { system = previous })
	logger := log.New(os.Stderr, "", 0)
	if install(logger, false) || logger.Writer() != os.Stderr {
		t.Fatal("native setup failure replaced stderr")
	}
}
