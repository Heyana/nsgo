package nsgotest

import (
	"image"
	"time"

	"github.com/nsgo-dev/nsgo/core"
)

// Surface steps use the same App behavior after selecting this window's model.

// Press delegates to App.Press in this surface.
func (s *Surface) Press(selector string) {
	s.app.t.Helper()
	s.open().Press(selector)
}

// PressWith delegates to App.PressWith in this surface.
func (s *Surface) PressWith(selector string, mods core.Modifiers) {
	s.app.t.Helper()
	s.open().PressWith(selector, mods)
}

// PressAt delegates to App.PressAt in this surface.
func (s *Surface) PressAt(selector string, at core.Point) {
	s.app.t.Helper()
	s.open().PressAt(selector, at)
}

// PressAtWith delegates to App.PressAtWith in this surface.
func (s *Surface) PressAtWith(selector string, at core.Point, mods core.Modifiers) {
	s.app.t.Helper()
	s.open().PressAtWith(selector, at, mods)
}

// PressDown delegates to App.PressDown in this surface.
func (s *Surface) PressDown(selector string) {
	s.app.t.Helper()
	s.open().PressDown(selector)
}

// PressUp delegates to App.PressUp in this surface.
func (s *Surface) PressUp(selector string) {
	s.app.t.Helper()
	s.open().PressUp(selector)
}

// LongPress delegates to App.LongPress in this surface.
func (s *Surface) LongPress(selector string) {
	s.app.t.Helper()
	s.open().LongPress(selector)
}

// Type delegates to App.Type in this surface.
func (s *Surface) Type(selector, text string) {
	s.app.t.Helper()
	s.open().Type(selector, text)
}

// Clear delegates to App.Clear in this surface.
func (s *Surface) Clear(selector string) {
	s.app.t.Helper()
	s.open().Clear(selector)
}

// Submit delegates to App.Submit in this surface.
func (s *Surface) Submit(selector string) {
	s.app.t.Helper()
	s.open().Submit(selector)
}

// KeyPress delegates to App.KeyPress in this surface.
func (s *Surface) KeyPress(selector, key string) {
	s.app.t.Helper()
	s.open().KeyPress(selector, key)
}

// Paste delegates to App.Paste in this surface.
func (s *Surface) Paste(selector string, p core.PasteEvent) {
	s.app.t.Helper()
	s.open().Paste(selector, p)
}

// Select delegates to App.Select in this surface.
func (s *Surface) Select(selector string, start, end int) {
	s.app.t.Helper()
	s.open().Select(selector, start, end)
}

// SelectTab delegates to App.SelectTab in this surface.
func (s *Surface) SelectTab(selector string, index int) {
	s.app.t.Helper()
	s.open().SelectTab(selector, index)
}

// Compose delegates to App.Compose in this surface.
func (s *Surface) Compose(selector, text string) {
	s.app.t.Helper()
	s.open().Compose(selector, text)
}

// Commit delegates to App.Commit in this surface.
func (s *Surface) Commit(selector, text string) {
	s.app.t.Helper()
	s.open().Commit(selector, text)
}

// Key delegates to App.Key in this surface.
func (s *Surface) Key(key string) {
	s.app.t.Helper()
	s.open().Key(key)
}

// KeyDown delegates to App.KeyDown in this surface.
func (s *Surface) KeyDown(key string) {
	s.app.t.Helper()
	s.open().KeyDown(key)
}

// KeyUp delegates to App.KeyUp in this surface.
func (s *Surface) KeyUp(key string) {
	s.app.t.Helper()
	s.open().KeyUp(key)
}

// KeyCommands delegates to App.KeyCommands in this surface.
func (s *Surface) KeyCommands() []KeyCommand {
	s.app.t.Helper()
	return s.open().KeyCommands()
}

// Focus delegates to App.Focus in this surface.
func (s *Surface) Focus(selector string) {
	s.app.t.Helper()
	s.open().Focus(selector)
}

// RightClick delegates to App.RightClick in this surface.
func (s *Surface) RightClick(selector string) {
	s.app.t.Helper()
	s.open().RightClick(selector)
}

// Drag delegates to App.Drag in this surface.
func (s *Surface) Drag(selector string, path ...core.Point) {
	s.app.t.Helper()
	s.open().Drag(selector, path...)
}

// DragAt delegates to App.DragAt in this surface.
func (s *Surface) DragAt(selector string, at core.Point, path ...core.Point) {
	s.app.t.Helper()
	s.open().DragAt(selector, at, path...)
}

// Touch delegates to App.Touch in this surface.
func (s *Surface) Touch(selector string, at core.Point) *Finger {
	s.app.t.Helper()
	return s.open().Touch(selector, at)
}

