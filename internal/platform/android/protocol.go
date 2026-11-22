package android

import (
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"math"
	"net/url"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// This file holds what builds on every system: the protocol's encoding and
// the encoders of animations, so tests can check them against the Java
// runtime on a machine with a JDK.

// maxMessage is the largest payload Java's reader takes (Protocol.MAX_MESSAGE_SIZE); it ends the process on a longer one as on a corrupt pipe, so the writer refuses it instead.
const maxMessage = 64 << 20

// checkMessage reports an error for a message of kind whose payload is n bytes, more than Java reads.
func checkMessage(kind byte, n int) error {
	if n > maxMessage {
		return fmt.Errorf("a message (kind %d) of %d bytes is over the %d MiB the Android runtime reads", kind, n, maxMessage>>20)
	}
	return nil
}

// maxShaderPixels is the most bytes of shader pixels a batch carries, leaving room in its message for the rest of the batch (maxMessage). An image over it never goes.
const maxShaderPixels = maxMessage / 2

// shaderPixels says which images of a shader item send their bytes, given the copies Java holds (sent) and the bytes the batch has left for pixels (budget). It returns which images send their bytes, what Java holds after the batch, the remaining budget and whether a follow-up batch needs to send any new copy.
func shaderPixels(images []driver.ImageSource, sent []*image.RGBA, budget int) (fresh []bool, held []*image.RGBA, left int, deferred bool) {
	fresh = make([]bool, len(images))
	held = make([]*image.RGBA, len(images))
	for i, img := range images {
		var had *image.RGBA
		if i < len(sent) {
			had = sent[i]
		}
		px := img.Pixels
		held[i] = px
		if px == nil || px == had {
			continue
		}
		n := 4 * px.Rect.Dx() * px.Rect.Dy()
		if n > maxShaderPixels {
			// writeShader leaves it out and logs it.
			fresh[i] = true
			continue
		}
		if n > budget {
			held[i] = had
			deferred = true
			continue
		}
		fresh[i] = true
		budget -= n
	}
	return fresh, held, budget, deferred
}

// Native components on Android depart from NSH's, whose components are
// Java objects the embedding app registers: the Go process cannot make
// Android views, so a component's View returns the fully qualified name
// of an android.view.View subclass, such as "android.widget.SeekBar".
// The runtime builds it by reflection with its (Context) constructor,
// sets each prop by calling its public one-argument set method (prop
// "rating" calls setRating) with the JSON value coerced to the parameter,
// runs a command by calling the public method of that name with the
// arguments, reports the view's measured size as EventNativeSize and
// reports each callback of the listeners its class declares (its own
// setOn…Listener methods, not View's) as an event named after the
// callback, its arguments in a JSON array. The Go component sees every
// prop and command too, and may implement core.AndroidPropMapper and
// core.AndroidEventMapper to translate them.

// writeLayoutAnimation writes a batch's layout animation: whether there
// is one, then each phase as LayoutTransition.read reads it.
func writeLayoutAnimation(w *writer, a *driver.LayoutAnimation) {
	if a == nil {
		w.byte(0)
		return
	}
	w.byte(1)
	for _, tr := range [...]*driver.LayoutTransition{a.Create, a.Update, a.Delete} {
		writeLayoutTransition(w, tr)
	}
}

// writeLayoutTransition writes a layout animation's phase or a frame's
// transition: whether there is one, then what LayoutTransition.read reads.
func writeLayoutTransition(w *writer, tr *driver.LayoutTransition) {
	if tr == nil {
		w.byte(0)
		return
	}
	w.byte(1)
	w.byte(byte(tr.Curve))
	w.float(tr.Damping)
	w.floats(tr.Bezier.X1, tr.Bezier.Y1, tr.Bezier.X2, tr.Bezier.Y2)
	w.byte(byte(tr.Property))
	w.u64(uint64(tr.Duration))
	w.u64(uint64(tr.Delay))
}

// writeKeyframes writes the steps that Keyframes.read loads and eases between.
func writeKeyframes(w *writer, a driver.KeyframeAnimation) {
	d, frames := a.Cycle()
	w.u32(uint32(max(d.Milliseconds(), 0)))
	w.u32(uint32(max(a.Delay.Milliseconds(), 0)))
	w.u32(uint32(max(a.Iterations, 0)))
	w.byte(byte(len(frames)))
	for _, k := range frames {
		w.floats(k.Offset, k.Opacity, k.Scale, k.Rotate, k.TranslateY, k.Easing.X1, k.Easing.Y1, k.Easing.X2, k.Easing.Y2)
	}
}

// writeAnimation writes an ItemAnimation's ops as Animator.readOp reads
// them. Numbers stay float64, so Java computes what core.Animator would.
func writeAnimation(w *writer, ops []driver.AnimationOp) {
	w.u32(uint32(len(ops)))
	for i := range ops {
		o := &ops[i]
		w.byte(byte(o.Kind))
		w.u32(o.Node)
		switch o.Kind {
		case driver.AnimCreateValue, driver.AnimSet, driver.AnimSetOffset:
			w.f64(o.Value)
		case driver.AnimCreateInterpolate:
			w.u32(o.Input)
			writeInterpolation(w, o.Interpolation)
		case driver.AnimCreateMath:
			w.byte(byte(o.Math))
			w.u32(o.A.Node)
			w.f64(o.A.Const)
			w.u32(o.B.Node)
			w.f64(o.B.Const)
		case driver.AnimCreateDiffClamp:
			w.u32(o.Input)
			w.f64(o.Min)
			w.f64(o.Max)
			w.flags(o.Snap)
		case driver.AnimCreateRubberBand:
			w.u32(o.Input)
			w.f64(o.Min)
			w.f64(o.Max)
			w.f64(o.Value)
		case driver.AnimRead, driver.AnimStop, driver.AnimReset, driver.AnimForgetRun:
			w.u32(o.ID)
		case driver.AnimStart:
			w.u32(o.ID)
			w.flags(o.Animation != nil)
			if o.Animation != nil {
				writeSpec(w, o.Animation)
				w.f64(o.Animation.FrameRate)
			}
		case driver.AnimBind:
			w.u32(uint32(o.View))
			w.byte(byte(o.Prop))
		case driver.AnimBindPan, driver.AnimBindScroll:
			w.u32(uint32(o.View))
			w.u32(o.X)
			w.u32(o.Y)
		case driver.AnimBindPinch, driver.AnimBindRotation, driver.AnimBindPull:
			w.u32(uint32(o.View))
			w.u32(o.Node)
		}
	}
}

func writeInterpolation(w *writer, r *driver.Interpolation) {
	if r == nil {
		r = &driver.Interpolation{}
	}
	w.u32(uint32(len(r.Input)))
	for _, x := range r.Input {
		w.f64(x)
	}
	w.u32(uint32(len(r.Output)))
	for _, x := range r.Output {
		w.f64(x)
	}
	w.u32(uint32(len(r.Colors)))
	for _, c := range r.Colors {
		w.u32(uint32(c))
	}
	w.byte(byte(r.Extrapolate))
	w.byte(byte(r.ExtrapolateLeft))
	w.byte(byte(r.ExtrapolateRight))
	writeEasing(w, r.Easing)
}

func writeEasing(w *writer, e driver.Easing) {
	w.byte(byte(e.Kind))
	w.byte(byte(e.Mode))
	w.floats(e.Bezier.X1, e.Bezier.Y1, e.Bezier.X2, e.Bezier.Y2)
	w.f64(e.Param)
	w.byte(byte(e.Steps))
}

// writeSpec writes an animation tree as Animator.Spec.read reads it; a
// nil step is kind 0, which Java runs as an empty delay.
func writeSpec(w *writer, a *driver.Animation) {
	if a == nil {
		a = &driver.Animation{}
	}
	w.byte(byte(a.Kind))
	w.u32(a.Value)
	w.f64(a.To)
	w.u64(uint64(a.Timing.Duration))
	w.u64(uint64(a.Timing.Delay))
	writeEasing(w, a.Timing.Easing)
	sp, d := a.Spring, a.Decay
	w.f64(sp.Stiffness)
	w.f64(sp.Damping)
	w.f64(sp.Mass)
	w.f64(sp.Velocity)
	w.f64(d.Velocity)
	w.f64(d.Deceleration)
	w.flags(d.Clamp)
	w.f64(d.Min)
	w.f64(d.Max)
	w.f64(d.Bounce)
	w.u64(uint64(a.Delay))
	// A loop's iterations, its NoReset in the top bit.
	// Below 1 is forever, which 0 says.
	iterations := uint32(min(max(a.Iterations, 0), 1<<31-1))
	if a.NoReset {
		iterations |= 1 << 31
	}
	w.u32(iterations)
	w.u32(uint32(len(a.Steps)))
	for _, st := range a.Steps {
		writeSpec(w, st)
	}
}

// writeGradient writes a fill or stroke gradient as CanvasNode reads it:
// its kind, 0 linear, 1 radial and 2 conic, first.
func writeGradient(w *writer, g *driver.CanvasGradient) {
	if g == nil {
		g = &driver.CanvasGradient{}
	}
	switch {
	case g.Conic:
		w.byte(2)
	case g.Radial:
		w.byte(1)
	default:
		w.byte(0)
	}
	w.floats(g.X1, g.Y1, g.R1, g.X2, g.Y2, g.R2, g.Angle)
	w.floats(g.Transform[:]...)
	w.byte(byte(g.Spread))
	w.u32(uint32(len(g.Stops)))
	for _, st := range g.Stops {
		w.float(st.Offset)
		w.u32(uint32(st.Color))
	}
}

// What a msgSurface reports, after the surface's ID, as Protocol.java's SURFACE_*.
const (
	// surfaceState is the window's size and place in points, then the environment as msgEnvironment's; the first attaches the tree.
	surfaceState = 1
	// surfaceEnded is the end Java gave the overlay, an OverlayEnd* of driver's.
	surfaceEnded = 2
)

// What writeWindow sends of an overlay's ItemWindow, as Protocol.java's WINDOW_*: the commands an overlay takes, and windowNone for the rest, which it ignores.
const (
	windowNone    = 0
	windowFrame   = 1
	windowShow    = 2
	windowHide    = 3
	windowFront   = 4
	windowDestroy = 5
)

// writeSurface writes an ItemOpenSurface as NsgoView reads it: the surface's ID, its size and place in points, then whether it is an overlay, the only kind Java opens, whether it is placed, hidden, focusable, passive and accessible.
func writeSurface(w *writer, c *driver.SurfaceConfig) {
	w.u32(c.ID)
	w.points(c.Width)
	w.points(c.Height)
	w.points(c.Frame.X)
	w.points(c.Frame.Y)
	w.flags(c.Kind == driver.SurfaceOverlay, c.Frame.Move, c.Hidden, c.Focusable, c.Passive, c.Accessible)
}

// writeWindow writes an ItemWindow as an overlay's view reads it: its command (windowFrame, ...), then the frame it sets in points, whether it moves and whether it resizes.
func writeWindow(w *writer, c *driver.WindowCommand) {
	op := byte(windowNone)
	switch c.Action {
	case driver.WindowSetFrame:
		op = windowFrame
	case driver.WindowShow:
		op = windowShow
	case driver.WindowHide:
		op = windowHide
	case driver.WindowOrderFront:
		op = windowFront
	case driver.WindowDestroy:
		op = windowDestroy
	}
	w.byte(op)
	f := c.Frame
	w.points(f.X)
	w.points(f.Y)
	w.points(f.Width)
	w.points(f.Height)
	w.flags(f.Move, f.Width > 0 && f.Height > 0)
}

// spoken returns the words TalkBack announces for the roles and states it has no word of its own for, in lang, by Protocol.java's SPOKEN_* indexes: Android's classes name buttons, checkboxes and fields in the system's language, but nothing names a link, a tab or a table, as Material's tabs name themselves from resources of their own.
func spoken(lang string) []string {
	m := locales.In(lang)
	return []string{
		m.RoleLink(), m.RoleTab(), m.RoleSearchBox(), m.RoleStatus(), m.RoleMenu(), m.RoleMenuItem(), m.RoleDialog(), m.RoleAlert(),
		m.RoleTable(), m.RoleGrid(), m.RoleRow(), m.RoleColumnHeader(), m.RoleCell(),
		m.StatePartiallyChecked(), m.StateExpanded(), m.StateCollapsed(), m.SpokenSeparator(),
	}
}

// writeDirection writes ItemDirection as NsgoView.apply reads it: the direction, then the spoken words in the language the app shows.
func writeDirection(w *writer, it *driver.MountItem) {
	w.flags(it.RTL)
	words := spoken(it.Language)
	w.u32(uint32(len(words)))
	for _, s := range words {
		w.str(s)
	}
}

// writer and reader are the protocol's encoding: big-endian, lengths
// as uint32, points as float32, strings as a length and UTF-8 bytes and
// booleans as the bits of one byte, the first lowest.
type writer struct{ buf []byte }

func (w *writer) byte(b byte)            { w.buf = append(w.buf, b) }
func (w *writer) u16(v uint16)           { w.buf = binary.BigEndian.AppendUint16(w.buf, v) }
func (w *writer) u32(v uint32)           { w.buf = binary.BigEndian.AppendUint32(w.buf, v) }
func (w *writer) u64(v uint64)           { w.buf = binary.BigEndian.AppendUint64(w.buf, v) }
func (w *writer) float(f float32)        { w.u32(math.Float32bits(f)) }
func (w *writer) f64(f float64)          { w.u64(math.Float64bits(f)) }
func (w *writer) points(p driver.Points) { w.float(float32(p)) }
func (w *writer) str(s string)           { w.u32(uint32(len(s))); w.buf = append(w.buf, s...) }

func (w *writer) floats(fs ...float32) {
	for _, f := range fs {
		w.float(f)
	}
}

func (w *writer) bounds(b driver.Bounds) {
	w.points(b.Origin.X)
	w.points(b.Origin.Y)
	w.points(b.Size.Width)
	w.points(b.Size.Height)
}

func (w *writer) flags(bits ...bool) {
	var b byte
	for i, on := range bits {
		if on {
			b |= 1 << i
		}
	}
	w.byte(b)
}

type reader struct{ buf []byte }

func (r *reader) byte() byte {
	if len(r.buf) < 1 {
		return 0
	}
	b := r.buf[0]
	r.buf = r.buf[1:]
	return b
}

func (r *reader) u32() uint32 {
	if len(r.buf) < 4 {
		return 0
	}
	v := binary.BigEndian.Uint32(r.buf)
	r.buf = r.buf[4:]
	return v
}

func (r *reader) u64() uint64 {
	if len(r.buf) < 8 {
		return 0
	}
	v := binary.BigEndian.Uint64(r.buf)
	r.buf = r.buf[8:]
	return v
}

func (r *reader) float() float32        { return math.Float32frombits(r.u32()) }
func (r *reader) points() driver.Points { return driver.Points(r.float()) }

// bytes reads a length and that many bytes.
func (r *reader) bytes() []byte {
	n := min(int(r.u32()), len(r.buf))
	b := r.buf[:n:n]
	r.buf = r.buf[n:]
	return b
}

func (r *reader) str() string {
	n := int(r.u32())
	if n > len(r.buf) {
		n = len(r.buf)
	}
	s := string(r.buf[:n])
	r.buf = r.buf[n:]
	return s
}

// readResize reads a msgResize: the size and the geometry measured at it, then the environment that changed with them, nil if none did.
func readResize(r *reader) (driver.Size, driver.Geometry, *driver.Environment) {
	size, folds := readSize(r)
	g := foldGeometry(size, folds)
	if len(r.buf) == 0 {
		return size, g, nil
	}
	env := readEnvironment(r)
	return size, g, &env
}

// readEnvironment reads the environment NsgoView.environment writes.
func readEnvironment(r *reader) driver.Environment {
	env := driver.Environment{
		OS:             driver.OSAndroid,
		HasWebView:     true,
		SafeArea:       driver.Edges{Top: r.points(), Right: r.points(), Bottom: r.points(), Left: r.points()},
		KeyboardHeight: r.points(),
		FontScale:      r.float(),
		PixelRatio:     r.float(),

		// The runtime's alarms show them, its receiver setting them again after a reboot.
		ScheduledNotifications: true,
		Gamepads:               true,
	}
	flags := r.byte()
	env.Dark = flags&1 != 0
	env.ReduceMotion = flags&2 != 0
	env.IncreaseContrast = flags&128 != 0
	env.ScreenReader = flags&4 != 0
	env.RTL = flags&8 != 0
	env.Camera = flags&64 != 0
	switch {
	case flags&32 != 0:
		env.Lifecycle = driver.LifecycleBackground
	case flags&16 != 0:
		env.Lifecycle = driver.LifecycleInactive
	}
	// The first byte's bits are all taken; a second holds what else the device can do.
	more := r.byte()
	env.Speak = more&1 != 0
	// A camera the app may read frames of: one the device has, and CAMERA in the app's manifest.
	env.ScanCode = more&2 != 0
	env.CanRequestReview = more&4 != 0
	if more&8 != 0 {
		env.Network = &driver.Network{Online: more&16 != 0, Expensive: more&32 != 0}
	}
	env.Idiom = driver.Idiom(r.byte())
	env.Locale = r.str()
	env.DeviceName = r.str()
	env.DeviceModel = r.str()
	// The toolbar's heights, without and with a large title.
	env.HeaderHeight, env.LargeHeaderHeight = r.points(), r.points()
	env.OSVersion.Major = int(r.u32()) // the API level

	switch r.byte() {
	case 1: // none
		env.ThermalState = driver.ThermalNominal
	case 2, 3: // light, moderate
		env.ThermalState = driver.ThermalFair
	case 4: // severe
		env.ThermalState = driver.ThermalSerious
	case 5, 6, 7: // critical, emergency, shutdown
		env.ThermalState = driver.ThermalCritical
	}
	switch r.byte() {
	case 1:
		env.LowPowerMode = driver.LowPowerOff
	case 2:
		env.LowPowerMode = driver.LowPowerOn
	}
	if len(r.buf) > 0 {
		if first := int(r.byte()); first >= 1 && first <= 7 {
			env.LocaleSettings.FirstDayOfWeek = first
		}
		env.LocaleSettings.HourCycle = r.str()
		env.LocaleSettings.MeasurementSystem = r.str()
		env.LocaleSettings.TemperatureUnit = r.str()
		env.LocaleSettings.Region = r.str()
		// The second capability byte's bits are all taken too; a third follows the locale settings, at the end of the message, so a runtime built before it, which ends here, reads as having none of them. Its bits: none assigned yet.
		var third byte
		if len(r.buf) > 0 {
			third = r.byte()
		}
		_ = third
	}
	return env
}

// writeWebCookies writes a msgCookies as NsgoView.cookies reads it: the surface, the request's count and its op, then what the op takes: each cookie to set as a URL it goes to and its Set-Cookie value, which CookieManager.setCookie takes, the URL whose cookies to list, or nothing to clear.
func writeWebCookies(w *writer, surface, count uint32, r *driver.WebCookiesRequest) {
	w.u32(surface)
	w.u32(count)
	w.byte(byte(r.Op))
	switch r.Op {
	case driver.WebCookiesList:
		w.str(r.Filter.URL)
	case driver.WebCookiesSet:
		w.u32(uint32(len(r.Cookies)))
		for _, c := range r.Cookies {
			w.str(c.URL())
			w.str(c.SetCookie())
		}
	}
}

// readWebCookiesSet reads a msgCookiesSet: the surface and the count of the request it answers, an error, "" for none, and the request's op, which for a list is followed by its URL and the Cookie header CookieManager.getCookie gave for it.
func readWebCookiesSet(r *reader) (uint32, driver.Event) {
	surface, count := r.u32(), r.u32()
	ev := driver.Event{Kind: driver.EventWebCookies, Count: count, Text: r.str()}
	if driver.WebCookiesOp(r.byte()) == driver.WebCookiesList {
		u, header := r.str(), r.str()
		if ev.Text == "" {
			ev.Data = cookiesOfHeader(u, header)
		}
	}
	return surface, ev
}

// cookiesOfHeader returns the cookies of a Cookie header CookieManager gave for rawURL: names and values alone, for the URL's host, as CookieManager tells nothing else.
func cookiesOfHeader(rawURL, header string) []driver.WebCookie {
	host := ""
	if u, err := url.Parse(rawURL); err == nil {
		host = strings.ToLower(u.Hostname())
	}
	cookies := []driver.WebCookie{}
	for pair := range strings.SplitSeq(header, ";") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		// Chromium writes a cookie without a name as its value alone.
		name, value, ok := strings.Cut(pair, "=")
		if !ok {
			name, value = "", pair
		}
		cookies = append(cookies, driver.WebCookie{Name: name, Value: value, Domain: host})
	}
	return cookies
}

