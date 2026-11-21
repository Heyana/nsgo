package ui

import "github.com/nsgo-dev/nsgo/core"

// Option is one choice of Tabs, a Select or a Combobox. Label names it. In Tabs, Icon leads the label, Disabled turns it off, and Content shows in place of both, inheriting the tab's text style, with Label still naming the tab for assistive technology. In a Select, Leading goes before the label, in its menu's row and in the field while chosen, and Trailing at the end of its row, where a button's presses stay its own. In a Select or a Combobox, adjacent options of one non-empty Group show under a header of it, which cannot be chosen or focused; Tabs ignore it.
type Option[T comparable] struct {
	Value     T
	Label     string
	LabelFunc func() string
	Group     string
	Icon      func() *core.SVG
	Disabled  func() bool
	Content   core.Node
	Leading   core.Node
	Trailing  core.Node
	TestID    string
}

func (o Option[T]) label() string { return textValue(o.Label, o.LabelFunc) }
