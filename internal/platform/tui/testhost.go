//go:build nsgotest && ((darwin && !ios) || (linux && !android) || windows)

package tui

import (
	"bytes"
	"fmt"
	"image"
	imagecolor "image/color"
	"image/draw"
	"image/png"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/clipperhouse/uax29/v2/graphemes"
	"github.com/gdamore/tcell/v3"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/testhost"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// The test host uses the same cell renderer and terminal event conversion
// as Run, on an actual console. It owns one surface, as a terminal does;
// additional application windows are not silently emulated.
type host struct {
	mu      sync.Mutex
	surface *surface
	exited  chan struct{}
	pin     engine.TestPin
}

func init()                 { testhost.Register(&host{}) }
func (*host) Several() bool { return false }
func (h *host) Main(run func() int) {
	code := run()
	h.mu.Lock()
	if w := h.surface; w != nil {
		w.stop()
		<-h.exited
		w.engine.Close()
		w.tree.close()
		w.screen.Fini()
	}
	h.mu.Unlock()
	os.Exit(code)
}

func (h *host) Open(app func(*core.Scope) core.Node, o testhost.Options) (testhost.Surface, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.surface != nil {
		return nil, fmt.Errorf("terminal tests open one application per process")
	}
	screen, err := openScreen()
	if err != nil {
		return nil, err
	}
	w := newSurface(screen)
	w.testSize = true
	opts := engine.Options{Environment: terminalEnvironment()}
	opts.UseTest(o, &h.pin)
	w.start(app, opts)
	h.surface = w
	h.exited = make(chan struct{})
	go func() {
		defer close(h.exited)
		if err := w.loop(nil); err != nil {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	return &testSurface{w: w, host: h}, nil
}

type testSurface struct {
	w    *surface
	host *host
}

func (t *testSurface) onLoop(fn func()) error {
	done := make(chan struct{})
	t.w.post(func() { defer close(done); fn() })
	select {
	case <-done:
		return nil
	case <-t.w.done:
		return fmt.Errorf("terminal surface closed")
	case <-time.After(10 * time.Second):
		return fmt.Errorf("terminal event loop did not respond")
	}
}

func (t *testSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	if err := t.onLoop(func() { t.w.engine.InputModality(driver.ModalityPointer); t.w.tree.mouseDown(at, mods) }); err != nil {
		return err
	}
	if hold > 0 {
		time.Sleep(hold)
	}
	return t.onLoop(func() { t.w.tree.mouseUp(at, mods) })
}
func (t *testSurface) RightClick(at driver.Point) error {
	return t.onLoop(func() {
		t.w.engine.InputModality(driver.ModalityPointer)
		t.w.tree.contextMenu(at, driver.Modifiers{})
	})
}
func (t *testSurface) Drag(path []driver.Point) error {
	if len(path) < 2 {
		return fmt.Errorf("terminal drag needs two points")
	}
	if err := t.onLoop(func() { t.w.tree.mouseDown(path[0], driver.Modifiers{}) }); err != nil {
		return err
	}
	for _, p := range path[1:] {
		if err := t.Hover(p); err != nil {
			return err
		}
	}
	return t.onLoop(func() { t.w.tree.mouseUp(path[len(path)-1], driver.Modifiers{}) })
}
func (t *testSurface) Scroll(at, delta driver.Point) error {
	return t.onLoop(func() { t.w.tree.wheel(at, delta) })
}
func (t *testSurface) Hover(at driver.Point) error {
	return t.onLoop(func() { t.w.tree.hover(at, driver.Modifiers{}) })
}
func (t *testSurface) Type(text string) error {
	return t.onLoop(func() {
		it := graphemes.FromString(text)
		for it.Next() {
			_ = t.w.event(tcell.NewEventKey(tcell.KeyRune, it.Value(), 0))
		}
	})
}
func (*testSurface) Compose(string) error { return testhost.ErrNoInputMethod }
func (*testSurface) Commit(string) error  { return testhost.ErrNoInputMethod }
func (t *testSurface) Key(key string) error {
	parts := strings.Split(driver.NormalizeKey(key), "+")
	name := parts[len(parts)-1]
	mods := tcell.ModMask(0)
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "ctrl", "mod":
			mods |= tcell.ModCtrl
		case "alt":
			mods |= tcell.ModAlt
		case "shift":
			mods |= tcell.ModShift
		case "meta":
			mods |= tcell.ModMeta
		}
	}
	keys := map[string]tcell.Key{"enter": tcell.KeyEnter, "tab": tcell.KeyTab, "escape": tcell.KeyEscape, "backspace": tcell.KeyBackspace, "delete": tcell.KeyDelete, "left": tcell.KeyLeft, "right": tcell.KeyRight, "up": tcell.KeyUp, "down": tcell.KeyDown, "home": tcell.KeyHome, "end": tcell.KeyEnd, "pageup": tcell.KeyPgUp, "pagedown": tcell.KeyPgDn}
	k, text := tcell.KeyRune, name
	if v, ok := keys[name]; ok {
		k, text = v, ""
	} else if name == "space" {
		text = " "
	} else if len([]rune(text)) != 1 {
		return fmt.Errorf("unsupported terminal test key %q", key)
	}
	return t.onLoop(func() {
		_ = t.w.event(tcell.NewEventKey(k, text, mods))
		if protocol := t.w.screen.KeyboardProtocol(); protocol == tcell.KittyKeyboard || protocol == tcell.Win32Keyboard {
			_ = t.w.event(tcell.NewEventKeyEx(k, text, mods, false, 0, 1))
		}
	})
}
func (t *testSurface) Back() error { return t.Key("Escape") }
func (*testSurface) Answer(uint32, int) error {
	return fmt.Errorf("terminal backend has no system alerts; use an application dialog")
}
func (*testSurface) ChooseMenu(uint32) error     { return testhost.ErrNoMenu }
func (*testSurface) ChooseDockMenu(uint32) error { return testhost.ErrNoMenu }
func (*testSurface) ChoosePopup(uint32) error    { return testhost.ErrNoMenu }
func (t *testSurface) Animating() bool           { return t.w.batches.Len() > 0 || t.w.frame.Load() }
func (t *testSurface) Resize(size driver.Size) error {
	if size.Width <= 0 || size.Height <= 0 {
		return fmt.Errorf("terminal surface size must be positive")
	}
	return t.onLoop(func() {
		t.w.width, t.w.height = max(int(size.Width/cellWidth), 1), max(int(size.Height/cellHeight), 1)
		t.w.previous = nil
		t.w.engine.Resize(size, driver.Geometry{})
	})
}
func (t *testSurface) SetEnvironment(env driver.Environment) error {
	t.host.pin.Set(&env)
	t.w.engine.Repin()
	return nil
}
func (t *testSurface) Sync() { t.w.engine.Sync(); _ = t.onLoop(func() {}) }

func pixelColor(c driver.Color, fallback imagecolor.RGBA) imagecolor.RGBA {
	if c&255 == 0 {
		return fallback
	}
	return imagecolor.RGBA{R: uint8(c >> 24), G: uint8(c >> 16), B: uint8(c >> 8), A: 255}
}

// Screenshot records the cell content, not the terminal emulator's choice
// of font. The built-in raster face is intentionally fixed and portable.
func (t *testSurface) Screenshot() ([]byte, error) {
	var g *grid
	if err := t.onLoop(func() { g = t.w.tree.render(t.w.width, t.w.height) }); err != nil {
		return nil, err
	}
	parsed, err := opentype.Parse(gomono.TTF)
	if err != nil {
		return nil, err
	}
	face, err := opentype.NewFace(parsed, &opentype.FaceOptions{Size: 13.333333, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		return nil, err
	}
	defer face.Close()
	img := image.NewRGBA(image.Rect(0, 0, g.width*8, g.height*16))
	for y := range g.height {
		for x := range g.width {
			c := g.at(x, y)
			bg := pixelColor(c.bg, imagecolor.RGBA{A: 255})
			draw.Draw(img, image.Rect(x*8, y*16, (x+1)*8, (y+1)*16), image.NewUniform(bg), image.Point{}, draw.Src)
		}
	}
	for y := range g.height {
		for x := range g.width {
			c := g.at(x, y)
			if c.width == 0 {
				continue
			}
			d := font.Drawer{Dst: img, Src: image.NewUniform(pixelColor(c.fg, imagecolor.RGBA{R: 230, G: 230, B: 230, A: 255})), Face: face, Dot: fixed.P(x*8, y*16+13)}
			d.DrawString(c.text)
		}
	}
	var out bytes.Buffer
	err = png.Encode(&out, img)
	return out.Bytes(), err
}
