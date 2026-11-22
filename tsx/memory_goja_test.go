//go:build !js

package tsx

import (
	"errors"
	goruntime "runtime"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx/internal/js"
)

// limited is a sandboxed runtime with no app whose built-ins hold limit bytes and the nesting, hardened as a sandbox's are.
func limited(limit int64) *runtime {
	r := &runtime{vm: js.New(), warned: map[string]bool{}, sandbox: &sandboxed{}}
	if limit > 0 {
		r.limitMemory(limit)
	}
	r.limitNesting()
	r.harden()
	return r
}

// bare is a runtime with no app whose built-ins are goja's own, hardened as a sandbox's are.
func bare() *runtime {
	r := &runtime{vm: js.New(), warned: map[string]bool{}, sandbox: &sandboxed{}}
	r.harden()
	return r
}

// used is what r's built-ins hold now.
func (r *runtime) used() int64 {
	m := r.sandbox.memory
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.used
}

// TestMemoryStopsWhatWouldNotFit holds each built-in that allocates by a size to the budget before it allocates: none of these, which would take from a megabyte to exabytes at once, allocates, and each stops the runtime with the quota error, which the script cannot catch.
func TestMemoryStopsWhatWouldNotFit(t *testing.T) {
	for _, src := range []string{
		`new ArrayBuffer(512 * 1024 * 1024)`,
		`new ArrayBuffer(2 ** 32)`,
		`new ArrayBuffer(2 ** 52)`,
		`new ArrayBuffer({ valueOf: () => 2 ** 32 })`,
		`new Uint8Array(2 ** 32)`,
		`new Float16Array(1 << 22)`,
		`new Float64Array(2 ** 28)`,
		`new BigUint64Array(2 ** 28)`,
		`new Uint8Array({ length: 2 ** 32 })`,
		`new Int32Array(new Uint8Array(500_000))`,
		`new (class extends Uint16Array {})(2 ** 30)`,
		`Uint8Array.from({ length: 2 ** 32 })`,
		`new ArrayBuffer(8).transfer(2 ** 32)`,
		`new ArrayBuffer(8).transferToFixedLength(2 ** 32)`,
		`const b = new ArrayBuffer(700_000); b.slice()`,
		`const a = new Uint8Array(700_000); a.toReversed()`,
		`const a = new Uint8Array(700_000); a.slice()`,
		`const a = new Uint8Array(700_000); a.map((x) => x)`,
		`"x".repeat(2 ** 32)`,
		`"ab".repeat({ valueOf: () => 2 ** 30 })`,
		`"x".padStart(2 ** 32)`,
		`"x".padEnd(2 ** 32, "yz")`,
		`String.raw({ raw: { length: 2 ** 32 } })`,
		`new Array(2 ** 30).join("x")`,
		`Array.prototype.join.call({ length: 2 ** 32 })`,
		`new Array(1000).fill("x".repeat(100_000)).join("")`,
		`new Array(1000).fill({ toString: () => "x".repeat(100_000) }).join()`,
		`new Array(2 ** 30).fill(0)`,
		`[].fill.call({ length: 2 ** 32 }, 0)`,
		`Array.from({ length: 2 ** 30 })`,
		`Array.from("x".repeat(500_000))`,
		`Array.prototype.sort.call({ length: 2 ** 32 - 1 })`,
		`Array.prototype.toSorted.call({ length: 2 ** 32 - 2 })`,
		`Array.prototype.toReversed.call({ length: 2 ** 32 - 2 })`,
		`Array.prototype.with.call({ length: 2 ** 32 - 2 }, 0, 0)`,
		`Array.prototype.toSpliced.call({ length: 2 ** 32 - 2 })`,
		`Array.prototype.toLocaleString.call({ length: 2 ** 32 })`,
		`new Array(1000).fill("x".repeat(100_000)).toLocaleString()`,
		`new Float64Array(100_000).join()`,
		`new Uint8Array(500_000).toLocaleString()`,
		`Math.max.apply(null, { length: 2 ** 31 })`,
		`Reflect.apply(Math.max, null, { length: 2 ** 31 })`,
		`Reflect.construct(Array, { length: 2 ** 31 })`,
		`JSON.stringify(new Array(1000).fill("x".repeat(100_000)))`,
		`JSON.stringify({ a: 1 }, new Array(2 ** 31))`,
		`const s = "x".repeat(300_000); JSON.stringify(new Array(10).fill(0), (k, v) => k === "" ? v : s)`,
	} {
		r := limited(1 << 20)
		_, err := r.vm.RunString(`let caught = false; try { ` + src + ` } catch (e) { caught = true; }`)
		var interrupted *js.InterruptedError
		if !errors.As(err, &interrupted) || core.ErrorCode(r.stoppedErr()) != core.CodeQuotaExceeded {
			t.Errorf("%s: ran to %v, stopped with %v", src, err, r.stoppedErr())
		}
		if used := r.used(); used > 1<<20 {
			t.Errorf("%s: holds %d bytes", src, used)
		}
	}
}

