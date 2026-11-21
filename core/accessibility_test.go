package core

import (
	"reflect"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func propsOf(b *driver.MountBatch) map[NodeID]driver.ViewProps {
	out := map[NodeID]driver.ViewProps{}
	for _, it := range b.Items {
		if it.Kind == driver.ItemViewProps {
			out[it.ID] = *it.ViewProps
		}
	}
	return out
}

func TestAccessibilityPropsFollowTheirSignals(t *testing.T) {
	var now *Signal[float32]
	var open, hidden *Signal[bool]
	actions := []AccessibilityAction{{Name: ActionIncrement}, {Name: "reset", Label: "Reset"}}
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		now = NewSignal(cx, float32(40))
		open = NewSignal(cx, false)
		hidden = NewSignal(cx, false)
		return View{Children: []Node{
			View{TestID: "bar", Role: RoleProgressBar, Hint: "Downloads the file",
				AccessibilityValue:   func() AccessibilityValue { return AccessibilityValue{Max: 100, Now: now.Get()} },
				AccessibilityActions: actions, LiveRegion: LiveRegionPolite},
			View{TestID: "menu", Role: RoleButton, Expanded: open.Get},
			View{TestID: "hidden", AccessibilityElementsHidden: hidden.Get},
			View{TestID: "nohide", ImportantForAccessibility: ImportantNoHideDescendants},
			View{TestID: "multi", Role: RoleGrid, MultiSelectable: true},
			View{TestID: "no", ImportantForAccessibility: ImportantNo},
		}}
	})
	props := propsOf(h.commit())
	bar := props[h.find("bar").id]
	if bar.Hint != "Downloads the file" || bar.Value != (AccessibilityValue{Max: 100, Now: 40}) || !bar.Value.IsRange() ||
		bar.LiveRegion != LiveRegionPolite || !slices.Equal(bar.ActionList(), actions) || !bar.HasAction("reset") {
		t.Errorf("bar props = %+v", bar)
	}
	if m := props[h.find("menu").id]; !m.Expandable || m.Expanded {
		t.Errorf("menu props = %+v, want expandable and collapsed", m)
	}
	if p := props[h.find("nohide").id]; !p.ElementsHidden || p.Accessible != driver.AccessibleNo {
		t.Errorf("no-hide-descendants props = %+v", p)
	}
	if p := props[h.find("no").id]; p.ElementsHidden || p.Accessible != driver.AccessibleNo {
		t.Errorf("importantForAccessibility no props = %+v", p)
	}
	if p := props[h.find("multi").id]; !p.MultiSelectable || p.Role != RoleGrid {
		t.Errorf("multi props = %+v", p)
	}

	now.Set(60)
	open.Set(true)
	hidden.Set(true)
	props = propsOf(h.commit())
	if len(props) != 3 {
		t.Errorf("changes sent %d props, want the three that changed", len(props))
	}
	if p := props[h.find("bar").id]; p.Value.Now != 60 || p.Actions != bar.Actions {
		t.Errorf("bar props = %+v after the value changed, keeping its actions", p)
	}
	if !props[h.find("menu").id].Expanded || !props[h.find("hidden").id].ElementsHidden {
		t.Error("expanded and hidden states were not sent")
	}
	if k := kinds(h.commit()); k[driver.ItemViewProps] != 0 {
		t.Errorf("an idle commit sent %v", k)
	}
}

func TestALabelChangingAloneGoesAlone(t *testing.T) {
	var count *Signal[int]
	var off *Signal[bool]
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		count = NewSignal(cx, 1)
		off = NewSignal(cx, false)
		return View{TestID: "v", Role: RoleButton, Disabled: off.Get,
			LabelFunc: func() string { return string(rune('0' + count.Get())) }}
	})
	if p := propsOf(h.commit())[h.find("v").id]; p.Label != "1" {
		t.Fatalf("label = %q", p.Label)
	}
	count.Set(2)
	b := h.commit()
	if k := kinds(b); k[driver.ItemViewProps] != 0 || k[driver.ItemAccessibilityLabel] != 1 {
		t.Fatalf("a label change sent %v, want one ItemAccessibilityLabel", k)
	}
	for _, it := range b.Items {
		if it.Kind == driver.ItemAccessibilityLabel && (it.ID != h.find("v").id || it.Value != "2") {
			t.Errorf("label item = %+v", it)
		}
	}
	count.Set(3)
	off.Set(true)
	if p, ok := propsOf(h.commit())[h.find("v").id]; !ok || p.Label != "3" || !p.Disabled {
		t.Errorf("a label changing with other props sent %+v, want the whole props", p)
	}
}

