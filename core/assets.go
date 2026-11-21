package core

import (
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"strings"
	"unicode"
)

// ErrAssetNotFound means the app does not ship a file at the requested asset URL.
var ErrAssetNotFound = errors.New("core: bundled asset not found")

// ErrAssetsUnavailable means the app's host cannot read bundled asset files.
var ErrAssetsUnavailable = errors.New("core: bundled assets unavailable")

// ReadAsset reads the bytes of a file in the app's resources, named by an
// asset: URL such as asset:maps/level.bin. It calls done later on the
// engine's goroutine, unless the owner that asked was disposed or ran
// again. A missing file returns ErrAssetNotFound, which errors.Is matches.
func (cx *Scope) ReadAsset(uri string, done func(data []byte, err error)) {
	if done == nil {
		return
	}
	owner := cx.current()
	ctx := owner.context()
	finish := func(data []byte, err error) {
		owner.Post(func() {
			if ctx.Err() == nil {
				done(data, err)
			}
		})
	}
	path, err := assetPath(uri)
	read := owner.rt.readAsset
	if err == nil && read == nil {
		err = ErrAssetsUnavailable
	}
	if err != nil {
		finish(nil, err)
		return
	}
	go func() {
		data, err := read(ctx, path)
		if errors.Is(err, fs.ErrNotExist) {
			err = fmt.Errorf("%w: %s", ErrAssetNotFound, uri)
		}
		if ctx.Err() == nil {
			finish(data, err)
		}
	}()
}

// assetPath returns a slash-separated path within the app's resources.
// An asset URL's query and fragment are independent of the file name.
func assetPath(uri string) (string, error) {
	u, err := url.Parse(uri)
	if err == nil && u.Scheme == "asset" && u.Host == "" && u.User == nil && !strings.HasPrefix(u.Path, "//") {
		name := strings.TrimPrefix(u.Path, "/")
		if u.Opaque != "" {
			name, err = url.PathUnescape(u.Opaque)
		}
		if err == nil && fs.ValidPath(name) && name != "." &&
			!strings.ContainsAny(name, `\:`) && strings.IndexFunc(name, unicode.IsControl) < 0 {
			return name, nil
		}
	}
	return "", fmt.Errorf("core: invalid asset URL %q", uri)
}
