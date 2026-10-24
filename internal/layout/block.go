package layout

// This file is Taffy's compute/block.rs: the CSS block layout algorithm for
// block containers that contain only block-level boxes, with margin
// collapsing. NSH disables float layout, so a block formatting context
// carries no floats and only tracks whether it is a formatting context's
// root.

// blockContext is a block formatting context as seen by one block container
// in it: Taffy's BlockContext without the float_layout feature. Layout
// passes a nil context where Taffy passes None.
type blockContext struct {
	// isRoot says whether the container is the root of the formatting
	// context.
	isRoot bool
}

// blockSubContext is the context a block container passes to its children
// in the same formatting context. Without floats it carries nothing but
// isRoot, so every child can share this one immutable value.
var blockSubContext = blockContext{}

// computeBlockAlignContentOffset is the offset align-content gives a block
// container's in-flow content, treated as a single alignment subject.
func computeBlockAlignContentOffset(alignContent AlignContent, freeSpace float32) float32 {
	keyword := applyAlignmentFallback(freeSpace, 1, alignContent)
	return computeAlignmentOffset(freeSpace, 1, 0, keyword, false, true)
}

// blockItem is per-child data accumulated over the block layout algorithm.
type blockItem struct {
	node Node
	// order is the item's index among the generated (non-hidden) children,
	// controlling the order in which items are placed.
	order uint32

	// Tables are not stretch-sized.
	isTable bool
	// Compressible replaced items resolve an auto width to their intrinsic
	// size rather than being stretch-sized
	// (https://www.w3.org/TR/CSS22/visudet.html#block-replaced-width).
	isCompressibleReplaced bool
	// isInSameBFC says whether the child is a block box in this container's
	// formatting context (not establishing an independent one).
	isInSameBFC bool

	// sizeStyle is the item's size style, used to resolve sizing keywords.
	// In the block axis only stretch needs resolving: the intrinsic keywords
	// all equal the content size, which an auto height resolves to anyway.
	sizeStyle Size[Length]

	size, minSize, maxSize Size[Opt]

	overflow       Point[Overflow]
	contain        Contain
	scrollbarWidth float32

	position  Position
	direction Direction
	// alignSelf and justifySelf are unset (AlignAuto) when auto.
	alignSelf, justifySelf Align
	inset                  Rect[Length]
	margin                 Rect[Length]
	padding, border        Rect[float32]
	paddingBorderSum       Size[float32]

	// computedSize is the item's border box size.
	computedSize Size[float32]
	// staticPosition is the position taking padding, border, margins and
	// scrollbar gutters into account, but not insets.
	staticPosition        Point[float32]
	canBeCollapsedThrough bool

	// finalLayout is the pending layout of an in-flow item, held back so
	// that align-content can shift it before it is committed.
	finalLayout    Layout
	hasFinalLayout bool

	// oofCandidates are out-of-flow candidates bubbled out of this in-flow
	// item's subtree, relative to its border box.
	oofCandidates []oofCandidate
}

// computeBlockLayout lays out a block container: Taffy's
// compute_block_layout. bctx is the formatting context of a parent block
// container whose context this one may share.
func computeBlockLayout(n Node, in layoutInput, bctx *blockContext) layoutOutput {
	known, parentSize := in.knownDimensions, in.parentSize
	style := n.Style()

	overflow := style.Overflow
	isScrollContainer := overflow.X.isScrollContainer() || overflow.Y.isScrollContainer()
	contain := style.Contain
	// css-align-3 §5.1.1: a non-normal align-content makes a block container
	// establish an independent formatting context
	// (https://drafts.csswg.org/css-align-3/#distribution-block). Layout and
	// paint containment do too
	// (https://drafts.csswg.org/css-contain-2/#containment-layout).
	establishesNewBFC := isScrollContainer || style.AlignContent.Keyword != ContentNormal ||
		contain.establishesIndependentFormattingContext()
	aspectRatio := style.AspectRatio
	padding := rectResolveOrZero(style.padding(), parentSize.Width)
	border := rectResolveOrZero(style.border(), parentSize.Width)
	paddingBorderSize := rectSumAxes(rectAdd(padding, border))
	var boxSizingAdjustment Size[float32]
	if style.BoxSizing == ContentBox {
		boxSizingAdjustment = paddingBorderSize
	}

	minSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.MinSize, parentSize), aspectRatio), boxSizingAdjustment)
	maxSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.MaxSize, parentSize), aspectRatio), boxSizingAdjustment)
	var clampedStyleSize Size[Opt]
	if in.sizingMode == inherentSize {
		clampedStyleSize = optsClamp(
			optsAddF(optsAspectRatio(sizeMaybeResolve(style.Size, parentSize), aspectRatio), boxSizingAdjustment),
			minSize, maxSize)
	}

	// If both min and max on an axis are set and max <= min then they
	// determine the size on that axis.
	minMaxDefinite := minMaxDefiniteSize(minSize, maxSize)

	styledKnown := optsMaxF(optsOr(optsOr(known, minMaxDefinite), clampedStyleSize), paddingBorderSize)

	// Short-circuit layout if the container's size is fully determined and
	// only its size is wanted.
	if in.runMode == computeSize {
		if optsBoth(styledKnown) {
			return outputFromOuterSize(optsUnwrapOr(styledKnown, Size[float32]{}))
		}
		// Also if the width is known and only the width is requested.
		if in.axis == requestHorizontal && styledKnown.Width.Valid {
			return outputFromOuterSize(Size[float32]{Width: styledKnown.Width.Value})
		}
	}

	// Join the inherited formatting context, or else establish a new one.
	isRoot := true
	if bctx != nil && !establishesNewBFC {
		isRoot = bctx.isRoot
	}
	in.knownDimensions = styledKnown
	output := computeBlockInner(n, in, isRoot)

	// Layout containment suppresses the box's baseline for baseline
	// alignment.
	if contain.suppressesBaseline() {
		output.baselines = baselines{}
	}
	return output
}

