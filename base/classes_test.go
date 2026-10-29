package base_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"strings"
	"testing"

	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/nsgotest"
)

// TestEveryPartStyleHasAClass checks that each style a component takes
// for a part has the part's class beside it, as NSH pairs them: Style and
// Class, PopupStyle and PopupClass, Styles and Classes.
func TestEveryPartStyleHasAClass(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi fs.FileInfo) bool { return !strings.HasSuffix(fi.Name(), "_test.go") }, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range pkgs["base"].Files {
		ast.Inspect(f, func(n ast.Node) bool {
			spec, ok := n.(*ast.TypeSpec)
			if !ok || !spec.Name.IsExported() {
				return true
			}
			st, ok := spec.Type.(*ast.StructType)
			if !ok {
				return true
			}
			fields := map[string]string{}
			for _, fl := range st.Fields.List {
				for _, name := range fl.Names {
					fields[name.Name] = types.ExprString(fl.Type)
				}
			}
			for name, typ := range fields {
				var class, want string
				switch {
				case strings.HasSuffix(name, "Styles") && !strings.HasPrefix(typ, "func"):
					class, want = strings.TrimSuffix(name, "Styles")+"Classes", strings.ReplaceAll(typ, "Styles", "Classes")
				case strings.HasSuffix(name, "Style") && strings.Contains(typ, "core.Style"):
					class, want = strings.TrimSuffix(name, "Style")+"Class", strings.ReplaceAll(typ, "core.Style", "string")
				default:
					continue
				}
				if got, ok := fields[class]; !ok || got != want {
					t.Errorf("%s.%s is a %s without %s %s beside it", spec.Name.Name, name, typ, class, want)
				}
			}
			return true
		})
	}
}

func TestClassesStyleTheirParts(t *testing.T) {
	var open *core.Signal[bool]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		checked := core.NewSignal(cx, false)
		open = core.NewSignal(cx, false)
		return core.View{Children: []core.Node{
			base.Button{TestID: "button", Class: "w-20 p-2", Style: core.Style{Width: core.Pt(30)}},
			base.Checkbox{TestID: "check", Checked: checked.Get, OnChange: checked.Set,
				IndicatorClass: "w-2.5 h-2.5 checked:w-3"},
			base.Popover{TestID: "popover", Open: open.Get, Trigger: core.Text{Content: "More"},
				PopupClass: "w-24", Children: []core.Node{core.Text{Content: "Inside"}}},
			core.View{Style: core.Style{Height: core.Pt(40), Direction: core.Row}, Children: []core.Node{
				base.ResizeHandle{TestID: "handle", Class: "w-[3px]!", ThumbClass: "w-1 h-6"}}},
		}}
	})
	if w := app.Find("id:button").Frame.Size.Width; w != 30 {
		t.Errorf("the button is %v wide, want its style's width over its class's", w)
	}
	if p := app.Find("id:button").Frame.Size.Height; p != 16 {
		t.Errorf("the button is %v tall, want its class's padding", p)
	}
	indicator := func() core.Points { return app.Find("id:check").Children[0].Frame.Size.Width }
	if w := indicator(); w != 10 {
		t.Errorf("the indicator is %v wide", w)
	}
	app.Press("id:check")
	if w := indicator(); w != 12 {
		t.Errorf("checked, the indicator is %v wide", w)
	}
	open.Set(true)
	app.Settle()
	if w := app.Find("text:Inside").Parent.Frame.Size.Width; w != 96 {
		t.Errorf("the popup is %v wide", w)
	}
	band := app.Find("id:handle")
	if line := band.Children[0].Frame.Size.Width; line != 3 {
		t.Errorf("the handle's line is %v wide, want its important class's", line)
	}
	if len(band.Children) < 2 {
		t.Fatal("the handle has no thumb with only a ThumbClass")
	}
}

func TestToastStateClassesWinOverItsStyle(t *testing.T) {
	var status *core.Signal[base.ToastStatus]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		status = core.NewSignal(cx, base.ToastPresent)
		return base.Toast{TestID: "toast", Status: status.Get, Class: "h-10 opacity-90", Style: core.Style{Opacity: core.Alpha(0.8)},
			EndingClass: "opacity-50"}
	})
	if o := app.Find("id:toast").Style.Opacity; o != 0.8 {
		t.Errorf("shown, the toast's opacity is %v, want its style's", o)
	}
	status.Set(base.ToastEnding)
	app.Settle()
	if o := app.Find("id:toast").Style.Opacity; o != 0.5 {
		t.Errorf("ending, the toast's opacity is %v, want its EndingClass's", o)
	}
}
