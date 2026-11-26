package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Services the platform performs for the app, asked for with command
// items and answered with events.

// ShowAlert is the core's part of base.ShowAlertDialog, which apps use: it shows a native alert and calls done with the index of the button chosen, or -1 when it was dismissed without one. done runs even when the owner that showed the alert is gone, such as a closed menu. A confined owner shows one only as Scope.Confine allows, else done gets -1 at once.
func (cx *Scope) ShowAlert(a AlertConfig, done func(index int)) {
	t := cx.tree()
	if t == nil {
		panic("core: ShowAlert needs a mounted tree")
	}
	if !t.confinedMay(cx.current(), refusedPresent, askedToPresent) {
		if done != nil {
			done(-1)
		}
		return
	}
	t.alertID++
	a.ID = t.alertID
	if t.alerts == nil {
		t.alerts = map[uint32]alertWait{}
	}
	t.alerts[a.ID] = alertWait{cx.current(), done}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAlert, Alert: &a})
	t.changed = true
}

type alertWait struct {
	owner *Scope
	done  func(int)
}

func (t *Tree) answerAlert(id uint32, index int) {
	w, ok := t.alerts[id]
	if !ok {
		return
	}
	delete(t.alerts, id)
	owner := w.owner
	if owner.disposed {
		owner = t.root
	}
	if w.done != nil {
		t.within(owner, func() { w.done(index) })
	}
}

// wait is a request the platform answers with an event.
type wait struct {
	owner       *Scope
	done        func(driver.Event)
	preflight   func(path string, attempt uint32)
	saveAttempt uint32
}

// request numbers item, sends it and calls done with the event that
// answers it, under the current owner, or the root's when that owner is
// gone by then.
func (t *Tree) request(cx *Scope, item driver.MountItem, done func(driver.Event)) uint32 {
	t.alertID++
	item.Count = t.alertID
	if t.waits == nil {
		t.waits = map[uint32]wait{}
	}
	t.waits[item.Count] = wait{owner: cx.current(), done: done}
	t.commands = append(t.commands, item)
	t.changed = true
	return item.Count
}

// answerSavePreflight checks one chosen destination and keeps the wait for
// the final result. Like answer, it uses the root if the requesting owner closed.
func (t *Tree) answerSavePreflight(ev driver.Event) {
	w := t.waits[ev.Count]
	if w.preflight == nil {
		return
	}
	attempt, _ := ev.Data.(uint32)
	if attempt != 0 && attempt <= w.saveAttempt {
		return
	}
	check := w.preflight
	if attempt == 0 {
		w.preflight = nil
	}
	w.saveAttempt = attempt
	t.waits[ev.Count] = w
	owner := w.owner
	if owner.disposed {
		owner = t.root
	}
	t.within(owner, func() { check(ev.Text, attempt) })
}

// answer runs the wait ev answers, once.
func (t *Tree) answer(ev driver.Event) {
	w, ok := t.waits[ev.Count]
	if !ok {
		return
	}
	delete(t.waits, ev.Count)
	owner := w.owner
	if owner.disposed {
		owner = t.root
	}
	t.within(owner, func() { w.done(ev) })
}

// DismissKeyboard takes focus away from the focused text field, hiding the software keyboard. Under a confined owner it does only as Scope.Confine allows.
func (cx *Scope) DismissKeyboard() {
	if t := cx.tree(); t != nil && t.mayMoveFocus(cx.current()) {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemDismissKeyboard})
		t.changed = true
	}
}

// SetClipboard puts text on the system clipboard, replacing what it
// holds.
func (cx *Scope) SetClipboard(text string) {
	if t := cx.tree(); t != nil && !cx.denied(serviceClipboard, "") {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemSetClipboard, Value: text})
		t.changed = true
	}
}

// SetClipboardImage puts the image data encodes on the system clipboard,
// replacing what it holds: PNG, JPEG, GIF or WebP bytes, as
// ReadClipboardImage gives them or an ImageSource holds them.
func (cx *Scope) SetClipboardImage(data []byte) {
	if t := cx.tree(); t != nil && len(data) > 0 && !cx.denied(serviceClipboard, "") {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemSetClipboardImage, Data: data})
		t.changed = true
	}
}

func (cx *Scope) readClipboard(what driver.ClipboardRead, done func(driver.Event)) {
	if t := cx.tree(); t != nil && !cx.denied(serviceClipboard, "") {
		t.request(cx, driver.MountItem{Kind: driver.ItemReadClipboard, Clipboard: what}, done)
	}
}

// ReadClipboard calls done with the clipboard's text, "" when it holds
// none. On iOS, reading it may show the system's paste notice.
func (cx *Scope) ReadClipboard(done func(text string)) {
	cx.readClipboard(driver.ClipboardReadText, func(ev driver.Event) { done(ev.Text) })
}

// HasClipboardText calls done with whether the clipboard holds text,
// without reading it, so iOS shows no paste notice.
func (cx *Scope) HasClipboardText(done func(bool)) {
	cx.readClipboard(driver.ClipboardReadHasText, func(ev driver.Event) { done(ev.Finished) })
}

