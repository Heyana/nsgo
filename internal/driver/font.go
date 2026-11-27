package driver

import (
	"cmp"
	"encoding/binary"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
)

// FontAxis is one variable font axis coordinate, keyed by its four-character
// OpenType tag (such as "wdth", "opsz" or "slnt").
type FontAxis struct {
	Tag   string
	Value float32
}

// FontFeature is one OpenType feature setting, keyed by its four-character
// OpenType tag (such as "ss01", "zero" or "liga"); 0 disables the feature,
// 1 enables it, and higher values pick an alternate index.
type FontFeature struct {
	Tag   string
	Value int
}

// FontKey is a comparable cache key representing every field of a Font.
type FontKey struct {
	Family   string
	Language string
	Size     Points
	Weight   FontWeight
	Italic   bool
	Variant  FontVariant
	Axes     string
	Features string
}

// TextStyleKey is a comparable cache key representing every field of a TextStyle.
type TextStyleKey struct {
	Font                FontKey
	Color               Color
	LineHeight          Points
	LetterSpacing       Points
	Align               TextAlign
	RTL                 bool
	Decoration          TextDecoration
	DecorationStyle     TextDecorationStyle
	DecorationColor     Color
	DecorationThickness Points
	UnderlineOffset     Length
	Transform           uint8
}

// ValidFontTag reports whether tag is a valid four-character ASCII OpenType tag.
func ValidFontTag(tag string) bool {
	if len(tag) != 4 {
		return false
	}
	nonSpace := false
	for i := range 4 {
		b := tag[i]
		if b < 0x20 || b > 0x7e || b == '"' || b == '\'' || b == ',' || b == ';' || b == '\\' {
			return false
		}
		if b != ' ' {
			nonSpace = true
		}
	}
	return nonSpace
}

// FontTagBE packs a four-character OpenType tag into a big-endian FourCharCode,
// as CoreText's kCTFontVariationAttribute expects.
func FontTagBE(tag string) uint32 {
	return uint32(tag[0])<<24 | uint32(tag[1])<<16 | uint32(tag[2])<<8 | uint32(tag[3])
}

// FontTagLE packs a four-character OpenType tag into a little-endian OpenType
// tag, as DirectWrite's DWRITE_MAKE_OPENTYPE_TAG and DWRITE_FONT_AXIS_TAG expect.
func FontTagLE(tag string) uint32 {
	return uint32(tag[0]) | uint32(tag[1])<<8 | uint32(tag[2])<<16 | uint32(tag[3])<<24
}

// ResolvedFontAxes returns f.Axes sorted by tag, omitting invalid tags and non-finite values.
func ResolvedFontAxes(f Font) []FontAxis {
	if len(f.Axes) == 0 {
		return nil
	}
	out := make([]FontAxis, 0, len(f.Axes))
	for tag, val := range f.Axes {
		if !ValidFontTag(tag) || math.IsNaN(float64(val)) || math.IsInf(float64(val), 0) {
			continue
		}
		if val == 0 {
			val = 0 // normalize -0 to +0
		}
		out = append(out, FontAxis{Tag: tag, Value: val})
	}
	if len(out) == 0 {
		return nil
	}
	slices.SortFunc(out, func(a, b FontAxis) int { return cmp.Compare(a.Tag, b.Tag) })
	return out
}

// ResolvedFontFeatures returns the effective OpenType features of f sorted by tag:
// shorthand flags from f.Variant enabled at 1, overridden by any explicit entries
// in f.Features (including 0 to disable a feature).
func ResolvedFontFeatures(f Font) []FontFeature {
	v := f.Variant &^ FontVariantNormal
	if v == 0 && len(f.Features) == 0 {
		return nil
	}
	merged := make(map[string]int, len(f.Features)+len(fontFeatures))
	for _, feat := range fontFeatures {
		if v&feat.bit != 0 {
			merged[feat.tag] = 1
		}
	}
	for tag, val := range f.Features {
		if !ValidFontTag(tag) {
			continue
		}
		if val < 0 {
			val = 0
		}
		merged[tag] = val
	}
	if len(merged) == 0 {
		return nil
	}
	out := make([]FontFeature, 0, len(merged))
	for tag, val := range merged {
		out = append(out, FontFeature{Tag: tag, Value: val})
	}
	slices.SortFunc(out, func(a, b FontFeature) int { return cmp.Compare(a.Tag, b.Tag) })
	return out
}

