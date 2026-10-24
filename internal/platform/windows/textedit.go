//go:build windows

package windows

// The editing model of a drawn text input, as nsh-core's text_edit.rs,
// without a window: the text, the selection, caret stops and word
// boundaries, undo and redo, and the IME's composition shown at the
// caret. Offsets are bytes of the UTF-8 text, always on character
// boundaries; DirectWrite counts UTF-16 units, which the helpers at the
// end convert.

import (
	"strings"
	"unicode"
	"unicode/utf16"
	"unicode/utf8"
)

// editUnit is how far a caret movement or a deletion goes.
type editUnit uint8

const (
	// unitChar is one user-perceived character: a base character with its
	// marks.
	unitChar editUnit = iota
	// unitWord goes to the next word boundary.
	unitWord
)

// editKind is what an edit was, to merge runs of typing (or of deleting)
// into one undo step.
type editKind uint8

const (
	editNone editKind = iota // the caret moved since the last edit
	editTyping
	editDeleting
	editOther
)

// snapshot is the text and selection at one point of the history.
type snapshot struct {
	text          string
	anchor, caret int
}

// imeText is text an input method is composing, shown at the caret
// until it is committed or cancelled.
type imeText struct {
	text string
	// cursor is the caret inside text.
	cursor int
	// target is the clause being converted (bytes of text), drawn with a
	// thicker underline; hasTarget says whether there is one.
	targetStart, targetEnd int
	hasTarget              bool
}

// maxUndo is the most undo steps kept.
const maxUndo = 200

// editModel is a text input's text, selection and history.
type editModel struct {
	text string
	// anchor is where the selection started, the end that stays when it
	// extends; caret is the end that moves.
	anchor, caret int
	undos, redos  []snapshot
	last          editKind
	// composition is the IME's text while it composes: non-nil from the
	// composition's start to its end, empty until the IME sends text.
	composition *imeText
}

func (m *editModel) selection() (start, end int) {
	return min(m.anchor, m.caret), max(m.anchor, m.caret)
}

func (m *editModel) hasSelection() bool { return m.anchor != m.caret }

func (m *editModel) selected() string {
	s, e := m.selection()
	return m.text[s:e]
}

// composing reports whether the IME shows text.
func (m *editModel) composing() bool { return m.composition != nil && m.composition.text != "" }

// setText replaces the text without history (a value from the app),
// keeping the caret where it was, or at the end if it was there.
func (m *editModel) setText(text string) {
	caret := len(text)
	if m.caret < len(m.text) {
		caret = floorChar(text, min(m.caret, len(text)))
	}
	m.text = text
	m.anchor, m.caret = caret, caret
	// The old history would bring back text the app replaced.
	m.undos, m.redos = nil, nil
	m.last = editNone
	m.composition = nil
}

// selectRange selects from anchor to caret, clamped to the text and moved
// onto character boundaries.
func (m *editModel) selectRange(anchor, caret int) {
	anchor = floorChar(m.text, min(anchor, len(m.text)))
	caret = floorChar(m.text, min(caret, len(m.text)))
	if anchor != m.anchor || caret != m.caret {
		m.last = editNone
	}
	m.anchor, m.caret = anchor, caret
}

// moveTo moves the caret to to, extending the selection or not.
func (m *editModel) moveTo(to int, extend bool) {
	anchor := to
	if extend {
		anchor = m.anchor
	}
	m.selectRange(anchor, to)
}

// step moves the caret by unit, forward or back. Without extend, a
// selection collapses to its end in that direction instead.
func (m *editModel) step(unit editUnit, forward, extend bool) {
	if !extend && m.hasSelection() && unit == unitChar {
		s, e := m.selection()
		to := s
		if forward {
			to = e
		}
		m.selectRange(to, to)
		return
	}
	m.moveTo(m.boundary(m.caret, unit, forward), extend)
}

// boundary returns the position unit away from at.
func (m *editModel) boundary(at int, unit editUnit, forward bool) int {
	switch {
	case unit == unitChar && forward:
		return nextCluster(m.text, at)
	case unit == unitChar:
		return prevCluster(m.text, at)
	case forward:
		return wordRight(m.text, at)
	default:
		return wordLeft(m.text, at)
	}
}

func (m *editModel) selectAll() { m.selectRange(0, len(m.text)) }

func (m *editModel) snapshot() snapshot { return snapshot{m.text, m.anchor, m.caret} }

// record keeps the state before an edit of kind, unless it continues a
// run of the same kind.
func (m *editModel) record(kind editKind) {
	if kind == editOther || m.last != kind {
		m.undos = append(m.undos, m.snapshot())
		if len(m.undos) > maxUndo {
			m.undos = m.undos[1:]
		}
	}
	m.redos = nil
	m.last = kind
}

// insert replaces the selection with text and reports whether anything
// changed.
func (m *editModel) insert(text string) bool {
	s, e := m.selection()
	if text == "" && s == e {
		return false
	}
	// A run of typing merges; a paste or a replaced selection does not.
	kind := editOther
	if s == e && utf8.RuneCountInString(text) == 1 && !strings.HasPrefix(text, "\n") {
		kind = editTyping
	}
	m.record(kind)
	m.text = m.text[:s] + text + m.text[e:]
	m.anchor, m.caret = s+len(text), s+len(text)
	return true
}

