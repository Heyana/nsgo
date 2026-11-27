package driver

import (
	"errors"
	"fmt"
	"net/url"
	"runtime"
	"strings"
)

// FilePathURL formats an absolute local path as a file: URL matching FileURLPath:
// file:///path on Unix, file:///C:/path for a Windows drive, and
// file://server/share/path for a Windows UNC share.
func FilePathURL(path string) string {
	return filePathURL(path, runtime.GOOS == "windows")
}

func filePathURL(path string, windows bool) string {
	if !windows {
		return (&url.URL{Scheme: "file", Path: path}).String()
	}
	p := strings.ReplaceAll(path, `\`, "/")
	if rest, ok := strings.CutPrefix(p, "//"); ok {
		host, tail, hasSlash := strings.Cut(rest, "/")
		if hasSlash {
			return (&url.URL{Scheme: "file", Host: host, Path: "/" + tail}).String()
		}
		return (&url.URL{Scheme: "file", Host: host, Path: "/"}).String()
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// FileURLPath is the local path a parsed file: URL names, for core.FileFromURL and the image loader alike. On Windows /C:/ names a drive, with no host or localhost, and any other host a UNC share (the share \\localhost\C$ is written file://localhost/C$); elsewhere only localhost may be its host. The path is the URL's decoded path, with backslashes on Windows.
func FileURLPath(u *url.URL, windows bool) (string, error) {
	if !strings.HasPrefix(u.Path, "/") {
		return "", errors.New("file URL has no absolute path")
	}
	local := u.Host == "" || strings.EqualFold(u.Host, "localhost")
	if !windows {
		if !local {
			return "", fmt.Errorf("file URL names a file on %s", u.Host)
		}
		return u.Path, nil
	}
	p := u.Path
	drive := len(p) >= 3 && p[2] == ':' && 'a' <= p[1]|0x20 && p[1]|0x20 <= 'z'
	switch {
	case drive && local:
		p = p[1:]
	case u.Host != "":
		p = "//" + u.Host + p
	}
	return strings.ReplaceAll(p, "/", `\`), nil
}
