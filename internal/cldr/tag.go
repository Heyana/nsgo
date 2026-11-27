package cldr

import "strings"

// Tag is a parsed BCP 47 language tag (language, optional script and region,
// and optional -u- Unicode extension key-value pairs).
type Tag struct {
	Lang   string
	Script string
	Region string
	Ext    map[string]string
}

// ParseTag parses a BCP 47 tag (accepting '_' or '-' separators). It returns
// ok=false for empty or malformed tags.
func ParseTag(s string) (Tag, bool) {
	if s == "" {
		return Tag{}, false
	}
	s = strings.ReplaceAll(s, "_", "-")
	parts := strings.Split(s, "-")
	if len(parts) == 0 {
		return Tag{}, false
	}
	lang := parts[0]
	if !isASCIIAlpha(lang) || len(lang) < 2 || len(lang) > 8 {
		return Tag{}, false
	}
	t := Tag{Lang: strings.ToLower(lang)}
	i := 1
	// Optional 4-letter script.
	if i < len(parts) && len(parts[i]) == 4 && isASCIIAlpha(parts[i]) {
		p := parts[i]
		t.Script = strings.ToUpper(p[:1]) + strings.ToLower(p[1:])
		i++
	}
	// Optional 2-letter or 3-digit region.
	if i < len(parts) && ((len(parts[i]) == 2 && isASCIIAlpha(parts[i])) || (len(parts[i]) == 3 && isASCIIDigit(parts[i]))) {
		t.Region = strings.ToUpper(parts[i])
		i++
	}
	// Remaining subtags: variants and extensions.
	for i < len(parts) {
		sub := parts[i]
		if sub == "" || !isASCIIAlnum(sub) {
			return Tag{}, false
		}
		if len(sub) == 1 {
			ext := strings.ToLower(sub)
			i++
			if ext == "u" {
				for i < len(parts) && len(parts[i]) > 1 {
					if !isASCIIAlnum(parts[i]) {
						return Tag{}, false
					}
					if len(parts[i]) == 2 {
						k := strings.ToLower(parts[i])
						i++
						var vals []string
						for i < len(parts) && len(parts[i]) >= 3 {
							if !isASCIIAlnum(parts[i]) {
								return Tag{}, false
							}
							vals = append(vals, strings.ToLower(parts[i]))
							i++
						}
						if t.Ext == nil {
							t.Ext = map[string]string{}
						}
						t.Ext[k] = strings.Join(vals, "-")
					} else {
						i++
					}
				}
				continue
			}
			// Skip other singleton extensions (-x-, -t-, etc.).
			for i < len(parts) && len(parts[i]) > 1 {
				if !isASCIIAlnum(parts[i]) {
					return Tag{}, false
				}
				i++
			}
			continue
		}
		// Variant subtag: 5-8 alnum or digit + 3 alnum.
		if len(sub) < 4 || len(sub) > 8 || (len(sub) == 4 && (sub[0] < '0' || sub[0] > '9')) {
			return Tag{}, false
		}
		i++
	}
	return t, true
}

// TypeForKey returns the value of Unicode extension key k (e.g. "hc", "fw").
func (t Tag) TypeForKey(k string) string {
	if t.Ext == nil {
		return ""
	}
	return t.Ext[k]
}

// CanonicalScript returns t.Script if present, else CLDR's likely script for t.Lang and t.Region.
func (t Tag) CanonicalScript() string {
	if t.Script != "" {
		return t.Script
	}
	return LikelyScript(t.Lang, t.Region)
}

// CanonicalRegion returns t.Region if present, else CLDR's likely region for t.Lang and script.
func (t Tag) CanonicalRegion() string {
	if t.Region != "" {
		return t.Region
	}
	return LikelyRegion(t.Lang, t.CanonicalScript())
}

