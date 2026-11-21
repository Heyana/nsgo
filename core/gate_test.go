package core

import (
	"go/ast"
	"go/parser"
	gotoken "go/token"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// gated are the exported Scope methods that reach what Access holds, by the service each asks gate for.
var gated = map[string]service{
	"OpenURL":                       serviceOpenURL,
	"SetStatusBar":                  serviceStatusBar,
	"NotificationPermission":        serviceNotifications,
	"RequestNotificationPermission": serviceNotifications,
	"ShowNotification":              serviceNotifications,
	"CloseNotification":             serviceNotifications,
	"CloseNotifications":            serviceNotifications,
	"SetBadge":                      serviceNotifications,
	"OnNotificationPress":           serviceNotifications,
	"BiometricAvailability":         serviceBiometrics,
	"AuthenticateBiometric":         serviceBiometrics,
	"SignInWithApple":               serviceSignInWithApple,
	"AppleCredentialState":          serviceSignInWithApple,
	"Products":                      servicePurchases,
	"Purchase":                      servicePurchases,
	"FinishTransaction":             servicePurchases,
	"CurrentEntitlements":           servicePurchases,
	"SyncPurchases":                 servicePurchases,
	"OnPurchaseUpdate":              servicePurchases,
	"Secrets":                       serviceSecrets,
	"PermissionStatus":              servicePermissions,
	"RequestPermission":             servicePermissions,
	"OpenPermissionSettings":        servicePermissions,
	"GuidePermission":               servicePermissions,
	"CallHost":                      serviceHost,
	"CallHostTimeout":               serviceHost,
	"PostHost":                      serviceHost,
	"ImageSize":                     serviceImage,
	"PrefetchImage":                 serviceImage,
	"KeepImage":                     serviceImage,
	"SetClipboard":                  serviceClipboard,
	"SetClipboardImage":             serviceClipboard,
	"ReadClipboard":                 serviceClipboard,
	"HasClipboardText":              serviceClipboard,
	"ReadClipboardImage":            serviceClipboard,
	"HasClipboardImage":             serviceClipboard,
	"ReadClipboardFiles":            serviceClipboard,
	"PickImages":                    servicePicker,
	"TakePhoto":                     servicePicker,
	"ScanCode":                      servicePicker,
	"PickFiles":                     servicePicker,
	"PickDirectories":               servicePicker,
	"PickScreenColor":               servicePicker,
	"SaveFile":                      servicePicker,
	"StartSpeech":                   serviceSpeech,
	"AdvertiseService":              serviceDiscovery,
	"BrowseServices":                serviceDiscovery,
	"Speak":                         serviceSpeak,
	"Voices":                        serviceSpeak,
	"OpenOverlay":                   serviceOverlay,
	"OverlayPermission":             serviceOverlay,
	"RequestOverlayPermission":      serviceOverlay,
	"SetWindowPointerEvents":        serviceOverlay,
	"OpenWindow":                    serviceOverlay,
	"OpenPanel":                     serviceOverlay,
	"SetMainView":                   serviceOverlay,
	"LoadSound":                     serviceAudio,
	"NewAudioPlayer":                serviceAudio,
	"AuthSession":                   serviceAuthSession,
	"Locate":                        serviceLocation,
	"WatchLocation":                 serviceLocation,
	"WatchMotion":                   serviceMotion,
	"SetWebCookies":                 serviceWebCookies,
	"WebCookies":                    serviceWebCookies,
	"ClearWebsiteData":              serviceWebCookies,
	"FrontmostApp":                  serviceFrontmostApp,
	"CanReadFrontmostApp":           serviceFrontmostApp,
}

// exempt are the exported Scope methods that do not ask gate, by why.
var exempt = map[string][]string{
	"the graph, its scopes and what they read": {"Child", "Dispose", "Abort", "Disposed", "HasTree", "Cleanup", "Context", "Post", "AfterFunc",
		"Untrack", "PauseTracking", "Batch", "Effect", "RenderEffect", "Run", "Env", "Theme", "Dark", "Palette", "Now",
		"WallTime", "KeyLabel", "Present", "OnExit", "CurrentWindow", "MainWindow", "Quitting", "Hiding", "LaunchAtLogin"},
	"Access itself, Confine, and what ProvideHost, ProvideStorage and ProvideSecrets scope to the owner": {"ProvideAccess", "CheckAccess",
		"ProvideHost", "HasHost", "HostParams", "OnHostEvent", "ProvideStorage", "Storage", "ProvideSecrets", "Confine"},
	"the view the code shows and the user's input to it, which no capability holds": {"StartAnimation", "AnimateLayout",
		"MeasureText", "NewOffscreenImage", "RequestAnimationFrame", "RunFrame", "RunFrameAt", "Animate", "Tween", "PlayHaptic", "DeferSystemGestures", "OnKey", "OnKeyWith", "OnBackPress",
		"SetTitle", "DismissKeyboard", "Announce", "SetScrollbars", "WatchScroll", "PopupMenu", "ShowAlert", "RegisterFont",
		"SetTheme", "SetSubtreeAppearance", "SetAppearance", "SetLocale", "SelectInputSource", "RestoreInputSource",
		"ServeScheme", "ServeSchemeRequest"},
	"the app's windows, menus, lifecycle and hand-offs to the system, which no capability holds": {"SetTitleBar",
		"MinimizeWindow", "ToggleMaximize", "ToggleFullScreen", "CloseWindow", "SetWindowFrame", "SetWindowLevel",
		"SetWindowWorkspaces", "SetWindowConstraints", "OrderFront", "SetMenuBar", "SetWindowControls", "SetWindowMaterial", "SetWindowTransparent",
		"SetWindowShadow", "SetSkipTaskbar", "RequestOrientation",
		"SetApplicationMenu", "ExitApp", "RequestQuit", "OnQuitRequest", "OnQuit", "Quit", "Restart",
		"SetShutdownMode", "SetMainWindow", "OnReopen", "NewTray", "RegisterHotKey", "SetDockVisible",
		"SetLaunchAtLogin", "OnOpenFiles", "OnOpenLinks", "OnShared", "CheckForUpdates", "InstallPackage", "CanInstallPackages",
		"OpenInstallSettings", "Store", "DataDir", "CacheDir", "LogsDir", "OpenFile", "Share", "RevealFile", "TrashFile", "KeepAwake",
		"SetTaskProgress", "SetDockMenu", "AddRecentFile", "ClearRecentFiles", "LookUp", "QuickLook", "RequestReview"},
	"app-volume capacity, exposed to Go and gated by the project's declaration on Apple": {"DiskSpace"},
	"stopping what the code's own requests started, which asked the gate":                {"StopSpeaking"},
}

// TestServicesGoThroughTheGate holds every exported Scope method to a listing: a gated one must reach gate or denied with its service, through the Scope methods and functions it calls, and an exempt one must not. A new method fails until it is listed, so a service that reaches the platform cannot skip the gate unnoticed.
func TestServicesGoThroughTheGate(t *testing.T) {
	funcs, methods, consts := scopeSources(t)
	exempted := map[string]bool{}
	for _, names := range exempt {
		for _, name := range names {
			if _, ok := gated[name]; ok || exempted[name] {
				t.Errorf("%s is listed twice", name)
			}
			exempted[name] = true
		}
	}
	// The compiler's method set, not the parsed one, says what is exported, so a method declared on an alias of Scope or on its value is listed too.
	scope := reflect.TypeFor[*Scope]()
	exported := map[string]bool{}
	for i := range scope.NumMethod() {
		name := scope.Method(i).Name
		exported[name] = true
		kinds := gatesReached(name, funcs, methods, consts)
		kind, isGated := gated[name]
		switch {
		case isGated && !slices.Contains(kinds, kind):
			t.Errorf("Scope.%s is gated as %q but reaches gate with %q", name, kind, kinds)
		case exempted[name] && len(kinds) > 0:
			t.Errorf("Scope.%s is exempt but reaches gate with %q: list it in gated", name, kinds)
		case !isGated && !exempted[name]:
			t.Errorf("Scope.%s is new: list it in gated, by the service it asks gate for, or in exempt, by why it needs none", name)
		}
	}
	for name := range gated {
		if !exported[name] {
			t.Errorf("gated lists Scope.%s, which is gone", name)
		}
	}
	for name := range exempted {
		if !exported[name] {
			t.Errorf("exempt lists Scope.%s, which is gone", name)
		}
	}
}

// TestGateRules checks the gate's table: each service constant has a rule, the runtime's own checks are explicit entries, and a service with none is refused to held code.
func TestGateRules(t *testing.T) {
	_, _, consts := scopeSources(t)
	if len(consts) == 0 {
		t.Fatal("no service constants found")
	}
	for name, kind := range consts {
		if services[kind].permission == "" {
			t.Errorf("%s has no rule in services", name)
		}
	}
	// docs/architecture/tsx.md: the clipboard, the picker and speech are checked in the runtime's modules only.
	for _, kind := range []service{serviceClipboard, servicePicker, serviceSpeech} {
		if !services[kind].runtimeOnly {
			t.Errorf("service %q is checked in core", kind)
		}
	}
	var held *Scope
	h := newHost(t, Size{Width: 100, Height: 100}, func(cx *Scope) Node {
		held = cx.Child()
		held.Run(func() { held.ProvideAccess(allows(func(AccessRequest) bool { return true })) })
		return View{}
	})
	h.commit()
	if err := held.gate("unlisted", ""); ErrorCode(err) != CodeAccessDenied {
		t.Fatalf("an unlisted service gives %v to held code", err)
	}
	if err := held.gate(serviceOpenURL, "https://example.com"); err != nil {
		t.Fatalf("a granted service gives %v", err)
	}
	if err := h.root.gate("unlisted", ""); err != nil {
		t.Fatalf("an unlisted service gives %v to the app's code", err)
	}
}

// scopeSources parses the package's non-test files into its functions and Scope's methods, by name, and its service constants, by identifier; a name a build constraint gives several bodies keeps them all.
func scopeSources(t *testing.T) (funcs, methods map[string][]*ast.FuncDecl, kinds map[string]service) {
	t.Helper()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	funcs, methods, kinds = map[string][]*ast.FuncDecl{}, map[string][]*ast.FuncDecl{}, map[string]service{}
	// scopes are Scope's names, its aliases included, and receivers the methods by the type they are declared on.
	scopes := map[string]bool{"Scope": true}
	receivers := map[string][]*ast.FuncDecl{}
	fset := gotoken.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == gotoken.TYPE {
				for _, spec := range g.Specs {
					if ts := spec.(*ast.TypeSpec); ts.Assign.IsValid() {
						if id, ok := ts.Type.(*ast.Ident); ok && id.Name == "Scope" {
							scopes[ts.Name.Name] = true
						}
					}
				}
			}
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == gotoken.CONST {
				for _, spec := range g.Specs {
					v := spec.(*ast.ValueSpec)
					if id, ok := v.Type.(*ast.Ident); !ok || id.Name != "service" {
						continue
					}
					for i, n := range v.Names {
						lit, err := strconv.Unquote(v.Values[i].(*ast.BasicLit).Value)
						if err != nil {
							t.Fatal(err)
						}
						kinds[n.Name] = service(lit)
					}
				}
			}
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if fn.Recv == nil {
				funcs[fn.Name.Name] = append(funcs[fn.Name.Name], fn)
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if id, ok := recv.(*ast.Ident); ok {
				receivers[id.Name] = append(receivers[id.Name], fn)
			}
		}
	}
	for name := range scopes {
		for _, fn := range receivers[name] {
			methods[fn.Name.Name] = append(methods[fn.Name.Name], fn)
		}
	}
	return funcs, methods, kinds
}

// gatesReached are the services Scope's method name asks gate or denied for, itself or through the Scope methods and package functions it calls. Calls are matched by name, not type: a call x.m follows Scope's method m whatever x is.
func gatesReached(name string, funcs, methods map[string][]*ast.FuncDecl, consts map[string]service) []service {
	var kinds []service
	seen := map[*ast.FuncDecl]bool{}
	var visit func(fns []*ast.FuncDecl)
	visit = func(fns []*ast.FuncDecl) {
		for _, fn := range fns {
			if seen[fn] {
				continue
			}
			seen[fn] = true
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				switch f := call.Fun.(type) {
				case *ast.SelectorExpr:
					if (f.Sel.Name == "gate" || f.Sel.Name == "denied") && len(call.Args) > 0 {
						if id, ok := call.Args[0].(*ast.Ident); ok {
							if kind, ok := consts[id.Name]; ok {
								kinds = append(kinds, kind)
							}
						}
					}
					visit(methods[f.Sel.Name])
				case *ast.Ident:
					visit(funcs[f.Name])
				}
				return true
			})
		}
	}
	visit(methods[name])
	return kinds
}
