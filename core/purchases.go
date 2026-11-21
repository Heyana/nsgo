package core

import (
	"fmt"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// PurchaseError is why an in-app purchase service answered without what it was asked for. A failure the platform explains wraps PurchaseFailed with its words, so test it with errors.Is.
type PurchaseError string

const (
	// PurchaseCancelled: the user closed the store's sheet, which is no failure.
	PurchaseCancelled PurchaseError = driver.PurchaseCancelled
	// PurchasePending: the purchase waits for someone's approval, as Ask to Buy's, or for the user to act outside the app; its transaction comes later through OnPurchaseUpdate.
	PurchasePending PurchaseError = driver.PurchasePending
	// PurchaseFailed: the store refused or could not answer, such as for a product it does not sell, a network error or a device that may not make payments.
	PurchaseFailed PurchaseError = driver.PurchaseFailed
	// PurchaseNotFound: no unfinished transaction has the ID FinishTransaction was given, as one already finished has not.
	PurchaseNotFound PurchaseError = driver.PurchaseNotFound
	// PurchasesUnavailable: this platform or build has no store to buy from (Env.CanPurchase).
	PurchasesUnavailable PurchaseError = driver.PurchasesUnavailable
)

func (e PurchaseError) Error() string { return "nsgo: purchase: " + string(e) }

// What the store sells and records, as the platform reports them.
type (
	// Product is what the store sells, in the user's storefront's language and currency: show DisplayPrice as it is and never a price of the app's own, as stores price each storefront themselves.
	Product = driver.Product
	// ProductType is what kind of thing a product sells: ProductConsumable, ProductNonConsumable, ProductAutoRenewable or ProductNonRenewable.
	ProductType = driver.ProductType
	// Subscription is what an auto-renewable subscription's product says of it: its group, its period and its offers.
	Subscription = driver.Subscription
	// SubscriptionPeriod is how long a subscription or an offer's period lasts, Value of Unit.
	SubscriptionPeriod = driver.SubscriptionPeriod
	// PeriodUnit is PeriodDay, PeriodWeek, PeriodMonth or PeriodYear.
	PeriodUnit = driver.PeriodUnit
	// SubscriptionOffer is an introductory or promotional offer of a subscription.
	SubscriptionOffer = driver.SubscriptionOffer
	// OfferPaymentMode is how an offer is paid: OfferFreeTrial, OfferPayAsYouGo or OfferPayUpFront.
	OfferPaymentMode = driver.OfferPaymentMode
	// PromotionalOffer is a subscription's promotional offer as a purchase takes it, which the app's server signs with its In-App Purchase key.
	PromotionalOffer = driver.PromotionalOffer
	// Transaction is a purchase the store recorded. Send Signed, the store's JWS, to the app's server to verify, deliver what was bought, then finish it with FinishTransaction; until then the store hands it to the app again at each launch.
	Transaction = driver.Transaction
)

// The kinds of product.
const (
	ProductConsumable    = driver.ProductConsumable
	ProductNonConsumable = driver.ProductNonConsumable
	ProductAutoRenewable = driver.ProductAutoRenewable
	ProductNonRenewable  = driver.ProductNonRenewable
)

// The units of a subscription's period.
const (
	PeriodDay   = driver.PeriodDay
	PeriodWeek  = driver.PeriodWeek
	PeriodMonth = driver.PeriodMonth
	PeriodYear  = driver.PeriodYear
)

// How an offer is paid.
const (
	OfferFreeTrial  = driver.OfferFreeTrial
	OfferPayAsYouGo = driver.OfferPayAsYouGo
	OfferPayUpFront = driver.OfferPayUpFront
)

// PurchaseOptions are what Purchase buys with.
type PurchaseOptions struct {
	// AccountToken is a UUID of the app's own that ties the purchase to its user, which the transaction and the store's notifications to the app's server carry: StoreKit's appAccountToken. "" sends none.
	AccountToken string
	// Quantity is how many of a consumable to buy; 0 is 1.
	Quantity int
	// Offer is the promotional offer a subscription is bought with, nil for none; an introductory offer the user is eligible for applies without one.
	Offer *PromotionalOffer
}

// Products answers done with the products of ids the store sells, in the store's order, leaving out IDs it does not know; no IDs answer none without asking. Their prices are the store's, formatted for the user's storefront. Where the platform has no store, as Env.CanPurchase says, it answers PurchasesUnavailable without asking. Code whose Access does not grant AccessPurchases gets AccessDenied.
func (cx *Scope) Products(ids []string, done func([]Product, error)) {
	answer := func(p []Product, err error) {
		if done != nil {
			done(p, err)
		}
	}
	t, err := cx.purchases()
	if err != nil {
		answer(nil, err)
		return
	}
	ids = slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == "" })
	if len(ids) == 0 {
		answer(nil, nil)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemPurchase, Purchase: &driver.PurchaseRequest{Command: driver.PurchaseProducts, IDs: ids}}, func(ev driver.Event) {
		a, err := purchaseAnswer(ev)
		answer(a.Products, err)
	})
}

