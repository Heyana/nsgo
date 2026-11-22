//go:build darwin

package apple

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// The library's JSON, as cmd/nsgo/storekit writes it, reads as driver's products and transactions.
func TestStoreKitAnswerReadsTheLibrarysJSON(t *testing.T) {
	const answer = `{"products":[{"id":"pro.monthly","type":"autoRenewable","title":"Pro","description":"Everything","displayPrice":"$4.99","price":"4.99","currencyCode":"USD",
		"subscription":{"groupID":"21456789","period":{"unit":"month","value":1},"introductoryOffer":{"displayPrice":"$0.00","period":{"unit":"week","value":1},"periodCount":1,"paymentMode":"freeTrial"},
		"offers":[{"id":"winback","displayPrice":"$1.99","period":{"unit":"month","value":1},"periodCount":3,"paymentMode":"payAsYouGo"}]}}],
		"transaction":{"id":"2000000123","originalID":"2000000100","productID":"pro.monthly","productType":"autoRenewable","purchaseDate":1767225600000,"expirationDate":1769904000000,
		"quantity":1,"accountToken":"6f9619ff-8b86-d011-b42d-00c04fc964ff","familyShared":false,"upgraded":false,"environment":"Sandbox","signed":"eyJ.a.b","verified":true}}`
	var a storeKitAnswer
	if err := json.Unmarshal([]byte(answer), &a); err != nil {
		t.Fatal(err)
	}
	got := a.driver()
	p := got.Products[0]
	if p.ID != "pro.monthly" || p.Type != driver.ProductAutoRenewable || p.DisplayPrice != "$4.99" || p.CurrencyCode != "USD" {
		t.Errorf("product %+v", p)
	}
	s := p.Subscription
	if s == nil || s.GroupID != "21456789" || s.Period != (driver.SubscriptionPeriod{Unit: driver.PeriodMonth, Value: 1}) ||
		s.IntroductoryOffer == nil || s.IntroductoryOffer.PaymentMode != driver.OfferFreeTrial || len(s.Offers) != 1 || s.Offers[0].ID != "winback" || s.Offers[0].PeriodCount != 3 {
		t.Errorf("subscription %+v", s)
	}
	tr := got.Transaction
	if tr == nil || tr.ID != "2000000123" || tr.OriginalID != "2000000100" || tr.ProductType != driver.ProductAutoRenewable || tr.Signed != "eyJ.a.b" || !tr.Verified || tr.Environment != "Sandbox" {
		t.Fatalf("transaction %+v", tr)
	}
	if !tr.PurchaseDate.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) || !tr.ExpirationDate.Equal(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)) || !tr.RevocationDate.IsZero() {
		t.Errorf("dates %v, %v, %v", tr.PurchaseDate, tr.ExpirationDate, tr.RevocationDate)
	}
}

// The request carries the offer's signature in base64, as Swift's JSONDecoder reads Data.
func TestStoreKitRequestWritesTheLibrarysJSON(t *testing.T) {
	data, err := json.Marshal(storeKitRequest{Command: driver.PurchaseBuy.String(), Product: "pro.monthly", Quantity: 1,
		Offer: &storeKitOffer{ID: "winback", KeyID: "K", Nonce: "n", Signature: []byte{0xfb, 0xff}, Timestamp: 5}})
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"command":"buy","product":"pro.monthly","quantity":1,"offer":{"id":"winback","keyID":"K","nonce":"n","signature":"+/8=","timestamp":5}}`
	if string(data) != want {
		t.Errorf("request %s, want %s", data, want)
	}
}

func TestStoreKitSubscriptionAndRefundJSON(t *testing.T) {
	data, err := json.Marshal(storeKitRequest{Command: driver.PurchaseManageSubscriptions.String()})
	if err != nil || string(data) != `{"command":"manageSubscriptions"}` {
		t.Fatalf("manage request %s, %v", data, err)
	}
	data, err = json.Marshal(storeKitRequest{Command: driver.PurchaseRefund.String(), Transaction: "42"})
	if err != nil || string(data) != `{"command":"refund","transaction":"42"}` {
		t.Fatalf("refund request %s, %v", data, err)
	}
	var a storeKitAnswer
	if err := json.Unmarshal([]byte(`{"refundStatus":"cancelled"}`), &a); err != nil {
		t.Fatal(err)
	}
	if got := a.driver().RefundStatus; got != driver.RefundCancelled {
		t.Fatalf("refund status %q", got)
	}
}

// A process whose bundle has no library, as a test's has not, has no purchases, and answers a request unavailable.
func TestPurchasesUnavailableWithoutTheLibrary(t *testing.T) {
	if PurchasesAvailable() {
		t.Skip("this process's bundle has NSGoStoreKit")
	}
	var got driver.Event
	HandlePurchase(&driver.MountItem{Kind: driver.ItemPurchase, Count: 4, Purchase: &driver.PurchaseRequest{Command: driver.PurchaseEntitlements}}, func(ev driver.Event) { got = ev })
	if a, _ := got.Data.(*driver.PurchaseAnswer); got.Kind != driver.EventPurchase || got.Count != 4 || a == nil || a.Error != driver.PurchasesUnavailable {
		t.Fatalf("answer %+v", got)
	}
}
