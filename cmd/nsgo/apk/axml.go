package apk

import (
	"bytes"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Manifest is what packaging fills into the runtime's manifest template.
type Manifest struct {
	Package string
	Label   string
	// Names are the app's name in its languages, by BCP 47 tag, "" for
	// the default, which the launcher lists it under in place of Label.
	Names       map[string]string
	VersionName string
	VersionCode int
	// URLSchemes are the app's own links, registered on the launcher activity.
	URLSchemes []string
	// ShareTypes are the MIME types registered for incoming text and files.
	ShareTypes []string
	// Icon is the launcher icon, an IconSize PNG; nil keeps the system's.
	Icon []byte
	// Adaptive is the adaptive launcher icon's layers, which @mipmap/ic_launcher names from API 26 on, Icon below it; nil leaves Icon alone.
	Adaptive *AdaptiveIcon
	// Theme replaces the application's theme, a framework style; empty
	// keeps the runtime's.
	Theme string
	// Debuggable marks a test build, whose launch intent may name the test
	// the runtime runs.
	Debuggable bool
	// CrashReportBuild enables Java crash reports with this release build number.
	CrashReportBuild string
	// Permissions are the permissions the app asks for beyond the
	// runtime's, by their full names.
	Permissions []string
	// TargetSDK replaces the runtime's targetSdkVersion when it is set.
	TargetSDK int
	// LoopbackCleartext adds a network policy permitting HTTP to loopback hosts.
	LoopbackCleartext bool
	// Overlay declares the runtime's OverlayService, the specialUse foreground service an overlay over other apps takes, for an app that asks for SYSTEM_ALERT_WINDOW; Permissions name its FOREGROUND_SERVICE permissions.
	Overlay bool
	// Recording declares the runtime's RecordingService, the microphone foreground service microphone.RecordWAV records in, with the permissions it needs. An app without it declares no such service, as Google Play asks an app to justify each foreground-service type its manifest has.
	Recording bool
	// Location declares the runtime's LocationService, the location foreground service a background location stream runs in (core.LocationOptions.Background), for an app whose Permissions name FOREGROUND_SERVICE_LOCATION; an app without it declares no such service, which Google Play would ask it to justify.
	Location bool
	// Libraries are the shared libraries of the device the app uses where it has them, by name, as uses-library elements that do not require them.
	Libraries []string
	// Game classifies the app as a game and declares its Game Mode XML.
	Game bool
	// Gamepad declares an optional gamepad and touchscreen.
	Gamepad bool
}

// recordingPermissions are what the runtime's recording service needs: the microphone, a foreground service, from Android 14 one of the microphone type, and from Android 13 the notifications the service's notification shows with, which RecordWAV asks for with the microphone.
var recordingPermissions = []string{
	"android.permission.RECORD_AUDIO",
	"android.permission.FOREGROUND_SERVICE",
	"android.permission.FOREGROUND_SERVICE_MICROPHONE",
	"android.permission.POST_NOTIFICATIONS",
}

// The binary XML chunk and value types, from Android's ResourceTypes.h.
const (
	xmlHeader       = 0x0003
	stringPool      = 0x0001
	resourceMap     = 0x0180
	xmlStartNS      = 0x0100
	xmlEndNS        = 0x0101
	xmlStartElement = 0x0102
	xmlEndElement   = 0x0103
	xmlText         = 0x0104

	typeReference = 0x01
	typeString    = 0x03
	typeInt       = 0x10
	typeHex       = 0x11
	typeBool      = 0x12

	androidNS = "http://schemas.android.com/apk/res/android"
	noIndex   = ^uint32(0)
)

// attrSpec is one android:* attribute packaging can encode: the
// framework's resource ID for it, its binary type and how its XML value
// becomes the typed value. String attributes have no parse function.
type attrSpec struct {
	id    uint32
	kind  uint8
	parse func(string) (uint32, error)
}

// androidAttrs are the attributes the runtime's manifest uses. An
// attribute missing here fails the build instead of being dropped.
var androidAttrs = map[string]attrSpec{
	"versionCode":         {0x0101021b, typeInt, parseInt},
	"versionName":         {0x0101021c, typeString, nil},
	"minSdkVersion":       {0x0101020c, typeInt, parseInt},
	"targetSdkVersion":    {0x01010270, typeInt, parseInt},
	"name":                {0x01010003, typeString, nil},
	"launchMode":          {0x0101001d, typeInt, parseLaunchMode},
	"mimeType":            {0x01010026, typeString, nil},
	"scheme":              {0x01010027, typeString, nil},
	"label":               {0x01010001, typeString, nil},
	"exported":            {0x01010010, typeBool, parseBool},
	"authorities":         {0x01010018, typeString, nil},
	"grantUriPermissions": {0x0101001b, typeBool, parseBool},
	"theme":               {0x01010000, typeReference, parseStyle},
	"icon":                {0x01010002, typeReference, parseAppResource},
	"debuggable":          {0x0101000f, typeBool, parseBool},
	"required":            {0x0101028e, typeBool, parseBool},
	"appCategory":         {0x01010545, typeInt, parseAppCategory},
	"isGame":              {0x010103f4, typeBool, parseBool},
	"resource":            {0x01010025, typeReference, parseResourceID},

	"extractNativeLibs":           {0x010104ea, typeBool, parseBool},
	"enableOnBackInvokedCallback": {0x0101066c, typeBool, parseBool},
	"configChanges":               {0x0101001f, typeHex, parseConfigChanges},
	"windowSoftInputMode":         {0x0101022b, typeHex, parseSoftInputMode},
	"foregroundServiceType":       {0x01010599, typeHex, parseForegroundServiceType},
	"value":                       {0x01010024, typeString, nil},
	"drawable":                    {0x01010199, typeReference, parseAppResource},
	"networkSecurityConfig":       {0x01010527, typeReference, parseResourceID},
}

// foregroundServiceTypes are the ServiceInfo.FOREGROUND_SERVICE_TYPE_* names a service lists, as attrs_manifest.xml has them.
var foregroundServiceTypes = map[string]uint32{
	"dataSync":        0x1,
	"mediaPlayback":   0x2,
	"phoneCall":       0x4,
	"location":        0x8,
	"connectedDevice": 0x10,
	"mediaProjection": 0x20,
	"camera":          0x40,
	"microphone":      0x80,
	"health":          0x100,
	"remoteMessaging": 0x200,
	"systemExempted":  0x400,
	"shortService":    0x800,
	"fileManagement":  0x1000,
	"mediaProcessing": 0x2000,
	"specialUse":      0x40000000,
}

// The runtime's foreground services: RecordingService is the one microphone.RecordWAV records in (Manifest.Recording), of the microphone type, OverlayService the one overlays over other apps take (Manifest.Overlay), of the specialUse type, LocationService the one background location streams run in (Manifest.Location), of the location type, and OverlaySubtype the subtype of its specialUse type, which stores read.
const (
	RecordingService = "dev.oneo.nsgo.RecordingService"
	OverlayService   = "dev.oneo.nsgo.OverlayService"
	LocationService  = "dev.oneo.nsgo.LocationService"
	OverlaySubtype   = "Keeps a window the user opened over other apps showing"
)

// styles are the framework styles a manifest may name. A reference needs
// the framework's resource ID, which the SDK's aapt2 resolves; these stay
// stable across Android versions.
var styles = map[string]uint32{
	"@android:style/Theme.DeviceDefault.NoActionBar":       0x01030129,
	"@android:style/Theme.DeviceDefault.Light.NoActionBar": 0x0103012c,
}

// configChanges are the ActivityInfo.CONFIG_* names a manifest lists.
var configChanges = map[string]uint32{
	"mcc":                0x0001,
	"mnc":                0x0002,
	"locale":             0x0004,
	"touchscreen":        0x0008,
	"keyboard":           0x0010,
	"keyboardHidden":     0x0020,
	"navigation":         0x0040,
	"orientation":        0x0080,
	"screenLayout":       0x0100,
	"uiMode":             0x0200,
	"screenSize":         0x0400,
	"smallestScreenSize": 0x0800,
	"density":            0x1000,
	"layoutDirection":    0x2000,
	"fontScale":          0x40000000,
}

// softInputModes are the WindowManager.LayoutParams SOFT_INPUT_* names a
// manifest lists.
var softInputModes = map[string]uint32{
	"stateUnspecified":   0x00,
	"stateUnchanged":     0x01,
	"stateHidden":        0x02,
	"stateAlwaysHidden":  0x03,
	"stateVisible":       0x04,
	"stateAlwaysVisible": 0x05,
	"adjustUnspecified":  0x00,
	"adjustResize":       0x10,
	"adjustPan":          0x20,
	"adjustNothing":      0x30,
}

// EncodeManifest turns the runtime's readable manifest into the binary XML
// an APK carries, with the app's package, version and label set.
func EncodeManifest(template []byte, m Manifest) ([]byte, error) {
	root, err := appManifest(template, m)
	if err != nil {
		return nil, err
	}
	return encodeXML(root, m.Icon != nil)
}

func encodeXML(root *node, icon bool) ([]byte, error) {
	e := &encoder{index: map[string]int{}, icon: icon}
	// Attribute names come first in the string pool: the resource map
	// that gives each one its framework ID is indexed by pool position.
	// It ascends by resource ID, as aapt2 writes it and the system reads
	// it, so the document may order its attributes freely.
	names := attrNames(root)
	sort.Slice(names, func(i, j int) bool { return androidAttrs[names[i]].id < androidAttrs[names[j]].id })
	for _, name := range names {
		e.str(name)
		e.ids = append(e.ids, androidAttrs[name].id)
	}

	if len(names) > 0 {
		e.namespace(xmlStartNS, e.str("android"), e.str(androidNS))
	}
	if err := e.element(root); err != nil {
		return nil, err
	}
	if len(names) > 0 {
		e.namespace(xmlEndNS, e.str("android"), e.str(androidNS))
	}

	pool := e.pool()
	var out bytes.Buffer
	put := func(v any) { binary.Write(&out, binary.LittleEndian, v) }
	put(uint16(xmlHeader))
	put(uint16(8))
	put(uint32(8 + pool.Len() + 8 + 4*len(e.ids) + e.chunks.Len()))
	out.Write(pool.Bytes())
	put(uint16(resourceMap))
	put(uint16(8))
	put(uint32(8 + 4*len(e.ids)))
	for _, id := range e.ids {
		put(id)
	}
	out.Write(e.chunks.Bytes())
	return out.Bytes(), nil
}

// appManifest is the runtime's manifest with m filled in.
func appManifest(template []byte, m Manifest) (*node, error) {
	if err := validPackage(m.Package); err != nil {
		return nil, err
	}
	// The table holds the adaptive icon's layers beside the legacy icon, @mipmap/ic_launcher's value before API 26.
	if m.Adaptive != nil && m.Icon == nil {
		return nil, errors.New("manifest: an adaptive icon needs Icon too")
	}
	root, err := parseXML(template)
	if err != nil {
		return nil, err
	}
	if root.name.Local != "manifest" {
		return nil, fmt.Errorf("manifest: root element is %q, want manifest", root.name.Local)
	}
	setAttr(root, "", "package", m.Package)
	setAttr(root, androidNS, "versionCode", strconv.Itoa(m.VersionCode))
	setAttr(root, androidNS, "versionName", m.VersionName)
	app := child(root, "application")
	if app == nil {
		return nil, errors.New("manifest: no application element")
	}
	if len(m.Names) > 0 {
		setAttr(app, androidNS, "label", nameResource)
	} else {
		setAttr(app, androidNS, "label", m.Label)
	}
	if m.Theme != "" {
		setAttr(app, androidNS, "theme", m.Theme)
	}
	if m.Debuggable {
		setAttr(app, androidNS, "debuggable", "true")
	}
	if m.CrashReportBuild != "" {
		meta := &node{name: xml.Name{Local: "meta-data"}}
		setAttr(meta, androidNS, "name", "dev.oneo.nsgo.crash_report_build")
		setAttr(meta, androidNS, "value", m.CrashReportBuild)
		app.kids = append(app.kids, meta)
	}
	if p := child(app, "provider"); p != nil {
		// Each app's provider needs an authority of its own.
		setAttr(p, androidNS, "authorities", m.Package+".nsgo.files")
	}
	if m.Icon != nil {
		setAttr(app, androidNS, "icon", iconResource)
	}
	if m.LoopbackCleartext {
		setAttr(app, androidNS, "networkSecurityConfig", fmt.Sprintf("@0x%08x", xmlResourceID(m)))
	}
	if m.Game {
		setAttr(app, androidNS, "appCategory", "game")
		setAttr(app, androidNS, "isGame", "true")
		meta := &node{name: xml.Name{Local: "meta-data"}}
		setAttr(meta, androidNS, "name", "android.game_mode_config")
		id := xmlResourceID(m)
		if m.LoopbackCleartext {
			id++
		}
		setAttr(meta, androidNS, "resource", fmt.Sprintf("@0x%08x", id))
		app.kids = append(app.kids, meta)
	}
	if m.Gamepad {
		for _, feature := range []string{"android.hardware.gamepad", "android.hardware.touchscreen"} {
			use := &node{name: xml.Name{Local: "uses-feature"}}
			setAttr(use, androidNS, "name", feature)
			setAttr(use, androidNS, "required", "false")
			root.kids = slices.Insert(root.kids, slices.Index(root.kids, app), use)
		}
	}
	if len(m.URLSchemes) > 0 || len(m.ShareTypes) > 0 {
		launcher := xml.Attr{Name: xml.Name{Space: androidNS, Local: "name"}, Value: "dev.oneo.nsgo.NsgoActivity"}
		activity := child(app, "activity")
		if activity == nil || !slices.Contains(activity.attrs, launcher) {
			return nil, errors.New("manifest: no NsgoActivity for URL schemes or incoming shares")
		}
		// Reuse the running app's task when a link or share comes from another app.
		setAttr(activity, androidNS, "launchMode", "singleTask")
		for _, scheme := range m.URLSchemes {
			activity.kids = append(activity.kids, openLinkFilter(scheme))
		}
		if len(m.ShareTypes) > 0 {
			activity.kids = append(activity.kids, shareFilter(m.ShareTypes))
		}
	}
	perms := m.Permissions
	if m.Recording {
		perms = append(slices.Clip(perms), recordingPermissions...)
	}
	for _, perm := range perms {
		if err := validPermission(perm); err != nil {
			return nil, err
		}
		if !asksFor(root, perm) {
			// Before the application, as the runtime's own.
			use := &node{name: xml.Name{Local: "uses-permission"}}
			setAttr(use, androidNS, "name", perm)
			i := slices.Index(root.kids, app)
			root.kids = slices.Insert(root.kids, i, use)
		}
	}
	if m.Recording {
		app.kids = append(app.kids, foregroundService(RecordingService, "microphone"))
	}
	if m.Location {
		app.kids = append(app.kids, foregroundService(LocationService, "location"))
	}
	if m.Overlay {
		service := foregroundService(OverlayService, "specialUse")
		property := &node{name: xml.Name{Local: "property"}}
		setAttr(property, androidNS, "name", "android.app.PROPERTY_SPECIAL_USE_FGS_SUBTYPE")
		setAttr(property, androidNS, "value", OverlaySubtype)
		service.kids = append(service.kids, property)
		app.kids = append(app.kids, service)
	}
	for _, lib := range m.Libraries {
		if err := validPackage(lib); err != nil {
			return nil, fmt.Errorf("library %q is not a library's name", lib)
		}
		use := &node{name: xml.Name{Local: "uses-library"}}
		setAttr(use, androidNS, "name", lib)
		setAttr(use, androidNS, "required", "false")
		app.kids = append(app.kids, use)
	}
	if m.TargetSDK > 0 {
		sdk := child(root, "uses-sdk")
		if sdk == nil {
			return nil, errors.New("manifest: no uses-sdk element")
		}
		setAttr(sdk, androidNS, "targetSdkVersion", strconv.Itoa(m.TargetSDK))
	}
	return root, nil
}

func openLinkFilter(scheme string) *node {
	filter := &node{name: xml.Name{Local: "intent-filter"}}
	for _, element := range []struct{ tag, attr, value string }{
		{"action", "name", "android.intent.action.VIEW"},
		{"category", "name", "android.intent.category.DEFAULT"},
		{"category", "name", "android.intent.category.BROWSABLE"},
		{"data", "scheme", scheme},
	} {
		kid := &node{name: xml.Name{Local: element.tag}}
		setAttr(kid, androidNS, element.attr, element.value)
		filter.kids = append(filter.kids, kid)
	}
	return filter
}

func shareFilter(types []string) *node {
	filter := &node{name: xml.Name{Local: "intent-filter"}}
	add := func(tag, attr, value string) {
		kid := &node{name: xml.Name{Local: tag}}
		setAttr(kid, androidNS, attr, value)
		filter.kids = append(filter.kids, kid)
	}
	add("action", "name", "android.intent.action.SEND")
	add("action", "name", "android.intent.action.SEND_MULTIPLE")
	add("category", "name", "android.intent.category.DEFAULT")
	for _, media := range types {
		add("data", "mimeType", media)
	}
	return filter
}

func parseLaunchMode(s string) (uint32, error) {
	if s == "singleTask" {
		return 2, nil // ActivityInfo.LAUNCH_SINGLE_TASK.
	}
	return 0, fmt.Errorf("unknown launch mode %q", s)
}

// foregroundService declares the runtime's service name, not exported, as a foreground service of types, names of foregroundServiceTypes.
func foregroundService(name, types string) *node {
	service := &node{name: xml.Name{Local: "service"}}
	setAttr(service, androidNS, "name", name)
	setAttr(service, androidNS, "exported", "false")
	setAttr(service, androidNS, "foregroundServiceType", types)
	return service
}

// validPackage rejects IDs Android cannot install.
func validPackage(id string) error {
	parts := strings.Split(id, ".")
	if len(parts) < 2 {
		return fmt.Errorf("id %q is not an Android package name: it needs a dot", id)
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("id %q is not an Android package name: it has an empty part", id)
		}
		for i, r := range part {
			letter := r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z'
			digit := r >= '0' && r <= '9'
			if !letter && !(digit && i > 0) {
				return fmt.Errorf("id %q is not an Android package name: %q is not a Java identifier", id, part)
			}
		}
	}
	return nil
}

