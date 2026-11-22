//go:build windows

package windows

import "unsafe"

var (
	setThreadPreferredUILanguages  = kernel32.NewProc("SetThreadPreferredUILanguages")
	setProcessPreferredUILanguages = kernel32.NewProc("SetProcessPreferredUILanguages")
)

const muiLanguageName = 0x00000008

// forcedUILanguage is the canonical BCP 47 tag last applied to the standalone
// app's thread and process preferred UI languages, or "" when unforced.
// UI-thread only.
var forcedUILanguage string

// setForcedUILanguage sets Windows' thread and process preferred UI languages
// so system dialogs (file open/save, MB_GetString, common controls and COM
// shell worker threads) follow Scope.SetLocale. Empty or pseudo tags clear
// any prior override so Windows follows the user's languages again.
func setForcedUILanguage(lang string) {
	cands := windowsUILanguageCandidates(lang)
	if len(cands) == 0 {
		clearForcedUILanguage()
		return
	}
	if cands[0] == forcedUILanguage {
		return
	}
	for _, cand := range cands {
		buf := utf16MultiString(cand)
		var count uint32
		if r, _, _ := setThreadPreferredUILanguages.Call(muiLanguageName, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&count))); r != 0 {
			setProcessPreferredUILanguages.Call(muiLanguageName, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&count)))
			forcedUILanguage = cands[0]
			return
		}
	}
	clearForcedUILanguage()
}

func clearForcedUILanguage() {
	if forcedUILanguage == "" {
		return
	}
	setThreadPreferredUILanguages.Call(0, 0, 0)
	setProcessPreferredUILanguages.Call(0, 0, 0)
	forcedUILanguage = ""
}
