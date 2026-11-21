// Package automation is the protocol between the nsgo tool and an app `nsgo dev -automation` built: the tool sends a Request and the app's agent (package nsgotest) answers it. Tool and app come from one module version, so the protocol is internal and unversioned; MCP (`nsgo mcp`) and nsgotest are the public interfaces (docs/architecture/tool.md, Automation).
package automation

import (
	"encoding/json"
	"fmt"
)

// Request asks the app, or the tool for Status, Logs and Reload, to do Method with Params, one of the params types below.
type Request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// Answer answers the Request of its ID with Result, one of the result types below, or Error.
type Answer struct {
	ID     uint64          `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Reply is the Answer to request id: result as JSON, or err.
func Reply(id uint64, result any, err *Error) Answer {
	if err != nil {
		return Answer{ID: id, Error: err}
	}
	data, merr := json.Marshal(result)
	if merr != nil {
		return Answer{ID: id, Error: Errorf(Failed, "%v", merr)}
	}
	return Answer{ID: id, Result: data}
}

// Error is why a request failed.
type Error struct {
	Code    Code   `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return string(e.Code) + ": " + e.Message }

// Errorf returns an Error of code with a formatted message.
func Errorf(code Code, format string, args ...any) *Error {
	return &Error{Code: code, Message: fmt.Sprintf(format, args...)}
}

// Code says what kind of failure an Error is.
type Code string

const (
	// InvalidRequest is a request that cannot be read: bad JSON, a bad selector, a button the alert lacks.
	InvalidRequest Code = "invalid_request"
	// UnknownMethod names no method.
	UnknownMethod Code = "unknown_method"
	// NoApp is a request no app connected to answer, or whose app went away.
	NoApp Code = "no_app"
	// NoWindow names no window the app shows.
	NoWindow Code = "no_window"
	// NoMatch is a selector or a menu path that matches nothing shown.
	NoMatch Code = "no_match"
	// Unsupported is what the platform cannot do, such as screenshots headless.
	Unsupported Code = "unsupported"
	// Timeout is a wait that ran out of time.
	Timeout Code = "timeout"
	// Failed is a step that could not be taken, such as a press on a disabled view.
	Failed Code = "failed"
)

// The app's methods, which its agent answers.
const (
	Windows    = "windows"
	Snapshot   = "snapshot"
	Find       = "find"
	Press      = "press"
	Type       = "type"
	Key        = "key"
	Scroll     = "scroll"
	Drag       = "drag"
	Hover      = "hover"
	Menu       = "menu"
	Alert      = "alert"
	Wait       = "wait"
	Screenshot = "screenshot"
)

// The tool's methods, which nsgo dev answers itself.
const (
	Status = "status"
	Logs   = "logs"
	Reload = "reload"
)

// Params are each method's params type, as a new value.
var Params = map[string]func() any{
	Windows: func() any { return &WindowsParams{} }, Snapshot: func() any { return &SnapshotParams{} },
	Find: func() any { return &FindParams{} }, Press: func() any { return &PressParams{} },
	Type: func() any { return &TypeParams{} }, Key: func() any { return &KeyParams{} },
	Scroll: func() any { return &ScrollParams{} }, Drag: func() any { return &DragParams{} },
	Hover: func() any { return &HoverParams{} }, Menu: func() any { return &MenuParams{} },
	Alert: func() any { return &AlertParams{} }, Wait: func() any { return &WaitParams{} },
	Screenshot: func() any { return &ScreenshotParams{} },
	Status:     func() any { return &StatusParams{} }, Logs: func() any { return &LogsParams{} },
	Reload: func() any { return &ReloadParams{} },
}

// Point is a point in a view's own points, from its top left.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// WindowsParams lists the app's windows.
type WindowsParams struct{}

// SnapshotParams outline what a window shows.
type SnapshotParams struct {
	Window uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	All    bool   `json:"all,omitempty" jsonschema:"list every view shown, containers too, not only the named ones"`
	Bounds bool   `json:"bounds,omitempty" jsonschema:"add each element's frame in the window's points"`
}

// FindParams list the elements a selector matches.
type FindParams struct {
	Window   uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector string `json:"selector" jsonschema:"an nsgotest selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, view:<ref> (a ref snapshot printed), a >> b looks inside a, and >> nth=<i> keeps one match"`
	Mounted  bool   `json:"mounted,omitempty" jsonschema:"match every view mounted, shown or not"`
}

