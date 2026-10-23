package core

import "time"

// Node is what a tree is made of: a host element (View, Text, Input,
// Scroll), a region (Group, Dynamic, Show, Each, Catch) or a component.
//
// A component is any type whose Render returns its nodes; its fields are
// its props. Render runs once, under a scope of the component's own, and
// signals keep what it returned up to date. Host elements and regions
// return themselves.
type Node interface {
	Render(cx *Scope) Node
}

// Func is a component written as a function.
type Func func(cx *Scope) Node

// Render implements Node.
func (f Func) Render(cx *Scope) Node { return f(cx) }

// Group is several nodes in a row, laid out as children of the parent.
type Group []Node

// Render implements Node.
func (g Group) Render(*Scope) Node { return g }

// View is a container. Its look is its style; it has none of its own.
type View struct {
	Style Style
	// StyleFunc's result is merged over Style, again when what it reads
	// changes.
	StyleFunc func() Style
	Children  []Node

	// OnPress runs on a completed press: tap, click or accessibility
	// action. Pressable views capture presses on their descendants.
	OnPress     func()
	OnLongPress func()
	OnPressDown func()
	OnPressUp   func()
	OnHoverIn   func()
	OnHoverOut  func()
	// OnContextMenu receives a secondary click at a point in the view.
	OnContextMenu func(at Point)
	// OnLayout receives the view's frame after each layout that moves or
	// sizes it.
	OnLayout func(Bounds)

	// OnPan follows a drag on the view, once PanActivation activates it;
	// an active pan takes the touch from presses and scroll views.
	OnPan         func(PanEvent)
	PanActivation PanActivation
	// OnPinch follows a two-finger pinch (a trackpad's on desktops);
	// OnRotation a two-finger rotation.
	OnPinch    func(GestureEvent)
	OnRotation func(GestureEvent)
	// PanValues are animated values a drag on the view moves on the
	// platform's main thread, to its translation, with no engine work per
	// move. They need no OnPan.
	PanValues AnimatedXY
	// OnFileDrop receives the file: URLs of files dropped on the view from
	// another app, on desktops and the web; OnFileDragEnter and
	// OnFileDragLeave follow them while they are dragged over it. The
	// innermost view with OnFileDrop under the pointer takes them, unless
	// it is Disabled.
	OnFileDrop      func(uris []string)
	OnFileDragEnter func()
	OnFileDragLeave func()
	// WindowDrag makes the view a handle that moves the desktop window.
	WindowDrag WindowDrag
	// SelectionArea makes the texts inside select together, as a web
	// page's do. base.SelectionArea sets it.
	SelectionArea bool

	Ref *Ref
	// Disabled stops presses and is reported to assistive technology.
	Disabled func() bool
	Selected func() bool
	Checked  func() bool
	// Indeterminate is neither checked nor unchecked, as a "select all"
	// over a partial selection.
	Indeterminate func() bool
	Role          Role
	Label         string
	TestID        string
}

// Render implements Node.
func (v View) Render(*Scope) Node { return v }

// Text is a run of text the engine lays out and the platform draws.
type Text struct {
	Content string
	// ContentFunc replaces Content, again when what it reads changes.
	ContentFunc func() string
	Style       Style
	StyleFunc   func() Style
	// MaxLines limits the lines shown, ending the last with an ellipsis.
	MaxLines int
	// Spans, when set, are the text: runs in order, each styled over the
	// Text's style. A span with OnPress is a link.
	Spans []Span
	// Selectable lets the user select and copy the text; the highlight is
	// the style's SelectionColor.
	Selectable Selectable

	OnPress  func()
	OnLayout func(Bounds)
	Ref      *Ref
	Role     Role
	Label    string
	TestID   string
}

// Render implements Node.
func (t Text) Render(*Scope) Node { return t }

// Selectable says whether the user can select a text.
type Selectable uint8

const (
	// SelectableAuto selects the text with its selection area when it is
	// in one. Elsewhere only the web selects it, on desktops, as a page's
	// text.
	SelectableAuto Selectable = iota
	// SelectableOn selects the text, alone when it is in no area.
	SelectableOn
	// SelectableOff keeps the text out of selections, its area's too.
	SelectableOff
)