// FontCSSVariationSettings formats f's variable font axes as CSS font-variation-settings
// (for example `"opsz" 14, "wdth" 75`), or empty when none are set.
func FontCSSVariationSettings(f Font) string {
	return formatAxes(ResolvedFontAxes(f), '"', ' ')
}

// FontAndroidVariationSettings formats f's variable font axes for Android's
// Typeface.Builder.setFontVariationSettings and Paint.setFontVariationSettings
// (for example `'opsz' 14, 'wdth' 75`), or empty when none are set.
func FontAndroidVariationSettings(f Font) string {
	return formatAxes(ResolvedFontAxes(f), '\'', ' ')
}

// FontPangoVariations formats f's variable font axes for Pango's
// pango_font_description_set_variations (for example `opsz=14,wdth=75`),
// or empty when none are set.
func FontPangoVariations(f Font) string {
	axes := ResolvedFontAxes(f)
	if len(axes) == 0 {
		return ""
	}
	var b strings.Builder
	for i, a := range axes {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(a.Tag)
		b.WriteByte('=')
		b.WriteString(strconv.FormatFloat(float64(a.Value), 'f', -1, 32))
	}
	return b.String()
}

func formatAxes(axes []FontAxis, quote, sep byte) string {
	if len(axes) == 0 {
		return ""
	}
	var b strings.Builder
	for i, a := range axes {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte(quote)
		b.WriteString(a.Tag)
		b.WriteByte(quote)
		b.WriteByte(sep)
		b.WriteString(strconv.FormatFloat(float64(a.Value), 'f', -1, 32))
	}
	return b.String()
}

// FontCSSFeatureSettings formats f's resolved OpenType features as CSS
// font-feature-settings (for example `"ss01" 1, "zero" 1`), or empty when none.
func FontCSSFeatureSettings(f Font) string {
	return formatFeatures(ResolvedFontFeatures(f), '"')
}

// FontAndroidFeatureSettings formats f's resolved OpenType features for Android's
// Paint.setFontFeatureSettings (for example `'ss01' 1, 'zero' 1`), or empty when none.
func FontAndroidFeatureSettings(f Font) string {
	return formatFeatures(ResolvedFontFeatures(f), '\'')
}

// FontPangoFeatures formats f's resolved OpenType features for Pango's
// pango_attr_font_features_new (for example `ss01 1, zero 1`), or empty when none.
func FontPangoFeatures(f Font) string {
	feats := ResolvedFontFeatures(f)
	if len(feats) == 0 {
		return ""
	}
	var b strings.Builder
	for i, feat := range feats {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(feat.Tag)
		b.WriteByte(' ')
		b.WriteString(strconv.Itoa(feat.Value))
	}
	return b.String()
}

func formatFeatures(feats []FontFeature, quote byte) string {
	if len(feats) == 0 {
		return ""
	}
	var b strings.Builder
	for i, feat := range feats {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteByte(quote)
		b.WriteString(feat.Tag)
		b.WriteByte(quote)
		b.WriteByte(' ')
		b.WriteString(strconv.Itoa(feat.Value))
	}
	return b.String()
}

// FontAxesKey returns a canonical comparable key for f's resolved variable font axes.
func FontAxesKey(f Font) string {
	axes := ResolvedFontAxes(f)
	if len(axes) == 0 {
		return ""
	}
	b := make([]byte, 0, 8*len(axes))
	for _, a := range axes {
		b = append(b, a.Tag...)
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(a.Value))
	}
	return string(b)
}

// FontFeaturesKey returns a canonical comparable key for f's resolved OpenType features.
func FontFeaturesKey(f Font) string {
	feats := ResolvedFontFeatures(f)
	if len(feats) == 0 {
		return ""
	}
	b := make([]byte, 0, 8*len(feats))
	for _, feat := range feats {
		b = append(b, feat.Tag...)
		b = binary.LittleEndian.AppendUint32(b, uint32(feat.Value))
	}
	return string(b)
}

// EncodeFontAxes encodes axes as a comparable string preserving the distinction
// between nil (empty string) and an explicit empty map ("a").
func EncodeFontAxes(axes map[string]float32) string {
	if axes == nil {
		return ""
	}
	resolved := ResolvedFontAxes(Font{Axes: axes})
	b := make([]byte, 1, 1+8*len(resolved))
	b[0] = 'a'
	for _, a := range resolved {
		b = append(b, a.Tag...)
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(a.Value))
	}
	return string(b)
}

