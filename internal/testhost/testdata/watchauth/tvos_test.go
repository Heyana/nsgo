//go:build ios && nsgotest && !nsgowatch

package watchauth_test

import (
	"errors"
	"sync"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestTVAuthSessionUnavailable(t *testing.T) {
	if nsgotest.Target() != "tvos_simulator" {
		t.Skip("checks the native TV rejection")
	}
	var mu sync.Mutex
	var urlCalled bool
	var replies int
	var failure error
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.AuthSession(func(string) string {
			mu.Lock()
			urlCalled = true
			mu.Unlock()
			return "" // Never open a provider, including when reproducing a regression.
		}, core.AuthSessionOptions{Scheme: "nsgo-watch-auth1400", Path: "/callback"}, func(callback string, err error) {
			mu.Lock()
			defer mu.Unlock()
			replies++
			failure = err
			if callback != "" {
				failure = errors.New("TV returned a callback")
			}
		})
		return core.View{}
	})
	app.WaitUntil(5*time.Second, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return replies != 0
	})
	mu.Lock()
	defer mu.Unlock()
	if replies != 1 || urlCalled || !errors.Is(failure, core.AuthUnavailable) {
		t.Fatalf("TV auth: replies=%d, urlCalled=%v, err=%v", replies, urlCalled, failure)
	}
}
