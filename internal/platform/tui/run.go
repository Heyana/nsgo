//go:build (darwin && !ios) || (linux && !android) || windows

package tui

import (
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gdamore/tcell/v3"
	"github.com/gdamore/tcell/v3/color"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/engine"
	"github.com/nsgo-dev/nsgo/internal/platform/knownfolder"
	"github.com/nsgo-dev/nsgo/internal/platform/queue"
)

// Options configure a terminal application. The terminal determines its
// size; the ordinary application width and height do not resize a console.
type Options struct {
	ID, Title  string
	Appearance driver.Appearance
}

type surface struct {
	textEngine
	engine        *engine.Engine
	tree          *tree
	screen        tcell.Screen
	batches       queue.Batches
	wake          chan struct{}
	done          chan struct{}
	once          sync.Once
	mu            sync.Mutex
	tasks         []func()
	frame         atomic.Bool
	frameReady    atomic.Bool
	testSize      bool
	width, height int
	previous      *grid
	buttons       tcell.ButtonMask
	pasting       bool
	paste         strings.Builder
	held          map[string]driver.NodeID
}

func terminalEnvironment() driver.Environment {
	osname := driver.OS(runtime.GOOS)
	if runtime.GOOS == "darwin" {
		osname = driver.OSMacOS
	}
	locale := os.Getenv("LC_ALL")
	if locale == "" {
		locale = os.Getenv("LC_MESSAGES")
	}
	if locale == "" {
		locale = os.Getenv("LANG")
	}
	locale, _, _ = strings.Cut(locale, ".")
	locale = strings.ReplaceAll(locale, "_", "-")
	if locale == "" || locale == "C" || locale == "POSIX" {
		locale = "en"
	}
	return driver.Environment{OS: osname, Idiom: driver.IdiomDesktop, Pointer: driver.PointerFine, Simulated: true, Dark: true, FontScale: 1, PixelRatio: 1, Locale: locale, Lifecycle: driver.LifecycleActive}
}

func openScreen() (tcell.Screen, error) {
	s, err := tcell.NewScreen()
	if err != nil {
		return nil, fmt.Errorf("terminal: %w", err)
	}
	if err = s.Init(); err != nil {
		s.Fini()
		return nil, fmt.Errorf("terminal: %w (run in an interactive terminal)", err)
	}
	s.EnableMouse()
	s.EnablePaste()
	s.EnableFocus()
	return s, nil
}

func newSurface(screen tcell.Screen) *surface {
	w := &surface{screen: screen, wake: make(chan struct{}, 1), done: make(chan struct{})}
	w.tree = newTree(func(ev driver.Event) { w.engine.Dispatch(ev) })
	w.tree.quit = w.stop
	if screen != nil {
		w.width, w.height = screen.Size()
		w.tree.title = screen.SetTitle
	} else {
		w.width, w.height = 80, 24
	}
	return w
}

func (w *surface) start(app engine.App, o engine.Options) {
	if o.Size.Width <= 0 || o.Size.Height <= 0 {
		o.Size = driver.Size{Width: driver.Points(w.width) * cellWidth, Height: driver.Points(w.height) * cellHeight}
	}
	w.width, w.height = max(int(o.Size.Width/cellWidth), 1), max(int(o.Size.Height/cellHeight), 1)
	w.engine = engine.Start(w, app, o)
	w.engine.SetFrameWindow(true, true)
}

// Run owns the alternate screen and raw mode until the app quits, receives
// an interrupt, or its terminal disappears. Deferred restoration also runs
// while a panic unwinds the application loop.
func Run(app engine.App, o Options) error {
	screen, err := openScreen()
	if err != nil {
		return err
	}
	defer screen.Fini()
	w := newSurface(screen)
	screen.SetTitle(safeText(o.Title))
	data, cache, logs := engine.AppDirs(o.ID)
	w.start(app, engine.Options{Environment: terminalEnvironment(), Appearance: o.Appearance, DataDir: data, CacheDir: cache, LogsDir: logs, UserDir: engine.SystemUserDir(knownfolder.Path)})
	defer w.tree.close()
	defer w.engine.Close()
	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(interrupts)
	return w.loop(interrupts)
}

