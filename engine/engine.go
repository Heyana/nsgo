// Package engine runs an app on a goroutine of its own and hands the
// platform one MountBatch per commit.
package engine

import (
	"runtime"

	"github.com/oneo-me/nsgo/core"
)

// Platform is what a backend gives the engine.
type Platform interface {
	core.TextEngine
	// Mount receives each commit on the engine goroutine. The platform
	// applies it on its main thread, in one hop, without waiting.
	Mount(batch *core.MountBatch)
}

// App builds the root element once.
type App func(cx *core.Scope) *core.Element

// Engine is one running app: one tree, one reactive scope, one goroutine.
type Engine struct {
	tasks chan func(*core.Tree)
}

// Start runs app on a new goroutine at the surface size.
func Start(p Platform, app App, size core.Size) *Engine {
	e := &Engine{tasks: make(chan func(*core.Tree), 256)}
	go e.run(p, app, size)
	return e
}

func (e *Engine) run(p Platform, app App, size core.Size) {
	// Text engines such as Android's need the same OS thread each call.
	runtime.LockOSThread()
	cx := core.NewScope()
	tree := core.NewTree(cx, p)
	tree.Resize(size)
	tree.Mount(app(cx))
	for {
		if batch := tree.Commit(); batch != nil {
			p.Mount(batch)
		}
		task := <-e.tasks
		task(tree)
		// Run what else is queued before committing once for all of it.
		for drained := false; !drained; {
			select {
			case task := <-e.tasks:
				task(tree)
			default:
				drained = true
			}
		}
	}
}

func (e *Engine) post(task func(*core.Tree)) { e.tasks <- task }

// Resize sets the surface size.
func (e *Engine) Resize(size core.Size) { e.post(func(t *core.Tree) { t.Resize(size) }) }

// Press reports a press of node id.
func (e *Engine) Press(id core.NodeID) { e.post(func(t *core.Tree) { t.Press(id) }) }

// PressAt reports a press at p in surface points, for backends that hit
// test in the engine.
func (e *Engine) PressAt(p core.Point) {
	e.post(func(t *core.Tree) { t.Press(t.HitTest(p)) })
}

// Change reports an input's edited text.
func (e *Engine) Change(id core.NodeID, text string) {
	e.post(func(t *core.Tree) { t.Change(id, text) })
}

// Submit reports Return in an input.
func (e *Engine) Submit(id core.NodeID, text string) {
	e.post(func(t *core.Tree) { t.Submit(id, text) })
}
