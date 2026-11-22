package windows

import (
	"encoding/binary"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
)

// winSystemTime matches Win32's SYSTEMTIME layout.
type winSystemTime struct {
	Year         uint16
	Month        uint16
	DayOfWeek    uint16
	Day          uint16
	Hour         uint16
	Minute       uint16
	Second       uint16
	Milliseconds uint16
}

// winDynamicTimeZoneInfo matches Win32's DYNAMIC_TIME_ZONE_INFORMATION layout (432 bytes).
type winDynamicTimeZoneInfo struct {
	Bias                        int32
	StandardName                [32]uint16
	StandardDate                winSystemTime
	StandardBias                int32
	DaylightName                [32]uint16
	DaylightDate                winSystemTime
	DaylightBias                int32
	TimeZoneKeyName             [128]uint16
	DynamicDaylightTimeDisabled uint8
	_                           [3]byte
}

type windowsZoneMeta struct {
	iana string
	std  string
	dst  string
}

// windowsZones maps Windows time zone key names (CLDR windowsZones.xml territory "001") to their canonical IANA zone IDs and standard/daylight abbreviations.
var windowsZones = map[string]windowsZoneMeta{
	"Egypt Standard Time":             {"Africa/Cairo", "EET", "EEST"},
	"Morocco Standard Time":           {"Africa/Casablanca", "+00", "+01"},
	"South Africa Standard Time":      {"Africa/Johannesburg", "SAST", "SAST"},
	"South Sudan Standard Time":       {"Africa/Juba", "CAT", "CAT"},
	"Sudan Standard Time":             {"Africa/Khartoum", "CAT", "CAT"},
	"W. Central Africa Standard Time": {"Africa/Lagos", "WAT", "WAT"},
	"E. Africa Standard Time":         {"Africa/Nairobi", "EAT", "EAT"},
	"Sao Tome Standard Time":          {"Africa/Sao_Tome", "GMT", "GMT"},
	"Libya Standard Time":             {"Africa/Tripoli", "EET", "EET"},
	"Namibia Standard Time":           {"Africa/Windhoek", "CAT", "CAT"},
	"Aleutian Standard Time":          {"America/Adak", "HST", "HDT"},
	"Alaskan Standard Time":           {"America/Anchorage", "AKST", "AKDT"},
	"Tocantins Standard Time":         {"America/Araguaina", "-03", "-03"},
	"Paraguay Standard Time":          {"America/Asuncion", "-04", "-03"},
	"Bahia Standard Time":             {"America/Bahia", "-03", "-03"},
	"SA Pacific Standard Time":        {"America/Bogota", "-05", "-05"},
	"Argentina Standard Time":         {"America/Argentina/Buenos_Aires", "-03", "-03"},
	"Eastern Standard Time (Mexico)":  {"America/Cancun", "EST", "EST"},
	"Venezuela Standard Time":         {"America/Caracas", "-04", "-04"},
	"SA Eastern Standard Time":        {"America/Cayenne", "-03", "-03"},
	"Central Standard Time":           {"America/Chicago", "CST", "CDT"},
	"Central Brazilian Standard Time": {"America/Cuiaba", "-04", "-04"},
	"Mountain Standard Time":          {"America/Denver", "MST", "MDT"},
	"Greenland Standard Time":         {"America/Nuuk", "-02", "-01"},
	"Turks And Caicos Standard Time":  {"America/Grand_Turk", "EST", "EDT"},
	"Central America Standard Time":   {"America/Guatemala", "CST", "CST"},
	"Atlantic Standard Time":          {"America/Halifax", "AST", "ADT"},
	"Cuba Standard Time":              {"America/Havana", "CST", "CDT"},
	"US Eastern Standard Time":        {"America/Indiana/Indianapolis", "EST", "EDT"},
	"SA Western Standard Time":        {"America/La_Paz", "-04", "-04"},
	"Pacific Standard Time":           {"America/Los_Angeles", "PST", "PDT"},
	"Mountain Standard Time (Mexico)": {"America/Mazatlan", "MST", "MST"},
	"Central Standard Time (Mexico)":  {"America/Mexico_City", "CST", "CST"},
	"Saint Pierre Standard Time":      {"America/Miquelon", "-03", "-02"},
	"Montevideo Standard Time":        {"America/Montevideo", "-03", "-03"},
	"Eastern Standard Time":           {"America/New_York", "EST", "EDT"},
	"US Mountain Standard Time":       {"America/Phoenix", "MST", "MST"},
	"Haiti Standard Time":             {"America/Port-au-Prince", "EST", "EDT"},
	"Magallanes Standard Time":        {"America/Punta_Arenas", "-03", "-03"},
	"Canada Central Standard Time":    {"America/Regina", "CST", "CST"},
	"Pacific SA Standard Time":        {"America/Santiago", "-04", "-03"},
	"E. South America Standard Time":  {"America/Sao_Paulo", "-03", "-03"},
	"Newfoundland Standard Time":      {"America/St_Johns", "NST", "NDT"},
	"Pacific Standard Time (Mexico)":  {"America/Tijuana", "PST", "PDT"},
	"Yukon Standard Time":             {"America/Whitehorse", "MST", "MST"},
	"Jordan Standard Time":            {"Asia/Amman", "+03", "+03"},
	"Arabic Standard Time":            {"Asia/Baghdad", "+03", "+03"},
	"Azerbaijan Standard Time":        {"Asia/Baku", "+04", "+04"},
	"SE Asia Standard Time":           {"Asia/Bangkok", "+07", "+07"},
	"Altai Standard Time":             {"Asia/Barnaul", "+07", "+07"},
	"Middle East Standard Time":       {"Asia/Beirut", "EET", "EEST"},
	"Central Asia Standard Time":      {"Asia/Bishkek", "+06", "+06"},
	"India Standard Time":             {"Asia/Kolkata", "IST", "IST"},
	"Transbaikal Standard Time":       {"Asia/Chita", "+09", "+09"},
	"Sri Lanka Standard Time":         {"Asia/Colombo", "+0530", "+0530"},
	"Syria Standard Time":             {"Asia/Damascus", "+03", "+03"},
	"Bangladesh Standard Time":        {"Asia/Dhaka", "+06", "+06"},
	"Arabian Standard Time":           {"Asia/Dubai", "+04", "+04"},
	"West Bank Standard Time":         {"Asia/Hebron", "EET", "EEST"},
	"W. Mongolia Standard Time":       {"Asia/Hovd", "+07", "+07"},
	"North Asia East Standard Time":   {"Asia/Irkutsk", "+08", "+08"},
	"Israel Standard Time":            {"Asia/Jerusalem", "IST", "IDT"},
	"Afghanistan Standard Time":       {"Asia/Kabul", "+0430", "+0430"},
	"Russia Time Zone 11":             {"Asia/Kamchatka", "+12", "+12"},
	"Pakistan Standard Time":          {"Asia/Karachi", "PKT", "PKT"},
	"Nepal Standard Time":             {"Asia/Kathmandu", "+0545", "+0545"},
	"North Asia Standard Time":        {"Asia/Krasnoyarsk", "+07", "+07"},
	"Magadan Standard Time":           {"Asia/Magadan", "+11", "+11"},
	"N. Central Asia Standard Time":   {"Asia/Novosibirsk", "+07", "+07"},
	"Omsk Standard Time":              {"Asia/Omsk", "+06", "+06"},
	"North Korea Standard Time":       {"Asia/Pyongyang", "KST", "KST"},
	"Qyzylorda Standard Time":         {"Asia/Qyzylorda", "+05", "+05"},
	"Myanmar Standard Time":           {"Asia/Yangon", "+0630", "+0630"},
	"Arab Standard Time":              {"Asia/Riyadh", "+03", "+03"},
	"Sakhalin Standard Time":          {"Asia/Sakhalin", "+11", "+11"},
	"Korea Standard Time":             {"Asia/Seoul", "KST", "KST"},
	"China Standard Time":             {"Asia/Shanghai", "CST", "CST"},
	"Singapore Standard Time":         {"Asia/Singapore", "+08", "+08"},
	"Russia Time Zone 10":             {"Asia/Srednekolymsk", "+11", "+11"},
	"Taipei Standard Time":            {"Asia/Taipei", "CST", "CST"},
	"West Asia Standard Time":         {"Asia/Tashkent", "+05", "+05"},
	"Georgian Standard Time":          {"Asia/Tbilisi", "+04", "+04"},
	"Iran Standard Time":              {"Asia/Tehran", "+0330", "+0330"},
	"Tokyo Standard Time":             {"Asia/Tokyo", "JST", "JST"},
	"Tomsk Standard Time":             {"Asia/Tomsk", "+07", "+07"},
	"Ulaanbaatar Standard Time":       {"Asia/Ulaanbaatar", "+08", "+08"},
	"Vladivostok Standard Time":       {"Asia/Vladivostok", "+10", "+10"},
	"Yakutsk Standard Time":           {"Asia/Yakutsk", "+09", "+09"},
	"Ekaterinburg Standard Time":      {"Asia/Yekaterinburg", "+05", "+05"},
	"Caucasus Standard Time":          {"Asia/Yerevan", "+04", "+04"},
	"Azores Standard Time":            {"Atlantic/Azores", "-01", "+00"},
	"Cape Verde Standard Time":        {"Atlantic/Cape_Verde", "-01", "-01"},
	"Greenwich Standard Time":         {"Atlantic/Reykjavik", "GMT", "GMT"},
	"Cen. Australia Standard Time":    {"Australia/Adelaide", "ACST", "ACDT"},
	"E. Australia Standard Time":      {"Australia/Brisbane", "AEST", "AEST"},
	"AUS Central Standard Time":       {"Australia/Darwin", "ACST", "ACST"},
	"Aus Central W. Standard Time":    {"Australia/Eucla", "+0845", "+0845"},
	"Tasmania Standard Time":          {"Australia/Hobart", "AEST", "AEDT"},
	"Lord Howe Standard Time":         {"Australia/Lord_Howe", "+1030", "+11"},
	"W. Australia Standard Time":      {"Australia/Perth", "AWST", "AWST"},
	"AUS Eastern Standard Time":       {"Australia/Sydney", "AEST", "AEDT"},
	"UTC-11":                          {"Etc/GMT+11", "-11", "-11"},
	"Dateline Standard Time":          {"Etc/GMT+12", "-12", "-12"},
	"UTC-02":                          {"Etc/GMT+2", "-02", "-02"},
	"UTC-08":                          {"Etc/GMT+8", "-08", "-08"},
	"UTC-09":                          {"Etc/GMT+9", "-09", "-09"},
	"UTC+12":                          {"Etc/GMT-12", "+12", "+12"},
	"UTC+13":                          {"Etc/GMT-13", "+13", "+13"},
	"UTC":                             {"Etc/UTC", "UTC", "UTC"},
	"Astrakhan Standard Time":         {"Europe/Astrakhan", "+04", "+04"},
	"W. Europe Standard Time":         {"Europe/Berlin", "CET", "CEST"},
	"GTB Standard Time":               {"Europe/Bucharest", "EET", "EEST"},
	"Central Europe Standard Time":    {"Europe/Budapest", "CET", "CEST"},
	"E. Europe Standard Time":         {"Europe/Chisinau", "EET", "EEST"},
	"Turkey Standard Time":            {"Europe/Istanbul", "+03", "+03"},
	"Kaliningrad Standard Time":       {"Europe/Kaliningrad", "EET", "EET"},
	"FLE Standard Time":               {"Europe/Kyiv", "EET", "EEST"},
	"GMT Standard Time":               {"Europe/London", "GMT", "BST"},
	"Belarus Standard Time":           {"Europe/Minsk", "+03", "+03"},
	"Russian Standard Time":           {"Europe/Moscow", "MSK", "MSK"},
	"Romance Standard Time":           {"Europe/Paris", "CET", "CEST"},
	"Russia Time Zone 3":              {"Europe/Samara", "+04", "+04"},
	"Saratov Standard Time":           {"Europe/Saratov", "+04", "+04"},
	"Volgograd Standard Time":         {"Europe/Volgograd", "MSK", "MSK"},
	"Central European Standard Time":  {"Europe/Warsaw", "CET", "CEST"},
	"Mauritius Standard Time":         {"Indian/Mauritius", "+04", "+04"},
	"Samoa Standard Time":             {"Pacific/Apia", "+13", "+13"},
	"New Zealand Standard Time":       {"Pacific/Auckland", "NZST", "NZDT"},
	"Bougainville Standard Time":      {"Pacific/Bougainville", "+11", "+11"},
	"Chatham Islands Standard Time":   {"Pacific/Chatham", "+1245", "+1345"},
	"Easter Island Standard Time":     {"Pacific/Easter", "-06", "-05"},
	"Fiji Standard Time":              {"Pacific/Fiji", "+12", "+12"},
	"Central Pacific Standard Time":   {"Pacific/Guadalcanal", "+11", "+11"},
	"Hawaiian Standard Time":          {"Pacific/Honolulu", "HST", "HST"},
	"Line Islands Standard Time":      {"Pacific/Kiritimati", "+14", "+14"},
	"Marquesas Standard Time":         {"Pacific/Marquesas", "-0930", "-0930"},
	"Norfolk Standard Time":           {"Pacific/Norfolk", "+11", "+12"},
	"West Pacific Standard Time":      {"Pacific/Port_Moresby", "+10", "+10"},
	"Tonga Standard Time":             {"Pacific/Tongatapu", "+13", "+13"},
}

