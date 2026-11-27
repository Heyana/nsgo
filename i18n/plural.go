package i18n

import (
	"math"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/cldr"
)

// pluralOperands holds the CLDR plural operands derived from a formatted
// decimal (n = absolute value, i = integer digits, v = visible fraction digit
// count, f = visible fraction digits as integer).
type pluralOperands struct {
	i int
	v int
	f int
}

func operandsOf(n float64) pluralOperands {
	s := strconv.FormatFloat(math.Abs(n), 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	whole, frac, _ := strings.Cut(s, ".")
	i, err := strconv.Atoi(whole)
	if err != nil {
		// Beyond int's range: a million's multiple, as the rules see it.
		i = 1_000_000_000
	}
	f, _ := strconv.Atoi(frac)
	return pluralOperands{i: i, v: len(frac), f: f}
}

func langBase(lang string) string {
	if t, ok := cldr.ParseTag(lang); ok {
		return t.Lang
	}
	b, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(lang, "_", "-")), "-")
	return b
}

// cardinalCategory implements Unicode CLDR cardinal plural rules without
// importing golang.org/x/text/feature/plural.
func cardinalCategory(lang string, n float64) Category {
	op := operandsOf(n)
	i, v, f := op.i, op.v, op.f
	i10, i100 := i%10, i%100

	switch langBase(lang) {
	// No plural distinction (other only).
	case "zh", "yue", "ja", "ko", "vi", "th", "lo", "km", "my", "id", "ms", "jv", "su", "tr", "az", "ka", "kk", "ky", "uz", "mn", "bo", "dz", "ug", "yo", "ig", "wo", "bm", "sg", "to", "sm", "haw", "mi":
		return Other

	// French / Portuguese / Armenian / Persian / Hindi / Bengali / Gujarati / Kannada / Zulu / Amharic / etc.
	case "fr":
		if i == 0 || i == 1 {
			return One
		}
		if v == 0 && i != 0 && i%1000000 == 0 {
			return Many
		}
		return Other
	case "pt":
		if i == 0 || i == 1 {
			return One
		}
		if v == 0 && i != 0 && i%1000000 == 0 {
			return Many
		}
		return Other
	case "hi", "bn", "gu", "kn", "fa", "am", "as", "zu", "hy", "mr", "ne":
		if i == 0 || (i == 1 && v == 0) {
			return One
		}
		return Other

	// Spanish / Italian / Catalan (one: i=1, v=0; many: e/millions).
	case "es", "it", "ca":
		if i == 1 && v == 0 {
			return One
		}
		if v == 0 && i != 0 && i%1000000 == 0 {
			return Many
		}
		return Other

	// East Slavic: Russian, Ukrainian.
	case "ru", "uk":
		if v == 0 {
			if i10 == 1 && i100 != 11 {
				return One
			}
			if i10 >= 2 && i10 <= 4 && (i100 < 12 || i100 > 14) {
				return Few
			}
			if i10 == 0 || (i10 >= 5 && i10 <= 9) || (i100 >= 11 && i100 <= 14) {
				return Many
			}
		}
		return Other

	// Belarusian.
	case "be":
		if v == 0 {
			if i10 == 1 && i100 != 11 {
				return One
			}
			if i10 >= 2 && i10 <= 4 && (i100 < 12 || i100 > 14) {
				return Few
			}
			if i10 == 0 || (i10 >= 5 && i10 <= 9) || (i100 >= 11 && i100 <= 14) {
				return Many
			}
		}
		return Other

	// Polish.
	case "pl":
		if v == 0 {
			if i == 1 {
				return One
			}
			if i10 >= 2 && i10 <= 4 && (i100 < 12 || i100 > 14) {
				return Few
			}
			if i != 1 && (i10 == 0 || i10 == 1) || (i10 >= 5 && i10 <= 9) || (i100 >= 12 && i100 <= 14) {
				return Many
			}
		}
		return Other

	// Czech, Slovak.
	case "cs", "sk":
		if v == 0 {
			if i == 1 {
				return One
			}
			if i >= 2 && i <= 4 {
				return Few
			}
			return Other
		}
		return Many

	// Bosnian, Croatian, Serbian.
	case "bs", "hr", "sr":
		f10, f100 := f%10, f%100
		if (v == 0 && i10 == 1 && i100 != 11) || (f10 == 1 && f100 != 11) {
			return One
		}
		if (v == 0 && i10 >= 2 && i10 <= 4 && (i100 < 12 || i100 > 14)) || (f10 >= 2 && f10 <= 4 && (f100 < 12 || f100 > 14)) {
			return Few
		}
		return Other

	// Slovenian.
	case "sl":
		if v == 0 {
			switch i100 {
			case 1:
				return One
			case 2:
				return Two
			case 3, 4:
				return Few
			}
		}
		return Other

	// Arabic.
	case "ar":
		if v == 0 && i == 0 {
			return Zero
		}
		if v == 0 && i == 1 {
			return One
		}
		if v == 0 && i == 2 {
			return Two
		}
		if v == 0 && i100 >= 3 && i100 <= 10 {
			return Few
		}
		if v == 0 && i100 >= 11 && i100 <= 99 {
			return Many
		}
		return Other

	// Hebrew.
	case "he", "iw":
		if i == 1 && v == 0 {
			return One
		}
		if i == 2 && v == 0 {
			return Two
		}
		return Other

	// Romanian.
	case "ro":
		if i == 1 && v == 0 {
			return One
		}
		if v != 0 || i == 0 || (i != 1 && i100 >= 1 && i100 <= 19) {
			return Few
		}
		return Other

	// Lithuanian.
	case "lt":
		if i10 == 1 && (i100 < 11 || i100 > 19) {
			return One
		}
		if i10 >= 2 && i10 <= 9 && (i100 < 11 || i100 > 19) {
			return Few
		}
		if f != 0 {
			return Many
		}
		return Other

	// Latvian.
	case "lv":
		f10, f100 := f%10, f%100
		if i10 == 0 || (i100 >= 11 && i100 <= 19) || (v == 2 && f100 >= 11 && f100 <= 19) {
			return Zero
		}
		if (i10 == 1 && i100 != 11) || (v == 2 && f10 == 1 && f100 != 11) || (v != 2 && f10 == 1) {
			return One
		}
		return Other

	// Welsh.
	case "cy":
		if v == 0 {
			switch i {
			case 0:
				return Zero
			case 1:
				return One
			case 2:
				return Two
			case 3:
				return Few
			case 6:
				return Many
			}
		}
		return Other

	// Irish.
	case "ga":
		if v == 0 {
			switch {
			case i == 1:
				return One
			case i == 2:
				return Two
			case i >= 3 && i <= 6:
				return Few
			case i >= 7 && i <= 10:
				return Many
			}
		}
		return Other

	// Icelandic, Macedonian.
	case "is":
		if (v == 0 && i10 == 1 && i100 != 11) || f%10 == 1 && f%100 != 11 {
			return One
		}
		return Other
	case "mk":
		if (v == 0 && i10 == 1 && i100 != 11) || (f%10 == 1 && f%100 != 11) {
			return One
		}
		return Other

	// Filipino / Tagalog.
	case "fil", "tl":
		if v == 0 && (i == 1 || i == 2 || i == 3) || (v == 0 && i10 != 4 && i10 != 6 && i10 != 9) || (v != 0 && f%10 != 4 && f%10 != 6 && f%10 != 9) {
			return One
		}
		return Other

	// Default (English, German, Dutch, Scandinavian, Greek, Finnish, Hungarian, Estonian, Bulgarian, Swahili, etc.).
	default:
		if i == 1 && v == 0 {
			return One
		}
		return Other
	}
}

