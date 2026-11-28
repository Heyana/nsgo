package core

import (
	"cmp"
	"errors"
	"reflect"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/layout"
)

// The shadow tree has one node per native view and one per region. A
// region is a span of its parent's children that a Show, Each, Dynamic
// or Catch replaces; it mounts nothing natively and lays its children out
// as its parent's.
//
// The Go element layer builds the tree through a handful of operations:
// create a node, set its parts, place it among its siblings, discard it.

// Tree is the shadow tree of one surface. Commit turns its changes into a
// MountBatch. It is not safe for concurrent use: the engine owns it.
type Tree struct {
	sensors        map[uint32]*SensorStream
	discovery      map[uint32]discoveryOperation
	systemGestures []*SystemGestureOptions

	root *Scope
	// env is a window's own environment (Scope.Env); the first tree's is
	// the graph's. reported is the platform's last report of it.
	env      *Env
	reported Environment
	// id numbers an opened surface's tree, 0 the first's; window is its
	// handle, closed set once it is gone.
	id              uint32
	window          *Window
	closed          bool
	animationFrames frameQueue
	// menuOrder is the sequence in which this tree last sent an application menu, including a nil menu that restores the default.
	menuOrder    uint64
	rt           *runtime
	text         driver.TextEngine
	nodes        []*node // index is NodeID; 0 is unused
	free         []NodeID
	view         *node // the surface's root view
	touches      touchState
	size         Size
	changed      bool
	deleted      []NodeID // retired nodes kept for command cancellation, created or not
	commands     []driver.MountItem
	hapticActive bool
	moves        []move
	items        []driver.MountItem
	laidOut      []*node
	transits     []*node // nodes whose next frame their layout transition moves
	ending       []*node // nodes with OnTransitionEnd restyled since the last commit
	reports      []report
	stacks       []*node

	// remounting keeps logical focus while the next commit recreates views.
	remounting bool

	pointerLock     *pointerLock
	nextPointerLock uint32

	// fit is the largest size of a panel that takes its content's (PanelOptions.FitContent), zero for a surface of the platform's size; reportedSize is the size its platform last reported, which a fitted size differing from is sent (driver.WindowFitSize), and refit says the environment has yet to hear t.size.
	fit, reportedSize Size
	refit             bool

	// textColors shares default ink bindings among each owner's views.
	textColors map[*Scope]*textColorBinding
	// matchGone and matchNew are the views with a MatchID the round
	// deletes and creates, which emitMatches pairs.
	matchGone, matchNew []*node
	// pictured are the nodes whose shaders read pictures; shaderStarts
	// the transitions starting in the next batch, which come first in it;
	// navTops each stack's top screen as the last commit showed it.
	pictured     []*node
	shaderStarts []NodeID
	navTops      map[*node]*node
	lists        []*node
	pips         []*node
	accessories  []*node
	// labels registers elements declaring label associations; labelled also holds controls named by those labels in the previous round.
	labels, labelled []*node
	// tooltips registers anchors; described retains their previous description targets.
	tooltips, described []*node
	headerViews         []*node
	inputBars           []*node
	status              statusBars
	pageHead            pageHeads
	orientations        orientationRequests
	anim                animations
	layoutAnim          *LayoutAnimation
	fontScale           float32
	// ratio is the device pixels per point layout rounds to.
	ratio      float32
	alerts     map[uint32]alertWait
	waits      map[uint32]wait
	auths      map[uint32]*authSession // the auth sessions waiting for the platform
	imageInfos map[uint32]imageInfoWait
	// dismissing holds deleted modals awaiting EventModalDismissed or its fallback (awaitDismissal); their IDs stay out of free until then, so a report never reaches a node that reused one.
	dismissing map[NodeID]dismissingModal
	// lateDismissals are the deleted modals dismissFallback released before the platform reported them gone, whose report Dispatch drops if it still comes.
	lateDismissals map[NodeID]bool
	keys           []*keyBinding
	keyID          uint32
	keysChanged    bool
	// held counts the keys down on the surface by name, from EventKeyDown and EventKeyUp, so both Shifts read as one "shift" until both are up; heldRead is what KeysDown reads, made by the first read, which asks the platform for every key (heldKeys).
	held, heldCodes    map[string]int
	heldRead, codeRead *Signal[[]string]
	keyCodeNames       *Signal[map[string]string]
	heldKeys           bool
	// heldButtons is a bitmask of the mouse buttons down on the surface (1<<b), from EventMouseDown and EventMouseUp; heldMouseRead is what MouseButtonsDown reads, made by the first read, which asks the platform for every mouse button (heldMouse).
	heldButtons   uint8
	heldMouseRead *Signal[[]MouseButton]
	heldMouse     bool
	backs         []*backHandler
	// regions are the live regions, which the views do not number: an owner may have no view but a region, as a Holder's content may (keyPlaces).
	regions map[*node]struct{}
	// peeking is the screen a back gesture may uncover (Tree.peek).
	peeking *node
	// history is the depth ItemHistory last sent, which historyAgain
	// sends again.
	history      int
	historyAgain bool
	alertID      uint32
	menuActions  map[uint32]menuAction
	menu         []driver.MenuEntry // the application menu set, nil for the platform's default (menuHasKey)
	popup        popupMenu
	menuID       uint32
	menuGen      uint32
	rtl          bool // the layout direction the nodes were mapped for
	// directionSent records that the platform has the direction and
	// language, the language it was told last and whether the app forced it.
	directionSent bool
	language      string
	forced        bool
	// sticky are the views whose position is Sticky; stuckIn the scroll
	// views that pinned some at the last commit.
	sticky  map[*node]bool
	stuckIn []*node
	// full has the next commit resolve and compare every node; fullRound
	// the round running.
	full, fullRound bool
	// boundaries are the relayout boundaries a change inside marked.
	boundaries []*node
	canvases   []*node
	offscreens map[uint64]*OffscreenImage
	measures   measureCache
	// fonts counts the fonts registered as the app runs: text prepared
	// with fewer is prepared again.
	fonts uint32
	// fontsRead invalidates reactive Scope.MeasureText users when fonts change.
	fontsRead *Signal[uint32]
	// followers are the views that follow another's frame, and followMoved
	// says what they follow changed.
	followers   []*node
	followMoved bool
	// framesMoved says a frame, a scroll, a deletion, a restyle layout reads or a Visibility changed the boxes the layout audit draws since a round last looked; auditRead, bumpAudit and tickAudit are what keep them up to date, nil until DevTools' overlay mounts (keepAudit).
	framesMoved bool
	auditRead   func() bool
	bumpAudit   func()
	tickAudit   func()
	// devDrawn tracks drawn geometry (transforms, animations, transitions, sticky shifts) while DevTools or their overlay read bounds; nil in release builds.
	devDrawn          *devDrawn
	syncDevDrawnFn    func(time.Time)
	devNative         *devGeometryState
	devSource         func(func(driver.GeometrySnapshot))
	webDispatch       func(*Tree, *node, driver.Event)
	updateListsFn     func() bool
	sizeHeadersFn     func() bool
	emitHeaderViewsFn func()
	// edgeBars are the views that are bars over a scroll view's side, and
	// edgeScrolls the scroll views they last reached into.
	edgeBars, edgeScrolls []*node
	// commits counts commits, the last of which started at commitStart;
	// paceStop cancels the frame paced lists wait for.
	commits     int
	commitStart time.Time
	paceStop    func()
	// views and waiting pace views over frames (pace.go): waiting are the
	// views not made yet whose parents are.
	views   viewPace
	waiting []*node
	// classParents coalesces child-order and spacing changes per parent until resolve, so invalidating child order never walks siblings. Its bool keeps spacing that a class replacement removed.
	classParents map[*node]bool
	// peerStyled enables sibling-state tracking; peerParents batches changes per parent.
	peerStyled      bool
	peerParents     map[*node]struct{}
	refreshingPeers bool
	// Relational classes coalesce updates within their source's scope.
	relationSources     map[*node]*node
	relationRoots       map[*node]struct{}
	relationScopes      map[*node][]*node // relationSources by scope, nil until needed
	refreshingRelations bool
	// bars draws the bars of scroll views that have none of their own,
	// and watches hear scroll views move (scrollbars.go).
	bars    ScrollbarMaker
	watches []*scrollWatch
	// focus is the node with the keyboard focus, whatever tracks it, and
	// focusChain it and the nodes around it, which hold stateFocusWithin
	// (updateFocusWithin). focusMoved asks the next commit to walk them.
	focus      *node
	focusChain []*node
	focusMoved bool
	// modality is what the user last gave the surface input with
	// (driver.EventInputModality), 0 before any. focusVisible is whether
	// the focus shows (focusShows), decided as it moves and again as a key
	// is typed; focusVisibleOn is the node holding stateFocusVisible, and
	// focusParts the nodes whose part shows the focus (PartStates.Focused),
	// restyled as it starts or stops showing (updateFocusVisible).
	modality       uint32
	focusVisible   bool
	focusVisibleOn *node
	focusParts     []*node
	// focusByKeyboard is whether the keyboard moved the focus where it
	// is (keyboardMovesFocus), decided as it moves and kept whatever is
	// typed meanwhile; focusHint is the node whose tooltip that focus
	// shows (updateFocusHint).
	focusByKeyboard bool
	focusHint       *node
	// activated is the owner of the view the user's input last landed on, a press, a touch or a menu chosen, until the commit after it, which lets a confined owner move the focus (mayMoveFocus).
	activated *Scope
	// frames are the views other than the root and modals that host a confined owner's overlays (Scope.Confine), which layout places after the root's.
	frames []*node
	// reveals are the Reveals asked for, done at the commit's layout.
	reveals []reveal
	// pointerStyled is set once a style has named PointerEvents, as apps
	// without one skip their walk; pointersMoved asks the next commit
	// for it (updatePointerEvents).
	pointerStyled, pointersMoved bool
	// replacing holds, by element and component type, the fields an
	// XFunc beside them replaces (twins.go).
	replacing map[reflect.Type][]twin
	// lscratch and sscratch are where restyle builds layout and resolved styles
	// without escaping locals to the heap on every restyle.
	lscratch layout.Style
	sscratch Style
	// lastItems is how many items the last commit sent, lastShared
	// whether its batch was these items (CommitShared), and sharedBatch
	// the MountBatch header reused by CommitShared.
	lastItems   int
	lastShared  bool
	sharedBatch driver.MountBatch
	// highlights, while an UpdateHighlights is mounted, outlines what each commit changed.
	highlights *highlights
	// devSurface is this tree's devtools.Surface, shared between devtools.Register and devtools.MountOverlay.
	devSurface *devSurface
	// record, while DevTools record a test, hears what the user does; devScrolls holds each scroll view's offset until the scroll is a step (devFlushScrolls).
	record     func(devtools.Step)
	devScrolls map[*node]Point
	// follower, while nsgo dev -automation's agent follows the tree (devtools.Surface.Follow), hears each batch it commits and each event it dispatches.
	follower devtools.Follower
	// suspended holds the regions of the Contents of the Suspenses that wait, whose views hide.
	suspended map[*node]bool
	// gpuSubmit forwards off-commit GPUFrames directly to the platform backend.
	gpuSubmit func(driver.GPUFrame)
}

// report is a handler to run under owner after the commit.
type report struct {
	owner *node
	fn    func()
}

// after runs fn under n's owner once the commit's items are out.
func (t *Tree) after(n *node, fn func()) { t.reports = append(t.reports, report{n, fn}) }

// nodeFlags holds copy-on-write ownership bits for shared node sub-structs.
type nodeFlags uint8

const (
	flagBaseOwned nodeFlags = 1 << iota
	flagStyleOwned
	flagPropsOwned
	flagSentVSOwned
)

// nodeExtra holds rare per-node element payloads and feature state, allocated
// lazily only when a node sets one of these fields; plain views and texts share
// the read-only zeroExtra sentinel.
type nodeExtra struct {
	// contentBox is allocated only when a Ref observes its layout content box.
	contentBox *Signal[Bounds]
	// pointerLocked is allocated only when a Ref observes its pointer lock.
	pointerLocked *Signal[bool]

	responder *Responder // a view's part in the touch responder
	follow    *following // what its frame follows (View.Follow)
	match     string     // View.MatchID
	// tip is the tooltip's: what shows it as the pointer comes onto the
	// view or the keyboard's focus into it, and hides it.
	tip *tooltipHooks
	// tooltipOwner marks the synthetic root containing an anchor's hint.
	tooltipOwner       *node
	tooltipDescription string

	labels *labelRelations
	// bound are the animated values its gestures and scrolling set, as
	// an element last bound them (rebind).
	bound        *boundValues
	env          *envValues // what its padding's and margin's terms come to
	input        *inputNode
	scroll       *scrollNode
	image        *imageNode
	cursorPixels *[2]pixelFrame
	hoist        *hoistNode // an overlay's or a modal's
	hosted       []*node    // the overlays and modals this root view, modal or frame hosts
	attach       *attachNode
	screen       *screenNode
	list         *listNode
	canvas       *canvasNode
	gpu          *gpuNode
	shader       *shaderNode
	fx           *fxNode // an effect's, a transition's or a navigation's shader
	icon         *iconNode
	web          *webNode
	video        *videoNode
	native       *nativeNode
	// accessory is an InputAccessoryView's: a layout root as wide as the
	// surface that no view holds.
	accessory *accessoryNode
	pip       *pictureInPictureNode
	anim      *viewAnimation // the animated values bound to the view
	part      PartStates     // the states it is styled in as a control's part
	// edgeBar is the scroll view side the view is a bar over.
	edgeBar *EdgeBar
	// place is the holder a place region shows; held the holder whose
	// content this region is.
	place, held *Holder
	// elem is the Element the node is, which its own owner releases.
	elem *Element
	// selection holds rich Markdown/HTML selection semantics when set (e.g. by base/textview).
	selection *selectionNode
	endStop   func() bool // stops the OnTransitionEnd waiting for its transition
	fill      *Size       // the size a root fills, if it fills one

	elevation  Points // Android.Elevation
	material   driver.BackdropMaterial
	appearance Appearance // what its subtree forces (Scope.SetSubtreeAppearance)
}