// windowsZoneIDToIANA returns the default IANA zone ID for a Windows time zone key name.
func windowsZoneIDToIANA(winID string) (string, bool) {
	meta, ok := windowsZones[winID]
	return meta.iana, ok
}

// localeRegion extracts the ISO 3166-1 / UN M.49 region subtag from a BCP-47 locale tag (such as "en-US" -> "US", "zh-Hans-CN" -> "CN", "es-419" -> "419").
func localeRegion(locale string) string {
	parts := strings.Split(strings.ReplaceAll(locale, "_", "-"), "-")
	for i := 1; i < len(parts); i++ {
		p := parts[i]
		if len(p) == 1 {
			break // extension or private-use subtag
		}
		if len(p) == 2 && isASCIIAlpha(p[0]) && isASCIIAlpha(p[1]) {
			return strings.ToUpper(p)
		}
		if len(p) == 3 && isASCIIDigit(p[0]) && isASCIIDigit(p[1]) && isASCIIDigit(p[2]) {
			return p
		}
	}
	return ""
}

func isASCIIAlpha(b byte) bool { return ('a' <= b && b <= 'z') || ('A' <= b && b <= 'Z') }
func isASCIIDigit(b byte) bool { return '0' <= b && b <= '9' }

// utf16String decodes a NUL-terminated UTF-16 slice.
func utf16String(buf []uint16) string {
	for i, c := range buf {
		if c == 0 {
			buf = buf[:i]
			break
		}
	}
	return string(utf16.Decode(buf))
}