// LikelyScript returns CLDR's likely script for lang and optional region.
func LikelyScript(lang, region string) string {
	switch lang {
	case "zh":
		switch region {
		case "TW", "HK", "MO":
			return "Hant"
		}
		return "Hans"
	case "yue":
		if region == "CN" {
			return "Hans"
		}
		return "Hant"
	case "sr":
		if region == "ME" {
			return "Latn"
		}
		return "Cyrl"
	case "az":
		if region == "IR" {
			return "Arab"
		}
		return "Latn"
	case "uz":
		if region == "AF" {
			return "Arab"
		}
		return "Latn"
	case "pa":
		if region == "PK" {
			return "Arab"
		}
		return "Guru"
	case "kk":
		if region == "CN" || region == "IR" {
			return "Arab"
		}
		return "Cyrl"
	case "ja":
		return "Jpan"
	case "ko":
		return "Kore"
	case "ar", "fa", "ur", "ps", "sd", "ug", "ks", "ku":
		return "Arab"
	case "he", "iw", "yi":
		return "Hebr"
	case "ru", "uk", "be", "bg", "mk", "ky", "mn", "tg", "ba", "cv", "ce", "tt", "os":
		return "Cyrl"
	case "el":
		return "Grek"
	case "hi", "mr", "ne", "kok", "sa", "mai", "bho", "doi":
		return "Deva"
	case "bn", "as":
		return "Beng"
	case "gu":
		return "Gujr"
	case "ta":
		return "Taml"
	case "te":
		return "Telu"
	case "kn":
		return "Knda"
	case "ml":
		return "Mlym"
	case "or":
		return "Orya"
	case "si":
		return "Sinh"
	case "th":
		return "Thai"
	case "lo":
		return "Laoo"
	case "my", "shn":
		return "Mymr"
	case "km":
		return "Khmr"
	case "ka":
		return "Geor"
	case "hy":
		return "Armn"
	case "am", "ti":
		return "Ethi"
	case "chr":
		return "Cher"
	}
	return "Latn"
}

// LikelyRegion returns CLDR's likely region for lang and script.
func LikelyRegion(lang, script string) string {
	if lang == "zh" && script == "Hant" {
		return "TW"
	}
	if lang == "yue" && script == "Hans" {
		return "CN"
	}
	switch lang {
	case "en":
		return "US"
	case "zh":
		return "CN"
	case "yue":
		return "HK"
	case "de":
		return "DE"
	case "fr":
		return "FR"
	case "es":
		return "ES"
	case "pt":
		return "BR"
	case "ja":
		return "JP"
	case "ko":
		return "KR"
	case "ru":
		return "RU"
	case "ar":
		return "EG"
	case "it":
		return "IT"
	case "nl":
		return "NL"
	case "pl":
		return "PL"
	case "tr":
		return "TR"
	case "sv":
		return "SE"
	case "da":
		return "DK"
	case "nb", "no", "nn":
		return "NO"
	case "fi":
		return "FI"
	case "cs":
		return "CZ"
	case "sk":
		return "SK"
	case "hu":
		return "HU"
	case "ro":
		return "RO"
	case "bg":
		return "BG"
	case "el":
		return "GR"
	case "he", "iw":
		return "IL"
	case "hi", "mr", "kok", "pa", "gu", "ta", "te", "kn", "ml", "or", "as":
		return "IN"
	case "th":
		return "TH"
	case "vi":
		return "VN"
	case "id", "jv":
		return "ID"
	case "ms":
		return "MY"
	case "uk":
		return "UA"
	case "fa":
		return "IR"
	case "ur", "sd":
		return "PK"
	case "bn":
		return "BD"
	case "ca", "gl", "eu":
		return "ES"
	case "hr":
		return "HR"
	case "sr":
		return "RS"
	case "sl":
		return "SI"
	case "lt":
		return "LT"
	case "lv":
		return "LV"
	case "et":
		return "EE"
	case "is":
		return "IS"
	case "ga":
		return "IE"
	case "cy", "gd":
		return "GB"
	case "fil", "tl":
		return "PH"
	case "sw":
		return "TZ"
	case "af", "zu":
		return "ZA"
	case "am", "ti":
		return "ET"
	case "km":
		return "KH"
	case "lo":
		return "LA"
	case "my", "shn":
		return "MM"
	case "ka":
		return "GE"
	case "hy":
		return "AM"
	case "kk":
		return "KZ"
	case "uz":
		return "UZ"
	case "az":
		return "AZ"
	case "be":
		return "BY"
	case "sq":
		return "AL"
	case "mk":
		return "MK"
	case "bs":
		return "BA"
	case "ne":
		return "NP"
	case "si":
		return "LK"
	case "ps":
		return "AF"
	case "mn":
		return "MN"
	case "ky":
		return "KG"
	case "tk":
		return "TM"
	case "ha", "yo", "ig", "pcm":
		return "NG"
	case "so":
		return "SO"
	case "rm":
		return "CH"
	case "ht":
		return "HT"
	case "qu":
		return "PE"
	case "ak":
		return "GH"
	case "chr":
		return "US"
	}
	return ""
}