// validPermission checks that perm names a permission: dotted Java
// identifiers, as android.permission.CAMERA.
func validPermission(perm string) error {
	if err := validPackage(perm); err != nil {
		return fmt.Errorf("permission %q is not a permission's name", perm)
	}
	return nil
}

// asksFor reports whether the manifest root already asks for perm.
func asksFor(root *node, perm string) bool {
	for _, kid := range root.kids {
		if kid.name.Local != "uses-permission" {
			continue
		}
		for _, a := range kid.attrs {
			if a.Name.Space == androidNS && a.Name.Local == "name" && a.Value == perm {
				return true
			}
		}
	}
	return false
}

func parseInt(s string) (uint32, error) {
	v, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0, fmt.Errorf("%q is not a number", s)
	}
	return uint32(v), nil
}

func parseBool(s string) (uint32, error) {
	v, err := strconv.ParseBool(s)
	if err != nil {
		return 0, fmt.Errorf("%q is not true or false", s)
	}
	if v {
		return ^uint32(0), nil
	}
	return 0, nil
}

// parseAppResource resolves a reference to the app's own resources,
// which the table packaging writes defines.
func parseAppResource(s string) (uint32, error) {
	id, ok := appResources[s]
	if !ok {
		return 0, fmt.Errorf("unknown resource %q", s)
	}
	return id, nil
}

