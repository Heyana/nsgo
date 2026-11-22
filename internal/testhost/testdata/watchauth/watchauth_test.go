//go:build ios && nsgowatch && nsgotest

package watchauth_test

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// This opt-in integration test opens only a synthetic provider, never an
// account's authorization endpoint. The system may require accepting this
// fixture's consent sheet; never accept an unrelated sign-in request.
func TestWatchAuthSessionProviderAndCancel(t *testing.T) {
	if os.Getenv("NSGO_TEST_WATCH_AUTH") != "1" {
		t.Skip("set NSGO_TEST_WATCH_AUTH=1 on an isolated watch simulator")
	}
	switch nsgotest.Target() {
	case "watchos_simulator":
	default:
		t.Skip("requires the watchOS authentication backend")
	}
	verifier, state, code := rand.Text()+rand.Text(), rand.Text(), rand.Text()
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	var mu sync.Mutex
	var authorized, waiting, exchanged bool
	var registeredRedirect string
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		switch r.URL.Path {
		case "/authorize", "/wait":
			q := r.URL.Query()
			if q.Get("response_type") != "code" || q.Get("client_id") != "nsgo-native-test" || q.Get("state") != state ||
				q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") != challenge {
				http.Error(w, "invalid synthetic authorization request", http.StatusBadRequest)
				return
			}
			mu.Lock()
			registeredRedirect = q.Get("redirect_uri")
			if r.URL.Path == "/wait" {
				waiting = true
			} else {
				authorized = true
			}
			mu.Unlock()
			if r.URL.Path == "/wait" {
				fmt.Fprint(w, "<!doctype html><title>NSGo WatchAuth1400 test provider</title>Waiting for cancellation.")
				return
			}
			callback, err := url.Parse(q.Get("redirect_uri"))
			if err != nil {
				http.Error(w, "invalid redirect", http.StatusBadRequest)
				return
			}
			callback.RawQuery = url.Values{"code": {code}, "state": {state}}.Encode()
			http.Redirect(w, r, callback.String(), http.StatusFound)
		case "/token":
			_ = r.ParseForm()
			mu.Lock()
			valid := authorized && r.Method == "POST" && r.Form.Get("code") == code && r.Form.Get("code_verifier") == verifier &&
				r.Form.Get("redirect_uri") == registeredRedirect && r.Form.Get("grant_type") == "authorization_code"
			exchanged = valid
			mu.Unlock()
			if !valid {
				http.Error(w, "invalid synthetic code exchange", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, "synthetic exchange accepted")
		default:
			http.NotFound(w, r)
		}
	}))
	defer provider.Close()
	type result struct {
		callback string
		err      error
	}
	results := make(chan result, 4)
	var cancel func()
	var redirect string
	path := "/authorize"
	callbackPath := os.Getenv("NSGO_TEST_WATCH_AUTH_PATH")
	if callbackPath == "" {
		callbackPath = "/nsgo-watch-auth1400"
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return core.View{TestID: "sign-in", Style: core.Style{Width: core.Pt(200), Height: core.Pt(60)},
			OnPress: func(core.PressEvent) {
				cancel = cx.AuthSession(func(uri string) string {
					redirect = uri
					q := url.Values{"response_type": {"code"}, "client_id": {"nsgo-native-test"}, "redirect_uri": {uri},
						"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
					return provider.URL + path + "?" + q.Encode()
				}, core.AuthSessionOptions{Scheme: "nsgo-watch-auth1400", Path: callbackPath, Ephemeral: true, Timeout: 30 * time.Second},
					func(callback string, err error) { results <- result{callback, err} })
			}}
	}, nsgotest.Size(300, 200))
	app.Press("sign-in")
	app.WaitUntil(35*time.Second, func() bool { return len(results) != 0 })
	answer := <-results
	if answer.err != nil {
		t.Fatalf("native callback: %v", answer.err)
	}
	callback, err := url.Parse(answer.callback)
	if err != nil || callback.Query().Get("code") != code || callback.Query().Get("state") != state {
		t.Fatal("native callback lost the synthetic code or state")
	}
	if callback.Scheme != "nsgo-watch-auth1400" {
		t.Fatal("the watch session did not catch its requested callback scheme")
	}
	response, err := http.PostForm(provider.URL+"/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {redirect}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	mu.Lock()
	valid := exchanged
	mu.Unlock()
	if response.StatusCode != http.StatusOK || !valid {
		t.Fatal("synthetic PKCE exchange failed")
	}
	app.Post(func() { path = "/wait" })
	app.Press("sign-in")
	app.WaitUntil(20*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return waiting || len(results) != 0
	})
	if len(results) != 0 {
		t.Fatalf("session ended before cancellation: %v", (<-results).err)
	}
	app.Post(func() { cancel() })
	app.WaitUntil(5*time.Second, func() bool { return len(results) != 0 })
	if answer := <-results; !errors.Is(answer.err, core.AuthCancelled) || answer.callback != "" {
		t.Fatalf("native cancel: %v", answer.err)
	}
	app.Advance(300 * time.Millisecond)
	if len(results) != 0 {
		t.Fatal("cancelled session answered twice")
	}
}
