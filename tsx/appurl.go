package tsx

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

func (r *runtime) urlScheme() js.Value {
	o := r.vm.NewObject()
	_ = o.Set("url", r.accessor(func() js.Value {
		value := r.cx.AppURL()
		if value == "" {
			return js.Null()
		}
		return r.vm.ToValue(value)
	}))
	_ = o.Set("setUrl", r.function(func(call js.FunctionCall) js.Value {
		opts := r.options(arg(call, 1))
		history := opts.str("history", "replace")
		var behavior core.HistoryBehavior
		switch history {
		case "replace":
		case "push":
			behavior = core.HistoryPush
		default:
			panic(r.vm.NewTypeError("UrlScheme.setUrl: history must be replace or push"))
		}
		if err := r.cx.SetAppURL(arg(call, 0).String(), core.AppURLOptions{History: behavior}); err != nil {
			panic(r.goError(err))
		}
		return js.Undefined()
	}))
	_ = o.Set("on", r.function(func(call js.FunctionCall) js.Value {
		if arg(call, 0).String() != "change" || !isFunction(arg(call, 1)) {
			panic(r.vm.NewTypeError("UrlScheme.on expects change and a listener"))
		}
		callback := arg(call, 1)
		stop := r.cx.OnAppURLChange(func(e core.AppURLChange) {
			r.inJS(func() {
				out := r.vm.NewObject()
				_ = out.Set("url", e.URL)
				_ = out.Set("reason", e.Reason)
				r.call(callback, out)
			})
		})
		return r.function(func(js.FunctionCall) js.Value { stop(); return js.Undefined() })
	}))
	return o
}
