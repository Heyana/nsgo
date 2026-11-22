package harmony

import (
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/nsgo-dev/goja"
)

// Run the actual ArkTS paint helpers, keeping native layout and Canvas APIs outside the test: native rectangles remain supplied by ArkUI itself.
func TestSpanBackgroundGeometry(t *testing.T) {
	source, err := templates.ReadFile("templates/entry/src/main/ets/pages/SpanBackground.ets")
	if err != nil {
		t.Fatal(err)
	}
	js := api.Transform(string(source), api.TransformOptions{Loader: api.LoaderTS, Format: api.FormatIIFE, GlobalName: "paint"})
	if len(js.Errors) > 0 {
		t.Fatal(js.Errors)
	}
	for _, tc := range []struct{ name, expression string }{
		{"inset band", `JSON.stringify(paint.backgroundBand(10,20,40,40,{c:'red',r:99,t:50,tp:true,b:2})) === JSON.stringify({x:10,y:30,width:30,height:8,radius:4,color:'red'})`},
		{"empty band", `paint.backgroundBand(0,0,20,10,{c:'red',t:60,tp:true,b:5}) === undefined`},
		{"negative inset", `paint.backgroundBand(0,0,20,10,{c:'red',t:-5,r:-2}).height === 10 && paint.backgroundBand(0,0,20,10,{c:'red',r:-2}).radius === 0`},
		{"UTF-16 line range", `JSON.stringify(paint.spanLineRange('😀aa   \nbb',0,8,2,10)) === '{"start":2,"end":4}' && JSON.stringify(paint.spanLineRange('😀aa   \nbb',8,10,2,10)) === '{"start":8,"end":10}'`},
		{"hanging spaces", `paint.spanLineRange('aa   ',0,5,2,5) === undefined`},
		{"fallback fonts", `JSON.stringify(paint.joinSpanBoxes([{left:10,top:1,right:20,bottom:11},{left:30,top:0,right:40,bottom:12},{left:0,top:0,right:10,bottom:12}])) === '[{"left":0,"top":0,"right":20,"bottom":12},{"left":30,"top":0,"right":40,"bottom":12}]'`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			vm := goja.New()
			if _, err := vm.RunString(string(js.Code)); err != nil {
				t.Fatal(err)
			}
			result, err := vm.RunString(tc.expression)
			if err != nil || !result.ToBoolean() {
				t.Fatalf("%s: result %v, error %v", tc.expression, result, err)
			}
		})
	}
}
