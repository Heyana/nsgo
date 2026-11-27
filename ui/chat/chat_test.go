package chat

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

// TestMain hands the main thread to the platform under nsgo test -target.
func TestMain(m *testing.M) { nsgotest.Main(m) }

var (
	desktopEnv = core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US"}
	phoneEnv   = core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone, Locale: "en-US"}
)

// each runs fn in the four presentations every component is tested in.
func each(t *testing.T, fn func(t *testing.T, e core.Environment)) {
	darkDesktop, darkPhone := desktopEnv, phoneEnv
	darkDesktop.Dark, darkPhone.Dark = true, true
	for name, e := range map[string]core.Environment{"desktop": desktopEnv, "desktop-dark": darkDesktop, "phone": phoneEnv, "phone-dark": darkPhone} {
		t.Run(name, func(t *testing.T) { fn(t, e) })
	}
}

// start runs content under ui's theme for e, as an app sets it, with opts.
func start(t *testing.T, e core.Environment, content func(cx *core.Scope) core.Node, opts ...nsgotest.Option) *nsgotest.App {
	t.Helper()
	size := nsgotest.Size(1000, 760)
	if e.Idiom != core.IdiomDesktop {
		size = nsgotest.Size(390, 844)
	}
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.SetTheme(core.DefaultTheme())
		colors := ui.ColorsOf(cx)
		return core.View{Style: core.Style{FlexGrow: 1, Padding: core.All(16), Gap: 12}, StyleFunc: ui.Filled(colors.Background),
			Children: []core.Node{core.Func(content)}}
	}, append([]nsgotest.Option{size, nsgotest.Environment(e)}, opts...)...)
}

// chatStage is a transcript of one turn and a composer the test drives.
type chatStage struct {
	status *core.Signal[Status]
	final  *core.Signal[bool]
	draft  *core.Signal[string]
	sends  int
	stops  int
}

func startChat(t *testing.T, e core.Environment) (*nsgotest.App, *chatStage) {
	t.Helper()
	s := &chatStage{}
	app := start(t, e, func(cx *core.Scope) core.Node {
		s.status, s.final, s.draft = core.NewSignal(cx, Running), core.NewSignal(cx, false), core.NewSignal(cx, "")
		state := NewConversationState(cx)
		return FloatingConversation{State: state, JumpLabel: "Latest",
			Children: []core.Node{ui.Button{Text: "Complete", OnPress: func() { s.status.Set(Completed) }}, ui.Button{Text: "Finish", OnPress: func() { s.final.Set(true) }}, Turn{Status: s.status.Get, Conversation: state,
				Summary: func() string { return "Worked" }, Working: func() string { return "Working" },
				Prompt: func(*core.Scope) core.Node { return core.Text{Content: "Prompt"} },
				Steps: func() []Step {
					return []Step{{ID: "work", Title: "Ran it", Activities: []Activity{{ID: "read", Action: "Read", Detail: "main.go", Status: Completed,
						Content: func(*core.Scope) core.Node { return core.Text{Content: "Output"} }}}}}
				},
				HasFinal: s.final.Get, Final: func(*core.Scope) core.Node { return core.Text{Content: "Answer"} }}},
			Composer: []core.Node{Composer{Draft: s.draft.Get, OnDraftChange: s.draft.Set, Label: "Message", SendLabel: "Send", StopLabel: "Stop",
				Running: func() bool { return s.status.Get() == Running }, Glass: true, State: state,
				OnSend: func() { s.sends++ }, OnStop: func() { s.stops++ }}}}
	})
	return app, s
}

