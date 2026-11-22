package core

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/layout"
)

// A stack shows its last screen that has not been popped. Back pops the
// top screen: it reports OnDismiss and stays hidden until the app removes
// it, and the last screen never pops.
//
// Where the platform reports header heights (iOS, Android), a screen has
// the system header, as NSH's do: the platform draws it over the top of
// the screen's view from ItemScreen, the core pads the content below it,
// and the header views are layout roots of their own that the platform
// puts in the bar. HeaderLeft and HeaderRight take their content's size;
// HeaderTitle fills the room between the bar's ends when its views grow
// or ask for a share of the width, and otherwise takes its content's
// size bounded by that room. Elsewhere the screen's Header is its first
// child.

type screenNode struct {
	screen *Screen
	stack  *node // the stack it is in, if any
	popped bool
	hidden bool
	// peek lays the screen out under the top one while a back gesture
	// may uncover it (EventBackStarted, until it pops or is cancelled).
	peek bool

	// native is set where the platform draws the header.
	native                      bool
	title                       string
	headerStyle                 Style
	headerClass                 Style  // HeaderClass's, under headerStyle
	symbols                     string // HeaderRightSymbols's, as ScreenConfig has them
	left, right, view           *node  // the header views
	bar                         *node  // Header, unused under a native header
	titleStyle, largeTitleStyle Style

	// header is the area the system header covers from the top; pad what
	// the content is laid out below.
	header, pad Points
	top         *Signal[Points] // header, for a transparent header's insets
	// reported are the insets the platform reported, nil before it does.
	reported *Signal[*Edges]
	// measured is the bar height the platform measured, 0 before it does.
	measured Points
	// titleWidth is the room between the bar's ends the platform measured
	// for HeaderTitle, 0 before it does, and titleScreenWidth is the screen
	// width when it was recorded.
	titleWidth       Points
	titleScreenWidth Points
	// titleLaidWidth and titleFills are the width and fill mode HeaderTitle
	// was last laid out with.
	titleLaidWidth Points
	titleFills     bool
	sent           driver.ScreenConfig
	sentOK         bool
}

func (t *Tree) mountStack(cx *Scope, parent *node, s Stack) {
	n := t.newNode(driver.KindView, parent, cx)
	t.bindStyle(cx, n, Style{Overflow: Hidden}.Merge(s.Style), nil)
	n.props = driver.ViewProps{TestID: s.TestID}
	n.isStack = true
	t.stacks = append(t.stacks, n)
	for _, c := range s.Children {
		t.mount(cx, n, c)
	}
}

var fill = Style{Position: Absolute, Top: Pt(0), Right: Pt(0), Bottom: Pt(0), Left: Pt(0)}

