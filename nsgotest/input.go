package nsgotest

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// InputConfig records the appearance and editing behavior of a mounted text field.
type InputConfig struct {
	Style        core.TextStyle
	Placeholder  string
	Masked       bool
	Readonly     bool
	Disabled     bool
	KeyboardType core.KeyboardType
	// Content is where the text goes in the view: inside padding and border.
	Content core.Bounds
	// Padding surrounds Content inside the border. A multi-line editor
	// scrolls in the padding box, its text inset by Padding.
	Padding core.Edges
	// Accessory identifies the view above the keyboard while the field
	// has focus. AccessoryID is its application identifier.
	Accessory   core.NodeID
	AccessoryID string
	// Multiline takes several lines in a multi-line control.
	Multiline bool
	// MaxLength caps input in UTF-16 units. Zero leaves it unlimited.
	MaxLength int
	// SubmitBehavior records the requested Return action. Submit resolves its default.
	SubmitBehavior core.SubmitBehavior
	ReturnKey      core.ReturnKeyType
	AutoCapitalize core.AutoCapitalize
	NoAutoCorrect  bool
	AutoComplete   core.AutoComplete
	// KeyboardAppearance is iOS's.
	KeyboardAppearance core.KeyboardAppearance
	// PlaceholderColor selects the placeholder text color. Zero uses the platform default.
	PlaceholderColor core.Color
	// SelectionColor colors selected text. Caret resolves CaretColor
	// against SelectionColor.
	SelectionColor core.Color
	CaretColor     core.Color
	// AccentColor is the accent of the input's theme, for what the
	// platform draws around it, such as an input method's candidates.
	AccentColor core.Color
	HideCaret   bool
	// PasteBehavior says whether a paste inserts its text.
	PasteBehavior core.PasteBehavior
}

func inputConfig(c driver.InputConfig) InputConfig {
	return InputConfig{
		Style:              c.Style,
		Placeholder:        c.Placeholder,
		Masked:             c.Masked,
		Readonly:           c.Readonly,
		Disabled:           c.Disabled,
		KeyboardType:       c.KeyboardType,
		Content:            c.Content,
		Padding:            c.Padding,
		Accessory:          c.Accessory,
		AccessoryID:        c.AccessoryID,
		Multiline:          c.Multiline,
		MaxLength:          c.MaxLength,
		SubmitBehavior:     c.SubmitBehavior,
		ReturnKey:          c.ReturnKey,
		AutoCapitalize:     c.AutoCapitalize,
		NoAutoCorrect:      c.NoAutoCorrect,
		AutoComplete:       c.AutoComplete,
		KeyboardAppearance: c.KeyboardAppearance,
		PlaceholderColor:   c.PlaceholderColor,
		SelectionColor:     c.SelectionColor,
		CaretColor:         c.CaretColor,
		AccentColor:        c.AccentColor,
		HideCaret:          c.HideCaret,
		PasteBehavior:      c.PasteBehavior,
	}
}

func (c *InputConfig) inputPolicy() *driver.InputConfig {
	return &driver.InputConfig{
		Multiline:      c.Multiline,
		SubmitBehavior: c.SubmitBehavior,
		SelectionColor: c.SelectionColor,
		CaretColor:     c.CaretColor,
	}
}

// Submit reports the effective action of the Return key.
func (c *InputConfig) Submit() core.SubmitBehavior {
	return c.inputPolicy().Submit()
}

// Highlight returns the background color of selected text.
func (c *InputConfig) Highlight() core.Color {
	return c.inputPolicy().Highlight()
}

// Caret returns the effective caret color.
func (c *InputConfig) Caret() core.Color {
	return c.inputPolicy().Caret()
}
