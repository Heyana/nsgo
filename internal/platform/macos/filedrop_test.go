//go:build darwin && !ios

package macos

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
)

type testDragState struct {
	pasteboard objc.ID
	location   apple.CGPoint
	mask       uint
	window     objc.ID
}

var (
	dragInfoClassOnce sync.Once
	classTestDragInfo objc.Class
	activeDragInfo    testDragState
)

func testDragInfoClass() objc.Class {
	dragInfoClassOnce.Do(func() {
		classTestDragInfo = register("NsgoTestDraggingInfo", apple.Class("NSObject"), nil, []objc.MethodDef{
			{Cmd: sel("draggingPasteboard"), Fn: func(objc.ID, objc.SEL) objc.ID { return activeDragInfo.pasteboard }},
			{Cmd: sel("draggingLocation"), Fn: func(objc.ID, objc.SEL) apple.CGPoint { return activeDragInfo.location }},
			{Cmd: sel("draggingSourceOperationMask"), Fn: func(objc.ID, objc.SEL) uint { return activeDragInfo.mask }},
			{Cmd: sel("draggingDestinationWindow"), Fn: func(objc.ID, objc.SEL) objc.ID { return activeDragInfo.window }},
		})
	})
	return classTestDragInfo
}

// finderPasteboard fills a private NSPasteboard the way Finder does: one
// NSPasteboardItem per dragged file whose public.file-url value is a file
// reference URL (file:///.file/id=...), plus an unresolvable reference URL
// that must be skipped.
func finderPasteboard(paths []string) objc.ID {
	pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
	pb.Send(sel("clearContents"))
	items := objc.ID(apple.Class("NSMutableArray")).Send(selAlloc).Send(selInit)
	fileURLType := apple.NSString(pasteboardFileURL)
	for _, p := range paths {
		u := objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(p))
		ref := u.Send(sel("fileReferenceURL"))
		if ref == 0 {
			ref = u
		}
		item := objc.ID(apple.Class("NSPasteboardItem")).Send(selAlloc).Send(selInit)
		item.Send(sel("setString:forType:"), ref.Send(sel("absoluteString")), fileURLType)
		items.Send(sel("addObject:"), item)
		item.Send(selRelease)
	}
	bad := objc.ID(apple.Class("NSPasteboardItem")).Send(selAlloc).Send(selInit)
	bad.Send(sel("setString:forType:"), apple.NSString("file:///.file/id=0.0"), fileURLType)
	items.Send(sel("addObject:"), bad)
	bad.Send(selRelease)
	pb.Send(sel("writeObjects:"), items)
	items.Send(selRelease)
	return pb
}

// legacyFilenamesPasteboard fills a private NSPasteboard with only the legacy
// NSFilenamesPboardType property list.
func legacyFilenamesPasteboard(paths []string) objc.ID {
	pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
	pb.Send(sel("clearContents"))
	legacyType := apple.NSString("NSFilenamesPboardType")
	types := objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), legacyType)
	pb.Send(sel("declareTypes:owner:"), types, objc.ID(0))
	arr := objc.ID(apple.Class("NSMutableArray")).Send(selAlloc).Send(selInit)
	for _, p := range paths {
		arr.Send(sel("addObject:"), apple.NSString(p))
	}
	pb.Send(sel("setPropertyList:forType:"), arr, legacyType)
	arr.Send(selRelease)
	return pb
}

// nonFilePasteboard fills a private NSPasteboard with plain text and a web URL.
func nonFilePasteboard() objc.ID {
	pb := objc.ID(apple.Class("NSPasteboard")).Send(sel("pasteboardWithUniqueName"))
	pb.Send(sel("clearContents"))
	item := objc.ID(apple.Class("NSPasteboardItem")).Send(selAlloc).Send(selInit)
	item.Send(sel("setString:forType:"), apple.NSString("hello"), apple.NSString("public.utf8-plain-text"))
	item.Send(sel("setString:forType:"), apple.NSString("https://example.com/a.txt"), apple.NSString("public.url"))
	items := objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), item)
	pb.Send(sel("writeObjects:"), items)
	item.Send(selRelease)
	return pb
}

