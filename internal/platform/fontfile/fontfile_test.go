package fontfile

import (
	"testing"

	"golang.org/x/image/font/gofont/gobolditalic"
	"golang.org/x/image/font/gofont/gosmallcaps"
)

func TestDescribeReadsFamilyWeightAndSlant(t *testing.T) {
	for _, c := range []struct {
		data   []byte
		family string
		weight int
		italic bool
	}{
		{gosmallcaps.TTF, "Go Smallcaps", 400, false},
		{gobolditalic.TTF, "Go", 600, true}, // the Go fonts declare bold as 600
	} {
		faces, err := Describe(c.data)
		if err != nil || len(faces) != 1 {
			t.Fatalf("faces %v, %v", faces, err)
		}
		if f := faces[0]; f.Family != c.family || f.Weight != c.weight || f.Italic != c.italic {
			t.Errorf("face = %+v, want %s %d %v", f, c.family, c.weight, c.italic)
		}
	}
	if _, err := Describe([]byte("not a font, not at all")); err == nil {
		t.Error("garbage described")
	}
}