func TestAccessibilityDescriptionAndInputLabelFollowSignals(t *testing.T) {
	var label, description *Signal[string]
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		label, description = NewSignal(cx, "Name"), NewSignal(cx, "Your full name")
		return View{Children: []Node{
			View{TestID: "view", Role: RoleGroup, AccessibilityDescriptionFunc: description.Get},
			Input{TestID: "input", DefaultValue: "Ada", LabelFunc: label.Get, AccessibilityDescriptionFunc: description.Get},
		}}
	})
	props := propsOf(h.commit())
	view, input := h.find("view").id, h.find("input").id
	if p := props[view]; p.Description != "Your full name" {
		t.Fatalf("initial view props = %+v", p)
	}
	if p := props[input]; p.Label != "Name" || p.Description != "Your full name" {
		t.Fatalf("initial input props = %+v", p)
	}
	// A field's label changing alone goes alone, as a view's does.
	label.Set("Nombre")
	b := h.commit()
	if k := kinds(b); k[driver.ItemViewProps] != 0 || k[driver.ItemAccessibilityLabel] != 1 {
		t.Fatalf("an input's label change sent %v, want one ItemAccessibilityLabel", k)
	}
	for _, it := range b.Items {
		if it.Kind == driver.ItemAccessibilityLabel && (it.ID != input || it.Value != "Nombre") {
			t.Errorf("label item = %+v", it)
		}
	}
	description.Set("Su nombre completo")
	props = propsOf(h.commit())
	if p := props[view]; p.Description != "Su nombre completo" {
		t.Errorf("updated view props = %+v", p)
	}
	if p := props[input]; p.Label != "Nombre" || p.Description != "Su nombre completo" {
		t.Errorf("updated input props = %+v", p)
	}
	// The same elements change, keeping what the field holds.
	if h.find("view").id != view || h.find("input").id != input {
		t.Error("a changing label or description remounted its element")
	}
	if v := h.find("input").input.value; v != "Ada" {
		t.Errorf("input value = %q after its texts changed, want Ada", v)
	}
	if k := kinds(h.commit()); k[driver.ItemViewProps] != 0 || k[driver.ItemAccessibilityLabel] != 0 {
		t.Errorf("an idle commit sent %v", k)
	}
}

func TestAccessibilityTextFollowsSignalsOnTheSameViews(t *testing.T) {
	var text *Signal[string]
	h := newHost(t, Size{Width: 200, Height: 180}, func(cx *Scope) Node {
		text = NewSignal(cx, "Play")
		return View{Children: []Node{
			View{TestID: "view", HintFunc: text.Get},
			Text{TestID: "text", HintFunc: text.Get, Content: "A label"},
			Scroll{TestID: "scroll", Role: RoleListbox, LabelFunc: text.Get},
			Image{TestID: "image", LabelFunc: text.Get, ElementsHidden: true},
			Icon{TestID: "icon", LabelFunc: text.Get},
			SVGView{TestID: "svg", LabelFunc: text.Get},
			Shader{TestID: "shader", LabelFunc: text.Get},
			Overlay{TestID: "overlay", LabelFunc: text.Get},
			Modal{TestID: "modal", LabelFunc: text.Get},
			Native{TestID: "native", LabelFunc: text.Get},
			SystemSwitch{TestID: "switch", LabelFunc: text.Get},
			WebView{TestID: "web", LabelFunc: text.Get},
			Video{TestID: "video", LabelFunc: text.Get},
		}}
	})
	h.tree.SetEnvironment(Environment{OS: OSMacOS})
	props := propsOf(h.commit())
	labels := []string{"scroll", "image", "icon", "svg", "shader", "overlay", "modal", "native", "switch", "web", "video"}
	ids := map[string]NodeID{}
	for _, id := range labels {
		ids[id] = h.find(id).id
		if got := props[ids[id]].Label; got != "Play" {
			t.Errorf("%s's initial label = %q", id, got)
		}
	}
	for _, id := range []string{"view", "text"} {
		ids[id] = h.find(id).id
		if got := props[ids[id]].Hint; got != "Play" {
			t.Errorf("%s's initial hint = %q", id, got)
		}
	}
	if !props[ids["image"]].ElementsHidden {
		t.Error("a label exposed an image whose elements are hidden")
	}
	for _, id := range []string{"icon", "svg", "shader"} {
		if props[ids[id]].Role != RoleImage {
			t.Errorf("the named %s did not have the image role", id)
		}
	}

	text.Set("Pause")
	b := h.commit()
	if k := kinds(b); k[driver.ItemCreate] != 0 || k[driver.ItemDelete] != 0 {
		t.Fatalf("changing accessibility text recreated views: %v", k)
	}
	updated := map[NodeID]string{}
	for _, item := range b.Items {
		switch item.Kind {
		case driver.ItemViewProps:
			updated[item.ID] = item.ViewProps.Label
		case driver.ItemAccessibilityLabel:
			updated[item.ID] = item.Value
		}
	}
	for _, id := range labels {
		if h.find(id).id != ids[id] || updated[ids[id]] != "Pause" {
			t.Errorf("%s did not receive the new label on its original view", id)
		}
	}
	props = propsOf(b)
	for _, id := range []string{"view", "text"} {
		if h.find(id).id != ids[id] || props[ids[id]].Hint != "Pause" {
			t.Errorf("%s did not receive the new hint on its original view", id)
		}
	}

	text.Set("")
	props = propsOf(h.commit())
	for _, id := range []string{"icon", "svg", "shader"} {
		if p, ok := props[ids[id]]; !ok || p.Role != RoleNone || p.Label != "" {
			t.Errorf("the %s is decorative again, got %+v", id, p)
		}
	}
}

