package menutext

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestLabel(t *testing.T) {
	for _, c := range []struct {
		lang, label, app, want string
		role                   driver.MenuRole
	}{
		{lang: "zh-Hans", role: driver.MenuPaste, want: "粘贴"},
		{lang: "zh-Hans", role: driver.MenuPaste, label: "Paste", want: "粘贴"},
		{lang: "zh-Hans", role: driver.MenuQuit, app: "Neorio", want: "退出 Neorio"},
		{lang: "en", role: driver.MenuAbout, app: "Neorio", want: "About Neorio"},
		{lang: "en", role: driver.MenuAbout, want: "About"},
		{lang: "de", role: driver.MenuClose, want: "Close"},
		{lang: "zh-Hans", role: driver.MenuPaste, label: "Paste Text", want: "Paste Text"},
		{lang: "zh-Hans", label: "Paste", want: "Paste"},
		{lang: "zh-Hans", role: driver.MenuPasteAndMatchStyle, want: "粘贴并匹配样式"},
		{lang: "zh-Hans", role: driver.MenuDelete, want: "删除"},
		{lang: "zh-Hans", role: driver.MenuStartSpeaking, want: "开始讲话"},
		{lang: "zh-Hans", role: driver.MenuStopSpeaking, want: "停止讲话"},
		{lang: "zh-Hans", role: driver.MenuBringAllToFront, want: "前置全部窗口"},
		{lang: "zh-Hans", role: driver.MenuServices, want: "服务"},
		{lang: "zh-Hans", role: driver.MenuWindowMenu, want: "窗口"},
		{lang: "zh-Hans", role: driver.MenuHelpMenu, want: "帮助"},
		{lang: "zh-Hans", role: driver.MenuHelpMenu, label: "Hilfe", want: "Hilfe"},
	} {
		e := driver.MenuEntry{Label: c.label, Role: c.role}
		if got := Label(c.lang, &e, c.app); got != c.want {
			t.Errorf("Label(%q, %+v, %q) = %q, want %q", c.lang, e, c.app, got, c.want)
		}
	}
}

// TestEveryRoleIsSaid holds each role's catalog label in the source language to the English one core fills in, which Label recognizes as no label of the app's own.
func TestEveryRoleIsSaid(t *testing.T) {
	for r := driver.MenuAbout; r.Label() != ""; r++ {
		e := driver.MenuEntry{Role: r}
		if got := Label("en", &e, ""); got != r.Label() {
			t.Errorf("Label(en, %v) = %q, want %q", r, got, r.Label())
		}
		e.Label = r.Label()
		if got := Label("zh-Hans", &e, ""); got == "" || got == r.Label() {
			t.Errorf("Label(zh-Hans, %v) = %q, want a translation", r, got)
		}
	}
}

func TestMnemonic(t *testing.T) {
	for _, test := range []struct {
		label string
		want  rune
	}{
		{"File", 'f'},
		{"Édit", 'é'},
		{"Файл", 'ф'},
		// NSGo's labels mark no access key: '&' shows as itself.
		{"E&xit", 'e'},
		{"&Help", 'h'},
		{"(Beta) Tools", 'b'},
		{"123", '1'},
		{"---", 0},
	} {
		if got := Mnemonic(test.label); got != test.want {
			t.Errorf("Mnemonic(%q) = %q, want %q", test.label, got, test.want)
		}
	}
}