// Hover delegates to App.Hover in this surface.
func (s *Surface) Hover(selector string) {
	s.app.t.Helper()
	s.open().Hover(selector)
}

// HoverAt delegates to App.HoverAt in this surface.
func (s *Surface) HoverAt(selector string, at core.Point) {
	s.app.t.Helper()
	s.open().HoverAt(selector, at)
}

// HoverOut delegates to App.HoverOut in this surface.
func (s *Surface) HoverOut() {
	s.app.t.Helper()
	s.open().HoverOut()
}

// MouseDown delegates to App.MouseDown in this surface.
func (s *Surface) MouseDown(selector string, button core.MouseButton) {
	s.app.t.Helper()
	s.open().MouseDown(selector, button)
}

// MouseDownWith delegates to App.MouseDownWith in this surface.
func (s *Surface) MouseDownWith(selector string, button core.MouseButton, mods core.Modifiers) {
	s.app.t.Helper()
	s.open().MouseDownWith(selector, button, mods)
}

// MouseDownAt delegates to App.MouseDownAt in this surface.
func (s *Surface) MouseDownAt(selector string, at core.Point, button core.MouseButton) {
	s.app.t.Helper()
	s.open().MouseDownAt(selector, at, button)
}

// MouseDownAtWith delegates to App.MouseDownAtWith in this surface.
func (s *Surface) MouseDownAtWith(selector string, at core.Point, button core.MouseButton, mods core.Modifiers) {
	s.app.t.Helper()
	s.open().MouseDownAtWith(selector, at, button, mods)
}

// MouseUp delegates to App.MouseUp in this surface.
func (s *Surface) MouseUp(selector string, button core.MouseButton) {
	s.app.t.Helper()
	s.open().MouseUp(selector, button)
}

// MouseUpWith delegates to App.MouseUpWith in this surface.
func (s *Surface) MouseUpWith(selector string, button core.MouseButton, mods core.Modifiers) {
	s.app.t.Helper()
	s.open().MouseUpWith(selector, button, mods)
}

// MouseUpAt delegates to App.MouseUpAt in this surface.
func (s *Surface) MouseUpAt(selector string, at core.Point, button core.MouseButton) {
	s.app.t.Helper()
	s.open().MouseUpAt(selector, at, button)
}

// MouseUpAtWith delegates to App.MouseUpAtWith in this surface.
func (s *Surface) MouseUpAtWith(selector string, at core.Point, button core.MouseButton, mods core.Modifiers) {
	s.app.t.Helper()
	s.open().MouseUpAtWith(selector, at, button, mods)
}

// BlurWindow delegates to App.BlurWindow in this surface.
func (s *Surface) BlurWindow() {
	s.app.t.Helper()
	s.open().BlurWindow()
}

// SelectText delegates to App.SelectText in this surface.
func (s *Surface) SelectText(selector string, box core.Bounds) {
	s.app.t.Helper()
	s.open().SelectText(selector, box)
}

// SelectTextRange delegates to App.SelectTextRange in this surface.
func (s *Surface) SelectTextRange(selector, from string, start int, to string, end int) {
	s.app.t.Helper()
	s.open().SelectTextRange(selector, from, start, to, end)
}

// ActiveCursor delegates to App.ActiveCursor in this surface.
func (s *Surface) ActiveCursor() ActiveCursor {
	s.app.t.Helper()
	return s.open().ActiveCursor()
}

// CursorAt delegates to App.CursorAt in this surface.
func (s *Surface) CursorAt(selector string) ActiveCursor {
	s.app.t.Helper()
	return s.open().CursorAt(selector)
}

// Scroll delegates to App.Scroll in this surface.
func (s *Surface) Scroll(selector string, distance core.Points) {
	s.app.t.Helper()
	s.open().Scroll(selector, distance)
}

// ScrollTo delegates to App.ScrollTo in this surface.
func (s *Surface) ScrollTo(selector string, offset core.Point) {
	s.app.t.Helper()
	s.open().ScrollTo(selector, offset)
}

// Pan delegates to App.Pan in this surface.
func (s *Surface) Pan(selector string, translations ...core.Point) {
	s.app.t.Helper()
	s.open().Pan(selector, translations...)
}

// Pinch delegates to App.Pinch in this surface.
func (s *Surface) Pinch(selector string, scales ...float32) {
	s.app.t.Helper()
	s.open().Pinch(selector, scales...)
}

// Wheel delegates to App.Wheel in this surface.
func (s *Surface) Wheel(selector string, delta core.Point, mods core.Modifiers) {
	s.app.t.Helper()
	s.open().Wheel(selector, delta, mods)
}

// TouchpadScroll delegates to App.TouchpadScroll in this surface.
func (s *Surface) TouchpadScroll(selector string, velocity core.Point, deltas ...core.Point) {
	s.app.t.Helper()
	s.open().TouchpadScroll(selector, velocity, deltas...)
}

