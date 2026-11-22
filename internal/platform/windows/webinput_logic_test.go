package windows

import (
	"encoding/json"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebClickPointScaling(t *testing.T) {
	tests := []struct {
		name         string
		pt           driver.Point
		raster, zoom float64
		wantX, wantY int32
	}{
		{"identity", driver.Point{X: 12, Y: 34}, 1, 1, 12, 34},
		{"hidpi", driver.Point{X: 12.5, Y: 20}, 1.5, 1, 19, 30},
		{"zoomed", driver.Point{X: 10, Y: 15}, 2, 1.25, 25, 38},
		{"fallback_nonpositive", driver.Point{X: 7, Y: 9}, 0, -2, 7, 9},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			x, y := webClickPoint(tc.pt, tc.raster, tc.zoom)
			if x != tc.wantX || y != tc.wantY {
				t.Fatalf("webClickPoint(%v, %v, %v) = (%d, %d), want (%d, %d)",
					tc.pt, tc.raster, tc.zoom, x, y, tc.wantX, tc.wantY)
			}
		})
	}
}

func TestWebKeyEventsForText(t *testing.T) {
	events := webKeyEventsForInput(driver.WebInput{Kind: driver.WebInputText, Text: "aB1!\r\n日"})
	// 6 characters: 'a', 'B', '1', '!', '\r\n' (one Enter), '日' => 12 down/up events.
	if len(events) != 12 {
		t.Fatalf("got %d events, want 12: %+v", len(events), events)
	}
	checkPair := func(i int, key, code, text, unmod string, vk, mods int) {
		t.Helper()
		down, up := events[2*i], events[2*i+1]
		if down.Type != "keyDown" || down.Key != key || down.Code != code || down.Text != text ||
			down.UnmodifiedText != unmod || down.WindowsVirtualKeyCode != vk || down.Modifiers != mods {
			t.Errorf("pair %d down = %+v, want key=%q code=%q text=%q unmod=%q vk=%d mods=%d",
				i, down, key, code, text, unmod, vk, mods)
		}
		if up.Type != "keyUp" || up.Key != key || up.Code != code || up.WindowsVirtualKeyCode != vk || up.Modifiers != mods {
			t.Errorf("pair %d up = %+v", i, up)
		}
		for _, ev := range []webCDPKeyEvent{down, up} {
			if _, err := json.Marshal(ev); err != nil {
				t.Errorf("marshal pair %d: %v", i, err)
			}
		}
	}
	checkPair(0, "a", "KeyA", "a", "a", 'A', 0)
	checkPair(1, "B", "KeyB", "B", "b", 'B', webCDPShift)
	checkPair(2, "1", "Digit1", "1", "1", '1', 0)
	checkPair(3, "!", "Digit1", "!", "1", '1', webCDPShift)
	checkPair(4, "Enter", "Enter", "\r", "\r", 0x0D, 0)
	checkPair(5, "日", "", "日", "日", 0, 0)
}

func TestWebKeyEventsForNamedKeys(t *testing.T) {
	tests := []struct {
		key      string
		wantType string
		wantKey  string
		wantCode string
		wantText string
		wantVK   int
	}{
		{"Enter", "keyDown", "Enter", "Enter", "\r", 0x0D},
		{"return", "keyDown", "Enter", "Enter", "\r", 0x0D},
		{"Tab", "rawKeyDown", "Tab", "Tab", "", 0x09},
		{"Escape", "rawKeyDown", "Escape", "Escape", "", 0x1B},
		{"Backspace", "rawKeyDown", "Backspace", "Backspace", "", 0x08},
		{"Delete", "rawKeyDown", "Delete", "Delete", "", 0x2E},
		{"Space", "keyDown", " ", "Space", " ", 0x20},
		{" ", "keyDown", " ", "Space", " ", 0x20},
		{"ArrowLeft", "rawKeyDown", "ArrowLeft", "ArrowLeft", "", 0x25},
		{"right", "rawKeyDown", "ArrowRight", "ArrowRight", "", 0x27},
		{"Home", "rawKeyDown", "Home", "Home", "", 0x24},
		{"PageDown", "rawKeyDown", "PageDown", "PageDown", "", 0x22},
		{"F1", "rawKeyDown", "F1", "F1", "", 0x70},
		{"f12", "rawKeyDown", "F12", "F12", "", 0x7B},
	}
	for _, tc := range tests {
		t.Run(tc.key, func(t *testing.T) {
			events := webKeyEventsForInput(driver.WebInput{Kind: driver.WebInputKey, Key: tc.key})
			if len(events) != 2 {
				t.Fatalf("len(events) = %d, want 2", len(events))
			}
			down, up := events[0], events[1]
			if down.Type != tc.wantType || down.Key != tc.wantKey || down.Code != tc.wantCode ||
				down.Text != tc.wantText || down.WindowsVirtualKeyCode != tc.wantVK {
				t.Fatalf("down = %+v, want type=%q key=%q code=%q text=%q vk=%d",
					down, tc.wantType, tc.wantKey, tc.wantCode, tc.wantText, tc.wantVK)
			}
			if up.Type != "keyUp" || up.Key != tc.wantKey || up.Code != tc.wantCode || up.WindowsVirtualKeyCode != tc.wantVK {
				t.Fatalf("up = %+v", up)
			}
		})
	}
	if got := webKeyEventsForInput(driver.WebInput{Kind: driver.WebInputKey, Key: "NotARealKey"}); len(got) != 0 {
		t.Fatalf("unknown key produced %+v", got)
	}
	if got := webKeyEventsForInput(driver.WebInput{Kind: driver.WebInputText, Text: ""}); len(got) != 0 {
		t.Fatalf("empty text produced %+v", got)
	}
}
