// Command quotes is the quotes case on Fyne: a table of 40 quotes, ten of which change every frame Fyne draws.
package main

import (
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"github.com/nsgo-dev/nsgo/benchmark/internal/bench"
)

var widths = [5]float32{80, 90, 80, 80, 100}

var (
	ink   = color.NRGBA{0x11, 0x11, 0x11, 0xff}
	muted = color.NRGBA{0x6b, 0x72, 0x80, 0xff}
	up    = color.NRGBA{0x1a, 0x7f, 0x37, 0xff}
	down  = color.NRGBA{0xcf, 0x22, 0x2e, 0xff}
	zebra = color.NRGBA{0xf6, 0xf7, 0xf9, 0xff}
	white = color.NRGBA{0xff, 0xff, 0xff, 0xff}
)

func tone(v float64) color.Color {
	if v < 0 {
		return down
	}
	return up
}

// columns lays a row's cells out at the case's fixed widths, 12 apart.
type columns struct{}

func (columns) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	x := float32(0)
	for i, o := range objs {
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(widths[i], size.Height))
		x += widths[i] + 12
	}
}

func (columns) MinSize([]fyne.CanvasObject) fyne.Size {
	w := float32(12 * (len(widths) - 1))
	for _, v := range widths {
		w += v
	}
	return fyne.NewSize(w, bench.QuoteRowH)
}

func text(s string, c color.Color, size float32, right bool) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	if right {
		t.Alignment = fyne.TextAlignTrailing
	}
	return t
}

func main() {
	a := app.New()
	w := a.NewWindow("Quotes")
	market := bench.NewQuotes()

	title := text("Streaming quotes · tick 0", ink, 18, false)
	title.TextStyle.Bold = true
	head := container.New(columns{})
	for i, label := range []string{"Symbol", "Price", "Change", "%", "Volume"} {
		head.Add(text(label, muted, 12, i > 0))
	}
	rows := container.NewVBox()
	cells := make([][5]*canvas.Text, len(market.Rows))
	for i, q := range market.Rows {
		bg := white
		if i%2 == 1 {
			bg = zebra
		}
		cells[i] = [5]*canvas.Text{text(q.Symbol, ink, 13, false), text("", ink, 13, true), text("", ink, 13, true), text("", ink, 13, true), text("", muted, 13, true)}
		cells[i][0].TextStyle.Bold = true
		row := container.New(columns{})
		for _, c := range cells[i] {
			row.Add(c)
		}
		back := canvas.NewRectangle(bg)
		back.CornerRadius = 4
		rows.Add(container.NewStack(back, row))
	}
	show := func(i int) {
		q := market.Rows[i]
		c := cells[i]
		c[1].Text = strconv.FormatFloat(q.Price, 'f', 2, 64)
		c[2].Text, c[2].Color = strconv.FormatFloat(q.Change(), 'f', 2, 64), tone(q.Change())
		c[3].Text, c[3].Color = strconv.FormatFloat(q.Percent(), 'f', 2, 64)+"%", tone(q.Change())
		c[4].Text = strconv.Itoa(q.Volume)
		for _, t := range c[1:] {
			t.Refresh()
		}
	}
	for i := range market.Rows {
		show(i)
	}
	page := canvas.NewRectangle(white)
	w.SetContent(container.NewStack(page, container.NewPadded(container.NewVBox(title, head, rows))))
	w.Resize(fyne.NewSize(560, 860))

	// Fyne ticks its animations just before it draws each frame, so the tick is the frame's work: the market ticks and the rows it changed are set. The first tick comes before the first frame, which is ready.
	ticks := 0
	frame := fyne.NewAnimation(time.Second, func(float32) {
		if ticks > 0 {
			bench.Frame()
		}
		ticks++
		for _, i := range market.Tick() {
			show(i)
		}
		title.Text = "Streaming quotes · tick " + strconv.Itoa(ticks)
		title.Refresh()
	})
	frame.RepeatCount = fyne.AnimationRepeatForever
	a.Lifecycle().SetOnStarted(frame.Start)
	w.ShowAndRun()
}
