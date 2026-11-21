package core

import (
	"log"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Access says what code under a scope may reach (NSH's nsh_core::access): a host gives it to code it did not write, such as a plugin, with Scope.ProvideAccess. Without one, as for an app's own code, everything is allowed. Core checks it where each request is made, whoever makes it, so a Go component showing the code's data is held to it too: images, videos and web views refused show nothing and report OnError, and the services fail as AccessDenied or answer as the system would with nothing there.
//
// Allows is asked about a web view's source, not about where its pages go: the platform decides those as they start, from WebHosts and, for the about:, blob: and data: pages a page makes itself, AccessWebSchemes.
//
// Elements are held by the Access of the owner that rendered them, services by the running owner's, the one whose render, effect, handler or Run is running, or outside one by that of the scope they are called on. A Host's handlers run outside the owner that called them, so they are held to the scope they call services on.
type Access interface {
	// Allows reports whether req is granted. It is called on the engine's goroutine, so it may read only what does not change.
	Allows(req AccessRequest) bool
	// WebHosts are the hosts a web view's pages may navigate to at the top level: "*", "*.example.com" or a host. The platform refuses navigations to other http and https hosts, and to other schemes but the pages' own (AccessWebSchemes) and the app's (Scope.ServeScheme), as they start.
	WebHosts() []string
}

// AccessWebSchemes is an Access that also says which pages of their own making a web view's pages may navigate to at the top level, by scheme: "about", "blob" and "data"; other names are ignored. The platform refuses the rest as they start, as it refuses hosts WebHosts leaves out, but for an about: page on Linux, which WebKitGTK shows before the app can tell the top level's navigation from a frame's: it is refused as it commits, posts nothing, reports no load start or navigation state, and the view goes back to the page before it. An Access without WebSchemes allows all three; one whose WebSchemes returns nil allows none. The app's own sources, WebView.Source and what Ref.Load loads, and the schemes it serves (Scope.ServeScheme) are not held to it, nor are a page's frames, nor, on the native backends, Ref.Reload, GoBack and GoForward taking the view to such a source's page again. A page reloading itself or stepping through its history is held to it, but on Android, which checks neither, and on Apple platforms for a reload of the app's source. On the web, where a web view is an iframe whose navigations the page cannot see, neither WebHosts nor WebSchemes holds its pages.
type AccessWebSchemes interface {
	Access
	WebSchemes() []string
}

// AccessRequest is what Access is asked for.
type AccessRequest struct {
	// Permission is the capability asked for: AccessNetwork, ...
	Permission string
	// Resource is what is asked of it: a URL, a host handler's name, or
	// with AccessMedia "microphone" or "camera"; "" for a capability
	// with none.
	Resource string
	// Method is a network request's HTTP method, AccessInput for a web view's trusted input, AccessMedia for what its pages capture, or AccessCookies for a cookie the web views' store takes.
	Method string
}

// The capabilities Access is asked for, by NSH's keys.
const (
	// AccessNetwork is fetch and the images and videos loaded from the
	// network, by URL.
	AccessNetwork = "network"
	// AccessWebView is the pages a web view shows, by URL, with AccessInput the trusted input Ref.SendInput gives them, with AccessMedia the devices WebView.Media lets them capture, and with AccessCookies the cookies SetWebCookies writes and WebCookies lists, by a URL each goes to, "" standing for the whole store WebCookies lists and ClearWebsiteData clears.
	AccessWebView = "webview"
	// AccessOpenURL is OpenURL, by URL.
	AccessOpenURL = "openUrl"
	// AccessClipboard is reading and writing the system clipboard.
	AccessClipboard = "clipboard"
	// AccessPicker is the system's photo, camera and file pickers, and its code scanner.
	AccessPicker = "picker"
	// AccessSpeech is the microphone and the speech recognizer.
	AccessSpeech = "speech"
	// AccessTextToSpeech is speaking text aloud with the system's voices (Speak), refused with SpeakNotAllowed, and listing them (Voices).
	AccessTextToSpeech = "textToSpeech"
	// AccessStatusBar is setting the status bar.
	AccessStatusBar = "statusBar"
	// AccessNotifications is system notifications and the app's badge.
	AccessNotifications = "notifications"
	// AccessBiometrics is Face ID, Touch ID and device biometric unlock.
	AccessBiometrics = "biometrics"
	// AccessSignInWithApple is Sign in with Apple and its credentials' states.
	AccessSignInWithApple = "signInWithApple"
	// AccessPurchases is in-app purchases: the store's products, buying them, their transactions and updates.
	AccessPurchases = "purchases"
	// AccessSecrets is the app's secure store (Scope.Secrets), with Resource the service of a refused item.
	AccessSecrets = "secrets"
	// AccessPermissions is checking, asking for and opening the settings of the system's permissions, by the permission's key (AccessRequest.Resource), and guiding the user to give them.
	AccessPermissions = "permissions"
	// AccessOverlay is windows over other apps on Android (OpenOverlay), and desktop windows that let the pointer through to them (SetWindowPointerEvents), which TSX packages cannot declare.
	AccessOverlay = "overlay"
	// AccessAudio is the audio package's sounds and players, refused with AudioNotAllowed.
	AccessAudio = "audio"
	// AccessGamepad is reading connected game controllers and their per-frame input (package gamepad).
	AccessGamepad = "gamepad"
	// AccessAuthSession is signing in through the system's authentication session (AuthSession).
	AccessAuthSession = "authSession"
	// AccessHost is the host's handlers, by name (CallHost, PostHost).
	AccessHost = "host"
	// AccessLocation is the location fixes Locate and WatchLocation give and PermissionLocation, AccessBackgroundLocation a stream that goes on in the background and PermissionBackgroundLocation, and AccessMotion the motion samples WatchMotion gives and PermissionMotion: three independent grants.
	AccessLocation           = "location"
	AccessBackgroundLocation = "backgroundLocation"
	AccessMotion             = "motion"
	// AccessFrontmostApp is the application in front on the desktop (Scope.FrontmostApp and Scope.CanReadFrontmostApp), as it says what else the user runs.
	AccessFrontmostApp = "frontmostApp"
	// AccessServiceDiscovery is DNS-SD advertising and browsing by service type.
	AccessServiceDiscovery = "serviceDiscovery"
	// AccessCursorPoint is the pointer position outside the app's windows (desktop.CursorPoint).
	AccessCursorPoint = "cursorPoint"
)

// AccessInput is AccessRequest.Method for a web view's trusted input, AccessMedia for the devices its pages capture, and AccessCookies for writing, listing or clearing the store every web view of the app shares, which no TSX capability grants.
const (
	AccessInput   = "input"
	AccessMedia   = "media"
	AccessCookies = "cookies"
)

// accessKey is where ProvideAccess keeps an owner's Access.
type accessKey struct{}

// ProvideAccess holds the current owner and what it owns to a: what they
// render and the services they call, until the owner is disposed. An
// owner below with an Access of its own is held to both.
func (cx *Scope) ProvideAccess(a Access) {
	if a == nil {
		return
	}
	o := cx.current()
	if outer := accessOf(o); outer != nil {
		a = bothAccess{outer, a}
	}
	if o.values == nil {
		o.values = map[any]any{}
	}
	o.values[accessKey{}] = a
	cx.rt.restricted = true
}

// CheckAccess returns AccessDenied when the current owner's Access refuses req, for the requests core does not make itself, such as a fetch of tsx's. Call it on the graph's goroutine, as it reads the current owner; Scope.Post enters the receiving scope on that goroutine.
func (cx *Scope) CheckAccess(req AccessRequest) error {
	if !cx.rt.restricted {
		return nil
	}
	return refusal(accessOf(cx.current()), req)
}

// accessOf is the Access nearest o, nil for none.
func accessOf(o *Scope) Access {
	if o == nil || !o.rt.restricted {
		return nil
	}
	for s := o; s != nil; s = s.parent {
		if a, ok := s.values[accessKey{}]; ok {
			return a.(Access)
		}
	}
	return nil
}

// refusal is AccessDenied when a refuses req.
func refusal(a Access, req AccessRequest) error {
	if a == nil || a.Allows(req) {
		return nil
	}
	resource := req.Resource
	if req.Method == AccessInput {
		resource = AccessInput
	}
	return AccessDenied(req.Permission, resource)
}

// A service is a kind of request core's services make for code an Access may hold. Each such service asks gate for its kind before it reaches the platform, and services says how each kind is held, so a kind missing there is refused to held code rather than allowed. gate_test.go lists every exported Scope method as gated, with its kind, or exempt, with why.
type service string

// The services core holds to Access.
const (
	serviceOpenURL            service = "openURL"
	serviceStatusBar          service = "statusBar"
	serviceNotifications      service = "notifications"
	serviceBiometrics         service = "biometrics"
	serviceSignInWithApple    service = "signInWithApple"
	servicePurchases          service = "purchases"
	serviceSecrets            service = "secrets"
	servicePermissions        service = "permissions"
	serviceHost               service = "host"
	serviceImage              service = "image"
	serviceClipboard          service = "clipboard"
	servicePicker             service = "picker"
	serviceSpeech             service = "speech"
	serviceSpeak              service = "speak"
	serviceOverlay            service = "overlay"
	serviceAudio              service = "audio"
	serviceAuthSession        service = "authSession"
	serviceLocation           service = "location"
	serviceBackgroundLocation service = "backgroundLocation"
	serviceMotion             service = "motion"
	serviceWebCookies         service = "webCookies"
	serviceFrontmostApp       service = "frontmostApp"
	serviceDiscovery          service = "serviceDiscovery"
	serviceCursorPoint        service = "cursorPoint"
)

// serviceRule is how core holds a kind of service to Access.
type serviceRule struct {
	// permission is the capability Access is asked for, with the request's resource.
	permission string
	// source says the resource is a URL the service loads, held as an element's source is (sourceRefusal).
	source bool
	// method is the request's Method, with the request's resource.
	method string
	// runtimeOnly says core lets the service through and only TSX's runtime asks Access for it, as a Go component's copy, paste, pickers and microphone act on the user's own input (docs/architecture/tsx.md).
	runtimeOnly bool
}

// services are the kinds of service gate knows, and how it holds each.
var services = map[service]serviceRule{
	serviceOpenURL:            {permission: AccessOpenURL},
	serviceStatusBar:          {permission: AccessStatusBar},
	serviceNotifications:      {permission: AccessNotifications},
	serviceBiometrics:         {permission: AccessBiometrics},
	serviceSignInWithApple:    {permission: AccessSignInWithApple},
	servicePurchases:          {permission: AccessPurchases},
	serviceSecrets:            {permission: AccessSecrets},
	servicePermissions:        {permission: AccessPermissions},
	serviceHost:               {permission: AccessHost},
	serviceImage:              {permission: AccessNetwork, source: true},
	serviceClipboard:          {permission: AccessClipboard, runtimeOnly: true},
	servicePicker:             {permission: AccessPicker, runtimeOnly: true},
	serviceSpeech:             {permission: AccessSpeech, runtimeOnly: true},
	serviceSpeak:              {permission: AccessTextToSpeech},
	serviceOverlay:            {permission: AccessOverlay},
	serviceAudio:              {permission: AccessAudio},
	serviceAuthSession:        {permission: AccessAuthSession},
	serviceLocation:           {permission: AccessLocation},
	serviceBackgroundLocation: {permission: AccessBackgroundLocation},
	serviceMotion:             {permission: AccessMotion},
	serviceWebCookies:         {permission: AccessWebView, method: AccessCookies},
	serviceFrontmostApp:       {permission: AccessFrontmostApp},
	serviceDiscovery:          {permission: AccessServiceDiscovery},
	serviceCursorPoint:        {permission: AccessCursorPoint},
}

// gate is the refusal of a service of kind for resource by the running owner's Access (and cx's own when they differ), as services holds it; a kind services lacks is refused.
func (cx *Scope) gate(kind service, resource string) error {
	if !cx.rt.restricted {
		return nil
	}
	o := cx.current()
	if err := gateScope(o, kind, resource); err != nil {
		return err
	}
	if o != cx {
		return gateScope(cx, kind, resource)
	}
	return nil
}

func gateScope(o *Scope, kind service, resource string) error {
	a := accessOf(o)
	if a == nil {
		return nil
	}
	rule, ok := services[kind]
	switch {
	case !ok:
		return AccessDenied(string(kind), resource)
	case rule.runtimeOnly:
		return nil
	case rule.source:
		return sourceRefusal(o, rule.permission, resource)
	}
	return refusal(a, AccessRequest{Permission: rule.permission, Resource: resource, Method: rule.method})
}

// denied reports whether gate refuses a service of kind for resource, logging the first refusal of each kind.
func (cx *Scope) denied(kind service, resource string) bool {
	err := cx.gate(kind, resource)
	if err == nil {
		return false
	}
	if !cx.rt.refusals[kind] {
		if cx.rt.refusals == nil {
			cx.rt.refusals = map[service]bool{}
		}
		cx.rt.refusals[kind] = true
		log.Printf("nsgo: %v", err)
	}
	return true
}

// bothAccess allows what both allow.
type bothAccess struct{ outer, inner Access }

func (b bothAccess) Allows(req AccessRequest) bool { return b.outer.Allows(req) && b.inner.Allows(req) }

func (b bothAccess) WebHosts() []string {
	return driver.IntersectHosts(b.outer.WebHosts(), b.inner.WebHosts())
}

func (b bothAccess) WebSchemes() []string {
	return driver.IntersectSchemes(webSchemes(b.outer), webSchemes(b.inner))
}

// webSchemes are the page schemes a allows, in lower case and without names AccessWebSchemes does not know: all of them for an Access without WebSchemes.
func webSchemes(a Access) []string {
	if s, ok := a.(AccessWebSchemes); ok {
		return driver.IntersectSchemes(s.WebSchemes(), driver.PageSchemes)
	}
	return driver.PageSchemes
}

// sourceRefusal is the refusal of what a node of owner o loads from uri:
// one from the network needs permission's grant, as fetch does, and a
// file on the device is refused. Bytes the app holds (data:, blob:) and
// its assets need nothing.
func sourceRefusal(o *Scope, permission, uri string) error {
	a := accessOf(o)
	if a == nil || uri == "" {
		return nil
	}
	scheme, _, _ := strings.Cut(uri, ":")
	switch strings.ToLower(scheme) {
	case "data", "blob", "asset", "about":
		return nil
	case "http", "https":
		return refusal(a, AccessRequest{Permission: permission, Resource: uri, Method: "GET"})
	}
	if servesWebScheme(o, strings.ToLower(scheme)) {
		return nil
	}
	return AccessDenied(permission, uri)
}

// refuseSource empties *uri when the Access of n's owner refuses it, and
// has report hear each refused URI once, after the commit.
func (t *Tree) refuseSource(n *node, permission string, uri, refused *string, report func(error)) {
	err := sourceRefusal(n.owner, permission, *uri)
	if err == nil {
		return
	}
	if *refused != *uri && report != nil && t.rt.post != nil {
		owner := n.owner
		t.rt.post(func() {
			if !owner.disposed {
				t.within(owner, func() { report(err) })
			}
		})
	}
	*refused, *uri = *uri, ""
}

// refuseImage empties the image sources n's Access refuses; one the app holds the bytes, pixels or offscreen drawing of loads nothing.
func (t *Tree) refuseImage(n *node, src *ImageSource, refused *string, report func(error)) {
	if len(src.Data) > 0 || src.Pixels != nil || src.Offscreen != nil {
		return
	}
	t.refuseSource(n, AccessNetwork, &src.URI, refused, report)
	if src.URI == "" {
		src.Variants = nil
		return
	}
	var kept map[float32]string
	for scale, uri := range src.Variants {
		if sourceRefusal(n.owner, AccessNetwork, uri) == nil {
			if kept == nil {
				kept = map[float32]string{}
			}
			kept[scale] = uri
		}
	}
	src.Variants = kept
}

func (t *Tree) refuseImageOwner(owner *Scope, src *ImageSource) {
	if len(src.Data) > 0 || src.Pixels != nil || src.Offscreen != nil {
		return
	}
	if sourceRefusal(owner, AccessNetwork, src.URI) != nil {
		src.URI = ""
		src.Variants = nil
		return
	}
	var kept map[float32]string
	for scale, uri := range src.Variants {
		if sourceRefusal(owner, AccessNetwork, uri) == nil {
			if kept == nil {
				kept = map[float32]string{}
			}
			kept[scale] = uri
		}
	}
	src.Variants = kept
}
