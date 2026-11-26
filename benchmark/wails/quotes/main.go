// Command quotes is the quotes case on Wails: a table of 40 quotes, ten of which change every display frame. The market lives in Go, as in the other Go programs: on each animation frame the page asks Go to tick it and sets the rows that changed.
package main

import (
	"embed"
	"log"

	"github.com/nsgo-dev/nsgo/benchmark/internal/bench"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed index.html
var assets embed.FS

// Row is a quote as the page shows it.
type Row struct {
	I       int     `json:"i"`
	Symbol  string  `json:"symbol"`
	Price   float64 `json:"price"`
	Change  float64 `json:"change"`
	Percent float64 `json:"percent"`
	Volume  int     `json:"volume"`
}

// Quotes is the market the page calls.
type Quotes struct {
	market *bench.Quotes
	ticks  int
}

func (q *Quotes) row(i int) Row {
	r := q.market.Rows[i]
	return Row{I: i, Symbol: r.Symbol, Price: r.Price, Change: r.Change(), Percent: r.Percent(), Volume: r.Volume}
}

// Rows is every row, for the page's first frame.
func (q *Quotes) Rows() []Row {
	rows := make([]Row, len(q.market.Rows))
	for i := range rows {
		rows[i] = q.row(i)
	}
	return rows
}

// Tick counts the frame the page drew, ticks the market and returns the rows it changed. The page calls it first on the frame after its first, which is ready.
func (q *Quotes) Tick() []Row {
	bench.Frame()
	q.ticks++
	touched := q.market.Tick()
	rows := make([]Row, len(touched))
	for k, i := range touched {
		rows[k] = q.row(i)
	}
	return rows
}

func main() {
	app := application.New(application.Options{
		Name:     "Quotes",
		Services: []application.Service{application.NewService(&Quotes{market: bench.NewQuotes()})},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "Quotes", Width: 560, Height: 860, URL: "/"})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
