package gpu

import "testing"

// A frame that creates and closes bind groups must not grow the device for its lifetime: a closed group and its layout leave the lists a restore walks.
func TestClosedBindGroupsLeaveTheRestoreLists(t *testing.T) {
	d := newDevice(nil, nil, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	layouts, groups := len(d.retainedLayouts), len(d.retainedBindGroups)
	for range 16 {
		l, err := d.CreateBindGroupLayout(nil, nil)
		if err != nil {
			t.Fatalf("CreateBindGroupLayout = %v", err)
		}
		g, err := d.CreateBindGroupDesc(nil, BindGroupDesc{Layout: l})
		if err != nil {
			t.Fatalf("CreateBindGroupDesc = %v", err)
		}
		if err := g.Close(); err != nil {
			t.Fatalf("BindGroup.Close = %v", err)
		}
		if err := l.Close(); err != nil {
			t.Fatalf("BindGroupLayout.Close = %v", err)
		}
	}
	if got := len(d.retainedLayouts); got != layouts {
		t.Fatalf("retainedLayouts after the closes = %d, want %d", got, layouts)
	}
	if got := len(d.retainedBindGroups); got != groups {
		t.Fatalf("retainedBindGroups after the closes = %d, want %d", got, groups)
	}
}

// A restore rebuilds every open bind group and layout and none that a close removed.
func TestRestoreKeepsOnlyOpenBindGroups(t *testing.T) {
	d := newDevice(nil, nil, 1, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	d.retainResources = true
	openLayout, err := d.CreateBindGroupLayout(nil, nil)
	if err != nil {
		t.Fatalf("CreateBindGroupLayout = %v", err)
	}
	openGroup, err := d.CreateBindGroupDesc(nil, BindGroupDesc{Layout: openLayout})
	if err != nil {
		t.Fatalf("CreateBindGroupDesc = %v", err)
	}
	closedLayout, err := d.CreateBindGroupLayout(nil, nil)
	if err != nil {
		t.Fatalf("CreateBindGroupLayout = %v", err)
	}
	closedGroup, err := d.CreateBindGroupDesc(nil, BindGroupDesc{Layout: closedLayout})
	if err != nil {
		t.Fatalf("CreateBindGroupDesc = %v", err)
	}
	if err := closedGroup.Close(); err != nil {
		t.Fatalf("BindGroup.Close = %v", err)
	}
	if err := closedLayout.Close(); err != nil {
		t.Fatalf("BindGroupLayout.Close = %v", err)
	}

	restored := newDevice(nil, nil, 2, 64, 64, FormatRGBA8Unorm, ColorSpaceDefault, 2, Limits{}, Features{}, nil)
	restored.retainResources = true
	restored.restoreRetainedFrom(d)

	if len(restored.retainedLayouts) != 1 || restored.retainedLayouts[0] != openLayout {
		t.Fatalf("restored layouts = %v, want the open one", restored.retainedLayouts)
	}
	if len(restored.retainedBindGroups) != 1 || restored.retainedBindGroups[0] != openGroup {
		t.Fatalf("restored bind groups = %v, want the open one", restored.retainedBindGroups)
	}
	if !openGroup.Valid() {
		t.Fatal("the restored bind group is not valid on the new device")
	}
}
