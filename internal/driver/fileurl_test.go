package driver

import (
	"net/url"
	"testing"
)

func TestFileURLPath(t *testing.T) {
	for _, test := range []struct {
		raw     string
		windows bool
		want    string
	}{
		{"file:///C:/dir/a%20b.txt", true, `C:\dir\a b.txt`},
		{"file:///c:/dir/x", true, `c:\dir\x`},
		{"file:///d:/a%20b/%E6%97%A5%E6%9C%AC%23%25.png", true, `d:\a b\日本#%.png`},
		{"file:///a:/x", true, `a:\x`},
		{"file:///1:/x", true, `\1:\x`},
		{"file://localhost/C:/dir/x", true, `C:\dir\x`},
		{"file://LOCALHOST/c:/a.png", true, `c:\a.png`},
		{"file://server/share/x.txt", true, `\\server\share\x.txt`},
		{"file://server/share/a%20b.png", true, `\\server\share\a b.png`},
		{"file:////server/share/x.txt", true, `\\server\share\x.txt`},
		{"file://localhost/C$/Users/x.txt", true, `\\localhost\C$\Users\x.txt`},
		{"file:///tmp/a%20b.png", true, `\tmp\a b.png`},
		{"file:///home/me/a%20b%23c.txt", false, "/home/me/a b#c.txt"},
		{"file:///d:/a%20b/%E6%97%A5%E6%9C%AC%23%25.png", false, "/d:/a b/日本#%.png"},
		{"file:///C:/Users/me/poster.jpg", false, "/C:/Users/me/poster.jpg"},
		{"file://LOCALHOST/home/me/x", false, "/home/me/x"},
		{"file:////server/share/a.png", false, "//server/share/a.png"},
	} {
		u, err := url.Parse(test.raw)
		if err != nil {
			t.Fatal(err)
		}
		got, err := FileURLPath(u, test.windows)
		if err != nil {
			t.Errorf("FileURLPath(%q, %v): %v", test.raw, test.windows, err)
			continue
		}
		if got != test.want {
			t.Errorf("FileURLPath(%q, %v) = %q, want %q", test.raw, test.windows, got, test.want)
		}
	}
	for _, bad := range []struct {
		raw     string
		windows bool
	}{
		{"file://server/share/x.txt", false},
		{"file:relative/x.txt", true},
		{"file:relative/x.txt", false},
	} {
		u, err := url.Parse(bad.raw)
		if err != nil {
			t.Fatal(err)
		}
		if p, err := FileURLPath(u, bad.windows); err == nil {
			t.Errorf("FileURLPath(%q, %v) = %q, want an error", bad.raw, bad.windows, p)
		}
	}
}

func TestFilePathURL(t *testing.T) {
	for _, test := range []struct {
		path    string
		windows bool
		want    string
	}{
		{"/Users/me/Music/01 Aria #1.flac", false, "file:///Users/me/Music/01%20Aria%20%231.flac"},
		{"/tmp/日本語/song 100%.mp3", false, "file:///tmp/%E6%97%A5%E6%9C%AC%E8%AA%9E/song%20100%25.mp3"},
		{`C:\dir\a b.flac`, true, "file:///C:/dir/a%20b.flac"},
		{`d:\a b\日本#%.flac`, true, "file:///d:/a%20b/%E6%97%A5%E6%9C%AC%23%25.flac"},
		{`\\server\share\a b.flac`, true, "file://server/share/a%20b.flac"},
	} {
		got := filePathURL(test.path, test.windows)
		if got != test.want {
			t.Errorf("filePathURL(%q, %v) = %q, want %q", test.path, test.windows, got, test.want)
			continue
		}
		u, err := url.Parse(got)
		if err != nil {
			t.Fatalf("url.Parse(%q): %v", got, err)
		}
		back, err := FileURLPath(u, test.windows)
		if err != nil || back != test.path {
			t.Errorf("round-trip FileURLPath(%q, %v) = %q, %v; want %q", got, test.windows, back, err, test.path)
		}
	}
}