// Purchase buys the product whose ID is product through the store's sheet, from the app's own button, and answers done once: with the transaction, which the app's server verifies before the app delivers what was bought and finishes it, or with a PurchaseError, PurchaseCancelled when the user closes the sheet and PurchasePending while the purchase waits for approval, whose transaction comes through OnPurchaseUpdate. An AccountToken that is no UUID fails without asking. It is held and unavailable as Products is.
func (cx *Scope) Purchase(product string, opts PurchaseOptions, done func(Transaction, error)) {
	answer := func(tr Transaction, err error) {
		if done != nil {
			done(tr, err)
		}
	}
	t, err := cx.purchases()
	if err != nil {
		answer(Transaction{}, err)
		return
	}
	token, ok := uuidString(opts.AccountToken)
	switch {
	case product == "":
		answer(Transaction{}, fmt.Errorf("%w: no product", PurchaseFailed))
		return
	case !ok:
		answer(Transaction{}, fmt.Errorf("%w: the account token %q is not a UUID", PurchaseFailed, opts.AccountToken))
		return
	}
	r := driver.PurchaseRequest{Command: driver.PurchaseBuy, Product: product, AccountToken: token, Quantity: max(opts.Quantity, 1)}
	if o := opts.Offer; o != nil {
		offer := *o
		if offer.Nonce, ok = uuidString(o.Nonce); !ok || offer.Nonce == "" {
			answer(Transaction{}, fmt.Errorf("%w: the offer's nonce %q is not a UUID", PurchaseFailed, o.Nonce))
			return
		}
		r.Offer = &offer
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemPurchase, Purchase: &r}, func(ev driver.Event) {
		a, err := purchaseAnswer(ev)
		if err == nil && a.Transaction == nil {
			err = PurchaseFailed
		}
		if err != nil {
			answer(Transaction{}, err)
			return
		}
		answer(*a.Transaction, nil)
	})
}

// FinishTransaction tells the store the app delivered what the transaction whose ID is id bought, and answers done: nil, or PurchaseNotFound when no unfinished transaction has the ID. Finish each transaction Purchase, OnPurchaseUpdate or CurrentEntitlements gives once its server verified it: the store hands an unfinished one to the app again at each launch, and refunds an unfinished purchase after some days. It is held and unavailable as Products is.
func (cx *Scope) FinishTransaction(id string, done func(error)) {
	answer := func(err error) {
		if done != nil {
			done(err)
		}
	}
	t, err := cx.purchases()
	if err != nil {
		answer(err)
		return
	}
	if id == "" {
		answer(PurchaseNotFound)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemPurchase, Purchase: &driver.PurchaseRequest{Command: driver.PurchaseFinish, Transaction: id}}, func(ev driver.Event) {
		_, err := purchaseAnswer(ev)
		answer(err)
	})
}

// CurrentEntitlements answers done with the latest transaction of each product the user is entitled to now: non-consumables, subscriptions in their period and non-renewing subscriptions, refunded and revoked ones left out. Read them as the app starts to unlock what the user bought, on this device or another. It is held and unavailable as Products is.
func (cx *Scope) CurrentEntitlements(done func([]Transaction, error)) {
	answer := func(tr []Transaction, err error) {
		if done != nil {
			done(tr, err)
		}
	}
	t, err := cx.purchases()
	if err != nil {
		answer(nil, err)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemPurchase, Purchase: &driver.PurchaseRequest{Command: driver.PurchaseEntitlements}}, func(ev driver.Event) {
		a, err := purchaseAnswer(ev)
		answer(a.Transactions, err)
	})
}

