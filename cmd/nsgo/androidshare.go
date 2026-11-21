package main

import (
	"fmt"
	"mime"
	"strings"
)

func (p *project) validateAndroidShareTypes() error {
	seen := make(map[string]bool, len(p.Android.ShareTypes))
	for i, value := range p.Android.ShareTypes {
		media, params, err := mime.ParseMediaType(value)
		major, minor, pair := strings.Cut(media, "/")
		wildcard := media == "*/*" || !strings.Contains(major, "*") &&
			(minor == "*" || !strings.Contains(minor, "*"))
		if err != nil || !pair || major == "" || minor == "" || len(params) != 0 || !wildcard {
			return fmt.Errorf("android.share_types: %q needs a MIME type, such as text/plain or image/*", value)
		}
		if seen[media] {
			return fmt.Errorf("android.share_types: duplicate MIME type %q", media)
		}
		seen[media] = true
		p.Android.ShareTypes[i] = media
	}
	return nil
}
