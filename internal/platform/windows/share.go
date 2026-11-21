//go:build windows

package windows

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Sharing through WinRT's DataTransferManager: IDataTransferManagerInterop::GetForWindow
// binds the window's manager, DataRequested populates the DataPackage's title, text,
// optional web link and StorageFile items (from disk paths or staged in-memory bytes),
// and ShowShareUIForWindow opens Windows' share sheet.

var (
	iidDataTransferManagerInterop = parseGUID("3a3dcd6c-3eab-43dc-bcde-45671ce800c8")
	iidDataTransferManager        = parseGUID("a5caee9b-8708-49d1-8d36-67d25a8da00c")
	iidDataPackage2               = parseGUID("041c1fe9-2409-45e1-a538-4c53eeee04a7")
	iidUriRuntimeClassFactory     = parseGUID("44a9796f-723e-4fdf-a218-033e75b0c084")
	iidStorageFileStatics         = parseGUID("5984c710-daf2-43c8-8bb4-a4d3eacfd03f")
	iidStorageItem                = parseGUID("4207a996-ca2f-42f7-bde8-8b10457a7f30")
	iidInspectable                = parseGUID("af86e2e0-b12d-4c6a-9c5a-d7aa65101e90")
	// Parameterized WinRT interface IDs derived from their signatures (namespace
	// 11f47ad5-7b73-42c0-abae-878b1e16adee, SHA-1):
	//   TypedEventHandler<DataTransferManager, DataRequestedEventArgs>
	//   AsyncOperationCompletedHandler<StorageFile>
	//   IIterable<IStorageItem>
	//   IIterator<IStorageItem>
	iidDataRequested        = parseGUID("ec6f9cc8-46d0-5e0e-b4d2-7d7773ae37a0")
	iidStorageFileCompleted = parseGUID("e521c894-2c26-5946-9e61-2b5e188d01ed")
	iidStorageItemIterable  = parseGUID("bb8b8418-65d1-544b-b083-6d172f568c73")
	iidStorageItemIterator  = parseGUID("05b487c2-3830-5d3c-98da-25fa11542dbd")
)

// Vtable slots for the WinRT sharing interfaces. IDataTransferManagerInterop
// inherits IUnknown (slots 0..2); the WinRT interfaces inherit IInspectable (0..5).
const (
	dtmInteropGetForWindow             = 3
	dtmInteropShowShareUIForWindow     = 4
	dtmAddDataRequested                = 6
	dtmRemoveDataRequested             = 7
	dataRequestedGetRequest            = 6
	dataRequestGetData                 = 6
	dataPackageGetProperties           = 7
	dataPackageSetText                 = 16
	dataPackageSetStorageItemsReadOnly = 22
	dataPackage2SetWebLink             = 7
	dataPackagePropsPutTitle           = 7
	uriFactoryCreateUri                = 6
	storageFileGetFileFromPathAsync    = 6

	dataTransferManagerClass = "Windows.ApplicationModel.DataTransfer.DataTransferManager"
	storageFileClass         = "Windows.Storage.StorageFile"
	winRTUriClass            = "Windows.Foundation.Uri"

	eBounds            = 0x8000000b
	classENotAvailable = 0x80040111
	regdbEClassNotReg  = 0x80040154
	hrNotSupported     = 0x80070032
	hrNotFound         = 0x80070490
)

type shareState struct {
	mu         sync.Mutex
	manager    *comObject
	owner      uintptr
	token      eventToken
	registered bool
	closed     bool
	pending    *sharePayload
}

type sharePayload struct {
	title   string
	text    string
	webLink string
	items   []*comObject
}

func (p *sharePayload) release() {
	if p == nil {
		return
	}
	for _, it := range p.items {
		it.release()
	}
	p.items = nil
}

