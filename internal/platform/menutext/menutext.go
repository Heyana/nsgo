// Package menutext says the labels of the menu items the backends show.
package menutext

import (
	"github.com/nsgo-dev/nsgo/desktop/locales"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"unicode"
)

// Label returns what e says in lang: its own label, or its role's when
// the app gave none. app, when not "", is named where macOS names it.
func Label(lang string, e *driver.MenuEntry, app string) string {
	if e.Role == driver.MenuRoleNone || e.Label != "" && e.Label != e.Role.Label() {
		return e.Label
	}
	msgs := locales.In(lang)
	switch e.Role {
	case driver.MenuAbout:
		if app != "" {
			return msgs.AboutApp(app)
		}
		return msgs.About()
	case driver.MenuHide:
		if app != "" {
			return msgs.HideApp(app)
		}
		return msgs.Hide()
	case driver.MenuHideOthers:
		return msgs.HideOthers()
	case driver.MenuShowAll:
		return msgs.ShowAll()
	case driver.MenuQuit:
		if app != "" {
			return msgs.QuitApp(app)
		}
		return msgs.Quit()
	case driver.MenuUndo:
		return msgs.Undo()
	case driver.MenuRedo:
		return msgs.Redo()
	case driver.MenuCut:
		return msgs.Cut()
	case driver.MenuCopy:
		return msgs.Copy()
	case driver.MenuPaste:
		return msgs.Paste()
	case driver.MenuSelectAll:
		return msgs.SelectAll()
	case driver.MenuMinimize:
		return msgs.Minimize()
	case driver.MenuZoom:
		return msgs.Zoom()
	case driver.MenuToggleFullScreen:
		return msgs.ToggleFullScreen()
	case driver.MenuClose:
		return msgs.Close()
	case driver.MenuPasteAndMatchStyle:
		return msgs.PasteAndMatchStyle()
	case driver.MenuDelete:
		return msgs.Delete()
	case driver.MenuStartSpeaking:
		return msgs.StartSpeaking()
	case driver.MenuStopSpeaking:
		return msgs.StopSpeaking()
	case driver.MenuBringAllToFront:
		return msgs.BringAllToFront()
	case driver.MenuServices:
		return msgs.Services()
	case driver.MenuWindowMenu:
		return msgs.Window()
	case driver.MenuHelpMenu:
		return msgs.Help()
	}
	return e.Label
}

// Mnemonic returns the first letter or digit in label, lowercased, or zero
// when it has neither. Labels have no access-key marker: '&' shows as itself.
func Mnemonic(label string) rune {
	for _, r := range label {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
	}
	return 0
}