// appResources are the resources of the app's table an attribute can refer to: the launcher icon and its adaptive layers.
var appResources = map[string]uint32{
	iconResource:       iconResourceID,
	foregroundResource: foregroundResourceID,
	backgroundResource: backgroundResourceID,
	monochromeResource: monochromeResourceID,
}

func parseResourceID(value string) (uint32, error) {
	id, err := strconv.ParseUint(strings.TrimPrefix(value, "@"), 0, 32)
	return uint32(id), err
}

func parseAppCategory(s string) (uint32, error) {
	if s == "game" {
		return 0, nil // ApplicationInfo.CATEGORY_GAME
	}
	return 0, fmt.Errorf("unknown app category %q", s)
}

func parseStyle(s string) (uint32, error) {
	id, ok := styles[s]
	if !ok {
		return 0, fmt.Errorf("unknown style %q", s)
	}
	return id, nil
}

// parseFlags ORs the names of a "|"-separated attribute, such as
// configChanges, that the system defines as a bit mask.
func parseFlags(table map[string]uint32) func(string) (uint32, error) {
	return func(s string) (uint32, error) {
		var v uint32
		for _, name := range strings.Split(s, "|") {
			bit, ok := table[strings.TrimSpace(name)]
			if !ok {
				return 0, fmt.Errorf("unknown flag %q", name)
			}
			v |= bit
		}
		return v, nil
	}
}

