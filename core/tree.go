package core

import (
	"errors"

	"github.com/oneo-me/nsgo/core/internal/layout"
)

// The shadow tree has one node per native view and one per region. A
// region is a span of its parent's children that a Show, Each, Dynamic
// or Catch replaces; it mounts nothing natively and lays its children out
// as its parent's.
//
// The Go element layer builds the tree through a handful of operations:
// create a node, set its parts, place it among its siblings, discard it.
// They are the entry a TSX runtime would drive the same way NSH's does.

// Tree is the shadow tree of one surface. Commit turns its changes into a
// MountBatch. It is not safe for concurrent use: the engine owns it.
type Tree struct {
	root        *Scope
	rt          *runtime
	text        TextEngine
	nodes       []*node // index is NodeID; 0 is unused
	free        []NodeID
	view        *node // the surface's root view
	touches     touchState
	size        Size
	changed     bool
	deleted     []NodeID
	commands    []MountItem
	items       []MountItem
	laidOut     []*node
	reports     []report
	stacks      []*node
	lists       []*node
	accessories []*node
	headerViews []*node
	status      statusBars
	anim        animations
	layoutAnim  *LayoutAnimation
	fontScale   float32
	alerts      map[uint32]alertWait
	reads       map[uint32]readWait
	imageInfos  map[uint32]imageInfoWait
	keys        []*keyBinding
	keysChanged bool
	backs       []*backHandler
	alertID     uint32
	menuActions map[uint32]menuAction
	menuID      uint32
	menuGen     uint32
	rtl         bool // the layout direction the nodes were mapped for
}

// report is a handler to run under owner after the commit.
type report struct {
	owner *node
	fn    func()
}

// after runs fn under n's owner once the commit's items are out.
func (t *Tree) after(n *node, fn func()) { t.reports = append(t.reports, report{n, fn}) }

type node struct {
	id       NodeID   // 0 for regions
	kind     ViewKind // 0 for regions
	parent   *node
	children []*node
	owner    *Scope

	responder *Responder // a view's part in the touch responder

	base   Style
	dyn    Style
	style  Style // base and dyn merged, with the states' styles
	states uint8
	h      handlers
	props  ViewProps
	text   *textNode
	input  *inputNode
	scroll *scrollNode
	image  *imageNode
	hoist  *hoistNode // an overlay's or a modal's
	hosted []*node    // the overlays and modals this root view or modal hosts
	// hidden is set, as the last layout found, for a node DisplayNone
	// hides: its own or a shadow ancestor's, across hoisting too.
	hidden bool
	attach *attachNode
	screen *screenNode
	list   *listNode
	canvas *canvasNode
	icon   *iconNode
	web    *webNode
	video  *videoNode
	native *nativeNode
	// accessory is an InputAccessoryView's: a layout root as wide as the
	// surface that no view holds.
	accessory *accessoryNode
	// headerView is a view of a screen's system header: a layout root
	// at its content size that the platform puts in the bar.
	headerView bool
	panning    bool           // it follows pans: OnPan or PanValues
	pinching   bool           // it follows pinches: OnPinch or PinchValue
	rotating   bool           // it follows rotations: OnRotation or RotationValue
	anim       *viewAnimation // the animated values bound to the view
	isStack    bool
	ref        *Ref

	frame       Bounds
	layoutDirty bool
	// lstate is what layout keeps for the node; lstyle its style mapped for
	// layout, while lstyleOK.
	lstate   layout.State
	lstyle   layout.Style
	lstyleOK bool
	rtl      bool  // laid out right to left
	isRoot   bool  // a layout root: the root view, an overlay or a modal
	fill     *Size // the size a root fills, if it fills one

	created       bool
	deleted       bool
	childrenDirty bool
	flat          []*node // native children, through regions
	sent          sent
}

type handlers struct {
	press, longPress, pressDown, pressUp func(PressEvent)
	hoverIn, hoverOut, focus, blur       func()
	hoverMove                            func(HoverEvent)
	touch                                func(TouchEvent)
	contextMenu                          func(ContextMenuEvent)
	change, submit                       func(string)
	scroll, pressOutside, mouseMove      func(Point)
	layout                               func(Bounds)
	cancel                               func()
	loadStart, loadEnd                   func()
	progress                             func(int64, int64)
	load                                 func(Size)
	loadError                            func(error)
	pan                                  func(PanEvent)
	pinch, rotation                      func(GestureEvent)
	fileEnter, fileLeave                 func()
	fileDrop                             func([]string)
	action                               func(string)
}

type textNode struct {
	content   string
	maxLines  int
	ellipsize EllipsizeMode
	// noScale and maxScale limit the user's text size setting.
	noScale  bool
	maxScale float32
	// fit shrinks the text into its box, down to minScale.
	fit      bool
	minScale float32
	lines    func([]TextLine) // OnTextLayout
	alignV   TextAlignVertical
	spans    []spanNode
	text     AttributedText // what was prepared
	style    TextStyle
	prepared PreparedText
	// fitted is the text shrunk to fit its box, when fit shrank it.
	fitted   PreparedText
	stale    bool // content or style changed since prepared
	box      Bounds
	sentBox  Bounds
	shownBox Bounds // where the layout went: box, moved by its vertical alignment
	resent   bool   // a new prepared text needs a new layout
	// attachments counts the spans that are attachments.
	attachments int
}