// computeBlockInner is Taffy's compute_inner. isRoot says whether the
// container is the root of its block formatting context.
func computeBlockInner(n Node, in layoutInput, isRoot bool) layoutOutput {
	known, parentSize, available := in.knownDimensions, in.parentSize, in.availableSpace
	runMode := in.runMode
	collapsible := in.verticalMarginsAreCollapsible

	style := n.Style()
	rawPadding := style.padding()
	rawBorder := style.border()
	rawMargin := style.margin()
	aspectRatio := style.AspectRatio
	padding := rectResolveOrZero(rawPadding, parentSize.Width)
	border := rectResolveOrZero(rawBorder, parentSize.Width)
	direction := style.Direction

	// Scrollbar gutters are reserved when overflow is scroll, on the axis
	// opposite the scrolling one: a node that scrolls vertically needs
	// horizontal space for its scrollbar.
	offsets := style.scrollbarGutter()
	scrollbarGutter := Rect[float32]{Right: offsets.X, Bottom: offsets.Y}
	if direction.isRTL() {
		scrollbarGutter = Rect[float32]{Left: offsets.X, Bottom: offsets.Y}
	}
	paddingBorder := rectAdd(padding, border)
	paddingBorderSize := rectSumAxes(paddingBorder)
	contentBoxInset := rectAdd(paddingBorder, scrollbarGutter)

	var boxSizingAdjustment Size[float32]
	if style.BoxSizing == ContentBox {
		boxSizingAdjustment = paddingBorderSize
	}
	size := optsAddF(optsAspectRatio(sizeMaybeResolve(style.Size, parentSize), aspectRatio), boxSizingAdjustment)
	minSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.MinSize, parentSize), aspectRatio), boxSizingAdjustment)
	maxSize := optsAddF(optsAspectRatio(sizeMaybeResolve(style.MaxSize, parentSize), aspectRatio), boxSizingAdjustment)

	// css-sizing-4: a definite size on one axis transfers through
	// aspect-ratio to make the other definite. Deriving it from the known
	// dimensions gates the transfer: a block parent fills an axis only when
	// it is a real constraint (such as the stretched width at final layout)
	// and leaves it absent while probing intrinsic sizes, so measure passes
	// stay content-based. Only a newly filled axis is adopted (and clamped);
	// an incoming known size is left as the parent resolved it (clamping it
	// again would undo padding and border overrides).
	derived := optsClamp(optsAspectRatio(known, aspectRatio), minSize, maxSize)
	known = Size[Opt]{known.Width.OrOpt(derived.Width), known.Height.OrOpt(derived.Height)}

	percentageBasis := Size[Opt]{Width: known.Width}
	if in.knownDimensionsAreDefinite.Height {
		percentageBasis.Height = known.Height
	}
	containerContentBoxSize := optsSubF(percentageBasis, rectSumAxes(contentBoxInset))

	overflow := style.Overflow
	isScrollContainer := overflow.X.isScrollContainer() || overflow.Y.isScrollContainer()
	establishesNewBFC := isScrollContainer || style.AlignContent.Keyword != ContentNormal ||
		style.Contain.establishesIndependentFormattingContext()

	// Determine margin collapsing behaviour.
	//
	// Whether a child's margins could collapse through each of this box's
	// block-axis edges: the box does not establish a new formatting context
	// and has neither border nor padding on that side.
	outOfFlow := style.Position.isOutOfFlow()
	childrenMarginsAdjoinOwnEdges := Line[bool]{
		Start: collapsible.Start && !establishesNewBFC && !outOfFlow && padding.Top == 0 && border.Top == 0,
		End:   collapsible.End && !establishesNewBFC && !outOfFlow && padding.Bottom == 0 && border.Bottom == 0,
	}
	ownMarginsCollapseWithChildren := Line[bool]{
		Start: childrenMarginsAdjoinOwnEdges.Start,
		End:   childrenMarginsAdjoinOwnEdges.End && !size.Height.Valid,
	}
	preventsCollapseThrough := !style.isBlock() || isRoot || establishesNewBFC || outOfFlow ||
		padding.Top > 0 || padding.Bottom > 0 || border.Top > 0 || border.Bottom > 0

	textAlign := style.TextAlign
	alignContent := style.AlignContent
	justifyItems := style.justifyItems()

	// 1. Generate items.
	items := blockGenerateItems(n, containerContentBoxSize)

	// 2. Compute the container's width.
	var containerOuterWidth float32
	if known.Width.Valid {
		containerOuterWidth = known.Width.Value
	} else {
		availableWidth := available.Width.subF(rectHSum(contentBoxInset))
		intrinsicWidth := blockContentBasedWidth(items, availableWidth) + rectHSum(contentBoxInset)
		containerOuterWidth = fmax(fclamp(intrinsicWidth, minSize.Width, maxSize.Width), paddingBorderSize.Width)
	}

	// Short-circuit if computing size and both dimensions are known.
	if runMode == computeSize && known.Height.Valid {
		return outputFromOuterSize(Size[float32]{containerOuterWidth, known.Height.Value})
	}

	// Also if the width is known and only the width is requested.
	if runMode == computeSize && in.axis == requestHorizontal {
		return outputFromOuterSize(Size[float32]{Width: containerOuterWidth})
	}

	// In content-size mode the container's own height and min-height are
	// ignored, so they cannot be the percentage basis for children either
	// (https://drafts.csswg.org/css-flexbox/#min-size-auto: the content size
	// suggestion must not be influenced by the item's specified height).
	containerPercentageHeight := percentageBasis.Height
	if !containerPercentageHeight.Valid && in.sizingMode == inherentSize {
		containerPercentageHeight = size.Height.max(minSize.Height)
	}

	// 3. Perform final item layout and return the content height.
	//
	// Percentage padding and borders resolve against the containing block's
	// width (parentSize), not the box's own width. These only differ when
	// the box has a non-stretch width. Fall back to the box's own width when
	// the parent size is unknown (at the root of the tree).
	percentageWidth := parentSize.Width.Or(containerOuterWidth)
	resolvedPadding := rectResolveOrZero(rawPadding, Some(percentageWidth))
	resolvedBorder := rectResolveOrZero(rawBorder, Some(percentageWidth))
	resolvedContentBoxInset := rectAdd(rectAdd(resolvedPadding, resolvedBorder), scrollbarGutter)
	flow := blockFinalLayout(runMode, items, containerOuterWidth, containerPercentageHeight,
		resolvedContentBoxInset, resolvedBorder, textAlign, justifyItems, direction, ownMarginsCollapseWithChildren,
		childrenMarginsAdjoinOwnEdges, isScrollContainer)
	inflowOverflow, intrinsicOuterHeight, firstBaseline := flow.overflow, flow.contentHeight, flow.firstBaseline

	var containerOuterHeight float32
	if known.Height.Valid {
		containerOuterHeight = known.Height.Value
	} else {
		containerOuterHeight = fclamp(intrinsicOuterHeight, minSize.Height, maxSize.Height)
	}
	containerOuterHeight = fmax(containerOuterHeight, paddingBorderSize.Height)
	finalOuterSize := Size[float32]{containerOuterWidth, containerOuterHeight}

	// CSS2 §8.3.1: the bottom margin of a block with height: auto collapses
	// with its last in-flow child's bottom margin only if the box's
	// min-height is less than its used height. When min-height determines
	// the used height, the last child's bottom margin no longer adjoins the
	// box's bottom edge, so it stays inside the box instead of collapsing
	// with the box's own bottom margin. (max-height has no such effect.)
	heightConstrainedByMinHeight := minSize.Height.Valid && minSize.Height.Value > 0 &&
		minSize.Height.Value >= containerOuterHeight
	ownBottomMarginCollapsesWithChildren := ownMarginsCollapseWithChildren.End && !heightConstrainedByMinHeight

	// Apply align-content to in-flow items if requested. Their layouts were
	// held back so that this step can shift them before they are committed.
	//
	// Block layout treats the whole stack of in-flow children as a single
	// alignment subject, so the distribution keywords (space-between,
	// space-around, space-evenly, stretch) always take the single-subject
	// fallback, which passing one item to applyAlignmentFallback does. The
	// group then shifts by one offset, with no gaps between items.
	if alignContent.Keyword != ContentNormal {
		containerInnerHeight := containerOuterHeight - rectVSum(resolvedContentBoxInset)
		inflowContentHeight := intrinsicOuterHeight - rectVSum(resolvedContentBoxInset)
		freeSpace := containerInnerHeight - inflowContentHeight
		groupOffset := computeBlockAlignContentOffset(alignContent, freeSpace)
		if firstBaseline.Valid {
			firstBaseline.Value += groupOffset
		}
		anyInFlow := false
		for i := range items {
			item := &items[i]
			if item.hasFinalLayout {
				item.finalLayout.Location.Y += groupOffset
				anyInFlow = true
			}
			// The static positions of out-of-flow children move with the
			// in-flow content.
			if item.position.isOutOfFlow() {
				item.staticPosition.Y += groupOffset
			}
		}
		if anyInFlow {
			inflowOverflow = Rect[float32]{}
			for i := range items {
				item := &items[i]
				if !item.hasFinalLayout {
					continue
				}
				l := &item.finalLayout
				loc := Point[float32]{l.Location.X - resolvedBorder.Left, l.Location.Y - resolvedBorder.Top}
				if direction.isRTL() {
					loc.X = containerOuterWidth - (l.Location.X + l.Size.Width) - resolvedBorder.Right
				}
				inflowOverflow = rectUnion(inflowOverflow, scrollableOverflowContribution(loc, l.Size,
					l.ScrollableOverflow, item.overflow, item.contain, isScrollContainer))
			}
		}
	}

	// Determine whether this node can be collapsed through.
	allInFlowCollapseThrough := true
	for i := range items {
		if !items[i].position.isOutOfFlow() && !items[i].canBeCollapsedThrough {
			allInFlowCollapseThrough = false
			break
		}
	}
	// CSS2 §8.3.1: a box's top and bottom margins collapse through it only if
	// its used height is zero, so height, min-height and max-height are
	// judged by the height they resolve to (height: 1px with max-height: 0
	// collapses, min-height: 1% under an auto-height parent collapses) rather
	// than by their computed values.
	canBeCollapsedThrough := !preventsCollapseThrough && containerOuterHeight == 0 && allInFlowCollapseThrough

	output := layoutOutput{
		size:                      finalOuterSize,
		baselines:                 baselines{first: firstBaseline},
		marginsCanCollapseThrough: canBeCollapsedThrough,
	}
	if ownMarginsCollapseWithChildren.Start {
		output.topMargin = flow.firstChildTopMargin
	} else {
		output.topMargin = marginSetFrom(rawMargin.Top.resolveOrZero(parentSize.Width))
	}
	if ownBottomMarginCollapsesWithChildren {
		output.bottomMargin = flow.lastChildBottomMargin
	} else {
		output.bottomMargin = marginSetFrom(rawMargin.Bottom.resolveOrZero(parentSize.Width))
	}

	// Short-circuit if computing size. The margin-collapsing outputs matter
	// here: parent block containers use their children's top and bottom
	// margins to compute their own intrinsic height.
	if runMode == computeSize {
		return output
	}

	// Commit the deferred child layouts.
	for i := range items {
		if items[i].hasFinalLayout {
			items[i].node.LayoutState().Unrounded = items[i].finalLayout
		}
	}

	// 4. Collect out-of-flow candidates in document order: direct
	// out-of-flow children, and candidates bubbled from in-flow children's
	// subtrees. The out-of-flow pass (computeOofLayout) lays them out after
	// this algorithm.
	var candidates []oofCandidate
	for i := range items {
		item := &items[i]
		if item.position.isOutOfFlow() {
			// The static-position rectangle of an out-of-flow child of a block
			// container spans the container's content box on the inline axis,
			// and is a zero-height line at the child's hypothetical position on
			// the block axis. The child's self-alignment aligns it within that
			// rectangle (https://www.w3.org/TR/css-position-3/#staticpos-rect).
			inlineArea := Line[float32]{resolvedContentBoxInset.Left, containerOuterWidth - resolvedContentBoxInset.Right}
			blockArea := Line[float32]{item.staticPosition.Y, item.staticPosition.Y}
			// An auto justify-self takes the container's justify-items. Block
			// containers have no align-items, so an auto align-self behaves as
			// normal.
			justifySelf := blockSelfOr(item.justifySelf, justifyItems).resolveSelfRelative(item.direction, direction, true)
			alignSelf := blockSelfOr(item.alignSelf, alignOf(AlignNormal)).resolveSelfRelative(item.direction, direction, false)
			candidates = append(candidates, oofCandidate{
				node:     item.node,
				order:    item.order,
				position: item.position,
				staticPosition: Point[axisStaticPosition]{
					X: staticPositionFromAlignment(justifySelf, inlineArea, direction.isRTL()),
					Y: staticPositionFromAlignment(alignSelf, blockArea, false),
				},
			})
		} else if len(item.oofCandidates) > 0 {
			// Translate anchors from item-relative to container-relative
			// coordinates.
			if item.hasFinalLayout {
				translateCandidates(item.oofCandidates, item.finalLayout.Location)
			}
			candidates = append(candidates, item.oofCandidates...)
		}
	}

	positionInset := rectAdd(resolvedBorder, scrollbarGutter)
	output.oofCandidates = candidates
	output.oofArea = oofPositioningArea{
		size:   sizeSub(finalOuterSize, rectSumAxes(positionInset)),
		offset: Point[float32]{positionInset.Left, positionInset.Top},
	}
	output.hasOofArea = true

	// A scroll container's own end padding is part of its scrollable
	// overflow, so it extends the in-flow overflow rect. Boxes that are not
	// scroll containers do not extend their overflow by their padding.
	if isScrollContainer {
		if direction.isRTL() {
			inflowOverflow.Right += resolvedPadding.Left
		} else {
			inflowOverflow.Right += resolvedPadding.Right
		}
		inflowOverflow.Bottom += resolvedPadding.Bottom
	}
	output.scrollableOverflow = inflowOverflow

	// 5. Perform hidden layout on hidden children.
	for i, count := 0, n.ChildCount(); i < count; i++ {
		child := n.Child(i)
		if child.Style().Display == DisplayNone {
			child.LayoutState().Unrounded = Layout{Order: uint32(i)}
			performChildLayout(child, noneSize, noneSize, maxContentSpaces, inherentSize, Line[bool]{})
		}
	}

	return output
}

