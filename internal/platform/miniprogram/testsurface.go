//go:build !js || nsgoautomation

package miniprogram

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

// liveSurface drives a mini program's mounter as a user does for nsgo dev -automation. Mini programs give the logic layer no API to post synthetic touches into the render layer, so input enters the mounter directly: hit testing, presses, long presses, touches, scrolls, field focus and edits dispatch through the same handlers the render layer's events call, and update the render layer's ops (fo, iv, sel, so) so the simulator stays in sync.
type liveSurface struct {
	m          *mounter
	run        func(func())
	syncFn     func()
	screenshot func() ([]byte, error)
	seq        int
}

var _ testhost.Surface = (*liveSurface)(nil)

func (t *liveSurface) exec(fn func()) {
	if t.run != nil {
		t.run(fn)
		return
	}
	fn()
}

// liveHit returns the view a touch at page point at lands on, as the render layer hits it (hitAt), or the nearest of its ancestors that match takes, nil for none; it stops at a modal, which hides what is behind it.
func (m *mounter) liveHit(at driver.Point, match func(*view) bool) *view {
	for v := m.hitAt(at); v != nil; v = v.parent {
		if match(v) {
			return v
		}
		if v.kind == driver.KindModal {
			break
		}
	}
	return nil
}

func (t *liveSurface) Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error {
	if mods != (driver.Modifiers{}) {
		return errors.New("mini programs have no keyboard modifier input")
	}
	var pressTarget driver.NodeID
	t.exec(func() {
		if v := t.m.liveHit(at, func(v *view) bool {
			return v.events.Has(driver.EventPressDown) || v.events.Has(driver.EventPressUp)
		}); v != nil {
			pressTarget = v.id
			t.m.press(v.id, driver.EventPressDown, at)
		}
	})
	if hold > 0 {
		time.Sleep(hold)
	}
	t.exec(func() {
		if pressTarget != 0 {
			t.m.press(pressTarget, driver.EventPressUp, at)
		}
		if hold > 0 {
			if v := t.m.liveHit(at, func(v *view) bool { return v.events.Has(driver.EventLongPress) }); v != nil {
				t.m.press(v.id, driver.EventLongPress, at)
			}
			return
		}
		if v := t.m.liveHit(at, func(v *view) bool { return v.events.Has(driver.EventPress) }); v != nil {
			t.m.press(v.id, driver.EventPress, at)
		}
		// Tapping a text field focuses it; tapping elsewhere blurs the focused field.
		if input := t.m.liveHit(at, func(v *view) bool { return v.kind == driver.KindInput && !v.inputDisabled }); input != nil {
			if t.m.focused != input {
				if prev := t.m.focused; prev != nil {
					t.m.o.op("fo", prev.id).boolean(false)
					t.m.field(prev.id, driver.EventBlur, "")
				}
				t.m.focused = input
				t.m.o.op("fo", input.id).boolean(true)
				t.m.field(input.id, driver.EventFocus, "")
				t.m.flush()
			}
		} else if prev := t.m.focused; prev != nil {
			t.m.o.op("fo", prev.id).boolean(false)
			t.m.field(prev.id, driver.EventBlur, "")
			t.m.flush()
		}
	})
	return nil
}

func (t *liveSurface) RightClick(driver.Point) error {
	return errors.New("a mini program has no secondary click: use LongPress")
}

// Drag moves a finger along path through the mounter's touch handler, 10 points per step, resting still before lifting so pans end without a fling.
func (t *liveSurface) Drag(path []driver.Point) error {
	if len(path) == 0 {
		return errors.New("an empty drag")
	}
	var target driver.NodeID
	t.exec(func() {
		v := t.m.liveHit(path[0], func(v *view) bool {
			return t.m.nearest(v, driver.EventPan, driver.EventTouch, driver.EventPinch, driver.EventRotation) != nil
		})
		if v != nil {
			target = v.id
		}
	})
	if target == 0 {
		return nil
	}
	now := time.Duration(0)
	send := func(phase driver.GesturePhase, pt driver.Point) {
		t.exec(func() {
			t.seq++
			f := []finger{{id: 1, page: pt}}
			all := f
			if phase == driver.GestureEnded || phase == driver.GestureCancelled {
				all = nil
			}
			t.m.touch(target, t.seq, phase, f, all, now)
		})
	}
	send(driver.GestureBegan, path[0])
	at := path[0]
	for _, to := range path[1:] {
		n := int(math.Max(math.Abs(float64(to.X-at.X)), math.Abs(float64(to.Y-at.Y)))/10) + 1
		for i := 1; i <= n; i++ {
			now += 16 * time.Millisecond
			frac := driver.Points(i) / driver.Points(n)
			send(driver.GestureChanged, driver.Point{
				X: at.X + (to.X-at.X)*frac,
				Y: at.Y + (to.Y-at.Y)*frac,
			})
		}
		at = to
	}
	for range 6 {
		now += 16 * time.Millisecond
		send(driver.GestureChanged, at)
	}
	now += 16 * time.Millisecond
	send(driver.GestureEnded, at)
	return nil
}

