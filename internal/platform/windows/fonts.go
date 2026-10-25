//go:build windows

package windows

import (
	"log"
	"unsafe"

	"github.com/oneo-me/nsgo/internal/platform/imageload"
)

// An app's font files join DirectWrite's system fonts in a collection of
// NSGo's own, which every text format looks families up in: an in-memory
// font file loader holds the files' bytes, and a font set builder makes
// one set of the system's fonts and those files. It needs IDWriteFactory5,
// Windows 10 1703 or later; earlier Windows shows the system's fonts.

var iidDWriteFactory5 = parseGUID("958db99a-be2a-4f09-af7d-65189803d1d3")

const (
	dwRegisterFontFileLoader        = 13
	dw3GetSystemFontSet             = 35
	dw3CreateFontCollectionFromSet  = 37
	dw5CreateFontSetBuilder         = 43
	dw5CreateInMemoryFontFileLoader = 44
	memoryLoaderCreateReference     = 4
	fontSetBuilderAddFontSet        = 5
	fontSetBuilderCreateFontSet     = 6
	fontSetBuilder1AddFontFile      = 7
)

// appFonts is the system's fonts and the app's, nil without app fonts or
// when DirectWrite cannot make the collection.
var appFonts *comObject

// registerFonts makes appFonts of the app's font files, paths under its
// resources. Call it before the first text.
func registerFonts(paths []string) {
	if len(paths) == 0 {
		return
	}
	f := dwriteFactory()
	if f == nil {
		return
	}
	f5 := f.query(&iidDWriteFactory5)
	if f5 == nil {
		log.Printf("nsgo: fonts need Windows 10 1703 or later")
		return
	}
	defer f5.release()
	var loader, builder, system, set *comObject
	if failed(f5.call(dw5CreateInMemoryFontFileLoader, uintptr(unsafe.Pointer(&loader)))) {
		return
	}
	// DirectWrite keeps the registered loader, which keeps the bytes.
	f5.call(dwRegisterFontFileLoader, loader.ptr())
	if failed(f5.call(dw5CreateFontSetBuilder, uintptr(unsafe.Pointer(&builder)))) {
		return
	}
	defer builder.release()
	if !failed(f5.call(dw3GetSystemFontSet, uintptr(unsafe.Pointer(&system)))) {
		builder.call(fontSetBuilderAddFontSet, system.ptr())
		system.release()
	}
	for _, p := range paths {
		data, err := imageload.ReadResource(p)
		if err != nil || len(data) == 0 {
			log.Printf("nsgo: font %s: %v", p, err)
			continue
		}
		var file *comObject
		// With no owner object, DirectWrite copies the bytes.
		if failed(loader.call(memoryLoaderCreateReference, f5.ptr(), uintptr(unsafe.Pointer(&data[0])),
			uintptr(len(data)), 0, uintptr(unsafe.Pointer(&file)))) {
			log.Printf("nsgo: font %s: DirectWrite cannot read it", p)
			continue
		}
		if failed(builder.call(fontSetBuilder1AddFontFile, file.ptr())) {
			log.Printf("nsgo: font %s: DirectWrite cannot read it", p)
		}
		file.release()
	}
	if failed(builder.call(fontSetBuilderCreateFontSet, uintptr(unsafe.Pointer(&set)))) {
		return
	}
	defer set.release()
	var c *comObject
	if !failed(f5.call(dw3CreateFontCollectionFromSet, set.ptr(), uintptr(unsafe.Pointer(&c)))) {
		appFonts = c
	}
}