var (
	parseConfigChanges         = parseFlags(configChanges)
	parseSoftInputMode         = parseFlags(softInputModes)
	parseForegroundServiceType = parseFlags(foregroundServiceTypes)
)

// encoder builds the element chunks and the string pool they index.
type encoder struct {
	icon    bool // the table has the icon, before the names
	strings []string
	index   map[string]int
	ids     []uint32 // the framework ID of each leading attribute name
	chunks  bytes.Buffer
}

// str returns the string pool index of s, adding it when it is new.
func (e *encoder) str(s string) uint32 {
	if i, ok := e.index[s]; ok {
		return uint32(i)
	}
	e.index[s] = len(e.strings)
	e.strings = append(e.strings, s)
	return uint32(len(e.strings) - 1)
}

func (e *encoder) namespace(kind uint16, prefix, uri uint32) {
	write := func(v any) { binary.Write(&e.chunks, binary.LittleEndian, v) }
	write(kind)
	write(uint16(16))
	write(uint32(24))
	write(uint32(0))       // line
	write(uint32(noIndex)) // comment
	write(prefix)
	write(uri)
}

// attribute is one encoded attribute's name and typed value.
type attribute struct {
	ns   int32
	id   uint32 // the framework ID that orders it
	name uint32
	raw  int32
	kind uint8
	data uint32
}