type node struct {
	parent   *node
	owner    *Scope
	children []*node
	flat     []*node // native children, through regions

	// base, style, props, h, and nodeExtra are always non-nil, pointing at
	// read-only sentinels or shared interned values until a node needs its own.
	base   *Style
	dyn    *Style // the StyleFunc's, kept once and updated in place
	style  *Style // base and dyn merged, with the states' styles
	props  *driver.ViewProps
	h      *handlers
	ink    *textColorBinding
	cls    *nodeClasses // its Tailwind classes
	passed *textFields  // interned text fields it passed its children last
	lstyle *layout.Style
	text   *textNode
	ref    *Ref
	*nodeExtra

	// synced is the count of its children's roundings syncFrames last
	// took their frames after (layout.State.Rounds).
	synced uint64
	sent   sent
	lstate layout.State
	frame  Bounds

	id    NodeID // 0 for regions
	color Color  // text color the node has, for CurrentColor

	kind       driver.ViewKind // 0 for regions
	states     uint8
	ownPointer PointerEvents
	pace       uint8 // paceWaiting or paceNow, until its view is made
	flags      nodeFlags

	showsExpanded              bool
	pointerNone, pointerTarget bool
	envBound                   bool
	confines                   bool
	transformed                bool
	hidden                     bool
	headerView                 bool
	inputBar                   bool
	panning                    bool // it follows pans: OnPan or PanValues
	pinching                   bool // it follows pinches: OnPinch or PinchValue
	rotating                   bool // it follows rotations: OnRotation or RotationValue
	isStack                    bool
	layoutDirty                bool
	transit                    bool // in Tree.transits
	restyled                   bool // in Tree.ending
	queued                     bool // a boundary waiting in Tree.boundaries
	rtl                        bool // laid out right to left
	isRoot                     bool // a layout root: the root view, an overlay or a modal
	created                    bool
	deleted                    bool
	childrenDirty              bool
	dirty, dirtyKids           bool
	unresolved, resolveKids    bool
}

func (n *node) ownExtra() *nodeExtra {
	if n.nodeExtra == &zeroExtra {
		n.nodeExtra = new(nodeExtra)
	}
	return n.nodeExtra
}

func (n *node) hasFlag(f nodeFlags) bool { return n.flags&f != 0 }

func (n *node) setFlag(f nodeFlags, on bool) {
	if on {
		n.flags |= f
	} else {
		n.flags &^= f
	}
}

func (n *node) setBase(t *Tree, s *Style) {
	n.flags &^= flagBaseOwned
	n.base = t.internStyle(s)
}

func (n *node) ownBase() *Style {
	if n.flags&flagBaseOwned == 0 {
		cp := *n.base
		n.base = &cp
		n.flags |= flagBaseOwned
	}
	return n.base
}

func (n *node) ownProps() *driver.ViewProps {
	if n.flags&flagPropsOwned == 0 {
		cp := *n.props
		n.props = &cp
		n.flags |= flagPropsOwned
	}
	return n.props
}

func (n *node) setProps(p driver.ViewProps) {
	if p == zeroViewProps {
		n.props = &zeroViewProps
		n.flags &^= flagPropsOwned
		return
	}
	if p == defaultTextProps {
		n.props = &defaultTextProps
		n.flags &^= flagPropsOwned
		return
	}
	*n.ownProps() = p
}

func (n *node) ownHandlers() *handlers {
	if n.h == &noHandlers {
		n.h = new(handlers)
	}
	return n.h
}

func (n *node) setHandlers(h handlers) {
	if h.empty() {
		n.h = &noHandlers
		return
	}
	if n.h == &noHandlers {
		n.h = new(handlers)
	}
	*n.h = h
}

func (n *node) hasParentSpacing() bool {
	p := n.nativeParent()
	return p != nil && p.cls != nil && (p.cls.props.hasSpaceX || p.cls.props.hasSpaceY)
}

type handlers struct {
	press, longPress, pressDown, pressUp func(PressEvent)
	focusIn, focusOut                    func()
	mouseEnter, mouseLeave, mouseMove    func(MouseEvent)
	mouseDown, mouseUp                   func(MouseButtonEvent)
	pointerDelta                         func(dx, dy float64)
	pointerUnlock                        func()
	textSelection                        func(TextSelection)
	focus, blur                          func()
	keyDown                              func(KeyEvent) bool
	keyUp                                func(KeyEvent)
	touch, touchCapture                  func(TouchEvent)
	contextMenu                          func(ContextMenuEvent)
	change, submit                       func(string)
	scroll, pressOutside                 func(Point)
	layout                               func(Bounds)
	cancel                               func()
	loadStart, loadEnd                   func()
	progress                             func(int64, int64)
	load                                 func(Size)
	loadError                            func(error)
	pan                                  func(PanEvent)
	pinch, rotation                      func(GestureEvent)
	wheel                                func(WheelEvent)
	fileEnter, fileLeave                 func()
	fileDrop                             func([]string)
	action                               func(string)
	canvasAction                         func(CanvasActionEvent)
	transitionEnd                        func()
}

func (h *handlers) empty() bool {
	return h.press == nil && h.longPress == nil && h.pressDown == nil && h.pressUp == nil &&
		h.focusIn == nil && h.focusOut == nil &&
		h.mouseEnter == nil && h.mouseLeave == nil && h.mouseMove == nil &&
		h.mouseDown == nil && h.mouseUp == nil &&
		h.pointerDelta == nil && h.pointerUnlock == nil &&
		h.textSelection == nil &&
		h.focus == nil && h.blur == nil &&
		h.keyDown == nil && h.keyUp == nil &&
		h.touch == nil && h.touchCapture == nil &&
		h.contextMenu == nil &&
		h.change == nil && h.submit == nil &&
		h.scroll == nil && h.pressOutside == nil &&
		h.layout == nil && h.cancel == nil &&
		h.loadStart == nil && h.loadEnd == nil &&
		h.progress == nil && h.load == nil && h.loadError == nil &&
		h.pan == nil && h.pinch == nil && h.rotation == nil &&
		h.wheel == nil &&
		h.fileEnter == nil && h.fileLeave == nil && h.fileDrop == nil &&
		h.action == nil && h.canvasAction == nil && h.transitionEnd == nil
}

type textNode struct {
	// fonts is the tree's fonts it was prepared with.
	fonts     uint32
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
	text     driver.AttributedText // what was prepared
	style    driver.TextStyle
	prepared driver.PreparedText
	// layout is the layout sent last, which Ref's text queries read.
	layout driver.TextLayout
	// fitted is the text shrunk to fit its box, when fit shrank it.
	fitted   driver.PreparedText
	stale    bool // content or style changed since prepared
	box      Bounds
	sentBox  Bounds
	shownBox Bounds // where the layout went: box, moved by its vertical alignment
	resent   bool   // a new prepared text needs a new layout
	// An attachment reads the line's baseline before the text is sent. Keep that layout for emission, and its baseline for later commits.
	pending  *driver.TextContent
	baseline *textBaseline
	// attachments counts the spans that are attachments.
	attachments int
	// Fitted attachment layouts keep the natural layout intact while drawing at the fitted font.
	fitTrees []*fittedAttachmentNode
	fitText  driver.AttributedText
	fitStale bool
	// selectable is the text's own Selectable, over the UserSelect it
	// inherits.
	selectable Selectable
	// noWrap keeps the lines unwrapped (WhiteSpaceNoWrap).
	noWrap bool
	// engine measures the words of minContent, whose width minWidth
	// caches while it is not negative.
	engine   driver.TextEngine
	minWidth Points
	// ratio is the device pixels per point its sizes round up to.
	ratio float32
	// hash is text's layout hash, which measures keys its sizes by.
	hash     uint64
	measures *measureCache
}

// size returns what the prepared text measures wrapped at width.
func (tn *textNode) size(width Points) Size {
	if tn.measures == nil {
		return tn.prepared.Measure(width)
	}
	return tn.measures.size(tn.hash, tn.prepared, width)
}

// minContent returns the text's min-content width: its widest word,
// measured alone.
func (tn *textNode) minContent() Points {
	if tn.minWidth >= 0 {
		return tn.minWidth
	}
	tn.minWidth = 0
	if tn.engine == nil {
		return 0
	}
	seen := map[string]bool{}
	for _, word := range breakWords(tn.text.Text) {
		if seen[word] {
			continue
		}
		seen[word] = true
		var w Points
		if tn.measures != nil {
			w = tn.measures.word(tn.engine, word, tn.text.Style)
		} else {
			p := tn.engine.Prepare(&driver.AttributedText{Text: word, Style: tn.text.Style})
			w = p.Measure(Auto).Width
			p.Release()
		}
		tn.minWidth = max(tn.minWidth, ceil(w, tn.ratio))
	}
	return tn.minWidth
}

// breakWords splits s where lines may break: after spaces and hyphens, and
// around each Han, Kana and Hangul character, as most of Unicode's line
// breaking rules allow. Trailing spaces hang, so words leave them out.
func breakWords(s string) []string {
	var words []string
	start := 0
	cut := func(end int) {
		if w := strings.TrimRight(s[start:end], " \t\n"); w != "" {
			words = append(words, w)
		}
		start = end
	}
	for i, r := range s {
		size := utf8.RuneLen(r)
		switch {
		case wide(r):
			cut(i)
			cut(i + size)
		case r == ' ' || r == '\t' || r == '\n' || r == '-' || r == '\u2014':
			cut(i + size)
		}
	}
	cut(len(s))
	return words
}

// wide reports whether lines may break on either side of r: a Han, Kana
// or Hangul character.
func wide(r rune) bool {
	return unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
		unicode.Is(unicode.Hangul, r)
}

type textBaseline struct {
	y  Points
	ok bool
}

// dropTextLayout releases a layout the platform has yet to take.
func (tn *textNode) dropTextLayout() {
	if tn.pending != nil {
		tn.pending.Layout.Release()
		tn.pending = nil
	}
	tn.baseline = nil
}

// release frees the text's prepared texts and any unsent layout.
func (tn *textNode) release() {
	tn.dropTextLayout()
	for _, p := range []*driver.PreparedText{&tn.prepared, &tn.fitted} {
		if *p != nil {
			(*p).Release()
			*p = nil
		}
	}
}

// fontMultiplier is what the user's text size setting, scale, multiplies
// the text's sizes by.
func (tn *textNode) fontMultiplier(scale float32) float32 {
	return fontMultiplier(scale, tn.noScale, tn.maxScale)
}

// fontMultiplier is what scale, the user's text size setting, multiplies
// a text's sizes by when noScale keeps them or maxScale caps it.
func fontMultiplier(scale float32, noScale bool, maxScale float32) float32 {
	if noScale {
		return 1
	}
	if maxScale >= 1 {
		return min(scale, maxScale)
	}
	return scale
}

type spanNode struct {
	content  string
	copyItem *driver.ClipboardItem
	style    Style
	press    func(PressEvent)
	long     func(PressEvent)
	attach   *node        // the view of an attachment
	cls      *spanClasses // its classes, when it has any
	// made is, for an attachment a SpansFunc mounted, the span its view was made of, which the spans that follow keep the view for when they hold the same; scope owns that view.
	made  *Span
	scope *Scope
}

// attachNode is the view of a text's attachment: a layout root of its own at its content size, which the text places in its lines.
type attachNode struct {
	// size, align and descent are what the text was prepared with.
	size    Size
	align   VerticalAlign
	descent Points
	cut     bool // cut off by the text's line limit
	// fitSize is the selected visual size when fitScale is positive.
	fitSize  Size
	fitScale float32
}

type inputNode struct {
	sent      driver.InputConfig // the config last sent
	config    driver.InputConfig
	valueFn   func() string
	value     string // the app's
	shown     string // what the field shows, as far as the engine knows
	count     uint32 // the platform's last edit
	autoFocus bool
	accessory string // the InputAccessoryView's ID
	suggest   *Suggest
	// themeSelection and themeCaret are its theme's colors, which the
	// input shows without a SelectionColor of its style's.
	themeSelection, themeCaret Color
	lineH                      Points
	// ratio is the device pixels per point its sizes round up to.
	ratio   float32
	lineFor driver.TextStyle
	// noScale and maxScale limit the user's text size setting.
	noScale  bool
	maxScale float32

	selection   func(TextRange)
	keyPress    func(string)
	composition func(Composition)
	contentSize func(Size)
	caret       func(CaretEvent)
	paste       func(PasteEvent)
	editAction  func(EditAction)
	// measured is what a multiline field is measured with: its value, or
	// its placeholder while empty; sized the value's text, for
	// OnContentSizeChange.
	measured, sized preparedInput
	measuredDirty   bool // measured is deferred while layout is independent of the value
	reported        Size // the content size last reported
	reportedOK      bool
	// Multiline scrolling and app-drawn scroll bar (scrollbars.go).
	ownBar         ScrollbarMaker
	hideIndicator  bool
	barHost        *node
	bar            *Signal[ScrollbarMaker]
	scroll         scrollTracker
	platformScroll Size // non-zero when platform reported its exact scrollable content size
	onScroll       func(ScrollEvent)
}

// scrollTracker tracks a scrollable view's offset and unapplied scroll commands.
type scrollTracker struct {
	// offset is the reported offset with the scrolls the platform had not applied yet replayed over it.
	offset Point
	// issued counts the ItemScrollTo and ItemScrollBy items sent, and pending holds those a report has not shown applied, oldest first.
	issued  uint32
	applied uint32
	pending []pendingScroll
}

// issue records a scroll sent to the platform.
func (st *scrollTracker) issue(to, animated bool, p Point) {
	st.issued++
	st.pending = append(st.pending, pendingScroll{n: st.issued, to: to, animated: animated, p: p, before: st.offset})
	if to {
		st.offset = p
	} else {
		st.offset.X += p.X
		st.offset.Y += p.Y
	}
}

// report records an offset the platform reported after applying applied scrolls: those it had not applied yet still count, so a report that predates them does not move the content back.
func (st *scrollTracker) report(p Point, applied uint32) {
	st.applied = max(st.applied, applied)
	k := 0
	for k < len(st.pending) && st.pending[k].n <= st.applied {
		k++
	}
	st.pending = st.pending[k:]
	for i := range st.pending {
		s := &st.pending[i]
		s.before = p
		if s.to {
			p = s.p
		} else {
			p.X += s.p.X
			p.Y += s.p.Y
		}
	}
	st.offset = p
}

// settled reports whether every scroll sent so far has been applied.
func (st *scrollTracker) settled() bool { return len(st.pending) == 0 }

// preparedInput is text of a field prepared in its style.
type preparedInput struct {
	text     driver.AttributedText
	prepared driver.PreparedText
}

type scrollNode struct {
	// ownBar is what draws its bar over the app's; hideIndicator is what
	// the app set, config's also while a bar stands in for the indicator.
	// barHost holds the bar (scrollbars.go).
	ownBar        ScrollbarMaker
	hideIndicator bool
	barHost       *node
	bar           *Signal[ScrollbarMaker]
	barSize       *Signal[Size]
	// sent and sentContent are the config and content size last sent.
	sent        driver.ScrollConfig
	sentContent Size
	// sentFitted is the content size the platform has (fittedContent), and
	// contentEnd how far down the content reaches, even short of the view.
	sentFitted Size
	contentEnd Points
	config     driver.ScrollConfig
	content    Size
	scrollTracker
	// start is the ContentOffset it was sent before it first showed.
	start  Point
	zoom   float32 // the last reported
	on     scrollSpec
	resend bool // EventRefresh came: send the config again
	// by is the ItemScrollBy the lists ask for in this commit, sent with
	// the content size; to the ItemScrollTo their parent-viewport lists
	// ask for.
	by Point
	to *driver.MountItem
	// dependents are the lists in this view's viewport; sticky the cells
	// sent in its last ItemStickyCells.
	dependents []*listNode
	sticky     []stickyRange
	// reported is when OnScroll last ran, latest the offset it runs with
	// next, and waiting whether a timer waits for the throttle to pass.
	reported time.Time
	latest   Point
	waiting  bool
	// effects are the edge effects it asks for, and edges what its bars
	// measured (markEdgeScrolls).
	effects EdgeEffects
	edges   [4]driver.ScrollEdge
}

