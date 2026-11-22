package core

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/layout"
)

func sameWebConfig(a, b driver.WebConfig) bool {
	return a.Source == b.Source && a.InjectedJavaScript == b.InjectedJavaScript &&
		a.InjectedJavaScriptBeforeContent == b.InjectedJavaScriptBeforeContent &&
		a.UserAgent == b.UserAgent && a.UserAgentSuffix == b.UserAgentSuffix && maps.Equal(a.Headers, b.Headers) && a.Incognito == b.Incognito &&
		a.NoScroll == b.NoScroll && a.NoJavaScript == b.NoJavaScript && a.PinchZoom == b.PinchZoom &&
		a.BackForwardGestures == b.BackForwardGestures && a.Mobile == b.Mobile && a.DownloadDir == b.DownloadDir &&
		a.Restricted == b.Restricted && a.Hosts == b.Hosts && a.Schemes == b.Schemes && a.Media == b.Media && a.Muted == b.Muted
}

// maxRounds bounds the commits OnLayout handlers may cause in one batch.
const maxRounds = 8

// Commit resolves styles, prepares text, lays the tree out and returns
// what changed since the last commit, or nil when nothing did. Changes
// that OnLayout handlers make go into the same batch.
func (t *Tree) Commit() *driver.MountBatch { return t.commit(false) }

// CommitShared is Commit for a platform that applies a batch before the
// next commit, as the web does on its one thread: the batch's items are
// the tree's own, until the next commit writes over them, so a commit
// copies nothing. A busy table's commit copied half a megabyte, which
// Go's collector paid for on the page's thread.
func (t *Tree) CommitShared() *driver.MountBatch { return t.commit(true) }

// hiddenInk is a hidden span's color: transparent, and not 0, which
// some engines read as none set.
const hiddenInk = driver.Color(0xffffff00)

func (t *Tree) commit(shared bool) *driver.MountBatch {
	t.rt.check()
	if !t.changed {
		t.activated = nil
		return nil
	}
	if t.remounting {
		defer func() { t.remounting = false }()
	}
	if t.follower != nil {
		// The automation's agent keeps batches past the next commit.
		shared = false
	}
	if t.lastShared {
		// The last batch was these items, done with now.
		t.shrinkItems(t.lastItems)
	}
	t.items = t.items[:0]
	t.commits++
	t.commitStart = t.rt.clock.Now()
	if t.devSource == nil && (t.devDrawn != nil || devtools.Enabled || t.highlights != nil) {
		t.ensureDevDrawn().beginCommit()
	}
	t.resetPace()
	for round := 0; t.changed && round < maxRounds; round++ {
		if t.focusMoved {
			t.updateFocusWithin()
			t.updateFocusVisible()
			t.updateFocusHint()
		}
		if t.pointersMoved {
			t.updatePointerEvents()
		}
		t.updateRelationStyles()
		t.commitRound()
	}
	// The user's input let its owner move the focus until what it caused was committed.
	t.activated = nil
	t.emitOrientation()
	t.markShaderInputs()
	if len(t.shaderStarts) > 0 {
		// The platform pictures what a transition starts from before it
		// applies the rest.
		starts := make([]driver.MountItem, 0, len(t.shaderStarts))
		for _, id := range t.shaderStarts {
			if n := t.nodes[id]; n != nil && !n.deleted {
				starts = append(starts, driver.MountItem{Kind: driver.ItemShaderTransition, ID: id})
			}
		}
		t.shaderStarts = t.shaderStarts[:0]
		t.items = slices.Insert(t.items, 0, starts...)
	}
	t.paceLater()
	t.timeTransitionEnds()
	if t.rt.checkCommits && !t.changed {
		t.checkCommit()
	}
	var causes *driver.Causes
	if driver.TraceCauses {
		// What ran since the graph last committed is this commit's, sent or not, so a later commit does not claim it. Writes that changed no tree wait for the next tree that changes.
		causes = t.rt.takeCauses()
	}
	if len(t.items) == 0 && (t.devNative == nil || !t.devNative.pending) {
		return nil
	}
	if t.devDrawn != nil {
		t.syncDevDrawn(t.commitStart)
	}
	t.noteGeometryVersion(t.items)
	if t.highlights != nil {
		t.noteChanges(t.items)
	}
	t.noteGeometryVersion(t.items)
	t.lastItems, t.lastShared = len(t.items), shared
	items := t.items
	if !shared {
		items = append([]driver.MountItem(nil), t.items...)
		t.shrinkItems(len(t.items))
	}
	b := &driver.MountBatch{Root: t.view.id, Items: items, LayoutAnimation: t.layoutAnim, Causes: causes}
	if t.devNative != nil && t.devNative.requested {
		b.GeometryVersion = uint64(t.commits)
		t.devNative.pending = false
	}
	t.layoutAnim = nil
	if f := t.follower; f != nil {
		f.Batch(b)
	}
	return b
}

// timeLayout lays the tree out, adding the time to the commit's causes while traces record them.
func (t *Tree) timeLayout() {
	if !driver.TraceCauses {
		t.layout()
		return
	}
	began := time.Now()
	t.layout()
	t.rt.causes.layout += time.Since(began)
}

// timeTransitionEnds runs each restyled node's OnTransitionEnd once its
// new transition's time is over, in place of an earlier wait.
func (t *Tree) timeTransitionEnds() {
	reduce := t.root.Env().reduce.GetUntracked()
	for _, n := range t.ending {
		n.restyled = false
		if n.endStop != nil {
			n.endStop()
			n.endStop = nil
		}
		if n.deleted {
			continue
		}
		var d time.Duration
		if tr := n.style.Transition; !reduce && tr.Duration > 0 {
			d = tr.Delay + tr.Duration
		}
		t.within(n.owner, func() {
			n.endStop = n.owner.AfterFunc(d, func() {
				n.endStop = nil
				if !n.deleted {
					n.h.transitionEnd()
				}
			})
		})
	}
	clear(t.ending)
	t.ending = t.ending[:0]
}

// PendingTransitionEnds reports whether a live view waits for its style transition to end before its OnTransitionEnd runs. It is nsgotest's: a step takes frames until it is false.
func (t *Tree) PendingTransitionEnds() bool {
	for _, n := range t.nodes {
		if n != nil && !n.deleted && !n.owner.disposed && n.endStop != nil {
			return true
		}
	}
	return false
}

