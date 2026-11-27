//go:build js && wasm && !nsgoharmony && !nsgominiprogram

package formula

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/caelyreth/untie"
)

const (
	formulaWorkerEnvKey = "NSGO_FORMULA_WORKER"
	workerBootTimeout   = 15 * time.Second
)

// inlineWorkerScript is the fallback bootstrap when a page embeds an NSGo app
// cross-origin (such as from a CDN), where browsers forbid constructing a
// Worker directly from a cross-origin script URL. Same-origin pages use the
// static nsgo-worker.js file first so strict Content-Security-Policy holds.
const inlineWorkerScript = `self.onmessage = async (e) => {
  const msg = e.data;
  if (!msg || msg.kind !== "init") return;
  try {
    importScripts(msg.exec);
    const go = new Go();
    go.env = msg.env || {};
    const instance = await WebAssembly.instantiate(msg.module, go.importObject);
    go.run(instance);
  } catch (err) {
    self.postMessage({ kind: "bootError", error: String(err && err.message || err) });
  }
};
`

func init() {
	if os.Getenv(formulaWorkerEnvKey) != "1" {
		return
	}
	serveFormulaWorker()
}

// serveFormulaWorker runs inside the dedicated Web Worker, handling font loads
// and formula layouts without touching the page's main thread or running main.
func serveFormulaWorker() {
	self := js.Global()
	uint8Array := self.Get("Uint8Array")
	engines := map[uint64]*untie.Engine{}

	sendError := func(id int, err error) {
		self.Call("postMessage", map[string]any{
			"kind":  "reply",
			"id":    id,
			"ok":    false,
			"error": err.Error(),
		})
	}

	sendBox := func(id int, box *untie.Box) {
		raw := encodeBox(box)
		arr := uint8Array.New(len(raw))
		if len(raw) > 0 {
			js.CopyBytesToJS(arr, raw)
		}
		buf := arr.Get("buffer")
		self.Call("postMessage", map[string]any{
			"kind": "reply",
			"id":   id,
			"ok":   true,
			"box":  arr,
		}, []any{buf})
	}

	readBytes := func(v js.Value) []byte {
		if !v.Truthy() {
			return nil
		}
		n := v.Get("byteLength").Int()
		if n == 0 {
			return nil
		}
		out := make([]byte, n)
		js.CopyBytesToGo(out, v)
		return out
	}

	onMessage := js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		msg := args[0].Get("data")
		if !msg.Truthy() {
			return nil
		}
		kind := msg.Get("kind").String()
		id := msg.Get("id").Int()
		rendererID := uint64(msg.Get("rendererID").Float())
		defer func() {
			if r := recover(); r != nil && id != 0 {
				sendError(id, fmt.Errorf("formula worker panic: %v", r))
			}
		}()

		switch kind {
		case "load":
			var fonts Fonts
			if msg.Get("useDefault").Bool() {
				fonts.Math = newCMMath
			} else {
				fonts.Math = readBytes(msg.Get("math"))
			}
			fonts.Text = readBytes(msg.Get("text"))
			if fbs := msg.Get("fallbacks"); fbs.Truthy() {
				n := fbs.Length()
				fonts.Fallbacks = make([][]byte, n)
				for i := range n {
					fonts.Fallbacks[i] = readBytes(fbs.Index(i))
				}
			}
			engine, err := loadUntieEngine(fonts)
			if err != nil {
				sendError(id, err)
				return nil
			}
			engines[rendererID] = engine
			self.Call("postMessage", map[string]any{
				"kind": "reply",
				"id":   id,
				"ok":   true,
			})

		case "layout":
			engine := engines[rendererID]
			if engine == nil {
				sendError(id, errors.New("formula: unknown renderer"))
				return nil
			}
			req, err := decodeLayoutRequest(readBytes(msg.Get("payload")))
			if err != nil {
				sendError(id, err)
				return nil
			}
			expr := untie.Parse(req.source, req.dialect, req.mode, untie.ParseOptions{})
			box, err := engine.LayoutContextWithOptions(context.Background(), expr, req.style, req.opts)
			if err != nil {
				sendError(id, err)
				return nil
			}
			sendBox(id, box)

		case "systemText":
			engine := engines[rendererID]
			if engine == nil {
				sendError(id, errors.New("formula: unknown renderer"))
				return nil
			}
			req, err := decodeSystemTextRequest(readBytes(msg.Get("payload")))
			if err != nil {
				sendError(id, err)
				return nil
			}
			dialect, mode := untie.TeX, untie.Inline
			if req.key.dialect == Typst {
				dialect = untie.Typst
			}
			if req.key.display {
				mode = untie.Display
			}
			expr := untie.Parse(req.key.source, dialect, mode, untie.ParseOptions{})
			root, err := expr.RootSnapshot()
			if err != nil {
				sendError(id, err)
				return nil
			}
			replaySystemText(root, req.missing, req.language, req.measured)
			box, err := engine.LayoutFormulaContextWithOptions(context.Background(), root, mode, req.key.style(), untie.LayoutOptions{Recovery: untie.RecoveryVisible})
			if err != nil {
				sendError(id, err)
				return nil
			}
			sendBox(id, box)

		case "drop":
			delete(engines, rendererID)
		}
		return nil
	})
	self.Set("onmessage", onMessage)
	self.Call("postMessage", map[string]any{"kind": "ready"})
	select {}
}

