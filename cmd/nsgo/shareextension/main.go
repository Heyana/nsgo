//go:build ios

// The iOS share extension is a Go executable with a runtime-registered UIKit principal class. It calls only extension-safe public APIs, through purego.
package shareextension

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/platform/shareinbox"
)

var sel = objc.RegisterName
var ns = shareinbox.String
var text = shareinbox.GoString
var loadTypes = sync.OnceFunc(func() {
	if _, err := purego.Dlopen("/System/Library/Frameworks/UniformTypeIdentifiers.framework/UniformTypeIdentifiers", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
})

// Run starts the extension host after the generated entry point locks the main thread.
func Run() {
	lib, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW|purego.RTLD_GLOBAL)
	if err != nil {
		panic(err)
	}
	if _, err = purego.Dlopen("/System/Library/Frameworks/UIKit.framework/UIKit", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		panic(err)
	}
	loadTypes()
	_, err = objc.RegisterClass("NSGoShareController", objc.GetClass("UIViewController"), []*objc.Protocol{}, []objc.FieldDef{}, []objc.MethodDef{
		{Cmd: sel("viewDidLoad"), Fn: loaded},
	})
	if err != nil {
		panic(err)
	}
	var extensionMain func(int, unsafe.Pointer) int
	purego.RegisterLibFunc(&extensionMain, lib, "NSExtensionMain")
	argv := make([]*byte, 0, len(os.Args)+1)
	for _, arg := range os.Args {
		b := append([]byte(arg), 0)
		argv = append(argv, &b[0])
	}
	argv = append(argv, nil)
	code := extensionMain(len(os.Args), unsafe.Pointer(&argv[0]))
	runtime.KeepAlive(argv)
	os.Exit(code)
}

func loaded(self objc.ID, _ objc.SEL) {
	objc.SendSuper[objc.ID](self, sel("viewDidLoad"))
	view := self.Send(sel("view"))
	view.Send(sel("setBackgroundColor:"), objc.ID(objc.GetClass("UIColor")).Send(sel("systemBackgroundColor")))
	spinner := objc.ID(objc.GetClass("UIActivityIndicatorView")).Send(sel("alloc")).Send(sel("initWithActivityIndicatorStyle:"), int(100))
	spinner.Send(sel("setTranslatesAutoresizingMaskIntoConstraints:"), false)
	view.Send(sel("addSubview:"), spinner)
	for _, anchor := range []string{"centerXAnchor", "centerYAnchor"} {
		spinner.Send(sel(anchor)).Send(sel("constraintEqualToAnchor:"), view.Send(sel(anchor))).Send(sel("setActive:"), true)
	}
	spinner.Send(sel("startAnimating"))
	spinner.Send(sel("release"))
	context := self.Send(sel("extensionContext"))
	context.Send(sel("retain"))
	receive(context)
}

func receive(context objc.ID) {
	receiveInto(context, shareinbox.GroupPath(), shareinbox.Info(shareinbox.TypesKey))
}