// delete deletes the selection, or unit before (forward false) or after
// the caret, and reports whether it deleted anything.
func (m *editModel) delete(unit editUnit, forward bool) bool {
	var s, e int
	switch {
	case m.hasSelection():
		s, e = m.selection()
	case forward:
		s, e = m.caret, m.boundary(m.caret, unit, true)
	default:
		// Backspace takes one code point, as Windows' editors do: an
		// accent typed after its letter goes alone.
		s, e = prevChar(m.text, m.caret), m.caret
		if unit == unitWord {
			s = wordLeft(m.text, m.caret)
		}
	}
	if s == e {
		return false
	}
	kind := editOther
	if unit == unitChar && m.anchor == m.caret {
		kind = editDeleting
	}
	m.record(kind)
	m.text = m.text[:s] + m.text[e:]
	m.anchor, m.caret = s, s
	return true
}

func (m *editModel) canUndo() bool { return len(m.undos) > 0 }

// undo undoes the last edit and reports whether there was one.
func (m *editModel) undo() bool {
	n := len(m.undos)
	if n == 0 {
		return false
	}
	m.redos = append(m.redos, m.snapshot())
	m.restore(m.undos[n-1])
	m.undos = m.undos[:n-1]
	return true
}

// redo redoes the last undone edit and reports whether there was one.
func (m *editModel) redo() bool {
	n := len(m.redos)
	if n == 0 {
		return false
	}
	m.undos = append(m.undos, m.snapshot())
	m.restore(m.redos[n-1])
	m.redos = m.redos[:n-1]
	return true
}

func (m *editModel) restore(s snapshot) {
	m.text, m.anchor, m.caret = s.text, s.anchor, s.caret
	m.last = editNone
}

// wordAt returns the word, run of spaces or punctuation at at, for
// double clicks.
func (m *editModel) wordAt(at int) (int, int) { return wordAt(m.text, at) }

// display returns the text shown: the text with the composition at the
// caret.
func (m *editModel) display() string {
	if m.composing() {
		return m.text[:m.caret] + m.composition.text + m.text[m.caret:]
	}
	return m.text
}

// toDisplay returns the UTF-16 offset in the shown text (masked: one
// bullet per character) of text offset at.
func (m *editModel) toDisplay(at int, masked bool) uint32 {
	units := func(s string) int {
		if masked {
			return utf8.RuneCountInString(s)
		}
		return utf16Len(s)
	}
	u := units(m.text[:min(at, len(m.text))])
	if m.composition != nil && at > m.caret {
		u += units(m.composition.text)
	}
	return uint32(u)
}

// textAt returns the text offset of UTF-16 offset u in the shown text;
// offsets in the composition go to the caret.
func (m *editModel) textAt(u uint32, masked bool) int {
	at := int(u)
	if m.composing() {
		start := int(m.toDisplay(m.caret, masked))
		n := utf16Len(m.composition.text)
		if masked {
			n = utf8.RuneCountInString(m.composition.text)
		}
		if at >= start+n {
			at -= n
		} else if at > start {
			return m.caret
		}
	}
	if masked {
		i := 0
		for b := range m.text {
			if i == at {
				return b
			}
			i++
		}
		return len(m.text)
	}
	return utf16ToByte(m.text, uint32(at))
}

// displayCaret returns where the caret shows in the shown text: inside
// the composition while there is one.
func (m *editModel) displayCaret(masked bool) uint32 {
	base := m.toDisplay(m.caret, masked)
	if c := m.composition; c != nil {
		base += uint32(utf16Len(c.text[:min(c.cursor, len(c.text))]))
	}
	return base
}

// floorChar returns the largest character boundary of s at or before at.
func floorChar(s string, at int) int {
	at = min(max(at, 0), len(s))
	for at > 0 && at < len(s) && !utf8.RuneStart(s[at]) {
		at--
	}
	return at
}

// prevChar returns the start of the character before at.
func prevChar(s string, at int) int {
	if at <= 0 {
		return 0
	}
	_, n := utf8.DecodeLastRuneInString(s[:at])
	return at - n
}

// extends reports whether c joins the character before it into one
// cluster: marks, variation selectors, emoji modifiers and tags, and what
// follows a zero-width joiner.
func extends(c rune) bool {
	switch {
	case c >= 0x0300 && c <= 0x036F, c >= 0x0483 && c <= 0x0489, c >= 0x0591 && c <= 0x05BD,
		c >= 0x0610 && c <= 0x061A, c >= 0x064B && c <= 0x065F,
		c == 0x0E31, c >= 0x0E34 && c <= 0x0E3A, c >= 0x0E47 && c <= 0x0E4E,
		c >= 0x1AB0 && c <= 0x1AFF, c >= 0x1DC0 && c <= 0x1DFF, c >= 0x200C && c <= 0x200D,
		c >= 0x20D0 && c <= 0x20FF, c >= 0x3099 && c <= 0x309A, c >= 0xFE00 && c <= 0xFE0F,
		c >= 0xFE20 && c <= 0xFE2F, c >= 0x1F3FB && c <= 0x1F3FF, c >= 0xE0020 && c <= 0xE007F,
		c >= 0xE0100 && c <= 0xE01EF:
		return true
	}
	return false
}