// SyncPurchases has the store bring the user's transactions up to date, which it does on its own otherwise, and answers done: nil, then CurrentEntitlements answers what it found, or a PurchaseError, PurchaseCancelled when the user declines to sign in. Call it only from a Restore Purchases button the user presses, as it may ask them to sign in to their account. It is held and unavailable as Products is.
func (cx *Scope) SyncPurchases(done func(error)) {
	answer := func(err error) {
		if done != nil {
			done(err)
		}
	}
	t, err := cx.purchases()
	if err != nil {
		answer(err)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemPurchase, Purchase: &driver.PurchaseRequest{Command: driver.PurchaseSync}}, func(ev driver.Event) {
		_, err := purchaseAnswer(ev)
		answer(err)
	})
}

type purchaseUpdateHandler struct {
	owner *Scope
	fn    func(Transaction)
}

// OnPurchaseUpdate runs fn, while the current owner lives, with each transaction the store makes or changes outside a Purchase call: a renewal, a purchase approved after Ask to Buy or completed outside the app, one made on another device, a refund or a revocation, and as the app starts each transaction it has not finished. Transactions that came before any handler go to the first one registered, so register it as the app starts. Code whose Access does not grant AccessPurchases hears none.
func (cx *Scope) OnPurchaseUpdate(fn func(Transaction)) {
	if fn == nil || cx.denied(servicePurchases, "") {
		return
	}
	rt := cx.rt
	h := &purchaseUpdateHandler{owner: cx.current(), fn: fn}
	rt.purchaseUpdates = append(rt.purchaseUpdates, h)
	h.owner.addCleanup(func() {
		rt.purchaseUpdates = slices.DeleteFunc(rt.purchaseUpdates, func(o *purchaseUpdateHandler) bool { return o == h })
	})
	if pending := rt.updatesPending; len(pending) > 0 {
		rt.updatesPending = nil
		// Not while the caller builds its view: the handler may well change what it shows.
		h.owner.Post(func() {
			for _, tr := range pending {
				h.fn(tr)
			}
		})
	}
}

// purchaseUpdated hands tr to the update handlers, or keeps it for the first one.
func (rt *runtime) purchaseUpdated(tr Transaction) {
	if len(rt.purchaseUpdates) == 0 {
		rt.updatesPending = append(rt.updatesPending, tr)
		return
	}
	for _, h := range slices.Clone(rt.purchaseUpdates) {
		rt.tree.within(h.owner, func() { h.fn(tr) })
	}
}

// purchases is the tree the current owner's purchase requests go to, or why they reach none.
func (cx *Scope) purchases() (*Tree, error) {
	if err := cx.gate(servicePurchases, ""); err != nil {
		return nil, err
	}
	t := cx.tree()
	if t == nil || !Untracked(cx, cx.Env().CanPurchase) {
		return nil, PurchasesUnavailable
	}
	return t, nil
}

// purchaseAnswer is ev's answer and the error it names, a failure carrying the platform's words.
func purchaseAnswer(ev driver.Event) (driver.PurchaseAnswer, error) {
	a, _ := ev.Data.(*driver.PurchaseAnswer)
	if a == nil {
		return driver.PurchaseAnswer{}, PurchaseFailed
	}
	switch e := PurchaseError(a.Error); e {
	case "":
		return *a, nil
	case PurchaseCancelled, PurchasePending, PurchaseNotFound, PurchasesUnavailable:
		return *a, e
	}
	if a.Message != "" {
		return *a, fmt.Errorf("%w: %s", PurchaseFailed, a.Message)
	}
	return *a, PurchaseFailed
}

// uuidString is s as a UUID in lowercase, as StoreKit writes them, "" for "", and whether s is one or empty.
func uuidString(s string) (string, bool) {
	if s == "" {
		return "", true
	}
	if len(s) != 36 {
		return "", false
	}
	for i, c := range s {
		switch {
		case i == 8 || i == 13 || i == 18 || i == 23:
			if c != '-' {
				return "", false
			}
		case '0' <= c && c <= '9', 'a' <= c && c <= 'f', 'A' <= c && c <= 'F':
		default:
			return "", false
		}
	}
	return strings.ToLower(s), true
}