// receiveInto separates provider import from discovering the containing app's shared container. The same importer can be tested with isolated storage.
func receiveInto(context objc.ID, group string, accepted objc.ID) {
	loadTypes()
	if group == "" {
		finish(context, errors.New("no app group"))
		return
	}
	batch, err := shareinbox.Begin(group)
	if err != nil {
		finish(context, err)
		return
	}
	items := context.Send(sel("inputItems"))
	type request struct {
		provider objc.ID
		uti      string
		file     bool
	}
	var requests []request
	for i := uint(0); i < objc.Send[uint](items, sel("count")); i++ {
		attachments := items.Send(sel("objectAtIndex:"), i).Send(sel("attachments"))
		for j := uint(0); j < objc.Send[uint](attachments, sel("count")); j++ {
			provider := attachments.Send(sel("objectAtIndex:"), j)
			for k := uint(0); k < objc.Send[uint](accepted, sel("count")); k++ {
				uti := accepted.Send(sel("objectAtIndex:"), k)
				if objc.Send[bool](provider, sel("hasItemConformingToTypeIdentifier:"), uti) {
					provider.Send(sel("retain"))
					requests = append(requests, request{provider: provider, uti: representationType(provider, uti), file: !inlineType(uti) || objc.Send[int](provider, sel("preferredPresentationStyle")) == 2})
					break
				}
			}
		}
	}
	// Load sequentially so provider callbacks preserve the sender's item order.
	var load func(int)
	load = func(index int) {
		if index == len(requests) {
			err := batch.Commit()
			batch.Abort()
			if err == nil {
				shareinbox.Notify()
			}
			finish(context, err)
			return
		}
		request := requests[index]
		var once sync.Once
		callback := objcblock.New(func(_ objc.Block, value, failure objc.ID) {
			once.Do(func() {
				var err error
				if failure != 0 || value == 0 {
					err = fmt.Errorf("provider could not load %s: %s", request.uti, text(failure.Send(sel("description"))))
				} else {
					err = copyItem(batch, value, request.uti)
				}
				request.provider.Send(sel("release"))
				if err != nil {
					for _, remaining := range requests[index+1:] {
						remaining.provider.Send(sel("release"))
					}
					batch.Abort()
					finish(context, err)
				} else {
					load(index + 1)
				}
			})
		})
		uti := objc.ID(objc.GetClass("UTType")).Send(sel("typeWithIdentifier:"), ns(request.uti))
		url := objc.ID(objc.GetClass("UTType")).Send(sel("typeWithIdentifier:"), ns("public.url"))
		isURL := objc.Send[bool](uti, sel("conformsToType:"), url)
		// A file-URL representation describes the source file. Decode the NSURL itself, rather than writing its URL descriptor to a file.
		if request.file && !isURL {
			request.provider.Send(sel("loadFileRepresentationForTypeIdentifier:completionHandler:"), ns(request.uti), callback)
		} else {
			class := objc.GetClass("NSString")
			if isURL {
				class = objc.GetClass("NSURL")
			}
			// Ask for the public object class when available. loadItem's generic NSData form may be an archive of a URL, rather than its UTF-8 bytes; loadObject performs the provider's decoding.
			if objc.Send[bool](request.provider, sel("canLoadObjectOfClass:"), class) {
				request.provider.Send(sel("loadObjectOfClass:completionHandler:"), class, callback)
			} else {
				request.provider.Send(sel("loadItemForTypeIdentifier:options:completionHandler:"), ns(request.uti), objc.ID(0), callback)
			}
		}
		callback.Release()
	}
	load(0)
}

func inlineType(identifier objc.ID) bool {
	class := objc.ID(objc.GetClass("UTType"))
	uti := class.Send(sel("typeWithIdentifier:"), identifier)
	for _, kind := range []string{"public.url", "public.text"} {
		if objc.Send[bool](uti, sel("conformsToType:"), class.Send(sel("typeWithIdentifier:"), ns(kind))) {
			return true
		}
	}
	return false
}

// Prefer a registered concrete representation over a broad accepted UTI so file providers retain their real media type and preferred extension.
func representationType(provider objc.ID, accepted objc.ID) string {
	class := objc.ID(objc.GetClass("UTType"))
	wanted := class.Send(sel("typeWithIdentifier:"), accepted)
	registered := provider.Send(sel("registeredTypeIdentifiers"))
	for i := uint(0); i < objc.Send[uint](registered, sel("count")); i++ {
		identifier := registered.Send(sel("objectAtIndex:"), i)
		candidate := class.Send(sel("typeWithIdentifier:"), identifier)
		if objc.Send[bool](candidate, sel("conformsToType:"), wanted) {
			return text(identifier)
		}
	}
	return text(accepted)
}