func (w *surface) stop() { w.once.Do(func() { close(w.done) }) }
func (w *surface) notify() {
	select {
	case w.wake <- struct{}{}:
	default:
	}
}
func (w *surface) Mount(b *driver.MountBatch) {
	if w.batches.Push(b) {
		w.notify()
	}
}
func (w *surface) RequestFrame() {
	if w.frame.CompareAndSwap(false, true) {
		time.AfterFunc(time.Second/60, func() { w.frameReady.Store(true); w.notify() })
	}
}
func (*surface) SubmitGPU(driver.GPUFrame) {}
func (w *surface) post(fn func()) {
	w.mu.Lock()
	w.tasks = append(w.tasks, fn)
	w.mu.Unlock()
	w.notify()
}
func (w *surface) drain() {
	for _, b := range w.batches.Drain() {
		w.tree.apply(b)
	}
	w.mu.Lock()
	tasks := w.tasks
	w.tasks = nil
	w.mu.Unlock()
	for _, f := range tasks {
		f()
	}
}

func (w *surface) loop(signals <-chan os.Signal) error {
	defer w.stop()
	var events chan tcell.Event
	if w.screen != nil {
		events = w.screen.EventQ()
	}
	for {
		select {
		case <-w.done:
			return nil
		case <-signals:
			return nil
		case <-w.wake:
			w.drain()
			if w.frameReady.Swap(false) {
				w.frame.Store(false)
				w.engine.Frame()
			}
		case event, ok := <-events:
			if !ok {
				return nil
			}
			if err := w.event(event); err != nil {
				return err
			}
		}
		w.paint()
	}
}

func termColor(c driver.Color) color.Color {
	if c&255 == 0 {
		return color.Default
	}
	return color.NewRGBColor(int32(c>>24), int32(c>>16&255), int32(c>>8&255))
}

func (w *surface) paint() {
	g := w.tree.render(w.width, w.height)
	if w.screen != nil {
		for y := range g.height {
			for x := range g.width {
				c := g.at(x, y)
				if c.width == 0 {
					continue
				}
				if old := w.previous; old != nil && old.width == g.width && old.height == g.height && *old.at(x, y) == *c {
					continue
				}
				style := tcell.StyleDefault.Foreground(termColor(c.fg)).Background(termColor(c.bg)).Bold(c.bold).Italic(c.italic).StrikeThrough(c.strike).Underline(c.underline)
				w.screen.Put(x, y, c.text, style)
			}
		}
		w.screen.ShowCursor(g.cursorX, g.cursorY)
		w.screen.Show()
	}
	w.previous = g
}

func modifiers(m tcell.ModMask) driver.Modifiers {
	return driver.Modifiers{Control: m&tcell.ModCtrl != 0, Alt: m&tcell.ModAlt != 0, Shift: m&tcell.ModShift != 0, Platform: m&tcell.ModMeta != 0}
}

func keyOf(ev *tcell.EventKey) (string, string) {
	if ev.Key() == tcell.KeyRune {
		if ev.Str() == " " {
			return "space", " "
		}
		return strings.ToLower(ev.Str()), ev.Str()
	}
	switch ev.Key() {
	case tcell.KeyEnter:
		return "enter", ""
	case tcell.KeyTab, tcell.KeyBacktab:
		return "tab", ""
	case tcell.KeyEscape:
		return "escape", ""
	case tcell.KeyBackspace, tcell.KeyBackspace2:
		return "backspace", ""
	case tcell.KeyPgUp:
		return "pageup", ""
	case tcell.KeyPgDn:
		return "pagedown", ""
	}
	if ev.Key() >= tcell.KeyCtrlA && ev.Key() <= tcell.KeyCtrlZ {
		return string(rune('a' + ev.Key() - tcell.KeyCtrlA)), ""
	}
	return strings.ToLower(tcell.KeyNames[ev.Key()]), ""
}

