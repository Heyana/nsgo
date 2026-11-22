//go:build ios

package ios

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// The services UIKit performs: the pasteboard's images and files,
// opening URLs and files, the photo picker, the document picker for
// files and saves, the share sheet, and the app's appearance.

func generalPasteboard() objc.ID {
	if onTV() {
		return 0 // UIPasteboard is unavailable on tvOS.
	}
	return objc.ID(apple.Class("UIPasteboard")).Send(sel("generalPasteboard"))
}

// The pasteboard types of images, in the order a read prefers them.
var imageTypes = []string{"public.png", "public.jpeg", "public.heic", "public.tiff", "com.compuserve.gif"}

// setClipboardImage puts the image data encodes on the pasteboard as a
// PNG.
func setClipboardImage(data []byte) {
	pasteboard := generalPasteboard()
	if pasteboard == 0 {
		return
	}
	png, _, err := apple.PNG(data)
	if err != nil {
		return
	}
	pasteboard.Send(sel("setData:forPasteboardType:"), apple.NSData(png), apple.NSString("public.png"))
}

var uiImagePNG func(image objc.ID) objc.ID

// readClipboard answers an ItemReadClipboard. Asking whether the
// pasteboard holds text or images reads nothing, so iOS shows no paste
// notice for it.
func (s *surface) readClipboard(it *driver.MountItem) {
	pb := generalPasteboard()
	ev := driver.Event{Kind: driver.EventClipboard, Count: it.Count}
	if pb == 0 {
		s.engine.Dispatch(ev)
		return
	}
	switch it.Clipboard {
	case driver.ClipboardReadText:
		ev.Text = apple.GoString(pb.Send(sel("string")))
	case driver.ClipboardReadHasText:
		ev.Finished = objc.Send[bool](pb, sel("hasStrings"))
	case driver.ClipboardReadHasImage:
		ev.Finished = objc.Send[bool](pb, sel("hasImages"))
	case driver.ClipboardReadFiles:
		apple.ForEach(pb.Send(sel("URLs")), func(u objc.ID) {
			if objc.Send[bool](u, sel("isFileURL")) {
				ev.URIs = append(ev.URIs, apple.GoString(u.Send(sel("absoluteString"))))
			}
		})
	case driver.ClipboardReadImage:
		if !objc.Send[bool](pb, sel("hasImages")) {
			break
		}
		var data []byte
		for _, t := range imageTypes {
			if d := pb.Send(sel("dataForPasteboardType:"), apple.NSString(t)); d != 0 {
				data = apple.Bytes(d)
				break
			}
		}
		if data == nil {
			if uiImagePNG == nil {
				lib, err := purego.Dlopen(uikitPath, purego.RTLD_NOW)
				if err == nil {
					purego.RegisterLibFunc(&uiImagePNG, lib, "UIImagePNGRepresentation")
				}
			}
			if img := pb.Send(sel("image")); img != 0 && uiImagePNG != nil {
				data = apple.Bytes(uiImagePNG(img))
			}
		}
		id := it.Count
		go func() {
			ev := driver.Event{Kind: driver.EventClipboard, Count: id}
			ev.Data = driver.ReadClipboardImage(apple.PNG(data))
			s.engine.Dispatch(ev)
		}()
		return
	}
	s.engine.Dispatch(ev)
}

// notify carries out a notification's item through the notification center; the badge needs the permission, and before iOS 16 is the application's. On tvOS, the center supports badges only.
func (s *surface) notify(it *driver.MountItem) {
	switch it.Kind {
	case driver.ItemNotificationPermission, driver.ItemShowNotification, driver.ItemCloseNotification, driver.ItemBadge:
		dispatchIOSNotification(onTV(), it, iosNotificationBackend{
			permission: apple.NotificationPermission,
			show:       apple.ShowNotification,
			close:      apple.CloseNotification,
			badge: func(count uint32) {
				if !apple.SetCenterBadge(count) {
					objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication")).Send(sel("setApplicationIconBadgeNumber:"), int(count))
				}
			},
		}, s.engine.Dispatch)
	case driver.ItemTaskProgress:
		// iOS has no app-icon progress surface.
	case driver.ItemBiometricAvailability, driver.ItemBiometricAuthenticate:
		apple.HandleBiometric(it, s.engine.Dispatch)
	}
}

// openURL opens u in the app that handles it and reports whether one did.
func (s *surface) openURL(u string, id uint32) {
	openSystemURL(u, func(ok bool) { s.engine.Dispatch(driver.Event{Kind: driver.EventOpenURL, Count: id, Finished: ok}) })
}

