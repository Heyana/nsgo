package nsgotest

import (
	"slices"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// SpeechSession records the active speech recognizer's language and
// whether it reports interim results.
type SpeechSession struct {
	Lang    string
	Interim bool
}

// AppleSignInRequest records the requested credential fields and the
// SHA-256 hex of the nonce, empty when the app supplies none.
type AppleSignInRequest struct {
	FullName, Email bool
	Nonce           string
}

// PurchaseAction identifies a store operation in App.PurchaseRequests.
type PurchaseAction string

const (
	PurchaseProducts     PurchaseAction = "products"
	PurchaseBuy          PurchaseAction = "buy"
	PurchaseFinish       PurchaseAction = "finish"
	PurchaseEntitlements PurchaseAction = "entitlements"
	PurchaseSync         PurchaseAction = "sync"
)

// PurchaseRequest records a store operation and its options. IDs are the
// products to look up. Transaction is the ID of a purchase to finish.
type PurchaseRequest struct {
	Action       PurchaseAction
	IDs          []string
	Product      string
	AccountToken string
	Quantity     int
	Offer        *core.PromotionalOffer
	Transaction  string
}

var purchaseActions = map[driver.PurchaseCommand]PurchaseAction{
	driver.PurchaseProducts:     PurchaseProducts,
	driver.PurchaseBuy:          PurchaseBuy,
	driver.PurchaseFinish:       PurchaseFinish,
	driver.PurchaseEntitlements: PurchaseEntitlements,
	driver.PurchaseSync:         PurchaseSync,
}

func purchaseRequest(r driver.PurchaseRequest) PurchaseRequest {
	request := PurchaseRequest{
		Action:       purchaseActions[r.Command],
		IDs:          slices.Clone(r.IDs),
		Product:      r.Product,
		AccountToken: r.AccountToken,
		Quantity:     r.Quantity,
		Transaction:  r.Transaction,
	}
	if r.Offer != nil {
		offer := *r.Offer
		offer.Signature = slices.Clone(offer.Signature)
		request.Offer = &offer
	}
	return request
}