// ReadClipboardImage calls done with the clipboard's image as a PNG, or
// ok false when it holds none. An image it cannot read, or whose PNG is
// over MaxClipboardImage bytes (CodeQuotaExceeded), fails. On iOS,
// reading it may show the system's paste notice.
func (cx *Scope) ReadClipboardImage(done func(img ClipboardImage, ok bool, err error)) {
	cx.readClipboard(driver.ClipboardReadImage, func(ev driver.Event) {
		img, _ := ev.Data.(*driver.ClipboardImage)
		switch {
		case img == nil:
			done(ClipboardImage{}, false, nil)
		case img.Err != "":
			done(ClipboardImage{}, false, fmt.Errorf("nsgo: the clipboard's image cannot be read: %s", img.Err))
		case img.TooLarge > 0:
			done(ClipboardImage{}, false, &Error{Code: CodeQuotaExceeded,
				Message: fmt.Sprintf("nsgo: the clipboard's image is %d bytes as a PNG, over %d", img.TooLarge, MaxClipboardImage)})
		case len(img.PNG) == 0:
			done(ClipboardImage{}, false, nil)
		default:
			done(*img, true, nil)
		}
	})
}

// HasClipboardImage calls done with whether the clipboard holds an
// image, without reading it.
func (cx *Scope) HasClipboardImage(done func(bool)) {
	cx.readClipboard(driver.ClipboardReadHasImage, func(ev driver.Event) { done(ev.Finished) })
}

// ReadClipboardFiles calls done with the file: URLs of the local files on the clipboard, such as files copied in a file manager, which FileFromURL opens, none when it holds none.
func (cx *Scope) ReadClipboardFiles(done func(uris []string)) {
	cx.readClipboard(driver.ClipboardReadFiles, func(ev driver.Event) { done(ev.URIs) })
}

// Announce has the screen reader say text. A confined owner's is said only as Scope.Confine allows, else dropped.
func (cx *Scope) Announce(text string) {
	if t := cx.tree(); t != nil && t.confinedMay(cx.current(), refusedAnnounce, "asked the screen reader to speak") {
		t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAnnounce, Value: text})
		t.changed = true
	}
}

// SelectInputSource selects the enabled keyboard input source of id, as
// Input's InputSource does, remembering the one it replaces. An id is the platform's: a Text Input Sources ID on macOS
// ("com.apple.keylayout.ABC"), a keyboard layout ID on Windows
// ("00000409"), and a Fcitx 5 input method ("keyboard-us") or an IBus
// engine ("xkb:us::eng") on Linux. Phones and the web ignore it.
func (cx *Scope) SelectInputSource(id string) {
	if t := cx.tree(); t != nil {
		t.selectInputSource(id)
	}
}

// selectInputSource is SelectInputSource's on t.
func (t *Tree) selectInputSource(id string) {
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemInputSource, Value: id})
	t.changed = true
}

// RestoreInputSource goes back to the source SelectInputSource replaced,
// unless the user chose another since.
func (cx *Scope) RestoreInputSource() { cx.SelectInputSource("") }

// MaxURLLength is the longest URL OpenURL opens, in bytes: a mailto:
// link with a body fits.
const MaxURLLength = driver.MaxURLLength

// CheckURL reports why OpenURL would refuse u: it must be absolute, at
// most MaxURLLength bytes, without spaces or control characters, a web
// address with a host, and of a scheme that opens another app, not
// javascript:, data:, blob:, file:, about: or asset:.
func CheckURL(u string) error { return driver.CheckURL(u) }

// RegisterFont registers the font file data (TrueType, OpenType or a collection) with the platform, so text finds its families as a Style's FontFamily names them, as nsgo.Fonts' are, and prepares the text shown and records the canvases shown again once the font is available: call it before the text that uses it shows. Without a platform that registers fonts as the app runs (a headless test's) it does nothing.
func (cx *Scope) RegisterFont(data []byte) error {
	t := cx.tree()
	if t == nil {
		return nil
	}
	fr, ok := t.text.(driver.FontRegistrar)
	if !ok {
		return nil
	}
	if err := fr.RegisterFont(data); err != nil {
		return err
	}
	t.FontsChanged()
	return nil
}

// FontsChanged prepares every text and records every canvas again, with fonts the platform made available since they were prepared.
func (t *Tree) FontsChanged() {
	t.fonts++
	for _, n := range t.nodes {
		if n == nil || n.deleted {
			continue
		}
		switch {
		case n.text != nil:
			n.text.stale = true
			t.markLayout(n)
		case n.input != nil:
			n.input.lineH = 0
			t.markLayout(n)
		case n.canvas != nil && n.canvas.effect != nil:
			if n.icon != nil && !n.icon.svg.GetUntracked().HasText() {
				continue
			}
			// Draw can measure text without recording any text operations, so run it again rather than replay its recording: both its geometry and its glyphs take the new fonts.
			n.canvas.effect.mark(dirty)
		}
	}
	for _, off := range t.offscreens {
		off.Invalidate()
	}
	t.measures = newMeasureCache()
	t.markAll()
	if t.fontsRead != nil {
		t.fontsRead.Set(t.fonts)
	}
	t.rt.maybeFlush()
}

