package main

import (
	"flag"
	"fmt"
	"strings"
)

// parseCommand keeps flag.Parse's flags-before-path convention, but never silently discards what it leaves after the path. A negative paths count lets test forward go test arguments instead.
func parseCommand(fs *flag.FlagSet, args []string, paths int) error {
	if err := fs.Parse(args); err != nil {
		return err
	}
	if paths < 0 {
		patterns, rest := splitTestArgs(fs.Args())
		if err := misplacedFlags(fs, patterns, rest, goTestOwnFlags); err != nil {
			return err
		}
		return testPathError(rest)
	}
	if fs.NArg() <= paths {
		return nil
	}
	if paths > 0 {
		if err := misplacedFlags(fs, fs.Args()[:paths], fs.Args()[paths:], nil); err != nil {
			return err
		}
		return fmt.Errorf("%s: takes one path, not %q as well", fs.Name(), fs.Arg(paths))
	}
	return fmt.Errorf("%s: takes no positional arguments, got %q", fs.Name(), fs.Arg(0))
}

// testPathError refuses a package among the flags a run forwards: patterns go before the first flag, as go test's do.
func testPathError(args []string) error {
	if packages := packageArgs(args); len(packages) > 0 {
		return fmt.Errorf("test: takes package patterns before its flags, not %q after them", packages[0])
	}
	return nil
}

// goTestOwnFlags are test's flags that go test defines too. After the directory they are go test's: headless runs forward them, and a target run refuses them in parseTargetTestFlags with its own hint.
var goTestOwnFlags = map[string]bool{"run": true, "timeout": true, "bench": true, "trace": true, "ldflags": true}

// misplacedFlags refuses one of the command's own flags written after its path or patterns, unless forwarded names it as an argument the command hands on.
func misplacedFlags(fs *flag.FlagSet, paths, args []string, forwarded map[string]bool) error {
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-args" || a == "--args" || a == "--" {
			break // The headless test binary's own arguments.
		}
		if !strings.HasPrefix(a, "-") {
			continue
		}
		name, value, hasValue := strings.Cut(strings.TrimPrefix(strings.TrimPrefix(a, "-"), "-"), "=")
		name = strings.TrimPrefix(name, "test.")
		if f := fs.Lookup(name); f != nil && !forwarded[name] {
			option := "-" + name
			if hasValue {
				option += "=" + value
			} else if b, ok := f.Value.(interface{ IsBoolFlag() bool }); !ok || !b.IsBoolFlag() {
				if i+1 < len(args) {
					option += " " + fmt.Sprintf("%q", args[i+1])
				}
			}
			if len(paths) == 0 {
				paths = []string{"."}
			}
			quoted := make([]string, len(paths))
			for j, p := range paths {
				quoted[j] = fmt.Sprintf("%q", p)
			}
			return fmt.Errorf("%s: flags go before the directory: %s %s %s %s", fs.Name(), nsgoCommand(), fs.Name(), option, strings.Join(quoted, " "))
		}
		if !hasValue && !goTestBoolFlags[name] && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
			i++
		}
	}
	return nil
}
