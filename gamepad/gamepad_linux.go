//go:build linux && !android && !nsgoandroidhost

package gamepad

import (
	"math"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"structs"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Linux input event constants from <linux/input.h> and <linux/input-event-codes.h>.
const (
	evSyn = 0x00
	evKey = 0x01
	evAbs = 0x03
	evFF  = 0x15

	synDropped = 3

	// Force-feedback rumble constants.
	ffRumble = 0x50
	ffMax    = 0x7f

	// Gamepad face & shoulder buttons.
	btnJoystick = 0x120
	btnSouth    = 0x130 // BTN_SOUTH / BTN_A
	btnEast     = 0x131 // BTN_EAST / BTN_B
	btnNorth    = 0x133 // BTN_NORTH / BTN_X (top face button)
	btnWest     = 0x134 // BTN_WEST / BTN_Y (left face button)
	btnTL       = 0x136 // Left bumper
	btnTR       = 0x137 // Right bumper
	btnTL2      = 0x138 // Left trigger (digital)
	btnTR2      = 0x139 // Right trigger (digital)
	btnSelect   = 0x13a
	btnStart    = 0x13b
	btnMode     = 0x13c // Home / Guide
	btnThumbL   = 0x13d
	btnThumbR   = 0x13e

	// Digital D-pad buttons.
	btnDPadUp    = 0x220
	btnDPadDown  = 0x221
	btnDPadLeft  = 0x222
	btnDPadRight = 0x223

	keyMax = 0x2ff

	// Absolute axes.
	absX     = 0x00
	absY     = 0x01
	absZ     = 0x02 // Left trigger (analog)
	absRX    = 0x03
	absRY    = 0x04
	absRZ    = 0x05 // Right trigger (analog)
	absHat0X = 0x10
	absHat0Y = 0x11
	absMax   = 0x3f

	// Linux ioctl direction/bit encoding (identical on amd64 and arm64).
	iocNRShift   = 0
	iocTypeShift = 8
	iocSizeShift = 16
	iocDirShift  = 30
	iocWrite     = 1
	iocRead      = 2
)

func evIor(nr, size uintptr) uintptr {
	return (iocRead << iocDirShift) | (uintptr('E') << iocTypeShift) | (nr << iocNRShift) | (size << iocSizeShift)
}

func evIow(nr, size uintptr) uintptr {
	return (iocWrite << iocDirShift) | (uintptr('E') << iocTypeShift) | (nr << iocNRShift) | (size << iocSizeShift)
}

func eviocGID() uintptr                  { return evIor(0x02, unsafe.Sizeof(evdevInputID{})) }
func eviocGName(size uintptr) uintptr    { return evIor(0x06, size) }
func eviocGKey(size uintptr) uintptr     { return evIor(0x18, size) }
func eviocGBit(ev, size uintptr) uintptr { return evIor(0x20+ev, size) }
func eviocGAbs(abs uintptr) uintptr      { return evIor(0x40+abs, unsafe.Sizeof(evdevAbsInfo{})) }
func eviocSFF() uintptr                  { return evIow(0x80, unsafe.Sizeof(evdevFFEffect{})) }
func eviocRMFF() uintptr                 { return evIow(0x81, unsafe.Sizeof(int32(0))) }

type evdevInputID struct {
	_       structs.HostLayout
	bustype uint16
	vendor  uint16
	product uint16
	version uint16
}

type evdevAbsInfo struct {
	_          structs.HostLayout
	value      int32
	minimum    int32
	maximum    int32
	fuzz       int32
	flat       int32
	resolution int32
}

type evdevInputEvent struct {
	_     structs.HostLayout
	time  syscall.Timeval
	typ   uint16
	code  uint16
	value int32
}

// evdevFFEffect matches Linux <linux/input.h> struct ff_effect with its
// ff_rumble_effect union arm. Union size matches struct ff_periodic_effect
// (32 bytes on 64-bit, 28 bytes on 32-bit; total 48 / 44 bytes).
type evdevFFEffect struct {
	_               structs.HostLayout
	typ             uint16
	id              int16
	direction       uint16
	triggerButton   uint16
	triggerInterval uint16
	replayLength    uint16
	replayDelay     uint16
	_pad            uint16
	strongMagnitude uint16
	weakMagnitude   uint16
	_uPad           [5]uint32
	_uPtr           uintptr
}

type evdevDevice struct {
	id         int
	path       string
	fd         int
	name       string
	ffEffectID int16
	absInfo    map[uint16]evdevAbsInfo
	hasTrigger [2]bool // true when ABS_Z / ABS_RZ reports analog triggers
	state      driver.GamepadState
}

var linuxDefault = &linuxSource{
	devices: map[string]*evdevDevice{},
}

func platformSource() driver.GamepadSource {
	return linuxDefault
}

type linuxSource struct {
	mu      sync.Mutex
	devices map[string]*evdevDevice
	scanned bool
}

func (l *linuxSource) Poll() []driver.GamepadState {
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.scanned {
		l.rescanLocked()
		l.scanned = true
	}
	for path, dev := range l.devices {
		if !dev.drain() {
			dev.close()
			delete(l.devices, path)
		}
	}
	if len(l.devices) == 0 {
		return nil
	}
	out := make([]driver.GamepadState, 0, len(l.devices))
	for _, dev := range l.devices {
		out = append(out, dev.state)
	}
	slices.SortFunc(out, func(a, b driver.GamepadState) int { return a.ID - b.ID })
	return out
}

func (l *linuxSource) Watch(onChange func()) func() {
	if onChange == nil {
		return func() {}
	}
	done := make(chan struct{})
	l.mu.Lock()
	l.rescanLocked()
	l.scanned = true
	l.mu.Unlock()
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				l.mu.Lock()
				changed := l.rescanLocked()
				l.mu.Unlock()
				if changed {
					onChange()
				}
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

func (l *linuxSource) Rumble(id int, lowFreq, highFreq float32, duration time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	dev := l.findByIDLocked(id)
	if dev == nil || !dev.state.CanRumble {
		return
	}
	ms := duration.Milliseconds()
	if ms <= 0 && duration > 0 {
		ms = 1
	}
	if ms > 0xffff {
		ms = 0xffff
	}
	eff := evdevFFEffect{
		typ:             ffRumble,
		id:              dev.ffEffectID,
		replayLength:    uint16(ms),
		strongMagnitude: uint16(math.Round(float64(clamp01(lowFreq)) * 0xffff)),
		weakMagnitude:   uint16(math.Round(float64(clamp01(highFreq)) * 0xffff)),
	}
	if err := ioctlPtr(dev.fd, eviocSFF(), unsafe.Pointer(&eff)); err != nil {
		return
	}
	dev.ffEffectID = eff.id
	dev.writeFFPlay(1)
}

func (l *linuxSource) StopRumble(id int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	dev := l.findByIDLocked(id)
	if dev == nil || dev.ffEffectID < 0 {
		return
	}
	dev.writeFFPlay(0)
}

func (l *linuxSource) findByIDLocked(id int) *evdevDevice {
	for _, dev := range l.devices {
		if dev.id == id {
			return dev
		}
	}
	return nil
}

func (l *linuxSource) rescanLocked() bool {
	changed := false
	entries, err := os.ReadDir("/dev/input")
	if err != nil {
		return false
	}
	present := map[string]bool{}
	var paths []string
	for _, e := range entries {
		name := e.Name()
		if !strings.HasPrefix(name, "event") {
			continue
		}
		if _, err := strconv.Atoi(name[len("event"):]); err != nil {
			continue
		}
		p := filepath.Join("/dev/input", name)
		present[p] = true
		paths = append(paths, p)
	}
	slices.SortFunc(paths, func(a, b string) int {
		na, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(a), "event"))
		nb, _ := strconv.Atoi(strings.TrimPrefix(filepath.Base(b), "event"))
		return na - nb
	})

	for path, dev := range l.devices {
		if !present[path] {
			dev.close()
			delete(l.devices, path)
			changed = true
		}
	}
	for _, path := range paths {
		if _, exists := l.devices[path]; exists {
			continue
		}
		dev := openEvdevGamepad(path, l.nextFreeIDLocked())
		if dev != nil {
			l.devices[path] = dev
			changed = true
		}
	}
	return changed
}

