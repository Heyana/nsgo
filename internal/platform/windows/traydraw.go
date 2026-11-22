//go:build windows

package windows

import (
	"math"
	"unsafe"
)

// queueTrayDraw coalesces invalidations on the hidden surface's own message queue. Invalidating its HWND alone cannot paint it while it stays hidden.
func (w *window) queueTrayDraw() {
	if w.hwnd == 0 || w.trayDrawPending || w.destroying {
		return
	}
	w.trayDrawPending = true
	w.post(func() {
		w.trayDrawPending = false
		if w.hwnd != 0 && !w.destroying {
			w.drawTray()
		}
	})
}

// drawTray paints the retained tree into an alpha bitmap at the shell's icon size, without a swap chain or a visible window. Async image completions and animated views invalidate this same surface, just as a visible tree does.
func (w *window) drawTray() {
	owner := w.hostWindow
	if owner == nil || w.trayID == 0 || w.canvasClosed {
		return
	}
	tr := owner.trays[w.trayID]
	root := w.view(w.root)
	if tr == nil || tr.cfg.View == 0 || root == nil {
		return
	}
	var client rect
	getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client)))
	if client.empty() || w.scale <= 0 {
		return
	}
	g, err := devices()
	if err != nil {
		return
	}
	const smCxSmallIcon, smCySmallIcon = 49, 50
	dpi := uintptr(math.Round(96 * w.scale))
	pw, _, _ := getSystemMetricsForDpi.Call(smCxSmallIcon, dpi)
	ph, _, _ := getSystemMetricsForDpi.Call(smCySmallIcon, dpi)
	if pw == 0 || ph == 0 || pw > 256 || ph > 256 {
		return
	}
	width, height := float64(client.Right)/w.scale, float64(client.Bottom)/w.scale
	scale := float32(min(float64(pw)/width, float64(ph)/height))
	props := bitmapProperties1{format: dxgiFormatBGRA, alphaMode: d2dAlphaPremultiplied,
		dpiX: 96 * scale, dpiY: 96 * scale, options: d2dBitmapTarget}
	var target *comObject
	if failed(g.dc.callSize(dcCreateBitmap, uint32(pw), uint32(ph), 0, 0, uintptr(unsafe.Pointer(&props)), uintptr(unsafe.Pointer(&target)))) {
		return
	}
	defer target.release()
	var previous *comObject
	g.dc.call(dcGetTarget, uintptr(unsafe.Pointer(&previous)))
	g.dc.call(dcSetTarget, target.ptr())
	g.dc.callF(rtSetDpi, 0b11, f32(96*scale), f32(96*scale))
	g.dc.call(rtBeginDraw)
	clear := colorF{}
	g.dc.call(rtClear, uintptr(unsafe.Pointer(&clear)))
	w.frameDamage(client, true)
	p := newPainter(w, g)
	p.scale = scale
	world := translation((float64(pw)/float64(scale)-width)/2, (float64(ph)/float64(scale)-height)/2)
	f := frameOf(root)
	p.draw(root, world, f.Origin.X, f.Origin.Y)
	p.release()
	hr := g.dc.call(rtEndDraw, 0, 0)
	g.dc.call(dcSetTarget, previous.ptr())
	previous.release()
	if failed(hr) {
		if hr == d2dErrRecreateTarget {
			w.lost()
		}
		return
	}
	pix := bitmapPixels(g.dc, target, uint32(pw), uint32(ph), scale)
	if len(pix) == 0 {
		return
	}
	icon := iconFromPixels(pix, int(pw), int(ph))
	if icon == 0 {
		return
	}
	data := owner.trayData(tr.cfg, icon)
	data.Size, data.Flags = uint32(unsafe.Sizeof(data)), data.Flags|nifIcon
	op := nimModify
	if !tr.added {
		op = nimAdd
	}
	ok, _, _ := shellNotifyIcon.Call(uintptr(op), uintptr(unsafe.Pointer(&data)))
	// Keep the latest rendered icon even before Explorer starts or while it restarts. TaskbarCreated can then publish it without another view change.
	if tr.private && tr.icon != 0 {
		destroyIcon.Call(tr.icon)
	}
	tr.icon, tr.private, tr.drawn = icon, true, true
	if ok == 0 {
		return
	}
	if op == nimAdd {
		tr.added = true
		data.Version = notifyIconVersion
		shellNotifyIcon.Call(nimSetVersion, uintptr(unsafe.Pointer(&data)))
	}
}
