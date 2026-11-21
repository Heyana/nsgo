package core_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

var purchases = nsgotest.Environment(core.Environment{Purchases: true})

var (
	coins   = core.Product{ID: "coins.100", Type: core.ProductConsumable, Title: "100 coins", DisplayPrice: "¥160", Price: "160", CurrencyCode: "JPY"}
	monthly = core.Product{ID: "pro.monthly", Type: core.ProductAutoRenewable, Title: "Pro", DisplayPrice: "$4.99", Price: "4.99", CurrencyCode: "USD",
		Subscription: &core.Subscription{GroupID: "pro", Period: core.SubscriptionPeriod{Unit: core.PeriodMonth, Value: 1}}}
)

const token = "6F9619FF-8B86-D011-B42D-00C04FC964FF"

// scoped starts a view whose scope the test calls the services on.
func scoped(t *testing.T, opts ...nsgotest.Option) (*nsgotest.App, *core.Scope) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	}, opts...)
	return app, scope
}

func TestProductsAnswersWhatTheStoreSells(t *testing.T) {
	app, scope := scoped(t, purchases)
	app.Products = []core.Product{coins, monthly}
	var got []core.Product
	scope.Products([]string{"pro.monthly", "", "gone", "coins.100"}, func(p []core.Product, err error) {
		if err != nil {
			t.Error(err)
		}
		got = p
	})
	app.Settle()
	if len(got) != 2 || got[0].ID != "pro.monthly" || got[0].DisplayPrice != "$4.99" || got[0].Subscription.Period.Unit != core.PeriodMonth || got[1].ID != "coins.100" {
		t.Fatalf("products %+v", got)
	}
	if r := app.PurchaseRequests; len(r) != 1 || r[0].Action != nsgotest.PurchaseProducts || !slices.Equal(r[0].IDs, []string{"pro.monthly", "gone", "coins.100"}) {
		t.Fatalf("requests %+v", r)
	}
	called := false
	scope.Products(nil, func(p []core.Product, err error) { called = p == nil && err == nil })
	if !called || len(app.PurchaseRequests) != 1 {
		t.Fatalf("no IDs: answered %v, requests %d", called, len(app.PurchaseRequests))
	}
}

func TestPurchaseAnswersTheTransactionToFinish(t *testing.T) {
	app, scope := scoped(t, purchases)
	app.Products = []core.Product{coins, monthly}
	var got core.Transaction
	scope.Purchase("pro.monthly", core.PurchaseOptions{AccountToken: token}, func(tr core.Transaction, err error) {
		if err != nil {
			t.Error(err)
		}
		got = tr
	})
	app.Settle()
	if got.ID == "" || got.ProductID != "pro.monthly" || got.ProductType != core.ProductAutoRenewable || got.Signed == "" || got.ExpirationDate.Sub(got.PurchaseDate) < 28*24*60*60*1e9 {
		t.Fatalf("transaction %+v", got)
	}
	if r := app.PurchaseRequests[0]; r.Action != nsgotest.PurchaseBuy || r.AccountToken != "6f9619ff-8b86-d011-b42d-00c04fc964ff" || r.Quantity != 1 {
		t.Fatalf("request %+v", r)
	}
	if len(app.Unfinished) != 1 || len(app.Entitlements) != 1 {
		t.Fatalf("unfinished %v, entitlements %v", app.Unfinished, app.Entitlements)
	}
	var entitled []core.Transaction
	scope.CurrentEntitlements(func(tr []core.Transaction, err error) { entitled = tr })
	var finished, again error = errors.New("unanswered"), nil
	scope.FinishTransaction(got.ID, func(err error) { finished = err })
	scope.FinishTransaction(got.ID, func(err error) { again = err })
	app.Settle()
	if requests := app.PurchaseRequests; len(requests) != 4 ||
		requests[1].Action != nsgotest.PurchaseEntitlements ||
		requests[2].Action != nsgotest.PurchaseFinish || requests[2].Transaction != got.ID ||
		requests[3].Action != nsgotest.PurchaseFinish || requests[3].Transaction != got.ID {
		t.Fatalf("requests %+v", requests)
	}
	if len(entitled) != 1 || entitled[0].ID != got.ID {
		t.Fatalf("entitlements %+v", entitled)
	}
	if finished != nil || !errors.Is(again, core.PurchaseNotFound) || len(app.Unfinished) != 0 || !slices.Equal(app.Finished, []string{got.ID}) {
		t.Fatalf("finish %v, again %v, unfinished %v", finished, again, app.Unfinished)
	}
}