// shrinkItems drops the items' slice for a smaller one once a commit of n
// items left most of it unused: the slice the first commit grew would stay
// this tree's capacity for good, and a long list keep megabytes for the
// handful of items a busy commit sends.
func (t *Tree) shrinkItems(n int) {
	if c := cap(t.items); c >= 4096 && c > 4*n {
		t.items = make([]driver.MountItem, 0, max(512, 2*n))
	}
}

// checkCommit commits again, laying every node out anew and comparing
// every node, and panics if that finds what the commit's marks missed.
func (t *Tree) checkCommit() {
	sent := len(t.items)
	frames := map[NodeID]Bounds{}
	for _, n := range t.nodes {
		if n != nil {
			frames[n.id] = n.sent.frame
		}
	}
	for _, n := range t.nodes {
		if n != nil && !n.deleted {
			n.lstyleOK = false
			t.markLayout(n)
		}
	}
	t.full = true
	t.commitRound()
	if missed := t.items[sent:]; len(missed) > 0 {
		var b strings.Builder
		for _, it := range missed {
			fmt.Fprintf(&b, " kind %d of view %d", it.Kind, it.ID)
			if it.Kind == driver.ItemFrame {
				fmt.Fprintf(&b, " (%v, sent %v)", it.Frame, frames[it.ID])
			}
			b.WriteString(";")
		}
		panic("core: a commit missed changes it should have sent:" + b.String())
	}
}

func (t *Tree) commitRound() {
	t.changed = false
	t.fullRound, t.full = t.full, false
	env := t.root.Env()
	if scale := env.fontScale.GetUntracked(); scale != t.fontScale {
		t.fontScale = scale
		t.fullRound = true
	}
	if ratio := env.ratio.GetUntracked(); ratio != t.ratio {
		// Texts round up to the new pixels and frames to them, so every
		// layout is laid out again.
		t.ratio = ratio
		t.fullRound = true
		t.relayoutAll()
	}
	rtl, lang, forced := env.rtl.GetUntracked(), t.shownLanguage(), env.forced.GetUntracked() != ""
	if rtl != t.rtl || lang != t.language || forced != t.forced || !t.directionSent {
		if rtl != t.rtl {
			t.setRTL(rtl)
		}
		if lang != t.language {
			t.fullRound = true
		}
		t.language, t.forced, t.directionSent = lang, forced, true
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemDirection, RTL: rtl, Language: lang, Forced: forced})
	}
	live := t.stacks[:0]
	for _, s := range t.stacks {
		if !s.deleted {
			t.navigate(s, t.topScreen(s))
			t.showTop(s)
			t.markScreens(s)
			live = append(live, s)
		} else {
			delete(t.navTops, s) // else the map keeps the removed stack and its subtree
		}
	}
	clear(t.stacks[len(live):]) // still in the array, which would keep the stacks removed
	t.stacks = live
	t.updatePeerStyles()
	t.updateClassKids()
	t.resolve(t.view, textFields{Language: lang}, false)
	if t.updateLabels() {
		t.resolve(t.view, textFields{Language: lang}, false)
	}
	t.timeLayout()
	// Lists fill their windows from the layout, which may need another.
	for range 4 {
		if !t.updateLists() {
			break
		}
		t.updatePeerStyles()
		t.updateClassKids()
		t.resolve(t.view, textFields{Language: lang}, false)
		if t.updateLabels() {
			t.resolve(t.view, textFields{Language: lang}, false)
		}
		t.timeLayout()
	}
	t.updateTooltipDescriptions()
	// System headers cover what the stacks' places decide.
	for range 2 {
		if !t.sizeHeaders() {
			break
		}
		t.timeLayout()
	}
	t.markHeaderScrolls()
	t.markEdgeScrolls()
	t.revealViews()
	if len(t.view.hosted) > 0 {
		t.placeTabStops(t.view)
	}
	for _, f := range t.frames {
		t.placeTabStops(f)
	}
	t.sendSticky()
	t.sizeCanvases()
	clear(t.laidOut)
	t.laidOut = t.laidOut[:0]
	t.emitPictureInPictures()
	t.emitAccessories()
	t.emitHeaderViews()
	t.checkPointerLock()
	t.makeCommanded()
	t.emit(t.view)
	t.syncPaced()
	for _, n := range t.transits {
		n.transit = false
	}
	clear(t.transits)
	t.transits = t.transits[:0]
	t.emitMoves()
	t.emitAnimation()
	if t.keysChanged {
		t.keysChanged = false
		t.items = append(t.items, driver.MountItem{Kind: driver.ItemKeymap, Keys: t.keymap(), HeldKeys: t.heldKeys, HeldMouseButtons: t.heldMouse})
		t.followHome()
	}
	t.emitStatusBar()
	t.emitPageHead()
	t.emitHistory()
	t.items = append(t.items, t.commands...)
	t.commands = t.commands[:0]
	t.emitMatches()
	t.emitDeletes()
	if len(t.deleted) > 0 {
		t.framesMoved = true
	}
	for _, id := range t.deleted {
		n := t.nodes[id]
		t.nodes[id] = nil
		if n != nil && n.created && n.hoist != nil && n.hoist.modal != nil && !n.hoist.dismissReported {
			t.awaitDismissal(id, n)
			continue
		}
		t.free = append(t.free, id)
	}
	t.deleted = t.deleted[:0]
	for _, n := range t.laidOut {
		if !n.deleted && n.h.layout != nil {
			frame := n.frame
			t.within(n.owner, func() { n.h.layout(frame) })
		}
	}
	if t.devDrawn != nil {
		t.syncDevDrawn(t.commitStart)
	}
	if t.bumpAudit != nil {
		t.bumpAudit()
	}
	t.sizeBars()
	t.reportFit()
	t.reportWatches()
	reports := t.reports
	t.reports = nil
	for _, r := range reports {
		if !r.owner.deleted {
			t.within(r.owner.owner, r.fn)
		}
	}
}

