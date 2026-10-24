package layout

// Length is a CSS length, percentage, auto or sizing keyword: Taffy's
// Dimension, LengthPercentage, LengthPercentageAuto and track sizing
// functions in one type. Which units a property accepts follows CSS; layout
// treats the others as auto.
//
// The zero Length is unset: the initial value of the property holding it,
// auto for sizes, insets, flex-basis and grid tracks, 0 for margins, padding,
// borders and gaps.
type Length struct {
	Value float32
	Unit  Unit
}

// Unit says how to read a Length's Value.
type Unit uint8

const (
	// UnitUnset is the zero Unit: the property's initial value.
	UnitUnset Unit = iota
	UnitAuto
	// UnitPx is a length in pixels (points).
	UnitPx
	// UnitPercent is a percentage of the containing block's size, as a
	// fraction: 0.5 is 50%.
	UnitPercent
	// UnitFr is a fraction of a grid's free space.
	UnitFr
	UnitMinContent
	UnitMaxContent
	// UnitFitContent is the fit-content keyword.
	UnitFitContent
	// UnitFitContentPx is fit-content(<length>), Value being the limit.
	UnitFitContentPx
	// UnitFitContentPercent is fit-content(<percentage>), Value being the
	// limit as a fraction.
	UnitFitContentPercent
	UnitStretch
	// UnitContent is flex-basis's content keyword.
	UnitContent
)

// Px returns a length in pixels.
func Px(v float32) Length { return Length{v, UnitPx} }

// Percent returns a percentage given as a fraction: Percent(0.5) is 50%.
func Percent(f float32) Length { return Length{f, UnitPercent} }

// Fr returns a flexible grid track size.
func Fr(v float32) Length { return Length{v, UnitFr} }

// FitContentPx returns fit-content(<length>).
func FitContentPx(limit float32) Length { return Length{limit, UnitFitContentPx} }

// FitContentPercent returns fit-content(<percentage>), the limit given as a
// fraction.
func FitContentPercent(limit float32) Length { return Length{limit, UnitFitContentPercent} }

// Keyword lengths.
var (
	Auto       = Length{Unit: UnitAuto}
	MinContent = Length{Unit: UnitMinContent}
	MaxContent = Length{Unit: UnitMaxContent}
	FitContent = Length{Unit: UnitFitContent}
	Stretch    = Length{Unit: UnitStretch}
	Content    = Length{Unit: UnitContent}
)

// IsAuto reports whether l is auto or unset.
func (l Length) IsAuto() bool { return l.Unit <= UnitAuto }

func (l Length) isLength() bool  { return l.Unit == UnitPx }
func (l Length) isPercent() bool { return l.Unit == UnitPercent }
func (l Length) isFr() bool      { return l.Unit == UnitFr }
func (l Length) isContent() bool { return l.Unit == UnitContent }
func (l Length) isStretch() bool { return l.Unit == UnitStretch }

func (l Length) isMinContent() bool { return l.Unit == UnitMinContent }
func (l Length) isMaxContent() bool { return l.Unit == UnitMaxContent }

// isZero reports whether l is exactly 0px.
func (l Length) isZero() bool { return l.Unit == UnitPx && l.Value == 0 }

// isLengthOrPercentage reports whether l is a px length or a percentage.
func (l Length) isLengthOrPercentage() bool { return l.Unit == UnitPx || l.Unit == UnitPercent }

// isFitContent reports whether l is fit-content(<length-percentage>) (not the
// bare keyword).
func (l Length) isFitContent() bool {
	return l.Unit == UnitFitContentPx || l.Unit == UnitFitContentPercent
}

// isSizingKeyword reports whether l is min-content, max-content, a
// fit-content form or stretch.
func (l Length) isSizingKeyword() bool {
	switch l.Unit {
	case UnitMinContent, UnitMaxContent, UnitFitContent, UnitFitContentPx, UnitFitContentPercent, UnitStretch:
		return true
	}
	return false
}

func (l Length) isMaxOrFitContent() bool {
	return l.Unit == UnitMaxContent || l.isFitContent()
}

// isMaxContentAlike reports whether a max track sizing function behaves like
// max-content: auto, max-content or fit-content().
func (l Length) isMaxContentAlike() bool { return l.IsAuto() || l.isMaxOrFitContent() }

func (l Length) isMinOrMaxContent() bool {
	return l.Unit == UnitMinContent || l.Unit == UnitMaxContent
}

// isIntrinsic reports whether a track sizing function is intrinsic: auto,
// min-content, max-content or fit-content().
func (l Length) isIntrinsic() bool {
	return l.IsAuto() || l.isMinOrMaxContent() || l.isFitContent()
}

// usesPercentage reports whether l depends on a percentage basis.
func (l Length) usesPercentage() bool {
	return l.Unit == UnitPercent || l.Unit == UnitFitContentPercent
}

// behavesAsAuto reports whether a min track sizing function behaves as auto:
// it is auto, or a percentage with no basis.
func (l Length) behavesAsAuto(parentSize Opt) bool {
	return l.IsAuto() || (!parentSize.Valid && l.usesPercentage())
}