// release frees the text's prepared texts.
func (tn *textNode) release() {
	for _, p := range []*PreparedText{&tn.prepared, &tn.fitted} {
		if *p != nil {
			(*p).Release()
			*p = nil
		}
	}
}

// fontMultiplier is what the user's text size setting, scale, multiplies
// the text's sizes by.
func (tn *textNode) fontMultiplier(scale float32) float32 {
	if tn.noScale {
		return 1
	}
	if tn.maxScale >= 1 {
		return min(scale, tn.maxScale)
	}
	return scale
}

type spanNode struct {
	content string
	style   Style
	press   func(PressEvent)
	attach  *node // the view of an attachment
}

// attachNode is the view of a text's attachment: a layout root of its own
// at its content size, which the text places in its lines.
type attachNode struct {
	size Size // what the text was prepared with
	cut  bool // cut off by the text's line limit
}

type inputNode struct {
	config    InputConfig
	valueFn   func() string
	value     string // the app's
	shown     string // what the field shows, as far as the engine knows
	count     uint32 // the platform's last edit
	autoFocus bool
	accessory string // the InputAccessoryView's ID
	lineH     Points
	lineFor   TextStyle

	selection   func(TextRange)
	keyPress    func(string)
	composition func(Composition)
	contentSize func(Size)
	caret       func(CaretEvent)
	paste       func(PasteEvent)
	// measured is what a multiline field is measured with: its value, or
	// its placeholder while empty; sized the value's text, for
	// OnContentSizeChange.
	measured, sized preparedInput
	reported        Size // the content size last reported
	reportedOK      bool
}

// preparedInput is text of a field prepared in its style.
type preparedInput struct {
	text     AttributedText
	prepared PreparedText
}

type scrollNode struct {
	config  ScrollConfig
	content Size
	// offset is the reported offset with the scrolls the platform had not
	// applied yet replayed over it.
	offset Point
	zoom   float32 // the last reported
	on     scrollSpec
	resend bool // EventRefresh came: send the config again
	// issued counts the ItemScrollTo and ItemScrollBy items sent, and
	// pending holds those a report has not shown applied, oldest first.
	issued  uint32
	applied uint32
	pending []pendingScroll
	// by is the ItemScrollBy the lists ask for in this commit, sent with
	// the content size; to the ItemScrollTo their parent-viewport lists
	// ask for.
	by Point
	to *MountItem
	// dependents are the lists in this view's viewport; sticky the cells
	// sent in its last ItemStickyCells.
	dependents []*listNode
	sticky     []StickyCell
}

// pendingScroll is a scroll the core sent and the platform has not shown
// applied.
type pendingScroll struct {
	n  uint32
	to bool // an ItemScrollTo; else an ItemScrollBy
	p  Point
}

// issue records a scroll sent to the platform.
func (sc *scrollNode) issue(to bool, p Point) {
	sc.issued++
	sc.pending = append(sc.pending, pendingScroll{sc.issued, to, p})
	if to {
		sc.offset = p
	} else {
		sc.offset.X += p.X
		sc.offset.Y += p.Y
	}
}

// report records an offset the platform reported after applying applied
// scrolls: those it had not applied yet still count, so a report that
// predates them does not move the content back.
func (sc *scrollNode) report(p Point, applied uint32) {
	sc.applied = max(sc.applied, applied)
	k := 0
	for k < len(sc.pending) && sc.pending[k].n <= sc.applied {
		k++
	}
	sc.pending = sc.pending[k:]
	for _, s := range sc.pending {
		if s.to {
			p = s.p
		} else {
			p.X += s.p.X
			p.Y += s.p.Y
		}
	}
	sc.offset = p
}

// settled reports whether every scroll the core asked for was applied.
func (sc *scrollNode) settled() bool {
	return len(sc.pending) == 0 && sc.by == (Point{}) && sc.to == nil
}

// main is p along the scroll view's axis.
func (sc *scrollNode) main(p Point) Points {
	if sc.config.Horizontal {
		return p.X
	}
	return p.Y
}

// along is a point offset by d along the scroll view's axis.
func (sc *scrollNode) along(d Points) Point {
	if sc.config.Horizontal {
		return Point{X: d}
	}
	return Point{Y: d}
}

type sent struct {
	frame     Bounds
	viewStyle ViewStyle
	props     ViewProps
	events    EventMask
	input     InputConfig
	scroll    ScrollConfig
	content   Size
	children  []*node
	layout    Bounds
	overflow  Bounds // the area ItemOverflow last gave
}

// NewTree returns an empty tree under root, laying text out with text.
func NewTree(root *Scope, text TextEngine) *Tree {
	t := &Tree{root: root, rt: root.rt, text: text, nodes: []*node{nil}}
	t.anim.t = t
	root.rt.tree = t
	t.view = t.newNode(KindView, nil, root)
	t.view.style = Style{Direction: Column}
	return t
}

// Mount builds what app returns as the root view's content.
func (t *Tree) Mount(app func(cx *Scope) Node) {
	t.within(t.root, func() {
		t.mount(t.root, t.view, Func(app))
	})
}

// Resize sets the surface size the root fills.
func (t *Tree) Resize(size Size) {
	if size != t.size {
		t.size = size
		t.view.markLayout()
		t.changed = true
		t.root.Env().window.Set(size)
	}
}