// TestConversationSearch searches folded steps and reasoning across turns from Mod+F, reveals each selected match clear of the find bar and the floating composer, and closes with Escape, the focus back in the composer and the turns folded again.
func TestConversationSearch(t *testing.T) {
	var state *ConversationState
	app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		state = NewConversationState(cx)
		draft := core.NewSignal(cx, "")
		turn := func(n int) core.Node {
			id := strconv.Itoa(n)
			word := "hay"
			if n%4 == 0 {
				word = "needle"
			}
			return Turn{Status: func() Status { return Completed }, HasFinal: func() bool { return true }, Conversation: state,
				Summary: func() string { return "Worked " + id }, Prompt: func(*core.Scope) core.Node { return core.Text{Content: "Prompt " + id} },
				Reasoning: func(*core.Scope) core.Node {
					return Reasoning{Text: func() string { return "reason " + id + " " + word }, Status: func() Status { return Completed }, Conversation: state}
				},
				Steps: func() []Step {
					return []Step{{ID: "step", Message: func(*core.Scope) core.Node {
						return textview.TextView{Markdown: "step " + id + " " + word}
					}}}
				},
				Final: func(*core.Scope) core.Node {
					return core.View{Style: core.Style{Height: core.Pt(240)}, Children: []core.Node{core.Text{Content: "Answer " + id}}}
				}}
		}
		var turns []core.Node
		for n := range 12 {
			turns = append(turns, turn(n+1))
		}
		return core.View{Style: core.Style{Height: core.Pt(700)}, Children: []core.Node{FloatingConversation{State: state, JumpLabel: "Latest", TopInset: 20, Children: turns,
			Composer: []core.Node{Composer{Draft: draft.Get, OnDraftChange: draft.Set, Label: "Message", SendLabel: "Send", StopLabel: "Stop", Glass: true, State: state}}}}}
	})
	if app.Exists("text:reason 4 needle") {
		t.Fatal("a completed turn's reasoning should start folded")
	}
	app.Focus("id:composer")
	app.Key("mod+f")
	if !app.Find("id:chat-find-input").Focused {
		t.Fatal("Mod+F did not focus the find field")
	}
	if app.Exists("text:reason 4 needle") {
		t.Fatal("the find bar unfolded the transcript before a query")
	}
	app.Type("id:chat-find-input", "needle")
	if !app.Exists("text:1 of 6") {
		t.Fatalf("the count is not 1 of 6\n%s", app.Dump())
	}
	if !app.Exists("text:reason 4 needle") || !app.Exists("text:step 12 needle") {
		t.Fatal("the search did not unfold the transcript's reasoning and steps")
	}
	clear := func(n int) {
		t.Helper()
		match, ok := state.Search().SelectedBounds()
		bar, composer := app.Find("id:chat-find-input").Parent.Bounds(), app.Find("id:chat-composer").Bounds()
		if !ok || match.Origin.Y < bar.Origin.Y+bar.Size.Height || match.Origin.Y+match.Size.Height > composer.Origin.Y {
			t.Fatalf("match %d at %v is not clear of the bar %v and the composer %v", n, match, bar, composer)
		}
	}
	clear(1)
	for n := 2; n <= 6; n++ {
		app.Press("id:chat-find-next")
		if !app.Exists("text:" + strconv.Itoa(n) + " of 6") {
			t.Fatalf("after next the count is not %d of 6", n)
		}
		clear(n)
	}
	app.Press("id:chat-find-previous")
	if !app.Exists("text:5 of 6") {
		t.Fatal("previous did not go back to 5 of 6")
	}
	app.Focus("id:chat-find-input")
	app.Key("escape")
	if app.Exists("id:chat-find-input") {
		t.Fatal("Escape left the find bar open")
	}
	if !app.Find("id:composer").Focused {
		t.Fatal("closing the find bar did not give the focus back to the composer")
	}
	if app.Exists("text:reason 4 needle") {
		t.Fatal("the reasoning stayed unfolded after the search closed")
	}
}

