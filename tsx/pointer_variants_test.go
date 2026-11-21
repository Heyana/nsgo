package tsx_test

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
)

func TestPointerVariantsFromTSX(t *testing.T) {
	app := run(t, `import { mount, View } from "nsgo";
function App() {
    return <View>
        <View testID="box" class="h-9 bg-white pointer-coarse:h-11 pointer-fine:h-7 dark:pointer-fine:bg-black" />
    </View>;
}
mount(App);`)
	// Default headless environment is phone (coarse).
	if got := app.Find("id:box").Frame.Size.Height; got != 44 {
		t.Fatalf("phone height = %v, want 44", got)
	}
	app.SetEnvironment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Dark: true})
	box := app.Find("id:box")
	if box.Frame.Size.Height != 28 || box.Style.Background != core.RGB(0) {
		t.Fatalf("desktop dark: height=%v bg=%#x, want 28 and black", box.Frame.Size.Height, box.Style.Background)
	}
	app.SetEnvironment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomTV})
	if got := app.Find("id:box").Frame.Size.Height; got != 36 {
		t.Fatalf("tv height = %v, want 36", got)
	}
}
