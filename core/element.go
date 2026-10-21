package core

// Tag is the kind of native view a node mounts.
type Tag uint8

const (
	// TagView is a container: background, corners and presses.
	TagView Tag = iota + 1
	// TagText is a run of text the engine lays out and the platform draws.
	TagText
	// TagInput is the platform's single-line text field.
	TagInput
)

// Role is a node's accessibility role.
type Role uint8

const (
	RoleNone Role = iota
	RoleButton
)

// Element describes a node before it mounts. Components run once and
// return elements; reactive props bind to signals when they mount.
type Element struct {
	tag         Tag
	style       Style
	text        func() string
	value       func() string
	placeholder string
	role        Role
	onPress     func()
	onChange    func(string)
	onSubmit    func(string)
	children    []*Element
}

// View returns a container of children.
func View(children ...*Element) *Element {
	return &Element{tag: TagView, children: children}
}

// Text returns a run of static text.
func Text(content string) *Element {
	return &Element{tag: TagText, text: func() string { return content }}
}

// TextFn returns text that fn computes; it updates when the signals fn
// reads change.
func TextFn(fn func() string) *Element {
	return &Element{tag: TagText, text: fn}
}

// Input returns a single-line text field. Its value is the app's: pass
// Value and update it in OnChange.
func Input() *Element {
	return &Element{tag: TagInput}
}

// Style sets the element's style.
func (e *Element) Style(s Style) *Element {
	e.style = s
	return e
}

// Child appends children.
func (e *Element) Child(children ...*Element) *Element {
	e.children = append(e.children, children...)
	return e
}

// OnPress calls fn when the element is pressed (clicked or tapped).
func (e *Element) OnPress(fn func()) *Element {
	e.onPress = fn
	return e
}

// Role sets the accessibility role.
func (e *Element) Role(r Role) *Element {
	e.role = r
	return e
}

// Value binds an input's text to a func() string, usually a signal's Get.
func (e *Element) Value(fn func() string) *Element {
	e.value = fn
	return e
}

// Placeholder sets an input's text shown while it is empty.
func (e *Element) Placeholder(s string) *Element {
	e.placeholder = s
	return e
}

// OnChange calls fn with an input's text after each edit.
func (e *Element) OnChange(fn func(string)) *Element {
	e.onChange = fn
	return e
}

// OnSubmit calls fn with an input's text when the user presses Return.
func (e *Element) OnSubmit(fn func(string)) *Element {
	e.onSubmit = fn
	return e
}
