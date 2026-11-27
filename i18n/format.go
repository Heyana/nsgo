package i18n

import (
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/cldr"
)

// Category is a number's plural category in a language, by CLDR's
// rules: one and other in English, other alone in Chinese, six in Arabic.
type Category uint8

const (
	Other Category = iota
	Zero
	One
	Two
	Few
	Many
)

// String returns the category's name in a message: "one", "other", ...
func (c Category) String() string {
	return [...]string{"other", "zero", "one", "two", "few", "many"}[c]
}

// Cardinal returns the plural category of n in lang, as FormatNumber
// writes it: 1.0004 is written 1, whose category in English is one.
func Cardinal(lang string, n float64) Category { return cardinalCategory(lang, n) }

// Ordinal returns the ordinal category of n in lang: one, two, few and
// other in English (1st, 2nd, 3rd, 4th).
func Ordinal(lang string, n float64) Category { return ordinalCategory(lang, n) }

type numberSymbols struct {
	decimal     string
	group       string
	minGroup    int // minimum integer digits before inserting grouping (4 means group at >= 1000, 5 at >= 10000)
	indianGroup bool
	percentPre  string
	percentPost string
}

func symbolsFor(lang string) numberSymbols {
	t, ok := cldr.ParseTag(lang)
	base, region := "", ""
	if ok {
		base, region = t.Lang, t.Region
	} else {
		base = langBase(lang)
	}
	// Defaults match root / en.
	s := numberSymbols{decimal: ".", group: ",", minGroup: 4, percentPost: "%"}
	switch base {
	case "de":
		if region == "CH" || region == "LI" {
			s.decimal, s.group, s.percentPost = ".", "\u2019", "\u00a0%"
		} else {
			s.decimal, s.group, s.percentPost = ",", ".", "\u00a0%"
		}
	case "fr":
		s.decimal, s.group, s.percentPost = ",", "\u00a0", "\u00a0%"
	case "es":
		if region == "MX" || region == "US" || region == "PR" || region == "DO" || region == "GT" || region == "HN" || region == "NI" || region == "PA" || region == "SV" || region == "PE" {
			s.decimal, s.group, s.percentPost = ".", ",", "\u00a0%"
		} else {
			s.decimal, s.group, s.minGroup, s.percentPost = ",", ".", 5, "\u00a0%"
		}
	case "it":
		if region == "CH" {
			s.decimal, s.group, s.percentPost = ".", "\u2019", "%"
		} else {
			s.decimal, s.group, s.percentPost = ",", ".", "%"
		}
	case "nl", "pt", "da", "el", "ro", "id", "vi", "ca", "gl", "eu", "is", "hr", "sr", "bs", "sl", "mk":
		s.decimal, s.group = ",", "."
		if base == "da" || base == "el" || base == "sl" || base == "hr" {
			s.percentPost = "\u00a0%"
		}
	case "tr":
		s.decimal, s.group, s.percentPre, s.percentPost = ",", ".", "%", ""
	case "pl", "cs", "sk", "fi", "sv", "nb", "nn", "no", "ru", "uk", "be", "bg", "hu", "lt", "lv", "et", "kk", "ky", "uz", "az", "ka", "hy", "sq":
		s.decimal, s.group = ",", "\u00a0"
		if base == "pl" || base == "bg" || base == "et" {
			s.minGroup = 5
		}
		if base == "cs" || base == "sk" || base == "fi" || base == "sv" || base == "nb" || base == "nn" || base == "no" || base == "lt" || base == "ka" {
			s.percentPost = "\u00a0%"
		}
	case "hi", "bn", "gu", "kn", "ml", "mr", "or", "pa", "ta", "te", "as":
		s.indianGroup = true
	case "en":
		if region == "IN" {
			s.indianGroup = true
		}
	}
	return s
}

func formatDecimal(lang string, n float64, minFrac, maxFrac int, group, percent bool) string {
	if math.IsNaN(n) {
		return "NaN"
	}
	if math.IsInf(n, 1) {
		return "∞"
	}
	if math.IsInf(n, -1) {
		return "-∞"
	}
	sym := symbolsFor(lang)
	if percent {
		n *= 100
	}
	neg := math.Signbit(n)
	abs := math.Abs(n)
	raw := strconv.FormatFloat(abs, 'f', maxFrac, 64)
	whole, frac, hasFrac := strings.Cut(raw, ".")
	if hasFrac {
		for len(frac) > minFrac && frac[len(frac)-1] == '0' {
			frac = frac[:len(frac)-1]
		}
	}
	if whole == "0" && frac == "" {
		neg = false
	}
	if group && len(whole) >= sym.minGroup {
		whole = insertGrouping(whole, sym.group, sym.indianGroup)
	}
	var b strings.Builder
	if neg {
		b.WriteByte('-')
	}
	if percent && sym.percentPre != "" {
		b.WriteString(sym.percentPre)
	}
	b.WriteString(whole)
	if len(frac) > 0 {
		b.WriteString(sym.decimal)
		b.WriteString(frac)
	}
	if percent && sym.percentPost != "" {
		b.WriteString(sym.percentPost)
	}
	return b.String()
}

