package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/base/editor"
	"github.com/oneo-me/nsgo/core"
)

// The sources the editor opens, by language.
var editorSources = map[string]string{
	"go": `package main

import (
	"fmt"
	"strings"
)

// Greeter says hello to people by name.
type Greeter struct {
	Prefix string
	count  int
}

// Greet greets name and counts the greetings.
func (g *Greeter) Greet(name string) string {
	g.count++
	// TODO: localize the greeting.
	return fmt.Sprintf("%s, %s!", g.Prefix, strings.TrimSpace(name))
}

func main() {
	g := &Greeter{Prefix: "Hello"}
	for _, name := range []string{"Ada", "Grace", "Linus"} {
		fmt.Println(g.Greet(name))
	}
	fmt.Printl("done")
}
`,
	"ts": `// A tiny store with subscribers.
type Listener<T> = (value: T) => void;

export class Store<T> {
  private listeners: Listener<T>[] = [];

  constructor(private value: T) {}

  get(): T {
    return this.value;
  }

  set(next: T) {
    this.value = next;
    // TODO: batch notifications.
    for (const listener of this.listeners) listener(next);
  }

  subscribe(listener: Listener<T>): () => void {
    this.listeners.push(listener);
    return () => {
      this.listeners = this.listeners.filter((l) => l !== listener);
    };
  }
}
`,
	"python": `"""Word counts of a text."""
from collections import Counter


def words(text: str) -> list[str]:
    return [w.strip(".,;:!?").lower() for w in text.split()]


def top(text: str, n: int = 3) -> list[tuple[str, int]]:
    # TODO: skip stop words.
    return Counter(words(text)).most_common(n)


if __name__ == "__main__":
    print(top("the quick brown fox jumps over the lazy dog the end"))
`,
	"markdown": "# Editor\n\nA **code editor** in `base/editor`:\n\n- several carets\n- folding by indentation\n- find and replace\n\n> Its look is the app's.\n",
}

var goKeywords = []string{"break", "case", "chan", "const", "continue", "default", "defer", "else", "fallthrough", "for",
	"func", "go", "goto", "if", "import", "interface", "map", "package", "range", "return", "select", "struct", "switch",
	"type", "var"}

var identifier = regexp.MustCompile(`[A-Za-z_][A-Za-z0-9_]*`)

// editorStyles is the gallery's editor look; base has none.
func editorStyles() editor.Styles {
	popup := fin.menu.over(core.Style{CornerRadius: 8, BorderWidth: 1, BorderColor: colors.border, Padding: core.All(4)})
	return editor.Styles{
		Gutter:                 core.Style{Color: colors.faint, FontSize: 12},
		ActiveLineNumber:       core.Style{Color: colors.text},
		FoldMarker:             core.Style{Color: colors.faint, Cursor: core.CursorPointer},
		Content:                core.Style{Color: colors.text},
		ActiveLine:             core.Style{Background: alpha(colors.text, 0.04)},
		Selection:              core.Style{Background: alpha(colors.accent, 0.22)},
		Caret:                  core.Style{Background: colors.accent, Width: core.Pt(2)},
		SearchMatch:            core.Style{Background: core.RGBA(0xfbbf24, 0.25)},
		CurrentSearchMatch:     core.Style{Background: core.RGBA(0xfbbf24, 0.55)},
		MatchingBracket:        core.Style{BorderWidth: 1, BorderColor: alpha(colors.muted, 0.6)},
		FoldPlaceholder:        core.Style{Color: colors.faint},
		Placeholder:            core.Style{Color: colors.faint},
		DiagnosticError:        core.Style{TextDecoration: core.TextDecorationUnderline, TextDecorationStyle: core.DecorationWavy, TextDecorationColor: colors.danger},
		DiagnosticWarning:      core.Style{TextDecoration: core.TextDecorationUnderline, TextDecorationStyle: core.DecorationWavy, TextDecorationColor: colors.warn},
		DiagnosticInfo:         core.Style{TextDecoration: core.TextDecorationUnderline, TextDecorationStyle: core.DecorationWavy, TextDecorationColor: colors.info},
		SelectionHandle:        core.Style{CornerRadius: 5, Background: colors.accent},
		CompletionMenu:         popup,
		CompletionItem:         core.Style{Padding: core.Symmetric(0, 8), Gap: 6, CornerRadius: 5, Color: colors.text},
		SelectedCompletionItem: core.Style{Background: alpha(colors.accent, 0.16)},
		CompletionMatch:        core.Style{FontWeight: semibold, Color: colors.accent},
		CompletionDetail:       core.Style{Color: colors.muted, FontSize: 12},
		CompletionKind:         core.Style{Color: colors.faint, FontSize: 11, Width: core.Pt(30)},
		CompletionDocumentation: fin.menu.over(core.Style{CornerRadius: 8, BorderWidth: 1, BorderColor: colors.border,
			Padding: core.Symmetric(6, 8), Color: colors.text, FontSize: 12}),
		DefinitionLink:   core.Style{Background: colors.accent},
		IndentGuide:      core.Style{Background: colors.border},
		InlineCompletion: core.Style{Color: colors.faint, Background: colors.surface},
	}
}

