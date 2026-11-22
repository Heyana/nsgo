//go:build ios && nsgotest

package ios

import (
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

// NativeEditMenuLabelsForTest reads visible UIKit menu views in this app's
// windows, rather than the unfiltered suggested actions or NSGo's menu model.
func NativeEditMenuLabelsForTest() (labels []string) {
	onMainSync(func() {
		var visit func(objc.ID, bool)
		visit = func(view objc.ID, menu bool) {
			if objc.Send[bool](view, sel("isHidden")) || objc.Send[float64](view, sel("alpha")) <= 0 {
				return
			}
			name := apple.GoString(objc.ID(view.Class()).Send(sel("description")))
			menu = menu || strings.Contains(name, "EditMenu") || strings.Contains(name, "ContextMenu")
			if menu {
				if objc.Send[bool](view, sel("isKindOfClass:"), apple.Class("UILabel")) {
					if text := apple.GoString(view.Send(sel("text"))); text != "" {
						labels = append(labels, text)
					}
				}
				if text := apple.GoString(view.Send(sel("accessibilityLabel"))); text != "" {
					labels = append(labels, text)
				}
			}
			apple.ForEach(view.Send(sel("subviews")), func(child objc.ID) { visit(child, menu) })
		}
		app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
		apple.ForEach(app.Send(sel("windows")), func(window objc.ID) { visit(window, false) })
	})
	return
}

// ChooseNativeEditActionForTest invokes the menu's standard responder action
// on its actual input after a test has verified UIKit visibly offered it.
func ChooseNativeEditActionForTest(action driver.EditAction) (chosen bool) {
	onMainSync(func() {
		var command objc.SEL
		switch action {
		case driver.EditCopy:
			command = sel("copy:")
		case driver.EditCut:
			command = sel("cut:")
		case driver.EditSelectAll:
			command = sel("selectAll:")
		default:
			return
		}
		for _, v := range runner.views {
			if v != nil && v.input != nil && v.input.editMenu&action != 0 {
				v.input.field.Send(command, objc.ID(0))
				chosen = true
				break
			}
		}
	})
	return
}
