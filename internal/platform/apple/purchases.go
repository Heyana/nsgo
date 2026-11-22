//go:build darwin

package apple

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// In-app purchases for macOS, iOS and tvOS through StoreKit 2, whose products and transactions are Swift's alone: a library of Swift that nsgo build puts among the bundle's frameworks when nsgo.yaml sets apple.purchases (cmd/nsgo/storekit) carries each request to StoreKit and its answer back as JSON, through two C functions and a callback. Apps without it link none of StoreKit, and report Environment.Purchases false.

// storeKitLibraries are where the bundle's frameworks hold the library, iOS's and tvOS's framework, then macOS's plain library.
var storeKitLibraries = []string{"NSGoStoreKit.framework/NSGoStoreKit", "libNSGoStoreKit.dylib"}

// storeKit is the loaded library and the calls it has not answered yet.
var storeKit struct {
	mu   sync.Mutex
	call func(id uint32, request string)
	// calls are the requests waiting for an answer, by the library's call ID, which next numbers.
	calls map[uint32]storeKitCall
	next  uint32
	// updates hears the transactions Transaction.updates delivers; pending keeps those delivered before it is set.
	updates func(driver.Event)
	pending []driver.Event
}

// storeKitCall is an ItemPurchase the library carries out: the request's Count and where its answer goes.
type storeKitCall struct {
	count    uint32
	dispatch func(driver.Event)
}

// PurchasesAvailable is Environment.Purchases: whether the bundle has the library, which loads on macOS 12, iOS 15 and tvOS 15 and later. Loading it starts listening to Transaction.updates, as StoreKit asks apps to from their launch.
func PurchasesAvailable() bool { return loadStoreKit() }

var loadStoreKit = sync.OnceValue(func() bool {
	frameworks := GoString(objc.ID(Class("NSBundle")).Send(sel("mainBundle")).Send(sel("privateFrameworksPath")))
	if frameworks == "" {
		return false
	}
	var lib uintptr
	for _, name := range storeKitLibraries {
		path := filepath.Join(frameworks, name)
		if l, err := purego.Dlopen(path, purego.RTLD_LOCAL|purego.RTLD_NOW); err == nil {
			lib = l
			break
		} else if _, statErr := os.Stat(path); statErr == nil {
			// The library is there but does not load, as on a macOS older than 12, which has no StoreKit 2.
			log.Printf("nsgo: in-app purchases are unavailable: %v", err)
		}
	}
	if lib == 0 {
		return false
	}
	var start func(answer uintptr)
	for _, f := range []struct {
		fn   any
		name string
	}{{&start, "nsgo_storekit_start"}, {&storeKit.call, "nsgo_storekit_call"}} {
		if _, err := purego.Dlsym(lib, f.name); err != nil {
			log.Printf("nsgo: in-app purchases are unavailable: the app's NSGoStoreKit has no %s: build it again", f.name)
			return false
		}
		purego.RegisterLibFunc(f.fn, lib, f.name)
	}
	start(purego.NewCallback(storeKitAnswered))
	return true
})

// storeKitAnswered hears the library's answer to call id, or a transaction Transaction.updates delivered when id is 0, on any thread: n bytes of JSON at data, which stay the library's.
func storeKitAnswered(id uint32, data *byte, n int) {
	var a storeKitAnswer
	if err := json.Unmarshal(unsafe.Slice(data, n), &a); err != nil {
		a = storeKitAnswer{Error: driver.PurchaseFailed, Message: err.Error()}
	}
	storeKit.mu.Lock()
	if id == 0 {
		if a.Transaction == nil {
			storeKit.mu.Unlock()
			return
		}
		ev := driver.Event{Kind: driver.EventPurchaseUpdate, Data: a.Transaction.driver()}
		fn := storeKit.updates
		if fn == nil {
			storeKit.pending = append(storeKit.pending, ev)
		}
		storeKit.mu.Unlock()
		if fn != nil {
			fn(ev)
		}
		return
	}
	c, ok := storeKit.calls[id]
	delete(storeKit.calls, id)
	storeKit.mu.Unlock()
	if ok {
		c.dispatch(driver.Event{Kind: driver.EventPurchase, Count: c.count, Data: a.driver()})
	}
}

// SetPurchaseUpdates has fn hear the transactions the store updates outside a purchase, as EventPurchaseUpdate, those it delivered before first.
func SetPurchaseUpdates(fn func(driver.Event)) {
	storeKit.mu.Lock()
	storeKit.updates = fn
	pending := storeKit.pending
	storeKit.pending = nil
	storeKit.mu.Unlock()
	for _, ev := range pending {
		fn(ev)
	}
}

