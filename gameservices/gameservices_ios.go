//go:build ios && !nsgowatch

package gameservices

import "github.com/ebitengine/purego/objc"

func topViewController() objc.ID {
	appCls := objc.GetClass("UIApplication")
	if appCls == 0 || !responds(objc.ID(appCls), sel("sharedApplication")) {
		return 0
	}
	app := objc.ID(appCls).Send(sel("sharedApplication"))
	if app == 0 {
		return 0
	}
	var win objc.ID
	if responds(app, sel("connectedScenes")) {
		scenes := app.Send(sel("connectedScenes"))
		if responds(scenes, sel("allObjects")) {
			arr := scenes.Send(sel("allObjects"))
			n := objc.Send[uint](arr, sel("count"))
			for i := range n {
				scene := arr.Send(sel("objectAtIndex:"), i)
				if !responds(scene, sel("windows")) {
					continue
				}
				wins := scene.Send(sel("windows"))
				wn := objc.Send[uint](wins, sel("count"))
				for j := range wn {
					w := wins.Send(sel("objectAtIndex:"), j)
					if responds(w, sel("isKeyWindow")) && objc.Send[bool](w, sel("isKeyWindow")) {
						win = w
						break
					}
					if win == 0 {
						win = w
					}
				}
				if win != 0 {
					break
				}
			}
		}
	}
	if win == 0 && responds(app, sel("keyWindow")) {
		win = app.Send(sel("keyWindow"))
	}
	if win == 0 || !responds(win, sel("rootViewController")) {
		return 0
	}
	top := win.Send(sel("rootViewController"))
	for top != 0 && responds(top, sel("presentedViewController")) {
		next := top.Send(sel("presentedViewController"))
		if next == 0 {
			break
		}
		top = next
	}
	return top
}

func presentGameCenterVC(vc objc.ID) {
	if vc == 0 {
		return
	}
	top := topViewController()
	if responds(top, sel("presentViewController:animated:completion:")) {
		top.Send(sel("presentViewController:animated:completion:"), vc, true, objc.ID(0))
	}
}

func dismissGameCenterVC(vc objc.ID) {
	if responds(vc, sel("dismissViewControllerAnimated:completion:")) {
		vc.Send(sel("dismissViewControllerAnimated:completion:"), true, objc.ID(0))
	}
}
