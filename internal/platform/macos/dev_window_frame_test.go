//go:build darwin && !ios && nsgodev

package macos

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestDevWindowFrameKeepsTheLatestNormalFrame(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frame")
	initial := driver.WindowFrame{Move: true, X: 1, Y: 2, Width: 340, Height: 200}
	frame := driver.WindowFrame{Move: true, X: -40, Y: 110, Width: 480, Height: 320}
	d := devWindowFrame{pending: make(chan driver.WindowFrame, 1)}
	d.record(driver.Environment{WindowFrame: initial})
	d.record(driver.Environment{WindowFrame: frame})
	d.record(driver.Environment{WindowFrame: initial, FullScreen: true})
	d.record(driver.Environment{WindowFrame: initial, Minimized: true})
	d.record(driver.Environment{WindowFrame: initial, Maximized: true})
	close(d.pending)
	saveDevWindowFrames(path, d.pending)

	t.Setenv(devWindowFrameEnv, path)
	var next devWindowFrame
	if got := next.restore(initial); got != frame {
		t.Fatalf("restored frame = %+v, want %+v", got, frame)
	}
	if _, ok := os.LookupEnv(devWindowFrameEnv); ok {
		t.Fatal("the frame file path was left in the child environment")
	}
	if err := os.WriteFile(path, []byte("incomplete frame"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(devWindowFrameEnv, path)
	if got := next.restore(initial); got != initial {
		t.Fatalf("restored frame = %+v with invalid data, want %+v", got, initial)
	}
}
