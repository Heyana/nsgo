package core

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
	root     *Scope
	rt       *runtime
	text     TextEngine
	nodes    []*node // index is NodeID; 0 is unused
	free     []NodeID
	view     *node // the surface's root view
	size     Size
	changed  bool
	deleted  []NodeID
	commands []MountItem
	items    []MountItem
	laidOut  []*node
}

type node struct {
	id       NodeID   // 0 for regions
	kind     ViewKind // 0 for regions
	parent   *node
	children []*node
	owner    *Scope

	base   Style
	dyn    Style
	style  Style // base and dyn merged, with the states' styles
	states uint8
	h      handlers
	props  ViewProps
	text   *textNode
	input  *inputNode
	scroll *scrollNode

	frame       Bounds
	layoutDirty bool
	limits      [4]Points // min and max width, min and max height
	cache       []measured

	created       bool
	deleted       bool
	childrenDirty bool
	flat          []*node // native children, through regions
	sent          sent
}

type handlers struct {
	press, longPress, pressDown, pressUp, hoverIn, hoverOut func()
	focus, blur                                             func()
	change, submit                                          func(string)
	scroll                                                  func(Point)
	layout                                                  func(Bounds)
}

type textNode struct {
	content  string
	maxLines int
	style    TextStyle
	prepared PreparedText
	stale    bool // content or style changed since prepared
	box      Bounds
	sentBox  Bounds
	resent   bool // a new prepared text needs a new layout
}

type inputNode struct {
	config    InputConfig
	valueFn   func() string
	value     string // the app's
	shown     string // what the field shows, as far as the engine knows
	count     uint32 // the platform's last edit
	autoFocus bool
	lineH     Points
	lineFor   TextStyle
}

type scrollNode struct {
	config  ScrollConfig
	content Size
	offset  Point
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
}

// NewTree returns an empty tree under root, laying text out with text.
func NewTree(root *Scope, text TextEngine) *Tree {
	t := &Tree{root: root, rt: root.rt, text: text, nodes: []*node{nil}}
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
	}
}

// Size returns the surface size.
func (t *Tree) Size() Size { return t.size }

// Changed reports whether a Commit would produce items.
func (t *Tree) Changed() bool { return t.changed }

// within runs fn with o owning what it creates, untracked, in a batch.
func (t *Tree) within(o *Scope, fn func()) { t.rt.run(o, fn) }