func TestAccessibilityTextReadsFunctionsWhenAnElementIsSet(t *testing.T) {
	var elements []*Element
	var values *Signal[string]
	props := func(read func() string) []Node {
		return []Node{
			View{TestID: "view", HintFunc: read, AccessibilityDescriptionFunc: read},
			Text{TestID: "text", HintFunc: read, Content: "A label"},
			Scroll{TestID: "scroll", Role: RoleListbox, LabelFunc: read},
			Image{TestID: "image", LabelFunc: read},
		}
	}
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		values = NewSignal(cx, "First")
		var children []Node
		for _, p := range props(values.Get) {
			el := NewElement(cx, p)
			elements = append(elements, el)
			children = append(children, el)
		}
		return View{Children: children}
	})
	h.commit()
	values.Set("Second")
	if got := h.find("view").props.Hint; got != "First" {
		t.Fatalf("the detached-element API bound the hint to its signal, got %q", got)
	}
	for i, p := range props(values.Get) {
		elements[i].Set(p)
	}
	h.commit()
	view, text := h.find("view").props, h.find("text").props
	if view.Hint != "Second" || view.Description != "Second" || text.Hint != "Second" {
		t.Errorf("updated view and text accessibility = %+v, %+v", view, text)
	}
	scroll, image := h.find("scroll").props, h.find("image").props
	if scroll.Label != "Second" || scroll.Role != RoleListbox || image.Label != "Second" {
		t.Errorf("updated scroll and image labels = %+v, %+v", scroll, image)
	}
}

func TestInputElementAccessibilityMatchesMountedInput(t *testing.T) {
	for _, tc := range []struct {
		name  string
		input Input
	}{
		{"default", Input{}},
		{"static", Input{Role: RoleSearchField, Label: "Name", AccessibilityDescription: "As on your passport",
			AccessibilityActions: []AccessibilityAction{{Name: "clear", Label: "Clear"}}}},
		{"functions", Input{
			LabelFunc: func() string { return "Country" }, AccessibilityDescriptionFunc: func() string { return "Choose a country" },
			AccessibilityValue: func() AccessibilityValue { return AccessibilityValue{Text: "Japan"} },
			Expanded:           func() bool { return true }}},
		{"empty-functions", Input{
			LabelFunc: func() string { return "" }, AccessibilityDescriptionFunc: func() string { return "" },
			Expanded: func() bool { return false }}},
		{"states", Input{Invalid: func() bool { return true }, Disabled: func() bool { return true }}},
		{"state-styles", Input{Invalid: func() bool { return true }, Expanded: func() bool { return true },
			OnAccessibilityAction: func(string) {}, AccessibilityActions: []AccessibilityAction{{Name: "clear", Label: "Clear"}},
			Style: Style{Opacity: Alpha(0.5), Expanded: &Style{BorderWidth: 2}, AriaInvalid: &Style{Background: RGB(0xff0000)}}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
				mounted, element := tc.input, tc.input
				mounted.TestID, element.TestID = "mounted", "element"
				return View{Children: []Node{mounted, NewElement(cx, element)}}
			})
			props := propsOf(h.commit())
			if len(h.errs) != 0 {
				t.Fatalf("mount errors: %v", h.errs)
			}
			want, got := props[h.find("mounted").id], props[h.find("element").id]
			want.TestID, got.TestID = "", ""
			if !reflect.DeepEqual(got, want) {
				t.Errorf("element props = %+v, want mounted input's %+v", got, want)
			}
			// The states the props carry style the field, and its handlers set its events, as a mounted input's.
			m, e := h.find("mounted"), h.find("element")
			if !reflect.DeepEqual(e.style, m.style) {
				t.Errorf("element style = %+v, want mounted input's %+v", e.style, m.style)
			}
			if e.sent.events != m.sent.events {
				t.Errorf("element events = %v, want mounted input's %v", e.sent.events, m.sent.events)
			}
		})
	}
}

