// Command quotes benchmarks per-frame text shaping: 40 quote rows, 10 of
// them reticked every 16 ms, four text cells each. The other 30 rows stay
// untouched, so the run shows what re-shaping changed text costs per frame.
//
// It matches the mygo, gogui and nsh ports of the same case item by item:
//   - 40 rows; each tick picks 10 with replacement and changes price and
//     volume (the same as mygo's `for range rows/4`)
//   - columns Symbol 80 / Price 90 / Change 80 / % 80 / Volume 100, row
//     height 22, column gap 12, row gap 2
//   - green for a rise, red for a fall (MyGo's Success/Danger)
//   - a 560x1040 window, injected by -ldflags; the root view stays 560 wide
//
// Unlike nsh, which drives a tick signal from requestAnimationFrame, NSGo
// has no rAF on screen, so time.Ticker(16ms) drives the same signal writes
// on the engine goroutine. That is Go's idiom for it and matches gogui's
// built-in 16 ms ticker: the comparison is what the same moving screen
// costs in CPU.
//
// With NSGO_BENCH_STATS=1 it writes `nsgo 1s: fps=N` to stderr once a
// second, the format mygo's benchstats, gogui's framestat and nsh's
// frameStat write, so one regex reads every port. Otherwise the counter is
// one increment.
package main

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/core"
)

const (
	rows     = 40
	rowH     = 22
	gap      = 12
	rowGap   = 2
	fontSize = 13
)

var colW = [5]float32{80, 90, 80, 80, 100}

var (
	ink     = core.RGB(0x111111)
	muted   = core.RGB(0x6b7280)
	good    = core.RGB(0x1a7f37)
	bad     = core.RGB(0xcf222e)
	surface = core.RGB(0xffffff)
	border  = core.RGB(0xe5e7eb)
)

// pick is green for a rise, red for a fall, as mygo's Success/Danger.
func pick(v float64) core.Color {
	if v < 0 {
		return bad
	}
	return good
}

type quote struct {
	sym    string
	open   float64
	price  float64
	volume int
}

// rowState holds one row's current values. The whole row is one signal, so
// one Set recomputes only its four texts and the other 30 rows do nothing
// in the frame. (The nsh port went through three versions: one large array
// per column, one signal per cell, then one object signal per row.)
type rowState struct {
	price, change, pct float64
	volume             int
}

var (
	data  = mkRows()
	state = make([]*core.Signal[rowState], rows)
)

// mulberry32 is the PRNG and (seed 42) the nsh and mygo ports use.
func mulberry32(a uint32) func() float64 {
	return func() float64 {
		a += 0x6d2b79f5
		t := a
		t = (t ^ (t >> 15)) * (t | 1)
		t ^= t + (t^(t>>7))*(t|61)
		return float64(t^(t>>14)) / 4294967296.0
	}
}

var rnd = mulberry32(42)

func mkRows() []quote {
	out := make([]quote, rows)
	for i := range out {
		var b [4]byte
		for j := range b {
			b[j] = byte('A' + int(rnd()*26))
		}
		px := 20 + rnd()*480
		out[i] = quote{sym: string(b[:]), open: px, price: px, volume: int(rnd() * 1000000)}
	}
	return out
}

// frameStat writes a line of fps to stderr every second for bench/run.sh.
var (
	statsOn      = os.Getenv("NSGO_BENCH_STATS") == "1"
	statN        int
	statT0       time.Time
	ticks        *core.Signal[int]
	tickerPeriod = 16 * time.Millisecond
)

func frameStat() {
	if !statsOn {
		return
	}
	now := time.Now()
	if statT0.IsZero() {
		statT0 = now
	}
	statN++
	if d := now.Sub(statT0); d >= time.Second {
		fmt.Fprintf(os.Stderr, "nsgo 1s: fps=%.1f\n", float64(statN)*1000/float64(d.Milliseconds()))
		statN = 0
		statT0 = now
	}
}

// tick changes a quarter of the rows, with replacement, exactly as mygo's
// `rows/4` draws do.
func tick() {
	touched := make(map[int]struct{}, rows/4)
	for range rows / 4 {
		i := int(rnd() * rows)
		data[i].price *= 1 + (rnd()-0.5)*0.004
		data[i].volume += int(rnd() * 500)
		touched[i] = struct{}{}
	}
	// Write only the touched rows, once each: the other 30 stay still.
	for i := range touched {
		q := data[i]
		state[i].Set(rowState{
			price:  q.price,
			change: q.price - q.open,
			pct:    (q.price - q.open) / q.open * 100,
			volume: q.volume,
		})
	}
}

