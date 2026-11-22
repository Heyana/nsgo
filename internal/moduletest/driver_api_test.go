package moduletest

// allowedPublicDriverType lists the driver types the public API may
// reach. Adding an alias alone does not make a new type public.
func allowedPublicDriverType(name string) bool {
	switch name {
	// Values apps and tests build or read.
	case
		"AccessibilityAction", "AccessibilityValue", "Accessible", "Affinity",
		"AlertAction", "AlertConfig", "AlertRole", "AndroidEventMapper",
		"AndroidPropMapper", "AndroidViewProp", "AnimatedArg", "AnimatedProp",
		"AnimatedPropSetter", "Animation", "AnimationKind",
		"Appearance", "AtlasSprite", "AtlasSprites", "AttributedText", "AudioSession", "AutoCapitalize",
		"AutoComplete", "BackdropMaterial", "BlendMode", "BorderStyle",
		"Bounds", "BoxShadow", "CanvasGradient", "CanvasNode",
		"Caret", "Channel", "ClipboardImage", "CodeKind",
		"Color", "ColorMatrix", "ColorSpace", "ColorStop",
		"CompositeOp", "Corners", "Cursor", "Curve",
		"CustomCursor", "DecayConfig", "Directory", "DisplayInfo",
		"DrawArg", "DrawKind", "DrawOp", "DrawSlot", "OffscreenCanvas", "CanvasMesh", "CanvasVertex", "WebSchemeRequest", "WebSchemeResponse", "SharedItem", "DiskSpace", "VideoHeaders", "GPUDeviceLostReason", "WebWindowRequest", "SelectedRange", "WebSchemeHandler",
		"Drawing", "EaseMode", "Easing", "EasingKind",
		"EdgeColors", "EdgeEffect", "Edges", "EditAction", "Effects",
		"EllipsizeMode", "Environment", "Extrapolate", "File",
		"FillRule", "Filter", "FilterKind", "Font",
		"FontExtent", "FontVariant", "FontWeight", "FrontmostAppInfo",
		"GameAchievement", "GamePlayer", "GamePlayerScope", "GameScoreEntry",
		"GameServices", "GameTimeScope", "Geometry", "GesturePhase", "GlassEffect", "Gradient",
		"GradientCorner", "GradientExtent", "GradientKind", "GradientPaint",
		"GradientSpread", "GradientStop", "HapticEvent", "HapticPattern", "HeaderSystemItem", "HingeState",
		"HotKeyState", "Idiom", "ImageSampling", "ImageScales", "ImageSource",
		"Interpolation", "KeyboardAppearance", "KeyboardDismissMode", "KeyboardTaps",
		"KeyboardType", "Keyframe", "KeyframeAnimation", "Keyframes",
		"LayoutAnimation", "LayoutCurve", "LayoutProperty", "LayoutTransition",
		"Length", "Lifecycle", "LineCap", "LineDash",
		"LineJoin", "LiveRegion", "LocaleSettings", "LocationFix",
		"LocationOptions", "LowPowerMode", "Matrix3D", "MemorySecrets", "MenuBarMode",
		"MenuIcon", "MenuRole", "ModalAnimation", "ModalPresentation", "Modifiers",
		"MotionOptions", "MotionSample", "MotionSensors", "MouseButton",
		"StepSample", "ActivityKinds", "ActivityConfidence", "ActivitySample",
		"PressureSample", "GeofenceRegion", "GeofenceState", "GeofenceOptions", "GeofenceSample", "BackgroundMotionOptions",
		"NativeComponent", "NativeComponents", "NativeContext", "NativeFactory", "Network",
		"NodeID", "OS", "OSVersion", "ObjectPosition",
		"Obstruction", "OfferPaymentMode", "Orientation", "Outline",
		"Paint", "PaintStyle", "PanActivation", "PasteBehavior",
		"PasteEvent", "PastedFile", "Path", "PeriodUnit",
		"PickFormat", "PickedFile", "PictureInPictureState", "Pictures",
		"Point", "Pointer", "PointerEvents", "Points", "PositionAxis",
		"PreparedText", "Product", "ProductType", "PromotionalOffer",
		"RSXform", "Region", "ResizeMode", "ReturnKeyType", "Ripple",
		"Role", "SVG", "ScannedCode", "ScreenAnimation",
		"ScrollBehavior", "ScrollEdge", "SecretAccessible", "SecretItem",
		"SecretStore", "Selectable", "SensorNumber", "SensorQuaternion",
		"SensorVector", "ShaderNode", "ShaderRole", "SharedFile",
		"Size", "SnapAlignment", "SpringConfig", "StatusBarStyle",
		"StepPosition", "SubmitBehavior", "Subscription", "SubscriptionOffer", "SystemGestureEdge", "SystemGestureOptions",
		"SubscriptionPeriod", "TextAlign", "TextAttachment", "TextBackground",
		"TextBaseline", "TextDecoration", "TextDecorationStyle", "TextEngine",
		"TextLayout", "TextLine", "TextMetrics", "TextRange",
		"TextShadow", "TextSpan", "TextStyle", "ThermalState", "TimingConfig",
		"TitleBarStyle", "Touch", "Transaction", "Transform",
		"Transition", "TransitionProperty", "VerticalAlign", "VerticalBarEdge",
		"ViewPaint", "ViewPaintKind", "Voice", "WebCookie",
		"WebCookieFilter", "WebDownload", "WebDownloadState", "WebInput",
		"WebInputKind", "WebMedia", "WebNavigation", "WebSameSite",
		"WebSource", "WindowConstraints", "WindowControls", "WindowControlsStyle",
		"WindowDrag", "WindowFrame", "WindowLevel", "WindowMaterial",
		"WindowPlacement", "WindowPointerEvents", "WindowWorkspaces":
		return true
	// Protocol types still exported by the existing API. Move callers
	// to app-facing types as these entries are removed (#181).
	case
		"EventKind", "EventMask", "ImageConfig",
		"InputConfig", "ModalConfig", "PictureInPictureConfig", "ScreenConfig",
		"ScrollConfig", "StickyCell",
		"VideoConfig":
		return true
	}
	return false
}
