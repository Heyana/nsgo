// Command features constructs a shader, canvas, video, web view and native component: linked_test.go checks that each backend links their code when an app uses them.
package main

import (
	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
)

func main() {
	prog := core.MustCompileShader(`package shader

func Fragment(pos vec4) vec4 {
	return vec4(1, 0, 0, 1)
}
`)
	nsgo.Run(func(cx *core.Scope) core.Node {
		return core.View{
			Children: []core.Node{
				core.Shader{Program: prog},
				core.Canvas{Draw: func(d *core.Drawing, _ core.Size) {
					d.FillRect(0, 0, 10, 10)
				}},
				core.Video{Source: "https://example.test/clip.mp4"},
				core.WebView{Source: core.WebSource{HTML: "<p>hi</p>"}},
				core.Native{Component: "custom"},
			},
		}
	})
}
