package windowsinstaller

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var errCancelled = errors.New("installation cancelled")

func lockInstallation(id string) (func(), error) {
	digest := sha256.Sum256([]byte(id))
	name := fmt.Sprintf(`Local\NSGoInstaller-%x`, digest[:16])
	handle, _, err := kernel32.NewProc("CreateMutexW").Call(0, 0, ptr(wide(name)))
	if handle == 0 {
		return nil, err
	}
	result, _, _ := kernel32.NewProc("WaitForSingleObject").Call(handle, 0)
	if result != 0 && result != 0x80 {
		syscall.CloseHandle(syscall.Handle(handle))
		return nil, errors.New("another installer for this app is running")
	}
	return func() { kernel32.NewProc("ReleaseMutex").Call(handle); syscall.CloseHandle(syscall.Handle(handle)) }, nil
}

// Run is the standalone program's entry point. Installation never elevates and changes only HKCU, its selected file list and the user's app shortcuts/PATH.
func Run(m Manifest) int {
	o, err := arguments(os.Args[1:], rawCommandLine())
	if err == nil {
		err = m.Validate()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if !o.silent {
			showFailure(m.Name, err)
		}
		return 1
	}
	code := 0
	err = withCOM(func() error {
		language, _, _ := kernel32.NewProc("GetUserDefaultUILanguage").Call()
		if o.language != 0 {
			language = uintptr(o.language)
		}
		w := installerWords(m, uint16(language))
		shown := m
		shown.Name = displayName(m, uint16(language)) // Presentation never changes the stable install/shortcut name.

		self, err := os.Executable()
		if err != nil {
			return err
		}
		if m.Uninstall && o.uninstallFrom == "" {
			if o.directory != "" || o.relaunch {
				return errors.New("the uninstaller does not take installation options")
			}
			err := temporaryUninstaller(self, filepath.Dir(self), o.silent, o.language)
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				code = exit.ExitCode()
				return nil
			}
			return err
		}
		if !m.Uninstall && o.uninstallFrom != "" {
			return errors.New("this is an installer, not an uninstaller")
		}
		unlock, err := lockInstallation(m.ID)
		if err != nil {
			return err
		}
		defer unlock()
		dir := o.uninstallFrom
		if m.Uninstall {
			if !strings.HasPrefix(filepath.Base(filepath.Dir(self)), "nsgo-uninstall-") || dir == "" {
				return errors.New("the uninstaller runs from its temporary copy")
			}
			// An uninstaller left in a folder an upgrade moved from removes its own files alone: the registrations, shortcuts and Path belong to the installation InstallLocation names, by a later build or an explicit /D.
			registered := sameDirectory(readString(m.UninstallKey(), "InstallLocation"), dir)
			removeInstalled := false
			defer func() {
				installed := ""
				if removeInstalled {
					installed = filepath.Join(dir, "Uninstall.exe")
				}
				removeTemporaryUninstaller(self, installed, m.ID)
			}()
			if !o.silent {
				ok, err := confirmUninstall(shown, dir, w)
				if err != nil {
					return err
				}
				if !ok {
					return nil
				}
			}
			operation := func(_ func(uint64, uint64)) error {
				if err := stopApp(shown, dir, w, o.silent); err != nil {
					return err
				}
				if !registered {
					return RemoveFiles(dir, m, true)
				}
				if err := shortcuts(m, dir, false, true); err != nil {
					return err
				}
				if owned := readString(m.UninstallKey(), pathOwnedValue); owned != "" {
					if _, err := updatePath(owned, true); err != nil {
						return err
					}
				}
				if err := removeDeclarations(m); err != nil {
					return err
				}
				if err := RemoveFiles(dir, m, true); err != nil {
					return err
				}
				return deleteTree(m.UninstallKey())
			}
			if o.silent {
				err = operation(nil)
			} else {
				err = runProgress(shown, w, operation)
			}
			if err == nil {
				removeInstalled = true
			}
			return err
		}
		payload, closer, err := OpenPayload(self)
		if err != nil {
			return err
		}
		defer closer.Close()
		dir, err = installationDirectory(m, o.directory)
		if err != nil {
			return err
		}
		desktop := false
		if !o.silent {
			var ok bool
			dir, desktop, ok, err = confirmInstall(shown, dir, w)
			if err != nil {
				return err
			}
			if !ok {
				return nil
			}
			dir, err = installationDirectory(m, dir)
			if err != nil {
				return err
			}
		}
		operation := func(progress func(uint64, uint64)) error {
			if err := stopApp(shown, dir, w, o.silent); err != nil {
				return err
			}
			if err := Extract(dir, m, payload, progress); err != nil {
				return err
			}
			if err := registerDeclarations(m, dir); err != nil {
				return err
			}
			if err := shortcuts(m, dir, desktop, false); err != nil {
				return err
			}
			// The folder an installer put on the user's Path, NSIS's installers included, comes off when this version puts none there or installs elsewhere; a folder the user had there stays theirs.
			owned := readString(m.UninstallKey(), pathOwnedValue)
			if owned != "" && (!m.Command || !sameDirectory(owned, dir)) {
				if _, err := updatePath(owned, true); err != nil {
					return err
				}
				owned = ""
			}
			if m.Command {
				changed, err := updatePath(dir, false)
				if err != nil {
					return err
				}
				if changed {
					owned = dir
				}
			}
			return registerUninstall(m, dir, owned)
		}
		if o.silent {
			err = operation(nil)
		} else {
			err = runProgress(shown, w, operation)
		}
		if err != nil {
			return err
		}
		launch := o.relaunch
		if !o.silent {
			launch, err = showFinished(shown, w)
			if err != nil {
				return err
			}
		}
		if launch {
			command := exec.Command(filepath.Join(dir, m.Exe))
			command.Dir = dir
			if err := command.Start(); err != nil {
				return err
			}
			command.Process.Release()
		}
		return nil
	})
	if errors.Is(err, errCancelled) {
		return 0
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if !o.silent {
			showFailure(m.Name, err)
		}
		return 1
	}
	return code
}
func stopApp(m Manifest, dir string, w dialogWords, silent bool) error {
	processes, err := installedProcesses(dir)
	if err != nil {
		return err
	}
	if len(processes) == 0 {
		return nil
	}
	if !silent && !confirmRunning(m, w) {
		return errCancelled
	}
	return closeInstalledProcesses(processes, dir)
}

func rawCommandLine() string {
	p := syscall.GetCommandLine()
	if p == nil {
		return ""
	}
	n, _, _ := kernel32.NewProc("lstrlenW").Call(ptr(p))
	return syscall.UTF16ToString(unsafe.Slice(p, int(n)))
}
