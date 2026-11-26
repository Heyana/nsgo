package main

import (
	"os/exec"
	"strconv"
	"strings"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

var (
	procPidRusage    func(pid int32, flavor int32, buf unsafe.Pointer) int32
	machTimebaseInfo func(info unsafe.Pointer) int32
	procListAllPids  func(buf unsafe.Pointer, size int32) int32
	procPidPath      func(pid int32, buf unsafe.Pointer, size uint32) int32
	// responsibleFor is the process macOS holds responsible for a process: the app, for the XPC services that work for it.
	responsibleFor func(pid int32) int32
)

func init() {
	lib, err := purego.Dlopen("/usr/lib/libSystem.B.dylib", purego.RTLD_LAZY)
	if err != nil {
		panic(err)
	}
	purego.RegisterLibFunc(&procPidRusage, lib, "proc_pid_rusage")
	purego.RegisterLibFunc(&machTimebaseInfo, lib, "mach_timebase_info")
	purego.RegisterLibFunc(&procListAllPids, lib, "proc_listallpids")
	purego.RegisterLibFunc(&procPidPath, lib, "proc_pidpath")
	purego.RegisterLibFunc(&responsibleFor, lib, "responsibility_get_pid_responsible_for_pid")
}

// process reads a process' rusage_info_v2, and those of the XPC services working for it. A system webview (WKWebView, which Wails draws in) lays out, runs and paints its page in WebKit's XPC services, apart from the app's process; they count with it, or a page would cost only what its app's process does.
type process struct {
	pid          int32
	numer, denom uint64
	// before is what ran when the program started; helpers are the XPC services it started since, found once, when the program has settled.
	before  pidSet
	helpers []int32
	found   bool
}

func allPids() []int32 {
	n := procListAllPids(nil, 0)
	pids := make([]int32, max(n, 0)+64)
	n = procListAllPids(unsafe.Pointer(&pids[0]), int32(len(pids)*4))
	return pids[:max(n, 0)]
}

func runningPids() pidSet {
	set := pidSet{}
	for _, pid := range allPids() {
		set[pid] = true
	}
	return set
}

func pidPath(pid int32) string {
	var buf [4096]byte
	n := procPidPath(pid, unsafe.Pointer(&buf[0]), uint32(len(buf)))
	if n <= 0 {
		return ""
	}
	return string(buf[:n])
}

// group is the process and its helpers: XPC services started since the program, held by macOS on the same process' account as the program (the app for an app bundle, the terminal for a program a terminal starts).
func (p *process) group() []int32 {
	if !p.found {
		p.found = true
		owner := responsibleFor(p.pid)
		for _, pid := range allPids() {
			if pid <= 0 || pid == p.pid || p.before[pid] || responsibleFor(pid) != owner {
				continue
			}
			if strings.Contains(pidPath(pid), ".xpc/") {
				p.helpers = append(p.helpers, pid)
			}
		}
	}
	return append([]int32{p.pid}, p.helpers...)
}

func openProcess(pid int, before pidSet) (*process, error) {
	var tb [2]uint32
	machTimebaseInfo(unsafe.Pointer(&tb))
	return &process{pid: int32(pid), numer: uint64(tb[0]), denom: uint64(tb[1]), before: before}, nil
}

func (p *process) close() {}

// usage is a process' rusage_info_v2, by 8 bytes: the UUID, then ri_user_time,
// ri_system_time, …, ri_phys_footprint at 9.
func usage(pid int32) ([32]uint64, error) {
	var buf [32]uint64
	if procPidRusage(pid, 2, unsafe.Pointer(&buf)) != 0 {
		return buf, errProcess
	}
	return buf, nil
}

// sum adds a value of rusage_info_v2 over the process and its helpers. A helper that has gone counts nothing; the process itself must be there.
func (p *process) sum(at int) (uint64, error) {
	var total uint64
	for k, pid := range p.group() {
		u, err := usage(pid)
		if err != nil {
			if k == 0 {
				return 0, err
			}
			continue
		}
		total += u[at]
	}
	return total, nil
}

// cpu is the user and system time of the process and its helpers, in Mach time converted.
func (p *process) cpu() (time.Duration, error) {
	user, err := p.sum(2)
	if err != nil {
		return 0, err
	}
	system, err := p.sum(3)
	if err != nil {
		return 0, err
	}
	return time.Duration((user + system) * p.numer / p.denom), nil
}

// memory is the physical footprint of the process and its helpers, as Activity Monitor shows each.
func (p *process) memory() (int64, error) {
	m, err := p.sum(9)
	return int64(m), err
}

func cpuAndMemory() (string, int64) {
	name, _ := exec.Command("sysctl", "-n", "machdep.cpu.brand_string").Output()
	mem, _ := exec.Command("sysctl", "-n", "hw.memsize").Output()
	n, _ := strconv.ParseInt(strings.TrimSpace(string(mem)), 10, 64)
	return strings.TrimSpace(string(name)), n
}

// compositorCPU is WindowServer's CPU time, which draws and composites
// every app's layers: ps reads it for any user, in hundredths of a second.
func compositorCPU() (time.Duration, error) {
	pid, err := exec.Command("pgrep", "-x", "WindowServer").Output()
	if err != nil {
		return 0, err
	}
	out, err := exec.Command("ps", "-o", "time=", "-p", strings.TrimSpace(string(pid))).Output()
	if err != nil {
		return 0, err
	}
	// [hours:]minutes:seconds.hundredths
	var d time.Duration
	for _, part := range strings.Split(strings.TrimSpace(string(out)), ":") {
		v, err := strconv.ParseFloat(part, 64)
		if err != nil {
			return 0, err
		}
		d = d*60 + time.Duration(v*float64(time.Second))
	}
	return d, nil
}
