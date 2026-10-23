package core

import (
	"strings"
	"unicode/utf16"

	"github.com/oneo-me/nsgo/core/internal/layout"
)

// TextRange is a selection in a text field, in UTF-16 units as the
// platforms' text APIs count; Start equals End for a caret.
type TextRange struct {
	Start, End int
}

// Composition is an input method's composition in a text field.
type Composition struct {
	// Composing is false once the composition ended.
	Composing bool
	// Text is what is being composed; empty once it ended.
	Text string
}

// SubmitBehavior is what Return does in a text field.
type SubmitBehavior uint8

const (
	// SubmitBehaviorDefault inserts a line break in a multiline field,
	// and blurs and submits in a single line.
	SubmitBehaviorDefault SubmitBehavior = iota
	// SubmitBehaviorSubmit reports OnSubmit and keeps the focus.
	SubmitBehaviorSubmit
	// SubmitBehaviorBlurAndSubmit reports OnSubmit and takes the focus
	// away.
	SubmitBehaviorBlurAndSubmit
	// SubmitBehaviorNewline inserts a line break in a multiline field; a
	// single line blurs and submits.
	SubmitBehaviorNewline
)

// Submit is what Return does, as SubmitBehavior and Multiline say
// together, as NSH's InputConfig::submit: never SubmitBehaviorDefault,
// and SubmitBehaviorNewline only when multiline. Platforms read only
// this. A multiline field that submits still takes a line break from
// Shift, Option or Alt with Return on hardware keyboards, and a Return
// that commits an input method's composition never submits.
func (c *InputConfig) Submit() SubmitBehavior {
	switch b := c.SubmitBehavior; {
	case b == SubmitBehaviorNewline && !c.Multiline:
		return SubmitBehaviorBlurAndSubmit
	case b != SubmitBehaviorDefault:
		return b
	case c.Multiline:
		return SubmitBehaviorNewline
	}
	return SubmitBehaviorBlurAndSubmit
}

// ReturnKeyType labels the software keyboard's return key.
type ReturnKeyType uint8

const (
	ReturnKeyDefault ReturnKeyType = iota
	ReturnKeyDone
	ReturnKeyGo
	ReturnKeyNext
	ReturnKeySearch
	ReturnKeySend
)

// AutoCapitalize is a field's automatic capitalization; sentences are
// the default.
type AutoCapitalize uint8

const (
	AutoCapitalizeSentences AutoCapitalize = iota
	AutoCapitalizeNone
	AutoCapitalizeWords
	AutoCapitalizeCharacters
)

// KeyboardAppearance is the look of iOS's keyboard; the default follows
// the app's appearance.
type KeyboardAppearance uint8

const (
	KeyboardAppearanceDefault KeyboardAppearance = iota
	KeyboardAppearanceLight
	KeyboardAppearanceDark
)

// AutoComplete says what a field holds, for the system's autofill and
// suggestions: React Native's autoComplete, which iOS maps to its
// textContentType.
type AutoComplete uint8

const (
	// AutoCompleteDefault lets the platform decide.
	AutoCompleteDefault AutoComplete = iota
	AutoCompleteOff
	AutoCompleteUsername
	// AutoCompletePassword is an existing password.
	AutoCompletePassword
	// AutoCompleteNewPassword is a password being chosen.
	AutoCompleteNewPassword
	// AutoCompleteOneTimeCode is a code from a text message or e-mail.
	AutoCompleteOneTimeCode
	AutoCompleteEmail
	AutoCompleteTel
	AutoCompleteName
	AutoCompleteGivenName
	AutoCompleteFamilyName
	AutoCompleteMiddleName
	AutoCompleteNickname
	AutoCompleteOrganization
	// AutoCompleteStreetAddress is every line of a street address.
	AutoCompleteStreetAddress
	AutoCompleteAddressLine1
	AutoCompleteAddressLine2
	AutoCompleteCity
	// AutoCompleteRegion is a state or region.
	AutoCompleteRegion
	AutoCompletePostalCode
	AutoCompleteCountry
	AutoCompleteCreditCardNumber
	AutoCompleteCreditCardExpiration
	AutoCompleteCreditCardSecurityCode
	AutoCompleteCreditCardName
	AutoCompleteURL
	AutoCompleteBirthdate
)

var autoCompleteNames = [...]string{"", "off", "username", "current-password", "new-password", "one-time-code",
	"email", "tel", "name", "given-name", "family-name", "additional-name", "nickname", "organization",
	"street-address", "address-line1", "address-line2", "address-level2", "address-level1", "postal-code",
	"country-name", "cc-number", "cc-exp", "cc-csc", "cc-name", "url", "bday"}

// String returns the HTML autocomplete token, empty for the default.
func (a AutoComplete) String() string {
	if int(a) < len(autoCompleteNames) {
		return autoCompleteNames[a]
	}
	return ""
}

