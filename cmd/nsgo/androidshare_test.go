package main

import (
	"slices"
	"strings"
	"testing"
)

func TestAndroidShareTypes(t *testing.T) {
	for _, tc := range []struct {
		yaml string
		want []string
		err  string
	}{
		{yaml: "[]"},
		{yaml: "[Text/Plain, 'image/*', '*/*']", want: []string{"text/plain", "image/*", "*/*"}},
		{yaml: "[image/png, Image/PNG]", err: "duplicate MIME type"},
		{yaml: "[png]", err: "needs a MIME type"},
		{yaml: "['*/png']", err: "needs a MIME type"},
		{yaml: "['image/p*']", err: "needs a MIME type"},
		{yaml: "['text/plain; charset=utf-8']", err: "needs a MIME type"},
	} {
		t.Run(tc.yaml, func(t *testing.T) {
			dir := t.TempDir()
			writeFiles(t, dir, map[string]string{"nsgo.yaml": "name: Shares\nandroid:\n  share_types: " + tc.yaml + "\n"})
			p, err := load(dir)
			if tc.err != "" {
				if err == nil || !strings.Contains(err.Error(), "android.share_types:") || !strings.Contains(err.Error(), tc.err) {
					t.Fatalf("load = %v, want %q", err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if got := p.Android.ShareTypes; !slices.Equal(got, tc.want) {
				t.Fatalf("share_types = %q, want %q", got, tc.want)
			}
		})
	}
}
