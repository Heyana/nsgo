//go:build js && wasm

package miniprogram

import (
	"log"
	"syscall/js"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

var _ driver.FontRegistrar = (*surface)(nil)

// registerFonts loads the app's bundled font files from resources/ through the
// platform's loadFontFace API and notifies the engine as each face becomes
// available.
func (s *surface) registerFonts(paths []string) {
	for _, p := range paths {
		data, ok := s.readResource(p)
		if !ok {
			log.Printf("nsgo: font %s: could not read resource", p)
			continue
		}
		if err := s.RegisterFont(data); err != nil {
			log.Printf("nsgo: font %s: %v", p, err)
		}
	}
}

// RegisterFont implements driver.FontRegistrar: loads data through
// loadFontFace with a data-URI source and triggers a font refresh once the
// host confirms the face loaded.
func (s *surface) RegisterFont(data []byte) error {
	spec, err := buildFontFaceSpec(data)
	if err != nil {
		return err
	}
	s.loadFontSpec(spec)
	return nil
}

func (s *surface) loadFontSpec(spec fontFaceSpec) {
	opts := map[string]any{
		"family": spec.Family,
		"source": spec.Source,
		"global": true,
		"scopes": []any{"webview", "native"},
		"desc": map[string]any{
			"style":  spec.Style,
			"weight": spec.Weight,
		},
	}
	s.call("loadFontFace", opts, func(res js.Value, ok bool) {
		if !ok {
			if res.Type() == js.TypeObject {
				if msg := res.Get("errMsg"); msg.Type() == js.TypeString {
					log.Printf("nsgo: font %s: %s", spec.Family, msg.String())
				}
			}
			return
		}
		s.mu.Lock()
		s.text.ClearWidths()
		s.mu.Unlock()
		if s.engine != nil {
			s.engine.FontsChanged()
		}
	})
}

// readResource reads a file bundled under the mini program's resources/ folder.
func (s *surface) readResource(p string) ([]byte, bool) {
	for _, candidate := range resourceFileCandidates(p) {
		if data, ok := s.readFile(candidate); ok && len(data) > 0 {
			return data, true
		}
	}
	return nil, false
}
