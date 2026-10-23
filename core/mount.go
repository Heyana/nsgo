package core

import "time"

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
	// KindImage shows an image the platform loads (ItemImage).
	KindImage
	// KindModal is a layer covering the root view or an outer modal,
	// presented when inserted and dismissed when removed (ItemModal).
	KindModal
	// KindCanvas plays a Drawing (ItemCanvas).
	KindCanvas
	// KindWeb hosts the platform's web view, filling it (ItemWeb).
	KindWeb
	// KindVideo hosts the platform's video player, filling it (ItemVideo).
	KindVideo
	// KindNative hosts a view of a component the app registered, filling
	// it (ItemNative).
	KindNative
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
	// ItemImage sets an image view's source and look (Image).
	ItemImage
	// ItemModal configures a modal (Modal).
	ItemModal
	// ItemAlert shows a native alert (Alert); the platform answers with
	// EventAlert.
	ItemAlert
	// ItemDismissKeyboard takes focus away from the focused text field.
	ItemDismissKeyboard
	// ItemScrollTo scrolls ID to Point (its content's offset, UIKit's
	// contentOffset: ContentInset makes it negative at the start),
	// animated when Animated; the platform clamps it and reports
	// EventScroll. Platforms count the ItemScrollTo and ItemScrollBy
	// items they applied to each scroll view and report the count with
	// its offsets.
	ItemScrollTo
	// ItemCanvas replaces a canvas's drawing (Drawing), in the view's
	// coordinates.
	ItemCanvas
	// ItemSetClipboard puts Value on the clipboard.
	ItemSetClipboard
	// ItemReadClipboard asks for the clipboard's text; the platform
	// answers with EventClipboard carrying Count.
	ItemReadClipboard
	// ItemAnnounce has the screen reader say Value.
	ItemAnnounce
	// ItemKeymap replaces the surface's keyboard shortcuts (Keys); the
	// platform consumes matching key presses and reports EventKey.
	ItemKeymap
	// ItemStatusBar sets the status bar (StatusBar): the layers of
	// settings merged.
	ItemStatusBar
	// ItemTitle sets the window's or page's title (Value).
	ItemTitle
	// ItemBackUnhandled says nothing took the last EventBack: Android
	// leaves the app, as its back button does; others ignore it.
	ItemBackUnhandled
	// ItemWindow asks the desktop window to change (Window); platforms
	// without one ignore it.
	ItemWindow
	// ItemMenu replaces the application menu (Menu; nil restores the
	// platform's); a chosen item is reported with EventMenu.
	ItemMenu
	// ItemWeb configures a web view (Web).
	ItemWeb
	// ItemWebCommand has a web view do WebCommand, with Value for the
	// message or script.
	ItemWebCommand
	// ItemVideo configures a video (Video).
	ItemVideo
	// ItemVideoSeek jumps a video to Time.
	ItemVideoSeek
	// ItemNative names a native view's component and sets its changed
	// props (Native).
	ItemNative
	// ItemNativeCommand has a native view's component run the command
	// Value with Data.
	ItemNativeCommand
	// ItemAnimation changes the native animation graph (Animation), after
	// the batch's views exist.
	ItemAnimation
	// ItemZoomTo zooms a zoomable scroll view (Zoom), animated when
	// Animated; the platform clamps it and reports EventScroll.
	ItemZoomTo
	// ItemImageInfo loads Image's Source, keeping its bytes for the views
	// that show it next; the platform answers with EventImageInfo carrying
	// Count.
	ItemImageInfo
	// ItemSelection selects Selection in text field ID, after the batch's
	// value; the platform clamps it to the text.
	ItemSelection
	// ItemScrollBy moves scroll view ID's offset by Point without
	// clamping it, carrying a drag or glide in flight along, so the
	// visible content stays in place as rows above it change size.
	ItemScrollBy
	// ItemStickyCells replaces the sticky cells of scroll view ID
	// (Sticky): on every scroll the platform moves each along the main
	// axis by clamp(offset - Start, 0, End - Start) from its frame.
	ItemStickyCells
	// ItemAccessibilityLabel changes the label (Value) of a view whose
	// ViewProps were sent before, and nothing else: a label following app
	// state changes alone, so the rest is not sent again.
	ItemAccessibilityLabel
	// ItemAccessibilityFocus moves the screen reader's focus to ID.
	ItemAccessibilityFocus
	// ItemScreen configures a screen's system header (Screen) on the
	// platforms that report header heights, before the screen is
	// inserted and when it changes.
	ItemScreen
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
	// Point and Animated are ItemScrollTo's.
	Point    Point
	Animated bool
	Events   EventMask

	ViewStyle *ViewStyle
	Text      *TextContent
	Input     *InputConfig
	ViewProps *ViewProps
	Scroll    *ScrollConfig
	Image     *ImageConfig
	Modal     *ModalConfig
	Alert     *AlertConfig
	Drawing   *Drawing
	// Keys are ItemKeymap's normalized bindings (see NormalizeKey).
	Keys   []string
	Window *WindowCommand
	Menu   []MenuEntry
	Web    *WebConfig
	// WebCommand is ItemWebCommand's.
	WebCommand WebCommand
	Video      *VideoConfig
	Time       time.Duration // ItemVideoSeek
	Native     *NativeConfig
	// Data is ItemNativeCommand's argument.
	Data      any
	Animation []AnimationOp
	Zoom      *ZoomRequest
	// Selection is ItemSelection's range.
	Selection TextRange
	// Sticky are ItemStickyCells' cells.
	Sticky    []StickyCell
	StatusBar *StatusBarConfig
	Screen    *ScreenConfig
}

