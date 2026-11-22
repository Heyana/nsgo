package harmony

import (
	"strings"
	"testing"

	"github.com/evanw/esbuild/pkg/api"
	"github.com/nsgo-dev/goja"
)

// Exercise the actual ArkTS cache, with synthetic PixelMaps whose release is observable. A Go mirror would not catch changes in the shipped host.
func TestHostCanvasImageCache(t *testing.T) {
	data, err := templates.ReadFile("templates/entry/src/main/ets/pages/Index.ets")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, "interface CanvasImageEntry {")
	end := strings.Index(source, "@Component\nstruct CanvasView")
	if start < 0 || end < start {
		t.Fatal("canvas cache definition not found")
	}
	js := api.Transform(source[start:end], api.TransformOptions{Loader: api.LoaderTS, Target: api.ES2015})
	if len(js.Errors) != 0 {
		t.Fatal(js.Errors)
	}
	vm := goja.New()
	if _, err := vm.RunString(string(js.Code)); err != nil {
		t.Fatal(err)
	}
	_, err = vm.RunString(`
function assert(ok, why) { if (!ok) throw new Error(why); }
const c = new CanvasImageCache();
const released = [];
function image(key) { return {release() { released.push(key); }}; }
function put(key, bytes) {
  c.addPending(key);
  c.complete(key, image(key), bytes);
  c.endPass();
}
c.setLimit(200);
put('a', 100); put('b', 100);
c.get('a'); c.endPass(); put('c', 100);
assert(c.has('a') && !c.has('b') && c.has('c'), 'byte pressure must evict LRU');
assert(released.join() === 'b', 'eviction must release PixelMap once');
c.keep('a'); c.keep('a'); c.setLimit(-1);
assert(c.has('a') && !c.has('c'), 'zero budget must preserve kept image');
c.release('a'); assert(c.has('a'), 'second pin must remain');
c.release('a'); assert(!c.has('a'), 'last pin must release at zero budget');
assert(released.join() === 'b,c,a', 'each evicted PixelMap must release once');

c.setLimit(1000);
c.addPending('shared');
let first = 0, second = 0;
const repaintFirst = () => { assert(c.get('shared') !== undefined, 'loaded before repaint'); first++; };
const repaintSecond = () => { assert(c.get('shared') !== undefined, 'loaded before repaint'); second++; };
c.wait('shared', repaintFirst); c.wait('shared', repaintFirst); c.wait('shared', repaintSecond);
c.endPass(); c.setLimit(0);
assert(c.has('shared'), 'pending decode must survive pressure');
c.complete('shared', image('shared'), 100);
assert(first === 1 && second === 1, 'every waiting canvas must repaint once');
c.endPass(); assert(!c.has('shared'), 'completed decode must obey zero budget');

c.setLimit(1000);
c.addPending('failed'); c.wait('failed', repaintFirst); c.fail('failed'); c.endPass();
c.setLimit(0); assert(!c.has('failed'), 'failed source must be evictable');
assert(first === 1, 'failure must not repaint or retain waiters');

c.setLimit(32 * 1024 * 1024);
for (let i = 0; i < 100; i++) put('source-' + i, 1);
let retained = 0;
for (let i = 0; i < 100; i++) if (c.has('source-' + i)) retained++;
assert(retained === 64, 'unkept source ceiling must hold');
`)
	if err != nil {
		t.Fatal(err)
	}
}
