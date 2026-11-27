package core

import (
	"hash/maphash"
	"maps"
	"slices"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/layout"
)

// Read-only sentinels shared by nodes whose fields are at their defaults so
// reads never nil-check and plain views allocate nothing extra.
var (
	zeroStyle        Style
	zeroViewProps    driver.ViewProps
	defaultTextProps = driver.ViewProps{Role: RoleText}
	defaultViewStyle = driver.DefaultViewStyle
	noHandlers       handlers
	zeroExtra        nodeExtra
)

const internCacheSize = 1024

// styleIntern shares immutable *Style values across nodes in one engine using
// two bounded generations, as parseCache and measureCache do.
type styleIntern struct {
	seed     maphash.Seed
	now, old map[uint64][]*Style
	nowCount int
}

func (c *styleIntern) intern(s *Style) *Style {
	if s == nil || isZeroStyle(s) {
		return &zeroStyle
	}
	// Styles carrying per-node AnimatedStyle or CustomCursor buffers stay private.
	if s.Animated != nil || s.CursorImage != nil {
		cp := *s
		return &cp
	}
	if c.seed == (maphash.Seed{}) {
		c.seed = maphash.MakeSeed()
	}
	h := hashStyle(c.seed, s)
	if bucket := c.now[h]; len(bucket) > 0 {
		for _, cand := range bucket {
			if sameStyle(cand, s) {
				return cand
			}
		}
	}
	if bucket := c.old[h]; len(bucket) > 0 {
		for _, cand := range bucket {
			if sameStyle(cand, s) {
				c.put(h, cand)
				return cand
			}
		}
	}
	stored := c.cloneImmutable(s)
	c.put(h, stored)
	return stored
}

func (c *styleIntern) put(h uint64, s *Style) {
	if c.now == nil {
		c.now = make(map[uint64][]*Style)
	}
	if c.nowCount >= internCacheSize {
		c.old, c.now = c.now, c.old
		if c.now == nil {
			c.now = make(map[uint64][]*Style)
		} else {
			clear(c.now)
		}
		c.nowCount = 0
	}
	c.now[h] = append(c.now[h], s)
	c.nowCount++
}

// cloneImmutable copies s and detaches slices/nested pointers so external
// mutation after mounting can never corrupt a shared interned *Style.
func (c *styleIntern) cloneImmutable(s *Style) *Style {
	cp := *s
	if cp.Gradients != nil {
		gs := make([]Gradient, len(cp.Gradients))
		for i, g := range cp.Gradients {
			if g.Stops != nil {
				g.Stops = slices.Clone(g.Stops)
			}
			gs[i] = g
		}
		cp.Gradients = gs
	}
	if cp.Shadows != nil {
		cp.Shadows = slices.Clone(cp.Shadows)
	}
	if cp.Filters != nil {
		cp.Filters = slices.Clone(cp.Filters)
	}
	if cp.BackdropFilters != nil {
		cp.BackdropFilters = slices.Clone(cp.BackdropFilters)
	}
	if cp.Mask != nil {
		m := *cp.Mask
		if m.Stops != nil {
			m.Stops = slices.Clone(m.Stops)
		}
		cp.Mask = &m
	}
	if cp.Transforms != nil {
		cp.Transforms = slices.Clone(cp.Transforms)
	}
	if cp.StrokeDash != nil {
		cp.StrokeDash = slices.Clone(cp.StrokeDash)
	}
	if cp.FontAxes != nil {
		cp.FontAxes = maps.Clone(cp.FontAxes)
	}
	if cp.FontFeatures != nil {
		cp.FontFeatures = maps.Clone(cp.FontFeatures)
	}
	if cp.GroupStyle != nil {
		g := *cp.GroupStyle
		c.internStateSet(g.states())
		cp.GroupStyle = &g
	}
	c.internStateSet(cp.states())
	return &cp
}