// StickyCell is a sticky list cell and its travel, in its scroll view's
// main-axis offsets: from Start it moves with the offset, as far as End
// (which may be infinite).
type StickyCell struct {
	Cell       NodeID
	Start, End Points
}

// StickyShift is how far a sticky cell moves from its frame along the
// main axis at offset.
func (c StickyCell) StickyShift(offset Points) Points {
	return min(max(offset-c.Start, 0), max(c.End-c.Start, 0))
}

// MountBatch is one commit's changes, applied in order on the main thread.
// Root is the view the platform shows in its surface.
type MountBatch struct {
	Root  NodeID
	Items []MountItem
	// LayoutAnimation, when set, animates the batch's frame changes,
	// insertions and removals on the platform (Scope.AnimateLayout).
	LayoutAnimation *LayoutAnimation
	// Sent is when the engine handed the batch to the platform, for the
	// performance monitor's latency reading; the zero time when no monitor
	// measures.
	Sent time.Time
}

// ViewStyle is everything a platform draws for a view itself.
type ViewStyle struct {
	Background   Color
	CornerRadius Points
	// BorderWidth and BorderColor are the border on every side, unless
	// Borders is set: then each side has its width there and its color
	// in BorderColors.
	BorderWidth  Points
	BorderColor  Color
	Borders      Edges
	BorderColors EdgeColors
	Opacity      float32
	// Clip clips children to the bounds and corners.
	Clip         bool
	ShadowColor  Color
	ShadowOffset Point
	ShadowRadius Points
	// TintColor recolors an image's opaque pixels; 0 keeps its colors.
	TintColor Color
	// Cursor is the pointer's look over the view; Auto inherits the
	// nearest ancestor's, and a pressable view's is a pointer on the web.
	Cursor Cursor
	// Transform moves, scales and rotates the view around its center
	// without moving its children's frames; hit testing follows it.
	Transform Transform
	// Transition, when it has a duration, animates the change from the
	// previous ViewStyle to this one.
	Transition Transition
	// Hidden hides the view and its children (display none).
	Hidden bool
}

// ImageConfig is an image view's source and look.
type ImageConfig struct {
	Source       ImageSource
	Placeholder  ImageSource
	ResizeMode   ResizeMode
	BlurRadius   Points
	FadeDuration time.Duration
	// Position is the style's ObjectPosition: where the fitted image sits
	// (ResizeMode.Fit). It travels with the image, not the ViewStyle.
	Position ObjectPosition
}

// ModalConfig is how a modal appears and goes.
type ModalConfig struct {
	Animation ModalAnimation
	// Presentation, NoCloseOnSwipe and Orientations are iOS's. A sheet
	// reports its size with EventModalSize, its swipes with
	// EventRequestClose.
	Presentation   ModalPresentation
	NoCloseOnSwipe bool
	Orientations   Orientation
}

