package tsx_test

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestSignInWithApple(t *testing.T) {
	app := run(t, `import { mount, signal, SignInWithApple, Text, View } from "nsgo";
function App() {
	const said = signal("idle");
	const available = signal("?");
	SignInWithApple.available().then((on) => available.set(String(on)));
	const show = (p) => p.then((c) => said.set(c === null ? "cancelled" : c.user + " " + c.identityToken + " " + c.fullName.givenName),
		(e) => said.set("failed " + (e.code ?? e.message)));
	return <View>
		<View testID="sign-in" style={{ height: 20 }} onPress={() => show(SignInWithApple.signIn({ fullName: true, nonce: "n" }))} />
		<View testID="state" style={{ height: 20 }} onPress={() => SignInWithApple.credentialState("ada").then((s) => said.set(s))} />
		<Text>{available() + " " + said()}</Text>
	</View>;
}
mount(App);`, nsgotest.Environment(core.Environment{OS: core.OSIOS, SignInWithApple: true}))
	app.Press("id:sign-in")
	if !slices.Contains(app.Texts(), "true cancelled") {
		t.Fatalf("a cancel: %q", app.Texts())
	}
	app.AppleSignIn = func(r nsgotest.AppleSignInRequest) (core.AppleCredential, error) {
		return core.AppleCredential{User: "ada", IdentityToken: "jwt", GivenName: "Ada"}, nil
	}
	app.Press("id:sign-in")
	if !slices.Contains(app.Texts(), "true ada jwt Ada") {
		t.Fatalf("a sign-in: %q", app.Texts())
	}
	if r := app.AppleSignIns[1]; !r.FullName || r.Email || r.Nonce == "n" || len(r.Nonce) != 64 {
		t.Fatalf("request %+v", r)
	}
	app.AppleSignIn = func(nsgotest.AppleSignInRequest) (core.AppleCredential, error) {
		return core.AppleCredential{}, core.AppleSignInFailed
	}
	app.Press("id:sign-in")
	if !slices.Contains(app.Texts(), "true failed nsgo: sign in with Apple: failed") {
		t.Fatalf("a failure: %q", app.Texts())
	}
	app.AppleCredentialStates = map[string]core.AppleCredentialState{"ada": core.AppleCredentialRevoked}
	app.Press("id:state")
	if !slices.Contains(app.Texts(), "true revoked") {
		t.Fatalf("a state: %q", app.Texts())
	}
}

func TestSignInWithAppleUnavailable(t *testing.T) {
	app := run(t, `import { mount, signal, SignInWithApple, Text, View } from "nsgo";
function App() {
	const said = signal("idle");
	return <View>
		<View testID="sign-in" style={{ height: 20 }} onPress={() => SignInWithApple.signIn().then(() => said.set("signed in"), (e) => said.set(e.code))} />
		<Text>{said()}</Text>
	</View>;
}
mount(App);`)
	app.Press("id:sign-in")
	if !slices.Contains(app.Texts(), core.CodeUnavailable) {
		t.Fatalf("texts %q", app.Texts())
	}
}
