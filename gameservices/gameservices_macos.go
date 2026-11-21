//go:build darwin && !ios && !nsgowatch

package gameservices

import "github.com/ebitengine/purego/objc"

func presentGameCenterVC(vc objc.ID) {
	if vc == 0 {
		return
	}
	dcCls := objc.GetClass("GKDialogController")
	if dcCls == 0 || !responds(objc.ID(dcCls), sel("sharedDialogController")) {
		return
	}
	dc := objc.ID(dcCls).Send(sel("sharedDialogController"))
	if dc == 0 {
		return
	}
	if appCls := objc.GetClass("NSApplication"); appCls != 0 && responds(objc.ID(appCls), sel("sharedApplication")) {
		app := objc.ID(appCls).Send(sel("sharedApplication"))
		var win objc.ID
		if responds(app, sel("keyWindow")) {
			win = app.Send(sel("keyWindow"))
		}
		if win == 0 && responds(app, sel("mainWindow")) {
			win = app.Send(sel("mainWindow"))
		}
		if win != 0 && responds(dc, sel("setParentWindow:")) {
			dc.Send(sel("setParentWindow:"), win)
		}
	}
	if responds(dc, sel("presentViewController:")) {
		dc.Send(sel("presentViewController:"), vc)
	}
}

func dismissGameCenterVC(vc objc.ID) {
	if vc == 0 {
		return
	}
	dcCls := objc.GetClass("GKDialogController")
	if dcCls == 0 || !responds(objc.ID(dcCls), sel("sharedDialogController")) {
		return
	}
	dc := objc.ID(dcCls).Send(sel("sharedDialogController"))
	if responds(dc, sel("dismiss:")) {
		dc.Send(sel("dismiss:"), vc)
	}
}
