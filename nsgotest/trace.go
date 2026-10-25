package nsgotest

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/oneo-me/nsgo/internal/driver"
)

// traceBatch prints batch b, which the step running produced in a commit
// that took took, as `nsgo test -trace` shows it: a line naming the step,
// the items and the commit's time, then a line per item.
func (a *App) traceBatch(b *driver.MountBatch, took time.Duration) {
	if a.trace == nil {
		return
	}
	printBatch(a.trace, a.stepName, b, took)
}

func printBatch(w io.Writer, step string, b *driver.MountBatch, took time.Duration) {
	fmt.Fprintf(w, "── %s: %d items, commit %.2f ms\n", step, len(b.Items), float64(took.Microseconds())/1000)
	for i := range b.Items {
		fmt.Fprintf(w, "   %s\n", itemLine(&b.Items[i]))
	}
}

var itemNames = map[driver.ItemKind]string{
	driver.ItemCreate: "create", driver.ItemInsert: "insert", driver.ItemRemove: "remove", driver.ItemDelete: "delete",
	driver.ItemFrame: "frame", driver.ItemViewStyle: "style", driver.ItemText: "text", driver.ItemInput: "input",
	driver.ItemInputValue: "value", driver.ItemEvents: "events", driver.ItemViewProps: "props",
	driver.ItemScrollConfig: "scroll-config", driver.ItemScrollContent: "scroll-content", driver.ItemFocus: "focus",
	driver.ItemBlur: "blur", driver.ItemImage: "image", driver.ItemModal: "modal", driver.ItemAlert: "alert",
	driver.ItemDismissKeyboard: "dismiss-keyboard", driver.ItemScrollTo: "scroll-to", driver.ItemCanvas: "canvas",
	driver.ItemSetClipboard: "set-clipboard", driver.ItemReadClipboard: "read-clipboard", driver.ItemAnnounce: "announce",
	driver.ItemKeymap: "keymap", driver.ItemStatusBar: "status-bar", driver.ItemTitle: "title",
	driver.ItemBackUnhandled: "back-unhandled", driver.ItemWindow: "window", driver.ItemMenu: "menu", driver.ItemWeb: "web",
	driver.ItemWebCommand: "web-command", driver.ItemVideo: "video", driver.ItemVideoSeek: "video-seek",
	driver.ItemNative: "native", driver.ItemNativeCommand: "native-command", driver.ItemAnimation: "animation",
	driver.ItemZoomTo: "zoom-to", driver.ItemImageInfo: "image-info", driver.ItemSelection: "selection",
	driver.ItemScrollBy: "scroll-by", driver.ItemStickyCells: "sticky-cells", driver.ItemAccessibilityLabel: "label",
	driver.ItemAccessibilityFocus: "accessibility-focus", driver.ItemScreen: "screen", driver.ItemOverflow: "overflow",
	driver.ItemBlockNative: "block-native", driver.ItemShader: "shader", driver.ItemSetClipboardImage: "set-clipboard-image",
	driver.ItemOpenURL: "open-url", driver.ItemPickImages: "pick-images", driver.ItemToast: "toast",
	driver.ItemAppearance: "appearance", driver.ItemPopupMenu: "popup-menu", driver.ItemQuit: "quit",
}

// itemLine is one item as traces print it: its kind, its view and what
// it carries that a reader looks for.
func itemLine(it *driver.MountItem) string {
	name := itemNames[it.Kind]
	if name == "" {
		name = fmt.Sprintf("item-%d", it.Kind)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%-14s view:%d", name, it.ID)
	switch it.Kind {
	case driver.ItemCreate:
		fmt.Fprintf(&b, " %s", kindName(it.View))
	case driver.ItemInsert:
		fmt.Fprintf(&b, " into view:%d", it.Parent)
		if it.Before != 0 {
			fmt.Fprintf(&b, " before view:%d", it.Before)
		}
	case driver.ItemFrame:
		f := it.Frame
		fmt.Fprintf(&b, " %g,%g %gx%g", f.Origin.X, f.Origin.Y, f.Size.Width, f.Size.Height)
	case driver.ItemText:
		fmt.Fprintf(&b, " %q", it.Text.Text)
	case driver.ItemInputValue, driver.ItemTitle, driver.ItemToast, driver.ItemAnnounce, driver.ItemAccessibilityLabel,
		driver.ItemSetClipboard, driver.ItemOpenURL:
		fmt.Fprintf(&b, " %q", it.Value)
	case driver.ItemViewProps:
		if p := it.ViewProps; p.TestID != "" || p.Label != "" {
			fmt.Fprintf(&b, " id=%q label=%q", p.TestID, p.Label)
		}
	case driver.ItemScrollTo, driver.ItemScrollBy:
		fmt.Fprintf(&b, " %g,%g", it.Point.X, it.Point.Y)
	case driver.ItemScrollContent:
		fmt.Fprintf(&b, " %gx%g", it.Size.Width, it.Size.Height)
	case driver.ItemAnimation:
		fmt.Fprintf(&b, " %d ops", len(it.Animation))
	}
	return b.String()
}