// maybeResolve resolves l against a basis: lengths are themselves,
// percentages need the basis, and everything else is absent.
func (l Length) maybeResolve(basis Opt) Opt {
	switch l.Unit {
	case UnitPx:
		return Some(l.Value)
	case UnitPercent:
		if basis.Valid {
			return Some(basis.Value * l.Value)
		}
	}
	return None
}

// resolveOrZero is maybeResolve with absent values as 0.
func (l Length) resolveOrZero(basis Opt) float32 { return l.maybeResolve(basis).Or(0) }

// resolveToOption resolves a length-percentage-auto against a known basis:
// auto is absent.
func (l Length) resolveToOption(basis float32) Opt { return l.maybeResolve(Some(basis)) }

// definiteValue is a track sizing function's fixed size, if it has one.
func (l Length) definiteValue(parentSize Opt) Opt { return l.maybeResolve(parentSize) }

func (l Length) hasDefiniteValue(parentSize Opt) bool {
	return l.Unit == UnitPx || (l.Unit == UnitPercent && parentSize.Valid)
}

// fitContentLimit is the limit of fit-content(), if it resolves.
func (l Length) fitContentLimit(parentSize Opt) Opt {
	switch l.Unit {
	case UnitFitContentPx:
		return Some(l.Value)
	case UnitFitContentPercent:
		if parentSize.Valid {
			return Some(l.Value * parentSize.Value)
		}
	}
	return None
}

// definiteLimit is a max track sizing function's fit-content limit or fixed
// size.
func (l Length) definiteLimit(parentSize Opt) Opt {
	if l.isFitContent() {
		return l.fitContentLimit(parentSize)
	}
	return l.definiteValue(parentSize)
}

// resolvedPercentageSize resolves a percentage against a known basis.
func (l Length) resolvedPercentageSize(parentSize float32) Opt {
	if l.Unit == UnitPercent {
		return Some(l.Value * parentSize)
	}
	return None
}

// intoOption is Dimension::into_option: a px length, or absent.
func (l Length) intoOption() Opt {
	if l.Unit == UnitPx {
		return Some(l.Value)
	}
	return None
}

func sizeMaybeResolve(s Size[Length], basis Size[Opt]) Size[Opt] {
	return Size[Opt]{s.Width.maybeResolve(basis.Width), s.Height.maybeResolve(basis.Height)}
}

func sizeResolveOrZero(s Size[Length], basis Size[Opt]) Size[float32] {
	return Size[float32]{s.Width.resolveOrZero(basis.Width), s.Height.resolveOrZero(basis.Height)}
}

// rectResolveOrZero resolves every side against one basis, as padding and
// borders resolve against the containing block's width.
func rectResolveOrZero(r Rect[Length], basis Opt) Rect[float32] {
	return Rect[float32]{
		Left:   r.Left.resolveOrZero(basis),
		Right:  r.Right.resolveOrZero(basis),
		Top:    r.Top.resolveOrZero(basis),
		Bottom: r.Bottom.resolveOrZero(basis),
	}
}

// rectResolveOrZeroSize resolves the horizontal sides against the width and
// the vertical ones against the height.
func rectResolveOrZeroSize(r Rect[Length], basis Size[Opt]) Rect[float32] {
	return Rect[float32]{
		Left:   r.Left.resolveOrZero(basis.Width),
		Right:  r.Right.resolveOrZero(basis.Width),
		Top:    r.Top.resolveOrZero(basis.Height),
		Bottom: r.Bottom.resolveOrZero(basis.Height),
	}
}

func rectMaybeResolve(r Rect[Length], basis Opt) Rect[Opt] {
	return Rect[Opt]{
		Left:   r.Left.maybeResolve(basis),
		Right:  r.Right.maybeResolve(basis),
		Top:    r.Top.maybeResolve(basis),
		Bottom: r.Bottom.maybeResolve(basis),
	}
}

func rectIsAuto(r Rect[Length]) Rect[bool] {
	return Rect[bool]{r.Left.IsAuto(), r.Right.IsAuto(), r.Top.IsAuto(), r.Bottom.IsAuto()}
}

// AvailableSpace is the space a node may lay out in on one axis: a definite
// size, or the min-content or max-content sizing constraint. The zero value
// is a definite 0.
type AvailableSpace struct {
	Value float32
	Kind  SpaceKind
}

// SpaceKind is the kind of an AvailableSpace.
type SpaceKind uint8

const (
	SpaceDefinite SpaceKind = iota
	SpaceMinContent
	SpaceMaxContent
)

// Definite returns definite available space.
func Definite(v float32) AvailableSpace { return AvailableSpace{Value: v} }

// The sizing constraints.
var (
	MinContentSpace = AvailableSpace{Kind: SpaceMinContent}
	MaxContentSpace = AvailableSpace{Kind: SpaceMaxContent}
)

// spaceFromOpt is AvailableSpace::from(Option<f32>): absent is max-content.
func spaceFromOpt(o Opt) AvailableSpace {
	if o.Valid {
		return Definite(o.Value)
	}
	return MaxContentSpace
}