// PressParams press what a selector names.
type PressParams struct {
	Window   uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector string `json:"selector" jsonschema:"an nsgotest selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, view:<ref> (a ref snapshot printed), a >> b looks inside a, and >> nth=<i> keeps one match"`
	At       *Point `json:"at,omitempty" jsonschema:"press this point of the match, in its own points, instead of its middle"`
	Mode     string `json:"mode,omitempty" jsonschema:"tap (the default), long or right"`
	Snapshot bool   `json:"snapshot,omitempty" jsonschema:"answer with the window's snapshot after the step"`
}

// TypeParams type into the field a selector names.
type TypeParams struct {
	Window   uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector string `json:"selector" jsonschema:"an nsgotest selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, view:<ref> (a ref snapshot printed), a >> b looks inside a, and >> nth=<i> keeps one match"`
	Text     string `json:"text,omitempty" jsonschema:"what to type at the end of the field's text"`
	Clear    bool   `json:"clear,omitempty" jsonschema:"empty the field first"`
	Submit   bool   `json:"submit,omitempty" jsonschema:"press Return after typing"`
	Snapshot bool   `json:"snapshot,omitempty" jsonschema:"answer with the window's snapshot after the step"`
}

// KeyParams press a key.
type KeyParams struct {
	Window   uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector string `json:"selector,omitempty" jsonschema:"a field to press the key in, which takes the focus first; the window's focus by default"`
	Key      string `json:"key" jsonschema:"the key as keymaps name it: Enter, Escape, Backspace, Tab, a, mod+a, shift+tab (mod is Command on macOS)"`
	Snapshot bool   `json:"snapshot,omitempty" jsonschema:"answer with the window's snapshot after the step"`
}

// ScrollParams scroll the scroll view a selector names, or the one around it.
type ScrollParams struct {
	Window   uint32   `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector string   `json:"selector" jsonschema:"an nsgotest selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, view:<ref> (a ref snapshot printed), a >> b looks inside a, and >> nth=<i> keeps one match"`
	By       float64  `json:"by,omitempty" jsonschema:"points to scroll along the view's axis; positive shows what is further down or right"`
	To       *float64 `json:"to,omitempty" jsonschema:"the offset to scroll to along the view's axis, instead of by"`
	Snapshot bool     `json:"snapshot,omitempty" jsonschema:"answer with the window's snapshot after the step"`
}

// DragParams drag from what a selector names.
type DragParams struct {
	Window   uint32  `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector string  `json:"selector" jsonschema:"an nsgotest selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, view:<ref> (a ref snapshot printed), a >> b looks inside a, and >> nth=<i> keeps one match"`
	At       *Point  `json:"at,omitempty" jsonschema:"start at this point of the match, in its own points, instead of its middle"`
	Path     []Point `json:"path" jsonschema:"the points to move through, as offsets from where the drag starts"`
	Snapshot bool    `json:"snapshot,omitempty" jsonschema:"answer with the window's snapshot after the step"`
}

// HoverParams move the mouse over what a selector names.
type HoverParams struct {
	Window   uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector string `json:"selector" jsonschema:"an nsgotest selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, view:<ref> (a ref snapshot printed), a >> b looks inside a, and >> nth=<i> keeps one match"`
	Snapshot bool   `json:"snapshot,omitempty" jsonschema:"answer with the window's snapshot after the step"`
}

