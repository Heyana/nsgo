//go:build windows || (linux && !android)

package authscheme_test

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
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/engine"
)

const schemeFixtureID = "dev.nsgo.authscheme1399"

// This fixture uses real scheme registration, process launch and single-instance delivery. The provider, code, state and verifier are all local and synthetic.
func TestRegisteredSchemeAuthCallback(t *testing.T) {
	for _, raw := range engine.LaunchLinks() {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if strings.HasPrefix(u.Scheme, "com.nsgo.auth1399.p") {
			// The registered command starts this same executable. Run must forward its launch to the waiting owner and return, not open a second engine or window.
			nsgo.Run(func(cx *core.Scope) core.Node {
				t.Error("callback started a second engine")
				cx.Quit()
				return core.View{}
			})
			return
		}
		if u.Scheme == "http" && u.Query().Get("client_id") == "nsgo-auth1399" {
			fixtureBrowser(t, raw)
			return
		}
	}
	if os.Getenv("NSGO_TEST_AUTH_SCHEME_NATIVE") != "1" {
		t.Skip("requires the opt-in isolated native scheme fixture")
	}
	if core.App().ID != schemeFixtureID {
		t.Fatal("compile the fixture with -ldflags '-X github.com/nsgo-dev/nsgo/core.appID=dev.nsgo.authscheme1399'")
	}
	scheme := fmt.Sprintf("com.nsgo.auth1399.p%d", os.Getpid())
	expectedRedirect := scheme + ":/%E6%88%BB%E3%82%8B/%252F"
	registerFixtureScheme(t, scheme)
	verifier, state, code := rand.Text()+rand.Text(), rand.Text(), rand.Text()
	hash := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(hash[:])
	requested := make(chan string, 4)
	var requests atomic.Int32
	launchErrors := make(chan error, 4)
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Cache-Control", "no-store")
		if r.URL.Path == "/token" {
			_ = r.ParseForm()
			if r.Form.Get("code") != code || r.Form.Get("code_verifier") != verifier || r.Form.Get("grant_type") != "authorization_code" || r.Form.Get("redirect_uri") != expectedRedirect {
				http.Error(w, "invalid synthetic exchange", http.StatusBadRequest)
				return
			}
			fmt.Fprint(w, "synthetic exchange accepted")
			return
		}
		q := r.URL.Query()
		if q.Get("client_id") != "nsgo-auth1399" || q.Get("response_type") != "code" || q.Get("state") != state || q.Get("code_challenge") != challenge || q.Get("code_challenge_method") != "S256" {
			http.Error(w, "invalid synthetic authorization", http.StatusBadRequest)
			return
		}
		requested <- r.URL.Path
		if r.URL.Path == "/wait" {
			fmt.Fprint(w, "<!doctype html><title>NSGo Auth1399 fixture</title>Waiting for app cancellation.")
			return
		}
		callback, err := url.Parse(q.Get("redirect_uri"))
		if err != nil || callback.Scheme != scheme || q.Get("redirect_uri") != expectedRedirect {
			launchErrors <- fmt.Errorf("prepared callback URI %q, expected %q", q.Get("redirect_uri"), expectedRedirect)
			http.Error(w, "the platform did not prepare the requested private scheme", http.StatusBadRequest)
			return
		}
		callback.RawQuery = url.Values{"code": {code}, "state": {state}}.Encode()
		fixtureProviderCallback(w, r, callback.String(), launchErrors)
	}))
	defer provider.Close()
	type result struct {
		callback string
		err      error
	}
	results := make(chan result, 4)
	ordinary := make(chan []string, 4)
	ready, stopped := make(chan *core.Scope, 1), make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(stopped)
		nsgo.Run(func(cx *core.Scope) core.Node {
			cx.OnOpenLinks(func(links []string) { ordinary <- links })
			ready <- cx
			return core.Text{Content: "NSGo synthetic scheme callback fixture"}
		}, nsgo.WindowSize(360, 100))
	}()
	var scope *core.Scope
	select {
	case scope = <-ready:
	case <-time.After(10 * time.Second):
		stack := make([]byte, 64<<10)
		t.Logf("native startup stacks:\n%s", stack[:runtime.Stack(stack, true)])
		t.Fatal("the native app did not start")
	}
	t.Cleanup(func() {
		scope.Post(scope.Quit)
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Error("the native app did not stop")
		}
	})
	var cancel func()
	start := func(path string) {
		scope.Post(func() {
			cancel = scope.AuthSession(func(redirect string) string {
				q := url.Values{"client_id": {"nsgo-auth1399"}, "response_type": {"code"}, "redirect_uri": {redirect}, "state": {state},
					"code_challenge": {challenge}, "code_challenge_method": {"S256"}}
				return provider.URL + path + "?" + q.Encode()
			}, core.AuthSessionOptions{Scheme: scheme, Path: "/before/../戻る/%2F", Timeout: 25 * time.Second},
				func(uri string, err error) { results <- result{uri, err} })
		})
	}
	start("/authorize")
	var answer result
	select {
	case answer = <-results:
	case err := <-launchErrors:
		t.Fatal(err)
	case <-time.After(30 * time.Second):
		t.Fatal("the OS callback did not reach the running app")
	}
	callback, err := url.Parse(answer.callback)
	if answer.err != nil || err != nil || callback.Scheme != scheme || callback.Path != "/戻る/%2F" || callback.Query().Get("code") != code || callback.Query().Get("state") != state {
		unclaimed := "none"
		select {
		case links := <-ordinary:
			if len(links) != 0 {
				if link, err := url.Parse(links[0]); err == nil {
					unclaimed = link.Scheme + ":" + link.EscapedPath()
				}
			}
		default:
		}
		t.Fatalf("native scheme callback failed: %v (provider requests %d, validated authorizations %d, ordinary URI %s)", answer.err, requests.Load(), len(requested), unclaimed)
	}
	response, err := http.PostForm(provider.URL+"/token", url.Values{"grant_type": {"authorization_code"}, "code": {code}, "code_verifier": {verifier}, "redirect_uri": {expectedRedirect}})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatal("the synthetic PKCE exchange failed")
	}
	start("/wait")
	for waiting := false; !waiting; {
		select {
		case path := <-requested:
			waiting = path == "/wait"
		case <-time.After(20 * time.Second):
			t.Fatal("the cancellation session did not reach the provider")
		}
	}
	scope.Post(func() { cancel() })
	select {
	case answer = <-results:
		if !errors.Is(answer.err, core.AuthCancelled) {
			t.Fatalf("native scheme cancellation: %v", answer.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the session did not cancel")
	}
	if err := launchFixtureURL(answerURL(scheme)); err != nil {
		t.Fatal(err)
	}
	select {
	case links := <-ordinary:
		if len(links) != 1 || links[0] != answerURL(scheme) {
			t.Fatal("a late callback did not remain an ordinary link")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the late callback did not reach OnOpenLinks")
	}
}

func answerURL(scheme string) string { return scheme + ":/%E6%88%BB%E3%82%8B/%252F?state=synthetic" }

// Linux's isolated process-local HTTP handler is a small test browser. It follows the provider's redirect through the real OS URL dispatcher.
func fixtureBrowser(t *testing.T, raw string) {
	t.Helper()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Get(raw)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, response.Body)
	response.Body.Close()
	if response.StatusCode >= 400 {
		t.Fatalf("synthetic provider returned HTTP %d", response.StatusCode)
	}
	if location := response.Header.Get("Location"); location != "" {
		t.Log("following the synthetic provider's redirect through the OS dispatcher")
		if err := launchFixtureURL(location); err != nil {
			t.Fatal(err)
		}
	}
}
