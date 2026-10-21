package core

// Node is one mounted element in the shadow tree.
type Node struct {
	id       NodeID
	tag      Tag
	style    Style
	text     string
	value    string
	parent   *Node
	children []*Node
	frame    Bounds

	placeholder string
	role        Role
	onPress     func()
	onChange    func(string)
	onSubmit    func(string)

	created   bool
	inserted  bool
	sentFrame Bounds
	sentProps Props
}

// Tree is the shadow tree of one surface. Commit turns its changes into a
// MountBatch. It is not safe for concurrent use: the engine owns it.
type Tree struct {
	cx    *Scope
	text  TextEngine
	nodes []*Node // index is NodeID; 0 is unused
	root  *Node
	size  Size
	dirty bool

	cache map[measureKey]Size
	items []MountItem
}

type measureKey struct {
	text     string
	font     Font
	maxWidth Points
}

// NewTree returns an empty tree measuring text with text.
func NewTree(cx *Scope, text TextEngine) *Tree {
	return &Tree{cx: cx, text: text, nodes: []*Node{nil}, cache: map[measureKey]Size{}}
}

// Mount builds the nodes of el as the tree's root.
func (t *Tree) Mount(el *Element) {
	t.root = t.build(el, nil, 0)
	t.dirty = true
}

func (t *Tree) build(el *Element, parent *Node, index int) *Node {
	n := &Node{
		id:          NodeID(len(t.nodes)),
		tag:         el.tag,
		style:       el.style,
		parent:      parent,
		placeholder: el.placeholder,
		role:        el.role,
		onPress:     el.onPress,
		onChange:    el.onChange,
		onSubmit:    el.onSubmit,
	}
	n.style.normalize()
	t.nodes = append(t.nodes, n)
	if el.text != nil {
		t.cx.Effect(func() {
			n.text = el.text()
			t.dirty = true
		})
	}
	if el.value != nil {
		t.cx.Effect(func() {
			n.value = el.value()
			t.dirty = true
		})
	}
	for i, c := range el.children {
		n.children = append(n.children, t.build(c, n, i))
	}
	return n
}

// Resize sets the surface size the root fills.
func (t *Tree) Resize(size Size) {
	if size != t.size {
		t.size = size
		t.dirty = true
	}
}

// Press runs the press handler of node id, if it has one.
func (t *Tree) Press(id NodeID) {
	if n := t.node(id); n != nil && n.onPress != nil {
		t.cx.Batch(n.onPress)
	}
}

// Change records an input's edited text and runs its change handler.
func (t *Tree) Change(id NodeID, text string) {
	n := t.node(id)
	if n == nil {
		return
	}
	// The native field already shows text: record it as sent so a value
	// that follows the edit is not mounted back.
	n.value = text
	n.sentProps.Text = text
	if n.onChange != nil {
		t.cx.Batch(func() { n.onChange(text) })
	}
}

// Submit runs an input's submit handler.
func (t *Tree) Submit(id NodeID, text string) {
	if n := t.node(id); n != nil && n.onSubmit != nil {
		t.cx.Batch(func() { n.onSubmit(text) })
	}
}

// HitTest returns the innermost pressable node at p, in surface points.
func (t *Tree) HitTest(p Point) NodeID {
	if t.root == nil {
		return 0
	}
	return hit(t.root, p)
}

func hit(n *Node, p Point) NodeID {
	if !n.frame.Contains(p) {
		return 0
	}
	local := Point{p.X - n.frame.Origin.X, p.Y - n.frame.Origin.Y}
	for i := len(n.children) - 1; i >= 0; i-- {
		if id := hit(n.children[i], local); id != 0 {
			return id
		}
	}
	if n.onPress != nil {
		return n.id
	}
	return 0
}

func (t *Tree) node(id NodeID) *Node {
	if int(id) <= 0 || int(id) >= len(t.nodes) {
		return nil
	}
	return t.nodes[id]
}

// Commit lays the tree out and returns what changed since the last commit,
// or nil when nothing did.
func (t *Tree) Commit() *MountBatch {
	if !t.dirty || t.root == nil {
		return nil
	}
	t.dirty = false
	t.layout(t.root, t.size.Width, t.size.Width, t.size.Height)
	t.root.frame.Origin = Point{}
	t.items = t.items[:0]
	t.emit(t.root, 0, 0)
	if len(t.items) == 0 {
		return nil
	}
	return &MountBatch{Root: t.root.id, Items: append([]MountItem(nil), t.items...)}
}

func (t *Tree) emit(n *Node, parent NodeID, index int) {
	if !n.created {
		n.created = true
		t.items = append(t.items, MountItem{Kind: ItemCreate, ID: n.id, Tag: n.tag})
	}
	if p := n.props(); p != n.sentProps || !n.inserted {
		n.sentProps = p
		t.items = append(t.items, MountItem{Kind: ItemProps, ID: n.id, Props: p})
	}
	for i, c := range n.children {
		t.emit(c, n.id, i)
	}
	if !n.inserted && parent != 0 {
		t.items = append(t.items, MountItem{Kind: ItemInsert, ID: n.id, Parent: parent, Index: index})
	}
	if n.frame != n.sentFrame || !n.inserted {
		n.sentFrame = n.frame
		t.items = append(t.items, MountItem{Kind: ItemFrame, ID: n.id, Frame: n.frame})
	}
	n.inserted = true
}

func (n *Node) props() Props {
	p := Props{
		Background:   n.style.Background,
		CornerRadius: n.style.CornerRadius,
		Pressable:    n.onPress != nil,
		Role:         n.role,
	}
	switch n.tag {
	case TagText:
		p.Text = n.text
	case TagInput:
		p.Text = n.value
		p.Placeholder = n.placeholder
	default:
		return p
	}
	p.Font = n.style.font()
	p.Color = n.style.Color
	p.Padding = n.style.Padding
	return p
}

func (t *Tree) measure(text string, font Font, maxWidth Points) Size {
	key := measureKey{text, font, maxWidth}
	if maxWidth.IsAuto() {
		key.maxWidth = -1 // NaN never equals itself
	}
	if s, ok := t.cache[key]; ok {
		return s
	}
	if len(t.cache) >= 1024 {
		clear(t.cache)
	}
	s := t.text.Measure(text, font, maxWidth)
	t.cache[key] = s
	return s
}