// TestFileDropFromFinderReachesView drives a real NsgoWindow and NsgoSurface
// through AppKit's NSDraggingDestination selectors using Finder-style file
// reference URLs on a real NSPasteboard, legacy NSFilenamesPboardType, non-file
// drags, flipped coordinates, nested disabled zones, mid-drag disabling and
// modal bounds, and opens every dropped file with core.FileFromURL (#1601).
func TestFileDropFromFinderReachesView(t *testing.T) {
	if os.Getenv("NSGO_TEST_FILEDROP_PROCESS") != t.Name() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+t.Name()+"$", "-test.v")
		cmd.Env = append(os.Environ(), "NSGO_TEST_FILEDROP_PROCESS="+t.Name(), "HOME="+t.TempDir())
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return
	}

	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	files := []struct {
		name    string
		content string
		isDir   bool
	}{
		{name: "a b.txt", content: "hello space"},
		{name: "日本語.txt", content: "こんにちは"},
		{name: "100%#?.txt", content: "special"},
		{name: "folder", isDir: true},
	}
	var paths, wantURIs []string
	for _, f := range files {
		p := filepath.Join(dir, f.name)
		if f.isDir {
			if err := os.Mkdir(p, 0o755); err != nil {
				t.Fatal(err)
			}
		} else {
			if err := os.WriteFile(p, []byte(f.content), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		paths = append(paths, p)
		wantURIs = append(wantURIs, (&url.URL{Scheme: "file", Path: p}).String())
	}

	const title = "nsgo-file-drop"
	var (
		mu           sync.Mutex
		events       []string
		lastDrop     []string
		disableOuter *core.Signal[bool]
		showModal    *core.Signal[bool]
		appScope     *core.Scope
		scopeReady   = make(chan struct{})
	)
	record := func(s string) {
		mu.Lock()
		events = append(events, s)
		mu.Unlock()
	}
	snapshot := func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), events...)
	}
	clearEvents := func() {
		mu.Lock()
		events = nil
		lastDrop = nil
		mu.Unlock()
	}
	waitEvents := func(want []string) error {
		deadline := time.Now().Add(5 * time.Second)
		for {
			got := snapshot()
			if slices.Equal(got, want) {
				return nil
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("events = %v, want %v", got, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}

	done := make(chan error, 1)
	go func() {
		run := func() error {
			<-scopeReady
			findSurface := func(needModal bool) *surface {
				var found *surface
				barrier := make(chan struct{})
				apple.OnMain(func() {
					defer close(barrier)
					for s := range surfaces {
						if s.window == 0 || apple.GoString(s.window.Send(sel("title"))) != title {
							continue
						}
						var hasOuter, hasBottom, hasModal bool
						for _, v := range s.views {
							if v == nil || !v.framed {
								continue
							}
							switch v.props.TestID {
							case "outer":
								hasOuter = true
							case "bottom":
								hasBottom = true
							case "modalZone":
								hasModal = true
							}
						}
						if hasOuter && hasBottom && (!needModal || (hasModal && len(s.modals) > 0)) {
							found = s
							return
						}
					}
				})
				<-barrier
				return found
			}
			waitSurface := func(needModal bool) (*surface, error) {
				deadline := time.Now().Add(5 * time.Second)
				for {
					if s := findSurface(needModal); s != nil {
						return s, nil
					}
					if time.Now().After(deadline) {
						return nil, fmt.Errorf("surface never laid out (needModal=%v)", needModal)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}

			s, err := waitSurface(false)
			if err != nil {
				return err
			}

			var info objc.ID
			var checkErr error
			onMainSync := func(fn func()) {
				ch := make(chan struct{})
				apple.OnMain(func() {
					defer close(ch)
					fn()
				})
				<-ch
			}

			// Verify registration and NSDraggingDestination selectors on NsgoSurface.
			onMainSync(func() {
				types := s.native.Send(sel("registeredDraggedTypes"))
				n := objc.Send[uint](types, sel("count"))
				var registered []string
				for i := range n {
					registered = append(registered, apple.GoString(types.Send(sel("objectAtIndex:"), i)))
				}
				if !slices.Contains(registered, pasteboardFileURL) {
					checkErr = fmt.Errorf("registeredDraggedTypes = %v, want %q", registered, pasteboardFileURL)
					return
				}
				for _, name := range []string{
					"draggingEntered:", "draggingUpdated:", "draggingExited:",
					"draggingEnded:", "prepareForDragOperation:", "performDragOperation:",
				} {
					if !objc.Send[bool](s.native, sel("respondsToSelector:"), sel(name)) {
						checkErr = fmt.Errorf("surface does not respond to %s", name)
						return
					}
				}
				info = objc.ID(testDragInfoClass()).Send(selAlloc).Send(selInit)
			})
			if checkErr != nil {
				return checkErr
			}
			defer onMainSync(func() {
				if info != 0 {
					info.Send(selRelease)
				}
			})

			// Surface coordinates (top-left) -> window base coordinates (bottom-left).
			winPoint := func(x, y float64) apple.CGPoint {
				var p apple.CGPoint
				onMainSync(func() {
					p = objc.Send[apple.CGPoint](s.native, sel("convertPoint:toView:"), apple.CGPoint{X: x, Y: y}, objc.ID(0))
				})
				return p
			}
			topP, botP := winPoint(25, 25), winPoint(40, 180)
			if !(topP.Y > botP.Y) {
				return fmt.Errorf("expected flipped window coordinates: top=%v bottom=%v", topP, botP)
			}

			const dragEvery = ^uint(0)
			const dragMoveOnly = 16

			sendDrag := func(pb objc.ID, mask uint, x, y float64, op string) (uint, bool) {
				var u uint
				var b bool
				onMainSync(func() {
					loc := objc.Send[apple.CGPoint](s.native, sel("convertPoint:toView:"), apple.CGPoint{X: x, Y: y}, objc.ID(0))
					activeDragInfo = testDragState{pasteboard: pb, location: loc, mask: mask, window: s.window}
					switch op {
					case "enter":
						u = objc.Send[uint](s.native, sel("draggingEntered:"), info)
					case "update":
						u = objc.Send[uint](s.native, sel("draggingUpdated:"), info)
					case "exit":
						s.native.Send(sel("draggingExited:"), info)
					case "end":
						s.native.Send(sel("draggingEnded:"), info)
					case "prepare":
						b = objc.Send[bool](s.native, sel("prepareForDragOperation:"), info)
					case "perform":
						b = objc.Send[bool](s.native, sel("performDragOperation:"), info)
					}
				})
				return u, b
			}

			// 1. Non-file drag (plain text + web URL) is refused.
			var textPB objc.ID
			onMainSync(func() { textPB = nonFilePasteboard() })
			defer onMainSync(func() { textPB.Send(sel("releaseGlobally")) })
			if got, _ := sendDrag(textPB, dragEvery, 25, 25, "enter"); got != 0 {
				return fmt.Errorf("text drag enter = %d, want 0", got)
			}
			if _, ok := sendDrag(textPB, dragEvery, 25, 25, "prepare"); ok {
				return errors.New("text drag prepared")
			}
			if _, ok := sendDrag(textPB, dragEvery, 25, 25, "perform"); ok {
				return errors.New("text drag performed")
			}
			sendDrag(textPB, dragEvery, 25, 25, "exit")
			if got := snapshot(); len(got) != 0 {
				return fmt.Errorf("text drag emitted events %v", got)
			}

			// 2. Finder pasteboard with fileReferenceURLs (plus an unresolvable reference).
			var finderPB objc.ID
			onMainSync(func() { finderPB = finderPasteboard(paths) })
			defer onMainSync(func() { finderPB.Send(sel("releaseGlobally")) })

			// 2a. Source forbidding Copy is refused.
			if got, _ := sendDrag(finderPB, dragMoveOnly, 25, 25, "enter"); got != 0 {
				return fmt.Errorf("move-only drag enter = %d, want 0", got)
			}
			if _, ok := sendDrag(finderPB, dragMoveOnly, 25, 25, "prepare"); ok {
				return errors.New("move-only drag prepared")
			}
			sendDrag(finderPB, dragMoveOnly, 25, 25, "exit")

			// 2b. Drag over child -> outer; stay in outer; move to nested disabled zone -> leave outer;
			// move to standalone disabled and empty areas; move to bottom zone; exit.
			if got, _ := sendDrag(finderPB, dragEvery, 25, 25, "enter"); got != dragCopy {
				return fmt.Errorf("enter over inner = %d, want Copy", got)
			}
			if got, _ := sendDrag(finderPB, dragEvery, 70, 80, "update"); got != dragCopy {
				return fmt.Errorf("update over outer = %d, want Copy", got)
			}
			if got, _ := sendDrag(finderPB, dragEvery, 110, 30, "update"); got != 0 {
				return fmt.Errorf("update over nested disabled = %d, want 0", got)
			}
			if got, _ := sendDrag(finderPB, dragEvery, 240, 40, "update"); got != 0 {
				return fmt.Errorf("update over disabled = %d, want 0", got)
			}
			if got, _ := sendDrag(finderPB, dragEvery, 240, 180, "update"); got != 0 {
				return fmt.Errorf("update over empty = %d, want 0", got)
			}
			if got, _ := sendDrag(finderPB, dragEvery, 40, 180, "update"); got != dragCopy {
				return fmt.Errorf("update over bottom = %d, want Copy", got)
			}
			sendDrag(finderPB, dragEvery, 40, 180, "exit")
			if err := waitEvents([]string{"enter outer", "leave outer", "enter bottom", "leave bottom"}); err != nil {
				return fmt.Errorf("hover sequence: %w", err)
			}

			// 2c. Full drop sequence over child inside outer, then draggingEnded:.
			clearEvents()
			if got, _ := sendDrag(finderPB, dragEvery, 25, 25, "enter"); got != dragCopy {
				return fmt.Errorf("re-enter outer = %d, want Copy", got)
			}
			if _, ok := sendDrag(finderPB, dragEvery, 25, 25, "prepare"); !ok {
				return errors.New("prepareForDragOperation over outer returned false")
			}
			if _, ok := sendDrag(finderPB, dragEvery, 25, 25, "perform"); !ok {
				return errors.New("performDragOperation over outer returned false")
			}
			sendDrag(finderPB, dragEvery, 25, 25, "end")
			wantDrop := "drop outer " + strings.Join(wantURIs, " ")
			if err := waitEvents([]string{"enter outer", wantDrop, "leave outer"}); err != nil {
				return fmt.Errorf("finder drop: %w", err)
			}

			// Verify every dropped file URL opens cleanly through core.FileFromURL.
			mu.Lock()
			dropped := append([]string(nil), lastDrop...)
			mu.Unlock()
			if !slices.Equal(dropped, wantURIs) {
				return fmt.Errorf("dropped URIs = %v, want %v", dropped, wantURIs)
			}
			for i, f := range files {
				cf, err := core.FileFromURL(dropped[i])
				if f.isDir {
					if err == nil {
						return fmt.Errorf("FileFromURL(%q) opened a folder", dropped[i])
					}
					continue
				}
				if err != nil {
					return fmt.Errorf("FileFromURL(%q): %w", dropped[i], err)
				}
				rc, err := cf.Open()
				if err != nil {
					return fmt.Errorf("Open(%q): %w", dropped[i], err)
				}
				data, err := io.ReadAll(rc)
				rc.Close()
				if err != nil {
					return fmt.Errorf("Read(%q): %w", dropped[i], err)
				}
				if string(data) != f.content {
					return fmt.Errorf("content of %q = %q, want %q", f.name, string(data), f.content)
				}
			}

			// 3. Legacy NSFilenamesPboardType drag over bottom zone.
			clearEvents()
			var legacyPB objc.ID
			onMainSync(func() { legacyPB = legacyFilenamesPasteboard(paths[:2]) })
			defer onMainSync(func() { legacyPB.Send(sel("releaseGlobally")) })
			if got, _ := sendDrag(legacyPB, dragEvery, 40, 180, "enter"); got != dragCopy {
				return fmt.Errorf("legacy enter over bottom = %d, want Copy", got)
			}
			if _, ok := sendDrag(legacyPB, dragEvery, 40, 180, "prepare"); !ok {
				return errors.New("legacy prepare over bottom returned false")
			}
			if _, ok := sendDrag(legacyPB, dragEvery, 40, 180, "perform"); !ok {
				return errors.New("legacy perform over bottom returned false")
			}
			sendDrag(legacyPB, dragEvery, 40, 180, "end")
			wantLegacy := "drop bottom " + strings.Join(wantURIs[:2], " ")
			if err := waitEvents([]string{"enter bottom", wantLegacy, "leave bottom"}); err != nil {
				return fmt.Errorf("legacy drop: %w", err)
			}

			// 4. Disabling the active drag target mid-drag reports leave and refuses updates.
			clearEvents()
			if got, _ := sendDrag(finderPB, dragEvery, 25, 25, "enter"); got != dragCopy {
				return fmt.Errorf("enter before disable = %d, want Copy", got)
			}
			if err := waitEvents([]string{"enter outer"}); err != nil {
				return err
			}
			appScope.Post(func() { disableOuter.Set(true) })
			if err := waitEvents([]string{"enter outer", "leave outer"}); err != nil {
				return fmt.Errorf("disable mid-drag: %w", err)
			}
			if got, _ := sendDrag(finderPB, dragEvery, 25, 25, "update"); got != 0 {
				return fmt.Errorf("update after disable = %d, want 0", got)
			}
			sendDrag(finderPB, dragEvery, 25, 25, "exit")
			appScope.Post(func() { disableOuter.Set(false) })

			// 5. A modal bounds file drops: background zones refuse, modal zone accepts.
			clearEvents()
			appScope.Post(func() { showModal.Set(true) })
			if _, err := waitSurface(true); err != nil {
				return err
			}
			if got, _ := sendDrag(finderPB, dragEvery, 25, 25, "enter"); got != 0 {
				return fmt.Errorf("enter over background behind modal = %d, want 0", got)
			}
			if _, ok := sendDrag(finderPB, dragEvery, 25, 25, "perform"); ok {
				return errors.New("drop behind modal succeeded")
			}
			if got, _ := sendDrag(finderPB, dragEvery, 240, 160, "enter"); got != dragCopy {
				return fmt.Errorf("enter over modalZone = %d, want Copy", got)
			}
			if _, ok := sendDrag(finderPB, dragEvery, 240, 160, "prepare"); !ok {
				return errors.New("prepare over modalZone returned false")
			}
			if _, ok := sendDrag(finderPB, dragEvery, 240, 160, "perform"); !ok {
				return errors.New("perform over modalZone returned false")
			}
			sendDrag(finderPB, dragEvery, 240, 160, "end")
			wantModal := "drop modalZone " + strings.Join(wantURIs, " ")
			if err := waitEvents([]string{"enter modalZone", wantModal, "leave modalZone"}); err != nil {
				return fmt.Errorf("modal drop: %w", err)
			}

			return nil
		}
		done <- run()
		apple.OnMain(finish)
	}()

	onMain(func() {
		Run(core.AppRoot(func(cx *core.Scope) {
			cx.SetMainView(func(cx *core.Scope) core.Node {
				desktop.SetTitle(cx, title)
				disableOuter = core.NewSignal(cx, false)
				showModal = core.NewSignal(cx, false)
				appScope = cx
				close(scopeReady)

				abs := func(x, y, w, h driver.Points, bg driver.Color) core.Style {
					return core.Style{
						Position: core.Absolute,
						Left:     driver.Pt(x), Top: driver.Pt(y),
						Width: driver.Pt(w), Height: driver.Pt(h),
						Background: bg,
					}
				}
				zone := func(name string, style core.Style, disabled func() bool, children ...core.Node) core.Node {
					return core.View{
						TestID: name, Style: style, Disabled: disabled, Children: children,
						OnFileDragEnter: func() { record("enter " + name) },
						OnFileDragLeave: func() { record("leave " + name) },
						OnFileDrop: func(uris []string) {
							mu.Lock()
							lastDrop = append([]string(nil), uris...)
							mu.Unlock()
							record("drop " + name + " " + strings.Join(uris, " "))
						},
					}
				}
				return core.View{
					Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), Background: driver.RGB(0xffffff)},
					Children: []core.Node{
						zone("outer", abs(0, 0, 160, 120, driver.RGB(0xeeeeee)), disableOuter.Get,
							core.View{TestID: "inner", Style: abs(10, 10, 40, 40, driver.RGB(0xcccccc))},
							zone("nestedOff", abs(90, 10, 50, 50, driver.RGB(0xbbbbbb)), func() bool { return true }),
						),
						zone("bottom", abs(0, 140, 160, 80, driver.RGB(0xddffee)), nil),
						zone("off", abs(200, 0, 120, 100, driver.RGB(0xffdddd)), func() bool { return true }),
						core.Show{
							When: showModal.Get,
							Then: func(cx *core.Scope) core.Node {
								return core.Modal{
									Children: []core.Node{
										core.View{
											Style: core.Style{Width: driver.Pt(360), Height: driver.Pt(240), PointerEvents: core.PointerEventsBoxNone},
											Children: []core.Node{
												zone("modalZone", abs(180, 120, 140, 80, driver.RGB(0xddeeff)), nil),
											},
										},
									},
								}
							},
						},
					},
				}
			})
		}), Options{Title: title, Width: 360, Height: 240})
	})

	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
