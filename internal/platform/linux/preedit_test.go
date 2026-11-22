package linux

import (
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/field"
)

// mockGTKField drives preeditState + field.State through the same signal
// sequence GTK 4's GtkText/GtkTextView and GtkIMContext emit on edit,
// preedit-changed, reset and EventControllerFocus leave.
type mockGTKField struct {
	text    string
	focused bool
	fs      field.State
	preedit preeditState
	events  []driver.Event
}

func newMockGTKField(initial string) *mockGTKField {
	m := &mockGTKField{text: initial, focused: true}
	m.fs = field.State{
		ID:     42,
		Events: (1 << driver.EventChangeText) | (1 << driver.EventCompositionChange) | (1 << driver.EventBlur) | (1 << driver.EventFocus),
		Dispatch: func(ev driver.Event) {
			m.events = append(m.events, ev)
		},
	}
	if initial != "" {
		m.fs.Value(initial, 0)
	}
	return m
}

func (m *mockGTKField) insert(s string) {
	if s == "" || m.fs.Config.Readonly || m.fs.Config.Disabled {
		return
	}
	kept, ok, _ := m.fs.Insert(m.text, 0, s)
	if !ok || kept == "" {
		return
	}
	m.text += kept
	if m.fs.Edited(m.text) {
		m.preedit.Edited()
	}
}

func (m *mockGTKField) onPreedit(text string, editorHasFocus bool) {
	focused := !m.focused || editorHasFocus
	switch m.preedit.Changed(text, focused) {
	case preeditUpdate:
		if v, apply := m.fs.Compose(true, text, m.text); apply {
			m.text = v
		}
	case preeditEnd:
		if v, apply := m.fs.Compose(false, "", m.text); apply {
			m.text = v
		}
	}
}

func (m *mockGTKField) finishPreedit() {
	if commit, _ := m.preedit.Finish(); commit != "" {
		m.insert(commit)
	}
	if v, apply := m.fs.Compose(false, "", m.text); apply {
		m.text = v
	}
}

func (m *mockGTKField) leave() {
	if !m.focused {
		return
	}
	m.focused = false
	m.finishPreedit()
	if m.fs.Events.Has(driver.EventBlur) {
		m.fs.Dispatch(driver.Event{Kind: driver.EventBlur, ID: m.fs.ID})
	}
}

func (m *mockGTKField) kinds() []string {
	var out []string
	for _, ev := range m.events {
		switch ev.Kind {
		case driver.EventChangeText:
			out = append(out, "change:"+ev.Text)
		case driver.EventCompositionChange:
			if ev.Finished {
				out = append(out, "comp:end")
			} else {
				out = append(out, "comp:"+ev.Text)
			}
		case driver.EventBlur:
			out = append(out, "blur")
		}
	}
	return out
}