// DecodeFontAxes decodes a string produced by EncodeFontAxes; returns nil when empty.
func DecodeFontAxes(encoded string) map[string]float32 {
	if len(encoded) <= 1 {
		return nil
	}
	out := make(map[string]float32, (len(encoded)-1)/8)
	for i := 1; i+8 <= len(encoded); i += 8 {
		tag := encoded[i : i+4]
		bits := binary.LittleEndian.Uint32([]byte(encoded[i+4 : i+8]))
		out[tag] = math.Float32frombits(bits)
	}
	return out
}

// EncodeFontFeatures encodes features as a comparable string preserving the
// distinction between nil (empty string) and an explicit empty map ("f").
func EncodeFontFeatures(features map[string]int) string {
	if features == nil {
		return ""
	}
	resolved := ResolvedFontFeatures(Font{Features: features})
	b := make([]byte, 1, 1+8*len(resolved))
	b[0] = 'f'
	for _, feat := range resolved {
		b = append(b, feat.Tag...)
		b = binary.LittleEndian.AppendUint32(b, uint32(feat.Value))
	}
	return string(b)
}

// DecodeFontFeatures decodes a string produced by EncodeFontFeatures; returns nil when empty.
func DecodeFontFeatures(encoded string) map[string]int {
	if len(encoded) <= 1 {
		return nil
	}
	out := make(map[string]int, (len(encoded)-1)/8)
	for i := 1; i+8 <= len(encoded); i += 8 {
		tag := encoded[i : i+4]
		val := int(binary.LittleEndian.Uint32([]byte(encoded[i+4 : i+8])))
		out[tag] = val
	}
	return out
}

// FontKeyOf returns a comparable cache key for f.
func FontKeyOf(f Font) FontKey {
	return FontKey{
		Family:   f.Family,
		Language: f.Language,
		Size:     f.Size,
		Weight:   f.Weight,
		Italic:   f.Italic,
		Variant:  f.Variant,
		Axes:     FontAxesKey(f),
		Features: FontFeaturesKey(f),
	}
}

// Equal reports whether a and b describe the same font.
func (a Font) Equal(b Font) bool {
	if a.Family != b.Family || a.Language != b.Language || a.Size != b.Size ||
		a.Weight != b.Weight || a.Italic != b.Italic || a.Variant != b.Variant {
		return false
	}
	if len(a.Axes) != len(b.Axes) || len(a.Features) != len(b.Features) {
		return false
	}
	return maps.Equal(a.Axes, b.Axes) && maps.Equal(a.Features, b.Features)
}

// IsZero reports whether every field of f is unset.
func (f Font) IsZero() bool {
	return f.Family == "" && f.Language == "" && f.Size == 0 && f.Weight == 0 &&
		!f.Italic && f.Variant == 0 && len(f.Axes) == 0 && len(f.Features) == 0
}

// TextStyleKeyOf returns a comparable cache key for s.
func TextStyleKeyOf(s TextStyle) TextStyleKey {
	return TextStyleKey{
		Font:                FontKeyOf(s.Font),
		Color:               s.Color,
		LineHeight:          s.LineHeight,
		LetterSpacing:       s.LetterSpacing,
		Align:               s.Align,
		RTL:                 s.RTL,
		Decoration:          s.Decoration,
		DecorationStyle:     s.DecorationStyle,
		DecorationColor:     s.DecorationColor,
		DecorationThickness: s.DecorationThickness,
		UnderlineOffset:     s.UnderlineOffset,
		Transform:           s.Transform,
	}
}

// Equal reports whether a and b are identical resolved text styles.
func (a TextStyle) Equal(b TextStyle) bool {
	return a.Font.Equal(b.Font) && a.Color == b.Color && a.LineHeight == b.LineHeight &&
		a.LetterSpacing == b.LetterSpacing && a.Align == b.Align && a.RTL == b.RTL &&
		a.Decoration == b.Decoration && a.DecorationStyle == b.DecorationStyle &&
		a.DecorationColor == b.DecorationColor && a.DecorationThickness == b.DecorationThickness &&
		a.UnderlineOffset == b.UnderlineOffset && a.Transform == b.Transform
}
