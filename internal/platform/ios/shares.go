//go:build ios

package ios

import (
	"log"
	"os"
	"path/filepath"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/shareinbox"
)

// observeShares keeps provider I/O off UIKit's thread. Notifications merely wake the worker; durable batches are also read on every normal app activation.
func (s *surface) observeShares() {
	// One reader per process: a scene that attaches again keeps the one already draining the inbox.
	if s != runner || s.engine == nil || s.shareWake != nil {
		return
	}
	group := shareinbox.GroupPath()
	if group == "" {
		return
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		return
	}
	jobs, stop := make(chan struct{}, 1), make(chan struct{})
	engine := s.engine
	reader := shareinbox.NewReader(group, filepath.Join(cache, "nsgo-shares"))
	s.shareWake = func() {
		if appLifecycle() != driver.LifecycleActive {
			return
		}
		select {
		case jobs <- struct{}{}:
		default:
		}
	}
	unobserve := shareinbox.Observe(func() {
		apple.OnMain(func() {
			if s.shareWake != nil {
				s.shareWake()
			}
		})
	})
	s.stops = append(s.stops, func() { s.shareWake = nil; unobserve(); close(stop) })
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-jobs:
			}
			err := reader.Drain(func(items []driver.SharedItem, acknowledge func() error) {
				ack := func() {
					go func() {
						if err := acknowledge(); err != nil {
							log.Printf("nsgo: cannot acknowledge shared items: %v", err)
						}
					}()
				}
				engine.Dispatch(driver.Event{Kind: driver.EventShared, Data: driver.SharedDelivery{Items: items, Acknowledge: ack}})
			})
			if err != nil {
				log.Printf("nsgo: cannot receive shared items: %v", err)
			}
		}
	}()
	s.shareWake()
}