// TestTurnFolds checks a live group, then the fold a completed turn
// takes with its final response, and that a choice outlives updates.
func TestTurnFolds(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, _ := startChat(t, e)
		app.Find("text:Read")
		app.Find("text:Working")
		app.Press("role:button Complete")
		// A settled group closes, but the turn stays open for its answer.
		if app.Exists("text:Read") {
			t.Fatal("a settled group stayed open")
		}
		app.Find("text:Ran it")
		app.Press("role:button Finish")
		app.Find("text:Answer")
		if app.Exists("text:Ran it") {
			t.Fatal("a completed turn kept its steps")
		}
		app.Press("role:button Worked")
		app.Press("role:button Ran it")
		app.Press("role:button Read main.go")
		app.Find("text:Output")
	})
}

// TestTurnReadsItsStepsOnce checks that what reads a turn's steps shares one
// computation of them.
func TestTurnReadsItsStepsOnce(t *testing.T) {
	var tick *core.Signal[int]
	reads := 0
	app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		tick = core.NewSignal(cx, 0)
		return Turn{Status: func() Status { return Completed }, Summary: func() string { return "Worked" },
			Steps: func() []Step {
				tick.Get()
				reads++
				return []Step{{ID: "work", Title: "Ran it"}}
			}}
	})
	reads = 0
	tick.Set(1)
	app.Settle()
	if reads != 1 {
		t.Fatalf("%d reads", reads)
	}
}

// TestComposer checks Send follows the draft and that a draft takes
// Stop's place while running.
func TestComposer(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app, s := startChat(t, e)
		app.Press("role:button Stop")
		if s.stops != 1 {
			t.Fatalf("stops = %d", s.stops)
		}
		app.Type("id:composer", "Hello")
		if app.Exists("role:button Stop") {
			t.Fatal("Stop shows beside a draft")
		}
		app.Press("role:button Send")
		if s.sends != 1 {
			t.Fatalf("sends = %d", s.sends)
		}
	})
}

// TestFloatingEdgeFollowsComposer blurs the transcript behind a composer,
// and not at all once it goes.
func TestFloatingEdgeFollowsComposer(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		var composing *core.Signal[bool]
		app := start(t, e, func(cx *core.Scope) core.Node {
			composing = core.NewSignal(cx, true)
			state := NewConversationState(cx)
			return FloatingConversation{State: state, JumpLabel: "Latest", Children: []core.Node{core.Text{Content: "Done"}},
				Composer: []core.Node{core.Show{When: composing.Get, Then: func(*core.Scope) core.Node {
					return Composer{Label: "Message", SendLabel: "Send", Glass: true, State: state}
				}}}}
		})
		if h := app.Find("id:chat-edge").Frame.Size.Height; h <= 0 {
			t.Fatalf("edge %v behind a composer", h)
		}
		composing.Set(false)
		app.Settle()
		if app.Exists("id:chat-edge") {
			t.Fatal("edge without a composer")
		}
	})
}

// TestContentWidth caps the transcript and the composer at a conversation's
// reading width, keeping the scroll view at its edges, and caps neither
// without one.
func TestContentWidth(t *testing.T) {
	for _, width := range []core.Points{0, 660} {
		app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
			return FloatingConversation{State: NewConversationState(cx), JumpLabel: "Latest", ContentWidth: width,
				Children: []core.Node{core.View{TestID: "turn", Style: core.Style{Height: core.Pt(40)}}},
				Composer: []core.Node{core.View{TestID: "composer", Style: core.Style{Height: core.Pt(40)}}}}
		})
		scroll := app.Find("id:chat-transcript").Frame.Size.Width
		want := scroll - 40
		if width > 0 {
			want = width
		}
		for _, id := range []string{"turn", "composer"} {
			if got := app.Find("id:" + id).Frame.Size.Width; got != want {
				t.Errorf("ContentWidth %v: %s %v wide in %v, want %v", width, id, got, scroll, want)
			}
		}
		if scroll != 968 {
			t.Errorf("ContentWidth %v: transcript %v wide, want the conversation's 968", width, scroll)
		}
	}
}

