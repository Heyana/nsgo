package layout

import "math"

// cacheSize is the number of size measurements a node keeps.
const cacheSize = 9

var (
	infinityBits    = math.Float32bits(float32(math.Inf(1)))
	negInfinityBits = math.Float32bits(float32(math.Inf(-1)))
)

// A cacheKey packs two float32s per u64. The values are never negative, so
// the requested axis is packed into the sign bits of the parent size.
const (
	signBit1         uint64 = 1 << 63
	signBit2         uint64 = 1 << 31
	bothSignBitsMask        = signBit1 | signBit2
	nonSignBitsMask         = ^bothSignBitsMask
	xAxisValueMask   uint64 = uint64(math.MaxUint32) << 32
)

// Cache modes: a cache holds results of one sizing mode and margin
// collapsibility.
const (
	cacheInherentSize uint8 = 1 << iota
	cacheCollapsibleStart
	cacheCollapsibleEnd
)

type cacheKey struct {
	kdAvailableSpace           uint64
	parentSize                 uint64
	knownDimensionsAreDefinite Size[bool]
}

func optionCacheKey(o Opt) uint32 {
	if o.Valid {
		return math.Float32bits(o.Value)
	}
	return infinityBits
}

func spaceCacheKey(a AvailableSpace) uint32 {
	switch a.Kind {
	case SpaceMinContent:
		return negInfinityBits
	case SpaceMaxContent:
		return infinityBits
	}
	return math.Float32bits(-a.Value)
}

func mixedCacheKey(kd Opt, a AvailableSpace) uint32 {
	if kd.Valid {
		return math.Float32bits(kd.Value)
	}
	return spaceCacheKey(a)
}

func newCacheKey(in *layoutInput) cacheKey {
	var axisBits uint64
	switch in.axis {
	case requestHorizontal:
		axisBits = signBit1
	case requestVertical:
		axisBits = signBit2
	default:
		axisBits = signBit1 | signBit2
	}
	kd, as := in.knownDimensions, in.availableSpace
	parent := uint64(optionCacheKey(in.parentSize.Width))<<32 | uint64(optionCacheKey(in.parentSize.Height))
	return cacheKey{
		kdAvailableSpace: uint64(mixedCacheKey(kd.Width, as.Width))<<32 | uint64(mixedCacheKey(kd.Height, as.Height)),
		parentSize:       parent&nonSignBitsMask | axisBits,
		knownDimensionsAreDefinite: Size[bool]{
			in.knownDimensionsAreDefinite.Width || !kd.Width.Valid,
			in.knownDimensionsAreDefinite.Height || !kd.Height.Valid,
		},
	}
}

func (k cacheKey) xAxisParentSize() uint64 { return k.parentSize & (xAxisValueMask & nonSignBitsMask) }

func (k cacheKey) requestedAxisBits() uint64 { return k.parentSize & bothSignBitsMask }

// sizeIsValidFor reports whether a size measured for k answers other: a
// measurement of both axes answers any request.
func (k cacheKey) sizeIsValidFor(other cacheKey) bool {
	axis := k.requestedAxisBits()
	return axis == bothSignBitsMask || axis == other.requestedAxisBits()
}

func cacheModeOf(in *layoutInput) uint8 {
	var mode uint8
	if in.sizingMode == inherentSize {
		mode |= cacheInherentSize
	}
	if in.verticalMarginsAreCollapsible.Start {
		mode |= cacheCollapsibleStart
	}
	if in.verticalMarginsAreCollapsible.End {
		mode |= cacheCollapsibleEnd
	}
	return mode
}

type measureEntry struct {
	key   cacheKey
	size  Size[float32]
	valid bool
}

// cache is Taffy's Cache: one final layout and a few size measurements, all
// computed with one sizing mode and margin collapsibility. Measurements are
// replaced in a second-chance clock order.
type cache struct {
	finalKey     cacheKey
	final        layoutOutput
	hasFinal     bool
	measure      [cacheSize]measureEntry
	recentlyUsed uint16
	next         uint8
	filled       bool
	mode         uint8
}

func (c *cache) get(in *layoutInput) (layoutOutput, bool) {
	if c.mode != cacheModeOf(in) {
		return layoutOutput{}, false
	}
	key := newCacheKey(in)
	switch in.runMode {
	case performLayout:
		if c.hasFinal && c.finalKey == key {
			return c.final.clone(), true
		}
	case computeSize:
		for i := range c.measure {
			e := &c.measure[i]
			if !e.valid {
				continue
			}
			if e.key.kdAvailableSpace == key.kdAvailableSpace &&
				e.key.knownDimensionsAreDefinite == key.knownDimensionsAreDefinite &&
				e.key.xAxisParentSize() == key.xAxisParentSize() &&
				e.key.sizeIsValidFor(key) {
				c.recentlyUsed |= 1 << i
				return outputFromOuterSize(e.size), true
			}
		}
	}
	return layoutOutput{}, false
}

func (c *cache) store(in *layoutInput, out *layoutOutput) {
	if in.runMode == performHiddenLayout {
		return
	}
	if mode := cacheModeOf(in); c.mode != mode {
		c.clear()
		c.mode = mode
	}
	key := newCacheKey(in)
	switch in.runMode {
	case performLayout:
		c.filled = true
		c.finalKey = key
		c.final = out.clone()
		c.hasFinal = true
	case computeSize:
		// Measurements keep only the size, and a hit rebuilds the output from
		// it, losing margin-collapse metadata: outputs carrying some are not
		// cached.
		if out.marginsCanCollapseThrough || out.topMargin != (collapsibleMarginSet{}) ||
			out.bottomMargin != (collapsibleMarginSet{}) {
			return
		}
		c.filled = true
		for i := range c.measure {
			if c.measure[i].valid && c.measure[i].key == key {
				c.measure[i].size = out.size
				c.recentlyUsed |= 1 << i
				return
			}
		}
		for c.recentlyUsed&(1<<c.next) != 0 {
			c.recentlyUsed &^= 1 << c.next
			c.next++
			if c.next == cacheSize {
				c.next = 0
			}
		}
		i := c.next
		c.measure[i] = measureEntry{key: key, size: out.size, valid: true}
		c.recentlyUsed |= 1 << i
		c.next++
		if c.next == cacheSize {
			c.next = 0
		}
	}
}

// clear empties the cache and reports whether it held anything.
func (c *cache) clear() bool {
	if !c.filled {
		return false
	}
	mode := c.mode
	*c = cache{mode: mode}
	return true
}

// isEmpty reports whether the cache holds no results.
func (c *cache) isEmpty() bool {
	if c.hasFinal {
		return false
	}
	for i := range c.measure {
		if c.measure[i].valid {
			return false
		}
	}
	return true
}
