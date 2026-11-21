//go:build js

package web

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestShare(t *testing.T) {
	nav := js.Global().Get("navigator")
	if nav.IsUndefined() || nav.IsNull() {
		t.Skip("no navigator")
	}

	origShare := js.Global().Get("Object").Call("getOwnPropertyDescriptor", nav, "share")
	origCanShare := js.Global().Get("Object").Call("getOwnPropertyDescriptor", nav, "canShare")
	t.Cleanup(func() {
		restore := func(prop string, desc js.Value) {
			if desc.IsUndefined() {
				nav.Delete(prop)
			} else {
				js.Global().Get("Object").Call("defineProperty", nav, prop, desc)
			}
		}
		restore("share", origShare)
		restore("canShare", origCanShare)
	})

	defineProp := func(prop string, val any) {
		js.Global().Get("Object").Call("defineProperty", nav, prop, map[string]any{
			"value":        val,
			"configurable": true,
			"writable":     true,
		})
	}

	events := make(chan driver.Event, 16)
	dispatch := func(ev driver.Event) { events <- ev }
	next := func() driver.Event {
		t.Helper()
		select {
		case ev := <-events:
			return ev
		case <-time.After(5 * time.Second):
			t.Fatal("timed out waiting for share event")
			return driver.Event{}
		}
	}

	// 1. Missing navigator.share answers NoShareSheet.
	defineProp("share", js.Undefined())
	defineProp("canShare", js.Undefined())
	share(driver.ShareRequest{Text: "hello"}, 1, dispatch)
	if ev := next(); ev.Count != 1 || ev.Finished || ev.Text != driver.NoShareSheet {
		t.Fatalf("missing share = %+v, want NoShareSheet", ev)
	}

	// 2. Text, URL and byte-backed files pass through to navigator.share and resolve.
	var lastData js.Value
	var shareMode string
	var shareCalls int
	canShareOK := true

	mockCanShare := js.FuncOf(func(_ js.Value, args []js.Value) any {
		return canShareOK
	})
	defer mockCanShare.Release()

	mockShare := js.FuncOf(func(_ js.Value, args []js.Value) any {
		shareCalls++
		lastData = args[0]
		promise := js.Global().Get("Promise")
		switch shareMode {
		case "abort":
			err := js.Global().Get("DOMException").New("cancelled", "AbortError")
			return promise.Call("reject", err)
		case "denied":
			err := js.Global().Get("DOMException").New("user activation required", "NotAllowedError")
			return promise.Call("reject", err)
		default:
			return promise.Call("resolve", js.Undefined())
		}
	})
	defer mockShare.Release()

	defineProp("share", mockShare)
	defineProp("canShare", mockCanShare)

	// Plain text.
	share(driver.ShareRequest{Text: "hello web"}, 2, dispatch)
	if ev := next(); ev.Count != 2 || !ev.Finished || ev.Text != "" {
		t.Fatalf("text share = %+v", ev)
	}
	if got := lastData.Get("text").String(); got != "hello web" || !lastData.Get("url").IsUndefined() {
		t.Fatalf("text ShareData = text:%q url:%v", got, lastData.Get("url"))
	}

	// Standalone URL.
	share(driver.ShareRequest{Text: "https://example.com/docs"}, 3, dispatch)
	if ev := next(); ev.Count != 3 || !ev.Finished || ev.Text != "" {
		t.Fatalf("url share = %+v", ev)
	}
	if got := lastData.Get("url").String(); got != "https://example.com/docs" || !lastData.Get("text").IsUndefined() {
		t.Fatalf("url ShareData = url:%q text:%v", got, lastData.Get("text"))
	}

	// Files from bytes with name and MIME.
	share(driver.ShareRequest{
		Text: "report",
		Files: []driver.SharedFile{
			{Data: []byte("abc"), Name: "notes.txt", MIME: "text/plain"},
		},
	}, 4, dispatch)
	if ev := next(); ev.Count != 4 || !ev.Finished || ev.Text != "" {
		t.Fatalf("file share = %+v", ev)
	}
	files := lastData.Get("files")
	if files.Length() != 1 {
		t.Fatalf("files length = %d, want 1", files.Length())
	}
	f0 := files.Index(0)
	if f0.Get("name").String() != "notes.txt" || f0.Get("type").String() != "text/plain" || f0.Get("size").Int() != 3 {
		t.Fatalf("file[0] = name:%q type:%q size:%d", f0.Get("name").String(), f0.Get("type").String(), f0.Get("size").Int())
	}
	buf, err := await(f0.Call("arrayBuffer"))
	if err != nil {
		t.Fatalf("arrayBuffer: %v", err)
	}
	gotBytes := make([]byte, 3)
	js.CopyBytesToGo(gotBytes, js.Global().Get("Uint8Array").New(buf))
	if string(gotBytes) != "abc" {
		t.Fatalf("file bytes = %q, want abc", gotBytes)
	}

	// 3. canShare returning false answers NoShareSheet without calling share.
	before := shareCalls
	canShareOK = false
	share(driver.ShareRequest{Files: []driver.SharedFile{{Data: []byte("x"), Name: "x.bin", MIME: "application/octet-stream"}}}, 5, dispatch)
	if ev := next(); ev.Count != 5 || ev.Finished || ev.Text != driver.NoShareSheet {
		t.Fatalf("canShare false = %+v, want NoShareSheet", ev)
	}
	if shareCalls != before {
		t.Fatalf("navigator.share called despite canShare=false")
	}

	// 4. AbortError maps to cancelled (Finished: false, Text: "").
	canShareOK = true
	shareMode = "abort"
	share(driver.ShareRequest{Text: "cancel me"}, 6, dispatch)
	if ev := next(); ev.Count != 6 || ev.Finished || ev.Text != "" {
		t.Fatalf("abort = %+v, want cancelled", ev)
	}

	// 5. Other rejections propagate error text.
	shareMode = "denied"
	share(driver.ShareRequest{Text: "blocked"}, 7, dispatch)
	if ev := next(); ev.Count != 7 || ev.Finished || ev.Text == "" || ev.Text == driver.NoShareSheet {
		t.Fatalf("denied = %+v, want error text", ev)
	}
}