// TestComposerAutoFocus focuses the editor as the composer mounts only with AutoFocus.
func TestComposerAutoFocus(t *testing.T) {
	for _, auto := range []bool{false, true} {
		app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
			return Composer{Label: "Message", SendLabel: "Send", AutoFocus: auto}
		})
		if focused := app.Find("id:composer").Focused; focused != auto {
			t.Fatalf("AutoFocus %v: focused %v", auto, focused)
		}
	}
}

// suggestStage is a composer whose Suggest offers the files a query starts, README.md and release.md unless given others, and whose OnSend clears the draft, beside another field.
type suggestStage struct {
	draft                     *core.Signal[string]
	keyboardHeight            atomic.Int64
	changes, sends, submitted int
}

func startSuggest(t *testing.T, e core.Environment, files ...Suggestion) (*nsgotest.App, *suggestStage) {
	t.Helper()
	s := &suggestStage{}
	if files == nil {
		files = []Suggestion{file("readme", "README.md"), file("release", "release.md")}
	}
	app := start(t, e, func(cx *core.Scope) core.Node {
		s.draft = core.NewSignal(cx, "")
		cx.Effect(func() { s.keyboardHeight.Store(int64(cx.Env().KeyboardHeight())) })
		return core.View{Children: []core.Node{
			core.Scroll{
				TestID: "keyboard-dismiss-control", Style: core.Style{Height: core.Pt(40)},
				Behavior: core.ScrollBehavior{KeyboardShouldPersistTaps: core.KeyboardTapsNever},
				Children: []core.Node{core.View{
					TestID: "keyboard-dismiss-tap", NoFocus: true,
					OnPress: func(core.PressEvent) {}, Style: core.Style{Height: core.Pt(40)},
				}},
			},
			Composer{Draft: s.draft.Get, OnDraftChange: func(v string) { s.changes++; s.draft.Set(v) }, Label: "Message", SendLabel: "Send", SuggestLabel: "Files",
				OnSend: func() { s.sends++; s.draft.Set("") },
				Suggest: func(query string) []Suggestion {
					var out []Suggestion
					for _, row := range files {
						if strings.HasPrefix(strings.ToLower(row.Label), strings.ToLower(query)) {
							out = append(out, row)
						}
					}
					return out
				}},
			core.Input{TestID: "other", Label: "Other", OnSubmit: func(string) { s.submitted++ }},
		}}
	})
	return app, s
}

// file is a row of a file inserted by its name.
func file(id, name string) Suggestion {
	return Suggestion{SuggestItem: base.SuggestItem{ID: id, Label: name, Insert: name}}
}

// TestComposerSuggestionsSkipDisabledRows highlights and accepts the enabled rows only, and shows a row's detail.
func TestComposerSuggestionsSkipDisabledRows(t *testing.T) {
	off := file("rejected", "rejected.md")
	off.Disabled, off.Detail = true, "Locked"
	notes := file("notes", "release-notes.md")
	notes.Icon, notes.Detail = lucide.File, "docs/"
	app, s := startSuggest(t, desktopEnv, off, file("readme", "README.md"), notes)
	app.Type("id:composer", "@re")
	app.Find("text:Locked")
	app.Find("text:docs/")
	if !app.Find("role:option rejected.md").Props.Disabled {
		t.Fatal("the disabled row is enabled")
	}
	app.Key("down")
	app.Key("down")
	app.Key("enter")
	if draft := s.draft.GetUntracked(); draft != "README.md" {
		t.Fatalf("draft %q", draft)
	}
}

// TestComposerSuggestionsScroll scrolls the list to the highlighted row as the keys move it past the rows in view.
func TestComposerSuggestionsScroll(t *testing.T) {
	var files []Suggestion
	for i := range 20 {
		files = append(files, file("f"+strconv.Itoa(i), "file"+strconv.Itoa(i)+".go"))
	}
	app, s := startSuggest(t, desktopEnv, files...)
	app.Type("id:composer", "@file")
	list := app.Find("role:listbox Files")
	if list.Content.Height <= list.Frame.Size.Height {
		t.Fatalf("content %v in a list %v tall: nothing to scroll", list.Content.Height, list.Frame.Size.Height)
	}
	app.Key("up")
	if y := app.Find("role:listbox Files").Offset.Y; y <= 0 {
		t.Fatalf("offset %v with the last row highlighted", y)
	}
	app.Key("enter")
	if draft := s.draft.GetUntracked(); draft != "file19.go" {
		t.Fatalf("draft %q", draft)
	}
}

