package driver

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"
)

// SetImageSourceCheck attaches a per-source access validator to src, called by
// imageload for HTTP redirect hops and local file re-verification. Core sets it
// for sources owned by a restricted Scope; unrestricted sources leave it nil.
func SetImageSourceCheck(src *ImageSource, check func(url string) error) {
	if src != nil {
		src.sourceCheck = check
	}
}

// ImageSourceCheck returns the per-source access validator attached by core, or
// nil for unrestricted sources.
func ImageSourceCheck(src ImageSource) func(url string) error {
	return src.sourceCheck
}

// CanonicalHTTPURL parses and canonicalizes an http or https URL so the exact
// string validated against grants is the string backends load: rejects spaces,
// control characters, backslashes before the query/fragment, opaque forms,
// missing hosts and invalid ports; lowercases scheme and host; strips default
// ports (:80, :443); resolves "." and ".." path segments; and clears RawPath.
func CanonicalHTTPURL(raw string) (string, error) {
	if raw == "" {
		return "", errors.New("empty URL")
	}
	if strings.IndexFunc(raw, func(r rune) bool { return unicode.IsSpace(r) || unicode.IsControl(r) }) >= 0 {
		return "", errors.New("URL contains whitespace or control characters")
	}
	beforeQuery := raw
	if i := strings.IndexAny(beforeQuery, "?#"); i >= 0 {
		beforeQuery = beforeQuery[:i]
	}
	if strings.ContainsRune(beforeQuery, '\\') {
		return "", errors.New("URL contains backslash")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	if u.Opaque != "" {
		return "", errors.New("opaque HTTP URL")
	}
	scheme := strings.ToLower(u.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return "", errors.New("URL has no host")
	}
	port := u.Port()
	if port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return "", fmt.Errorf("invalid port %q", port)
		}
		if (scheme == "http" && n == 80) || (scheme == "https" && n == 443) {
			port = ""
		}
	}
	formattedHost := host
	if strings.ContainsRune(host, ':') {
		formattedHost = "[" + host + "]"
	}
	if port != "" {
		formattedHost += ":" + port
	}
	u.Scheme = scheme
	u.Host = formattedHost
	path := u.Path
	if path == "" {
		path = "/"
	}
	u.Path = RemoveDotSegments(path)
	u.RawPath = ""
	return u.String(), nil
}

// RemoveDotSegments resolves "." and ".." segments in a URL path per RFC 3986
// so prefix/path grant checks cannot be bypassed via traversal segments.
func RemoveDotSegments(p string) string {
	if !strings.Contains(p, ".") {
		return p
	}
	var out []string
	segs := strings.Split(p, "/")
	for i, s := range segs {
		last := i == len(segs)-1
		switch s {
		case ".":
			if last {
				out = append(out, "")
			}
		case "..":
			if len(out) > 1 {
				out = out[:len(out)-1]
			}
			if last {
				out = append(out, "")
			}
		default:
			out = append(out, s)
		}
	}
	res := strings.Join(out, "/")
	if !strings.HasPrefix(res, "/") {
		res = "/" + res
	}
	return res
}

// CanonicalFileURL encodes an absolute local filesystem path as a canonical
// file:/// URL.
func CanonicalFileURL(absPath string, windows bool) string {
	slash := filepath.ToSlash(absPath)
	if windows && !strings.HasPrefix(slash, "/") {
		slash = "/" + slash
	}
	return (&url.URL{Scheme: "file", Path: slash}).String()
}

// ResolveLocalMediaPath turns a file: URL or bare absolute local path into its
// symlink-resolved canonical filesystem path, verifying that it names an
// existing regular file on the local machine. Remote hosts (non-empty and not
// localhost), relative paths, query/fragment/userinfo components, control
// characters, broken symlinks, directories and special files are refused.
func ResolveLocalMediaPath(raw string, windows bool) (string, error) {
	if raw == "" {
		return "", errors.New("empty path")
	}
	if strings.IndexFunc(raw, func(r rune) bool { return unicode.IsControl(r) }) >= 0 {
		return "", errors.New("path contains control characters")
	}
	var p string
	if HasSchemePrefix(raw, "file") {
		if strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
			return "", errors.New("file URL contains spaces")
		}
		u, err := url.Parse(raw)
		if err != nil {
			return "", err
		}
		if u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
			return "", errors.New("file URL has unsupported components")
		}
		if u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			return "", fmt.Errorf("file URL names a remote host %q", u.Host)
		}
		p, err = FileURLPath(u, windows)
		if err != nil {
			return "", err
		}
	} else {
		p = raw
	}
	if !filepath.IsAbs(p) {
		return "", errors.New("path is not absolute")
	}
	resolved, err := filepath.EvalSymlinks(p)
	if err != nil {
		return "", err
	}
	resolved = filepath.Clean(resolved)
	info, err := os.Stat(resolved)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("%q is not a regular file", resolved)
	}
	return resolved, nil
}

// HasSchemePrefix reports whether raw starts with scheme followed by ":" (case-insensitive).
func HasSchemePrefix(raw, scheme string) bool {
	return len(raw) > len(scheme) && raw[len(scheme)] == ':' && strings.EqualFold(raw[:len(scheme)], scheme)
}
