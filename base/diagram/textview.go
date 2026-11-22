package diagram

import (
	"github.com/nsgo-dev/nsgo/base/textview"
	"github.com/nsgo-dev/nsgo/core"
)

// RenderDiagram draws a text view's mermaid code block, as textview.TextView.RenderDiagram takes it, for textview.TextView{RenderDiagram: diagram.RenderDiagram}. It lays the diagram out to the block's viewport width, centered, retaining sideways scrolling for a diagram limu cannot fit. A copy takes its source between ```mermaid fences. A diagram that fails shows its source in the monospaced font.
func RenderDiagram(cx *core.Scope, code textview.CodeBlock) core.Node {
	return View{Source: code.Code, ErrorStyle: core.Style{FontFamily: core.FontMonospace}}
}
