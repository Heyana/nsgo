// Command openedlinks records actual Android invocation and browser addresses.
package main

import (
	"log"
	"os"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
)

func main() {
	nsgo.Run(func(cx *core.Scope) core.Node {
		log.Printf("FIRST1475 pid=%d url=%q", os.Getpid(), mobile.OpenedURL(cx))
		mobile.OnOpenLinks(cx, func(links []string) {
			log.Printf("OPEN1475 pid=%d links=%q current=%q", os.Getpid(), links, mobile.OpenedURL(cx))
		})
		cx.Effect(func() { log.Printf("CHANGE1475 pid=%d url=%q", os.Getpid(), mobile.OpenedURL(cx)) })
		return core.View{Style: core.Style{Padding: core.All(48)}, Children: []core.Node{
			core.Text{Content: "Opened link"},
			core.Text{TestID: "opened", ContentFunc: func() string { return mobile.OpenedURL(cx) }},
		}}
	})
}