// Scroll moves the innermost scroll view under at by delta, clamping to its content bounds, updating the render layer and dispatching the scroll events.
func (t *liveSurface) Scroll(at driver.Point, delta driver.Point) error {
	var err error
	t.exec(func() {
		if t.m.view(t.m.root) == nil && len(t.m.modals) == 0 {
			err = errors.New("nothing shows")
			return
		}
		v := t.m.liveHit(at, func(v *view) bool { return v.kind == driver.KindScroll })
		if v == nil {
			err = fmt.Errorf("no scroll view at %v", at)
			return
		}
		to := clampScrollOffset(v, driver.Point{X: v.offset.X + delta.X, Y: v.offset.Y + delta.Y})
		t.m.o.op("so", v.id).pt(to.X).pt(to.Y).boolean(false)
		t.m.flush()
		t.m.scrolled(v.id, to)
		t.m.scrollPhase(v.id, driver.EventScrollEnd)
	})
	return err
}

func clampScrollOffset(v *view, to driver.Point) driver.Point {
	maxX := max(driver.Points(0), v.contentSize.Width-v.frame.Size.Width)
	maxY := max(driver.Points(0), v.contentSize.Height-v.frame.Size.Height)
	to.X = min(max(to.X, 0), maxX)
	to.Y = min(max(to.Y, 0), maxY)
	return to
}

func (t *liveSurface) Hover(driver.Point) error { return nil }

// Type enters text into the focused field, a character at a time, updating both the mounter and the render layer's input value.
func (t *liveSurface) Type(text string) error {
	var err error
	t.exec(func() {
		v := t.m.focused
		if v == nil {
			err = errors.New("no field has the focus")
			return
		}
		if v.inputDisabled {
			err = errors.New("the focused field is disabled")
			return
		}
		for _, r := range text {
			var next string
			if v.selectedAll {
				next = string(r)
				v.selectedAll = false
			} else {
				next = v.value + string(r)
			}
			if v.maxLength > 0 && utf8.RuneCountInString(next) > v.maxLength {
				break
			}
			t.m.edited(v.id, next)
			t.m.o.op("iv", v.id).str(next)
		}
		t.m.flush()
	})
	return err
}

func (t *liveSurface) Compose(string) error { return testhost.ErrNoInputMethod }
func (t *liveSurface) Commit(string) error  { return testhost.ErrNoInputMethod }

// Key handles editing keys in the focused field (Enter, Backspace, mod+a for Clear) and Escape as Back.
func (t *liveSurface) Key(key string) error {
	var err error
	t.exec(func() {
		lower := strings.ToLower(key)
		if lower == "escape" {
			t.m.h.dispatch(driver.Event{Kind: driver.EventBack})
			return
		}
		v := t.m.focused
		if v == nil {
			err = errors.New("no field has the focus")
			return
		}
		switch lower {
		case "enter", "return":
			t.m.field(v.id, driver.EventSubmitEditing, v.value)
		case "mod+a", "cmd+a", "ctrl+a":
			v.selectedAll = true
			end := driver.UTF16Offset(v.value, len(v.value))
			t.m.o.op("sel", v.id).num(0).num(float64(end))
			t.m.flush()
		case "backspace":
			if v.inputDisabled {
				err = errors.New("the focused field is disabled")
				return
			}
			if v.selectedAll {
				v.selectedAll = false
				if v.value != "" {
					t.m.edited(v.id, "")
					t.m.o.op("iv", v.id).str("")
					t.m.flush()
				}
				return
			}
			if _, n := utf8.DecodeLastRuneInString(v.value); n > 0 {
				next := v.value[:len(v.value)-n]
				t.m.edited(v.id, next)
				t.m.o.op("iv", v.id).str(next)
				t.m.flush()
			}
		default:
			err = fmt.Errorf("a mini program has no %s key", key)
		}
	})
	return err
}

func (t *liveSurface) Back() error {
	t.exec(func() {
		t.m.h.dispatch(driver.Event{Kind: driver.EventBack})
	})
	return nil
}

func (t *liveSurface) Answer(id uint32, index int) error {
	var err error
	t.exec(func() {
		c, ok := t.m.alerts[id]
		if !ok {
			err = fmt.Errorf("no alert %d shows", id)
			return
		}
		if index < -1 || index >= len(c.Actions) {
			err = fmt.Errorf("the alert has no button %d", index)
			return
		}
		t.m.answerAlert(id, index)
	})
	return err
}

func (t *liveSurface) ChooseMenu(uint32) error     { return testhost.ErrNoMenu }
func (t *liveSurface) ChooseDockMenu(uint32) error { return testhost.ErrNoMenu }
func (t *liveSurface) ChoosePopup(uint32) error    { return testhost.ErrNoMenu }