// openSystemURL opens u in the app that handles it and calls answer with whether one did.
func openSystemURL(u string, answer func(opened bool)) {
	nsURL := objc.ID(apple.Class("NSURL")).Send(sel("URLWithString:"), apple.NSString(u))
	if nsURL == 0 {
		answer(false)
		return
	}
	done := objcblock.New(func(_ objc.Block, ok bool) { answer(ok) })
	app := objc.ID(apple.Class("UIApplication")).Send(sel("sharedApplication"))
	opts := objc.ID(apple.Class("NSDictionary")).Send(sel("dictionary"))
	app.Send(sel("openURL:options:completionHandler:"), nsURL, opts, done)
	done.Release()
}

// openFile shows the apps that open f, under its name, for the user to
// choose one, from the root's center; none shows when no app opens its
// type.
func (s *surface) openFile(f driver.SharedFile, id uint32) {
	u := objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(f.Path))
	if s.opening != 0 {
		s.opening.Send(sel("dismissMenuAnimated:"), false)
		s.opening.Send(selRelease)
	}
	s.opening = objc.ID(apple.Class("UIDocumentInteractionController")).Send(sel("interactionControllerWithURL:"), u)
	s.opening.Send(sel("retain"))
	s.opening.Send(sel("setName:"), apple.NSString(f.Name))
	b := objc.Send[apple.CGRect](s.native, selBounds)
	shown := objc.Send[bool](s.opening, sel("presentOpenInMenuFromRect:inView:animated:"),
		apple.Rect(b.Size.Width/2, b.Size.Height/2, 0, 0), s.native, true)
	s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Finished: shown})
}

// share presents UIActivityViewController for r's files, under their
// names, and text; on iPads it is a popover pointing at r.From.
func (s *surface) share(r driver.ShareRequest, id uint32) {
	items := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	for _, f := range r.Files {
		path, err := f.Stage(filepath.Join(os.TempDir(), "nsgo-share"))
		if err != nil {
			s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: err.Error()})
			return
		}
		items.Send(sel("addObject:"), objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(path)))
	}
	if r.Text != "" {
		items.Send(sel("addObject:"), apple.NSString(r.Text))
	}
	ctrl := objc.ID(apple.Class("UIActivityViewController")).Send(selAlloc).
		Send(sel("initWithActivityItems:applicationActivities:"), items, objc.ID(0))
	defer ctrl.Send(selRelease)
	if pop := ctrl.Send(sel("popoverPresentationController")); pop != 0 {
		from := r.From
		if from == (driver.Bounds{}) {
			b := objc.Send[apple.CGRect](s.native, selBounds)
			from.Origin = driver.Point{X: driver.Points(b.Size.Width / 2), Y: driver.Points(b.Size.Height / 2)}
		}
		pop.Send(sel("setSourceView:"), s.native)
		pop.Send(sel("setSourceRect:"), apple.Rect(float64(from.Origin.X), float64(from.Origin.Y),
			float64(from.Size.Width), float64(from.Size.Height)))
	}
	s.topController().Send(sel("presentViewController:animated:completion:"), ctrl, true, objc.ID(0))
	s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Finished: true})
}

var (
	pickerOnce     sync.Once
	classPicker    objc.Class
	pickerRequests = map[objc.ID]pickRequest{}
)

type pickRequest struct {
	r  driver.PickRequest
	id uint32
}

// pickImages presents PHPickerViewController for the images r asks for;
// picking needs no permission. The picked images load and are prepared
// off the main thread, in the order picked.
func (s *surface) pickImages(r driver.PickRequest, id uint32) {
	if onTV() {
		s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id})
		return // PHPickerViewController is unavailable on tvOS.
	}
	pickerOnce.Do(func() {
		if _, err := purego.Dlopen("/System/Library/Frameworks/PhotosUI.framework/PhotosUI", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
			return
		}
		var protocols []*objc.Protocol
		if p := objc.GetProtocol("PHPickerViewControllerDelegate"); p != nil {
			protocols = append(protocols, p)
		}
		classPicker = register("NsgoPickerDelegate", apple.Class("NSObject"), protocols, []objc.MethodDef{{
			Cmd: sel("picker:didFinishPicking:"),
			Fn: func(self objc.ID, _ objc.SEL, picker, results objc.ID) {
				picker.Send(sel("dismissViewControllerAnimated:completion:"), true, objc.ID(0))
				req, ok := pickerRequests[self]
				if !ok {
					return
				}
				delete(pickerRequests, self)
				defer self.Send(selRelease)
				s.picked(req, results)
			},
		}})
	})
	if classPicker == 0 {
		s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id})
		return
	}
	config := objc.ID(apple.Class("PHPickerConfiguration")).Send(selAlloc).Send(sel("init"))
	defer config.Send(selRelease)
	config.Send(sel("setSelectionLimit:"), max(r.Limit, 1))
	config.Send(sel("setFilter:"), objc.ID(apple.Class("PHPickerFilter")).Send(sel("imagesFilter")))
	if objc.Send[bool](config, sel("respondsToSelector:"), sel("setSelection:")) {
		const ordered = 1
		config.Send(sel("setSelection:"), ordered)
	}
	picker := objc.ID(apple.Class("PHPickerViewController")).Send(selAlloc).Send(sel("initWithConfiguration:"), config)
	defer picker.Send(selRelease)
	delegate := objc.ID(classPicker).Send(selNew)
	pickerRequests[delegate] = pickRequest{r, id}
	picker.Send(sel("setDelegate:"), delegate)
	s.topController().Send(sel("presentViewController:animated:completion:"), picker, true, objc.ID(0))
}

