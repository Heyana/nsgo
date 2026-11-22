//go:build linux && !android

package linux

import (
	"sync"
	"sync/atomic"

	"github.com/ebitengine/purego"
)

// lcMessages is Linux's LC_MESSAGES category constant in <locale.h>.
const lcMessages = 5

var (
	uiLangOnce   sync.Once
	gGetenv      func(*byte) uintptr
	gSetenv      func(*byte, *byte, int32) int32
	gUnsetenv    func(*byte)
	cSetlocale   func(int32, *byte) uintptr
	nlMsgCatCntr uintptr

	// Saved process state before the first forced language is applied.
	// Main-thread only.
	forcedLangSaved   bool
	hadOrigLanguage   bool
	origLanguage      string
	origLCMessages    string
	changedLCMessages bool
	appliedLanguage   string
)

func initUILang() {
	uiLangOnce.Do(func() {
		lib := glibLib
		if lib == 0 {
			for _, name := range []string{"libglib-2.0.so.0", "libglib-2.0.so"} {
				if h, err := purego.Dlopen(name, purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
					lib = h
					break
				}
			}
		}
		if lib != 0 {
			if sym, err := purego.Dlsym(lib, "g_getenv"); err == nil {
				purego.RegisterFunc(&gGetenv, sym)
			}
			if sym, err := purego.Dlsym(lib, "g_setenv"); err == nil {
				purego.RegisterFunc(&gSetenv, sym)
			}
			if sym, err := purego.Dlsym(lib, "g_unsetenv"); err == nil {
				purego.RegisterFunc(&gUnsetenv, sym)
			}
		}
		lookupLibc := func(name string) uintptr {
			if sym, err := purego.Dlsym(purego.RTLD_DEFAULT, name); err == nil {
				return sym
			}
			if libc, err := purego.Dlopen("libc.so.6", purego.RTLD_NOW|purego.RTLD_GLOBAL); err == nil {
				if sym, err := purego.Dlsym(libc, name); err == nil {
					return sym
				}
			}
			return 0
		}
		if sym := lookupLibc("setlocale"); sym != 0 {
			purego.RegisterFunc(&cSetlocale, sym)
		}
		nlMsgCatCntr = lookupLibc("_nl_msg_cat_cntr")
	})
}

func bumpGettextCache() {
	if nlMsgCatCntr != 0 {
		atomic.AddInt32((*int32)(cPointer(nlMsgCatCntr)), 1)
	}
}

// setForcedLanguage updates the C process's LANGUAGE environment variable,
// ensures LC_MESSAGES is not the bare C/POSIX locale (which makes glibc ignore
// LANGUAGE), and bumps GNU gettext's _nl_msg_cat_cntr so GTK's widgets, text
// selection menus and file dialogs follow Scope.SetLocale. Empty or pseudo
// tags restore the process's original LANGUAGE and LC_MESSAGES.
func setForcedLanguage(lang string) {
	gnu := gnuLanguageList(lang)
	if gnu == "" {
		clearForcedLanguage()
		return
	}
	if gnu == appliedLanguage {
		return
	}
	initUILang()
	if gSetenv == nil {
		return
	}
	if !forcedLangSaved {
		if gGetenv != nil {
			if p := gGetenv(cstr("LANGUAGE")); p != 0 {
				hadOrigLanguage = true
				origLanguage = goString(p)
			}
		}
		if cSetlocale != nil {
			if p := cSetlocale(lcMessages, nil); p != 0 {
				origLCMessages = goString(p)
			}
		}
		forcedLangSaved = true
	}
	gSetenv(cstr("LANGUAGE"), cstr(gnu), 1)
	if cSetlocale != nil {
		cur := goString(cSetlocale(lcMessages, nil))
		if isPOSIXLocale(cur) {
			for _, cand := range lcMessagesCandidates(lang) {
				if cSetlocale(lcMessages, cstr(cand)) != 0 {
					changedLCMessages = true
					break
				}
			}
		}
	}
	bumpGettextCache()
	appliedLanguage = gnu
}

func clearForcedLanguage() {
	if appliedLanguage == "" {
		return
	}
	initUILang()
	if hadOrigLanguage {
		if gSetenv != nil {
			gSetenv(cstr("LANGUAGE"), cstr(origLanguage), 1)
		}
	} else if gUnsetenv != nil {
		gUnsetenv(cstr("LANGUAGE"))
	}
	if changedLCMessages && origLCMessages != "" && cSetlocale != nil {
		cSetlocale(lcMessages, cstr(origLCMessages))
		changedLCMessages = false
	}
	bumpGettextCache()
	appliedLanguage = ""
}