// OpenURL opens u outside the app: a web page in the browser, a mailto:
// or tel: link in its app, any scheme another app registered. An
// embedding host's handler (Host.HandleOpenURL) takes it first. done,
// when not nil, learns whether it opened: false for a URL CheckURL
// refuses, or one no app handles.
func (cx *Scope) OpenURL(u string, done func(opened bool)) {
	answer := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	if CheckURL(u) != nil || cx.denied(serviceOpenURL, u) {
		answer(false)
		return
	}
	o := cx.current()
	platform := func() {
		t := cx.tree()
		if t == nil || o.disposed {
			answer(false)
			return
		}
		t.request(o, driver.MountItem{Kind: driver.ItemOpenURL, Value: u}, func(ev driver.Event) { answer(ev.Finished) })
	}
	asked := cx.hostApp().openURL(u, func(handled bool) {
		if handled {
			cx.rt.run(o, func() { answer(true) })
		} else {
			cx.rt.run(o, platform)
		}
	})
	if !asked {
		platform()
	}
}

// LookUp is the core's part of desktop.LookUp, which apps use: it shows the platform's dictionary panel for text at at, a box in the view in refers to (the surface's without in), as a TextSelection's Bounds is in its view. It reports whether it asked the platform: false for blank text, an in that is not mounted, where Env.CanLookUp is false, and for a confined owner as Scope.Confine allows a PopupMenu.
func (cx *Scope) LookUp(text string, in *Ref, at Bounds) bool {
	t := cx.tree()
	if t == nil || strings.TrimSpace(text) == "" || !Untracked(cx, cx.Env().CanLookUp) {
		return false
	}
	if in != nil {
		b, ok := in.Bounds()
		if !ok {
			return false
		}
		at.Origin.X += b.Origin.X
		at.Origin.Y += b.Origin.Y
	}
	if !t.confinedMay(cx.current(), refusedPresent, askedToPresent) {
		return false
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemLookUp, Value: text, Frame: at})
	t.changed = true
	return true
}

// QuickLook is the core's part of desktop.QuickLook, which apps use: it shows the system's file preview panel on paths, starting at index, clamped to the list. It reports whether it asked the platform: false for an empty list, a path that is not a regular file, where Env.CanQuickLook is false, and for a confined owner as Scope.Confine allows a PopupMenu.
func (cx *Scope) QuickLook(paths []string, index int) bool {
	t := cx.tree()
	if t == nil || len(paths) == 0 || !Untracked(cx, cx.Env().CanQuickLook) {
		return false
	}
	resolved := make([]string, len(paths))
	for i, path := range paths {
		f, err := resolvePath(path, false)
		if err != nil {
			return false
		}
		resolved[i] = f.Path
	}
	if !t.confinedMay(cx.current(), refusedPresent, askedToPresent) {
		return false
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemQuickLook,
		QuickLook: &driver.QuickLookRequest{Paths: resolved, Index: min(max(index, 0), len(resolved)-1)}})
	t.changed = true
	return true
}

// RequestReview asks the platform's store for an in-app rating where
// Env.CanRequestReview is true. The store decides when to show the prompt
// and reports no result. Under a confined owner, Scope.Confine applies.
func (cx *Scope) RequestReview() {
	t := cx.tree()
	if t == nil || !Untracked(cx, cx.Env().CanRequestReview) {
		return
	}
	if !t.confinedMay(cx.current(), refusedPresent, askedToPresent) {
		return
	}
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemRequestReview})
	t.changed = true
}

// PickOptions say how PickImages picks and prepares images.
type PickOptions struct {
	// Multiple lets the user pick several images, at most Limit (0: up
	// to MaxPicked).
	Multiple bool
	Limit    int
	// MaxDimension scales images whose longer side has more pixels down
	// to it, encoding them again; 0 keeps their size.
	MaxDimension int
	// Quality is the quality JPEGs are encoded at, from 0 (smallest) to
	// 1; 0 means 0.9, and keeps JPEGs that need no other change as they
	// are.
	Quality float32
	// Format says which formats images come in: by default JPEG, PNG, GIF
	// and WebP as they are and anything else, such as HEIC, as JPEG.
	Format PickFormat
}

// request is o as the platform carries it out.
func (o PickOptions) request() driver.PickRequest {
	return driver.PickRequest{Limit: pickLimit(o.Multiple, o.Limit), MaxDimension: max(o.MaxDimension, 0),
		Quality: min(max(o.Quality, 0), 1), Format: o.Format}
}

// pickLimit is the most a pick answers with: 1 for a single pick, else
// limit, up to MaxPicked (0: MaxPicked).
func pickLimit(multiple bool, limit int) int {
	switch {
	case !multiple:
		return 1
	case limit > 0:
		return min(limit, driver.MaxPicked)
	}
	return driver.MaxPicked
}