// pendingScroll is a scroll the core sent and the platform has not shown
// applied.
type pendingScroll struct {
	n        uint32
	to       bool // an ItemScrollTo; else an ItemScrollBy
	animated bool
	p        Point
	before   Point // predicted offset before this command, for cancellation
}

// settled reports whether every scroll the core asked for was applied.
func (sc *scrollNode) settled() bool {
	return sc.scrollTracker.settled() && sc.by == (Point{}) && sc.to == nil
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
	parent    *node
	viewStyle *driver.ViewStyle // always non-nil (&defaultViewStyle, interned, or owned)
	props     *driver.ViewProps // always non-nil (&zeroViewProps, &defaultTextProps, or immutable snapshot)
	children  []*node
	events    driver.EventMask
	frame     Bounds
	overflow  Bounds // the area ItemOverflow last gave
	// transformed is the flag ItemOverflow last gave.
	transformed bool
	// removed is set from removal until the view is inserted again.
	removed bool
}

// NewTree returns an empty tree under root, laying text out with text.
func NewTree(root *Scope, text driver.TextEngine) *Tree {
	root.rt.tree = nil
	t := newTree(root, text, 0)
	root.rt.tree = t
	return t
}

// newTree returns an empty tree under root, numbered id, which becomes the graph's first unless one is.
func newTree(root *Scope, text driver.TextEngine, id uint32) *Tree {
	t := &Tree{root: root, rt: root.rt, id: id, text: text, nodes: []*node{nil}, measures: newMeasureCache(), ratio: 1}
	t.anim.t = t
	if root.rt.tree != nil {
		root.surf = t
	}
	t.view = t.newNode(driver.KindView, nil, root)
	t.view.base = t.internStyle(&Style{Direction: Column})
	t.view.style = t.view.base
	// The app-wide bindings (KeyOptions.AllSurfaces) come before what it shows binds.
	for _, g := range root.rt.globalKeys {
		g.bind(t, false)
	}
	root.addCleanup(func() {
		for _, g := range root.rt.globalKeys {
			delete(g.bindings, t)
		}
	})
	// Prepared texts belong to this tree even when its platform will receive no final commit, as when an embedded engine stops.
	root.addCleanup(func() { t.discard(t.view) })
	if root.rt.dev != nil {
		// An app that opens windows has a first tree of its own that shows no view (RootOptions.Windows): DevTools pages leave it out, while the automation's agent follows it for the app's alerts and main menu.
		t.devSurface = &devSurface{t}
		devtools.Register(t.devSurface, t.listed())
		root.addCleanup(func() { devtools.Unregister(t.devSurface) })
	}
	return t
}

// listed reports whether t shows a view of its own, as DevTools list surfaces: every tree except the hidden app tree of an app that opens windows.
func (t *Tree) listed() bool { return t.rt.windows == nil || t.id != 0 }

// Mount builds what app returns as the root view's content.
func (t *Tree) Mount(app func(cx *Scope) Node) {
	if t.id == 0 {
		t.rt.rootView = app
	}
	t.within(t.root, func() {
		if fn := devtools.MountOverlay; fn != nil && t.root.rt.post != nil && t.listed() && t.highlights == nil {
			if t.devSurface == nil {
				t.devSurface = &devSurface{t}
			}
			if raw, mounted, cleanup := fn(t.devSurface); raw != nil {
				if n, ok := raw.(Node); ok {
					t.mount(t.root, t.view, n)
				}
				if mounted != nil {
					mounted()
				}
				if cleanup != nil {
					t.root.addCleanup(cleanup)
				}
			}
		}
		t.mount(t.root, t.view, Func(app))
	})
}

// Resize sets the surface size the root fills and the fold geometry measured at it (Env.Geometry), in one batch, so what reads either sees both new. A geometry without regions is divided at size; the zero Geometry is a surface without a fold. A new geometry at the same size still applies.
func (t *Tree) Resize(size Size, g Geometry) {
	if t.fit != (Size{}) {
		// The content decides the size: what the platform reports only says whether it still has to hear it.
		if size != t.reportedSize {
			t.reportedSize = size
			t.markLayout(t.view)
			t.changed = true
		}
		return
	}
	if len(g.Regions) == 0 {
		g = divide(size, g)
	}
	if size != t.size {
		t.size = size
		t.markLayout(t.view)
		t.changed = true
	}
	e := t.root.Env()
	t.root.Batch(func() {
		e.window.Set(size)
		if !e.geometry.GetUntracked().Equal(g) {
			e.geometry.Set(g.Clone())
		}
	})
}

// divide returns g's obstructions divided at size, with its hinge's state.
func divide(size Size, g Geometry) Geometry {
	d := driver.Divide(size, g.Obstructions)
	d.State, d.Angle, d.HasAngle = g.State, g.Angle, g.HasAngle
	return d
}

// fitContent has the surface take its content's size, up to largest, from the next commit on.
func (t *Tree) fitContent(largest Size) {
	t.fit, t.reportedSize = largest, t.size
	t.markLayout(t.view)
	t.changed = true
}

// fitted lays a surface that takes its content's size out at that size, up to t.fit, and sends its platform the size when it changed.
func (t *Tree) fitted() {
	t.layoutRoot(t.view, t.fit, false)
	l := t.view.lstate.Rounded.Size
	size := Size{Width: min(max(Points(l.Width), 1), t.fit.Width), Height: min(max(Points(l.Height), 1), t.fit.Height)}
	if size.Width != Points(l.Width) || size.Height != Points(l.Height) {
		// Too large, or empty: the content fills the size it gets.
		t.layoutRoot(t.view, size, true)
	}
	if size != t.size {
		t.size, t.refit = size, true
	}
	if size != t.reportedSize {
		t.reportedSize = size
		frame := WindowFrame{Width: size.Width, Height: size.Height}
		// A commit that lays out again, as its lists fill, sends only the last size: the platform would end at an earlier one.
		if i := slices.IndexFunc(t.commands, func(it driver.MountItem) bool {
			return it.Kind == driver.ItemWindow && it.Window.Action == driver.WindowFitSize
		}); i >= 0 {
			t.commands[i].Window.Frame = frame
			return
		}
		// Ahead of what the commit shows it with.
		t.commands = slices.Insert(t.commands, 0, driver.MountItem{Kind: driver.ItemWindow,
			Window: &driver.WindowCommand{Action: driver.WindowFitSize, Frame: frame}})
	}
}

// reportFit has the environment hear the size the content gave the surface, once its commit round is done.
func (t *Tree) reportFit() {
	if !t.refit {
		return
	}
	t.refit = false
	e := t.root.Env()
	g := divide(t.size, e.geometry.GetUntracked())
	t.root.Batch(func() {
		e.window.Set(t.size)
		if !e.geometry.GetUntracked().Equal(g) {
			e.geometry.Set(g)
		}
	})
}

// SetEnvironment records what the platform reports about where the app runs.
func (t *Tree) SetEnvironment(env Environment) {
	if t.window != nil && t.window.edgeDocked {
		env.CanPlaceWindows = false
	}
	previous := t.reported
	t.reported = env
	if t.hapticActive && (env.Lifecycle == LifecycleBackground || !env.HapticPatterns) {
		t.setHapticPattern(nil)
	}
	t.updateAppAddress(previous, env)
	e := t.root.Env()
	safe, headers := e.safe.GetUntracked(), e.headers
	// In one batch, so what reads the environment sees all of it new.
	t.root.Batch(func() {
		e.set(env)
		if t == t.rt.tree {
			e.network.setEnvironment(env.Network)
		}
	})
	// Modals and system headers are placed by the safe area.
	if e.safe.GetUntracked() != safe || e.headers != headers {
		t.markLayout(t.view)
	}
	// Resolving and emitting read parts of the environment directly, such as an image's scale.
	if environmentNeedsFullRound(previous, env) {
		t.markAll()
	}
}

// environmentNeedsFullRound reports whether env changes from previous in a field that resolving or emitting the tree reads directly. The fields below reach only what reads their Env signals (#178).
func environmentNeedsFullRound(previous, env Environment) bool {
	previous.WindowFrame = env.WindowFrame
	previous.Maximized = env.Maximized
	previous.Lifecycle = env.Lifecycle
	previous.TitleBar = env.TitleBar
	previous.ThermalState = env.ThermalState
	previous.LowPowerMode = env.LowPowerMode
	previous.Network = env.Network
	previous.HapticPatterns = env.HapticPatterns
	previous.PageURL, previous.PageBaseURL = env.PageURL, env.PageBaseURL
	previous.Displays = env.Displays
	previous.DisplayID = env.DisplayID
	return !previous.Equal(env)
}

// Size returns the surface size.
func (t *Tree) Size() Size { return t.size }

// Language returns the language the app shows (Env.Language): of its catalogs' languages, the one that suits the locale best, "" for an app without catalogs. The engine reads it for platform services that take a language only as they start, as WinSparkle does.
func (t *Tree) Language() string { return t.root.Env().Language() }

// Changed reports whether a Commit would produce items.
func (t *Tree) Changed() bool { return t.changed }

// within runs fn with o owning what it creates, untracked, in a batch.
func (t *Tree) within(o *Scope, fn func()) { t.rt.run(o, fn) }

func (t *Tree) newNode(kind driver.ViewKind, parent *node, owner *Scope) *node {
	n := &node{
		kind:          kind,
		parent:        parent,
		owner:         owner,
		base:          &zeroStyle,
		style:         &zeroStyle,
		props:         &zeroViewProps,
		h:             &noHandlers,
		nodeExtra:     &zeroExtra,
		sent:          sent{viewStyle: &defaultViewStyle, props: &zeroViewProps},
		layoutDirty:   true,
		childrenDirty: true,
		rtl:           t.rtl,
	}
	n.lstate.SkipAnswers = kind != driver.KindText
	if owner != nil && t.rt.workObserved {
		n.lstate.SetBeginWork(owner.BeginWork)
	}
	t.pointersMoved = t.pointerStyled
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
		addChild(parent, n)
		t.childrenChanged(parent)
	}
	t.markResolve(n)
	if kind != 0 && owner != nil && owner.rt.subtreeAppearance {
		owner.renderEffect(func() {
			if a := subtreeAppearance(owner); a != n.appearance {
				n.ownExtra().appearance = a
				t.mark(n)
			}
		})
	}
	return n
}

func (t *Tree) newRegion(parent *node, owner *Scope) *node {
	n := t.newNode(0, parent, owner)
	if t.regions == nil {
		t.regions = map[*node]struct{}{}
	}
	t.regions[n] = struct{}{}
	return n
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
		n.flat = nil
		t.queuePeerParent(n)
		t.queueRelations(n)
		if len(t.relationSources) > 0 {
			t.queueRelationRoot(n)
		}
		t.reorder(n)
		t.markLayout(n)
		t.queueClassKids(n)
	}
}

// reorder records that n's native children may have changed or moved.
func (t *Tree) reorder(n *node) {
	n.childrenDirty = true
	t.mark(n)
	t.markGroups(n)
}

// markGroups marks the groups n is in that read their texts for a label, and the texts it is an attachment of, which read its label (sentProps): what n shows of its own may have changed.
func (t *Tree) markGroups(n *node) {
	for p := n.parent; p != nil; p = p.parent {
		// A text reads its attachments' labels where it has hidden spans.
		if p.props.Accessible == AccessibleYes && p.props.Label == "" && p.text == nil || p.text != nil && p.text.attachments > 0 {
			t.mark(p)
		}
	}
}

// mark records that n's parts may differ from what was sent, so the next
// commit compares them. A region marks the view it lays out in.
func (t *Tree) mark(n *node) {
	t.changed = true
	for n != nil && n.kind == 0 {
		n = n.parent
	}
	if n == nil {
		return
	}
	n.dirty = true
	for p := n.nativeParent(); p != nil && !p.dirtyKids; p = p.nativeParent() {
		p.dirtyKids = true
	}
}

// markResolve records that n's styles or content changed, so the next
// commit resolves it and sends what changed.
func (t *Tree) markResolve(n *node) {
	t.mark(n)
	t.markGroups(n)
	n.unresolved = true
	for p := n.parent; p != nil && !p.resolveKids; p = p.parent {
		p.resolveKids = true
	}
}

// carry marks the chains above n again after n moved to another parent,
// for the marks inside it.
func (t *Tree) carry(n *node) {
	if n.unresolved || n.resolveKids {
		for p := n.parent; p != nil && !p.resolveKids; p = p.parent {
			p.resolveKids = true
		}
	}
	if n.dirty || n.dirtyKids {
		for p := n.nativeParent(); p != nil && !p.dirtyKids; p = p.nativeParent() {
			p.dirtyKids = true
		}
	}
}

// markAll has the next commit resolve and compare every node, as after a
// change of the environment they all read.
func (t *Tree) markAll() {
	t.full = true
	t.changed = true
}

// touch records that n's parts changed without a layout change.
func (t *Tree) touch(n *node) {
	t.mark(n)
	t.markGroups(n)
}

// relayout records that n's parts changed its layout.
func (t *Tree) relayout(n *node) {
	t.markLayout(n)
	t.markResolve(n)
}

// clear discards a region's children.
func (t *Tree) clear(region *node) {
	for _, c := range region.children {
		t.discard(c)
	}
	clear(region.children)
	region.children = region.children[:0]
	t.childrenChanged(region)
}

// discard deletes n's subtree. Its owner disposes the bindings.
func (t *Tree) discard(n *node) {
	if n.deleted {
		return
	}
	if n.elem != nil && !n.elem.released {
		// An element only leaves; its owner releases it.
		n.parent = nil
		return
	}
	n.deleted = true
	if n.ink != nil {
		delete(n.ink.nodes, n)
	}
	if n.kind == 0 {
		delete(t.regions, n)
	}
	if n.kind == driver.KindModal {
		// What it covered comes back to the keymap (keyActive).
		t.keysChanged = true
	}
	if n.states&stateFocusWithin != 0 || t.focus == n {
		t.focusMoved = true
	}
	if n.place != nil {
		// What it shows is the holder's, which keeps it.
		n.place.unplace(n)
	}
	delete(t.sticky, n)
	if n.part.Focused {
		t.forgetFocusPart(n)
	}
	if n.ref != nil && n.ref.n == n {
		n.ref.n = nil
	}
	if n.hoist != nil {
		h := n.hoist.host
		// Its host lets go of it, else the root view keeps the whole subtree. A new slice, as a loop over the host's may be what removes it.
		if i := slices.Index(h.hosted, n); i >= 0 {
			h.ownExtra().hosted = slices.Concat(h.hosted[:i:i], h.hosted[i+1:])
		}
		t.reorder(h)
		t.markLayout(h)
	}
	if n.accessory != nil {
		t.markAll() // the inputs naming it
	}
	if n.kind != 0 {
		t.deleted = append(t.deleted, n.id)
		if n.created && n.match != "" {
			t.matchGone = append(t.matchGone, n)
		}
	}
	if n.text != nil {
		n.text.release()
	}
	if n.input != nil {
		n.input.measured.release()
		n.input.sized.release()
		if s := n.input.suggest; s != nil {
			s.unbind(n)
		}
	}
	for _, c := range n.children {
		t.discard(c)
	}
	t.changed = true
}