// element writes n and its children, in the pre-order the system expects.
func (e *encoder) element(n *node) error {
	name := e.str(n.name.Local)
	attrs := make([]attribute, 0, len(n.attrs))
	for _, a := range n.attrs {
		if a.Name.Local == "xmlns" || a.Name.Space == "xmlns" {
			continue
		}
		attr := attribute{ns: -1, raw: -1}
		switch a.Name.Space {
		case androidNS:
			// An attribute's namespace is a string pool index of the URI.
			attr.ns = int32(e.str(androidNS))
			spec, ok := androidAttrs[a.Name.Local]
			if !ok {
				return fmt.Errorf("manifest: packaging does not know android:%s", a.Name.Local)
			}
			attr.id = spec.id
			attr.name = e.str(a.Name.Local)
			attr.kind = spec.kind
			if a.Name.Local == "label" && a.Value == nameResource {
				attr.kind, attr.data = typeReference, nameResourceID(e.icon)
				break
			}
			if spec.kind == typeString {
				attr.data = e.str(a.Value)
				attr.raw = int32(attr.data)
				break
			}
			v, err := spec.parse(a.Value)
			if err != nil {
				return fmt.Errorf("manifest: android:%s: %w", a.Name.Local, err)
			}
			attr.data = v
		case "":
			attr.name = e.str(a.Name.Local)
			attr.kind = typeString
			attr.data = e.str(a.Value)
			attr.raw = int32(attr.data)
			if a.Name.Local == cleartextAttribute {
				value, err := parseBool(a.Value)
				if err != nil {
					return err
				}
				attr.kind, attr.data = typeBool, value
			}
		default:
			return fmt.Errorf("manifest: unknown namespace %q", a.Name.Space)
		}
		attrs = append(attrs, attr)
	}
	// The system binary-searches an element's attributes by resource ID,
	// so they ascend within the element as aapt2 writes them, and the
	// ones without a resource ID, such as package, come last.
	sort.SliceStable(attrs, func(i, j int) bool {
		if (attrs[i].id == 0) != (attrs[j].id == 0) {
			return attrs[j].id == 0
		}
		return attrs[i].id < attrs[j].id
	})

	w := func(v any) { binary.Write(&e.chunks, binary.LittleEndian, v) }
	w(uint16(xmlStartElement))
	w(uint16(16))
	w(uint32(16 + 20 + 20*len(attrs)))
	w(uint32(0))          // line
	w(uint32(noIndex))    // comment
	w(int32(-1))          // namespace
	w(name)               // element name
	w(uint16(20))         // attributeStart
	w(uint16(20))         // attributeSize
	w(uint16(len(attrs))) // attributeCount
	w(uint16(0))          // idIndex
	w(uint16(0))          // classIndex
	w(uint16(0))          // styleIndex
	for _, a := range attrs {
		w(a.ns)
		w(a.name)
		w(a.raw)
		w(uint16(8)) // Res_value size
		w(uint8(0))  // res0
		w(a.kind)
		w(a.data)
	}
	if n.text != "" {
		text := e.str(n.text)
		w(uint16(xmlText))
		w(uint16(16))
		w(uint32(28))
		w(uint32(0))       // line
		w(uint32(noIndex)) // comment
		w(text)
		w(uint16(8)) // Res_value size
		w(uint8(0))  // res0
		w(uint8(typeString))
		w(text)
	}
	for _, kid := range n.kids {
		if err := e.element(kid); err != nil {
			return err
		}
	}
	w(uint16(xmlEndElement))
	w(uint16(16))
	w(uint32(24))
	w(uint32(0))
	w(uint32(noIndex))
	w(int32(-1))
	w(name)
	return nil
}

