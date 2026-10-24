package core

import "reflect"

// Native shows a view of a component the app registered with its
// platform by name (nsgo.Component), filling the element's box. Props
// reach the component's SetProp when they change, events it emits reach
// OnEvent, and Ref.Command runs its commands. An unknown name shows an
// empty box.
type Native struct {
	// Component is the registered name.
	Component string
	// Props are the component's own props; PropsFunc, when set, replaces
	// them, again when what it reads changes. A prop that goes away is
	// set to nil. Values cross to Android as JSON.
	Props     map[string]any
	PropsFunc func() map[string]any
	Style     Style
	StyleFunc func() Style

	// OnEvent receives what the component emits.
	OnEvent  func(name string, data any)
	OnPress  func(PressEvent)
	OnLayout func(Bounds)
	Ref      *Ref
	Label    string
	TestID   string
}

// Render implements Node.
func (v Native) Render(*Scope) Node { return v }

// NativeConfig is what ItemNative sends: the component's name and the
// props that changed since the last item, nil for one removed. The first
// item of a view carries all of them.
type NativeConfig struct {
	Component string
	Props     map[string]any
}

type nativeNode struct {
	component string
	props     map[string]any
	sent      map[string]any
	named     bool // the component's name was sent
	size      Size // what the component asked for
	event     func(name string, data any)
}

func (t *Tree) mountNative(cx *Scope, parent *node, v Native) {
	n := t.newNode(KindNative, parent, cx)
	t.bindStyle(cx, n, v.Style, v.StyleFunc)
	t.setRef(n, v.Ref)
	n.h = handlers{press: v.OnPress, layout: v.OnLayout}
	n.props = ViewProps{Label: v.Label, TestID: v.TestID}
	nn := &nativeNode{component: v.Component, props: v.Props, event: v.OnEvent}
	n.native = nn
	if v.PropsFunc != nil {
		cx.renderEffect(func() {
			nn.props = v.PropsFunc()
			t.touch(n)
		})
	}
}

// changes returns the props that differ from those sent, or nil, and
// records them as sent.
func (nn *nativeNode) changes() map[string]any {
	var out map[string]any
	set := func(k string, v any) {
		if out == nil {
			out = map[string]any{}
		}
		out[k] = v
	}
	for k, v := range nn.props {
		if old, ok := nn.sent[k]; !ok || !reflect.DeepEqual(old, v) {
			set(k, v)
		}
	}
	for k := range nn.sent {
		if _, ok := nn.props[k]; !ok {
			set(k, nil)
		}
	}
	if out != nil {
		nn.sent = make(map[string]any, len(nn.props))
		for k, v := range nn.props {
			nn.sent[k] = v
		}
	}
	return out
}

// emitNative adds n's ItemNative when its component or props changed.
func (t *Tree) emitNative(n *node) {
	nn := n.native
	props := nn.changes()
	if nn.named && props == nil {
		return
	}
	nn.named = true
	t.items = append(t.items, MountItem{Kind: ItemNative, ID: n.id,
		Native: &NativeConfig{Component: nn.component, Props: props}})
}

// dispatchNative handles a native view's events.
func (t *Tree) dispatchNative(n *node, ev Event) bool {
	nn := n.native
	switch ev.Kind {
	case EventNative:
		if nn.event != nil {
			t.within(n.owner, func() { nn.event(ev.Text, ev.Data) })
		}
	case EventNativeSize:
		if ev.Size != nn.size {
			nn.size = ev.Size
			t.relayout(n)
		}
	default:
		return false
	}
	return true
}

// Command has the component of the native view r names run name with
// args.
func (r *Ref) Command(name string, args any) {
	if r.n == nil || r.n.deleted || r.n.native == nil {
		return
	}
	r.t.commands = append(r.t.commands, MountItem{Kind: ItemNativeCommand, ID: r.n.id, Value: name, Data: args})
	r.t.changed = true
}

// NativeComponents are the components an app registers with its
// platform, by name.
type NativeComponents map[string]NativeFactory

// NativeFactory makes a component's view for one Native element, on the
// platform's main thread; nil leaves the box empty.
type NativeFactory func(cx *NativeContext) NativeComponent

// NativeComponent is one Native element's platform view. Its methods run
// on the main thread.
type NativeComponent interface {
	// View is the view that fills the box: an NSView or UIView pointer as
	// a uintptr on Apple platforms, a child HWND on Windows, a GtkWidget
	// pointer on Linux, a DOM element (syscall/js.Value) on the web, and
	// on Android the name of a View class the runtime builds by
	// reflection.
	View() any
	// SetProp sets a prop; nil removes it.
	SetProp(name string, value any)
	// Command runs a command Ref.Command sent.
	Command(name string, args any)
	// Close releases the component when its element goes.
	Close()
}

// NativeContext is how a component talks back to its element. Its
// methods may be called from any goroutine.
type NativeContext struct {
	emit func(name string, data any)
	size func(Size)
}

// NewNativeContext returns the context a platform hands a factory: emit
// reports an event, size the size the component asks for.
func NewNativeContext(emit func(name string, data any), size func(Size)) *NativeContext {
	return &NativeContext{emit: emit, size: size}
}

// Emit sends an event to the element's OnEvent.
func (c *NativeContext) Emit(name string, data any) { c.emit(name, data) }

// SetSize asks for a size: the element measures to it where its style
// leaves a dimension open, as an image does to its natural size.
func (c *NativeContext) SetSize(s Size) { c.size(s) }