// SetEnvironment records what the platform reports about where the app
// runs.
func (t *Tree) SetEnvironment(env Environment) {
	e := t.root.Env()
	safe, headers := e.safe.GetUntracked(), e.headers
	e.set(env)
	// Modals and system headers are placed by the safe area.
	if e.safe.GetUntracked() != safe || e.headers != headers {
		t.view.markLayout()
	}
	t.changed = true
}

// Size returns the surface size.
func (t *Tree) Size() Size { return t.size }

// Changed reports whether a Commit would produce items.
func (t *Tree) Changed() bool { return t.changed }

// within runs fn with o owning what it creates, untracked, in a batch.
func (t *Tree) within(o *Scope, fn func()) { t.rt.run(o, fn) }

func (t *Tree) newNode(kind ViewKind, parent *node, owner *Scope) *node {
	n := &node{kind: kind, parent: parent, owner: owner, layoutDirty: true, childrenDirty: true, rtl: t.rtl}
	if kind != 0 {
		if k := len(t.free); k > 0 {
			n.id = t.free[k-1]
			t.free = t.free[:k-1]
			t.nodes[n.id] = n
		} else {
			n.id = NodeID(len(t.nodes))
			t.nodes = append(t.nodes, n)
		}
	}
	if parent != nil {
		parent.children = append(parent.children, n)
		t.childrenChanged(parent)
	}
	t.changed = true
	return n
}

func (t *Tree) newRegion(parent *node, owner *Scope) *node {
	return t.newNode(0, parent, owner)
}

// nativeParent is the view n's native view goes into.
func (n *node) nativeParent() *node {
	if n.hoist != nil {
		return n.hoist.host
	}
	p := n.parent
	for p != nil && p.kind == 0 {
		p = p.parent
	}
	return p
}

func (t *Tree) childrenChanged(n *node) {
	for n != nil && n.kind == 0 {
		n = n.parent
	}
	if n != nil {
		n.childrenDirty = true
		n.flat = nil
		n.markLayout()
		t.changed = true
	}
}

// touch records that n's parts changed without a layout change.
func (t *Tree) touch(n *node) { t.changed = true }

// relayout records that n's parts changed its layout.
func (t *Tree) relayout(n *node) {
	n.markLayout()
	t.changed = true
}

// clear discards a region's children.
func (t *Tree) clear(region *node) {
	for _, c := range region.children {
		t.discard(c)
	}
	region.children = region.children[:0]
	t.childrenChanged(region)
}

// discard deletes n's subtree. Its owner disposes the bindings.
func (t *Tree) discard(n *node) {
	if n.deleted {
		return
	}
	n.deleted = true
	if n.ref != nil && n.ref.n == n {
		n.ref.n = nil
	}
	if n.hoist != nil {
		h := n.hoist.host
		h.childrenDirty = true
		h.markLayout()
	}
	if n.kind != 0 {
		if n.created {
			t.deleted = append(t.deleted, n.id)
		} else {
			t.nodes[n.id] = nil
			t.free = append(t.free, n.id)
		}
	}
	if n.text != nil {
		n.text.release()
	}
	if n.input != nil {
		n.input.measured.release()
		n.input.sized.release()
	}
	for _, c := range n.children {
		t.discard(c)
	}
	t.changed = true
}

// setChildren replaces a region's children with nodes already in it.
func (t *Tree) setChildren(region *node, children []*node) {
	region.children = append(region.children[:0], children...)
	t.childrenChanged(region)
}

func (t *Tree) node(id NodeID) *node {
	if int(id) <= 0 || int(id) >= len(t.nodes) {
		return nil
	}
	if n := t.nodes[id]; n != nil && !n.deleted {
		return n
	}
	return nil
}

// Building elements.

// mount appends what el builds under cx to parent's children.
func (t *Tree) mount(cx *Scope, parent *node, el Node) {
	switch el := el.(type) {
	case nil:
	case View:
		t.mountView(cx, parent, el)
	case *View:
		t.mountView(cx, parent, *el)
	case Text:
		t.mountText(cx, parent, el)
	case *Text:
		t.mountText(cx, parent, *el)
	case Input:
		t.mountInput(cx, parent, el)
	case *Input:
		t.mountInput(cx, parent, *el)
	case Scroll:
		t.mountScroll(cx, parent, el)
	case *Scroll:
		t.mountScroll(cx, parent, *el)
	case Image:
		t.mountImage(cx, parent, el)
	case *Image:
		t.mountImage(cx, parent, *el)
	case Overlay:
		t.mountOverlay(cx, parent, el)
	case *Overlay:
		t.mountOverlay(cx, parent, *el)
	case Modal:
		t.mountModal(cx, parent, el)
	case *Modal:
		t.mountModal(cx, parent, *el)
	case Stack:
		t.mountStack(cx, parent, el)
	case *Stack:
		t.mountStack(cx, parent, *el)
	case Screen:
		t.mountScreen(cx, parent, el)
	case *Screen:
		t.mountScreen(cx, parent, *el)
	case Canvas:
		t.mountCanvas(cx, parent, el)
	case *Canvas:
		t.mountCanvas(cx, parent, *el)
	case Icon:
		t.mountIcon(cx, parent, el)
	case *Icon:
		t.mountIcon(cx, parent, *el)
	case WebView:
		t.mountWebView(cx, parent, el)
	case *WebView:
		t.mountWebView(cx, parent, *el)
	case Video:
		t.mountVideo(cx, parent, el)
	case *Video:
		t.mountVideo(cx, parent, *el)
	case Native:
		t.mountNative(cx, parent, el)
	case *Native:
		t.mountNative(cx, parent, *el)
	case InputAccessoryView:
		t.mountAccessory(cx, parent, el)
	case *InputAccessoryView:
		t.mountAccessory(cx, parent, *el)
	case List:
		t.mountList(cx, parent, el)
	case *List:
		t.mountList(cx, parent, *el)
	case Group:
		for _, c := range el {
			t.mount(cx, parent, c)
		}
	case Dynamic:
		t.mountDynamic(cx, parent, el)
	case Show:
		t.mountShow(cx, parent, el)
	case Catch:
		t.mountCatch(cx, parent, el)
	case regionMounter:
		el.mount(t, t.newRegion(parent, cx), cx)
	default:
		child := cx.Child()
		t.within(child, func() { t.mount(child, parent, el.Render(child)) })
	}
}