// resolve inherits text styles and directions down the tree and prepares
// changed text: in the nodes marked for it, and in every node inside one
// whose inherited fields changed, or everywhere when force is set.
func (t *Tree) resolve(n *node, inherited textFields, force bool) {
	force = force || t.fullRound
	// A text resolves its attachments itself.
	self := force || n.unresolved || n.resolveKids && n.text != nil
	if !self && !n.resolveKids {
		return
	}
	n.unresolved, n.resolveKids = false, false
	// A region resolves anew only as it is made or moves, with what it
	// holds.
	kids := force || self && n.kind == 0
	if n.kind != 0 {
		if inherited.Color == 0 || inherited.paletteColor {
			inherited.Color = t.paletteText(n)
			inherited.paletteColor = true
		} else if n.ink != nil {
			delete(n.ink.nodes, n)
			n.ink = nil
		}
		inherited = n.style.inherit(inherited)
		if n.style.Color != 0 {
			inherited.paletteColor = false
		}
		if self && inherited != n.passed {
			n.passed = inherited
			kids = true
		}
	}
	if !self {
		for _, c := range n.children {
			t.resolve(c, inherited, kids)
		}
		return
	}
	t.mark(n)
	if n.kind != 0 {
		// An element made apart meets the trigger around it here.
		if f := n.nativeParent(); f != nil && f.showsExpanded && !n.props.Expandable {
			t.restyleExpanded(n)
		}
		n.color = inherited.Color
		rtl := t.rtl
		if d := inherited.LayoutDirection; d != 0 {
			rtl = d == DirectionRTL
		}
		if rtl != n.rtl {
			n.rtl = rtl
			// Physical classes (ml-*) follow the node's direction, which
			// its ancestors' styles may set apart from the app's.
			if c := n.cls; c != nil && c.list.env && c.env.rtl != rtl {
				c.env.rtl = rtl
				t.resolveClasses(n)
				t.restyle(n)
			} else if n.env != nil {
				t.restyle(n)
			}
			n.lstyleOK = false
			t.markLayout(n)
		}
	}
	switch {
	case n.icon != nil:
		// An icon paints in the text color it inherits.
		in := n.icon
		in.text = t.scaled(inherited.textStyle())
		in.text.Align = sidedAlign(in.text.Align, n.rtl)
		t.within(n.owner, func() { in.color.Set(in.text.Color) })
	case n.canvas != nil:
		t.resolvePaint(n, &inherited)
		if em := n.canvas.em; em != nil {
			// An Em canvas follows its font as text does, in an attachment as far as the text's own scaling goes.
			scale := t.fontScale
			if inherited.attachScale > 0 {
				scale = inherited.attachScale
			}
			if fs := scaleText(inherited.textStyle(), scale).Font.Size; fs != em.fontSize {
				em.fontSize = fs
				t.markLayout(n)
			}
		}
	case n.text != nil:
		n.props.SelectionColor = inherited.SelectionColor
		tn := n.text
		n.props.Selectable = tn.selectable
		if tn.selectable == SelectableAuto {
			switch inherited.UserSelect {
			case UserSelectText:
				n.props.Selectable = SelectableOn
			case UserSelectNone:
				n.props.Selectable = SelectableOff
			}
		}
		if noWrap := inherited.WhiteSpace == WhiteSpaceNoWrap; noWrap != tn.noWrap {
			tn.noWrap = noWrap
			tn.dropTextLayout()
			tn.resent = true
			t.markLayout(n)
		}
		if tn.ratio != t.ratio {
			tn.ratio, tn.minWidth = t.ratio, -1
		}
		tn.engine, tn.measures = t.text, &t.measures
		if inherited.TextAlignVertical != tn.alignV {
			tn.alignV = inherited.TextAlignVertical
			tn.dropTextLayout()
			tn.resent = true
		}
		scale := tn.fontMultiplier(t.fontScale)
		ts := scaleText(inherited.textStyle(), scale)
		ts.RTL = n.textRTL(inherited, tn.content)
		ts.Align = sidedAlign(ts.Align, ts.RTL)
		sized := tn.attachments > 0 && t.sizeAttachments(n, inherited, scale, kids)
		shadow := inherited.textShadow()
		if tn.stale || sized || tn.prepared == nil || ts != tn.style || shadow != tn.text.Shadow {
			lines, ellipsize := n.textLimits()
			at := driver.AttributedText{Text: inherited.TextTransform.Apply(tn.content), Style: ts, MaxLines: lines,
				Ellipsize: ellipsize, Shadow: shadow}
			if at.Ellipsize == 0 {
				at.Ellipsize = EllipsizeTail
			}
			if len(tn.spans) > 0 {
				var b strings.Builder
				for _, sp := range tn.spans {
					start := b.Len()
					if w := sp.attach; w != nil {
						b.WriteRune(driver.AttachmentRune)
						at.Attachments = append(at.Attachments, driver.TextAttachment{Offset: start,
							Size: w.attach.size, Align: w.attach.align, Descent: w.attach.descent})
						at.Spans = append(at.Spans, driver.TextSpan{Start: start, End: b.Len(), Style: ts, Cursor: sp.style.Cursor, Copy: sp.content})
						continue
					}
					own := sp.style.inherit(inherited)
					b.WriteString(own.TextTransform.Apply(sp.content))
					spanStyle := scaleText(own.textStyle(), scale)
					spanStyle.RTL = ts.RTL
					spanStyle.Align = sidedAlign(spanStyle.Align, ts.RTL)
					span := driver.TextSpan{Start: start, End: b.Len(), Style: spanStyle, Cursor: sp.style.Cursor, Link: sp.press != nil,
						Background: driver.TextBackground{
							Color: currentIn(sp.style.Background, spanStyle.Color), Radius: sp.style.CornerRadius,
							Top: sp.style.BackgroundInsetTop, Bottom: sp.style.BackgroundInsetBottom,
						}}
					if sp.style.Visibility == VisibilityHidden {
						// In place: laid out, drawn as nothing, no link.
						span.Hidden, span.Link, span.Background = true, false, driver.TextBackground{}
						span.Style.Color, span.Style.DecorationColor = hiddenInk, hiddenInk
					}
					at.Spans = append(at.Spans, span)
				}
				at.Text = b.String()
			}
			driver.UprightEmoji(&at)
			tn.style = ts
			tn.stale = false
			if tn.prepared == nil || !at.Equal(&tn.text) || tn.fonts != t.fonts {
				tn.dropTextLayout()
				if tn.prepared != nil {
					tn.prepared.Release()
				}
				// A text that lays out as before, its colors aside, or
				// that measures as before, lays nothing out: only its own
				// lines change. Attachments are placed by them.
				// Prepared from tn.text, at stays on the stack.
				tn.text = at
				h := t.measures.hash(&tn.text)
				same := tn.prepared != nil && h == tn.hash && tn.fonts == t.fonts
				tn.hash, tn.fonts = h, t.fonts
				tn.prepared = t.text.Prepare(&tn.text)
				tn.minWidth = -1
				tn.resent = true
				if tn.attachments > 0 || !same && !layout.MeasuresAsBefore(n) {
					t.markLayout(n)
				}
			}
		}
	case n.input != nil:
		in := n.input
		in.config.PlaceholderColor, in.config.SelectionColor = inherited.PlaceholderColor, inherited.SelectionColor
		in.config.CaretColor, in.config.AccentColor = n.style.CaretColor, in.themeCaret
		cc := func(c *Color) { *c = currentIn(*c, inherited.Color) }
		cc(&in.config.PlaceholderColor)
		cc(&in.config.SelectionColor)
		cc(&in.config.CaretColor)
		if in.config.SelectionColor == 0 {
			in.config.SelectionColor = in.themeSelection
			if in.config.CaretColor == 0 {
				in.config.CaretColor = in.themeCaret
			}
		}
		ts := scaleText(inherited.textStyle(), fontMultiplier(t.fontScale, in.noScale, in.maxScale))
		ts.RTL = n.textRTL(inherited, in.value)
		ts.Align = sidedAlign(ts.Align, ts.RTL)
		if ts != in.config.Style || in.lineH == 0 {
			in.config.Style = ts
			p := t.text.Prepare(&driver.AttributedText{Text: "Ag", Style: ts})
			in.lineH = p.Measure(Auto).Height
			p.Release()
			t.markLayout(n)
		}
		in.ratio = t.ratio
		t.prepareInput(n)
	}
	if n.text != nil {
		return // its children are attachments, which it resolved
	}
	for _, c := range n.children {
		t.resolve(c, inherited, kids)
	}
}

