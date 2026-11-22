package ios

// UIEventType values delivered to UIWindow.sendEvent: that carry physical user input on iOS, iPadOS and tvOS.
const (
	uiEventTouches       = 0
	uiEventMotion        = 1
	uiEventRemoteControl = 2
	uiEventPresses       = 3
	uiEventScroll        = 10
	uiEventHover         = 11
	uiEventTransform     = 14
)

// isPhysicalUIEventType reports whether kind is a UIKit UIEventType carrying touch, pointer, hover, scroll, transform, hardware-key/remote or motion input.
func isPhysicalUIEventType(kind int) bool {
	switch kind {
	case uiEventTouches, uiEventMotion, uiEventRemoteControl, uiEventPresses,
		uiEventScroll, uiEventHover, uiEventTransform:
		return true
	default:
		return false
	}
}
