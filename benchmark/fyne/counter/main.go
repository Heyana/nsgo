// Command counter is the counter case on Fyne: a count and two buttons.
package main

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/nsgo-dev/nsgo/benchmark/internal/bench"
)

func main() {
	a := app.New()
	w := a.NewWindow("Counter")

	n := 0
	count := canvas.NewText("0", color.Black)
	count.TextSize = 56
	count.TextStyle.Bold = true
	count.Alignment = fyne.TextAlignCenter
	set := func(v int) {
		n = v
		count.Text = strconv.Itoa(n)
		count.Refresh()
	}
	dec := widget.NewButton("−1", func() { set(n - 1) })
	inc := widget.NewButton("+1", func() { set(n + 1) })
	inc.Importance = widget.HighImportance
	w.SetContent(container.NewCenter(container.NewVBox(count, container.NewHBox(dec, inc))))
	w.Resize(fyne.NewSize(360, 260))

	// Fyne ticks its animations just before it draws each frame: the second tick comes after the first frame, which is ready.
	ticks := 0
	var ready *fyne.Animation
	ready = fyne.NewAnimation(time.Second, func(float32) {
		if ticks++; ticks == 2 {
			bench.Frame()
			ready.Stop()
		}
	})
	ready.RepeatCount = fyne.AnimationRepeatForever
	a.Lifecycle().SetOnStarted(ready.Start)
	w.ShowAndRun()
}