// MenuParams choose a menu item.
type MenuParams struct {
	Window   uint32   `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Path     []string `json:"path" jsonschema:"the item's labels from the menu bar down, such as File, New"`
	Popup    bool     `json:"popup,omitempty" jsonschema:"choose in the context menu showing instead of the application menu"`
	Dock     bool     `json:"dock,omitempty" jsonschema:"choose in the app icon's menu (desktop.SetDockMenu), the Dock menu on macOS and the Jump List's tasks on Windows, instead of the application menu: the app's, whichever window set it last"`
	Snapshot bool     `json:"snapshot,omitempty" jsonschema:"answer with the window's snapshot after the step"`
}

// AlertParams list the alerts showing, or answer one.
type AlertParams struct {
	Window *uint32 `json:"window,omitempty" jsonschema:"only the alerts of this window as listed, 0 for the first app's own; every alert by default"`
	ID     uint32  `json:"id,omitempty" jsonschema:"the alert to answer, as listed, with its window when another window's alert has the same ID; the oldest by default"`
	Answer *int    `json:"answer,omitempty" jsonschema:"press the button at this index, or -1 to dismiss; without it the alerts are listed"`
}

// WaitParams wait until a selector matches, or matches nothing.
type WaitParams struct {
	Window    uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
	Selector  string `json:"selector" jsonschema:"an nsgotest selector: id:<test ID>, label:<label>, text:<text>, role:<role> <name>, view:<ref> (a ref snapshot printed), a >> b looks inside a, and >> nth=<i> keeps one match"`
	Gone      bool   `json:"gone,omitempty" jsonschema:"wait until nothing matches instead"`
	TimeoutMS int    `json:"timeout_ms,omitempty" jsonschema:"how long to wait in milliseconds; 5000 by default, 60000 at most"`
}

// ScreenshotParams capture a window.
type ScreenshotParams struct {
	Window uint32 `json:"window,omitempty" jsonschema:"a window ID as windows lists it; the newest window by default"`
}

// StatusParams ask how the app's build and run stand.
type StatusParams struct{}

// LogsParams read the app's latest output.
type LogsParams struct {
	Lines int `json:"lines,omitempty" jsonschema:"how many of the latest lines; 200 by default, 2000 at most"`
}

// ReloadParams build and start the app again.
type ReloadParams struct{}

// Window is a window or embedded view the process shows.
type Window struct {
	ID uint32 `json:"id"`
	// Graph numbers the engine's reactive graph within the process, from 1, telling apart surfaces of several engines in one host.
	Graph  int     `json:"graph,omitempty"`
	Title  string  `json:"title"`
	Kind   string  `json:"kind"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
	// Default is the window requests without a window act on.
	Default bool `json:"default,omitempty"`
}

// WindowsResult lists the app's windows, oldest first.
type WindowsResult struct {
	Windows []Window `json:"windows"`
}

// Element is a view a selector matched.
type Element struct {
	// Ref is a selector naming the view alone, view:<node>, until the app starts again.
	Ref    string   `json:"ref"`
	Role   string   `json:"role"`
	Name   string   `json:"name,omitempty"`
	Text   string   `json:"text,omitempty"`
	Value  string   `json:"value,omitempty"`
	TestID string   `json:"test_id,omitempty"`
	States []string `json:"states,omitempty"`
	// Bounds are its frame in the window's points: x, y, width, height.
	Bounds *[4]float64 `json:"bounds,omitempty"`
}

// FindResult lists what a selector matched, in the order a user finds it.
type FindResult struct {
	Elements []Element `json:"elements"`
}

// SnapshotResult outlines what a window shows, an element a line: `- button "Add" [view:12] id=add focused`.
type SnapshotResult struct {
	Window uint32 `json:"window"`
	Text   string `json:"text"`
}

// StepResult is what a step left: Settled is false when the app kept changing past the step's wait of 5 seconds.
type StepResult struct {
	Settled  bool   `json:"settled"`
	Snapshot string `json:"snapshot,omitempty"`
}

// ScreenshotResult is a window as a PNG, in pixels.
type ScreenshotResult struct {
	PNG    []byte `json:"png"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// AlertInfo is an alert showing, which a window or an app's own surface (window 0 for the first graph) shows.
type AlertInfo struct {
	ID      uint32   `json:"id"`
	Window  uint32   `json:"window"`
	Title   string   `json:"title,omitempty"`
	Message string   `json:"message,omitempty"`
	Buttons []string `json:"buttons"`
}

// AlertResult lists the alerts showing, oldest first, after any answer.
type AlertResult struct {
	Alerts []AlertInfo `json:"alerts"`
}

// StatusResult is how the app's build and run stand.
type StatusResult struct {
	Target     string `json:"target"`
	Building   bool   `json:"building"`
	BuildError string `json:"build_error,omitempty"`
	// App is whether a running app is connected; Run counts the apps that connected, and view refs of an earlier run are stale.
	App bool `json:"app"`
	Run int  `json:"run"`
}

// LogsResult is the app's latest output, oldest line first.
type LogsResult struct {
	Lines []string `json:"lines"`
}
