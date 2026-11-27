package driver

import (
	"crypto/sha256"
	"testing"
)

func TestSum256MatchesStdlib(t *testing.T) {
	cases := [][]byte{
		nil,
		{},
		[]byte("abc"),
		[]byte("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"),
		make([]byte, 55),
		make([]byte, 56),
		make([]byte, 63),
		make([]byte, 64),
		make([]byte, 65),
		make([]byte, 1000),
	}
	for i := range cases[len(cases)-1] {
		cases[len(cases)-1][i] = byte(i*31 + 7)
	}
	for i, in := range cases {
		if got, want := Sum256(in), sha256.Sum256(in); got != want {
			t.Errorf("case %d (len %d): got %x, want %x", i, len(in), got, want)
		}
	}
}
