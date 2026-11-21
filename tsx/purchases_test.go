package tsx_test

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestPurchases(t *testing.T) {
	app := run(t, `import { mount, signal, Purchases, Text, View } from "nsgo";
function App() {
	const said = signal("idle");
	const updates = signal("");
	Purchases.on("update", (tr) => updates.set(updates() + tr.id + ";"));
	const buy = () => Purchases.purchase("pro.monthly", { accountToken: "6f9619ff-8b86-d011-b42d-00c04fc964ff" }).then(
		(r) => r.status === "purchased" ? Purchases.finish(r.transaction.id).then((ok) => said.set(r.transaction.productId + " " + r.transaction.signed + " " + ok)) : said.set(r.status),
		(e) => said.set("failed " + e.message));
	return <View>
		<View testID="products" style={{ height: 20 }} onPress={() => Purchases.products(["pro.monthly", "gone"]).then((ps) => said.set(ps.map((p) => p.id + " " + p.displayPrice + " " + p.subscription.period.unit).join()))} />
		<View testID="buy" style={{ height: 20 }} onPress={buy} />
		<View testID="entitlements" style={{ height: 20 }} onPress={() => Purchases.currentEntitlements().then((ts) => said.set("entitled " + ts.map((t) => t.productId).join()))} />
		<View testID="sync" style={{ height: 20 }} onPress={() => Purchases.sync().then((ok) => said.set("synced " + ok))} />
		<Text>{said()}</Text>
		<Text>{"updates " + updates()}</Text>
	</View>;
}
mount(App);`, nsgotest.Environment(core.Environment{OS: core.OSIOS, Purchases: true}))
	app.Products = []core.Product{{ID: "pro.monthly", Type: core.ProductAutoRenewable, DisplayPrice: "$4.99",
		Subscription: &core.Subscription{Period: core.SubscriptionPeriod{Unit: core.PeriodMonth, Value: 1}}}}
	app.Press("id:products")
	if !slices.Contains(app.Texts(), "pro.monthly $4.99 month") {
		t.Fatalf("products: %q", app.Texts())
	}
	app.Press("id:buy")
	if !slices.Contains(app.Texts(), "pro.monthly nsgotest.2000000001 true") {
		t.Fatalf("a purchase: %q", app.Texts())
	}
	app.Press("id:entitlements")
	if !slices.Contains(app.Texts(), "entitled pro.monthly") {
		t.Fatalf("entitlements: %q", app.Texts())
	}
	for _, c := range []struct {
		err  error
		said string
	}{{core.PurchaseCancelled, "cancelled"}, {core.PurchasePending, "pending"}, {core.PurchaseFailed, "failed nsgo: purchase: failed"}} {
		app.PurchaseAnswer = func(nsgotest.PurchaseRequest) (core.Transaction, error) { return core.Transaction{}, c.err }
		app.Press("id:buy")
		if !slices.Contains(app.Texts(), c.said) {
			t.Fatalf("%v: %q", c.err, app.Texts())
		}
	}
	app.UpdatePurchase(core.Transaction{ID: "42", ProductID: "pro.monthly"})
	if !slices.Contains(app.Texts(), "updates 42;") {
		t.Fatalf("an update: %q", app.Texts())
	}
	app.SyncAnswer = core.PurchaseCancelled
	app.Press("id:sync")
	if !slices.Contains(app.Texts(), "synced false") {
		t.Fatalf("a declined sync: %q", app.Texts())
	}
	if r := app.PurchaseRequests[1]; r.Action != nsgotest.PurchaseBuy || r.AccountToken != "6f9619ff-8b86-d011-b42d-00c04fc964ff" {
		t.Fatalf("request %+v", r)
	}
}

func TestPurchasesUnavailable(t *testing.T) {
	app := run(t, `import { mount, signal, Purchases, Text, View } from "nsgo";
function App() {
	const said = signal("idle");
	return <View>
		<View testID="buy" style={{ height: 20 }} onPress={() => Purchases.available().then((on) => Purchases.purchase("coins").then(() => said.set("bought"), (e) => said.set(on + " " + e.code)))} />
		<Text>{said()}</Text>
	</View>;
}
mount(App);`)
	app.Press("id:buy")
	if !slices.Contains(app.Texts(), "false "+core.CodeUnavailable) {
		t.Fatalf("texts %q", app.Texts())
	}
}
