//go:build !js

package driver

import "mime"

func sysTypeByExtension(ext string) string    { return mime.TypeByExtension(ext) }
func sysExtensionsByType(typ string) []string { exts, _ := mime.ExtensionsByType(typ); return exts }
