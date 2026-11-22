//go:build ios && nsgotest

package ios

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The real Foundation coordinator reads this app's own file. Mutating or
// removing its source after import must not invalidate the delivered path.
func TestOpenedDocumentIsAnIndependentCoordinatedCopy(t *testing.T) {
	name := "a #日本 document.nsgo"
	source := filepath.Join(t.TempDir(), name)
	const body = "NSGo imported document\n"
	if err := os.WriteFile(source, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	var url objc.ID
	onMainSync(func() {
		url = objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(source)).Send(sel("retain"))
	})
	defer onMainSync(func() { url.Send(selRelease) })
	path, err := importOpenedFile(url)
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(filepath.Dir(path))
	if path == source || filepath.Base(path) != name || !filepath.IsAbs(path) {
		t.Fatalf("imported path = %q, source = %q", path, source)
	}
	if err := os.Remove(source); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != body {
		t.Fatalf("imported bytes = %q, %v", got, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("imported file permissions = %v, %v", info, err)
	}
}

func TestOpenedDocumentsRefuseDirectoriesAndMissingFiles(t *testing.T) {
	for _, source := range []string{t.TempDir(), filepath.Join(t.TempDir(), "missing.nsgo")} {
		var url objc.ID
		onMainSync(func() {
			url = objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(source)).Send(sel("retain"))
		})
		path, err := importOpenedFile(url)
		onMainSync(func() { url.Send(selRelease) })
		if err == nil || path != "" {
			t.Fatalf("invalid document %q returned %q, %v", source, path, err)
		}
	}
}

// Use ordinary test objects exposing URL/URLContexts, rather than trying
// to construct UIKit-owned UIOpenURLContexts. This exercises the registered
// scene delegate and connection-options paths without a system picker.
var fileTestURLs = map[objc.ID]objc.ID{}
var fileTestContexts = map[objc.ID]objc.ID{}
var fileTestContextClass, fileTestOptionsClass objc.Class

func fileContextsForTest(urls []objc.ID) (contexts objc.ID, release func()) {
	if fileTestContextClass == 0 {
		fileTestContextClass = register("NsgoFileOpenContextForTest", apple.Class("NSObject"), nil,
			[]objc.MethodDef{{Cmd: sel("URL"), Fn: func(self objc.ID, _ objc.SEL) objc.ID { return fileTestURLs[self] }}})
		fileTestOptionsClass = register("NsgoFileOpenOptionsForTest", apple.Class("NSObject"), nil,
			[]objc.MethodDef{{Cmd: sel("URLContexts"), Fn: func(self objc.ID, _ objc.SEL) objc.ID { return fileTestContexts[self] }}})
	}
	contexts = objc.ID(apple.Class("NSMutableSet")).Send(selNew)
	var objects []objc.ID
	for _, url := range urls {
		c := objc.ID(fileTestContextClass).Send(selNew)
		fileTestURLs[c] = url.Send(sel("retain"))
		objects = append(objects, c)
		contexts.Send(sel("addObject:"), c)
	}
	return contexts, func() {
		for _, c := range objects {
			fileTestURLs[c].Send(selRelease)
			delete(fileTestURLs, c)
			c.Send(selRelease)
		}
		contexts.Send(selRelease)
	}
}

func waitForFileImport(t *testing.T) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		fileOpens.Lock()
		running := fileOpens.running
		fileOpens.Unlock()
		if !running {
			// The worker posted delivery before reporting itself idle.
			onMainSync(func() {})
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the native document import did not complete")
		}
	}
}