func (t *Tree) mountScreen(cx *Scope, parent *node, s Screen) {
	n := t.newNode(driver.KindView, parent, cx)
	// iOS and Android draw the header, not a watch or a web build that runs as them; elsewhere a platform does where it reports a header's height.
	env := t.root.Env()
	sn := &screenNode{screen: &s, title: s.Title, headerStyle: s.HeaderStyle,
		titleStyle: s.HeaderTitleStyle, largeTitleStyle: s.HeaderLargeTitleStyle,
		native: env.headers[0] > 0 || Untracked(cx, env.mobileBars), top: NewSignal(cx, Points(0)),
		reported: NewSignal[*Edges](cx, nil)}
	for p := parent; p != nil; p = p.parent {
		if p.isStack {
			sn.stack = p
			break
		}
	}
	n.screen = sn
	t.setClasses(n, s.Class)
	t.bindStyle(cx, n, fill.Merge(s.Style), s.StyleFunc)
	n.props = driver.ViewProps{Label: s.Title, TestID: s.TestID}
	if s.TitleFunc != nil {
		cx.renderEffect(func() {
			sn.title = s.TitleFunc()
			n.props.Label = sn.title
			t.touch(n)
		})
	}
	if s.NoPopOnBackFunc != nil || s.NoPopOnSwipeFunc != nil {
		cx.renderEffect(func() {
			if s.NoPopOnBackFunc != nil {
				sn.screen.NoPopOnBack = s.NoPopOnBackFunc()
			}
			if s.NoPopOnSwipeFunc != nil {
				sn.screen.NoPopOnSwipe = s.NoPopOnSwipeFunc()
			}
			t.touch(n)
		})
	}
	if s.HeaderStyleFunc != nil {
		cx.renderEffect(func() {
			sn.headerStyle = s.HeaderStyle.Merge(s.HeaderStyleFunc())
			t.touch(n)
		})
	}
	if s.HeaderTitleStyleFunc != nil || s.HeaderLargeTitleStyleFunc != nil {
		cx.renderEffect(func() {
			if s.HeaderTitleStyleFunc != nil {
				sn.titleStyle = s.HeaderTitleStyle.Merge(s.HeaderTitleStyleFunc())
			}
			if s.HeaderLargeTitleStyleFunc != nil {
				sn.largeTitleStyle = s.HeaderLargeTitleStyle.Merge(s.HeaderLargeTitleStyleFunc())
			}
			t.touch(n)
		})
	}
	if f := s.IOS.HeaderRightSymbols; f != nil {
		cx.renderEffect(func() {
			var lines []string
			for _, b := range f() {
				lines = append(lines, b.Name+"\t"+b.Label)
			}
			sn.symbols = strings.Join(lines, "\n")
			t.touch(n)
		})
	}
	if s.HeaderClass != "" {
		cx.renderEffect(func() {
			env := t.classEnv(n)
			l := t.rt.parseClasses(&env, s.HeaderClass)
			t.rt.reportClassesWithoutPeers(l, "HeaderClass")
			normal, important, _ := l.resolve(&env)
			sn.headerClass = mergeStyles(normal, important)
			t.touch(n)
		})
	}
	t.bindStatusBar(cx, n, s.StatusBar, s.StatusBarFunc)
	header := false
	if sn.native {
		sn.left = t.mountHeaderView(cx, n, s.HeaderLeft)
		sn.right = t.mountHeaderView(cx, n, s.HeaderRight)
		sn.view = t.mountHeaderView(cx, n, s.HeaderTitle)
		// The header a platform without a native one shows, made as NSH
		// makes it, which the system header leaves unused.
		sn.bar = t.mountHeaderView(cx, n, s.Header)
	} else if s.Header != nil {
		header = true
		t.mount(cx, n, s.Header)
	}
	// Content below a header keeps clear of the sides and bottom of what the screen's place keeps clear of (Insets), such as a tab section's that leaves out the bottom; under a transparent header, of the header too.
	inner := cx.Child()
	outer := InsetsFunc(cx)
	insets := func() Edges {
		if r := sn.reported.Get(); r != nil {
			return *r
		}
		in := outer()
		switch {
		case header:
			in.Top = 0
		case sn.native && !s.HeaderHidden:
			in.Top = 0
			if s.HeaderTransparent {
				in.Top = sn.top.Get()
			}
		}
		return in
	}
	// Provided while inner runs, so it is inner's own.
	t.within(inner, func() {
		insetsKey.Provide(inner, insets)
		for _, c := range s.Children {
			t.mount(inner, n, c)
		}
	})
}

// mountHeaderView builds a header view where it is written, under screen
// n, as a layout root that no view holds.
func (t *Tree) mountHeaderView(cx *Scope, screen *node, el Node) *node {
	if el == nil {
		return nil
	}
	n := t.newNode(driver.KindView, screen, cx)
	n.headerView = true
	outOfFlow(n)
	t.headerViews = append(t.headerViews, n)
	t.mount(cx, n, el)
	return n
}