// HandlePurchase carries out an ItemPurchase, answering through dispatch.
func HandlePurchase(it *driver.MountItem, dispatch func(driver.Event)) {
	if !loadStoreKit() {
		dispatch(driver.Event{Kind: driver.EventPurchase, Count: it.Count, Data: &driver.PurchaseAnswer{Error: driver.PurchasesUnavailable}})
		return
	}
	r := it.Purchase
	req := storeKitRequest{Command: r.Command.String(), IDs: r.IDs, Product: r.Product, AccountToken: r.AccountToken, Quantity: r.Quantity, Transaction: r.Transaction}
	if o := r.Offer; o != nil {
		req.Offer = &storeKitOffer{ID: o.ID, KeyID: o.KeyID, Nonce: o.Nonce, Signature: o.Signature, Timestamp: o.Timestamp}
	}
	data, err := json.Marshal(req)
	if err != nil {
		dispatch(driver.Event{Kind: driver.EventPurchase, Count: it.Count, Data: &driver.PurchaseAnswer{Error: driver.PurchaseFailed, Message: err.Error()}})
		return
	}
	storeKit.mu.Lock()
	storeKit.next++
	if storeKit.next == 0 {
		storeKit.next = 1
	}
	id := storeKit.next
	if storeKit.calls == nil {
		storeKit.calls = map[uint32]storeKitCall{}
	}
	storeKit.calls[id] = storeKitCall{count: it.Count, dispatch: dispatch}
	storeKit.mu.Unlock()
	storeKit.call(id, string(data))
}

// storeKitRequest is a request as the library reads it (cmd/nsgo/storekit's Request).
type storeKitRequest struct {
	Command      string         `json:"command"`
	IDs          []string       `json:"ids,omitempty"`
	Product      string         `json:"product,omitempty"`
	AccountToken string         `json:"accountToken,omitempty"`
	Quantity     int            `json:"quantity,omitempty"`
	Offer        *storeKitOffer `json:"offer,omitempty"`
	Transaction  string         `json:"transaction,omitempty"`
}

// storeKitOffer is a promotional offer as the library reads it, the signature in base64 as Swift's JSONDecoder reads Data.
type storeKitOffer struct {
	ID        string `json:"id"`
	KeyID     string `json:"keyID"`
	Nonce     string `json:"nonce"`
	Signature []byte `json:"signature"`
	Timestamp int64  `json:"timestamp"`
}

// storeKitAnswer is the library's answer; its products read as driver's do.
type storeKitAnswer struct {
	Error        string                `json:"error"`
	Message      string                `json:"message"`
	Products     []driver.Product      `json:"products"`
	Transaction  *storeKitTransaction  `json:"transaction"`
	Transactions []storeKitTransaction `json:"transactions"`
	RefundStatus string                `json:"refundStatus"`
}

// storeKitTransaction is a transaction as the library writes it, its dates in milliseconds since the Unix epoch.
type storeKitTransaction struct {
	ID           string `json:"id"`
	OriginalID   string `json:"originalID"`
	ProductID    string `json:"productID"`
	ProductType  string `json:"productType"`
	PurchaseDate int64  `json:"purchaseDate"`
	Expiration   *int64 `json:"expirationDate"`
	Revocation   *int64 `json:"revocationDate"`
	Quantity     int    `json:"quantity"`
	AccountToken string `json:"accountToken"`
	FamilyShared bool   `json:"familyShared"`
	Upgraded     bool   `json:"upgraded"`
	Environment  string `json:"environment"`
	Signed       string `json:"signed"`
	Verified     bool   `json:"verified"`
}

func (a *storeKitAnswer) driver() *driver.PurchaseAnswer {
	out := &driver.PurchaseAnswer{Error: a.Error, Message: a.Message, Products: a.Products, RefundStatus: a.RefundStatus}
	if a.Transaction != nil {
		out.Transaction = a.Transaction.driver()
	}
	for _, t := range a.Transactions {
		out.Transactions = append(out.Transactions, *t.driver())
	}
	return out
}

func (t *storeKitTransaction) driver() *driver.Transaction {
	return &driver.Transaction{
		ID: t.ID, OriginalID: t.OriginalID, ProductID: t.ProductID, ProductType: driver.ProductType(t.ProductType),
		PurchaseDate: time.UnixMilli(t.PurchaseDate), ExpirationDate: unixMilli(t.Expiration), RevocationDate: unixMilli(t.Revocation),
		Quantity: t.Quantity, AccountToken: t.AccountToken, FamilyShared: t.FamilyShared, Upgraded: t.Upgraded,
		Environment: t.Environment, Signed: t.Signed, Verified: t.Verified,
	}
}

// unixMilli is the time ms milliseconds after the Unix epoch, zero for none.
func unixMilli(ms *int64) time.Time {
	if ms == nil {
		return time.Time{}
	}
	return time.UnixMilli(*ms)
}
