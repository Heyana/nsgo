//go:build js && wasm

package web

import (
	"log"
	"strconv"
	"syscall/js"

	"github.com/oneo-me/nsgo/internal/platform/fontfile"
)

// registerFonts loads the app's font files, paths under its resources,
// into the document's fonts as FontFaces named by their families, read
// from the files, and returns once they loaded or failed: text measures
// with them from the first view.
func registerFonts(paths []string) {
	if len(paths) == 0 {
		return
	}
	g := js.Global()
	done := make(chan struct{}, len(paths))
	var funcs []js.Func
	fn := func(f func(js.Value) any) js.Value {
		jf := js.FuncOf(func(_ js.Value, args []js.Value) any {
			var v js.Value
			if len(args) > 0 {
				v = args[0]
			}
			return f(v)
		})
		funcs = append(funcs, jf)
		return jf.Value
	}
	for _, p := range paths {
		failed := fn(func(err js.Value) any {
			log.Printf("nsgo: font %s: %s", p, err.Call("toString").String())
			done <- struct{}{}
			return nil
		})
		loaded := fn(func(buf js.Value) any {
			data := make([]byte, buf.Get("byteLength").Int())
			js.CopyBytesToGo(data, g.Get("Uint8Array").New(buf))
			faces, err := fontfile.Describe(data)
			if err != nil || len(faces) == 0 {
				log.Printf("nsgo: font %s: %v", p, err)
				done <- struct{}{}
				return nil
			}
			// A collection's faces cannot be told apart by a FontFace:
			// it takes the first.
			f := faces[0]
			style := "normal"
			if f.Italic {
				style = "italic"
			}
			face := g.Get("FontFace").New(f.Family, buf, map[string]any{"weight": strconv.Itoa(f.Weight), "style": style})
			face.Call("load").Call("then", fn(func(face js.Value) any {
				g.Get("document").Get("fonts").Call("add", face)
				done <- struct{}{}
				return nil
			}), failed)
			return nil
		})
		g.Call("fetch", resourceDir()+p).Call("then", fn(func(r js.Value) any {
			if !r.Get("ok").Bool() {
				return g.Get("Promise").Call("reject", g.Get("Error").New("HTTP "+strconv.Itoa(r.Get("status").Int())))
			}
			return r.Call("arrayBuffer")
		})).Call("then", loaded).Call("catch", failed)
	}
	for range paths {
		<-done
	}
	for _, f := range funcs {
		f.Release()
	}
}