func insertGrouping(whole, sep string, indian bool) string {
	n := len(whole)
	if n <= 3 {
		return whole
	}
	var parts []string
	end := n
	first := 3
	step := 3
	if indian {
		step = 2
	}
	if end > first {
		parts = append(parts, whole[end-first:end])
		end -= first
		for end > step {
			parts = append(parts, whole[end-step:end])
			end -= step
		}
	}
	if end > 0 {
		parts = append(parts, whole[:end])
	}
	var b strings.Builder
	for i := len(parts) - 1; i >= 0; i-- {
		if i < len(parts)-1 {
			b.WriteString(sep)
		}
		b.WriteString(parts[i])
	}
	return b.String()
}

// FormatNumber writes n in lang's way, with up to three fraction digits
// and grouped: 1,234.5 in English, 1.234,5 in German.
func FormatNumber(lang string, n float64) string { return formatDecimal(lang, n, 0, 3, true, false) }

// FormatInteger writes n rounded to an integer in lang's way.
func FormatInteger(lang string, n float64) string { return formatDecimal(lang, n, 0, 0, true, false) }

// FormatPercent writes n, a fraction, as a percentage in lang's way: 25%
// in English for 0.25, 25 % in German.
func FormatPercent(lang string, n float64) string { return formatDecimal(lang, n, 0, 0, true, true) }

// FormatNumberSkeleton writes n in lang's way by an ICU number skeleton,
// as a message's {n, number, ::.00} does: of its stems percent, integer,
// the fraction digits (.00 two, .0# one or two) and group-off.
func FormatNumberSkeleton(lang string, n float64, skeleton string) string {
	percent := false
	group := true
	minFrac, maxFrac := 0, 3
	setFrac := false
	for _, stem := range strings.Fields(skeleton) {
		switch {
		case stem == "percent":
			percent = true
			if !setFrac {
				maxFrac = 0
			}
		case stem == "integer", stem == "precision-integer":
			minFrac, maxFrac, setFrac = 0, 0, true
		case stem == "group-off":
			group = false
		case strings.HasPrefix(stem, "."):
			minFrac = strings.Count(stem, "0")
			maxFrac = len(stem) - 1
			setFrac = true
		}
	}
	return formatDecimal(lang, n, minFrac, maxFrac, group, percent)
}

// Style is the length of a date or a time as CLDR's patterns write it.
type Style = cldr.Style

const (
	// NoStyle leaves the date, or the time, out.
	NoStyle = cldr.NoStyle
	// ShortStyle is numeric: "1/5/26", "1:07 PM".
	ShortStyle = cldr.ShortStyle
	// MediumStyle abbreviates: "Jan 5, 2026", "1:07:09 PM".
	MediumStyle = cldr.MediumStyle
	// LongStyle writes the month out: "January 5, 2026".
	LongStyle = cldr.LongStyle
	// FullStyle writes everything: "Monday, January 5, 2026".
	FullStyle = cldr.FullStyle
)

// DateOptions are how FormatDate writes a time: its date, its time or
// both by style, or else the fields of a skeleton of CLDR's letters
// ("yMMMd", "jm") in the order and form the language writes them.
type DateOptions = cldr.DateOptions

// FormatDate writes t, in its location, in lang's way, in the Gregorian
// calendar, by CLDR's patterns and names: "Jan 5, 2026", "2026年1月5日".
// Time zones are written as offsets from GMT.
func FormatDate(lang string, t time.Time, o DateOptions) string { return cldr.FormatDate(lang, t, o) }

// RegionalSettings are how weeks, times and measures read, in Intl's
// names: FirstDayOfWeek 1 for Monday to 7 for Sunday, HourCycle "h11",
// "h12", "h23" or "h24", MeasurementSystem "metric", "us" or "uk",
// TemperatureUnit "celsius" or "fahrenheit".
type RegionalSettings = cldr.Regional

// Regional returns how the user reads weeks, times and measures under
// cx: what they set on the platform (core.Env.LocaleSettings), else what
// the region of the locale the app runs in does, by CLDR's data. Read in
// a binding, it follows them as they change.
func Regional(cx *core.Scope) RegionalSettings {
	env := cx.Env()
	locale, s := env.Locale(), env.LocaleSettings()
	if s.Region != "" {
		// The language's, in the region whose formats the user chose.
		lang, _, _ := strings.Cut(locale, "-")
		if lang == "" {
			lang = "und"
		}
		locale = lang + "-" + s.Region
	}
	r := cldr.RegionalOf(locale)
	if s.FirstDayOfWeek >= 1 && s.FirstDayOfWeek <= 7 {
		r.FirstDayOfWeek = s.FirstDayOfWeek
	}
	if s.HourCycle != "" {
		r.HourCycle = s.HourCycle
	}
	if s.MeasurementSystem != "" {
		r.MeasurementSystem = s.MeasurementSystem
	}
	if s.TemperatureUnit != "" {
		r.TemperatureUnit = s.TemperatureUnit
	}
	return r
}

// WithHourCycle is lang with the hour cycle the user set under cx as its
// Unicode extension (-u-hc-), which FormatDate writes times in; lang as
// it is where they set none.
func WithHourCycle(cx *core.Scope, lang string) string {
	if strings.Contains(lang, "-u-") {
		return lang
	}
	hc := cx.Env().LocaleSettings().HourCycle
	if hc == "" {
		return lang
	}
	return lang + "-u-hc-" + hc
}