// adaptiveIcon is res/mipmap-anydpi-v26/ic_launcher.xml's <adaptive-icon>, whose layers are the table's mipmaps, with the themed icon's monochrome layer when the app has one.
func adaptiveIcon(monochrome bool) *node {
	root := &node{name: xml.Name{Local: "adaptive-icon"}}
	layers := [][2]string{{"background", backgroundResource}, {"foreground", foregroundResource}}
	if monochrome {
		layers = append(layers, [2]string{"monochrome", monochromeResource})
	}
	for _, l := range layers {
		n := &node{name: xml.Name{Local: l[0]}}
		setAttr(n, androidNS, "drawable", l[1])
		root.kids = append(root.kids, n)
	}
	return root
}

// encodeAdaptiveIcon is the adaptive icon's binary XML, as an APK holds it.
func encodeAdaptiveIcon(monochrome bool) ([]byte, error) {
	return encodeXML(adaptiveIcon(monochrome), true)
}

// adaptiveFiles are the adaptive icon's files in an APK: its XML, binary, and its layers' PNGs.
func adaptiveFiles(adaptive *AdaptiveIcon) ([]File, error) {
	xml, err := encodeAdaptiveIcon(len(adaptive.Monochrome) > 0)
	if err != nil {
		return nil, err
	}
	return append([]File{{Name: adaptiveIconFile, Data: xml, Stored: true}}, adaptiveLayers(adaptive)...), nil
}