// setChildren replaces a region's children with nodes already in it.
func (t *Tree) setChildren(region *node, children []*node) {
	old := region.children
	region.children = append(region.children[:0], children...)
	if len(children) < len(old) {
		clear(old[len(children):]) // still in the array, which would keep the views dropped
	}
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
	if end := cx.BeginWork(); end != nil {
		defer end()
	}
	t.checkTwins(el)
	switch el := el.(type) {
	case nil:
	case mounter:
		el.mountIn(t, cx, parent)
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
	case *Element:
		if el.t == t {
			t.place(el.n, parent, nil)
		}
	case regionMounter:
		el.mount(t, t.newRegion(parent, cx), cx)
	default:
		child := cx.Child()
		if t.rt.dev != nil {
			t.rt.dev.component(child, el)
		}
		t.within(child, func() { t.mount(child, parent, el.Render(child)) })
	}
}

// mounter is an element the tree mounts: each element's own method,
// rather than a case here, links its mounting only into apps that make
// one.
type mounter interface {
	mountIn(t *Tree, cx *Scope, parent *node)
}

type regionMounter interface {
	mount(t *Tree, region *node, owner *Scope)
}

// bindStyle sets n's style, binding StyleFunc when there is one.
func (t *Tree) bindStyle(cx *Scope, n *node, base Style, fn func() Style) {
	n.setBase(t, &base)
	t.restyle(n)
	t.syncPosition(n)
	if fn == nil {
		if safe, keyboard := n.base.envUses(); safe || keyboard {
			cx.renderEffect(func() {
				t.readEnv(cx, n, n.base)
				if t.restyle(n) {
					t.markLayout(n)
				}
			})
		}
		return
	}
	cx.renderEffect(func() {
		if n.dyn == nil {
			n.dyn = new(Style)
		}
		*n.dyn = fn()
		t.readEnv(cx, n, n.base, n.dyn)
		changed := t.restyle(n)
		t.syncPosition(n)
		if changed {
			t.markLayout(n)
		}
	})
}

// restyle merges n's style from its parts and states, which the next
// commit resolves and sends, and reports whether what layout reads of it
// changed: a color does not.
func (t *Tree) restyle(n *node) bool {
	t.queuePeerSiblings(n)
	t.markResolve(n)
	if n.h.transitionEnd != nil && n.created && !n.restyled {
		n.restyled = true
		t.ending = append(t.ending, n)
	}
	// Fast path: static view sharing its interned base style directly.
	if n.dyn == nil && n.cls == nil && !n.hasFlag(flagBaseOwned) &&
		!n.base.hasStateStyles() && !n.base.hasZeroSentinel() &&
		n.env == nil && (n.screen == nil || !n.screen.hidden) &&
		len(t.suspended) == 0 && !n.hasParentSpacing() {
		if f := n.nativeParent(); f != nil && f.showsExpanded {
			p := n.labelProps()
			t.showTrigger(n, &p)
		}
		z, sticky := n.stackRank()
		was := n.style
		lines, overflow, pointer, visibility, display := was.NumberOfLines, was.TextOverflow, was.PointerEvents, was.Visibility, was.Display
		n.style = n.base
		n.flags &^= flagStyleOwned
		return t.finishRestyle(n, z, sticky, lines, overflow, pointer, visibility, display)
	}
	p := n.labelProps()
	p.Checked = p.Checked || n.part.Checked
	p.Indeterminate = p.Indeterminate || n.part.Indeterminate
	p.Selected = p.Selected || n.part.Selected
	p.Disabled = p.Disabled || n.part.Disabled
	p.Invalid = p.Invalid || n.part.Invalid
	t.showTrigger(n, &p)
	own := *n.base
	if n.dyn != nil {
		own = own.Merge(*n.dyn)
	}
	if p.Disabled {
		// A disabled view's own state styles wait, but disabled's; its
		// classes' variants, which merge next, still apply (NSH's
		// state_style).
		disabled := own.Disabled
		var none Style
		own.states().copyFrom(none.states())
		own.Disabled = disabled
	}
	merged := n.withClasses(own)
	var g shownStates
	if gs := merged.GroupStyle; gs != nil {
		g = t.groupState(n, gs.Name)
	}
	z, sticky := n.stackRank()
	was := n.style
	lines, overflow, pointer, visibility, display := was.NumberOfLines, was.TextOverflow, was.PointerEvents, was.Visibility, was.Display
	t.sscratch = merged.withStates(statesOf(t.shownInteraction(n), &p, n.part.Data), g)
	if n.cls != nil && n.cls.related && n.cls.important != &zeroStyle {
		t.sscratch = mergeStyles(t.sscratch, *n.cls.important)
	}
	if t.sscratch.hasZeroSentinel() {
		t.sscratch = t.sscratch.unzeroed()
	}
	if p.Disabled {
		// The platform still follows the states a disabled view's own
		// styles wait for, which the event mask reads.
		orig := *n.base
		if n.dyn != nil {
			orig = orig.Merge(*n.dyn)
		}
		full := n.withClasses(orig)
		t.sscratch.Hover, t.sscratch.Focus, t.sscratch.Active, t.sscratch.FocusWithin = full.Hover, full.Focus, full.Active, full.FocusWithin
		t.sscratch.FocusVisible = full.FocusVisible
	}
	n.applyEnvStyle(&t.sscratch)
	if n.screen != nil && n.screen.hidden {
		t.sscratch.Visibility = VisibilityHidden
	}
	if len(t.suspended) > 0 && t.suspendedIn(n) {
		t.sscratch.Display = DisplayNone
	}
	switch {
	case n.hasFlag(flagStyleOwned):
		*n.style = t.sscratch
	case !n.hasFlag(flagBaseOwned) && sameStyle(&t.sscratch, n.base):
		n.style = n.base
	case (n.dyn == nil || !n.created) && !n.hasFlag(flagBaseOwned):
		n.style = t.internStyle(&t.sscratch)
	default:
		s := new(Style)
		*s = t.sscratch
		n.style = s
		n.flags |= flagStyleOwned
	}
	return t.finishRestyle(n, z, sticky, lines, overflow, pointer, visibility, display)
}

func (t *Tree) finishRestyle(n *node, z int, sticky bool, lines int, overflow EllipsizeMode, pointer PointerEvents, visibility Visibility, display Display) bool {
	if n.style.NumberOfLines != lines || n.style.TextOverflow != overflow {
		if n.text != nil {
			n.text.stale = true
		}
	}
	if n.style.PointerEvents != pointer {
		t.pointerStyled, t.pointersMoved = true, true
	}
	if n.style.Visibility != visibility {
		// Hiding a view leaves its frame where it is, but not among the boxes the layout audit draws.
		t.framesMoved = true
	}
	if z2, sticky2 := n.stackRank(); z2 != z || sticky2 != sticky {
		if parent := n.nativeParent(); parent != nil {
			t.reorder(parent)
		}
	}
	if (n.style.Display != display || n.style.Visibility != visibility && n.screen != nil) && len(t.keys) > 0 {
		// The bindings of what it hides leave the keymap, and come back as it shows (keyActive).
		t.keysChanged = true
	}
	// Roots are placed by style the layout leaves out of theirs.
	if n.lstyle != nil && !n.isRoot && n.hoist == nil {
		t.lscratch = n.layoutStyle()
		if t.lscratch.Equal(n.lstyle) {
			return false
		}
	}
	n.lstyle = nil
	// Its edges may change where its frame does not, as a padding does.
	t.framesMoved = true
	// A style transition moves the frame its own change gives n, as CSS
	// transitions a changed width; a parent's resize alone does not.
	if n.created && !n.transit && n.style.Transition.Properties&TransitionLayout != 0 && n.style.Transition.Duration > 0 {
		n.transit = true
		t.transits = append(t.transits, n)
	}
	return true
}

// groupState returns the states of n's nearest group named name, or of
// any group when name is empty.
func (t *Tree) groupState(n *node, name string) shownStates {
	for a := n.parent; a != nil; a = a.parent {
		if g := a.style.Group; g != "" && (name == "" || g == name) {
			return t.sharedStates(a)
		}
	}
	return shownStates{}
}

// sharedStates returns the states n shows the views that follow it as their group or peer: its props with its part's and its input's.
func (t *Tree) sharedStates(n *node) shownStates {
	p := n.labelProps()
	p.Checked = p.Checked || n.part.Checked
	p.Indeterminate = p.Indeterminate || n.part.Indeterminate
	p.Selected = p.Selected || n.part.Selected
	p.Disabled = p.Disabled || n.part.Disabled || n.input != nil && n.input.config.Disabled
	p.Invalid = p.Invalid || n.part.Invalid
	return statesOf(t.shownInteraction(n), &p, n.part.Data)
}

// restyleExpanded restyles n, and the view that shows its expanded state,
// as it changed.
func (t *Tree) restyleExpanded(n *node) {
	nodes := []*node{n}
	if n.showsExpanded {
		nodes = append(nodes, n.flatChildren()...)
	}
	for _, k := range nodes {
		if t.restyle(k) {
			t.markLayout(k)
		}
		t.restyleGroup(k)
	}
}

// showTrigger gives n the expanded state of a trigger around it that
// shows it (View.ShowsExpanded), for its styles and the platform's prop.
// The trigger stays the control screen readers hear.
func (t *Tree) showTrigger(n *node, p *driver.ViewProps) {
	f := n.nativeParent()
	if f == nil || !f.showsExpanded {
		return
	}
	if !n.props.Expandable || n.props.Expanded != f.props.Expanded {
		np := n.ownProps()
		np.Expandable, np.Expanded = true, f.props.Expanded
		t.touch(n)
	}
	p.Expandable, p.Expanded = true, f.props.Expanded
}

// setState records an interaction state and restyles n if a state style
// depends on it, and the nodes inside it that follow it as their group.
func (t *Tree) setState(n *node, state uint8, on bool) {
	was := n.states
	if on {
		n.states |= state
	} else {
		n.states &^= state
	}
	if n.states == was {
		return
	}
	if n.style.Hover != nil || n.style.Focus != nil || n.style.FocusVisible != nil || n.style.Active != nil ||
		n.style.FocusWithin != nil {
		if t.restyle(n) {
			t.markLayout(n)
		}
	}
	t.restyleGroup(n)
}

// focusReported records what the platform says of the keyboard focus. A
// view taking it is the focus, in whatever order a move is reported: the
// blur of another view changes nothing, and the root taking it is none.
func (t *Tree) focusReported(n *node, focused bool) {
	switch {
	case focused && n.isRoot:
		n = nil
	case !focused && t.focus != n:
		return
	case !focused:
		n = nil
	}
	if n != nil {
		t.focusVisible = t.focusShows(n)
	}
	if t.focus != n {
		gone := t.focus
		t.focusByKeyboard = n != nil && t.keyboardMovesFocus()
		if t.focusByKeyboard {
			// A stop the keyboard reaches scrolls into view.
			t.reveals = append(t.reveals, reveal{n: n, opts: RevealOptions{Vertical: ScrollAlignNearest, Horizontal: ScrollAlignNearest}})
		}
		t.focus, t.focusMoved = n, true
		t.changed = true
		// Confined bindings and those yielding to text-sensitive menu items follow the focus.
		if t.focusKeys() {
			t.keysChanged = true
		}
		if gone != nil && gone.deleted {
			// The focused view went in this pass, before a commit noticed: its blur runs now, before the OnFocus of the view the platform reports, as a move between two views that stay runs them.
			t.blurRemoved(gone, true)
		}
	} else if n != nil && (t.focusVisibleOn == n) != t.focusVisible {
		t.focusMoved, t.changed = true, true
	}
}

// inputModality records what the user gave the surface input with last,
// as the platform reports it before the focus moves it causes. A key
// typed while the focus does not show makes it show, as browsers do; a
// pointer leaves a focus that shows as it is, as only a move of the focus
// decides again.
func (t *Tree) inputModality(m uint32) {
	t.modality = m
	if m == driver.ModalityKeyboard && t.focus != nil && !t.focusVisible {
		t.focusVisible = true
		t.focusMoved, t.changed = true, true
	}
}

// focusShows reports whether the focus coming to n shows (focus-visible:),
// as CSS's :focus-visible decides: an input that edits always shows it,
// as it takes typing, and any other view unless the last input was a
// pointer. So Tab, a group's arrows and a shortcut show it, a press or a
// tap does not, and a move the app makes shows it after a key and not
// after a press. Before any input it shows, as a browser's autofocus.
func (t *Tree) focusShows(n *node) bool {
	edits := n.kind == driver.KindInput && n.input != nil && !n.input.config.Readonly && !n.input.config.Disabled
	return edits || t.keyboardMovesFocus()
}

// keyboardMovesFocus reports whether the focus moving now is the
// keyboard's, by the surface's last input: unless that was a pointer.
// focus-visible: (focusShows) and the tooltips the focus shows
// (updateFocusHint) follow it, the tooltips with no exception for an
// input that edits, so a click into a field leaves its tooltip to the
// pointer.
func (t *Tree) keyboardMovesFocus() bool {
	return t.modality != driver.ModalityPointer
}

// updateFocusHint shows the tooltip of the view the keyboard moved the
// focus onto or into, the nearest around the focus that has one, and
// hides the one it showed before, once the commit is out.
func (t *Tree) updateFocusHint() {
	var tip *node
	if t.focusByKeyboard {
		for n := t.focus; n != nil; n = n.parent {
			if n.tip != nil {
				tip = n
				break
			}
		}
	}
	old := t.focusHint
	if tip == old {
		return
	}
	t.focusHint = tip
	if old != nil && !old.deleted && old.tip != nil {
		t.after(old, func() { old.tip.focus(false) })
	}
	if tip != nil {
		t.after(tip, func() { tip.tip.focus(true) })
	}
}

// updateFocusVisible puts stateFocusVisible on the focus while it shows
// and takes it off the node that had it, and restyles the parts that show
// the focus when it starts or stops showing.
func (t *Tree) updateFocusVisible() {
	target := t.focus
	if !t.focusVisible || target != nil && target.deleted {
		target = nil
	}
	old := t.focusVisibleOn
	if target == old {
		return
	}
	t.focusVisibleOn = target
	if old != nil && !old.deleted {
		t.setState(old, stateFocusVisible, false)
	}
	if target != nil {
		t.setState(target, stateFocusVisible, true)
	}
	if (old == nil) != (target == nil) {
		parts := t.focusParts[:0]
		for _, p := range t.focusParts {
			if p.deleted || !p.part.Focused {
				continue
			}
			parts = append(parts, p)
			if t.restyle(p) {
				t.markLayout(p)
			}
			t.restyleGroup(p)
		}
		clear(t.focusParts[len(parts):])
		t.focusParts = parts
	}
}