func (t *liveSurface) Screenshot() ([]byte, error) {
	if t.screenshot != nil {
		if data, err := t.screenshot(); err == nil && len(data) > 0 {
			return data, nil
		}
	}
	var data []byte
	var err error
	t.exec(func() {
		data, err = t.m.rasterizePNG()
	})
	return data, err
}

func (t *liveSurface) Animating() bool {
	var busy bool
	t.exec(func() {
		busy = t.m.animator.Ending()
	})
	return busy
}

func (t *liveSurface) Resize(driver.Size) error { return nil }

func (t *liveSurface) SetEnvironment(driver.Environment) error {
	return errors.New("a running app follows the system's settings")
}

func (t *liveSurface) Sync() {
	if t.syncFn != nil {
		t.syncFn()
	}
}

// rasterizePNG renders a fallback software PNG of the mounted view tree (backgrounds, borders, clipping, scroll offsets and modals) when an offscreen 2D canvas is unavailable, such as in host unit tests.
func (m *mounter) rasterizePNG() ([]byte, error) {
	size := m.viewport
	if root := m.view(m.root); root != nil && (size.Width <= 0 || size.Height <= 0) {
		size = root.frame.Size
	}
	w, h := int(math.Ceil(float64(size.Width))), int(math.Ceil(float64(size.Height)))
	if w <= 0 || h <= 0 {
		return nil, errors.New("nothing shows")
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	fillRect(img, img.Bounds(), color.RGBA{R: 255, G: 255, B: 255, A: 255})
	if root := m.view(m.root); root != nil {
		rasterizeView(img, root, driver.Point{}, img.Bounds())
	}
	for _, modal := range m.modals {
		rasterizeView(img, modal, driver.Point{}, img.Bounds())
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func rasterizeView(img *image.RGBA, v *view, origin driver.Point, clip image.Rectangle) {
	if v == nil || v.style.Hidden || clip.Empty() {
		return
	}
	f := v.frame
	f.Origin.X += v.shift.X
	f.Origin.Y += v.shift.Y
	x := origin.X + f.Origin.X
	y := origin.Y + f.Origin.Y
	rect := image.Rect(
		int(math.Round(float64(x))),
		int(math.Round(float64(y))),
		int(math.Round(float64(x+f.Size.Width))),
		int(math.Round(float64(y+f.Size.Height))),
	)
	if bg := driverRGBA(v.style.Background); bg.A > 0 {
		fillRect(img, rect.Intersect(clip), bg)
	}
	if b := v.border(); b != (driver.Edges{}) {
		bc := driverRGBA(v.style.BorderColor)
		if bc.A > 0 {
			if t := int(math.Round(float64(b.Top))); t > 0 {
				fillRect(img, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+t).Intersect(clip), bc)
			}
			if bt := int(math.Round(float64(b.Bottom))); bt > 0 {
				fillRect(img, image.Rect(rect.Min.X, rect.Max.Y-bt, rect.Max.X, rect.Max.Y).Intersect(clip), bc)
			}
			if l := int(math.Round(float64(b.Left))); l > 0 {
				fillRect(img, image.Rect(rect.Min.X, rect.Min.Y, rect.Min.X+l, rect.Max.Y).Intersect(clip), bc)
			}
			if r := int(math.Round(float64(b.Right))); r > 0 {
				fillRect(img, image.Rect(rect.Max.X-r, rect.Min.Y, rect.Max.X, rect.Max.Y).Intersect(clip), bc)
			}
		}
	}
	childClip := clip
	if v.style.Clip || v.kind == driver.KindScroll {
		childClip = clip.Intersect(rect)
	}
	childOrigin := driver.Point{X: x, Y: y}
	if v.kind == driver.KindScroll {
		childOrigin.X -= v.offset.X
		childOrigin.Y -= v.offset.Y
	}
	for _, c := range v.children {
		if c.kind == driver.KindModal {
			continue
		}
		rasterizeView(img, c, childOrigin, childClip)
	}
}

func driverRGBA(c driver.Color) color.RGBA {
	return color.RGBA{
		R: uint8(c >> 24),
		G: uint8(c >> 16),
		B: uint8(c >> 8),
		A: uint8(c),
	}
}

func fillRect(img *image.RGBA, r image.Rectangle, src color.RGBA) {
	if r.Empty() || src.A == 0 {
		return
	}
	if src.A == 255 {
		for y := r.Min.Y; y < r.Max.Y; y++ {
			for x := r.Min.X; x < r.Max.X; x++ {
				img.SetRGBA(x, y, src)
			}
		}
		return
	}
	a := uint32(src.A)
	inv := 255 - a
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			dst := img.RGBAAt(x, y)
			img.SetRGBA(x, y, color.RGBA{
				R: uint8((uint32(src.R)*a + uint32(dst.R)*inv) / 255),
				G: uint8((uint32(src.G)*a + uint32(dst.G)*inv) / 255),
				B: uint8((uint32(src.B)*a + uint32(dst.B)*inv) / 255),
				A: uint8(min(255, a+uint32(dst.A)*inv/255)),
			})
		}
	}
}
