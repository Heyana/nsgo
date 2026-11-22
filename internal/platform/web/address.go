//go:build js

package web

import (
	"fmt"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func init() { driver.ResolvePageAddress = resolvePageAddress }
func resolvePageAddress(raw, page, base string) (result string, err error) {
	defer func() {
		if failure := recover(); failure != nil {
			result = ""
			err = fmt.Errorf("core: invalid application address: %v", failure)
		}
	}()
	constructor := js.Global().Get("URL")
	origin := constructor.New(page)
	scheme := origin.Get("protocol").String()
	if scheme != "http:" && scheme != "https:" {
		return "", fmt.Errorf("core: application address needs an HTTP page")
	}
	if base == "" {
		base = page
	}
	next := constructor.New(raw, base)
	if next.Get("origin").String() != origin.Get("origin").String() || next.Get("username").String() != "" || next.Get("password").String() != "" {
		return "", fmt.Errorf("core: application address must stay on the page's origin")
	}
	return next.Get("href").String(), nil
}
