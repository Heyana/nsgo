package core_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

var signsInWithApple = nsgotest.Environment(core.Environment{SignInWithApple: true})

func TestSignInWithAppleAnswersTheCredential(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	}, signsInWithApple)
	want := core.AppleCredential{User: "001234.abcd", IdentityToken: "eyJ.token", AuthorizationCode: "c0de", Email: "a@privaterelay.appleid.com", GivenName: "Ada", FamilyName: "Lovelace"}
	app.AppleSignIn = func(nsgotest.AppleSignInRequest) (core.AppleCredential, error) { return want, nil }
	var got core.AppleCredential
	var gotErr error
	calls := 0
	scope.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: "raw-nonce"}, func(c core.AppleCredential, err error) {
		got, gotErr = c, err
		calls++
	})
	app.Settle()
	if calls != 1 || gotErr != nil || got != want {
		t.Fatalf("answered %d times: %+v, %v", calls, got, gotErr)
	}
	sum := sha256.Sum256([]byte("raw-nonce"))
	if len(app.AppleSignIns) != 1 || app.AppleSignIns[0] != (nsgotest.AppleSignInRequest{FullName: true, Email: true, Nonce: hex.EncodeToString(sum[:])}) {
		t.Fatalf("requests %+v", app.AppleSignIns)
	}
}

func TestSignInWithAppleTellsACancelFromAFailure(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	}, signsInWithApple)
	var errs []error
	record := func(c core.AppleCredential, err error) {
		if c != (core.AppleCredential{}) {
			t.Errorf("a credential with %v: %+v", err, c)
		}
		errs = append(errs, err)
	}
	scope.SignInWithApple(core.AppleSignInOptions{}, record)
	app.Settle()
	app.AppleSignIn = func(nsgotest.AppleSignInRequest) (core.AppleCredential, error) {
		return core.AppleCredential{}, core.AppleSignInFailed
	}
	scope.SignInWithApple(core.AppleSignInOptions{}, record)
	app.Settle()
	app.AppleSignIn = func(nsgotest.AppleSignInRequest) (core.AppleCredential, error) {
		return core.AppleCredential{}, errors.New("no network")
	}
	scope.SignInWithApple(core.AppleSignInOptions{}, record)
	app.Settle()
	want := []error{core.AppleSignInCancelled, core.AppleSignInFailed, core.AppleSignInFailed}
	if len(errs) != len(want) {
		t.Fatalf("answers %v", errs)
	}
	for i, err := range errs {
		if !errors.Is(err, want[i]) {
			t.Errorf("answer %d: %v, want %v", i, err, want[i])
		}
	}
	if app.AppleSignIns[0].Nonce != "" {
		t.Errorf("a request without a nonce sent %q", app.AppleSignIns[0].Nonce)
	}
}

func TestAppleCredentialState(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	}, signsInWithApple)
	app.AppleCredentialStates = map[string]core.AppleCredentialState{"ada": core.AppleCredentialAuthorized, "bob": core.AppleCredentialRevoked}
	var states []core.AppleCredentialState
	for _, user := range []string{"ada", "bob", "eve", ""} {
		scope.AppleCredentialState(user, func(s core.AppleCredentialState, err error) {
			if err != nil {
				t.Errorf("%s: %v", user, err)
			}
			states = append(states, s)
		})
	}
	app.Settle()
	want := []core.AppleCredentialState{core.AppleCredentialNotFound, core.AppleCredentialAuthorized, core.AppleCredentialRevoked, core.AppleCredentialNotFound}
	// The empty user answers at once, before the platform's answers.
	if len(states) != len(want) {
		t.Fatalf("states %v", states)
	}
	for i, s := range states {
		if s != want[i] {
			t.Errorf("state %d: %v, want %v", i, s, want[i])
		}
	}
}

func TestSignInWithAppleUnavailableWithoutThePlatform(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		return core.View{}
	})
	app.AppleSignIn = func(nsgotest.AppleSignInRequest) (core.AppleCredential, error) {
		return core.AppleCredential{User: "ada"}, nil
	}
	var signIn, state error
	scope.SignInWithApple(core.AppleSignInOptions{}, func(_ core.AppleCredential, err error) { signIn = err })
	scope.AppleCredentialState("ada", func(_ core.AppleCredentialState, err error) { state = err })
	app.Settle()
	if signIn != core.AppleSignInUnavailable || state != core.AppleSignInUnavailable || len(app.AppleSignIns) != 0 {
		t.Fatalf("answers %v, %v, requests %v", signIn, state, app.AppleSignIns)
	}
}

func TestSignInWithAppleHeldToAccess(t *testing.T) {
	var scope *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		cx.ProvideAccess(denyAll{})
		return core.View{}
	}, signsInWithApple)
	app.AppleSignIn = func(nsgotest.AppleSignInRequest) (core.AppleCredential, error) {
		return core.AppleCredential{User: "ada"}, nil
	}
	var signIn, state error
	scope.SignInWithApple(core.AppleSignInOptions{}, func(_ core.AppleCredential, err error) { signIn = err })
	scope.AppleCredentialState("ada", func(_ core.AppleCredentialState, err error) { state = err })
	app.Settle()
	for _, err := range []error{signIn, state} {
		if core.ErrorCode(err) != core.CodeAccessDenied {
			t.Errorf("refused answer %v", err)
		}
	}
	if len(app.AppleSignIns) != 0 {
		t.Fatalf("requests %v", app.AppleSignIns)
	}
}
