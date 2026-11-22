//go:build js && wasm

package harmonyos

import (
	"encoding/json"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/textruns"
	"github.com/nsgo-dev/nsgo/internal/platform/webstyle"
)

// textRun is one styled run of a text with spans, as the host draws a Span
// for it.
type textRun struct {
	T string          `json:"t"`
	C string          `json:"c,omitempty"`
	S float32         `json:"s,omitempty"`
	W int             `json:"w,omitempty"`
	I bool            `json:"i,omitempty"`
	F string          `json:"f,omitempty"`
	B *textBackground `json:"b,omitempty"`
}

// textBackground keeps inset units until ArkUI supplies each line's font box.
type textBackground struct {
	C  string  `json:"c"`
	R  float32 `json:"r,omitempty"`
	T  float32 `json:"t,omitempty"`
	B  float32 `json:"b,omitempty"`
	TP bool    `json:"tp,omitempty"`
	BP bool    `json:"bp,omitempty"`
}

// textRunsJSON draws a text's spans as one run each, taking the text's own
// font and color where a span does not set its own. It returns "" when the
// text has no spans, which the host draws as one Text.
func textRunsJSON(t *driver.TextContent) string {
	if t == nil || len(t.Spans) == 0 {
		return ""
	}
	runs := textruns.Split(textruns.Input{Text: t.Text, Spans: t.Spans, Style: t.Style})
	if len(runs) == 0 {
		return ""
	}
	base := t.Style.Font
	if base.Size == 0 {
		base.Size = 16
	}
	if base.Weight == 0 {
		base.Weight = 400
	}
	out := make([]textRun, 0, len(runs))
	for _, r := range runs {
		f := r.Style.Font
		if f.Size == 0 {
			f.Size = base.Size
		}
		if f.Weight == 0 {
			f.Weight = base.Weight
		}
		if f.Family == "" {
			f.Family = base.Family
		}
		c := r.Style.Color
		if c == 0 {
			c = t.Style.Color
		}
		run := textRun{T: drawnText(r.Text), C: webstyle.Color(c), S: float32(f.Size),
			W: int(f.Weight), I: f.Italic, F: f.Family}
		if bg := r.Background; bg.Color != 0 && run.T != "" {
			run.B = &textBackground{C: webstyle.Color(bg.Color), R: float32(max(0, bg.Radius))}
			if n, ok := bg.Top.Percent(); ok {
				run.B.T, run.B.TP = n, true
			} else if n, ok := bg.Top.Points(); ok {
				run.B.T = float32(n)
			}
			if n, ok := bg.Bottom.Percent(); ok {
				run.B.B, run.B.BP = n, true
			} else if n, ok := bg.Bottom.Points(); ok {
				run.B.B = float32(n)
			}
		}
		out = append(out, run)
	}
	b, err := json.Marshal(out)
	if err != nil {
		return ""
	}
	return string(b)
}

// drawnText is a text as ArkUI draws and measures it: without the attachment runes, which ArkUI would draw as boxes. The attachments' views are not drawn yet (#811).
func drawnText(s string) string { return strings.ReplaceAll(s, string(driver.AttachmentRune), "") }