func TestMemoryMetersArrayAndStringStorage(t *testing.T) {
	for _, test := range []struct {
		name string
		code string
	}{
		{"concat", `items.concat(items)`},
		{"flat", `[items, items].flat()`},
		{"slice", `rows.slice()`},
		{"spread", `[...rows]`},
		{"addition", `text + text`},
		{"template", "`${text}${text}`"},
		{"replace", `"aa".replace(/a/g, text)`},
		{"replaceAll", `"aa".replaceAll("a", text)`},
		{"utf8", `encode(text)`},
	} {
		t.Run(test.name, func(t *testing.T) {
			const limit = 16 << 10
			r := limited(limit)
			if err := r.vm.Set("items", r.vm.NewArray(make([]any, 600)...)); err != nil {
				t.Fatal(err)
			}
			if err := r.vm.Set("rows", r.vm.NewArray(make([]any, 1200)...)); err != nil {
				t.Fatal(err)
			}
			if err := r.vm.Set("text", strings.Repeat("界", limit/2)); err != nil {
				t.Fatal(err)
			}
			if err := r.vm.Set("encode", func(call js.FunctionCall) js.Value { return r.encodeUTF8(call.Argument(0)) }); err != nil {
				t.Fatal(err)
			}
			_, err := r.vm.RunString(`try { ` + test.code + ` } catch (_) {}`)
			var interrupted *js.InterruptedError
			var quota *core.Error
			if !errors.As(err, &interrupted) || !errors.As(r.stoppedErr(), &quota) || quota.Code != core.CodeQuotaExceeded || quota.Resource != "memory" {
				t.Fatalf("operation returned %v and stopped with %v, want the memory quota", err, r.stoppedErr())
			}
			if used := r.used(); used > limit {
				t.Fatalf("metered usage %d exceeds %d", used, limit)
			}
		})
	}
}

// TestMemoryChargesAppendsForWhatTheyAdd builds strings by appending: each append is charged for what it adds, not the whole rope it makes, so a chain of appends costs what it holds; a string built past the budget still stops, and reading a rope as it grows, which flattens it, stays within it.
func TestMemoryChargesAppendsForWhatTheyAdd(t *testing.T) {
	r := limited(64 << 20)
	v, err := r.vm.RunString(`let s = ""; for (let i = 0; i < 12000; i++) s += "x"; s.length`)
	if err != nil || v.ToInteger() != 12000 {
		t.Fatalf("12,000 appends returned %v, %v", v, err)
	}
	if used := r.used(); used > 2<<20 {
		t.Fatalf("12,000 appends charged %d bytes", used)
	}
	for _, test := range []struct {
		code  string
		limit int64
	}{
		{`let s = ""; const c = "x".repeat(1024); for (;;) s += c;`, 64 << 20},
		{`let s = ""; for (;;) s += "x";`, 1 << 20},
	} {
		r := limited(test.limit)
		_, err := r.vm.RunString(`try { ` + test.code + ` } catch (_) {}`)
		var interrupted *js.InterruptedError
		var quota *core.Error
		if !errors.As(err, &interrupted) || !errors.As(r.stoppedErr(), &quota) || quota.Resource != "memory" {
			t.Fatalf("%s returned %v and stopped with %v, want the memory quota", test.code, err, r.stoppedErr())
		}
	}
	r = limited(4 << 20)
	v, err = r.vm.RunString(`let s = "", n = 0; for (let i = 0; i < 20000; i++) { s += "ab"; n += s.length + s.charCodeAt(i) + (s + "!").charCodeAt(0); } n`)
	if err != nil || v.ToInteger() == 0 {
		t.Fatalf("reading a growing rope returned %v, %v", v, err)
	}
	if used := r.used(); used > 4<<20 {
		t.Fatalf("reading a growing rope charged %d bytes", used)
	}
}

