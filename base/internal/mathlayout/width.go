// Package mathlayout carries a display formula's viewport across its text attachment.
package mathlayout

import "github.com/nsgo-dev/nsgo/core"

// Width is scoped to one display block, so siblings keep separate constraints.
var Width = core.NewKey[func() core.Points]("display math viewport width", nil)
