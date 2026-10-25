package highlight

import (
	"testing"
	"time"

	"github.com/oneo-me/nsgo/base/textview"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

func TestTokens(t *testing.T) {
	code := "// hi\nfunc main() { x := \"s\" + 42 }"
	got := map[string]kind{}
	for _, tk := range tokens("go", code) {
		got[code[tk.start:tk.end]] = tk.kind
	}
	for text, want := range map[string]kind{"// hi": comment, "func": keyword, "main": title, `"s"`: str, "42": number} {
		if got[text] != want {
			t.Errorf("%q is %d, want %d (all: %v)", text, got[text], want, got)
		}
	}
	if tokens("no-such-language", code) != nil || tokens("", code) != nil {
		t.Error("an unknown language was colored")
	}
}

func TestNewLexesUnmeasuredLanguagesOffTheEngine(t *testing.T) {
	var last []textview.Highlight
	calls := 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		h := New(cx)
		return textview.TextView{Markdown: "```python\ndef f(x):\n    return x  # done\n```",
			Highlighter: func(c textview.CodeBlock) []textview.Highlight {
				calls++
				last = h(c)
				return last
			}}
	})
	if len(last) != 0 {
		t.Fatal("a language never measured was lexed on the engine goroutine")
	}
	for i := 0; i < 200 && len(last) == 0; i++ {
		time.Sleep(5 * time.Millisecond)
		app.Settle()
	}
	if len(last) == 0 || calls < 2 {
		t.Fatalf("the block was not colored once lexed (%d calls)", calls)
	}
	if _, ok := speeds.Load("python"); !ok {
		t.Error("python's speed was not measured")
	}
}
