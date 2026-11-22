package nsgotest

import (
	"errors"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func (a *App) signInMiniProgram(it driver.MountItem) {
	opts := core.MiniProgramSignInOptions{Prompt: it.On}
	a.MiniProgramSignIns = append(a.MiniProgramSignIns, opts)
	ev := driver.Event{Kind: driver.EventMiniProgramSignIn, Count: it.Count, Text: string(core.MiniProgramSignInUnavailable)}
	if a.MiniProgramSignIn != nil {
		credential, err := a.MiniProgramSignIn(opts)
		var code core.MiniProgramSignInError
		switch {
		case err == nil:
			ev.Text = ""
			ev.Data = &driver.MiniProgramCredential{Platform: string(credential.Platform), Code: credential.Code}
		case errors.As(err, &code):
			ev.Text = string(code)
		default:
			ev.Text = string(core.MiniProgramSignInFailed)
		}
	}
	a.post(ev)
}