// picked loads the data of the picker's results and answers with them
// prepared as the request asks.
func (s *surface) picked(req pickRequest, results objc.ID) {
	var providers []objc.ID
	apple.ForEach(results, func(res objc.ID) {
		if len(providers) < max(req.r.Limit, 1) {
			providers = append(providers, res.Send(sel("itemProvider")))
		}
	})
	files := make([]driver.PickedFile, len(providers))
	var wg sync.WaitGroup
	for i, p := range providers {
		files[i].Name = apple.GoString(p.Send(sel("suggestedName")))
		wg.Add(1)
		done := objcblock.New(func(_ objc.Block, data, err objc.ID) {
			files[i].Data = apple.Bytes(data)
			wg.Done()
		})
		p.Send(sel("loadDataRepresentationForTypeIdentifier:completionHandler:"), apple.NSString("public.image"), done)
		done.Release()
	}
	go func() {
		wg.Wait()
		var out []driver.PickedFile
		for _, f := range files {
			if len(f.Data) > 0 {
				out = append(out, apple.PreparePicked(req.r, f))
			}
		}
		s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: req.id, Data: driver.FinishPick(req.r, out, nil)})
	}()
}

var (
	documentsOnce    sync.Once
	classDocuments   objc.Class
	documentRequests = map[objc.ID]documentRequest{}
)

// documentRequest is a pick or a save a document picker answers.
type documentRequest struct {
	id    uint32
	limit int
	// folders says the pick is of folders.
	folders bool
	// staged is the directory of the file a save exports, removed once
	// the picker closed; "" for a pick.
	staged string
}

// presentDocuments presents a document picker for req, made by its
// initializer, whose delegate answers it.
func (s *surface) presentDocuments(picker objc.ID, req documentRequest) {
	documentsOnce.Do(func() {
		var protocols []*objc.Protocol
		if p := objc.GetProtocol("UIDocumentPickerDelegate"); p != nil {
			protocols = append(protocols, p)
		}
		answer := func(self objc.ID, urls objc.ID) {
			req, ok := documentRequests[self]
			if !ok {
				return
			}
			delete(documentRequests, self)
			defer self.Send(selRelease)
			ev := driver.Event{Kind: driver.EventFiles, Count: req.id}
			if req.staged != "" {
				os.RemoveAll(req.staged)
				ev.Finished = urls != 0 && objc.Send[int](urls, sel("count")) > 0
			} else if urls != 0 && req.folders {
				ev.Data = pickedFolders(urls, req.limit)
			} else if urls != 0 {
				var files []driver.File
				apple.ForEach(urls, func(u objc.ID) {
					if len(files) < max(req.limit, 1) {
						files = append(files, driver.File{Path: apple.GoString(u.Send(sel("path"))), Temporary: true})
					}
				})
				ev.Data = files
			}
			s.engine.Dispatch(ev)
		}
		classDocuments = register("NsgoDocumentPickerDelegate", apple.Class("NSObject"), protocols, []objc.MethodDef{{
			Cmd: sel("documentPicker:didPickDocumentsAtURLs:"),
			Fn:  func(self objc.ID, _ objc.SEL, _, urls objc.ID) { answer(self, urls) },
		}, {
			Cmd: sel("documentPickerWasCancelled:"),
			Fn:  func(self objc.ID, _ objc.SEL, _ objc.ID) { answer(self, 0) },
		}})
	})
	defer picker.Send(selRelease)
	delegate := objc.ID(classDocuments).Send(selNew)
	documentRequests[delegate] = req
	picker.Send(sel("setDelegate:"), delegate)
	s.topController().Send(sel("presentViewController:animated:completion:"), picker, true, objc.ID(0))
}

