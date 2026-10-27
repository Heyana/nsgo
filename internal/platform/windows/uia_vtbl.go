//go:build windows

package windows

import (
	"math"
	"syscall"
	"unsafe"
)

// The vtables of the elements' interfaces, IUnknown's three methods
// first. ElementProviderFromPoint and RangeValue's SetValue take doubles,
// which callbacks receive only as words: their slots hold thunks that
// move them (uiaFromPointThunk, uiaSetRangeThunk in uia_*.s).
var uiaSimpleVtbl, uiaFragmentVtbl, uiaRootVtbl, uiaInvokeVtbl, uiaToggleVtbl, uiaRangeVtbl,
	uiaExpandVtbl, uiaSelectionVtbl, uiaValueVtbl [16]uintptr

func init() {
	uiaQI := syscall.NewCallback(uiaQueryInterface)
	uiaAR := syscall.NewCallback(uiaAddRef)
	uiaRel := syscall.NewCallback(uiaRelease)

	uiaSimpleVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		syscall.NewCallback(uiaProviderOptions),
		syscall.NewCallback(uiaGetPatternProvider),
		syscall.NewCallback(uiaGetPropertyValue),
		syscall.NewCallback(uiaHostProvider),
	}
	uiaFragmentVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		syscall.NewCallback(uiaNavigate),
		syscall.NewCallback(uiaRuntimeID),
		syscall.NewCallback(uiaBoundingRectangle),
		syscall.NewCallback(uiaEmbeddedRoots),
		syscall.NewCallback(uiaSetFocus),
		syscall.NewCallback(uiaFragmentRoot),
	}
	uiaRootVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		fromPointSlot(),
		syscall.NewCallback(uiaGetFocus),
	}
	uiaInvokeVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		syscall.NewCallback(uiaInvoke),
	}
	uiaToggleVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		syscall.NewCallback(uiaInvoke),
		syscall.NewCallback(uiaToggleState),
	}
	uiaRangeVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		setRangeSlot(),
		syscall.NewCallback(uiaRangeValue),
		syscall.NewCallback(uiaReadOnly),
		syscall.NewCallback(uiaRangeMax),
		syscall.NewCallback(uiaRangeMin),
		syscall.NewCallback(uiaRangeStep),
		syscall.NewCallback(uiaRangeStep),
	}
	uiaExpandVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		syscall.NewCallback(uiaExpand),
		syscall.NewCallback(uiaCollapse),
		syscall.NewCallback(uiaExpandState),
	}
	uiaSelectionVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		syscall.NewCallback(uiaSelect),
		syscall.NewCallback(uiaSelect),
		syscall.NewCallback(uiaNotImplemented),
		syscall.NewCallback(uiaIsSelected),
		syscall.NewCallback(uiaSelectionContainer),
	}
	uiaValueVtbl = [16]uintptr{uiaQI, uiaAR, uiaRel,
		syscall.NewCallback(uiaSetFieldValue),
		syscall.NewCallback(uiaFieldValue),
		syscall.NewCallback(uiaReadOnly),
	}
}

// uiaFromPointWords is ElementProviderFromPoint with its doubles' bits.
func uiaFromPointWords(i *uiaIface, x, y uintptr, out *unsafe.Pointer) uintptr {
	return uiaElementFromPoint(i, math.Float64frombits(uint64(x)), math.Float64frombits(uint64(y)), out)
}

// uiaSetRangeWords is RangeValue's SetValue with its double's bits.
func uiaSetRangeWords(i *uiaIface, value uintptr) uintptr {
	return i.el.setRange(math.Float64frombits(uint64(value)))
}