func (l *linuxSource) nextFreeIDLocked() int {
	used := make(map[int]bool, len(l.devices))
	for _, dev := range l.devices {
		used[dev.id] = true
	}
	for id := 0; ; id++ {
		if !used[id] {
			return id
		}
	}
}

func openEvdevGamepad(path string, id int) *evdevDevice {
	writable := true
	fd, err := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if err != nil {
		writable = false
		fd, err = syscall.Open(path, syscall.O_RDONLY|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
		if err != nil {
			return nil
		}
	}
	var evBits [4]byte
	if err := ioctlBytes(fd, eviocGBit(0, uintptr(len(evBits))), evBits[:]); err != nil {
		_ = syscall.Close(fd)
		return nil
	}
	if !testBit(evBits[:], evKey) {
		_ = syscall.Close(fd)
		return nil
	}
	var keyBits [(keyMax / 8) + 1]byte
	if err := ioctlBytes(fd, eviocGBit(evKey, uintptr(len(keyBits))), keyBits[:]); err != nil {
		_ = syscall.Close(fd)
		return nil
	}
	var absBits [(absMax / 8) + 1]byte
	if testBit(evBits[:], evAbs) {
		_ = ioctlBytes(fd, eviocGBit(evAbs, uintptr(len(absBits))), absBits[:])
	}
	isGamepad := testBit(keyBits[:], btnSouth) ||
		testBit(keyBits[:], btnEast) ||
		(testBit(keyBits[:], btnJoystick) && testBit(absBits[:], absX) && testBit(absBits[:], absY))
	if !isGamepad {
		_ = syscall.Close(fd)
		return nil
	}

	var nameBuf [256]byte
	name := "Gamepad"
	if err := ioctlBytes(fd, eviocGName(uintptr(len(nameBuf))), nameBuf[:]); err == nil {
		if n := strings.TrimRight(string(nameBuf[:]), "\x00"); n != "" {
			name = n
		}
	}

	var inID evdevInputID
	var vid, pid uint16
	if ioctlPtr(fd, eviocGID(), unsafe.Pointer(&inID)) == nil {
		vid = inID.vendor
		pid = inID.product
	}

	canRumble := false
	if writable && testBit(evBits[:], evFF) {
		var ffBits [(ffMax / 8) + 1]byte
		if ioctlBytes(fd, eviocGBit(evFF, uintptr(len(ffBits))), ffBits[:]) == nil && testBit(ffBits[:], ffRumble) {
			canRumble = true
		}
	}

	dev := &evdevDevice{
		id:         id,
		path:       path,
		fd:         fd,
		name:       name,
		ffEffectID: -1,
		absInfo:    map[uint16]evdevAbsInfo{},
		state: driver.GamepadState{
			ID:        id,
			Name:      name,
			VendorID:  vid,
			ProductID: pid,
			Family:    driver.GamepadFamily(DetectFamily(vid, pid, name)),
			CanRumble: canRumble,
		},
	}
	for _, code := range []uint16{absX, absY, absZ, absRX, absRY, absRZ, absHat0X, absHat0Y} {
		if !testBit(absBits[:], int(code)) {
			continue
		}
		var info evdevAbsInfo
		if err := ioctlPtr(fd, eviocGAbs(uintptr(code)), unsafe.Pointer(&info)); err == nil {
			dev.absInfo[code] = info
			if code == absZ {
				dev.hasTrigger[0] = true
			} else if code == absRZ {
				dev.hasTrigger[1] = true
			}
			dev.applyAbs(code, info.value)
		}
	}
	dev.resyncKeys()
	return dev
}

func (d *evdevDevice) writeFFPlay(value int32) {
	if d.ffEffectID < 0 {
		return
	}
	ev := evdevInputEvent{
		typ:   evFF,
		code:  uint16(d.ffEffectID),
		value: value,
	}
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&ev)), unsafe.Sizeof(ev))
	_, _ = syscall.Write(d.fd, buf)
}

