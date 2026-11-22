package nsgotest

import (
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// UnfinishedTransactions starts the app with transactions it has not finished, as if bought in an earlier run: where the environment says Purchases, the store hands each to OnPurchaseUpdate as the app starts, as StoreKit does, and they stay in Unfinished until the app finishes them.
func UnfinishedTransactions(txs ...core.Transaction) Option {
	return func(a *App) { a.Unfinished = append(a.Unfinished, txs...) }
}

// launchPurchases delivers the unfinished transactions as updates, as the store does as the app starts.
func (a *App) launchPurchases() {
	if a.env == nil || !a.env.Purchases {
		return
	}
	for _, tr := range a.Unfinished {
		a.post(driver.Event{Kind: driver.EventPurchaseUpdate, Data: &tr})
	}
}

// UpdatePurchase has the store deliver tr to OnPurchaseUpdate, as it does a transaction made or changed outside a Purchase call: a renewal, a purchase approved after Ask to Buy, one made on another device or a refund, and settles. tr joins Unfinished, and Entitlements as a purchase does, or leaves them once its RevocationDate is set.
func (a *App) UpdatePurchase(tr core.Transaction) {
	a.t.Helper()
	tr = a.recordTransaction(tr)
	a.post(driver.Event{Kind: driver.EventPurchaseUpdate, Data: &tr})
	a.Settle()
}

// purchase answers an ItemPurchase from the model's store.
func (a *App) purchase(id uint32, r driver.PurchaseRequest) {
	request := purchaseRequest(r)
	a.PurchaseRequests = append(a.PurchaseRequests, request)
	answer := &driver.PurchaseAnswer{}
	switch r.Command {
	case driver.PurchaseProducts:
		for _, want := range r.IDs {
			if i := slices.IndexFunc(a.Products, func(p core.Product) bool { return p.ID == want }); i >= 0 {
				answer.Products = append(answer.Products, a.Products[i])
			}
		}
	case driver.PurchaseBuy:
		tr, err := a.buy(request)
		if err != nil {
			answer.Error, answer.Message = purchaseErrorCode(err)
			break
		}
		tr = a.recordTransaction(tr)
		answer.Transaction = &tr
	case driver.PurchaseFinish:
		i := slices.IndexFunc(a.Unfinished, func(tr core.Transaction) bool { return tr.ID == r.Transaction })
		if i < 0 {
			answer.Error = driver.PurchaseNotFound
			break
		}
		a.Unfinished = slices.Delete(a.Unfinished, i, i+1)
		a.Finished = append(a.Finished, r.Transaction)
	case driver.PurchaseEntitlements:
		answer.Transactions = slices.Clone(a.Entitlements)
	case driver.PurchaseSync:
		if a.SyncAnswer != nil {
			answer.Error, answer.Message = purchaseErrorCode(a.SyncAnswer)
		}
	case driver.PurchaseManageSubscriptions:
		if a.ManageSubscriptionsError != nil {
			answer.Error, answer.Message = purchaseErrorCode(a.ManageSubscriptionsError)
		}
	case driver.PurchaseRefund:
		answer.RefundStatus = string(a.RefundStatus)
		if answer.RefundStatus == "" {
			answer.RefundStatus = driver.RefundSuccess
		}
	}
	a.post(driver.Event{Kind: driver.EventPurchase, Count: id, Data: answer})
}

// buy is the transaction a purchase makes: PurchaseAnswer's, else the product's among Products.
func (a *App) buy(r PurchaseRequest) (core.Transaction, error) {
	if a.PurchaseAnswer != nil {
		tr, err := a.PurchaseAnswer(r)
		if err == nil && tr.ProductID == "" {
			tr.ProductID = r.Product
		}
		return tr, err
	}
	i := slices.IndexFunc(a.Products, func(p core.Product) bool { return p.ID == r.Product })
	if i < 0 {
		return core.Transaction{}, errors.New("the store sells no " + r.Product)
	}
	return core.Transaction{ProductID: r.Product, AccountToken: r.AccountToken, Quantity: r.Quantity}, nil
}

// recordTransaction fills in what the store gives tr that it lacks, its ID, dates and signature among them, and keeps it in Unfinished and, but for a consumable or a revoked one, Entitlements.
func (a *App) recordTransaction(tr core.Transaction) core.Transaction {
	i := slices.IndexFunc(a.Products, func(p core.Product) bool { return p.ID == tr.ProductID })
	if tr.ProductType == "" && i >= 0 {
		tr.ProductType = a.Products[i].Type
	}
	if tr.ID == "" {
		a.transactions++
		tr.ID = strconv.Itoa(2000000000 + a.transactions)
	}
	if tr.OriginalID == "" {
		tr.OriginalID = tr.ID
	}
	if tr.PurchaseDate.IsZero() {
		tr.PurchaseDate = a.Clock.WallTime()
	}
	if tr.ExpirationDate.IsZero() && tr.ProductType == core.ProductAutoRenewable && i >= 0 && a.Products[i].Subscription != nil {
		tr.ExpirationDate = addPeriod(tr.PurchaseDate, a.Products[i].Subscription.Period)
	}
	tr.Quantity = max(tr.Quantity, 1)
	if tr.Environment == "" {
		tr.Environment = "Xcode"
	}
	if tr.Signed == "" {
		// No server verifies it: it only stands where the store's JWS goes.
		tr.Signed = "nsgotest." + tr.ID
		tr.Verified = true
	}
	a.Unfinished = append(slices.DeleteFunc(a.Unfinished, func(o core.Transaction) bool { return o.ID == tr.ID }), tr)
	if tr.ProductType != core.ProductConsumable {
		a.Entitlements = slices.DeleteFunc(a.Entitlements, func(o core.Transaction) bool { return o.ProductID == tr.ProductID })
		if tr.RevocationDate.IsZero() {
			a.Entitlements = append(a.Entitlements, tr)
		}
	}
	return tr
}

// addPeriod is t a subscription's period later.
func addPeriod(t time.Time, p core.SubscriptionPeriod) time.Time {
	n := max(p.Value, 1)
	switch p.Unit {
	case core.PeriodDay:
		return t.AddDate(0, 0, n)
	case core.PeriodWeek:
		return t.AddDate(0, 0, 7*n)
	case core.PeriodYear:
		return t.AddDate(n, 0, 0)
	}
	return t.AddDate(0, n, 0)
}

// purchaseErrorCode is what the platform answers a failure with: a core.PurchaseError's code, else failed with the error's words, those after PurchaseFailed's own where it wraps it.
func purchaseErrorCode(err error) (code, message string) {
	var e core.PurchaseError
	if errors.As(err, &e) && e != core.PurchaseFailed {
		return string(e), ""
	}
	if err == error(core.PurchaseFailed) {
		return driver.PurchaseFailed, ""
	}
	return driver.PurchaseFailed, strings.TrimPrefix(err.Error(), core.PurchaseFailed.Error()+": ")
}