// layoutHeaderViews lays each header view out: HeaderLeft and HeaderRight
// at their content size, and HeaderTitle in the room between the bar's
// ends, filling it when its views grow or take a percentage width.
func (t *Tree) layoutHeaderViews() {
	live := t.headerViews[:0]
	for _, n := range t.headerViews {
		if n.deleted {
			continue
		}
		live = append(live, n)
		space := Size{Width: attachmentSpace, Height: attachmentSpace}
		if n.parent != nil && n.parent.screen != nil && n.parent.screen.view == n {
			sn := n.parent.screen
			fills := headerTitleFills(n)
			room := t.titleRoom(n.parent, fills)
			if fills != sn.titleFills {
				sn.titleFills = fills
				n.lstyleOK = false
				t.markLayout(n)
				t.mark(n.parent)
			}
			if room != sn.titleLaidWidth {
				sn.titleLaidWidth = room
				n.lstyleOK = false
				t.markLayout(n)
			}
			if room > 0 {
				space.Width = room
			}
		}
		if !n.layoutDirty {
			continue
		}
		t.layoutRoot(n, space, false)
		l := &n.lstate.Rounded
		t.setFrame(n, Bounds{Size: Size{Width: Points(l.Size.Width), Height: Points(l.Size.Height)}})
		t.syncFrames(n)
	}
	clear(t.headerViews[len(live):])
	t.headerViews = live
}

// titleRoom is the horizontal room available to screen's HeaderTitle: the
// width the platform measured between the bar's ends, or an estimate from
// the screen's width less HeaderLeft and HeaderRight before it reports.
func (t *Tree) titleRoom(screen *node, fills bool) Points {
	sn := screen.screen
	sw := screen.frame.Size.Width
	if !fills && sn.titleWidth > 0 && sn.titleScreenWidth > 0 && sw > 0 && sw != sn.titleScreenWidth {
		// Drop a prior compression clamp when the screen resized so a
		// non-filling title can expand again on wider layouts.
		sn.titleWidth = 0
		sn.titleScreenWidth = 0
	}
	if sn.titleWidth > 0 {
		return sn.titleWidth
	}
	w := sw
	if w <= 0 {
		w = t.size.Width
	}
	if sn.left != nil && !sn.left.deleted {
		w -= sn.left.frame.Size.Width
	}
	if sn.right != nil && !sn.right.deleted {
		w -= sn.right.frame.Size.Width
	}
	return max(w, 0)
}

// headerTitleFills reports whether HeaderTitle root holds a view that
// expands across the room between the system header's ends: a direct child
// with FlexGrow, a horizontal flex container with growing children, auto
// horizontal margins or space distribution, a percentage width or
// min-width, or a grid with flexible or percentage columns, outside any
// ancestor with a fixed width of its own.
func headerTitleFills(root *node) bool {
	for _, c := range root.flatChildren() {
		if c.deleted || c.style.Display == DisplayNone || c.style.Position == Absolute || c.style.Position == Fixed {
			continue
		}
		if c.style.FlexGrow > 0 || nodeFillsWidth(c) {
			return true
		}
	}
	return false
}

func nodeFillsWidth(n *node) bool {
	s := &n.style
	if s.Display == DisplayNone || s.Position == Absolute || s.Position == Fixed {
		return false
	}
	if _, ok := s.Width.Percent(); ok {
		return true
	}
	if _, ok := s.MinWidth.Percent(); ok {
		return true
	}
	if !s.Width.IsAuto() {
		return false
	}
	if s.Display == DisplayGrid && gridFillsWidth(n) {
		return true
	}
	isRow := s.Display != DisplayBlock && s.Display != DisplayGrid &&
		(s.Direction == Row || s.Direction == RowReverse || (s.Direction == 0 && s.RowByDefault))
	if isRow {
		switch s.JustifyContent {
		case JustifyEnd, JustifySpaceBetween, JustifySpaceAround, JustifySpaceEvenly, JustifyStretch:
			return true
		}
	}
	for _, c := range n.flatChildren() {
		if c.deleted || c.style.Display == DisplayNone || c.style.Position == Absolute || c.style.Position == Fixed {
			continue
		}
		if isRow {
			if c.style.FlexGrow > 0 || c.style.AutoMargin&(SideLeft|SideRight) != 0 {
				return true
			}
			if _, ok := c.style.FlexBasis.Percent(); ok {
				return true
			}
		}
		if nodeFillsWidth(c) {
			return true
		}
	}
	return false
}