// lint is the gallery's toy linter: TODOs are information, a misspelled
// Println is an error, and lines past 100 columns are warnings.
func lint(text string) []editor.Diagnostic {
	var out []editor.Diagnostic
	for i, line := range strings.Split(text, "\n") {
		if at := strings.Index(line, "TODO"); at >= 0 {
			out = append(out, editor.Diagnostic{Severity: editor.SeverityInfo, Message: "A task left to do", Source: "lint",
				Range: editor.Range{Start: editor.Position{Line: i, Column: at}, End: editor.Position{Line: i, Column: len(line)}}})
		}
		if at := strings.Index(line, "Printl("); at >= 0 {
			out = append(out, editor.Diagnostic{Severity: editor.SeverityError, Message: "fmt.Printl is not declared; did you mean Println?",
				Source: "compiler", Range: editor.Range{Start: editor.Position{Line: i, Column: at}, End: editor.Position{Line: i, Column: at + 6}}})
		}
		if len(line) > 100 {
			out = append(out, editor.Diagnostic{Severity: editor.SeverityWarning, Message: "Longer than 100 columns", Source: "lint",
				Range: editor.Range{Start: editor.Position{Line: i, Column: 100}, End: editor.Position{Line: i, Column: len(line)}}})
		}
	}
	return out
}

// bigFile is 200 000 lines of Go.
func bigFile() string {
	var b strings.Builder
	for i := range 200_000 {
		fmt.Fprintf(&b, "func step%d(x int) int { return x*%d + %d } // line %d\n", i, i%7+1, i%13, i+1)
	}
	return b.String()
}