// adaptiveLayers are the adaptive icon's layer PNGs.
func adaptiveLayers(adaptive *AdaptiveIcon) []File {
	files := []File{
		{Name: foregroundFile, Data: adaptive.Foreground, Stored: true},
		{Name: backgroundFile, Data: adaptive.Background, Stored: true},
	}
	if len(adaptive.Monochrome) > 0 {
		files = append(files, File{Name: monochromeFile, Data: adaptive.Monochrome, Stored: true})
	}
	return files
}

// pool encodes the strings as a UTF-16 string pool, as aapt2 does.
func (e *encoder) pool() *bytes.Buffer {
	var strings bytes.Buffer
	write16 := func(v uint16) {
		var b [2]byte
		binary.LittleEndian.PutUint16(b[:], v)
		strings.Write(b[:])
	}
	offsets := make([]uint32, len(e.strings))
	for i, s := range e.strings {
		offsets[i] = uint32(strings.Len())
		units := utf16.Encode([]rune(s))
		// A string's length is one 16-bit word, or two when it does not
		// fit in 15 bits.
		if len(units) > 0x7fff {
			write16(uint16(0x8000 | len(units)>>16))
		}
		write16(uint16(len(units)))
		for _, u := range units {
			write16(u)
		}
		write16(0)
	}
	for strings.Len()%4 != 0 {
		strings.WriteByte(0)
	}

	start := 28 + 4*len(e.strings)
	out := new(bytes.Buffer)
	put := func(v any) { binary.Write(out, binary.LittleEndian, v) }
	put(uint16(stringPool))
	put(uint16(28))
	put(uint32(start + strings.Len()))
	put(uint32(len(e.strings)))
	put(uint32(0))     // styleCount
	put(uint32(0))     // UTF-16, unsorted
	put(uint32(start)) // stringsStart
	put(uint32(0))     // stylesStart
	for _, off := range offsets {
		put(off)
	}
	out.Write(strings.Bytes())
	return out
}