// readEvent reads a msgEvent as NsgoView.event writes it: the target surface, the event and its trailing payload string.
func readEvent(r *reader) (uint32, driver.Event, string) {
	// The surface the event is for: 0 for the activity's view, else an overlay's.
	from := r.u32()
	ev := driver.Event{
		Kind:  driver.EventKind(r.byte()),
		ID:    driver.NodeID(r.u32()),
		Count: r.u32(),
		Point: driver.Point{X: r.points(), Y: r.points()},
		Text:  r.str(),
		Size:  driver.Size{Width: r.points(), Height: r.points()},
		Index: int(int32(r.u32())),
		// A gesture's; zero for other events.
		Phase:    driver.GesturePhase(r.byte()),
		Velocity: driver.Point{X: r.points(), Y: r.points()},
		Value:    r.float(),
	}
	ev.Navigation = readNavigation(r)
	// A video's; zero for other events.
	ev.Time = time.Duration(r.u64()) * time.Millisecond
	ev.Duration = time.Duration(r.u64()) * time.Millisecond
	ev.Buffering = r.byte() != 0
	ev.RateChangeable = ev.Kind == driver.EventLoad
	// A native view's callback's arguments, as JSON, and whether
	// an animation finished.
	data := r.str()
	ev.Finished = r.byte() != 0
	// A pointer's position in its root view and modifier keys.
	ev.Page = driver.Point{X: r.points(), Y: r.points()}
	ev.Position = driver.Point{X: r.points(), Y: r.points()}
	if ev.Kind == driver.EventPan {
		// A pan's page point comes in the pointer's page slot.
		ev.PagePosition = ev.Page
	}
	mods := r.byte()
	ev.Modifiers = driver.Modifiers{Control: mods&1 != 0, Alt: mods&2 != 0, Shift: mods&4 != 0, Platform: mods&8 != 0}
	return from, ev, data
}