type workerReply struct {
	ok    bool
	err   string
	data  []byte
	fatal bool
}

type formulaWorker struct {
	worker  js.Value
	onMsg   js.Func
	onErr   js.Func
	nextID  atomic.Int64
	mu      sync.Mutex
	closed  bool
	pending map[int]chan workerReply
}

var (
	sharedWorkerOnce sync.Once
	sharedWorkerInst *formulaWorker
	nextRendererID   atomic.Uint64
)

func getFormulaWorker() *formulaWorker {
	sharedWorkerOnce.Do(func() {
		sharedWorkerInst = startFormulaWorker()
	})
	w := sharedWorkerInst
	if w == nil {
		return nil
	}
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return nil
	}
	return w
}

func startFormulaWorker() *formulaWorker {
	key := os.Getenv("NSGO_WASM")
	if key == "" {
		return nil
	}
	g := js.Global()
	if !g.Get("Worker").Truthy() {
		return nil
	}
	info := g.Get(key)
	if !info.Truthy() {
		return nil
	}
	module := info.Get("module")
	execURL := info.Get("exec")
	if !module.Truthy() || !execURL.Truthy() {
		return nil
	}
	workerURL := ""
	if v := info.Get("worker"); v.Type() == js.TypeString {
		workerURL = v.String()
	}

	// Prefer the same-origin static worker script so strict CSP holds without
	// blob: workers; fall back to a Blob URL when hosted cross-origin (CDN).
	if workerURL != "" {
		if w := bootWorker(g, workerURL, "", module, execURL); w != nil {
			return w
		}
	}
	if !g.Get("Blob").Truthy() || !g.Get("URL").Truthy() {
		return nil
	}
	blob := g.Get("Blob").New([]any{inlineWorkerScript}, map[string]any{"type": "text/javascript"})
	blobURL := g.Get("URL").Call("createObjectURL", blob).String()
	return bootWorker(g, blobURL, blobURL, module, execURL)
}

func constructWorker(g js.Value, scriptURL string) (w js.Value, ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	return g.Get("Worker").New(scriptURL), true
}

func bootWorker(g js.Value, scriptURL, revokeURL string, module, execURL js.Value) *formulaWorker {
	if revokeURL != "" {
		defer g.Get("URL").Call("revokeObjectURL", revokeURL)
	}
	raw, ok := constructWorker(g, scriptURL)
	if !ok || !raw.Truthy() {
		return nil
	}
	ready := make(chan error, 1)
	var readyOnce sync.Once
	signalReady := func(err error) {
		readyOnce.Do(func() { ready <- err })
	}

	fw := &formulaWorker{
		worker:  raw,
		pending: map[int]chan workerReply{},
	}
	fw.onMsg = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		msg := args[0].Get("data")
		if !msg.Truthy() {
			return nil
		}
		switch msg.Get("kind").String() {
		case "ready":
			signalReady(nil)
		case "bootError":
			signalReady(errors.New(msg.Get("error").String()))
		case "reply":
			id := msg.Get("id").Int()
			rep := workerReply{ok: msg.Get("ok").Bool()}
			if !rep.ok {
				rep.err = msg.Get("error").String()
			} else if box := msg.Get("box"); box.Truthy() {
				n := box.Get("byteLength").Int()
				if n > 0 {
					rep.data = make([]byte, n)
					js.CopyBytesToGo(rep.data, box)
				}
			}
			fw.deliver(id, rep)
		}
		return nil
	})
	fw.onErr = js.FuncOf(func(_ js.Value, args []js.Value) any {
		msg := "formula worker error"
		if len(args) > 0 && args[0].Truthy() {
			if m := args[0].Get("message"); m.Type() == js.TypeString && m.String() != "" {
				msg = m.String()
			}
		}
		signalReady(errors.New(msg))
		fw.failAll(msg)
		return nil
	})
	raw.Set("onmessage", fw.onMsg)
	raw.Set("onerror", fw.onErr)
	raw.Call("postMessage", map[string]any{
		"kind":   "init",
		"module": module,
		"exec":   execURL,
		"env":    map[string]any{formulaWorkerEnvKey: "1"},
	})

	timer := time.NewTimer(workerBootTimeout)
	defer timer.Stop()
	select {
	case err := <-ready:
		if err != nil {
			fw.shutdown()
			return nil
		}
		return fw
	case <-timer.C:
		fw.shutdown()
		return nil
	}
}

func (w *formulaWorker) deliver(id int, rep workerReply) {
	w.mu.Lock()
	ch := w.pending[id]
	delete(w.pending, id)
	w.mu.Unlock()
	if ch != nil {
		ch <- rep
	}
}

