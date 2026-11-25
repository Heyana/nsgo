//go:build windows

package windows

import (
	"errors"
	"syscall"
	"unsafe"
)

var (
	dxgiDLL                  = syscall.NewLazyDLL("dxgi.dll")
	createDXGIFactory1       = dxgiDLL.NewProc("CreateDXGIFactory1")
	getDisplayConfigBufSizes = user32.NewProc("GetDisplayConfigBufferSizes")
	queryDisplayConfig       = user32.NewProc("QueryDisplayConfig")
	displayConfigGetDevInfo  = user32.NewProc("DisplayConfigGetDeviceInfo")
)

var (
	iidIDXGIFactory1  = parseGUID("770aae78-f26f-4dba-a829-253c83d1b387")
	iidIDXGIOutput6   = parseGUID("068346e8-aaec-4b84-add7-137f513f77a1")
	iidDXGISwapChain3 = parseGUID("94d99bdb-f1f8-4ab0-b236-7da0170edab1")
)

// Vtable indexes for DXGI enumeration, IDXGIOutput6, and IDXGISwapChain3.
const (
	// IDXGIFactory1
	dxgiFactory1EnumAdapters1 = 12
	// IDXGIAdapter
	dxgiAdapterEnumOutputs = 7
	// IDXGIOutput6
	dxgiOutput6GetDesc1 = 27
	// IDXGISwapChain3
	swap3CheckColorSpaceSupport = 37
	swap3SetColorSpace1         = 38

	dxgiSwapChainColorSpaceSupportPresent = 0x1
)

// DisplayConfig constants (wingdi.h).
const (
	qdcOnlyActivePaths                   = 0x00000002
	displayConfigDeviceInfoGetSourceName = 1
	displayConfigDeviceInfoGetAdvColor   = 9
	displayConfigDeviceInfoGetSDRWhite   = 11
	displayConfigDeviceInfoGetAdvColor2  = 15

	displayConfigActiveColorModeHDR = 2
)

type luid struct {
	lowPart  uint32
	highPart int32
}

type displayConfigDeviceInfoHeader struct {
	typ       uint32
	size      uint32
	adapterID luid
	id        uint32
}

type displayConfigPathSourceInfo struct {
	adapterID   luid
	id          uint32
	modeInfoIdx uint32
	statusFlags uint32
}

type displayConfigRational struct {
	numerator   uint32
	denominator uint32
}

type displayConfigPathTargetInfo struct {
	adapterID        luid
	id               uint32
	modeInfoIdx      uint32
	outputTechnology uint32
	rotation         uint32
	scaling          uint32
	refreshRate      displayConfigRational
	scanLineOrdering uint32
	targetAvailable  int32
	statusFlags      uint32
}

type displayConfigPathInfo struct {
	sourceInfo displayConfigPathSourceInfo
	targetInfo displayConfigPathTargetInfo
	flags      uint32
}

// displayConfigModeInfo is 64 bytes (DISPLAYCONFIG_MODE_INFO).
type displayConfigModeInfo struct {
	infoType  uint32
	id        uint32
	adapterID luid
	data      [48]byte
}

type displayConfigSourceDeviceName struct {
	header            displayConfigDeviceInfoHeader
	viewGdiDeviceName [32]uint16
}

type displayConfigGetAdvancedColorInfo struct {
	header              displayConfigDeviceInfoHeader
	value               uint32
	colorEncoding       uint32
	bitsPerColorChannel uint32
}

type displayConfigGetAdvancedColorInfo2 struct {
	header              displayConfigDeviceInfoHeader
	value               uint32
	colorEncoding       uint32
	bitsPerColorChannel uint32
	activeColorMode     uint32
}

type displayConfigSDRWhiteLevel struct {
	header        displayConfigDeviceInfoHeader
	sdrWhiteLevel uint32
}

// dxgiOutputDesc1 matches DXGI_OUTPUT_DESC1 (dxgi1_6.h).
type dxgiOutputDesc1 struct {
	deviceName            [32]uint16
	desktopCoordinates    rect
	attachedToDesktop     int32
	rotation              uint32
	monitor               uintptr
	bitsPerColor          uint32
	colorSpace            uint32
	redPrimary            [2]float32
	greenPrimary          [2]float32
	bluePrimary           [2]float32
	whitePoint            [2]float32
	minLuminance          float32
	maxLuminance          float32
	maxFullFrameLuminance float32
}