// share carries out ItemShare through Windows' DataTransferManager share UI.
func (w *window) share(r driver.ShareRequest, id uint32) {
	comApartment()
	owner := w.owner()
	if owner == 0 {
		w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		return
	}
	interop := activationFactory(dataTransferManagerClass, &iidDataTransferManagerInterop)
	if interop == nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		return
	}
	interop.release()

	title := windowsShareTitle(w.opts.Title, r)
	webLink := shareWebLink(r.Text)
	if len(r.Files) == 0 {
		w.showSharePayload(owner, &sharePayload{title: title, text: r.Text, webLink: webLink}, id)
		return
	}
	files := append([]driver.SharedFile(nil), r.Files...)
	text := r.Text
	go func() {
		paths, err := stageWindowsShareFiles(files)
		w.post(func() {
			if w.shareClosed() {
				return
			}
			if err != nil {
				w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: err.Error()})
				return
			}
			w.loadShareStorageItems(paths, func(items []*comObject, loadErr error) {
				if w.shareClosed() {
					for _, it := range items {
						it.release()
					}
					return
				}
				if loadErr != nil {
					for _, it := range items {
						it.release()
					}
					w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: loadErr.Error()})
					return
				}
				w.showSharePayload(owner, &sharePayload{title: title, text: text, webLink: webLink, items: items}, id)
			})
		})
	}()
}

func stageWindowsShareFiles(files []driver.SharedFile) ([]string, error) {
	stageDir := filepath.Join(os.TempDir(), "nsgo-share")
	paths := make([]string, 0, len(files))
	for _, f := range files {
		p, err := f.Stage(stageDir)
		if err != nil {
			return nil, err
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return nil, err
		}
		paths = append(paths, filepath.Clean(abs))
	}
	return paths, nil
}

func (w *window) shareClosed() bool {
	return w.sharing != nil && w.sharing.closed
}

// loadShareStorageItems resolves each disk path to a WinRT IStorageItem via
// StorageFile.GetFileFromPathAsync and delivers them on the UI thread in order.
func (w *window) loadShareStorageItems(paths []string, done func([]*comObject, error)) {
	statics := activationFactory(storageFileClass, &iidStorageFileStatics)
	if statics == nil {
		done(nil, errors.New(driver.NoShareSheet))
		return
	}
	defer statics.release()

	items := make([]*comObject, len(paths))
	remaining := len(paths)
	var firstErr error
	finish := func(i int, item *comObject, err error) {
		if err != nil && firstErr == nil {
			firstErr = err
		}
		items[i] = item
		remaining--
		if remaining == 0 {
			done(items, firstErr)
		}
	}

	for i, p := range paths {
		idx := i
		hp := hstring(p)
		var op *comObject
		hr := statics.call(storageFileGetFileFromPathAsync, hp, uintptr(unsafe.Pointer(&op)))
		deleteHString(hp)
		if failed(hr) || op == nil {
			if !failed(hr) {
				finish(idx, nil, errors.New("nsgo: share: GetFileFromPathAsync returned no operation"))
			} else {
				finish(idx, nil, hresultError("nsgo: share: GetFileFromPathAsync", hr))
			}
			continue
		}
		completed := newHandler(&iidStorageFileCompleted, func(_ uintptr, status unsafe.Pointer) {
			var item *comObject
			var err error
			if uintptr(status) != asyncStatusCompleted {
				err = errors.New("nsgo: share: StorageFile load did not complete")
			} else {
				var file *comObject
				if rhr := op.call(asyncGetResults, uintptr(unsafe.Pointer(&file))); failed(rhr) || file == nil {
					if !failed(rhr) {
						err = errors.New("nsgo: share: StorageFile result was empty")
					} else {
						err = hresultError("nsgo: share: StorageFile GetResults", rhr)
					}
				} else {
					item = file.query(&iidStorageItem)
					file.release()
					if item == nil {
						err = errors.New("nsgo: share: StorageFile lacked IStorageItem")
					}
				}
			}
			op.release()
			w.post(func() { finish(idx, item, err) })
		})
		completed.agile = true
		hr = op.call(asyncPutCompleted, completed.ptr())
		completed.release()
		if failed(hr) {
			op.release()
			finish(idx, nil, hresultError("nsgo: share: StorageFile put_Completed", hr))
		}
	}
}