// ScreenConfig is a screen's system header, which the platform draws
// over the top of the screen's view, HeaderHeight tall.
type ScreenConfig struct {
	Title string
	// HeaderHidden hides the header; HeaderHeight is then 0.
	HeaderHidden      bool
	HeaderTransparent bool
	LargeTitle        bool
	// CanGoBack says a screen is below; BackHidden hides the back button
	// to it, and BackTitle is its label.
	CanGoBack  bool
	BackHidden bool
	BackTitle  string
	// NoPopOnSwipe turns the swipe back off, as NoPopOnBack does with the
	// back button's popping: a press still reports EventBack.
	NoPopOnSwipe bool
	NoPopOnBack  bool
	// Background, TitleColor and TintColor are the header's colors; 0
	// keeps the platform's.
	Background, TitleColor, TintColor Color
	// Left, Right and TitleView are the header views, each a layout root
	// at its content's size that no view holds, or 0.
	Left, Right, TitleView NodeID
	// HeaderHeight is the area the header covers from the screen's top:
	// the bar and the part of the status bar the screen reaches under.
	HeaderHeight Points
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
	// Accessory is the InputAccessoryView that rides above the keyboard
	// while the field has focus, or 0.
	Accessory NodeID
	// Multiline takes several lines in a multi-line control.
	Multiline bool
	// MaxLength caps what the user enters, in UTF-16 units; 0 sets none.
	MaxLength int
	// SubmitBehavior is as set; platforms read Submit.
	SubmitBehavior SubmitBehavior
	ReturnKey      ReturnKeyType
	AutoCapitalize AutoCapitalize
	NoAutoCorrect  bool
	AutoComplete   AutoComplete
	// KeyboardAppearance is iOS's.
	KeyboardAppearance KeyboardAppearance
	// PlaceholderColor is 0 for the platform's: the text color at half
	// strength where the platform styles placeholders itself.
	PlaceholderColor Color
	// SelectionColor colors the caret and the selection; 0 is the
	// platform's.
	SelectionColor Color
	HideCaret      bool
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
	Role  Role
	Label string
	// Hint says what activating the element does.
	Hint     string
	Disabled bool
	// Selected, Checked and Indeterminate (mixed) are reported to
	// assistive technology, and Expanded when Expandable.
	Selected      bool
	Checked       bool
	Indeterminate bool
	Expandable    bool
	Expanded      bool
	// Value is a range's or a textual element's value.
	Value AccessibilityValue
	// Accessible makes the view one element, or none.
	Accessible Accessible
	// ElementsHidden hides the view and its subtree from assistive
	// technology.
	ElementsHidden bool
	LiveRegion     LiveRegion
	// Actions are the actions offered to assistive technology, reported
	// with EventAccessibilityAction. The list is shared and never changed,
	// so ViewProps stay comparable; read it with ActionList.
	Actions *[]AccessibilityAction
	TestID  string
	// WindowDrag, PanActivation and selections are decided on the main
	// thread.
	WindowDrag    WindowDrag
	PanActivation PanActivation
	// SelectionArea is a view's, Selectable and SelectionColor a text's.
	SelectionArea  bool
	Selectable     Selectable
	SelectionColor Color
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
	RoleRadioGroup
	RoleTabList
	RoleMenu
	RoleMenuItem
	RoleDialog
	RoleAlert
	// RoleAdjustable is a slider: its Value is a range that the
	// increment and decrement actions adjust.
	RoleAdjustable
	// RoleProgressBar shows progress, its Value a range.
	RoleProgressBar
)

// ScrollConfig configures a scroll view.
type ScrollConfig struct {
	Horizontal bool
	// HideIndicator hides the scroll bar.
	HideIndicator bool
	Behavior      ScrollBehavior
	// Refreshable says the app listens for EventRefresh: pulling the
	// content down past its top refreshes. Refreshing shows the spinner;
	// a config after EventRefresh says whether the app keeps it.
	Refreshable bool
	Refreshing  bool
	// MinZoomScale and MaxZoomScale bound pinches (1 and at least
	// MinZoomScale); the content zooms when Zooms. ZoomScale is the app's
	// scale, which the platform zooms to when it changes.
	MinZoomScale, MaxZoomScale float32
	ZoomScale                  float32
	// NoBouncesZoom stops pinches at the limits; NoPinchZoom ignores
	// pinches; CenterContent centers content smaller than the viewport.
	NoBouncesZoom bool
	NoPinchZoom   bool
	CenterContent bool
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