// partStates sets n's part states, keeping the nodes whose part shows the
// focus.
func (t *Tree) partStates(n *node, p PartStates) {
	if p.Focused && !n.part.Focused && !slices.Contains(t.focusParts, n) {
		t.focusParts = append(t.focusParts, n)
	} else if !p.Focused && n.part.Focused {
		t.forgetFocusPart(n)
	}
	if !p.Checked && !p.Indeterminate && !p.Selected && !p.Disabled && !p.Invalid && !p.Focused && len(p.Data) == 0 {
		if n.nodeExtra != &zeroExtra {
			n.nodeExtra.part = PartStates{}
		}
		return
	}
	n.ownExtra().part = p
}

// forgetFocusPart takes n out of the nodes whose part shows the focus,
// as its part stops showing it or it is deleted, so the list keeps no
// removed subtree.
func (t *Tree) forgetFocusPart(n *node) {
	if i := slices.Index(t.focusParts, n); i >= 0 {
		t.focusParts = slices.Delete(t.focusParts, i, i+1)
	}
}

// shownInteraction is n's interaction states with those its part shows:
// a part focused shows the focus too while the focus shows.
func (t *Tree) shownInteraction(n *node) uint8 {
	if n.part.Focused {
		if t.focusVisibleOn != nil {
			return n.states | stateFocused | stateFocusVisible
		}
		return n.states | stateFocused
	}
	return n.states
}

// updateFocusWithin puts stateFocusWithin on the focus and the nodes
// around it, and takes it off those it left: the two chains are walked
// and nothing else. The nodes around are parents in the core's tree,
// where an overlay is the child of the view the app put it in, so the
// focus in a popup is within the view holding its trigger, as in a React
// portal.
func (t *Tree) updateFocusWithin() {
	t.focusMoved = false
	if t.focus != nil && t.focus.deleted {
		t.blurRemoved(t.focus, false)
		t.focus = nil
		if t.focusKeys() {
			t.keysChanged = true
		}
	}
	old := t.focusChain
	var chain []*node
	for n := t.focus; n != nil; n = n.parent {
		chain = append(chain, n)
	}
	t.focusChain = chain
	for _, n := range old {
		if !n.deleted && !slices.Contains(chain, n) {
			t.setState(n, stateFocusWithin, false)
			t.focusCrossed(n, n.h.focusOut)
		}
	}
	for _, n := range chain {
		if !slices.Contains(old, n) {
			t.focusCrossed(n, n.h.focusIn)
		}
		t.setState(n, stateFocusWithin, true)
	}
}

// blurRemoved ends the focus of n, the focused view, which left the tree: the platform's blur for it finds no view (node), so its OnBlur runs here, once. From the commit (updateFocusWithin) it is posted to run after it, as OnFocusOut is on the views around it that stay; when the platform reports the next focus first (now), it runs at once, before that view's OnFocus. Its own owner usually went with it, a Show's branch or a list's row, so it runs under the nearest owner around it still there, or the tree's root for content discarded out of the tree, such as a Holder's.
func (t *Tree) blurRemoved(n *node, now bool) {
	n.states &^= stateFocused
	blur := n.h.blur
	if blur == nil {
		return
	}
	o := t.root
	for p := n; p != nil; p = p.parent {
		if p.owner != nil && !p.owner.disposed {
			o = p.owner
			break
		}
	}
	switch {
	case o.disposed:
	case now:
		t.within(o, blur)
	default:
		o.Post(blur)
	}
}

// focusCrossed runs fn, a view's OnFocusIn or OnFocusOut, after the commit
// that saw the focus cross it.
func (t *Tree) focusCrossed(n *node, fn func()) {
	if fn != nil {
		n.owner.Post(fn)
	}
}

// updatePointerEvents works out what each node mounts of the pointer
// events of the styles around it, as CSS's none inherits: a node that is
// none by its own value or the nearest above mounts box-none when a node
// inside it takes the pointer, none at the top of what it blocks, and
// nothing below that.
func (t *Tree) updatePointerEvents() {
	t.pointersMoved = false
	if !t.pointerStyled || t.view == nil {
		return
	}
	var mark func(n *node, none bool) bool
	mark = func(n *node, none bool) bool {
		own := cmpOr(n.style.PointerEvents, n.ownPointer)
		switch own {
		case PointerEventsNone:
			none = true
		case 0:
		default:
			none = false
		}
		n.pointerNone = none
		// The box values are the view's alone: what is in it inherits auto.
		target := own != 0 && own != PointerEventsNone
		for _, k := range n.children {
			if mark(k, none) {
				target = true
			}
		}
		n.pointerTarget = target
		return target
	}
	var apply func(n *node, covered bool)
	apply = func(n *node, covered bool) {
		v := cmpOr(n.style.PointerEvents, n.ownPointer)
		switch {
		case n.pointerNone && n.pointerTarget:
			v, covered = PointerEventsBoxNone, false
		case n.pointerNone && covered:
			v = PointerEventsAuto
		case n.pointerNone:
			v, covered = PointerEventsNone, true
		case v == PointerEventsOn:
			v = PointerEventsAuto
		}
		if n.props.PointerEvents != v {
			n.ownProps().PointerEvents = v
			t.touch(n)
		}
		for _, k := range n.children {
			apply(k, covered)
		}
	}
	mark(t.view, false)
	apply(t.view, false)
}

// restyleGroup restyles the nodes inside group n that follow its states.
func (t *Tree) restyleGroup(n *node) {
	t.queuePeerSiblings(n)
	t.queueRelations(n)
	if n.style.Group == "" {
		return
	}
	t.restyleFollowers(n)
}

// restyleFollowers restyles the nodes inside n that follow a group's
// states.
func (t *Tree) restyleFollowers(n *node) {
	var walk func(c *node)
	walk = func(c *node) {
		for _, k := range c.children {
			if k.style.GroupStyle != nil {
				if t.restyle(k) {
					t.markLayout(k)
				}
			}
			walk(k)
		}
		if c.text != nil {
			for _, sp := range c.text.spans {
				if sp.attach != nil {
					walk(sp.attach)
				}
			}
		}
	}
	walk(n)
}

func (t *Tree) bindBool(cx *Scope, n *node, fn func() bool, dst *bool) {
	if fn == nil {
		return
	}
	cx.renderEffect(func() {
		*dst = fn()
		if t.restyle(n) {
			t.markLayout(n)
		}
		t.restyleGroup(n)
	})
}

func (t *Tree) mountView(cx *Scope, parent *node, v View) {
	n := t.newNode(driver.KindView, parent, cx)
	t.bindClasses(cx, n, v.Class, v.ClassFunc)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	t.setViewParts(n, v)
	t.bindAccessibility(cx, n, a11y{labelFunc: v.LabelFunc, hintFunc: v.HintFunc, descriptionFunc: v.AccessibilityDescriptionFunc, value: v.AccessibilityValue, expanded: v.Expanded, expandable: v.Expandable,
		hidden: v.AccessibilityElementsHidden, actions: v.AccessibilityActions, actionsFunc: v.AccessibilityActionsFunc, accessible: v.Accessible,
		important: v.ImportantForAccessibility})
	if v.Disabled != nil {
		t.bindBool(cx, n, v.Disabled, &n.ownProps().Disabled)
	}
	if v.Follow != nil {
		t.bindFollow(cx, n, v)
	}
	if focusable := v.Focusable; focusable != nil {
		p := n.ownProps()
		cx.renderEffect(func() {
			p.Focusable = focusable()
			p.NoFocus = !p.Focusable
			t.restyle(n)
		})
	}
	if v.Selected != nil {
		t.bindBool(cx, n, v.Selected, &n.ownProps().Selected)
	}
	if v.SelectionItem != nil {
		t.bindBool(cx, n, v.SelectionItem, &n.ownProps().SelectionItem)
	}
	if v.Checked != nil {
		t.bindBool(cx, n, v.Checked, &n.ownProps().Checked)
	}
	if v.Indeterminate != nil {
		t.bindBool(cx, n, v.Indeterminate, &n.ownProps().Indeterminate)
	}
	if v.Pressed != nil {
		t.bindBool(cx, n, v.Pressed, &n.ownProps().Pressed)
	}
	if v.Busy != nil {
		t.bindBool(cx, n, v.Busy, &n.ownProps().Busy)
	}
	if v.Invalid != nil {
		t.bindBool(cx, n, v.Invalid, &n.ownProps().Invalid)
	}
	if v.HitSlopFunc != nil {
		p := n.ownProps()
		cx.renderEffect(func() {
			if e := v.HitSlopFunc(); e != p.HitSlop {
				p.HitSlop = e
				t.touch(n)
			}
		})
	}
	if v.AccessibilityLevel != nil {
		p := n.ownProps()
		cx.renderEffect(func() {
			if l := v.AccessibilityLevel(); l != p.Level {
				p.Level = l
				t.touch(n)
			}
		})
	}
	if href := v.Web.href(n.owner); href != "" || n.props.Href != "" {
		n.ownProps().Href = href
	}
	if v.Web.HrefFunc != nil {
		t.bindAccessibilityText(cx, n, &n.ownProps().Href, func() string { return v.Web.href(n.owner) })
	}
	if v.PartStates != nil {
		cx.renderEffect(func() {
			t.partStates(n, v.PartStates())
			if t.restyle(n) {
				t.markLayout(n)
			}
			t.restyleGroup(n)
		})
	}
	t.rebind(n, boundValues{pan: v.PanValues, pinch: v.PinchValue, rotation: v.RotationValue})
	for _, c := range v.Children {
		t.mount(cx, n, c)
	}
	if v.Tooltip != nil {
		t.mountTooltip(cx, n, v.Tooltip)
	}
}

// liveRegion is the live region of a view or text with role and live: a status is a polite one unless it says otherwise, as ARIA's status is.
func liveRegion(role Role, live LiveRegion) LiveRegion {
	if live == LiveRegionNone && role == RoleStatus {
		return LiveRegionPolite
	}
	return live
}

// setViewParts sets the parts of view n that v gives as values: its
// handlers and props, before the accessibility and states bound over
// them.
func (t *Tree) setViewParts(n *node, v View) {
	t.setLabels(n, v.LabelFor, v.LabelledBy)
	n.ownPointer, n.showsExpanded = v.PointerEvents, v.ShowsExpanded
	t.pointersMoved = t.pointerStyled
	n.setHandlers(handlers{focusIn: v.OnFocusIn, focusOut: v.OnFocusOut, press: v.OnPress, longPress: v.OnLongPress, pressDown: v.OnPressDown, pressUp: v.OnPressUp,
		mouseEnter: v.OnMouseEnter, mouseLeave: v.OnMouseLeave, mouseMove: v.OnMouseMove, pointerDelta: v.OnPointerDelta, pointerUnlock: v.OnPointerUnlock,
		mouseDown: v.OnMouseDown, mouseUp: v.OnMouseUp,
		textSelection: v.OnTextSelection,
		layout:        v.OnLayout, contextMenu: v.OnContextMenu, touch: v.OnTouch, touchCapture: v.OnTouchCapture,
		pan: v.OnPan, pinch: v.OnPinch, rotation: v.OnRotation, wheel: v.OnWheel,
		fileEnter: v.OnFileDragEnter, fileLeave: v.OnFileDragLeave, fileDrop: v.OnFileDrop,
		focus: v.OnFocus, blur: v.OnBlur, keyDown: v.OnKeyDown, keyUp: v.OnKeyUp, transitionEnd: v.OnTransitionEnd,
		action: v.OnAccessibilityAction})
	if v.Responder != nil || n.nodeExtra != &zeroExtra {
		n.ownExtra().responder = v.Responder
	}
	if v.MatchID != "" || n.nodeExtra != &zeroExtra {
		n.ownExtra().match = v.MatchID
	}
	n.setProps(driver.ViewProps{Role: v.Role, Label: v.Label, Hint: v.Hint, Description: v.AccessibilityDescription,
		LiveRegion: liveRegion(v.Role, v.LiveRegion), TestID: v.TestID, MultiSelectable: v.MultiSelectable, WindowDrag: v.Desktop.WindowDrag, SelectionArea: v.Selectable, PointerEvents: v.PointerEvents, HitSlop: v.HitSlop,
		DelayLongPress: v.DelayLongPress, NoFocus: v.NoFocus, Ripple: v.Android.Ripple,
		PanActivation: v.PanActivation, PanScrollHandoff: v.PanScrollHandoff})
	if v.IOS.BackdropMaterial != 0 || v.Android.Elevation != 0 || n.nodeExtra != &zeroExtra {
		ex := n.ownExtra()
		ex.material, ex.elevation = v.IOS.BackdropMaterial, v.Android.Elevation
	}
	if v.EdgeBar.Scroll != nil {
		bar := v.EdgeBar
		n.ownExtra().edgeBar = &bar
		t.edgeBars = append(t.edgeBars, n)
	}
	_, panClaim := v.Responder.panClaim()
	n.panning = v.OnPan != nil || v.PanValues != (AnimatedXY{}) || panClaim
	if f := n.nativeParent(); f != nil && f.showsExpanded {
		t.showTrigger(n, n.ownProps())
	}
	n.pinching, n.rotating = v.OnPinch != nil || v.PinchValue != nil, v.OnRotation != nil || v.RotationValue != nil
	setSelectionBlock(n, v.SelectionBlock, v.SelectionBlockFunc)
	t.setRef(n, v.Ref)
}

