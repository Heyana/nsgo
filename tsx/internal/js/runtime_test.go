package js

import (
	"reflect"
	"testing"
)

func TestRuntimeMethodSet(t *testing.T) {
	checkMethodSet(t, reflect.TypeFor[*Runtime](), reflect.TypeFor[runtimeAPI]())
}

func TestObjectMethodSet(t *testing.T) {
	checkMethodSet(t, reflect.TypeFor[*Object](), reflect.TypeFor[objectAPI]())
}

func checkMethodSet(t *testing.T, implementation, api reflect.Type) {
	t.Helper()
	for i := range implementation.NumMethod() {
		method := implementation.Method(i)
		if _, ok := api.MethodByName(method.Name); !ok {
			t.Errorf("%v.%s is outside the shared API", implementation, method.Name)
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
