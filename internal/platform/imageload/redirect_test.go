package imageload

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestCheckedImageRedirectsFollowAllowedAndRefuseUngranted(t *testing.T) {
	isolatedCache(t)

	var evilHits atomic.Int32
	evilSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilHits.Add(1)
		w.Write([]byte("evil-secret"))
	}))
	defer evilSrv.Close()

	var cdnHits atomic.Int32
	cdnSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cdnHits.Add(1)
		w.Write([]byte("cdn-ok"))
	}))
	defer cdnSrv.Close()

	apiSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/to-cdn":
			http.Redirect(w, r, cdnSrv.URL+"/pic.png", http.StatusFound)
		case "/to-evil":
			http.Redirect(w, r, evilSrv.URL+"/secret.png", http.StatusFound)
		case "/to-file":
			http.Redirect(w, r, "file:///etc/passwd", http.StatusFound)
		case "/loop":
			http.Redirect(w, r, "/loop", http.StatusFound)
		default:
			w.Write([]byte("api-ok"))
		}
	}))
	defer apiSrv.Close()

	hostOf := func(raw string) string {
		u, _ := url.Parse(raw)
		return u.Host
	}
	allowHosts := func(hosts ...string) func(string) error {
		return func(raw string) error {
			u, err := url.Parse(raw)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
				return core.AccessDenied(core.AccessNetwork, raw)
			}
			for _, h := range hosts {
				if u.Host == h {
					return nil
				}
			}
			return core.AccessDenied(core.AccessNetwork, raw)
		}
	}

	// Allowed redirect: api -> cdn succeeds when both are granted.
	srcAllowed := core.Src(apiSrv.URL + "/to-cdn")
	driver.SetImageSourceCheck(&srcAllowed, allowHosts(hostOf(apiSrv.URL), hostOf(cdnSrv.URL)))
	data, err := Load(context.Background(), srcAllowed, nil)
	if err != nil || string(data) != "cdn-ok" {
		t.Fatalf("allowed redirect = %q, %v; want cdn-ok", data, err)
	}
	if cdnHits.Load() != 1 {
		t.Fatalf("cdn hits = %d, want 1", cdnHits.Load())
	}

	// Cache re-validation: a stricter caller that only grants api (not cdn)
	// must NOT receive the cached bytes of api -> cdn.
	srcStricter := core.Src(apiSrv.URL + "/to-cdn")
	driver.SetImageSourceCheck(&srcStricter, allowHosts(hostOf(apiSrv.URL)))
	if data, err := Load(context.Background(), srcStricter, nil); err == nil || core.ErrorCode(err) != core.CodeAccessDenied {
		t.Fatalf("stricter caller on cached redirect = %q, %v; want ERR_ACCESS_DENIED", data, err)
	}

	// Denied redirect: api -> evil is refused without contacting evilSrv.
	srcEvil := core.Src(apiSrv.URL + "/to-evil")
	driver.SetImageSourceCheck(&srcEvil, allowHosts(hostOf(apiSrv.URL)))
	if data, err := Load(context.Background(), srcEvil, nil); err == nil || core.ErrorCode(err) != core.CodeAccessDenied {
		t.Fatalf("ungranted redirect = %q, %v; want ERR_ACCESS_DENIED", data, err)
	}
	if evilHits.Load() != 0 {
		t.Fatalf("ungranted redirect target was contacted %d times", evilHits.Load())
	}

	// Redirect to file:// scheme is refused.
	srcFile := core.Src(apiSrv.URL + "/to-file")
	driver.SetImageSourceCheck(&srcFile, allowHosts(hostOf(apiSrv.URL)))
	if data, err := Load(context.Background(), srcFile, nil); err == nil || core.ErrorCode(err) != core.CodeAccessDenied {
		t.Fatalf("redirect to file:// = %q, %v; want ERR_ACCESS_DENIED", data, err)
	}

	// Redirect loop stops cleanly.
	srcLoop := core.Src(apiSrv.URL + "/loop")
	driver.SetImageSourceCheck(&srcLoop, allowHosts(hostOf(apiSrv.URL)))
	if _, err := Load(context.Background(), srcLoop, nil); err == nil || !strings.Contains(err.Error(), "too many redirects") {
		t.Fatalf("redirect loop = %v, want too many redirects", err)
	}

	// Cache poisoning prevention: an unrestricted load of api -> evil caches
	// bytes with hopsVerified=false; a subsequent restricted caller must not
	// be served that entry and must fail at the redirect check.
	unrestricted := core.Src(apiSrv.URL + "/to-evil")
	if data, err := Load(context.Background(), unrestricted, nil); err != nil || string(data) != "evil-secret" {
		t.Fatalf("unrestricted load = %q, %v", data, err)
	}
	hitsBefore := evilHits.Load()
	if data, err := Load(context.Background(), srcEvil, nil); err == nil || core.ErrorCode(err) != core.CodeAccessDenied {
		t.Fatalf("restricted load after unrestricted cache fill = %q, %v; want ERR_ACCESS_DENIED", data, err)
	}
	if evilHits.Load() != hitsBefore {
		t.Fatalf("restricted load after cache fill contacted evil server (%d -> %d)", hitsBefore, evilHits.Load())
	}

	// Fail closed when backend has no HTTPNoRedirect client.
	savedNoRedirect := HTTPNoRedirect
	HTTPNoRedirect = nil
	defer func() { HTTPNoRedirect = savedNoRedirect }()
	srcNoClient := core.Src(apiSrv.URL + "/plain")
	driver.SetImageSourceCheck(&srcNoClient, allowHosts(hostOf(apiSrv.URL)))
	if _, err := Load(context.Background(), srcNoClient, nil); !errors.Is(err, errNoRedirectControl) {
		t.Fatalf("nil HTTPNoRedirect = %v, want %v", err, errNoRedirectControl)
	}
}