func gridFillsWidth(n *node) bool {
	trackFills := func(ts layout.TrackSizing) bool {
		return ts.Max.Unit == layout.UnitFr || ts.Max.Unit == layout.UnitPercent ||
			ts.Max.Unit == layout.UnitFitContentPercent || ts.Min.Unit == layout.UnitPercent
	}
	for _, comp := range n.Style().GridTemplateColumns {
		if comp.Repeat != nil {
			if comp.Repeat.Count == layout.AutoFill || comp.Repeat.Count == layout.AutoFit {
				return true
			}
			for _, ts := range comp.Repeat.Tracks {
				if trackFills(ts) {
					return true
				}
			}
			continue
		}
		if trackFills(comp.Track) {
			return true
		}
	}
	return false
}

// emitHeaderViews sends the header views before the tree's, so the
// screens naming them find them.
func (t *Tree) emitHeaderViews() {
	for _, n := range t.headerViews {
		if !n.deleted {
			t.emit(n)
		}
	}
}

// sizeHeaders sets the area each screen's system header covers: the bar,
// and the part of the status bar the screen reaches under, as NSH's
// top_overlap. It reports whether a screen's content moved.
func (t *Tree) sizeHeaders() bool {
	changed := false
	heights := t.root.Env().headers
	for _, s := range t.stacks {
		for _, n := range s.flatChildren() {
			sn := n.screen
			if sn == nil || !sn.native {
				continue
			}
			sc := sn.screen
			var header, pad Points
			if !sc.HeaderHidden {
				bar := heights[0]
				if sc.HeaderLargeTitle {
					bar = heights[1]
				}
				if sn.measured > 0 {
					bar = sn.measured
				}
				header = bar + t.topOverlap(n)
				// Content goes under a transparent header, and under a
				// large title that collapses as it scrolls; the scroll
				// views at the top are inset by it instead.
				if !sc.HeaderTransparent && !sc.HeaderLargeTitle {
					pad = header
				}
			}
			if header == sn.header && pad == sn.pad {
				continue
			}
			sn.header = header
			if pad != sn.pad {
				sn.pad = pad
				n.lstyleOK = false
				t.markLayout(n)
				changed = true
			}
			t.within(n.owner, func() { sn.top.Set(header) })
			t.mark(n)
		}
	}
	return changed
}

// headerInset is how far the system header of the screen around scroll
// view n covers its top, where the header lays the content under it.
func (t *Tree) headerInset(n *node) Points {
	if n.scroll == nil || n.scroll.config.Horizontal {
		return 0
	}
	var screen *node
	for a := n.parent; a != nil && a.hoist == nil; a = a.parent {
		if a.screen != nil {
			screen = a
			break
		}
	}
	if screen == nil || !screen.screen.native || screen.screen.pad != 0 || screen.screen.header <= 0 {
		return 0
	}
	top := t.surfaceBounds(n).Origin.Y - t.surfaceBounds(screen).Origin.Y
	return max(screen.screen.header-top, 0)
}

// screenHeader records the bar height and available HeaderTitle width
// the platform measured for screen n.
func (t *Tree) screenHeader(n *node, h, titleWidth Points) {
	sn := n.screen
	if sn == nil {
		return
	}
	if h > 0 && sn.measured != h {
		sn.measured = h
		t.markLayout(t.view)
		t.mark(n)
	}
	if titleWidth > 0 && sn.titleWidth != titleWidth {
		sn.titleWidth = titleWidth
		sn.titleScreenWidth = n.frame.Size.Width
		if sn.view != nil && !sn.view.deleted {
			t.markLayout(sn.view)
			t.mark(sn.view)
		}
	}
}

// topOverlap is how far into n the top inset of its host reaches.
func (t *Tree) topOverlap(n *node) Points {
	host := t.modalHost(n.parent)
	top := t.root.Env().safe.GetUntracked().Top
	y := t.surfaceBounds(n).Origin.Y
	if host.kind == driver.KindModal {
		top = t.modalInsets(host).Top
		y -= t.surfaceBounds(host).Origin.Y
	}
	return max(top-y, 0)
}

