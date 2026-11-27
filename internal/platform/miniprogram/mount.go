package miniprogram

import (
	"image"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/textselect"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// The mounter turns each batch into the runtime's operations and the
// user's touches back into events. It is plain Go over a host, the logic
// layer's or a test's, so everything but the bridge is tested on any
// machine.
//
// The operations, which nsgo/tree.js applies to the tree the page draws:
//
//	c  id kind               create a view: view, text, input, scroll, image, canvas, video, modal
//	root id                  the view the page shows
//	i  parent id before      insert id below sibling before (null: on top)
//	r  id                    detach a view, keeping it
//	d  id                    destroy a view
//	f  id x y w h            its frame, in its parent's padding box
//	s  id css                its style as inline CSS
//	a  id css                what its animated values draw over its style
//	tr id css                the transition of its frame's next change
//	ka id frames             its keyframe animation (null: none)
//	t  id text               a text's runs: {x, y, s: [css], r: [[text, x, y, h, style]], b: [[x, y, w, h, color, radius]]}
//	hl id highlight          a text's selection highlight rectangles (null: none)
//	sh id handles            a selection area's start and end handles (null: none)
//	e  id bits               the touches it reports (touch* constants)
//	sc id config             a scroll view's: {h, hide, rf, rt}
//	ss id w h                a scroll view's content size
//	so id x y animated       scroll to an offset
//	im id src mode css [t]   an encoded image's source, mode, inline CSS and optional tint mask CSS
//	px id css                a pixel image's fitted canvas, or "" to clear it
//	vd id config             a video's configuration: {src, ap, loop, muted, ctrl, fit}
//	vc id [seq, cmds]        ordered VideoContext commands (play, pause, seek, rate)
//	fd id config             a text field's configuration
//	iv id text               a text field's value
//	fo id focused            focus or blur a text field
//	sel id start end         a text field's selection, in UTF-16 units
//	cv id url                what a canvas drew, as an image
//	ax id props              its accessibility attributes (null: none)
//	bk id on                 whether back goes through the app (id 0)
//
// What no view shows (alerts, the clipboard, titles) goes to the host.

// The touches a view reports (op e).
const (
	touchPress      = 1 << iota // taps: EventPress
	touchPressInOut             // EventPressDown and EventPressUp
	touchLongPress              // EventLongPress
	touchFollow                 // every touch, for pans, pinches and EventTouch
	touchCapture                // and the scroll views around it scroll no more
)

// host is what the mounter needs of the mini program.
type host interface {
	// submit hands the runtime a batch of operations (JSON).
	submit(ops []byte)
	// dispatch hands the engine an event.
	dispatch(ev driver.Event)
	// service carries out an item no view shows: an alert, the
	// clipboard, the title.
	service(it *driver.MountItem)
	// imageURL is what an <image> or <video> loads for src.
	imageURL(src driver.ImageSource) string
	// imageMaskURL is a CSS-usable URL (https/http or data URI) for a tinted
	// image's mask-image layer.
	imageMaskURL(src driver.ImageSource) string
	supportsPixelImages() bool
	// imagePixels writes a version into an image view (recoloring opaque pixels
	// when tint != 0), or releases it for nil.
	imagePixels(id driver.NodeID, pixels *image.RGBA, tint driver.Color)
	// setClipboard writes text to the host clipboard (for selection copy).
	setClipboard(text string)
	// paint draws a canvas's ops at size points, its pixel ratio's
	// pixels times scale (Drawing.ResolutionScale), stretched to the
	// pixel ratio's to their nearest unless smooth, and returns the
	// picture as a URL; "" while an image it draws loads, which paints it
	// again (repaint).
	paint(id driver.NodeID, d *driver.Drawing, ops []driver.DrawOp, size driver.Size, scale float64, smooth bool) string
	// tick asks for a call of mounter.tick at the next frame.
	tick()
}

// view is the mounter's record of one view.
type view struct {
	id       driver.NodeID
	kind     driver.ViewKind
	parent   *view
	children []*view
	frame    driver.Bounds
	framed   bool
	// transition is set while a layout transition moves the frame.
	transition bool
	style      driver.ViewStyle
	css        string // as last sent
	animated   driver.AnimatedValues
	acss       string // as last sent
	// shift is a sticky cell's from its frame.
	shift           driver.Point
	events          driver.EventMask
	props           driver.ViewProps
	ax              string // last sent accessibility JSON, empty for none
	overflow        driver.Bounds
	transformedKids bool
	layout          *TextLayout
	text            *driver.TextContent
	selHL           string // last sent selection highlight JSON
	selSH           string // last sent selection handles JSON
	// A scroll view's offset, content size, axis, the scrolls it applied and sticky cells.
	offset      driver.Point
	contentSize driver.Size
	horizontal  bool
	applied     uint32
	sticky      []driver.StickyCell
	// A text field's edits by the user, the value it shows, its configuration and whether the automation selected all of it.
	edits         uint32
	value         string
	inputConfig   *driver.InputConfig
	inputDisabled bool
	inputReadonly bool
	maxLength     int
	selectedAll   bool
	// A canvas's drawing, an image's state, a video's state.
	drawing      *driver.Drawing
	image        *driver.ImageConfig
	imageNatural driver.Size
	pixelImage   *driver.ImageConfig
	pixelTint    driver.Color
	pixelCSS     string
	video        *videoState
	modal        driver.ModalAnimation
	shown        bool // a modal was presented
}

// border returns the width of each side of v's border.
func (v *view) border() driver.Edges {
	if v.style.Borders != (driver.Edges{}) {
		return v.style.Borders
	}
	return driver.All(v.style.BorderWidth)
}

type mounter struct {
	h        host
	views    []*view
	root     driver.NodeID
	modals   []*view
	blocking *view
	// focused is the text field with the focus, alerts the alerts showing by ID and viewport the page's size, which nsgo dev -automation reads (testsurface.go).
	focused  *view
	alerts   map[uint32]*driver.AlertConfig
	viewport driver.Size
	o        ops
	animator *driver.Animator
	// ticking is set while a frame is asked for.
	ticking     bool
	frameWanted bool
	// frame is called when a frame the engine asked for begins.
	frame func()
	// back is whether back goes through the app, as last sent.
	back    bool
	reduce  bool // the environment reduces motion
	touches touches
	// repaint are canvases whose drawing waits for an image.
	repaint map[driver.NodeID]bool
	// presenting holds modals currently playing their entrance keyframe animation until tick reaches their end and reports EventModalShown.
	presenting map[driver.NodeID]modalPresent
	now        time.Duration
	hasNow     bool
	// Active text selection state across a SelectionArea or standalone SelectableOn text.
	selecting       *textselect.Selection
	selected        []*view
	selArea         *view
	selDragging     bool
	selHandle       driver.SelectionHandle
	selHandleActive bool
	selGrab         driver.Point
}

type modalPresent struct {
	view    *view
	dur     time.Duration
	started bool
	end     time.Duration
}

func newMounter(h host) *mounter {
	m := &mounter{h: h, alerts: map[uint32]*driver.AlertConfig{}, repaint: map[driver.NodeID]bool{}, presenting: map[driver.NodeID]modalPresent{}}
	m.animator = driver.NewAnimator(h.dispatch)
	return m
}

// answerAlert dispatches an alert's answer once, forgetting the alert so a later native dismissal or duplicate call cannot answer it twice.
func (m *mounter) answerAlert(id uint32, index int) bool {
	if _, ok := m.alerts[id]; !ok {
		return false
	}
	delete(m.alerts, id)
	m.h.dispatch(driver.Event{Kind: driver.EventAlert, Count: id, Index: index})
	return true
}

func (m *mounter) view(id driver.NodeID) *view {
	if int(id) < len(m.views) {
		return m.views[id]
	}
	return nil
}

// kindName is the kind a view is drawn as.
func kindName(k driver.ViewKind) string {
	switch k {
	case driver.KindText:
		return "text"
	case driver.KindInput:
		return "input"
	case driver.KindScroll:
		return "scroll"
	case driver.KindImage:
		return "image"
	case driver.KindCanvas:
		return "canvas"
	case driver.KindVideo:
		return "video"
	case driver.KindModal:
		return "modal"
	}
	return "view"
}

// mount applies a batch and hands its operations to the host.
func (m *mounter) mount(b *driver.MountBatch) {
	m.o.reset()
	var lb *driver.LayoutAnimation
	if b.LayoutAnimation != nil && !m.reduce {
		lb = b.LayoutAnimation
	}
	for i := range b.Items {
		m.item(b, &b.Items[i], lb)
	}
	m.applyAnimated()
	m.refreshSelection()
	m.flushVideos()
	m.flush()
	if m.animator.Running() {
		m.startTicking()
	}
}

// flush hands what was written to the host.
func (m *mounter) flush() {
	if !m.o.empty() {
		m.h.submit(m.o.bytes())
		m.o.reset()
	}
}

func (m *mounter) item(b *driver.MountBatch, it *driver.MountItem, lb *driver.LayoutAnimation) {
	switch it.Kind {
	case driver.ItemAnimation:
		m.animator.Apply(it.Animation)
		return
	case driver.ItemCreate:
		for len(m.views) <= int(it.ID) {
			m.views = append(m.views, nil)
		}
		v := &view{id: it.ID, kind: it.View, style: driver.DefaultViewStyle}
		m.views[it.ID] = v
		m.o.op("c", it.ID).str(kindName(it.View))
		if it.ID == b.Root {
			m.root = it.ID
			m.o.op("root", it.ID)
		}
		return
	case driver.ItemHistory:
		if on := it.Count > 0; on != m.back {
			m.back = on
			m.o.op("bk", 0).boolean(on)
		}
		return
	case driver.ItemAppearance, driver.ItemDirection:
		m.h.service(it)
		return
	case driver.ItemBlockNative:
		prev := m.blocking
		m.blocking = nil
		if prev != nil {
			m.sendEvents(prev)
		}
		if it.ID != 0 {
			if next := m.view(it.ID); next != nil {
				m.blocking = next
				m.sendEvents(next)
			}
		}
		return
	}
	v := m.view(it.ID)
	if v == nil {
		if it.ID == 0 {
			if it.Kind == driver.ItemAlert && it.Alert != nil {
				m.alerts[it.Alert.ID] = it.Alert
			}
			m.h.service(it)
		}
		return
	}
	switch it.Kind {
	case driver.ItemInsert:
		parent := m.view(it.Parent)
		if parent == nil {
			return
		}
		m.detach(v)
		at := len(parent.children)
		if before := m.view(it.Before); before != nil {
			for k, c := range parent.children {
				if c == before {
					at = k
					break
				}
			}
		}
		parent.children = append(parent.children[:at], append([]*view{v}, parent.children[at:]...)...)
		v.parent = parent
		if v.kind == driver.KindModal {
			m.modals = append(m.modals, v)
		}
		m.o.op("i", parent.id).num(float64(v.id))
		if before := m.view(it.Before); before != nil && before.parent == parent {
			m.o.num(float64(before.id))
		} else {
			m.o.null()
		}
		// Placed in its new parent's padding box, styled by its props.
		if v.framed {
			m.sendFrame(v)
		}
		m.sendStyle(v)
		if v.kind == driver.KindModal && !v.shown {
			v.shown = true
			m.present(v)
		}
		if lb != nil && lb.Create != nil {
			m.appear(v, lb.Create)
		}
	case driver.ItemRemove:
		m.detach(v)
		m.o.op("r", v.id)
	case driver.ItemDelete:
		if v.pixelImage != nil {
			m.h.imagePixels(v.id, nil, 0)
		}
		if m.selecting != nil && m.selecting.Area == v.id {
			m.clearSelection()
		}
		isModal := v.kind == driver.KindModal
		delete(m.presenting, v.id)
		m.detach(v)
		for _, c := range v.children {
			c.parent = nil
		}
		m.views[it.ID] = nil
		if m.blocking == v {
			m.blocking = nil
		}
		if m.focused == v {
			m.focused = nil
		}
		if m.selArea == v {
			m.selArea = nil
		}
		m.animator.Forget(v.id)
		delete(m.repaint, v.id)
		m.touches.forget(v)
		m.o.op("d", v.id)
		if isModal {
			m.h.dispatch(driver.Event{Kind: driver.EventModalDismissed, ID: v.id})
		}
	case driver.ItemFrame:
		resized := v.frame.Size != it.Frame.Size
		v.frame, v.framed = it.Frame, true
		tr := it.Transition
		if lb != nil && lb.Update != nil {
			tr = lb.Update
		}
		switch {
		case tr != nil && tr.Duration > 0 && !m.reduce:
			v.transition = true
			m.o.op("tr", v.id).str(layoutTransitionCSS(tr))
		case v.transition:
			v.transition = false
			m.o.op("tr", v.id).str("")
		}
		m.sendFrame(v)
		if resized {
			// Gradients are resolved against the box.
			if e := v.style.Effects; e != nil && (len(e.Gradients) > 0 || e.Mask != nil) {
				m.sendStyle(v)
			}
			if v.kind == driver.KindCanvas {
				m.paint(v)
			}
			if v.pixelImage != nil {
				m.placePixelImage(v)
			}
			if v.image != nil && v.style.TintColor != 0 {
				m.sendImage(v)
			}
		}
	case driver.ItemViewStyle:
		old := v.border()
		oldTint := v.style.TintColor
		anim := v.style.Animation
		v.style = *it.ViewStyle
		m.sendStyle(v)
		if v.style.TintColor != oldTint {
			if v.pixelImage != nil {
				v.pixelTint = v.style.TintColor
				m.h.imagePixels(v.id, v.pixelImage.Source.Pixels, v.style.TintColor)
			}
			if v.image != nil {
				m.sendImage(v)
			}
		}
		if v.border() != old {
			for _, c := range v.children {
				if c.framed {
					m.sendFrame(c)
				}
			}
			if v.kind == driver.KindCanvas {
				m.paint(v)
			}
			if v.pixelImage != nil {
				m.placePixelImage(v)
			}
			if v.image != nil && v.style.TintColor != 0 && v.style.TintColor == oldTint {
				m.sendImage(v)
			}
		}
		if v.style.Animation != anim {
			m.keyframes(v)
		}
		if v.animated.Set != 0 {
			m.animator.Touch(v.id)
		}
	case driver.ItemViewProps:
		boxNone := v.props.PointerEvents == driver.PointerEventsBoxNone
		v.props = *it.ViewProps
		m.sendStyle(v)
		if boxNone != (v.props.PointerEvents == driver.PointerEventsBoxNone) {
			for _, c := range v.children {
				m.sendStyle(c)
			}
		}
		m.sendAccess(v)
		m.sendEvents(v)
	case driver.ItemAccessibilityLabel:
		v.props.Label = it.Value
		m.sendAccess(v)
	case driver.ItemEvents:
		v.events = it.Events
		m.sendEvents(v)
	case driver.ItemOverflow:
		v.overflow, v.transformedKids = it.Frame, it.Transformed
	case driver.ItemText:
		v.text = it.Text
		v.layout, _ = it.Text.Layout.(*TextLayout)
		m.sendText(v)
	case driver.ItemInput:
		v.inputConfig = it.Input
		v.inputDisabled = it.Input.Disabled
		v.inputReadonly = it.Input.Readonly
		v.maxLength = it.Input.MaxLength
		m.sendInput(v, it.Input)
	case driver.ItemInputValue:
		// A value the app set before the user's latest edit is stale; one
		// the field shows is not sent again, which would move the caret.
		if it.Count < v.edits || it.Value == v.value {
			return
		}
		v.value = it.Value
		v.selectedAll = false
		m.o.op("iv", v.id).str(it.Value)
	case driver.ItemFocus:
		m.focused = v
		m.o.op("fo", v.id).boolean(true)
	case driver.ItemBlur:
		if m.focused == v {
			m.focused = nil
		}
		m.o.op("fo", v.id).boolean(false)
	case driver.ItemSelection:
		m.o.op("sel", v.id).num(float64(it.Selection.Start)).num(float64(it.Selection.End))
	case driver.ItemScrollConfig:
		v.horizontal = it.Scroll.Horizontal
		var cfg object
		cfg.boolean("h", it.Scroll.Horizontal).boolean("hide", it.Scroll.HideIndicator).
			boolean("rf", it.Scroll.Refreshable).boolean("rt", it.Scroll.Refreshing).
			boolean("off", it.Scroll.Behavior.NoScroll).boolean("bounce", !it.Scroll.Behavior.NoBounces).
			boolean("paging", it.Scroll.Behavior.Paging)
		m.o.op("sc", v.id).raw(cfg.bytes())
	case driver.ItemScrollContent:
		v.contentSize = it.Size
		m.o.op("ss", v.id).pt(it.Size.Width).pt(it.Size.Height)
	case driver.ItemScrollTo:
		v.applied++
		m.o.op("so", v.id).pt(it.Point.X).pt(it.Point.Y).boolean(it.Animated)
	case driver.ItemScrollBy:
		v.applied++
		to := driver.Point{X: v.offset.X + it.Point.X, Y: v.offset.Y + it.Point.Y}
		m.o.op("so", v.id).pt(to.X).pt(to.Y).boolean(false)
	case driver.ItemStickyCells:
		for _, c := range v.sticky {
			if cell := m.view(c.Cell); cell != nil && cell.shift != (driver.Point{}) {
				cell.shift = driver.Point{}
				m.animatedStyle(cell)
			}
		}
		v.sticky = it.Sticky
		m.stick(v)
	case driver.ItemImage:
		if it.Image.Source.Pixels != nil && m.h.supportsPixelImages() {
			if v.pixelImage == nil || v.pixelImage.Source.Pixels != it.Image.Source.Pixels || v.pixelTint != v.style.TintColor {
				v.pixelTint = v.style.TintColor
				m.h.imagePixels(v.id, it.Image.Source.Pixels, v.style.TintColor)
			}
			if v.pixelImage == nil {
				m.o.op("im", v.id).str("").str("").str("")
			}
			v.image, v.imageNatural = nil, driver.Size{}
			v.pixelImage = it.Image
			m.placePixelImage(v)
			break
		}
		if v.pixelImage != nil {
			v.pixelImage, v.pixelCSS, v.pixelTint = nil, "", 0
			m.h.imagePixels(v.id, nil, 0)
			m.o.op("px", v.id).str("")
		}
		if v.image == nil || v.image.Source.Key() != it.Image.Source.Key() {
			v.imageNatural = driver.Size{}
		}
		v.image = it.Image
		m.sendImage(v)
	case driver.ItemVideo:
		m.setVideo(v, it.Video)
	case driver.ItemVideoSeek:
		m.seekVideo(v, it.Time)
	case driver.ItemCanvas:
		v.drawing = it.Drawing
		var slots []driver.DrawSlot
		if it.Drawing != nil {
			slots = it.Drawing.Slots
		}
		m.animator.BindCanvas(v.id, slots)
		m.paint(v)
	case driver.ItemModal:
		v.modal = it.Modal.Animation
	default:
		m.h.service(it)
	}
}

// samplingCSS is the inline CSS that scales an image with sampling, as the web's image views do: image-rendering, which the image component's inner element and the pixel canvas inherit.
func samplingCSS(sampling driver.ImageSampling) string {
	if sampling == driver.ImageSamplingNearest {
		return "image-rendering:pixelated"
	}
	return ""
}

// sendImage emits the "im" op for an encoded image view, including a CSS mask
// layer when TintColor is non-zero so opaque pixels recolor without leaking
// untinted pixels.
func (m *mounter) sendImage(v *view) {
	c := v.image
	if c == nil {
		return
	}
	mode := "aspectFill"
	switch c.ResizeMode {
	case driver.Contain, driver.ScaleDown:
		mode = "aspectFit"
	case driver.Stretch:
		mode = "scaleToFill"
	case driver.Center:
		mode = "center"
	}
	src := ""
	if c.Source.URI != "" || len(c.Source.Data) > 0 {
		src = m.h.imageURL(c.Source)
	}
	inline := imageInlineCSS(c)
	if v.style.TintColor != 0 && src != "" {
		maskURL := m.h.imageMaskURL(c.Source)
		if maskURL == "" {
			maskURL = src
		}
		if tintCSS := imageTintCSS(v, maskURL); tintCSS != "" {
			m.o.op("im", v.id).str(src).str(mode).str(inline).str(tintCSS)
			return
		}
	}
	m.o.op("im", v.id).str(src).str(mode).str(inline)
}

// imageLoaded records natural dimensions from <image>'s bindload, refreshes
// tinted mask sizing when needed, and dispatches EventLoad and EventLoadEnd.
func (m *mounter) imageLoaded(id driver.NodeID, width, height driver.Points) {
	v := m.view(id)
	if v == nil {
		return
	}
	if width > 0 && height > 0 {
		natural := driver.Size{Width: width, Height: height}
		if natural != v.imageNatural {
			v.imageNatural = natural
			if v.image != nil && v.style.TintColor != 0 {
				m.sendImage(v)
				m.flush()
			}
		}
	}
	m.send(v, driver.Event{Kind: driver.EventLoad, Size: driver.Size{Width: width, Height: height}})
	m.send(v, driver.Event{Kind: driver.EventLoadEnd})
}

// placePixelImage keeps the buffer at its fitted rectangle as its box changes.
func (m *mounter) placePixelImage(v *view) {
	c := v.pixelImage
	natural := c.Source.Size
	if natural.Width <= 0 || natural.Height <= 0 {
		b := c.Source.Pixels.Rect
		natural = driver.Size{Width: driver.Points(b.Dx()), Height: driver.Points(b.Dy())}
	}
	border := v.border()
	box := driver.Size{Width: max(0, v.frame.Size.Width-border.Left-border.Right),
		Height: max(0, v.frame.Size.Height-border.Top-border.Bottom)}
	fit := c.ResizeMode.Fit(natural, box, c.Position)
	css := "left:" + webstyle.Px(fit.Origin.X) + ";top:" + webstyle.Px(fit.Origin.Y) +
		";width:" + webstyle.Px(fit.Size.Width) + ";height:" + webstyle.Px(fit.Size.Height)
	if c.BlurRadius > 0 {
		css += ";filter:blur(" + webstyle.Px(c.BlurRadius) + ")"
	}
	if s := samplingCSS(c.Sampling); s != "" {
		css += ";" + s
	}
	if css != v.pixelCSS {
		v.pixelCSS = css
		m.o.op("px", v.id).str(css)
	}
}

// detach takes v out of its parent's children.
func (m *mounter) detach(v *view) {
	if v.kind == driver.KindModal {
		for k, modal := range m.modals {
			if modal == v {
				m.modals = append(m.modals[:k], m.modals[k+1:]...)
				break
			}
		}
	}
	p := v.parent
	if p == nil {
		return
	}
	for k, c := range p.children {
		if c == v {
			p.children = append(p.children[:k], p.children[k+1:]...)
			break
		}
	}
	v.parent = nil
}

// sendFrame sends v's frame in its parent's padding box, which CSS
// places absolute children in.
func (m *mounter) sendFrame(v *view) {
	f := v.frame
	if p := v.parent; p != nil && v.kind != driver.KindModal {
		b := p.border()
		f.Origin.X -= b.Left
		f.Origin.Y -= b.Top
	}
	m.o.op("f", v.id).pt(f.Origin.X).pt(f.Origin.Y).pt(f.Size.Width).pt(f.Size.Height)
}

func (m *mounter) sendStyle(v *view) {
	if c := viewCSS(v, v.parent); c != v.css {
		v.css = c
		m.o.op("s", v.id).str(c)
	}
}

func (m *mounter) sendAccess(v *view) {
	data := accessJSON(v.kind, &v.props)
	s := string(data)
	if s == v.ax {
		return
	}
	v.ax = s
	if len(data) == 0 {
		m.o.op("ax", v.id).null()
		return
	}
	m.o.op("ax", v.id).raw(data)
}

// animatedStyle sends what v's animated values draw, when it changed.
func (m *mounter) animatedStyle(v *view) {
	if c := animatedCSS(v); c != v.acss {
		v.acss = c
		m.o.op("a", v.id).str(c)
	}
}

func (m *mounter) sendEvents(v *view) {
	ev := v.events
	if v.props.PointerEvents == driver.PointerEventsNone {
		ev = 0
	}
	bits := 0
	if ev.Has(driver.EventPress) {
		bits |= touchPress
	}
	if ev.Has(driver.EventPressDown) || ev.Has(driver.EventPressUp) {
		bits |= touchPressInOut
	}
	if ev.Has(driver.EventLongPress) {
		bits |= touchLongPress
	}
	if ev.Has(driver.EventPan) || ev.Has(driver.EventPinch) || ev.Has(driver.EventRotation) ||
		ev.Has(driver.EventTouch) || m.animator.PanBound(v.id) {
		bits |= touchFollow
		// A pan that fails on vertical drags leaves them to the scroll
		// views around it; others keep the drags they start.
		pa := v.props.PanActivation
		if (ev.Has(driver.EventPan) || m.animator.PanBound(v.id)) && pa.FailOffsetY == 0 && pa.ActiveOffsetX == 0 {
			bits |= touchCapture
		}
		if m.blocking == v && ev.Has(driver.EventTouch) {
			bits |= touchCapture
		}
	}
	if isSelectionTrigger(v) {
		bits |= touchLongPress | touchFollow
	}
	if m.selecting != nil && v.id == m.root {
		bits |= touchFollow
	}
	m.o.op("e", v.id).num(float64(bits))
}

// sendText sends a text's runs, in its view's padding box.
func (m *mounter) sendText(v *view) {
	l := v.layout
	if l == nil {
		m.o.op("t", v.id).null()
		return
	}
	var t object
	off := v.text.Content.Origin
	b := v.border()
	t.num("x", float64(off.X-b.Left)).num("y", float64(off.Y-b.Top))
	var styles []byte
	styles = append(styles, '[')
	for k, s := range l.styles {
		if k > 0 {
			styles = append(styles, ',')
		}
		styles = appendString(styles, s)
	}
	t.raw("s", append(styles, ']'))
	hasLang := false
	for _, lang := range l.languages {
		if lang != "" {
			hasLang = true
			break
		}
	}
	if hasLang {
		langs := []byte{'['}
		for k, lang := range l.languages {
			if k > 0 {
				langs = append(langs, ',')
			}
			langs = appendString(langs, lang)
		}
		t.raw("l", append(langs, ']'))
	}
	runs := []byte{'['}
	n := 0
	for _, r := range l.runs {
		if r.text == "" {
			continue
		}
		if n > 0 {
			runs = append(runs, ',')
		}
		n++
		runs = append(runs, '[')
		runs = appendString(runs, r.text)
		for _, x := range []driver.Points{r.x, r.y, r.height} {
			runs = append(runs, ',')
			runs = appendNum(runs, float64(x))
		}
		runs = append(runs, ',')
		runs = appendNum(runs, float64(r.style))
		runs = append(runs, ']')
	}
	t.raw("r", append(runs, ']'))
	backgrounds := []byte{'['}
	for i, bg := range l.under {
		if i > 0 {
			backgrounds = append(backgrounds, ',')
		}
		backgrounds = append(backgrounds, '[')
		for k, x := range []driver.Points{bg.box.Origin.X, bg.box.Origin.Y, bg.box.Size.Width, bg.box.Size.Height} {
			if k > 0 {
				backgrounds = append(backgrounds, ',')
			}
			backgrounds = appendNum(backgrounds, float64(x))
		}
		backgrounds = append(backgrounds, ',')
		backgrounds = appendString(backgrounds, webstyle.Color(bg.color))
		backgrounds = append(backgrounds, ',')
		backgrounds = appendNum(backgrounds, float64(min(bg.radius, bg.box.Size.Width/2, bg.box.Size.Height/2)))
		backgrounds = append(backgrounds, ']')
	}
	t.raw("b", append(backgrounds, ']'))
	if s := v.text.Shadow; s.Color != 0 {
		t.str("sh", webstyle.TextShadow(s))
	}
	m.o.op("t", v.id).raw(t.bytes())
}

// sendInput configures a text field: the <input> or <textarea> the
// render layer shows in its content box.
func (m *mounter) sendInput(v *view, c *driver.InputConfig) {
	var j object
	css := fontCSS(c.Style.Font) + "color:" + webstyle.Color(c.Style.Color) + ";"
	if c.Multiline && driver.KeepAllWords(c.Style.Font.Language) {
		css += "word-break:keep-all;"
	}
	if c.CaretColor != 0 {
		css += "caret-color:" + webstyle.Color(c.CaretColor) + ";"
	}
	switch c.Style.Align {
	case driver.TextAlignCenter:
		css += "text-align:center;"
	case driver.TextAlignEnd, driver.TextAlignRight:
		css += "text-align:right;"
	}
	kind := "text"
	switch c.KeyboardType {
	case driver.KeyboardNumber, driver.KeyboardPhone:
		kind = "number"
	case driver.KeyboardDecimal:
		kind = "digit"
	}
	confirm := "done"
	switch c.ReturnKey {
	case driver.ReturnKeyGo:
		confirm = "go"
	case driver.ReturnKeyNext:
		confirm = "next"
	case driver.ReturnKeySearch:
		confirm = "search"
	case driver.ReturnKeySend:
		confirm = "send"
	}
	ph := c.PlaceholderColor
	if ph == 0 {
		ph = c.Style.Color&0xffffff00 | (c.Style.Color&0xff)/2
	}
	maxLength := -1
	if c.MaxLength > 0 {
		maxLength = c.MaxLength
	}
	b := v.border()
	submit := c.Submit()
	j.str("css", css).str("ph", c.Placeholder).str("phs", "color:"+webstyle.Color(ph)+";"+fontCSS(c.Style.Font)).
		boolean("pw", c.Masked).str("type", kind).boolean("ml", c.Multiline).num("max", float64(maxLength)).
		boolean("off", c.Disabled).boolean("ro", c.Readonly).str("confirm", confirm).
		boolean("hold", submit == driver.SubmitBehaviorSubmit).
		boolean("nl", c.Multiline && submit == driver.SubmitBehaviorNewline).
		num("x", float64(c.Content.Origin.X-b.Left)).num("y", float64(c.Content.Origin.Y-b.Top)).
		num("w", float64(c.Content.Size.Width)).num("h", float64(c.Content.Size.Height))
	if c.Style.Font.Language != "" {
		j.str("lang", c.Style.Font.Language)
	}
	m.o.op("fd", v.id).raw(j.bytes())
}

// present animates a modal in as it is first inserted and schedules EventModalShown for when the entrance finishes (or at once when none runs); reduced motion fades what would slide.
func (m *mounter) present(v *view) {
	anim := v.modal
	if anim == driver.ModalSlide && m.reduce {
		anim = driver.ModalFade
	}
	switch anim {
	case driver.ModalSlide:
		m.o.op("ka", v.id).raw([]byte(`{"d":` + ms(driver.ModalSlide.Duration()) +
			`,"n":1,"f":[{"offset":0,"translateY":"100%","ease":"cubic-bezier(0.2,0.8,0.2,1)"},{"offset":1,"translateY":"0%"}]}`))
	case driver.ModalFade:
		m.o.op("ka", v.id).raw([]byte(`{"d":` + ms(driver.ModalFade.Duration()) +
			`,"n":1,"f":[{"offset":0,"opacity":0,"ease":"ease-out"},{"offset":1,"opacity":1}]}`))
	default:
		m.h.dispatch(driver.Event{Kind: driver.EventModalShown, ID: v.id})
		return
	}
	p := modalPresent{view: v, dur: anim.Duration()}
	if m.hasNow {
		p.started, p.end = true, m.now+p.dur
	}
	m.presenting[v.id] = p
	m.startTicking()
}

// appear fades a view the batch inserts in, as its layout animation's
// Create says.
func (m *mounter) appear(v *view, tr *driver.LayoutTransition) {
	if tr.Duration <= 0 {
		return
	}
	from := `"opacity":0`
	if tr.Property == driver.LayoutScale {
		from = `"scale":[0,0]`
	}
	m.o.op("ka", v.id).raw([]byte(`{"d":` + ms(tr.Duration) + `,"n":1,"f":[{"offset":0,` + from +
		`,"ease":"ease-out"},{"offset":1}]}`))
}

// keyframes runs v's keyframe animation in the render layer, which keeps
// it going with no work here.
func (m *mounter) keyframes(v *view) {
	a := v.style.Animation
	d, frames := a.Cycle()
	if d <= 0 || m.reduce {
		m.o.op("ka", v.id).null()
		return
	}
	var j object
	j.num("d", float64(d.Milliseconds())).num("n", float64(a.Iterations)).num("delay", float64(a.Delay.Milliseconds()))
	list := []byte{'['}
	for k, f := range frames {
		if k > 0 {
			list = append(list, ',')
		}
		var o object
		o.num("offset", float64(f.Offset)).num("opacity", float64(f.Opacity)).num("scale", float64(f.Scale)).
			num("rotate", float64(f.Rotate)).str("translateY", formatPercent(f.TranslateY))
		if f.Easing != (driver.Curve{}) {
			o.str("ease", webstyle.Easing(f.Easing))
		}
		list = append(list, o.bytes()...)
	}
	j.raw("f", append(list, ']'))
	m.o.op("ka", v.id).raw(j.bytes())
}

func ms(d time.Duration) string { return string(appendNum(nil, float64(d.Milliseconds()))) }

func formatPercent(f float32) string { return string(appendNum(nil, float64(f)*100)) + "%" }

// paint draws a canvas's drawing, with its animated arguments, inside
// its border.
func (m *mounter) paint(v *view) {
	if v.drawing == nil || !v.framed {
		if v.drawing == nil {
			m.o.op("cv", v.id).str("")
		}
		return
	}
	b := v.border()
	size := driver.Size{Width: max(v.frame.Size.Width-b.Left-b.Right, 0), Height: max(v.frame.Size.Height-b.Top-b.Bottom, 0)}
	if size.Width <= 0 || size.Height <= 0 {
		return
	}
	url := m.h.paint(v.id, v.drawing, m.animator.CanvasOps(v.id, v.drawing), size, v.drawing.ResolutionScale(), v.drawing.Smooth())
	if url == "" {
		m.repaint[v.id] = true
		return
	}
	delete(m.repaint, v.id)
	m.o.op("cv", v.id).str(url)
}

// repaintWaiting paints the canvases that waited for an image again.
func (m *mounter) repaintWaiting() {
	for id := range m.repaint {
		if v := m.view(id); v != nil {
			m.paint(v)
		} else {
			delete(m.repaint, id)
		}
	}
	m.flush()
}

// applyAnimated writes the animated values that changed.
func (m *mounter) applyAnimated() {
	m.animator.Changed(func(id driver.NodeID, vals driver.AnimatedValues) {
		if v := m.view(id); v != nil {
			v.animated = vals
			m.animatedStyle(v)
		}
	})
	m.animator.ChangedCanvases(func(id driver.NodeID) {
		if v := m.view(id); v != nil && v.kind == driver.KindCanvas {
			m.paint(v)
		}
	})
}

// stick moves scroll view v's sticky cells for its offset.
func (m *mounter) stick(v *view) {
	for _, c := range v.sticky {
		cell := m.view(c.Cell)
		if cell == nil {
			continue
		}
		var shift driver.Point
		if v.scrollHorizontal() {
			shift.X = c.StickyShift(v.offset.X)
		} else {
			shift.Y = c.StickyShift(v.offset.Y)
		}
		if shift != cell.shift {
			cell.shift = shift
			m.animatedStyle(cell)
		}
	}
}

// scrollHorizontal reports whether a scroll view's sticky cells move
// across: its content is wider than it and no taller.
func (v *view) scrollHorizontal() bool {
	return v.offset.X != 0 && v.offset.Y == 0
}

func (m *mounter) startTicking() {
	if !m.ticking {
		m.ticking = true
		m.h.tick()
	}
}

// requestFrame has the next frame call the engine's frame callbacks.
func (m *mounter) requestFrame() {
	m.frameWanted = true
	m.startTicking()
}

// tick runs one frame at now, a monotonic time.
func (m *mounter) tick(now time.Duration) {
	m.ticking = false
	m.now, m.hasNow = now, true
	m.animator.Step(now)
	m.applyAnimated()
	m.flush()
	for id, p := range m.presenting {
		if !p.started {
			p.started, p.end = true, now+p.dur
			m.presenting[id] = p
			continue
		}
		if now >= p.end {
			delete(m.presenting, id)
			if m.view(id) == p.view {
				m.h.dispatch(driver.Event{Kind: driver.EventModalShown, ID: id})
			}
		}
	}
	if m.frameWanted {
		m.frameWanted = false
		if m.frame != nil {
			m.frame()
		}
	}
	if m.animator.Running() || m.frameWanted || len(m.presenting) > 0 {
		m.startTicking()
	}
}

// origin returns where v's top left is on the page: its frames' origins
// less the offsets of the scroll views it is in.
func (m *mounter) origin(v *view) driver.Point {
	var p driver.Point
	for c := v; c != nil; c = c.parent {
		p.X += c.frame.Origin.X + c.shift.X
		p.Y += c.frame.Origin.Y + c.shift.Y
		if c.parent != nil && c.parent.kind == driver.KindScroll {
			p.X -= c.parent.offset.X
			p.Y -= c.parent.offset.Y
		}
		if c.kind == driver.KindModal {
			break
		}
	}
	return p
}

// pointer returns an event of kind for v at page point page.
func (m *mounter) pointer(v *view, kind driver.EventKind, page driver.Point) driver.Event {
	local, ok := m.surfacePoint(v, page)
	if !ok {
		o := m.origin(v)
		local = driver.Point{X: page.X - o.X, Y: page.Y - o.Y}
	}
	return driver.Event{Kind: kind, ID: v.id, Page: page, Count: 1, Point: local}
}

// send dispatches ev for v when v is mounted and asked for its kind.
func (m *mounter) send(v *view, ev driver.Event) {
	if m.view(v.id) != v || !v.events.Has(ev.Kind) {
		return
	}
	ev.ID = v.id
	if m.animator.Gesture(&ev) {
		m.applyAnimated()
		m.flush()
		if m.animator.Running() {
			m.startTicking()
		}
	}
	m.h.dispatch(ev)
}

// press reports a tap, a long press or a press going down or up on view
// id at page point page.
func (m *mounter) press(id driver.NodeID, kind driver.EventKind, page driver.Point) {
	v := m.view(id)
	if v == nil || v.props.Disabled && kind == driver.EventPress {
		return
	}
	if kind == driver.EventPress && m.selecting != nil && !m.selectionContains(page) {
		m.clearSelection()
		m.flush()
	}
	if kind == driver.EventLongPress && !v.events.Has(driver.EventLongPress) {
		if m.selectLongPress(v, page) {
			return
		}
	}
	ev := m.pointer(v, kind, page)
	if (kind == driver.EventPress || kind == driver.EventLongPress) && v.layout != nil && v.text != nil {
		// A text's links take the presses on them.
		p := driver.Point{X: ev.Point.X - v.text.Content.Origin.X, Y: ev.Point.Y - v.text.Content.Origin.Y}
		ev.Index = -1
		if c := v.layout.HitTest(p); c.Offset >= 0 {
			ev.Index = driver.UTF16Offset(v.text.Text, c.Offset)
		}
		if !driver.PressesAt(v.text.Spans, ev.Index) {
			if kind == driver.EventLongPress {
				m.selectLongPress(v, page)
			}
			return
		}
	}
	m.send(v, ev)
}

// scrolled reports that scroll view id scrolled to offset.
func (m *mounter) scrolled(id driver.NodeID, offset driver.Point) {
	v := m.view(id)
	if v == nil {
		return
	}
	v.offset = offset
	if m.animator.Scroll(v.id, offset) {
		m.applyAnimated()
		if m.animator.Running() {
			m.startTicking()
		}
	}
	m.stick(v)
	m.flush()
	m.send(v, driver.Event{Kind: driver.EventScroll, Point: offset, Count: v.applied})
}

// scrollPhase reports a drag of scroll view id starting or ending, or
// its scrolling coming to rest.
func (m *mounter) scrollPhase(id driver.NodeID, kind driver.EventKind) {
	if v := m.view(id); v != nil {
		if kind == driver.EventScrollEnd {
			m.animator.Settle(v.id)
		}
		m.send(v, driver.Event{Kind: kind, Point: v.offset, Count: v.applied})
	}
}

// edited reports the user changing text field id's text.
func (m *mounter) edited(id driver.NodeID, text string) {
	if v := m.view(id); v != nil {
		v.edits++
		v.value = text
		m.send(v, driver.Event{Kind: driver.EventChangeText, Text: text, Count: v.edits})
	}
}

// backPressed clears the page-container guard and dispatches EventBack so the
// next ItemHistory re-arms it whenever the app still has history to pop.
func (m *mounter) backPressed() {
	m.back = false
	m.h.dispatch(driver.Event{Kind: driver.EventBack})
}

// field reports a text field's focus, blur or submit.
func (m *mounter) field(id driver.NodeID, kind driver.EventKind, text string) {
	if v := m.view(id); v != nil {
		switch kind {
		case driver.EventFocus:
			m.focused = v
		case driver.EventBlur:
			if m.focused == v {
				m.focused = nil
			}
		}
		m.send(v, driver.Event{Kind: kind, Text: text})
	}
}