func (w *surface) event(ev tcell.Event) error {
	switch ev := ev.(type) {
	case *tcell.EventResize:
		if w.testSize {
			w.previous = nil
			w.screen.Sync()
			return nil
		}
		w.width, w.height = ev.Size()
		w.previous = nil
		w.screen.Sync()
		w.engine.Resize(driver.Size{Width: driver.Points(w.width) * cellWidth, Height: driver.Points(w.height) * cellHeight}, driver.Geometry{})
	case *tcell.EventError:
		return fmt.Errorf("terminal: %w", ev)
	case *tcell.EventFocus:
		w.engine.SetFrameWindow(true, ev.Focused)
		if !ev.Focused {
			w.tree.pressed = nil
			w.buttons = 0
			for name, id := range w.held {
				w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: id, Text: name})
			}
			clear(w.held)
		} else {
			w.previous = nil
			w.screen.EnableMouse()
			w.screen.EnablePaste()
			w.screen.EnableFocus()
		}
	case *tcell.EventPaste:
		w.pasting = ev.Start()
		if ev.End() {
			if v := w.tree.focus; v != nil && v.kind == driver.KindInput {
				w.tree.pasteText(v, w.paste.String())
			}
			w.paste.Reset()
		}
	case *tcell.EventKey:
		if ev.Pressed() {
			w.engine.InputModality(driver.ModalityKeyboard)
		}
		name, text := keyOf(ev)
		mods := modifiers(ev.Modifiers())
		if ev.Key() >= tcell.KeyCtrlA && ev.Key() <= tcell.KeyCtrlZ && name != "tab" && name != "enter" && name != "backspace" {
			mods.Control = true
		}
		if ev.Key() == tcell.KeyBacktab {
			mods.Shift = true
		}
		if w.pasting {
			if ev.Pressed() {
				if name == "enter" {
					text = "\n"
				}
				if name == "tab" {
					text = "\t"
				}
				w.paste.WriteString(text)
			}
			return nil
		}
		if !ev.Pressed() {
			w.reportKey(name, mods, false, false)
			return nil
		}
		consumed := w.reportKey(name, mods, true, ev.Repeat() > 1)
		if !consumed && !w.tree.key(name, text, mods, ev.Repeat() > 1) && name == "c" && mods.Control {
			w.stop()
		}
		// Older protocols have no releases: bracket each observed press,
		// rather than leaving KeysDown stuck until the next focus change.
		if w.screen == nil || w.screen.KeyboardProtocol() == tcell.LegacyKeyboard || w.screen.KeyboardProtocol() == tcell.XTermKeyboard {
			w.reportKey(name, mods, false, false)
		}
	case *tcell.EventMouse:
		w.engine.InputModality(driver.ModalityPointer)
		x, y := ev.Position()
		p := driver.Point{X: (driver.Points(x) + 0.5) * cellWidth, Y: (driver.Points(y) + 0.5) * cellHeight}
		mods := modifiers(ev.Modifiers())
		buttons := ev.Buttons()
		if buttons&tcell.Button2 != 0 && w.buttons&tcell.Button2 == 0 {
			w.tree.contextMenu(p, mods)
		}
		if buttons&tcell.WheelUp != 0 {
			w.tree.wheel(p, driver.Point{Y: -3 * cellHeight})
		}
		if buttons&tcell.WheelDown != 0 {
			w.tree.wheel(p, driver.Point{Y: 3 * cellHeight})
		}
		if buttons&tcell.WheelLeft != 0 {
			w.tree.wheel(p, driver.Point{X: -6 * cellWidth})
		}
		if buttons&tcell.WheelRight != 0 {
			w.tree.wheel(p, driver.Point{X: 6 * cellWidth})
		}
		if buttons&tcell.Button1 != 0 && w.buttons&tcell.Button1 == 0 {
			w.tree.mouseDown(p, mods)
		}
		if buttons&tcell.Button1 == 0 && w.buttons&tcell.Button1 != 0 {
			w.tree.mouseUp(p, mods)
		}
		w.tree.hover(p, mods)
		w.buttons = buttons
	}
	return nil
}

func (w *surface) reportKey(name string, mods driver.Modifiers, down, repeat bool) bool {
	if !down {
		if id, ok := w.held[name]; ok {
			delete(w.held, name)
			w.engine.Dispatch(driver.Event{Kind: driver.EventKeyUp, ID: id, Text: name, Modifiers: mods})
		}
		return false
	}
	v := w.tree.focus
	if v != nil && v.kind == driver.KindInput {
		return false
	}
	var id driver.NodeID
	if v != nil && v.events.Has(driver.EventKeyDown) {
		id = v.id
	} else if !w.tree.heldKeys {
		return false
	}
	if w.held == nil {
		w.held = map[string]driver.NodeID{}
	}
	if old, exists := w.held[name]; exists {
		id, repeat = old, true
	} else {
		w.held[name] = id
	}
	kind := driver.EventKeyUp
	if down {
		kind = driver.EventKeyDown
	}
	ev := driver.Event{Kind: kind, ID: id, Text: name, Modifiers: mods, Repeat: repeat}
	if down && id != 0 {
		return w.engine.DispatchKeyDown(ev)
	}
	w.engine.Dispatch(ev)
	return false
}
