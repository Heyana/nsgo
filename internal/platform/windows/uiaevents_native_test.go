//go:build windows

package windows

import (
	"fmt"
	"math"
	"runtime"
	"slices"
	"sync"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A UI Automation client, as Narrator is one: UIAutomationClient.h's IUIAutomation, IUIAutomationElement and IUIAutomationElementArray, and a property-changed event handler made here. The handler's VARIANT comes by value, which x64 and arm64 pass as a pointer to a copy and 386 pushes, four words (propertyChangedSlot).
var (
	clsidCUIAutomation                    = parseGUID("ff48dba4-60ef-4201-aa87-54103eef594e")
	iidUIAutomation                       = parseGUID("30cbe57d-d9d0-452a-ab13-7ac5ac4825ee")
	iidUIAutomationPropertyChangedHandler = parseGUID("40cd37d4-c756-4b0c-8c6f-bddfeeb13b50")
)

const (
	uiaElementFromHandle                       = 6
	uiaCreateTrueCondition                     = 21
	uiaAddPropertyChangedEventHandlerNativeArr = 34
	uiaRemoveAllEventHandlers                  = 41
	uiaElementFindAll                          = 6
	uiaArrayLength                             = 3
	treeScopeDescendants                       = 4
	treeScopeSubtree                           = 7
)

// propertyEvent is one property-changed event a client heard: the property and its new value.
type propertyEvent struct {
	property int32
	value    string // a BSTR's text, an integer's, a double's or a VARIANT_BOOL's in decimal
}

// propertyHandler is an IUIAutomationPropertyChangedEventHandler: its vtable first, as COM reads it.
type propertyHandler struct {
	vtbl *[4]uintptr
	mu   sync.Mutex
	got  []propertyEvent
}

var propertyHandlerVtbl = [4]uintptr{
	syscall.NewCallback(func(this *propertyHandler, iid *guid, out *uintptr) uintptr {
		if *iid == iidUnknown || *iid == iidUIAutomationPropertyChangedHandler {
			*out = uintptr(unsafe.Pointer(this))
			return sOK
		}
		*out = 0
		return eNoInterface
	}),
	syscall.NewCallback(func(this *propertyHandler) uintptr { return 1 }), // the test owns it
	syscall.NewCallback(func(this *propertyHandler) uintptr { return 1 }),
	propertyChangedSlot(),
}

// heard records the event of property with its new value.
func (h *propertyHandler) heard(property int32, value *variant) {
	e := propertyEvent{property: property}
	switch value.vt {
	case vtBSTR:
		e.value = wideString(*(**uint16)(unsafe.Pointer(&value.val)))
	case vtI4:
		e.value = fmt.Sprint(int32(uint32(value.val)))
	case vtR8:
		e.value = fmt.Sprint(math.Float64frombits(value.val))
	case vtBool:
		e.value = fmt.Sprint(int16(uint16(value.val)))
	default:
		e.value = fmt.Sprintf("vt %d", value.vt)
	}
	h.mu.Lock()
	h.got = append(h.got, e)
	h.mu.Unlock()
}

func (h *propertyHandler) events() []propertyEvent {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.got)
}

func uiaPropertyEventsApp(title string) func(*core.Scope) core.Node {
	return func(cx *core.Scope) core.Node {
		changed := core.NewSignal(cx, false)
		cx.Effect(func() { cx.SetTitle(fmt.Sprintf("%s|%v", title, changed.Get())) })
		cx.OnKeyWith("mod+n", core.KeyOptions{When: core.KeyAlways}, func() { changed.Set(true) })
		pick := func(before, after string) func() string {
			return func() string {
				if changed.Get() {
					return after
				}
				return before
			}
		}
		box := core.Style{Width: core.Pt(120), Height: core.Pt(24)}
		return core.View{Style: core.Style{Width: core.Pt(240), Height: core.Pt(400)}, Children: []core.Node{
			core.View{TestID: "play", Role: core.RoleButton, Style: box, LabelFunc: pick("Play", "Pause"), OnPress: func(core.PressEvent) {}},
			core.View{TestID: "grouped", Role: core.RoleButton, Style: box, OnPress: func(core.PressEvent) {},
				Children: []core.Node{core.Text{ContentFunc: pick("Start", "Stop")}}},
			core.View{TestID: "described", Role: core.RoleButton, Style: box, Label: "Info", AccessibilityDescriptionFunc: pick("before", "after")},
			core.View{TestID: "switch", Role: core.RoleSwitch, Style: box, Label: "Wi-Fi", Checked: changed.Get},
			core.View{TestID: "disclosure", Role: core.RoleButton, Style: box, Label: "More", Expanded: changed.Get},
			core.View{TestID: "tab", Role: core.RoleTab, Style: box, Label: "Tab", Selected: changed.Get},
			core.View{TestID: "slider", Role: core.RoleAdjustable, Style: box, Label: "Volume", AccessibilityValue: func() core.AccessibilityValue {
				if changed.Get() {
					return core.AccessibilityValue{Max: 100, Now: 60}
				}
				return core.AccessibilityValue{Max: 100, Now: 20}
			}},
			core.Input{TestID: "field", Style: box, Value: pick("", "set by the app")},
			core.Input{TestID: "typed", Style: box},
			core.Input{TestID: "secret", Style: box, Masked: true, Value: pick("", "hunter2")},
		}}
	}
}

