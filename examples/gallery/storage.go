package main

import (
	"fmt"
	"strconv"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

// Storage keys of the demo.
const (
	launchesKey = "launches"
	noteKey     = "note"
)

// storageDemo is Scope.Storage: a count of this demo's openings, a note
// saved as it is typed and every key the app keeps (the gallery's own last
// demo among them), all still there after a restart. Reads are synchronous
// and subscribe to changes.
func storageDemo(cx *core.Scope) core.Node {
	store := cx.Storage()
	saved := core.NewSignal(cx, "")
	launches := core.Untracked(cx, func() int {
		v, _ := store.GetItem(launchesKey)
		n, _ := strconv.Atoi(v)
		return n + 1
	})
	store.SetItem(launchesKey, strconv.Itoa(launches))
	shown := core.NewSignal(cx, launches)
	note := func() string { v, _ := store.GetItem(noteKey); return v }

	write := func(text string) {
		var err error
		if text != "" {
			err = store.SetItem(noteKey, text)
		} else {
			store.RemoveItem(noteKey)
		}
		if err != nil {
			saved.Set("failed")
		} else {
			saved.Set("saved")
		}
	}

	clearAll := func() {
		base.ShowAlertDialog(cx, base.AlertDialog{
			Title:       "Clear the storage?",
			Description: "Removes every key the gallery keeps, the launch count and the note among them.",
			Actions: []core.AlertAction{
				{Label: "Cancel", Role: core.AlertCancel},
				{Label: "Clear", Role: core.AlertDestructive},
			},
			CloseOnEscape: core.ToggleOn, CloseOnBackdropPress: core.ToggleOn,
		}, func(label string) {
			if label != "Clear" {
				return
			}
			store.Clear()
			shown.Set(0)
			saved.Set("")
		})
	}

	launchesCard := func(cx *core.Scope) core.Node {
		colors := colorsOf(cx)
		return card(cx, cardProps{icon: iconStorage, title: "Launches", subtitle: "Counted each time this demo opens", right: badge(cx, "Kept", toneUp)},
			figure(cx, func() string { return strconv.Itoa(shown.Get()) }, "", func() string {
				if shown.Get() == 1 {
					return "opening"
				}
				return "openings"
			}, toneNeutral),
			core.Text{ContentFunc: func() string { return fmt.Sprintf("SetItem(%q, %q)", launchesKey, strconv.Itoa(shown.Get())) },
				Style: core.Style{FontSize: 12, FontFamily: mono}, StyleFunc: colored(colors.muted)},
		)
	}

	noteCard := func(cx *core.Scope) core.Node {
		colors := colorsOf(cx)
		return card(cx, cardProps{icon: iconForm, title: "Note", subtitle: "Saved as you type, back when the demo opens again"},
			field(cx, base.Input{Multiline: true, Value: note, OnChange: write, Placeholder: "Write something, then come back to it",
				Label: "Note", TestID: "note", Style: core.Style{Height: core.Pt(120), Padding: core.Edges{Top: 12, Right: 14, Bottom: 12, Left: 14}}}),
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, MinHeight: core.Pt(16)}, Children: []core.Node{
				core.Show{When: func() bool { return saved.Get() == "saved" }, Then: func(*core.Scope) core.Node { return icon(iconCheck, 14, colors.up) }},
				core.Text{ContentFunc: func() string {
					return map[string]string{"": "", "saved": "Saved", "failed": "Not saved"}[saved.Get()]
				}, StyleFunc: func() core.Style {
					c := colors.faint()
					switch saved.Get() {
					case "failed":
						c = colors.danger()
					case "saved":
						c = colors.up()
					}
					return core.Style{FlexGrow: 1, FontSize: 12, Color: c}
				}},
				core.Text{ContentFunc: func() string { return fmt.Sprintf("%d chars", len([]rune(note()))) }, Style: core.Style{FontSize: 12}, StyleFunc: colored(colors.faint)},
			}},
		)
	}

	type entry struct {
		key    string
		length int
	}
	entries := func() []entry {
		var out []entry
		for _, k := range store.Keys() {
			v, _ := store.GetItem(k)
			out = append(out, entry{k, len(v)})
		}
		return out
	}
	keysCard := func(cx *core.Scope) core.Node {
		colors := colorsOf(cx)
		total := func() int {
			n := 0
			for _, e := range entries() {
				n += len(e.key) + e.length
			}
			return n
		}
		return card(cx, cardProps{icon: iconLists, title: "Stored keys", subtitle: "Keys and each value's length"},
			core.Show{When: func() bool { return len(store.Keys()) > 0 }, Else: func(*core.Scope) core.Node { return caption(cx, "Nothing stored", nil) },
				Then: func(*core.Scope) core.Node {
					return core.View{Children: []core.Node{core.Each(entries, func(e entry) string { return e.key },
						func(cx *core.Scope, e func() entry, i func() int) core.Node {
							colors := colorsOf(cx)
							return core.Dynamic(func(*core.Scope) core.Node {
								return listRow(cx, i() == 0, nil,
									core.Text{ContentFunc: func() string { return e().key }, MaxLines: 1, Style: core.Style{FlexGrow: 1, FlexShrink: 1, FontSize: 14, FontFamily: mono}, StyleFunc: colored(colors.text)},
									core.Text{ContentFunc: func() string { return fmt.Sprintf("%d chars", e().length) }, Style: core.Style{FontSize: 13}, StyleFunc: colored(colors.muted)},
								)
							})
						})}}
				}},
			core.Text{ContentFunc: func() string { return fmt.Sprintf("%s · %d chars of 5 MB", count(len(store.Keys()), "key"), total()) },
				Style: core.Style{FontSize: 13}, StyleFunc: colored(colors.faint)},
			button(buttonProps{label: "Clear all", variant: secondary, disabled: func() bool { return len(store.Keys()) == 0 }}, clearAll),
			text("The data stays on this device and survives restarts, under the app's id from nsgo.yaml.", 12, regular, colors.faint),
		)
	}

	dirsCard := func(cx *core.Scope) core.Node {
		dir := func(d string) string {
			if d == "" {
				return "none on this target"
			}
			return d
		}
		return card(cx, cardProps{icon: iconFile, title: "App directories", subtitle: "DataDir and CacheDir, named by the app's id"},
			caption(cx, "Data", nil), code(cx, dir(cx.DataDir())),
			caption(cx, "Cache", nil), code(cx, dir(cx.CacheDir())),
		)
	}

	return demoPage(cx, pageProps{}, launchesCard, noteCard, keysCard, dirsCard)
}
