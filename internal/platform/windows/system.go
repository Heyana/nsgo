//go:build windows

package windows

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/image/bmp"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/imagedecode"
	"github.com/nsgo-dev/nsgo/internal/platform/picked"
)

// The services Win32 performs: the clipboard's images and files, opening
// URLs, the photo and file pickers, the save dialog, context menus, the app's appearance, the
// window's frame and level, and the menu bar that hides.

var (
	comdlg32             = syscall.NewLazyDLL("comdlg32.dll")
	uxtheme              = syscall.NewLazyDLL("uxtheme.dll")
	getOpenFileNameW     = comdlg32.NewProc("GetOpenFileNameW")
	commDlgExtendedError = comdlg32.NewProc("CommDlgExtendedError")
	shellExecuteExW      = shell32.NewProc("ShellExecuteExW")
	isIconic             = user32.NewProc("IsIconic")
	setForegroundWindow  = user32.NewProc("SetForegroundWindow")
	bringWindowToTop     = user32.NewProc("BringWindowToTop")
	getWindowRect        = user32.NewProc("GetWindowRect")
	endMenu              = user32.NewProc("EndMenu")
)

const (
	swShowNormal = 1

	hwndBottom    = 1
	hwndTopmost   = ^uintptr(0) // -1
	hwndNoTopmost = ^uintptr(1) // -2

	ofnAllowMultiSelect = 0x00000200
	ofnPathMustExist    = 0x00000800
	ofnFileMustExist    = 0x00001000
	ofnExplorer         = 0x00080000
	ofnNoChangeDir      = 0x00000008

	wmDialog       = 0x8000 + 5 // an open or save dialog waits
	wmPopup        = 0x8000 + 6 // a context menu waits
	wmFolderChange = 0x8000 + 8 // the folder dialog opened another folder
	wmSysKeyUp     = 0x0105
	wmSysChar      = 0x0106

	wmEnterMenuLoop = 0x0211
	wmExitMenuLoop  = 0x0212
	vkF10           = 0x79
)

// pngFormat is the clipboard format PNG-aware apps share images in.
func pngFormat() uintptr {
	name, _ := syscall.UTF16PtrFromString("PNG")
	f, _, _ := registerClipboardFormatW.Call(uintptr(unsafe.Pointer(name)))
	return f
}

func formatAvailable(f uintptr) bool {
	r, _, _ := isClipboardFormatAvailable.Call(f)
	return f != 0 && r != 0
}

// readClipboard answers an ItemReadClipboard.
func (w *window) readClipboard(it *driver.MountItem) {
	ev := driver.Event{Kind: driver.EventClipboard, Count: it.Count}
	switch it.Clipboard {
	case driver.ClipboardReadText:
		ev.Text = w.clipboard()
	case driver.ClipboardReadHasText:
		ev.Finished = formatAvailable(cfUnicodeText)
	case driver.ClipboardReadHasImage:
		ev.Finished = !formatAvailable(cfHDrop) && (formatAvailable(pngFormat()) || formatAvailable(cfDIB))
	case driver.ClipboardReadFiles, driver.ClipboardReadImage:
		if r, _, _ := openClipboard.Call(w.hwnd); r == 0 {
			break
		}
		h, _, _ := getClipboardData.Call(cfHDrop)
		if h != 0 {
			ev.URIs = hdropFiles(h)
		}
		var files []driver.PastedFile
		if it.Clipboard == driver.ClipboardReadImage && len(ev.URIs) == 0 {
			files = clipboardImage()
		}
		closeClipboard.Call()
		if it.Clipboard == driver.ClipboardReadImage {
			ev.URIs = nil
			if len(files) > 0 {
				// Converting a large bitmap takes a while: off the thread.
				data := files[0].Data
				go func() {
					ev.Data = pngOf(data)
					w.engine.Dispatch(ev)
				}()
				return
			}
		}
	}
	w.engine.Dispatch(ev)
}

// pngOf returns the clipboard's PNG or bitmap as a PNG, or why it could
// not.
func pngOf(data []byte) *driver.ClipboardImage {
	if driver.SniffImage(data) != "image/png" {
		m, err := bmp.Decode(bytes.NewReader(data))
		if err != nil {
			return driver.ReadClipboardImage(nil, driver.Size{}, err)
		}
		var b bytes.Buffer
		if err := png.Encode(&b, m); err != nil {
			return driver.ReadClipboardImage(nil, driver.Size{}, err)
		}
		data = b.Bytes()
	}
	return driver.ClipboardPNG(data)
}

// setClipboardImage puts the image data encodes on the clipboard as a
// PNG, which keeps its transparency, and a device-independent bitmap
// over white, which every app reads.
func (w *window) setClipboardImage(data []byte) {
	img, err := imagedecode.Decode(data, imagedecode.Options{Layout: imagedecode.RGBA})
	if err != nil || len(img.Frames) == 0 {
		return
	}
	pix := img.Frames[0].Pix
	rgba := &image.RGBA{Pix: pix, Stride: img.Width * 4, Rect: image.Rect(0, 0, img.Width, img.Height)}
	var encoded bytes.Buffer
	if png.Encode(&encoded, rgba) != nil {
		return
	}
	// BITMAPINFOHEADER, then rows bottom up of blue, green, red and an
	// unused byte.
	dib := make([]byte, 40+len(pix))
	binary.LittleEndian.PutUint32(dib[0:], 40)
	binary.LittleEndian.PutUint32(dib[4:], uint32(img.Width))
	binary.LittleEndian.PutUint32(dib[8:], uint32(img.Height))
	binary.LittleEndian.PutUint16(dib[12:], 1)
	binary.LittleEndian.PutUint16(dib[14:], 32)
	binary.LittleEndian.PutUint32(dib[20:], uint32(len(pix)))
	row := img.Width * 4
	for y := range img.Height {
		src := pix[y*row : (y+1)*row]
		dst := dib[40+(img.Height-1-y)*row:]
		for x := 0; x < row; x += 4 {
			white := 255 - src[x+3] // premultiplied over white
			dst[x], dst[x+1], dst[x+2], dst[x+3] = src[x+2]+white, src[x+1]+white, src[x]+white, 255
		}
	}
	if r, _, _ := openClipboard.Call(w.hwnd); r == 0 {
		return
	}
	defer closeClipboard.Call()
	emptyClipboard.Call()
	put := func(format uintptr, b []byte) {
		h, _, _ := globalAlloc.Call(gmemMoveable, uintptr(len(b)))
		if h == 0 {
			return
		}
		p, _, _ := globalLock.Call(h)
		if p == 0 {
			globalFree.Call(h)
			return
		}
		rtlMoveMemory.Call(p, uintptr(unsafe.Pointer(&b[0])), uintptr(len(b)))
		globalUnlock.Call(h)
		if r, _, _ := setClipboardData.Call(format, h); r == 0 {
			globalFree.Call(h)
		}
	}
	if f := pngFormat(); f != 0 {
		put(f, encoded.Bytes())
	}
	put(cfDIB, dib)
}

// shellExecuteInfo is SHELLEXECUTEINFOW.
type shellExecuteInfo struct {
	Size       uint32
	Mask       uint32
	Hwnd       uintptr
	Verb       *uint16
	File       *uint16
	Parameters *uint16
	Directory  *uint16
	Show       int32
	InstApp    uintptr
	IDList     uintptr
	Class      *uint16
	KeyClass   uintptr
	HotKey     uint32
	Icon       uintptr
	Process    uintptr
}

// openURL has the shell open u with the app registered for it, answering whether one took it.
func (w *window) openURL(u string, id uint32) {
	w.engine.Dispatch(driver.Event{Kind: driver.EventOpenURL, Count: id, Finished: w.shellOpenURL(u)})
}

