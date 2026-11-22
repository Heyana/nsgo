//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestTextRunsKeepBackgroundInsets(t *testing.T) {
	txt := "before 😀 after"
	style := driver.TextStyle{Font: driver.Font{Size: 18, Weight: 400}, Color: driver.RGB(0)}
	data := textRunsJSON(&driver.TextContent{Text: txt, Style: style,
		Spans: []driver.TextSpan{{Start: 7, End: 11, Style: style,
			Background: driver.TextBackground{Color: driver.RGB(0xffcc00), Radius: 4,
				Top: driver.Pct(62), Bottom: driver.Pt(2)}}}})
	var runs []struct {
		T string
		B *struct {
			C       string
			R, T, B float32
			TP, BP  bool
		}
	}
	if err := json.Unmarshal([]byte(data), &runs); err != nil {
		t.Fatal(err)
	}
	if len(runs) != 3 || runs[0].B != nil || runs[2].B != nil || runs[1].B == nil {
		t.Fatalf("background runs = %s, want only the emoji highlighted", data)
	}
	bg := runs[1].B
	if runs[1].T != "😀" || bg.C != "rgba(255,204,0,1)" || bg.R != 4 || bg.T != 62 || !bg.TP || bg.B != 2 || bg.BP {
		t.Fatalf("background = %s, want radius 4, top 62%% and bottom 2 points", data)
	}
}

// TestTextDrawsNoAttachmentRunes keeps the rune an inline view stands on out of what ArkUI draws, which would show it as a box, in a plain text and in a text's runs alike.
func TestTextDrawsNoAttachmentRunes(t *testing.T) {
	text := "an icon " + string(driver.AttachmentRune) + " inline"
	if got := drawnText(text); got != "an icon  inline" {
		t.Errorf("drawn text %q, want the rune left out", got)
	}
	runs := textRunsJSON(&driver.TextContent{Text: text,
		Spans: []driver.TextSpan{{Start: 0, End: 3}, {Start: 3, End: len(text), Style: driver.TextStyle{Color: 0xff0000ff}}}})
	if runs == "" || strings.ContainsRune(runs, driver.AttachmentRune) {
		t.Errorf("runs %s, want them without the attachment rune", runs)
	}
}
