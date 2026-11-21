//go:build darwin

package apple

import "github.com/ebitengine/purego/objc"

// CloudStorageAvailable is Environment.CloudStorage on Apple platforms: true when NSFileManager.defaultManager.ubiquityIdentityToken is non-nil, meaning the user is signed into iCloud and the app has iCloud access. This check never blocks on network I/O.
func CloudStorageAvailable() bool {
	fmClass := objc.GetClass("NSFileManager")
	if fmClass == 0 {
		return false
	}
	fm := objc.ID(fmClass).Send(objc.RegisterName("defaultManager"))
	if fm == 0 {
		return false
	}
	return fm.Send(objc.RegisterName("ubiquityIdentityToken")) != 0
}
