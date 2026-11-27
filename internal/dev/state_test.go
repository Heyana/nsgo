//go:build nsgodev

package dev

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/devtools"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDevStateRoundTripAndHandoffOverHTTP(t *testing.T) {
	var (
		mu         sync.Mutex
		state      = map[string]json.RawMessage{}
		handoffCh  = make(chan string, 1)
		handoffAck = make(chan string, 1)
		stopServer = make(chan struct{})
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-NSGo-Token") != "state-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch r.URL.Path {
		case "/_nsgo/state":
			switch r.Method {
			case http.MethodGet:
				mu.Lock()
				snap := make(map[string]json.RawMessage, len(state))
				for k, v := range state {
					snap[k] = append(json.RawMessage(nil), v...)
				}
				mu.Unlock()
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(snap)
			case http.MethodPost:
				var incoming map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}
				mu.Lock()
				for k, v := range incoming {
					state[k] = append(json.RawMessage(nil), v...)
				}
				mu.Unlock()
				w.WriteHeader(http.StatusNoContent)
			}
		case "/_nsgo/events":
			w.Header().Set("Content-Type", "text/event-stream")
			flusher, _ := w.(http.Flusher)
			fmt.Fprint(w, "data: hello\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			for {
				select {
				case id := <-handoffCh:
					fmt.Fprintf(w, "data: handoff %s\n\n", id)
					if flusher != nil {
						flusher.Flush()
					}
				case <-stopServer:
					return
				case <-r.Context().Done():
					return
				}
			}
		case "/_nsgo/handoff":
			handoffAck <- r.URL.Query().Get("id")
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer func() {
		close(stopServer)
		server.Close()
	}()

	prevToken := devtools.ServerToken
	devtools.ServerToken = "state-token"
	devtools.LoadState(nil)
	handoffMu.Lock()
	lastHandoff = ""
	handoffMu.Unlock()
	t.Cleanup(func() {
		devtools.ServerToken = prevToken
		devtools.SetStateWriter(nil, nil)
		devtools.LoadState(nil)
		handoffMu.Lock()
		lastHandoff = ""
		handoffMu.Unlock()
	})

	// Run 1: attach state writer and SSE listener, mutate DevKeep and Nav.
	startDevState(server.URL)
	go listenDev(server.URL, nil)

	root1 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	tab1 := core.NewSignal(root1, "home")
	core.DevKeep(root1, "tab", tab1)
	nav1 := base.NewNav(root1, "root")
	tab1.Set("billing")
	nav1.Push("invoices")
	nav1.Push("invoice-9")

	// Request a handoff flush over SSE; the ack must arrive only after state is posted.
	handoffCh <- "h1"
	select {
	case id := <-handoffAck:
		if id != "h1" {
			t.Fatalf("handoff ack id = %q, want h1", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for handoff ack")
	}
	root1.Dispose()

	// Run 2: clear local state and restore from the dev server.
	devtools.SetStateWriter(nil, nil)
	devtools.LoadState(nil)
	startDevState(server.URL)

	root2 := core.NewRoot(core.RootOptions{Post: func(func()) {}})
	defer root2.Dispose()
	tab2 := core.NewSignal(root2, "home")
	core.DevKeep(root2, "tab", tab2)
	nav2 := base.NewNav(root2, "root")
	if got := tab2.Get(); got != "billing" {
		t.Fatalf("restored tab = %q, want billing", got)
	}
	if got := nav2.Routes(); !slices.Equal(got, []string{"root", "invoices", "invoice-9"}) {
		t.Fatalf("restored nav = %v, want [root invoices invoice-9]", got)
	}
}

func TestDevReadySignalsOnceOnFirstViewBatch(t *testing.T) {
	var readyCalls atomic.Int32
	readyIDs := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-NSGo-Token") != "ready-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if r.URL.Path == "/_nsgo/ready" && r.Method == http.MethodPost {
			readyCalls.Add(1)
			readyIDs <- r.URL.Query().Get("id")
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer server.Close()

	prevToken, prevApplied := devtools.ServerToken, driver.Applied
	devtools.ServerToken = "ready-token"
	var prevCalled atomic.Int32
	driver.Applied = func(*driver.MountBatch, time.Duration) { prevCalled.Add(1) }
	t.Cleanup(func() {
		devtools.ServerToken = prevToken
		driver.Applied = prevApplied
	})

	t.Setenv(devReadyEnv, "r7")
	startDevReady(server.URL)
	if _, ok := os.LookupEnv(devReadyEnv); ok {
		t.Fatal("NSGO_DEV_READY_ID was not unset from the environment")
	}

	// A batch with no ItemCreate (e.g., hidden app-only surface) must not trigger ready.
	driver.Apply(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemOpenSurface}}}, func(*driver.MountBatch) {})
	time.Sleep(50 * time.Millisecond)
	if n := readyCalls.Load(); n != 0 {
		t.Fatalf("non-view batch triggered ready %d times", n)
	}

	// The first batch with ItemCreate triggers ready once and chains previous Applied.
	driver.Apply(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCreate}}}, func(*driver.MountBatch) {})
	driver.Apply(&driver.MountBatch{Items: []driver.MountItem{{Kind: driver.ItemCreate}}}, func(*driver.MountBatch) {})
	select {
	case id := <-readyIDs:
		if id != "r7" {
			t.Fatalf("ready id = %q, want r7", id)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for /_nsgo/ready")
	}
	time.Sleep(50 * time.Millisecond)
	if n := readyCalls.Load(); n != 1 {
		t.Fatalf("ready posted %d times, want 1", n)
	}
	if n := prevCalled.Load(); n != 3 {
		t.Fatalf("chained driver.Applied ran %d times, want 3", n)
	}
}