func (c *styleIntern) internStateSet(st stateSet) {
	for i := range st.slots {
		if p := *st.slots[i]; p != nil {
			*st.slots[i] = c.intern(p)
		}
	}
	if *st.data != nil {
		ds := make([]DataStyle, len(*st.data))
		for i, d := range *st.data {
			if d.Style != nil {
				d.Style = c.intern(d.Style)
			}
			ds[i] = d
		}
		*st.data = ds
	}
}

func (rt *runtime) internStyle(s *Style) *Style {
	if rt == nil {
		if s == nil || isZeroStyle(s) {
			return &zeroStyle
		}
		cp := *s
		return &cp
	}
	return rt.styles.intern(s)
}

func (t *Tree) internStyle(s *Style) *Style {
	return t.rt.internStyle(s)
}

// isZeroStyle reports whether every field of s is at its zero value.
func isZeroStyle(s *Style) bool {
	return sameStyle(s, &zeroStyle)
}

// hasStateStyles reports whether s defines any state style, Data style, or GroupStyle.
func (s *Style) hasStateStyles() bool {
	return s.GroupStyle != nil || len(s.Data) > 0 ||
		s.Hover != nil || s.Focus != nil || s.Active != nil || s.Selected != nil ||
		s.Checked != nil || s.Indeterminate != nil || s.Disabled != nil ||
		s.FocusWithin != nil || s.AriaBusy != nil || s.AriaChecked != nil ||
		s.Expanded != nil || s.Pressed != nil || s.AriaSelected != nil ||
		s.FocusVisible != nil || s.AriaInvalid != nil
}

// hasZeroSentinel reports whether s holds the Zero sentinel in any field unzeroed clears.
func (s *Style) hasZeroSentinel() bool {
	return s.Margin.Top == Zero || s.Margin.Right == Zero || s.Margin.Bottom == Zero || s.Margin.Left == Zero ||
		s.Padding.Top == Zero || s.Padding.Right == Zero || s.Padding.Bottom == Zero || s.Padding.Left == Zero ||
		s.Borders.Top == Zero || s.Borders.Right == Zero || s.Borders.Bottom == Zero || s.Borders.Left == Zero ||
		s.Gap == Zero || s.RowGap == Zero || s.ColumnGap == Zero || s.BorderWidth == Zero || s.CornerRadius == Zero
}

