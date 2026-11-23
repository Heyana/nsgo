package main

import (
	"regexp"
	"strings"
	"testing"

	"github.com/nsgo-dev/goja"
)

// runNodeComponent runs a host's nsgo-node component with Component and require stubbed, with self an instance of it whose node is node (JS), the sends it makes recorded in sent.
func runNodeComponent(t *testing.T, host, node string) *goja.Runtime {
	t.Helper()
	src, err := miniprogramRuntime.ReadFile("miniprogram/" + host + "/components/nsgo-node/index.js")
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	if _, err := vm.RunString(`var def; function Component(d) { def = d }; function require() { return {} }`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(string(src)); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(`var sent = [], node = ` + node + `
var self = Object.assign({ data: { node }, props: { node, onNsgo() {} }, sent,
  triggerEvent(_, detail) { sent.push(detail) }, setData() { throw new Error('setData') } }, def.methods)
self.send = function (...detail) { sent.push(detail) }`); err != nil {
		t.Fatal(err)
	}
	return vm
}

// TestMiniProgramReadonlyInputsRevertEdits holds a read-only field to the value Go holds on each host: WeChat's single-line input stays enabled and its bindinput returns that value, which WeChat puts back in the input, while its textarea, whose bindinput return WeChat ignores, and Alipay's fields, whose onInput has no such return, are disabled. Neither sends Go the edit.
func TestMiniProgramReadonlyInputsRevertEdits(t *testing.T) {
	const readonly = `{ id: 4, v: 'copy me', fd: { ro: true } }`
	wechat := runNodeComponent(t, "wechat", readonly)
	got, err := wechat.RunString(`String(self.onInput({ detail: { value: 'copy meX' } })) + '|' + JSON.stringify(sent)`)
	if err != nil {
		t.Fatal(err)
	}
	if got.String() != "copy me|[]" {
		t.Errorf("WeChat read-only input returned and sent %s, want the held value and nothing sent", got)
	}
	if got, err = wechat.RunString(`sent.length = 0; self.onConfirm({ detail: { value: 'copy meX' } }); JSON.stringify(sent)`); err != nil {
		t.Fatal(err)
	}
	if got.String() != `[["confirm",4,"copy me"]]` {
		t.Errorf("WeChat read-only confirm sent %s, want the held value", got)
	}
	editable := runNodeComponent(t, "wechat", `{ id: 4, v: 'a', fd: {} }`)
	if got, err = editable.RunString(`String(self.onInput({ detail: { value: 'ab' } })) + '|' + JSON.stringify(sent)`); err != nil {
		t.Fatal(err)
	}
	if got.String() != `undefined|[["input",4,"ab"]]` {
		t.Errorf("WeChat editable input returned and sent %s", got)
	}
	alipay := runNodeComponent(t, "alipay", readonly)
	if got, err = alipay.RunString(`self.onInput({ detail: { value: 'copy meX' } }); JSON.stringify(sent)`); err != nil {
		t.Fatal(err)
	}
	if got.String() != "[]" {
		t.Errorf("Alipay read-only input sent %s", got)
	}

	disabled := regexp.MustCompile(`disabled="\{\{([^}]*)\}\}"`)
	for _, c := range []struct {
		file            string
		textarea, input bool
	}{
		{"miniprogram/wechat/components/nsgo-node/index.wxml", true, false},
		{"miniprogram/alipay/components/nsgo-node/index.axml", true, true},
	} {
		src, err := miniprogramRuntime.ReadFile(c.file)
		if err != nil {
			t.Fatal(err)
		}
		markup := string(src)
		i, j := strings.Index(markup, "<textarea"), strings.Index(markup, "<input")
		if i < 0 || j < i {
			t.Fatalf("%s has no textarea before its input", c.file)
		}
		for _, f := range []struct {
			name, part string
			ro         bool
		}{{"textarea", markup[i:j], c.textarea}, {"input", markup[j:], c.input}} {
			m := disabled.FindStringSubmatch(f.part)
			if m == nil {
				t.Fatalf("%s's %s has no disabled binding", c.file, f.name)
			}
			if strings.Contains(m[1], "node.fd.ro") != f.ro {
				t.Errorf("%s's %s disabled=%q, want read-only disabling it: %v", c.file, f.name, m[1], f.ro)
			}
		}
	}
}
