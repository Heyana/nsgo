// Package formula gives TSX nsgo/base's Formula, base/formula's View, which links Untie and its bundled math font. Apps whose TSX typesets formulas import this package for its effect; TextView takes renderMath={(math) => <Formula source={math.tex} display={math.display} />}.
package formula

import (
	"github.com/nsgo-dev/nsgo/base/formula"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
)

func init() {
	tsx.Register("nsgo/base", map[string]core.Node{"Formula": formula.View{}},
		tsx.Omit("Renderer"), tsx.Enum(map[string]formula.Dialect{"tex": formula.TeX, "typst": formula.Typst}), tsx.SandboxSafe())
}