// TestUIAPropertyChangedEvents listens as Narrator does, with IUIAutomation::AddPropertyChangedEventHandler on the window's subtree once it has seen every element, and checks that changing the elements' labels, a text a button groups, a description, toggle, expanded and selected states, a range's value and fields' text raises the matching events, the user's typing too, and that a masked field's text is not sent.
func TestUIAPropertyChangedEvents(t *testing.T) {
	if testing.Short() {
		t.Skip("opens a window")
	}
	const title = "nsgo-uia-property-events"
	hwnd := startApp(t, title, uiaPropertyEventsApp(title), 240, 400)
	waitForTitle(t, hwnd, title+"|false", 5*time.Second)

	// The client runs on a thread of its own, in the multithreaded apartment: the window's thread answers its calls.
	handler := &propertyHandler{vtbl: &propertyHandlerVtbl}
	listening, stop, stopped := make(chan error, 1), make(chan struct{}), make(chan struct{})
	go func() {
		defer close(stopped)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		const coinitMultithreaded = 0
		coInitializeEx.Call(0, coinitMultithreaded)
		defer coUninitialize.Call()
		var automation, root, condition, all *comObject
		if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidCUIAutomation)), 0, clsctxInprocServer,
			uintptr(unsafe.Pointer(&iidUIAutomation)), uintptr(unsafe.Pointer(&automation))); failed(hr) {
			listening <- hresultError("CoCreateInstance(CUIAutomation)", hr)
			return
		}
		defer releaseAll(&all, &condition, &root, &automation)
		if hr := automation.call(uiaElementFromHandle, hwnd, uintptr(unsafe.Pointer(&root))); failed(hr) || root == nil {
			listening <- hresultError("ElementFromHandle", hr)
			return
		}
		// Reading the tree gives the client every element, as Narrator holds the ones it reads.
		if hr := automation.call(uiaCreateTrueCondition, uintptr(unsafe.Pointer(&condition))); failed(hr) {
			listening <- hresultError("CreateTrueCondition", hr)
			return
		}
		if hr := root.call(uiaElementFindAll, treeScopeDescendants, condition.ptr(), uintptr(unsafe.Pointer(&all))); failed(hr) || all == nil {
			listening <- hresultError("FindAll", hr)
			return
		}
		properties := []int32{propName, propFullDescription, propToggleState, propExpandCollapseState,
			propSelectionItemSelected, propRangeValueValue, propValueValue}
		if hr := automation.call(uiaAddPropertyChangedEventHandlerNativeArr, root.ptr(), treeScopeSubtree, 0,
			uintptr(unsafe.Pointer(handler)), uintptr(unsafe.Pointer(&properties[0])), uintptr(len(properties))); failed(hr) {
			listening <- hresultError("AddPropertyChangedEventHandlerNativeArray", hr)
			return
		}
		listening <- nil
		<-stop
		automation.call(uiaRemoveAllEventHandlers)
	}()
	defer func() {
		close(stop)
		select {
		case <-stopped:
		case <-time.After(10 * time.Second):
			t.Error("the client did not remove its handler")
		}
	}()
	select {
	case err := <-listening:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the client did not start listening")
	}

	byTestID := func(w *window, id string) *view {
		for _, v := range w.views {
			if v != nil && !v.deleted && v.props.TestID == id {
				return v
			}
		}
		return nil
	}
	onMain(t, hwnd, func(w *window) {
		if !uiaListening() {
			t.Error("UiaClientsAreListening is false with a handler registered")
		}
		for _, id := range []string{"play", "grouped", "switch", "slider", "field", "secret"} {
			if v := byTestID(w, id); v == nil || w.uia[uiaKey(v.id, 0)] == nil {
				t.Errorf("the client was not given %s's element", id)
			}
		}
		for _, b := range w.keys {
			if b.Key == "mod+n" {
				w.engine.Dispatch(driver.Event{Kind: driver.EventKey, Count: b.ID})
			}
		}
	})
	waitForTitle(t, hwnd, title+"|true", 5*time.Second)
	onMain(t, hwnd, func(w *window) {
		if v := byTestID(w, "typed"); v != nil && v.field != nil {
			v.field.insert("hi")
		} else {
			t.Error("no typed field")
		}
	})

	want := []propertyEvent{
		{propName, "Pause"},
		{propName, "Stop"},
		{propFullDescription, "after"},
		{propToggleState, "1"},
		{propExpandCollapseState, "1"},
		{propSelectionItemSelected, "-1"},
		{propRangeValueValue, "60"},
		{propValueValue, "set by the app"},
		{propValueValue, "hi"},
	}
	deadline := time.Now().Add(5 * time.Second)
	var got []propertyEvent
	for {
		got = handler.events()
		missing := slices.DeleteFunc(slices.Clone(want), func(e propertyEvent) bool { return slices.Contains(got, e) })
		if len(missing) == 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("missing events %v; heard %v", missing, got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	for _, e := range got {
		if e.property == propValueValue && e.value == "hunter2" {
			t.Error("the masked field's text was sent")
		}
	}
}
