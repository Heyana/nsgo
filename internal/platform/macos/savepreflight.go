//go:build darwin && !ios

package macos

import (
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

var (
	classSavePanelDelegate objc.Class
	savePanelDelegates     = map[objc.ID]*savePanelPreflight{}
)

type savePanelPreflight struct {
	s           *surface
	id, attempt uint32
	approved    string
	refusal     driver.Event
}

func (p *savePanelPreflight) delegate(panel objc.ID) objc.ID {
	if classSavePanelDelegate == 0 {
		protocol := objc.GetProtocol("NSOpenSavePanelDelegate")
		classSavePanelDelegate = register("NsgoSavePanelDelegate", apple.Class("NSObject"), []*objc.Protocol{protocol}, []objc.MethodDef{{
			Cmd: sel("panel:validateURL:error:"),
			Fn:  validateSaveURL,
		}})
	}
	delegate := objc.ID(classSavePanelDelegate).Send(selNew)
	savePanelDelegates[delegate] = p
	panel.Send(selSetDelegate, delegate)
	return delegate
}

// AppKit presents the error on the panel and keeps the filename
// editable when the delegate returns false.
func validateSaveURL(self objc.ID, _ objc.SEL, _ objc.ID, url objc.ID, outError *objc.ID) bool {
	p := savePanelDelegates[self]
	path := apple.GoString(url.Send(sel("path")))
	p.attempt++
	p.s.checkingSave.Store(true)
	approved, reason := p.s.saves.Check(p.id, p.attempt, path, p.s.engine.Dispatch)
	p.s.checkingSave.Store(false)

	p.approved = ""
	p.refusal = driver.Event{}
	if approved {
		p.approved = path
		return true
	}
	p.refusal = driver.Event{Kind: driver.EventFiles, Count: p.id, Data: path, Text: reason}
	if outError != nil {
		message := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), apple.NSString(reason), apple.NSString("NSLocalizedDescription"))
		*outError = objc.ID(apple.Class("NSError")).Send(sel("errorWithDomain:code:userInfo:"), apple.NSString("NSGoSavePreflight"), 1, message)
	}
	return false
}