func (w *window) showSharePayload(owner uintptr, payload *sharePayload, id uint32) {
	interop := activationFactory(dataTransferManagerClass, &iidDataTransferManagerInterop)
	if interop == nil {
		payload.release()
		w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet})
		return
	}
	defer interop.release()

	if w.sharing == nil {
		w.sharing = &shareState{}
	}
	if w.sharing.closed {
		payload.release()
		return
	}
	if w.sharing.manager == nil || w.sharing.owner != owner {
		w.detachShareManager()
		var manager *comObject
		hr := interop.call(dtmInteropGetForWindow, owner, uintptr(unsafe.Pointer(&iidDataTransferManager)), uintptr(unsafe.Pointer(&manager)))
		if failed(hr) || manager == nil {
			payload.release()
			w.engine.Dispatch(windowsShareEvent(id, "GetForWindow", hr))
			return
		}
		h := newHandler(&iidDataRequested, w.onDataRequested)
		h.agile = true
		var token eventToken
		hr = manager.call(dtmAddDataRequested, h.ptr(), uintptr(unsafe.Pointer(&token)))
		h.release()
		if failed(hr) {
			manager.release()
			payload.release()
			w.engine.Dispatch(windowsShareEvent(id, "add_DataRequested", hr))
			return
		}
		w.sharing.manager = manager
		w.sharing.owner = owner
		w.sharing.token = token
		w.sharing.registered = true
	}

	w.sharing.mu.Lock()
	old := w.sharing.pending
	w.sharing.pending = payload
	w.sharing.mu.Unlock()
	old.release()

	hr := interop.call(dtmInteropShowShareUIForWindow, owner)
	if failed(hr) {
		w.engine.Dispatch(windowsShareEvent(id, "ShowShareUIForWindow", hr))
		return
	}
	w.engine.Dispatch(driver.Event{Kind: driver.EventFiles, Count: id, Finished: true})
}

func (w *window) onDataRequested(_ uintptr, args unsafe.Pointer) {
	if args == nil || w.sharing == nil {
		return
	}
	w.sharing.mu.Lock()
	p := w.sharing.pending
	if p == nil {
		w.sharing.mu.Unlock()
		return
	}
	title, text, webLink := p.title, p.text, p.webLink
	var items []*comObject
	if len(p.items) > 0 {
		items = make([]*comObject, len(p.items))
		for i, it := range p.items {
			it.addRef()
			items[i] = it
		}
	}
	w.sharing.mu.Unlock()
	defer func() {
		for _, it := range items {
			it.release()
		}
	}()

	reqArgs := (*comObject)(args)
	var request *comObject
	if failed(reqArgs.call(dataRequestedGetRequest, uintptr(unsafe.Pointer(&request)))) || request == nil {
		return
	}
	defer request.release()

	var pkg *comObject
	if failed(request.call(dataRequestGetData, uintptr(unsafe.Pointer(&pkg)))) || pkg == nil {
		return
	}
	defer pkg.release()

	populateDataPackage(pkg, title, text, webLink, items)
}

func populateDataPackage(pkg *comObject, title, text, webLink string, items []*comObject) {
	var props *comObject
	if !failed(pkg.call(dataPackageGetProperties, uintptr(unsafe.Pointer(&props)))) && props != nil {
		ht := hstring(title)
		props.call(dataPackagePropsPutTitle, ht)
		deleteHString(ht)
		props.release()
	}
	if text != "" {
		ht := hstring(text)
		pkg.call(dataPackageSetText, ht)
		deleteHString(ht)
	}
	if webLink != "" {
		if pkg2 := pkg.query(&iidDataPackage2); pkg2 != nil {
			if uriFactory := activationFactory(winRTUriClass, &iidUriRuntimeClassFactory); uriFactory != nil {
				hu := hstring(webLink)
				var uri *comObject
				if !failed(uriFactory.call(uriFactoryCreateUri, hu, uintptr(unsafe.Pointer(&uri)))) && uri != nil {
					pkg2.call(dataPackage2SetWebLink, uri.ptr())
					uri.release()
				}
				deleteHString(hu)
				uriFactory.release()
			}
			pkg2.release()
		}
	}
	if len(items) > 0 {
		iterable := newStorageItemIterable(items)
		pkg.call(dataPackageSetStorageItemsReadOnly, iterable.ptr())
		iterable.release()
	}
}

func (w *window) detachShareManager() {
	if w.sharing == nil || w.sharing.manager == nil {
		return
	}
	if w.sharing.registered {
		w.sharing.manager.call(dtmRemoveDataRequested, u64Args(uint64(w.sharing.token.value))...)
		w.sharing.registered = false
	}
	w.sharing.manager.release()
	w.sharing.manager = nil
	w.sharing.owner = 0
}

