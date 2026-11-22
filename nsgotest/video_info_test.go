package nsgotest

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

func TestPlayReportsRateCapabilityToTheApp(t *testing.T) {
	var loaded core.VideoInfo
	var starts, loads int
	app := Start(t, func(cx *core.Scope) core.Node {
		canChangeRate := core.NewSignal(cx, false)
		return core.View{Children: []core.Node{
			core.Video{TestID: "video", Source: "test.mp4",
				Style:       core.Style{Width: core.Pt(160), Height: core.Pt(90)},
				OnLoadStart: func() { starts++ },
				OnLoad: func(info core.VideoInfo) {
					loaded = info
					loads++
					canChangeRate.Set(info.RateChangeable)
				}},
			core.Text{ContentFunc: func() string {
				if canChangeRate.Get() {
					return "Speed enabled"
				}
				return "Speed unavailable"
			}},
		}}
	})
	for i, changeable := range []bool{true, false, true} {
		want := core.VideoInfo{Duration: time.Duration(i+1) * time.Second,
			Size: core.Size{Width: core.Points(320 + i), Height: 180}, RateChangeable: changeable}
		app.Play("id:video", want)
		if loaded != want || starts != i+1 || loads != i+1 {
			t.Fatalf("load %d: info = %+v, want %+v; starts/loads = %d/%d", i, loaded, want, starts, loads)
		}
		text := "Speed unavailable"
		if changeable {
			text = "Speed enabled"
		}
		app.Find("text:" + text)
	}
}