// sidedAlign turns an alignment to a side into one to the start or the
// end of text that runs right to left when rtl.
func sidedAlign(a TextAlign, rtl bool) TextAlign {
	switch {
	case a == TextAlignLeft && !rtl, a == TextAlignRight && rtl:
		return TextAlignStart
	case a == TextAlignLeft, a == TextAlignRight:
		return TextAlignEnd
	}
	return a
}

// textRTL reports whether text node n, styled inherited, writes content
// right to left.
func (n *node) textRTL(inherited textFields, content string) bool {
	switch inherited.WritingDirection {
	case WritingLTR:
		return false
	case WritingRTL:
		return true
	case WritingAuto:
		if n.text != nil && len(n.text.spans) > 0 {
			var b strings.Builder
			for _, sp := range n.text.spans {
				if sp.attach == nil { // an attachment's content is what it copies as
					b.WriteString(sp.content)
				}
			}
			content = b.String()
		}
		return rtlText(content)
	}
	return n.rtl
}

// attachmentSpace is the space attachments are laid out in: they take
// their content size.
const attachmentSpace = 1 << 20

// sizeAttachments resolves and lays out the attachments of text n, each a layout root at its content size, with scale the user's text size setting as far as the text takes it, and reports whether a size or where one sits in its line changed.
func (t *Tree) sizeAttachments(n *node, inherited textFields, scale float32, force bool) bool {
	changed := false
	inherited.attachScale = scale
	for _, sp := range n.text.spans {
		w := sp.attach
		if w == nil {
			continue
		}
		t.resolve(w, inherited, force)
		if w.layoutDirty {
			t.layoutRoot(w, Size{Width: attachmentSpace, Height: attachmentSpace}, false)
			t.syncFrames(w)
			l := &w.lstate.Rounded
			if size := (Size{Width: Points(l.Size.Width), Height: Points(l.Size.Height)}); size != w.attach.size {
				w.attach.size = size
				changed = true
			}
		}
		align := w.verticalAlign()
		descent := t.baselineDescent(w, align)
		if align != w.attach.align || descent != w.attach.descent {
			w.attach.align, w.attach.descent = align, descent
			changed = true
		}
	}
	return changed
}

// baselineDescent is how far attachment view w, aligned as align says, reaches below the line's baseline. Text inside an attachment supplies the baseline of its first line, while an Em canvas supplies its declared descent.
func (t *Tree) baselineDescent(w *node, align VerticalAlign) Points {
	if align != 0 && align != VerticalAlignBaseline {
		return 0
	}
	c := w
	if kids := w.flatChildren(); len(kids) == 1 {
		c = kids[0]
	}
	if c.canvas == nil || c.canvas.em == nil {
		if baseline, ok := t.firstTextBaseline(w); ok {
			return max(Points(w.lstate.Rounded.Size.Height)-baseline, 0)
		}
		return 0
	}
	l := &c.lstate.Rounded
	h := Points(w.lstate.Rounded.Size.Height)
	if c != w {
		// Its frame in w's.
		bottom := Points(l.Location.Y) + Points(l.Size.Height)
		return max(h-bottom+c.canvas.em.descent(Points(l.Size.Height)), 0)
	}
	return c.canvas.em.descent(h)
}

// firstTextBaseline returns the first laid-out text baseline in n's subtree, measured from n's top. It lets an inline view containing text participate in its surrounding line's baseline alignment.
func (t *Tree) firstTextBaseline(n *node) (Points, bool) {
	if n.style.Display == DisplayNone || n.style.Visibility == VisibilityHidden {
		return 0, false
	}
	if tn := n.text; tn != nil && tn.prepared != nil && tn.text.Text != "" {
		if tn.baseline == nil {
			tn.pending = t.layoutText(n)
			t.mark(n)
			y, ok := firstLineBaseline(tn.pending.Layout)
			tn.baseline = &textBaseline{y: tn.pending.Content.Origin.Y + y, ok: ok}
		}
		return tn.baseline.y, tn.baseline.ok
	}
	for _, child := range n.flatChildren() {
		if child.style.Position == Absolute {
			continue
		}
		if baseline, ok := t.firstTextBaseline(child); ok {
			return child.frame.Origin.Y + baseline, true
		}
	}
	return 0, false
}

func firstLineBaseline(l driver.TextLayout) (Points, bool) {
	if lines, ok := l.(interface{ FirstBaseline() (Points, bool) }); ok {
		return lines.FirstBaseline()
	}
	if lines, ok := l.(LineLayout); ok {
		if rows := lines.TextLines(); len(rows) > 0 {
			line := rows[0]
			leading := line.Bounds.Size.Height - line.Ascender - line.Descender
			return line.Bounds.Origin.Y + line.Ascender + leading/2, true
		}
	}
	return 0, false
}

