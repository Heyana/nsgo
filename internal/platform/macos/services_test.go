//go:build darwin && !ios

package macos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

func TestSavePanelStartFolder(t *testing.T) {
	dir := t.TempDir()
	var initial string
	onMain(func() {
		loadAppKit()
		objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		panel := savePanel(driver.SaveRequest{Name: "report.txt", Start: dir})
		initial = apple.GoString(panel.Send(sel("directoryURL")).Send(sel("path")))
	})
	got, err := os.Stat(initial)
	want, wantErr := os.Stat(dir)
	if err != nil || wantErr != nil || !os.SameFile(got, want) {
		t.Fatalf("save panel starts in %q, want %q", initial, dir)
	}
}

func TestSavePanelValidationDelegate(t *testing.T) {
	dir := t.TempDir()
	refused := filepath.Join(dir, "report.txt")
	accepted := filepath.Join(dir, "renamed.txt")
	const reason = "The app keeps this name for its reports."
	onMain(func() {
		loadAppKit()
		_ = objc.ID(apple.Class("NSApplication")).Send(sel("sharedApplication"))
		s := &surface{}
		s.engine = engine.Start(actionEngine{}, func(*core.Scope) core.Node { return core.View{} }, engine.Options{
			Event: func(_ uint32, ev driver.Event) {
				if ev.Kind != driver.EventSavePreflight {
					return
				}
				answer := driver.MountItem{Kind: driver.ItemSavePreflightAnswer, Count: ev.Count, Data: ev.Data}
				if answer.On = ev.Text == accepted; !answer.On {
					answer.Value = reason
				}
				go s.Mount(&driver.MountBatch{Items: []driver.MountItem{answer}})
			},
		})
		defer s.engine.Close()
		p := &savePanelPreflight{s: s, id: 1}
		panel := savePanel(driver.SaveRequest{Name: "report.txt", Start: dir})
		delegate := p.delegate(panel)
		defer func() {
			panel.Send(selSetDelegate, objc.ID(0))
			delete(savePanelDelegates, delegate)
			delegate.Send(selRelease)
		}()
		validate := sel("panel:validateURL:error:")
		urlClass := objc.ID(apple.Class("NSURL"))
		var nativeError objc.ID
		url := urlClass.Send(sel("fileURLWithPath:"), apple.NSString(refused))
		if objc.Send[bool](delegate, validate, panel, url, &nativeError) {
			t.Error("the delegate approved the refused path")
		}
		if got := apple.GoString(nativeError.Send(sel("localizedDescription"))); got != reason {
			t.Errorf("refusal = %q, want %q", got, reason)
		}
		if p.refusal.Data != refused || p.refusal.Text != reason || p.refusal.Count != p.id {
			t.Errorf("cancel result = %+v, want the refused path and reason", p.refusal)
		}

		nativeError = 0
		url = urlClass.Send(sel("fileURLWithPath:"), apple.NSString(accepted))
		if !objc.Send[bool](delegate, validate, panel, url, &nativeError) || nativeError != 0 {
			t.Error("the delegate did not approve the second path")
		}
		if p.attempt != 2 || p.approved != accepted || p.refusal.Data != nil {
			t.Errorf("after the retry, attempt = %d, approved = %q, refusal = %+v", p.attempt, p.approved, p.refusal)
		}
	})
}