func (w *window) closeShare() {
	if w.sharing == nil {
		return
	}
	w.sharing.closed = true
	w.detachShareManager()
	w.sharing.mu.Lock()
	old := w.sharing.pending
	w.sharing.pending = nil
	w.sharing.mu.Unlock()
	old.release()
}

// windowsShareTitle returns the non-empty DataPackage title required by Windows' share UI.
func windowsShareTitle(windowTitle string, r driver.ShareRequest) string {
	if t := strings.TrimSpace(windowTitle); t != "" {
		return t
	}
	if len(r.Files) > 0 && strings.TrimSpace(r.Files[0].Name) != "" {
		return strings.TrimSpace(r.Files[0].Name)
	}
	return "Share"
}

// shareWebLink returns s trimmed when it is a single http or https URL with a host.
func shareWebLink(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.ContainsAny(s, " \t\r\n") {
		return ""
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	if !strings.EqualFold(u.Scheme, "http") && !strings.EqualFold(u.Scheme, "https") {
		return ""
	}
	return s
}

func isNoShareSheetHR(hr uintptr) bool {
	switch uint32(hr) {
	case eNotImpl, eNoInterface, classENotAvailable, regdbEClassNotReg, hrNotSupported, hrNotFound:
		return true
	}
	return false
}

func windowsShareEvent(id uint32, what string, hr uintptr) driver.Event {
	if hr == 0 || isNoShareSheetHR(hr) {
		return driver.Event{Kind: driver.EventFiles, Count: id, Text: driver.NoShareSheet}
	}
	return driver.Event{Kind: driver.EventFiles, Count: id, Text: hresultError("nsgo: share: "+what, hr).Error()}
}

// storageItemIterable implements WinRT's IIterable<IStorageItem> in Go so
// IDataPackage::SetStorageItemsReadOnly can read the staged StorageFiles
// without a C++/WinRT vector.
type storageItemIterable struct {
	vtbl  *[7]uintptr
	refs  int32
	items []*comObject
}

// storageItemIterator implements WinRT's IIterator<IStorageItem>.
type storageItemIterator struct {
	vtbl  *[10]uintptr
	refs  int32
	items []*comObject
	index int
}

var (
	storageIterableVtbl = [7]uintptr{
		syscall.NewCallback(storageIterableQueryInterface),
		syscall.NewCallback(storageIterableAddRef),
		syscall.NewCallback(storageIterableRelease),
		syscall.NewCallback(inspectableGetIids),
		syscall.NewCallback(inspectableGetRuntimeClassName),
		syscall.NewCallback(inspectableGetTrustLevel),
		syscall.NewCallback(storageIterableFirst),
	}
	storageIteratorVtbl = [10]uintptr{
		syscall.NewCallback(storageIteratorQueryInterface),
		syscall.NewCallback(storageIteratorAddRef),
		syscall.NewCallback(storageIteratorRelease),
		syscall.NewCallback(inspectableGetIids),
		syscall.NewCallback(inspectableGetRuntimeClassName),
		syscall.NewCallback(inspectableGetTrustLevel),
		syscall.NewCallback(storageIteratorCurrent),
		syscall.NewCallback(storageIteratorHasCurrent),
		syscall.NewCallback(storageIteratorMoveNext),
		syscall.NewCallback(storageIteratorGetMany),
	}
	storageCollectionsMu sync.Mutex
	storageIterables     = map[*storageItemIterable]struct{}{}
	storageIterators     = map[*storageItemIterator]struct{}{}
)

func newStorageItemIterable(items []*comObject) *storageItemIterable {
	copied := make([]*comObject, len(items))
	for i, it := range items {
		it.addRef()
		copied[i] = it
	}
	s := &storageItemIterable{vtbl: &storageIterableVtbl, refs: 1, items: copied}
	storageCollectionsMu.Lock()
	storageIterables[s] = struct{}{}
	storageCollectionsMu.Unlock()
	return s
}

func (s *storageItemIterable) ptr() uintptr { return uintptr(unsafe.Pointer(s)) }
func (s *storageItemIterable) release()     { storageIterableRelease(s) }

func storageIterableQueryInterface(s *storageItemIterable, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == iidInspectable || *iid == iidStorageItemIterable || *iid == iidAgileObject {
		*out = unsafe.Pointer(s)
		storageIterableAddRef(s)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func storageIterableAddRef(s *storageItemIterable) uintptr {
	storageCollectionsMu.Lock()
	defer storageCollectionsMu.Unlock()
	s.refs++
	return uintptr(s.refs)
}

func storageIterableRelease(s *storageItemIterable) uintptr {
	storageCollectionsMu.Lock()
	s.refs--
	refs := s.refs
	var toRelease []*comObject
	if refs == 0 {
		delete(storageIterables, s)
		toRelease = s.items
		s.items = nil
	}
	storageCollectionsMu.Unlock()
	for _, it := range toRelease {
		it.release()
	}
	return uintptr(refs)
}

func storageIterableFirst(s *storageItemIterable, out *unsafe.Pointer) uintptr {
	storageCollectionsMu.Lock()
	copied := make([]*comObject, len(s.items))
	for i, it := range s.items {
		it.addRef()
		copied[i] = it
	}
	it := &storageItemIterator{vtbl: &storageIteratorVtbl, refs: 1, items: copied}
	storageIterators[it] = struct{}{}
	storageCollectionsMu.Unlock()
	*out = unsafe.Pointer(it)
	return sOK
}

func storageIteratorQueryInterface(it *storageItemIterator, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == iidInspectable || *iid == iidStorageItemIterator || *iid == iidAgileObject {
		*out = unsafe.Pointer(it)
		storageIteratorAddRef(it)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func storageIteratorAddRef(it *storageItemIterator) uintptr {
	storageCollectionsMu.Lock()
	defer storageCollectionsMu.Unlock()
	it.refs++
	return uintptr(it.refs)
}

func storageIteratorRelease(it *storageItemIterator) uintptr {
	storageCollectionsMu.Lock()
	it.refs--
	refs := it.refs
	var toRelease []*comObject
	if refs == 0 {
		delete(storageIterators, it)
		toRelease = it.items
		it.items = nil
	}
	storageCollectionsMu.Unlock()
	for _, item := range toRelease {
		item.release()
	}
	return uintptr(refs)
}

func storageIteratorCurrent(it *storageItemIterator, out *unsafe.Pointer) uintptr {
	storageCollectionsMu.Lock()
	if it.index >= len(it.items) {
		storageCollectionsMu.Unlock()
		*out = nil
		return eBounds
	}
	item := it.items[it.index]
	item.addRef()
	storageCollectionsMu.Unlock()
	*out = unsafe.Pointer(item)
	return sOK
}

func storageIteratorHasCurrent(it *storageItemIterator, out *byte) uintptr {
	storageCollectionsMu.Lock()
	has := it.index < len(it.items)
	storageCollectionsMu.Unlock()
	if has {
		*out = 1
	} else {
		*out = 0
	}
	return sOK
}

func storageIteratorMoveNext(it *storageItemIterator, out *byte) uintptr {
	storageCollectionsMu.Lock()
	if it.index < len(it.items) {
		it.index++
	}
	has := it.index < len(it.items)
	storageCollectionsMu.Unlock()
	if has {
		*out = 1
	} else {
		*out = 0
	}
	return sOK
}

func storageIteratorGetMany(it *storageItemIterator, capacity uint32, dst unsafe.Pointer, actual *uint32) uintptr {
	storageCollectionsMu.Lock()
	avail := len(it.items) - it.index
	n := min(int(capacity), avail)
	if n > 0 && dst != nil {
		out := unsafe.Slice((*uintptr)(dst), n)
		for i := range n {
			item := it.items[it.index+i]
			item.addRef()
			out[i] = item.ptr()
		}
		it.index += n
	} else {
		n = 0
	}
	storageCollectionsMu.Unlock()
	if actual != nil {
		*actual = uint32(n)
	}
	return sOK
}

func inspectableGetIids(_ uintptr, count *uint32, iids *unsafe.Pointer) uintptr {
	if count != nil {
		*count = 0
	}
	if iids != nil {
		*iids = nil
	}
	return sOK
}

func inspectableGetRuntimeClassName(_ uintptr, name *uintptr) uintptr {
	if name != nil {
		*name = 0
	}
	return eNotImpl
}

func inspectableGetTrustLevel(_ uintptr, level *int32) uintptr {
	if level != nil {
		*level = 0
	}
	return sOK
}