func TestMemoryMeteredUTF8PreservesUnpairedSurrogateReplacement(t *testing.T) {
	r := limited(16 << 10)
	if err := r.vm.Set("encode", func(call js.FunctionCall) js.Value { return r.encodeUTF8(call.Argument(0)) }); err != nil {
		t.Fatal(err)
	}
	v, err := r.vm.RunString(`encode("a\ud800é")`)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(r.bytesOf(v)); got != "a�é" {
		t.Fatalf("encoded %q, want %q", got, "a�é")
	}
}

// TestMemoryReadsSizesOnce holds a built-in to the size it was checked by when what it is given reads differently the second time, through a length getter, a proxy or a valueOf: each of these read 0 first and would then allocate from 16 MiB to over 1 GiB under a 1 MiB budget.
func TestMemoryReadsSizesOnce(t *testing.T) {
	const grows = `let c = 0; const o = { get length() { return c++ ? 1 << 24 : 0; } };`
	for _, src := range []string{
		grows + `new Uint8Array(o)`,
		grows + `new Float64Array(o)`,
		grows + `Array.from(o)`,
		grows + `Array.prototype.toReversed.call(o)`,
		grows + `Array.prototype.toSorted.call(o)`,
		grows + `Array.prototype.with.call(o, 0, 0)`,
		grows + `Array.prototype.toSpliced.call(o)`,
		grows + `Array.prototype.sort.call(o)`,
		`let c = 0; Array.prototype.fill.call({ get length() { return c++ ? 1 << 20 : 0; } }, 0)`,
		grows + `Math.max.apply(null, o)`,
		grows + `Reflect.apply(Math.max, null, o)`,
		grows + `Reflect.construct(Array, o)`,
		`let c = 0; Math.max.apply(null, { length: { valueOf: () => c++ ? 1 << 22 : 0 } })`,
		`let c = 0; Math.max.apply(null, new Proxy({}, { get: (t, k) => k === "length" ? (c++ ? 1 << 22 : 0) : 0 }))`,
		grows + `String.raw({ raw: o })`,
		`let c = 0; JSON.stringify({}, new Proxy([], { get: (t, k) => k === "length" ? (c++ ? 1 << 22 : 0) : undefined }))`,
	} {
		r := limited(1 << 20)
		if n := allocated(func() { _, _ = r.vm.RunString(src) }); n > 8<<20 {
			t.Errorf("%s: allocated %d MiB", src, n>>20)
		}
	}
}

// TestMemoryStopsWhatAddsUp stops a built-in before it allocates past the budget in parts that a check of its arguments alone would not see: the strings a template or JSON joins, the indentation of deep JSON, raw JSON, a typed array's own length, a subclass's constructor or species. Each would allocate from 4 MiB to over 400 MiB under a 1 MiB budget.
func TestMemoryStopsWhatAddsUp(t *testing.T) {
	for _, src := range []string{
		`const a = new Uint8Array(1 << 19); Object.defineProperty(a, "length", { value: 0 }); new Float64Array(a)`,
		`String.raw({ raw: new Array(1000).fill("x".repeat(100_000)) })`,
		`String.raw({ raw: ["", ""] }, { toString: () => "x".repeat(900_000) }) + String.raw({ raw: new Array(200).fill("") }, ...new Array(199).fill("x".repeat(100_000)))`,
		`let a = []; for (let i = 0; i < 3000; i++) a = [a]; JSON.stringify(a, null, 10)`,
		`let a = []; for (let i = 0; i < 3000; i++) a = [a]; JSON.stringify(a, null, new Number(10))`,
		`JSON.stringify(new Array(1000).fill("x".repeat(100_000)), [])`,
		`JSON.stringify(new Array(1000).fill({ a: "x".repeat(100_000) }), ["a"])`,
		`const r = JSON.rawJSON('"' + "x".repeat(100_000) + '"'); JSON.stringify(new Array(1000).fill(r))`,
		`JSON.stringify("\x01".repeat(300_000))`,
		`const s = "é".repeat(100_000); for (let i = 0; i < 100; i++) globalThis["k" + i] = JSON.stringify(s)`,
		`new (class extends ArrayBuffer {})(2 ** 32)`,
		`Reflect.construct(ArrayBuffer, [2 ** 32], function () {})`,
		`Reflect.construct(Uint8Array, [2 ** 32], Object)`,
		`new (class extends Uint8Array { static get [Symbol.species]() { return Float64Array; } })(500_000).map((x) => x)`,
	} {
		r := limited(1 << 20)
		var err error
		n := allocated(func() {
			_, err = r.vm.RunString(`let caught = false; try { ` + src + ` } catch (e) { caught = true; }`)
		})
		var interrupted *js.InterruptedError
		if !errors.As(err, &interrupted) || core.ErrorCode(r.stoppedErr()) != core.CodeQuotaExceeded {
			t.Errorf("%s: ran to %v, stopped with %v", src, err, r.stoppedErr())
		}
		if n > 8<<20 {
			t.Errorf("%s: allocated %d MiB", src, n>>20)
		}
	}
}

