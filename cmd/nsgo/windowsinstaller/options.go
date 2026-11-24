package windowsinstaller

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type options struct {
	silent, relaunch         bool
	directory, uninstallFrom string
	language                 uint16
}

func arguments(args []string, raw string) (options, error) {
	var o options
	for i, arg := range args {
		switch {
		case strings.EqualFold(arg, "/S"):
			o.silent = true
		case strings.EqualFold(arg, "/RELAUNCH"):
			o.relaunch = true
		case strings.HasPrefix(strings.ToUpper(arg), "/LANG="):
			value := arg[len("/LANG="):]
			base := 10
			if strings.HasPrefix(strings.ToLower(value), "0x") {
				base = 0
			}
			n, err := strconv.ParseUint(value, base, 16)
			if err != nil || n == 0 {
				return o, errors.New("/LANG needs a Windows UI language ID, such as 2052")
			}
			o.language = uint16(n)
		case strings.HasPrefix(strings.ToUpper(arg), "/D="):
			o.directory = strings.Join(append([]string{arg[3:]}, args[i+1:]...), " ")
			if exact, ok := rawDirectory(raw); ok {
				o.directory = exact
			}
			if o.directory == "" {
				return o, errors.New("/D needs an installation directory")
			}
			return o, nil // NSIS-compatible final, unquoted /D
		case strings.HasPrefix(arg, "--nsgo-uninstall-from="):
			o.uninstallFrom = strings.TrimPrefix(arg, "--nsgo-uninstall-from=")
		default:
			return o, fmt.Errorf("unknown installer argument %q", arg)
		}
	}
	return o, nil
}

// NSIS accepts a final unquoted /D path, including repeated spaces. Windows' argv splitting loses those spaces, so recover that last value from the raw command line without changing Unicode byte indexes by uppercasing the string.
func rawDirectory(raw string) (string, bool) {
	for i := 0; i+3 <= len(raw); i++ {
		if raw[i] != '/' || raw[i+1]|0x20 != 'd' || raw[i+2] != '=' || (i > 0 && raw[i-1] != ' ' && raw[i-1] != '"') {
			continue
		}
		value := strings.TrimSpace(raw[i+3:])
		if i > 0 && raw[i-1] == '"' {
			value = strings.TrimSuffix(value, `"`)
		} else if strings.HasPrefix(value, `"`) {
			value = strings.TrimSuffix(strings.TrimPrefix(value, `"`), `"`)
		}
		return value, true
	}
	return "", false
}
