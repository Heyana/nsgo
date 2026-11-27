package miniprogram

import (
	"encoding/base64"
	"errors"
	"strconv"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/platform/fontfile"
)

// fontFaceSpec is the normalized descriptor passed to loadFontFace in the
// mini program's logic layer.
type fontFaceSpec struct {
	Family string
	Source string
	Style  string
	Weight string
}

// buildFontFaceSpec reads the family, weight and slant of font file data and
// builds a data-URI source acceptable to WeChat, QQ, Alipay and Douyin's
// loadFontFace APIs.
func buildFontFaceSpec(data []byte) (fontFaceSpec, error) {
	faces, err := fontfile.Describe(data)
	if err != nil {
		return fontFaceSpec{}, err
	}
	if len(faces) == 0 {
		return fontFaceSpec{}, errors.New("miniprogram: font has no faces")
	}
	f := faces[0]
	style := "normal"
	if f.Italic {
		style = "italic"
	}
	mime := fontMIME(data)
	src := `url("data:` + mime + `;base64,` + base64.StdEncoding.EncodeToString(data) + `")`
	return fontFaceSpec{
		Family: f.Family,
		Source: src,
		Style:  style,
		Weight: strconv.Itoa(f.Weight),
	}, nil
}

// fontMIME returns the MIME type of an OpenType, TrueType, collection or WOFF
// font file from its signature bytes.
func fontMIME(data []byte) string {
	if len(data) >= 4 {
		switch string(data[:4]) {
		case "OTTO":
			return "font/otf"
		case "ttcf":
			return "font/collection"
		case "wOFF":
			return "font/woff"
		case "wOF2":
			return "font/woff2"
		}
	}
	return "font/ttf"
}

// resourceFileCandidates returns the package file paths a mini program's
// FileSystemManager may read for a resource path under resources/.
func resourceFileCandidates(p string) []string {
	p = strings.TrimPrefix(p, "asset:")
	p = strings.TrimPrefix(p, "/")
	p = strings.TrimPrefix(p, "resources/")
	if p == "" {
		return nil
	}
	return []string{"resources/" + p, "/resources/" + p}
}
