package main

import (
	"fmt"

	"github.com/oneo-me/nsgo/base/diff"
	"github.com/oneo-me/nsgo/core"
)

const diffOld = `package store

import (
	"errors"
	"sync"
)

// Store keeps values by key.
type Store struct {
	mu     sync.Mutex
	values map[string]string
}

// New returns an empty store.
func New() *Store {
	return &Store{values: map[string]string{}}
}

// Get returns the value of key.
func (s *Store) Get(key string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.values[key]
	if !ok {
		return "", errors.New("not found")
	}
	return v, nil
}

// Set sets the value of key.
func (s *Store) Set(key, value string) {
	s.mu.Lock()
	s.values[key] = value
	s.mu.Unlock()
}

// Len counts the keys.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.values)
}
`

const diffNew = `package store

import (
	"errors"
	"sync"
)

// ErrNotFound is what Get returns for a key the store lacks.
var ErrNotFound = errors.New("store: not found")

// Store keeps values by key, safe for concurrent use.
type Store struct {
	mu     sync.RWMutex
	values map[string]string
}

// New returns an empty store.
func New() *Store {
	return &Store{values: map[string]string{}}
}

// Get returns the value of key.
func (s *Store) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.values[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set sets the value of key.
func (s *Store) Set(key, value string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[key] = value
}

// Len counts the keys.
func (s *Store) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.values)
}
`

const diffPatch = `diff --git a/README.md b/README.md
index 3b18e51..a7c4f2d 100644
--- a/README.md
+++ b/README.md
@@ -1,6 +1,7 @@
 # Store

-A map with a lock.
+A map with a lock, safe for concurrent use.

 ## Use

+Get returns ErrNotFound for a key the store lacks.
diff --git a/store.go b/store.go
index 1f2e3d4..5a6b7c8 100644
--- a/store.go
+++ b/store.go
@@ -9,7 +9,7 @@ import (
 // Store keeps values by key.
 type Store struct {
-	mu     sync.Mutex
+	mu     sync.RWMutex
 	values map[string]string
 }

@@ -40,6 +40,6 @@ func (s *Store) Set(key, value string) {
 // Len counts the keys.
 func (s *Store) Len() int {
-	s.mu.Lock()
-	defer s.mu.Unlock()
+	s.mu.RLock()
+	defer s.mu.RUnlock()
 	return len(s.values)
 }
diff --git a/old_test.go b/store_test.go
similarity index 92%
rename from old_test.go
rename to store_test.go
`

// diffStyles is the gallery's diff look; base has none.
func diffStyles() diff.Styles {
	added, deleted := colors.up, colors.down
	return diff.Styles{
		Side:             core.Style{Borders: core.Edges{Right: 1}, BorderColor: colors.line},
		LineNumber:       core.Style{Color: colors.faint, FontSize: 12},
		LineAdded:        core.Style{Background: alpha(added, 0.12)},
		LineDeleted:      core.Style{Background: alpha(deleted, 0.12)},
		Filler:           core.Style{Background: alpha(colors.inset, 0.5)},
		WordAdded:        core.Style{Background: alpha(added, 0.30)},
		WordDeleted:      core.Style{Background: alpha(deleted, 0.30)},
		IndicatorAdded:   core.Style{Background: added, Color: added},
		IndicatorDeleted: core.Style{Background: deleted, Color: deleted},
		Content:          core.Style{Color: colors.text},
		Separator:        core.Style{Background: colors.inset, Color: colors.muted, Padding: core.Symmetric(0, 12), Cursor: core.CursorPointer},
		FileHeader: core.Style{Background: colors.raised, Color: colors.text, Padding: core.Symmetric(0, 12), Borders: core.Edges{Bottom: 1},
			BorderColor: colors.border, Cursor: core.CursorPointer},
	}
}