// blockSelfOr returns a self-alignment, or def when it is auto.
func blockSelfOr(a, def Align) Align {
	if a.Keyword == AlignAuto {
		return def
	}
	return a
}

// blockGenerateItems makes a blockItem for each generated child, in order.
func blockGenerateItems(n Node, innerSize Size[Opt]) []blockItem {
	count := n.ChildCount()
	items := make([]blockItem, 0, count)
	for i := 0; i < count; i++ {
		child := n.Child(i)
		cs := child.Style()
		if cs.Display == DisplayNone {
			continue
		}
		aspectRatio := cs.AspectRatio
		padding := rectResolveOrZero(cs.padding(), innerSize.Width)
		border := rectResolveOrZero(cs.border(), innerSize.Width)
		pbSum := rectSumAxes(rectAdd(padding, border))
		var boxSizingAdjustment Size[float32]
		if cs.BoxSizing == ContentBox {
			boxSizingAdjustment = pbSum
		}

		position := cs.Position
		overflow := cs.Overflow
		isTable := cs.ItemIsTable
		isScrollContainer := overflow.X.isScrollContainer() || overflow.Y.isScrollContainer()
		contain := cs.Contain

		isInSameBFC := cs.isBlock() && !isTable && !position.isOutOfFlow() && !isScrollContainer &&
			!contain.establishesIndependentFormattingContext() && cs.AlignContent.Keyword == ContentNormal

		items = append(items, blockItem{
			node:                   child,
			order:                  uint32(len(items)),
			isTable:                isTable,
			isCompressibleReplaced: cs.isCompressibleReplaced(),
			isInSameBFC:            isInSameBFC,
			sizeStyle:              cs.Size,
			size: optsAddF(optsAspectRatio(sizeMaybeResolve(cs.Size, innerSize), aspectRatio),
				boxSizingAdjustment),
			minSize: optsAddF(optsAspectRatio(sizeMaybeResolve(cs.MinSize, innerSize), aspectRatio),
				boxSizingAdjustment),
			maxSize: optsAddF(optsAspectRatio(sizeMaybeResolve(cs.MaxSize, innerSize), aspectRatio),
				boxSizingAdjustment),
			overflow:         overflow,
			contain:          contain,
			scrollbarWidth:   cs.ScrollbarWidth,
			position:         position,
			direction:        cs.Direction,
			alignSelf:        cs.AlignSelf,
			justifySelf:      cs.JustifySelf,
			inset:            cs.Inset,
			margin:           cs.margin(),
			padding:          padding,
			border:           border,
			paddingBorderSum: pbSum,
		})
	}
	return items
}