func (t *Tree) mountText(cx *Scope, parent *node, v Text) {
	n := t.newNode(driver.KindText, parent, cx)
	t.setLabels(n, v.LabelFor, v.LabelledBy)
	t.bindClasses(cx, n, v.Class, v.ClassFunc)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	n.setHandlers(handlers{press: v.OnPress, longPress: v.OnLongPress, layout: v.OnLayout})
	role := v.Role
	if role == RoleNone {
		role = RoleText
	}
	n.setProps(driver.ViewProps{Role: role, Label: v.Label, Hint: v.Hint, LiveRegion: liveRegion(v.Role, v.LiveRegion), TestID: v.TestID,
		Selectable: v.Selectable, PointerEvents: v.PointerEvents})
	n.ownPointer = v.PointerEvents
	t.bindAccessibility(cx, n, a11y{labelFunc: v.LabelFunc, hintFunc: v.HintFunc})
	if v.AccessibilityLevel != nil {
		p := n.ownProps()
		cx.renderEffect(func() {
			if level := v.AccessibilityLevel(); level != p.Level {
				p.Level = level
				t.touch(n)
			}
		})
	}
	setSelectionBlock(n, v.SelectionBlock, v.SelectionBlockFunc)
	t.setRef(n, v.Ref)
	n.text = &textNode{content: v.Content, maxLines: v.MaxLines, ellipsize: v.EllipsizeMode,
		noScale: v.NoFontScaling, maxScale: v.MaxFontSizeMultiplier,
		fit: v.AdjustsFontSizeToFit, minScale: max(v.MinimumFontScale, 0.1), lines: v.OnTextLayout, stale: true,
		selectable: v.Selectable, minWidth: -1}
	for i, sp := range v.Spans {
		if sp.Attachment != nil {
			w := t.newNode(driver.KindView, n, cx)
			w.ownExtra().attach = &attachNode{}
			outOfFlow(w)
			t.setClasses(w, sp.Class)
			t.bindStyle(cx, w, sp.Style, sp.StyleFunc)
			t.mount(cx, w, sp.Attachment)
			n.text.spans = append(n.text.spans, spanNode{content: sp.Content, copyItem: driver.CloneClipboardItem(sp.CopyItem), attach: w})
			n.text.attachments++
		} else {
			style, cls := t.spanStyle(n, sp.Class, sp.Style)
			n.text.spans = append(n.text.spans, spanNode{content: sp.Content, style: style, press: sp.OnPress, long: sp.OnLongPress, cls: cls})
			t.listenForSpan(n, sp)
		}
		if sp.ContentFunc != nil {
			cx.renderEffect(func() {
				if s := sp.ContentFunc(); s != n.text.spans[i].content {
					n.text.spans[i].content = s
					n.text.stale = true
					t.markResolve(n) // which lays it out unless it measures as before
				}
			})
		}
		if sp.StyleFunc != nil && sp.Attachment == nil {
			cx.renderEffect(func() {
				s := sp.Style.Merge(sp.StyleFunc())
				if c := n.text.spans[i].cls; c != nil {
					// The classes go under what StyleFunc gives.
					c.base = s
					var env classEnv
					Untracked(n.owner, func() bool { env = t.classEnv(n); return true })
					s = c.resolve(&env)
				}
				if !reflect.DeepEqual(s, n.text.spans[i].style) {
					n.text.spans[i].style = s
					n.text.stale = true
					t.markResolve(n)
				}
			})
		}
	}
	t.followSpanClasses(n)
	if v.SpansFunc != nil {
		cx.renderEffect(func() {
			spans := v.SpansFunc()
			for i, sp := range spans {
				if sp.StyleFunc != nil {
					spans[i].Style = sp.Style.Merge(sp.StyleFunc())
				}
			}
			same := len(spans) == len(n.text.spans) && n.text.prepared != nil
			for i := 0; same && i < len(spans); i++ {
				old := &n.text.spans[i]
				own := old.style
				if old.cls != nil {
					own = old.cls.base
				}
				if old.attach != nil || spans[i].Attachment != nil {
					same = spans[i].Content == old.content && sameAttachment(old, spans[i])
					continue
				}
				same = spans[i].Content == old.content && (old.cls == nil) == (spans[i].Class == "") &&
					(old.cls == nil || old.cls.list.class == spans[i].Class) && reflect.DeepEqual(spans[i].Style, own) &&
					(spans[i].OnPress == nil) == (old.press == nil) &&
					(spans[i].OnLongPress == nil) == (old.long == nil)
			}
			if same {
				// The presses cannot be compared: the latest take them.
				for i, sp := range spans {
					if sp.Attachment == nil {
						n.text.spans[i].press, n.text.spans[i].long = sp.OnPress, sp.OnLongPress
						t.listenForSpan(n, sp)
					}
				}
				return
			}
			// Each attachment the spans still hold keeps its view, whatever text lies around it and wherever it moves; the others go, and the new ones are made, each in a scope of its own.
			var olds []spanNode
			for _, sp := range n.text.spans {
				if sp.attach != nil {
					olds = append(olds, sp)
				}
			}
			kept := make([]bool, len(olds))
			moved := false // the attachments' views, or their order, changed
			k := 0         // the attachments so far
			clear(n.text.spans)
			n.text.spans = n.text.spans[:0]
			for _, sp := range spans {
				if sp.Attachment == nil {
					style, cls := t.spanStyle(n, sp.Class, sp.Style)
					n.text.spans = append(n.text.spans, spanNode{content: sp.Content, style: style, press: sp.OnPress, long: sp.OnLongPress, cls: cls})
					t.listenForSpan(n, sp)
					continue
				}
				j := keptAttachment(olds, kept, sp)
				if j >= 0 {
					kept[j] = true
					old := olds[j]
					old.content = sp.Content
					old.copyItem = driver.CloneClipboardItem(sp.CopyItem)
					n.text.spans = append(n.text.spans, old)
				} else {
					s := &Scope{}
					cx.adopt(s)
					w := t.newNode(driver.KindView, n, s)
					w.ownExtra().attach = &attachNode{}
					outOfFlow(w)
					t.setClasses(w, sp.Class)
					t.bindStyle(s, w, sp.Style, nil)
					t.within(s, func() { t.mount(s, w, sp.Attachment) })
					made := sp
					n.text.spans = append(n.text.spans, spanNode{content: sp.Content, copyItem: driver.CloneClipboardItem(sp.CopyItem), attach: w, made: &made, scope: s})
				}
				moved = moved || k >= len(olds) || j != k
				k++
			}
			moved = moved || k != len(olds)
			for j, old := range olds {
				if !kept[j] {
					if old.scope != nil {
						old.scope.Dispose()
					}
					t.discard(old.attach)
				}
			}
			n.text.attachments = k
			if moved {
				n.children = slices.DeleteFunc(n.children, func(c *node) bool { return c.deleted })
				t.childrenChanged(n)
			}
			t.followSpanClasses(n)
			n.text.stale = true
			t.markResolve(n)
		})
	}
	if v.ContentFunc != nil {
		cx.renderEffect(func() {
			if s := v.ContentFunc(); s != n.text.content || n.text.prepared == nil {
				n.text.content = s
				n.text.stale = true
				t.markResolve(n) // which lays it out unless it measures as before
			}
		})
	}
}

// keptAttachment returns the index of the first attachment of olds not yet kept that span sp holds with the same classes and style, or -1.
func keptAttachment(olds []spanNode, kept []bool, sp Span) int {
	for j := range olds {
		if !kept[j] && sameAttachment(&olds[j], sp) {
			return j
		}
	}
	return -1
}

// sameAttachment reports whether span sp is an attachment the view of attachment old was made of, with the same classes and style.
func sameAttachment(old *spanNode, sp Span) bool {
	return old.made != nil && sp.Attachment != nil && sameNode(old.made.Attachment, sp.Attachment) &&
		old.made.Class == sp.Class && reflect.DeepEqual(old.made.Style, sp.Style)
}

// sameNode reports whether a and b are the same node: the same pointer, or equal values of a type that compares. Other nodes, a Func among them, cannot be told apart and are never the same.
func sameNode(a, b Node) bool {
	if a == nil || b == nil || reflect.TypeOf(a) != reflect.TypeOf(b) || !reflect.ValueOf(a).Comparable() {
		return false
	}
	return a == b
}

// bindInputTheme keeps input n's theme colors, as cx's palette has them.
func (t *Tree) bindInputTheme(cx *Scope, n *node) {
	cx.renderEffect(func() {
		p, in := cx.Palette(), n.input
		if n.deleted || p.Selection == in.themeSelection && p.Accent == in.themeCaret {
			return
		}
		in.themeSelection, in.themeCaret = p.Selection, p.Accent
		t.markResolve(n)
	})
}

// inputSourced is v's OnFocus and OnBlur, selecting its InputSource as the
// field takes the focus and restoring it as the field loses it.
func (t *Tree) inputSourced(v Input) (focus, blur func()) {
	focus, blur = v.OnFocus, v.OnBlur
	if v.InputSource == "" {
		return focus, blur
	}
	return func() {
			t.selectInputSource(v.InputSource)
			if v.OnFocus != nil {
				v.OnFocus()
			}
		}, func() {
			if !v.KeepInputSource {
				t.selectInputSource("")
			}
			if v.OnBlur != nil {
				v.OnBlur()
			}
		}
}

func (t *Tree) mountInput(cx *Scope, parent *node, v Input) {
	n := t.newNode(driver.KindInput, parent, cx)
	t.setLabels(n, nil, v.LabelledBy)
	t.setClasses(n, v.Class)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	focus, blur := t.inputSourced(v)
	n.setHandlers(handlers{change: v.OnChange, submit: v.OnSubmit, layout: v.OnLayout, focus: focus, blur: blur, action: v.OnAccessibilityAction})
	n.setProps(driver.ViewProps{Role: cmpOr(v.Role, RoleTextField), Label: v.Label, Description: v.AccessibilityDescription, TestID: v.TestID})
	if v.Invalid != nil {
		t.bindBool(cx, n, v.Invalid, &n.ownProps().Invalid)
	}
	t.bindAccessibility(cx, n, a11y{labelFunc: v.LabelFunc, descriptionFunc: v.AccessibilityDescriptionFunc, value: v.AccessibilityValue, actions: v.AccessibilityActions, actionsFunc: v.AccessibilityActionsFunc, expanded: v.Expanded})
	t.setRef(n, v.Ref)
	in := &inputNode{valueFn: v.Value, value: v.DefaultValue, autoFocus: v.AutoFocus, accessory: v.InputAccessoryViewID,
		selection: v.OnSelectionChange, keyPress: v.OnKeyPress, composition: v.OnCompositionChange,
		contentSize: v.OnContentSizeChange, caret: v.OnCaretMove, paste: v.OnPaste, editAction: v.OnEditAction,
		noScale: v.NoFontScaling, maxScale: v.MaxFontSizeMultiplier,
		ownBar: v.Bar, hideIndicator: v.HideIndicator, onScroll: v.OnScroll}
	in.config = driver.InputConfig{Placeholder: v.Placeholder, Masked: v.Masked, Readonly: v.Readonly, KeyboardType: v.KeyboardType,
		Multiline: v.Multiline, MaxLength: max(v.MaxLength, 0), SubmitBehavior: v.SubmitBehavior, ReturnKey: v.ReturnKeyType,
		AutoCapitalize: v.AutoCapitalize, NoAutoCorrect: v.NoAutoCorrect, AutoComplete: v.AutoComplete,
		KeyboardAppearance: v.IOS.KeyboardAppearance, HideCaret: v.HideCaret, HideIndicator: v.Multiline && v.HideIndicator,
		PasteBehavior: v.PasteBehavior}
	n.ownExtra().input = in
	in.suggest = v.Suggest
	if in.suggest != nil {
		in.suggest.bind(t, n)
		in.suggest.sync(in.value)
		if in.selection == nil {
			in.selection = func(TextRange) {}
		}
		if in.caret == nil {
			in.caret = func(CaretEvent) {}
		}
		if in.composition == nil {
			in.composition = func(Composition) {}
		}
	}
	t.bindInputTheme(cx, n)
	t.bindBool(cx, n, v.Disabled, &in.config.Disabled)
	if v.PlaceholderFunc != nil {
		cx.renderEffect(func() {
			if p := v.PlaceholderFunc(); p != in.config.Placeholder {
				in.config.Placeholder = p
				t.markResolve(n) // a field fits it
				t.mark(n)
			}
		})
	}
	if v.Value != nil {
		cx.renderEffect(func() {
			in.value = v.Value()
			if s := in.suggest; s != nil {
				value := in.value
				cx.Untrack(func() { s.sync(value) })
			}
			t.markResolve(n) // a multiline field measures it
		})
	}
	if v.ReadonlyFunc != nil {
		cx.renderEffect(func() {
			if r := v.ReadonlyFunc(); r != in.config.Readonly {
				in.config.Readonly = r
				t.touch(n)
			}
		})
	}
	if v.KeyboardTypeFunc != nil {
		cx.renderEffect(func() {
			if k := v.KeyboardTypeFunc(); k != in.config.KeyboardType {
				in.config.KeyboardType = k
				t.touch(n)
			}
		})
	}
	if v.SelectedRange != nil {
		// After the value, which the commit sends before its commands.
		cx.renderEffect(func() {
			r := v.SelectedRange()
			t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemSelection, ID: n.id, Selection: r})
			t.changed = true
		})
	}
	t.refreshBar(n)
}

// scrollDefaults lets a scroll view shrink below its content, as React
// Native's ScrollView does; its content scrolls instead.
var scrollDefaults = Style{FlexShrink: 1}

