package core

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
	// OnLayout receives the view's frame after each layout that moves or
	// sizes it.
	OnLayout func(Bounds)

	// Disabled stops presses and is reported to assistive technology.
	Disabled func() bool
	Selected func() bool
	Checked  func() bool
	Role     Role
	Label    string
	TestID   string
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

	OnPress  func()
	OnLayout func(Bounds)
	Role     Role
	Label    string
	TestID   string
}

// Render implements Node.
func (t Text) Render(*Scope) Node { return t }

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
	Style        Style
	StyleFunc    func() Style

	OnChange func(text string)
	OnSubmit func(text string)
	OnFocus  func()
	OnBlur   func()
	OnLayout func(Bounds)
	Label    string
	TestID   string
}

// Render implements Node.
func (i Input) Render(*Scope) Node { return i }

// Scroll scrolls its children, laid out as a column (or a row when
// Horizontal) as long as they need.
type Scroll struct {
	Horizontal    bool
	HideIndicator bool
	Style         Style
	StyleFunc     func() Style
	Children      []Node
	// OnScroll receives the offset of the content as it scrolls.
	OnScroll func(offset Point)
	OnLayout func(Bounds)
	TestID   string
}

// Render implements Node.
func (s Scroll) Render(*Scope) Node { return s }