// ordinalCategory implements Unicode CLDR ordinal plural rules.
func ordinalCategory(lang string, n float64) Category {
	op := operandsOf(n)
	i := op.i
	i10, i100 := i%10, i%100

	switch langBase(lang) {
	case "en":
		if i10 == 1 && i100 != 11 {
			return One
		}
		if i10 == 2 && i100 != 12 {
			return Two
		}
		if i10 == 3 && i100 != 13 {
			return Few
		}
		return Other
	case "fr", "ga", "hy", "lo", "ms", "vi", "fil", "tl", "mo", "ro", "sq":
		if i == 1 {
			return One
		}
		return Other
	case "sv":
		if (i10 == 1 || i10 == 2) && i100 != 11 && i100 != 12 {
			return One
		}
		return Other
	case "it":
		switch i {
		case 8, 11, 80, 800:
			return Many
		}
		return Other
	case "ca":
		switch i {
		case 1, 3:
			return One
		case 2:
			return Two
		case 4:
			return Few
		}
		return Other
	case "cy":
		switch i {
		case 0, 7, 8, 9:
			return Zero
		case 1:
			return One
		case 2:
			return Two
		case 3, 4:
			return Few
		case 5, 6:
			return Many
		}
		return Other
	case "hu":
		if i == 1 || i == 5 {
			return One
		}
		return Other
	case "uk":
		if i10 == 3 && i100 != 13 {
			return Few
		}
		return Other
	case "be":
		if (i10 == 2 || i10 == 3) && i100 != 12 && i100 != 13 {
			return Few
		}
		return Other
	case "hi":
		switch i {
		case 1:
			return One
		case 2, 3:
			return Two
		case 4:
			return Few
		case 6:
			return Many
		}
		return Other
	case "bn", "as":
		switch i {
		case 1, 5, 7, 8, 9, 10:
			return One
		case 2, 3:
			return Two
		case 4:
			return Few
		case 6:
			return Many
		}
		return Other
	case "ka":
		if i == 1 {
			return One
		}
		if i == 0 || (i100 >= 2 && i100 <= 20) || i100 == 40 || i100 == 60 || i100 == 80 {
			return Many
		}
		return Other
	case "mk":
		if i10 == 1 && i100 != 11 {
			return One
		}
		if i10 == 2 && i100 != 12 {
			return Two
		}
		if (i10 == 7 || i10 == 8) && i100 != 17 && i100 != 18 {
			return Many
		}
		return Other
	default:
		return Other
	}
}