// screenConfig is what screen n tells the platform of its system header.
func (t *Tree) screenConfig(n *node) driver.ScreenConfig {
	sn := n.screen
	s := sn.screen
	c := driver.ScreenConfig{
		Title:             sn.title,
		HeaderHidden:      s.HeaderHidden,
		HeaderTransparent: s.HeaderTransparent,
		LargeTitle:        s.HeaderLargeTitle,
		BackHidden:        s.HeaderBackHidden,
		OwnBackTitle:      s.HeaderBackTitle,
		NoPopOnSwipe:      s.NoPopOnSwipe,
		NoPopOnBack:       s.NoPopOnBack,
		Background:        cmpOr(sn.headerStyle.Background, sn.headerClass.Background),
		TitleColor:        cmpOr(sn.headerStyle.Color, sn.headerClass.Color),
		TintColor:         cmpOr(sn.headerStyle.TintColor, sn.headerClass.TintColor),
		Animation:         s.Animation,
		Left:              headerID(sn.left),
		Right:             headerID(sn.right),
		TitleView:         headerID(sn.view),
		TitleFills:        sn.titleFills,
		Header:            headerID(sn.bar),
		HeaderHeight:      sn.header,

		TitleFont:           headerFont(sn.titleStyle),
		LargeTitleFont:      headerFont(sn.largeTitleStyle),
		RightWithLargeTitle: s.Android.HeaderRightWithLargeTitle,
	}
	if c.Right != 0 {
		c.RightSystemItem = s.IOS.HeaderRightSystemItem
		c.RightSymbols = sn.symbols
		if sn.symbols == "" && s.IOS.HeaderRightSymbols != nil {
			c.Right = 0
		}
	}
	if sn.stack != nil {
		var below *node
		for _, o := range sn.stack.screens() {
			if o == n {
				break
			}
			below = o
		}
		if below != nil {
			c.CanGoBack = true
			c.BackTitle = below.screen.screen.HeaderBackTitle
			if c.BackTitle == "" {
				c.BackTitle = below.screen.title
			}
		}
	}
	return c
}

// emitScreen sends screen n's header config when it changed.
func (t *Tree) emitScreen(n *node) {
	sn, c := n.screen, t.screenConfig(n)
	if sn.sentOK && c == sn.sent {
		return
	}
	sn.sent, sn.sentOK = c, true
	t.items = append(t.items, driver.MountItem{Kind: driver.ItemScreen, ID: n.id, Screen: &c})
}

func headerID(n *node) NodeID {
	if n == nil || n.deleted {
		return 0
	}
	return n.id
}

// topScreen is stack n's top screen that has not been popped.
func (t *Tree) topScreen(n *node) *node {
	var top *node
	for _, c := range n.flatChildren() {
		if c.screen != nil && !c.screen.popped && !c.deleted {
			top = c
		}
	}
	return top
}

// showTop hides every screen of stack n but its top one.
func (t *Tree) showTop(n *node) {
	var top *node
	for _, c := range n.flatChildren() {
		if c.screen != nil && !c.screen.popped {
			top = c
		}
	}
	for _, c := range n.flatChildren() {
		if c.screen == nil {
			continue
		}
		if hidden := c != top && !c.screen.peek; hidden != c.screen.hidden {
			c.screen.hidden = hidden
			if t.restyle(c) {
				t.markLayout(c)
			}
		}
	}
}

// historyDepth counts shown screens above each stack's first and modals.
func (t *Tree) historyDepth() int {
	depth := 0
	for _, s := range t.stacks {
		if !s.hidden {
			depth += max(len(s.screens())-1, 0)
		}
	}
	var modals func(host *node)
	modals = func(host *node) {
		for _, n := range host.hosted {
			if !n.deleted && n.kind == driver.KindModal && !n.hoist.dismissed && !n.hidden {
				depth++
				modals(n)
			}
		}
	}
	modals(t.view)
	return depth
}

// emitHistory commits navigation depth and the application address together.
func (t *Tree) emitHistory() {
	depth := t.historyDepth()
	a := t.rt.address
	addressed := a != nil && a.tree == t && a.page != ""
	if depth == t.history && !t.historyAgain && (!addressed || a.pending == nil) {
		return
	}
	t.history, t.historyAgain = depth, false
	item := driver.MountItem{Kind: driver.ItemHistory, Count: uint32(depth)}
	if addressed {
		item.Address = a.pending
		if item.Address == nil {
			item.Address = &driver.PageAddress{URL: a.value.GetUntracked()}
		}
		a.pending = nil
	}
	t.items = append(t.items, item)
}

