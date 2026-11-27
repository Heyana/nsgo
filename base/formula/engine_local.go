//go:build !(js && wasm) || nsgoharmony || nsgominiprogram

package formula

func loadMathEngine(fonts Fonts) (mathEngine, error) {
	return loadLocalEngine(fonts)
}