type regionMounter interface {
	mount(t *Tree, region *node, owner *Scope)
}

// bindStyle sets n's style, binding StyleFunc when there is one.
func (t *Tree) bindStyle(cx *Scope, n *node, base Style, fn func() Style) {
	n.base = base
	n.restyle()
	if fn == nil {
		return
	}
	cx.renderEffect(func() {
		n.dyn = fn()
		n.restyle()
		t.relayout(n)
	})
}

// restyle merges n's style from its parts and states.
func (n *node) restyle() {
	p := n.props
	if n.input != nil && n.input.config.Disabled {
		p.Disabled = true
	}
	n.style = n.base.Merge(n.dyn).withStates(n.states, p)
	n.lstyleOK = false
	if n.screen != nil && n.screen.hidden {
		n.style.Display = DisplayNone
	}
}

// setState records an interaction state and restyles n if a state style
// depends on it.
func (t *Tree) setState(n *node, state uint8, on bool) {
	was := n.states
	if on {
		n.states |= state
	} else {
		n.states &^= state
	}
	if n.states != was && (n.style.Hover != nil || n.style.Focus != nil || n.style.Active != nil) {
		n.restyle()
		t.relayout(n)
	}
}

func (t *Tree) bindBool(cx *Scope, n *node, fn func() bool, dst *bool) {
	if fn == nil {
		return
	}
	cx.renderEffect(func() {
		*dst = fn()
		n.restyle()
		t.relayout(n)
	})
}

func (t *Tree) mountView(cx *Scope, parent *node, v View) {
	n := t.newNode(KindView, parent, cx)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	n.h = handlers{press: v.OnPress, longPress: v.OnLongPress, pressDown: v.OnPressDown, pressUp: v.OnPressUp,
		hoverIn: v.OnHoverIn, hoverOut: v.OnHoverOut, hoverMove: v.OnHoverMove, mouseMove: v.OnMouseMove,
		layout: v.OnLayout, contextMenu: v.OnContextMenu, touch: v.OnTouch,
		pan: v.OnPan, pinch: v.OnPinch, rotation: v.OnRotation,
		fileEnter: v.OnFileDragEnter, fileLeave: v.OnFileDragLeave, fileDrop: v.OnFileDrop}
	n.h.action = v.OnAccessibilityAction
	n.responder = v.Responder
	n.props = ViewProps{Role: v.Role, Label: v.Label, Hint: v.Hint, LiveRegion: v.LiveRegion, TestID: v.TestID,
		WindowDrag: v.WindowDrag, SelectionArea: v.SelectionArea, PointerEvents: v.PointerEvents, HitSlop: v.HitSlop,
		DelayLongPress: v.DelayLongPress, NoFocus: v.NoFocus}
	t.bindAccessibility(cx, n, a11y{labelFunc: v.LabelFunc, value: v.AccessibilityValue, expanded: v.Expanded,
		hidden: v.AccessibilityElementsHidden, actions: v.AccessibilityActions, accessible: v.Accessible,
		important: v.ImportantForAccessibility})
	_, panClaim := v.Responder.panClaim()
	if v.OnPan != nil || v.PanValues != (AnimatedXY{}) || panClaim {
		n.props.PanActivation = v.PanActivation
		n.panning = true
	}
	t.setRef(n, v.Ref)
	t.bindBool(cx, n, v.Disabled, &n.props.Disabled)
	t.bindBool(cx, n, v.Selected, &n.props.Selected)
	t.bindBool(cx, n, v.Checked, &n.props.Checked)
	t.bindBool(cx, n, v.Indeterminate, &n.props.Indeterminate)
	t.bindPan(cx, n, v.PanValues)
	t.bindGestureValue(n, AnimBindPinch, v.PinchValue)
	t.bindGestureValue(n, AnimBindRotation, v.RotationValue)
	n.pinching, n.rotating = v.OnPinch != nil || v.PinchValue != nil, v.OnRotation != nil || v.RotationValue != nil
	for _, c := range v.Children {
		t.mount(cx, n, c)
	}
}