func copyItem(batch *shareinbox.Batch, value objc.ID, uti string) error {
	typeInfo := objc.ID(objc.GetClass("UTType")).Send(sel("typeWithIdentifier:"), ns(uti))
	mime := text(typeInfo.Send(sel("preferredMIMEType")))
	if objc.Send[bool](value, sel("isKindOfClass:"), objc.GetClass("NSData")) {
		if objc.Send[uint](value, sel("length")) > shareinbox.MaxTextBytes {
			return errors.New("shared text is too large")
		}
		isText := objc.Send[bool](typeInfo, sel("conformsToType:"), objc.ID(objc.GetClass("UTType")).Send(sel("typeWithIdentifier:"), ns("public.text")))
		isURL := objc.Send[bool](typeInfo, sel("conformsToType:"), objc.ID(objc.GetClass("UTType")).Send(sel("typeWithIdentifier:"), ns("public.url")))
		if !isText && !isURL {
			return errors.New("unsupported shared data representation")
		}
		decoded := objc.ID(objc.GetClass("NSString")).Send(sel("alloc")).Send(sel("initWithData:encoding:"), value, uint(4))
		if decoded == 0 {
			return errors.New("shared text is not UTF-8")
		}
		defer decoded.Send(sel("release"))
		return storeText(batch, decoded, mime)
	}
	if objc.Send[bool](value, sel("isKindOfClass:"), objc.GetClass("NSString")) {
		if mime == "" {
			mime = "text/plain"
		}
		return storeText(batch, value, mime)
	}
	if objc.Send[bool](value, sel("isKindOfClass:"), objc.GetClass("NSURL")) {
		if !objc.Send[bool](value, sel("isFileURL")) {
			return storeText(batch, value.Send(sel("absoluteString")), "text/plain")
		}
		scoped := objc.Send[bool](value, sel("startAccessingSecurityScopedResource"))
		if scoped {
			defer value.Send(sel("stopAccessingSecurityScopedResource"))
		}
		path := text(value.Send(sel("path")))
		// Unscoped provider representations must be temporary copies, never a sender-chosen URL into the extension's App Group or private storage.
		temporary, tmpErr := filepath.EvalSymlinks(os.TempDir())
		resolved, pathErr := filepath.EvalSymlinks(path)
		rel, err := filepath.Rel(temporary, resolved)
		if tmpErr != nil || pathErr != nil {
			return errors.New("shared file is unavailable")
		}
		if !scoped && (err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || filepath.IsAbs(rel)) {
			return errors.New("shared file has no read grant")
		}
		if ext := strings.TrimPrefix(filepath.Ext(path), "."); ext != "" {
			actual := objc.ID(objc.GetClass("UTType")).Send(sel("typeWithFilenameExtension:"), ns(ext))
			if preferred := text(actual.Send(sel("preferredMIMEType"))); preferred != "" {
				mime = preferred
			}
		}
		if mime == "" {
			mime = "application/octet-stream"
		}
		return batch.File(path, filepath.Base(path), mime)
	}
	return errors.New("unsupported shared item representation")
}

func storeText(batch *shareinbox.Batch, value objc.ID, mime string) error {
	if objc.Send[uint](value, sel("lengthOfBytesUsingEncoding:"), uint(4)) > shareinbox.MaxTextBytes {
		return errors.New("shared text is too large")
	}
	return batch.Text(text(value), mime)
}

func finish(context objc.ID, err error) {
	// UIKit completion belongs on its main operation queue, including callbacks from providers that perform their work on a background queue.
	block := objcblock.New(func(objc.Block) {
		if err == nil {
			context.Send(sel("completeRequestReturningItems:completionHandler:"), objc.ID(0), objc.ID(0))
		} else {
			info := objc.ID(objc.GetClass("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"), ns(err.Error()), ns("NSDebugDescription"))
			failure := objc.ID(objc.GetClass("NSError")).Send(sel("errorWithDomain:code:userInfo:"), ns("NSCocoaErrorDomain"), int(256), info)
			context.Send(sel("cancelRequestWithError:"), failure)
		}
		context.Send(sel("release"))
	})
	objc.ID(objc.GetClass("NSOperationQueue")).Send(sel("mainQueue")).Send(sel("addOperationWithBlock:"), block)
	block.Release()
}
