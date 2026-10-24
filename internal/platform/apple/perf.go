//go:build darwin && !ios

package apple

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

// The performance monitor's process readings, which macOS publishes without
// privileges: the physical footprint from libproc, and the GPU time the
// process' accelerator clients accumulate in the IO registry.

var perfOnce sync.Once

var (
	procPidRusage func(pid, flavor int32, buffer unsafe.Pointer) int32

	cfGetTypeID           func(ref uintptr) uintptr
	cfStringGetTypeID     func() uintptr
	cfArrayGetTypeID      func() uintptr
	cfNumberGetTypeID     func() uintptr
	cfDictionaryGetTypeID func() uintptr
	ioServiceMatching     func(name *byte) uintptr
	ioGetMatching         func(port uint32, matching uintptr, existing *uint32) int32
	ioIteratorNext        func(iterator uint32) uint32
	ioObjectRelease       func(object uint32) int32
	ioChildren            func(entry uint32, plane *byte, iterator *uint32) int32
	ioCreateProperties    func(entry uint32, properties *uintptr, allocator uintptr, options uint32) int32
)

// loadPerf loads what process readings call, once.
func loadPerf() {
	perfOnce.Do(func() {
		system := load("/usr/lib/libSystem.B.dylib")
		reg := purego.RegisterLibFunc
		reg(&procPidRusage, system, "proc_pid_rusage")
		cf := load("/System/Library/Frameworks/CoreFoundation.framework/CoreFoundation")
		reg(&cfGetTypeID, cf, "CFGetTypeID")
		reg(&cfStringGetTypeID, cf, "CFStringGetTypeID")
		reg(&cfArrayGetTypeID, cf, "CFArrayGetTypeID")
		reg(&cfNumberGetTypeID, cf, "CFNumberGetTypeID")
		reg(&cfDictionaryGetTypeID, cf, "CFDictionaryGetTypeID")
		iokit := load("/System/Library/Frameworks/IOKit.framework/IOKit")
		reg(&ioServiceMatching, iokit, "IOServiceMatching")
		reg(&ioGetMatching, iokit, "IOServiceGetMatchingServices")
		reg(&ioIteratorNext, iokit, "IOIteratorNext")
		reg(&ioObjectRelease, iokit, "IOObjectRelease")
		reg(&ioChildren, iokit, "IORegistryEntryGetChildIterator")
		reg(&ioCreateProperties, iokit, "IORegistryEntryCreateCFProperties")
	})
}

// rusageInfoV2 is libproc's rusage_info_v2, the flavor whose physical
// footprint Activity Monitor shows.
type rusageInfoV2 struct {
	UUID                [16]byte
	UserTime            uint64
	SystemTime          uint64
	PkgIdleWkups        uint64
	InterruptWkups      uint64
	Pageins             uint64
	WiredSize           uint64
	ResidentSize        uint64
	PhysFootprint       uint64
	ProcStartAbstime    uint64
	ProcExitAbstime     uint64
	ChildUserTime       uint64
	ChildSystemTime     uint64
	ChildPkgIdleWkups   uint64
	ChildInterruptWkups uint64
	ChildPageins        uint64
	ChildElapsedAbstime uint64
	DiskIOBytesRead     uint64
	DiskIOBytesWritten  uint64
}

// ProcessMemory returns the process' physical footprint in bytes: the dirty
// and compressed memory it owns, what Activity Monitor shows and what the
// system judges it by under memory pressure (unlike the resident size, it
// leaves out the frameworks' clean pages every process maps).
func ProcessMemory() (uint64, bool) {
	loadPerf()
	const rusageInfoV2Flavor = 2
	info := new(rusageInfoV2)
	status := procPidRusage(int32(os.Getpid()), rusageInfoV2Flavor, unsafe.Pointer(info))
	if status != 0 {
		return 0, false
	}
	return info.PhysFootprint, true
}

// GPUProbe reads the process' GPU use from the accumulatedGPUTime its
// accelerator clients report in the IO registry, the counter behind Activity
// Monitor's GPU column.
type GPUProbe struct {
	// How the registry tags the clients this process opened ("pid 4242,
	// name"; the name is truncated, so only the pid is matched).
	creator string
	last    uint64
	at      time.Time
	hasLast bool
}