// diffDemo is base/diff's DiffView: two texts side by side or unified,
// the words changed marked, unchanged lines collapsed, changes navigated,
// reverted and kept, and a patch of several files.
func diffDemo(cx *core.Scope) core.Node {
	oldText, newText := core.NewSignal(cx, diffOld), core.NewSignal(cx, diffNew)
	unified, patch := core.NewSignal(cx, false), core.NewSignal(cx, false)
	inline := core.NewSignal(cx, diff.InlineWordAlt)
	stats := core.NewSignal(cx, diff.Stats{})
	last := core.NewSignal(cx, "—")
	ref := &diff.Ref{}
	pad := core.Points(16)
	if shell.desktop() {
		pad = 24
	}
	toggle := func(s *core.Signal[bool]) func() { return func() { s.Set(!s.GetUntracked()) } }
	small := func(label string, onPress func()) core.Node {
		return button(buttonProps{label: label, variant: secondary, small: true}, onPress)
	}
	modes := []option[diff.Inline]{{value: diff.InlineWordAlt, label: "Words"}, {value: diff.InlineWord, label: "Each word"},
		{value: diff.InlineChar, label: "Characters"}, {value: diff.InlineNone, label: "None"}}
	return core.View{Style: fillPage.Merge(core.Style{Background: colors.background, Padding: core.All(pad), Gap: 12}), Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
			tool("Show a patch", "Patch", patch.Get, toggle(patch)),
			tool("One column", "Unified", unified.Get, toggle(unified)),
			core.View{Style: core.Style{Width: core.Pt(360)}, Children: []core.Node{segmented(modes, inline.Get, inline.Set, true)}},
			tool("Previous change", "↑", nil, func() { last.Set(fmt.Sprintf("Change %d", ref.PreviousHunk()+1)) }),
			tool("Next change", "↓", nil, func() { last.Set(fmt.Sprintf("Change %d", ref.NextHunk()+1)) }),
			tool("Expand every line", "Expand", nil, ref.ExpandAll),
			tool("Collapse unchanged lines", "Collapse", nil, ref.CollapseAll),
			tool("Start again", "Reset", nil, func() { oldText.Set(diffOld); newText.Set(diffNew) }),
		}},
		core.View{Style: fin.card.over(core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), CornerRadius: radiusInset,
			BorderWidth: 1, BorderColor: colors.border, Background: colors.surface, Overflow: core.Hidden}), Children: []core.Node{
			core.Dynamic(func(cx *core.Scope) core.Node {
				d := diff.DiffView{
					Ref: ref, TestID: "diff", Language: "go", Unified: unified.Get(), Inline: inline.Get(),
					OnStatsChange: stats.Set,
					OnChange:      func(o, n string) { oldText.Set(o); newText.Set(n) },
					OnLineNumberPress: func(side diff.Side, line int) {
						last.Set(fmt.Sprintf("Line %d of the %s text", line, map[diff.Side]string{diff.Old: "old", diff.New: "new"}[side]))
					},
					OnFileToggle: func(f diff.File) { last.Set(fmt.Sprintf("%s collapsed: %v", f.Path, f.Collapsed)) },
					RenderHunkActions: func(cx *core.Scope, h diff.Hunk) core.Node {
						return core.View{Style: core.Style{Direction: core.Row, Gap: 4}, Children: []core.Node{
							small("Revert", func() { ref.RevertHunk(h.Index); last.Set(fmt.Sprintf("Reverted change %d", h.Index+1)) }),
							small("Keep", func() { ref.AcceptHunk(h.Index); last.Set(fmt.Sprintf("Kept change %d", h.Index+1)) }),
						}}
					},
					Styles: diffStyles(),
					Style:  core.Style{FlexGrow: 1, Color: colors.text},
				}
				if patch.Get() {
					d.Language = ""
					d.RenderHunkActions = nil
					d.Patch = func() string { return diffPatch }
				} else {
					d.OldValue, d.NewValue = oldText.Get, newText.Get
				}
				return d
			}),
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 16, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Text{Style: core.Style{FontSize: 12, FontFamily: mono}, SpansFunc: func() []core.Span {
				s := stats.Get()
				return []core.Span{{Content: fmt.Sprintf("+%d", s.Added), Style: core.Style{Color: colors.up}},
					{Content: " "}, {Content: fmt.Sprintf("−%d", s.Deleted), Style: core.Style{Color: colors.down}},
					{Content: fmt.Sprintf(" · %d changes", s.Hunks), Style: core.Style{Color: colors.muted}}}
			}},
			core.Text{ContentFunc: last.Get, MaxLines: 1, Style: core.Style{FlexShrink: 1, FontSize: 12, FontFamily: mono, Color: colors.muted}},
		}},
	}}
}