func (a AvailableSpace) isDefinite() bool { return a.Kind == SpaceDefinite }

func (a AvailableSpace) isMinContent() bool { return a.Kind == SpaceMinContent }

func (a AvailableSpace) isMaxContent() bool { return a.Kind == SpaceMaxContent }

// intoOption returns the definite size, if any.
func (a AvailableSpace) intoOption() Opt {
	if a.Kind == SpaceDefinite {
		return Some(a.Value)
	}
	return None
}

func (a AvailableSpace) unwrapOr(d float32) float32 {
	if a.Kind == SpaceDefinite {
		return a.Value
	}
	return d
}

// or returns a if it is definite and d otherwise.
func (a AvailableSpace) or(d AvailableSpace) AvailableSpace {
	if a.Kind == SpaceDefinite {
		return a
	}
	return d
}

// maybeSet replaces a with a definite v when v is present.
func (a AvailableSpace) maybeSet(v Opt) AvailableSpace {
	if v.Valid {
		return Definite(v.Value)
	}
	return a
}

// computeFreeSpace returns the free space left after used: infinite under a
// max-content constraint, 0 under a min-content one.
func (a AvailableSpace) computeFreeSpace(used float32) float32 {
	switch a.Kind {
	case SpaceMaxContent:
		return inf32
	case SpaceMinContent:
		return 0
	}
	return a.Value - used
}

func (a AvailableSpace) isRoughlyEqual(b AvailableSpace) bool {
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind == SpaceDefinite {
		return abs32(a.Value-b.Value) < epsilon32
	}
	return true
}

// Maybe-math with a float operand applies to definite space only; min with a
// size turns a sizing constraint into that definite size.

func (a AvailableSpace) minF(v float32) AvailableSpace {
	if a.Kind == SpaceDefinite {
		return Definite(fmin(a.Value, v))
	}
	return Definite(v)
}

func (a AvailableSpace) maxF(v float32) AvailableSpace {
	if a.Kind == SpaceDefinite {
		return Definite(fmax(a.Value, v))
	}
	return a
}

func (a AvailableSpace) clampF(lo, hi float32) AvailableSpace {
	if a.Kind == SpaceDefinite {
		return Definite(fmax(fmin(a.Value, hi), lo))
	}
	return a
}

func (a AvailableSpace) addF(v float32) AvailableSpace {
	if a.Kind == SpaceDefinite {
		return Definite(a.Value + v)
	}
	return a
}

func (a AvailableSpace) subF(v float32) AvailableSpace {
	if a.Kind == SpaceDefinite {
		return Definite(a.Value - v)
	}
	return a
}

func (a AvailableSpace) minOpt(v Opt) AvailableSpace {
	if !v.Valid {
		return a
	}
	return a.minF(v.Value)
}

func (a AvailableSpace) maxOpt(v Opt) AvailableSpace {
	if !v.Valid {
		return a
	}
	return a.maxF(v.Value)
}

func (a AvailableSpace) clampOpt(lo, hi Opt) AvailableSpace {
	if a.Kind != SpaceDefinite {
		return a
	}
	return Definite(fclamp(a.Value, lo, hi))
}

func (a AvailableSpace) addOpt(v Opt) AvailableSpace {
	if !v.Valid {
		return a
	}
	return a.addF(v.Value)
}

func (a AvailableSpace) subOpt(v Opt) AvailableSpace {
	if !v.Valid {
		return a
	}
	return a.subF(v.Value)
}

func spacesOpts(s Size[AvailableSpace]) Size[Opt] {
	return Size[Opt]{s.Width.intoOption(), s.Height.intoOption()}
}

func spacesMaybeSet(s Size[AvailableSpace], v Size[Opt]) Size[AvailableSpace] {
	return Size[AvailableSpace]{s.Width.maybeSet(v.Width), s.Height.maybeSet(v.Height)}
}

func spacesSubF(s Size[AvailableSpace], v Size[float32]) Size[AvailableSpace] {
	return Size[AvailableSpace]{s.Width.subF(v.Width), s.Height.subF(v.Height)}
}

func spacesMaxF(s Size[AvailableSpace], v Size[float32]) Size[AvailableSpace] {
	return Size[AvailableSpace]{s.Width.maxF(v.Width), s.Height.maxF(v.Height)}
}

func spacesMinOpt(s Size[AvailableSpace], v Size[Opt]) Size[AvailableSpace] {
	return Size[AvailableSpace]{s.Width.minOpt(v.Width), s.Height.minOpt(v.Height)}
}

func spacesClampOpt(s Size[AvailableSpace], lo, hi Size[Opt]) Size[AvailableSpace] {
	return Size[AvailableSpace]{s.Width.clampOpt(lo.Width, hi.Width), s.Height.clampOpt(lo.Height, hi.Height)}
}

func spacesDefinite(s Size[float32]) Size[AvailableSpace] {
	return Size[AvailableSpace]{Definite(s.Width), Definite(s.Height)}
}

var maxContentSpaces = Size[AvailableSpace]{MaxContentSpace, MaxContentSpace}

var minContentSpaces = Size[AvailableSpace]{MinContentSpace, MinContentSpace}
