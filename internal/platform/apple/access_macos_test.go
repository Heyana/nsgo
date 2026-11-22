//go:build darwin && !ios

package apple

import (
	"reflect"
	"testing"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestAccessibleTextTraversesVisibleWrappersAndStopsAtText(t *testing.T) {
	onMain(func() {
		type node struct {
			children     objc.ID
			name         string
			text, hidden bool
		}
		nodes := map[objc.ID]node{}
		class := RegisterClass("NsgoTestAccessibleTextNode", Class("NSObject"), nil, []objc.MethodDef{
			{Cmd: sel("subviews"), Fn: func(self objc.ID, _ objc.SEL) objc.ID { return nodes[self].children }},
			{Cmd: sel("isHidden"), Fn: func(self objc.ID, _ objc.SEL) bool { return nodes[self].hidden }},
			{Cmd: sel("name"), Fn: func(self objc.ID, _ objc.SEL) objc.ID { return NSString(nodes[self].name) }},
		})
		newNode := func(name string, text, hidden bool, children ...objc.ID) objc.ID {
			array := objc.ID(Class("NSMutableArray")).Send(sel("new"))
			for _, child := range children {
				array.Send(sel("addObject:"), child)
			}
			view := objc.ID(class).Send(sel("new"))
			nodes[view] = node{array, name, text, hidden}
			return view
		}
		defer func() {
			for view, node := range nodes {
				node.children.Send(selRelease)
				view.Send(selRelease)
			}
		}()
		first := newNode("First", true, false)
		ignored := newNode("Ignored", true, false)
		empty := newNode("", true, false, ignored)
		hidden := newNode("", false, true, ignored)
		last := newNode("Last", true, false)
		wrapper := newNode("", false, false, empty, last)
		root := newNode("", false, false, first, hidden, wrapper)
		var got []string
		CollectText(root, &got, func(view objc.ID) (string, bool) { n := nodes[view]; return n.name, n.text })
		if !reflect.DeepEqual(got, []string{"First", "Last"}) {
			t.Fatalf("spoken text = %v", got)
		}
		action := newNode("Read it", false, false)
		var performed []string
		perform := func(name string) bool { performed = append(performed, name); return true }
		actions := []driver.AccessibilityAction{{Name: "read", Label: "Read it"}, {Name: "fallback"}}
		if !CustomAccessibilityAction(action, actions, perform) {
			t.Fatal("localized action did not resolve its stable name")
		}
		n := nodes[action]
		n.name = "fallback"
		nodes[action] = n
		if !CustomAccessibilityAction(action, actions, perform) {
			t.Fatal("an unlabelled action did not use its name")
		}
		n.name = "unknown"
		nodes[action] = n
		if CustomAccessibilityAction(action, actions, perform) || !reflect.DeepEqual(performed, []string{"read", "fallback"}) {
			t.Fatal("an unknown action was dispatched or the spoken label was sent instead of its name")
		}
	})
}
