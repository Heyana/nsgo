//go:build darwin && !ios && !nsgo_steam

package macos

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
)

// TestMacAppStoreLiveLookup reads Apple's public listing and resolves the
// returned link's native handler. It neither installs an app nor uses an account.
func TestMacAppStoreLiveLookup(t *testing.T) {
	if os.Getenv("NSGO_MAC_STORE_NATIVE") != "1" {
		t.Skip("set NSGO_MAC_STORE_NATIVE=1 for the public storefront and Launch Services check")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	update := updates.FindInMacAppStore(ctx, updates.Config{App: "Things", ID: "com.culturedcode.ThingsMac", Version: "0", Channel: "store"}, "US")
	if update.Err != "" || update.Version == "" || !strings.HasPrefix(update.URL, "macappstore://apps.apple.com/") {
		t.Fatalf("live Mac storefront = %+v", update)
	}
	var handler string
	onMain(func() {
		loadAppKit()
		workspace := objc.ID(apple.Class("NSWorkspace")).Send(sel("sharedWorkspace"))
		url := objc.ID(apple.Class("NSURL")).Send(sel("URLWithString:"), apple.NSString(update.URL))
		app := workspace.Send(sel("URLForApplicationToOpenURL:"), url)
		if app != 0 {
			bundle := objc.ID(apple.Class("NSBundle")).Send(sel("bundleWithURL:"), app)
			handler = apple.GoString(bundle.Send(sel("bundleIdentifier")))
		}
	})
	if handler != "com.apple.AppStore" {
		t.Fatalf("store page handler = %q", handler)
	}
	t.Logf("public Mac app version %s resolves to %s", update.Version, handler)
}

func TestMacStoreCapabilityDoesNotRequireSparkle(t *testing.T) {
	old := sparkleConfig
	sparkleConfig = updates.Config{Channel: "store", Automatic: true}
	t.Cleanup(func() { sparkleConfig = old })
	state := sparkleStatus()
	if state.Managed != driver.UpdateManagedPage || !state.AutomaticChecks || state.NativePreferences || state.AutomaticDownloads {
		t.Fatalf("Mac store capability = %+v", state)
	}
}

// TestMacStoreUsesInstalledBundle runs from an owned .app fixture whose
// CFBundleIdentifier is dev.nsgo.test.store1421 and version is 2.7.0. The
// captured HTTP request proves the backend uses that bundle rather than the
// deliberately stale configuration; the store itself is stubbed in this test.
func TestMacStoreUsesInstalledBundle(t *testing.T) {
	if os.Getenv("NSGO_MAC_STORE_BUNDLE") != "1" {
		t.Skip("requires the owned store1421 .app test fixture")
	}
	asked := make(chan string, 1)
	old := imageload.HTTP
	imageload.HTTP = func(_ context.Context, address string, _ imageload.Header, _ imageload.Progress) (*imageload.Response, error) {
		asked <- address
		return &imageload.Response{Status: 200, Body: []byte(`{"results":[{"version":"3.0.0","trackId":123}]}`)}, nil
	}
	t.Cleanup(func() { imageload.HTTP = old })
	answers := make(chan driver.Event, 1)
	onMain(func() {
		loadAppKit()
		apple.CheckAppStore(updates.Config{App: "Store fixture", ID: "wrong.bundle", Version: "0", Channel: "store"}, true, 77, func(event driver.Event) { answers <- event })
	})
	select {
	case event := <-answers:
		update := event.Data.(*driver.Update)
		if event.Count != 77 || update.Current != "2.7.0" || update.Version != "3.0.0" || update.URL != "macappstore://apps.apple.com/app/id123" || update.Err != "" {
			t.Fatalf("installed bundle update = %+v, event %+v", update, event)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("installed bundle store check did not answer")
	}
	address := <-asked
	request, err := url.Parse(address)
	if err != nil || request.Query().Get("bundleId") != "dev.nsgo.test.store1421" || request.Query().Get("entity") != "desktopSoftware" {
		t.Fatalf("installed bundle lookup = %q, %v", address, err)
	}
}
