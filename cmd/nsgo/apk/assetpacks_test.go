package apk

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

func TestWriteBundleAssetPacks(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	asset := filepath.Join(t.TempDir(), "fixture.txt")
	contents := []byte("separately delivered file")
	if err := os.WriteFile(asset, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	packs := []AssetPack{
		{Name: "install", Delivery: "install-time", Files: []AssetPackFile{{Name: "data/fixture.txt", Source: asset}}},
		{Name: "prefetch", Delivery: "fast-follow", Files: []AssetPackFile{{Name: "data/fixture.txt", Source: asset}}},
	}
	var buf bytes.Buffer
	m := Manifest{Package: "dev.nsgo.assetpacks", VersionCode: 1, AssetPacks: packs}
	if err := WriteBundle(&buf, androidruntime.Manifest, m, nil, key); err != nil {
		t.Fatal(err)
	}
	z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	mf, err := fs.ReadFile(z, "META-INF/MANIFEST.MF")
	if err != nil {
		t.Fatal(err)
	}
	signed := strings.ReplaceAll(string(mf), "\r\n ", "")
	sum := sha256.Sum256(contents)
	for _, pack := range packs {
		moduleManifest, err := fs.ReadFile(z, pack.Name+"/manifest/AndroidManifest.xml")
		if err != nil || !bytes.Contains(moduleManifest, []byte(pack.Delivery)) || !bytes.Contains(moduleManifest, []byte(distNS)) {
			t.Fatalf("%s manifest has no delivery declaration: %v", pack.Name, err)
		}
		entry := pack.Name + "/assets/nsgo_asset_packs/" + pack.Name + "/data/fixture.txt"
		got, err := fs.ReadFile(z, entry)
		if err != nil || !bytes.Equal(got, contents) {
			t.Errorf("%s = %q, %v", entry, got, err)
		}
		if !strings.Contains(signed, "Name: "+entry+"\r\nSHA-256-Digest: "+base64.StdEncoding.EncodeToString(sum[:])) {
			t.Errorf("%s has no signed digest", entry)
		}
	}
}