func TestSceneOpenedDocumentsStaySeparateFromLinks(t *testing.T) {
	source := filepath.Join(t.TempDir(), "warm #日本.nsgo")
	if err := os.WriteFile(source, []byte("warm native document"), 0600); err != nil {
		t.Fatal(err)
	}
	files := make(chan []string, 1)
	links := make(chan []string, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.OnOpenFiles(func(paths []string) { files <- paths })
		cx.OnOpenLinks(func(urls []string) { links <- urls })
		return core.Text{Content: "Document delegate test"}
	})
	app.WaitFor("text:Document delegate test", 5*time.Second)
	onMainSync(func() {
		file := objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(source))
		link := objc.ID(apple.Class("NSURL")).Send(sel("URLWithString:"), apple.NSString("nsgo-test://documents"))
		contexts, release := fileContextsForTest([]objc.ID{file, link})
		defer release()
		scene := runner.window.Send(sel("windowScene"))
		scene.Send(sel("delegate")).Send(sel("scene:openURLContexts:"), scene, contexts)
	})
	select {
	case paths := <-files:
		if len(paths) != 1 || filepath.Base(paths[0]) != filepath.Base(source) || paths[0] == source {
			t.Fatalf("native file event = %v", paths)
		}
		defer os.RemoveAll(filepath.Dir(paths[0]))
		if err := os.Remove(source); err != nil {
			t.Fatal(err)
		}
		if data, err := os.ReadFile(paths[0]); err != nil || string(data) != "warm native document" {
			t.Fatalf("native imported contents = %q, %v", data, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the scene delegate did not deliver the file")
	}
	select {
	case urls := <-links:
		if len(urls) != 1 || urls[0] != "nsgo-test://documents" {
			t.Fatalf("native link event = %v", urls)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the scene delegate dropped the custom link")
	}
}

func TestSceneConnectionDocumentsWaitForTheFirstHandler(t *testing.T) {
	source := filepath.Join(t.TempDir(), "launch.nsgo")
	if err := os.WriteFile(source, []byte("launch native document"), 0600); err != nil {
		t.Fatal(err)
	}
	ready := make(chan *core.Scope, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ready <- cx
		return core.Text{Content: "Waiting for a document handler"}
	})
	app.WaitFor("text:Waiting for a document handler", 5*time.Second)
	cx := <-ready
	onMainSync(func() {
		url := objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(source))
		contexts, release := fileContextsForTest([]objc.ID{url})
		defer release()
		options := objc.ID(fileTestOptionsClass).Send(selNew)
		defer options.Send(selRelease)
		fileTestContexts[options] = contexts
		defer delete(fileTestContexts, options)
		takeConnectionLinks(options)
	})
	waitForFileImport(t)
	files := make(chan []string, 1)
	cx.Post(func() { cx.OnOpenFiles(func(paths []string) { files <- paths }) })
	select {
	case paths := <-files:
		if len(paths) != 1 || filepath.Base(paths[0]) != "launch.nsgo" {
			t.Fatalf("queued connection documents = %v", paths)
		}
		defer os.RemoveAll(filepath.Dir(paths[0]))
		if data, err := os.ReadFile(paths[0]); err != nil || string(data) != "launch native document" {
			t.Fatalf("queued import = %q, %v", data, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the first handler did not receive the connection's document")
	}
}

// TestSystemDocumentOpenInRoutesToRegisteredApp needs the owned
// testdata/iosdocuments app installed by the packaging integration test.
// It presents Open In, then invokes UIKit's default-recipient action to
// perform a real system transfer without tapping an out-of-process picker.
// Its auxiliary link case uses UIApplication through the public core API.
func TestSystemDocumentOpenInRoutesToRegisteredApp(t *testing.T) {
	which := os.Getenv("NSGO_IOS_DOCUMENT_CASE")
	if which != "cold" && which != "warm" && which != "link" {
		t.Skip("needs the owned iOS document receiver and NSGO_IOS_DOCUMENT_CASE=cold, warm or link")
	}
	ready := make(chan *core.Scope, 1)
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		ready <- cx
		return core.Text{Content: "Send a system document"}
	})
	app.WaitFor("text:Send a system document", 5*time.Second)
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		active := false
		onMainSync(func() {
			application := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
			active = objc.Send[int](application, sel("applicationState")) == 0 && objc.Send[bool](runner.window, sel("isKeyWindow"))
		})
		if active {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the document sender did not become active")
		}
	}
	cx := <-ready
	if which == "link" {
		opened := make(chan bool, 1)
		cx.Post(func() {
			cx.OpenURL("nsgo1385documents://separate-link", func(ok bool) { opened <- ok })
		})
		select {
		case ok := <-opened:
			if !ok {
				t.Fatal("UIKit did not open the receiver's custom link")
			}
		case <-time.After(10 * time.Second):
			t.Fatal("UIKit did not answer the custom-link request")
		}
		return
	}
	name := which + " #日本.nsgo1385doc"
	source := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(source, []byte("NSGo system document "+which+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	shown := make(chan error, 1)
	cx.Post(func() {
		cx.OpenFile(core.SharedFile{Path: source}, func(opened bool, err error) {
			if err == nil && !opened {
				err = fmt.Errorf("UIKit has no app for the registered document")
			}
			shown <- err
		})
	})
	select {
	case err := <-shown:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("UIKit did not present Open In")
	}
	sent := make(chan string, 1)
	var delegate objc.ID
	onMainSync(func() {
		t.Logf("native document UTI: %s", apple.GoString(runner.opening.Send(sel("UTI"))))
		class := register("NsgoDocumentSendingForTest", apple.Class("NSObject"), nil, []objc.MethodDef{{
			Cmd: sel("documentInteractionController:didEndSendingToApplication:"),
			Fn: func(_ objc.ID, _ objc.SEL, _, application objc.ID) {
				select {
				case sent <- apple.GoString(application):
				default:
				}
			},
		}})
		delegate = objc.ID(class).Send(selNew)
		runner.opening.Send(sel("setDelegate:"), delegate)
	})
	defer onMainSync(func() { runner.opening.Send(sel("setDelegate:"), objc.ID(0)); delegate.Send(selRelease) })
	// The test host already uses private UIKit input helpers. Keep this
	// selector test-only too: UIKit performs the transfer and creates the
	// receiver's real UIOpenURLContext; no delegate event is fabricated.
	var unavailable bool
	onMainSync(func() {
		action := sel("openDocumentWithDefaultApplication")
		if !objc.Send[bool](runner.opening, sel("respondsToSelector:"), action) {
			unavailable = true
			return
		}
		runner.opening.Send(action)
	})
	if unavailable {
		t.Fatal("UIKit's native document handoff action is unavailable")
	}
	select {
	case application := <-sent:
		if application != "dev.nsgo.documents1385" {
			t.Fatalf("UIKit sent the file to %q", application)
		}
		t.Logf("UIKit completed sending %s to %s", name, application)
	case <-time.After(10 * time.Second):
		t.Fatal("UIKit did not finish sending the file")
	}
}
