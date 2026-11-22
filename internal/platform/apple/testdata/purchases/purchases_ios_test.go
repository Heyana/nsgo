//go:build ios && !nsgowatch

package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

const fixtureProduct = "dev.nsgo.test.storekit1367.entitlement"

// The SDK requires a .storekit JSON artifact; it is generated in the test's temporary directory rather than adding a hand-edited JSON configuration.
func localStoreKitConfiguration(t *testing.T) string {
	t.Helper()
	config := map[string]any{
		"identifier":               "61464A57-9B78-414B-B5ED-BD0C227E1367",
		"nonRenewingSubscriptions": []any{},
		"products": []any{map[string]any{
			"displayPrice": "1.99", "familyShareable": false, "internalID": "13670001",
			"localizations": []any{map[string]any{"description": "Isolated native transaction fixture", "displayName": "Fixture entitlement", "locale": "en_US"}},
			"productID":     fixtureProduct, "referenceName": "Fixture entitlement", "type": "NonConsumable",
		}},
		"settings":           map[string]any{"_applicationInternalID": "1367", "_failTransactionsEnabled": false, "_locale": "en_US", "_storefront": "USA", "_storeKitErrors": []any{}},
		"subscriptionGroups": []any{}, "version": map[string]any{"major": 3, "minor": 0},
	}
	data, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "Native.storekit")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func nsString(value string) objc.ID {
	return objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), value)
}
func nativeString(value objc.ID) string {
	if value == 0 {
		return ""
	}
	p := objc.Send[uintptr](value, objc.RegisterName("UTF8String"))
	n := objc.Send[uintptr](value, objc.RegisterName("lengthOfBytesUsingEncoding:"), uintptr(4))
	if p == 0 || n > 64*1024 {
		return ""
	}
	return string(unsafe.Slice((*byte)(unsafe.Pointer(p)), int(n)))
}

func storeKitSession(t *testing.T) objc.ID {
	t.Helper()
	if os.Getenv("NSGO_STOREKIT_NATIVE_TEST") != "1" {
		t.Skip("set NSGO_STOREKIT_NATIVE_TEST=1 for isolated StoreKit Testing")
	}
	if nsgotest.Target() != "ios_simulator" {
		t.Fatal("native StoreKit fixture requires -target ios_simulator")
	}
	frameworks := os.Getenv("NSGO_STOREKIT_TEST_FRAMEWORKS")
	if frameworks == "" {
		t.Fatal("NSGO_STOREKIT_TEST_FRAMEWORKS must name Xcode's simulator test frameworks")
	}
	for _, path := range []string{filepath.Join(frameworks, "XCTest.framework", "XCTest"), "/Developer/Library/Frameworks/StoreKitTest.framework/StoreKitTest"} {
		if _, err := purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			t.Fatalf("load %s: %v", path, err)
		}
	}
	class := objc.GetClass("SKTestSession")
	if class == 0 {
		t.Fatal("Xcode did not register SKTestSession")
	}
	url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), nsString(localStoreKitConfiguration(t)))
	var nativeError objc.ID
	session := objc.ID(class).Send(objc.RegisterName("alloc")).Send(objc.RegisterName("initWithContentsOfURL:error:"), url, unsafe.Pointer(&nativeError))
	if session == 0 {
		t.Fatalf("create local StoreKit session: %s", nativeString(nativeError.Send(objc.RegisterName("localizedDescription"))))
	}
	session.Send(objc.RegisterName("setDisableDialogs:"), true)
	session.Send(objc.RegisterName("clearTransactions"))
	t.Cleanup(func() {
		session.Send(objc.RegisterName("clearTransactions"))
		session.Send(objc.RegisterName("resetToDefaultState"))
		session.Send(objc.RegisterName("release"))
	})
	return session
}

func awaitResult[T any](t *testing.T, ch <-chan T, operation string) T {
	t.Helper()
	select {
	case result := <-ch:
		return result
	case <-time.After(25 * time.Second):
		t.Fatalf("StoreKit %s did not answer", operation)
	}
	var zero T
	return zero
}

func TestNativeFinishedTransactionStaysFinishedAfterEntitlements(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	session := storeKitSession(t)
	ready := make(chan *core.Scope, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { ready <- cx; return core.Text{Content: "StoreKit native fixture"} }, nsgotest.Size(320, 240))
	cx := awaitResult(t, ready, "start")
	type productsResult struct {
		products []core.Product
		err      error
	}
	products := make(chan productsResult, 1)
	app.Post(func() {
		cx.Products([]string{fixtureProduct}, func(p []core.Product, err error) { products <- productsResult{p, err} })
	})
	app.Settle()
	listed := awaitResult(t, products, "products")
	if listed.err != nil || len(listed.products) != 1 || listed.products[0].DisplayPrice == "" {
		t.Fatalf("local product: count %d, error %v", len(listed.products), listed.err)
	}
	purchase := make(chan core.Transaction, 8)
	app.Post(func() {
		cx.OnPurchaseUpdate(func(tr core.Transaction) { purchase <- tr })
	})
	app.Settle()
	// Inject a local transaction through StoreKit Testing. This exercises the real update and finish bridge without opening a sandbox payment sheet.
	var nativeError objc.ID
	if !objc.Send[bool](session, objc.RegisterName("buyProductWithIdentifier:error:"), nsString(fixtureProduct), unsafe.Pointer(&nativeError)) {
		t.Fatalf("create local transaction: %s", nativeString(nativeError.Send(objc.RegisterName("localizedDescription"))))
	}
	tr := awaitResult(t, purchase, "transaction update")
	if !tr.Verified || !strings.EqualFold(tr.Environment, "Xcode") || strings.Count(tr.Signed, ".") != 2 {
		t.Fatal("purchase was not a verified local Xcode transaction")
	}
	finish := func() error {
		done := make(chan error, 1)
		app.Post(func() { cx.FinishTransaction(tr.ID, func(err error) { done <- err }) })
		app.Settle()
		return awaitResult(t, done, "finish")
	}
	concurrent := make(chan error, 2)
	app.Post(func() {
		for range 2 {
			cx.FinishTransaction(tr.ID, func(err error) { concurrent <- err })
		}
	})
	app.Settle()
	var successes, missing int
	for range 2 {
		switch err := awaitResult(t, concurrent, "concurrent finish"); {
		case err == nil:
			successes++
		case errors.Is(err, core.PurchaseNotFound):
			missing++
		default:
			t.Fatalf("concurrent finish: %v", err)
		}
	}
	if successes != 1 || missing != 1 {
		t.Fatalf("concurrent finishes: %d succeeded, %d not found", successes, missing)
	}
	type entitlementsResult struct {
		transactions []core.Transaction
		err          error
	}
	entitlements := make(chan entitlementsResult, 1)
	app.Post(func() {
		cx.CurrentEntitlements(func(txs []core.Transaction, err error) { entitlements <- entitlementsResult{txs, err} })
	})
	app.Settle()
	owned := awaitResult(t, entitlements, "entitlements")
	if owned.err != nil {
		t.Fatal(owned.err)
	}
	found := false
	for _, transaction := range owned.transactions {
		if transaction.ID == tr.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("finished non-consumable disappeared from entitlements")
	}
	if err := finish(); !errors.Is(err, core.PurchaseNotFound) {
		t.Fatalf("finished transaction became finishable after entitlement read: %v", err)
	}
}
