//go:build !darwin && !windows && !android && !linux && !(js && wasm)

package nsgo

func run(app App) {
	panic("nsgo: no backend for this platform yet")
}
