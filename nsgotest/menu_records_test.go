package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

func TestMenuRecordsAreIndependentOfDispatch(t *testing.T) {
	if Target() != "" {
		t.Skip("checks the headless menu model")
	}
	var selected int
	app := StartApp(t, func(cx *core.Scope) {
		desktop.SetApplicationMenu(cx, []desktop.MenuItem{{Label: "File", Items: []desktop.MenuItem{
			{Label: "New", Shortcut: "mod+n", Radio: true, Checked: true, OnSelect: func() { selected++ }},
			{Separator: true},
			{Label: "Close", Role: desktop.MenuClose, Disabled: true},
		}}})
		cx.SetMainView(func(*core.Scope) core.Node { return core.View{} })
	}, Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop}))

	if len(app.Menu) != 1 || app.Menu[0].Label != "File" || len(app.Menu[0].Items) != 3 {
		t.Fatalf("recorded menu = %+v", app.Menu)
	}
	items := app.Menu[0].Items
	if item := items[0]; item.Label != "New" || item.Shortcut != "mod+n" || !item.HasHandler ||
		!item.Radio || !item.Checkable || !item.Checked {
		t.Fatalf("recorded New = %+v", item)
	}
	if !items[1].Separator || items[2].Role != desktop.MenuClose || !items[2].Disabled || items[2].HasHandler {
		t.Fatalf("recorded separator and Close = %+v", items[1:])
	}

	items[0].Label, items[0].Disabled = "Changed", true
	app.ChooseMenu("File", "New")
	app.Key("mod+n")
	if selected != 2 {
		t.Fatalf("New ran %d times, want one selection and one shortcut", selected)
	}
}
