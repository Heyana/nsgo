package core_test

import (
	"errors"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMiniProgramSignInReturnsOnlyHostUserCodes(t *testing.T) {
	for _, platform := range []core.MiniProgramPlatform{core.MiniProgramWeChat, core.MiniProgramQQ, core.MiniProgramAlipay, core.MiniProgramDouyin} {
		t.Run(string(platform), func(t *testing.T) {
			var scope *core.Scope
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node { scope = cx; return core.View{} }, nsgotest.Environment(core.Environment{MiniProgramSignIn: true}))
			want := core.MiniProgramCredential{Platform: platform, Code: "owned-one-use-code"}
			app.MiniProgramSignIn = func(opts core.MiniProgramSignInOptions) (core.MiniProgramCredential, error) {
				if !opts.Prompt {
					t.Error("lost explicit prompt option")
				}
				return want, nil
			}
			calls := 0
			scope.SignInMiniProgram(core.MiniProgramSignInOptions{Prompt: true}, func(c core.MiniProgramCredential, err error) {
				calls++
				if err != nil || c != want {
					t.Errorf("incorrect credential or error: platform=%q empty=%v error=%v", c.Platform, c.Code == "", err)
				}
			})
			app.Settle()
			if calls != 1 || len(app.MiniProgramSignIns) != 1 {
				t.Fatalf("callbacks=%d requests=%d", calls, len(app.MiniProgramSignIns))
			}
		})
	}
}

func TestMiniProgramSignInValidatesReplies(t *testing.T) {
	for _, tc := range []struct {
		name       string
		credential core.MiniProgramCredential
		err, want  error
	}{
		{"empty code", core.MiniProgramCredential{Platform: core.MiniProgramWeChat}, nil, core.MiniProgramSignInFailed},
		{"unknown platform", core.MiniProgramCredential{Platform: "other", Code: "owned"}, nil, core.MiniProgramSignInFailed},
		{"cancelled", core.MiniProgramCredential{}, core.MiniProgramSignInCancelled, core.MiniProgramSignInCancelled},
		{"unavailable", core.MiniProgramCredential{}, core.MiniProgramSignInUnavailable, core.MiniProgramSignInUnavailable},
		{"anonymous only", core.MiniProgramCredential{}, core.MiniProgramSignInNotSignedIn, core.MiniProgramSignInNotSignedIn},
		{"unknown error", core.MiniProgramCredential{}, errors.New("host network failed"), core.MiniProgramSignInFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var scope *core.Scope
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node { scope = cx; return core.View{} }, nsgotest.Environment(core.Environment{MiniProgramSignIn: true}))
			app.MiniProgramSignIn = func(core.MiniProgramSignInOptions) (core.MiniProgramCredential, error) { return tc.credential, tc.err }
			calls := 0
			scope.SignInMiniProgram(core.MiniProgramSignInOptions{}, func(c core.MiniProgramCredential, err error) {
				calls++
				if c != (core.MiniProgramCredential{}) || !errors.Is(err, tc.want) {
					t.Errorf("credential returned=%v error=%v", c.Code != "", err)
				}
			})
			app.Settle()
			if calls != 1 {
				t.Fatalf("callbacks=%d", calls)
			}
		})
	}
}

func TestMiniProgramSignInNeedsCapabilityAndHost(t *testing.T) {
	for _, denied := range []bool{false, true} {
		t.Run(map[bool]string{false: "no host", true: "denied"}[denied], func(t *testing.T) {
			var scope *core.Scope
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				scope = cx
				if denied {
					cx.ProvideAccess(denyAll{})
				}
				return core.View{}
			}, nsgotest.Environment(core.Environment{MiniProgramSignIn: denied}))
			calls := 0
			scope.SignInMiniProgram(core.MiniProgramSignInOptions{}, func(c core.MiniProgramCredential, err error) {
				calls++
				if denied {
					if core.ErrorCode(err) != core.CodeAccessDenied {
						t.Errorf("error=%v", err)
					}
				} else if !errors.Is(err, core.MiniProgramSignInUnavailable) {
					t.Errorf("error=%v", err)
				}
			})
			app.Settle()
			if calls != 1 || len(app.MiniProgramSignIns) != 0 {
				t.Fatalf("callbacks=%d requests=%d", calls, len(app.MiniProgramSignIns))
			}
			scope.SignInMiniProgram(core.MiniProgramSignInOptions{}, nil)
		})
	}
}
