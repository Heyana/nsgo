//go:build nsgodev

package devtools

import (
	"bytes"
	"encoding/json"
	"sync"
)

var (
	stateMu      sync.Mutex
	stateValues  map[string]json.RawMessage
	stateOnWrite func(key string, raw json.RawMessage)
	stateFlush   func()
)

// LoadState replaces the in-memory dev state snapshot restored from the dev
// server on startup. Passing nil or an empty map clears stored values.
func LoadState(values map[string]json.RawMessage) {
	stateMu.Lock()
	if len(values) == 0 {
		stateValues = nil
	} else {
		stateValues = make(map[string]json.RawMessage, len(values))
		for k, v := range values {
			stateValues[k] = append(json.RawMessage(nil), v...)
		}
	}
	stateMu.Unlock()
}

// SnapshotState returns a copy of the current in-memory dev state map.
func SnapshotState() map[string]json.RawMessage {
	stateMu.Lock()
	defer stateMu.Unlock()
	if len(stateValues) == 0 {
		return nil
	}
	out := make(map[string]json.RawMessage, len(stateValues))
	for k, v := range stateValues {
		out[k] = append(json.RawMessage(nil), v...)
	}
	return out
}

// SetStateWriter installs the dev client's persistence callbacks for state
// writes and synchronous handoff flushes.
func SetStateWriter(onWrite func(key string, raw json.RawMessage), flush func()) {
	stateMu.Lock()
	stateOnWrite = onWrite
	stateFlush = flush
	stateMu.Unlock()
}

// ReadState returns the saved JSON value for key, if present.
func ReadState(key string) (json.RawMessage, bool) {
	stateMu.Lock()
	defer stateMu.Unlock()
	v, ok := stateValues[key]
	if !ok {
		return nil, false
	}
	return append(json.RawMessage(nil), v...), true
}

// WriteState stores raw JSON for key in memory and notifies the dev client
// writer when the value changed.
func WriteState(key string, raw json.RawMessage) {
	stateMu.Lock()
	if prev, ok := stateValues[key]; ok && bytes.Equal(prev, raw) {
		stateMu.Unlock()
		return
	}
	cp := append(json.RawMessage(nil), raw...)
	if stateValues == nil {
		stateValues = map[string]json.RawMessage{}
	}
	stateValues[key] = cp
	w := stateOnWrite
	stateMu.Unlock()
	if w != nil {
		w(key, cp)
	}
}

// FlushState synchronously flushes any pending dev state writes.
func FlushState() {
	stateMu.Lock()
	f := stateFlush
	stateMu.Unlock()
	if f != nil {
		f()
	}
}