// Rotate delegates to App.Rotate in this surface.
func (s *Surface) Rotate(selector string, angles ...float32) {
	s.app.t.Helper()
	s.open().Rotate(selector, angles...)
}

// DragScroll delegates to App.DragScroll in this surface.
func (s *Surface) DragScroll(selector string, offset core.Point, velocity float32) {
	s.app.t.Helper()
	s.open().DragScroll(selector, offset, velocity)
}

// PullToRefresh delegates to App.PullToRefresh in this surface.
func (s *Surface) PullToRefresh(selector string) {
	s.app.t.Helper()
	s.open().PullToRefresh(selector)
}

// PinchZoom delegates to App.PinchZoom in this surface.
func (s *Surface) PinchZoom(selector string, scale float32) {
	s.app.t.Helper()
	s.open().PinchZoom(selector, scale)
}

// Back delegates to App.Back in this surface.
func (s *Surface) Back() {
	s.app.t.Helper()
	s.open().Back()
}

// HeaderBack delegates to App.HeaderBack in this surface.
func (s *Surface) HeaderBack(selector string) {
	s.app.t.Helper()
	s.open().HeaderBack(selector)
}

// SwipeClose delegates to App.SwipeClose in this surface.
func (s *Surface) SwipeClose(selector string) {
	s.app.t.Helper()
	s.open().SwipeClose(selector)
}

// PresentModal delegates to App.PresentModal in this surface.
func (s *Surface) PresentModal(selector string, size core.Size, insets core.Edges) {
	s.app.t.Helper()
	s.open().PresentModal(selector, size, insets)
}

// DropFiles delegates to App.DropFiles in this surface.
func (s *Surface) DropFiles(selector string, uris ...string) {
	s.app.t.Helper()
	s.open().DropFiles(selector, uris...)
}

// AccessibilityAction delegates to App.AccessibilityAction in this surface.
func (s *Surface) AccessibilityAction(selector, name string) {
	s.app.t.Helper()
	s.open().AccessibilityAction(selector, name)
}

// Report delegates to App.Report in this surface.
func (s *Surface) Report(ev Event) {
	s.app.t.Helper()
	s.open().Report(ev)
}

// Load delegates to App.Load in this surface.
func (s *Surface) Load(selector string, pixels core.Size) {
	s.app.t.Helper()
	s.open().Load(selector, pixels)
}

// Play delegates to App.Play in this surface.
func (s *Surface) Play(selector string, info core.VideoInfo) {
	s.app.t.Helper()
	s.open().Play(selector, info)
}

// Progress delegates to App.Progress in this surface.
func (s *Surface) Progress(selector string, position, duration time.Duration) {
	s.app.t.Helper()
	s.open().Progress(selector, position, duration)
}

// PauseVideo delegates to App.PauseVideo in this surface.
func (s *Surface) PauseVideo(selector string, reason core.VideoPauseReason) {
	s.app.t.Helper()
	s.open().PauseVideo(selector, reason)
}

// PlayVideo delegates to App.PlayVideo in this surface.
func (s *Surface) PlayVideo(selector string) {
	s.app.t.Helper()
	s.open().PlayVideo(selector)
}

// Navigate delegates to App.Navigate in this surface.
func (s *Surface) Navigate(selector string, nav core.WebNavigation) {
	s.app.t.Helper()
	s.open().Navigate(selector, nav)
}

// PostMessage delegates to App.PostMessage in this surface.
func (s *Surface) PostMessage(selector, data string) {
	s.app.t.Helper()
	s.open().PostMessage(selector, data)
}

// PostMessageFrom delegates to App.PostMessageFrom in this surface.
func (s *Surface) PostMessageFrom(selector, url string, mainFrame bool, data string) {
	s.app.t.Helper()
	s.open().PostMessageFrom(selector, url, mainFrame, data)
}

// Emit delegates to App.Emit in this surface.
func (s *Surface) Emit(selector, name string, data any) {
	s.app.t.Helper()
	s.open().Emit(selector, name, data)
}

// AnswerScript delegates to App.AnswerScript in this surface.
func (s *Surface) AnswerScript(selector, result, errText string) {
	s.app.t.Helper()
	s.open().AnswerScript(selector, result, errText)
}

// ReportPictureInPicture delegates to App.ReportPictureInPicture in this surface.
func (s *Surface) ReportPictureInPicture(selector string, state core.PictureInPictureState) {
	s.app.t.Helper()
	s.open().ReportPictureInPicture(selector, state)
}

// PictureInPictureControl delegates to App.PictureInPictureControl in this surface.
func (s *Surface) PictureInPictureControl(selector, action string, interval time.Duration) {
	s.app.t.Helper()
	s.open().PictureInPictureControl(selector, action, interval)
}