// verticalAlign is where attachment view w sits in its line: its span's
// VerticalAlign, else that of the one view the attachment made.
func (w *node) verticalAlign() VerticalAlign {
	if a := w.style.VerticalAlign; a != 0 {
		return a
	}
	if kids := w.flatChildren(); len(kids) == 1 {
		return kids[0].style.VerticalAlign
	}
	return 0
}

// placeAttachments puts the views of text n's attachments where layout
// put their placeholders, hiding those its line limit cut off.
func (t *Tree) placeAttachments(n *node, l driver.TextLayout) {
	al, _ := l.(AttachmentLayout)
	i := 0
	for _, sp := range n.text.spans {
		w := sp.attach
		if w == nil {
			continue
		}
		var b Bounds
		ok := false
		if al != nil {
			b, ok = al.Attachment(i)
		}
		i++
		if w.attach.cut != !ok {
			w.attach.cut = !ok
			t.framesMoved = true
			t.mark(w)
		}
		if ok {
			b.Origin.X += n.text.shownBox.Origin.X
			b.Origin.Y += n.text.shownBox.Origin.Y
			t.setFrame(w, Bounds{Origin: b.Origin, Size: w.attach.size})
		}
	}
}

// sizeCanvases tells canvases their laid-out size, which draws those
// that changed.
func (t *Tree) sizeCanvases() {
	live := t.canvases[:0]
	for _, n := range t.canvases {
		if n.deleted {
			continue
		}
		live = append(live, n)
		if n.canvas.size.GetUntracked() != n.frame.Size {
			size := n.frame.Size
			t.within(n.owner, func() { n.canvas.size.Set(size) })
		}
	}
	clear(t.canvases[len(live):])
	t.canvases = live
}

// within2 runs fn under n's owner and returns its result.
func (t *Tree) within2(n *node, fn func() bool) (r bool) {
	t.within(n.owner, func() { r = fn() })
	return r
}

// scaled applies the user's text size setting.
func (t *Tree) scaled(ts driver.TextStyle) driver.TextStyle { return scaleText(ts, t.fontScale) }

// scaleText multiplies ts's font size and line height by scale.
func scaleText(ts driver.TextStyle, scale float32) driver.TextStyle {
	if s := Points(scale); s != 1 && s > 0 {
		ts.Font.Size *= s
		ts.LineHeight *= s
	}
	return ts
}

// emit sends what changed of view n and the views inside it, visiting
// only those marked, and reports whether n moved, resized or changed how
// it shows, which its parent's overflow area follows.
func (t *Tree) emit(n *node) bool {
	self := n.dirty || t.fullRound
	if !self && !n.dirtyKids {
		return false
	}
	n.dirty, n.dirtyKids = false, false
	first := !n.created
	moved := false
	if n.childrenDirty && t.rt.confined {
		n.confines = confines(n)
	}
	if self {
		moved = t.emitParts(n)
	}
	kids := n.nativeChildren()
	kidsMoved, held := false, false
	for _, c := range kids {
		if !c.created && t.holdView(n, c) {
			held = true
			continue
		}
		if t.emit(c) {
			kidsMoved = true
		}
	}
	if n.childrenDirty {
		if held {
			t.syncChildren(n, madeChildren(kids))
		} else {
			t.syncChildren(n, kids)
		}
		n.childrenDirty = false
		kidsMoved = true
	}
	if n != t.view && (self || kidsMoved) {
		if area, transformed := n.overflowArea(kids); area != n.sent.overflow || transformed != n.sent.transformed {
			n.sent.overflow, n.sent.transformed = area, transformed
			t.items = append(t.items, driver.MountItem{Kind: driver.ItemOverflow, ID: n.id, Frame: area, Transformed: transformed})
			moved = true
		}
	}
	if self && (first || n.frame != n.sent.frame) {
		n.sent.frame = n.frame
		it := driver.MountItem{Kind: driver.ItemFrame, ID: n.id, Frame: n.frame}
		if n.transit && !first && !t.root.Env().reduce.GetUntracked() {
			it.Transition = n.style.Transition.Layout()
		}
		if f := n.follow; f != nil {
			if f.moved != nil && !first && !t.root.Env().reduce.GetUntracked() {
				it.Transition = f.moved
			}
			f.moved = nil
		}
		t.items = append(t.items, it)
		if n.h.layout != nil {
			t.laidOut = append(t.laidOut, n)
		}
		moved = true
	}
	return moved
}