// sameStyle reports whether a and b are identical in every field, preserving
// nil vs empty-slice distinctions (which Merge's setSlice relies on).
func sameStyle(a, b *Style) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if a.Display != b.Display || a.Position != b.Position || a.Direction != b.Direction || a.Wrap != b.Wrap ||
		a.JustifyContent != b.JustifyContent || a.AlignItems != b.AlignItems || a.AlignSelf != b.AlignSelf ||
		a.AlignContent != b.AlignContent || a.JustifyItems != b.JustifyItems || a.JustifySelf != b.JustifySelf ||
		a.FlexGrow != b.FlexGrow || a.FlexShrink != b.FlexShrink || a.FlexBasis != b.FlexBasis ||
		a.Width != b.Width || a.Height != b.Height || a.MinWidth != b.MinWidth || a.MinHeight != b.MinHeight ||
		a.MaxWidth != b.MaxWidth || a.MaxHeight != b.MaxHeight || a.AspectRatio != b.AspectRatio ||
		a.BoxSizing != b.BoxSizing || a.Top != b.Top || a.Right != b.Right || a.Bottom != b.Bottom || a.Left != b.Left ||
		a.RowByDefault != b.RowByDefault || a.PhysicalEdges != b.PhysicalEdges || a.LayoutDirection != b.LayoutDirection ||
		a.ZIndex != b.ZIndex || a.Margin != b.Margin || a.AutoMargin != b.AutoMargin || a.Padding != b.Padding ||
		a.MarginPercent != b.MarginPercent || a.PaddingPercent != b.PaddingPercent ||
		a.PaddingEnv != b.PaddingEnv || a.MarginEnv != b.MarginEnv ||
		a.Gap != b.Gap || a.RowGap != b.RowGap || a.ColumnGap != b.ColumnGap ||
		a.GridTemplateColumns != b.GridTemplateColumns || a.GridTemplateRows != b.GridTemplateRows ||
		a.GridAutoColumns != b.GridAutoColumns || a.GridAutoRows != b.GridAutoRows ||
		a.GridAutoFlow != b.GridAutoFlow || a.GridRow != b.GridRow || a.GridColumn != b.GridColumn {
		return false
	}
	if a.Background != b.Background || a.BackgroundInsetTop != b.BackgroundInsetTop || a.BackgroundInsetBottom != b.BackgroundInsetBottom ||
		a.CornerRadius != b.CornerRadius || a.CornerRadii != b.CornerRadii || a.BorderCurve != b.BorderCurve ||
		a.BorderWidth != b.BorderWidth || a.Borders != b.Borders || a.BorderColor != b.BorderColor ||
		a.BorderColors != b.BorderColors || a.PhysicalBorders != b.PhysicalBorders || a.BorderStyle != b.BorderStyle ||
		a.OutlineWidth != b.OutlineWidth || a.OutlineOffset != b.OutlineOffset || a.OutlineColor != b.OutlineColor ||
		a.OutlineStyle != b.OutlineStyle || a.Opacity != b.Opacity || a.Visibility != b.Visibility ||
		a.Overflow != b.Overflow || a.OverflowX != b.OverflowX || a.OverflowY != b.OverflowY ||
		a.ShadowColor != b.ShadowColor || a.ShadowOffset != b.ShadowOffset || a.ShadowRadius != b.ShadowRadius ||
		a.GlassEffect != b.GlassEffect || a.GlassInteractive != b.GlassInteractive ||
		a.BlendMode != b.BlendMode || a.TintColor != b.TintColor || a.Cursor != b.Cursor ||
		a.ObjectPosition != b.ObjectPosition || a.VerticalAlign != b.VerticalAlign ||
		a.Transform != b.Transform || a.TransformOrigin != b.TransformOrigin || a.TransformOriginZ != b.TransformOriginZ ||
		a.Perspective != b.Perspective || a.BackfaceHidden != b.BackfaceHidden ||
		a.Transition != b.Transition || a.Animation != b.Animation ||
		a.Animated != b.Animated || a.CursorImage != b.CursorImage {
		return false
	}
	if a.Color != b.Color || a.FontFamily != b.FontFamily || a.Language != b.Language ||
		a.FontSize != b.FontSize || a.FontWeight != b.FontWeight || a.FontStyle != b.FontStyle ||
		a.FontVariant != b.FontVariant || a.LineHeight != b.LineHeight || a.LineHeightMultiple != b.LineHeightMultiple ||
		a.LineHeightEm != b.LineHeightEm || a.LetterSpacing != b.LetterSpacing || a.LetterSpacingEm != b.LetterSpacingEm ||
		a.TextAlign != b.TextAlign || a.TextDecoration != b.TextDecoration || a.TextDecorationStyle != b.TextDecorationStyle ||
		a.TextDecorationColor != b.TextDecorationColor || a.TextDecorationThickness != b.TextDecorationThickness ||
		a.TextUnderlineOffset != b.TextUnderlineOffset || a.TextTransform != b.TextTransform ||
		a.TextShadowColor != b.TextShadowColor || a.TextShadowOffset != b.TextShadowOffset || a.TextShadowRadius != b.TextShadowRadius ||
		a.TextAlignVertical != b.TextAlignVertical || a.SelectionColor != b.SelectionColor ||
		a.CaretColor != b.CaretColor || a.PlaceholderColor != b.PlaceholderColor ||
		a.Fill != b.Fill || a.Stroke != b.Stroke || a.StrokeWidth != b.StrokeWidth ||
		a.WhiteSpace != b.WhiteSpace || a.UserSelect != b.UserSelect ||
		a.NumberOfLines != b.NumberOfLines || a.TextOverflow != b.TextOverflow ||
		a.PointerEvents != b.PointerEvents || a.ObjectFit != b.ObjectFit || a.WritingDirection != b.WritingDirection ||
		a.Group != b.Group {
		return false
	}
	if (a.Gradients == nil) != (b.Gradients == nil) || !slices.EqualFunc(a.Gradients, b.Gradients, gradientEqual) ||
		(a.Shadows == nil) != (b.Shadows == nil) || !slices.Equal(a.Shadows, b.Shadows) ||
		(a.Filters == nil) != (b.Filters == nil) || !slices.Equal(a.Filters, b.Filters) ||
		(a.BackdropFilters == nil) != (b.BackdropFilters == nil) || !slices.Equal(a.BackdropFilters, b.BackdropFilters) ||
		(a.Transforms == nil) != (b.Transforms == nil) || !slices.Equal(a.Transforms, b.Transforms) ||
		(a.StrokeDash == nil) != (b.StrokeDash == nil) || !slices.Equal(a.StrokeDash, b.StrokeDash) ||
		(a.FontAxes == nil) != (b.FontAxes == nil) || !maps.Equal(a.FontAxes, b.FontAxes) ||
		(a.FontFeatures == nil) != (b.FontFeatures == nil) || !maps.Equal(a.FontFeatures, b.FontFeatures) {
		return false
	}
	if a.Mask != b.Mask {
		if a.Mask == nil || b.Mask == nil || !gradientEqual(*a.Mask, *b.Mask) {
			return false
		}
	}
	if !groupStyleEqual(a.GroupStyle, b.GroupStyle) {
		return false
	}
	return sameStyle(a.Hover, b.Hover) && sameStyle(a.Focus, b.Focus) && sameStyle(a.Active, b.Active) &&
		sameStyle(a.Selected, b.Selected) && sameStyle(a.Checked, b.Checked) && sameStyle(a.Indeterminate, b.Indeterminate) &&
		sameStyle(a.Disabled, b.Disabled) && sameStyle(a.FocusWithin, b.FocusWithin) && sameStyle(a.AriaBusy, b.AriaBusy) &&
		sameStyle(a.AriaChecked, b.AriaChecked) && sameStyle(a.Expanded, b.Expanded) && sameStyle(a.Pressed, b.Pressed) &&
		sameStyle(a.AriaSelected, b.AriaSelected) && sameStyle(a.FocusVisible, b.FocusVisible) && sameStyle(a.AriaInvalid, b.AriaInvalid) &&
		dataStylesEqual(a.Data, b.Data)
}

