package core

import (
	"encoding/hex"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// AppleSignInError is why Sign in with Apple answered without a credential or a state.
type AppleSignInError string

const (
	// AppleSignInCancelled: the user closed the sheet, which is no failure.
	AppleSignInCancelled AppleSignInError = driver.AppleSignInCancelled
	// AppleSignInFailed: the system refused or could not answer, as it does for an app not signed with the entitlement (nsgo.yaml's apple.sign_in_with_apple) or a device signed in to no Apple Account.
	AppleSignInFailed AppleSignInError = driver.AppleSignInFailed
	// AppleSignInUnavailable: this platform has no Sign in with Apple (Env.CanSignInWithApple).
	AppleSignInUnavailable AppleSignInError = driver.AppleSignInUnavailable
)

func (e AppleSignInError) Error() string { return "nsgo: sign in with Apple: " + string(e) }

// AppleSignInOptions are what SignInWithApple asks for.
type AppleSignInOptions struct {
	// FullName and Email ask for the user's name and email, which Apple gives only the first time the user signs in to the app.
	FullName, Email bool
	// Nonce goes into the request as its SHA-256, in lowercase hex, which the identity token's nonce claim then holds, so the app's server, which knows Nonce, can tell the token was made for this sign-in. "" sends none.
	Nonce string
}

// AppleCredential is what a sign-in with Apple answers.
type AppleCredential struct {
	// User is the user's stable ID for the app's team, the same on each sign-in and what AppleCredentialState asks about.
	User string
	// IdentityToken is a JSON Web Token Apple signed, for the app's server to verify, and AuthorizationCode a short-lived code the server exchanges for tokens.
	IdentityToken, AuthorizationCode string
	// Email, which may be a private relay address, and the name's parts are given only on the user's first sign-in to the app and only as AppleSignInOptions asked for them; "" otherwise, so keep them then.
	Email                                                               string
	GivenName, FamilyName, MiddleName, NamePrefix, NameSuffix, Nickname string
}

// AppleCredentialState is the state of a user's Sign in with Apple credential for the app.
type AppleCredentialState uint8

const (
	// AppleCredentialAuthorized: the user is signed in to the app with Apple.
	AppleCredentialAuthorized AppleCredentialState = driver.AppleCredentialAuthorized
	// AppleCredentialRevoked: the user stopped using Sign in with Apple with the app, so it signs them out.
	AppleCredentialRevoked AppleCredentialState = driver.AppleCredentialRevoked
	// AppleCredentialNotFound: no credential of the user is known for the app.
	AppleCredentialNotFound AppleCredentialState = driver.AppleCredentialNotFound
	// AppleCredentialTransferred: the app moved to another team, and the user's ID with it, which the app's server migrates.
	AppleCredentialTransferred AppleCredentialState = driver.AppleCredentialTransferred
)

// String returns the state's name in TSX: "authorized", "revoked", "notFound" or "transferred"; "" for the zero state.
func (s AppleCredentialState) String() string {
	switch s {
	case AppleCredentialAuthorized:
		return "authorized"
	case AppleCredentialRevoked:
		return "revoked"
	case AppleCredentialNotFound:
		return "notFound"
	case AppleCredentialTransferred:
		return "transferred"
	}
	return ""
}

// SignInWithApple shows the system's Sign in with Apple sheet, from the app's own button, and answers done once: with the credential, or with an AppleSignInError, AppleSignInCancelled when the user closes the sheet. macOS, iOS and tvOS show it to an app signed with the com.apple.developer.applesignin entitlement, which nsgo build claims when nsgo.yaml's apple.sign_in_with_apple is set; elsewhere it answers AppleSignInUnavailable without asking the platform. Code whose Access does not grant AccessSignInWithApple gets AccessDenied.
func (cx *Scope) SignInWithApple(opts AppleSignInOptions, done func(AppleCredential, error)) {
	answer := func(c AppleCredential, err error) {
		if done != nil {
			done(c, err)
		}
	}
	t := cx.tree()
	if err := cx.gate(serviceSignInWithApple, ""); err != nil {
		answer(AppleCredential{}, err)
		return
	}
	if t == nil || !Untracked(cx, cx.Env().CanSignInWithApple) {
		answer(AppleCredential{}, AppleSignInUnavailable)
		return
	}
	r := driver.AppleSignInRequest{FullName: opts.FullName, Email: opts.Email}
	if opts.Nonce != "" {
		sum := driver.Sum256([]byte(opts.Nonce))
		r.Nonce = hex.EncodeToString(sum[:])
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemSignInWithApple, AppleSignIn: &r}, func(ev driver.Event) {
		if c, ok := ev.Data.(*driver.AppleCredential); ok && c != nil {
			answer(AppleCredential(*c), nil)
			return
		}
		answer(AppleCredential{}, appleSignInError(ev.Text))
	})
}

// AppleCredentialState answers done with the state of user's credential, the ID an AppleCredential gave, or with an AppleSignInError: check it as the app starts, to sign out a user who revoked it. An empty user is AppleCredentialNotFound without asking the platform. It is held as SignInWithApple is.
func (cx *Scope) AppleCredentialState(user string, done func(AppleCredentialState, error)) {
	answer := func(s AppleCredentialState, err error) {
		if done != nil {
			done(s, err)
		}
	}
	t := cx.tree()
	if err := cx.gate(serviceSignInWithApple, ""); err != nil {
		answer(0, err)
		return
	}
	if t == nil || !Untracked(cx, cx.Env().CanSignInWithApple) {
		answer(0, AppleSignInUnavailable)
		return
	}
	if user == "" {
		answer(AppleCredentialNotFound, nil)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemAppleCredentialState, Value: user}, func(ev driver.Event) {
		if ev.Text != "" {
			answer(0, appleSignInError(ev.Text))
			return
		}
		answer(AppleCredentialState(ev.Index), nil)
	})
}

// appleSignInError is the error a platform's answer names, AppleSignInFailed for one it does not.
func appleSignInError(code string) error {
	switch e := AppleSignInError(code); e {
	case AppleSignInCancelled, AppleSignInUnavailable:
		return e
	}
	return AppleSignInFailed
}
