//go:build ios && nsgotest

package ios

import (
	"slices"
	"strconv"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestTitledKeyCommandsUseUIKit(t *testing.T) {
	if onTV() {
		t.Skip("UIKit shortcut discovery is on iOS")
	}
	var locale func()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		count := core.NewSignal(cx, 0)
		cx.SetLocale("en")
		locale = func() { cx.SetLocale("fr") }
		cx.OnKeyWith("mod+1", core.KeyOptions{Title: func() string {
			if cx.Env().Locale() == "fr" {
				return "Ouvrir"
			}
			return "Open"
		}, Group: func() string { return "Document" }}, func() { count.Update(func(n int) int { return n + 1 }) })
		cx.OnKey("mod+2", func() { count.Update(func(n int) int { return n + 10 }) })
		return core.Text{TestID: "shortcut-count", ContentFunc: func() string { return strconv.Itoa(count.Get()) }}
	})

	check := func(title string) {
		t.Helper()
		want := []nsgotest.KeyCommand{{Key: "mod+1", Title: title, Group: "Document"}}
		if got := app.KeyCommands(); !slices.Equal(got, want) {
			t.Errorf("KeyCommands = %v, want %v", got, want)
		}
		var titles []string
		onMainSync(func() {
			apple.ForEach(runner.controller.Send(sel("keyCommands")), func(c objc.ID) {
				if objc.Send[objc.SEL](c, sel("action")) == sel("nsgoKeyCommand:") {
					titles = append(titles, apple.GoString(c.Send(sel("discoverabilityTitle"))))
					if key := apple.GoString(c.Send(sel("input"))); key != "1" || objc.Send[uint](c, sel("modifierFlags")) != 1<<20 {
						t.Errorf("UIKit command input = %q, want Command+1", key)
					}
				}
			})
			menus := runner.keyCommandMenus()
			if len(menus) != 1 {
				t.Errorf("UIKit has %d command groups, want one", len(menus))
				return
			}
			if group := apple.GoString(menus[0].Send(sel("title"))); group != "Document" {
				t.Errorf("UIKit command group = %q, want Document", group)
			}
			var items []string
			apple.ForEach(menus[0].Send(sel("children")), func(c objc.ID) {
				items = append(items, apple.GoString(c.Send(sel("title"))))
			})
			if !slices.Equal(items, []string{title}) {
				t.Errorf("UIKit group commands = %v, want [%s]", items, title)
			}
		})
		if !slices.Equal(titles, []string{title}) {
			t.Errorf("UIKit titles = %v, want [%s]", titles, title)
		}
	}
	check("Open")
	// UIKit drops the modifier state of the first chord a synthesized keyboard sends in a newly launched app, and matches its key commands by that state: a Shift tap first sets the keyboard up.
	app.Key("shift")
	app.Key("mod+1")
	if got := app.Find("id:shortcut-count").Text; got != "1" {
		t.Errorf("Command+1 left the count at %q, want 1", got)
	}
	app.Key("mod+2")
	if got := app.Find("id:shortcut-count").Text; got != "11" {
		t.Errorf("Command+2 left the count at %q, want 11", got)
	}
	app.Post(locale)
	check("Ouvrir")
	onMainSync(func() {
		action := sel("nsgoKeyCommand:")
		apple.ForEach(runner.controller.Send(sel("keyCommands")), func(c objc.ID) {
			if objc.Send[objc.SEL](c, sel("action")) == action {
				runner.controller.Send(action, c)
			}
		})
	})
	app.Settle()
	if got := app.Find("id:shortcut-count").Text; got != "12" {
		t.Errorf("choosing the UIKit command left the count at %q, want 12", got)
	}
}