func (t *Tree) mountText(cx *Scope, parent *node, v Text) {
	n := t.newNode(KindText, parent, cx)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	n.h = handlers{press: v.OnPress, layout: v.OnLayout}
	n.props = ViewProps{Role: v.Role, Label: v.Label, Hint: v.Hint, LiveRegion: v.LiveRegion, TestID: v.TestID,
		Selectable: v.Selectable}
	if n.props.Role == RoleNone {
		n.props.Role = RoleText
	}
	t.bindAccessibility(cx, n, a11y{labelFunc: v.LabelFunc})
	t.setRef(n, v.Ref)
	n.text = &textNode{content: v.Content, maxLines: v.MaxLines, ellipsize: v.EllipsizeMode,
		noScale: v.NoFontScaling, maxScale: v.MaxFontSizeMultiplier,
		fit: v.AdjustsFontSizeToFit, minScale: max(v.MinimumFontScale, 0.1), lines: v.OnTextLayout, stale: true}
	for i, sp := range v.Spans {
		if sp.Attachment != nil {
			w := t.newNode(KindView, n, cx)
			w.attach = &attachNode{}
			t.bindStyle(cx, w, sp.Style, nil)
			t.mount(cx, w, sp.Attachment)
			n.text.spans = append(n.text.spans, spanNode{attach: w})
			n.text.attachments++
			continue
		}
		n.text.spans = append(n.text.spans, spanNode{content: sp.Content, style: sp.Style, press: sp.OnPress})
		if sp.OnPress != nil {
			n.h.press = func(PressEvent) {} // presses come with the offset of the span
		}
		if sp.ContentFunc != nil {
			cx.renderEffect(func() {
				if s := sp.ContentFunc(); s != n.text.spans[i].content {
					n.text.spans[i].content = s
					n.text.stale = true
					t.relayout(n)
				}
			})
		}
	}
	if v.ContentFunc != nil {
		cx.renderEffect(func() {
			if s := v.ContentFunc(); s != n.text.content || n.text.prepared == nil {
				n.text.content = s
				n.text.stale = true
				t.relayout(n)
			}
		})
	}
}

func (t *Tree) mountInput(cx *Scope, parent *node, v Input) {
	n := t.newNode(KindInput, parent, cx)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	n.h = handlers{change: v.OnChange, submit: v.OnSubmit, focus: v.OnFocus, blur: v.OnBlur, layout: v.OnLayout}
	n.props = ViewProps{Role: RoleTextField, Label: v.Label, TestID: v.TestID}
	t.setRef(n, v.Ref)
	in := &inputNode{valueFn: v.Value, value: v.DefaultValue, autoFocus: v.AutoFocus, accessory: v.InputAccessoryViewID,
		selection: v.OnSelectionChange, keyPress: v.OnKeyPress, composition: v.OnCompositionChange,
		contentSize: v.OnContentSizeChange, caret: v.OnCaretMove, paste: v.OnPaste}
	in.config = InputConfig{Placeholder: v.Placeholder, Masked: v.Masked, Readonly: v.Readonly, KeyboardType: v.KeyboardType,
		Multiline: v.Multiline, MaxLength: max(v.MaxLength, 0), SubmitBehavior: v.SubmitBehavior, ReturnKey: v.ReturnKeyType,
		AutoCapitalize: v.AutoCapitalize, NoAutoCorrect: v.NoAutoCorrect, AutoComplete: v.AutoComplete,
		KeyboardAppearance: v.KeyboardAppearance, HideCaret: v.HideCaret, PasteBehavior: v.PasteBehavior}
	n.input = in
	t.bindBool(cx, n, v.Disabled, &in.config.Disabled)
	if v.Value != nil {
		cx.renderEffect(func() {
			in.value = v.Value()
			t.touch(n)
		})
	}
	if v.SelectedRange != nil {
		// After the value, which the commit sends before its commands.
		cx.renderEffect(func() {
			r := v.SelectedRange()
			t.commands = append(t.commands, MountItem{Kind: ItemSelection, ID: n.id, Selection: r})
			t.changed = true
		})
	}
}

// scrollDefaults lets a scroll view shrink below its content, as React
// Native's ScrollView does; its content scrolls instead.
var scrollDefaults = Style{FlexShrink: 1}

func (t *Tree) mountScroll(cx *Scope, parent *node, v Scroll) {
	n := t.newNode(KindScroll, parent, cx)
	t.bindStyle(cx, n, scrollDefaults.Merge(v.Style), v.StyleFunc)
	n.h = handlers{layout: v.OnLayout}
	if on := v.OnScroll; on != nil {
		n.h.scroll = func(p Point) { on(n.scrollEvent(p)) }
	}
	n.props = ViewProps{TestID: v.TestID}
	t.setRef(n, v.Ref)
	n.scroll = &scrollNode{config: ScrollConfig{Horizontal: v.Horizontal, HideIndicator: v.HideIndicator}}
	t.bindScrollSpec(cx, n, scrollSpec{behavior: v.Behavior, behaviorFunc: v.BehaviorFunc, offset: v.ContentOffset,
		refreshing: v.Refreshing, onRefresh: v.OnRefresh, onScroll: v.OnScroll,
		onDragStart: v.OnScrollDragStart, onDragEnd: v.OnScrollDragEnd,
		onMomentum: v.OnMomentumStart, onMomentumEnd: v.OnMomentumEnd,
		minZoom: v.MinZoomScale, maxZoom: v.MaxZoomScale, zoom: v.ZoomScale, zoomFunc: v.ZoomScaleFunc,
		noBouncesZoom: v.NoBouncesZoom, noPinch: v.NoPinchZoom, centerContent: v.CenterContent})
	t.bindScroll(cx, n, v.OffsetValues)
	for _, c := range v.Children {
		t.mount(cx, n, c)
	}
}