// TestComposerSuggestions filters rows at the caret and accepts the selected row in the middle of the draft, the caret after it, without sending the message.
func TestComposerSuggestions(t *testing.T) {
	app, s := startSuggest(t, desktopEnv)
	app.Type("id:composer", "@re and more")
	app.Select("id:composer", 3, 3)
	app.Find("role:listbox Files")
	app.Find("text:README.md")
	app.Find("text:release.md")
	if !app.Find("role:combobox Message").Props.Expanded {
		t.Fatal("the editor is not expanded while the rows show")
	}
	changes := s.changes
	app.Key("down")
	app.Key("enter")
	if draft := s.draft.GetUntracked(); draft != "release.md and more" || s.changes != changes+1 || s.sends != 0 || app.Exists("id:composer-suggestions") {
		t.Fatalf("draft %q, %d changes, %d sends, list shown %v", draft, s.changes-changes, s.sends, app.Exists("id:composer-suggestions"))
	}
	if app.Find("role:combobox Message").Props.Expanded {
		t.Fatal("the editor stayed expanded")
	}
	if r := app.Find("id:composer").Selection; r.Start != 10 || r.End != 10 {
		t.Fatalf("selection %v", r)
	}
}

// TestComposerSuggestionsTapOnAPhone chooses a row by tapping it, as a phone's software keyboard sends no arrow keys: the list's taps leave the keyboard up and its rows take no focus, so the field keeps the focus.
func TestComposerSuggestionsTapOnAPhone(t *testing.T) {
	target := os.Getenv("NSGO_TEST_TARGET")
	if (runtime.GOOS == "ios" || runtime.GOOS == "android") && target == "" {
		t.Fatal("mobile mention acceptance needs NSGo's native test host, not a headless simulator process")
	}
	nativePhone := strings.HasPrefix(target, "ios") || strings.HasPrefix(target, "android")
	app, s := startSuggest(t, phoneEnv)
	if nativePhone {
		// Prove the real keyboard is present and the native dismissing tap works.
		app.Focus("id:composer")
		app.WaitUntil(10*time.Second, func() bool { return s.keyboardHeight.Load() > 0 })
		app.Press("id:keyboard-dismiss-tap")
		app.WaitUntil(10*time.Second, func() bool { return s.keyboardHeight.Load() == 0 && !app.Find("id:composer").Focused })
	}
	app.Type("id:composer", "@re")
	if nativePhone {
		app.WaitUntil(10*time.Second, func() bool { return s.keyboardHeight.Load() > 0 })
	}
	if taps := app.Find("role:listbox Files").Scroll.Behavior.KeyboardShouldPersistTaps; taps != core.KeyboardTapsHandled {
		t.Fatalf("KeyboardShouldPersistTaps = %v", taps)
	}
	if !app.Find("role:option README.md").Props.NoFocus {
		t.Fatal("a row takes the focus")
	}
	app.Press("role:option README.md")
	if draft := s.draft.GetUntracked(); draft != "README.md" || app.Exists("id:composer-suggestions") || !app.Find("id:composer").Focused {
		t.Fatalf("draft %q, list shown %v, field focused %v", draft, app.Exists("id:composer-suggestions"), app.Find("id:composer").Focused)
	}
	if nativePhone {
		// Observe through a keyboard dismissal animation, which can outlive Press.
		until := time.Now().Add(500 * time.Millisecond)
		app.WaitUntil(2*time.Second, func() bool {
			if s.keyboardHeight.Load() <= 0 || !app.Find("id:composer").Focused {
				t.Fatal("accepting a mention dismissed the software keyboard or blurred the field")
			}
			return time.Now().After(until)
		})
	}
}

