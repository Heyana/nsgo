//go:build ios

package microphone

import (
	"errors"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/audiosession"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
	"github.com/nsgo-dev/nsgo/internal/platform/privacy"
)

const (
	uikitPath      = "/System/Library/Frameworks/UIKit.framework/UIKit"
	foundationPath = "/System/Library/Frameworks/AVFoundation.framework/AVFoundation"
)

var errSession = errors.New("microphone: cannot activate the audio session")

func sel(name string) objc.SEL { return objc.RegisterName(name) }

// global reads the object a framework exports under name, 0 if it has none.
func global(path, name string) objc.ID {
	lib, err := purego.Dlopen(path, purego.RTLD_NOW)
	if err != nil {
		return 0
	}
	symbol, err := purego.Dlsym(lib, name)
	if err != nil {
		return 0
	}
	return **(**objc.ID)(unsafe.Pointer(&symbol))
}

// enterSession holds the shared AVAudioSession at the level that records, which internal/audiosession weighs against its other holders, such as a video playing, calling onInterrupt(true) when an interruption begins or the app leaves the screen and onInterrupt(false) when the interruption ends or the app returns. Its leave lets the session go off the caller's thread, as deactivating waits for the system.
func enterSession(onInterrupt func(silenced bool)) (leave func(), err error) {
	release, err := audiosession.Hold(audiosession.Recording)
	if err != nil {
		return nil, errSession
	}
	center := objc.ID(objc.GetClass("NSNotificationCenter")).Send(sel("defaultCenter"))
	interruptionTypeKey := global(foundationPath, "AVAudioSessionInterruptionTypeKey")
	interruptionBlock := objcblock.New(func(_ objc.Block, notification objc.ID) {
		silenced := true
		if notification != 0 && interruptionTypeKey != 0 {
			if info := notification.Send(sel("userInfo")); info != 0 {
				if val := info.Send(sel("objectForKey:"), interruptionTypeKey); val != 0 {
					// AVAudioSessionInterruptionTypeEnded == 0, Began == 1.
					if objc.Send[uint](val, sel("unsignedIntegerValue")) == 0 {
						silenced = false
					}
				}
			}
		}
		onInterrupt(silenced)
	})
	bgBlock := objcblock.New(func(objc.Block, objc.ID) { onInterrupt(true) })
	fgBlock := objcblock.New(func(objc.Block, objc.ID) { onInterrupt(false) })

	var tokens []objc.ID
	add := func(name objc.ID, block objc.Block) {
		if name == 0 {
			return
		}
		token := center.Send(sel("addObserverForName:object:queue:usingBlock:"), name, objc.ID(0), objc.ID(0), block)
		tokens = append(tokens, token.Send(sel("retain")))
	}
	add(global(foundationPath, "AVAudioSessionInterruptionNotification"), interruptionBlock)
	add(global(uikitPath, "UIApplicationDidEnterBackgroundNotification"), bgBlock)
	add(global(uikitPath, "UIApplicationWillEnterForegroundNotification"), fgBlock)
	interruptionBlock.Release()
	bgBlock.Release()
	fgBlock.Release()
	return func() {
		for _, token := range tokens {
			center.Send(sel("removeObserver:"), token)
			token.Send(sel("release"))
		}
		go release()
	}, nil
}

// checkUsageDescription stands in front of the permission prompt, as iOS aborts an app that declares no usage description.
func checkUsageDescription() error {
	if !privacy.DeclaresUsage("NSMicrophoneUsageDescription") {
		return ErrNoUsageDescription
	}
	return nil
}