// SetSelection selects start to end, in UTF-16 units, in the element, a
// text field; equal offsets place the caret.
func (r *Ref) SetSelection(start, end int) {
	if r.n == nil || r.n.deleted || r.n.input == nil {
		return
	}
	r.t.commands = append(r.t.commands, MountItem{Kind: ItemSelection, ID: r.n.id, Selection: TextRange{start, end}})
	r.t.changed = true
}

// UTF16Len is the length of s in UTF-16 units.
func UTF16Len(s string) int {
	n := 0
	for _, r := range s {
		n += utf16.RuneLen(r)
	}
	return n
}

// ClipUTF16 cuts s to at most max UTF-16 units, never inside a
// character; max 0 or less keeps all of it.
func ClipUTF16(s string, max int) string {
	if max <= 0 {
		return s
	}
	n := 0
	for i, r := range s {
		if n += utf16.RuneLen(r); n > max {
			return s[:i]
		}
	}
	return s
}

// UTF16Offset converts a UTF-16 offset in s to a byte offset, clamped
// to s.
func UTF16Offset(s string, units int) int {
	n := 0
	for i, r := range s {
		if n >= units {
			return i
		}
		n += utf16.RuneLen(r)
	}
	return len(s)
}

// UTF16Units converts a byte offset in s to UTF-16 units.
func UTF16Units(s string, bytes int) int {
	return UTF16Len(s[:min(max(bytes, 0), len(s))])
}

// LimitEdit is what a field with maxLength keeps of an edit replacing
// replaced units of text with insert: insert cut to the room left, as
// the platforms' own limits do. ok is false when nothing of a non-empty
// insert fits.
func LimitEdit(text string, replaced int, insert string, maxLength int) (kept string, ok bool) {
	if maxLength <= 0 {
		return insert, true
	}
	room := maxLength - (UTF16Len(text) - replaced)
	kept = ClipUTF16(insert, max(room, 0))
	if room <= 0 {
		kept = ""
	}
	return kept, kept != "" || insert == ""
}

// KeyOf is what EventKeyPress reports for an edit inserting text: the
// text, or "Enter" for a line break.
func KeyOf(text string) string {
	if text == "\n" || text == "\r" || text == "\r\n" {
		return "Enter"
	}
	return text
}

// defaultInputWidth is a field's width when its style sets none.
const defaultInputWidth Points = 160

func (p *preparedInput) set(e TextEngine, at AttributedText) bool {
	if p.prepared != nil && at.equal(&p.text) {
		return false
	}
	p.release()
	p.text = at
	p.prepared = e.Prepare(&p.text)
	return true
}

func (p *preparedInput) release() {
	if p.prepared != nil {
		p.prepared.Release()
		p.prepared = nil
	}
}

// inputText is s as field c lays it out. A multiline field's trailing
// line break still takes a line.
func inputText(s string, c *InputConfig) AttributedText {
	at := AttributedText{Text: s, Style: c.Style, MaxLines: 1}
	if c.Multiline {
		at.MaxLines = 0
		if strings.HasSuffix(s, "\n") {
			at.Text += "​"
		}
	}
	return at
}

// prepareInput prepares what measures field n: a multiline field's value
// or placeholder, and the value whose size OnContentSizeChange reports.
func (t *Tree) prepareInput(n *node) {
	in := n.input
	c := &in.config
	if c.Multiline {
		s := in.value
		if s == "" {
			s = c.Placeholder
		}
		if in.measured.set(t.text, inputText(s, c)) {
			n.markLayout()
		}
	} else if in.measured.prepared != nil {
		in.measured.release()
		n.markLayout()
	}
	if in.contentSize != nil && in.value != "" {
		in.sized.set(t.text, inputText(in.value, c))
	} else {
		in.sized.release()
	}
}

// measure is a field's content size: a line at the default width, or a
// multiline field's text wrapped at its width.
func (in *inputNode) measure(known layout.Size[layout.Opt], available layout.Size[layout.AvailableSpace]) Size {
	m := Size{defaultInputWidth, ceil(in.lineH)}
	if in.measured.prepared == nil {
		return m
	}
	width := defaultInputWidth
	if available.Width.Kind == layout.SpaceDefinite {
		width = Points(available.Width.Value)
	} else if known.Width.Valid {
		width = Points(known.Width.Value)
	}
	m.Height = max(m.Height, ceil(in.measured.prepared.Measure(max(width, 0)).Height))
	return m
}

// reportContentSize queues OnContentSizeChange when the size of field n's
// value changed: wrapped at its content width when multiline, one empty
// line's when it is empty.
func (t *Tree) reportContentSize(n *node) {
	in := n.input
	if in.contentSize == nil {
		return
	}
	size := Size{0, ceil(in.lineH)}
	if in.sized.prepared != nil {
		width := Auto
		if in.config.Multiline {
			width = in.config.Content.Size.Width
		}
		m := in.sized.prepared.Measure(width)
		size = Size{ceil(m.Width), ceil(m.Height)}
	}
	if in.reportedOK && size == in.reported {
		return
	}
	in.reported, in.reportedOK = size, true
	fn := in.contentSize
	t.after(n, func() { fn(size) })
}