// TestComposerSuggestionsPressDown accepts a row on a desktop as the button goes down, before the press could move the focus.
func TestComposerSuggestionsPressDown(t *testing.T) {
	app, s := startSuggest(t, desktopEnv)
	app.Type("id:composer", "@re")
	app.PressDown("role:option release.md")
	if draft := s.draft.GetUntracked(); draft != "release.md" || app.Exists("id:composer-suggestions") {
		t.Fatalf("draft %q, list shown %v", draft, app.Exists("id:composer-suggestions"))
	}
}

// TestComposerSuggestionsEscape closes the list and keeps the draft, which Return then sends.
func TestComposerSuggestionsEscape(t *testing.T) {
	app, s := startSuggest(t, desktopEnv)
	app.Type("id:composer", "@re")
	app.Key("escape")
	if app.Exists("id:composer-suggestions") || s.draft.GetUntracked() != "@re" {
		t.Fatalf("list shown %v, draft %q", app.Exists("id:composer-suggestions"), s.draft.GetUntracked())
	}
	app.Key("enter")
	if s.sends != 1 {
		t.Fatalf("sends = %d", s.sends)
	}
}

// TestComposerSuggestionsCloseWithTheFocus closes the list as another field takes the focus, whose Return is its own.
func TestComposerSuggestionsCloseWithTheFocus(t *testing.T) {
	app, s := startSuggest(t, desktopEnv)
	app.Type("id:composer", "@re")
	app.Type("id:other", "q")
	if app.Exists("id:composer-suggestions") {
		t.Fatal("the list stayed open")
	}
	app.Key("enter")
	if s.submitted != 1 || s.draft.GetUntracked() != "@re" {
		t.Fatalf("%d submits, draft %q", s.submitted, s.draft.GetUntracked())
	}
}

// TestComposerSuggestionsWithNoRows shows nothing for a query no row matches, and Return sends.
func TestComposerSuggestionsWithNoRows(t *testing.T) {
	app, s := startSuggest(t, desktopEnv)
	app.Type("id:composer", "ping @bob")
	if app.Exists("id:composer-suggestions") {
		t.Fatal("an empty list shows")
	}
	app.Key("enter")
	if s.sends != 1 {
		t.Fatalf("sends = %d", s.sends)
	}
}

// TestComposerSuggestionsFollowTheDraft closes the list as sending clears the draft, so Return does not bring the sent text back.
func TestComposerSuggestionsFollowTheDraft(t *testing.T) {
	app, s := startSuggest(t, desktopEnv)
	app.Type("id:composer", "hi @re")
	app.Find("id:composer-suggestions")
	app.Press("role:button Send")
	if s.sends != 1 || s.draft.GetUntracked() != "" || app.Exists("id:composer-suggestions") {
		t.Fatalf("%d sends, draft %q, list shown %v", s.sends, s.draft.GetUntracked(), app.Exists("id:composer-suggestions"))
	}
	app.Key("enter")
	if draft := s.draft.GetUntracked(); draft != "" {
		t.Fatalf("draft %q", draft)
	}
}

// TestComposerFooterHidesWithKeyboard leaves the footer out while the
// software keyboard shows.
func TestComposerFooterHidesWithKeyboard(t *testing.T) {
	app := start(t, phoneEnv, func(cx *core.Scope) core.Node {
		return Composer{Label: "Message", SendLabel: "Send", Footer: func(*core.Scope) core.Node { return ui.Button{Text: "Project"} }}
	})
	if !app.Exists("role:button Project") {
		t.Fatal("footer missing without the keyboard")
	}
	e := phoneEnv
	e.KeyboardHeight = 300
	app.SetEnvironment(e)
	if app.Exists("role:button Project") {
		t.Fatal("footer shows over the keyboard")
	}
	app.SetEnvironment(phoneEnv)
	if !app.Exists("role:button Project") {
		t.Fatal("footer stays away after the keyboard")
	}
}

