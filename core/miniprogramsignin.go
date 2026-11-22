package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// MiniProgramPlatform names the host that issued a MiniProgramCredential.
type MiniProgramPlatform string

const (
	MiniProgramWeChat MiniProgramPlatform = "wechat"
	MiniProgramQQ     MiniProgramPlatform = "qq"
	MiniProgramAlipay MiniProgramPlatform = "alipay"
	MiniProgramDouyin MiniProgramPlatform = "douyin"
)

// MiniProgramCredential is a short-lived login code for the app's server to
// exchange through Platform's API. It is not a verified identity or an access token.
type MiniProgramCredential struct {
	Platform MiniProgramPlatform
	Code     string
}

// MiniProgramSignInOptions configure SignInMiniProgram.
type MiniProgramSignInOptions struct {
	// Prompt permits Douyin to show its host-account login when not signed in.
	// False asks silently. WeChat, QQ and Alipay use their basic login APIs.
	Prompt bool
}

// MiniProgramSignInError is why a mini program returned no login code.
type MiniProgramSignInError string

const (
	MiniProgramSignInCancelled   MiniProgramSignInError = "cancelled"
	MiniProgramSignInFailed      MiniProgramSignInError = "failed"
	MiniProgramSignInUnavailable MiniProgramSignInError = "unavailable"
	// MiniProgramSignInNotSignedIn means Douyin returned only an anonymous
	// device code. It does not identify a signed-in user.
	MiniProgramSignInNotSignedIn MiniProgramSignInError = "notSignedIn"
)

func (e MiniProgramSignInError) Error() string { return "nsgo: mini program sign in: " + string(e) }

// SignInMiniProgram asks WeChat, QQ or Douyin for a login code, or Alipay for
// an auth_base authorization code. The app sends the credential to its server
// for exchange; it keeps app secrets there. It does not open an OAuth URL or
// call AuthSession's URL builder. Other targets answer MiniProgramSignInUnavailable.
// AccessMiniProgramSignIn holds the request, and done runs once under its owner.
func (cx *Scope) SignInMiniProgram(opts MiniProgramSignInOptions, done func(MiniProgramCredential, error)) {
	answer := func(c MiniProgramCredential, err error) {
		if done != nil {
			done(c, err)
		}
	}
	if err := cx.gate(serviceMiniProgramSignIn, ""); err != nil {
		answer(MiniProgramCredential{}, err)
		return
	}
	t := cx.tree()
	if t == nil || !Untracked(cx, cx.Env().CanSignInMiniProgram) {
		answer(MiniProgramCredential{}, MiniProgramSignInUnavailable)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemMiniProgramSignIn, On: opts.Prompt}, func(ev driver.Event) {
		if c, ok := ev.Data.(*driver.MiniProgramCredential); ok && c != nil && c.Code != "" && validMiniProgramPlatform(MiniProgramPlatform(c.Platform)) && ev.Text == "" {
			answer(MiniProgramCredential{Platform: MiniProgramPlatform(c.Platform), Code: c.Code}, nil)
			return
		}
		err := MiniProgramSignInError(ev.Text)
		switch err {
		case MiniProgramSignInCancelled, MiniProgramSignInUnavailable, MiniProgramSignInNotSignedIn:
		default:
			err = MiniProgramSignInFailed
		}
		answer(MiniProgramCredential{}, err)
	})
}

func validMiniProgramPlatform(platform MiniProgramPlatform) bool {
	switch platform {
	case MiniProgramWeChat, MiniProgramQQ, MiniProgramAlipay, MiniProgramDouyin:
		return true
	}
	return false
}
