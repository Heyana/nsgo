package updates

import (
	"context"
	"os"
	"os/exec"
	"strings"
	"time"
)

// LinuxConfig resolves Debian-package ownership before the first view. The generated Linux appcast offers a .deb; other installations offer its page. A timed-out or unavailable package manager is treated as an unowned copy.
func LinuxConfig(c Config) Config {
	if !c.Checks(false) {
		return c
	}
	c.PageOnly = true
	exe, err := os.Executable()
	if err != nil {
		return c
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	pattern := strings.NewReplacer(`\`, `\\`, `*`, `\*`, `?`, `\?`, `[`, `\[`).Replace(exe)
	c.PageOnly = exec.CommandContext(ctx, "dpkg-query", "--search", pattern).Run() != nil
	return c
}