func (w *formulaWorker) failAll(msg string) {
	w.mu.Lock()
	w.closed = true
	pending := w.pending
	w.pending = nil
	w.mu.Unlock()
	for _, ch := range pending {
		ch <- workerReply{fatal: true, err: msg}
	}
}

func (w *formulaWorker) shutdown() {
	w.failAll("formula worker closed")
	w.worker.Call("terminate")
	w.onMsg.Release()
	w.onErr.Release()
}

func (w *formulaWorker) call(ctx context.Context, msg map[string]any) (workerReply, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return workerReply{}, err
		}
	}
	id := int(w.nextID.Add(1))
	ch := make(chan workerReply, 1)
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return workerReply{fatal: true, err: "formula worker closed"}, nil
	}
	w.pending[id] = ch
	w.mu.Unlock()

	msg["id"] = id
	w.worker.Call("postMessage", msg)

	if ctx == nil || ctx.Done() == nil {
		return <-ch, nil
	}
	select {
	case rep := <-ch:
		return rep, nil
	case <-ctx.Done():
		w.mu.Lock()
		delete(w.pending, id)
		w.mu.Unlock()
		return workerReply{}, ctx.Err()
	}
}

func (w *formulaWorker) drop(rendererID uint64) {
	w.mu.Lock()
	closed := w.closed
	w.mu.Unlock()
	if closed {
		return
	}
	w.worker.Call("postMessage", map[string]any{
		"kind":       "drop",
		"rendererID": float64(rendererID),
	})
}

func bytesToJS(data []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(data))
	if len(data) > 0 {
		js.CopyBytesToJS(arr, data)
	}
	return arr
}

func isDefaultMathFont(b []byte) bool {
	return len(b) == len(newCMMath) && len(b) > 0 && &b[0] == &newCMMath[0]
}

type workerEngine struct {
	worker     *formulaWorker
	rendererID uint64
}

func (e *workerEngine) LayoutContext(ctx context.Context, expr *untie.Expr, style untie.Style) (*untie.Box, error) {
	return e.LayoutContextWithOptions(ctx, expr, style, untie.LayoutOptions{})
}

func (e *workerEngine) LayoutContextWithOptions(ctx context.Context, expr *untie.Expr, style untie.Style, opts untie.LayoutOptions) (*untie.Box, error) {
	if expr == nil {
		return nil, untie.ErrInvalidExpr
	}
	payload := encodeLayoutRequest(layoutRequest{
		source:  expr.Source(),
		dialect: expr.Dialect(),
		mode:    expr.Mode(),
		style:   style,
		opts:    opts,
	})
	rep, err := e.worker.call(ctx, map[string]any{
		"kind":       "layout",
		"rendererID": float64(e.rendererID),
		"payload":    bytesToJS(payload),
	})
	if err != nil {
		return nil, err
	}
	if !rep.ok {
		return nil, errors.New(rep.err)
	}
	return decodeBox(rep.data)
}

func (e *workerEngine) layoutSystemText(_ *untie.Formula, key layoutKey, missing []untie.MissingFallback, language string, measured []textMeasure) (*untie.Box, error) {
	payload := encodeSystemTextRequest(systemTextRequest{
		key:      key,
		language: language,
		missing:  missing,
		measured: measured,
	})
	rep, err := e.worker.call(context.Background(), map[string]any{
		"kind":       "systemText",
		"rendererID": float64(e.rendererID),
		"payload":    bytesToJS(payload),
	})
	if err != nil {
		return nil, err
	}
	if !rep.ok {
		return nil, errors.New(rep.err)
	}
	return decodeBox(rep.data)
}

func loadMathEngine(fonts Fonts) (mathEngine, error) {
	if len(fonts.Math) == 0 {
		return nil, errors.New("formula: no math font")
	}
	w := getFormulaWorker()
	if w == nil {
		return loadLocalEngine(fonts)
	}
	rendererID := nextRendererID.Add(1)
	msg := map[string]any{
		"kind":       "load",
		"rendererID": float64(rendererID),
		"useDefault": isDefaultMathFont(fonts.Math),
	}
	if !isDefaultMathFont(fonts.Math) {
		msg["math"] = bytesToJS(fonts.Math)
	}
	if len(fonts.Text) > 0 {
		msg["text"] = bytesToJS(fonts.Text)
	}
	if len(fonts.Fallbacks) > 0 {
		fbs := make([]any, len(fonts.Fallbacks))
		for i, f := range fonts.Fallbacks {
			fbs[i] = bytesToJS(f)
		}
		msg["fallbacks"] = fbs
	}
	rep, err := w.call(context.Background(), msg)
	if err != nil {
		return nil, err
	}
	if rep.fatal {
		return loadLocalEngine(fonts)
	}
	if !rep.ok {
		return nil, errors.New(rep.err)
	}
	eng := &workerEngine{worker: w, rendererID: rendererID}
	runtime.AddCleanup(eng, func(id uint64) { w.drop(id) }, rendererID)
	return eng, nil
}
