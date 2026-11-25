//go:build linux && !android

package linux

import (
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Wayland wp_color_management_v1 (version 1) client for querying per-output
// HDR state/luminance and configuring HDR surface color spaces.
const (
	wpCMFeatureParametric   uint32 = 5
	wpCMFeatureWindowsSCRGB uint32 = 9

	wpCMPrimariesSRGB      uint32 = 1
	wpCMPrimariesBT2020    uint32 = 5
	wpCMPrimariesDisplayP3 uint32 = 7

	wpCMTFSRGB      uint32 = 4
	wpCMTFST2084PQ  uint32 = 9
	wpCMTFHLG       uint32 = 10
	wpCMTFExtLinear uint32 = 11

	wpCMRenderIntentPerceptual uint32 = 0
)

type displayHDRInfo struct {
	hdr           bool
	supported     bool
	headroom      float32
	sdrWhiteLevel float32
	maxLuminance  float32
}

var wlColorMgmt struct {
	manager        *wlInterface
	output         *wlInterface
	surface        *wlInterface
	paramsCreator  *wlInterface
	imageDesc      *wlInterface
	imageDescInfo  *wlInterface
	registryEvents [2]uintptr
	managerEvents  [5]uintptr
	outputEvents   [1]uintptr
	descEvents     [2]uintptr
	infoEvents     [11]uintptr
	poll           uintptr

	getMonitorType func() uintptr
	getWLOutput    func(uintptr) uintptr
	getSurfaceType func() uintptr
	getWLSurface   func(uintptr) uintptr
}

var waylandColorMgmtOnce sync.Once
var waylandColorMgmtReady bool

func bindWaylandColorMgmt() bool {
	waylandColorMgmtOnce.Do(func() { waylandColorMgmtReady = loadWaylandColorMgmt() })
	return waylandColorMgmtReady
}

func loadWaylandColorMgmt() bool {
	if !bindWaylandClient() {
		return false
	}
	for name, fn := range map[string]any{
		"gdk_wayland_monitor_get_type":       &wlColorMgmt.getMonitorType,
		"gdk_wayland_monitor_get_wl_output":  &wlColorMgmt.getWLOutput,
		"gdk_wayland_surface_get_type":       &wlColorMgmt.getSurfaceType,
		"gdk_wayland_surface_get_wl_surface": &wlColorMgmt.getWLSurface,
	} {
		sym, err := purego.Dlsym(gtkLib, name)
		if err != nil {
			return false
		}
		purego.RegisterFunc(fn, sym)
	}
	outputAddr, err := purego.Dlsym(wlClient.lib, "wl_output_interface")
	if err != nil {
		return false
	}
	surfaceAddr, err := purego.Dlsym(wlClient.lib, "wl_surface_interface")
	if err != nil {
		return false
	}
	wlOutput := (*wlInterface)(cPointer(outputAddr))
	wlSurface := (*wlInterface)(cPointer(surfaceAddr))

	wlColorMgmt.imageDescInfo = newWLInterface("wp_image_description_info_v1", 1,
		nil,
		[]wlMessage{
			newWLMessage("done", ""),
			newWLMessage("icc_file", "hu", nil, nil),
			newWLMessage("primaries", "iiiiiiii", nil, nil, nil, nil, nil, nil, nil, nil),
			newWLMessage("primaries_named", "u", nil),
			newWLMessage("tf_power", "u", nil),
			newWLMessage("tf_named", "u", nil),
			newWLMessage("luminances", "uuu", nil, nil, nil),
			newWLMessage("target_primaries", "iiiiiiii", nil, nil, nil, nil, nil, nil, nil, nil),
			newWLMessage("target_luminance", "uu", nil, nil),
			newWLMessage("target_max_cll", "u", nil),
			newWLMessage("target_max_fall", "u", nil),
		})
	wlColorMgmt.imageDesc = newWLInterface("wp_image_description_v1", 1,
		[]wlMessage{
			newWLMessage("destroy", ""),
			newWLMessage("get_information", "n", wlColorMgmt.imageDescInfo),
		},
		[]wlMessage{
			newWLMessage("failed", "us", nil, nil),
			newWLMessage("ready", "u", nil),
		})
	wlColorMgmt.paramsCreator = newWLInterface("wp_image_description_creator_params_v1", 1,
		[]wlMessage{
			newWLMessage("create", "n", wlColorMgmt.imageDesc),
			newWLMessage("set_tf_named", "u", nil),
			newWLMessage("set_tf_power", "u", nil),
			newWLMessage("set_primaries_named", "u", nil),
			newWLMessage("set_primaries", "iiiiiiii", nil, nil, nil, nil, nil, nil, nil, nil),
			newWLMessage("set_luminances", "uuu", nil, nil, nil),
			newWLMessage("set_mastering_display_primaries", "iiiiiiii", nil, nil, nil, nil, nil, nil, nil, nil),
			newWLMessage("set_mastering_luminance", "uu", nil, nil),
			newWLMessage("set_max_cll", "u", nil),
			newWLMessage("set_max_fall", "u", nil),
		}, nil)
	wlColorMgmt.output = newWLInterface("wp_color_management_output_v1", 1,
		[]wlMessage{
			newWLMessage("destroy", ""),
			newWLMessage("get_image_description", "n", wlColorMgmt.imageDesc),
		},
		[]wlMessage{
			newWLMessage("image_description_changed", ""),
		})
	wlColorMgmt.surface = newWLInterface("wp_color_management_surface_v1", 1,
		[]wlMessage{
			newWLMessage("destroy", ""),
			newWLMessage("set_image_description", "ou", wlColorMgmt.imageDesc, nil),
			newWLMessage("unset_image_description", ""),
		}, nil)
	// Feedback interface placeholder for manager method signature alignment.
	feedback := newWLInterface("wp_color_management_surface_feedback_v1", 1,
		[]wlMessage{newWLMessage("destroy", "")}, nil)
	iccCreator := newWLInterface("wp_image_description_creator_icc_v1", 1,
		[]wlMessage{newWLMessage("create", "n", wlColorMgmt.imageDesc)}, nil)
	wlColorMgmt.manager = newWLInterface("wp_color_manager_v1", 1,
		[]wlMessage{
			newWLMessage("destroy", ""),
			newWLMessage("get_output", "no", wlColorMgmt.output, wlOutput),
			newWLMessage("get_surface", "no", wlColorMgmt.surface, wlSurface),
			newWLMessage("get_surface_feedback", "no", feedback, wlSurface),
			newWLMessage("create_icc_creator", "n", iccCreator),
			newWLMessage("create_parametric_creator", "n", wlColorMgmt.paramsCreator),
			newWLMessage("create_windows_scrgb", "n", wlColorMgmt.imageDesc),
		},
		[]wlMessage{
			newWLMessage("supported_intent", "u", nil),
			newWLMessage("supported_feature", "u", nil),
			newWLMessage("supported_tf_named", "u", nil),
			newWLMessage("supported_primaries_named", "u", nil),
			newWLMessage("done", ""),
		})

	wlColorMgmt.registryEvents = [2]uintptr{newCallback(wlCMGlobal), newCallback(wlCMGlobalRemoved)}
	wlColorMgmt.managerEvents = [5]uintptr{
		newCallback(wlCMIgnoredU),
		newCallback(wlCMFeature),
		newCallback(wlCMTFNamed),
		newCallback(wlCMPrimariesNamed),
		newCallback(wlCMManagerDone),
	}
	wlColorMgmt.outputEvents = [1]uintptr{newCallback(wlCMOutputChanged)}
	wlColorMgmt.descEvents = [2]uintptr{newCallback(wlCMDescFailed), newCallback(wlCMDescReady)}
	wlColorMgmt.infoEvents = [11]uintptr{
		newCallback(wlCMInfoDone),
		newCallback(wlCMInfoIgnoredHU),
		newCallback(wlCMInfoIgnored8I),
		newCallback(wlCMInfoPrimariesNamed),
		newCallback(wlCMInfoIgnoredU),
		newCallback(wlCMInfoTFNamed),
		newCallback(wlCMInfoLuminances),
		newCallback(wlCMInfoIgnored8I),
		newCallback(wlCMInfoTargetLuminance),
		newCallback(wlCMInfoIgnoredU),
		newCallback(wlCMInfoIgnoredU),
	}
	wlColorMgmt.poll = newCallback(wlCMPoll)
	return true
}

type wlColorServer struct {
	display, queue, registry, manager uintptr
	global                            uint32
	features                          map[uint32]bool
	tfs                               map[uint32]bool
	primaries                         map[uint32]bool
	done                              bool
	outputs                           map[uintptr]uintptr // wl_output -> wp_color_management_output_v1
	timer                             uint32
}

type wlDescWait struct {
	ready, failed bool
}

type wlInfoWait struct {
	done           bool
	tfNamed        uint32
	primariesNamed uint32
	refLum         uint32
	maxLum         uint32
	targetMaxLum   uint32
}

var (
	wlCMServer     *wlColorServer
	wlCMChecked    bool
	wlCMRegistries = map[uintptr]*wlColorServer{}
	wlCMManagers   = map[uintptr]*wlColorServer{}
	wlCMOutputs    = map[uintptr]*wlColorServer{}
	wlCMDescs      = map[uintptr]*wlDescWait{}
	wlCMInfos      = map[uintptr]*wlInfoWait{}
	wlCMDirty      bool
)

func ensureWaylandColorServer() *wlColorServer {
	if wlCMChecked {
		return wlCMServer
	}
	wlCMChecked = true
	gdk := gdkDisplayGetDefault()
	if gdk == 0 || !bindWaylandColorMgmt() || gTypeIsA(gdk, wlClient.getDisplayType()) == 0 {
		return nil
	}
	s := &wlColorServer{
		display:   wlClient.getDisplay(gdk),
		features:  map[uint32]bool{},
		tfs:       map[uint32]bool{},
		primaries: map[uint32]bool{},
		outputs:   map[uintptr]uintptr{},
	}
	s.queue = wlClient.createQueue(s.display)
	if s.queue == 0 {
		return nil
	}
	s.registry = wlClient.marshal(s.display, 1, wlClient.registry, 1, 0, unsafe.SliceData([]uintptr{0}))
	if s.registry == 0 {
		wlClient.destroyQueue(s.queue)
		return nil
	}
	wlClient.setQueue(s.registry, s.queue)
	wlCMRegistries[s.registry] = s
	if wlClient.addListener(s.registry, &wlColorMgmt.registryEvents[0], 0) != 0 ||
		wlClient.roundtrip(s.display, s.queue) < 0 || s.manager == 0 ||
		wlClient.roundtrip(s.display, s.queue) < 0 {
		delete(wlCMRegistries, s.registry)
		if s.manager != 0 {
			delete(wlCMManagers, s.manager)
			wlClient.destroy(s.manager)
		}
		wlClient.destroy(s.registry)
		wlClient.destroyQueue(s.queue)
		return nil
	}
	s.timer = gTimeoutAdd(250, wlColorMgmt.poll, 0)
	wlCMServer = s
	return s
}

func wlCMGlobal(_, registry uintptr, name uint32, iface *byte, version uint32) {
	s := wlCMRegistries[registry]
	if s == nil || s.manager != 0 || goString(uintptr(unsafe.Pointer(iface))) != "wp_color_manager_v1" || version < 1 {
		return
	}
	args := []uintptr{uintptr(name), uintptr(unsafe.Pointer(wlColorMgmt.manager.name)), 1, 0}
	s.manager = wlClient.marshal(registry, 0, wlColorMgmt.manager, 1, 0, unsafe.SliceData(args))
	if s.manager == 0 {
		return
	}
	s.global = name
	wlCMManagers[s.manager] = s
	wlClient.addListener(s.manager, &wlColorMgmt.managerEvents[0], 0)
}

func wlCMGlobalRemoved(_, registry uintptr, name uint32) {
	if s := wlCMRegistries[registry]; s != nil && s.global == name {
		s.manager = 0
	}
}

func wlCMIgnoredU(_, _ uintptr, _ uint32) {}

func wlCMFeature(_, manager uintptr, f uint32) {
	if s := wlCMManagers[manager]; s != nil {
		s.features[f] = true
	}
}

func wlCMTFNamed(_, manager uintptr, tf uint32) {
	if s := wlCMManagers[manager]; s != nil {
		s.tfs[tf] = true
	}
}

func wlCMPrimariesNamed(_, manager uintptr, p uint32) {
	if s := wlCMManagers[manager]; s != nil {
		s.primaries[p] = true
	}
}

func wlCMManagerDone(_, manager uintptr) {
	if s := wlCMManagers[manager]; s != nil {
		s.done = true
	}
}

func wlCMOutputChanged(_, output uintptr) {
	if wlCMOutputs[output] != nil {
		wlCMDirty = true
	}
}

func wlCMDescFailed(_, desc uintptr, _ uint32, _ *byte) {
	if w := wlCMDescs[desc]; w != nil {
		w.failed = true
	}
}

func wlCMDescReady(_, desc uintptr, _ uint32) {
	if w := wlCMDescs[desc]; w != nil {
		w.ready = true
	}
}

func wlCMInfoDone(_, info uintptr) {
	if w := wlCMInfos[info]; w != nil {
		w.done = true
	}
}

func wlCMInfoIgnoredHU(_, _ uintptr, _ int32, _ uint32)            {}
func wlCMInfoIgnored8I(_, _ uintptr, _, _, _, _, _, _, _, _ int32) {}
func wlCMInfoIgnoredU(_, _ uintptr, _ uint32)                      {}
func wlCMInfoPrimariesNamed(_, info uintptr, p uint32) {
	if w := wlCMInfos[info]; w != nil {
		w.primariesNamed = p
	}
}
func wlCMInfoTFNamed(_, info uintptr, tf uint32) {
	if w := wlCMInfos[info]; w != nil {
		w.tfNamed = tf
	}
}
func wlCMInfoLuminances(_, info uintptr, _ uint32, maxLum, refLum uint32) {
	if w := wlCMInfos[info]; w != nil {
		w.maxLum = maxLum
		w.refLum = refLum
	}
}
func wlCMInfoTargetLuminance(_, info uintptr, _ uint32, maxLum uint32) {
	if w := wlCMInfos[info]; w != nil {
		w.targetMaxLum = maxLum
	}
}

func wlCMPoll(_ uintptr) int32 {
	s := wlCMServer
	if s == nil {
		return 0
	}
	wlClient.dispatch(s.display, s.queue)
	if wlCMDirty {
		wlCMDirty = false
		displaysChangedCallback(0, 0, 0, 0)
	}
	return 1
}

// queryWaylandMonitorHDR queries the active HDR state and luminance metadata of
// monitor via wp_color_management_v1 on Wayland. Returns zero on X11 or when
// the compositor does not advertise wp_color_manager_v1.
func queryWaylandMonitorHDR(monitor uintptr) displayHDRInfo {
	s := ensureWaylandColorServer()
	if s == nil || s.manager == 0 || monitor == 0 || gTypeIsA(monitor, wlColorMgmt.getMonitorType()) == 0 {
		return displayHDRInfo{}
	}
	wlOut := wlColorMgmt.getWLOutput(monitor)
	if wlOut == 0 {
		return displayHDRInfo{}
	}
	cmOut := s.outputs[wlOut]
	if cmOut == 0 {
		args := []uintptr{0, wlOut}
		cmOut = wlClient.marshal(s.manager, 1, wlColorMgmt.output, 1, 0, unsafe.SliceData(args))
		if cmOut == 0 {
			return displayHDRInfo{}
		}
		wlClient.setQueue(cmOut, s.queue)
		wlCMOutputs[cmOut] = s
		wlClient.addListener(cmOut, &wlColorMgmt.outputEvents[0], 0)
		s.outputs[wlOut] = cmOut
	}
	desc := wlClient.marshal(cmOut, 1, wlColorMgmt.imageDesc, 1, 0, unsafe.SliceData([]uintptr{0}))
	if desc == 0 {
		return displayHDRInfo{}
	}
	wlClient.setQueue(desc, s.queue)
	dw := &wlDescWait{}
	wlCMDescs[desc] = dw
	defer func() {
		delete(wlCMDescs, desc)
		wlClient.marshal(desc, 0, nil, 1, 1, nil)
	}()
	if wlClient.addListener(desc, &wlColorMgmt.descEvents[0], 0) != 0 ||
		wlClient.roundtrip(s.display, s.queue) < 0 || !dw.ready || dw.failed {
		return displayHDRInfo{}
	}
	infoObj := wlClient.marshal(desc, 1, wlColorMgmt.imageDescInfo, 1, 0, unsafe.SliceData([]uintptr{0}))
	if infoObj == 0 {
		return displayHDRInfo{}
	}
	wlClient.setQueue(infoObj, s.queue)
	iw := &wlInfoWait{}
	wlCMInfos[infoObj] = iw
	defer func() {
		delete(wlCMInfos, infoObj)
		wlClient.destroy(infoObj)
	}()
	if wlClient.addListener(infoObj, &wlColorMgmt.infoEvents[0], 0) != 0 ||
		wlClient.roundtrip(s.display, s.queue) < 0 || !iw.done {
		return displayHDRInfo{}
	}
	return interpretWaylandHDRInfo(iw.tfNamed, iw.refLum, iw.maxLum, iw.targetMaxLum)
}

func interpretWaylandHDRInfo(tfNamed, refLum, maxLum, targetMaxLum uint32) displayHDRInfo {
	var out displayHDRInfo
	if refLum > 0 {
		out.sdrWhiteLevel = float32(refLum)
	}
	peak := max(targetMaxLum, maxLum)
	if peak > 0 {
		out.maxLuminance = float32(peak)
	}
	isHDRTF := tfNamed == wpCMTFST2084PQ || tfNamed == wpCMTFHLG || tfNamed == wpCMTFExtLinear
	if isHDRTF || (out.maxLuminance > out.sdrWhiteLevel && out.sdrWhiteLevel > 0) {
		out.hdr = true
		out.supported = true
	}
	if out.hdr && out.maxLuminance > 0 && out.sdrWhiteLevel > 0 {
		out.headroom = max(out.maxLuminance/out.sdrWhiteLevel, 1.0)
	} else if !out.hdr {
		out.headroom = 1.0
	}
	return out
}

// setWaylandSurfaceColorSpace configures a GdkSurface's underlying wl_surface
// with a wp_color_management_v1 image description matching cs. Returns the
// negotiated color space and an optional cleanup callback.
func setWaylandSurfaceColorSpace(gdkSurface uintptr, cs driver.GPUColorSpace) (driver.GPUColorSpace, func()) {
	if cs == driver.GPUColorSpaceDefault || cs == driver.GPUColorSpaceSRGB {
		return driver.GPUColorSpaceSRGB, nil
	}
	s := ensureWaylandColorServer()
	if s == nil || s.manager == 0 || gdkSurface == 0 || gTypeIsA(gdkSurface, wlColorMgmt.getSurfaceType()) == 0 {
		return driver.GPUColorSpaceSRGB, nil
	}
	wlSurf := wlColorMgmt.getWLSurface(gdkSurface)
	if wlSurf == 0 {
		return driver.GPUColorSpaceSRGB, nil
	}
	desc := s.createImageDescription(cs)
	if desc == 0 {
		return driver.GPUColorSpaceSRGB, nil
	}
	defer wlClient.marshal(desc, 0, nil, 1, 1, nil)

	cmSurf := wlClient.marshal(s.manager, 2, wlColorMgmt.surface, 1, 0, unsafe.SliceData([]uintptr{0, wlSurf}))
	if cmSurf == 0 {
		return driver.GPUColorSpaceSRGB, nil
	}
	wlClient.setQueue(cmSurf, s.queue)
	setArgs := []uintptr{desc, uintptr(wpCMRenderIntentPerceptual)}
	wlClient.marshal(cmSurf, 1, nil, 1, 0, unsafe.SliceData(setArgs))
	wlClient.flush(s.display)
	cleanup := func() {
		wlClient.marshal(cmSurf, 2, nil, 1, 0, nil) // unset_image_description
		wlClient.marshal(cmSurf, 0, nil, 1, 1, nil) // destroy
		wlClient.flush(s.display)
	}
	return cs, cleanup
}

func (s *wlColorServer) createImageDescription(cs driver.GPUColorSpace) uintptr {
	var desc uintptr
	switch cs {
	case driver.GPUColorSpaceExtendedLinearSRGB:
		if s.features[wpCMFeatureWindowsSCRGB] {
			desc = wlClient.marshal(s.manager, 6, wlColorMgmt.imageDesc, 1, 0, unsafe.SliceData([]uintptr{0}))
		} else if s.features[wpCMFeatureParametric] && s.primaries[wpCMPrimariesSRGB] && s.tfs[wpCMTFExtLinear] {
			desc = s.createParametricDescription(wpCMPrimariesSRGB, wpCMTFExtLinear)
		}
	case driver.GPUColorSpaceHDR10PQ:
		if s.features[wpCMFeatureParametric] && s.primaries[wpCMPrimariesBT2020] && s.tfs[wpCMTFST2084PQ] {
			desc = s.createParametricDescription(wpCMPrimariesBT2020, wpCMTFST2084PQ)
		}
	case driver.GPUColorSpaceHDR10HLG:
		if s.features[wpCMFeatureParametric] && s.primaries[wpCMPrimariesBT2020] && s.tfs[wpCMTFHLG] {
			desc = s.createParametricDescription(wpCMPrimariesBT2020, wpCMTFHLG)
		}
	case driver.GPUColorSpaceDisplayP3:
		if s.features[wpCMFeatureParametric] && s.primaries[wpCMPrimariesDisplayP3] && s.tfs[wpCMTFSRGB] {
			desc = s.createParametricDescription(wpCMPrimariesDisplayP3, wpCMTFSRGB)
		}
	}
	if desc == 0 {
		return 0
	}
	wlClient.setQueue(desc, s.queue)
	dw := &wlDescWait{}
	wlCMDescs[desc] = dw
	defer delete(wlCMDescs, desc)
	if wlClient.addListener(desc, &wlColorMgmt.descEvents[0], 0) != 0 ||
		wlClient.roundtrip(s.display, s.queue) < 0 || !dw.ready || dw.failed {
		wlClient.marshal(desc, 0, nil, 1, 1, nil)
		return 0
	}
	return desc
}

func (s *wlColorServer) createParametricDescription(primaries, tf uint32) uintptr {
	creator := wlClient.marshal(s.manager, 5, wlColorMgmt.paramsCreator, 1, 0, unsafe.SliceData([]uintptr{0}))
	if creator == 0 {
		return 0
	}
	wlClient.setQueue(creator, s.queue)
	wlClient.marshal(creator, 3, nil, 1, 0, unsafe.SliceData([]uintptr{uintptr(primaries)}))
	wlClient.marshal(creator, 1, nil, 1, 0, unsafe.SliceData([]uintptr{uintptr(tf)}))
	// create consumes and destroys creator on the server (flags = WL_MARSHAL_FLAG_DESTROY = 1).
	return wlClient.marshal(creator, 0, wlColorMgmt.imageDesc, 1, 1, unsafe.SliceData([]uintptr{0}))
}
