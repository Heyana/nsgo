package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// windowsCommandSource is a console-subsystem companion for a GUI app. A
// .com file wins Windows' command lookup before the same-name .exe, while the
// GUI executable remains the one Explorer and the Start menu launch.
//
// The app, a GUI program, is attached to no console, so the console's Ctrl+C, Ctrl+Break and close reach the companion alone: the companion ends the app before it exits, and a job that kills its processes as its last handle closes ends the app when the companion is killed outright. The job lets the app's own children break away, so an app that restarts itself or runs its updater's installer keeps them.
//
// Win+R and the other launchers that look a bare name up through Path find the .com before the .exe too, and Windows opens a new console for it: with that console its own and nothing reading its output, the companion starts the app untied to it and exits, so the console closes and the app runs as from the Start menu.
const windowsCommandSource = `package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"syscall"
	"unsafe"
)

// statusControlCExit is STATUS_CONTROL_C_EXIT, the code a console program ends with on Ctrl+C.
const statusControlCExit = 0xC000013A

// Job objects, as winnt.h declares them.
const (
	jobObjectExtendedLimitInformationClass = 9
	jobObjectLimitSilentBreakawayOK        = 0x1000
	jobObjectLimitKillOnJobClose           = 0x2000
	processSetQuota                        = 0x0100
)

type jobObjectBasicLimitInformation struct {
	PerProcessUserTimeLimit int64
	PerJobUserTimeLimit     int64
	LimitFlags              uint32
	MinimumWorkingSetSize   uintptr
	MaximumWorkingSetSize   uintptr
	ActiveProcessLimit      uint32
	Affinity                uintptr
	PriorityClass           uint32
	SchedulingClass         uint32
}

type jobObjectExtendedLimitInformation struct {
	BasicLimitInformation jobObjectBasicLimitInformation
	IoInfo                [6]uint64
	ProcessMemoryLimit    uintptr
	JobMemoryLimit        uintptr
	PeakProcessMemoryUsed uintptr
	PeakJobMemoryUsed     uintptr
}

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	createJobObject          = kernel32.NewProc("CreateJobObjectW")
	setInformationJobObject  = kernel32.NewProc("SetInformationJobObject")
	assignProcessToJobObject = kernel32.NewProc("AssignProcessToJobObject")
	getConsoleProcessList    = kernel32.NewProc("GetConsoleProcessList")
)

// ownConsole reports whether Windows opened a console for the companion alone, as it does when Win+R, Task Manager's "Run new task", Explorer's address bar or cmd's start find the .com through Path, and its output goes to that console rather than to a pipe or a file a caller reads.
func ownConsole() bool {
	var processes [2]uint32
	if n, _, _ := getConsoleProcessList.Call(uintptr(unsafe.Pointer(&processes[0])), uintptr(len(processes))); n != 1 {
		return false
	}
	var mode uint32
	return syscall.GetConsoleMode(syscall.Stdout, &mode) == nil
}

// killWithCompanion puts the process in a job that Windows ends as the companion's handle to it closes, when the companion exits or is killed. Without one, which an old system's job may refuse, the companion still ends the app on the console's events.
func killWithCompanion(pid int) {
	job, _, _ := createJobObject.Call(0, 0)
	if job == 0 {
		return
	}
	var info jobObjectExtendedLimitInformation
	info.BasicLimitInformation.LimitFlags = jobObjectLimitKillOnJobClose | jobObjectLimitSilentBreakawayOK
	if ok, _, _ := setInformationJobObject.Call(job, jobObjectExtendedLimitInformationClass, uintptr(unsafe.Pointer(&info)), unsafe.Sizeof(info)); ok == 0 {
		syscall.CloseHandle(syscall.Handle(job))
		return
	}
	process, err := syscall.OpenProcess(processSetQuota|syscall.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		syscall.CloseHandle(syscall.Handle(job))
		return
	}
	defer syscall.CloseHandle(process)
	if ok, _, _ := assignProcessToJobObject.Call(job, uintptr(process)); ok == 0 {
		syscall.CloseHandle(syscall.Handle(job))
	}
}

func main() {
	self, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	target := filepath.Join(filepath.Dir(self), filepath.Base(self[:len(self)-len(filepath.Ext(self))])+".exe")
	command := exec.Command(target, os.Args[1:]...)
	// No shell waits on a console of the companion's own: start the app as Explorer would and exit, which closes the console, rather than keep an empty window whose close or Ctrl+C would end the app.
	if ownConsole() {
		if err := command.Start(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		command.Process.Release()
		return
	}
	command.Stdin, command.Stdout, command.Stderr = os.Stdin, os.Stdout, os.Stderr
	// Ctrl+C and Ctrl+Break arrive as os.Interrupt, the console's close, logoff and shutdown as SIGTERM.
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	if err := command.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	killWithCompanion(command.Process.Pid)
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	select {
	case err = <-done:
	case <-signals:
		command.Process.Kill()
		os.Exit(statusControlCExit)
	}
	if err != nil {
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			os.Exit(exit.ExitCode())
		}
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
`

// buildWindowsCommand writes the console companion beside p.Name.exe, linking syso, the app's resource object, when there is one, so both files show the same icon and version. It is a separate Go program so the app keeps the GUI subsystem and Explorer never opens a console for the normal executable.
func buildWindowsCommand(ctx context.Context, p *project, dir, arch string, syso []byte) error {
	temp, err := os.MkdirTemp("", "nsgo-windows-command-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	// A module of its own, as go build links .syso files only into a package's build, never one of files named on the command line.
	files := map[string]string{"main.go": windowsCommandSource, "go.mod": "module nsgo-windows-command\n\ngo 1.26\n"}
	if syso != nil {
		files[windowsResourceObject(arch)] = string(syso)
	}
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(temp, name), []byte(data), 0o644); err != nil {
			return err
		}
	}
	output := filepath.Join(dir, p.Name+".com")
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-buildvcs=false", "-o", output, ".")
	cmd.Dir = temp
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH="+arch, "CGO_ENABLED=0", "GOWORK=off")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build Windows command companion failed: %w\n%s", err, out)
	}
	return nil
}

// windowsPath reports whether the Windows installer puts the app's folder on the user's Path for windows.path, which the Microsoft Store's MSIX, installing where no console's command lookup finds it, leaves alone.
func (p *project) windowsPath() bool {
	return p.Windows.Path && p.channel() != channelStore
}

// windowsCommandLine reports whether Windows builds write the command companion: with windows.command_line, but not for the Microsoft Store, whose MSIX installs where no console's command lookup finds it.
func (p *project) windowsCommandLine() bool {
	return p.Windows.CommandLine && p.channel() != channelStore
}
