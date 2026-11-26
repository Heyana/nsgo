package core

import "github.com/nsgo-dev/nsgo/internal/driver"

// unwrapPlatform unwraps platform wrappers (such as engine's observed) without the wrapper referencing any optional feature enabler interface.
func unwrapPlatform(te driver.TextEngine) any {
	var p any = te
	for {
		u, ok := p.(driver.PlatformUnwrapper)
		if !ok {
			return p
		}
		p = u.UnwrapPlatform()
	}
}

func (t *Tree) enableShaders() {
	if p, ok := unwrapPlatform(t.text).(driver.ShaderEnabler); ok {
		p.EnableShaders()
	}
}

func (t *Tree) enableCanvas() {
	if p, ok := unwrapPlatform(t.text).(driver.CanvasEnabler); ok {
		p.EnableCanvas()
	}
}

func (t *Tree) enableCanvasShaders() {
	if p, ok := unwrapPlatform(t.text).(driver.CanvasShaderEnabler); ok {
		p.EnableCanvasShaders()
	}
}

func (t *Tree) enableVideo() {
	if p, ok := unwrapPlatform(t.text).(driver.VideoEnabler); ok {
		p.EnableVideo()
	}
}

func (t *Tree) enableWebView() {
	if p, ok := unwrapPlatform(t.text).(driver.WebViewEnabler); ok {
		p.EnableWebView()
	}
}

func (t *Tree) enableNative() {
	if p, ok := unwrapPlatform(t.text).(driver.NativeEnabler); ok {
		p.EnableNative()
	}
}
