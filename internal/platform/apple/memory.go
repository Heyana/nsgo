//go:build darwin

package apple

import (
	"os"
	"sync"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	memoryOnce    sync.Once
	procPidRusage func(pid, flavor int32, buffer unsafe.Pointer) int32
)

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
	memoryOnce.Do(func() {
		purego.RegisterLibFunc(&procPidRusage, load("/usr/lib/libSystem.B.dylib"), "proc_pid_rusage")
	})
	const rusageInfoV2Flavor = 2
	info := new(rusageInfoV2)
	status := procPidRusage(int32(os.Getpid()), rusageInfoV2Flavor, unsafe.Pointer(info))
	if status != 0 {
		return 0, false
	}
	return info.PhysFootprint, true
}
