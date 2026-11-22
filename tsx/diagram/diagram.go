// Package diagram gives TSX nsgo/base's Diagram, base/diagram's View,
// which links limu: an app whose TSX uses it imports this package for its
// effect. TextView takes renderDiagram={(code) => <Diagram source={code.code} />}.
package diagram

import (
	"github.com/nsgo-dev/nsgo/base/diagram"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

func init() {
	tsx.Register("nsgo/base", map[string]core.Node{"Diagram": diagram.View{}}, tsx.SandboxSafe())
	// An app can override only the theme fields it needs, as in Go.
	tsx.Optional[diagram.Theme]("Background", "Text", "Muted", "Line", "Surface", "SurfaceAlt", "Border", "Accent", "Critical", "Series", "Radius", "StrokeWidth")
}