// PickImages shows the system's photo picker and calls done with the
// images the user picked, in the order picked, prepared as o says, or
// none when they cancelled: PHPickerViewController on iOS, Android's
// photo picker, an open panel on the desktops and a file chooser on the
// web. The mobile pickers need no permission.
func (cx *Scope) PickImages(o PickOptions, done func(files []PickedFile)) {
	t := cx.tree()
	if t == nil || cx.denied(servicePicker, "") {
		return
	}
	r := o.request()
	t.request(cx, driver.MountItem{Kind: driver.ItemPickImages, Pick: &r}, func(ev driver.Event) {
		files, _ := ev.Data.([]driver.PickedFile)
		if done != nil {
			done(driver.FinishPick(r, files, nil))
		}
	})
}

// PhotoOptions say how TakePhoto prepares the photo taken.
type PhotoOptions struct {
	// MaxDimension scales a photo whose longer side has more pixels down
	// to it; 0 keeps its size.
	MaxDimension int
	// Quality is the quality the JPEG is encoded at, from 0 (smallest) to
	// 1; 0 means 0.9.
	Quality float32
}

// ErrNoCamera is TakePhoto's failure where Env.CanTakePhoto is false.
var ErrNoCamera = errors.New("core: no camera to take a photo with")

// TakePhoto shows the system's camera for the user to take one photo
// and calls done with it, a JPEG turned upright and prepared as o says,
// or with nil when they cancelled; err says what failed, such as a
// refused permission or ErrNoCamera. iOS shows UIImagePickerController
// and needs NSCameraUsageDescription in the app's Info.plist; Android
// starts the camera app, and the web a file chooser that offers it. The desktops show ScanCode's camera preview with a button that takes the photo: macOS in an AppKit sheet with AVCapturePhotoOutput, needing NSCameraUsageDescription as iOS does, and Windows (MediaCapture) and Linux (the desktop Camera portal with GStreamer pipewiresrc) in a modal over the window, encoding the frame shown as the JPEG.
func (cx *Scope) TakePhoto(o PhotoOptions, done func(photo *PickedFile, err error)) {
	t := cx.tree()
	if t == nil || cx.denied(servicePicker, "") {
		return
	}
	if !Untracked(cx, cx.Env().CanTakePhoto) {
		if done != nil {
			done(nil, ErrNoCamera)
		}
		return
	}
	r := driver.PickRequest{Limit: 1, MaxDimension: max(o.MaxDimension, 0), Quality: min(max(o.Quality, 0), 1)}
	t.request(cx, driver.MountItem{Kind: driver.ItemTakePhoto, Pick: &r}, func(ev driver.Event) {
		if done == nil {
			return
		}
		if ev.Text == driver.NoCamera {
			done(nil, ErrNoCamera)
			return
		}
		if ev.Text != "" {
			done(nil, errors.New(ev.Text))
			return
		}
		files, _ := ev.Data.([]driver.PickedFile)
		if files = driver.FinishPick(r, files, nil); len(files) == 0 {
			done(nil, nil)
			return
		}
		done(&files[0], nil)
	})
}

// PickFilesOptions say which files PickFiles offers.
type PickFilesOptions struct {
	// Multiple lets the user pick several files, at most Limit (0: up to
	// MaxPicked).
	Multiple bool
	Limit    int
	// Types are the files offered: media types (application/pdf), families (image/*) and extensions (.zip); none offers every file.
	Types []string
	// TypesName names Types in the type menu of the pickers that have one, Windows' and Linux's; "" shows the English "Files", or on Windows "All files" where every file is offered.
	TypesName string
	// Title, Button and Start are hints a platform without the setting ignores, each "" for the system's own: Title says what the picker is for, as its title or, on macOS, whose sheets have none, its message; Button labels the button that picks; Start is the absolute path of the folder it opens in, ignored where no such folder exists. macOS, Windows and Linux take all three, iOS only Start, Android and the web none.
	Title  string
	Button string
	Start  string
}

// request is o as the platform carries it out.
func (o PickFilesOptions) request() driver.FilesRequest {
	r := driver.FilesRequest{Limit: pickLimit(o.Multiple, o.Limit), Title: o.Title, Button: o.Button, TypesName: o.TypesName, Start: o.Start}
	for _, t := range o.Types {
		t = strings.ToLower(strings.TrimSpace(t))
		switch {
		case t == "*" || t == "*/*" || t == ".*":
			r.Types = nil
			return r
		case t == "":
			continue
		case !strings.HasPrefix(t, ".") && !strings.Contains(t, "/"):
			t = "." + t
		}
		r.Types = append(r.Types, t)
	}
	return r
}