type displayHDRInfo struct {
	hdr           bool
	supported     bool
	headroom      float32
	sdrWhiteLevel float32
	maxLuminance  float32
}

// queryDisplayHDR collects HDR state, SDR white level, and peak luminance for
// every active display keyed by its GDI device name (e.g. \\.\DISPLAY1).
func queryDisplayHDR() map[string]displayHDRInfo {
	out := map[string]displayHDRInfo{}
	queryDisplayConfigHDR(out)
	queryDXGIOutputHDR(out)
	for name, info := range out {
		if info.hdr {
			if info.sdrWhiteLevel <= 0 {
				info.sdrWhiteLevel = 80
			}
			if info.maxLuminance > 0 && info.sdrWhiteLevel > 0 {
				info.headroom = max(info.maxLuminance/info.sdrWhiteLevel, 1.0)
			}
		} else {
			info.headroom = 1.0
		}
		out[name] = info
	}
	return out
}

func queryDisplayConfigHDR(out map[string]displayHDRInfo) {
	if getDisplayConfigBufSizes.Find() != nil || queryDisplayConfig.Find() != nil || displayConfigGetDevInfo.Find() != nil {
		return
	}
	var numPaths, numModes uint32
	if r, _, _ := getDisplayConfigBufSizes.Call(qdcOnlyActivePaths, uintptr(unsafe.Pointer(&numPaths)), uintptr(unsafe.Pointer(&numModes))); r != 0 || numPaths == 0 {
		return
	}
	paths := make([]displayConfigPathInfo, numPaths)
	modes := make([]displayConfigModeInfo, max(numModes, 1))
	if r, _, _ := queryDisplayConfig.Call(
		qdcOnlyActivePaths,
		uintptr(unsafe.Pointer(&numPaths)), uintptr(unsafe.Pointer(&paths[0])),
		uintptr(unsafe.Pointer(&numModes)), uintptr(unsafe.Pointer(&modes[0])),
		0,
	); r != 0 {
		return
	}
	for i := uint32(0); i < numPaths; i++ {
		p := paths[i]
		var srcName displayConfigSourceDeviceName
		srcName.header = displayConfigDeviceInfoHeader{
			typ:       displayConfigDeviceInfoGetSourceName,
			size:      uint32(unsafe.Sizeof(srcName)),
			adapterID: p.sourceInfo.adapterID,
			id:        p.sourceInfo.id,
		}
		if r, _, _ := displayConfigGetDevInfo.Call(uintptr(unsafe.Pointer(&srcName))); r != 0 {
			continue
		}
		name := syscall.UTF16ToString(srcName.viewGdiDeviceName[:])
		if name == "" {
			continue
		}
		info := out[name]

		// Try Windows 11 24H2+ ADVANCED_COLOR_INFO_2 first so Auto Color Management (ACM) is not mistaken for HDR.
		var adv2 displayConfigGetAdvancedColorInfo2
		adv2.header = displayConfigDeviceInfoHeader{
			typ:       displayConfigDeviceInfoGetAdvColor2,
			size:      uint32(unsafe.Sizeof(adv2)),
			adapterID: p.targetInfo.adapterID,
			id:        p.targetInfo.id,
		}
		if r, _, _ := displayConfigGetDevInfo.Call(uintptr(unsafe.Pointer(&adv2))); r == 0 {
			// Bit 3: highDynamicRangeSupported; activeColorMode == 2: HDR active.
			if adv2.value&(1<<3) != 0 {
				info.supported = true
			}
			if adv2.activeColorMode == displayConfigActiveColorModeHDR {
				info.hdr = true
				info.supported = true
			}
		} else {
			var adv displayConfigGetAdvancedColorInfo
			adv.header = displayConfigDeviceInfoHeader{
				typ:       displayConfigDeviceInfoGetAdvColor,
				size:      uint32(unsafe.Sizeof(adv)),
				adapterID: p.targetInfo.adapterID,
				id:        p.targetInfo.id,
			}
			if r, _, _ := displayConfigGetDevInfo.Call(uintptr(unsafe.Pointer(&adv))); r == 0 {
				// Bit 0: advancedColorSupported, Bit 1: advancedColorEnabled.
				if adv.value&0x1 != 0 {
					info.supported = true
				}
				if adv.value&0x2 != 0 {
					info.hdr = true
					info.supported = true
				}
			}
		}

		var sdr displayConfigSDRWhiteLevel
		sdr.header = displayConfigDeviceInfoHeader{
			typ:       displayConfigDeviceInfoGetSDRWhite,
			size:      uint32(unsafe.Sizeof(sdr)),
			adapterID: p.targetInfo.adapterID,
			id:        p.targetInfo.id,
		}
		if r, _, _ := displayConfigGetDevInfo.Call(uintptr(unsafe.Pointer(&sdr))); r == 0 && sdr.sdrWhiteLevel > 0 {
			info.sdrWhiteLevel = float32(sdr.sdrWhiteLevel) / 1000.0 * 80.0
		}

		out[name] = info
	}
}