// emitParts sends what changed of n's own parts, creating its view the
// first time, and reports whether its view style changed.
func (t *Tree) emitParts(n *node) bool {
	add := func(it driver.MountItem) { t.items = append(t.items, it) }
	first := !n.created
	styled := false
	if first {
		add(driver.MountItem{Kind: driver.ItemCreate, ID: n.id, View: n.kind})
		if n.match != "" {
			t.matchNew = append(t.matchNew, n)
		}
		n.created, n.pace = true, 0
		t.views.made++
	}
	// What goes into an item is a copy made only when it is sent: the
	// comparison alone would otherwise put both values on the heap for
	// every view of every commit.
	// A new view starts in the default style with no props and no events
	// on every backend, so they are sent only when they differ.
	if first {
		n.sent.viewStyle, n.sent.props, n.sent.events = driver.DefaultViewStyle, driver.ViewProps{}, 0
	}
	if vs := n.viewStyle(); vs != n.sent.viewStyle {
		n.sent.viewStyle = vs
		sent := vs
		add(driver.MountItem{Kind: driver.ItemViewStyle, ID: n.id, ViewStyle: &sent})
		styled = true
	}
	if p := n.sentProps(); p != n.sent.props {
		// A label changing alone goes alone: it is the one that follows
		// app state (a count, a price).
		rest := p
		rest.Label = n.sent.props.Label
		if !first && rest == n.sent.props {
			add(driver.MountItem{Kind: driver.ItemAccessibilityLabel, ID: n.id, Value: p.Label})
		} else {
			sent := p
			add(driver.MountItem{Kind: driver.ItemViewProps, ID: n.id, ViewProps: &sent})
		}
		n.sent.props = p
	}
	if n.style.Animated != nil || n.anim != nil {
		t.bindAnimated(n)
	}
	if !n.transformed && n.mayTransform() {
		// Its parent's hit tests look for it anywhere from now on.
		n.transformed, styled = true, true
	}
	ev := n.events()
	if t.devDrawn != nil && n.scroll != nil && n.bound != nil && n.bound.scroll != (AnimatedXY{}) {
		ev |= 1 << driver.EventScrollEnd
	}
	if ev != n.sent.events {
		n.sent.events = ev
		add(driver.MountItem{Kind: driver.ItemEvents, ID: n.id, Events: ev})
	}
	switch {
	case n.pip != nil:
		if p := n.pip; pictureInPicturePlatform(t.root) && (first || p.config != p.sent) {
			c := p.config
			p.sent = c
			add(driver.MountItem{Kind: driver.ItemPictureInPicture, ID: n.id, PictureInPicture: &c})
		}
	case n.text != nil:
		tn := n.text
		if tn.prepared != nil && (tn.pending != nil || tn.resent || tn.box != tn.sentBox) {
			tn.resent = false
			tn.sentBox = tn.box
			t.emitText(n)
		}
	case n.input != nil:
		in := n.input
		in.config.Accessory, in.config.AccessoryID = t.accessoryFor(in.accessory), in.accessory
		if first || in.config != in.sent {
			c := in.config
			in.sent = c
			add(driver.MountItem{Kind: driver.ItemInput, ID: n.id, Input: &c})
		}
		if first || in.value != in.shown {
			in.shown = in.value
			add(driver.MountItem{Kind: driver.ItemInputValue, ID: n.id, Value: in.value, Count: in.count})
		}
		if first && !t.remounting && in.autoFocus && t.mayMoveFocus(n.owner) {
			t.command(driver.ItemFocus, n)
		}
		t.reportContentSize(n)
	case n.image != nil:
		im := n.image
		c := im.config
		c.ResizeMode = n.resizeMode(c.ResizeMode, Cover)
		c.Position = n.style.ObjectPosition
		c.Source = t.pickScale(c.Source)
		c.Placeholder = t.pickScale(c.Placeholder)
		// A placeholder shows while a source loads, which pixels and offscreen drawings never do: they have none, and a placeholder's pixels or offscreen drawing are dropped.
		if c.Source.Pixels != nil || c.Source.Offscreen != nil {
			c.Placeholder = ImageSource{}
		}
		c.Placeholder.Pixels, c.Placeholder.Offscreen, c.Placeholder.Version = nil, nil, 0
		if t.rt.restricted {
			t.refuseImage(n, &c.Source, &im.refused, n.h.loadError)
			t.refuseImage(n, &c.Placeholder, &im.refusedPlaceholder, n.h.loadError)
		}
		if first || !c.Same(im.sent) {
			im.sent = c
			c.Source.Pixels = im.frame.of(c.Source)
			add(driver.MountItem{Kind: driver.ItemImage, ID: n.id, Image: &c})
		}
	case n.hoist != nil && n.hoist.modal != nil:
		if first {
			m := n.hoist.modal
			add(driver.MountItem{Kind: driver.ItemModal, ID: n.id, Modal: &driver.ModalConfig{Animation: m.Animation, Presentation: m.Presentation,
				NoCloseOnSwipe: m.NoCloseOnSwipe, Orientations: m.IOS.SupportedOrientations,
				NoCloseOnEscape: m.NoCloseOnEscape, NoCloseOnBackdropPress: m.NoCloseOnBackdropPress,
				StatusBarTranslucent: m.Android.StatusBarTranslucent}})
		}
	case n.canvas != nil:
		if cn := n.canvas; cn.fresh && cn.drawing != nil {
			cn.fresh = false
			add(driver.MountItem{Kind: driver.ItemCanvas, ID: n.id, Drawing: cn.withFrames(cn.drawing.Painted(&cn.paint, cn.color))})
		}
	case n.shader != nil:
		if sn := n.shader; sn.fresh {
			sn.fresh = false
			add(driver.MountItem{Kind: driver.ItemShader, ID: n.id, Shader: sn.withFrames()})
		}
	case n.gpu != nil:
		if gn := n.gpu; gn.fresh {
			gn.fresh = false
			cfg := gn.cfg
			add(driver.MountItem{Kind: driver.ItemGPU, ID: n.id, GPU: &cfg})
		}
	case n.web != nil:
		wn := n.web
		c := wn.config
		c.Headers = cloneWebHeaders(c.Headers)
		if t.rt.restricted {
			t.restrictWeb(n, &c)
		}
		c.Source = driver.ResolveWebSource(c.Source)
		if first || !sameWebConfig(c, wn.sent) {
			wn.sent = c
			add(driver.MountItem{Kind: driver.ItemWeb, ID: n.id, Web: &c})
		}
	case n.native != nil:
		t.emitNative(n)
	case n.video != nil:
		vn := n.video
		c := vn.config
		c.ResizeMode = n.resizeMode(c.ResizeMode, Contain)
		if t.rt.restricted {
			t.refuseSource(n, AccessNetwork, &c.Source, &vn.refused, vn.loadError)
			if c.Source == "" {
				c.AudioSource = ""
			} else if c.AudioSource != "" {
				t.refuseSource(n, AccessNetwork, &c.AudioSource, &vn.refusedAudio, vn.loadError)
				if c.AudioSource == "" {
					c.Source = ""
				}
			}
		}
		if first || c != vn.sent {
			if !first && c.Source == vn.sent.Source && c.AudioSource == vn.sent.AudioSource {
				t.emitSeeks(n)
			}
			vn.sent = c
			add(driver.MountItem{Kind: driver.ItemVideo, ID: n.id, Video: &c})
		}
	case n.scroll != nil:
		sc := n.scroll
		c := sc.config
		c.Edges = sc.scrollEdges()
		if inset := t.headerInset(n); inset > 0 {
			c.HeaderInset = inset
			c.Behavior.ContentInset.Top += inset
		}
		if first || sc.resend || !c.Equal(&sc.sent) {
			sc.resend = false
			c.Behavior.SnapToOffsets = slices.Clone(c.Behavior.SnapToOffsets)
			sc.sent = c
			add(driver.MountItem{Kind: driver.ItemScrollConfig, ID: n.id, Scroll: &c})
		}
		t.emitScrollBy(n, true)
		if fitted := n.fittedContent(&c); first || sc.content != sc.sentContent || fitted != sc.sentFitted {
			sc.sentContent, sc.sentFitted = sc.content, fitted
			add(driver.MountItem{Kind: driver.ItemScrollContent, ID: n.id, Size: fitted})
		}
		t.emitScrollBy(n, false)
		t.emitScrollTo(n)
	}
	if sc := n.screen; sc != nil && sc.native {
		t.emitScreen(n)
	}
	t.emitFx(n)
	return styled
}