// shellOpenURL has the shell open u with the app registered for it, reporting whether one took it; a scheme no app handles answers false without the shell asking for an app.
func (w *window) shellOpenURL(u string) bool {
	// The shell asks the user for an app when none handles the scheme, even without UI: ask the registry first.
	scheme, _, _ := strings.Cut(u, ":")
	var key syscall.Handle
	if syscall.RegOpenKeyEx(syscall.HKEY_CLASSES_ROOT, wide(scheme), 0, syscall.KEY_READ, &key) != nil {
		return false
	}
	syscall.RegCloseKey(key)
	const flagNoUI = 0x00000400 // SEE_MASK_FLAG_NO_UI
	info := shellExecuteInfo{Mask: flagNoUI, Hwnd: w.hwnd, Verb: wide("open"), File: wide(u), Show: swShowNormal}
	info.Size = uint32(unsafe.Sizeof(info))
	r, _, _ := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	return r != 0
}

// openFile has the shell open f with the app associated with its type.
// A type no app opens answers false without the shell asking for an app.
func (w *window) openFile(f driver.SharedFile, id uint32) {
	const flagNoUI = 0x00000400 // SEE_MASK_FLAG_NO_UI
	info := shellExecuteInfo{Mask: flagNoUI, Hwnd: w.hwnd, File: wide(f.Path), Show: swShowNormal}
	info.Size = uint32(unsafe.Sizeof(info))
	ev := driver.Event{Kind: driver.EventFiles, Count: id}
	r, _, err := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	const errNoAssociation = 1155 // ERROR_NO_ASSOCIATION
	switch {
	case r != 0:
		ev.Finished = true
	case err != syscall.Errno(errNoAssociation):
		ev.Text = err.Error()
	}
	w.engine.Dispatch(ev)
}

func (w *window) revealFile(f driver.SharedFile, id uint32) {
	const flagNoUI = 0x00000400 // SEE_MASK_FLAG_NO_UI
	params := wide(`/select,"` + f.Path + `"`)
	info := shellExecuteInfo{Mask: flagNoUI, Hwnd: w.hwnd, Verb: wide("open"), File: wide("explorer.exe"), Parameters: params, Show: swShowNormal}
	info.Size = uint32(unsafe.Sizeof(info))
	r, _, err := shellExecuteExW.Call(uintptr(unsafe.Pointer(&info)))
	ev := driver.Event{Kind: driver.EventFiles, Count: id, Finished: r != 0}
	if r == 0 {
		ev.Text = err.Error()
	}
	w.engine.Dispatch(ev)
}

var (
	clsidFileOperation           = parseGUID("3AD05575-8857-4850-9277-11B85BDB8E09")
	iidFileOperation             = parseGUID("947AAB5F-0A5C-4C13-B4D6-4BF7836FC9F8")
	iidFileOperationProgressSink = parseGUID("04B0F1A7-9490-44BC-96E1-4296A31252E2")

	// recycleSinkVtbl is IFileOperationProgressSink's: PreDeleteItem refuses an item the Recycle Bin cannot take, PostDeleteItem records the outcome, and the rest accept. Each callback takes exactly the method's arguments, which 386's stdcall pops.
	recycleSinkVtbl = [19]uintptr{
		syscall.NewCallback(recycleSinkQueryInterface),
		syscall.NewCallback(recycleSinkAddRef),
		syscall.NewCallback(recycleSinkRelease),
		syscall.NewCallback(func(s *recycleSink) uintptr { return sOK }),                                                       // StartOperations
		syscall.NewCallback(func(s *recycleSink, hr uintptr) uintptr { return sOK }),                                           // FinishOperations
		syscall.NewCallback(func(s *recycleSink, flags, item, name uintptr) uintptr { return sOK }),                            // PreRenameItem
		syscall.NewCallback(func(s *recycleSink, flags, item, name, hr, created uintptr) uintptr { return sOK }),               // PostRenameItem
		syscall.NewCallback(func(s *recycleSink, flags, item, dest, name uintptr) uintptr { return sOK }),                      // PreMoveItem
		syscall.NewCallback(func(s *recycleSink, flags, item, dest, name, hr, created uintptr) uintptr { return sOK }),         // PostMoveItem
		syscall.NewCallback(func(s *recycleSink, flags, item, dest, name uintptr) uintptr { return sOK }),                      // PreCopyItem
		syscall.NewCallback(func(s *recycleSink, flags, item, dest, name, hr, created uintptr) uintptr { return sOK }),         // PostCopyItem
		syscall.NewCallback(recycleSinkPreDelete),                                                                              // PreDeleteItem
		syscall.NewCallback(recycleSinkPostDelete),                                                                             // PostDeleteItem
		syscall.NewCallback(func(s *recycleSink, flags, dest, name uintptr) uintptr { return sOK }),                            // PreNewItem
		syscall.NewCallback(func(s *recycleSink, flags, dest, name, template, attrs, hr, item uintptr) uintptr { return sOK }), // PostNewItem
		syscall.NewCallback(func(s *recycleSink, total, done uintptr) uintptr { return sOK }),                                  // UpdateProgress
		syscall.NewCallback(func(s *recycleSink) uintptr { return sOK }),                                                       // ResetTimer
		syscall.NewCallback(func(s *recycleSink) uintptr { return sOK }),                                                       // PauseTimer
		syscall.NewCallback(func(s *recycleSink) uintptr { return sOK }),                                                       // ResumeTimer
	}
)

// IFileOperation's methods, and the flags trashFile sets.
const (
	fileOperationSetOperationFlags       = 5
	fileOperationDeleteItem              = 18
	fileOperationPerformOperations       = 21
	fileOperationGetAnyOperationsAborted = 22

	fofAllowUndo               = 0x40
	fofNoUI                    = 0x4 | 0x10 | 0x400 | 0x200 // FOF_NO_UI: FOF_SILENT, FOF_NOCONFIRMATION, FOF_NOERRORUI and FOF_NOCONFIRMMKDIR
	fofxRecycleOnDelete        = 0x00080000
	tsfDeleteRecycleIfPossible = 0x80 // TSF_DELETE_RECYCLE_IF_POSSIBLE
	eAbort                     = 0x80004004
)

// recycleSink is trashFile's IFileOperationProgressSink, implemented in Go. The operation calls it on the window's thread while it performs, and trashFile keeps it alive until the operation is released, so nothing is freed when it is released.
type recycleSink struct {
	vtbl *[19]uintptr
	refs atomic.Int32
	// refused is whether an item would have been deleted permanently. deleted is whether PostDeleteItem ran, with the delete's result, whether the item went to the Recycle Bin and its path there.
	refused, deleted, recycled bool
	result                     uint32
	at                         string
}

func recycleSinkQueryInterface(s *recycleSink, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == iidFileOperationProgressSink {
		*out = unsafe.Pointer(s)
		s.refs.Add(1)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func recycleSinkAddRef(s *recycleSink) uintptr { return uintptr(s.refs.Add(1)) }

func recycleSinkRelease(s *recycleSink) uintptr { return uintptr(s.refs.Add(-1)) }

// recycleSinkPreDelete cancels the operation before an item the shell would delete instead of recycling: one on a volume without a Recycle Bin, such as a network share, any when the user has the Recycle Bin remove files at once, and each item of a folder over the Recycle Bin's size, which the shell then deletes one by one without TSF_DELETE_RECYCLE_IF_POSSIBLE. A single file over the Recycle Bin's size has that flag and is deleted within the same call, which is why trashFile checks a file's size first. The flags are a DWORD, whose register's upper half is undefined.
func recycleSinkPreDelete(s *recycleSink, flags uintptr, item *comObject) uintptr {
	if uint32(flags)&tsfDeleteRecycleIfPossible == 0 {
		s.refused = true
		return eAbort
	}
	return sOK
}

// recycleSinkPostDelete records the delete's HRESULT, a 32-bit value, and the item now in the Recycle Bin, nil when it was deleted for good.
func recycleSinkPostDelete(s *recycleSink, flags uintptr, item *comObject, hr uintptr, created *comObject) uintptr {
	s.deleted, s.result, s.recycled = true, uint32(hr), created != nil
	if created != nil {
		var name *uint16
		if !failed(created.call(shellItemGetName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name)))) {
			s.at = takeString(name)
		}
	}
	return sOK
}

