//go:build js && !nsgominiprogram && !nsgoharmony

package terminal

import (
	"strings"
	"testing"
)

func TestBrowserNeedsRemoteConnection(t *testing.T) {
	term, err := New(Options{})
	if term != nil {
		term.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "give Options.Conn a remote session") {
		t.Fatalf("a browser's local program request: %v", err)
	}
}