// blockCollapsible is the margin collapsibility a block item is laid out
// with: an item in its parent's formatting context may collapse margins
// with its children. Measurements must use the same value as the final
// layout, as the cache keeps results of one collapsibility only.
func blockCollapsible(item *blockItem) Line[bool] {
	return Line[bool]{item.isInSameBFC, item.isInSameBFC}
}

// blockResolveStretchHeight resolves the stretch keyword of an item's height
// style. On the block axis the intrinsic sizing keywords all equal the
// content size, which an auto height already resolves to, so only stretch
// needs resolving.
//
// Each of the item's block-axis margins is subtracted from the stretch size
// unless flagged in marginsTreatedAsZero: when stretching a block-level box,
// a margin that could collapse through the corresponding edge of the
// containing block (which has neither border nor padding on that side and
// establishes no new formatting context) counts as zero
// (https://github.com/w3c/csswg-drafts/issues/11044#issuecomment-2599101601,
// https://github.com/w3c/csswg-drafts/issues/13260).
func blockResolveStretchHeight(heightStyle Length, containerInnerHeight Opt, itemMargin Rect[float32],
	marginsTreatedAsZero Line[bool]) Opt {
	if !heightStyle.isStretch() || !containerInnerHeight.Valid {
		return None
	}
	height := containerInnerHeight.Value
	if !marginsTreatedAsZero.Start {
		height -= itemMargin.Top
	}
	if !marginsTreatedAsZero.End {
		height -= itemMargin.Bottom
	}
	return Some(height)
}