// A cancel and a purchase waiting for approval are told apart from a failure, which carries the store's words.
func TestPurchaseTellsItsOutcomesApart(t *testing.T) {
	app, scope := scoped(t, purchases)
	app.Products = []core.Product{coins}
	var errs []error
	record := func(tr core.Transaction, err error) {
		if tr != (core.Transaction{}) {
			t.Errorf("a transaction with %v: %+v", err, tr)
		}
		errs = append(errs, err)
	}
	for _, answer := range []error{core.PurchaseCancelled, core.PurchasePending, errors.New("no network")} {
		app.PurchaseAnswer = func(nsgotest.PurchaseRequest) (core.Transaction, error) { return core.Transaction{}, answer }
		scope.Purchase("coins.100", core.PurchaseOptions{Quantity: 3}, record)
		app.Settle()
	}
	app.PurchaseAnswer = nil
	scope.Purchase("gone", core.PurchaseOptions{}, record)
	scope.Purchase("coins.100", core.PurchaseOptions{AccountToken: "user-7"}, record)
	scope.Purchase("", core.PurchaseOptions{}, record)
	app.Settle()
	want := []error{core.PurchaseCancelled, core.PurchasePending, core.PurchaseFailed, core.PurchaseFailed, core.PurchaseFailed, core.PurchaseFailed}
	if len(errs) != len(want) {
		t.Fatalf("answers %v", errs)
	}
	for i, err := range errs {
		if !errors.Is(err, want[i]) {
			t.Errorf("answer %d: %v, want %v", i, err, want[i])
		}
	}
	if errs[2].Error() != "nsgo: purchase: failed: no network" {
		t.Errorf("a failure says %q", errs[2])
	}
	if errors.Is(errs[0], core.PurchaseFailed) || errors.Is(errs[1], core.PurchaseFailed) {
		t.Errorf("a cancel or a pending purchase fails: %v", errs[:2])
	}
	// The token that is no UUID and the empty product never reach the store.
	if n := len(app.PurchaseRequests); n != 4 || app.PurchaseRequests[0].Quantity != 3 {
		t.Fatalf("requests %+v", app.PurchaseRequests)
	}
	if len(app.Unfinished) != 0 {
		t.Fatalf("unfinished %v", app.Unfinished)
	}
}

func TestPurchaseSendsAPromotionalOffer(t *testing.T) {
	app, scope := scoped(t, purchases)
	app.Products = []core.Product{monthly}
	offer := &core.PromotionalOffer{ID: "winback", KeyID: "KEY1", Nonce: token, Signature: []byte{1, 2}, Timestamp: 1767225600000}
	scope.Purchase("pro.monthly", core.PurchaseOptions{Offer: offer}, nil)
	var err error
	scope.Purchase("pro.monthly", core.PurchaseOptions{Offer: &core.PromotionalOffer{ID: "winback"}}, func(_ core.Transaction, e error) { err = e })
	app.Settle()
	if len(app.PurchaseRequests) != 1 {
		t.Fatalf("requests %+v", app.PurchaseRequests)
	}
	offer.Signature[0] = 9
	if o := app.PurchaseRequests[0].Offer; o == nil || o.ID != "winback" || o.Nonce != "6f9619ff-8b86-d011-b42d-00c04fc964ff" || !slices.Equal(o.Signature, []byte{1, 2}) {
		t.Fatalf("offer %+v", o)
	}
	if offer.Nonce != token {
		t.Error("the app's offer was changed")
	}
	if !errors.Is(err, core.PurchaseFailed) {
		t.Fatalf("an offer without a nonce: %v", err)
	}
}