// readNavigation reads the web navigation state NsgoView.event writes
// last, nil for events without one.
func readNavigation(r *reader) *driver.WebNavigation {
	flags := r.byte()
	if flags&1 == 0 {
		return nil
	}
	return &driver.WebNavigation{
		Loading:      flags&2 != 0,
		CanGoBack:    flags&4 != 0,
		CanGoForward: flags&8 != 0,
		URL:          r.str(),
		Title:        r.str(),
	}
}

// autofillHints are Android's autofill hints of each AutoComplete, as
// NSH's: "" turns autofill off and "-" leaves it to the system.
var autofillHints = map[driver.AutoComplete]string{
	driver.AutoCompleteDefault: "-", driver.AutoCompleteOff: "",
	driver.AutoCompleteUsername: "username", driver.AutoCompletePassword: "password",
	driver.AutoCompleteNewPassword: "newPassword", driver.AutoCompleteOneTimeCode: "smsOTPCode",
	driver.AutoCompleteEmail: "emailAddress", driver.AutoCompleteTel: "phone", driver.AutoCompleteName: "personName",
	driver.AutoCompleteGivenName: "personGivenName", driver.AutoCompleteFamilyName: "personFamilyName",
	driver.AutoCompleteMiddleName: "personMiddleName", driver.AutoCompleteNickname: "nickname",
	driver.AutoCompleteOrganization: "organization", driver.AutoCompleteStreetAddress: "postalAddress",
	driver.AutoCompleteAddressLine1: "streetAddress", driver.AutoCompleteAddressLine2: "extendedAddress",
	driver.AutoCompleteCity: "addressLocality", driver.AutoCompleteRegion: "addressRegion",
	driver.AutoCompletePostalCode: "postalCode", driver.AutoCompleteCountry: "addressCountry",
	driver.AutoCompleteCreditCardNumber: "creditCardNumber", driver.AutoCompleteCreditCardExpiration: "creditCardExpirationDate",
	driver.AutoCompleteCreditCardSecurityCode: "creditCardSecurityCode", driver.AutoCompleteCreditCardName: "personName",
	driver.AutoCompleteURL: "url", driver.AutoCompleteBirthdate: "birthDateFull",
}

