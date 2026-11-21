//go:build js

package web

import (
	"net/url"
	"os"
	"strings"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// share invokes the browser's Web Share API (navigator.share) for text,
// links and in-memory files built as JS File objects. It checks
// navigator.canShare when available, answering driver.NoShareSheet when
// sharing or the requested payload is unsupported, Finished: false with no
// error on AbortError (user cancelled), and Finished: true on resolution.
func (s *surface) share(r driver.ShareRequest, id uint32) {
	share(r, id, s.engine.Dispatch)
}

func share(r driver.ShareRequest, id uint32, dispatch func(driver.Event)) {
	nav := js.Global().Get("navigator")
	if nav.IsUndefined() || nav.IsNull() || nav.Get("share").Type() != js.TypeFunction {
		dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		return
	}

	data := js.Global().Get("Object").New()
	if r.Text != "" {
		if u := webShareURL(r.Text); u != "" {
			data.Set("url", u)
		} else {
			data.Set("text", r.Text)
		}
	}
	if len(r.Files) > 0 {
		fileCtor := js.Global().Get("File")
		if fileCtor.Type() != js.TypeFunction {
			dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
			return
		}
		files := js.Global().Get("Array").New(len(r.Files))
		for i, f := range r.Files {
			raw := f.Data
			if raw == nil && f.Path != "" {
				b, err := os.ReadFile(f.Path)
				if err != nil {
					dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: err.Error()})
					return
				}
				raw = b
			}
			buf := js.Global().Get("Uint8Array").New(len(raw))
			js.CopyBytesToJS(buf, raw)
			opts := map[string]any{}
			if f.MIME != "" {
				opts["type"] = f.MIME
			}
			name := f.Name
			if name == "" {
				name = "file"
			}
			files.SetIndex(i, fileCtor.New([]any{buf}, name, opts))
		}
		data.Set("files", files)
	}

	if nav.Get("canShare").Type() == js.TypeFunction && !nav.Call("canShare", data).Bool() {
		dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		return
	}

	then(nav.Call("share", data), func(js.Value) {
		dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Finished: true})
	}, func(e js.Value) {
		switch errorName(e) {
		case "AbortError":
			dispatch(driver.Event{Kind: driver.EventFiles, Count: id})
		case "NotSupportedError":
			dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		default:
			dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: jsError(e).Error()})
		}
	})
}

// webShareURL returns s trimmed when it is a standalone http or https URL
// with a host, so navigator.share sends it in ShareData.url.
func webShareURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return ""
	}
	return s
}