// recycleBinKeeps reports whether a Recycle Bin of maxMB megabytes, capped at policyPercent of a volume of volume bytes, keeps a file of size bytes. Windows 11 compares a file's length with the whole bin, not its free room: a 1 MB bin kept a file of 1 MiB, compressed or not, and deleted one a byte larger. This answers false at the bin's size exactly, a byte stricter, and for a bin of 0, which is what a size it cannot read is.
func recycleBinKeeps(size uint64, maxMB, policyPercent uint32, volume uint64) bool {
	return size < min(uint64(maxMB)<<20, volume/100*uint64(policyPercent))
}

var (
	getFinalPathNameByHandleW = kernel32.NewProc("GetFinalPathNameByHandleW")
	getDiskFreeSpaceExW       = kernel32.NewProc("GetDiskFreeSpaceExW")
)

// recycleBinTakes reports whether the Recycle Bin keeps the item at path rather than deleting it for good, which Windows does to a file over the bin's size without telling IFileOperation's sink first: PreDeleteItem has the same flags as for a file it recycles, and FOF_WANTNUKEWARNING asks the user. The bin's size is the MaxCapacity Explorer keeps, in megabytes, under HKEY_CURRENT_USER for the GUID of the volume the item is on, through junctions, mounted folders and subst drives, capped by the RecycleBinSize policy's percentage of the volume where one is set. It answers false when it cannot tell, as on a network share, which has no volume GUID, or a volume whose bin Explorer has not sized. A folder it answers true for: the shell sizes a folder before recycling it and deletes one too large item by item, which the progress sink refuses. err is why the item could not be read.
func recycleBinTakes(path string) (bool, error) {
	// The item itself, not what a symbolic link at path points to: the shell recycles the link.
	h, err := syscall.CreateFile(wide(path), 0, syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE, nil,
		syscall.OPEN_EXISTING, syscall.FILE_FLAG_BACKUP_SEMANTICS|syscall.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return false, err
	}
	defer syscall.CloseHandle(h)
	var info syscall.ByHandleFileInformation
	if err := syscall.GetFileInformationByHandle(h, &info); err != nil {
		return false, err
	}
	if info.FileAttributes&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 {
		return true, nil
	}
	const volumeNameGUID = 0x1 // VOLUME_NAME_GUID
	name := make([]uint16, len(path)+64)
	for {
		r, _, _ := getFinalPathNameByHandleW.Call(uintptr(h), uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)), volumeNameGUID)
		if r == 0 {
			return false, nil
		}
		if int(r) < len(name) {
			name = name[:r]
			break
		}
		// r is the length it needs, with the terminating NUL.
		name = make([]uint16, r)
	}
	// The name is \\?\Volume{GUID}\ and the item's path on the volume.
	final := syscall.UTF16ToString(name)
	const prefix = `\\?\Volume`
	end := strings.IndexByte(final, '}')
	if !strings.HasPrefix(final, prefix+"{") || end < 0 {
		return false, nil
	}
	root := wide(final[:end+1] + `\`)
	maxMB := regDWORD(`Software\Microsoft\Windows\CurrentVersion\Explorer\BitBucket\Volume\`+final[len(prefix):end+1], "MaxCapacity", 0)
	const policies = `Software\Microsoft\Windows\CurrentVersion\Policies\Explorer`
	percent := min(regDWORDIn(syscall.HKEY_CURRENT_USER, policies, "RecycleBinSize", 100), regDWORDIn(syscall.HKEY_LOCAL_MACHINE, policies, "RecycleBinSize", 100))
	var free, total, totalFree uint64
	getDiskFreeSpaceExW.Call(uintptr(unsafe.Pointer(root)), uintptr(unsafe.Pointer(&free)), uintptr(unsafe.Pointer(&total)), uintptr(unsafe.Pointer(&totalFree)))
	return recycleBinKeeps(uint64(info.FileSizeHigh)<<32|uint64(info.FileSizeLow), maxMB, percent, total), nil
}

// trashFile has the shell move f to the Recycle Bin without UI, as IFileOperation does with FOFX_RECYCLEONDELETE, and answers NoTrash, leaving f where it is, when the Recycle Bin cannot take it: FOF_ALLOWUNDO alone recycles only if possible and silently deletes the rest for good. It refuses a file the Recycle Bin may not keep before the operation, as recycleBinTakes tells, and the progress sink refuses the rest. It answers the item's path in the Recycle Bin when the shell gives one.
func (w *window) trashFile(f driver.SharedFile, id uint32) {
	ev := driver.Event{Kind: driver.EventFiles, Count: id}
	what := "Windows could not move " + f.Path + " to the Recycle Bin"
	if takes, err := recycleBinTakes(f.Path); err != nil || !takes {
		ev.Text = driver.NoTrash
		if err != nil {
			ev.Text = what + ": " + err.Error()
		}
		w.engine.Dispatch(ev)
		return
	}
	sink := &recycleSink{vtbl: &recycleSinkVtbl}
	// The operation holds the sink until it is released, which the deferred release below does first.
	defer runtime.KeepAlive(sink)
	if err := initCOM(); err != nil {
		ev.Text = err.Error()
		w.engine.Dispatch(ev)
		return
	}
	var op *comObject
	if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&op))); failed(hr) || op == nil {
		ev.Text = hresultError(what, hr).Error()
		w.engine.Dispatch(ev)
		return
	}
	defer op.release()
	var item *comObject
	hr, _, _ := shCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(wide(f.Path))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if failed(hr) || item == nil {
		ev.Text = hresultError(what, hr).Error()
		w.engine.Dispatch(ev)
		return
	}
	defer item.release()
	if hr = op.call(fileOperationSetOperationFlags, fofAllowUndo|fofNoUI|fofxRecycleOnDelete); !failed(hr) {
		if hr = op.call(fileOperationDeleteItem, item.ptr(), uintptr(unsafe.Pointer(sink))); !failed(hr) {
			hr = op.call(fileOperationPerformOperations)
		}
	}
	var aborted int32
	op.call(fileOperationGetAnyOperationsAborted, uintptr(unsafe.Pointer(&aborted)))
	switch {
	case sink.refused:
		ev.Text = driver.NoTrash
	case failed(hr):
		ev.Text = hresultError(what, hr).Error()
	case sink.deleted && failed(uintptr(sink.result)):
		ev.Text = hresultError(what, uintptr(sink.result)).Error()
	case aborted != 0 || !sink.deleted:
		ev.Text = what
	case !sink.recycled:
		ev.Text = "Windows deleted " + f.Path + " instead of moving it to the Recycle Bin"
	default:
		ev.Finished, ev.Data = true, sink.at
	}
	w.engine.Dispatch(ev)
}

// openFileName is OPENFILENAMEW.
type openFileName struct {
	StructSize    uint32
	Owner         uintptr
	Instance      uintptr
	Filter        *uint16
	CustomFilter  *uint16
	MaxCustFilter uint32
	FilterIndex   uint32
	File          *uint16
	MaxFile       uint32
	FileTitle     *uint16
	MaxFileTitle  uint32
	InitialDir    *uint16
	Title         *uint16
	Flags         uint32
	FileOffset    uint16
	FileExtension uint16
	DefExt        *uint16
	CustData      uintptr
	Hook          uintptr
	TemplateName  *uint16
	Reserved      uintptr
	ReservedDW    uint32
	FlagsEx       uint32
}

// pickImages queues a pick: the open dialog's modal loop must not run in
// the batch's. The images chosen are read as they are on a goroutine.
func (w *window) pickImages(r driver.PickRequest, id uint32) {
	if t := w.opts.test; t != nil && t.options.PickImages != nil {
		t.options.PickImages(r, func(files []driver.PickedFile) {
			w.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Data: driver.FinishPick(r, files, nil)})
		})
		return
	}
	w.queueDialog(func() {
		paths, _ := w.openDialog(syscall.StringToUTF16("Images"), r.DialogExtensions(), r.Limit)
		go func() {
			w.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Data: picked.ReadFiles(r, paths)})
		}()
	})
}

// pickFiles queues a pick of the files r offers.
func (w *window) pickFiles(r driver.FilesRequest, id uint32) {
	w.queueDialog(func() {
		paths, err := w.fileDialog(r)
		ev := driver.Event{Kind: driver.EventFiles, Count: id}
		if err != nil {
			ev.Text = err.Error()
		}
		var files []driver.File
		for _, p := range paths {
			files = append(files, driver.File{Path: p})
		}
		ev.Data = files
		w.engine.Dispatch(ev)
	})
}

// pickDirectories queues a pick of folders.
func (w *window) pickDirectories(r driver.DirectoriesRequest, id uint32) {
	w.queueDialog(func() {
		paths, err := w.folderDialog(r)
		ev := driver.Event{Kind: driver.EventFiles, Count: id}
		if err != nil {
			ev.Text = err.Error()
		}
		var dirs []driver.Directory
		for _, p := range paths {
			dirs = append(dirs, driver.Directory{Path: p})
		}
		ev.Data = dirs
		w.engine.Dispatch(ev)
	})
}

// saveFile queues a save: the save dialog suggests r's name, and the
// file is written where the user chose on a goroutine.
func (w *window) saveFile(r driver.SaveRequest, id uint32) {
	w.queueDialog(func() {
		path, err := w.saveDialog(r.Name, r.Start)
		if err != nil || path == "" {
			ev := driver.Event{Kind: driver.EventFiles, Count: id}
			if err != nil {
				ev.Text = err.Error()
			}
			w.engine.Dispatch(ev)
			return
		}
		go w.saves.Save(r, id, path, w.engine.Dispatch)
	})
}

// queueDialog runs show from a message of its own, one dialog at a
// time.
func (w *window) queueDialog(show func()) {
	w.dialogs = append(w.dialogs, show)
	postMessageW.Call(w.hwnd, wmDialog, 0, 0)
}

// showDialog shows the first dialog queued.
func (w *window) showDialog() {
	if w.inDialog || len(w.dialogs) == 0 {
		return
	}
	show := w.dialogs[0]
	w.dialogs = w.dialogs[1:]
	w.inDialog = true
	show()
	w.inDialog = false
	if len(w.dialogs) > 0 {
		postMessageW.Call(w.hwnd, wmDialog, 0, 0)
	}
}

// dialogError is what CommDlgExtendedError says failed in a dialog that
// returned false, nil when the user cancelled it.
func dialogError() error {
	code, _, _ := commDlgExtendedError.Call()
	if code == 0 {
		return nil
	}
	return fmt.Errorf("common dialog error %#x", code)
}

// openDialog returns the paths of the files chosen in Windows' open
// dialog, at most limit, offering those of exts under name, or every
// file for none; none when it was cancelled.
func (w *window) openDialog(name []uint16, exts []string, limit int) ([]string, error) {
	var pattern []uint16
	for i, ext := range exts {
		if i > 0 {
			pattern = append(pattern, ';')
		}
		pattern = append(pattern, syscall.StringToUTF16("*."+ext)...)
		pattern = pattern[:len(pattern)-1]
	}
	if len(exts) == 0 {
		pattern = append(pattern, '*', '.', '*')
	}
	filter := append(name, append(pattern, 0, 0)...)
	buf := make([]uint16, 64<<10)
	ofn := openFileName{Owner: w.owner(), Filter: &filter[0], File: &buf[0], MaxFile: uint32(len(buf)),
		Flags: ofnExplorer | ofnFileMustExist | ofnPathMustExist | ofnNoChangeDir}
	if limit > 1 {
		ofn.Flags |= ofnAllowMultiSelect
	}
	ofn.StructSize = uint32(unsafe.Sizeof(ofn))
	if ok, _, _ := getOpenFileNameW.Call(uintptr(unsafe.Pointer(&ofn))); ok == 0 {
		return nil, dialogError()
	}
	// One path, or the directory then each name, NUL-separated.
	var parts []string
	for start, i := 0, 0; i < len(buf); i++ {
		if buf[i] == 0 {
			if i == start {
				break
			}
			parts = append(parts, syscall.UTF16ToString(buf[start:i]))
			start = i + 1
		}
	}
	if len(parts) <= 1 {
		return parts, nil
	}
	dir := parts[0]
	paths := make([]string, 0, len(parts)-1)
	for _, name := range parts[1:] {
		if len(paths) < max(limit, 1) {
			paths = append(paths, dir+`\`+name)
		}
	}
	return paths, nil
}

var (
	clsidFileOpenDialog = parseGUID("DC1C5A9C-E88A-4DDE-A5A1-60F82A20AEF7")
	iidFileOpenDialog   = parseGUID("D57C7288-D4AD-4768-BE02-9D969532D960")
	clsidFileSaveDialog = parseGUID("C0B4E2F3-BA21-4773-8DBA-335EC946EB8B")
	iidFileSaveDialog   = parseGUID("84BCCD23-5FDE-4CDB-AEA4-AF64B83D78AB")
)

// IFileOpenDialog's methods, from IModalWindow's and IFileDialog's, and
// those of the shell items it answers with.
const (
	fileDialogShow                = 3
	fileDialogSetFileTypes        = 4
	fileDialogAdvise              = 7
	fileDialogUnadvise            = 8
	fileDialogSetOptions          = 9
	fileDialogGetOptions          = 10
	fileDialogSetFolder           = 12
	fileDialogGetFolder           = 13
	fileDialogSetFileName         = 15
	fileDialogSetTitle            = 17
	fileDialogSetOkButtonLabel    = 18
	fileDialogGetResult           = 20
	fileDialogSetDefaultExtension = 22
	fileDialogGetResults          = 27
	itemArrayGetCount             = 7
	itemArrayGetItemAt            = 8
	shellItemGetName              = 5

	fosOverwritePrompt          = 0x2
	fosNoChangeDir              = 0x8
	fosPickFolders              = 0x20
	fosForceFileSystem          = 0x40
	fosAllowMultiSelect         = 0x200
	fosPathMustExist            = 0x800
	sigdnFileSysPath            = 0x80058000
	sigdnDesktopAbsoluteParsing = 0x80028000
	hresultCancelled            = 0x800704C7 // HRESULT_FROM_WIN32(ERROR_CANCELLED)

	// homeFolder is the CLSID of Windows 11's Home, which is not on the
	// file system.
	homeFolder = "{F874310E-B6B7-47DC-BC84-B9E6B38F5903}"
)

var (
	iidFileDialogEvents         = parseGUID("973510DB-7D7F-452B-8975-74A85828D354")
	iidShellItem                = parseGUID("43826D1E-E718-42EE-BC55-A1E261C37BFE")
	shCreateItemFromParsingName = shell32.NewProc("SHCreateItemFromParsingName")

	// folderEventsVtbl is IFileDialogEvents': OnFolderChange answers,
	// and the rest accept.
	folderEventsVtbl = [10]uintptr{
		syscall.NewCallback(folderEventsQueryInterface),
		syscall.NewCallback(folderEventsAddRef),
		syscall.NewCallback(folderEventsRelease),
		syscall.NewCallback(func(e *folderEvents, dialog uintptr) uintptr { return sOK }),                 // OnFileOk
		syscall.NewCallback(func(e *folderEvents, dialog, folder uintptr) uintptr { return sOK }),         // OnFolderChanging
		syscall.NewCallback(folderEventsFolderChange),                                                     // OnFolderChange
		syscall.NewCallback(func(e *folderEvents, dialog uintptr) uintptr { return sOK }),                 // OnSelectionChange
		syscall.NewCallback(func(e *folderEvents, dialog, item, response uintptr) uintptr { return sOK }), // OnShareViolation
		syscall.NewCallback(func(e *folderEvents, dialog uintptr) uintptr { return sOK }),                 // OnTypeChange
		syscall.NewCallback(func(e *folderEvents, dialog, item, response uintptr) uintptr { return sOK }), // OnOverwrite
	}
)

// folderEvents is the folder dialog's IFileDialogEvents, implemented in
// Go. The dialog calls it on the window's thread, and the window keeps it
// alive while the dialog shows, so nothing is freed when it is released.
type folderEvents struct {
	vtbl *[10]uintptr
	refs atomic.Int32
	w    *window
}

func folderEventsQueryInterface(e *folderEvents, iid *guid, out *unsafe.Pointer) uintptr {
	if *iid == iidUnknown || *iid == iidFileDialogEvents {
		*out = unsafe.Pointer(e)
		e.refs.Add(1)
		return sOK
	}
	*out = nil
	return eNoInterface
}

func folderEventsAddRef(e *folderEvents) uintptr { return uintptr(e.refs.Add(1)) }

func folderEventsRelease(e *folderEvents) uintptr { return uintptr(e.refs.Add(-1)) }

// folderEventsFolderChange has the window check the folder opened once
// the dialog has finished opening it, which fills in the name field.
func folderEventsFolderChange(e *folderEvents, dialog uintptr) uintptr {
	postMessageW.Call(e.w.hwnd, wmFolderChange, 0, 0)
	return sOK
}

// leaveVirtualFolder corrects the folder dialog when it opens a folder
// that is not on the file system, whose name it puts in the name field
// though Select Folder cannot resolve it: Home opens the user's profile,
// as Explorer's does, and the field is emptied so Select Folder picks the
// folder open.
func (w *window) leaveVirtualFolder() {
	d := w.folderPick
	if d == nil {
		return
	}
	var folder *comObject
	if failed(d.call(fileDialogGetFolder, uintptr(unsafe.Pointer(&folder)))) || folder == nil {
		return
	}
	defer folder.release()
	var name *uint16
	if !failed(folder.call(shellItemGetName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name)))) {
		takeString(name)
		return
	}
	name = nil
	folder.call(shellItemGetName, sigdnDesktopAbsoluteParsing, uintptr(unsafe.Pointer(&name)))
	if strings.Contains(strings.ToUpper(takeString(name)), homeFolder) {
		if home, err := os.UserHomeDir(); err == nil {
			var item *comObject
			if hr, _, _ := shCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(wide(home))), 0,
				uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item))); !failed(hr) && item != nil {
				d.call(fileDialogSetFolder, item.ptr())
				item.release()
			}
		}
	}
	empty := uint16(0)
	d.call(fileDialogSetFileName, uintptr(unsafe.Pointer(&empty)))
}

// comFilterSpec is COMDLG_FILTERSPEC, the name and pattern shown by an IFileDialog filter menu.
type comFilterSpec struct {
	name    *uint16
	pattern *uint16
}

// fileDialog returns the paths of the files chosen in Windows' open dialog (IFileOpenDialog), at most r.Limit, with r's title, button label, type menu and starting folder; none when it was cancelled.
func (w *window) fileDialog(r driver.FilesRequest) ([]string, error) {
	if err := initCOM(); err != nil {
		return nil, err
	}
	var dialog *comObject
	if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dialog))); failed(hr) || dialog == nil {
		return nil, hresultError("no file dialog", hr)
	}
	defer dialog.release()
	var options uint32
	dialog.call(fileDialogGetOptions, uintptr(unsafe.Pointer(&options)))
	options |= fosForceFileSystem | fosPathMustExist | fosNoChangeDir
	if r.Limit > 1 {
		options |= fosAllowMultiSelect
	}
	if hr := dialog.call(fileDialogSetOptions, uintptr(options)); failed(hr) {
		return nil, hresultError("file dialog options", hr)
	}
	if r.Title != "" {
		dialog.call(fileDialogSetTitle, uintptr(unsafe.Pointer(wide(r.Title))))
	}
	if r.Button != "" {
		dialog.call(fileDialogSetOkButtonLabel, uintptr(unsafe.Pointer(wide(r.Button))))
	}
	name, pattern := fileFilter(r)
	filter := comFilterSpec{name: wide(name), pattern: wide(pattern)}
	if hr := dialog.call(fileDialogSetFileTypes, 1, uintptr(unsafe.Pointer(&filter))); failed(hr) {
		return nil, hresultError("file dialog types", hr)
	}
	setDialogFolder(dialog, r.Start)
	hr := dialog.call(fileDialogShow, w.owner())
	if uint32(hr) == hresultCancelled {
		return nil, nil
	}
	if failed(hr) {
		return nil, hresultError("file dialog", hr)
	}
	if r.Limit <= 1 {
		var item *comObject
		if hr := dialog.call(fileDialogGetResult, uintptr(unsafe.Pointer(&item))); failed(hr) || item == nil {
			return nil, hresultError("no file chosen", hr)
		}
		defer item.release()
		path, err := shellItemPath(item)
		if err != nil {
			return nil, err
		}
		return []string{path}, nil
	}
	return dialogResults(dialog, r.Limit)
}

// fileFilter is the one entry of the file dialog's type menu: r's extensions under TypesName, else "Files", or every file under TypesName, else "All files", when a type has no extensions.
func fileFilter(r driver.FilesRequest) (name, pattern string) {
	exts, every := r.Extensions()
	name = r.TypesName
	if every {
		if name == "" {
			name = "All files"
		}
		return name, "*.*"
	}
	if name == "" {
		name = "Files"
	}
	for i, ext := range exts {
		if i > 0 {
			pattern += ";"
		}
		pattern += "*." + ext
	}
	return name, pattern
}

// setDialogFolder opens an IFileDialog in start when it is a folder on disk, which the shell names only by an absolute path.
func setDialogFolder(dialog *comObject, start string) {
	if start == "" {
		return
	}
	start, err := filepath.Abs(start)
	if err != nil {
		return
	}
	if info, err := os.Stat(start); err != nil || !info.IsDir() {
		return
	}
	var item *comObject
	if hr, _, _ := shCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(wide(start))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item))); !failed(hr) && item != nil {
		dialog.call(fileDialogSetFolder, item.ptr())
		item.release()
	}
}

// shellItemPath is the file system path of a shell item the dialog answered with.
func shellItemPath(item *comObject) (string, error) {
	var path *uint16
	if hr := item.call(shellItemGetName, sigdnFileSysPath, uintptr(unsafe.Pointer(&path))); failed(hr) || path == nil {
		return "", hresultError("file dialog path", hr)
	}
	return takeString(path), nil
}

// dialogResults are the paths of the items chosen in a multiple-selection dialog, at most limit.
func dialogResults(dialog *comObject, limit int) ([]string, error) {
	var items *comObject
	if hr := dialog.call(fileDialogGetResults, uintptr(unsafe.Pointer(&items))); failed(hr) || items == nil {
		return nil, hresultError("no files chosen", hr)
	}
	defer items.release()
	var n uint32
	items.call(itemArrayGetCount, uintptr(unsafe.Pointer(&n)))
	paths := make([]string, 0, min(int(n), max(limit, 1)))
	for i := uint32(0); i < n && len(paths) < max(limit, 1); i++ {
		var item *comObject
		if failed(items.call(itemArrayGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&item)))) || item == nil {
			continue
		}
		path, err := shellItemPath(item)
		item.release()
		if err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// folderDialog returns the paths of the folders chosen in Windows' open dialog for folders (IFileOpenDialog), at most r.Limit, with r's title, button label and starting folder; none when it was cancelled.
func (w *window) folderDialog(r driver.DirectoriesRequest) ([]string, error) {
	if err := initCOM(); err != nil {
		return nil, err
	}
	var dialog *comObject
	if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOpenDialog)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileOpenDialog)), uintptr(unsafe.Pointer(&dialog))); failed(hr) || dialog == nil {
		return nil, hresultError("no folder dialog", hr)
	}
	defer dialog.release()
	var options uint32
	dialog.call(fileDialogGetOptions, uintptr(unsafe.Pointer(&options)))
	options |= fosPickFolders | fosForceFileSystem | fosPathMustExist | fosNoChangeDir
	if r.Limit > 1 {
		options |= fosAllowMultiSelect
	}
	if r.Title != "" {
		dialog.call(fileDialogSetTitle, uintptr(unsafe.Pointer(wide(r.Title))))
	}
	if r.Button != "" {
		dialog.call(fileDialogSetOkButtonLabel, uintptr(unsafe.Pointer(wide(r.Button))))
	}
	setDialogFolder(dialog, r.Start)
	dialog.call(fileDialogSetOptions, uintptr(options))
	events := &folderEvents{vtbl: &folderEventsVtbl, w: w}
	var cookie uint32
	advised := !failed(dialog.call(fileDialogAdvise, uintptr(unsafe.Pointer(events)), uintptr(unsafe.Pointer(&cookie))))
	w.folderPick = dialog
	hr := dialog.call(fileDialogShow, w.owner())
	w.folderPick = nil
	if advised {
		dialog.call(fileDialogUnadvise, uintptr(cookie))
	}
	runtime.KeepAlive(events)
	if uint32(hr) == hresultCancelled {
		return nil, nil
	} else if failed(hr) {
		return nil, hresultError("folder dialog", hr)
	}
	var items *comObject
	if hr := dialog.call(fileDialogGetResults, uintptr(unsafe.Pointer(&items))); failed(hr) || items == nil {
		return nil, hresultError("no folders chosen", hr)
	}
	defer items.release()
	var n uint32
	items.call(itemArrayGetCount, uintptr(unsafe.Pointer(&n)))
	var paths []string
	for i := range min(int(n), max(r.Limit, 1)) {
		var item *comObject
		if failed(items.call(itemArrayGetItemAt, uintptr(i), uintptr(unsafe.Pointer(&item)))) || item == nil {
			continue
		}
		var name *uint16
		if !failed(item.call(shellItemGetName, sigdnFileSysPath, uintptr(unsafe.Pointer(&name)))) {
			if p := takeString(name); p != "" {
				paths = append(paths, p)
			}
		}
		item.release()
	}
	return paths, nil
}

// saveDialog suggests name, opens at start when it exists, and asks
// before replacing a file. It returns an empty path when cancelled.
func (w *window) saveDialog(name, start string) (string, error) {
	if err := initCOM(); err != nil {
		return "", err
	}
	var dialog *comObject
	if hr, _, _ := coCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileSaveDialog)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileSaveDialog)), uintptr(unsafe.Pointer(&dialog))); failed(hr) || dialog == nil {
		return "", hresultError("no save dialog", hr)
	}
	defer dialog.release()
	var options uint32
	dialog.call(fileDialogGetOptions, uintptr(unsafe.Pointer(&options)))
	options |= fosForceFileSystem | fosPathMustExist | fosNoChangeDir | fosOverwritePrompt
	if hr := dialog.call(fileDialogSetOptions, uintptr(options)); failed(hr) {
		return "", hresultError("save dialog options", hr)
	}
	filter := comFilterSpec{name: wide("All files"), pattern: wide("*.*")}
	if hr := dialog.call(fileDialogSetFileTypes, 1, uintptr(unsafe.Pointer(&filter))); failed(hr) {
		return "", hresultError("save dialog types", hr)
	}
	if hr := dialog.call(fileDialogSetFileName, uintptr(unsafe.Pointer(wide(name)))); failed(hr) {
		return "", hresultError("save dialog name", hr)
	}
	if ext := strings.TrimPrefix(filepath.Ext(name), "."); ext != "" {
		dialog.call(fileDialogSetDefaultExtension, uintptr(unsafe.Pointer(wide(ext))))
	}
	setDialogFolder(dialog, start)
	hr := dialog.call(fileDialogShow, w.owner())
	if uint32(hr) == hresultCancelled {
		return "", nil
	}
	if failed(hr) {
		return "", hresultError("save dialog", hr)
	}
	var item *comObject
	if hr := dialog.call(fileDialogGetResult, uintptr(unsafe.Pointer(&item))); failed(hr) || item == nil {
		return "", hresultError("no save path chosen", hr)
	}
	defer item.release()
	return shellItemPath(item)
}

type popupRequest struct {
	entries []driver.MenuEntry
	at      driver.Point
	id      uint32
	screen  bool
	tray    uint32
	// keyboard opens it with its first item selected, as a menu bar's menu opened from the keyboard is.
	keyboard bool
}

// popupMenu queues a context menu: TrackPopupMenuEx's loop must not run
// in the batch's. One shows at a time, so one tracking ends first.
func (w *window) popupMenu(entries []driver.MenuEntry, at driver.Point, id uint32) {
	if w.popupTracking {
		endMenu.Call()
	}
	w.popups = append(w.popups, popupRequest{entries: entries, at: at, id: id})
	postMessageW.Call(w.hwnd, wmPopup, 0, 0)
}

// trayMenu queues a menu for the shell icon. Its coordinates are already in
// screen pixels because the shell callback does not belong to the client.
func (w *window) trayMenu(id uint32, entries []driver.MenuEntry, popup uint32) {
	if w.popupTracking {
		endMenu.Call()
	}
	var at point
	getCursorPos.Call(uintptr(unsafe.Pointer(&at)))
	w.popups = append(w.popups, popupRequest{entries: entries, at: driver.Point{X: driver.Points(at.X), Y: driver.Points(at.Y)}, id: popup, screen: true, tray: id})
	postMessageW.Call(w.hwnd, wmPopup, 0, 0)
}

// showPopup tracks the first context menu queued, chooses its item and
// reports it closed. A later one replaces those queued before it.
func (w *window) showPopup() {
	if w.popupTracking || len(w.popups) == 0 {
		return
	}
	p := w.popups[0]
	w.popups = w.popups[1:]
	if p.tray != 0 && w.trays[p.tray] == nil {
		w.engine.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: p.id})
		if len(w.popups) > 0 {
			postMessageW.Call(w.hwnd, wmPopup, 0, 0)
		}
		return
	}
	if len(w.popups) > 0 {
		// Replaced before it showed.
		w.engine.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: p.id})
		postMessageW.Call(w.hwnd, wmPopup, 0, 0)
		return
	}
	menu, _, _ := createPopupMenu.Call()
	var refs []menuRef
	if menu != 0 {
		w.appendMenuTo(menu, p.entries, &refs)
		at := point{w.px(p.at.X), w.px(p.at.Y)}
		owner := w.hwnd
		if !p.screen {
			clientToScreen.Call(w.hwnd, uintptr(unsafe.Pointer(&at)))
		} else {
			at = point{int32(p.at.X), int32(p.at.Y)}
			// The menu closes as it loses the focus only when its owner is in front, which a shown window of the app's must not be brought for.
			owner = w.trayMenuOwner()
			setForegroundWindow.Call(owner)
		}
		w.popupTracking, w.popupShown = true, p.entries
		if p.keyboard {
			// The menu's loop takes the thread's key messages, so a Down queued before it starts selects the first item.
			postMessageW.Call(w.hwnd, wmKeyDown, vkDown, 0)
		}
		chosen, _, _ := trackPopupMenuEx.Call(menu, w.popupFlags(), uintptr(at.X), uintptr(at.Y), owner, 0)
		if p.screen {
			postMessageW.Call(owner, wmNull, 0, 0)
		}
		w.popupTracking, w.popupShown = false, nil
		destroyMenu.Call(menu)
		if c := w.popupChoice; c != nil {
			// A test chose, which ended the tracking.
			w.popupChoice, chosen = nil, 0
			for i, r := range refs {
				if *c != 0 && r.id == *c {
					chosen = uintptr(i + 1)
				}
			}
		}
		if i := int(chosen) - 1; i >= 0 && i < len(refs) {
			w.chooseMenu(refs[i])
		}
	}
	w.engine.Dispatch(driver.Event{Kind: driver.EventMenuClosed, Count: p.id})
	if len(w.popups) > 0 {
		postMessageW.Call(w.hwnd, wmPopup, 0, 0)
	}
}

// shownDark reports whether the app shows dark: as it forces, else as
// the system is.
func (w *window) shownDark() bool {
	for _, a := range []driver.Appearance{w.appearance, w.host} {
		switch a {
		case driver.AppearanceLight:
			return false
		case driver.AppearanceDark:
			return true
		}
	}
	return w.env.Dark
}

// setAppearance shows the title bar, the caption buttons and menus in a,
// or as the system's appearance is.
func (w *window) setAppearance(a driver.Appearance) {
	w.appearance = a
	w.darkTitleBar(w.shownDark())
	darkMenus(a)
	w.redrawAll()
}

// darkMenus has Win32's menus follow a: uxtheme's undocumented
// SetPreferredAppMode (ordinal 135) and FlushMenuThemes (136), as NSH and
// Explorer use them.
func darkMenus(a driver.Appearance) {
	if uxtheme.Load() != nil {
		return
	}
	mode, err := getProcOrdinal(uxtheme.Handle(), 135)
	if err != nil {
		return
	}
	const allowDark, forceDark, forceLight = 1, 2, 3
	m := allowDark
	switch a {
	case driver.AppearanceDark:
		m = forceDark
	case driver.AppearanceLight:
		m = forceLight
	}
	syscall.SyscallN(mode, uintptr(m))
	if flush, err := getProcOrdinal(uxtheme.Handle(), 136); err == nil && flush != 0 {
		syscall.SyscallN(flush)
	}
}

// getProcOrdinal returns the function of module exported at ordinal.
func getProcOrdinal(module uintptr, ordinal uintptr) (uintptr, error) {
	p, _, err := getProcAddress.Call(module, ordinal)
	if p == 0 {
		return 0, err
	}
	return p, nil
}

// windowFrame returns the window's frame from the main screen's top left, as
// setWindowFrame takes it; a minimized window, parked out of sight, keeps
// the one it had.
func (w *window) windowFrame() driver.WindowFrame {
	if w.embedded {
		return driver.WindowFrame{}
	}
	if isIconicWindow(w.hwnd) {
		return w.env.WindowFrame
	}
	var r rect
	getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	corner, size := w.point(r.Left, r.Top), w.point(r.Right-r.Left, r.Bottom-r.Top)
	// Through the window's monitor, in the points Env.Displays uses.
	if monitor, _, _ := monitorFromWindow.Call(w.hwnd, monitorDefaultNear); monitor != 0 {
		for _, placement := range monitorLayout() {
			if placement.monitor == monitor {
				corner = placement.point(r.Left, r.Top)
				size = driver.Point{X: driver.Points(float64(r.Right-r.Left) / placement.scale), Y: driver.Points(float64(r.Bottom-r.Top) / placement.scale)}
				break
			}
		}
	}
	return driver.WindowFrame{X: corner.X, Y: corner.Y, Move: true, Width: size.X, Height: size.Y}
}

// setWindowFrame moves and sizes the window: points from the main
// screen's top left, which is the virtual screen's origin, in the
// window's pixels.
func (w *window) setWindowFrame(f driver.WindowFrame) {
	if w.chrome.fullScreen {
		return
	}
	if w.zoomed() || isIconicWindow(w.hwnd) {
		showWindow.Call(w.hwnd, swRestore)
	}
	var r rect
	getWindowRect.Call(w.hwnd, uintptr(unsafe.Pointer(&r)))
	x, y, cx, cy := r.Left, r.Top, r.Right-r.Left, r.Bottom-r.Top
	px := w.px
	if f.Move {
		x, y = w.px(f.X), w.px(f.Y)
		// Through the monitor the frame is on, as windowFrame reads it.
		if placement, ok := placementForFrame(f); ok {
			x, y = placement.pixel(f.X, f.Y)
			px = placement.px
		}
	}
	if f.Width > 0 {
		cx = px(f.Width)
	}
	if f.Height > 0 {
		cy = px(f.Height)
	}
	w.setWindowPosAt(x, y, cx, cy)
}

func isIconicWindow(hwnd uintptr) bool {
	r, _, _ := isIconic.Call(hwnd)
	return r != 0
}

// setLevel puts the window above the others, among them, or at the
// bottom, where activating it raises it again.
func (w *window) setLevel(level driver.WindowLevel) {
	const flags = swpNoMove | swpNoSize | swpNoActivate
	switch level {
	case driver.WindowLevelFloating:
		setWindowPos.Call(w.hwnd, hwndTopmost, 0, 0, 0, 0, flags)
	case driver.WindowLevelDesktop:
		setWindowPos.Call(w.hwnd, hwndNoTopmost, 0, 0, 0, 0, flags)
		setWindowPos.Call(w.hwnd, hwndBottom, 0, 0, 0, 0, flags)
	default:
		setWindowPos.Call(w.hwnd, hwndNoTopmost, 0, 0, 0, 0, flags)
	}
}

// placePanel moves a panel beside the rectangle beside on side when side is set (core.Window.ShowAt), inside the work area of the monitor that holds its center, else beside tray icon anchor, else, with no anchor or none the shell places, to the upper middle of the monitor with the pointer, as macOS shows it.
func (w *window) placePanel(anchor uint32, beside driver.Bounds, side driver.WindowPlacement) {
	if side != driver.WindowPlaceDefault && w.placeBeside(beside, side) {
		return
	}
	if anchor != 0 && w.placeAtTray(anchor) {
		return
	}
	var at point
	getCursorPos.Call(uintptr(unsafe.Pointer(&at)))
	pointer := rect{at.X, at.Y, at.X + 1, at.Y + 1}
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	monitor, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&pointer)), monitorDefaultNear)
	if r, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return
	}
	width, height, ok := w.panelPixels(monitor)
	if !ok {
		return
	}
	screen, work := mi.Monitor, mi.Work
	x := screen.Left + (screen.Right-screen.Left-width)/2
	y := screen.Top + (screen.Bottom-screen.Top)/3 - height/2
	x = max(work.Left, min(x, work.Right-width))
	y = max(work.Top, min(y, work.Bottom-height))
	w.movePanel(x, y, width, height)
}

// fitPanel sizes a panel to width by height points, the size its content takes (driver.WindowFitSize), keeping its top left corner: one that shows beside the rectangle ShowAt placed it at, or beside its tray icon, is placed there again at its new size.
func (w *window) fitPanel(width, height driver.Points) {
	setWindowPos.Call(w.hwnd, 0, 0, 0, uintptr(w.px(width)), uintptr(w.px(height)), swpNoMove|swpNoZOrder|swpNoActivate)
	switch {
	case !w.visible:
	case w.placement != driver.WindowPlaceDefault:
		w.placeBeside(w.anchorBounds, w.placement)
	case w.anchor != 0:
		w.placeAtTray(w.anchor)
	}
}

// panelPixels returns the panel's size in pixels on monitor: its size in points now, at that monitor's scale, which may not be the one it was laid out at (a panel is made on the primary monitor).
func (w *window) panelPixels(monitor uintptr) (width, height int32, ok bool) {
	var client rect
	if r, _, _ := getClientRect.Call(w.hwnd, uintptr(unsafe.Pointer(&client))); r == 0 || client.Right <= 0 || client.Bottom <= 0 {
		return 0, 0, false
	}
	size := w.point(client.Right, client.Bottom)
	scale := w.scale
	var dpi, dpiY uint32
	if r, _, _ := getDpiForMonitor.Call(monitor, 0, uintptr(unsafe.Pointer(&dpi)), uintptr(unsafe.Pointer(&dpiY))); r == 0 && dpi > 0 {
		scale = float64(dpi) / 96 // MDT_EFFECTIVE_DPI
	}
	return int32(math.Round(float64(size.X) * scale)), int32(math.Round(float64(size.Y) * scale)), true
}

// movePanel moves and sizes the panel to the pixels it chose on its monitor, which a DPI change the move causes keeps (setWindowPosAt), then lays it out at the scale it has there: a hidden window may move to a monitor of another scale without a WM_DPICHANGED.
func (w *window) movePanel(x, y, width, height int32) {
	w.setWindowPosAt(x, y, width, height)
	dpi, _, _ := getDpiForWindow.Call(w.hwnd)
	if dpi == 0 || float64(dpi)/96 == w.scale {
		return
	}
	w.scale = float64(dpi) / 96
	if w.engine != nil {
		size := w.point(width, height)
		w.engine.Resize(driver.Size{Width: size.X, Height: size.Y}, driver.Geometry{})
		w.updateEnvironment()
	}
}

// placeAtTray moves a panel beside its notification-area icon, reporting whether it could. Shell owns the icon's screen rectangle; use the work area of that monitor to keep the panel visible and prefer the space above the taskbar.
func (w *window) placeAtTray(id uint32) bool {
	anchor, ok := w.trayRect(id)
	if !ok {
		return false
	}
	var mi monitorInfo
	mi.Size = uint32(unsafe.Sizeof(mi))
	monitor, _, _ := monitorFromRect.Call(uintptr(unsafe.Pointer(&anchor)), monitorDefaultNear)
	if r, _, _ := getMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return false
	}
	width, height, ok := w.panelPixels(monitor)
	if !ok {
		return false
	}
	work := mi.Work
	x := anchor.Left + (anchor.Right-anchor.Left-width)/2
	y := anchor.Top - height - 6
	if y < work.Top {
		y = anchor.Bottom + 6
	}
	x = max(work.Left, min(x, work.Right-width))
	y = max(work.Top, min(y, work.Bottom-height))
	w.movePanel(x, y, width, height)
	return true
}

// placeBeside moves a panel beside at, a rectangle in Env.Displays' points, on side, sized at the scale of the monitor it goes on; false where there is no monitor or the panel has no size yet.
func (w *window) placeBeside(at driver.Bounds, side driver.WindowPlacement) bool {
	monitors := monitorLayout()
	if len(monitors) == 0 {
		return false
	}
	center := driver.Point{X: at.Origin.X + at.Size.Width/2, Y: at.Origin.Y + at.Size.Height/2}
	on := monitors[0]
	for _, m := range monitors {
		if m.display.Frame.Contains(center) {
			on = m
			break
		}
	}
	width, height, ok := w.panelPixels(on.monitor)
	if !ok {
		return false
	}
	size := driver.Size{Width: driver.Points(float64(width) / on.scale), Height: driver.Points(float64(height) / on.scale)}
	p, _ := driver.PlaceWindow(at, size, on.display.VisibleFrame, side)
	x, y := on.pixel(p.X, p.Y)
	w.movePanel(x, y, width, height)
	return true
}

// orderFront shows the window, restored when minimized, in front and focused. A panel that never takes the focus (SurfaceConfig.NoActivate) shows at the top of the topmost windows instead, leaving the foreground window and its keyboard focus as they are.
func (w *window) orderFront() {
	if w.kind == driver.SurfacePanel && w.noActivate {
		showWindow.Call(w.hwnd, swShowNA)
		setWindowPos.Call(w.hwnd, hwndTopmost, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoActivate)
		return
	}
	if isIconicWindow(w.hwnd) {
		showWindow.Call(w.hwnd, swRestore)
	} else {
		showWindow.Call(w.hwnd, swShow)
	}
	bringWindowToTop.Call(w.hwnd)
	setForegroundWindow.Call(w.hwnd)
}

// setMenuBar says when the menu bar shows.
func (w *window) setMenuBar(mode driver.MenuBarMode) {
	w.chrome.menuBar, w.chrome.revealed = mode, false
	w.showMenuBar()
}

// menuBarKey follows the keys that show a hidden menu bar: Alt pressed and released alone toggles it, and is kept from DefWindowProc, which would enter menu mode; Alt with a mnemonic and F10 show it for the menu DefWindowProc then opens. A window whose menu pops up (menuPopups) follows menuPopupKey instead. It reports whether it took the key.
func (w *window) menuBarKey(message uint32, vk uintptr, lParam uintptr) bool {
	if w.menuPopups() {
		return w.menuPopupKey(message, vk, lParam)
	}
	c := &w.chrome
	if c.menuBar != driver.MenuBarAutoHide || c.menu == nil {
		return false
	}
	switch {
	case message == wmSysKeyDown && vk == vkMenu:
		if !repeated(lParam) {
			c.altAlone = true
		}
	case message == wmSysKeyUp && vk == vkMenu:
		alone := c.altAlone
		c.altAlone = false
		if alone {
			c.revealed = !c.revealed
			w.showMenuBar()
			return true
		}
	case message == wmSysKeyDown:
		c.altAlone = false
		if (vk == vkF10 || keyDown(vkMenu)) && !c.revealed && !c.menuOpen {
			c.menuOpen = true
			w.showMenuBar()
		}
	}
	return false
}

// menuPopupKey opens the application menu of a window with no menu bar as a popup, on Alt pressed and released alone and on F10, which it keeps from DefWindowProc's menu mode. Shift+F10 stays the context menu's key, and Alt with another key stays DefWindowProc's, such as Alt+F4 and Alt+Space, or menuChar's mnemonic. It reports whether it took the key.
func (w *window) menuPopupKey(message uint32, vk uintptr, lParam uintptr) bool {
	c := &w.chrome
	switch {
	case message == wmSysKeyDown && vk == vkMenu:
		if !repeated(lParam) {
			c.altAlone = true
		}
	case message == wmSysKeyUp && vk == vkMenu:
		alone := c.altAlone
		c.altAlone = false
		if alone {
			w.popupApplicationMenu(c.menu)
			return true
		}
	case message == wmSysKeyDown:
		c.altAlone = false
		if vk == vkF10 && !repeated(lParam) && !keyDown(vkShift) {
			w.popupApplicationMenu(c.menu)
			return true
		}
	}
	return false
}

// menuLoop follows a menu of the bar opening and closing: the bar shows
// while it is open, and hides once it closes.
func (w *window) menuLoop(enter bool, popup bool) {
	c := &w.chrome
	if c.menuBar != driver.MenuBarAutoHide || popup {
		return
	}
	if enter {
		return
	}
	if c.menuOpen || c.revealed {
		c.menuOpen, c.revealed = false, false
		w.showMenuBar()
	}
}

// hideMenuBar hides a shown menu bar again after a press in the window. A press with Alt down, such as Alt+click, makes Alt no longer alone, so its release opens no menu.
func (w *window) hideMenuBar() {
	w.chrome.altAlone = false
	if c := &w.chrome; c.menuBar == driver.MenuBarAutoHide && c.revealed {
		c.revealed = false
		w.showMenuBar()
	}
}

// osVersion returns Windows' version: 10.0 on Windows 11 too.
func osVersion() driver.OSVersion {
	var v osVersionInfo
	v.Size = uint32(unsafe.Sizeof(v))
	if r, _, _ := rtlGetVersion.Call(uintptr(unsafe.Pointer(&v))); r != 0 {
		return driver.OSVersion{}
	}
	return driver.OSVersion{Major: int(v.Major), Minor: int(v.Minor)}
}
