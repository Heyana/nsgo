// Command stream streams a Markdown reply into a TextView as a language
// model would, token by token at an uneven pace, to show how it reads:
// the syntax the end leaves open shows as it will once closed, only the
// block being written changes, and each character fades in after the one
// before (TextView.FadeIn), so a burst of tokens spreads out rather than
// lands at once. The header picks the pace, turns the fade off to compare
// and plays the reply again.
package main

import (
	"math/rand/v2"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base/formula"
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/base/textview/highlight"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/nsgo-dev/nsgo/ui/chat"
)

func main() {
	nsgo.Run(Root, nsgo.WindowSize(860, 760))
}

// fade is how long a character takes to fade in, FlowDown's.
const fade = 320 * time.Millisecond

// pace is how the tokens arrive: Rate a second, each up to Jitter apart
// from the rate's interval, or in bursts of Burst tokens at once.
type pace struct {
	Name   string
	Rate   float64
	Jitter float64
	Burst  int
}

var paces = []pace{
	{Name: "Slow", Rate: 12, Jitter: 0.5},
	{Name: "Steady", Rate: 40, Jitter: 0.6},
	{Name: "Fast", Rate: 140, Jitter: 0.8},
	{Name: "Bursts", Rate: 3, Jitter: 0.6, Burst: 24},
}

// Root is the app: the header over the conversation.
func Root(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	reply := core.NewSignal(cx, "")
	running := core.NewSignal(cx, false)
	paceAt := core.NewSignal(cx, 1)
	fading := core.NewSignal(cx, true)
	state := chat.NewConversationState(cx)
	hl := highlight.New(cx)

	// play streams the reply again from its start; a play started later
	// stops the one before.
	run := 0
	play := func() {
		run++
		mine := run
		reply.Set("")
		running.Set(true)
		state.JumpToLatest()
		tokens := tokenize(answer)
		var next func(i int)
		next = func(i int) {
			if mine != run {
				return
			}
			if i >= len(tokens) {
				running.Set(false)
				return
			}
			p := paces[paceAt.GetUntracked()]
			n := 1
			if p.Burst > 0 {
				n = p.Burst/2 + rand.IntN(p.Burst)
			}
			end := min(i+n, len(tokens))
			reply.Set(reply.GetUntracked() + strings.Join(tokens[i:end], ""))
			wait := time.Duration(float64(time.Second) / p.Rate * (1 + p.Jitter*(2*rand.Float64()-1)))
			cx.AfterFunc(wait, func() { next(end) })
		}
		cx.AfterFunc(300*time.Millisecond, func() { next(0) })
	}
	cx.AfterFunc(400*time.Millisecond, play)

	paceButtons := make([]core.Node, len(paces))
	for i, p := range paces {
		paceButtons[i] = ui.ToggleButton{Text: p.Name, TestID: "pace-" + strings.ToLower(p.Name),
			On: func() bool { return paceAt.Get() == i }, OnChange: func(bool) { paceAt.Set(i) }}
	}
	header := core.View{
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 16, Padding: core.Symmetric(12, 20), Borders: core.Edges{Bottom: 1}},
		StyleFunc: func() core.Style {
			return core.Style{BorderColors: core.EdgeColors{Bottom: colors.Line()}}
		},
		Children: []core.Node{
			core.Text{Content: "Markdown Stream", Style: ui.TypeTitle, StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} }},
			core.View{Style: core.Style{FlexGrow: 1}},
			ui.ToggleGroup{Label: "Pace", Children: paceButtons},
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
				core.Text{Content: "Fade in", Style: ui.TypeBody, StyleFunc: func() core.Style { return core.Style{Color: colors.Text()} }},
				ui.Switch{Label: "Fade in", Checked: fading.Get, OnChange: fading.Set, TestID: "fade"},
			}},
			ui.Button{TextFunc: func() string {
				if running.Get() {
					return "Restart"
				}
				return "Replay"
			}, OnPress: play, TestID: "replay"},
		},
	}

	prompt := core.View{Style: core.Style{AlignSelf: core.AlignEnd}, Children: []core.Node{
		chat.Bubble{Children: []core.Node{core.Text{Content: question}}},
	}}
	answerView := core.Dynamic(func(cx *core.Scope) core.Node {
		var d time.Duration
		if fading.Get() {
			d = fade
		}
		// The reply always reads as streaming: a complete one ends with a
		// line break, which leaves nothing open to repair.
		return ui.TextView{MarkdownFunc: reply.Get, Options: textview.Options{Streaming: true}, FadeIn: d, Selectable: true,
			Highlighter: hl, RenderMath: formula.RenderMath, NoCodeBlockScrollbar: true, TestID: "reply",
			CodeBlockActions: []textview.CodeAction{{ID: textview.CopyAction, Label: "Copy"}}}
	})
	return core.View{Style: ui.FillRest, StyleFunc: ui.Filled(colors.Background), Children: []core.Node{
		header,
		chat.Conversation{State: state, JumpLabel: "Jump to the latest", ContentWidth: 680, Children: []core.Node{prompt, answerView}},
	}}
}

// tokenize splits text as a model's tokenizer roughly does: words with
// the space before them, long words in pieces of three to five letters,
// and every other character alone.
func tokenize(text string) []string {
	var out []string
	for text != "" {
		r, size := utf8.DecodeRuneInString(text)
		n := size
		if r == ' ' || unicode.IsLetter(r) && r < unicode.MaxLatin1 {
			for n < len(text) {
				r, s := utf8.DecodeRuneInString(text[n:])
				if !unicode.IsLetter(r) || r >= unicode.MaxLatin1 || n >= 3+rand.IntN(3) {
					break
				}
				n += s
			}
		}
		out = append(out, text[:n])
		text = text[n:]
	}
	return out
}

const question = "How does NSGo stream a model's Markdown reply so smoothly?"

const answer = "## Streaming Markdown smoothly\n\n" +
	"A reply arrives **token by token**, often in uneven bursts. NSGo's `TextView` keeps it calm in three ways:\n\n" +
	"1. **Repair the end.** Syntax the last block leaves open, like `**bold` or a half-written [link](https://nsgo.dev), shows as it will once closed, instead of flashing its markers.\n" +
	"2. **Touch only the tail.** Blocks are rows keyed by position and kind, so the paragraphs above keep their native views while the last one grows in place.\n" +
	"3. **Fade each character in.** New characters fade in one after another, 14 ms apart and never more than 160 ms behind, so a burst of forty tokens glides in instead of landing at once.\n\n" +
	"> [!TIP]\n> Switch *Fade in* off in the header and pick **Bursts** to see the difference.\n\n" +
	"### In code\n\n" +
	"```go\nreturn ui.TextView{\n\tMarkdownFunc: reply.Get,\n\tOptions:      textview.Options{Streaming: true},\n\tFadeIn:       320 * time.Millisecond,\n}\n```\n\n" +
	"### What it costs\n\n" +
	"| Step | Work per frame |\n|---|---|\n| Parse | the last block |\n| Layout | only when text grows |\n| Fade | colors of the last lines |\n\n" +
	"A fade changes colors alone, so the text is not laid out again: the alpha of each character follows an ease-out curve, $\\alpha(t) = 1 - (1 - t)^3$, in 32 steps.\n\n" +
	"中文与日本語也逐字淡入，每个字符独立出现，像笔尖划过纸面。\n\n" +
	"- [x] Repair open syntax\n- [x] Keep earlier blocks\n- [x] Fade in new characters\n"
