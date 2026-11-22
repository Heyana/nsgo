package driver

import (
	"bytes"
	"image"
	"image/png"
	"testing"
)

func TestClipboardPNG(t *testing.T) {
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	if c := ClipboardPNG(b.Bytes()); c == nil || c.Err != "" || c.Size != (Size{Width: 3, Height: 2}) {
		t.Fatalf("ClipboardPNG = %+v", c)
	}
	for _, data := range [][]byte{[]byte("GIF89a"), b.Bytes()[:20]} {
		if c := ClipboardPNG(data); c == nil || c.Err == "" {
			t.Errorf("ClipboardPNG(%q) = %+v, want an error", data, c)
		}
	}
}

func TestAnswerPermission(t *testing.T) {
	var got []Event
	dispatch := func(ev Event) { got = append(got, ev) }
	for _, c := range []PermissionCommand{PermissionCheck, PermissionAsk, PermissionOpenSettings, PermissionGuide, PermissionStopGuide} {
		AnswerPermission(uint32(c)+1, PermissionRequest{Command: c, Name: PermissionCamera}, PhonePermission, dispatch)
	}
	want := []Event{{Kind: EventPermission, Count: 1, Index: PermissionUnknown}, {Kind: EventPermission, Count: 2, Index: PermissionUnknown},
		{Kind: EventPermission, Count: 3}, {Kind: EventPermission, Count: 4, Index: PermissionUnknown}}
	if len(got) != len(want) {
		t.Fatalf("answers %+v", got)
	}
	for i := range want {
		if got[i].Kind != want[i].Kind || got[i].Count != want[i].Count || got[i].Index != want[i].Index || got[i].Finished {
			t.Errorf("answer %d: %+v, want %+v", i, got[i], want[i])
		}
	}
	if PhonePermission(PermissionAccessibility) != PermissionUnavailable {
		t.Error("a phone keeps Accessibility")
	}
	for _, kept := range []string{PermissionContacts, PermissionPhotos, PermissionNotifications} {
		if PhonePermission(kept) != PermissionUnknown {
			t.Errorf("PhonePermission(%q) = %d, want PermissionUnknown", kept, PhonePermission(kept))
		}
	}
	for _, macOnly := range []string{PermissionAutomation, PermissionAutomation + ":com.apple.finder"} {
		if PhonePermission(macOnly) != PermissionUnavailable {
			t.Errorf("PhonePermission(%q) = %d, want PermissionUnavailable", macOnly, PhonePermission(macOnly))
		}
	}
}

func TestAutomationTarget(t *testing.T) {
	if id, ok := AutomationTarget("automation:com.apple.finder"); !ok || id != "com.apple.finder" {
		t.Errorf("AutomationTarget(valid) = %q, %v", id, ok)
	}
	for _, invalid := range []string{"", "automation", "automation:", "automation: ", "automation:com.apple.finder ", "automation:a\nb", "camera:foo"} {
		if id, ok := AutomationTarget(invalid); ok {
			t.Errorf("AutomationTarget(%q) = %q, true; want false", invalid, id)
		}
	}
}

func TestTaskProgressBarFraction(t *testing.T) {
	for _, tc := range []struct {
		in   TaskProgress
		want float64
	}{
		{TaskProgress{Value: .7}, 0},
		{TaskProgress{State: TaskProgressIndeterminate}, .5},
		{TaskProgress{State: TaskProgressNormal, Value: .25}, .25},
		{TaskProgress{State: TaskProgressPaused, Value: .4}, .4},
		{TaskProgress{State: TaskProgressError, Value: 1}, 1},
	} {
		if got := tc.in.BarFraction(); got != tc.want {
			t.Errorf("%+v.BarFraction() = %g, want %g", tc.in, got, tc.want)
		}
	}
}

func TestPaintedViews(t *testing.T) {
	plain := DefaultViewStyle
	for _, tc := range []struct {
		name   string
		kind   ViewKind
		style  ViewStyle
		events EventMask
		drag   WindowDrag
		want   bool
	}{
		{"an empty container", KindView, plain, 0, WindowDragInherit, false},
		{"a container with a background", KindView, ViewStyle{Opacity: 1, Background: RGB(0x336699)}, 0, WindowDragInherit, true},
		{"a transparent background", KindView, ViewStyle{Opacity: 1, Background: RGBA(0x336699, 0)}, 0, WindowDragInherit, false},
		{"a border", KindView, ViewStyle{Opacity: 1, BorderWidth: 1, BorderColor: RGB(0)}, 0, WindowDragInherit, true},
		{"a hidden border", KindView, ViewStyle{Opacity: 1, BorderWidth: 1, BorderColor: RGB(0), BorderStyle: BorderNone}, 0, WindowDragInherit, false},
		{"one side's border", KindView, ViewStyle{Opacity: 1, Borders: Edges{Bottom: 1}, BorderColors: EdgeColors{Bottom: RGB(0)}}, 0, WindowDragInherit, true},
		{"a shadow alone", KindView, ViewStyle{Opacity: 1, ShadowColor: RGB(0), ShadowRadius: 4}, 0, WindowDragInherit, false},
		{"a gradient", KindView, ViewStyle{Opacity: 1, Effects: &Effects{Gradients: []GradientPaint{{}}}}, 0, WindowDragInherit, true},
		{"a text", KindText, plain, 0, WindowDragInherit, true},
		{"an image", KindImage, plain, 0, WindowDragInherit, true},
		{"an invisible image", KindImage, ViewStyle{}, 0, WindowDragInherit, false},
		{"a pressable container", KindView, plain, 1 << EventPress, WindowDragInherit, true},
		{"a context menu's container", KindView, plain, 1 << EventContextMenu, WindowDragInherit, true},
		{"an invisible pressable", KindView, ViewStyle{Hidden: true}, 1 << EventPress, WindowDragInherit, true},
		{"a window drag handle", KindView, plain, 0, WindowDragOn, true},
		{"a container hearing no pointer event", KindView, plain, 1 << EventFocus, WindowDragInherit, false},
	} {
		if got := Painted(tc.kind, &tc.style, tc.events, tc.drag); got != tc.want {
			t.Errorf("%s: Painted = %v; want %v", tc.name, got, tc.want)
		}
	}
}
