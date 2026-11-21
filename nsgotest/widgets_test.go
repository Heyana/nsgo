package nsgotest_test

import (
	"errors"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/widget"
)

func TestWidgetTimelines(t *testing.T) {
	wall := time.Date(2026, time.November, 21, 10, 0, 0, 0, time.UTC)
	text := &widget.Text{Content: "Continue reading"}
	image := widget.Image{Data: []byte{1, 2, 3}}
	families := map[widget.Family]widget.View{widget.Small: text}
	app := nsgotest.Start(t, func(*core.Scope) core.Node {
		if !widget.Available() {
			t.Error("widget.Available is false")
		}
		err := widget.Update("continue", widget.Entry{
			View: widget.Row{Children: []widget.View{text, image}}, Families: families,
		}, widget.Entry{Date: wall.Add(time.Hour), View: widget.Text{Content: "New chapter"}})
		if err != nil {
			t.Error(err)
		}
		return core.View{}
	}, nsgotest.Widgets(true), nsgotest.WallTime(wall))

	text.Content = "changed"
	image.Data[0] = 0
	delete(families, widget.Small)
	got := app.Widgets()["continue"]
	if len(got) != 2 || !got[0].Date.Equal(wall) || !got[1].Date.Equal(wall.Add(time.Hour)) {
		t.Fatalf("timeline = %+v", got)
	}
	familyText := got[0].Families[widget.Small].(*widget.Text)
	children := got[0].View.(widget.Row).Children
	if familyText.Content != "Continue reading" ||
		children[0].(*widget.Text).Content != "Continue reading" || children[1].(widget.Image).Data[0] != 1 {
		t.Fatalf("published views changed with their source: %+v, %+v", familyText, children)
	}
	familyText.Content = "changed snapshot"
	if text := app.Widgets()["continue"][0].Families[widget.Small].(*widget.Text); text.Content != "Continue reading" {
		t.Fatalf("published view changed with a snapshot: %+v", text)
	}
	if err := widget.Update("continue"); err != nil {
		t.Fatal(err)
	}
	if entries, ok := app.Widgets()["continue"]; !ok || len(entries) != 0 {
		t.Errorf("cleared timeline = %+v, present = %v", entries, ok)
	}
}

func TestLiveActivityRecords(t *testing.T) {
	wall := time.Date(2026, time.November, 21, 10, 0, 0, 0, time.UTC)
	app := nsgotest.Start(t, func(*core.Scope) core.Node { return core.View{} },
		nsgotest.LiveActivities(true), nsgotest.WallTime(wall))
	if !widget.ActivitiesEnabled() {
		t.Fatal("Live Activities are disabled")
	}
	actions := []widget.ActivityAction{{ID: "help", Title: "Help"}}
	started, err := widget.StartActivity("delivery", widget.ActivityContent{Title: "Ordered", Actions: actions}, widget.ActivityOptions{Push: true})
	if err != nil {
		t.Fatal(err)
	}
	actions[0].Title = "changed"
	running := widget.Activities()
	if len(running) != 1 || running[0].ID() != started.ID() || running[0].Kind() != "delivery" {
		t.Fatalf("running activities = %+v", running)
	}
	app.Advance(time.Minute)
	if err := running[0].Update(widget.ActivityContent{Title: "Sent"}); err != nil {
		t.Fatal(err)
	}
	if err := started.Alert(widget.ActivityContent{Title: "Arrived"}, "Delivery", "At the door"); err != nil {
		t.Fatal(err)
	}
	final := &widget.ActivityContent{Title: "Received"}
	if err := started.End(final, widget.DismissNow); err != nil {
		t.Fatal(err)
	}
	final.Title = "changed"
	if got := widget.Activities(); len(got) != 0 {
		t.Fatalf("running activities after end = %+v", got)
	}
	if err := started.Update(widget.ActivityContent{Title: "Sent"}); !errors.Is(err, widget.ErrActivityEnded) {
		t.Errorf("update after end = %v, want ErrActivityEnded", err)
	}

	records := app.LiveActivities()
	if len(records) != 1 || records[0].Activity != started || !records[0].Options.Push {
		t.Fatalf("activity records = %+v", records)
	}
	record := records[0]
	changes := record.Changes
	if len(changes) != 3 || !changes[0].At.Equal(wall) || changes[0].Content.Title != "Ordered" ||
		changes[0].Content.Actions[0].Title != "Help" || changes[1].Content.Title != "Sent" ||
		!changes[2].At.Equal(wall.Add(time.Minute)) || !changes[2].Alert || changes[2].AlertTitle != "Delivery" ||
		changes[2].AlertBody != "At the door" || changes[2].Content.Title != "Arrived" {
		t.Fatalf("activity changes = %+v", changes)
	}
	if record.End == nil || record.End.Final == nil || record.End.Final.Title != "Received" || record.End.Dismissal != widget.DismissNow {
		t.Errorf("activity end = %+v", record.End)
	}
}

func TestUnavailableWidgetServices(t *testing.T) {
	for range 2 {
		t.Run("services disabled", func(t *testing.T) {
			app := nsgotest.Start(t, func(*core.Scope) core.Node { return core.View{} },
				nsgotest.Widgets(false), nsgotest.LiveActivities(false))
			if widget.Available() || widget.ActivitiesEnabled() {
				t.Fatal("widget service reports available")
			}
			if err := widget.Update("counter", widget.Entry{View: widget.Text{Content: "One"}}); !errors.Is(err, widget.ErrNoWidgets) {
				t.Errorf("update = %v, want ErrNoWidgets", err)
			}
			if _, err := widget.StartActivity("counter", widget.ActivityContent{Title: "One"}); !errors.Is(err, widget.ErrActivitiesDisabled) {
				t.Errorf("start = %v, want ErrActivitiesDisabled", err)
			}
			if len(app.Widgets()) != 0 || len(app.LiveActivities()) != 0 {
				t.Error("unavailable services recorded a change")
			}
		})
	}
}
