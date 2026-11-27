package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf16"
	"unicode/utf8"
)

// webTestPage runs the test binary in the page: its arguments and
// environment come in the query, its exit goes to the tool through the
// nsgoTest binding.
const webTestPage = `<!doctype html>
<html>
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<style>html, body { margin: 0; height: 100%; }</style>
<script src="wasm_exec.js"></script>
</head>
<body>
<script>
const query = new URLSearchParams(location.search);
const go = new Go();
go.argv = ["test"].concat(JSON.parse(query.get("args") || "[]"));
go.env = JSON.parse(query.get("env") || "{}");
go.env.NSGO_BASE = new URL(".", document.baseURI).href;
go.exit = (code) => nsgoTest(JSON.stringify({ op: "exit", code }));
WebAssembly.instantiateStreaming(fetch("app.wasm"), go.importObject)
  .then((r) => {
    globalThis.__nsgoWasm = {
      module: r.module,
      exec: new URL("wasm_exec.js", document.baseURI).href,
      worker: new URL("nsgo-worker.js", document.baseURI).href,
    };
    go.env.NSGO_WASM = "__nsgoWasm";
    go.run(r.instance);
  })
  .catch((e) => { console.error(String(e)); go.exit(2); });
</script>
</body>
</html>
`

// webTestRunner serves the test build on this machine and runs each test
// in a page of a headless browser, in a browser context of its own.
func webTestRunner(ctx context.Context, p *project, dir string, opts testOptions) (targetRunner, error) {
	if err := buildWeb(ctx, p, dir, true); err != nil {
		return targetRunner{}, err
	}
	if err := os.WriteFile(filepath.Join(dir, "test.html"), []byte(webTestPage), 0o644); err != nil {
		return targetRunner{}, err
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return targetRunner{}, err
	}
	srv := &http.Server{Handler: http.FileServer(http.Dir(dir))}
	go srv.Serve(ln)
	browser, err := startBrowser(ctx)
	if err != nil {
		srv.Close()
		return targetRunner{}, err
	}
	base := fmt.Sprintf("http://%s/test.html", ln.Addr())
	return targetRunner{
		run: func(ctx context.Context, args, env []string, out io.Writer) (int, error) {
			return runWebTest(ctx, browser, base, args, env, out)
		},
		close: func() {
			browser.close()
			srv.Close()
		},
	}, nil
}

// webModifiers is the modifier state the page sends across the test bridge.
type webModifiers struct {
	Control, Alt, Shift, Platform bool
}

// webOp is what the page asks the tool to do.
type webOp struct {
	ID        int          `json:"id"`
	Op        string       `json:"op"`
	X         float64      `json:"x"`
	Y         float64      `json:"y"`
	DX        float64      `json:"dx"`
	DY        float64      `json:"dy"`
	Hold      int64        `json:"hold,omitempty"`
	Modifiers webModifiers `json:"modifiers"`
	Path      []struct {
		X, Y float64
	} `json:"path,omitempty"`
	Scales []float32 `json:"scales,omitempty"`
	Text   string    `json:"text,omitempty"`
	Key    string    `json:"key,omitempty"`
	Button string    `json:"button,omitempty"`
	Width  float64   `json:"width,omitempty"`
	Height float64   `json:"height,omitempty"`
	Code   int       `json:"code,omitempty"`
	// Position is the physical position keyCodeDown and keyCodeUp press, as KeyboardEvent.code names it; Key is then the US layout's key there.
	Position string `json:"position,omitempty"`
}

// webAutomationWidth and webAutomationHeight are the CSS viewport size of nsgo dev -target web -automation's browser.
const (
	webAutomationWidth  = 1280
	webAutomationHeight = 800
)

// webPageOptions configure a page opened over CDP with the nsgoTest binding.
type webPageOptions struct {
	url           string
	width, height int
	out           io.Writer
	// onExit, when set, handles an "exit" op instead of answering it through the page.
	onExit func(int)
}