// PickFiles shows the system's file picker and calls done with the files
// the user picked, as they are, or none when they cancelled; err says
// what failed. Each is a file on disk the app opens and streams: the
// user's own on desktops, a copy in the app's storage on iOS and Android
// (File.Temporary) that is the app's to delete, and none on the web,
// where File.Open reads it. The pickers need no permission.
func (cx *Scope) PickFiles(o PickFilesOptions, done func(files []File, err error)) {
	t := cx.tree()
	if t == nil || cx.denied(servicePicker, "") {
		return
	}
	r := o.request()
	t.request(cx, driver.MountItem{Kind: driver.ItemPickFiles, Files: &r}, func(ev driver.Event) {
		if done == nil {
			return
		}
		if ev.Text != "" {
			done(nil, errors.New(ev.Text))
			return
		}
		files, _ := ev.Data.([]driver.File)
		done(driver.FinishFiles(r, files), nil)
	})
}

// PickScreenColor lets the user pick a color anywhere on the screen and
// calls done with it, or with false when they cancel or the platform has
// no way to (Env.CanPickScreenColor). base.ColorEyeDropper is its button.
func (cx *Scope) PickScreenColor(done func(c Color, ok bool)) {
	t := cx.tree()
	if t == nil || cx.denied(servicePicker, "") || !Untracked(cx, cx.Env().CanPickScreenColor) {
		done(0, false)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemPickScreenColor}, func(ev driver.Event) {
		c, ok := ev.Data.(Color)
		done(c, ok && ev.Finished)
	})
}

// PickDirectoriesOptions say how PickDirectories picks folders.
type PickDirectoriesOptions struct {
	// Multiple lets the user pick several folders, at most Limit (0: up
	// to MaxPicked).
	Multiple bool
	Limit    int
	// Title, Button and Start are hints as in PickFilesOptions.
	Title  string
	Button string
	Start  string
}

// PickDirectories shows the system's folder picker and calls done with
// the folders the user picked, or none when they cancelled; err says
// what failed. Each is the user's own folder, which the app reads as an
// fs.FS: NSOpenPanel on macOS, IFileOpenDialog on Windows, GtkFileDialog,
// through the desktop portal where there is one, on Linux and
// UIDocumentPickerViewController on iOS give its path; Android's document
// tree picker and the web's folder picker give none, so the Directory
// reads through the platform and the app reads it off the engine's
// goroutine. Android and the web pick one folder. The pickers need no
// permission.
func (cx *Scope) PickDirectories(o PickDirectoriesOptions, done func(dirs []Directory, err error)) {
	t := cx.tree()
	if t == nil || cx.denied(servicePicker, "") {
		return
	}
	r := driver.DirectoriesRequest{Limit: pickLimit(o.Multiple, o.Limit), Title: o.Title, Button: o.Button, Start: o.Start}
	t.request(cx, driver.MountItem{Kind: driver.ItemPickDirectories, Directories: &r}, func(ev driver.Event) {
		if done == nil {
			return
		}
		if ev.Text != "" {
			done(nil, errors.New(ev.Text))
			return
		}
		dirs, _ := ev.Data.([]driver.Directory)
		done(driver.FinishDirectories(r, dirs), nil)
	})
}

// SaveOptions say what SaveFile saves: the file at Path, or else Data.
type SaveOptions struct {
	// Name is the file name the dialog suggests; Path's when empty.
	Name string
	// MIME is its media type; Name's extension says when empty.
	MIME string
	// Start is the absolute path of an existing folder to open in the save
	// dialog on macOS, Windows, Linux and iOS. An empty or unusable path
	// leaves the choice to the system.
	Start string
	Path  string
	Data  []byte
	// Preflight asks the platform to check the chosen destination before saving.
	// Setting Accept also requests this check.
	Preflight bool
	// Accept runs on the engine's goroutine after the user chooses where to
	// save. A nil return approves the destination. If it returns an error,
	// SaveFile leaves the destination untouched. On macOS, Windows and
	// Linux's in-process GTK chooser, the dialog stays open for another choice.
	// Cancelling after a refusal reports the refused destination and error. Elsewhere a refusal
	// completes the save with that destination and error.
	// Android passes the chosen tree URI with the app's file name in its
	// name query parameter. The web has no destination path and skips Accept.
	Accept func(path string) error
}

