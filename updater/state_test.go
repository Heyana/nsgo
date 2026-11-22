//go:build !nsgo_steam

package updater_test

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/updater"
)

func TestUpdaterPreferenceTracksAndStopsDailyChecks(t *testing.T) {
	var seen updater.State
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { seen = updater.Status(cx) })
		return core.View{Children: []core.Node{
			base.Button{OnPress: func(core.PressEvent) { updater.SetAutomaticChecks(cx, false) }, Children: []core.Node{core.Text{Content: "off"}}},
			base.Button{OnPress: func(core.PressEvent) { updater.SetAutomaticChecks(cx, true) }, Children: []core.Node{core.Text{Content: "on"}}},
			base.Button{OnPress: func(core.PressEvent) { updater.CheckDaily(cx) }, Children: []core.Node{core.Text{Content: "daily"}}},
		}}
	}, nsgotest.UpdaterState(core.UpdaterState{Managed: core.UpdaterSelf, AutomaticChecks: true}))
	if !seen.AutomaticChecks || !seen.LastCheck.IsZero() {
		t.Fatalf("initial=%+v", seen)
	}
	app.Press("text:off")
	if seen.AutomaticChecks {
		t.Fatal("preference did not redraw")
	}
	app.Press("text:daily")
	if len(app.UpdateChecks) != 0 {
		t.Fatal("disabled daily check reached backend")
	}
	app.Press("text:on")
	app.Update = func(bool) core.UpdateCheck {
		return core.UpdateCheck{Managed: core.UpdaterSelf, Version: "2", Notes: "changes"}
	}
	app.Press("text:daily")
	if !seen.AutomaticChecks || seen.Available != "2" || seen.Notes != "changes" || !seen.LastCheck.Equal(app.Clock.WallTime().Truncate(time.Second)) {
		t.Fatalf("updated=%+v", seen)
	}
}

func TestUpdaterPreferencesSurviveRestart(t *testing.T) {
	var got updater.State
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node { got = updater.Status(cx); return core.Text{Content: "ready"} },
		nsgotest.UpdaterState(core.UpdaterState{Managed: core.UpdaterPage, AutomaticChecks: true}),
		nsgotest.Storage(map[string]string{"\x00nsgo.updater\x00automatic": "false", "\x00nsgo.updater\x00checked": "12345"}))
	app.Find("text:ready")
	if got.AutomaticChecks || got.LastCheck.Unix() != 12345 {
		t.Fatalf("restored=%+v", got)
	}
}

func TestUpdaterDisabledChecksExplainTheReason(t *testing.T) {
	for _, test := range []struct {
		managed     updater.Managed
		store, want string
	}{
		{updater.ManagedDevelopment, "", "development build"}, {updater.ManagedChannel, "", "distribution channel"}, {updater.ManagedStore, "Flatpak", "Flatpak"},
	} {
		t.Run(fmt.Sprint(test.managed), func(t *testing.T) {
			app := start(t, core.UpdateCheck{Managed: test.managed, Store: test.store})
			app.Press("text:Check")
			alerts := app.Alerts()
			if len(alerts) != 1 || !strings.Contains(alerts[0].Message, test.want) || strings.Contains(alerts[0].Message, "Try again later") {
				t.Fatalf("alerts=%+v", alerts)
			}
		})
	}
}

func TestUpdaterNativePreferencesUseTheBackend(t *testing.T) {
	var got updater.State
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() { got = updater.Status(cx) })
		return base.Button{OnPress: func(core.PressEvent) { updater.SetAutomaticChecks(cx, false); updater.SetAutomaticDownloads(cx, true) }, Children: []core.Node{core.Text{Content: "set"}}}
	}, nsgotest.UpdaterState(core.UpdaterState{Managed: core.UpdaterSelf, NativePreferences: true, AutomaticChecks: true}), nsgotest.Environment(core.Environment{OS: core.OSMacOS}))
	app.Press("text:set")
	if got.AutomaticChecks || !got.AutomaticDownloads || len(app.UpdateChecks) != 0 {
		t.Fatalf("state=%+v, checks=%v", got, app.UpdateChecks)
	}
}

func TestUpdaterPreferenceWritesSurviveNewGraphs(t *testing.T) {
	store := core.NewStore(nil)
	initial := core.UpdaterState{Managed: core.UpdaterPage, AutomaticChecks: true}
	first := core.NewRoot(core.RootOptions{Storage: store, Updater: initial})
	first.Run(func() { updater.SetAutomaticChecks(first, false); first.Storage().Clear() })
	first.Dispose()
	second := core.NewRoot(core.RootOptions{Storage: store, Updater: initial})
	defer second.Dispose()
	second.Run(func() {
		if updater.Status(second).AutomaticChecks {
			t.Fatal("private preference was cleared or lost at restart")
		}
	})
}
