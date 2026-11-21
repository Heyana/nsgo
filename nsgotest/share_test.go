package nsgotest_test

import (
	"fmt"
	"slices"
	"testing"

	_ "github.com/nsgo-dev/nsgo" // Links the native test host.
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestReceiveShareKeepsBatchesUntilAHandlerRegisters(t *testing.T) {
	first := []mobile.SharedItem{
		{Text: "https://example.com", MIME: "text/plain"},
		{Path: "/cache/share/notes.pdf", Name: "notes.pdf", MIME: "application/pdf"},
	}
	second := []mobile.SharedItem{{Text: "second", MIME: "text/plain"}}
	third := []mobile.SharedItem{{Text: "third", MIME: "text/plain"}}
	var show *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		show = core.NewSignal(cx, false)
		batches := core.NewSignalFunc(cx, [][]mobile.SharedItem(nil), nil)
		return core.View{Children: []core.Node{
			core.Show{When: show.Get, Then: func(cx *core.Scope) core.Node {
				mobile.OnShared(cx, func(items []mobile.SharedItem) {
					batches.Set(append(slices.Clone(batches.Get()), items))
				})
				return core.Text{Content: "Listening"}
			}},
			core.Text{TestID: "shares", ContentFunc: func() string { return fmt.Sprint(batches.Get()) }},
		}}
	})
	app.ReceiveShare(first...)
	app.ReceiveShare()
	app.ReceiveShare(second...)
	app.Post(func() { show.Set(true) })
	app.Settle()
	if got, want := app.Find("id:shares").Text, fmt.Sprint([][]mobile.SharedItem{first, second}); got != want {
		t.Fatalf("shares after registering = %q, want %q", got, want)
	}
	app.ReceiveShare(third...)
	if got, want := app.Find("id:shares").Text, fmt.Sprint([][]mobile.SharedItem{first, second, third}); got != want {
		t.Fatalf("shares while running = %q, want %q", got, want)
	}
}
