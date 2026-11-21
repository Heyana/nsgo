// Package testhost is where a backend built for `nsgo test -target` (the
// nsgotest build tag) offers its test host, and where nsgotest finds it:
// the platform's loop on the main thread, an app shown on a real surface,
// and native input at the surface's points.
package testhost

import (
	"errors"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Host is a backend's test host.
type Host interface {
	// Main runs the platform's loop on the calling goroutine, the main thread, and run on another; it ends the process with run's code once run has returned, never before, as run holds a TestMain's teardown (nsgotest.MainFunc).
	Main(run func() int)
	// Open shows app on a new surface as the backend's Run does, with
	// o's size and what it observes.
	Open(app func(cx *core.Scope) core.Node, o Options) (Surface, error)
	// Several reports whether a process opens several surfaces: a
	// desktop's windows; a phone and a page show one.
	Several() bool
}

// WindowHost is a Host whose apps can open their own windows, as Run
// does with core.RootOptions.Windows. app says whether StartApp supplied
// an app function; macOS uses its own windowless surface in either case.
// The main window opens at the platform's default size when Options.Size is zero.
type WindowHost interface {
	Host
	OpensWindows(app bool) bool
}

// Options are what a test opens an app with.
type Options struct {
	// CheckCommits enables full-layout diagnostics for this app only.
	CheckCommits bool
	Size         driver.Size
	// Windows runs the app with the windowless surface that opens its windows.
	Windows bool
	// Environment pins what the test sets over what the platform reports
	// (Pin's fields), from the first frame.
	Environment *driver.Environment
	// Storage seeds the app's storage; in memory when nil.
	Storage *core.Store
	// Secrets is the app's secure store, in place of the machine's; one in memory when nil.
	Secrets driver.SecretStore
	// Observe sees every batch the engine sends, on the engine's goroutine, before the platform applies it; Event every event the platform reports to the engine, on the goroutine it reports from. Each comes with the number of its surface: 0 for the first, the one Open opens, else the window or panel the app opened (driver.SurfaceConfig.ID). Neither may block.
	Observe func(surface uint32, b *driver.MountBatch)
	Event   func(surface uint32, ev driver.Event)
	// Host is the channel of the test's embedding host, nil without one.
	Host *core.Host
	// Clock is the test's, which the app's timers run on in place of the
	// system's: they fire as the test moves it.
	Clock core.Clock
	// FPS measures the app with the backend's performance monitor and
	// logs its readings once a second (nsgo test -fps), where the backend
	// has one: macOS, iOS, Windows and Linux; the web and Android measure
	// only the engine, with no frame readings.
	FPS bool
	// PickImages, when set, answers the photo picks the app asks for in place of the system's picker: done takes the files picked, none when the user cancels, and may be called on any goroutine. macOS, Windows and Linux use it; the other targets show their picker.
	PickImages func(driver.PickRequest, func([]driver.PickedFile))
	// TakePhoto, when set, answers the camera requests the app asks for in place of the system's camera: done takes the photo, an error, or nil for a cancellation, and may be called on any goroutine. Tests use it on every target.
	TakePhoto func(driver.PickRequest, func(*driver.PickedFile, error))
	// ScanCode, when set, answers the codes the app asks to scan in place of the platform's camera, on every target: done takes the code read, nil when the user closes the scanner, or why it failed, and may be called on any goroutine.
	ScanCode func(driver.ScanRequest, func(*driver.ScannedCode, error))
}

// Surface is an app shown on a platform. Its methods act as the user
// does, with the platform's own input, at points of the surface (its top
// left is 0, 0), and return once the input is posted.
type Surface interface {
	// Press presses at at for hold (a tap when 0), as a finger or the mouse's primary button does, with mods held for the press only.
	Press(at driver.Point, hold time.Duration, mods driver.Modifiers) error
	// RightClick clicks the secondary button at at.
	RightClick(at driver.Point) error
	// Drag puts a finger or the primary button down at path[0], rests,
	// moves through the rest of path and lifts.
	Drag(path []driver.Point) error
	// Scroll scrolls the content under at by delta, as a wheel, a
	// trackpad or a drag that rests before it lifts does.
	Scroll(at driver.Point, delta driver.Point) error
	// Hover moves the mouse to at; a point outside the surface leaves it.
	Hover(at driver.Point) error
	// Type types text into the focused field, a key at a time.
	Type(text string) error
	// Compose has an input method compose text in the focused field
	// through the platform's text input client, as one marks what it
	// composes: "" ends the composition with nothing. Commit has it
	// commit text there, ending the composition. Both answer
	// ErrNoInputMethod where none can compose.
	Compose(text string) error
	Commit(text string) error
	// Key presses a key with modifiers, as the keymap names them:
	// "Enter", "Backspace", "Escape", "mod+a".
	Key(key string) error
	// Back goes back as the platform's back button, gesture or Escape.
	Back() error
	// Answer presses the button at index of the alert id shows, or
	// dismisses it with -1.
	Answer(id uint32, index int) error
	// ChooseMenu chooses the application menu's item id, as its menu bar
	// does; ErrNoMenu where the platform shows none.
	ChooseMenu(id uint32) error
	// ChooseDockMenu chooses the app icon menu's item id, as the Dock menu or the Jump List does; ErrNoMenu where the platform has none.
	ChooseDockMenu(id uint32) error
	// ChoosePopup chooses item id of the context menu showing, or closes it for 0, doing nothing when none shows; ErrNoMenu where the platform shows none.
	ChoosePopup(id uint32) error
	// Screenshot returns the surface as a PNG.
	Screenshot() ([]byte, error)
	// Animating reports whether the platform is still delivering input, applying
	// batches or running a finite animation, such as a transition or scroll glide.
	Animating() bool
	// Resize resizes the surface; SetEnvironment changes what it reports,
	// as a system setting does.
	Resize(size driver.Size) error
	SetEnvironment(env driver.Environment) error
	// Sync returns once the app's engine ran what was posted to it before and handed the resulting commits to the platform.
	Sync()
}

// WindowSurfaces resolves the native test surface an app opened. It is
// implemented by the surface returned from a desktop test host's Open.
type WindowSurfaces interface {
	WindowSurface(id uint32) Surface
}

// CloseWindow is the native close-button action on a desktop test surface.
type CloseWindow interface {
	RequestClose() error
}

// KeyHoldSurface is a surface that presses one key and holds it (KeyDown), or lets it come up (KeyUp), as Key's two halves: a modifier it holds stays held through the keys after it until its KeyUp.
type KeyHoldSurface interface {
	KeyDown(key string) error
	KeyUp(key string) error
}

// OpenLinksSurface delivers links to the app under test as the system would.
type OpenLinksSurface interface {
	OpenLinks(links []string)
}

// MouseButtonSurface is a surface that presses one mouse button at a point and holds it (MouseDown), or lets it come up (MouseUp), with optional modifiers.
type MouseButtonSurface interface {
	MouseDown(at driver.Point, button driver.MouseButton, mods driver.Modifiers) error
	MouseUp(at driver.Point, button driver.MouseButton, mods driver.Modifiers) error
}

// BlurWindowSurface is a surface whose window can lose focus in a test, releasing held keys and mouse buttons as the platform does on focus loss.
type BlurWindowSurface interface {
	BlurWindow() error
}

// KeyboardSurface is a surface whose window takes the keyboard only while its app is active, as macOS's does.
type KeyboardSurface interface {
	// PrepareKeyboard activates the app and waits for its window to take the keyboard when the run opted in to that (nsgo test -target macos -activate), and does nothing otherwise.
	PrepareKeyboard() error
	// FocusRefusal names the cause after a click left a field without the focus: an inactive app the run did not opt in to activating. It is nil otherwise.
	FocusRefusal() error
}

// QuitSurface sends a user or system quit request to the test's app without ending the test host.
type QuitSurface interface {
	RequestQuit(id uint32, forced bool)
}

var host Host

// ErrNoMenu is a platform's answer to a menu step where it shows no
// native menus: a phone's, or the web's.
var ErrNoMenu = errors.New("the platform shows no native menus")

// ErrNoInputMethod is a composition step's answer where the machine has no
// input method to compose with, as Windows without an IME.
var ErrNoInputMethod = errors.New("no input method composes on this machine")

// Register makes h the test host; a backend's init calls it.
func Register(h Host) { host = h }

// Current returns the test host, or nil when the backend linked has none.
func Current() Host { return host }

// PostToTest queues fn on the goroutine of the test running app, an *nsgotest.App, to run as its next step or wait settles, also on a target, where nsgotest.App.Post runs fn on the engine's goroutine: tsx/tsxtest runs its test VM's work there, as the VM is the test's. nsgotest sets it.
var PostToTest func(app any, fn func())

// Pin applies what a test set in pin over env, as the platform reports it: the appearance, reduced motion, increased contrast, the login launch reason, the application in front and whether the platform follows it, the camera and code scanner capabilities, and the locale, text size, device name and device model when set. The rest, such as the OS, the idiom and the safe area, stays the platform's.
func Pin(env driver.Environment, pin *driver.Environment) driver.Environment {
	if pin == nil {
		return env
	}
	env.Dark, env.ReduceMotion, env.IncreaseContrast = pin.Dark, pin.ReduceMotion, pin.IncreaseContrast
	env.Camera, env.ScanCode = pin.Camera, pin.ScanCode
	env.LaunchedAtLogin, env.FrontmostApp, env.CanReadFrontmostApp = pin.LaunchedAtLogin, pin.FrontmostApp, pin.CanReadFrontmostApp
	if pin.Locale != "" {
		env.Locale = pin.Locale
	}
	if pin.FontScale != 0 {
		env.FontScale = pin.FontScale
	}
	if pin.DeviceName != "" {
		env.DeviceName = pin.DeviceName
	}
	if pin.DeviceModel != "" {
		env.DeviceModel = pin.DeviceModel
	}
	return env
}
