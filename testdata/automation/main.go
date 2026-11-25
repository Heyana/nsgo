// Command automation is the app cmd/nsgo's end-to-end test drives through nsgo dev -automation: its menu, its Dock menu and an alert come from the app's function, and its window has a counter, a field echoing its text, an alert of its own, whether the token nsgo dev gave it is still in its environment and a long list.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"runtime"
	"strconv"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/automation"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
)

func main() { nsgo.Run(App) }

// App sets the menu (on macOS on the app's own surface, elsewhere on the main window's menu bar), the Dock/Jump List menu, and the main window's view.
func App(app *core.Scope) {
	count := core.NewSignal(app, 0)
	if err := automation.Register(app, automation.Tool{
		Name:        "set_count",
		Description: "Set the counter to a test value",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"count":{"type":"integer"}},"required":["count"]}`),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Count int `json:"count"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, err
			}
			count.Set(input.Count)
			return input, nil
		},
	}); err != nil {
		panic(err)
	}
	samplePNG := func() []byte {
		img := image.NewRGBA(image.Rect(0, 0, 2, 1))
		img.Set(0, 0, color.White)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}
		return buf.Bytes()
	}()
	if err := automation.Register(app, automation.Tool{
		Name:        "render_badge",
		Description: "Render a test PNG badge with optional text",
		InputSchema: json.RawMessage(`{"type":"object","properties":{"text":{"type":"string"}}}`),
		Handler: func(_ context.Context, args json.RawMessage) (any, error) {
			var input struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal(args, &input); err != nil {
				return nil, err
			}
			return automation.Result{
				Text:   input.Text,
				Images: []automation.Image{{Data: samplePNG}},
			}, nil
		},
	}); err != nil {
		panic(err)
	}
	if err := automation.Register(app, automation.Tool{
		Name:        "png_json",
		Description: "Return a plain JSON struct containing a png field",
		InputSchema: json.RawMessage(`{"type":"object"}`),
		Handler: func(context.Context, json.RawMessage) (any, error) {
			return struct {
				PNG []byte `json:"png"`
			}{PNG: samplePNG}, nil
		},
	}); err != nil {
		panic(err)
	}
	answer := core.NewSignal(app, "none")
	ask := func(cx *core.Scope) {
		base.ShowAlertDialog(cx, base.AlertDialog{Title: "Keep going?", Actions: []core.AlertAction{{Label: "Yes"}, {Label: "No", Role: core.AlertCancel}}},
			func(label string) { answer.Set(label) })
	}
	menu := []desktop.MenuItem{{Label: "Actions", Items: []desktop.MenuItem{
		{Label: "Reset", OnSelect: func() { count.Set(0) }},
		{Label: "Ask", OnSelect: func() { ask(app) }},
	}}}
	if runtime.GOOS == "darwin" {
		desktop.SetApplicationMenu(app, menu)
	}
	desktop.SetDockMenu(app, []desktop.MenuItem{{Label: "Add Ten", OnSelect: func() { count.Set(count.Get() + 10) }}})
	nsgo.SetMainView(app, func(cx *core.Scope) core.Node {
		if runtime.GOOS != "darwin" {
			desktop.SetApplicationMenu(cx, menu)
		}
		name := core.NewSignal(cx, "")
		button := func(id, label string, press func()) core.Node {
			return core.View{Role: core.RoleButton, TestID: id, OnPress: func(core.PressEvent) { press() },
				Style:    core.Style{Padding: core.Symmetric(6, 12), Background: core.RGB(0xdddddd), CornerRadius: 6},
				Children: []core.Node{core.Text{Content: label}}}
		}
		rows := make([]core.Node, 100)
		for i := range rows {
			rows[i] = core.Text{Content: "Row " + strconv.Itoa(i), TestID: "row-" + strconv.Itoa(i), Style: core.Style{Padding: core.All(8)}}
		}
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea}, Gap: 8}, Children: []core.Node{
			button("add", "Add", func() { count.Set(count.Get() + 1) }),
			core.Text{ContentFunc: func() string { return "Count: " + strconv.Itoa(count.Get()) }, TestID: "count"},
			core.Input{TestID: "name", Placeholder: "Name", OnChange: func(s string) { name.Set(s) }},
			core.Text{ContentFunc: func() string { return "Hello, " + name.Get() }, TestID: "greeting"},
			button("ask", "Ask", func() { ask(cx) }),
			core.Text{ContentFunc: func() string { return "Answer: " + answer.Get() }, TestID: "answer"},
			core.Text{Content: "Token in the environment: " + strconv.FormatBool(os.Getenv("NSGO_DEV_TOKEN") != ""), TestID: "token"},
			core.Scroll{TestID: "list", Style: core.Style{Height: core.Pt(200)}, Children: rows},
		}}
	})
}
