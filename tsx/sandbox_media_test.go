//go:build !js

package tsx_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/bundler"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

func compileTSX(t *testing.T, source string) string {
	t.Helper()
	b, err := bundler.Compile(fstest.MapFS{"App.tsx": {Data: []byte(source)}}, "App.tsx")
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestSandboxedMediaUnderFSReadAndNetworkGrants(t *testing.T) {
	base := t.TempDir()
	allowedDir := filepath.Join(base, "allowed")
	outsideDir := filepath.Join(base, "outside")
	if err := os.MkdirAll(allowedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatal(err)
	}
	imgFile := filepath.Join(allowedDir, "photo.png")
	vidFile := filepath.Join(allowedDir, "clip.mp4")
	outsideFile := filepath.Join(outsideDir, "secret.png")
	for _, f := range []string{imgFile, vidFile, outsideFile} {
		if err := os.WriteFile(f, []byte("media-bytes"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	escapeLink := filepath.Join(allowedDir, "escape.png")
	_ = os.Symlink(outsideFile, escapeLink)

	win := runtime.GOOS == "windows"
	realImg, err := filepath.EvalSymlinks(imgFile)
	if err != nil {
		t.Fatal(err)
	}
	realVid, err := filepath.EvalSymlinks(vidFile)
	if err != nil {
		t.Fatal(err)
	}
	wantImgURL := driver.CanonicalFileURL(realImg, win)
	wantVidURL := driver.CanonicalFileURL(realVid, win)

	var evilHits atomic.Int32
	evilSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		evilHits.Add(1)
		w.Write([]byte("evil-img"))
	}))
	defer evilSrv.Close()

	allowedSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redirect-evil" {
			http.Redirect(w, r, evilSrv.URL+"/steal.png", http.StatusFound)
			return
		}
		w.Write([]byte("good-img"))
	}))
	defer allowedSrv.Close()

	allowedURL, _ := url.Parse(allowedSrv.URL)
	evilURL, _ := url.Parse(evilSrv.URL)

	// Configure a net/http HTTPNoRedirect for this test so imageload can
	// verify the redirect check attached by core.
	savedHTTP, savedNoRedir := imageload.HTTP, imageload.HTTPNoRedirect
	noRedirClient := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	imageload.HTTPNoRedirect = func(ctx context.Context, raw string, header imageload.Header, progress imageload.Progress) (*imageload.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
		if err != nil {
			return nil, err
		}
		resp, err := noRedirClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		body, err := imageload.ReadAll(resp.Body, resp.ContentLength, progress)
		if err != nil {
			return nil, err
		}
		return &imageload.Response{Status: resp.StatusCode, Header: imageload.Header(resp.Header), Body: body}, nil
	}
	t.Cleanup(func() { imageload.HTTP, imageload.HTTPNoRedirect = savedHTTP, savedNoRedir })

	bundle := compileTSX(t, fmt.Sprintf(`
import { mount, signal, View, Text, Image, Video } from "nsgo";
import { Menu } from "nsgo/desktop";

mount(() => {
  const denied = signal<string[]>([]);
  const addErr = (tag: string) => () => denied.set([...denied(), tag]);

  Menu.setApplicationMenu([
    { label: "Allowed", icon: %q },
    { label: "Denied", icon: %q },
    { label: "Escape", icon: %q },
  ]);

  return (
    <View>
      <Image testID="local-ok" src={%q} style={{ width: 20, height: 20 }} />
      <Image testID="local-bad" src={%q} style={{ width: 20, height: 20 }} onError={addErr("img-bad")} />
      <Image testID="local-esc" src={%q} style={{ width: 20, height: 20 }} onError={addErr("img-esc")} />
      <Image testID="net-redirect" src={%q} style={{ width: 20, height: 20 }} />
      <Video testID="vid-ok" src={%q} style={{ width: 20, height: 20 }} />
      <Video testID="vid-bad" src={%q} style={{ width: 20, height: 20 }} onError={addErr("vid-bad")} />
      <Text testID="errors">{denied().join(",")}</Text>
    </View>
  );
});
`, imgFile, outsideFile, escapeLink,
		imgFile, outsideFile, escapeLink,
		allowedSrv.URL+"/redirect-evil",
		vidFile, outsideFile))

	caps := &pkg.Capabilities{
		FS: &pkg.FS{Read: []string{allowedDir}},
		Network: &pkg.Network{
			HTTP: []pkg.HTTPRule{
				{Host: allowedURL.Hostname(), Port: mustPort(t, allowedURL)},
			},
		},
	}

	app := nsgotest.Start(t, tsx.Sandboxed("media-plugin", bundle, tsx.Sandbox{
		Capabilities:  caps,
		Modules:       []string{"nsgo/desktop"},
		WebUnisolated: true,
	}), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	app.Settle()
	app.WaitFor("id:errors", 2*time.Second)
	errText := app.Find("id:errors").Text
	for _, want := range []string{"img-bad", "img-esc", "vid-bad"} {
		if !strings.Contains(errText, want) {
			t.Errorf("errors = %q, missing %q", errText, want)
		}
	}

	if got := app.Find("id:local-ok").Image.Source.URI; got != wantImgURL {
		t.Errorf("local-ok URI = %q, want %q", got, wantImgURL)
	}
	if got := app.Find("id:local-bad").Image.Source.URI; got != "" {
		t.Errorf("local-bad URI = %q, want empty", got)
	}
	if got := app.Find("id:local-esc").Image.Source.URI; got != "" {
		t.Errorf("local-esc URI = %q, want empty", got)
	}
	if got := app.Find("id:vid-ok").Video.Source; got != wantVidURL {
		t.Errorf("vid-ok URI = %q, want %q", got, wantVidURL)
	}
	if got := app.Find("id:vid-bad").Video.Source; got != "" {
		t.Errorf("vid-bad URI = %q, want empty", got)
	}

	// Menu item icons: allowed kept as canonical file:/// URL, denied & escape stripped.
	if len(app.Menu) != 3 {
		t.Fatalf("menu items = %d, want 3", len(app.Menu))
	}
	if app.Menu[0].Icon.Source.URI != wantImgURL {
		t.Errorf("menu[0] icon URI = %q, want %q", app.Menu[0].Icon.Source.URI, wantImgURL)
	}
	if !app.Menu[1].Icon.Empty() || !app.Menu[2].Icon.Empty() {
		t.Errorf("denied menu icons leaked: [1]=%+v [2]=%+v", app.Menu[1].Icon, app.Menu[2].Icon)
	}

	// Loading the mounted ImageSource through imageload must check the redirect
	// against the package's network grant and refuse redirecting to evilSrv.
	redirSrc := app.Find("id:net-redirect").Image.Source
	if driver.ImageSourceCheck(redirSrc) == nil {
		t.Fatal("net-redirect ImageSource has no redirect check attached")
	}
	if data, err := imageload.Load(context.Background(), redirSrc, nil); err == nil || core.ErrorCode(err) != core.CodeAccessDenied {
		t.Fatalf("redirect to ungranted host (%s) = %q, %v; want ERR_ACCESS_DENIED", evilURL.Host, data, err)
	}
	if evilHits.Load() != 0 {
		t.Fatalf("evil server was contacted %d times", evilHits.Load())
	}

	// Loading the mounted local-ok ImageSource through imageload reads the real bytes.
	localSrc := app.Find("id:local-ok").Image.Source
	if data, err := imageload.Load(context.Background(), localSrc, nil); err != nil || string(data) != "media-bytes" {
		t.Fatalf("local-ok imageload.Load = %q, %v; want media-bytes", data, err)
	}
}

func mustPort(t *testing.T, u *url.URL) int {
	t.Helper()
	var p int
	if _, err := fmt.Sscanf(u.Port(), "%d", &p); err != nil || p == 0 {
		t.Fatalf("invalid port in %q", u.String())
	}
	return p
}