// markScreens marks the screens of stack n whose header config changed,
// which the stack and its other screens decide even where the screen did
// not change itself.
func (t *Tree) markScreens(n *node) {
	for _, c := range n.flatChildren() {
		sn := c.screen
		if sn == nil || !sn.native || c.deleted {
			continue
		}
		if !sn.sentOK || t.screenConfig(c) != sn.sent {
			t.mark(c)
		}
	}
}

// markHeaderScrolls marks the scroll views whose header inset changed:
// those near the top of a screen that lays its content under its system
// header.
func (t *Tree) markHeaderScrolls() {
	var limit Points
	// walk goes through n's children whose top, top being where n's
	// content starts below the screen's top, is above limit.
	var walk func(n *node, top Points)
	walk = func(n *node, top Points) {
		for _, c := range n.children {
			if c.deleted || c.hoist != nil || c.screen != nil {
				continue
			}
			inner := top
			if c.kind != 0 {
				if inner += c.frame.Origin.Y; inner >= limit {
					if c.scroll != nil && c.scroll.sent.HeaderInset > 0 {
						t.mark(c)
					}
					continue
				}
			}
			if sc := c.scroll; sc != nil {
				if !sc.config.Horizontal && c.created && t.headerInset(c) != c.scroll.sent.HeaderInset {
					t.mark(c)
				}
				inner -= sc.offset.Y
			}
			walk(c, inner)
		}
	}
	for _, s := range t.stacks {
		for _, c := range s.flatChildren() {
			if sn := c.screen; sn != nil && sn.native && sn.pad == 0 {
				if limit = max(sn.header, sn.sent.HeaderHeight); limit > 0 {
					walk(c, 0)
				}
			}
		}
	}
}

// screens returns the screens of stack n still shown or showable.
func (n *node) screens() (out []*node) {
	for _, c := range n.flatChildren() {
		if c.screen != nil && !c.screen.popped {
			out = append(out, c)
		}
	}
	return out
}

// pop handles back on stack n's top screen, reporting whether the stack
// took it.
func (t *Tree) pop(n *node) bool {
	screens := n.screens()
	if len(screens) < 2 {
		return false
	}
	top := screens[len(screens)-1]
	s := top.screen.screen
	if s.OnBackPress != nil {
		t.within(top.owner, s.OnBackPress)
	}
	if s.NoPopOnBack {
		return true
	}
	top.screen.popped = true
	t.showTop(n)
	if s.OnDismiss != nil {
		t.within(top.owner, s.OnDismiss)
	}
	return true
}

// backFrom handles the back button or swipe of screen n's header: its
// stack pops it while it is on top.
func (t *Tree) backFrom(n *node) {
	stack := n.screen.stack
	if stack == nil || n.screen.popped {
		return
	}
	if screens := stack.screens(); len(screens) > 0 && screens[len(screens)-1] == n {
		t.pop(stack)
	}
}

// innerStack returns the innermost stack under host, outside nested modals, that can pop: a confined owner's only while the focus is within it, as its OnBackPress handlers (Scope.Confine).
func (t *Tree) innerStack(host *node) *node {
	var found *node
	var walk func(n *node)
	walk = func(n *node) {
		if n.deleted || (n != host && n.kind == driver.KindModal) || n.style.Display == DisplayNone {
			return
		}
		if n.isStack && len(n.screens()) >= 2 && t.focusWithin(confinement(n.owner)) {
			found = n
		}
		for _, c := range n.children {
			if c.hoist == nil {
				walk(c)
			}
		}
	}
	walk(host)
	return found
}

// headerFont leaves defaults unresolved so each native bar keeps its own font.
func headerFont(s Style) driver.Font {
	return driver.Font{Family: s.FontFamily, Size: s.FontSize, Weight: s.FontWeight, Italic: s.FontStyle == FontStyleItalic}
}