// TestChipYields shortens a yielding chip before the one beside it.
func TestChipYields(t *testing.T) {
	app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		measured := Chip{Text: "High", OnPress: func() {}, TestID: "measured"}
		return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{measured}},
			core.View{Style: core.Style{Direction: core.Row, Width: core.Pt(150), Gap: 4}, Children: []core.Node{
				Chip{Text: "A rather long model name that cannot fit", Yield: true, OnPress: func() {}, TestID: "yielding"},
				Chip{Text: "High", OnPress: func() {}, TestID: "other"},
			}},
		}}
	})
	whole := app.Find("id:measured").Frame.Size.Width
	other, yielding := app.Find("id:other").Frame.Size.Width, app.Find("id:yielding").Frame.Size.Width
	if other != whole {
		t.Fatalf("other chip = %v, want %v", other, whole)
	}
	if yielding+other+4 > 150 {
		t.Fatalf("chips = %v and %v in 150", yielding, other)
	}
}

// TestPickerChooses checks the options a query leaves and choosing one.
func TestPickerChooses(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		app := start(t, e, func(cx *core.Scope) core.Node {
			v := core.NewSignal(cx, "a")
			return Picker{Label: v.Get, Selected: v.Get, OnChange: v.Set, SearchLabel: "Search", EmptyText: "None", Searchable: 1, TestID: "picker",
				Options: []Option{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta", Group: "Second"}}}
		})
		app.Press("id:picker")
		app.Press("role:button Beta")
		app.Find("text:b")
		if app.Exists("role:button Alpha") {
			t.Fatal("choosing kept the popover open")
		}
	})
}

// TestEffortStaysOpen checks a press on the bar picks its level and
// keeps the panel.
func TestEffortStaysOpen(t *testing.T) {
	chosen := ""
	app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		v := core.NewSignal(cx, "off")
		return EffortPicker{Label: "Effort", Selected: v.Get, OnChange: func(id string) { chosen = id; v.Set(id) }, Off: true, TestID: "effort",
			Levels: []Option{{ID: "off", Label: "Off"}, {ID: "low", Label: "Low"}, {ID: "max", Label: "Top"}}}
	})
	app.Press("id:effort")
	track := app.Find("id:effort-bar").Children[0]
	app.Touch("view:"+strconv.Itoa(int(track.ID)), core.Point{X: track.Frame.Size.Width / 2, Y: 2}).Lift()
	app.Advance(time.Second)
	if chosen != "low" || !app.Exists("id:effort-bar") {
		t.Fatalf("chose %q, want the middle level with the panel open", chosen)
	}
}

// TestEffortKeys moves the level with the keys, the bar's thumb taking the
// focus as the panel opens, and assistive technology hears the level's name.
func TestEffortKeys(t *testing.T) {
	chosen := ""
	app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		v := core.NewSignal(cx, "off")
		return EffortPicker{Label: "Effort", Selected: v.Get, OnChange: func(id string) { chosen = id; v.Set(id) }, TestID: "effort",
			Levels: []Option{{ID: "off", Label: "Off"}, {ID: "low", Label: "Low"}, {ID: "max", Label: "Top"}}}
	})
	app.Press("id:effort")
	app.Settle()
	app.Key("end")
	if chosen != "max" {
		t.Fatalf("End chose %q", chosen)
	}
	if got := app.Find("role:adjustable").Props.Value.Text; got != "Top" {
		t.Fatalf("the thumb reads %q", got)
	}
}

