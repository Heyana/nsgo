package tsx

import (
	"encoding/base64"
	"errors"

	"github.com/nsgo-dev/nsgo/tsx/internal/js"

	"github.com/nsgo-dev/nsgo/core"
)

// purchases is nsgo's Purchases, NSGo's own, on core's in-app purchase services: available(), products(ids), purchase(productId, { accountToken, quantity, offer }), resolving with { status, transaction }, finish(id), currentEntitlements(), sync(), manageSubscriptions(), requestRefund(id) and on("update", listener). All but available are held to the purchases capability, rejecting or throwing without it.
func (r *runtime) purchases() js.Value {
	o := r.vm.NewObject()
	set := func(name string, f func(call js.FunctionCall) js.Value) {
		_ = o.Set(name, r.function(func(call js.FunctionCall) js.Value {
			if p := r.refused(core.AccessPurchases); p != nil {
				return p
			}
			return f(call)
		}))
	}
	_ = o.Set("available", r.function(func(js.FunctionCall) js.Value {
		p, resolve, _ := r.newPromise()
		_ = resolve(core.Untracked(r.cx, r.cx.Env().CanPurchase))
		return r.vm.ToValue(p)
	}))
	set("products", func(call js.FunctionCall) js.Value {
		var ids []string
		for _, v := range r.arrayItems(arg(call, 0)) {
			ids = append(ids, v.String())
		}
		p, resolve, reject := r.newPromise()
		r.cx.Products(ids, func(products []core.Product, err error) {
			r.inJS(func() {
				if err != nil {
					_ = reject(r.goError(purchaseFailure(err)))
					return
				}
				out := make([]any, len(products))
				for i, pr := range products {
					out[i] = r.product(pr)
				}
				_ = resolve(r.vm.NewArray(out...))
			})
		})
		return r.vm.ToValue(p)
	})
	set("purchase", func(call js.FunctionCall) js.Value {
		op := r.options(arg(call, 1))
		opts := core.PurchaseOptions{AccountToken: op.str("accountToken", ""), Quantity: int(op.float("quantity", 1))}
		p, resolve, reject := r.newPromise()
		if v := op.get("offer"); defined(v) {
			offer := r.options(v)
			sig, err := base64.StdEncoding.DecodeString(offer.str("signature", ""))
			if err != nil {
				_ = reject(r.goError(&JSError{Name: "TypeError", Message: "Purchases.purchase: the offer's signature is not base64"}))
				return r.vm.ToValue(p)
			}
			opts.Offer = &core.PromotionalOffer{ID: offer.str("id", ""), KeyID: offer.str("keyId", ""), Nonce: offer.str("nonce", ""),
				Signature: sig, Timestamp: int64(offer.float("timestamp", 0))}
		}
		product := ""
		if v := arg(call, 0); defined(v) {
			product = v.String()
		}
		r.cx.Purchase(product, opts, func(tr core.Transaction, err error) {
			r.inJS(func() {
				out := r.vm.NewObject()
				switch {
				case err == nil:
					_ = out.Set("status", "purchased")
					_ = out.Set("transaction", r.transaction(tr))
				case errors.Is(err, core.PurchaseCancelled):
					_ = out.Set("status", "cancelled")
				case errors.Is(err, core.PurchasePending):
					_ = out.Set("status", "pending")
				default:
					_ = reject(r.goError(purchaseFailure(err)))
					return
				}
				_ = resolve(out)
			})
		})
		return r.vm.ToValue(p)
	})
	set("finish", func(call js.FunctionCall) js.Value {
		id := ""
		if v := arg(call, 0); defined(v) {
			id = v.String()
		}
		p, resolve, reject := r.newPromise()
		r.cx.FinishTransaction(id, func(err error) {
			r.inJS(func() {
				switch {
				case err == nil:
					_ = resolve(true)
				case errors.Is(err, core.PurchaseNotFound):
					_ = resolve(false)
				default:
					_ = reject(r.goError(purchaseFailure(err)))
				}
			})
		})
		return r.vm.ToValue(p)
	})
	set("currentEntitlements", func(js.FunctionCall) js.Value {
		p, resolve, reject := r.newPromise()
		r.cx.CurrentEntitlements(func(txs []core.Transaction, err error) {
			r.inJS(func() {
				if err != nil {
					_ = reject(r.goError(purchaseFailure(err)))
					return
				}
				out := make([]any, len(txs))
				for i, tr := range txs {
					out[i] = r.transaction(tr)
				}
				_ = resolve(r.vm.NewArray(out...))
			})
		})
		return r.vm.ToValue(p)
	})
	set("sync", func(js.FunctionCall) js.Value {
		p, resolve, reject := r.newPromise()
		r.cx.SyncPurchases(func(err error) {
			r.inJS(func() {
				switch {
				case err == nil:
					_ = resolve(true)
				case errors.Is(err, core.PurchaseCancelled):
					_ = resolve(false)
				default:
					_ = reject(r.goError(purchaseFailure(err)))
				}
			})
		})
		return r.vm.ToValue(p)
	})
	set("manageSubscriptions", func(js.FunctionCall) js.Value {
		p, resolve, reject := r.newPromise()
		r.cx.ManageSubscriptions(func(err error) {
			r.inJS(func() {
				if err != nil {
					_ = reject(r.goError(purchaseFailure(err)))
					return
				}
				_ = resolve(js.Undefined())
			})
		})
		return r.vm.ToValue(p)
	})
	set("requestRefund", func(call js.FunctionCall) js.Value {
		id := ""
		if v := arg(call, 0); defined(v) {
			id = v.String()
		}
		p, resolve, reject := r.newPromise()
		r.cx.RequestRefund(id, func(status core.RefundStatus, err error) {
			r.inJS(func() {
				if err != nil {
					_ = reject(r.goError(purchaseFailure(err)))
					return
				}
				_ = resolve(string(status))
			})
		})
		return r.vm.ToValue(p)
	})
	_ = o.Set("on", r.function(func(call js.FunctionCall) js.Value {
		if err := r.checkAccess(core.AccessRequest{Permission: core.AccessPurchases}); err != nil {
			panic(r.goError(err))
		}
		if ev := arg(call, 0).String(); ev != "update" {
			panic(r.goError(&JSError{Name: "TypeError", Message: "Purchases.on: no event " + ev}))
		}
		fn := arg(call, 1)
		owner := r.cx.Child()
		owner.Run(func() {
			owner.OnPurchaseUpdate(func(tr core.Transaction) { r.call(fn, r.transaction(tr)) })
		})
		return r.function(func(js.FunctionCall) js.Value {
			owner.Dispose()
			return js.Undefined()
		})
	}))
	return o
}