// node is the part of an XML document packaging needs.
type node struct {
	name  xml.Name
	attrs []xml.Attr
	kids  []*node
	text  string
}

func parseXML(data []byte) (*node, error) {
	d := xml.NewDecoder(bytes.NewReader(data))
	var root *node
	var stack []*node
	for {
		tok, err := d.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("manifest: %w", err)
		}
		switch t := tok.(type) {
		case xml.StartElement:
			n := &node{name: t.Name, attrs: t.Attr}
			if len(stack) > 0 {
				parent := stack[len(stack)-1]
				parent.kids = append(parent.kids, n)
			} else if root == nil {
				root = n
			} else {
				return nil, errors.New("manifest: more than one root element")
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		}
	}
	if root == nil {
		return nil, errors.New("manifest: no root element")
	}
	return root, nil
}

// attrNames lists the android:* attribute names in document order,
// without repeats. Their pool positions start at zero.
func attrNames(n *node) []string {
	var names []string
	seen := map[string]bool{}
	var walk func(*node)
	walk = func(n *node) {
		for _, a := range n.attrs {
			if a.Name.Space == androidNS && !seen[a.Name.Local] {
				seen[a.Name.Local] = true
				names = append(names, a.Name.Local)
			}
		}
		for _, kid := range n.kids {
			walk(kid)
		}
	}
	walk(n)
	return names
}

func child(n *node, name string) *node {
	for _, kid := range n.kids {
		if kid.name.Local == name {
			return kid
		}
	}
	return nil
}

// setAttr replaces an attribute's value, or adds the attribute.
func setAttr(n *node, space, name, value string) {
	for i, a := range n.attrs {
		if a.Name.Space == space && a.Name.Local == name {
			n.attrs[i].Value = value
			return
		}
	}
	n.attrs = append(n.attrs, xml.Attr{Name: xml.Name{Space: space, Local: name}, Value: value})
}