// layoutText places text in its content box, taking its vertical alignment and fitting into account.
func (t *Tree) layoutText(n *node) *driver.TextContent {
	tn := n.text
	box := tn.box
	if f := tn.alignV.fraction(); f > 0 {
		if h := tn.size(box.Size.Width).Height; h < box.Size.Height {
			box.Origin.Y += f * (box.Size.Height - h)
			box.Size.Height = h
		}
	}
	prepared, text := tn.prepared, &tn.text
	if tn.fitted != nil {
		tn.fitted.Release()
		tn.fitted = nil
	}
	if tn.fit {
		if fitted, ok := fitText(t.text, text, box.Size, tn.minScale); ok {
			tn.fitted = t.text.Prepare(&fitted)
			prepared, text = tn.fitted, &fitted
		}
	}
	if tn.noWrap {
		// Unwrapped lines overflow the box.
		box.Size.Width = max(box.Size.Width, ceil(prepared.Measure(Auto).Width, t.ratio))
	}
	l := prepared.Layout(box.Size)
	return &driver.TextContent{
		Text: text.Text, Layout: l, Content: box, Style: text.Style, Spans: text.Spans, Shadow: text.Shadow,
		MaxLines: text.MaxLines, Ellipsize: text.Ellipsize,
	}
}

// emitText sends the text's layout and reports its lines after the commit.
func (t *Tree) emitText(n *node) {
	tn := n.text
	text := tn.pending
	if text == nil {
		text = t.layoutText(n)
	}
	tn.pending = nil
	l := text.Layout
	tn.shownBox = text.Content
	tn.layout = l
	if tn.attachments > 0 {
		t.placeAttachments(n, l)
	}
	if tn.lines != nil {
		if ll, ok := l.(LineLayout); ok {
			lines := ll.TextLines()
			for i := range lines {
				a, b := min(max(lines[i].Start, 0), len(text.Text)), min(max(lines[i].End, 0), len(text.Text))
				lines[i].Text = text.Text[a:max(a, b)]
			}
			report := tn.lines
			t.after(n, func() { report(lines) })
		}
	}
	t.items = append(t.items, driver.MountItem{Kind: driver.ItemText, ID: n.id, Text: text})
}

// fitText returns text with its fonts, line heights and letter spacing
// scaled down, to no less than least of them, until it fits size within
// its line limit: the largest scale that fits, by bisection, as NSH's
// Paragraph::fit. It reports false when text fits as it is.
func fitText(engine driver.TextEngine, text *driver.AttributedText, size Size, least float32) (driver.AttributedText, bool) {
	scaled := func(scale float32) driver.AttributedText {
		s := *text
		s.Style = fitStyle(s.Style, scale)
		s.Spans = append([]driver.TextSpan(nil), text.Spans...)
		for i := range s.Spans {
			s.Spans[i].Style = fitStyle(s.Spans[i].Style, scale)
		}
		return s
	}
	measure := func(t *driver.AttributedText) Size {
		p := engine.Prepare(t)
		defer p.Release()
		return p.Measure(size.Width)
	}
	fits := func(t driver.AttributedText) bool {
		// Untruncated, the text must be no taller than its truncated self
		// (nothing cut) and than the box.
		limit := size.Height
		if t.MaxLines > 0 {
			limit = min(limit, measure(&t).Height)
		}
		all := t
		all.MaxLines = 0
		full := measure(&all)
		return full.Height <= limit+0.5 && full.Width <= size.Width+0.5
	}
	if fits(*text) {
		return driver.AttributedText{}, false
	}
	lo, hi := least, float32(1)
	if !fits(scaled(lo)) {
		return scaled(lo), true
	}
	for range 7 {
		mid := (lo + hi) / 2
		if fits(scaled(mid)) {
			lo = mid
		} else {
			hi = mid
		}
	}
	return scaled(lo), true
}

// fitStyle scales what fitting shrinks.
func fitStyle(ts driver.TextStyle, scale float32) driver.TextStyle {
	s := Points(scale)
	ts.Font.Size *= s
	ts.LineHeight *= s
	ts.LetterSpacing *= s
	return ts
}

func (n *node) viewStyle() driver.ViewStyle {
	vs := n.style.viewStyle(n.frame.Size, n.color, n.rtl)
	// A confined owner's frame clips what it shows (Scope.Confine).
	vs.Clip = vs.Clip || n.confines
	vs.Material, vs.Elevation = n.material, n.elevation
	if n.canvas != nil && n.canvas.drawing != nil && n.canvas.drawing.HasBackground() {
		// Its drawing fills its shapes with the background.
		vs.Background = 0
		if vs.Effects != nil && len(vs.Effects.Gradients) > 0 {
			e := *vs.Effects
			e.Gradients = nil
			vs.Effects = &e
			if e.Empty() {
				vs.Effects = nil
			}
		}
	}
	vs.Hidden = vs.Hidden || n.style.Display == DisplayNone || (n.hoist != nil && n.hidden) || (n.attach != nil && n.attach.cut)
	// Effects that draw as before keep the pointer that was sent, so the
	// comparison finds nothing to send.
	if vs.Effects != nil && vs.Effects.Equal(n.sent.viewStyle.Effects) {
		vs.Effects = n.sent.viewStyle.Effects
	}
	// Custom cursors copy any Pixels once per version (so the app may reuse
	// its buffer after commit) and keep the sent pointer while unchanged.
	if vs.CursorImage != nil {
		if vs.CursorImage.Image.Pixels != nil || vs.CursorImage.Image2x.Pixels != nil {
			if n.cursorPixels == nil {
				n.cursorPixels = new([2]pixelFrame)
			}
			ci := *vs.CursorImage
			ci.Image.Pixels = n.cursorPixels[0].of(ci.Image)
			ci.Image2x.Pixels = n.cursorPixels[1].of(ci.Image2x)
			vs.CursorImage = &ci
		} else if n.cursorPixels != nil {
			n.cursorPixels = nil
		}
		if vs.CursorImage.Equal(n.sent.viewStyle.CursorImage) {
			vs.CursorImage = n.sent.viewStyle.CursorImage
		}
	} else if n.cursorPixels != nil {
		n.cursorPixels = nil
	}
	return vs
}

