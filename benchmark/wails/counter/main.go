// Command counter is the counter case on Wails: a count and two buttons, in a page of the system webview.
package main

import (
	"embed"
	"log"

	"github.com/nsgo-dev/nsgo/benchmark/internal/bench"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed index.html
var assets embed.FS

// Bench is what the page calls: Ready once it has drawn its first frame.
type Bench struct{}

// Ready prints "bench ready".
func (*Bench) Ready() { bench.Frame() }

func main() {
	app := application.New(application.Options{
		Name:     "Counter",
		Services: []application.Service{application.NewService(&Bench{})},
		Assets:   application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{Title: "Counter", Width: 360, Height: 260, URL: "/"})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
