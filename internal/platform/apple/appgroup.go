//go:build darwin

package apple

import (
	"fmt"

	"github.com/ebitengine/purego/objc"
)

// AppGroupContainer returns the shared container granted by the bundle's
// signature. Call on the main thread, as other bundle startup work does.
func AppGroupContainer(group string) (string, error) {
	manager := objc.ID(Class("NSFileManager")).Send(Sel("defaultManager"))
	url := manager.Send(Sel("containerURLForSecurityApplicationGroupIdentifier:"), NSString(group))
	path := GoString(url.Send(Sel("path")))
	if path == "" {
		return "", fmt.Errorf("nsgo: the app cannot reach the container of its app group %s: its signature must grant the group", group)
	}
	return path, nil
}