// emitMatches sends ItemMatch for each view the round creates with the
// MatchID of one it deletes, after the inserts, so the backend has both
// in place, and before the deletes. Only a layout animation's Update
// moves it; otherwise the views come and go as any.
func (t *Tree) emitMatches() {
	if a := t.layoutAnim; a != nil && a.Update != nil && len(t.matchGone) > 0 && len(t.matchNew) > 0 {
		gone := make(map[string]*node, len(t.matchGone))
		for _, n := range t.matchGone {
			gone[n.match] = n
		}
		for _, n := range t.matchNew {
			if g := gone[n.match]; g != nil && !n.deleted {
				delete(gone, n.match)
				t.items = append(t.items, driver.MountItem{Kind: driver.ItemMatch, ID: n.id, Before: g.id})
			}
		}
	}
	clear(t.matchGone)
	t.matchGone = t.matchGone[:0]
	clear(t.matchNew)
	t.matchNew = t.matchNew[:0]
}

// emitDeletes deletes the views discarded since the last round. They go
// after the round's inserts, so a view moved out of a deleted one is in
// its new parent first; one taken out of the tree is removed before.
func (t *Tree) emitDeletes() {
	for _, id := range t.deleted {
		n := t.nodes[id]
		if !n.created {
			continue
		}
		for _, c := range n.sent.children {
			if !c.deleted && c.sent.parent == n {
				c.sent.parent, c.sent.removed = nil, true
				t.items = append(t.items, driver.MountItem{Kind: driver.ItemRemove, ID: c.id})
			}
		}
	}
	for _, id := range t.deleted {
		if t.nodes[id].created {
			t.items = append(t.items, driver.MountItem{Kind: driver.ItemDelete, ID: id})
		}
	}
}

// syncChildren inserts, moves and removes p's native children so they
// match want, moving as few as the longest run already in order allows.
// A view that changes parents is never removed: its one insert, after
// every new parent's, lets a backend move it in one step, keeping what a
// removal loses (a web page's scroll offsets).
func (t *Tree) syncChildren(p *node, want []*node) {
	old := p.sent.children
	pos := make(map[*node]int, len(old))
	for i, c := range old {
		if !c.deleted && c.sent.parent == p {
			pos[c] = i
		}
	}
	keep := make(map[*node]bool, len(want))
	for _, c := range want {
		keep[c] = true
	}
	for _, c := range old {
		if !c.deleted && !keep[c] && c.sent.parent == p && !t.insertedElsewhere(c, p) {
			c.sent.parent, c.sent.removed = nil, true
			t.items = append(t.items, driver.MountItem{Kind: driver.ItemRemove, ID: c.id})
		}
	}
	seq := make([]int, len(want))
	for i, c := range want {
		if j, ok := pos[c]; ok {
			seq[i] = j
		} else {
			seq[i] = -1
		}
	}
	stay := increasingRun(seq)
	var moving []bool
	for i := len(want) - 1; i >= 0; i-- {
		c := want[i]
		if stay[i] {
			continue
		}
		if from := c.sent.parent; from != nil && from != p || c.sent.removed {
			// Moved out of a view that still holds it, or put back: inserted
			// after the round's other inserts, once its new parent is in
			// place too.
			if moving == nil {
				moving = make([]bool, len(want))
			}
			moving[i] = true
			m := move{view: c, parent: p}
			if i+1 < len(want) {
				m.before = want[i+1]
			}
			t.moves = append(t.moves, m)
			c.sent.parent, c.sent.removed = p, false
			continue
		}
		var before NodeID
		for j := i + 1; j < len(want); j++ {
			if moving == nil || !moving[j] {
				before = want[j].id
				break
			}
		}
		c.sent.parent = p
		t.items = append(t.items, driver.MountItem{Kind: driver.ItemInsert, ID: c.id, Parent: p.id, Before: before})
	}
	p.sent.children = append(p.sent.children[:0], want...)
	if len(want) < len(old) {
		clear(old[len(want):]) // still in the array, which would keep the views dropped
	}
}

// move is the insert of a view inserted before into a parent, put off
// to the end of the round.
type move struct{ view, parent, before *node }

// emitMoves inserts the views that change parents or come back, the
// last of each parent's first, so the sibling each goes before is in
// place.
func (t *Tree) emitMoves() {
	for _, m := range t.moves {
		var before NodeID
		if m.before != nil {
			before = m.before.id
		}
		t.items = append(t.items, driver.MountItem{Kind: driver.ItemInsert, ID: m.view.id, Parent: m.parent.id, Before: before})
	}
	t.moves = t.moves[:0]
}

// insertedElsewhere reports whether c, no longer p's, is in another view
// this commit sends, whose sync inserts it.
func (t *Tree) insertedElsewhere(c, p *node) bool {
	n := c.nativeParent()
	if n == nil || n == p {
		return false
	}
	for n != nil && !n.deleted {
		switch {
		case n == t.view || n.accessory != nil || n.pip != nil || n.headerView:
			return true
		case n.hoist != nil:
			n = n.hoist.host
		default:
			n = n.parent
		}
	}
	return false
}

// increasingRun marks a longest strictly increasing subsequence of seq,
// ignoring negative entries.
func increasingRun(seq []int) []bool {
	stay := make([]bool, len(seq))
	tails := []int{} // index into seq of the smallest tail of each length
	prev := make([]int, len(seq))
	for i, v := range seq {
		if v < 0 {
			continue
		}
		lo, hi := 0, len(tails)
		for lo < hi {
			mid := (lo + hi) / 2
			if seq[tails[mid]] < v {
				lo = mid + 1
			} else {
				hi = mid
			}
		}
		if lo > 0 {
			prev[i] = tails[lo-1]
		} else {
			prev[i] = -1
		}
		if lo == len(tails) {
			tails = append(tails, i)
		} else {
			tails[lo] = i
		}
	}
	if len(tails) > 0 {
		for i := tails[len(tails)-1]; i >= 0; i = prev[i] {
			stay[i] = true
		}
	}
	return stay
}
