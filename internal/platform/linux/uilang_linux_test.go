//go:build linux && !android

package linux

import (
	"sync/atomic"
	"testing"
)

func TestSetForcedLanguageUpdatesProcessGettextState(t *testing.T) {
	initUILang()
	if gGetenv == nil || gSetenv == nil || gUnsetenv == nil {
		t.Skip("GLib env functions unavailable")
	}
	defer clearForcedLanguage()

	readEnv := func() (string, bool) {
		p := gGetenv(cstr("LANGUAGE"))
		if p == 0 {
			return "", false
		}
		return goString(p), true
	}
	origVal, origHad := readEnv()
	var cntBefore int32
	if nlMsgCatCntr != 0 {
		cntBefore = atomic.LoadInt32((*int32)(cPointer(nlMsgCatCntr)))
	}

	setForcedLanguage("zh-Hans")
	if got, ok := readEnv(); !ok || got != "zh_CN:zh" {
		t.Fatalf("after forcing zh-Hans, C LANGUAGE = (%q, %v), want (zh_CN:zh, true)", got, ok)
	}
	if nlMsgCatCntr != 0 {
		if cntAfter := atomic.LoadInt32((*int32)(cPointer(nlMsgCatCntr))); cntAfter <= cntBefore {
			t.Fatalf("_nl_msg_cat_cntr did not increment: before %d, after %d", cntBefore, cntAfter)
		}
	}

	setForcedLanguage("")
	if got, ok := readEnv(); ok != origHad || got != origVal {
		t.Fatalf("after clearing, C LANGUAGE = (%q, %v), want (%q, %v)", got, ok, origVal, origHad)
	}
}
