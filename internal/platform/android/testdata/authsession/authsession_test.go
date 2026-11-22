package authsession_test

import (
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

const verifier = "nsgo1397-synthetic-verifier-owned-fixture-only-1234567890"
const state = "nsgo1397-owned-state"

type authResult struct {
	callback string
	err      error
}

func nativeAuth(t *testing.T, mode, scheme string) (*nsgotest.App, <-chan authResult, func(), *httptest.Server, *atomic.Int32, *atomic.Bool) {
	t.Helper()
	if nsgotest.Target() != "android" {
		t.Skip("requires a native Android browser and app-owned callback scheme")
	}
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	seen := make(chan struct{}, 1)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/authorize":
			q := r.URL.Query()
			if q.Get("state") != state || q.Get("code_challenge") != challenge || q.Get("code_challenge_method") != "S256" {
				http.Error(w, "invalid synthetic authorization request", http.StatusBadRequest)
				return
			}
			select {
			case seen <- struct{}{}:
			default:
			}
			if mode == "success" {
				callback := q.Get("redirect_uri") + "?" + url.Values{"code": {"owned-code"}, "state": {state}}.Encode()
				http.Redirect(w, r, callback, http.StatusFound)
			} else {
				fmt.Fprint(w, "<!doctype html><title>NSGo 1397 owned provider</title><h1>Owned authentication fixture</h1><p>Return to NSGo to cancel.</p>")
			}
		case "/token":
			if r.Method != http.MethodPost || r.ParseForm() != nil || r.Form.Get("code") != "owned-code" || r.Form.Get("code_verifier") != verifier {
				http.Error(w, "invalid synthetic token exchange", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, "owned-token")
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(provider.Close)
	answer := make(chan authResult, 2)
	calls := &atomic.Int32{}
	active := &atomic.Bool{}
	var cancel func()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { active.Store(cx.Env().Lifecycle() == core.LifecycleActive) })
		return core.View{TestID: "start", Role: core.RoleButton,
			Style:    core.Style{Width: core.Pt(320), Height: core.Pt(80), Background: core.RGB(0x335577)},
			Children: []core.Node{core.Text{Content: "Start owned authentication fixture"}},
			OnPress: func(core.PressEvent) {
				cancel = cx.AuthSession(func(redirect string) string {
					return provider.URL + "/authorize?" + url.Values{"redirect_uri": {redirect}, "state": {state},
						"code_challenge": {challenge}, "code_challenge_method": {"S256"}}.Encode()
				}, core.AuthSessionOptions{Scheme: scheme, Path: "/old/../回调"}, func(callback string, err error) {
					calls.Add(1)
					answer <- authResult{callback, err}
				})
			},
		}
	})
	app.Press("id:start")
	if strings.EqualFold(scheme, "dev.nsgo.auth1397") {
		select {
		case <-seen:
		case got := <-answer:
			t.Fatalf("the session ended before the actual browser visited the provider: %v", got.err)
		case <-time.After(45 * time.Second):
			t.Fatal("the native browser never reached the owned provider")
		}
	}
	return app, answer, func() { app.Post(func() { cancel() }); app.Settle() }, provider, calls, active
}

func TestAndroidAuthSessionCallbackAndPKCE(t *testing.T) {
	_, answer, _, provider, calls, _ := nativeAuth(t, "success", "DEV.NSGO.AUTH1397")
	select {
	case got := <-answer:
		if got.err != nil {
			t.Fatal(got.err)
		}
		callback, err := url.Parse(got.callback)
		if err != nil || callback.Scheme != "dev.nsgo.auth1397" || callback.Path != "/回调" || callback.Query().Get("state") != state || callback.Query().Get("code") != "owned-code" {
			t.Fatalf("incorrect synthetic callback %q: %v", got.callback, err)
		}
		response, err := http.PostForm(provider.URL+"/token", url.Values{"code": {callback.Query().Get("code")}, "code_verifier": {verifier}})
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusOK || calls.Load() != 1 {
			t.Fatalf("token status=%d callback count=%d", response.StatusCode, calls.Load())
		}
	case <-time.After(45 * time.Second):
		t.Fatal("the browser did not return its callback to the running Activity")
	}
}

func TestAndroidAuthSessionExplicitCancel(t *testing.T) {
	_, answer, cancel, _, calls, active := nativeAuth(t, "hold", "dev.nsgo.auth1397")
	deadline := time.Now().Add(5 * time.Second)
	for active.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if active.Load() {
		t.Fatal("the browser never took the Activity's focus")
	}
	cancel()
	select {
	case got := <-answer:
		if !errors.Is(got.err, core.AuthCancelled) || calls.Load() != 1 {
			t.Fatalf("cancel result=%v callback count=%d", got.err, calls.Load())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the explicit cancellation did not finish")
	}
	deadline = time.Now().Add(5 * time.Second)
	for !active.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !active.Load() {
		t.Fatal("native cancellation did not bring the Activity back")
	}
}

func TestAndroidAuthSessionBrowserBack(t *testing.T) {
	if os.Getenv("NSGO_AUTH_BROWSER_BACK") != "1" {
		t.Skip("requires an external controller to send Back to the owned browser page")
	}
	_, answer, _, _, calls, _ := nativeAuth(t, "hold", "dev.nsgo.auth1397")
	fmt.Fprintln(os.Stderr, "nsgo1397-await-browser-back")
	select {
	case got := <-answer:
		if !errors.Is(got.err, core.AuthCancelled) || calls.Load() != 1 {
			t.Fatalf("browser close result=%v callback count=%d", got.err, calls.Load())
		}
	case <-time.After(45 * time.Second):
		t.Fatal("the controller did not close the owned browser tab")
	}
}

func TestAndroidAuthSessionUnclaimedCallback(t *testing.T) {
	_, answer, _, _, calls, _ := nativeAuth(t, "hold", "dev.nsgo.unclaimed1397")
	select {
	case got := <-answer:
		if !errors.Is(got.err, core.AuthUnavailable) || !strings.Contains(got.err.Error(), "url_schemes") || calls.Load() != 1 {
			t.Fatalf("unclaimed callback result=%v count=%d", got.err, calls.Load())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("an unclaimed callback waited instead of failing")
	}
}
