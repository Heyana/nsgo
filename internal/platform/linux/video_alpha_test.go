//go:build linux && !android

package linux

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/platform/videoalpha"
)

func TestVideoLookCSSClearsGtkVideoBackground(t *testing.T) {
	if !strings.Contains(lookCSS, "video.nsgo-video { background: none; }") {
		t.Fatalf("lookCSS does not clear GtkVideo's default black background: %q", lookCSS)
	}
}

func TestVideoAlphaRefusedMessages(t *testing.T) {
	if msg := alphaRefused(videoalpha.None); msg != "" {
		t.Fatalf("alphaRefused(None) = %q, want empty", msg)
	}
	if msg := alphaRefused(videoalpha.HEVC); !strings.Contains(msg, "HEVC with alpha") {
		t.Fatalf("alphaRefused(HEVC) = %q, want HEVC explanation", msg)
	}
}