func (t *Tree) newNode(kind ViewKind, parent *node, owner *Scope) *node {
	n := &node{kind: kind, parent: parent, owner: owner, layoutDirty: true, childrenDirty: true,
		limits: [4]Points{Auto, Auto, Auto, Auto}}
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

func (n *node) markLayout() {
	for p := n; p != nil && !(p.layoutDirty && p != n); p = p.parent {
		p.layoutDirty = true
		p.cache = p.cache[:0]
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
	if n.kind != 0 {
		if n.created {
			t.deleted = append(t.deleted, n.id)
		} else {
			t.nodes[n.id] = nil
			t.free = append(t.free, n.id)
		}
	}
	if n.text != nil && n.text.prepared != nil {
		n.text.prepared.Release()
		n.text.prepared = nil
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
	s := n.base.Merge(n.dyn)
	disabled := n.props.Disabled || (n.input != nil && n.input.config.Disabled)
	n.style = s.withStates(n.states, disabled)
	// The state styles stay, so the events they need stay reported.
	n.style.Hover, n.style.Focus, n.style.Active, n.style.Disabled = s.Hover, s.Focus, s.Active, s.Disabled
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
		hoverIn: v.OnHoverIn, hoverOut: v.OnHoverOut, layout: v.OnLayout}
	n.props = ViewProps{Role: v.Role, Label: v.Label, TestID: v.TestID}
	t.bindBool(cx, n, v.Disabled, &n.props.Disabled)
	t.bindBool(cx, n, v.Selected, &n.props.Selected)
	t.bindBool(cx, n, v.Checked, &n.props.Checked)
	for _, c := range v.Children {
		t.mount(cx, n, c)
	}
}

func (t *Tree) mountText(cx *Scope, parent *node, v Text) {
	n := t.newNode(KindText, parent, cx)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	n.h = handlers{press: v.OnPress, layout: v.OnLayout}
	n.props = ViewProps{Role: v.Role, Label: v.Label, TestID: v.TestID}
	if n.props.Role == RoleNone {
		n.props.Role = RoleText
	}
	n.text = &textNode{content: v.Content, maxLines: v.MaxLines, stale: true}
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
	in := &inputNode{valueFn: v.Value, value: v.DefaultValue, autoFocus: v.AutoFocus}
	in.config = InputConfig{Placeholder: v.Placeholder, Masked: v.Masked, Readonly: v.Readonly, KeyboardType: v.KeyboardType}
	n.input = in
	t.bindBool(cx, n, v.Disabled, &in.config.Disabled)
	if v.Value != nil {
		cx.renderEffect(func() {
			in.value = v.Value()
			t.touch(n)
		})
	}
}

func (t *Tree) mountScroll(cx *Scope, parent *node, v Scroll) {
	n := t.newNode(KindScroll, parent, cx)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	n.h = handlers{scroll: v.OnScroll, layout: v.OnLayout}
	n.props = ViewProps{TestID: v.TestID}
	n.scroll = &scrollNode{config: ScrollConfig{Horizontal: v.Horizontal, HideIndicator: v.HideIndicator}}
	for _, c := range v.Children {
		t.mount(cx, n, c)
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
	n := t.node(ev.ID)
	if n == nil {
		return
	}
	h := &n.h
	call := func(fn func()) {
		if fn != nil {
			t.within(n.owner, fn)
		}
	}
	switch ev.Kind {
	case EventPress:
		t.setState(n, statePressed, false)
		if !n.props.Disabled {
			call(h.press)
		}
	case EventLongPress:
		if !n.props.Disabled {
			call(h.longPress)
		}
	case EventPressDown:
		t.setState(n, statePressed, !n.props.Disabled)
		call(h.pressDown)
	case EventPressUp:
		t.setState(n, statePressed, false)
		call(h.pressUp)
	case EventHoverIn:
		t.setState(n, stateHovered, true)
		call(h.hoverIn)
	case EventHoverOut:
		t.setState(n, stateHovered, false)
		call(h.hoverOut)
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
	case EventScroll:
		if n.scroll != nil {
			n.scroll.offset = ev.Point
		}
		if h.scroll != nil {
			t.within(n.owner, func() { h.scroll(ev.Point) })
		}
	}
}

// HitTest returns the innermost pressable view at p in surface points,
// for platforms that draw their own views.
func (t *Tree) HitTest(p Point) NodeID {
	return hit(t.view, p)
}

func hit(n *node, p Point) NodeID {
	if n.style.Display == DisplayNone || !n.frame.Contains(p) {
		return 0
	}
	local := Point{p.X - n.frame.Origin.X, p.Y - n.frame.Origin.Y}
	if n.scroll != nil {
		local.X += n.scroll.offset.X
		local.Y += n.scroll.offset.Y
	}
	kids := n.flatChildren()
	for i := len(kids) - 1; i >= 0; i-- {
		if id := hit(kids[i], local); id != 0 {
			return id
		}
	}
	if n.h.press != nil || n.kind == KindInput {
		return n.id
	}
	return 0
}

// Focus asks the platform to focus node id's text field.
func (t *Tree) focus(n *node) {
	t.commands = append(t.commands, MountItem{Kind: ItemFocus, ID: n.id})
	t.changed = true
}

func (n *node) flatChildren() []*node {
	if n.flat == nil {
		n.flat = appendNative(make([]*node, 0, len(n.children)), n.children)
	}
	return n.flat
}

func appendNative(out []*node, children []*node) []*node {
	for _, c := range children {
		if c.kind == 0 {
			out = appendNative(out, c.children)
		} else {
			out = append(out, c)
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
	bit(EventFocus, h.focus != nil || s.Focus != nil)
	bit(EventBlur, h.blur != nil || s.Focus != nil)
	bit(EventSubmitEditing, h.submit != nil)
	bit(EventScroll, h.scroll != nil)
	bit(EventChangeText, n.kind == KindInput)
	return m
}
