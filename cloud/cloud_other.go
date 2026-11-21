//go:build !darwin && !android

package cloud

// Windows, Linux, Web and other targets have no built-in OS cloud store: every operation fails with ErrUnavailable rather than silently dropping writes. Desktop games distributed on Steam can use Steam Auto-Cloud to sync core.Scope.DataDir directly with no extra code.

func platformGet(string) ([]byte, error)            { return nil, ErrUnavailable }
func platformSet(string, []byte) error              { return ErrUnavailable }
func platformDelete(string) error                   { return ErrUnavailable }
func platformKeys() ([]string, error)               { return nil, ErrUnavailable }
func platformReadFile(string) ([]byte, error)       { return nil, ErrUnavailable }
func platformWriteFile(string, []byte) error        { return ErrUnavailable }
func platformDeleteFile(string) error               { return ErrUnavailable }
func platformListFiles() ([]string, error)          { return nil, ErrUnavailable }
func platformWatchChanges(func(ChangeEvent)) func() { return func() {} }
func platformOnConflict(func(Conflict)) func()      { return func() {} }
