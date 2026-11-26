// Command list is the list case on Fyne: 10,000 rows, scrolled a row further every frame Fyne draws.
package main

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/nsgo-dev/nsgo/benchmark/internal/bench"
)

var tones = [4]color.Color{
	color.NRGBA{0x00, 0x7a, 0xff, 0xff},
	color.NRGBA{0x34, 0xc7, 0x59, 0xff},
	color.NRGBA{0xff, 0x95, 0x00, 0xff},
	color.NRGBA{0xaf, 0x52, 0xde, 0xff},
}

var muted = color.NRGBA{0x6b, 0x72, 0x80, 0xff}

func main() {
	a := app.New()
	w := a.NewWindow("List")

	list := widget.NewList(
		func() int { return bench.ListRows },
		// A row: a dot, its name and a number, at least the case's row height.
		func() fyne.CanvasObject {
			dot := canvas.NewCircle(tones[0])
			name := canvas.NewText("", color.Black)
			name.TextSize = 14
			value := canvas.NewText("", muted)
			value.TextSize = 13
			height := canvas.NewRectangle(color.Transparent)
			height.SetMinSize(fyne.NewSize(0, bench.ListRowH))
			row := container.NewHBox(container.NewGridWrap(fyne.NewSize(10, 10), dot), name, layout.NewSpacer(), value)
			return container.NewStack(height, container.NewPadded(container.NewVBox(layout.NewSpacer(), row, layout.NewSpacer())))
		},
		func(i widget.ListItemID, o fyne.CanvasObject) {
			row := o.(*fyne.Container).Objects[1].(*fyne.Container).Objects[0].(*fyne.Container).Objects[1].(*fyne.Container)
			row.Objects[0].(*fyne.Container).Objects[0].(*canvas.Circle).FillColor = tones[i%len(tones)]
			row.Objects[1].(*canvas.Text).Text = "Row " + strconv.Itoa(i)
			row.Objects[3].(*canvas.Text).Text = strconv.Itoa(i * 7 % 1000)
			row.Refresh()
		},
	)
	w.SetContent(list)
	w.Resize(fyne.NewSize(480, 800))

	// Fyne ticks its animations just before it draws each frame, so the tick is the frame's work: the list scrolls a row further, as the others' do, bringing the row after the last shown into view. The first tick comes before the first frame, which is ready.
	ticks, top := 0, 0
	visible := int(800 / bench.ListRowH)
	frame := fyne.NewAnimation(time.Second, func(float32) {
		if ticks > 0 {
			bench.Frame()
		}
		ticks++
		top = (top + 1) % (bench.ListRows - visible)
		if top == 0 {
			list.ScrollToTop()
		} else {
			list.ScrollTo(top + visible)
		}
	})
	frame.RepeatCount = fyne.AnimationRepeatForever
	a.Lifecycle().SetOnStarted(frame.Start)
	w.ShowAndRun()
}