// ParentLocale returns the CLDR parent locale of lang, or "" after the base
// language (or a non-default script root such as zh-Hant).
func ParentLocale(lang string) string {
	t, ok := ParseTag(lang)
	if !ok || t.Lang == "und" {
		return ""
	}
	switch t.Lang {
	case "en":
		if t.Region != "" && t.Script == "" {
			switch t.Region {
			case "001":
				return "en"
			case "150":
				return "en-001"
			case "US", "AS", "BI", "GU", "MH", "MP", "PR", "UM", "VI":
				return "en"
			case "AT", "BE", "CH", "CY", "CZ", "DE", "DK", "EE", "ES", "FI", "FR", "HU", "IT", "LT", "LU", "LV", "NL", "NO", "PL", "PT", "RO", "SE", "SI", "SK":
				return "en-150"
			default:
				return "en-001"
			}
		}
	case "zh":
		if t.Region != "" {
			if t.Script == "Hant" || (t.Script == "" && (t.Region == "TW" || t.Region == "HK" || t.Region == "MO")) {
				if t.Region == "MO" && t.Script == "Hant" {
					return "zh-Hant-HK"
				}
				return "zh-Hant"
			}
			if t.Script == "Hans" {
				return "zh-Hans"
			}
			return "zh-Hans"
		}
		if t.Script == "Hant" {
			return ""
		}
		if t.Script == "Hans" {
			return "zh"
		}
		return ""
	case "pt":
		if t.Region != "" && t.Script == "" {
			switch t.Region {
			case "AO", "CH", "CV", "GQ", "GW", "LU", "MO", "MZ", "ST", "TL":
				return "pt-PT"
			}
			return "pt"
		}
	case "es":
		if t.Region != "" && t.Script == "" {
			switch t.Region {
			case "419", "ES", "EA", "IC", "GQ", "PH":
				return "es"
			default:
				return "es-419"
			}
		}
	}
	if t.Region != "" {
		if t.Script != "" {
			return t.Lang + "-" + t.Script
		}
		return t.Lang
	}
	if t.Script != "" {
		if t.Script != LikelyScript(t.Lang, "") {
			return ""
		}
		return t.Lang
	}
	return ""
}

// MatchCatalog selects the best supported language in langs for locale, or
// langs[0] when no supported language matches with high confidence.
func MatchCatalog(langs []string, locale string) string {
	if len(langs) == 0 {
		return ""
	}
	want, ok := ParseTag(locale)
	if !ok || want.Lang == "und" {
		return langs[0]
	}
	wantScript := want.CanonicalScript()
	wantRegion := want.CanonicalRegion()
	wantAnc := ancestors(want)

	bestIdx := -1
	bestScore := 1000
	for i, cand := range langs {
		ct, ok := ParseTag(cand)
		if !ok || ct.Lang != want.Lang {
			continue
		}
		cScript := ct.CanonicalScript()
		if cScript != wantScript {
			continue
		}
		cRegion := ct.CanonicalRegion()
		score := 50
		switch {
		case cand == locale:
			score = 0
		case ct.Region == want.Region && ct.Script == want.Script:
			score = 1
		case cRegion == wantRegion:
			if ct.Region != "" {
				score = 2
			} else {
				score = 3
			}
		case ct.Region != "" && shareNonRootParent(wantAnc, ancestors(ct)):
			// E.g. en-AU matches en-GB via shared parent en-001 better than en (en-US).
			score = 4
		case ct.Region == "":
			// Language (or language+script) catch-all such as de for de-CH or zh-Hant for zh-HK.
			score = 5
		default:
			score = 6
		}
		if score < bestScore {
			bestScore = score
			bestIdx = i
		}
	}
	if bestIdx >= 0 {
		return langs[bestIdx]
	}
	return langs[0]
}

func ancestors(t Tag) []string {
	var out []string
	cur := t.Lang
	if t.Script != "" {
		cur += "-" + t.Script
	}
	if t.Region != "" {
		cur += "-" + t.Region
	}
	for cur != "" {
		out = append(out, cur)
		cur = ParentLocale(cur)
	}
	return out
}

func shareNonRootParent(a, b []string) bool {
	for _, x := range a {
		if !strings.Contains(x, "-") {
			continue
		}
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func isASCIIAlpha(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')) {
			return false
		}
	}
	return len(s) > 0
}

func isASCIIDigit(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return len(s) > 0
}

func isASCIIAlnum(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')) {
			return false
		}
	}
	return len(s) > 0
}