func extractCaps(s string) string {
	var b strings.Builder
	for _, r := range s {
		if 'A' <= r && r <= 'Z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func offsetZoneAbbr(offset int) string {
	sign := "+"
	if offset < 0 {
		sign, offset = "-", -offset
	}
	s := "UTC" + sign + strconv.Itoa(offset/3600)
	if m := (offset % 3600) / 60; m != 0 {
		s += ":" + strconv.Itoa(m)
	}
	return s
}

func windowsZoneAbbrs(keyName, stdName, dstName string, stdOffset, dstOffset int) (std, dst string) {
	if meta, ok := windowsZones[keyName]; ok {
		return meta.std, meta.dst
	}
	if meta, ok := windowsZones[stdName]; ok {
		return meta.std, meta.dst
	}
	std = extractCaps(stdName)
	if std == "" {
		std = offsetZoneAbbr(stdOffset)
	}
	dst = extractCaps(dstName)
	if dst == "" {
		dst = offsetZoneAbbr(dstOffset)
	}
	return std, dst
}

// winPseudoUnix returns the local-time Unix seconds represented by d in year. When d.Year == 0, d specifies a recurring weekday occurrence (d.Day 1..5, where 5 means the last occurrence of d.DayOfWeek in d.Month); otherwise d specifies an absolute calendar date.
func winPseudoUnix(year int, d winSystemTime) int64 {
	if d.Year != 0 {
		return time.Date(int(d.Year), time.Month(d.Month), int(d.Day), int(d.Hour), int(d.Minute), int(d.Second), 0, time.UTC).Unix()
	}
	first := time.Date(year, time.Month(d.Month), 1, int(d.Hour), int(d.Minute), int(d.Second), 0, time.UTC)
	diff := int(d.DayOfWeek) - int(first.Weekday())
	if diff < 0 {
		diff += 7
	}
	day := 1 + diff
	if week := int(d.Day) - 1; week < 4 {
		day += week * 7
	} else {
		day += 4 * 7
		if day > daysInMonth(year, time.Month(d.Month)) {
			day -= 7
		}
	}
	return first.Unix() + int64(day-1)*86400
}

func daysInMonth(year int, month time.Month) int {
	return time.Date(year, month+1, 0, 0, 0, 0, 0, time.UTC).Day()
}

// windowsTZif encodes info's offsets and daylight saving transitions as a TZif v2 payload that time.LoadLocationFromTZData parses without needing embedded tzdata.
func windowsTZif(info *winDynamicTimeZoneInfo, stdAbbr, dstAbbr string, baseYear int) []byte {
	type tx struct {
		when  int64
		index byte
	}
	type zoneType struct {
		offset int
		isDST  byte
		abbr   string
	}

	hasDST := info.DynamicDaylightTimeDisabled == 0 && info.StandardDate.Month > 0 && info.DaylightDate.Month > 0
	var types []zoneType
	var transitions []tx

	if !hasDST {
		// StandardBias is ignored when StandardDate is unset, matching Windows and Go's initLocalFromTZI.
		types = []zoneType{{offset: -int(info.Bias) * 60, isDST: 0, abbr: stdAbbr}}
	} else {
		stdOffset := -int(info.Bias+info.StandardBias) * 60
		dstOffset := -int(info.Bias+info.DaylightBias) * 60
		types = []zoneType{
			{offset: stdOffset, isDST: 0, abbr: stdAbbr},
			{offset: dstOffset, isDST: 1, abbr: dstAbbr},
		}
		if info.StandardDate.Year != 0 && info.DaylightDate.Year != 0 {
			toStd := tx{when: winPseudoUnix(int(info.StandardDate.Year), info.StandardDate) - int64(dstOffset), index: 0}
			toDst := tx{when: winPseudoUnix(int(info.DaylightDate.Year), info.DaylightDate) - int64(stdOffset), index: 1}
			if toDst.when < toStd.when {
				transitions = []tx{toDst, toStd}
			} else {
				transitions = []tx{toStd, toDst}
			}
		} else {
			d0, d1 := info.StandardDate, info.DaylightDate
			i0, i1 := byte(0), byte(1)
			if d0.Month > d1.Month {
				d0, d1 = d1, d0
				i0, i1 = i1, i0
			}
			transitions = make([]tx, 0, 400)
			for y := baseYear - 100; y < baseYear+100; y++ {
				transitions = append(transitions,
					tx{when: winPseudoUnix(y, d0) - int64(types[i1].offset), index: i0},
					tx{when: winPseudoUnix(y, d1) - int64(types[i0].offset), index: i1},
				)
			}
		}
	}

	var infos []byte
	var names []byte
	for _, z := range types {
		infos = binary.BigEndian.AppendUint32(infos, uint32(int32(z.offset)))
		infos = append(infos, z.isDST, byte(len(names)))
		names = append(names, z.abbr...)
		names = append(names, 0)
	}

	header := func(version byte, counts ...int) []byte {
		h := append([]byte("TZif"), version)
		h = append(h, make([]byte, 15)...)
		for _, n := range counts {
			h = binary.BigEndian.AppendUint32(h, uint32(n))
		}
		return h
	}

	b := header('2', 0, 0, 0, 0, 1, 1)
	b = append(b, 0, 0, 0, 0, 0, 0, 0)
	b = append(b, header('2', 0, 0, 0, len(transitions), len(types), len(names))...)
	for _, tr := range transitions {
		b = binary.BigEndian.AppendUint64(b, uint64(tr.when))
	}
	for _, tr := range transitions {
		b = append(b, tr.index)
	}
	b = append(b, infos...)
	b = append(b, names...)
	return append(b, '\n', '\n')
}

// windowsLocation builds a *time.Location from a Windows DYNAMIC_TIME_ZONE_INFORMATION. When DST is enabled and an IANA ID is known, it first tries loadLocation (which succeeds when tzdata is available), otherwise it synthesizes TZif rules directly from info, and finally falls back to a fixed zone of the zone's standard bias.
func windowsLocation(info *winDynamicTimeZoneInfo, iana string, loadLocation func(string) (*time.Location, error), baseYear int) *time.Location {
	keyName := utf16String(info.TimeZoneKeyName[:])
	stdName := utf16String(info.StandardName[:])
	dstName := utf16String(info.DaylightName[:])

	if iana == "" {
		if id, ok := windowsZoneIDToIANA(keyName); ok {
			iana = id
		} else if id, ok := windowsZoneIDToIANA(stdName); ok {
			iana = id
		}
	}

	if info.DynamicDaylightTimeDisabled == 0 && iana != "" && loadLocation != nil {
		if loc, err := loadLocation(iana); err == nil {
			return loc
		}
	}

	name := "Local"
	if info.DynamicDaylightTimeDisabled == 0 && iana != "" {
		name = iana
	} else if keyName != "" {
		name = keyName
	}

	stdOffset := -int(info.Bias) * 60
	dstOffset := stdOffset
	if info.DynamicDaylightTimeDisabled == 0 && info.StandardDate.Month > 0 && info.DaylightDate.Month > 0 {
		stdOffset = -int(info.Bias+info.StandardBias) * 60
		dstOffset = -int(info.Bias+info.DaylightBias) * 60
	}
	stdAbbr, dstAbbr := windowsZoneAbbrs(keyName, stdName, dstName, stdOffset, dstOffset)
	if loc, err := time.LoadLocationFromTZData(name, windowsTZif(info, stdAbbr, dstAbbr, baseYear)); err == nil {
		return loc
	}
	return time.FixedZone(name, stdOffset)
}

type windowsZoneTracker struct {
	mu           sync.Mutex
	seeded       bool
	lastInfo     winDynamicTimeZoneInfo
	lastIANA     string
	read         func() (winDynamicTimeZoneInfo, bool)
	resolveIANA  func(winID string) string
	loadLocation func(string) (*time.Location, error)
	apply        func(*time.Location)
	nowYear      func() int
}

func (t *windowsZoneTracker) current() (winDynamicTimeZoneInfo, string, bool) {
	info, ok := t.read()
	if !ok {
		return winDynamicTimeZoneInfo{}, "", false
	}
	var iana string
	if t.resolveIANA != nil {
		keyName := utf16String(info.TimeZoneKeyName[:])
		if keyName == "" {
			keyName = utf16String(info.StandardName[:])
		}
		if keyName != "" {
			iana = t.resolveIANA(keyName)
		}
	}
	return info, iana, true
}

// seed records the system's initial zone without replacing Go's startup time.Local.
func (t *windowsZoneTracker) seed() {
	info, iana, ok := t.current()
	if !ok {
		return
	}
	t.mu.Lock()
	if !t.seeded {
		t.lastInfo, t.lastIANA, t.seeded = info, iana, true
	}
	t.mu.Unlock()
}

// refresh checks Windows' current zone and updates time.Local only when it changed.
func (t *windowsZoneTracker) refresh() bool {
	info, iana, ok := t.current()
	if !ok {
		return false
	}
	t.mu.Lock()
	if t.seeded && info == t.lastInfo && iana == t.lastIANA {
		t.mu.Unlock()
		return false
	}
	t.lastInfo, t.lastIANA, t.seeded = info, iana, true
	t.mu.Unlock()

	year := 2026
	if t.nowYear != nil {
		year = t.nowYear()
	}
	loc := windowsLocation(&info, iana, t.loadLocation, year)
	t.apply(loc)
	return true
}