func headerCell(label string, w float32, align core.TextAlign) core.Node {
	return core.View{
		Style: core.Style{Width: core.Pt(core.Points(w))},
		Children: []core.Node{core.Text{
			Content: label,
			Style:   core.Style{FontSize: 12, Color: muted, TextAlign: align},
		}},
	}
}

// cell is one grid cell: its text reads the row signal and is recomputed
// only when that row changes.
func cell(st *core.Signal[rowState], w float32, align core.TextAlign, color func(rowState) core.Color, text func(rowState) string) core.Node {
	return core.View{
		Style: core.Style{Width: core.Pt(core.Points(w))},
		Children: []core.Node{core.Text{
			StyleFunc: func() core.Style {
				return core.Style{FontSize: fontSize, Color: color(st.Get()), TextAlign: align}
			},
			ContentFunc: func() string { return text(st.Get()) },
		}},
	}
}

func rowNode(st *core.Signal[rowState], sym string, i int) core.Node {
	bg := surface
	if i%2 == 1 {
		bg = core.RGB(0xf6f7f9)
	}
	return core.View{
		Style: core.Style{
			Direction:    core.Row,
			Gap:          gap,
			Height:       core.Pt(rowH),
			AlignItems:   core.AlignCenter,
			Background:   bg,
			CornerRadius: 4,
		},
		Children: []core.Node{
			// The symbol never changes: a static text outside the signal's
			// dependencies.
			core.View{
				Style: core.Style{Width: core.Pt(core.Points(colW[0]))},
				Children: []core.Node{core.Text{
					Content: sym,
					Style:   core.Style{FontSize: fontSize, FontWeight: core.FontWeightSemibold, Color: ink},
				}},
			},
			cell(st, colW[1], core.TextAlignEnd, func(rowState) core.Color { return ink },
				func(s rowState) string { return strconv.FormatFloat(s.price, 'f', 2, 64) }),
			cell(st, colW[2], core.TextAlignEnd, func(s rowState) core.Color { return pick(s.change) },
				func(s rowState) string { return strconv.FormatFloat(s.change, 'f', 2, 64) }),
			cell(st, colW[3], core.TextAlignEnd, func(s rowState) core.Color { return pick(s.pct) },
				func(s rowState) string { return strconv.FormatFloat(s.pct, 'f', 2, 64) + "%" }),
			cell(st, colW[4], core.TextAlignEnd, func(rowState) core.Color { return muted },
				func(s rowState) string { return strconv.Itoa(s.volume) }),
		},
	}
}

// Root builds the quote table and starts the 16 ms ticker.
func Root(cx *core.Scope) core.Node {
	for i := range state {
		q := data[i]
		state[i] = core.NewSignal(cx, rowState{price: q.price, change: q.price - q.open, pct: 0, volume: q.volume})
	}
	ticks = core.NewSignal(cx, 0)

	// A 16 ms ticker stands in for gogui's built-in one. A goroutine off the
	// engine reaches the graph only through Scope.Post.
	go func() {
		for range time.Tick(tickerPeriod) {
			cx.Post(func() {
				frameStat()
				tick()
				ticks.Update(func(n int) int { return n + 1 })
			})
		}
	}()

	kids := make([]core.Node, 0, rows)
	for i := range data {
		kids = append(kids, rowNode(state[i], data[i].sym, i))
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, Background: surface, AlignItems: core.AlignStart},
		Children: []core.Node{
			core.View{
				Style: core.Style{Width: core.Pt(560), Padding: core.All(16), Gap: rowGap},
				Children: []core.Node{
					core.Text{
						ContentFunc: func() string {
							return fmt.Sprintf("Streaming quotes · tick %d", ticks.Get())
						},
						Style: core.Style{FontSize: 18, FontWeight: core.FontWeightSemibold, Color: ink},
					},
					core.View{
						Style: core.Style{
							Direction: core.Row, Gap: gap, Height: core.Pt(24), AlignItems: core.AlignCenter,
							Padding:     core.Symmetric(8, 0),
							BorderWidth: 1, BorderColor: border,
						},
						Children: []core.Node{
							headerCell("Symbol", colW[0], core.TextAlignStart),
							headerCell("Price", colW[1], core.TextAlignEnd),
							headerCell("Change", colW[2], core.TextAlignEnd),
							headerCell("%", colW[3], core.TextAlignEnd),
							headerCell("Volume", colW[4], core.TextAlignEnd),
						},
					},
					core.View{Style: core.Style{Gap: rowGap}, Children: kids},
				},
			},
		},
	}
}

func main() { nsgo.Run(Root) }