func (d *evdevDevice) close() {
	if d.ffEffectID >= 0 {
		d.writeFFPlay(0)
		_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, uintptr(d.fd), eviocRMFF(), uintptr(d.ffEffectID))
		d.ffEffectID = -1
	}
	_ = syscall.Close(d.fd)
}

func (d *evdevDevice) resyncKeys() {
	var pressed [(keyMax / 8) + 1]byte
	if err := ioctlBytes(d.fd, eviocGKey(uintptr(len(pressed))), pressed[:]); err != nil {
		return
	}
	for _, code := range []uint16{
		btnSouth, btnEast, btnWest, btnNorth,
		btnTL, btnTR, btnTL2, btnTR2,
		btnSelect, btnStart, btnThumbL, btnThumbR, btnMode,
		btnDPadUp, btnDPadDown, btnDPadLeft, btnDPadRight,
	} {
		if testBit(pressed[:], int(code)) {
			d.applyKey(code, 1)
		} else {
			d.applyKey(code, 0)
		}
	}
}

func (d *evdevDevice) resyncAbs() {
	for code := range d.absInfo {
		var info evdevAbsInfo
		if err := ioctlPtr(d.fd, eviocGAbs(uintptr(code)), unsafe.Pointer(&info)); err == nil {
			d.absInfo[code] = info
			d.applyAbs(code, info.value)
		}
	}
}

