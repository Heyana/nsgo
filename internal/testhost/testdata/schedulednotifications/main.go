package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

func main() {
	phase := flag.String("phase", os.Getenv("NSGO_NOTIFICATION_PHASE"), "prepare, schedule, inspect or clear")
	flag.Parse()
	nsgo.Run(func(cx *core.Scope) core.Node {
		write := func(name, text string) {
			if err := os.MkdirAll(cx.DataDir(), 0700); err != nil {
				log.Print(err)
				return
			}
			if err := os.WriteFile(filepath.Join(cx.DataDir(), name+".txt"), []byte(text), 0600); err != nil {
				log.Print(err)
			}
		}
		cx.OnNotificationPress(func(id string) { write("press", id+"\n"+time.Now().Format(time.RFC3339Nano)) })
		schedule := func() {
			cx.RequestNotificationPermission(func(p core.NotificationPermission) {
				if p != core.NotificationsGranted {
					write("schedule", "permission: "+p.String())
					return
				}
				cx.CloseNotifications()
				now := time.Now()
				accepted := 0
				done := func(ok bool) {
					if !ok {
						write("schedule", "FAIL: not accepted")
						return
					}
					accepted++
					if accepted == 4 {
						write("schedule", fmt.Sprintf("PASS: accepted 4 requests at %s\ndeliver %s\nmove %s\n", now.Format(time.RFC3339Nano), now.Add(time.Minute).Format(time.RFC3339Nano), now.Add(90*time.Second).Format(time.RFC3339Nano)))
					}
				}
				cx.ShowNotification(core.Notification{ID: "deliver1356", Title: "NSGo 1356 delivery", Body: "Task fixture: one minute", At: now.Add(time.Minute), Silent: true}, done)
				cx.ShowNotification(core.Notification{ID: "move1356", Title: "NSGo 1356 old time", At: now.Add(time.Minute), Silent: true}, func(ok bool) {
					done(ok)
					if ok {
						cx.ShowNotification(core.Notification{ID: "move1356", Title: "NSGo 1356 moved", Body: "Task fixture: ninety seconds", At: now.Add(90 * time.Second), Silent: true}, done)
					}
				})
				cx.ShowNotification(core.Notification{ID: "cancel1356", Title: "NSGo 1356 cancelled", At: now.Add(time.Minute), Silent: true}, func(ok bool) { cx.CloseNotification("cancel1356"); done(ok) })
			})
		}
		switch *phase {
		case "schedule":
			schedule()
		case "inspect":
			go func() { write("inspect", inspect()) }()
		case "clear":
			cx.CloseNotifications()
			write("clear", "cleared this fixture's notifications")
		}
		return core.View{Style: core.Style{Padding: core.All(24), Gap: 16}, Children: []core.Node{
			core.Text{Content: "NSGo 1356 scheduled notification fixture"},
			ui.Button{Text: "Schedule owned notifications", OnPress: schedule},
			ui.Button{Text: "Clear owned notifications", OnPress: func() { cx.CloseNotifications() }},
		}}
	})
}