func regional(c rune) bool { return c >= 0x1F1E6 && c <= 0x1F1FF }

// nextCluster returns the end of the cluster starting at at.
func nextCluster(s string, at int) int {
	first, n := utf8.DecodeRuneInString(s[at:])
	if n == 0 {
		return len(s)
	}
	prev := first
	// Two regional indicators make one flag.
	flag := regional(first)
	for i := at + n; i < len(s); {
		c, size := utf8.DecodeRuneInString(s[i:])
		if prev != 0x200D && !extends(c) && !(flag && regional(c)) {
			return i
		}
		if regional(c) {
			flag = false
		}
		prev = c
		i += size
	}
	return len(s)
}

// prevCluster returns the start of the cluster ending at at.
func prevCluster(s string, at int) int {
	// Clusters are found forward from the line's start: flags pair up from
	// there.
	start := strings.LastIndexByte(s[:at], '\n') + 1
	if at == start {
		return prevChar(s, at)
	}
	pos := start
	for {
		next := nextCluster(s, pos)
		if next >= at {
			return pos
		}
		pos = next
	}
}

// charClass is what a character counts as, for word boundaries.
type charClass uint8

const (
	classSpace charClass = iota
	classWord
	classIdeograph
	classPunct
)

func classOf(c rune) charClass {
	switch {
	case unicode.IsSpace(c):
		return classSpace
	case ideograph(c):
		return classIdeograph
	case unicode.IsLetter(c) || unicode.IsNumber(c) || c == '_' || c == '\'' || extends(c):
		return classWord
	}
	return classPunct
}

// ideograph reports CJK ideographs, kana and Hangul: a run of them is one
// word, as Windows' editors have no dictionary either.
func ideograph(c rune) bool {
	return c >= 0x3040 && c <= 0x30FF || c >= 0x3400 && c <= 0x4DBF || c >= 0x4E00 && c <= 0x9FFF ||
		c >= 0xAC00 && c <= 0xD7AF || c >= 0xF900 && c <= 0xFAFF || c >= 0x20000 && c <= 0x3134F
}

// wordRight is Control+Right: past the word at at and the spaces after it.
func wordRight(s string, at int) int {
	if at >= len(s) {
		return len(s)
	}
	c, _ := utf8.DecodeRuneInString(s[at:])
	first := classOf(c)
	i := at
	if first != classSpace {
		for i < len(s) {
			c, n := utf8.DecodeRuneInString(s[i:])
			if classOf(c) != first {
				break
			}
			i += n
		}
	}
	for i < len(s) {
		c, n := utf8.DecodeRuneInString(s[i:])
		if c == '\n' && first != classSpace || classOf(c) != classSpace {
			return i
		}
		i += n
	}
	return len(s)
}

// wordLeft is Control+Left: back over spaces, then to the start of the
// word before.
func wordLeft(s string, at int) int {
	i := at
	for i > 0 {
		c, n := utf8.DecodeLastRuneInString(s[:i])
		if classOf(c) != classSpace {
			break
		}
		i -= n
	}
	if i == 0 {
		return 0
	}
	c, _ := utf8.DecodeLastRuneInString(s[:i])
	kind := classOf(c)
	for i > 0 {
		c, n := utf8.DecodeLastRuneInString(s[:i])
		if classOf(c) != kind {
			break
		}
		i -= n
	}
	return i
}

// wordAt returns the run of one class of characters around at, for a
// double click.
func wordAt(s string, at int) (int, int) {
	at = floorChar(s, at)
	// The character after the point, or the one before it at the end.
	probe := at
	if at >= len(s) {
		probe = prevChar(s, at)
	}
	if probe >= len(s) {
		return at, at
	}
	c, _ := utf8.DecodeRuneInString(s[probe:])
	if c == '\n' {
		return probe, probe
	}
	kind := classOf(c)
	start := probe
	for start > 0 {
		c, n := utf8.DecodeLastRuneInString(s[:start])
		if classOf(c) != kind || c == '\n' {
			break
		}
		start -= n
	}
	end := probe
	for end < len(s) {
		c, n := utf8.DecodeRuneInString(s[end:])
		if classOf(c) != kind || c == '\n' {
			break
		}
		end += n
	}
	return start, end
}

// utf16Len returns the length of s in UTF-16 units.
func utf16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// utf16ToByte returns the byte offset of UTF-16 offset units of s: an
// offset inside a surrogate pair moves past the pair; one past the end
// clamps.
func utf16ToByte(s string, units uint32) int {
	at := 0
	for i, r := range s {
		if at >= int(units) {
			return i
		}
		at += utf16.RuneLen(r)
	}
	return len(s)
}

// pasted returns pasted text for a single-line field: Windows' line
// breaks become spaces, and trailing ones go.
func pasted(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	return strings.ReplaceAll(strings.TrimRight(text, "\n"), "\n", " ")
}
