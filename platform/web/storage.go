//go:build js && wasm

package web

import (
	"fmt"
	"log/slog"
	"syscall/js"

	"github.com/oneo-me/nsgo/core"
)

// localStorage keeps the app's entries in the browser's localStorage, each
// under its key prefixed with "nsgo:<app id>:", so apps on one origin keep
// apart.
type localStorage struct {
	prefix string
}

func newLocalStorage(id string) *localStorage {
	if id == "" {
		id = "default"
	}
	return &localStorage{prefix: "nsgo:" + id + ":"}
}

func (l *localStorage) Load() (entries map[string]string, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("localStorage: %v", r)
		}
	}()
	ls := js.Global().Get("localStorage")
	entries = map[string]string{}
	for _, k := range l.keys(ls) {
		entries[k[len(l.prefix):]] = ls.Call("getItem", k).String()
	}
	return entries, nil
}

// keys returns the stored keys with the app's prefix.
func (l *localStorage) keys(ls js.Value) []string {
	var keys []string
	for i, n := 0, ls.Get("length").Int(); i < n; i++ {
		k := ls.Call("key", i).String()
		if len(k) >= len(l.prefix) && k[:len(l.prefix)] == l.prefix {
			keys = append(keys, k)
		}
	}
	return keys
}

func (l *localStorage) Write(c core.StorageChange) {
	defer func() {
		// The browser refuses writes past its own quota or in some
		// private modes; the entries stay in memory.
		if r := recover(); r != nil {
			slog.Error("nsgo: the app's storage could not be written", "err", r)
		}
	}()
	ls := js.Global().Get("localStorage")
	switch c.Op {
	case core.StorageSet:
		ls.Call("setItem", l.prefix+c.Key, c.Value)
	case core.StorageRemove:
		ls.Call("removeItem", l.prefix+c.Key)
	case core.StorageClear:
		for _, k := range l.keys(ls) {
			ls.Call("removeItem", k)
		}
	}
}