// SaveFile shows the system's save dialog and copies the file o names to
// where the user chose, streaming it, then calls done with whether it
// was saved, false with no error when they cancelled, and on desktops
// the path it was saved at, which an app saves to directly next time. A
// save that fails leaves nothing where the user chose, or the file that
// was there. The web has no files on disk, so it saves Data: a save
// picker where the browser has one, else a download, which counts as
// saved. An iOS or HarmonyOS save requiring preflight reports an error.
func (cx *Scope) SaveFile(o SaveOptions, done func(saved bool, path string, err error)) {
	answer := func(saved bool, path string, err error) {
		if done != nil {
			done(saved, path, err)
		}
	}
	t := cx.tree()
	if t == nil || cx.denied(servicePicker, "") {
		return
	}
	r := driver.SaveRequest{Name: o.Name, MIME: o.MIME, Path: o.Path, Data: o.Data, Preflight: o.Preflight || o.Accept != nil}
	if filepath.IsAbs(o.Start) {
		r.Start = o.Start
	}
	if r.Path != "" {
		if _, err := os.Stat(r.Path); err != nil {
			answer(false, "", err)
			return
		}
	}
	if i := strings.LastIndexAny(r.Name, `/\`); i >= 0 {
		r.Name = r.Name[i+1:]
	}
	if r.Name == "" && r.Path != "" {
		r.Name = filepath.Base(r.Path)
	}
	if r.Name == "" {
		r.Name = "file"
	}
	if r.MIME == "" {
		r.MIME = driver.MIMEForFile(r.Name)
	}
	var denied error
	var deniedPath string
	id := t.request(cx, driver.MountItem{Kind: driver.ItemSaveFile, Save: &r}, func(ev driver.Event) {
		path, _ := ev.Data.(string)
		if denied != nil && !ev.Finished && path == deniedPath && ev.Text == denied.Error() {
			answer(false, path, denied)
			return
		}
		if ev.Text != "" {
			answer(false, path, errors.New(ev.Text))
			return
		}
		answer(ev.Finished, path, nil)
	})
	if r.Preflight {
		w := t.waits[id]
		w.preflight = func(path string, attempt uint32) {
			deniedPath = path
			denied = errors.New("the save preflight did not finish")
			defer func() {
				response := driver.MountItem{Kind: driver.ItemSavePreflightAnswer, Count: id, On: denied == nil}
				if attempt != 0 {
					response.Data = attempt
				}
				if denied != nil {
					response.Value = denied.Error()
				}
				t.commands = append(t.commands, response)
				t.changed = true
			}()
			if o.Accept == nil {
				denied = nil
				return
			}
			denied = o.Accept(path)
		}
		t.waits[id] = w
	}
}

// resolveShared returns f as the platform hands it on, with an absolute
// path or its bytes, a name and a type, or why it cannot be: a file with
// neither path nor data, one that is not there, or not a regular file.
func resolveShared(f SharedFile, allowURI bool) (driver.SharedFile, error) {
	if f.URI != "" {
		if !allowURI {
			return driver.SharedFile{}, errors.New("core: content URIs are supported on Android only")
		}
		if !strings.HasPrefix(f.URI, "content://") || len(f.URI) <= len("content://") {
			return driver.SharedFile{}, errors.New("core: a shared URI must be a content URI")
		}
		r := driver.SharedFile{URI: f.URI, Name: f.Name, MIME: f.MIME}
		if i := strings.LastIndexAny(r.Name, `/\`); i >= 0 {
			r.Name = r.Name[i+1:]
		}
		if r.Name == "" || r.Name == "." || r.Name == ".." {
			r.Name = "file"
		}
		if r.MIME == "" {
			r.MIME = driver.MIMEForFile(r.Name)
		}
		return r, nil
	}
	if f.Path != "" {
		return resolveOpened(f, false, allowURI)
	}
	if f.Data == nil {
		return driver.SharedFile{}, errors.New("core: a shared file has a path or data")
	}
	r := driver.SharedFile{Data: f.Data, Name: f.Name, MIME: f.MIME}
	if i := strings.LastIndexAny(r.Name, `/\`); i >= 0 {
		r.Name = r.Name[i+1:]
	}
	if r.Name == "" || r.Name == "." || r.Name == ".." {
		r.Name = "file"
	}
	if r.MIME == "" {
		r.MIME = driver.MIMEForFile(r.Name)
	}
	return r, nil
}

// resolveOpened is resolveShared, taking a folder too when folders is
// set.
func resolveOpened(f SharedFile, folders, allowURI bool) (driver.SharedFile, error) {
	if f.URI != "" {
		return resolveShared(f, allowURI)
	}
	if f.Path == "" {
		return driver.SharedFile{}, errors.New("core: a shared file has a path")
	}
	path, err := filepath.Abs(f.Path)
	if err != nil {
		return driver.SharedFile{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return driver.SharedFile{}, err
	}
	if info.IsDir() && folders {
		return driver.SharedFile{Path: path, Name: filepath.Base(path), MIME: "inode/directory"}, nil
	}
	if !info.Mode().IsRegular() {
		return driver.SharedFile{}, fmt.Errorf("core: %s is not a file", path)
	}
	r := driver.SharedFile{Path: path, Name: f.Name, MIME: f.MIME}
	if i := strings.LastIndexAny(r.Name, `/\`); i >= 0 {
		r.Name = r.Name[i+1:]
	}
	if r.Name == "" || r.Name == "." || r.Name == ".." {
		r.Name = filepath.Base(path)
	}
	if r.MIME == "" {
		r.MIME = driver.MIMEForFile(r.Name)
	}
	return r, nil
}

// OpenFile opens the file f names in the app the system associates with
// its type, the user choosing where several are: NSWorkspace on macOS,
// the shell on Windows, GtkFileLauncher on Linux, a document
// interaction's options on iOS and Intent.ACTION_VIEW on Android. On
// macOS, Windows and Linux f may name a folder, which opens in the file
// manager; phones have none to open it in, and answer an error. done,
// when not nil, learns whether an app opened it: false with no error
// when none opens its type, and the error when the file is missing or
// the platform failed. On Android the file is in the app's own storage,
// such as DataDir and CacheDir, and the app opening it reads it through
// a content URI the runtime grants it, until its activity finishes; the
// file stays the app's to keep there that long and to delete. The web
// opens no files.
func (cx *Scope) OpenFile(f SharedFile, done func(opened bool, err error)) {
	answer := func(opened bool, err error) {
		if done != nil {
			done(opened, err)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	os := cx.Env().hostUntracked()
	r, err := resolveOpened(f, os == OSMacOS || os == OSWindows || os == OSLinux, os == OSAndroid)
	if err != nil {
		answer(false, err)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemOpenFile, Open: &r}, func(ev driver.Event) {
		if ev.Text != "" {
			answer(false, errors.New(ev.Text))
			return
		}
		answer(ev.Finished, nil)
	})
}

// ErrNoTrash is what TrashFile answers when the item's volume has no Trash or Recycle Bin, or its Recycle Bin would not keep the item, such as a file larger than the bin on Windows, which leaves the item where it was.
var ErrNoTrash = errors.New("core: the volume has no trash")

// RevealFile is the core's part of desktop.RevealFile, which apps use: it opens the file manager with the file or folder at path selected, and answers an error when the path is missing or the platform has no file manager.
func (cx *Scope) RevealFile(path string, done func(error)) {
	answer := func(err error) {
		if done != nil {
			done(err)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	f, err := resolvePath(path, true)
	if err != nil {
		answer(err)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemRevealFile, Open: &f}, func(ev driver.Event) {
		if ev.Text != "" {
			answer(errors.New(ev.Text))
			return
		}
		if !ev.Finished {
			answer(errors.New(driver.NoFileRevealer))
			return
		}
		answer(nil)
	})
}

// TrashFile is the core's part of desktop.TrashFile, which apps use: it moves the file or folder at path to the user's Trash or Recycle Bin, and never deletes it permanently, answering ErrNoTrash where it cannot be recycled. trashedAt is the item's path in the trash where the platform reports it, and "" elsewhere, such as on Linux.
func (cx *Scope) TrashFile(path string, done func(trashedAt string, err error)) {
	answer := func(at string, err error) {
		if done != nil {
			done(at, err)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	f, err := resolvePath(path, true)
	if err != nil {
		answer("", err)
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemTrashFile, Open: &f}, func(ev driver.Event) {
		if ev.Text != "" {
			if ev.Text == driver.NoTrash {
				answer("", ErrNoTrash)
			} else {
				answer("", errors.New(ev.Text))
			}
			return
		}
		at, _ := ev.Data.(string)
		if !ev.Finished {
			answer(at, errors.New("core: file could not be trashed"))
			return
		}
		answer(at, nil)
	})
}

func resolvePath(path string, folders bool) (driver.SharedFile, error) {
	if path == "" {
		return driver.SharedFile{}, errors.New("core: a path is required")
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return driver.SharedFile{}, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return driver.SharedFile{}, err
	}
	if !folders && !info.Mode().IsRegular() {
		return driver.SharedFile{}, fmt.Errorf("core: %s is not a file", path)
	}
	return driver.SharedFile{Path: abs, Name: filepath.Base(abs), MIME: driver.MIMEForFile(filepath.Base(abs))}, nil
}

// ShareOptions say what Share shares: files, text, or both.
type ShareOptions struct {
	Files []SharedFile
	// Text goes with the files, or alone: a message or a link.
	Text string
	// Anchor is the element the sheet points at where it shows as a
	// popover, on macOS and iPads; nil points at the root's center.
	Anchor *Ref
}

// Share shows the system's share sheet for what o names, for the user to
// send it to another app: UIActivityViewController on iOS,
// NSSharingServicePicker on macOS, an Intent.ACTION_SEND chooser on
// Android, the Share UI (IDataTransferManagerInterop) on Windows, the
// desktop portal's OpenURI chooser or the file manager on Linux, and
// navigator.share on the web. Disk files go by path or content URI, never
// through the platform's bridge, so large ones cost nothing more; on the
// web, and anywhere else a file is held in memory, SharedFile.Data carries
// its bytes. done, when not nil, learns whether the sheet showed (or on
// the web and Linux portal, completed rather than being cancelled), with
// the error when it did not: a missing file, or a platform or browser
// without a share sheet. That it showed says nothing of what the app the
// user chose did with what it received. On Android the files are in the
// app's own storage, such as DataDir and CacheDir, and the app chosen
// reads them through content URIs the runtime grants it, until its
// activity finishes; the files stay the app's to keep there that long
// and to delete.
func (cx *Scope) Share(o ShareOptions, done func(shown bool, err error)) {
	answer := func(shown bool, err error) {
		if done != nil {
			done(shown, err)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	r := driver.ShareRequest{Text: o.Text}
	for _, f := range o.Files {
		sf, err := resolveShared(f, cx.Env().hostUntracked() == OSAndroid)
		if err != nil {
			answer(false, err)
			return
		}
		r.Files = append(r.Files, sf)
	}
	if len(r.Files) == 0 && r.Text == "" {
		answer(false, errors.New("core: a share has files or text"))
		return
	}
	if o.Anchor != nil {
		r.From, _ = o.Anchor.Bounds()
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemShare, Share: &r}, func(ev driver.Event) {
		if ev.Text == driver.NoShareSheet {
			answer(false, Unavailable(ev.Text))
			return
		}
		if ev.Text != "" {
			answer(false, errors.New(ev.Text))
			return
		}
		answer(ev.Finished, nil)
	})
}

// errNoInstaller is what InstallPackage answers off Android.
var errNoInstaller = Unavailable("core: only Android installs packages")

// InstallPackage is the core's part of mobile.InstallPackage, which apps
// use: it hands the APK at path to Android's package installer.
func (cx *Scope) InstallPackage(path string, done func(started bool, err error)) {
	if cx.tree() == nil {
		return
	}
	if cx.Env().hostUntracked() != OSAndroid {
		if done != nil {
			done(false, errNoInstaller)
		}
		return
	}
	cx.installPackage(SharedFile{Path: path, MIME: "application/vnd.android.package-archive"}, done)
}

func (cx *Scope) installPackage(f SharedFile, done func(bool, error)) {
	t := cx.tree()
	if t == nil {
		return
	}
	r, err := resolveOpened(f, false, false)
	// A backend without filesystem access can still report that installation is unavailable.
	if err != nil && !errors.Is(err, errors.ErrUnsupported) {
		if done != nil {
			done(false, err)
		}
		return
	}
	t.request(cx, driver.MountItem{Kind: driver.ItemInstallPackage, Open: &r}, func(ev driver.Event) {
		if done == nil {
			return
		}
		switch ev.Text {
		case "":
			done(ev.Finished, nil)
		case driver.NoInstaller:
			done(false, Unavailable(ev.Text))
		default:
			done(false, errors.New(ev.Text))
		}
	})
}

// CanInstallPackages is the core's part of mobile.CanInstallPackages,
// which apps use: it asks Android whether the user lets the app install
// packages.
func (cx *Scope) CanInstallPackages(done func(allowed bool)) {
	cx.androidAsk(driver.ItemCanInstallPackages, done)
}

// OpenInstallSettings is the core's part of mobile.OpenInstallSettings,
// which apps use: it opens Android's setting that lets the app install
// packages.
func (cx *Scope) OpenInstallSettings(done func(opened bool)) {
	cx.androidAsk(driver.ItemInstallSettings, done)
}

// androidAsk sends Android a request of kind that answers yes or no,
// and answers no elsewhere.
func (cx *Scope) androidAsk(kind driver.ItemKind, done func(bool)) {
	answer := func(ok bool) {
		if done != nil {
			done(ok)
		}
	}
	t := cx.tree()
	if t == nil {
		return
	}
	if cx.Env().hostUntracked() != OSAndroid {
		answer(false)
		return
	}
	t.request(cx, driver.MountItem{Kind: kind}, func(ev driver.Event) { answer(ev.Finished) })
}

// SetAppearance shows the app light or dark whatever the system's
// appearance, or follows it again with AppearanceSystem: Env.Dark
// reports the one shown, and the platform's own chrome, such as title
// bars, alerts, menus, system headers and the views it hosts, follows.
func (cx *Scope) SetAppearance(a Appearance) {
	if t := cx.rt.tree; t != nil {
		t.SetAppearance(a)
	}
}

// SetAppearance forces the appearance the app shows, as
// Scope.SetAppearance does: a host sets the one the app starts in.
func (t *Tree) SetAppearance(a Appearance) {
	e := t.root.Env()
	if e.appearance.GetUntracked() == a {
		return
	}
	t.root.Batch(func() {
		e.appearance.Set(a)
		e.dark.Set(e.shownDark())
		// The windows share the appearance the app forces.
		for _, o := range t.rt.surfaces {
			if o.env != nil {
				o.env.dark.Set(o.env.shownDark())
			}
		}
	})
	t.commands = append(t.commands, driver.MountItem{Kind: driver.ItemAppearance, Appearance: a})
	t.changed = true
}

// Quit ends the app: desktops close the window, the app's last, and
// return from nsgo.Run; Android finishes the app's activity. iOS apps do
// not quit themselves, and the web leaves the page open.
func (cx *Scope) Quit() { cx.rt.quit(false) }

// restarting records that an app asked to restart, for nsgo.Run, which
// starts the program again once the app has quit.
var restarting atomic.Bool

// Restart is the core's part of desktop.Restart, which apps use: the app
// quits as Quit ends it, and nsgo.Run starts it again.
func (cx *Scope) Restart() {
	restarting.Store(true)
	cx.rt.quit(true)
}

// Restarting reports whether the app asked to restart before it quit, for
// nsgo.Run.
func Restarting() bool { return restarting.Load() }
