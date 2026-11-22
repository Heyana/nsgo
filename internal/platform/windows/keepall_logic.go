package windows

import (
	"unicode/utf16"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// wordJoiner (U+2060) prohibits a line break between the characters around it in DirectWrite without drawing a glyph or changing metrics.
const wordJoiner uint16 = 0x2060

// encodeKeepAllText encodes text into UTF-16 units with wordJoiner inserted between adjacent runes inside each byte range in keepAllRanges (which must be sorted and non-overlapping). offsets maps every unit index (plus one past the end) to its UTF-8 byte offset in text. origUTF16 maps every unit index (plus one past the end) to its UTF-16 index in the original text, or is nil when no word joiners were inserted.
func encodeKeepAllText(text string, keepAllRanges [][2]int) (units []uint16, offsets, origUTF16 []int) {
	if len(keepAllRanges) == 0 {
		for i, r := range text {
			n := len(units)
			units = utf16.AppendRune(units, r)
			for range len(units) - n {
				offsets = append(offsets, i)
			}
		}
		offsets = append(offsets, len(text))
		return units, offsets, nil
	}
	wi, origPos := 0, 0
	for i, r := range text {
		for wi < len(keepAllRanges) && keepAllRanges[wi][1] <= i {
			wi++
		}
		if wi < len(keepAllRanges) && i > keepAllRanges[wi][0] && i < keepAllRanges[wi][1] {
			units = append(units, wordJoiner)
			offsets = append(offsets, i)
			origUTF16 = append(origUTF16, origPos)
		}
		n := len(units)
		units = utf16.AppendRune(units, r)
		added := len(units) - n
		for k := range added {
			offsets = append(offsets, i)
			origUTF16 = append(origUTF16, origPos+k)
		}
		origPos += added
	}
	offsets = append(offsets, len(text))
	origUTF16 = append(origUTF16, origPos)
	return units, offsets, origUTF16
}

// encodeKeepAllUTF16 encodes text for a multiline Korean text input with wordJoiner between adjacent runes of keep-all words, returning forward and reverse UTF-16 position maps (nil when no word joiners were inserted).
func encodeKeepAllUTF16(text string) (units []uint16, toLayout, fromLayout []uint32) {
	ranges := driver.KeepAllWordRanges(text)
	if len(ranges) == 0 {
		return utf16.Encode([]rune(text)), nil, nil
	}
	wi, origPos := 0, uint32(0)
	for i, r := range text {
		for wi < len(ranges) && ranges[wi][1] <= i {
			wi++
		}
		if wi < len(ranges) && i > ranges[wi][0] && i < ranges[wi][1] {
			units = append(units, wordJoiner)
			fromLayout = append(fromLayout, origPos)
		}
		n := len(units)
		units = utf16.AppendRune(units, r)
		added := len(units) - n
		for k := range added {
			toLayout = append(toLayout, uint32(n+k))
			fromLayout = append(fromLayout, origPos+uint32(k))
		}
		origPos += uint32(added)
	}
	toLayout = append(toLayout, uint32(len(units)))
	fromLayout = append(fromLayout, origPos)
	return units, toLayout, fromLayout
}