func gradientEqual(a, b Gradient) bool {
	return a.Kind == b.Kind && a.Angle == b.Angle && a.ToCorner == b.ToCorner &&
		a.Circle == b.Circle && a.Extent == b.Extent && a.Center == b.Center &&
		a.Repeating == b.Repeating && a.Space == b.Space &&
		(a.Stops == nil) == (b.Stops == nil) && slices.Equal(a.Stops, b.Stops)
}

func groupStyleEqual(a, b *GroupStyle) bool {
	if a == b {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Name == b.Name &&
		sameStyle(a.Hover, b.Hover) && sameStyle(a.Focus, b.Focus) && sameStyle(a.Active, b.Active) &&
		sameStyle(a.Selected, b.Selected) && sameStyle(a.Checked, b.Checked) && sameStyle(a.Indeterminate, b.Indeterminate) &&
		sameStyle(a.Disabled, b.Disabled) && sameStyle(a.FocusWithin, b.FocusWithin) && sameStyle(a.AriaBusy, b.AriaBusy) &&
		sameStyle(a.AriaChecked, b.AriaChecked) && sameStyle(a.Expanded, b.Expanded) && sameStyle(a.Pressed, b.Pressed) &&
		sameStyle(a.AriaSelected, b.AriaSelected) && sameStyle(a.FocusVisible, b.FocusVisible) && sameStyle(a.AriaInvalid, b.AriaInvalid) &&
		dataStylesEqual(a.Data, b.Data)
}

func dataStylesEqual(a, b []DataStyle) bool {
	if (a == nil) != (b == nil) || len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Name != b[i].Name || !sameStyle(a[i].Style, b[i].Style) {
			return false
		}
	}
	return true
}