// NewGPUProbe returns a probe, or nil when the machine has no accelerator.
func NewGPUProbe() *GPUProbe {
	loadPerf()
	accelerators := accelerators()
	if len(accelerators) == 0 {
		return nil
	}
	for _, accelerator := range accelerators {
		ioObjectRelease(accelerator)
	}
	return &GPUProbe{creator: fmt.Sprintf("pid %d,", os.Getpid())}
}

// Sample returns GPU time over wall time since the previous call, in percent:
// nil the first time, and while the process has no accelerator client.
func (p *GPUProbe) Sample() *float32 {
	used, ok := accumulated(p.creator)
	if !ok {
		return nil
	}
	now := time.Now()
	if !p.hasLast {
		p.last, p.at, p.hasLast = used, now, true
		return nil
	}
	elapsed := now.Sub(p.at)
	previous := p.last
	p.last, p.at = used, now
	if elapsed <= 0 {
		return nil
	}
	// A client that goes away takes its share of the total with it.
	busy := uint64(0)
	if used >= previous {
		busy = used - previous
	}
	share := float32(float64(busy) / float64(elapsed) * 100)
	return &share
}

// accumulated returns the nanoseconds every accelerator client of creator
// spent, and whether any client reported them.
func accumulated(creator string) (uint64, bool) {
	var plane [128]byte
	copy(plane[:], "IOService")
	var total uint64
	found := false
	for _, accelerator := range accelerators() {
		var clients uint32
		status := ioChildren(accelerator, &plane[0], &clients)
		ioObjectRelease(accelerator)
		if status != 0 || clients == 0 {
			continue
		}
		for {
			client := ioIteratorNext(clients)
			if client == 0 {
				break
			}
			if used, ok := clientNanos(client, creator); ok {
				total += used
				found = true
			}
			ioObjectRelease(client)
		}
		ioObjectRelease(clients)
	}
	return total, found
}

// clientNanos returns what one client accumulated, or false if it belongs to
// another process or reports no usage (Intel Macs publish no AppUsage).
func clientNanos(client uint32, creator string) (uint64, bool) {
	properties, ok := registryProperties(client)
	if !ok {
		return 0, false
	}
	defer Release(properties)
	owner := dictValue(properties, "IOUserClientCreator")
	if owner == 0 || cfGetTypeID(owner) != cfStringGetTypeID() || !strings.HasPrefix(GoString(objc.ID(owner)), creator) {
		return 0, false
	}
	usage := dictValue(properties, "AppUsage")
	if usage == 0 || cfGetTypeID(usage) != cfArrayGetTypeID() {
		return 0, false
	}
	var total uint64
	for i := 0; i < cfArrayGetCount(usage); i++ {
		queue := cfArrayGetValueAtIndex(usage, i)
		if queue == 0 || cfGetTypeID(queue) != cfDictionaryGetTypeID() {
			continue
		}
		number := dictValue(queue, "accumulatedGPUTime")
		if number == 0 || cfGetTypeID(number) != cfNumberGetTypeID() {
			continue
		}
		var value int64
		const sint64Type = 4
		if cfNumberGetValue(number, sint64Type, unsafe.Pointer(&value)) && value > 0 {
			total += uint64(value)
		}
	}
	return total, true
}

// accelerators returns every registered accelerator; the caller releases
// them.
func accelerators() []uint32 {
	name := append([]byte("IOAccelerator"), 0)
	matching := ioServiceMatching(&name[0])
	if matching == 0 {
		return nil
	}
	var iterator uint32
	const mainPortDefault = 0
	if ioGetMatching(mainPortDefault, matching, &iterator) != 0 || iterator == 0 {
		return nil
	}
	var found []uint32
	for {
		accelerator := ioIteratorNext(iterator)
		if accelerator == 0 {
			break
		}
		found = append(found, accelerator)
	}
	ioObjectRelease(iterator)
	return found
}

// registryProperties returns a snapshot of one registry entry's properties;
// the caller releases it.
func registryProperties(entry uint32) (uintptr, bool) {
	var properties uintptr
	if ioCreateProperties(entry, &properties, 0, 0) != 0 || properties == 0 {
		return 0, false
	}
	return properties, true
}

// dictValue borrows the value of key in a CFDictionary.
func dictValue(dictionary uintptr, key string) uintptr {
	k := CFString(key)
	defer Release(k)
	return cfDictionaryGetValue(dictionary, k)
}

func hasPrefix(s, prefix string) bool {
	return strings.HasPrefix(s, prefix)
}