func queryDXGIOutputHDR(out map[string]displayHDRInfo) {
	if createDXGIFactory1.Find() != nil {
		return
	}
	var factory *comObject
	if hr, _, _ := createDXGIFactory1.Call(uintptr(unsafe.Pointer(&iidIDXGIFactory1)), uintptr(unsafe.Pointer(&factory))); failed(hr) || factory == nil {
		return
	}
	defer factory.release()

	for ai := uintptr(0); ; ai++ {
		var adapter *comObject
		if hr := factory.call(dxgiFactory1EnumAdapters1, ai, uintptr(unsafe.Pointer(&adapter))); failed(hr) || adapter == nil {
			break
		}
		for oi := uintptr(0); ; oi++ {
			var output *comObject
			if hr := adapter.call(dxgiAdapterEnumOutputs, oi, uintptr(unsafe.Pointer(&output))); failed(hr) || output == nil {
				break
			}
			if out6 := output.query(&iidIDXGIOutput6); out6 != nil {
				var desc dxgiOutputDesc1
				if hr := out6.call(dxgiOutput6GetDesc1, uintptr(unsafe.Pointer(&desc))); !failed(hr) {
					name := syscall.UTF16ToString(desc.deviceName[:])
					if name != "" {
						info := out[name]
						if desc.colorSpace == dxgiColorSpaceRGBFullG2084NoneP2020 {
							info.hdr = true
							info.supported = true
						}
						if desc.maxLuminance > 0 {
							info.maxLuminance = desc.maxLuminance
						}
						out[name] = info
					}
				}
				out6.release()
			}
			output.release()
		}
		adapter.release()
	}
}

// createHDRCompositionSwapChain creates a DirectComposition flip-model swap
// chain configured with an HDR pixel format and DXGI color space via
// IDXGISwapChain3::SetColorSpace1.
func createHDRCompositionSwapChain(g *gfx, width, height, dxgiFmt, dxgiCS uint32) (*comObject, error) {
	factory, err := adapterFactory(g)
	if err != nil {
		return nil, err
	}
	defer factory.release()
	desc := swapChainDesc1{
		width:       max(width, 1),
		height:      max(height, 1),
		format:      dxgiFmt,
		sampleCount: 1,
		bufferUsage: dxgiUsageTarget,
		bufferCount: 2,
		scaling:     dxgiScalingStretch,
		swapEffect:  dxgiFlipSequential,
		alphaMode:   dxgiAlphaPremultiplied,
	}
	var swap *comObject
	if hr := factory.call(dxgiFactoryCreateSwapChainForComposition, g.d3d.ptr(), uintptr(unsafe.Pointer(&desc)), 0, uintptr(unsafe.Pointer(&swap))); failed(hr) || swap == nil {
		return nil, hresultError("d3d11: CreateSwapChainForComposition HDR", hr)
	}
	swap3 := swap.query(&iidDXGISwapChain3)
	if swap3 == nil {
		swap.release()
		return nil, errors.New("d3d11: IDXGISwapChain3 unavailable")
	}
	defer swap3.release()

	var support uint32
	if hr := swap3.call(swap3CheckColorSpaceSupport, uintptr(dxgiCS), uintptr(unsafe.Pointer(&support))); failed(hr) || support&dxgiSwapChainColorSpaceSupportPresent == 0 {
		swap.release()
		return nil, errors.New("d3d11: HDR swapchain color space unsupported")
	}
	if hr := swap3.call(swap3SetColorSpace1, uintptr(dxgiCS)); failed(hr) {
		swap.release()
		return nil, hresultError("d3d11: SetColorSpace1", hr)
	}
	return swap, nil
}