func TestCheckedLocalFileReverifiesBeforeRead(t *testing.T) {
	isolatedCache(t)
	base := t.TempDir()
	allowedDir := filepath.Join(base, "allowed")
	secretDir := filepath.Join(base, "secret")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(secretDir, 0o755); err != nil {
		t.Fatal(err)
	}
	goodFile := filepath.Join(allowedDir, "a.png")
	secretFile := filepath.Join(secretDir, "s.png")
	if err := os.WriteFile(goodFile, []byte("good-png"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secretFile, []byte("secret-png"), 0o644); err != nil {
		t.Fatal(err)
	}
	realAllowed, err := filepath.EvalSymlinks(allowedDir)
	if err != nil {
		t.Fatal(err)
	}
	win := runtime.GOOS == "windows"
	check := func(raw string) error {
		p, err := driver.ResolveLocalMediaPath(raw, win)
		if err != nil {
			return core.AccessDenied(core.AccessFS, raw)
		}
		rel, err := filepath.Rel(realAllowed, p)
		if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return core.AccessDenied(core.AccessFS, raw)
		}
		return nil
	}

	// Allowed local file loads cleanly.
	src := core.Src(driver.CanonicalFileURL(goodFile, win))
	driver.SetImageSourceCheck(&src, check)
	if data, err := Load(context.Background(), src, nil); err != nil || string(data) != "good-png" {
		t.Fatalf("allowed local file = %q, %v", data, err)
	}

	// Simulate TOCTOU: replace goodFile with a symlink pointing to secretFile
	// after core already validated goodFile. imageload must re-verify and refuse.
	if err := os.Remove(goodFile); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secretFile, goodFile); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if data, err := Load(context.Background(), src, nil); err == nil || core.ErrorCode(err) != core.CodeAccessDenied {
		t.Fatalf("symlink-swapped file = %q, %v; want ERR_ACCESS_DENIED", data, err)
	}
}
