//go:build !js

package tsx

import (
	"errors"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

func TestPluginKeepsIndependentPromiseJobsAfterBudgetCut(t *testing.T) {
	for _, tc := range []struct {
		name          string
		order         [2]string
		dispatchFails bool
	}{
		{"cutting job first", [2]string{"cancelWork", "waitForValue"}, false},
		{"continuation first", [2]string{"waitForValue", "cancelWork"}, false},
		{"cutting job first after a throw", [2]string{"cancelWork", "waitForValue"}, true},
		{"continuation first after a throw", [2]string{"waitForValue", "cancelWork"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			type result struct {
				value any
				err   error
			}
			results := map[string][]result{}
			discarded := 0
			app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
				r := newRuntime(cx)
				r.sandbox = &sandboxed{}
				r.limits.Call = time.Second
				r.keepPromiseJobs()
				// Interrupt at a fixed point instead of depending on CPU timing.
				r.vm.Set("cancelTestJob", func() { r.vm.Interrupt(ErrBudget) })
				r.vm.Set("recordDiscarded", func() { discarded++ })
				r.vm.Set("dispatchFails", tc.dispatchFails)
				v, err := r.vm.RunString(`
					let release
					const wait = new Promise(resolve => { release = resolve })
					;({
						waitForValue() { return wait.then(() => "ready") },
						cancelWork() {
							const result = wait.then(() => { cancelTestJob(); return "unreached" })
							wait.then(recordDiscarded)
							return result
						},
						dispatch() {
							release();
							if (dispatchFails) {
								const error = new Error("dispatch refused");
								error.code = "dispatchRefused";
								throw error;
							}
							return "sent";
						},
					})
				`)
				if err != nil {
					t.Fatal(err)
				}
				p := &Plugin{r: r, cx: cx, exports: v.(*js.Object)}
				for _, name := range []string{tc.order[0], tc.order[1], "dispatch"} {
					p.Call(name, nil, func(v any, err error) {
						results[name] = append(results[name], result{v, err})
					})
				}
				return core.View{}
			})
			app.WaitUntil(time.Second, func() bool { return len(results) == 3 })
			app.Advance(time.Second)
			if got := results["cancelWork"]; len(got) != 1 || !errors.Is(got[0].err, ErrBudget) {
				t.Fatalf("cancelled call = %+v", got)
			}
			if got := results["waitForValue"]; len(got) != 1 || got[0].err != nil || got[0].value != "ready" {
				t.Fatalf("independent call = %+v", got)
			}
			got := results["dispatch"]
			if len(got) != 1 {
				t.Fatalf("dispatching call = %+v, want one result", got)
			}
			if tc.dispatchFails {
				if got[0].err == nil || got[0].err.Error() != "dispatch refused" || core.ErrorCode(got[0].err) != "dispatchRefused" {
					t.Fatalf("dispatching call = %+v, want the original error", got)
				}
			} else if got[0].err != nil || got[0].value != "sent" {
				t.Fatalf("dispatching call = %+v, want sent", got)
			}
			if discarded != 0 {
				t.Fatal("a queued job ran after its call was cancelled")
			}
		})
	}
}
