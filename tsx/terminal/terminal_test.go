package terminal

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/tsx"
)

func TestRenderingProps(t *testing.T) {
	declaration := tsx.Declarations()["types/nsgo/base.d.ts"]
	for _, want := range []string{"cursorBlink?: boolean", "minimumContrast?: number", "paddingColor?:", "\"background\"", "\"extend\"", "\"extend-always\""} {
		if !strings.Contains(declaration, want) {
			t.Fatalf("terminal declaration is missing %s", want)
		}
	}
}
