package discovery_test

import (
	"fmt"
	"net"
	"os"
	"sync"
	"testing"
	"time"

	_ "github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestBonjourAdvertiseBrowseAndRemove(t *testing.T) {
	switch nsgotest.Target() {
	case "macos", "ios", "ios_simulator":
	default:
		t.Skip("checks native Bonjour")
	}
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	name := fmt.Sprintf("NSGo test %d", os.Getpid())
	port := listener.Addr().(*net.TCPAddr).Port
	var mu sync.Mutex
	var replies []core.ServiceEvent
	var failure error
	var scope *core.Scope
	var stop func()
	report := func(err error) {
		if err != nil {
			mu.Lock()
			failure = err
			mu.Unlock()
		}
	}
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		scope = cx
		_, err := cx.BrowseServices("_nsgo-test._tcp", func(e core.ServiceEvent) {
			report(e.Err)
			if e.Service.Name == name {
				mu.Lock()
				replies = append(replies, e)
				mu.Unlock()
			}
		})
		report(err)
		stop, err = cx.AdvertiseService(core.LocalService{Name: name, Type: "_nsgo-test._tcp", Port: port, TXT: map[string]string{"version": "1"}}, report)
		report(err)
		return core.View{}
	})
	wait := func(count int) core.ServiceEvent {
		t.Helper()
		app.WaitUntil(20*time.Second, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return failure != nil || len(replies) >= count
		})
		mu.Lock()
		defer mu.Unlock()
		if failure != nil {
			t.Fatal(failure)
		}
		return replies[count-1]
	}
	add := wait(1)
	if add.Removed || add.Service.Port != port || add.Service.Host == "" || add.Service.TXT["version"] != "1" {
		t.Fatalf("resolved advertisement: %+v", add)
	}
	scope.Post(stop)
	if remove := wait(2); !remove.Removed || remove.Service.Host != add.Service.Host || remove.Service.Port != port {
		t.Fatalf("removal: %+v", remove)
	}
}