// TestPickerLeavesNoFill checks the trigger returns to rest once the
// pointer has left while its popover was open.
func TestPickerLeavesNoFill(t *testing.T) {
	app := start(t, desktopEnv, func(cx *core.Scope) core.Node {
		v := core.NewSignal(cx, "a")
		return Picker{Label: v.Get, Selected: v.Get, OnChange: v.Set, TestID: "picker",
			Options: []Option{{ID: "a", Label: "Alpha"}, {ID: "b", Label: "Beta"}}}
	})
	rest := app.Find("id:picker").Style.Background
	app.Hover("id:picker")
	app.Press("id:picker")
	app.HoverOut()
	app.Press("role:button Beta")
	if got := app.Find("id:picker").Style.Background; got != rest {
		t.Fatalf("background after closing = %x, want %x", got, rest)
	}
}

// TestFloatingMargin lets a footer stand in the safe area, and the keyboard
// replace the safe area.
func TestFloatingMargin(t *testing.T) {
	e := phoneEnv
	e.SafeArea = core.Edges{Bottom: 34}
	var withFooter, without core.Points
	start(t, e, func(cx *core.Scope) core.Node {
		withFooter, without = FloatingMargin(cx, FooterHeight), FloatingMargin(cx, 0)
		return core.View{}
	})
	if withFooter != 14 || without != 42 {
		t.Fatalf("margins %v and %v", withFooter, without)
	}
}

// TestQuotaPace marks a row's pace across its bar, in the panel and the
// summary, and leaves a bar without one alone.
func TestQuotaPace(t *testing.T) {
	each(t, func(t *testing.T, e core.Environment) {
		rows := []QuotaRow{{Label: "5-hour limit", Value: "60% left", Fraction: 0.6, Pace: 0.25}, {Label: "Weekly", Value: "90% left", Fraction: 0.9}}
		app := start(t, e, func(cx *core.Scope) core.Node {
			return core.View{Style: core.Style{Width: core.Pt(300)}, Children: []core.Node{
				core.View{TestID: "panel", Children: []core.Node{Quota{ID: "demo", Title: "Account", Rows: func() []QuotaRow { return rows }}}},
				core.View{TestID: "summary", Style: core.Style{Direction: core.Row}, Children: []core.Node{QuotaSummary{Label: "Limits", Row: func() QuotaRow { return rows[0] }}}},
			}}
		})
		for _, id := range []string{"panel", "summary"} {
			var marks []core.DrawOp
			for _, v := range quotaViews(app.Find(id)) {
				for _, op := range v.Ops {
					if op.Kind == core.DrawRoundRect {
						marks = append(marks, op)
						if bar := v.Frame.Size.Width; op.X+op.W/2 != float32(bar)/4 || op.H != float32(v.Frame.Size.Height) {
							t.Errorf("%s mark at %v of %v, want a quarter across", id, op.X+op.W/2, bar)
						}
					}
				}
			}
			if len(marks) != 1 {
				t.Errorf("%s draws %d marks, want 1", id, len(marks))
			}
		}
	})
}

func quotaViews(v *nsgotest.View) []*nsgotest.View {
	out := []*nsgotest.View{v}
	for _, c := range v.Children {
		out = append(out, quotaViews(c)...)
	}
	return out
}

// The system's glass draws its own rim on macOS, iOS and Windows, but not on the watch or in a web build that runs as them.
func TestChatGlassEdge(t *testing.T) {
	rim := core.RGB(0x123456)
	for _, tc := range []struct {
		env  core.Environment
		want core.Color
	}{
		{desktopEnv, core.Transparent},
		{core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Simulated: true}, rim},
		{core.Environment{OS: core.OSIOS, Idiom: core.IdiomWatch}, rim},
		{core.Environment{OS: core.OSLinux, Idiom: core.IdiomDesktop}, rim},
	} {
		var got core.Color
		nsgotest.Start(t, func(cx *core.Scope) core.Node {
			got = chatGlassEdge(cx, rim)
			return core.View{}
		}, nsgotest.Environment(tc.env))
		if got != tc.want {
			t.Errorf("%s, idiom %v, simulated %v: edge %v", tc.env.OS, tc.env.Idiom, tc.env.Simulated, got)
		}
	}
}
