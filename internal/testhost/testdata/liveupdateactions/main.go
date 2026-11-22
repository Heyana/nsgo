// Command liveupdateactions verifies actual notification-shade presses on Android.
package main

import (
	"fmt"
	"log"
	"os"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
	"github.com/nsgo-dev/nsgo/widget"
)

var presses atomic.Int32

func content() widget.ActivityContent {
	return widget.ActivityContent{Title: "NSGo Actions 1465", Body: fmt.Sprintf("Actions received: %d", presses.Load()),
		ShowProgress: true, Progress: .4, Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause 1465"}}}
}

func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		n := presses.Add(1)
		log.Printf("ACTION1465 pid=%d count=%d action=%s kind=%s", os.Getpid(), n, p.Action, p.Activity.Kind())
		if err := p.Activity.Update(content()); err != nil {
			log.Printf("ACTION1465 update failed: %v", err)
		}
	})
	nsgo.Run(func(cx *core.Scope) core.Node {
		log.Printf("ROOT1465 pid=%d", os.Getpid())
		return core.View{Style: core.Style{Padding: core.All(48)}, Children: []core.Node{ui.Button{Text: "Start owned activity", OnPress: func() {
			cx.RequestNotificationPermission(func(p core.NotificationPermission) {
				if p != core.NotificationsGranted {
					log.Printf("START1465 permission=%v", p)
					return
				}
				a, err := widget.StartActivity("actions1465", content())
				if err != nil {
					log.Printf("START1465 error=%v", err)
					return
				}
				log.Printf("START1465 pid=%d id=%s", os.Getpid(), a.ID())
			})
		}}}}
	})
}