// Find delegates to App.Find in this surface.
func (s *Surface) Find(selector string) *View {
	s.app.t.Helper()
	return s.open().Find(selector)
}

// Query delegates to App.Query in this surface.
func (s *Surface) Query(selector string) []*View {
	s.app.t.Helper()
	return s.open().Query(selector)
}

// FindMounted delegates to App.FindMounted in this surface.
func (s *Surface) FindMounted(selector string) *View {
	s.app.t.Helper()
	return s.open().FindMounted(selector)
}

// QueryMounted delegates to App.QueryMounted in this surface.
func (s *Surface) QueryMounted(selector string) []*View {
	s.app.t.Helper()
	return s.open().QueryMounted(selector)
}

// Exists delegates to App.Exists in this surface.
func (s *Surface) Exists(selector string) bool {
	s.app.t.Helper()
	return s.open().Exists(selector)
}

// Enabled delegates to App.Enabled in this surface.
func (s *Surface) Enabled(selector string) bool {
	s.app.t.Helper()
	return s.open().Enabled(selector)
}

// Texts delegates to App.Texts in this surface.
func (s *Surface) Texts() []string {
	s.app.t.Helper()
	return s.open().Texts()
}

// SpanStyle delegates to App.SpanStyle in this surface.
func (s *Surface) SpanStyle(selector string) TextStyle {
	s.app.t.Helper()
	return s.open().SpanStyle(selector)
}

// Span delegates to App.Span in this surface.
func (s *Surface) Span(selector string) TextSpan {
	s.app.t.Helper()
	return s.open().Span(selector)
}

// Dump delegates to App.Dump in this surface.
func (s *Surface) Dump() string {
	s.app.t.Helper()
	return s.open().Dump()
}

// Bound delegates to App.Bound in this surface.
func (s *Surface) Bound(selector string) Bindings {
	s.app.t.Helper()
	return s.open().Bound(selector)
}

// AnimatedValue delegates to App.AnimatedValue in this surface.
func (s *Surface) AnimatedValue(node uint32) float64 {
	s.app.t.Helper()
	return s.open().AnimatedValue(node)
}

// Animated delegates to App.Animated in this surface.
func (s *Surface) Animated(selector string) ViewStyle {
	s.app.t.Helper()
	return s.open().Animated(selector)
}

// Resize delegates to App.Resize in this surface.
func (s *Surface) Resize(width, height core.Points) {
	s.app.t.Helper()
	s.open().Resize(width, height)
}

// Settle delegates to App.Settle in this surface.
func (s *Surface) Settle() {
	s.app.t.Helper()
	s.open().Settle()
}

// WaitFor delegates to App.WaitFor in this surface.
func (s *Surface) WaitFor(selector string, timeout time.Duration) *View {
	s.app.t.Helper()
	return s.open().WaitFor(selector, timeout)
}

// WaitForGone delegates to App.WaitForGone in this surface.
func (s *Surface) WaitForGone(selector string, timeout time.Duration) {
	s.app.t.Helper()
	s.open().WaitForGone(selector, timeout)
}

// WaitUntil delegates to App.WaitUntil in this surface.
func (s *Surface) WaitUntil(timeout time.Duration, cond func() bool) {
	s.app.t.Helper()
	s.open().WaitUntil(timeout, cond)
}

// CapturePNG delegates to App.CapturePNG in this surface.
func (s *Surface) CapturePNG() []byte {
	s.app.t.Helper()
	return s.open().CapturePNG()
}

// Capture delegates to App.Capture in this surface.
func (s *Surface) Capture() image.Image {
	s.app.t.Helper()
	return s.open().Capture()
}

// Screenshot delegates to App.Screenshot in this surface.
func (s *Surface) Screenshot(name string) {
	s.app.t.Helper()
	s.open().Screenshot(name)
}

// Post queues fn with App.Post's scheduling. The surface is checked on the
// test goroutine before fn is handed to the app's graph. Like App.Post, it is
// safe to call from any goroutine; fn takes no test steps.
func (s *Surface) Post(fn func()) {
	s.app.postTest(func() {
		if t := s.app.target; t != nil && !t.live {
			deadline := time.Now().Add(settleWait)
			// Finish a previous step's barrier before starting one ordered
			// after the graph callbacks already dispatched by this queue.
			if t.syncing != nil && !t.sync(deadline) || !t.sync(deadline) {
				s.app.t.Fatalf("nsgotest: surface %q: timed out waiting to post", s.title())
			}
			// Earlier callbacks may close this window or its parent. Apply
			// those closures before validating the handle on the test goroutine.
			t.drain()
		}
		s.checkOpen()
		s.app.postGraph(fn)
	})
}
