package js

import (
	"reflect"
	"testing"
)

func TestRuntimeMethodSet(t *testing.T) {
	api := reflect.TypeFor[runtimeAPI]()
	runtime := reflect.TypeFor[*Runtime]()
	for i := range runtime.NumMethod() {
		method := runtime.Method(i)
		if _, ok := api.MethodByName(method.Name); !ok {
			t.Errorf("%s is outside the shared runtime API", method.Name)
		}
	}
}

func TestRuntimeBoxesValuesInItsRealm(t *testing.T) {
	first, second := newRuntime(t), newRuntime(t)
	run(t, first, "String.prototype.realm = 'first'")
	run(t, second, "String.prototype.realm = 'second'")
	for _, test := range []struct {
		runtime *Runtime
		realm   string
	}{{first, "first"}, {second, "second"}} {
		r := test.runtime
		box := r.ToObject(r.ToValue("text"))
		if got := box.Get("realm").String(); got != test.realm {
			t.Errorf("boxed string uses realm %q, want %q", got, test.realm)
		}
	}
}