func (d *evdevDevice) drain() bool {
	var events [64]evdevInputEvent
	evSize := int(unsafe.Sizeof(evdevInputEvent{}))
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&events[0])), len(events)*evSize)
	for {
		n, err := syscall.Read(d.fd, buf)
		if err != nil {
			if err == syscall.EAGAIN || err == syscall.EWOULDBLOCK || err == syscall.EINTR {
				return true
			}
			return false
		}
		if n <= 0 {
			return true
		}
		count := n / evSize
		for i := 0; i < count; i++ {
			ev := events[i]
			switch ev.typ {
			case evKey:
				d.applyKey(ev.code, ev.value)
			case evAbs:
				d.applyAbs(ev.code, ev.value)
			case evSyn:
				if ev.code == synDropped {
					d.resyncKeys()
					d.resyncAbs()
				}
			}
		}
		if count < len(events) {
			return true
		}
	}
}

func (d *evdevDevice) applyKey(code uint16, value int32) {
	var v float32
	if value != 0 {
		v = 1
	}
	switch code {
	case btnSouth:
		d.state.Buttons[ButtonSouth] = v
	case btnEast:
		d.state.Buttons[ButtonEast] = v
	case btnWest:
		d.state.Buttons[ButtonWest] = v
	case btnNorth:
		d.state.Buttons[ButtonNorth] = v
	case btnTL:
		d.state.Buttons[ButtonLB] = v
	case btnTR:
		d.state.Buttons[ButtonRB] = v
	case btnTL2:
		if !d.hasTrigger[0] {
			d.state.Buttons[ButtonLT] = v
		}
	case btnTR2:
		if !d.hasTrigger[1] {
			d.state.Buttons[ButtonRT] = v
		}
	case btnSelect:
		d.state.Buttons[ButtonBack] = v
	case btnStart:
		d.state.Buttons[ButtonStart] = v
	case btnThumbL:
		d.state.Buttons[ButtonLeftStick] = v
	case btnThumbR:
		d.state.Buttons[ButtonRightStick] = v
	case btnMode:
		d.state.Buttons[ButtonHome] = v
	case btnDPadUp:
		d.state.Buttons[ButtonDPadUp] = v
	case btnDPadDown:
		d.state.Buttons[ButtonDPadDown] = v
	case btnDPadLeft:
		d.state.Buttons[ButtonDPadLeft] = v
	case btnDPadRight:
		d.state.Buttons[ButtonDPadRight] = v
	}
}

func (d *evdevDevice) applyAbs(code uint16, value int32) {
	info, ok := d.absInfo[code]
	switch code {
	case absX:
		if ok {
			d.state.Axes[AxisLeftX] = normalizeEvdevAxis(value, info.minimum, info.maximum, info.flat)
		}
	case absY:
		if ok {
			d.state.Axes[AxisLeftY] = normalizeEvdevAxis(value, info.minimum, info.maximum, info.flat)
		}
	case absRX:
		if ok {
			d.state.Axes[AxisRightX] = normalizeEvdevAxis(value, info.minimum, info.maximum, info.flat)
		}
	case absRY:
		if ok {
			d.state.Axes[AxisRightY] = normalizeEvdevAxis(value, info.minimum, info.maximum, info.flat)
		}
	case absZ:
		if ok {
			d.state.Buttons[ButtonLT] = normalizeEvdevTrigger(value, info.minimum, info.maximum)
		}
	case absRZ:
		if ok {
			d.state.Buttons[ButtonRT] = normalizeEvdevTrigger(value, info.minimum, info.maximum)
		}
	case absHat0X:
		d.state.Buttons[ButtonDPadLeft] = 0
		d.state.Buttons[ButtonDPadRight] = 0
		if value < 0 {
			d.state.Buttons[ButtonDPadLeft] = 1
		} else if value > 0 {
			d.state.Buttons[ButtonDPadRight] = 1
		}
	case absHat0Y:
		d.state.Buttons[ButtonDPadUp] = 0
		d.state.Buttons[ButtonDPadDown] = 0
		if value < 0 {
			d.state.Buttons[ButtonDPadUp] = 1
		} else if value > 0 {
			d.state.Buttons[ButtonDPadDown] = 1
		}
	}
}

func testBit(bits []byte, bit int) bool {
	if bit < 0 || bit/8 >= len(bits) {
		return false
	}
	return bits[bit/8]&(1<<(uint(bit)%8)) != 0
}

func ioctlBytes(fd int, req uintptr, buf []byte) error {
	if len(buf) == 0 {
		return nil
	}
	return ioctlPtr(fd, req, unsafe.Pointer(&buf[0]))
}

func ioctlPtr(fd int, req uintptr, ptr unsafe.Pointer) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(ptr))
	if errno != 0 {
		return errno
	}
	return nil
}