// blockContentBasedWidth computes the container's content-based width when
// its width is not known.
func blockContentBasedWidth(items []blockItem, availableWidth AvailableSpace) float32 {
	available := Size[AvailableSpace]{availableWidth, MinContentSpace}

	var maxChildWidth float32
	for i := range items {
		item := &items[i]
		if item.position.isOutOfFlow() {
			continue
		}
		known := optsClamp(item.size, item.minSize, item.maxSize)

		itemXMarginSum := rectHSum(rectResolveOrZero(item.margin, available.Width.intoOption()))
		var width float32
		if known.Width.Valid {
			width = known.Width.Value
		} else {
			itemAvailableWidth := available.Width.subF(itemXMarginSum)
			if r, ok := resolveSizingKeyword(item.sizeStyle.Width, None, None); ok {
				if r.measure {
					itemAvailableWidth = r.available
				} else {
					itemAvailableWidth = Definite(r.exact)
				}
			}
			width = measureChildSize(item.node, known, noneSize,
				Size[AvailableSpace]{itemAvailableWidth, available.Height}, inherentSize, Horizontal,
				blockCollapsible(item))
		}

		width = fmax(width, item.paddingBorderSum.Width) + itemXMarginSum
		maxChildWidth = fmax(maxChildWidth, width)
	}
	return maxChildWidth
}

// blockFlow is the result of blockFinalLayout.
type blockFlow struct {
	overflow              Rect[float32]
	contentHeight         float32
	firstChildTopMargin   collapsibleMarginSet
	lastChildBottomMargin collapsibleMarginSet
	firstBaseline         Opt
}

