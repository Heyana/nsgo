//go:build linux && !android

package linux

import (
	"log"
	"path/filepath"

	"github.com/ebitengine/purego"

	"github.com/oneo-me/nsgo/internal/platform/imageload"
)

// registerFonts adds the app's font files, paths under its resources, to
// fontconfig's application fonts, which Pango finds by their families.
// It runs before GTK and the text engine make their font maps.
func registerFonts(paths []string) {
	if len(paths) == 0 {
		return
	}
	lib, err := purego.Dlopen("libfontconfig.so.1", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		log.Printf("nsgo: fonts: %v", err)
		return
	}
	var addFile func(config uintptr, file *byte) int32
	purego.RegisterLibFunc(&addFile, lib, "FcConfigAppFontAddFile")
	for _, p := range paths {
		file := filepath.Join(imageload.ResourceRoot, filepath.FromSlash(p))
		if addFile(0, cstr(file)) == 0 {
			log.Printf("nsgo: font %s: fontconfig cannot read %s", p, file)
		}
	}
}