func hashStyle(seed maphash.Seed, s *Style) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	writeStyleHash(&h, s)
	return h.Sum64()
}

func writeStyleHash(h *maphash.Hash, s *Style) {
	if s == nil {
		h.WriteByte(0)
		return
	}
	h.WriteByte(1)
	maphash.WriteComparable(h, s.Display)
	maphash.WriteComparable(h, s.Position)
	maphash.WriteComparable(h, s.Direction)
	maphash.WriteComparable(h, s.Wrap)
	maphash.WriteComparable(h, s.JustifyContent)
	maphash.WriteComparable(h, s.AlignItems)
	maphash.WriteComparable(h, s.AlignSelf)
	maphash.WriteComparable(h, s.AlignContent)
	maphash.WriteComparable(h, s.JustifyItems)
	maphash.WriteComparable(h, s.JustifySelf)
	maphash.WriteComparable(h, s.FlexGrow)
	maphash.WriteComparable(h, s.FlexShrink)
	maphash.WriteComparable(h, s.FlexBasis)
	maphash.WriteComparable(h, s.Width)
	maphash.WriteComparable(h, s.Height)
	maphash.WriteComparable(h, s.MinWidth)
	maphash.WriteComparable(h, s.MinHeight)
	maphash.WriteComparable(h, s.MaxWidth)
	maphash.WriteComparable(h, s.MaxHeight)
	maphash.WriteComparable(h, s.AspectRatio)
	maphash.WriteComparable(h, s.BoxSizing)
	maphash.WriteComparable(h, s.Top)
	maphash.WriteComparable(h, s.Right)
	maphash.WriteComparable(h, s.Bottom)
	maphash.WriteComparable(h, s.Left)
	maphash.WriteComparable(h, s.RowByDefault)
	maphash.WriteComparable(h, s.PhysicalEdges)
	maphash.WriteComparable(h, s.LayoutDirection)
	maphash.WriteComparable(h, s.ZIndex)
	maphash.WriteComparable(h, s.Margin)
	maphash.WriteComparable(h, s.AutoMargin)
	maphash.WriteComparable(h, s.Padding)
	maphash.WriteComparable(h, s.MarginPercent)
	maphash.WriteComparable(h, s.PaddingPercent)
	maphash.WriteComparable(h, s.PaddingEnv)
	maphash.WriteComparable(h, s.MarginEnv)
	maphash.WriteComparable(h, s.Gap)
	maphash.WriteComparable(h, s.RowGap)
	maphash.WriteComparable(h, s.ColumnGap)
	h.WriteString(string(s.GridTemplateColumns))
	h.WriteString(string(s.GridTemplateRows))
	h.WriteString(string(s.GridAutoColumns))
	h.WriteString(string(s.GridAutoRows))
	maphash.WriteComparable(h, s.GridAutoFlow)
	maphash.WriteComparable(h, s.GridRow)
	maphash.WriteComparable(h, s.GridColumn)

	maphash.WriteComparable(h, s.Background)
	maphash.WriteComparable(h, s.BackgroundInsetTop)
	maphash.WriteComparable(h, s.BackgroundInsetBottom)
	maphash.WriteComparable(h, s.CornerRadius)
	maphash.WriteComparable(h, s.CornerRadii)
	maphash.WriteComparable(h, s.BorderCurve)
	maphash.WriteComparable(h, s.BorderWidth)
	maphash.WriteComparable(h, s.Borders)
	maphash.WriteComparable(h, s.BorderColor)
	maphash.WriteComparable(h, s.BorderColors)
	maphash.WriteComparable(h, s.PhysicalBorders)
	maphash.WriteComparable(h, s.BorderStyle)
	maphash.WriteComparable(h, s.OutlineWidth)
	maphash.WriteComparable(h, s.OutlineOffset)
	maphash.WriteComparable(h, s.OutlineColor)
	maphash.WriteComparable(h, s.OutlineStyle)
	maphash.WriteComparable(h, s.Opacity)
	maphash.WriteComparable(h, s.Visibility)
	maphash.WriteComparable(h, s.Overflow)
	maphash.WriteComparable(h, s.OverflowX)
	maphash.WriteComparable(h, s.OverflowY)
	maphash.WriteComparable(h, s.ShadowColor)
	maphash.WriteComparable(h, s.ShadowOffset)
	maphash.WriteComparable(h, s.ShadowRadius)
	maphash.WriteComparable(h, s.GlassEffect)
	maphash.WriteComparable(h, s.GlassInteractive)
	maphash.WriteComparable(h, s.BlendMode)
	maphash.WriteComparable(h, s.TintColor)
	maphash.WriteComparable(h, s.Cursor)
	maphash.WriteComparable(h, s.ObjectPosition)
	maphash.WriteComparable(h, s.VerticalAlign)
	maphash.WriteComparable(h, s.Transform)
	maphash.WriteComparable(h, s.TransformOrigin)
	maphash.WriteComparable(h, s.TransformOriginZ)
	maphash.WriteComparable(h, s.Perspective)
	maphash.WriteComparable(h, s.BackfaceHidden)
	maphash.WriteComparable(h, s.Transition)
	maphash.WriteComparable(h, s.Animation)

	maphash.WriteComparable(h, s.Color)
	h.WriteString(s.FontFamily)
	h.WriteString(s.Language)
	maphash.WriteComparable(h, s.FontSize)
	maphash.WriteComparable(h, s.FontWeight)
	maphash.WriteComparable(h, s.FontStyle)
	maphash.WriteComparable(h, s.FontVariant)
	maphash.WriteComparable(h, s.LineHeight)
	maphash.WriteComparable(h, s.LineHeightMultiple)
	maphash.WriteComparable(h, s.LineHeightEm)
	maphash.WriteComparable(h, s.LetterSpacing)
	maphash.WriteComparable(h, s.LetterSpacingEm)
	maphash.WriteComparable(h, s.TextAlign)
	maphash.WriteComparable(h, s.TextDecoration)
	maphash.WriteComparable(h, s.TextDecorationStyle)
	maphash.WriteComparable(h, s.TextDecorationColor)
	maphash.WriteComparable(h, s.TextDecorationThickness)
	maphash.WriteComparable(h, s.TextUnderlineOffset)
	maphash.WriteComparable(h, s.TextTransform)
	maphash.WriteComparable(h, s.TextShadowColor)
	maphash.WriteComparable(h, s.TextShadowOffset)
	maphash.WriteComparable(h, s.TextShadowRadius)
	maphash.WriteComparable(h, s.TextAlignVertical)
	maphash.WriteComparable(h, s.SelectionColor)
	maphash.WriteComparable(h, s.CaretColor)
	maphash.WriteComparable(h, s.PlaceholderColor)
	maphash.WriteComparable(h, s.Fill)
	maphash.WriteComparable(h, s.Stroke)
	maphash.WriteComparable(h, s.StrokeWidth)
	maphash.WriteComparable(h, s.WhiteSpace)
	maphash.WriteComparable(h, s.UserSelect)
	maphash.WriteComparable(h, s.NumberOfLines)
	maphash.WriteComparable(h, s.TextOverflow)
	maphash.WriteComparable(h, s.PointerEvents)
	maphash.WriteComparable(h, s.ObjectFit)
	maphash.WriteComparable(h, s.WritingDirection)
	h.WriteString(s.Group)

	if s.Gradients != nil {
		maphash.WriteComparable(h, len(s.Gradients)+1)
		for i := range s.Gradients {
			writeGradientHash(h, &s.Gradients[i])
		}
	}
	if s.Shadows != nil {
		maphash.WriteComparable(h, len(s.Shadows)+1)
		for _, sh := range s.Shadows {
			maphash.WriteComparable(h, sh)
		}
	}
	if s.Filters != nil {
		maphash.WriteComparable(h, len(s.Filters)+1)
		for _, f := range s.Filters {
			maphash.WriteComparable(h, f)
		}
	}
	if s.BackdropFilters != nil {
		maphash.WriteComparable(h, len(s.BackdropFilters)+1)
		for _, f := range s.BackdropFilters {
			maphash.WriteComparable(h, f)
		}
	}
	if s.Mask != nil {
		h.WriteByte(2)
		writeGradientHash(h, s.Mask)
	}
	if s.Transforms != nil {
		maphash.WriteComparable(h, len(s.Transforms)+1)
		for _, tr := range s.Transforms {
			maphash.WriteComparable(h, tr)
		}
	}
	if s.StrokeDash != nil {
		maphash.WriteComparable(h, len(s.StrokeDash)+1)
		for _, d := range s.StrokeDash {
			maphash.WriteComparable(h, d)
		}
	}
	if s.FontAxes != nil {
		h.WriteByte(4)
		h.WriteString(driver.EncodeFontAxes(s.FontAxes))
	}
	if s.FontFeatures != nil {
		h.WriteByte(5)
		h.WriteString(driver.EncodeFontFeatures(s.FontFeatures))
	}
	if s.GroupStyle != nil {
		h.WriteByte(3)
		h.WriteString(s.GroupStyle.Name)
		writeStateSetHash(h, s.GroupStyle.states())
	}
	if s.hasStateStyles() {
		writeStateSetHash(h, s.states())
	}
}

