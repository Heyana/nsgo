package formula

import (
	"context"

	"github.com/caelyreth/untie"
	"github.com/caelyreth/untie/mathml"

	"github.com/nsgo-dev/nsgo/core"
)

const (
	// MIMETeX is the media type of a TeX formula's source on the clipboard.
	MIMETeX = "text/x-tex"
	// MIMETypst is the media type of a Typst formula's source on the clipboard.
	MIMETypst = "text/x-typst"
	// MIMEMathML is the media type of Presentation MathML on the clipboard.
	MIMEMathML = "application/mathml+xml"
)

func untieDialectMode(dialect Dialect, display bool) (untie.Dialect, untie.Mode) {
	d, mode := untie.TeX, untie.Inline
	if dialect == Typst {
		d = untie.Typst
	}
	if display {
		mode = untie.Display
	}
	return d, mode
}

func dialectMIME(dialect Dialect) string {
	if dialect == Typst {
		return MIMETypst
	}
	return MIMETeX
}

// MathML returns deterministic Presentation MathML for source in dialect and
// display mode, produced through Untie's canonical IR and mathml.Marshal.
func MathML(source string, dialect Dialect, display bool) (string, error) {
	d, mode := untieDialectMode(dialect, display)
	expr := untie.Parse(source, d, mode, untie.ParseOptions{})
	root, err := expr.RootSnapshot()
	if err != nil {
		return "", err
	}
	data, err := mathml.Marshal(context.Background(), root, mode, mathml.Options{})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ClipboardItem builds one clipboard item carrying source as plain text,
// Presentation MathML (application/mathml+xml) when serialization succeeds,
// and the formula's dialect source (text/x-tex for TeX, text/x-typst for Typst).
func ClipboardItem(source string, dialect Dialect, display bool) core.ClipboardItem {
	reps := make([]core.ClipboardRepresentation, 0, 3)
	reps = append(reps, core.ClipboardRepresentation{Type: "text/plain", Data: []byte(source)})
	if ml, err := MathML(source, dialect, display); err == nil && ml != "" {
		reps = append(reps, core.ClipboardRepresentation{Type: MIMEMathML, Data: []byte(ml)})
	}
	reps = append(reps, core.ClipboardRepresentation{Type: dialectMIME(dialect), Data: []byte(source)})
	return core.ClipboardItem{Representations: reps}
}

// Copy puts a formula on the system clipboard with its plain text, Presentation
// MathML (application/mathml+xml) and dialect source (text/x-tex or text/x-typst)
// through WriteClipboardItems, falling back to plain text via SetClipboard on
// platforms without representation access.
func Copy(cx *core.Scope, source string, dialect Dialect, display bool) {
	item := ClipboardItem(source, dialect, display)
	cx.WriteClipboardItems([]core.ClipboardItem{item}, func(err error) {
		if err != nil {
			cx.SetClipboard(source)
		}
	})
}
