package nsgotest

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestReportedEventOwnsItsCollections(t *testing.T) {
	payload := &struct{ value string }{"native"}
	e := Event{
		Navigation: &core.WebNavigation{URL: "https://example.test/first", Title: "First"},
		URIs:       []string{"file:///first"},
		Ranges:     []SelectedRange{{Start: 1, End: 3}},
		Touches:    []core.Touch{{ID: 7, Changed: true}},
		TouchGeometry: &TouchGeometry{
			Targets:   map[int]core.NodeID{7: 10},
			Positions: map[core.NodeID][]core.Point{10: {{X: 11, Y: 12}}, 20: {}},
		},
		Data: payload,
	}
	got := e.driverEvent()
	e.Navigation.URL = "https://example.test/later"
	e.URIs[0] = "file:///later"
	e.Ranges[0].Start = 9
	e.Touches[0].ID = 9
	e.TouchGeometry.Targets[7] = 20
	e.TouchGeometry.Positions[10][0].X = 90
	delete(e.TouchGeometry.Positions, 20)
	if got.Navigation.URL != "https://example.test/first" || got.URIs[0] != "file:///first" || got.Ranges[0].Start != 1 || got.Touches[0].ID != 7 {
		t.Fatalf("reported collections changed with the caller: %+v", got)
	}
	if got.TouchGeometry.Targets[7] != 10 || got.TouchGeometry.Positions[10][0] != (core.Point{X: 11, Y: 12}) {
		t.Fatalf("reported touch snapshot changed: %+v", got.TouchGeometry)
	}
	if points, ok := got.TouchGeometry.Positions[20]; !ok || points == nil || len(points) != 0 {
		t.Fatal("an explicitly empty position list was lost")
	}
	if got.Data != payload {
		t.Fatal("native payload identity changed")
	}
	zero := (Event{}).driverEvent()
	if zero.Navigation != nil || zero.URIs != nil || zero.Ranges != nil || zero.Touches != nil || zero.TouchGeometry != nil {
		t.Fatal("zero report allocated absent data")
	}
}

func TestReportUsesTheSuppliedTouchSnapshot(t *testing.T) {
	var got core.TouchEvent
	app := Start(t, func(*core.Scope) core.Node {
		return core.View{TestID: "touch", Style: core.Style{Width: core.Pt(40), Height: core.Pt(40)},
			OnTouch: func(e core.TouchEvent) { got = e }}
	})
	id := app.Find("touch").ID
	at := core.Point{X: 200, Y: 300}
	local := core.Point{X: 11, Y: 12}
	app.Report(Event{Kind: EventTouch, Phase: core.GestureBegan, Time: time.Second,
		Touches:       []core.Touch{{ID: 7, PagePosition: at, Changed: true}},
		TouchGeometry: &TouchGeometry{Targets: map[int]core.NodeID{7: id}, Positions: map[core.NodeID][]core.Point{id: {local}}},
	})
	if got.Phase != core.TouchStart || got.Time != time.Second || len(got.Touches) != 1 || got.Touches[0].Position != local || got.Touches[0].PagePosition != at {
		t.Fatalf("reported touch = %+v", got)
	}
}