func TestInputElementAccessibilityChangesOnlyOnSet(t *testing.T) {
	var el *Element
	var state *Signal[bool]
	var input Input
	var actions []string
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		state = NewSignal(cx, false)
		input = Input{TestID: "field",
			LabelFunc: func() string {
				if state.Get() {
					return "Country"
				}
				return "Name"
			},
			AccessibilityDescriptionFunc: func() string {
				if state.Get() {
					return "Choose a country"
				}
				return "Your full name"
			},
			AccessibilityValue: func() AccessibilityValue {
				if state.Get() {
					return AccessibilityValue{Text: "Japan"}
				}
				return AccessibilityValue{Text: "Ada"}
			},
			Expanded: state.Get, Invalid: state.Get,
			AccessibilityActions:  []AccessibilityAction{{Name: "clear", Label: "Clear"}},
			OnAccessibilityAction: func(name string) { actions = append(actions, name) }}
		el = NewElement(cx, input)
		return el
	})
	h.commit()
	id := h.find("field").id
	initial := h.find("field").sent.props
	if initial.Value.Text != "Ada" || !initial.Expandable || !initial.HasAction("clear") {
		t.Fatalf("initial props = %+v", initial)
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, Text: "clear"})
	if !slices.Equal(actions, []string{"clear"}) {
		t.Fatalf("accessibility actions = %v, want clear", actions)
	}
	state.Set(true)
	if k := kinds(h.commit()); k[driver.ItemViewProps] != 0 || k[driver.ItemAccessibilityLabel] != 0 {
		t.Fatalf("changing signals without Set sent %v", k)
	}
	if got := h.find("field").sent.props; !reflect.DeepEqual(got, initial) {
		t.Fatalf("changing signals without Set changed props to %+v", got)
	}
	el.Set(input)
	h.commit()
	if p := h.find("field").sent.props; p.Label != "Country" || p.Description != "Choose a country" ||
		p.Value.Text != "Japan" || !p.Expanded || !p.Invalid || !p.HasAction("clear") {
		t.Fatalf("props after Set = %+v", p)
	}
	el.Set(Input{TestID: "field"})
	b := h.commit()
	if p := h.find("field").sent.props; p.Label != "" || p.Description != "" || p.Value != (AccessibilityValue{}) ||
		p.Expandable || p.Expanded || p.Invalid || len(p.ActionList()) != 0 {
		t.Errorf("removed accessibility props remain after Set: %+v", p)
	}
	if h.find("field").sent.events.Has(driver.EventAccessibilityAction) {
		t.Error("removed accessibility handler remains in the event mask")
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, Text: "clear"})
	if !slices.Equal(actions, []string{"clear"}) {
		t.Errorf("removed accessibility handler ran: %v", actions)
	}
	for _, it := range b.Items {
		if it.Kind == driver.ItemCreate || it.Kind == driver.ItemDelete {
			t.Errorf("Set remounted the field: %v", it.Kind)
		}
	}
}

func TestAGroupReadsItsTexts(t *testing.T) {
	var price *Signal[string]
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		price = NewSignal(cx, "$10")
		return View{Children: []Node{
			View{TestID: "row", Accessible: true, Children: []Node{
				Text{Content: "Apple"},
				View{Children: []Node{Text{ContentFunc: price.Get}}},
				Text{Content: "secret", Style: Style{Display: DisplayNone}},
				Icon{Label: "on sale"},
			}},
			View{TestID: "labelled", Accessible: true, Label: "Own", Children: []Node{Text{Content: "x"}}},
		}}
	})
	props := propsOf(h.commit())
	if p := props[h.find("row").id]; p.Label != "Apple $10 on sale" || p.Accessible != AccessibleYes {
		t.Errorf("group props = %+v, want its texts as the label", p)
	}
	if p := props[h.find("labelled").id]; p.Label != "Own" {
		t.Errorf("a labelled group reads %q", p.Label)
	}
	price.Set("$12")
	b := h.commit()
	found := false
	for _, it := range b.Items {
		if it.Kind == driver.ItemAccessibilityLabel && it.ID == h.find("row").id {
			found = it.Value == "Apple $12 on sale"
		}
	}
	if !found {
		t.Errorf("the group's label did not follow its text: %v", kinds(b))
	}
}

