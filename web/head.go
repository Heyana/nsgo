// Package web provides the browser document around an NSGo app. Its
// components render nothing and do nothing on native targets.
package web

import "github.com/nsgo-dev/nsgo/core"

// Head sets the document's metadata while its owner is visible. The latest
// mounted Head wins for each field it sets, restoring the earlier one when
// hidden or removed. An embedded app leaves its host's document alone.
// An empty static field inherits; a function returning empty clears it.
type Head struct {
	Title           string
	TitleFunc       func() string
	Description     string
	DescriptionFunc func() string
}

// Render implements core.Node.
func (h Head) Render(cx *core.Scope) core.Node {
	field := func(value string, fn func() string) func() string {
		if fn != nil || value == "" {
			return fn
		}
		return func() string { return value }
	}
	cx.BindWebHead(field(h.Title, h.TitleFunc), field(h.Description, h.DescriptionFunc))
	return nil
}