// allocated is how many bytes the process allocated while f ran: what a script needs besides what it is held to is far below a few MiB, and one allocation past a budget of 1 MiB here far above.
func allocated(f func()) uint64 {
	var before, after goruntime.MemStats
	goruntime.ReadMemStats(&before)
	f()
	goruntime.ReadMemStats(&after)
	return after.TotalAlloc - before.TotalAlloc
}

// TestMemoryKeepsWhatFits runs what fits the budget as the originals do, holding what it keeps.
func TestMemoryKeepsWhatFits(t *testing.T) {
	r := limited(64 << 20)
	if _, err := r.vm.RunString(`globalThis.kept = [new ArrayBuffer(1 << 20), new Float64Array(1 << 17), "x".repeat(1 << 20), new Array(1 << 16).fill(0)]`); err != nil {
		t.Fatal(err)
	}
	// 1 MiB, 1 MiB, 1 MiB of ASCII and 64 Ki values.
	if used, want := r.used(), int64(3<<20+1<<16*valueSize); used < want {
		t.Errorf("holds %d bytes, want at least %d", used, want)
	}
}

// TestMemoryKeepsTheBuiltinsBehavior runs scripts in runtimes whose built-ins are held, to the nesting alone and to a memory budget as well, and one whose are goja's own, which must agree.
func TestMemoryKeepsTheBuiltinsBehavior(t *testing.T) {
	for _, src := range []string{
		`const b = new ArrayBuffer(16); [b.byteLength, b instanceof ArrayBuffer, b.constructor === ArrayBuffer, ArrayBuffer.isView(new Uint8Array(b)), ArrayBuffer.name, ArrayBuffer.length, Object.getPrototypeOf(b) === ArrayBuffer.prototype]`,
		`const a = new Uint8Array([1, 2, 300]); [a.join(), a instanceof Uint8Array, a.constructor === Uint8Array, Uint8Array.BYTES_PER_ELEMENT, Uint8Array.name, Uint8Array.length, Object.getPrototypeOf(Uint8Array) === Object.getPrototypeOf(Int8Array)]`,
		`const a = new Float64Array(new Set([1.5, 2.5])); [a.length, a[1], Float64Array.from([1, 2], (x) => x * 2).join(), Int16Array.of(1, 2, 3).join()]`,
		`const b = new ArrayBuffer(8); const v = new Uint16Array(b, 2, 2); v[0] = 7; [v.length, new Uint8Array(b)[2], v.buffer === b]`,
		`class Bytes extends Uint8Array { sum() { return this.reduce((a, b) => a + b, 0); } } const x = new Bytes([1, 2, 3]); [x.sum(), x instanceof Bytes, x.slice(1) instanceof Bytes, x.map((v) => v * 2).sum(), x.subarray(1).length]`,
		`[new BigInt64Array([1n, 2n]).join(), (() => { try { new Uint8Array(new BigInt64Array(1)); } catch (e) { return e.name; } })()]`,
		`[(() => { try { ArrayBuffer(8); } catch (e) { return e.name; } })(), (() => { try { Uint8Array(8); } catch (e) { return e.name; } })(), (() => { try { new ArrayBuffer(-1); } catch (e) { return e.name; } })(), (() => { try { new Uint8Array(-1); } catch (e) { return e.name; } })()]`,
		`const b = new ArrayBuffer(8); new Uint8Array(b)[0] = 9; const t = b.transfer(16); [b.detached, t.byteLength, new Uint8Array(t)[0], new ArrayBuffer(4).transferToFixedLength().byteLength, new ArrayBuffer(10).slice(2, -2).byteLength]`,
		`const a = new Int8Array([3, 1, 2]); [a.toSorted().join(), a.toReversed().join(), a.with(0, 9).join(), a.filter((x) => x > 1).join(), a.join()]`,
		`["ab".repeat(3), "".repeat(5), "x".repeat(0), (() => { try { "x".repeat(-1); } catch (e) { return e.name; } })(), (() => { try { "x".repeat(Infinity); } catch (e) { return e.name; } })(), (() => { try { String.prototype.repeat.call(null, 1); } catch (e) { return e.name; } })(), String.prototype.repeat.call(12, 2), "é".repeat(2)]`,
		`["5".padStart(3, "0"), "5".padEnd(4, "ab"), "abc".padStart(2), "x".padStart(4), "x".padStart(5, ""), "é".padEnd(3, "ü"), String.prototype.padStart.name, String.prototype.padStart.length]`,
		`[String.raw` + "`a\\n${1}b${2}`" + `, String.raw({ raw: ["x", "y"] }, 1, 2)]`,
		`const a = [1, [2, [3]], null, undefined, "x", { toString: () => "o" }]; a.push(a); [a.join(), a.join("-"), [].join(), [,,].join(), Array.prototype.join.call({ length: 2, 0: "a", 1: "b" }, "+"), String(a), Array.prototype.join.name, Array.prototype.join.length]`,
		`[[1, 2, 3, 4].fill(0, 1, -1).join(), new Array(3).fill("z").join(), [1, 2, 3].fill(9, -2).join(), Array.prototype.fill.call({ length: 2 }, 1)[1], [1, 2].fill(5, NaN, undefined).join()]`,
		`const a = [1.5, null, "x", [2, 3], new Date(0)]; a.push(a); [a.toLocaleString(), [].toLocaleString(), (() => { try { [Object.create(null)].toLocaleString(); } catch (e) { return e.name; } })(), new Float64Array([1.5, -2]).join(";"), new Uint8Array([1, 2]).join(), new Int16Array([1000, -3]).toLocaleString(), new Uint8Array(0).join()]`,
		`[[3, 1, 2].sort().join(), [3, 1, 2].sort((a, b) => b - a).join(), [3, 1, 2].toSorted().join(), [1, 2, 3].toReversed().join(), [1, 2, 3].with(-1, 0).join(), [1, 2, 3].toSpliced(1, 1, "a", "b").join(), [1, 2].toLocaleString()]`,
		`[Array.from("abc").join(), Array.from({ length: 3 }, (_, i) => i * 2).join(), Array.from(new Set([1, 1, 2])).join(), Array.from(new Map([[1, 2]])).join(";"), (class extends Array {}).from([1]).constructor.name]`,
		`function f() { return [this.x, ...arguments].join(); } [f.apply({ x: 1 }, [2, 3]), f.apply({ x: 1 }, { length: 2, 0: "a" }), Reflect.apply(f, { x: 0 }, [1]), Reflect.construct(Array, [3]).length, Reflect.construct(Date, [0], Object).constructor === Date, Math.max.apply(null, [1, 5, 2])]`,
		`[JSON.stringify({ a: [1, "x", null], b: { toJSON: () => "j" }, c: undefined }), JSON.stringify({ a: 1, b: 2, c: { a: 3 } }, ["a", "c"]), JSON.stringify({ a: 1, b: "s" }, (k, v) => typeof v === "number" ? v * 10 : v, 2), JSON.stringify("é\n"), JSON.stringify(undefined), JSON.stringify([new Date(0)]), JSON.stringify({ k: 1 }, function (k, v) { return k === "" ? v : this === undefined; })]`,
		`const o = { b: 1, 1: 2, a: { b: [1, { a: 3, b: 4 }], c: 5 }, get g() { return "g"; }, toJSON: undefined }; [JSON.stringify(o, ["b", "1", "a", "g", 1, new String("c"), "b"]), JSON.stringify(o, ["a", "b"], 2), JSON.stringify([o, [o]], ["a"], "--"), JSON.stringify({ a: { toJSON: (k) => k + "!" } }, ["a"]), JSON.stringify(1, []), JSON.stringify([new Number(3), new String("s"), Object(true)], [])]`,
		`let a = [1]; for (let i = 0; i < 5; i++) a = [a, { k: a }]; [JSON.stringify(a, null, 3), JSON.stringify(a, null, new Number(2)), JSON.stringify(a, null, new String("\t")), JSON.stringify(a, null, "abcdefghijklmn"), JSON.stringify(a, null, 0.5), JSON.stringify(a, null, 20), JSON.stringify({ s: "\u0001\ud800é\ud83d\ude00\"\\" }), JSON.stringify([JSON.rawJSON("12"), JSON.rawJSON("null")]), JSON.stringify({ r: JSON.rawJSON('"x"') }, null, 1)]`,
		`[String.raw({ raw: { length: 3, 0: "a", 1: "b", 2: "c" } }, 1, { toString: () => "two" }, 3), String.raw({ raw: "xyz" }, "-", "+"), String.raw({ raw: [] }), (() => { try { String.raw({}); } catch (e) { return e.name; } })(), String.raw.name, String.raw.length]`,
		`const log = []; const o = { get length() { log.push("len"); return 3; }, get 0() { log.push(0); return "c"; }, 1: "a", set 0(v) { log.push(this === o); }, 2: "b" }; const s = Array.prototype.sort.call(o); [s === o, o[1], o[2], log.join(), Array.prototype.fill.call({ length: 2 }, 7)[1], Array.prototype.toSorted.call({ length: 2, 0: 2, 1: 1 }).join(), Array.prototype.toReversed.call("abc").join()]`,
		`class Bag { #items = [3, 4]; *[Symbol.iterator]() { yield* this.#items; } } [Array.from(new Bag()).join(), new Uint8Array(new Bag()).join(), Uint8Array.from(new Bag()).join(), Array.from({ length: 2, 0: "x" }, (v, i) => v + i).join(), new Int8Array({ length: 2, 0: 5 }).join(), Array.from(new Set([1, 2])).join(), (() => { try { Array.from({ [Symbol.iterator]: 1 }); } catch (e) { return e.name; } })()]`,
		`function f() { return [this && this.x, arguments.length, ...arguments].join(); } [f.apply({ x: 1 }, { length: 3, 0: "a", 2: "c" }), Reflect.apply(f, null, { length: 1, 0: 9 }), Reflect.construct(Array, { length: 2, 0: 1, 1: 2 }).join(), Math.max.apply(null, new Proxy([1, 7, 3], {})), (() => { try { Reflect.apply(f, null, 1); } catch (e) { return e.name; } })()]`,
		`[JSON.stringify.name, JSON.stringify.length, (function () {}).apply.name, Object.isFrozen(ArrayBuffer), Object.isFrozen(Uint8Array.prototype), typeof Uint8Array.fromBase64]`,
		`const f = [JSON.stringify, JSON.parse, Array.prototype.join, Array.prototype.toString, Array.prototype.toLocaleString, Array.prototype.flat]; [f.map((g) => g.name + g.length + Function.prototype.toString.call(g) + Object.keys(g).length + Object.getOwnPropertyNames(g).join("/") + ("prototype" in g)).join(), Object.getPrototypeOf(Uint8Array.prototype).toString === Array.prototype.toString, Object.getOwnPropertyNames(Object.getPrototypeOf(Uint8Array.prototype)).includes("toString"), JSON.stringify(Object.getOwnPropertyDescriptor(Array.prototype, "flat")), JSON.stringify(Object.getOwnPropertyDescriptor(Object.getPrototypeOf(Uint8Array.prototype), "toString"))]`,
		`[[1, [2, [3, [4]]]].flat().length, [1, [2, [3, [4]]]].flat(2).length, [1, [2, [3, [4]]]].flat(Infinity).join(), [1, [2]].flat(0).length, [1, [2]].flat(-1).length, [1, [2]].flat("1").length, [1, [2]].flat(NaN).length, [1, [2, [3]]].flat({ valueOf: () => 1 }).length, [, 1, [, 2]].flat().join(), Array.prototype.flat.call({ length: 2, 0: [1], 1: 2 }).join(), (class extends Array {}).from([[1]]).flat().constructor.name, (() => { try { Array.prototype.flat.call(null); } catch (e) { return e.name; } })()]`,
		`const seen = []; const v = JSON.parse('{"a":[1,{"b":2}],"c":"x","d":null}', function (k, v) { seen.push(k + ":" + (this === null) + ":" + typeof v); return k === "c" ? undefined : typeof v === "number" ? v * 10 : v; }); [JSON.stringify(v), seen.join(), JSON.parse("[1,2]", (k, v) => (k === "0" ? undefined : v)).length, 0 in JSON.parse("[1,2]", (k, v) => (k === "0" ? undefined : v)), JSON.parse('"s"', 1), (() => { try { JSON.parse("{", () => 1); } catch (e) { return e.name; } })(), JSON.parse('{"a":{"b":1}}', function (k, v) { if (k === "a") this.z = 1; return v; }).z, JSON.parse('{"a":1,"b":2}', function (k, v) { if (k === "a") this.b = [7]; return v; }).b.join()]`,
		`[JSON.stringify({ a: JSON.rawJSON("1e3"), b: { a: JSON.rawJSON('"r"') } }, ["a", "b"]), JSON.stringify([JSON.rawJSON("1"), { x: 1 }], ["x"]), JSON.stringify({ a: JSON.rawJSON("2") }, (k, v) => v, 1)]`,
		`[String([1, [2, [3]]]), [1, [2]].toString(), Array.prototype.toString.call({ join: () => "j" }), Array.prototype.toString.call({}), new Uint8Array([1, 2]).toString(), String(new Int8Array([3])), [[1.5], 2].toLocaleString(), (() => { const a = [1]; a.join = () => "own"; return String(a); })()]`,
	} {
		want, err := bare().vm.RunString(src)
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		for _, limit := range []int64{0, 64 << 20} {
			got, err := limited(limit).vm.RunString(src)
			if err != nil {
				t.Errorf("%s: %v", src, err)
				continue
			}
			if g, w := got.String(), want.String(); g != w {
				t.Errorf("%s, memory %d:\n got %s\nwant %s", src, limit, g, w)
			}
		}
	}
}