func TestAccessibilityActionsAndFocus(t *testing.T) {
	var got []string
	var off *Signal[bool]
	var ref Ref
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		off = NewSignal(cx, false)
		return View{TestID: "v", Ref: &ref, Disabled: off.Get, Role: RoleAdjustable,
			AccessibilityActions:  []AccessibilityAction{{Name: ActionIncrement}, {Name: ActionDecrement}},
			OnAccessibilityAction: func(name string) { got = append(got, name) }}
	})
	b := h.commit()
	id := h.find("v").id
	for _, it := range b.Items {
		if it.Kind == driver.ItemEvents && it.ID == id && !it.Events.Has(driver.EventAccessibilityAction) {
			t.Error("the view does not listen for accessibility actions")
		}
	}
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, Text: ActionIncrement})
	off.Set(true)
	h.commit()
	h.tree.Dispatch(driver.Event{Kind: driver.EventAccessibilityAction, ID: id, Text: ActionDecrement})
	if !slices.Equal(got, []string{ActionIncrement}) {
		t.Errorf("actions = %v, want the one before the view was disabled", got)
	}
	ref.SetAccessibilityFocus()
	b = h.commit()
	if k := kinds(b); k[driver.ItemAccessibilityFocus] != 1 || b.Items[len(b.Items)-1].ID != id {
		t.Errorf("SetAccessibilityFocus sent %v", k)
	}
	if !driver.IsStandardAction(ActionMagicTap) || driver.IsStandardAction("reset") {
		t.Error("IsStandardAction")
	}
}

func TestStatusAndSearchRoles(t *testing.T) {
	var elems []*Element
	h := newHost(t, Size{Width: 200, Height: 100}, func(cx *Scope) Node {
		root := NewElement(cx, View{})
		elems = []*Element{
			NewElement(cx, View{TestID: "element-status", Role: RoleStatus}),
			NewElement(cx, Text{TestID: "element-text", Role: RoleStatus, Content: "1 item"}),
			NewElement(cx, Input{TestID: "element-search", Role: RoleSearchField}),
			NewElement(cx, Input{TestID: "element-field"}),
		}
		for _, e := range elems {
			root.Insert(e, nil)
		}
		return View{Children: []Node{
			View{TestID: "status", Role: RoleStatus, Children: []Node{Text{Content: "Saved"}}},
			Text{TestID: "text", Role: RoleStatus, Content: "Saved"},
			View{TestID: "assertive", Role: RoleStatus, LiveRegion: LiveRegionAssertive},
			View{TestID: "region", LiveRegion: LiveRegionPolite},
			View{TestID: "plain", Role: RoleGroup},
			Input{TestID: "search", Role: RoleSearchField, Label: "Search"},
			root,
		}}
	})
	props := propsOf(h.commit())
	for id, want := range map[string]struct {
		role Role
		live LiveRegion
	}{
		"status": {RoleStatus, LiveRegionPolite}, "text": {RoleStatus, LiveRegionPolite},
		"assertive": {RoleStatus, LiveRegionAssertive}, "region": {RoleNone, LiveRegionPolite}, "plain": {RoleGroup, LiveRegionNone},
		"search": {RoleSearchField, LiveRegionNone}, "element-status": {RoleStatus, LiveRegionPolite},
		"element-text": {RoleStatus, LiveRegionPolite}, "element-search": {RoleSearchField, LiveRegionNone},
		"element-field": {RoleTextField, LiveRegionNone},
	} {
		if p := props[h.find(id).id]; p.Role != want.role || p.LiveRegion != want.live {
			t.Errorf("%s: role %v, live region %v; want %v, %v", id, p.Role, p.LiveRegion, want.role, want.live)
		}
	}
	// Set again, an element keeps its role and the status's live region.
	elems[1].Set(Text{TestID: "element-text", Role: RoleStatus, Content: "2 items"})
	elems[2].Set(Input{TestID: "element-search", Role: RoleSearchField, Label: "Find"})
	h.commit()
	if p := h.find("element-text").props; p.Role != RoleStatus || p.LiveRegion != LiveRegionPolite {
		t.Errorf("a status text set again: %+v", p)
	}
	if p := h.find("element-search").props; p.Role != RoleSearchField || p.Label != "Find" {
		t.Errorf("a search field set again: %+v", p)
	}
	if got := driver.RoleName(RoleStatus); got != "status" {
		t.Errorf("status selector name = %q", got)
	}
	if got := driver.RoleName(RoleSearchField); got != "searchbox" {
		t.Errorf("search selector name = %q", got)
	}
}