func (t *Tree) setRef(n *node, r *Ref) {
	if r != nil {
		r.t, r.n = t, n
		n.ref = r
	}
}

// branch builds what fn returns in region under a new scope of owner's.
func (t *Tree) branch(owner *Scope, region *node, fn func(cx *Scope) Node) *Scope {
	s := &Scope{}
	owner.adopt(s)
	if fn != nil {
		t.within(s, func() { t.mount(s, region, fn(s)) })
	}
	return s
}

func (t *Tree) mountDynamic(cx *Scope, parent *node, d Dynamic) {
	region := t.newRegion(parent, cx)
	var b *Scope
	cx.renderEffect(func() {
		if b != nil {
			b.Dispose()
			t.clear(region)
		}
		b = &Scope{}
		cx.adopt(b)
		// fn's own reads are tracked; what it returns mounts untracked.
		rt := t.rt
		prevOwner := rt.owner
		rt.owner = b
		var el Node
		rt.guard(b, func() { el = d(b) })
		rt.owner = prevOwner
		t.within(b, func() { t.mount(b, region, el) })
	})
}

func (t *Tree) mountShow(cx *Scope, parent *node, s Show) {
	region := t.newRegion(parent, cx)
	when := NewMemo(cx, s.When)
	var b *Scope
	cx.renderEffect(func() {
		on := when.Get()
		if b != nil {
			b.Dispose()
			t.clear(region)
		}
		if on {
			b = t.branch(cx, region, s.Then)
		} else {
			b = t.branch(cx, region, s.Else)
		}
	})
}

func (t *Tree) mountCatch(cx *Scope, parent *node, c Catch) {
	region := t.newRegion(parent, cx)
	failure := NewSignalFunc[error](cx, nil, nil)
	reset := func() { failure.Set(nil) }
	var b *Scope
	cx.renderEffect(func() {
		err := failure.Get()
		if b != nil {
			b.Dispose()
			t.clear(region)
		}
		b = &Scope{}
		cx.adopt(b)
		if err == nil {
			b.catch = func(err error) { failure.Set(err) }
			t.within(b, func() { t.mount(b, region, c.Try(b)) })
		} else if c.Fallback != nil {
			t.within(b, func() { t.mount(b, region, c.Fallback(b, err, reset)) })
		}
	})
}

// Events.