func (t *Tree) mountScroll(cx *Scope, parent *node, v Scroll) {
	n := t.newNode(driver.KindScroll, parent, cx)
	t.setClasses(n, v.Class)
	defaults := scrollDefaults
	if v.NoShrink {
		defaults.FlexShrink = 0
	}
	t.bindStyle(cx, n, defaults.Merge(v.Style), v.StyleFunc)
	h := handlers{layout: v.OnLayout}
	if on := v.OnScroll; on != nil {
		h.scroll = func(p Point) { on(n.scrollEvent(p)) }
	}
	n.setHandlers(h)
	n.setProps(driver.ViewProps{Role: v.Role, Label: v.Label, TestID: v.TestID})
	if v.LabelFunc != nil {
		t.bindAccessibilityText(cx, n, &n.ownProps().Label, v.LabelFunc)
	}
	if v.Busy != nil {
		t.bindBool(cx, n, v.Busy, &n.ownProps().Busy)
	}
	t.setRef(n, v.Ref)
	n.ownExtra().scroll = &scrollNode{config: driver.ScrollConfig{Horizontal: v.Horizontal, HideIndicator: v.HideIndicator}, effects: v.EdgeEffects,
		ownBar: v.Bar, hideIndicator: v.HideIndicator}
	t.bindScrollSpec(cx, n, scrollSpec{behavior: v.Behavior, behaviorFunc: v.BehaviorFunc, offset: v.ContentOffset,
		refreshing: v.Refreshing, onRefresh: v.OnRefresh, onScroll: v.OnScroll, throttle: v.ScrollEventThrottle,
		onDragStart: v.OnScrollDragStart, onDragEnd: v.OnScrollDragEnd,
		onMomentum: v.OnMomentumStart, onMomentumEnd: v.OnMomentumEnd, onScrollEnd: v.OnScrollEnd,
		minZoom: v.MinZoomScale, maxZoom: v.MaxZoomScale, zoom: v.ZoomScale, zoomFunc: v.ZoomScaleFunc,
		noBouncesZoom: v.NoBouncesZoom, noPinch: v.NoPinchZoom, centerContent: v.CenterContent})
	t.rebind(n, boundValues{scroll: v.OffsetValues, pull: v.PullValue})
	for _, c := range v.Children {
		t.mount(cx, n, c)
	}
	t.refreshBar(n)
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
			t.clear(region)
		}
		b = &Scope{}
		cx.current().adopt(b)
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
			t.clear(region)
		}
		if on {
			b = t.branch(cx.current(), region, s.Then)
		} else {
			b = t.branch(cx.current(), region, s.Else)
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
			t.clear(region)
		}
		b = &Scope{}
		cx.current().adopt(b)
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
func (t *Tree) Dispatch(ev driver.Event) {
	if f := t.follower; f != nil {
		f.Event(ev)
	}
	if ev.Kind >= driver.EventWindowClose && t.surfaceEvent(ev) {
		return
	}
	// A video's pause and play share their kinds with the answers EventImageInfo and EventMenuClosed, which name no view: one for a view is the video's, or a removed video's.
	if (ev.Kind == driver.EventVideoPause || ev.Kind == driver.EventVideoPlay) && ev.ID != 0 {
		if n := t.node(ev.ID); n != nil && n.video != nil {
			t.dispatchVideo(n, ev)
		}
		return
	}
	// A key down shares its kind with a binding's press, which has a Count: a key down has an ID, or neither when no view that hears keys has the focus.
	if ev.Kind == driver.EventKeyDown && (ev.ID != 0 || ev.Count == 0) {
		t.keyDown(ev)
		return
	}
	if ev.Kind == driver.EventKeyUp {
		t.keyUp(ev)
		return
	}
	// A mouse button down shares its kind with EventAlert, which carries no Button: a mouse button down always has Button != MouseButtonNone.
	if ev.Kind == driver.EventMouseDown && ev.Button != driver.MouseButtonNone {
		t.mouseButton(ev, true)
		return
	}
	if ev.Kind == driver.EventMouseUp {
		t.mouseButton(ev, false)
		return
	}
	switch ev.Kind {
	case driver.EventPointerDelta, driver.EventPointerUnlocked:
		t.pointerLockEvent(ev)
		return
	case driver.EventTouch:
		t.touched(ev)
		return
	case driver.EventInputModality:
		t.inputModality(ev.Count)
		return
	case driver.EventBack:
		n := t.node(ev.ID)
		// Escape on a view of an overlay is that overlay's, a confined owner's only while the focus is within it.
		var overlay *node
		for p := n; p != nil && overlay == nil; p = p.parent {
			if p.h.cancel != nil && t.focusWithin(confinement(p.owner)) {
				overlay = p
			}
		}
		switch {
		case n != nil && n.screen != nil:
			t.backFrom(n)
		case overlay != nil:
			t.within(overlay.owner, overlay.h.cancel)
		default:
			t.back()
		}
		t.peek(nil, nil)
		// The browser took an entry back for it, which a back the app
		// refused gets again.
		t.historyAgain = true
		t.changed = true
		return
	case driver.EventBackCancelled:
		t.peek(nil, nil)
		return
	case driver.EventBackStarted:
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemBackPreview, ID: t.backPreview()})
		t.changed = true
		return
	case driver.EventScreenHeader:
		if n := t.node(ev.ID); n != nil && n.screen != nil {
			t.screenHeader(n, Points(ev.Value), ev.Size.Width)
		}
		return
	case driver.EventScreenInsets:
		if n := t.node(ev.ID); n != nil && n.screen != nil {
			if r := n.screen.reported.GetUntracked(); r == nil || *r != ev.Insets {
				in := ev.Insets
				t.within(n.owner, func() { n.screen.reported.Set(&in) })
			}
		}
		return
	case driver.EventRequestClose, driver.EventModalSize, driver.EventModalShown:
		if n := t.node(ev.ID); n != nil && n.hoist != nil && n.hoist.modal != nil {
			t.modalEvent(n, ev)
		}
		return
	case driver.EventModalDismissed:
		if t.lateDismissals[ev.ID] {
			// The fallback released this modal before its report came (awaitDismissal): the report has nothing left to do.
			delete(t.lateDismissals, ev.ID)
			return
		}
		if n := t.node(ev.ID); n != nil && n.hoist != nil && n.hoist.modal != nil {
			t.modalEvent(n, ev)
		} else {
			t.finishDismissal(ev.ID)
		}
		return
	case driver.EventAlert:
		t.answerAlert(ev.Count, ev.Index)
		return
	case driver.EventUpdate:
		if state, ok := ev.Data.(*driver.UpdaterState); ok && state != nil {
			// The platform's report leaves the core's own download progress as it is.
			next, cur := updaterState(*state), t.rt.updater.GetUntracked()
			next.Phase, next.Loaded, next.Total = cur.Phase, cur.Loaded, cur.Total
			t.rt.updater.Set(next)
			return
		}
		if update, ok := ev.Data.(*driver.Update); ok && update != nil {
			state := t.rt.updater.GetUntracked()
			if update.Err == "" && !ev.Finished {
				state.Available, state.Notes = update.Version, update.Notes
			}
			t.rt.updater.Set(state)
		}
		t.answer(ev)
		return
	case driver.EventClipboard, driver.EventOpenURL, driver.EventPick, driver.EventFiles, driver.EventWebCookies, driver.EventDiskSpace:
		t.answer(ev)
		return
	case driver.EventSavePreflight:
		t.answerSavePreflight(ev)
		return
	case driver.EventMenuClosed:
		t.popupClosed(ev.Count, true)
		return
	case driver.EventSensor:
		t.sensorEvent(ev)
		return
	case driver.EventSpeech:
		t.rt.speechEvent(ev)
		return
	case driver.EventDiscovery:
		t.discoveryEvent(ev)
		return
	case driver.EventAuthSession:
		t.authEvent(ev)
		return
	case driver.EventMediaCommand:
		t.rt.mediaCommand(ev)
		return
	case driver.EventAudio:
		if e, _ := ev.Data.(*driver.AudioEvent); e != nil && e.Phase == driver.AudioLatency {
			t.answer(ev)
		} else {
			t.rt.audioEvent(ev)
		}
		return
	case driver.EventSpeak:
		t.speakEvent(ev)
		return
	case driver.EventImageInfo:
		t.answerImageInfo(ev)
		return
	case driver.EventKey:
		t.pressKey(ev.Count)
		return
	case driver.EventMenu:
		t.chooseMenu(ev.Count)
		return
	case driver.EventDockMenu:
		t.chooseDockMenu(ev)
		return
	case driver.EventAnimation:
		t.animationEvent(ev)
		return
	}
	n := t.node(ev.ID)
	if n == nil {
		return
	}
	switch ev.Kind {
	case driver.EventPressDown, driver.EventPressUp, driver.EventPress, driver.EventLongPress, driver.EventContextMenu,
		driver.EventAccessibilityAction:
		// The user's own input on n lets its owner move the focus until the commit after it (mayMoveFocus).
		t.activated = n.owner
	}
	if n.web != nil && t.webDispatch != nil && ev.Kind != driver.EventFocus && ev.Kind != driver.EventBlur {
		t.webDispatch(t, n, ev)
		return
	}
	if t.dispatchPictureInPicture(n, ev) {
		return
	}
	if n.video != nil && t.dispatchVideo(n, ev) {
		return
	}
	if n.native != nil && t.dispatchNative(n, ev) {
		return
	}
	if n.shader != nil && t.dispatchShader(n, ev) {
		return
	}
	if n.image != nil && (n.image.sent.Source.Pixels != nil || n.image.sent.Source.Offscreen != nil) {
		switch ev.Kind {
		case driver.EventLoadStart, driver.EventProgress, driver.EventLoad, driver.EventError, driver.EventLoadEnd:
			// Pixels and offscreen drawings load nothing: a backend that shows them through a PNG of its own reports no loads, nor does one of an earlier source.
			return
		}
	}
	h := n.h
	call := func(fn func()) {
		if fn != nil {
			t.within(n.owner, fn)
		}
	}
	press := func(fn func(PressEvent)) {
		if fn != nil {
			e := pressOf(ev)
			e.Keyboard = t.modality == driver.ModalityKeyboard
			t.within(n.owner, func() { fn(e) })
		}
	}
	mouse := func(fn func(MouseEvent)) {
		if fn != nil {
			t.within(n.owner, func() { fn(mouseOf(ev)) })
		}
	}
	switch ev.Kind {
	case driver.EventPress:
		t.setState(n, statePressed, false)
		if n.labelProps().Disabled {
			break
		}
		// A span's own press, else the text's.
		if tn := n.text; tn != nil && len(tn.spans) > 0 {
			if sp := tn.spanAt(ev.Index); sp != nil && sp.press != nil {
				press(sp.press)
				break
			}
		}
		if t.record != nil && h.press != nil {
			t.devFlushScrolls()
			t.devRecordPress(n)
		}
		press(h.press)
		t.pressLabel(n, ev)
	case driver.EventLongPress:
		if n.labelProps().Disabled {
			break
		}
		if tn := n.text; tn != nil && len(tn.spans) > 0 {
			if sp := tn.spanAt(ev.Index); sp != nil && sp.long != nil {
				press(sp.long)
				break
			}
		}
		press(h.longPress)
	case driver.EventAccessibilityAction:
		if h.canvasAction != nil && n.canvas != nil {
			if t.canvasNodeAccepts(n, ev.CanvasNode, ev.Text) {
				ae := CanvasActionEvent{
					NodeID:   ev.CanvasNode,
					Action:   ev.Text,
					Value:    ev.Value,
					HasValue: ev.HasValue,
				}
				t.within(n.owner, func() { h.canvasAction(ae) })
			}
			break
		}
		if p := n.labelProps(); h.action != nil && !p.Disabled && p.HasAction(ev.Text) {
			t.within(n.owner, func() { h.action(ev.Text) })
		}
	case driver.EventPressDown:
		t.setState(n, statePressed, !n.labelProps().Disabled)
		press(h.pressDown)
		if n.tip != nil {
			call(n.tip.hide)
		}
	case driver.EventPressUp:
		t.setState(n, statePressed, false)
		press(h.pressUp)
	case driver.EventMouseEnter:
		t.setState(n, stateHovered, true)
		mouse(h.mouseEnter)
		if n.tip != nil {
			call(n.tip.show)
		}
	case driver.EventMouseLeave:
		t.setState(n, stateHovered, false)
		mouse(h.mouseLeave)
		if n.tip != nil {
			call(n.tip.hide)
		}
	case driver.EventMouseMove:
		mouse(h.mouseMove)
	case driver.EventFocus:
		t.setState(n, stateFocused, true)
		t.focusReported(n, true)
		if in := n.input; in != nil && in.suggest != nil {
			s := in.suggest
			t.within(n.owner, func() { s.focus(true) })
		}
		call(h.focus)
	case driver.EventBlur:
		t.setState(n, stateFocused, false)
		t.focusReported(n, false)
		if in := n.input; in != nil && in.suggest != nil {
			s := in.suggest
			t.within(n.owner, func() { s.focus(false) })
		}
		call(h.blur)
	case driver.EventChangeText:
		in := n.input
		if in == nil {
			return
		}
		if t.record != nil && ev.Text != in.value {
			t.devFlushScrolls()
			t.devRecordEdit(n, in.value, ev.Text)
		}
		in.shown, in.value, in.count = ev.Text, ev.Text, ev.Count
		if s := in.suggest; s != nil {
			// The app's OnOpen and OnClose run in it.
			t.within(n.owner, func() { s.changed(ev.Text) })
		}
		if h.change != nil {
			t.within(n.owner, func() { h.change(ev.Text) })
		}
		// A controlled field the app did not follow goes back to its value.
		if in.valueFn != nil {
			if v := Untracked(t.root, in.valueFn); v != in.value {
				in.value = v
				if s := in.suggest; s != nil {
					t.within(n.owner, func() { s.sync(v) })
				}
			}
		}
		t.markResolve(n)
	case driver.EventSubmitEditing:
		if h.submit != nil {
			t.within(n.owner, func() { h.submit(ev.Text) })
		}
	case driver.EventSelectionChange:
		if in := n.input; in != nil {
			r := TextRange{Start: ev.Index, End: int(ev.Count)}
			if s := in.suggest; s != nil {
				t.within(n.owner, func() { s.selectionChanged(r) })
			}
			if in.selection != nil {
				t.within(n.owner, func() { in.selection(r) })
			}
		}
		if h.textSelection != nil {
			var ranges []SelectedText
			if ev.Ranges != nil {
				ranges = make([]SelectedText, len(ev.Ranges))
				for i, r := range ev.Ranges {
					ranges[i] = SelectedText{Start: r.Start, End: r.End}
					if n := t.node(r.ID); n != nil {
						ranges[i].Ref = n.ref
					}
				}
			}
			md, htmlOut := t.formatTextSelection(n, ev.Text, ev.Ranges)
			t.within(n.owner, func() {
				h.textSelection(TextSelection{
					Text:     ev.Text,
					Markdown: md,
					HTML:     htmlOut,
					Bounds:   Bounds{Origin: ev.Point, Size: ev.Size},
					Ranges:   ranges,
				})
			})
		}
	case driver.EventKeyPress:
		if in := n.input; in != nil && in.keyPress != nil {
			t.within(n.owner, func() { in.keyPress(ev.Text) })
		}
	case driver.EventCompositionChange:
		if in := n.input; in != nil && in.composition != nil {
			c := Composition{Composing: !ev.Finished, Text: ev.Text}
			if ev.Finished {
				c.Text = ""
			}
			t.within(n.owner, func() {
				if s := in.suggest; s != nil {
					s.compose(c.Composing)
				}
				in.composition(c)
			})
		}
	case driver.EventCaretMove:
		if in := n.input; in != nil && in.caret != nil {
			b := Bounds{Origin: ev.Point, Size: ev.Size}
			page := b
			o := t.surfaceBounds(n).Origin
			page.Origin = Point{X: o.X + b.Origin.X, Y: o.Y + b.Origin.Y}
			e := CaretEvent{Bounds: b, PageBounds: page}
			t.within(n.owner, func() {
				if s := in.suggest; s != nil {
					s.caretMoved(e)
				}
				in.caret(e)
			})
		}
	case driver.EventPaste:
		if in := n.input; in != nil && in.paste != nil {
			if p, ok := ev.Data.(*PasteEvent); ok {
				t.within(n.owner, func() { in.paste(*p) })
			}
		}
	case driver.EventEditAction:
		if in := n.input; in != nil && in.editAction != nil {
			t.activated = n.owner
			fn := in.editAction
			act := EditAction(ev.Count)
			t.within(n.owner, func() { fn(act) })
		}
	case driver.EventPan:
		if t.devDrawn != nil {
			t.devDrawnGesture(ev)
		}
		if ev.Phase == GestureBegan {
			t.panActivated(n)
		}
		if h.pan != nil {
			if ev.Phase == GestureBegan {
				t.setState(n, statePressed, false) // the pan took the press
			}
			t.within(n.owner, func() {
				h.pan(PanEvent{Phase: ev.Phase, Translation: ev.Point, Velocity: ev.Velocity, Position: ev.Position, PagePosition: ev.PagePosition, Values: ev.Values})
			})
		}
	case driver.EventPinch, driver.EventRotation:
		if t.devDrawn != nil {
			t.devDrawnGesture(ev)
		}
		fn := h.pinch
		if ev.Kind == driver.EventRotation {
			fn = h.rotation
		}
		if fn != nil {
			t.within(n.owner, func() {
				fn(GestureEvent{Phase: ev.Phase, Value: ev.Value, Velocity: float32(ev.Velocity.X), Position: ev.Point})
			})
		}
	case driver.EventWheel:
		if h.wheel != nil {
			t.within(n.owner, func() {
				h.wheel(WheelEvent{Position: ev.Point, Delta: ev.Delta, Modifiers: ev.Modifiers, Touchpad: ev.Precise,
					Phase: ev.Phase, Velocity: ev.Velocity})
			})
		}
	case driver.EventContextMenu:
		if h.contextMenu != nil && !n.labelProps().Disabled {
			t.within(n.owner, func() { h.contextMenu(ContextMenuEvent{ev.Point, ev.Page, ev.Modifiers}) })
		}
	case driver.EventPressOutside:
		if n.hoist != nil {
			t.pressOutside(n, ev.Point)
		}
	case driver.EventGPU:
		if n.gpu != nil && n.gpu.onEvent != nil && !n.gpu.surf.disposed {
			t.within(n.owner, func() { n.gpu.onEvent(ev.Data) })
		}
	case driver.EventFileDragEnter:
		call(h.fileEnter)
	case driver.EventFileDragLeave:
		call(h.fileLeave)
	case driver.EventFileDrop:
		if h.fileDrop != nil && !n.labelProps().Disabled {
			t.within(n.owner, func() { h.fileDrop(ev.URIs) })
		}
	case driver.EventLoadStart:
		call(h.loadStart)
	case driver.EventLoadEnd:
		call(h.loadEnd)
	case driver.EventProgress:
		if h.progress != nil {
			t.within(n.owner, func() { h.progress(ev.Loaded, ev.Total) })
		}
	case driver.EventLoad:
		if h.load != nil {
			t.within(n.owner, func() { h.load(ev.Size) })
		}
	case driver.EventError:
		if h.loadError != nil {
			t.within(n.owner, func() { h.loadError(errors.New(ev.Text)) })
		}
	case driver.EventScrollDragStart, driver.EventScrollDragEnd, driver.EventMomentumStart, driver.EventMomentumEnd,
		driver.EventScrollEnd, driver.EventRefresh:
		if t.record != nil && ev.Kind == driver.EventScrollEnd {
			t.devFlushScrolls()
		}
		if n.scroll != nil {
			t.scrolled(n, ev)
			if t.devDrawn != nil && ev.Kind == driver.EventScrollEnd {
				t.devDrawnScrollEnd(n)
			}
			t.dispatchScroll(n, ev)
		}
	case driver.EventScroll:
		if t.record != nil && n.scroll != nil {
			t.devRecordScroll(n, ev.Point)
		}
		if n.scroll != nil {
			t.scrolled(n, ev)
			if ev.Value > 0 {
				n.scroll.zoom = ev.Value
			}
			if t.devDrawn != nil {
				t.devDrawnScroll(n, ev.Point)
			}
			t.followAnchoredOverlays()
			if t.auditRead != nil && t.auditRead() {
				t.framesMoved, t.changed = true, true
			}
			if len(t.watches) > 0 {
				t.changed = true
			}
		} else if in := n.input; in != nil && in.config.Multiline {
			in.scroll.report(ev.Point, ev.Count)
			if in.value != "" && ev.Size.Height > 0 {
				in.platformScroll = ev.Size
			}
			if len(t.watches) > 0 {
				t.changed = true
			}
			if in.onScroll != nil {
				e := t.inputScrollEvent(n, ev.Point)
				t.within(n.owner, func() { in.onScroll(e) })
			}
		}
		if h.scroll != nil {
			if n.scroll != nil {
				t.reportScroll(n, ev.Point)
			} else {
				t.within(n.owner, func() { h.scroll(ev.Point) })
			}
		}
	}
}