func writeGradientHash(h *maphash.Hash, g *Gradient) {
	maphash.WriteComparable(h, g.Kind)
	maphash.WriteComparable(h, g.Angle)
	maphash.WriteComparable(h, g.ToCorner)
	maphash.WriteComparable(h, g.Circle)
	maphash.WriteComparable(h, g.Extent)
	maphash.WriteComparable(h, g.Center)
	maphash.WriteComparable(h, g.Repeating)
	maphash.WriteComparable(h, g.Space)
	if g.Stops != nil {
		maphash.WriteComparable(h, len(g.Stops)+1)
		for _, st := range g.Stops {
			maphash.WriteComparable(h, st)
		}
	}
}

func writeStateSetHash(h *maphash.Hash, st stateSet) {
	for i := range st.slots {
		if p := *st.slots[i]; p != nil {
			h.WriteByte(byte(i + 1))
			writeStyleHash(h, p)
		}
	}
	if *st.data != nil {
		maphash.WriteComparable(h, len(*st.data)+1)
		for _, d := range *st.data {
			h.WriteString(d.Name)
			writeStyleHash(h, d.Style)
		}
	}
}

// layoutStyleIntern shares immutable *layout.Style values across nodes in one engine.
type layoutStyleIntern struct {
	seed     maphash.Seed
	now, old map[uint64][]*layout.Style
	nowCount int
}

