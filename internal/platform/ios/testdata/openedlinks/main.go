// Command openedlinks is an owned full-app URL-invocation fixture. Its controller reads only this app's private proof file on an owned simulator.
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/mobile"
)

func main() { nsgo.Run(Root) }

type proof struct {
	PID         int
	Started     int64
	Initial     string
	URLs        []string
	Links       []string
	HandlerURLs []string
}

func Root(cx *core.Scope) core.Node {
	p := proof{PID: os.Getpid(), Started: time.Now().UnixNano(), Initial: mobile.OpenedURL(cx)}
	save := func() {
		data, _ := json.Marshal(p)
		path := filepath.Join(os.TempDir(), "nsgo1474-invocations.json")
		if os.WriteFile(path+".tmp", data, 0o600) == nil {
			_ = os.Rename(path+".tmp", path)
		}
	}
	cx.Effect(func() { p.URLs = append(p.URLs, mobile.OpenedURL(cx)); save() })
	mobile.OnOpenLinks(cx, func(links []string) {
		for _, link := range links {
			p.Links = append(p.Links, link)
			p.HandlerURLs = append(p.HandlerURLs, mobile.OpenedURL(cx))
		}
		save()
	})
	return core.Text{TestID: "invocation", ContentFunc: func() string { return mobile.OpenedURL(cx) }}
}