// Dispatch handles a platform event.
func (t *Tree) Dispatch(ev Event) {
	switch ev.Kind {
	case EventTouch:
		t.touched(ev)
		return
	case EventBack:
		if n := t.node(ev.ID); n != nil && n.screen != nil {
			t.backFrom(n)
		} else {
			t.back()
		}
		return
	case EventRequestClose, EventModalSize:
		if n := t.node(ev.ID); n != nil && n.hoist != nil && n.hoist.modal != nil {
			t.modalEvent(n, ev)
		}
		return
	case EventAlert:
		t.answerAlert(ev.Count, ev.Index)
		return
	case EventClipboard:
		t.answerRead(ev.Count, ev.Text)
		return
	case EventImageInfo:
		t.answerImageInfo(ev)
		return
	case EventKey:
		t.pressKey(ev.Text)
		return
	case EventMenu:
		t.chooseMenu(ev.Count)
		return
	case EventAnimation:
		t.animationEvent(ev)
		return
	}
	n := t.node(ev.ID)
	if n == nil {
		return
	}
	if n.web != nil {
		t.dispatchWeb(n, ev)
		return
	}
	if n.video != nil && t.dispatchVideo(n, ev) {
		return
	}
	if n.native != nil && t.dispatchNative(n, ev) {
		return
	}
	h := &n.h
	call := func(fn func()) {
		if fn != nil {
			t.within(n.owner, fn)
		}
	}
	press := func(fn func(PressEvent)) {
		if fn != nil {
			t.within(n.owner, func() { fn(ev.press()) })
		}
	}
	switch ev.Kind {
	case EventPress:
		t.setState(n, statePressed, false)
		if n.props.Disabled {
			break
		}
		if tn := n.text; tn != nil && len(tn.spans) > 0 {
			if sp := tn.spanAt(ev.Index); sp != nil && sp.press != nil {
				press(sp.press)
			}
			break
		}
		press(h.press)
	case EventLongPress:
		if !n.props.Disabled {
			press(h.longPress)
		}
	case EventAccessibilityAction:
		if h.action != nil && !n.props.Disabled {
			t.within(n.owner, func() { h.action(ev.Text) })
		}
	case EventPressDown:
		t.setState(n, statePressed, !n.props.Disabled)
		press(h.pressDown)
	case EventPressUp:
		t.setState(n, statePressed, false)
		press(h.pressUp)
	case EventHoverIn:
		t.setState(n, stateHovered, true)
		call(h.hoverIn)
	case EventHoverOut:
		t.setState(n, stateHovered, false)
		call(h.hoverOut)
	case EventHoverMove:
		if h.hoverMove != nil {
			t.within(n.owner, func() { h.hoverMove(HoverEvent{ev.Point, ev.Page, ev.Modifiers}) })
		}
	case EventMouseMove:
		if h.mouseMove != nil {
			t.within(n.owner, func() { h.mouseMove(ev.Point) })
		}
	case EventFocus:
		t.setState(n, stateFocused, true)
		call(h.focus)
	case EventBlur:
		t.setState(n, stateFocused, false)
		call(h.blur)
	case EventChangeText:
		in := n.input
		if in == nil {
			return
		}
		in.shown, in.value, in.count = ev.Text, ev.Text, ev.Count
		if h.change != nil {
			t.within(n.owner, func() { h.change(ev.Text) })
		}
		// A controlled field the app did not follow goes back to its value.
		if in.valueFn != nil {
			if v := Untracked(t.root, in.valueFn); v != in.value {
				in.value = v
			}
		}
		t.touch(n)
	case EventSubmitEditing:
		if h.submit != nil {
			t.within(n.owner, func() { h.submit(ev.Text) })
		}
	case EventSelectionChange:
		if in := n.input; in != nil && in.selection != nil {
			t.within(n.owner, func() { in.selection(TextRange{ev.Index, int(ev.Count)}) })
		}
	case EventKeyPress:
		if in := n.input; in != nil && in.keyPress != nil {
			t.within(n.owner, func() { in.keyPress(ev.Text) })
		}
	case EventCompositionChange:
		if in := n.input; in != nil && in.composition != nil {
			c := Composition{Composing: !ev.Finished, Text: ev.Text}
			if ev.Finished {
				c.Text = ""
			}
			t.within(n.owner, func() { in.composition(c) })
		}
	case EventCaretMove:
		if in := n.input; in != nil && in.caret != nil {
			b := Bounds{Origin: ev.Point, Size: ev.Size}
			page := b
			if b.Size != (Size{}) {
				o := t.surfaceBounds(n).Origin
				page.Origin = Point{o.X + b.Origin.X, o.Y + b.Origin.Y}
			}
			t.within(n.owner, func() { in.caret(CaretEvent{Bounds: b, PageBounds: page}) })
		}
	case EventPaste:
		if in := n.input; in != nil && in.paste != nil {
			if p, ok := ev.Data.(*PasteEvent); ok {
				t.within(n.owner, func() { in.paste(*p) })
			}
		}
	case EventPan:
		if ev.Phase == GestureBegan {
			t.panActivated(n)
		}
		if h.pan != nil {
			if ev.Phase == GestureBegan {
				t.setState(n, statePressed, false) // the pan took the press
			}
			t.within(n.owner, func() {
				h.pan(PanEvent{Phase: ev.Phase, Translation: ev.Point, Velocity: ev.Velocity, Values: ev.Values})
			})
		}
	case EventPinch, EventRotation:
		fn := h.pinch
		if ev.Kind == EventRotation {
			fn = h.rotation
		}
		if fn != nil {
			t.within(n.owner, func() { fn(GestureEvent{Phase: ev.Phase, Value: ev.Value, Velocity: float32(ev.Velocity.X)}) })
		}
	case EventContextMenu:
		if h.contextMenu != nil && !n.props.Disabled {
			t.within(n.owner, func() { h.contextMenu(ContextMenuEvent{ev.Point, ev.Page, ev.Modifiers}) })
		}
	case EventPressOutside:
		if n.hoist != nil {
			t.pressOutside(n, ev.Point)
		}
	case EventFileDragEnter:
		call(h.fileEnter)
	case EventFileDragLeave:
		call(h.fileLeave)
	case EventFileDrop:
		if h.fileDrop != nil && !n.props.Disabled {
			t.within(n.owner, func() { h.fileDrop(ev.URIs) })
		}
	case EventLoadStart:
		call(h.loadStart)
	case EventLoadEnd:
		call(h.loadEnd)
	case EventProgress:
		if h.progress != nil {
			t.within(n.owner, func() { h.progress(ev.Loaded, ev.Total) })
		}
	case EventLoad:
		if h.load != nil {
			t.within(n.owner, func() { h.load(ev.Size) })
		}
	case EventError:
		if h.loadError != nil {
			t.within(n.owner, func() { h.loadError(errors.New(ev.Text)) })
		}
	case EventScrollDragStart, EventScrollDragEnd, EventMomentumStart, EventMomentumEnd, EventRefresh:
		if n.scroll != nil {
			t.scrolled(n, ev)
			t.dispatchScroll(n, ev)
		}
	case EventScroll:
		if n.scroll != nil {
			t.scrolled(n, ev)
			if ev.Value > 0 {
				n.scroll.zoom = ev.Value
			}
			if anchored(t.view) {
				t.view.markLayout() // anchored overlays follow the scroll
				t.changed = true
			}
		}
		if h.scroll != nil {
			t.within(n.owner, func() { h.scroll(ev.Point) })
		}
	}
}

// spanAt returns the span holding byte offset i of the text.
func (tn *textNode) spanAt(i int) *spanNode {
	for k, sp := range tn.text.Spans {
		if i >= sp.Start && i < sp.End && k < len(tn.spans) {
			return &tn.spans[k]
		}
	}
	return nil
}

// HitTest returns the innermost pressable view at p in surface points,
// for platforms that draw their own views.
func (t *Tree) HitTest(p Point) NodeID {
	id, _ := hit(t.view, p)
	return id
}