// openWebPage opens a page in a fresh browser context of b, installs the nsgoTest binding at opts.width×opts.height CSS pixels, routes console output and binding calls, and navigates to opts.url.
func openWebPage(b *cdp, opts webPageOptions) (*webPage, func(), error) {
	raw, err := b.call("", "Target.createBrowserContext", map[string]any{"disposeOnDetach": true})
	if err != nil {
		return nil, nil, err
	}
	var bctx struct{ BrowserContextID string }
	json.Unmarshal(raw, &bctx)
	var cleanOnce sync.Once
	cleanup := func() {
		cleanOnce.Do(func() {
			b.mu.Lock()
			b.on = nil
			b.mu.Unlock()
			b.call("", "Target.disposeBrowserContext", map[string]any{"browserContextId": bctx.BrowserContextID})
		})
	}
	raw, err = b.call("", "Target.createTarget", map[string]any{"url": "about:blank", "browserContextId": bctx.BrowserContextID})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	var target struct{ TargetID string }
	json.Unmarshal(raw, &target)
	raw, err = b.call("", "Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true})
	if err != nil {
		cleanup()
		return nil, nil, err
	}
	var attached struct{ SessionID string }
	json.Unmarshal(raw, &attached)
	session := attached.SessionID
	page := &webPage{b: b, session: session}

	var outMu sync.Mutex
	write := func(s string) {
		if opts.out == nil {
			return
		}
		outMu.Lock()
		io.WriteString(opts.out, s)
		outMu.Unlock()
	}
	b.mu.Lock()
	b.on = func(m cdpMessage) {
		if m.SessionID != session {
			return
		}
		switch m.Method {
		case "Runtime.consoleAPICalled":
			write(formatConsoleEvent(m.Params))
		case "Runtime.exceptionThrown":
			write(formatExceptionEvent(m.Params))
		case "Page.frameNavigated":
			var ev struct {
				Frame struct {
					ParentID string `json:"parentId"`
				} `json:"frame"`
			}
			if json.Unmarshal(m.Params, &ev) == nil && ev.Frame.ParentID == "" {
				page.resetHeld()
			}
		case "Runtime.bindingCalled":
			var ev struct{ Name, Payload string }
			json.Unmarshal(m.Params, &ev)
			var op webOp
			if ev.Name != "nsgoTest" || json.Unmarshal([]byte(ev.Payload), &op) != nil {
				return
			}
			if op.Op == "exit" && opts.onExit != nil {
				opts.onExit(op.Code)
				return
			}
			// Off the reading goroutine: an op calls the browser itself.
			go page.answer(op)
		}
	}
	b.mu.Unlock()
	for _, call := range []struct {
		method string
		params any
	}{
		{"Runtime.enable", nil},
		{"Page.enable", nil},
		{"Runtime.addBinding", map[string]any{"name": "nsgoTest"}},
		{"Emulation.setDeviceMetricsOverride", map[string]any{"width": opts.width, "height": opts.height, "deviceScaleFactor": webTestScale, "mobile": false}},
	} {
		if _, err := b.call(session, call.method, call.params); err != nil {
			cleanup()
			return nil, nil, err
		}
	}
	if _, err := b.call(session, "Page.navigate", map[string]any{"url": opts.url}); err != nil {
		cleanup()
		return nil, nil, err
	}
	return page, cleanup, nil
}

// openWebAutomation opens targetURL in b with the nsgoTest binding installed for nsgo dev -target web -automation, writing console output and uncaught exceptions to out.
func openWebAutomation(b *cdp, targetURL string, out io.Writer) (*webPage, func(), error) {
	return openWebPage(b, webPageOptions{
		url:    targetURL,
		width:  webAutomationWidth,
		height: webAutomationHeight,
		out:    out,
	})
}

func formatConsoleEvent(params json.RawMessage) string {
	var ev struct {
		Args []struct {
			Value       any
			Description string
		}
	}
	json.Unmarshal(params, &ev)
	var parts []string
	for _, a := range ev.Args {
		if s, ok := a.Value.(string); ok {
			parts = append(parts, s)
		} else if a.Description != "" {
			parts = append(parts, a.Description)
		} else {
			parts = append(parts, fmt.Sprint(a.Value))
		}
	}
	return strings.Join(parts, " ") + "\n"
}

func formatExceptionEvent(params json.RawMessage) string {
	var ev struct {
		ExceptionDetails struct {
			Text      string
			Exception struct{ Description string }
		}
	}
	json.Unmarshal(params, &ev)
	return "uncaught: " + ev.ExceptionDetails.Text + " " + ev.ExceptionDetails.Exception.Description + "\n"
}

// runWebTest runs one test's process: a page in a fresh context.
func runWebTest(ctx context.Context, b *cdp, base string, args, env []string, out io.Writer) (int, error) {
	argData, _ := json.Marshal(args)
	envMap := map[string]string{}
	for _, kv := range env {
		k, v, _ := strings.Cut(kv, "=")
		envMap[k] = v
	}
	envData, _ := json.Marshal(envMap)
	u := base + "?args=" + url.QueryEscape(string(argData)) + "&env=" + url.QueryEscape(string(envData))
	exited := make(chan int, 1)
	_, cleanup, err := openWebPage(b, webPageOptions{
		url:    u,
		width:  390,
		height: 844,
		out:    out,
		onExit: func(code int) {
			select {
			case exited <- code:
			default:
			}
		},
	})
	if err != nil {
		return -1, err
	}
	defer cleanup()
	select {
	case code := <-exited:
		return code, nil
	case <-ctx.Done():
		return -1, ctx.Err()
	}
}

// webPage is a test's page and its session.
type webPage struct {
	b       *cdp
	session string
	// heldMu guards held, the modifiers bitmask of the modifier keys the keyDown op holds, which every key event after it carries, down, the physical keys it holds with the modifier bit of each, and buttons, the held mouse button bitmask.
	heldMu  sync.Mutex
	held    int
	down    map[string]int
	buttons int
}

func (p *webPage) resetHeld() {
	p.heldMu.Lock()
	p.held = 0
	p.down = nil
	p.heldMu.Unlock()
}

func (p *webPage) call(method string, params any) (json.RawMessage, error) {
	return p.b.call(p.session, method, params)
}

func (p *webPage) mouse(kind string, x, y float64, button string, buttons, clicks int) error {
	return p.mouseWith(kind, x, y, button, buttons, clicks, webModifiers{})
}

func (p *webPage) mouseWith(kind string, x, y float64, button string, buttons, clicks int, mods webModifiers) error {
	var flags int
	if mods.Alt {
		flags |= 1
	}
	if mods.Control {
		flags |= 2
	}
	if mods.Platform {
		flags |= 4
	}
	if mods.Shift {
		flags |= 8
	}
	params := map[string]any{"type": kind, "x": x, "y": y, "button": button, "buttons": buttons, "clickCount": clicks, "modifiers": flags}
	_, err := p.call("Input.dispatchMouseEvent", params)
	return err
}

// answer does op with the browser's trusted input and tells the page.
func (p *webPage) answer(op webOp) {
	var data any
	err := p.do(op, &data)
	r := map[string]any{"data": data}
	if err != nil {
		r["error"] = err.Error()
	}
	reply, _ := json.Marshal(r)
	p.call("Runtime.evaluate", map[string]any{"expression": fmt.Sprintf("nsgoTestReply(%d, %s)", op.ID, mustJSON(string(reply)))})
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// webTestScale is the device scale of the page tests run in.
const webTestScale = 2

// pinch sends two trusted touches around one center. The browser finds
// their targets and dispatches the pointer events the app uses.
func (p *webPage) pinch(op webOp) error {
	type touchPoint struct {
		X  float64 `json:"x"`
		Y  float64 `json:"y"`
		ID int     `json:"id"`
	}
	points := func(scale float32) []touchPoint {
		const radius = 12
		d := radius * float64(scale)
		return []touchPoint{{X: op.X - d, Y: op.Y, ID: 0}, {X: op.X + d, Y: op.Y, ID: 1}}
	}
	send := func(kind string, points []touchPoint) error {
		_, err := p.call("Input.dispatchTouchEvent", map[string]any{"type": kind, "touchPoints": points})
		return err
	}
	if err := send("touchStart", points(1)); err != nil {
		return err
	}
	for _, scale := range op.Scales {
		time.Sleep(16 * time.Millisecond)
		if err := send("touchMove", points(scale)); err != nil {
			_ = send("touchCancel", []touchPoint{})
			return err
		}
	}
	return send("touchEnd", []touchPoint{})
}

// touchDrag sends a single trusted touch dragged along op.Path so the browser
// runs its touch-action checks, touchmove listeners and native scroll handling.
func (p *webPage) touchDrag(op webOp) error {
	if len(op.Path) < 2 {
		return fmt.Errorf("a touch drag needs at least two points")
	}
	type touchPoint struct {
		X  float64 `json:"x"`
		Y  float64 `json:"y"`
		ID int     `json:"id"`
	}
	send := func(kind string, points []touchPoint) error {
		_, err := p.call("Input.dispatchTouchEvent", map[string]any{"type": kind, "touchPoints": points})
		return err
	}
	x, y := op.Path[0].X, op.Path[0].Y
	if err := send("touchStart", []touchPoint{{X: x, Y: y, ID: 0}}); err != nil {
		return err
	}
	for _, to := range op.Path[1:] {
		n := int(math.Max(math.Abs(to.X-x), math.Abs(to.Y-y))/10) + 1
		for i := 1; i <= n; i++ {
			time.Sleep(16 * time.Millisecond)
			f := float64(i) / float64(n)
			if err := send("touchMove", []touchPoint{{X: x + (to.X-x)*f, Y: y + (to.Y-y)*f, ID: 0}}); err != nil {
				_ = send("touchCancel", []touchPoint{})
				return err
			}
		}
		x, y = to.X, to.Y
	}
	time.Sleep(16 * time.Millisecond)
	return send("touchEnd", []touchPoint{})
}

func (p *webPage) do(op webOp, data *any) error {
	switch op.Op {
	case "viewport":
		_, err := p.call("Emulation.setDeviceMetricsOverride", map[string]any{
			"width": int(math.Round(op.Width)), "height": int(math.Round(op.Height)), "deviceScaleFactor": webTestScale, "mobile": false})
		return err
	case "press":
		if err := p.mouseWith("mouseMoved", op.X, op.Y, "none", 0, 0, op.Modifiers); err != nil {
			return err
		}
		if err := p.mouseWith("mousePressed", op.X, op.Y, "left", 1, 1, op.Modifiers); err != nil {
			return err
		}
		if op.Hold > 0 {
			time.Sleep(time.Duration(op.Hold+50) * time.Millisecond)
		}
		return p.mouseWith("mouseReleased", op.X, op.Y, "left", 0, 1, op.Modifiers)
	case "rightClick":
		if err := p.mouse("mousePressed", op.X, op.Y, "right", 2, 1); err != nil {
			return err
		}
		return p.mouse("mouseReleased", op.X, op.Y, "right", 0, 1)
	case "mouseDown", "mouseUp":
		bit := map[string]int{"left": 1, "right": 2, "middle": 4, "back": 8, "forward": 16}[op.Button]
		if bit == 0 {
			return fmt.Errorf("unknown mouse button %q", op.Button)
		}
		p.heldMu.Lock()
		if op.Op == "mouseDown" {
			p.buttons |= bit
		} else {
			p.buttons &^= bit
		}
		buttons := p.buttons
		p.heldMu.Unlock()
		kind := "mouseReleased"
		if op.Op == "mouseDown" {
			kind = "mousePressed"
		}
		return p.mouseWith(kind, op.X, op.Y, op.Button, buttons, 1, op.Modifiers)
	case "blurWindow":
		p.heldMu.Lock()
		p.held, p.down, p.buttons = 0, nil, 0
		p.heldMu.Unlock()
		_, err := p.call("Runtime.evaluate", map[string]any{"expression": `window.dispatchEvent(new Event("blur"))`})
		return err
	case "hover":
		return p.mouse("mouseMoved", op.X, op.Y, "none", 0, 0)
	case "drag":
		if len(op.Path) == 0 {
			return fmt.Errorf("an empty drag")
		}
		x, y := op.Path[0].X, op.Path[0].Y
		p.mouse("mouseMoved", x, y, "none", 0, 0)
		if err := p.mouse("mousePressed", x, y, "left", 1, 1); err != nil {
			return err
		}
		time.Sleep(100 * time.Millisecond)
		for _, to := range op.Path[1:] {
			n := int(math.Max(math.Abs(to.X-x), math.Abs(to.Y-y))/10) + 1
			for i := 1; i <= n; i++ {
				f := float64(i) / float64(n)
				p.mouse("mouseMoved", x+(to.X-x)*f, y+(to.Y-y)*f, "left", 1, 0)
				time.Sleep(16 * time.Millisecond)
			}
			x, y = to.X, to.Y
		}
		return p.mouse("mouseReleased", x, y, "left", 0, 1)
	case "pinch":
		return p.pinch(op)
	case "touchDrag":
		return p.touchDrag(op)
	case "scroll":
		p.mouse("mouseMoved", op.X, op.Y, "none", 0, 0)
		n := int(math.Max(math.Abs(op.DX), math.Abs(op.DY))/20) + 1
		// Small pixel deltas follow the touchpad path, including OnWheel.
		for range n {
			if _, err := p.call("Input.dispatchMouseEvent", map[string]any{"type": "mouseWheel", "x": op.X, "y": op.Y,
				"deltaX": op.DX / float64(n), "deltaY": op.DY / float64(n)}); err != nil {
				return err
			}
			time.Sleep(16 * time.Millisecond)
		}
		return nil
	case "type":
		for _, r := range op.Text {
			if r == '\n' {
				if err := p.key("enter"); err != nil {
					return err
				}
				continue
			}
			s := string(r)
			for _, kind := range []string{"keyDown", "keyUp"} {
				params := map[string]any{"type": kind, "key": s}
				if kind == "keyDown" {
					params["text"], params["unmodifiedText"] = s, s
				}
				if _, err := p.call("Input.dispatchKeyEvent", params); err != nil {
					return err
				}
			}
		}
		return nil
	case "compose":
		// Chrome's input method path: the page hears the composition
		// events an IME's marked text makes.
		n := len(utf16.Encode([]rune(op.Text)))
		_, err := p.call("Input.imeSetComposition", map[string]any{"text": op.Text, "selectionStart": n, "selectionEnd": n})
		return err
	case "commit":
		_, err := p.call("Input.insertText", map[string]any{"text": op.Text})
		return err
	case "key":
		return p.key(op.Key)
	case "keyDown":
		return p.keyDown(op.Key)
	case "keyUp":
		return p.keyUp(op.Key)
	case "keyCodeDown", "keyCodeUp":
		k, location, name, err := positionWebKey(op.Position, op.Key)
		if err != nil {
			return err
		}
		if op.Op == "keyCodeDown" {
			return p.holdDown(k, location, name)
		}
		return p.holdUp(k, location, op.Position)
	case "screenshot":
		raw, err := p.call("Page.captureScreenshot", map[string]any{"format": "png"})
		if err != nil {
			return err
		}
		var shot struct{ Data string }
		json.Unmarshal(raw, &shot)
		*data = shot.Data
		return nil
	}
	return fmt.Errorf("unknown op %q", op.Op)
}

// cdpKey is a key as the browser's key events name it: its DOM key and code, its Windows key code and the text it types.
type cdpKey struct {
	key, code string
	vk        int
	text      string
}

// cdpKeys are the DOM key, code and Windows key code of the keys tests name.
var cdpKeys = map[string]cdpKey{
	"enter": {"Enter", "Enter", 13, "\r"}, "return": {"Enter", "Enter", 13, "\r"}, "backspace": {"Backspace", "Backspace", 8, ""},
	"escape": {"Escape", "Escape", 27, ""}, "tab": {"Tab", "Tab", 9, ""}, "space": {" ", "Space", 32, " "},
	"delete": {"Delete", "Delete", 46, ""}, "left": {"ArrowLeft", "ArrowLeft", 37, ""}, "up": {"ArrowUp", "ArrowUp", 38, ""},
	"right": {"ArrowRight", "ArrowRight", 39, ""}, "down": {"ArrowDown", "ArrowDown", 40, ""},
	"home": {"Home", "Home", 36, ""}, "end": {"End", "End", 35, ""}, "pageup": {"PageUp", "PageUp", 33, ""},
	"pagedown": {"PageDown", "PageDown", 34, ""}, "shift": {"Shift", "ShiftLeft", 16, ""},
	"ctrl": {"Control", "ControlLeft", 17, ""}, "control": {"Control", "ControlLeft", 17, ""},
	"alt": {"Alt", "AltLeft", 18, ""}, "option": {"Alt", "AltLeft", 18, ""},
	"meta": {"Meta", "MetaLeft", 91, ""}, "cmd": {"Meta", "MetaLeft", 91, ""}, "command": {"Meta", "MetaLeft", 91, ""},
}

// macCommands are the editing commands Chrome on macOS runs for keys, as Playwright sends them: without them its headless build edits nothing.
var macCommands = map[string]string{
	"backspace": "deleteBackward", "delete": "deleteForward", "left": "moveLeft", "right": "moveRight",
	"up": "moveUp", "down": "moveDown", "home": "moveToBeginningOfLine", "end": "moveToEndOfLine",
	"shift+left": "moveLeftAndModifySelection", "shift+right": "moveRightAndModifySelection",
	"meta+a": "selectAll", "meta+c": "copy", "meta+x": "cut", "meta+v": "paste", "meta+z": "undo",
}

// key presses key ("Enter", "mod+a") with the browser's key events, with the modifiers keyDown holds as well as its own.
func (p *webPage) key(key string) error {
	k, name, mods, err := parseWebKey(key)
	if err != nil {
		return err
	}
	p.heldMu.Lock()
	mods |= p.held
	p.heldMu.Unlock()
	if err := p.sendKeyDown(k, 0, name, mods|modifierBit(k.key), false); err != nil {
		return err
	}
	return p.sendKeyUp(k, 0, mods)
}

// keyDown holds key, one key without modifiers, down until keyUp: a modifier's bit joins every key event after it. A key it already holds goes down again as an auto-repeat.
func (p *webPage) keyDown(key string) error {
	k, name, mods, err := parseWebKey(key)
	if err != nil {
		return err
	}
	if mods != 0 {
		return fmt.Errorf("%q is a chord: a key held is one key", key)
	}
	return p.holdDown(k, 0, name)
}

// positionKeys are the DOM key location and Windows key code of positions whose US key names another position's: the right-hand modifiers, and the keypad's digits and point, which Num Lock types.
var positionKeys = map[string]struct{ location, vk int }{
	"ShiftRight": {2, 16}, "ControlRight": {2, 17}, "AltRight": {2, 18}, "MetaRight": {2, 92}, "NumpadDecimal": {3, 110},
	"Numpad0": {3, 96}, "Numpad1": {3, 97}, "Numpad2": {3, 98}, "Numpad3": {3, 99}, "Numpad4": {3, 100},
	"Numpad5": {3, 101}, "Numpad6": {3, 102}, "Numpad7": {3, 103}, "Numpad8": {3, 104}, "Numpad9": {3, 105},
}

// positionWebKey is the key event for the physical position code, carrying name, the US layout's key there, with the position as its code, and its DOM key location: 0 for the standard keys, 2 for a right-hand modifier and 3 for the keypad.
func positionWebKey(code, name string) (k cdpKey, location int, _ string, _ error) {
	k, name, mods, err := parseWebKey(name)
	if err != nil {
		return k, 0, "", err
	}
	if mods != 0 {
		return k, 0, "", fmt.Errorf("%q is a chord: a position holds one key", name)
	}
	k.code = code
	if pk, ok := positionKeys[code]; ok {
		location, k.vk = pk.location, pk.vk
	}
	return k, location, name, nil
}

// holdDown sends k going down and holds it, a modifier's bit joining the events after it, as keyDown does.
func (p *webPage) holdDown(k cdpKey, location int, name string) error {
	p.heldMu.Lock()
	if p.down == nil {
		p.down = map[string]int{}
	}
	_, repeat := p.down[heldCode(k)]
	p.down[heldCode(k)] = modifierBit(k.key)
	p.held |= modifierBit(k.key)
	mods := p.held
	p.heldMu.Unlock()
	return p.sendKeyDown(k, location, name, mods, repeat)
}

// keyUp lets key, which keyDown holds, come up, its own modifier bit no longer among those its event and the ones after it carry.
func (p *webPage) keyUp(key string) error {
	k, _, mods, err := parseWebKey(key)
	if err != nil {
		return err
	}
	if mods != 0 {
		return fmt.Errorf("%q is a chord: a key held is one key", key)
	}
	return p.holdUp(k, 0, key)
}

// holdUp lets k, which holdDown held, come up, as keyUp does.
func (p *webPage) holdUp(k cdpKey, location int, key string) error {
	p.heldMu.Lock()
	if _, held := p.down[heldCode(k)]; !held {
		p.heldMu.Unlock()
		return fmt.Errorf("%q is not held", key)
	}
	delete(p.down, heldCode(k))
	// A modifier stays held while its other key is, as KeyDown of its name and KeyCodeDown of its other side can hold both.
	p.held = 0
	for _, bit := range p.down {
		p.held |= bit
	}
	mods := p.held
	p.heldMu.Unlock()
	return p.sendKeyUp(k, location, mods)
}

// heldCode is the physical key k is held by, as the page pairs a key's up with its down.
func heldCode(k cdpKey) string {
	if k.code != "" {
		return k.code
	}
	return k.key
}

// parseWebKey returns the key key ("Enter", "mod+a") names, its name without modifiers and the modifiers bitmask of its chord.
func parseWebKey(key string) (k cdpKey, name string, mods int, err error) {
	parts := strings.Split(strings.ToLower(key), "+")
	name = parts[len(parts)-1]
	for _, m := range parts[:len(parts)-1] {
		switch m {
		case "mod":
			if runtime.GOOS == "darwin" {
				mods |= 4
			} else {
				mods |= 2
			}
		case "cmd", "meta", "command":
			mods |= 4
		case "ctrl", "control":
			mods |= 2
		case "alt", "option":
			mods |= 1
		case "shift":
			mods |= 8
		default:
			return k, "", 0, fmt.Errorf("unknown modifier %q in %q", m, key)
		}
	}
	k, ok := cdpKeys[name]
	if !ok && utf8.RuneCountInString(name) == 1 {
		c := name[0]
		switch {
		case c >= 'a' && c <= 'z':
			k.key, k.code, k.vk, k.text = name, "Key"+strings.ToUpper(name), int(c-'a')+65, name
		case c >= '0' && c <= '9':
			k.key, k.code, k.vk, k.text = name, "Digit"+name, int(c), name
		default:
			k.key, k.text = name, name
		}
		ok = true
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(name, "f")); !ok && err == nil && name[0] == 'f' && n >= 1 && n <= 24 {
		f := "F" + strconv.Itoa(n)
		k.key, k.code, k.vk, ok = f, f, 111+n, true
	}
	if !ok {
		return k, "", 0, fmt.Errorf("unknown key %q", key)
	}
	return k, name, mods, nil
}

// modifierBit is the bit a modifier key, by its DOM key, sets in a key event's modifiers bitmask, or 0 for another key.
func modifierBit(key string) int {
	switch key {
	case "Shift":
		return 8
	case "Control":
		return 2
	case "Alt":
		return 1
	case "Meta":
		return 4
	}
	return 0
}

// sendKeyDown sends k going down with mods held, its own bit among them when it is a modifier, as the browser reports a modifier pressed alone.
func (p *webPage) sendKeyDown(k cdpKey, location int, name string, mods int, repeat bool) error {
	down := map[string]any{"type": "rawKeyDown", "key": k.key, "code": k.code, "windowsVirtualKeyCode": k.vk, "modifiers": mods, "location": location}
	if k.text != "" && mods&^8 == 0 {
		down["type"], down["text"], down["unmodifiedText"] = "keyDown", k.text, k.text
	}
	if repeat {
		down["autoRepeat"] = true
	}
	if runtime.GOOS == "darwin" {
		var prefix string
		for _, m := range []struct {
			bit  int
			name string
		}{{2, "ctrl+"}, {1, "alt+"}, {8, "shift+"}, {4, "meta+"}} {
			if mods&^modifierBit(k.key)&m.bit != 0 {
				prefix += m.name
			}
		}
		if c := macCommands[prefix+name]; c != "" {
			down["commands"] = []string{c}
		}
	}
	_, err := p.call("Input.dispatchKeyEvent", down)
	return err
}

// sendKeyUp sends k coming up with mods still held.
func (p *webPage) sendKeyUp(k cdpKey, location int, mods int) error {
	_, err := p.call("Input.dispatchKeyEvent", map[string]any{"type": "keyUp", "key": k.key, "code": k.code,
		"windowsVirtualKeyCode": k.vk, "modifiers": mods, "location": location})
	return err
}
