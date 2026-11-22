//go:build windows

package windows

import (
	"runtime"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

// Go reads the local zone from GetTimeZoneInformation once on Windows, at the
// first use of time.Local. refreshTimeZone re-reads GetDynamicTimeZoneInformation
// whenever WM_TIMECHANGE or an "intl" WM_SETTINGCHANGE arrives, maps the Windows
// zone ID to IANA through icu.dll (falling back to the CLDR table and TZif rules
// synthesized from DYNAMIC_TIME_ZONE_INFORMATION), and updates time.Local when
// the zone changed. Times read after a change are in the new zone; the app is
// not rendered again for it.

const wmTimeChange = 0x001E

var (
	getDynamicTimeZoneInformation = kernel32.NewProc("GetDynamicTimeZoneInformation")

	icuDLL                        = syscall.NewLazyDLL("icu.dll")
	ucalGetTimeZoneIDForWindowsID = icuDLL.NewProc("ucal_getTimeZoneIDForWindowsID")
)

var winTimeZone = windowsZoneTracker{
	read:         readWindowsTimeZone,
	resolveIANA:  windowsIDToIANA,
	loadLocation: time.LoadLocation,
	apply: func(loc *time.Location) {
		// Go has no other way to change the zone: the store races with readers of time.Local, but it is one pointer, which none sees torn.
		time.Local = loc
	},
	nowYear: func() int { return time.Now().UTC().Year() },
}

func seedTimeZone()    { winTimeZone.seed() }
func refreshTimeZone() { winTimeZone.refresh() }

func readWindowsTimeZone() (winDynamicTimeZoneInfo, bool) {
	var info winDynamicTimeZoneInfo
	const timeZoneIDInvalid = 0xFFFFFFFF
	r, _, _ := getDynamicTimeZoneInformation.Call(uintptr(unsafe.Pointer(&info)))
	if uint32(r) == timeZoneIDInvalid {
		return winDynamicTimeZoneInfo{}, false
	}
	return info, true
}

func windowsIDToIANA(winID string) string {
	// icu.dll exports C (cdecl) functions; syscall.LazyProc.Call uses stdcall on 386,
	// so skip icu.dll on 386 and fall back to the built-in CLDR table.
	if runtime.GOARCH != "386" && icuDLL.Load() == nil && ucalGetTimeZoneIDForWindowsID.Find() == nil {
		if region := localeRegion(userLocale()); region != "" {
			if id := icuWindowsIDToIANA(winID, region); id != "" {
				return id
			}
		}
		if id := icuWindowsIDToIANA(winID, ""); id != "" {
			return id
		}
	}
	if id, ok := windowsZoneIDToIANA(winID); ok {
		return id
	}
	return ""
}

func icuWindowsIDToIANA(winID, region string) string {
	in := utf16.Encode([]rune(winID))
	if len(in) == 0 {
		return ""
	}
	var regionPtr uintptr
	var regionBuf []byte
	if region != "" {
		regionBuf = append([]byte(region), 0)
		regionPtr = uintptr(unsafe.Pointer(&regionBuf[0]))
	}
	var out [128]uint16
	var status int32
	r, _, _ := ucalGetTimeZoneIDForWindowsID.Call(
		uintptr(unsafe.Pointer(&in[0])),
		uintptr(len(in)),
		regionPtr,
		uintptr(unsafe.Pointer(&out[0])),
		uintptr(len(out)),
		uintptr(unsafe.Pointer(&status)),
	)
	runtime.KeepAlive(in)
	runtime.KeepAlive(regionBuf)
	n := int32(r)
	if status > 0 || n <= 0 || int(n) > len(out) {
		return ""
	}
	return syscall.UTF16ToString(out[:n])
}

func isTimeZoneSettingChange(lParam uintptr) bool {
	if lParam == 0 {
		return true
	}
	n, _, _ := lstrlenW.Call(lParam)
	if n == 0 || n > 64 {
		return false
	}
	p := *(**uint16)(unsafe.Pointer(&lParam))
	return strings.EqualFold(syscall.UTF16ToString(unsafe.Slice(p, int(n))), "intl")
}
