package driver

import "time"

// PurchaseCommand is what an ItemPurchase asks of the platform's store.
type PurchaseCommand uint8

const (
	// PurchaseProducts looks up the products of IDs; the answer's Products hold those the store knows.
	PurchaseProducts PurchaseCommand = iota
	// PurchaseBuy buys Product with AccountToken, Quantity and Offer; the answer's Transaction is the purchase, else Error says why there is none.
	PurchaseBuy
	// PurchaseFinish finishes the unfinished transaction whose ID is Transaction.
	PurchaseFinish
	// PurchaseEntitlements asks for the transactions that entitle the user now; the answer's Transactions hold them.
	PurchaseEntitlements
	// PurchaseSync has the store bring the user's transactions up to date, signing them in to their account where it must.
	PurchaseSync
	// PurchaseManageSubscriptions opens the store's subscription management UI.
	PurchaseManageSubscriptions
	// PurchaseRefund asks the store to begin a refund request for Transaction.
	PurchaseRefund
)

var purchaseCommandNames = [...]string{"products", "buy", "finish", "entitlements", "sync", "manageSubscriptions", "refund"}

// String returns the command's name, as traces print it.
func (c PurchaseCommand) String() string {
	if int(c) < len(purchaseCommandNames) {
		return purchaseCommandNames[c]
	}
	return "purchase-command"
}

// PurchaseRequest is an ItemPurchase's request.
type PurchaseRequest struct {
	Command PurchaseCommand
	// IDs are PurchaseProducts' product IDs.
	IDs []string
	// Product is the ID of the product PurchaseBuy buys, AccountToken the UUID tying the purchase to the app's user ("" for none), Quantity how many, at least 1, and Offer the promotional offer it takes, nil for none.
	Product      string
	AccountToken string
	Quantity     int
	Offer        *PromotionalOffer
	// Transaction is the ID of the transaction PurchaseFinish finishes.
	Transaction string
}

// PromotionalOffer is a subscription's promotional offer as a purchase takes it, signed by the app's server with its In-App Purchase key: StoreKit's Product.PurchaseOption.promotionalOffer.
type PromotionalOffer struct {
	// ID is the offer's identifier in App Store Connect, KeyID the ID of the key that signed it.
	ID, KeyID string
	// Nonce is the UUID the server signed, in lowercase, Signature the signature's bytes and Timestamp the signing time in milliseconds since the Unix epoch.
	Nonce     string
	Signature []byte
	Timestamp int64
}

// Why a PurchaseAnswer has nothing to give, as core.PurchaseError.
const (
	PurchaseCancelled    = "cancelled"
	PurchasePending      = "pending"
	PurchaseFailed       = "failed"
	PurchaseNotFound     = "notFound"
	PurchasesUnavailable = "unavailable"
)

// PurchaseAnswer is what EventPurchase answers an ItemPurchase with.
type PurchaseAnswer struct {
	// Error is why there is no answer ("" when there is one), and Message what the platform said of a failure.
	Error, Message string
	// Products answer PurchaseProducts, Transaction PurchaseBuy and Transactions PurchaseEntitlements.
	Products     []Product
	Transaction  *Transaction
	Transactions []Transaction
	// RefundStatus is the result of PurchaseRefund: "success" or "cancelled".
	RefundStatus string
}

const (
	RefundSuccess   = "success"
	RefundCancelled = "cancelled"
)

// ProductType is what kind of thing a product sells, named as StoreKit 2 names it.
type ProductType string

const (
	ProductConsumable    ProductType = "consumable"
	ProductNonConsumable ProductType = "nonConsumable"
	ProductAutoRenewable ProductType = "autoRenewable"
	ProductNonRenewable  ProductType = "nonRenewable"
)

// Product is what the store sells, in the user's storefront's language and currency.
type Product struct {
	ID   string
	Type ProductType
	// Title and Description are the product's display name and description, DisplayPrice its price as the store formats it for the user's storefront, which apps show as it is. Price is the same amount as a decimal number, "4.99", and CurrencyCode its ISO 4217 currency.
	Title, Description string
	DisplayPrice       string
	Price              string
	CurrencyCode       string
	// Subscription is an auto-renewable subscription's, nil for other products.
	Subscription *Subscription
}

// Subscription is what an auto-renewable subscription's product says of it.
type Subscription struct {
	// GroupID is its subscription group's, and Period how long each renewal lasts.
	GroupID string
	Period  SubscriptionPeriod
	// IntroductoryOffer is the offer new subscribers get, nil for none; Offers are its promotional offers.
	IntroductoryOffer *SubscriptionOffer
	Offers            []SubscriptionOffer
}

// PeriodUnit is a unit of a subscription's period.
type PeriodUnit string

const (
	PeriodDay   PeriodUnit = "day"
	PeriodWeek  PeriodUnit = "week"
	PeriodMonth PeriodUnit = "month"
	PeriodYear  PeriodUnit = "year"
)

// SubscriptionPeriod is Value of Unit, such as 1 month.
type SubscriptionPeriod struct {
	Unit  PeriodUnit
	Value int
}

// OfferPaymentMode is how a subscription offer is paid.
type OfferPaymentMode string

const (
	OfferFreeTrial  OfferPaymentMode = "freeTrial"
	OfferPayAsYouGo OfferPaymentMode = "payAsYouGo"
	OfferPayUpFront OfferPaymentMode = "payUpFront"
)

// SubscriptionOffer is an offer of an auto-renewable subscription.
type SubscriptionOffer struct {
	// ID is a promotional offer's identifier, "" for the introductory offer.
	ID string
	// DisplayPrice is what each period costs as the store formats it, Period each period's length, PeriodCount how many periods it lasts and PaymentMode how it is paid.
	DisplayPrice string
	Period       SubscriptionPeriod
	PeriodCount  int
	PaymentMode  OfferPaymentMode
}

// Transaction is a purchase the store recorded.
type Transaction struct {
	// ID is the transaction's, which finishing names, and OriginalID that of the purchase a renewal or a restore goes back to; ProductID and ProductType are what was bought.
	ID, OriginalID string
	ProductID      string
	ProductType    ProductType
	// PurchaseDate is when it was bought or renewed, ExpirationDate when a subscription's period ends and RevocationDate when the store refunded or revoked it, each zero for none.
	PurchaseDate, ExpirationDate, RevocationDate time.Time
	Quantity                                     int
	// AccountToken is the UUID the purchase was made with, "" for none.
	AccountToken string
	// FamilyShared says the user has it through Family Sharing rather than having bought it, Upgraded that a subscription of a higher level in its group replaced it.
	FamilyShared, Upgraded bool
	// Environment is the store's: "Production", "Sandbox" or "Xcode".
	Environment string
	// Signed is the transaction as the store signed it, a JWS for the app's server to verify, and Verified whether the device verified that signature; the server trusts only its own check.
	Signed   string
	Verified bool
}