// product is p as JS sees it.
func (r *runtime) product(p core.Product) js.Value {
	o := r.vm.NewObject()
	_ = o.Set("id", p.ID)
	_ = o.Set("type", string(p.Type))
	_ = o.Set("title", p.Title)
	_ = o.Set("description", p.Description)
	_ = o.Set("displayPrice", p.DisplayPrice)
	_ = o.Set("price", p.Price)
	_ = o.Set("currencyCode", p.CurrencyCode)
	if s := p.Subscription; s != nil {
		sub := r.vm.NewObject()
		_ = sub.Set("groupId", s.GroupID)
		_ = sub.Set("period", r.period(s.Period))
		if s.IntroductoryOffer != nil {
			_ = sub.Set("introductoryOffer", r.offer(*s.IntroductoryOffer))
		} else {
			_ = sub.Set("introductoryOffer", js.Null())
		}
		offers := make([]any, len(s.Offers))
		for i, of := range s.Offers {
			offers[i] = r.offer(of)
		}
		_ = sub.Set("offers", r.vm.NewArray(offers...))
		_ = o.Set("subscription", sub)
	} else {
		_ = o.Set("subscription", js.Null())
	}
	return o
}

func (r *runtime) period(p core.SubscriptionPeriod) js.Value {
	o := r.vm.NewObject()
	_ = o.Set("unit", string(p.Unit))
	_ = o.Set("value", p.Value)
	return o
}

func (r *runtime) offer(of core.SubscriptionOffer) js.Value {
	o := r.vm.NewObject()
	_ = o.Set("id", of.ID)
	_ = o.Set("displayPrice", of.DisplayPrice)
	_ = o.Set("period", r.period(of.Period))
	_ = o.Set("periodCount", of.PeriodCount)
	_ = o.Set("paymentMode", string(of.PaymentMode))
	return o
}

// transaction is tr as JS sees it, its dates milliseconds since the Unix epoch, as Date's valueOf, null for none.
func (r *runtime) transaction(tr core.Transaction) js.Value {
	o := r.vm.NewObject()
	_ = o.Set("id", tr.ID)
	_ = o.Set("originalId", tr.OriginalID)
	_ = o.Set("productId", tr.ProductID)
	_ = o.Set("productType", string(tr.ProductType))
	for _, d := range []struct {
		key string
		ms  int64
		set bool
	}{{"purchaseDate", tr.PurchaseDate.UnixMilli(), !tr.PurchaseDate.IsZero()}, {"expirationDate", tr.ExpirationDate.UnixMilli(), !tr.ExpirationDate.IsZero()},
		{"revocationDate", tr.RevocationDate.UnixMilli(), !tr.RevocationDate.IsZero()}} {
		if d.set {
			_ = o.Set(d.key, d.ms)
		} else {
			_ = o.Set(d.key, js.Null())
		}
	}
	_ = o.Set("quantity", tr.Quantity)
	_ = o.Set("accountToken", tr.AccountToken)
	_ = o.Set("familyShared", tr.FamilyShared)
	_ = o.Set("upgraded", tr.Upgraded)
	_ = o.Set("environment", tr.Environment)
	_ = o.Set("signed", tr.Signed)
	_ = o.Set("verified", tr.Verified)
	return o
}

// purchaseFailure is err with the stable code JS reads: unavailable is ERR_FEATURE_UNAVAILABLE_ON_PLATFORM.
func purchaseFailure(err error) error {
	if errors.Is(err, core.PurchasesUnavailable) {
		return core.Unavailable(err.Error())
	}
	return err
}
