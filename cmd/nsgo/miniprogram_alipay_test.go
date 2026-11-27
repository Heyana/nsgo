package main

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/goja"
)

// runAlipayPage loads common/pages/index/index.js on Goja with Page and
// require stubbed, exposing page (the instantiated Page object with working
// setData and recorded events in pageEvents).
func runAlipayPage(t *testing.T) *goja.Runtime {
	t.Helper()
	src, err := miniprogramRuntime.ReadFile("miniprogram/common/pages/index/index.js")
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	if _, err := vm.RunString(`
var pageDef
function Page(d) { pageDef = d }
function require() { return { start() { return Promise.resolve({ event() {}, stop() {} }) } } }
`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(string(src)); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(`
var pageEvents = [], setDataLog = []
var page = Object.assign({}, pageDef, {
  data: Object.assign({}, pageDef.data),
  setData(patch) {
    setDataLog.push(patch)
    Object.assign(this.data, patch)
  },
  app: {
    event(...args) { pageEvents.push(args) },
    stop() {},
  },
})
`); err != nil {
		t.Fatal(err)
	}
	return vm
}

// runAlipayNodeVM loads alipay/components/nsgo-node/index.js on Goja with a
// deterministic fake timer queue (advance(ms)) so keyframes and scroll-end
// timers can be stepped synchronously without real wall-clock sleeps.
func runAlipayNodeVM(t *testing.T) *goja.Runtime {
	t.Helper()
	src, err := miniprogramRuntime.ReadFile("miniprogram/alipay/components/nsgo-node/index.js")
	if err != nil {
		t.Fatal(err)
	}
	vm := goja.New()
	if _, err := vm.RunString(`
var compDef, now = 0, nextTimer = 1, timers = []
function Component(d) { compDef = d }
function require() { return { run() {} } }
function setTimeout(fn, ms) {
  var id = nextTimer++
  timers.push({ id, at: now + (ms || 0), fn })
  return id
}
function clearTimeout(id) {
  timers = timers.filter(function (t) { return t.id !== id })
}
function advance(ms) {
  var target = now + ms
  while (true) {
    var idx = -1
    for (var i = 0; i < timers.length; i++) {
      if (timers[i].at <= target && (idx < 0 || timers[i].at < timers[idx].at)) idx = i
    }
    if (idx < 0) break
    var t = timers.splice(idx, 1)[0]
    now = t.at
    t.fn()
  }
  now = target
}
`); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(string(src)); err != nil {
		t.Fatal(err)
	}
	if _, err := vm.RunString(`
function mountNode(node) {
  var sent = [], styles = []
  var inst = Object.assign({
    data: Object.assign({}, compDef.data),
    props: { node: node, onNsgo: function (d) { sent.push(d) } },
    setData: function (patch) {
      Object.assign(this.data, patch)
      if ('kaStyle' in patch) styles.push(patch.kaStyle)
    },
    sent: sent,
    styles: styles,
  }, compDef.methods)
  inst.didMount = compDef.didMount
  inst.didUpdate = compDef.didUpdate
  inst.didUnmount = compDef.didUnmount
  inst.didMount()
  return inst
}
`); err != nil {
		t.Fatal(err)
	}
	return vm
}

func TestMiniProgramAlipayBackRouting(t *testing.T) {
	axml, err := miniprogramRuntime.ReadFile("miniprogram/alipay/pages/index/index.axml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`<page-container`,
		`show="{{back}}"`,
		`overlay="{{false}}"`,
		`duration="{{0}}"`,
		`onBeforeLeave="onBack"`,
	} {
		if !strings.Contains(string(axml), want) {
			t.Errorf("alipay pages/index/index.axml missing %q", want)
		}
	}

	vm := runAlipayPage(t)
	const script = `
function assert(cond, msg) { if (!cond) throw new Error(msg) }

// When back is false, neither onBeforeLeave nor events.onBack intercepts.
var prevented = false
page.events.onBack.call(page, { preventDefault() { prevented = true } })
assert(!prevented, 'preventDefault called when back=false')
assert(pageEvents.length === 0, 'dispatched back when back=false')

page.onBack()
assert(pageEvents.length === 0, 'onBack dispatched when back=false')

// When back is true, page-container onBeforeLeave clears back and notifies Go.
page.setData({ back: true })
page.onBack()
assert(page.data.back === false, 'onBack did not clear back')
assert(JSON.stringify(pageEvents) === '[["back",0]]', 'page-container events = ' + JSON.stringify(pageEvents))

// A duplicate events.onBack after onBeforeLeave does not double-dispatch.
prevented = false
page.events.onBack.call(page, { preventDefault() { prevented = true } })
assert(!prevented, 'unexpected preventDefault after back cleared')
assert(pageEvents.length === 1, 'duplicate back dispatched')

// Re-armed back via events.onBack calls preventDefault and dispatches once.
pageEvents.length = 0
page.setData({ back: true })
prevented = false
page.events.onBack.call(page, { preventDefault() { prevented = true } })
assert(prevented, 'events.onBack did not call preventDefault')
assert(page.data.back === false, 'events.onBack did not clear back')
assert(JSON.stringify(pageEvents) === '[["back",0]]', 'events.onBack events = ' + JSON.stringify(pageEvents))

// A subsequent onBeforeLeave after events.onBack does not double-dispatch.
page.onBack()
assert(pageEvents.length === 1, 'onBeforeLeave double-dispatched after events.onBack')
`
	if _, err := vm.RunString(script); err != nil {
		t.Fatal(err)
	}
}

func TestMiniProgramAlipayKeyframes(t *testing.T) {
	axmlBytes, err := miniprogramRuntime.ReadFile("miniprogram/alipay/components/nsgo-node/index.axml")
	if err != nil {
		t.Fatal(err)
	}
	axml := string(axmlBytes)
	if got := strings.Count(axml, `style="{{m.box(node, kaStyle)}}"`); got != 4 {
		t.Errorf("alipay index.axml binds m.box(node, kaStyle) %d times, want 4", got)
	}

	// Verify index.sjs box() appends kaStyle after static/animated styles.
	sjsBytes, err := miniprogramRuntime.ReadFile("miniprogram/alipay/components/nsgo-node/index.sjs")
	if err != nil {
		t.Fatal(err)
	}
	sjsVM := goja.New()
	sjsSrc := strings.Replace(string(sjsBytes), "export default ", "var sjs = ", 1)
	if _, err := sjsVM.RunString(sjsSrc + `
var out = sjs.box({ f: [1, 2, 30, 40], s: 'opacity:1;', a: '', tr: '' }, 'opacity:0;transition:none;')
if (out !== 'left:1px;top:2px;width:30px;height:40px;opacity:1;opacity:0;transition:none;') {
  throw new Error('sjs.box = ' + out)
}
var plain = sjs.box({ f: [0, 0, 10, 10], s: '', a: '', tr: '' })
if (plain !== 'left:0px;top:0px;width:10px;height:10px;') {
  throw new Error('sjs.box plain = ' + plain)
}
`); err != nil {
		t.Fatal(err)
	}

	vm := runAlipayNodeVM(t)
	const script = `
function assert(cond, msg) { if (!cond) throw new Error(msg) }

// 1. One-shot ModalSlide (n=1, translateY percentage + cubic-bezier) steps and clears.
var slide = mountNode({
  id: 10,
  ka: {
    d: 300,
    n: 1,
    f: [
      { offset: 0, translateY: '100%', ease: 'cubic-bezier(0.2,0.8,0.2,1)' },
      { offset: 1, translateY: '0%' },
    ],
  },
})
assert(slide.data.kaStyle === 'transform:translateY(100%);transition:none;', 'frame 0 = ' + slide.data.kaStyle)
advance(16)
assert(
  slide.data.kaStyle === 'transform:translateY(0%);transition:opacity 300ms cubic-bezier(0.2,0.8,0.2,1),transform 300ms cubic-bezier(0.2,0.8,0.2,1);',
  'frame 1 = ' + slide.data.kaStyle
)
advance(300)
assert(slide.data.kaStyle === '', 'one-shot did not clear: ' + slide.data.kaStyle)

// 2. LayoutScale appear with omitted end properties transitions back to base style and clears.
var scale = mountNode({
  id: 11,
  ka: { d: 200, n: 1, f: [{ offset: 0, scale: [0, 0], ease: 'ease-out' }, { offset: 1 }] },
})
assert(scale.data.kaStyle === 'transform:scale(0,0);transition:none;', 'scale frame 0 = ' + scale.data.kaStyle)
advance(16)
assert(
  scale.data.kaStyle === 'transition:opacity 200ms ease-out,transform 200ms ease-out;',
  'scale frame 1 = ' + scale.data.kaStyle
)
advance(200)
assert(scale.data.kaStyle === '', 'scale did not clear: ' + scale.data.kaStyle)

// 3. Looping multi-stop keyframe with delay, opacity, scale, rotate and translateY.
var pulse = mountNode({
  id: 12,
  ka: {
    d: 1000,
    n: 0,
    delay: 50,
    f: [
      { offset: 0, opacity: 1, scale: 1, rotate: 0, translateY: '0%', ease: 'ease-in' },
      { offset: 0.5, opacity: 0.4, scale: 1.2, rotate: 180, translateY: '-25%', ease: 'ease-out' },
      { offset: 1, opacity: 1, scale: 1, rotate: 360, translateY: '0%' },
    ],
  },
})
assert(pulse.styles.length === 0, 'started before delay')
advance(50)
assert(
  pulse.data.kaStyle === 'opacity:1;transform:translateY(0%) rotate(0deg) scale(1,1);transition:none;',
  'pulse frame 0 = ' + pulse.data.kaStyle
)
// Unrelated prop update with equal ka does not restart.
var countBefore = pulse.styles.length
pulse.props = Object.assign({}, pulse.props, { node: Object.assign({}, pulse.props.node, { f: [1, 2, 3, 4] }) })
pulse.didUpdate()
assert(pulse.styles.length === countBefore, 'unrelated didUpdate restarted keyframes')

advance(16)
assert(
  pulse.data.kaStyle === 'opacity:0.4;transform:translateY(-25%) rotate(180deg) scale(1.2,1.2);transition:opacity 500ms ease-in,transform 500ms ease-in;',
  'pulse frame 1 = ' + pulse.data.kaStyle
)
advance(500)
assert(
  pulse.data.kaStyle === 'opacity:1;transform:translateY(0%) rotate(360deg) scale(1,1);transition:opacity 500ms ease-out,transform 500ms ease-out;',
  'pulse frame 2 = ' + pulse.data.kaStyle
)
// Loops into iteration 2.
advance(500)
assert(
  pulse.data.kaStyle === 'opacity:1;transform:translateY(0%) rotate(0deg) scale(1,1);transition:none;',
  'pulse loop restart = ' + pulse.data.kaStyle
)

// Clearing ka stops the timer and clears kaStyle.
pulse.props = Object.assign({}, pulse.props, { node: { id: 12, ka: null } })
pulse.didUpdate()
assert(pulse.data.kaStyle === '', 'clearing ka did not reset kaStyle')
var stylesAfterClear = pulse.styles.length
advance(2000)
assert(pulse.styles.length === stylesAfterClear, 'cleared ka kept ticking')

// 4. Unmounting cancels timers without calling setData.
var unmounted = mountNode({
  id: 13,
  ka: { d: 100, n: 0, f: [{ offset: 0, opacity: 0 }, { offset: 1, opacity: 1 }] },
})
unmounted.setData = function () { throw new Error('setData after unmount') }
unmounted.didUnmount()
advance(500)
`
	if _, err := vm.RunString(script); err != nil {
		t.Fatal(err)
	}
}

func TestMiniProgramAlipayKeyboardHeight(t *testing.T) {
	// 1. Page events.onKeyboardHeight routes height + duration to app.event('keyboard', ...).
	vm := runAlipayPage(t)
	const pageScript = `
function assert(cond, msg) { if (!cond) throw new Error(msg) }
page.events.onKeyboardHeight.call(page, { height: 296 })
page.events.onKeyboardHeight.call(page, { detail: { height: 320, duration: 0.3 } })
page.events.onKeyboardHeight.call(page, { height: 0 })
assert(
  JSON.stringify(pageEvents) === '[["keyboard",0,296,0.25],["keyboard",0,320,0.3],["keyboard",0,0,0.25]]',
  'keyboard events = ' + JSON.stringify(pageEvents)
)
`
	if _, err := vm.RunString(pageScript); err != nil {
		t.Fatal(err)
	}

	// 2. runtime.js subscribes to onKeyboardHeightChange on Alipay when available.
	rtSrc, err := miniprogramRuntime.ReadFile("miniprogram/common/nsgo/runtime.js")
	if err != nil {
		t.Fatal(err)
	}
	rtVM := goja.New()
	rtMod := rtVM.NewObject()
	rtVM.Set("module", rtMod)
	rtVM.Set("console", map[string]any{"log": func() {}, "warn": func() {}, "error": func() {}})
	rtVM.Set("require", func(call goja.FunctionCall) goja.Value {
		switch call.Argument(0).String() {
		case "./config.js":
			return rtVM.ToValue(map[string]any{"platform": "alipay", "wasm": "/nsgo-app/app.wasm.br", "subpackage": "nsgo-app"})
		case "./tree.js":
			v, _ := rtVM.RunString(`({ Tree: class { add() {} take() { return {} } typed() {} scrolled() {} } })`)
			return v
		case "./pixels.js":
			return rtVM.ToValue(map[string]any{"clear": func() {}})
		case "./signin.js":
			return rtVM.ToValue(map[string]any{"available": func() bool { return false }, "signIn": func() {}})
		case "./speak.js":
			return rtVM.ToValue(map[string]any{"createSpeaker": func() any {
				return map[string]any{"available": false, "voices": func() []any { return nil }, "start": func() {}, "stop": func() {}, "close": func() {}}
			}})
		case "./updates.js":
			return rtVM.ToValue(map[string]any{"updateManager": func() any { return nil }})
		case "./api.js":
			return rtVM.ToValue(map[string]any{"globalOf": func() any { return nil }, "apiOf": func() any { return nil }})
		case "./wasm_exec.js":
			v, _ := rtVM.RunString(`(function (g) {
				g.Go = function () {
					this.importObject = {}
					this.run = function () {
						g.nsgoMP.event = function (...args) { g.__events.push(args) }
						g.nsgoMP.ready()
						return new Promise(function () {})
					}
				}
				g.__events = []
				globalThis.__goGlobal = g
			})`)
			return v
		}
		return goja.Undefined()
	})
	if _, err := rtVM.RunString(string(rtSrc)); err != nil {
		t.Fatal(err)
	}
	if _, err := rtVM.RunString(`
var listeners = {}
var host = {
  api: {},
  loadSubpackage: function () { return Promise.resolve() },
  instantiate: function () { return Promise.resolve({ instance: {} }) },
  device: function () { return { width: 375, height: 812 } },
  measure: function () { return 0 },
  setTimeout: function () { return 1 },
  clearTimeout: function () {},
  on: function (name, fn) { listeners[name] = fn },
}
var appPromise = module.exports.start({ setData() {} }, host)
var done = false, err = null
appPromise.then(function (app) {
  if (typeof listeners.onKeyboardHeightChange !== 'function') {
    throw new Error('Alipay runtime did not subscribe to onKeyboardHeightChange')
  }
  listeners.onKeyboardHeightChange({ height: 280 })
  listeners.onKeyboardHeightChange({ height: 0 })
  app.stop()
  listeners.onKeyboardHeightChange({ height: 260 })
  done = true
}).catch(function (e) { err = e })
`); err != nil {
		t.Fatal(err)
	}
	// Goja drains microtasks automatically after RunString.
	if _, err := rtVM.RunString(`
if (err) throw err
if (!done) throw new Error('start did not settle')
var got = JSON.stringify(globalThis.__goGlobal.__events)
if (got !== '[["keyboard",0,280,0.25],["keyboard",0,0,0.25]]') {
  throw new Error('runtime keyboard events = ' + got)
}
`); err != nil {
		t.Fatal(err)
	}

	// 3. Component onFocus/onBlur fallback forwards height when present and ignores when absent.
	nodeVM := runAlipayNodeVM(t)
	if _, err := nodeVM.RunString(`
function assert(cond, msg) { if (!cond) throw new Error(msg) }

// Without detail.height, onFocus/onBlur do not emit spurious keyboard events.
var plain = mountNode({ id: 5, k: 'input', fd: {} })
plain.onFocus({ detail: { value: '' } })
plain.onBlur({ detail: { value: '' } })
assert(JSON.stringify(plain.sent) === '[["focus",5],["blur",5]]', 'plain focus = ' + JSON.stringify(plain.sent))

// With detail.height, onFocus reports height and onBlur resets to 0.
var withHeight = mountNode({ id: 6, k: 'input', fd: {} })
withHeight.onFocus({ detail: { value: '', height: 310, duration: 0.2 } })
withHeight.onBlur({ detail: { value: '' } })
assert(
  JSON.stringify(withHeight.sent) === '[["focus",6],["keyboard",0,310,0.2],["blur",6],["keyboard",0,0,0.25]]',
  'height focus = ' + JSON.stringify(withHeight.sent)
)
`); err != nil {
		t.Fatal(err)
	}
}

func TestMiniProgramAlipayScrollDrags(t *testing.T) {
	axmlBytes, err := miniprogramRuntime.ReadFile("miniprogram/alipay/components/nsgo-node/index.axml")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`onTouchStart="onScrollTouchStart"`,
		`onTouchMove="onScrollTouchMove"`,
		`onTouchEnd="onScrollTouchEnd"`,
		`onTouchCancel="onScrollTouchEnd"`,
	} {
		if !strings.Contains(string(axmlBytes), want) {
			t.Errorf("alipay index.axml missing %q", want)
		}
	}

	vm := runAlipayNodeVM(t)
	const script = `
function assert(cond, msg) { if (!cond) throw new Error(msg) }
function touch(type, ts, x, y, count) {
  var list = []
  for (var i = 0; i < (count == null ? 1 : count); i++) {
    list.push({ identifier: i, pageX: x, pageY: y })
  }
  return { type: type, timeStamp: ts, touches: list, changedTouches: [{ identifier: 0, pageX: x, pageY: y }] }
}

// 1. Tap inside scroll view does not emit dragstart/dragend.
var sv = mountNode({ id: 20, k: 'scroll', sc: { h: false, off: false } })
sv.onScrollTouchStart(touch('touchstart', 100, 50, 200, 1))
sv.onScrollTouchMove(touch('touchmove', 110, 51, 198, 1))
sv.onScrollTouchEnd(touch('touchend', 120, 51, 198, 0))
assert(sv.sent.length === 0, 'tap emitted scroll drag: ' + JSON.stringify(sv.sent))

// 2. Vertical drag past slop emits dragstart -> scroll -> dragend -> scrollend.
sv.onScrollTouchStart(touch('touchstart', 200, 50, 200, 1))
sv.onScrollTouchMove(touch('touchmove', 210, 50, 190, 1))
sv.onScroll({ detail: { scrollLeft: 0, scrollTop: 10 } })
// Second finger down + one finger lift keeps drag active until last finger lifts.
sv.onScrollTouchStart(touch('touchstart', 215, 60, 180, 2))
sv.onScrollTouchEnd(touch('touchend', 220, 60, 180, 1))
assert(JSON.stringify(sv.sent) === '[["dragstart",20],["scroll",20,0,10]]', 'mid-drag = ' + JSON.stringify(sv.sent))
sv.onScrollTouchEnd(touch('touchend', 230, 50, 170, 0))
advance(150)
assert(
  JSON.stringify(sv.sent) === '[["dragstart",20],["scroll",20,0,10],["dragend",20],["scrollend",20]]',
  'full drag = ' + JSON.stringify(sv.sent)
)

// 3. Native onScroll arriving while finger is down (before touchmove slop) starts drag before scroll.
sv.sent.length = 0
sv.onScrollTouchStart(touch('touchstart', 300, 50, 200, 1))
sv.onScroll({ detail: { scrollLeft: 0, scrollTop: 3 } })
sv.onScrollTouchEnd(touch('touchcancel', 310, 50, 197, 1))
advance(150)
assert(
  JSON.stringify(sv.sent) === '[["dragstart",20],["scroll",20,0,3],["dragend",20],["scrollend",20]]',
  'scroll-first drag = ' + JSON.stringify(sv.sent)
)

// 4. Programmatic scroll without finger down emits scroll + scrollend, no dragstart/dragend.
sv.sent.length = 0
sv.onScroll({ detail: { scrollLeft: 0, scrollTop: 120 } })
advance(150)
assert(
  JSON.stringify(sv.sent) === '[["scroll",20,0,120],["scrollend",20]]',
  'programmatic scroll = ' + JSON.stringify(sv.sent)
)

// 5. Cross-axis drag (horizontal on vertical scroll view) does not report drag.
sv.sent.length = 0
sv.onScrollTouchStart(touch('touchstart', 400, 50, 200, 1))
sv.onScrollTouchMove(touch('touchmove', 410, 90, 201, 1))
sv.onScrollTouchEnd(touch('touchend', 420, 90, 201, 0))
assert(sv.sent.length === 0, 'cross-axis emitted drag: ' + JSON.stringify(sv.sent))

// 6. Disabled scroll view (off=true) never reports drag.
var off = mountNode({ id: 21, k: 'scroll', sc: { h: false, off: true } })
off.onScrollTouchStart(touch('touchstart', 500, 50, 200, 1))
off.onScrollTouchMove(touch('touchmove', 510, 50, 150, 1))
off.onScroll({ detail: { scrollLeft: 0, scrollTop: 0 } })
off.onScrollTouchEnd(touch('touchend', 520, 50, 150, 0))
assert(JSON.stringify(off.sent) === '[["scroll",21,0,0]]', 'disabled scroll emitted drag: ' + JSON.stringify(off.sent))

// 7. Nested scroll views: horizontal inner + vertical outer route drags to the matching axis,
//    and nested same-axis scroll views deduplicate so only the inner claims the drag.
var outerV = mountNode({ id: 30, k: 'scroll', sc: { h: false, off: false } })
var innerH = mountNode({ id: 31, k: 'scroll', sc: { h: true, off: false } })
var ts1 = 600, ts2 = 610, ts3 = 620
innerH.onScrollTouchStart(touch('touchstart', ts1, 100, 100, 1))
outerV.onScrollTouchStart(touch('touchstart', ts1, 100, 100, 1))
// Drag vertically: innerH ignores, outerV starts drag.
innerH.onScrollTouchMove(touch('touchmove', ts2, 100, 120, 1))
outerV.onScrollTouchMove(touch('touchmove', ts2, 100, 120, 1))
innerH.onScrollTouchEnd(touch('touchend', ts3, 100, 120, 0))
outerV.onScrollTouchEnd(touch('touchend', ts3, 100, 120, 0))
assert(innerH.sent.length === 0, 'innerH claimed vertical drag: ' + JSON.stringify(innerH.sent))
assert(JSON.stringify(outerV.sent) === '[["dragstart",30],["dragend",30]]', 'outerV vertical drag = ' + JSON.stringify(outerV.sent))

// Nested same-axis (innerV inside outerV): only innerV starts drag.
outerV.sent.length = 0
var innerV = mountNode({ id: 32, k: 'scroll', sc: { h: false, off: false } })
innerV.onScrollTouchStart(touch('touchstart', 700, 100, 100, 1))
outerV.onScrollTouchStart(touch('touchstart', 700, 100, 100, 1))
innerV.onScrollTouchMove(touch('touchmove', 710, 100, 120, 1))
outerV.onScrollTouchMove(touch('touchmove', 710, 100, 120, 1))
innerV.onScrollTouchEnd(touch('touchend', 720, 100, 120, 0))
outerV.onScrollTouchEnd(touch('touchend', 720, 100, 120, 0))
assert(JSON.stringify(innerV.sent) === '[["dragstart",32],["dragend",32]]', 'innerV drag = ' + JSON.stringify(innerV.sent))
assert(outerV.sent.length === 0, 'outerV duplicated same-axis inner drag: ' + JSON.stringify(outerV.sent))
`
	if _, err := vm.RunString(script); err != nil {
		t.Fatal(err)
	}
}