// editorDemo is base/editor's Editor: languages, find and replace with
// expressions, several carets, folds, soft wrap, a toy linter's
// diagnostics, completion from the text's words and go to definition,
// and 200 000 lines that open and scroll as a short file does.
func editorDemo(cx *core.Scope) core.Node {
	language := core.NewSignal(cx, "go")
	text := core.NewSignal(cx, editorSources["go"])
	softWrap, numbers, finding := core.NewSignal(cx, false), core.NewSignal(cx, true), core.NewSignal(cx, false)
	query, replacement := core.NewSignal(cx, ""), core.NewSignal(cx, "")
	regex := core.NewSignal(cx, false)
	result := core.NewSignal(cx, editor.SearchResult{Current: -1})
	caretAt := core.NewSignal(cx, "Ln 1, Col 1")
	last := core.NewSignal(cx, "—")
	problems := core.NewMemoFunc(cx, func() []editor.Diagnostic { return lint(text.Get()) }, func(a, b []editor.Diagnostic) bool {
		return slices.Equal(a, b)
	})
	ref := &editor.Ref{}
	pad := core.Points(16)
	if shell.desktop() {
		pad = 24
	}
	open := func(lang string) {
		language.Set(lang)
		if lang == "big" {
			text.Set(bigFile())
			return
		}
		text.Set(editorSources[lang])
	}
	complete := func(r editor.CompletionRequest) {
		seen := map[string]bool{}
		var items []editor.CompletionItem
		add := func(label, kind, detail string) {
			if !seen[label] && label != r.Query {
				seen[label] = true
				items = append(items, editor.CompletionItem{Label: label, Kind: kind, Detail: detail})
			}
		}
		if language.GetUntracked() == "go" {
			for _, k := range goKeywords {
				add(k, "kw", "keyword")
			}
		}
		for _, w := range identifier.FindAllString(text.GetUntracked(), 2000) {
			add(w, "id", "")
		}
		ref.PresentCompletions(r.ID, items, false)
		last.Set(fmt.Sprintf("Completion for %q", r.Query))
	}
	// suggest is the rest of the longest word of the text the word before
	// the caret starts, the gallery's inline completion.
	suggest := func(r editor.InlineCompletionRequest) {
		lines := strings.Split(text.GetUntracked(), "\n")
		if r.Position.Line >= len(lines) {
			return
		}
		before := lines[r.Position.Line][:min(r.Position.Column, len(lines[r.Position.Line]))]
		words := identifier.FindAllString(before, -1)
		if len(words) == 0 || !strings.HasSuffix(before, words[len(words)-1]) {
			return
		}
		typed := words[len(words)-1]
		best := ""
		for _, w := range identifier.FindAllString(text.GetUntracked(), 5000) {
			if len(w) > len(best) && len(w) > len(typed) && strings.HasPrefix(w, typed) {
				best = w
			}
		}
		if best != "" {
			ref.PresentInlineCompletion(r.ID, best[len(typed):])
		}
	}
	// definitionOf is where name is declared: a func, type or var line.
	definitionOf := func(name string) (int, int, bool) {
		decl := regexp.MustCompile(`^\s*(?:func(?:\s*\([^)]*\))?|type|var|const|class|def|export class)\s+` + regexp.QuoteMeta(name) + `\b`)
		for i, line := range strings.Split(text.GetUntracked(), "\n") {
			if loc := decl.FindStringIndex(line); loc != nil {
				return i, strings.Index(line[loc[0]:], name) + loc[0], true
			}
		}
		return 0, 0, false
	}
	toggle := func(s *core.Signal[bool]) func() { return func() { s.Set(!s.GetUntracked()) } }
	field := func(label string, value *core.Signal[string]) core.Node {
		return core.Input{Label: label, Placeholder: label, Value: value.Get, OnChange: value.Set,
			Style: fin.field.over(core.Style{Height: core.Pt(30), Width: core.Pt(180), CornerRadius: 6, BorderWidth: 1,
				BorderColor: colors.fieldBorder, Background: colors.field, Padding: core.Symmetric(0, 8), FontSize: 13, Color: colors.text})}
	}
	findBar := core.Show{When: finding.Get, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			field("Find", query),
			tool("Use a regular expression", ".*", regex.Get, toggle(regex)),
			tool("Previous match", "↑", nil, func() { ref.Run(editor.FindPrevious) }),
			tool("Next match", "↓", nil, func() { ref.Run(editor.FindNext) }),
			field("Replace", replacement),
			tool("Replace the match", "Replace", nil, func() { ref.Replace(replacement.GetUntracked()) }),
			tool("Replace every match", "All", nil, func() {
				n := ref.ReplaceAll(replacement.GetUntracked())
				last.Set(fmt.Sprintf("Replaced %d", n))
			}),
			core.Text{ContentFunc: func() string {
				r := result.Get()
				switch {
				case r.Err != "":
					return "Invalid expression"
				case r.Count == 0:
					return "No results"
				case r.Current < 0:
					return fmt.Sprintf("%d results", r.Count)
				}
				return fmt.Sprintf("%d of %d", r.Current+1, r.Count)
			}, Style: core.Style{FontSize: 12, Color: colors.muted, FontFamily: mono}},
		}}
	}}
	langs := []option[string]{{value: "go", label: "Go"}, {value: "ts", label: "TypeScript"}, {value: "python", label: "Python"},
		{value: "markdown", label: "Markdown"}, {value: "big", label: "200k lines"}}
	return core.View{Style: fillPage.Merge(core.Style{Background: colors.background, Padding: core.All(pad), Gap: 12}), Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(420)}, Children: []core.Node{segmented(langs, language.Get, open, true)}},
			tool("Find and replace", "Find", finding.Get, toggle(finding)),
			tool("Soft wrap", "Wrap", softWrap.Get, toggle(softWrap)),
			tool("Line numbers", "Numbers", numbers.Get, toggle(numbers)),
			tool("Fold everything", "Fold all", nil, func() { ref.Run(editor.FoldAll) }),
			tool("Unfold everything", "Unfold all", nil, func() { ref.Run(editor.UnfoldAll) }),
		}},
		findBar,
		core.View{Style: fin.card.over(core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), CornerRadius: radiusInset,
			BorderWidth: 1, BorderColor: colors.border, Background: colors.surface, Overflow: core.Hidden}), Children: []core.Node{
			// The props the editor reads once make another editor.
			core.Dynamic(func(cx *core.Scope) core.Node {
				lang, wrap, nums := language.Get(), softWrap.Get(), numbers.Get()
				if lang == "big" {
					lang = "go"
				}
				return editor.Editor{
					Ref: ref, TestID: "editor", Label: "Source", Language: lang, SoftWrap: wrap, NoLineNumbers: !nums,
					Value: text.Get, OnChange: text.Set, AutoFocus: true, Placeholder: "Type some code",
					Search: func() editor.Search {
						if !finding.Get() {
							return editor.Search{}
						}
						return editor.Search{Query: query.Get(), Regex: regex.Get()}
					},
					OnSearchChange:              result.Set,
					Diagnostics:                 problems.Get,
					CompletionTriggerCharacters: ".",
					OnCompletionRequest:         complete,
					OnCompletionAccept:          func(it editor.CompletionItem) { last.Set("Accepted " + it.Label) },
					InlineCompletionDelay:       150 * time.Millisecond,
					OnInlineCompletionRequest:   suggest,
					OnDefinitionRequest: func(r editor.DefinitionRequest) {
						lines := strings.Split(text.GetUntracked(), "\n")
						name := lines[r.Word.Start.Line][r.Word.Start.Column:r.Word.End.Column]
						line, col, ok := definitionOf(name)
						if !ok {
							return
						}
						switch r.Trigger {
						case editor.DefinitionHover:
							ref.PresentDefinitionLink(r.ID, r.Word)
						default:
							ref.SetSelections([]editor.Selection{{Anchor: editor.Position{Line: line, Column: col},
								Head: editor.Position{Line: line, Column: col + len(name)}}})
							last.Set(fmt.Sprintf("Definition of %s on line %d", name, line+1))
						}
					},
					OnHover: func(h editor.HoverEvent) {
						if len(h.Diagnostics) > 0 {
							last.Set(h.Diagnostics[0].Message)
						}
					},
					OnFindWithSelection: func(s string) { finding.Set(true); query.Set(s) },
					OnSelectionChange: func(sels []editor.Selection) {
						h := sels[len(sels)-1].Head
						s := fmt.Sprintf("Ln %d, Col %d", h.Line+1, h.Column+1)
						if len(sels) > 1 {
							s += fmt.Sprintf(" · %d carets", len(sels))
						}
						caretAt.Set(s)
					},
					OnLineNumberPress: func(line int) { last.Set(fmt.Sprintf("Line %d pressed", line+1)) },
					Styles:            editorStyles(),
					Style:             core.Style{FlexGrow: 1, Color: colors.text},
				}
			}),
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 16, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Text{ContentFunc: caretAt.Get, Style: core.Style{FontSize: 12, FontFamily: mono, Color: colors.text}},
			core.Text{ContentFunc: func() string {
				p := problems.Get()
				return fmt.Sprintf("%d problems", len(p))
			}, Style: core.Style{FontSize: 12, FontFamily: mono, Color: colors.muted}},
			core.Text{ContentFunc: last.Get, MaxLines: 1, Style: core.Style{FlexShrink: 1, FontSize: 12, FontFamily: mono, Color: colors.muted}},
		}},
	}}
}
