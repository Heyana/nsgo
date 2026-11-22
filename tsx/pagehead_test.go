package tsx_test

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx"
)

func TestPageHeadModuleAndDocumentProps(t *testing.T) {
	app := run(t, `import {mount, signal, View, Text} from "nsgo";
import {Head} from "nsgo/web";
function App() {
  const next = signal(false);
  return <View>
    <Head title={next() ? "Next page" : "First page"} description="Summary" />
    <Text testID="heading" role="header" accessibilityLevel={next() ? 6 : 1}>Heading</Text>
    <View testID="link" role="link" web={{href: next() ? "/two" : "/one"}}><Text>Link</Text></View>
    <View testID="flip" onPress={() => next.set(true)}><Text>Next</Text></View>
  </View>;
}
mount(App);`, nsgotest.Environment(core.Environment{OS: core.OSWeb, CanSetPageHead: true}))
	if app.Find("id:heading").Props.Level != 1 || app.Find("id:link").Props.Href != "/one" {
		t.Fatal("document semantics did not reach core")
	}
	app.Press("id:flip")
	if app.Find("id:heading").Props.Level != 6 || app.Find("id:link").Props.Href != "/two" {
		t.Fatal("reactive document semantics did not update")
	}
	decls := tsx.Declarations()
	if !strings.Contains(decls["types/nsgo/web.d.ts"], "HeadProps") || !strings.Contains(decls["types/nsgo/types.d.ts"], "accessibilityLevel?: number") {
		t.Fatal("document declarations are missing")
	}
}