func (c *layoutStyleIntern) intern(s *layout.Style) *layout.Style {
	if c.seed == (maphash.Seed{}) {
		c.seed = maphash.MakeSeed()
	}
	h := hashLayoutStyle(c.seed, s)
	if bucket := c.now[h]; len(bucket) > 0 {
		for _, cand := range bucket {
			if cand.Equal(s) {
				return cand
			}
		}
	}
	if bucket := c.old[h]; len(bucket) > 0 {
		for _, cand := range bucket {
			if cand.Equal(s) {
				c.put(h, cand)
				return cand
			}
		}
	}
	stored := new(layout.Style)
	*stored = *s
	c.put(h, stored)
	return stored
}

func (c *layoutStyleIntern) put(h uint64, s *layout.Style) {
	if c.now == nil {
		c.now = make(map[uint64][]*layout.Style)
	}
	if c.nowCount >= internCacheSize {
		c.old, c.now = c.now, c.old
		if c.now == nil {
			c.now = make(map[uint64][]*layout.Style)
		} else {
			clear(c.now)
		}
		c.nowCount = 0
	}
	c.now[h] = append(c.now[h], s)
	c.nowCount++
}

func hashLayoutStyle(seed maphash.Seed, s *layout.Style) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	maphash.WriteComparable(&h, s.Display)
	maphash.WriteComparable(&h, s.ItemIsTable)
	maphash.WriteComparable(&h, s.ItemIsReplaced)
	maphash.WriteComparable(&h, s.ItemIsCompressibleReplaced)
	maphash.WriteComparable(&h, s.BoxSizing)
	maphash.WriteComparable(&h, s.Direction)
	maphash.WriteComparable(&h, s.Overflow)
	maphash.WriteComparable(&h, s.ScrollbarWidth)
	maphash.WriteComparable(&h, s.Contain)
	maphash.WriteComparable(&h, s.Position)
	maphash.WriteComparable(&h, s.Inset)
	maphash.WriteComparable(&h, s.Size)
	maphash.WriteComparable(&h, s.MinSize)
	maphash.WriteComparable(&h, s.MaxSize)
	maphash.WriteComparable(&h, s.AspectRatio)
	maphash.WriteComparable(&h, s.Margin)
	maphash.WriteComparable(&h, s.Padding)
	maphash.WriteComparable(&h, s.Border)
	maphash.WriteComparable(&h, s.AlignItems)
	maphash.WriteComparable(&h, s.JustifyItems)
	maphash.WriteComparable(&h, s.AlignSelf)
	maphash.WriteComparable(&h, s.JustifySelf)
	maphash.WriteComparable(&h, s.AlignContent)
	maphash.WriteComparable(&h, s.JustifyContent)
	maphash.WriteComparable(&h, s.Gap)
	maphash.WriteComparable(&h, s.TextAlign)
	maphash.WriteComparable(&h, s.FlexDirection)
	maphash.WriteComparable(&h, s.FlexWrap)
	maphash.WriteComparable(&h, s.FlexBasis)
	maphash.WriteComparable(&h, s.FlexGrow)
	maphash.WriteComparable(&h, s.FlexShrink)
	maphash.WriteComparable(&h, s.GridAutoFlow)
	maphash.WriteComparable(&h, s.GridRow)
	maphash.WriteComparable(&h, s.GridColumn)
	if len(s.GridTemplateRows) > 0 || len(s.GridTemplateColumns) > 0 ||
		len(s.GridTemplateRowNames) > 0 || len(s.GridTemplateColumnNames) > 0 ||
		len(s.GridTemplateAreas.Areas) > 0 || len(s.GridAutoRows) > 0 || len(s.GridAutoColumns) > 0 {
		maphash.WriteComparable(&h, len(s.GridTemplateRows))
		maphash.WriteComparable(&h, len(s.GridTemplateColumns))
		maphash.WriteComparable(&h, len(s.GridTemplateRowNames))
		maphash.WriteComparable(&h, len(s.GridTemplateColumnNames))
		maphash.WriteComparable(&h, s.GridTemplateAreas.Rows)
		maphash.WriteComparable(&h, s.GridTemplateAreas.Columns)
		maphash.WriteComparable(&h, len(s.GridTemplateAreas.Areas))
		maphash.WriteComparable(&h, len(s.GridAutoRows))
		maphash.WriteComparable(&h, len(s.GridAutoColumns))
	}
	return h.Sum64()
}

func (rt *runtime) internLayoutStyle(s *layout.Style) *layout.Style {
	if rt == nil {
		cp := *s
		return &cp
	}
	return rt.layoutStyles.intern(s)
}

func (rt *runtime) internViewStyle(vs driver.ViewStyle) *driver.ViewStyle {
	if vs == defaultViewStyle {
		return &defaultViewStyle
	}
	if rt == nil {
		cp := vs
		return &cp
	}
	if cached, ok := rt.viewStyles.get(vs); ok {
		return cached
	}
	cp := vs
	rt.viewStyles.put(vs, &cp)
	return &cp
}

func (rt *runtime) internTextFields(tf textFields) *textFields {
	if rt == nil {
		cp := tf
		return &cp
	}
	if cached, ok := rt.textFieldCache.get(tf); ok {
		return cached
	}
	cp := tf
	rt.textFieldCache.put(tf, &cp)
	return &cp
}
