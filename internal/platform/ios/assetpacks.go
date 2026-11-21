//go:build ios

package ios

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"runtime"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

func init() {
	driver.AssetPacks.Ready = iosAssetPackReady
	driver.AssetPacks.Open = openIOSAssetPackFile
}

var iosAssetPackManager = sync.OnceValue(func() objc.ID {
	bundle := objc.ID(apple.Class("NSBundle")).Send(sel("mainBundle"))
	enabled := bundle.Send(sel("objectForInfoDictionaryKey:"), apple.NSString("BAHasManagedAssetPacks"))
	if enabled == 0 || !objc.Send[bool](enabled, sel("boolValue")) {
		return 0
	}
	if _, err := purego.Dlopen("/System/Library/Frameworks/BackgroundAssets.framework/BackgroundAssets", purego.RTLD_NOW|purego.RTLD_GLOBAL); err != nil {
		return 0
	}
	return objc.ID(apple.Class("BAAssetPackManager")).Send(sel("sharedManager"))
})

func iosAssetPackReady(ctx context.Context, name string) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	type reply struct {
		ready bool
		err   error
	}
	result := make(chan reply, 1)
	apple.OnMain(func() {
		manager := iosAssetPackManager()
		if manager == 0 {
			result <- reply{err: errors.ErrUnsupported}
			return
		}
		if query := sel("assetPackIsAvailableLocallyWithIdentifier:"); objc.Send[bool](manager, sel("respondsToSelector:"), query) {
			result <- reply{ready: objc.Send[bool](manager, query, apple.NSString(name))}
			return
		}
		done := objcblock.New(func(_ objc.Block, status uintptr, err objc.ID) {
			if err != 0 {
				result <- reply{err: errors.New(apple.GoString(err.Send(sel("localizedDescription"))))}
				return
			}
			const downloaded = 1 << 6 // BAAssetPackStatusDownloaded
			result <- reply{ready: status&downloaded != 0}
		})
		manager.Send(sel("getStatusOfAssetPackWithIdentifier:completionHandler:"), apple.NSString(name), done)
		done.Release()
	})
	select {
	case r := <-result:
		return r.ready, r.err
	case <-ctx.Done():
		return false, ctx.Err()
	}
}

func openIOSAssetPackFile(name, path string) (io.ReadCloser, error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(apple.Class("NSAutoreleasePool")).Send(sel("new"))
	defer pool.Send(sel("drain"))
	manager := iosAssetPackManager()
	if manager == 0 {
		return nil, errors.ErrUnsupported
	}
	var failure objc.ID
	fd := objc.Send[int32](manager, sel("fileDescriptorForPath:searchingInAssetPackWithIdentifier:error:"),
		apple.NSString(path), apple.NSString(name), unsafe.Pointer(&failure))
	if fd < 0 {
		if failure != 0 {
			return nil, errors.New(apple.GoString(failure.Send(sel("localizedDescription"))))
		}
		return nil, fs.ErrNotExist
	}
	return os.NewFile(uintptr(fd), path), nil
}
