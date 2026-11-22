package apk

import (
	"bytes"
	"testing"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

func TestManifestVerifiedAppLinkHosts(t *testing.T) {
	for _, schemes := range [][]string{nil, {"example"}} {
		m := Manifest{Package: "dev.nsgo.links", URLSchemes: schemes, AppLinks: []string{"example.com", "links.example.com"}}
		data, err := EncodeManifest(androidruntime.Manifest, m)
		if err != nil {
			t.Fatal(err)
		}
		activity := decodeXML(t, data).root.child(t, "application").child(t, "activity")
		if got := activity.attr(t, "launchMode"); got.kind != typeInt || got.data != 2 {
			t.Fatal("App Links must reuse the running task")
		}
		if len(activity.kids) != 3+len(schemes) {
			t.Fatalf("filters=%d", len(activity.kids))
		}
		for i, host := range m.AppLinks {
			filter := activity.kids[1+len(schemes)+i]
			if filter.name != "intent-filter" {
				t.Fatal("not an intent filter")
			}
			if attr := filter.attr(t, "autoVerify"); attr.kind != typeBool || attr.data != ^uint32(0) {
				t.Fatalf("autoVerify=%+v", attr)
			}
			data := filter.child(t, "data")
			if data.string(t, "scheme") != "https" || data.string(t, "host") != host {
				t.Fatalf("host %q data=%+v", host, data)
			}
			if filter.child(t, "action").string(t, "name") != "android.intent.action.VIEW" || len(filter.kids) != 4 {
				t.Fatal("App Link filter lost VIEW/categories")
			}
		}
		root, err := appManifest(androidruntime.Manifest, m)
		if err != nil {
			t.Fatal(err)
		}
		pb, err := protoElement(root, true, false)
		if err != nil {
			t.Fatal(err)
		}
		for _, host := range m.AppLinks {
			if !bytes.Contains(pb, []byte(host)) {
				t.Fatalf("AAB manifest omitted %q", host)
			}
		}
		verify := pbString(pbString(pbString(nil, 1, androidNS), 2, "autoVerify"), 3, "true")
		verify = pbVarint(verify, 5, 0x010104ee)
		verify = pbBytes(verify, 6, pbBytes(nil, 7, pbVarint(nil, 8, 1)))
		if bytes.Count(pb, verify) != len(m.AppLinks) {
			t.Fatal("AAB autoVerify is not a typed framework boolean for every host")
		}
	}
}