// Span is a run of a Text with a style of its own.
type Span struct {
	Content     string
	ContentFunc func() string
	// Style's text fields merge over the Text's. Its Background and
	// CornerRadius draw behind the span's characters, a box per line.
	Style   Style
	OnPress func()
	// Attachment, when set, is a view in the text's lines in place of
	// Content: laid out at its content size, it wraps with the words
	// around it and takes its own presses. Style is then its view's
	// style, and Style.VerticalAlign places it in its line, else the
	// VerticalAlign of the one view Attachment makes. Copying text leaves
	// attachments out.
	Attachment Node
}

// Input is the platform's single-line text field. With Value it is
// controlled: the app updates the value in OnChange.
type Input struct {
	Value        func() string
	DefaultValue string
	Placeholder  string
	// Masked hides the characters, for passwords.
	Masked       bool
	Readonly     bool
	Disabled     func() bool
	KeyboardType KeyboardType
	AutoFocus    bool
	// InputAccessoryViewID names the InputAccessoryView shown above the
	// software keyboard while the field has focus.
	InputAccessoryViewID string
	Style                Style
	StyleFunc            func() Style

	OnChange func(text string)
	OnSubmit func(text string)
	OnFocus  func()
	OnBlur   func()
	OnLayout func(Bounds)
	Ref      *Ref
	Label    string
	TestID   string
}

// Render implements Node.
func (i Input) Render(*Scope) Node { return i }

// Scroll scrolls its children, laid out as a column (or a row when
// Horizontal) as long as they need. Like React Native's ScrollView, it
// shrinks to fit its parent (FlexShrink 1) unless its style says else.
type Scroll struct {
	Horizontal    bool
	HideIndicator bool
	Style         Style
	StyleFunc     func() Style
	Children      []Node
	// OnScroll receives the offset of the content as it scrolls.
	OnScroll func(offset Point)
	// OffsetValues follow the offset on the platform's main thread, in
	// the same frame as it scrolls.
	OffsetValues AnimatedXY
	OnLayout     func(Bounds)
	Ref          *Ref
	TestID       string
}

// Render implements Node.
func (s Scroll) Render(*Scope) Node { return s }

// Ref refers to a mounted element: an overlay's anchor, focus, its
// bounds. Its zero value is ready to use; pass its address in an
// element's Ref field.
type Ref struct {
	t *Tree
	n *node
}

// Bounds returns the element's frame in surface points after the last
// commit, and false while it is not mounted.
func (r *Ref) Bounds() (Bounds, bool) {
	if r.n == nil || r.n.deleted {
		return Bounds{}, false
	}
	return r.t.surfaceBounds(r.n), true
}

// Focus moves keyboard focus to the element, a text field.
func (r *Ref) Focus() {
	if r.n != nil && !r.n.deleted {
		r.t.command(ItemFocus, r.n)
	}
}

// ScrollTo scrolls the element, a Scroll or List, so its content's
// offset is at.
func (r *Ref) ScrollTo(at Point, animated bool) {
	if r.n == nil || r.n.deleted || r.n.scroll == nil {
		return
	}
	r.t.commands = append(r.t.commands, MountItem{Kind: ItemScrollTo, ID: r.n.id, Point: at, Animated: animated})
	r.t.changed = true
}

// ScrollToIndex scrolls the element, a List, so row index is at the top
// (or the start of a horizontal list).
func (r *Ref) ScrollToIndex(index int, animated bool) {
	if r.n == nil || r.n.deleted || r.n.list == nil {
		return
	}
	ln := r.n.list
	at := ln.headerSize() + ln.sizes.sum(max(min(index, ln.n), 0))
	if ln.spec.Horizontal {
		r.ScrollTo(Point{X: at}, animated)
	} else {
		r.ScrollTo(Point{Y: at}, animated)
	}
}

// Blur takes keyboard focus away from the element.
func (r *Ref) Blur() {
	if r.n != nil && !r.n.deleted {
		r.t.command(ItemBlur, r.n)
	}
}

// Image shows a picture the platform loads, decodes and caches. Layout
// knows only the size Source declares: one dimension derives the other,
// and an image without one is 0 by 0 until its style sizes it.
type Image struct {
	Source       ImageSource
	SourceFunc   func() ImageSource
	Placeholder  ImageSource
	ResizeMode   ResizeMode
	BlurRadius   Points
	FadeDuration time.Duration // 0 is the platform's
	Style        Style
	StyleFunc    func() Style

	OnLoadStart func()
	// OnProgress receives bytes loaded and the total, 0 when unknown.
	OnProgress func(loaded, total int64)
	// OnLoad receives the source's size in pixels.
	OnLoad    func(pixels Size)
	OnError   func(err error)
	OnLoadEnd func()
	OnPress   func()
	OnLayout  func(Bounds)
	Ref       *Ref
	Label     string
	TestID    string
}

