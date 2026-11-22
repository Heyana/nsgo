// Package diagramlayout carries a diagram block's viewport width across its text attachment, which otherwise lays out in unbounded space.
package diagramlayout

import "github.com/nsgo-dev/nsgo/core"

// Width is the nearest diagram block's measured viewport width, nil outside a text view. Each block provides its own signal so nested and sibling blocks keep their own constraints.
var Width = core.NewKey[func() core.Points]("diagram viewport width", nil)