func TestPreeditBlurCommitsWhenIMEDoesNothing(t *testing.T) {
	m := newMockGTKField("hi ")
	m.onPreedit("ni", true)
	m.leave()

	want := []string{"comp:ni", "change:hi ni", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "hi ni" {
		t.Fatalf("text = %q, want %q", m.text, "hi ni")
	}
}

func TestPreeditBlurCommitsWhenIMEDropsPreeditOnFocusOut(t *testing.T) {
	// Models Wayland text-input-v3 or IBus/Fcitx with commit-on-focus-out off:
	// gtk_im_context_focus_out emits preedit-changed("") after GTK clears
	// focus, without emitting commit, followed by our leave callback.
	m := newMockGTKField("hi ")
	m.onPreedit("ni", true)
	m.onPreedit("", false) // focus already leaving, no commit
	m.leave()

	want := []string{"comp:ni", "change:hi ni", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "hi ni" {
		t.Fatalf("text = %q, want %q", m.text, "hi ni")
	}
}

func TestPreeditBlurDoesNotDuplicateWhenIMECommitsThenClears(t *testing.T) {
	// Models IBus/Fcitx committing on focus-out: commit -> preedit-changed("") -> leave.
	m := newMockGTKField("hi ")
	m.onPreedit("ni", true)
	m.insert("你")
	m.onPreedit("", false)
	m.leave()

	want := []string{"comp:ni", "change:hi 你", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "hi 你" {
		t.Fatalf("text = %q, want %q", m.text, "hi 你")
	}
}

func TestPreeditBlurDoesNotDuplicateWhenIMEClearsThenCommits(t *testing.T) {
	// Models an IME that emits preedit-changed("") before commit during focus-out.
	m := newMockGTKField("hi ")
	m.onPreedit("ni", true)
	m.onPreedit("", false)
	m.insert("你")
	m.leave()

	want := []string{"comp:ni", "change:hi 你", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "hi 你" {
		t.Fatalf("text = %q, want %q", m.text, "hi 你")
	}
}

func TestPreeditBlurDoesNotDuplicateWhenIMECommitsWithoutClearingPreedit(t *testing.T) {
	m := newMockGTKField("hi ")
	m.onPreedit("ni", true)
	m.insert("你")
	m.leave()

	want := []string{"comp:ni", "change:hi 你", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "hi 你" {
		t.Fatalf("text = %q, want %q", m.text, "hi 你")
	}
}

func TestPreeditCancelWhileFocusedDoesNotCommit(t *testing.T) {
	m := newMockGTKField("hi ")
	m.onPreedit("ni", true)
	m.onPreedit("", true) // cancelled via Escape/Backspace while still focused
	m.leave()

	want := []string{"comp:ni", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "hi " {
		t.Fatalf("text = %q, want %q", m.text, "hi ")
	}
}

func TestPreeditMultiSegmentCommitThenBlurCommitsRemainderOnly(t *testing.T) {
	// CJK clause commit: IME commits "你" while "hao" remains in preedit, then blurs.
	m := newMockGTKField("")
	m.onPreedit("nihao", true)
	m.insert("你")
	m.onPreedit("hao", true)
	m.leave()

	want := []string{"comp:nihao", "change:你", "comp:hao", "change:你hao", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "你hao" {
		t.Fatalf("text = %q, want %q", m.text, "你hao")
	}
}

func TestPreeditBlurRespectsMaxLengthAndDropsStaleHeldValue(t *testing.T) {
	m := newMockGTKField("ab")
	m.fs.Config.MaxLength = 3
	m.onPreedit("cd", true)
	// App attempts to overwrite text during composition with a stale count.
	if m.fs.Value("stale", 0) {
		t.Fatal("value applied during active composition")
	}
	m.leave()

	want := []string{"comp:cd", "change:abc", "comp:end", "blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
	if m.text != "abc" {
		t.Fatalf("text = %q, want %q", m.text, "abc")
	}
}

func TestPreeditForceFinishOnReadonlyOrDisabled(t *testing.T) {
	t.Run("reset drops preedit", func(t *testing.T) {
		m := newMockGTKField("")
		m.onPreedit("ni", true)
		m.preedit.BeginForceFinish()
		// Simulate gtk_text_view_reset_im_context clearing preedit while focused.
		m.onPreedit("", true)
		m.finishPreedit()
		m.fs.Config.Readonly = true

		want := []string{"comp:ni", "change:ni", "comp:end"}
		if !slices.Equal(m.kinds(), want) {
			t.Fatalf("events = %v, want %v", m.kinds(), want)
		}
		if m.text != "ni" {
			t.Fatalf("text = %q, want %q", m.text, "ni")
		}
	})

	t.Run("reset commits candidate", func(t *testing.T) {
		m := newMockGTKField("")
		m.onPreedit("ni", true)
		m.preedit.BeginForceFinish()
		m.insert("你")
		m.onPreedit("", true)
		m.finishPreedit()
		m.fs.Config.Disabled = true
		m.leave()

		want := []string{"comp:ni", "change:你", "comp:end", "blur"}
		if !slices.Equal(m.kinds(), want) {
			t.Fatalf("events = %v, want %v", m.kinds(), want)
		}
		if m.text != "你" {
			t.Fatalf("text = %q, want %q", m.text, "你")
		}
	})
}

func TestBlurWithoutCompositionOnlyEmitsBlur(t *testing.T) {
	m := newMockGTKField("abc")
	m.leave()

	want := []string{"blur"}
	if !slices.Equal(m.kinds(), want) {
		t.Fatalf("events = %v, want %v", m.kinds(), want)
	}
}
