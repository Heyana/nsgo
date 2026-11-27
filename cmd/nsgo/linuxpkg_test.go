package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	projectcfg "github.com/nsgo-dev/nsgo/project"
	"io"
	"strconv"
	"strings"
	"testing"
)

// TestArchMtreeMatchesArchive compares the manifest with the actual tar metadata and bytes, including metadata files, directories, Unicode, whitespace, backslashes and symbolic links.
func TestArchMtreeMatchesArchive(t *testing.T) {
	p := &project{Config: projectcfg.Config{Name: "App", ID: "dev.nsgo.test.mtree", Version: "1.0.0", URLSchemes: []string{"nsgo1410"}}}
	files := []linuxPackageEntry{
		{name: "opt", mode: 0o755, dir: true},
		{name: "opt/app", mode: 0o755, dir: true},
		{name: "opt/app/space #é\\name\tline\n.txt", data: []byte("a packaged asset\n"), mode: 0o640},
		{name: "opt/app/alias", link: "space #é\\name\tline\n.txt", mode: 0o777},
	}
	data, err := archPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	headers := map[string]*tar.Header{}
	contents := map[string][]byte{}
	for {
		h, err := archive.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(h.Name, "/")
		headers[name] = h
		contents[name], err = io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
	}
	manifest, err := gzip.NewReader(bytes.NewReader(contents[".MTREE"]))
	if err != nil {
		t.Fatal("package has no readable gzip .MTREE:", err)
	}
	mtree, err := io.ReadAll(manifest)
	if err != nil {
		t.Fatal(err)
	}
	manifest.Close()
	if !bytes.HasPrefix(mtree, []byte("#mtree\n")) {
		t.Fatal("missing mtree format marker")
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(mtree), "\n")[1:] {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := strings.TrimPrefix(decodeMtreeToken(t, fields[0]), "./")
		if name == ".MTREE" || seen[name] {
			t.Fatalf("unexpected manifest entry %q", name)
		}
		seen[name] = true
		h, ok := headers[name]
		if !ok {
			t.Fatalf("manifest entry %q is absent from tar", name)
		}
		attrs := map[string]string{}
		for _, field := range fields[1:] {
			k, v, ok := strings.Cut(field, "=")
			if !ok {
				t.Fatalf("invalid attribute %q", field)
			}
			attrs[k] = v
		}
		kind := "file"
		if h.Typeflag == tar.TypeDir {
			kind = "dir"
		} else if h.Typeflag == tar.TypeSymlink {
			kind = "link"
		}
		for key, want := range map[string]string{"type": kind, "uid": strconv.Itoa(h.Uid), "gid": strconv.Itoa(h.Gid), "mode": fmt.Sprintf("%o", h.Mode), "time": fmt.Sprintf("%d.0", h.ModTime.Unix())} {
			if got := attrs[key]; got != want {
				t.Errorf("%q %s=%q, tar has %q", name, key, got, want)
			}
		}
		if kind == "file" {
			if attrs["size"] != strconv.Itoa(len(contents[name])) || attrs["sha256digest"] != fmt.Sprintf("%x", sha256.Sum256(contents[name])) {
				t.Errorf("%q manifest does not describe its tar payload", name)
			}
		}
		if kind == "link" && decodeMtreeToken(t, attrs["link"]) != h.Linkname {
			t.Errorf("%q link target differs from tar", name)
		}
	}
	for name := range headers {
		if name != ".MTREE" && !seen[name] {
			t.Errorf("%q is absent from manifest", name)
		}
	}
	for _, name := range []string{".PKGINFO", ".INSTALL"} {
		if !seen[name] {
			t.Errorf("manifest lacks %s", name)
		}
	}
}

func decodeMtreeToken(t *testing.T, s string) string {
	t.Helper()
	var out strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out.WriteByte(s[i])
			continue
		}
		if i+3 >= len(s) {
			t.Fatalf("short octal escape in %q", s)
		}
		b, err := strconv.ParseUint(s[i+1:i+4], 8, 8)
		if err != nil {
			t.Fatal(err)
		}
		out.WriteByte(byte(b))
		i += 3
	}
	return out.String()
}

// Debian's filesystem tar reader rejects PAX headers, including ones Go's default format adds for Unicode names or long paths.
func TestDebPackageAcceptsUnicodeAndLongPaths(t *testing.T) {
	name := "opt/app/" + strings.Repeat("directory/", 30) + "space é.txt"
	target := strings.Repeat("../", 40) + "é.txt"
	files := []linuxPackageEntry{
		{name: name, data: []byte("asset"), mode: 0o644},
		{name: "opt/app/alias", link: target, mode: 0o777},
	}
	p := &project{Config: projectcfg.Config{Name: "App", ID: "dev.nsgo.test.deb", Version: "1.0.0"}}
	data, err := debPackage(p, files, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	members := arMembers(t, data)
	gz, err := gzip.NewReader(bytes.NewReader(members["data.tar.gz"]))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	archive := tar.NewReader(gz)
	for _, file := range files {
		h, err := archive.Next()
		if err != nil {
			t.Fatal(err)
		}
		if h.Format == tar.FormatPAX {
			t.Errorf("%q uses PAX, which dpkg rejects", h.Name)
		}
		if strings.TrimPrefix(h.Name, "./") != file.name || h.Linkname != file.link {
			t.Errorf("header changed filename or link: %+v", h)
		}
		payload, err := io.ReadAll(archive)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(payload, file.data) {
			t.Errorf("%q changed payload", file.name)
		}
	}
}
