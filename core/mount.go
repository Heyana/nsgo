package core

// NodeID identifies a view of one tree. IDs start at 1, stay dense and
// are reused after a Delete.
type NodeID uint32

// ViewKind is the kind of native view a node mounts.
type ViewKind uint8

const (
	// KindView is a container: background, border, corners and presses.
	KindView ViewKind = iota + 1
	// KindText draws a TextLayout the engine prepared.
	KindText
	// KindInput is the platform's text field.
	KindInput
	// KindScroll scrolls its children; ItemScrollContent sizes them.
	KindScroll
)

// ItemKind is what a mount item does.
type ItemKind uint8

const (
	// ItemCreate makes a detached view of Kind for ID.
	ItemCreate ItemKind = iota + 1
	// ItemInsert inserts or moves ID into Parent, below sibling Before
	// (on top when 0). Siblings rather than indices, so views a platform
	// adds of its own cannot shift positions.
	ItemInsert
	// ItemRemove detaches ID from its parent and keeps it.
	ItemRemove
	// ItemDelete detaches and destroys ID; its ID may be created again
	// later in the same batch.
	ItemDelete
	// ItemFrame moves and sizes ID relative to its parent (Frame).
	ItemFrame
	// ItemViewStyle sets what any view draws (ViewStyle).
	ItemViewStyle
	// ItemText sets a text view's content (Text).
	ItemText
	// ItemInput configures a text field (Input).
	ItemInput
	// ItemInputValue sets a text field's value (Value) unless the user
	// edited it after the edit numbered Count.
	ItemInputValue
	// ItemEvents sets which events the view reports (Events).
	ItemEvents
	// ItemViewProps sets accessibility and identity (ViewProps).
	ItemViewProps
	// ItemScrollConfig configures a scroll view (Scroll).
	ItemScrollConfig
	// ItemScrollContent sets the size of a scroll view's content (Size).
	ItemScrollContent
	// ItemFocus moves keyboard focus to ID.
	ItemFocus
	// ItemBlur takes keyboard focus away from ID.
	ItemBlur
)

// MountItem is one change to the native views. Kind says which fields
// are set; the large groups are pointers so items stay small.
type MountItem struct {
	Kind   ItemKind
	ID     NodeID
	View   ViewKind // ItemCreate
	Parent NodeID   // ItemInsert
	Before NodeID   // ItemInsert
	Frame  Bounds   // ItemFrame
	Size   Size     // ItemScrollContent
	Value  string   // ItemInputValue
	Count  uint32   // ItemInputValue
	Events EventMask

	ViewStyle *ViewStyle
	Text      *TextContent
	Input     *InputConfig
	ViewProps *ViewProps
	Scroll    *ScrollConfig
}

// MountBatch is one commit's changes, applied in order on the main thread.
// Root is the view the platform shows in its surface.
type MountBatch struct {
	Root  NodeID
	Items []MountItem
}

// ViewStyle is everything a platform draws for a view itself.
type ViewStyle struct {
	Background   Color
	CornerRadius Points
	BorderWidth  Points
	BorderColor  Color
	Opacity      float32
	// Clip clips children to the bounds and corners.
	Clip         bool
	ShadowColor  Color
	ShadowOffset Point
	ShadowRadius Points
	// Hidden hides the view and its children (display none).
	Hidden bool
}

// TextContent is a text view's content, laid out by the engine.
type TextContent struct {
	// Text is the plain text, for accessibility.
	Text string
	// Layout is the platform's TextLayout, ready to draw. The platform
	// owns it from now on and releases the one it replaces.
	Layout TextLayout
	// Content is where Layout goes in the view: inside padding and border.
	Content Bounds
}

// InputConfig is a text field's look and behavior.
type InputConfig struct {
	Style        TextStyle
	Placeholder  string
	Masked       bool
	Readonly     bool
	Disabled     bool
	KeyboardType KeyboardType
	// Content is where the text goes in the view: inside padding and border.
	Content Bounds
}

// KeyboardType picks a software keyboard.
type KeyboardType uint8

const (
	KeyboardDefault KeyboardType = iota
	KeyboardEmail
	KeyboardNumber
	KeyboardDecimal
	KeyboardPhone
	KeyboardURL
)

// ViewProps is a view's accessibility and identity.
type ViewProps struct {
	Role     Role
	Label    string
	Disabled bool
	// Selected and Checked are reported to assistive technology.
	Selected bool
	Checked  bool
	TestID   string
}

// Role is a view's accessibility role.
type Role uint8

const (
	RoleNone Role = iota
	RoleButton
	RoleLink
	RoleHeader
	RoleImage
	RoleCheckbox
	RoleSwitch
	RoleRadio
	RoleTab
	RoleText
	RoleTextField
)

// ScrollConfig configures a scroll view.
type ScrollConfig struct {
	Horizontal bool
	// HideIndicator hides the scroll bar.
	HideIndicator bool
}

// TextEngine prepares text for layout. The engine calls it on its own
// goroutine, so implementations must not touch main-thread-only objects.
type TextEngine interface {
	// Prepare shapes text once; the result measures it at any number of
	// widths and lays it out at its final size.
	Prepare(text *AttributedText) PreparedText
}

// PreparedText is text a TextEngine shaped.
type PreparedText interface {
	// Measure returns the size of the text wrapped at maxWidth (Auto: no
	// wrapping).
	Measure(maxWidth Points) Size
	// Layout returns the text laid out in size, for the main thread to
	// draw.
	Layout(size Size) TextLayout
	// Release frees what the prepared text holds.
	Release()
}

// TextLayout is a platform's laid-out text, drawn on the main thread and
// released there when replaced or deleted.
type TextLayout interface {
	Release()
}