// PressAt dispatches ev as a press on the innermost pressable view at its
// Page, in surface points, with its Point in that view.
func (t *Tree) PressAt(ev Event) {
	if id, local := hit(t.view, ev.Page); id != 0 {
		ev.Kind, ev.ID, ev.Point = EventPress, id, local
		t.Dispatch(ev)
	}
}

func hit(n *node, p Point) (NodeID, Point) {
	return hitWith(n, p, func(n *node) bool { return n.h.press != nil || n.kind == KindInput })
}

// hitWith returns the innermost view at p in n's parent that ok accepts,
// and p in it.
func hitWith(n *node, p Point, ok func(*node) bool) (NodeID, Point) {
	self, inside := Reach(n.frame, &n.props, n.sent.overflow, p)
	if n.style.Display == DisplayNone || n.hidden || !self && !inside || (n.attach != nil && n.attach.cut) {
		return 0, Point{}
	}
	local := Point{p.X - n.frame.Origin.X, p.Y - n.frame.Origin.Y}
	at := local
	if n.scroll != nil {
		local.X += n.scroll.offset.X
		local.Y += n.scroll.offset.Y
	}
	if inside {
		kids := n.nativeChildren()
		for i := len(kids) - 1; i >= 0; i-- {
			if id, in := hitWith(kids[i], local, ok); id != 0 {
				return id, in
			}
		}
	}
	if self && ok(n) {
		return n.id, at
	}
	return 0, Point{}
}

// command queues a command item for n after the next commit's changes.
func (t *Tree) command(kind ItemKind, n *node) {
	t.commands = append(t.commands, MountItem{Kind: kind, ID: n.id})
	t.changed = true
}

// flatChildren returns the views laid out as n's children: its children
// and those of its regions, without the overlays and modals it hoists.
func (n *node) flatChildren() []*node {
	if n.flat == nil {
		n.flat = appendNative(make([]*node, 0, len(n.children)), n.children)
	}
	return n.flat
}

func appendNative(out []*node, children []*node) []*node {
	for _, c := range children {
		switch {
		case c.kind == 0:
			out = appendNative(out, c.children)
		case c.hoist == nil && c.attach == nil && c.accessory == nil && !c.headerView:
			out = append(out, c)
		}
	}
	return out
}

// nativeChildren returns n's native children: the views it lays out, a
// text's attachments, then the overlays and modals it hosts, last on top.
func (n *node) nativeChildren() []*node {
	attached := n.text != nil && n.text.attachments > 0
	if len(n.hosted) == 0 && !attached {
		return n.flatChildren()
	}
	out := append([]*node(nil), n.flatChildren()...)
	if attached {
		for _, sp := range n.text.spans {
			if sp.attach != nil {
				out = append(out, sp.attach)
			}
		}
	}
	for _, h := range n.hosted {
		if !h.deleted {
			out = append(out, h)
		}
	}
	return out
}

func (n *node) events() EventMask {
	var m EventMask
	bit := func(k EventKind, on bool) {
		if on {
			m |= 1 << k
		}
	}
	h, s := &n.h, &n.style
	bit(EventPress, h.press != nil)
	bit(EventLongPress, h.longPress != nil)
	bit(EventPressDown, h.pressDown != nil || s.Active != nil)
	bit(EventPressUp, h.pressUp != nil || s.Active != nil)
	bit(EventHoverIn, h.hoverIn != nil || s.Hover != nil)
	bit(EventHoverOut, h.hoverOut != nil || s.Hover != nil)
	bit(EventHoverMove, h.hoverMove != nil)
	bit(EventMouseMove, h.mouseMove != nil)
	bit(EventTouch, n.hearsTouches())
	bit(EventFocus, h.focus != nil || s.Focus != nil)
	bit(EventBlur, h.blur != nil || s.Focus != nil)
	bit(EventSubmitEditing, h.submit != nil)
	bit(EventScroll, h.scroll != nil)
	bit(EventPressOutside, h.pressOutside != nil)
	bit(EventContextMenu, h.contextMenu != nil)
	bit(EventLoadStart, h.loadStart != nil)
	bit(EventProgress, h.progress != nil)
	bit(EventLoad, h.load != nil)
	bit(EventError, h.loadError != nil)
	bit(EventLoadEnd, h.loadEnd != nil)
	bit(EventChangeText, n.kind == KindInput)
	if in := n.input; in != nil {
		bit(EventSelectionChange, in.selection != nil)
		bit(EventKeyPress, in.keyPress != nil)
		bit(EventCompositionChange, in.composition != nil)
		bit(EventCaretMove, in.caret != nil)
		bit(EventPaste, in.paste != nil)
	}
	bit(EventPan, n.panning)
	bit(EventPinch, n.pinching)
	bit(EventRotation, n.rotating)
	bit(EventFileDragEnter, h.fileEnter != nil)
	bit(EventFileDragLeave, h.fileLeave != nil)
	bit(EventFileDrop, h.fileDrop != nil)
	bit(EventAccessibilityAction, h.action != nil)
	if n.native != nil {
		bit(EventNative, n.native.event != nil)
		bit(EventNativeSize, true)
	}
	if n.web != nil {
		m |= n.web.events()
	}
	if n.video != nil {
		m &^= maskOf(EventLoadStart, EventProgress, EventLoad, EventError)
		m |= n.video.events()
	}
	if n.scroll != nil {
		m |= n.scroll.events()
	}
	return m
}
