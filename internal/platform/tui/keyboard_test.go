//go:build (darwin && !ios) || (linux && !android) || windows

package tui

import (
	"testing"

	"github.com/gdamore/tcell/v3"
)

func TestTerminalKeysIncludeLegacyControlCodes(t *testing.T) {
	for _, tc := range []struct {
		event      *tcell.EventKey
		name, text string
	}{
		{tcell.NewEventKey(tcell.KeyCtrlK, "", tcell.ModCtrl), "k", ""},
		{tcell.NewEventKey(tcell.KeyRune, "k", tcell.ModCtrl), "k", ""},
		{tcell.NewEventKeyEx(tcell.KeyRune, "k", tcell.ModCtrl, true, 0, 1), "k", "k"},
		{tcell.NewEventKey(tcell.KeyEnter, "", 0), "enter", ""},
		{tcell.NewEventKey(tcell.KeyBacktab, "", 0), "tab", ""},
		{tcell.NewEventKey(tcell.KeyRune, " ", 0), "space", " "},
		{tcell.NewEventKey(tcell.KeyRune, "界", 0), "界", "界"},
		{tcell.NewEventKey(tcell.KeyBackspace2, "", 0), "backspace", ""},
	} {
		if name, text := keyOf(tc.event); name != tc.name || text != tc.text {
			t.Errorf("%s decoded as %q %q, want %q %q", tc.event.Name(), name, text, tc.name, tc.text)
		}
	}
}