// writeEditing appends ItemInput's editing props, which the runtime's
// InputField reads after the accessory: multiline, maxLength, what Return
// does (InputConfig.Submit), the return key, capitalization, flags (no
// autocorrection, no caret), the autofill hint, the placeholder and
// selection colors (RGBA, 0 for the platform's) and the PasteBehavior.
func writeEditing(w *writer, in *driver.InputConfig) {
	w.flags(in.Multiline)
	w.u32(uint32(max(in.MaxLength, 0)))
	w.byte(byte(in.Submit()))
	w.byte(byte(in.ReturnKey))
	w.byte(byte(in.AutoCapitalize))
	w.flags(in.NoAutoCorrect, in.HideCaret)
	hint, ok := autofillHints[in.AutoComplete]
	if !ok {
		hint = "-"
	}
	w.str(hint)
	w.u32(uint32(in.PlaceholderColor))
	w.u32(uint32(in.Caret()))
	w.byte(byte(in.PasteBehavior))
	// Padding, top, right, bottom and left in points, as InputField reads
	// after the paste behavior: a multiline field scrolls under it.
	w.points(in.Padding.Top)
	w.points(in.Padding.Right)
	w.points(in.Padding.Bottom)
	w.points(in.Padding.Left)
}

// writeScrollConfig writes ItemScrollConfig as Scrolling.Config.read reads it:
// scroll behavior, insets, zoom limits, header inset, and each side's drawn
// scroll edge effect (top, right, bottom, left): 0 for none, 1 for soft and 2
// for hard, followed when drawn by the bar's extent in points and its tint and
// hairline colors (RGBA).
func writeScrollConfig(w *writer, c *driver.ScrollConfig) {
	b := &c.Behavior
	w.flags(c.Horizontal, c.HideIndicator)
	w.flags(b.Paging, b.DisableIntervalMomentum, b.NoScroll, b.NoBounces, c.Refreshable, c.Refreshing,
		c.NoBouncesZoom, c.NoPinchZoom)
	w.flags(c.CenterContent)
	w.points(b.SnapToInterval)
	w.byte(byte(b.SnapToAlignment))
	w.u32(uint32(len(b.SnapToOffsets)))
	for _, o := range b.SnapToOffsets {
		w.points(o)
	}
	w.float(b.Deceleration())
	in := b.ContentInset
	w.points(in.Top)
	w.points(in.Right)
	w.points(in.Bottom)
	w.points(in.Left)
	w.byte(byte(b.KeyboardDismissMode))
	w.byte(byte(b.KeyboardShouldPersistTaps))
	w.float(c.MinZoomScale)
	w.float(c.MaxZoomScale)
	w.float(c.ZoomScale)
	w.points(b.RefreshIndicator)
	w.points(c.HeaderInset)
	for i := range c.Edges {
		e := &c.Edges[i]
		switch e.Drawn() {
		case driver.EdgeEffectSoft:
			w.byte(1)
		case driver.EdgeEffectHard:
			w.byte(2)
		default:
			w.byte(0)
			continue
		}
		w.points(e.Extent)
		w.u32(uint32(e.Tint))
		w.u32(uint32(e.Line))
	}
}

// readPaste reads the paste an EventPaste carries in its data: lines of
// "uri\t" and a file: URL, or "file\t", a MIME type, a tab and the
// bytes in base64.
func readPaste(text, data string) *driver.PasteEvent {
	p := &driver.PasteEvent{Text: text}
	for _, line := range strings.Split(data, "\n") {
		kind, rest, _ := strings.Cut(line, "\t")
		switch kind {
		case "uri":
			p.URIs = append(p.URIs, rest)
		case "file":
			mime, b64, _ := strings.Cut(rest, "\t")
			if b, err := base64.StdEncoding.DecodeString(b64); err == nil {
				p.Files = append(p.Files, driver.PastedFile{MIME: mime, Data: b})
			}
		}
	}
	return p
}