// TestMemoryReleasesWhatIsCollected frees what an allocation held once the collector frees it, so that a bundle that keeps nothing never runs out however much it allocates over time.
func TestMemoryReleasesWhatIsCollected(t *testing.T) {
	r := limited(32 << 20)
	for i := range 40 {
		if _, err := r.vm.RunString(`new ArrayBuffer(8 << 20).byteLength + "x".repeat(4 << 20).length`); err != nil {
			t.Fatalf("allocation %d: %v", i, err)
		}
		for deadline := time.Now().Add(5 * time.Second); r.used() > 12<<20; {
			if time.Now().After(deadline) {
				t.Fatalf("allocation %d: still holds %d bytes", i, r.used())
			}
			goruntime.GC()
			time.Sleep(time.Millisecond)
		}
	}
}

// hostHeap is a heap the host holds, which a collection takes as long again to come after.
var hostHeap []byte

// TestMemoryCollectsBeforeStopping does not stop a bundle that only makes and drops buffers, whose garbage fills its budget long before the Go collector would run in a host that holds a lot.
func TestMemoryCollectsBeforeStopping(t *testing.T) {
	hostHeap = make([]byte, 256<<20)
	defer func() { hostHeap = nil }()
	goruntime.GC()
	r := limited(32 << 20)
	if _, err := r.vm.RunString(`for (let i = 0; i < 128; i++) new Uint8Array(1 << 20)[0] = i`); err != nil {
		t.Fatalf("stopped with %v", err)
	}
}
