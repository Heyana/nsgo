package authenticode

import (
	"archive/zip"
	"bytes"
	"encoding/asn1"
	"encoding/binary"
	"fmt"
	"io"
	"strings"
	"testing"
)

func TestContentValueKeepsShortAndLongDERBodies(t *testing.T) {
	for _, n := range []int{0, 1, 127, 128, 255, 256, 65536} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			body := bytes.Repeat([]byte{0x42}, n)
			encoded := der(0x30, body)
			if got := contentValue(encoded); !bytes.Equal(got, body) {
				t.Fatalf("content length %d, want %d", len(got), len(body))
			}
		})
	}
}

func TestMSIXZIP64KeepsPayloadAndComment(t *testing.T) {
	var original bytes.Buffer
	w := zip.NewWriter(&original)
	if err := w.SetComment("package comment"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first.txt", "empty.txt", "last.txt"} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if name != "empty.txt" {
			if _, err := f.Write([]byte(name)); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	offset, _, _, _, err := msixDirectory(original.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	data, err := msixZIP64(original.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	end := len(data) - 22 - len("package comment")
	if got := binary.LittleEndian.Uint16(data[end+4:]); got != 0xffff {
		t.Fatalf("classic disk number = %#x, want ZIP64 sentinel", got)
	}
	if got := binary.LittleEndian.Uint16(data[end+6:]); got != 0xffff {
		t.Fatalf("classic directory disk = %#x, want ZIP64 sentinel", got)
	}
	if !bytes.Equal(data[:offset], original.Bytes()[:offset]) {
		t.Fatal("ZIP64 conversion changed local records")
	}
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if r.Comment != "package comment" || len(r.File) != 3 {
		t.Fatal("ZIP64 conversion changed comment or file count")
	}
	for _, f := range r.File {
		v, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(v)
		v.Close()
		if err != nil {
			t.Fatal(err)
		}
		want := f.Name
		if f.Name == "empty.txt" {
			want = ""
		}
		if string(b) != want {
			t.Errorf("%s: %q", f.Name, b)
		}
	}
	again, err := msixZIP64(data)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(again, data) {
		t.Fatal("ZIP64 normalization is not stable")
	}
}

func TestMSIXRejectsMalformedArchive(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("not a zip"), make([]byte, 22)} {
		if _, err := SignMSIX(data, nil, nil); err == nil {
			t.Fatal("signed an invalid package")
		}
		if _, _, _, _, err := msixDirectory(data); err == nil {
			t.Fatal("accepted invalid ZIP end records")
		}
	}
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for _, name := range []string{"AppxManifest.xml", "appxmanifest.xml"} {
		f, err := w.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		f.Write([]byte("manifest"))
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := SignMSIX(b.Bytes(), nil, nil); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate part: %v", err)
	}
}

func TestDERIntegerIncludesZeroOctet(t *testing.T) {
	for _, want := range []int{0, 1, 127, 128, 255, 256, 65536} {
		var got int
		rest, err := asn1.Unmarshal(smallInt(want), &got)
		if err != nil || len(rest) != 0 || got != want {
			t.Fatalf("integer %d: got %d, rest %x, error %v", want, got, rest, err)
		}
	}
}