// Render implements Node.
func (i Image) Render(*Scope) Node { return i }

// ImageSource is where an image comes from: a URI (https, file path,
// file:, data:) or bytes, such as a file embedded with go:embed.
type ImageSource struct {
	URI  string
	Data []byte
	// Size is the image's size in points, when known up front.
	Size Size
	// Scales are the pixel densities an "asset:" image ships in (a.png,
	// a@2x.png, ...): the core mounts the one for the screen's pixel
	// ratio. Empty for a single file.
	Scales ImageScales
}

// Src returns a source for uri.
func Src(uri string) ImageSource { return ImageSource{URI: uri} }

func (s ImageSource) same(o ImageSource) bool {
	return s.URI == o.URI && s.Size == o.Size && s.Scales == o.Scales && len(s.Data) == len(o.Data) &&
		(len(s.Data) == 0 || &s.Data[0] == &o.Data[0])
}

// ResizeMode fits an image or a video into its view. The zero value is
// the element's default: Cover for an image, Contain for a video, as in
// NSH. Platforms receive the resolved mode.
type ResizeMode uint8

const (
	// Cover fills the view, cropping what overflows.
	Cover ResizeMode = iota + 1
	// Contain fits the whole image, leaving space.
	Contain
	// Stretch fills the view, distorting the image.
	Stretch
	// Center keeps the image's size, centered.
	Center
)

// Overlay floats its children above everything in the nearest Modal or
// the surface, outside the flow: placed beside Anchor on Placement, or by
// its style's Top, Right, Bottom and Left without one. It never closes
// itself; the app removes it.
type Overlay struct {
	Anchor    *Ref
	Placement Placement
	// Align places the overlay along the anchor's side; centered when 0.
	Align Align
	// Offset is the gap to the anchor; Margin keeps the overlay off the
	// edges of its host.
	Offset, Margin Points
	Style          Style
	StyleFunc      func() Style
	Children       []Node

	// OnPressOutside receives presses outside the overlay and its anchor.
	OnPressOutside func(at Point)
	// OnCancel receives Escape or back while it is the topmost overlay
	// that listens.
	OnCancel   func()
	OnPosition func(Placement)
	Role       Role
	Label      string
	TestID     string
}

// Render implements Node.
func (o Overlay) Render(*Scope) Node { return o }

// Placement is the side of its anchor an overlay goes on.
type Placement uint8

const (
	// PlaceDefault is the component's own default: below for overlays.
	PlaceDefault Placement = iota
	PlaceBottom
	PlaceTop
	PlaceLeft
	PlaceRight
)

// Modal covers its host, the surface or an outer modal, with a layer of
// its own laid out at the host's size. Mounting it presents it, and
// removing it dismisses it.
type Modal struct {
	Animation ModalAnimation
	Style     Style
	StyleFunc func() Style
	Children  []Node
	// OnRequestClose receives Escape or back ("cancel") while the modal is
	// the topmost; the app decides whether to remove it.
	OnRequestClose func(reason string)
	Role           Role
	Label          string
	TestID         string
}

// Render implements Node.
func (m Modal) Render(*Scope) Node { return m }

// ModalAnimation is how a modal appears.
type ModalAnimation uint8

const (
	ModalSlide ModalAnimation = iota
	ModalFade
	ModalNone
)

// Stack shows the last of its screens: pushing is adding a screen,
// popping is removing it. Back pops the top screen, which reports
// OnDismiss and stays hidden until the app removes it.
type Stack struct {
	Style    Style
	Children []Node
	TestID   string
}

// Render implements Node.
func (s Stack) Render(*Scope) Node { return s }

// Screen is one page of a Stack, filling it. Header, when set, is drawn
// at its top and keeps the content's insets clear of it.
type Screen struct {
	Title    string
	Header   Node
	Style    Style
	Children []Node
	// NoPopOnBack keeps the screen when back is pressed.
	NoPopOnBack bool
	// OnBackPress receives every back press while the screen is on top.
	OnBackPress func()
	// OnDismiss reports that the user popped the screen; remove it.
	OnDismiss func()
	TestID    string
}

// Render implements Node.
func (s Screen) Render(*Scope) Node { return s }
