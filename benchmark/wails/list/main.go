// Command list is the list case on Wails: 10,000 rows, scrolled a row further every display frame. The page is a virtual list that renders the rows in view; scrolling is the page's own work, so it reports its frames to Go once a second rather than on each one.
package main

import (
	"embed"
	"fmt"
	"log"
	"strconv"
	"sync"

	"github.com/nsgo-dev/nsgo/benchmark/internal/bench"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed index.html
var assets embed.FS

// Bench is what the page calls.
type Bench struct{ once sync.Once }

// Size is the list's rows and their height.
func (*Bench) Size() [2]int { return [2]int{bench.ListRows, bench.ListRowH} }

// Ready prints "bench ready", once the page has drawn its first frame.
func (b *Bench) Ready() { b.once.Do(bench.Frame) }

// Frames prints the frames the page drew over the last ms milliseconds, per second, as bench.Frame does for the other programs.
func (*Bench) Frames(n int, ms float64) {
	fmt.Println("bench frames " + strconv.FormatFloat(float64(n)/(ms/1000), 'f', 1, 64))
}

func main() {
	app := application.New(application.Options{
		Name:     "List",
		Services: []application.Service{application.NewService(&Bench{})},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "List", Width: 480, Height: 800, URL: "/"})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