// blockFinalLayout computes each child's final size and position: Taffy's
// perform_final_layout_on_in_flow_children.
func blockFinalLayout(runMode runMode, items []blockItem, containerOuterWidth float32,
	containerPercentageHeight Opt, resolvedContentBoxInset, resolvedBorder Rect[float32],
	textAlign TextAlign, justifyItems Align, direction Direction, ownMarginsCollapseWithChildren,
	childrenMarginsAdjoinOwnEdges Line[bool], isScrollContainer bool) blockFlow {
	// Resolve the inner width for sizing children from the initial content
	// box inset.
	containerInnerWidth := fmax(containerOuterWidth-rectHSum(resolvedContentBoxInset), 0)
	containerPercentageHeight = containerPercentageHeight.subF(rectVSum(resolvedContentBoxInset))
	parentSize := Size[Opt]{Some(containerInnerWidth), containerPercentageHeight}
	// The vertical available space in block flow is indefinite, not a
	// min-content constraint: max-content is Taffy's representation of
	// indefinite. A min-content constraint would make every descendant grid
	// believe it was sized under one, in which the maximize-tracks step has
	// no free space, collapsing auto rows that hold only scroll containers
	// (whose automatic minimum size is 0). Browsers size such rows to the
	// item's content.
	available := Size[AvailableSpace]{Definite(containerInnerWidth), MaxContentSpace}

	var inflowOverflow Rect[float32]
	committedYOffset := resolvedContentBoxInset.Top
	yOffsetForAbsolute := resolvedContentBoxInset.Top
	var firstChildTopMarginSet, activeMarginSet collapsibleMarginSet
	isCollapsingWithFirstMarginSet := true
	var firstBaseline Opt
	// Whether the active margin set holds the margins of a self-collapsing
	// element with clearance. Such margins collapse with following
	// siblings' margins, but the result does not collapse with the parent's
	// bottom margin. Without floats there is no clearance.
	activeMarginSetHasClearance := false
	const hasActiveFloats = false
	clearPos := negInf32

	for i := range items {
		item := &items[i]
		if item.position.isOutOfFlow() {
			x := resolvedContentBoxInset.Left
			if direction.isRTL() {
				x = containerOuterWidth - resolvedContentBoxInset.Right
			}
			item.staticPosition = Point[float32]{x, yOffsetForAbsolute}
			continue
		}

		itemMargin := Rect[Opt]{
			Left:   item.margin.Left.resolveToOption(containerInnerWidth),
			Right:  item.margin.Right.resolveToOption(containerInnerWidth),
			Top:    item.margin.Top.resolveToOption(containerInnerWidth),
			Bottom: item.margin.Bottom.resolveToOption(containerInnerWidth),
		}
		itemNonAutoMargin := rectUnwrapOr(itemMargin, 0)
		itemNonAutoXMarginSum := rectHSum(itemNonAutoMargin)

		var scrollbarSize Size[float32]
		if item.overflow.Y == OverflowScroll {
			scrollbarSize.Width = item.scrollbarWidth
		}
		if item.overflow.X == OverflowScroll {
			scrollbarSize.Height = item.scrollbarWidth
		}

		var yMarginOffset float32

		var stretchWidth, floatAvoidingWidth float32
		var floatAvoidingPosition Point[float32]
		if item.isInSameBFC {
			stretchWidth = fmax(containerInnerWidth-itemNonAutoXMarginSum, 0)
		} else {
			// Set the margin offset of a child in another formatting context.
			if !isCollapsingWithFirstMarginSet || !ownMarginsCollapseWithChildren.Start {
				yMarginOffset = activeMarginSet.collapseWithMargin(itemNonAutoMargin.Top).resolve()
			}
			minY := committedYOffset + yMarginOffset
			if !hasActiveFloats {
				stretchWidth = fmax(containerInnerWidth-itemNonAutoXMarginSum, 0)
				floatAvoidingPosition = Point[float32]{resolvedContentBoxInset.Left, minY}
				floatAvoidingWidth = containerInnerWidth
			}
		}

		// justify-self on an in-flow block-level box (css-align-3 §6.1.1).
		// Auto takes the container's justify-items, and normal lays the box
		// out by the default block layout rules. So does stretch: the default
		// rules already stretch an auto-width box, so it only differs from
		// normal for tables and replaced boxes, which would otherwise resolve
		// their own width.
		justifySelf := blockSelfOr(item.justifySelf, justifyItems).resolveSelfRelative(item.direction, direction, true)
		isStretch := justifySelf.Keyword == AlignStretch
		// hasNonStretchJustifySelf says whether justify-self is neither normal
		// nor stretch.
		hasNonStretchJustifySelf := justifySelf.Keyword != AlignNormal && justifySelf.Keyword != AlignStretch

		// Unless stretched, tables and compressible replaced elements resolve
		// their own width
		// (https://www.w3.org/TR/CSS22/visudet.html#block-replaced-width).
		var known Size[Opt]
		if !((item.isTable || item.isCompressibleReplaced) && !isStretch) {
			// The automatic width of a block-level box whose justify-self is
			// neither normal nor stretch is fit-content rather than stretch.
			widthStyle := item.sizeStyle.Width
			if hasNonStretchJustifySelf && item.sizeStyle.Width.IsAuto() {
				widthStyle = FitContent
			}

			// Items with a sizing keyword width resolve it directly or by
			// measuring the item under the corresponding constraint.
			var keywordWidth Opt
			if r, ok := resolveSizingKeyword(widthStyle, Some(stretchWidth), Some(containerInnerWidth)); ok {
				if r.measure {
					keywordWidth = Some(measureChildSize(item.node, noneSize, parentSize,
						Size[AvailableSpace]{r.available, MaxContentSpace}, inherentSize, Horizontal,
						blockCollapsible(item)))
				} else {
					keywordWidth = Some(r.exact)
				}
			}

			keywordHeight := blockResolveStretchHeight(item.sizeStyle.Height, containerPercentageHeight,
				itemNonAutoMargin, childrenMarginsAdjoinOwnEdges)

			known = Size[Opt]{
				Width:  Some(fclamp(item.size.Width.OrOpt(keywordWidth).Or(stretchWidth), item.minSize.Width, item.maxSize.Width)),
				Height: item.size.Height.OrOpt(keywordHeight),
			}
			known = optsClamp(known, item.minSize, item.maxSize)
		}

		inputs := layoutInput{
			runMode:                       runMode,
			sizingMode:                    inherentSize,
			axis:                          requestBoth,
			knownDimensions:               known,
			knownDimensionsAreDefinite:    Size[bool]{true, true},
			parentSize:                    parentSize,
			availableSpace:                Size[AvailableSpace]{Definite(stretchWidth), available.Height},
			verticalMarginsAreCollapsible: blockCollapsible(item),
		}

		var itemLayout layoutOutput
		if item.isInSameBFC {
			itemLayout = computeNodeLayout(item.node, inputs, &blockSubContext)
		} else {
			itemLayout = computeChildLayout(item.node, inputs)
		}
		item.oofCandidates = itemLayout.oofCandidates
		itemLayout.oofCandidates = nil
		finalSize := itemLayout.size

		topMarginSet := itemLayout.topMargin.collapseWithMargin(itemMargin.Top.Or(0))
		bottomMarginSet := itemLayout.bottomMargin.collapseWithMargin(itemMargin.Bottom.Or(0))

		// Expand auto margins to fill the available space. Vertical auto
		// margins of relatively positioned block items resolve to 0
		// (https://www.w3.org/TR/CSS21/visudet.html#abs-non-replaced-width).
		freeXSpace := fmax(0, stretchWidth-finalSize.Width)
		var xAxisAutoMarginSize float32
		autoMarginCount := 0
		if !itemMargin.Left.Valid {
			autoMarginCount++
		}
		if !itemMargin.Right.Valid {
			autoMarginCount++
		}
		if autoMarginCount > 0 {
			xAxisAutoMarginSize = freeXSpace / float32(autoMarginCount)
		}
		resolvedMargin := Rect[float32]{
			Left:   itemMargin.Left.Or(xAxisAutoMarginSize),
			Right:  itemMargin.Right.Or(xAxisAutoMarginSize),
			Top:    itemMargin.Top.Or(0),
			Bottom: itemMargin.Bottom.Or(0),
		}

		// Resolve the item's inset.
		inset := Rect[Opt]{
			Left:   item.inset.Left.maybeResolve(Some(containerInnerWidth)),
			Right:  item.inset.Right.maybeResolve(Some(containerInnerWidth)),
			Top:    item.inset.Top.maybeResolve(containerPercentageHeight),
			Bottom: item.inset.Bottom.maybeResolve(containerPercentageHeight),
		}
		var insetOffset Point[float32]
		if item.position == PositionRelative {
			negRight := Opt{-inset.Right.Value, inset.Right.Valid}
			if direction.isRTL() {
				insetOffset.X = negRight.OrOpt(inset.Left).Or(0)
			} else {
				insetOffset.X = inset.Left.OrOpt(negRight).Or(0)
			}
			insetOffset.Y = inset.Top.OrOpt(Opt{-inset.Bottom.Value, inset.Bottom.Valid}).Or(0)
		}

		// Set the margin offset of a child in the same formatting context.
		if item.isInSameBFC && (!isCollapsingWithFirstMarginSet || !ownMarginsCollapseWithChildren.Start) {
			yMarginOffset = activeMarginSet.collapseWithSet(topMarginSet).resolve()
		}

		// Without floats there is no clearance.
		const hasClearance = false

		item.computedSize = itemLayout.size
		item.canBeCollapsedThrough = itemLayout.marginsCanCollapseThrough && !hasClearance
		if item.isInSameBFC {
			unclearedY := committedYOffset + activeMarginSet.resolve()
			x := resolvedContentBoxInset.Left
			if direction.isRTL() {
				x = containerOuterWidth - resolvedContentBoxInset.Right - finalSize.Width
			}
			item.staticPosition = Point[float32]{x, fmax(unclearedY, clearPos)}
		} else {
			x := floatAvoidingPosition.X
			if direction.isRTL() {
				x = floatAvoidingPosition.X + floatAvoidingWidth - finalSize.Width
			}
			item.staticPosition = Point[float32]{x, floatAvoidingPosition.Y}
		}
		var location Point[float32]
		if item.isInSameBFC {
			if direction.isRTL() {
				location.X = containerOuterWidth - resolvedContentBoxInset.Right - finalSize.Width -
					resolvedMargin.Right + insetOffset.X
			} else {
				location.X = resolvedContentBoxInset.Left + insetOffset.X + resolvedMargin.Left
			}
			location.Y = committedYOffset + yMarginOffset + insetOffset.Y
		} else {
			if direction.isRTL() {
				location.X = floatAvoidingPosition.X + floatAvoidingWidth - finalSize.Width - resolvedMargin.Right +
					insetOffset.X
			} else {
				location.X = floatAvoidingPosition.X + resolvedMargin.Left + insetOffset.X
			}
			location.Y = floatAvoidingPosition.Y + insetOffset.Y
		}

		// Apply alignment. justify-self aligns the item's margin box within
		// its alignment container and takes precedence over legacy
		// text-align, but auto margins (which have absorbed the free space)
		// take precedence over justify-self.
		hasAutoXMargin := !itemMargin.Left.Valid || !itemMargin.Right.Valid
		itemOuterWidth := itemLayout.size.Width + rectHSum(resolvedMargin)
		if hasNonStretchJustifySelf && !hasAutoXMargin {
			containerStart := resolvedContentBoxInset.Left
			freeX := containerInnerWidth - itemOuterWidth
			keyword := justifySelf.Keyword
			if justifySelf.Safety == Safe && freeX < 0 {
				keyword = AlignStart
			}
			var offset float32
			switch keyword {
			case AlignCenter:
				offset = freeX / 2
			case AlignEnd, AlignFlexEnd, AlignSelfEnd:
				if !direction.isRTL() {
					offset = freeX
				}
			default:
				if direction.isRTL() {
					offset = freeX
				}
			}
			location.X = containerStart + resolvedMargin.Left + insetOffset.X + offset
		} else if !hasNonStretchJustifySelf && itemOuterWidth < containerInnerWidth {
			freeX := containerInnerWidth - itemOuterWidth
			rtl := direction.isRTL()
			switch textAlign {
			case TextAlignLegacyLeft:
				// Left aligned by default in LTR.
				if rtl {
					location.X -= freeX
				}
			case TextAlignLegacyRight:
				// Right aligned by default in RTL.
				if !rtl {
					location.X += freeX
				}
			case TextAlignLegacyCenter:
				if rtl {
					location.X -= freeX / 2
				} else {
					location.X += freeX / 2
				}
			}
		}

		// A block container's first baseline is the first baseline of its
		// first in-flow child that has one.
		//
		// A scroll container's baseline is determined from its content as if
		// scrolled to the initial position, but is also clamped to its border
		// box; one with no baseline synthesizes one from its border-box
		// bottom edge (https://github.com/w3c/csswg-drafts/issues/7660).
		if !firstBaseline.Valid {
			childBaseline := itemLayout.baselines.first
			if item.overflow.Y.isScrollContainer() {
				childBaseline = Some(fmax(fmin(itemLayout.baselines.first.Or(itemLayout.size.Height),
					itemLayout.size.Height), 0))
			}
			if childBaseline.Valid {
				firstBaseline = Some(location.Y + childBaseline.Value)
			}
		}

		// The layout is committed after the loop, so that align-content can
		// shift it first.
		item.finalLayout = Layout{
			Order:              item.order,
			Size:               itemLayout.size,
			ScrollableOverflow: itemLayout.scrollableOverflow,
			ScrollbarSize:      scrollbarSize,
			Location:           location,
			Padding:            item.padding,
			Border:             item.border,
			Margin:             resolvedMargin,
		}
		item.hasFinalLayout = true

		contributionLocation := Point[float32]{location.X - resolvedBorder.Left, location.Y - resolvedBorder.Top}
		if direction.isRTL() {
			contributionLocation.X = containerOuterWidth - (location.X + finalSize.Width) - resolvedBorder.Right
		}
		inflowOverflow = rectUnion(inflowOverflow, scrollableOverflowContribution(contributionLocation, finalSize,
			itemLayout.scrollableOverflow, item.overflow, item.contain, isScrollContainer))

		// Update the first child's top margin set. The top margin of an item
		// with clearance does not collapse with the container's top margin,
		// so clearance ends collapsing without adding the item's margins.
		if isCollapsingWithFirstMarginSet && hasClearance {
			isCollapsingWithFirstMarginSet = false
		} else if isCollapsingWithFirstMarginSet {
			if item.canBeCollapsedThrough {
				firstChildTopMarginSet = firstChildTopMarginSet.collapseWithSet(topMarginSet).
					collapseWithSet(bottomMarginSet)
			} else {
				firstChildTopMarginSet = firstChildTopMarginSet.collapseWithSet(topMarginSet)
				isCollapsingWithFirstMarginSet = false
			}
		}

		// Update the active collapsible margin set.
		if item.canBeCollapsedThrough {
			activeMarginSet = activeMarginSet.collapseWithSet(topMarginSet).collapseWithSet(bottomMarginSet)
			yOffsetForAbsolute = committedYOffset + itemLayout.size.Height + yMarginOffset
		} else {
			committedYOffset = location.Y - insetOffset.Y + itemLayout.size.Height
			// A self-collapsing item with clearance is not collapsed through
			// (its margins don't collapse with preceding siblings'), but its
			// top and bottom margins still collapse with each other and with
			// following siblings'.
			if hasClearance && itemLayout.marginsCanCollapseThrough {
				// Its border edge stays at the cleared position, but its
				// collapsed margin extends below it: the border edge sits top
				// margin inside the collapsed margin, so following content is
				// offset by the collapsed margin less the top margin.
				committedYOffset -= topMarginSet.resolve()
				activeMarginSet = topMarginSet.collapseWithSet(bottomMarginSet)
				activeMarginSetHasClearance = true
			} else {
				activeMarginSet = bottomMarginSet
				activeMarginSetHasClearance = false
			}
			yOffsetForAbsolute = committedYOffset + activeMarginSet.resolve()
		}
	}

	// The margins of a self-collapsing element with clearance do not
	// collapse with the parent's bottom margin: they extend the parent's
	// content height instead of escaping it.
	lastChildBottomMarginSet := activeMarginSet
	if activeMarginSetHasClearance {
		lastChildBottomMarginSet = collapsibleMarginSet{}
	}
	var bottomYMarginOffset float32
	switch {
	case activeMarginSetHasClearance:
		bottomYMarginOffset = activeMarginSet.resolve()
	case ownMarginsCollapseWithChildren.End:
		bottomYMarginOffset = 0
	default:
		bottomYMarginOffset = lastChildBottomMarginSet.resolve()
	}

	committedYOffset += resolvedContentBoxInset.Bottom + bottomYMarginOffset
	return blockFlow{
		overflow:              inflowOverflow,
		contentHeight:         fmax(0, committedYOffset),
		firstChildTopMargin:   firstChildTopMarginSet,
		lastChildBottomMargin: lastChildBottomMarginSet,
		firstBaseline:         firstBaseline,
	}
}