// pickFiles presents UIDocumentPickerViewController for the files r
// offers, which copies the ones picked into the app's temporary
// directory; picking needs no permission.
func (s *surface) pickFiles(r driver.FilesRequest, id uint32) {
	picker := objc.ID(apple.Class("UIDocumentPickerViewController")).Send(selAlloc).
		Send(sel("initForOpeningContentTypes:asCopy:"), apple.ContentTypes(r), true)
	picker.Send(sel("setAllowsMultipleSelection:"), r.Limit > 1)
	setDocumentStart(picker, r.Start)
	s.presentDocuments(picker, documentRequest{id: id, limit: r.Limit})
}

// pickDirectories presents UIDocumentPickerViewController for folders,
// which it opens where they are rather than copying them; picking needs
// no permission.
func (s *surface) pickDirectories(r driver.DirectoriesRequest, id uint32) {
	folder := apple.UTType().Send(sel("typeWithIdentifier:"), apple.NSString("public.folder"))
	types := objc.ID(apple.Class("NSArray")).Send(sel("arrayWithObject:"), folder)
	picker := objc.ID(apple.Class("UIDocumentPickerViewController")).Send(selAlloc).
		Send(sel("initForOpeningContentTypes:asCopy:"), types, false)
	picker.Send(sel("setAllowsMultipleSelection:"), r.Limit > 1)
	setDocumentStart(picker, r.Start)
	s.presentDocuments(picker, documentRequest{id: id, limit: r.Limit, folders: true})
}

// setDocumentStart opens a document picker in start when it is a folder on disk, leaving UIKit's own folder otherwise.
func setDocumentStart(picker objc.ID, start string) {
	if start == "" {
		return
	}
	if info, err := os.Stat(start); err != nil || !info.IsDir() {
		return
	}
	url := objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(start))
	if url != 0 {
		picker.Send(sel("setDirectoryURL:"), url)
	}
}

// pickedFolders are the folders a document picker opened, at most limit.
// Each stays in its security scope while the app runs, so the app reads
// it by its path, as on the desktops.
func pickedFolders(urls objc.ID, limit int) []driver.Directory {
	var dirs []driver.Directory
	apple.ForEach(urls, func(u objc.ID) {
		if len(dirs) < max(limit, 1) {
			u.Send(sel("startAccessingSecurityScopedResource"))
			dirs = append(dirs, driver.Directory{Name: apple.GoString(u.Send(sel("lastPathComponent"))), Path: apple.GoString(u.Send(sel("path")))})
		}
	})
	return dirs
}

// saveFile presents UIDocumentPickerViewController exporting a copy of
// the file r saves, under its name.
func (s *surface) saveFile(r driver.SaveRequest, id uint32) {
	if r.Preflight {
		s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoSavePreflight})
		return
	}
	path, err := r.Stage(filepath.Join(os.TempDir(), "nsgo-save"))
	if err != nil {
		s.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: err.Error()})
		return
	}
	urls := objc.ID(apple.Class("NSMutableArray")).Send(sel("array"))
	urls.Send(sel("addObject:"), objc.ID(apple.Class("NSURL")).Send(sel("fileURLWithPath:"), apple.NSString(path)))
	picker := objc.ID(apple.Class("UIDocumentPickerViewController")).Send(selAlloc).
		Send(sel("initForExportingURLs:asCopy:"), urls, true)
	setDocumentStart(picker, r.Start)
	s.presentDocuments(picker, documentRequest{id: id, staged: filepath.Dir(path)})
}

// setAppearance shows a standalone app's windows, one a scene, and the system views in them, in a, or in the system's appearance: the app forces one for all its scenes.
func setAppearance(a driver.Appearance) {
	for _, s := range scenes {
		s.appearance = a
		if s.window != 0 {
			s.window.Send(sel("setOverrideUserInterfaceStyle:"), uiStyle(a))
		}
	}
}

// uiStyle is UIKit's interface style of a, unspecified for the system's.
func uiStyle(a driver.Appearance) int {
	switch a {
	case driver.AppearanceLight:
		return 1
	case driver.AppearanceDark:
		return 2
	}
	return 0
}

// osVersion returns iOS's version.
func osVersion() driver.OSVersion { return apple.OSVersion() }

// onMac reports whether the app is an iPhone or iPad app running on a
// Mac.
func onMac() bool {
	info := objc.ID(apple.Class("NSProcessInfo")).Send(sel("processInfo"))
	return objc.Send[bool](info, sel("respondsToSelector:"), sel("isiOSAppOnMac")) && objc.Send[bool](info, sel("isiOSAppOnMac"))
}