// Updates that come before any handler, as an unfinished transaction does at launch, go to the first one.
func TestPurchaseUpdatesKeptForTheFirstHandler(t *testing.T) {
	approved := core.Transaction{ID: "7", ProductID: "coins.100", ProductType: core.ProductConsumable}
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	}, purchases, nsgotest.UnfinishedTransactions(approved))
	app.Products = []core.Product{coins}
	var heard, second []string
	scope.OnPurchaseUpdate(func(tr core.Transaction) { heard = append(heard, tr.ID) })
	app.Settle()
	if !slices.Equal(heard, []string{"7"}) {
		t.Fatalf("heard %v at launch", heard)
	}
	scope.OnPurchaseUpdate(func(tr core.Transaction) { second = append(second, tr.ID) })
	app.UpdatePurchase(core.Transaction{ID: "8", ProductID: "coins.100"})
	if !slices.Equal(heard, []string{"7", "8"}) || !slices.Equal(second, []string{"8"}) {
		t.Fatalf("heard %v and %v", heard, second)
	}
	if len(app.Unfinished) != 2 || app.Unfinished[1].ProductType != core.ProductConsumable || len(app.Entitlements) != 0 {
		t.Fatalf("unfinished %+v, entitlements %+v", app.Unfinished, app.Entitlements)
	}
}

func TestSyncPurchases(t *testing.T) {
	app, scope := scoped(t, purchases)
	var errs []error
	scope.SyncPurchases(func(err error) { errs = append(errs, err) })
	app.Settle()
	app.SyncAnswer = core.PurchaseCancelled
	scope.SyncPurchases(func(err error) { errs = append(errs, err) })
	app.Settle()
	if len(errs) != 2 || errs[0] != nil || !errors.Is(errs[1], core.PurchaseCancelled) {
		t.Fatalf("answers %v", errs)
	}
	if requests := app.PurchaseRequests; len(requests) != 2 ||
		requests[0].Action != nsgotest.PurchaseSync || requests[1].Action != nsgotest.PurchaseSync {
		t.Fatalf("requests %+v", requests)
	}
}

func TestPurchasesUnavailableWithoutAStore(t *testing.T) {
	app, scope := scoped(t)
	app.Products = []core.Product{coins}
	var errs []error
	scope.Products([]string{"coins.100"}, func(_ []core.Product, err error) { errs = append(errs, err) })
	scope.Purchase("coins.100", core.PurchaseOptions{}, func(_ core.Transaction, err error) { errs = append(errs, err) })
	scope.FinishTransaction("1", func(err error) { errs = append(errs, err) })
	scope.CurrentEntitlements(func(_ []core.Transaction, err error) { errs = append(errs, err) })
	scope.SyncPurchases(func(err error) { errs = append(errs, err) })
	app.Settle()
	if len(errs) != 5 || len(app.PurchaseRequests) != 0 {
		t.Fatalf("answers %v, requests %v", errs, app.PurchaseRequests)
	}
	for _, err := range errs {
		if err != core.PurchasesUnavailable {
			t.Errorf("answer %v", err)
		}
	}
}

func TestPurchasesHeldToAccess(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		cx.ProvideAccess(denyAll{})
		return core.View{}
	}, purchases)
	app.Products = []core.Product{coins}
	var errs []error
	scope.Products([]string{"coins.100"}, func(_ []core.Product, err error) { errs = append(errs, err) })
	scope.Purchase("coins.100", core.PurchaseOptions{}, func(_ core.Transaction, err error) { errs = append(errs, err) })
	scope.FinishTransaction("1", func(err error) { errs = append(errs, err) })
	scope.CurrentEntitlements(func(_ []core.Transaction, err error) { errs = append(errs, err) })
	scope.SyncPurchases(func(err error) { errs = append(errs, err) })
	heard := false
	scope.OnPurchaseUpdate(func(core.Transaction) { heard = true })
	app.UpdatePurchase(core.Transaction{ID: "9", ProductID: "coins.100"})
	if len(errs) != 5 || len(app.PurchaseRequests) != 0 || heard {
		t.Fatalf("answers %v, requests %v, heard %v", errs, app.PurchaseRequests, heard)
	}
	for _, err := range errs {
		if core.ErrorCode(err) != core.CodeAccessDenied {
			t.Errorf("refused answer %v", err)
		}
	}
}
