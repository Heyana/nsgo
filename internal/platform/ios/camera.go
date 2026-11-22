//go:build ios

package ios

import (
	"log"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/apple"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

// Taking a photo: UIImagePickerController's camera, after the user allowed it through privacy, which asks AVCaptureDevice, as the picker itself shows only a notice when they have not.

// UIImagePickerControllerSourceTypeCamera.
const sourceCamera = 1

var (
	cameraOnce     sync.Once
	classCamera    objc.Class
	uiImageJPEG    func(image objc.ID, quality float64) objc.ID
	cameraRequests = map[objc.ID]pickRequest{}
)

// cameraAvailable is Environment.Camera: the device has a camera the
// picker shows, which the simulator has not.
var cameraAvailable = sync.OnceValue(func() bool {
	if onTV() {
		return false // UIImagePickerController is unavailable on tvOS.
	}
	return objc.Send[bool](objc.ID(apple.Class("UIImagePickerController")), sel("isSourceTypeAvailable:"), sourceCamera)
})

// takePhoto has the user allow the camera, then presents it for one photo, or answers with why it cannot (withCamera).
func (s *surface) takePhoto(r driver.PickRequest, id uint32) {
	withCamera("taking a photo", func(why string) { s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: id, Text: why}) },
		func() { s.presentCamera(r, id) })
}

// withCamera runs use once the user allowed the camera, asking once, for what (such as "taking a photo"); a device without one, a refusal or an Info.plist without NSCameraUsageDescription, for which iOS would end the app as it asks, calls fail with why instead.
func withCamera(what string, fail func(why string), use func()) {
	if !cameraAvailable() {
		fail("this device has no camera")
		return
	}
	if !privacy.DeclaresUsage("NSCameraUsageDescription") {
		log.Printf("nsgo: %s needs NSCameraUsageDescription in the app's Info.plist (nsgo.yaml's apple.info)", what)
		fail("the app declares no NSCameraUsageDescription")
		return
	}
	// The camera's check and prompt are privacy's, which core's permission requests share.
	switch privacy.Status(driver.PermissionCamera) {
	case driver.PermissionGranted:
		use()
	case driver.PermissionNotAsked:
		privacy.Request(driver.PermissionCamera, func(status int) {
			apple.OnMain(func() {
				if status == driver.PermissionGranted {
					use()
				} else {
					fail("camera access denied")
				}
			})
		})
	case driver.PermissionUnavailable:
		fail("AVFoundation is missing")
	default:
		fail("camera access denied")
	}
}

// presentCamera presents the camera, whose delegate answers id with the
// photo taken, prepared off the main thread, or with none for a cancel.
func (s *surface) presentCamera(r driver.PickRequest, id uint32) {
	cameraOnce.Do(func() {
		if lib, err := purego.Dlopen(uikitPath, purego.RTLD_NOW); err == nil {
			purego.RegisterLibFunc(&uiImageJPEG, lib, "UIImageJPEGRepresentation")
		}
		var protocols []*objc.Protocol
		for _, name := range []string{"UIImagePickerControllerDelegate", "UINavigationControllerDelegate"} {
			if p := objc.GetProtocol(name); p != nil {
				protocols = append(protocols, p)
			}
		}
		answer := func(self, picker, info objc.ID) {
			picker.Send(sel("dismissViewControllerAnimated:completion:"), true, objc.ID(0))
			req, ok := cameraRequests[self]
			if !ok {
				return
			}
			delete(cameraRequests, self)
			defer self.Send(selRelease)
			var data []byte
			if info != 0 && uiImageJPEG != nil {
				// The image's orientation goes into the JPEG, which
				// PreparePicked turns upright as it encodes it again.
				if img := info.Send(sel("objectForKey:"), apple.NSString("UIImagePickerControllerOriginalImage")); img != 0 {
					data = apple.Bytes(uiImageJPEG(img, 1))
				}
			}
			if len(data) == 0 {
				s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: req.id})
				return
			}
			go func() {
				// Always encode again, upright and at the request's quality.
				r := req.r
				r.Quality = r.JPEGQuality()
				f := apple.PreparePicked(r, driver.PickedFile{Name: "photo.jpg", Data: data})
				s.engine.Dispatch(driver.Event{Kind: driver.EventPick, Count: req.id, Data: []driver.PickedFile{f}})
			}()
		}
		classCamera = register("NsgoCameraDelegate", apple.Class("NSObject"), protocols, []objc.MethodDef{{
			Cmd: sel("imagePickerController:didFinishPickingMediaWithInfo:"),
			Fn:  func(self objc.ID, _ objc.SEL, picker, info objc.ID) { answer(self, picker, info) },
		}, {
			Cmd: sel("imagePickerControllerDidCancel:"),
			Fn:  func(self objc.ID, _ objc.SEL, picker objc.ID) { answer(self, picker, 0) },
		}})
	})
	picker := objc.ID(apple.Class("UIImagePickerController")).Send(selAlloc).Send(sel("init"))
	defer picker.Send(selRelease)
	picker.Send(sel("setSourceType:"), sourceCamera)
	delegate := objc.ID(classCamera).Send(selNew)
	cameraRequests[delegate] = pickRequest{r, id}
	picker.Send(sel("setDelegate:"), delegate)
	s.topController().Send(sel("presentViewController:animated:completion:"), picker, true, objc.ID(0))
}