// spanAt returns the span holding byte offset i of the text.
// listenForSpan has text n listen to the presses span sp takes, which
// come with the offset of the span.
func (t *Tree) listenForSpan(n *node, sp Span) {
	if sp.OnPress != nil && n.h.press == nil {
		n.ownHandlers().press = func(PressEvent) {}
	}
	if sp.OnLongPress != nil && n.h.longPress == nil {
		n.ownHandlers().longPress = func(PressEvent) {}
	}
}

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

// KeyDown dispatches EventKeyDown ev to its view's OnKeyDown, as Dispatch does, and reports whether the handler consumed the key, for the platform that waits to know whether the keymap and the view still take it (Engine.DispatchKeyDown).
func (t *Tree) KeyDown(ev driver.Event) bool {
	if f := t.follower; f != nil {
		f.Event(ev)
	}
	return t.keyDown(ev)
}

func (t *Tree) keyDown(ev driver.Event) bool {
	if !ev.Repeat {
		t.holdKey(ev.Text, ev.KeyCode, 1)
	}
	t.observeKeyCode(ev.KeyCode, ev.Text)
	n := t.node(ev.ID)
	if n == nil || n.h.keyDown == nil {
		return false
	}
	fn, e := n.h.keyDown, KeyEvent{Key: ev.Text, Code: ev.KeyCode, Modifiers: ev.Modifiers, Repeat: ev.Repeat}
	return t.within2(n, func() bool { return fn(e) })
}

// keyUp counts EventKeyUp ev's key as up and runs OnKeyUp of the view its down went to, a view gone or ID 0 hearing nothing.
func (t *Tree) keyUp(ev driver.Event) {
	t.holdKey(ev.Text, ev.KeyCode, -1)
	n := t.node(ev.ID)
	if n == nil || n.h.keyUp == nil {
		return
	}
	fn, e := n.h.keyUp, KeyEvent{Key: ev.Text, Code: ev.KeyCode, Modifiers: ev.Modifiers}
	t.within(n.owner, func() { fn(e) })
}

// mouseButton records ev's button going down or coming up and runs OnMouseDown or OnMouseUp of view ev.ID, a view gone or ID 0 hearing nothing.
func (t *Tree) mouseButton(ev driver.Event, down bool) {
	t.holdMouseButton(ev.Button, down)
	n := t.node(ev.ID)
	if n == nil {
		return
	}
	t.activated = n.owner
	fn := n.h.mouseUp
	if down {
		fn = n.h.mouseDown
	}
	if fn == nil {
		return
	}
	e := mouseButtonOf(ev, down)
	t.within(n.owner, func() { fn(e) })
}

// PressAt dispatches ev as a press on the innermost pressable view at its
// Page, in surface points, with its Point in that view.
func (t *Tree) PressAt(ev driver.Event) {
	if id, local := hit(t.view, ev.Page); id != 0 {
		ev.Kind, ev.ID, ev.Point = driver.EventPress, id, local
		t.Dispatch(ev)
	}
}

func hit(n *node, p Point) (NodeID, Point) {
	return hitWith(n, p, func(n *node) bool { return n.h.press != nil || n.kind == driver.KindInput })
}

// hitWith returns the innermost view at p in n's parent that ok accepts,
// and p in it.
func hitWith(n *node, p Point, ok func(*node) bool) (NodeID, Point) {
	local, mapped := driver.IntoView(n.frame, n.pointerMatrix(), p)
	if !mapped {
		return 0, Point{}
	}
	self, inside := driver.ReachView(n.frame.Size, n.props, n.sent.overflow, n.sent.transformed, local)
	if n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden || n.hidden || !self && !inside ||
		(n.attach != nil && n.attach.cut) {
		return 0, Point{}
	}
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
func (t *Tree) command(kind driver.ItemKind, n *node) {
	t.commands = append(t.commands, driver.MountItem{Kind: kind, ID: n.id})
	t.changed = true
}

// outOfFlow drops what n's parent cached of its native children while n was made, before it was marked as an attachment or a header view: styling it, as peer classes and its parent's spacing do, may already have read them.
func outOfFlow(n *node) {
	if p := n.nativeParent(); p != nil {
		p.flat = nil
	}
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
		case (c.nodeExtra == &zeroExtra || (c.hoist == nil && c.attach == nil && c.accessory == nil && c.pip == nil)) && !c.headerView && !c.inputBar:
			out = append(out, c)
		}
	}
	return out
}

// nativeChildren returns n's native children: the views it lays out, a
// text's attachments, a multiline input's scrollbar host, then the overlays and modals it hosts, last on top.
func (n *node) nativeChildren() []*node {
	attached := n.text != nil && n.text.attachments > 0
	inputBar := n.input != nil && n.input.barHost != nil && !n.input.barHost.deleted
	flat := stacked(n.flatChildren())
	if len(n.hosted) == 0 && !attached && !inputBar {
		return flat
	}
	out := append([]*node(nil), flat...)
	if attached {
		for _, sp := range n.text.spans {
			if sp.attach != nil {
				out = append(out, sp.attach)
			}
		}
	}
	if inputBar {
		out = append(out, n.input.barHost)
	}
	for _, h := range n.hosted {
		if !h.deleted {
			out = append(out, h)
		}
	}
	return out
}

// stackRank orders a view among its siblings: by ZIndex, a sticky view
// above the others of its ZIndex.
func (n *node) stackRank() (int, bool) {
	return n.style.ZIndex, n.style.Position == Sticky
}

// stacked returns kids in the order they stack, lowest first: by their
// stack ranks, the order they came in among equals. It returns kids
// itself when that is their order.
func stacked(kids []*node) []*node {
	sorted := true
	for i := 1; i < len(kids) && sorted; i++ {
		a, as := kids[i-1].stackRank()
		b, bs := kids[i].stackRank()
		sorted = a < b || a == b && (!as || bs)
	}
	if sorted {
		return kids
	}
	out := slices.Clone(kids)
	slices.SortStableFunc(out, func(x, y *node) int {
		a, as := x.stackRank()
		b, bs := y.stackRank()
		if a != b {
			return cmp.Compare(a, b)
		}
		switch {
		case as == bs:
			return 0
		case as:
			return 1
		}
		return -1
	})
	return out
}

// takesPresses reports whether n's handlers hear presses.
func (n *node) takesPresses() bool {
	h := n.h
	return h.press != nil || h.longPress != nil || h.pressDown != nil || h.pressUp != nil || n.labelTarget() != nil
}

func (n *node) events() driver.EventMask {
	h, s := n.h, n.style
	if h == &noHandlers && n.cls == nil && n.nodeExtra == &zeroExtra &&
		!n.panning && !n.pinching && !n.rotating && !n.props.Focusable &&
		n.kind != driver.KindInput && n.kind != driver.KindWeb &&
		s.Group == "" && s.Hover == nil && s.Focus == nil && s.FocusVisible == nil && s.Active == nil {
		return 0
	}
	var m driver.EventMask
	bit := func(k driver.EventKind, on bool) {
		if on {
			m |= 1 << k
		}
	}
	// Groups, peers and relational selectors report the states other views follow. own is what n's classes read of n's states after matching; inherited is what selectors of views around n may read of it.
	var own, inherited uint16
	if c := n.cls; c != nil && c.related {
		own = c.list.watch
		inherited = c.watch &^ own
	}
	group := s.Group != ""
	peer := n.peerMarked()
	// Style tracking must not turn a plain view into a press target or Tab stop. Only a view with press handlers requests presses for its own, group or peer active styles, or for the pressed state a relational selector reads of it.
	watched := own | inherited
	pressStyled := (s.Active != nil || group || peer || watched&classPressed != 0) && n.takesPresses()
	bit(driver.EventPress, h.press != nil || n.labelTarget() != nil)
	bit(driver.EventLongPress, h.longPress != nil)
	// A press hides the tooltip of a view that takes presses.
	bit(driver.EventPressDown, h.pressDown != nil || pressStyled || n.tip != nil && h.press != nil)
	bit(driver.EventPressUp, h.pressUp != nil || pressStyled)
	bit(driver.EventMouseEnter, h.mouseEnter != nil || s.Hover != nil || group || peer || n.tip != nil || watched&classHovered != 0)
	bit(driver.EventMouseLeave, h.mouseLeave != nil || s.Hover != nil || group || peer || n.tip != nil || watched&classHovered != 0)
	bit(driver.EventMouseMove, h.mouseMove != nil)
	// One bit says the view hears mouse buttons, going down and up: EventMouseUp has none.
	bit(driver.EventMouseDown, h.mouseDown != nil || h.mouseUp != nil)
	bit(driver.EventTouch, n.hearsTouches())
	// The focus of any view that can take it is reported, for the core's
	// own focus (focusReported).
	focusable := n.kind == driver.KindInput || n.kind == driver.KindWeb || n.takesPresses() || n.props.Focusable
	focusStyled := s.Focus != nil || s.FocusVisible != nil || peer || watched&(classFocused|classFocusVisible|classFocusWithin) != 0
	bit(driver.EventFocus, h.focus != nil || focusStyled || focusable)
	bit(driver.EventBlur, h.blur != nil || focusStyled || focusable)
	// One bit says the view hears keys, going down and up: EventKeyUp has none.
	bit(driver.EventKeyDown, h.keyDown != nil || h.keyUp != nil)
	bit(driver.EventSubmitEditing, h.submit != nil)
	bit(driver.EventScroll, h.scroll != nil)
	bit(driver.EventPressOutside, h.pressOutside != nil)
	bit(driver.EventContextMenu, h.contextMenu != nil)
	bit(driver.EventLoadStart, h.loadStart != nil)
	bit(driver.EventProgress, h.progress != nil)
	bit(driver.EventLoad, h.load != nil)
	bit(driver.EventError, h.loadError != nil)
	bit(driver.EventLoadEnd, h.loadEnd != nil)
	bit(driver.EventChangeText, n.kind == driver.KindInput)
	bit(driver.EventSelectionChange, h.textSelection != nil)
	if in := n.input; in != nil {
		bit(driver.EventScroll, in.config.Multiline)
		bit(driver.EventSelectionChange, in.selection != nil)
		bit(driver.EventKeyPress, in.keyPress != nil)
		bit(driver.EventCompositionChange, in.composition != nil)
		bit(driver.EventCaretMove, in.caret != nil)
		bit(driver.EventPaste, in.paste != nil)
	}
	bit(driver.EventPan, n.panning)
	bit(driver.EventPinch, n.pinching)
	bit(driver.EventRotation, n.rotating)
	bit(driver.EventWheel, h.wheel != nil)
	bit(driver.EventFileDragEnter, h.fileEnter != nil)
	bit(driver.EventFileDragLeave, h.fileLeave != nil)
	bit(driver.EventFileDrop, h.fileDrop != nil)
	bit(driver.EventAccessibilityAction, h.action != nil || h.canvasAction != nil)
	if n.native != nil {
		bit(driver.EventNative, n.native.event != nil)
		bit(driver.EventNativeSize, true)
		bit(driver.EventError, n.native.onError != nil)
	}
	if n.shader != nil {
		bit(driver.EventError, n.shader.onError != nil)
	}
	if n.web != nil {
		m |= n.web.events()
	}
	if n.pip != nil {
		bit(driver.EventNative, true)
	}
	if n.video != nil {
		m &^= maskOf(driver.EventLoadStart, driver.EventProgress, driver.EventLoad, driver.EventError)
		m |= n.video.events()
	}
	if n.scroll != nil {
		m |= n.scroll.events()
	}
	return m
}

func maskOf(kinds ...driver.EventKind) driver.EventMask {
	var m driver.EventMask
	for _, k := range kinds {
		m |= 1 << k
	}
	return m
}
