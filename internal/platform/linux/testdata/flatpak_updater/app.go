package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"time"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/imageload"
	"github.com/nsgo-dev/nsgo/internal/platform/updates"
	"github.com/nsgo-dev/nsgo/updater"
)

var requests atomic.Int32
var result = make(chan error, 1)

func main() {
	if _, err := os.Stat("/.flatpak-info"); err != nil {
		fmt.Fprintln(os.Stderr, "fixture requires an installed Flatpak")
		os.Exit(2)
	}
	imageload.HTTP = func(context.Context, string, imageload.Header, imageload.Progress) (*imageload.Response, error) {
		requests.Add(1)
		return nil, errors.New("fixture observes every attempted network fetch")
	}
	control := updates.Find(context.Background(), updates.Config{Channel: "direct", Feed: "https://fixture.invalid/appcast.xml"})
	if requests.Swap(0) != 1 || control.Err == "" {
		fmt.Fprintln(os.Stderr, "transport positive control failed")
		os.Exit(2)
	}
	timer := time.AfterFunc(15*time.Second, func() {
		fmt.Fprintln(os.Stderr, "installed updater fixture timed out")
		os.Exit(2)
	})
	nsgo.Run(root)
	timer.Stop()
	if err := <-result; err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println("PASS: installed Flatpak, automatic checks disabled, three explicit checks managed, network requests=0")
}

func root(cx *core.Scope) core.Node {
	capability := updater.CapabilityOf(cx)
	state := updater.Status(cx)
	if capability.Managed != updater.ManagedStore || capability.Store != "Flatpak" || state.AutomaticChecks {
		result <- fmt.Errorf("startup policy: capability=%+v state=%+v", capability, state)
		cx.Quit()
		return core.View{}
	}
	remaining := 3
	var failure error
	for range remaining {
		cx.CheckForUpdates(false, func(answer core.UpdateCheck) {
			if answer.Managed != core.UpdaterStore || answer.Store != "Flatpak" || answer.Err != nil || answer.Version != "" {
				failure = fmt.Errorf("explicit check = %+v", answer)
			}
			remaining--
			if remaining == 0 {
				if got := requests.Load(); got != 0 {
					failure = fmt.Errorf("attempted %d network requests", got)
				}
				result <- failure
				cx.Quit()
			}
		})
	}
	return core.View{Style: core.Style{Width: core.Pt(240), Height: core.Pt(160)}}
}
