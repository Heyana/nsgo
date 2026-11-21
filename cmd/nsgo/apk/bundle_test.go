package apk

import (
	"archive/zip"
	"bytes"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/asn1"
	"encoding/base64"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

// A bundle holds the base module and its config, signed as a JAR whose
// digests and signature check out.
func TestWriteBundle(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = WriteBundle(&buf, androidruntime.Manifest, Manifest{
		Package:     "dev.oneo.nsgo.test",
		Label:       "Test",
		Names:       map[string]string{"": "Test", "zh-Hans": "测试"},
		VersionName: "1.0.0",
		VersionCode: 10000,
		Icon:        []byte("\x89PNG icon"),
		TargetSDK:   35,
	}, []File{
		{Name: "classes.dex", Data: []byte("dex")},
		{Name: "lib/arm64-v8a/libnsgo.so", Data: []byte("arm64")},
		{Name: "lib/x86_64/libnsgo.so", Data: []byte("amd64")},
		{Name: "assets/a.txt", Data: []byte("a")},
	}, key)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := ReadBundleIdentity(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if want := (BundleIdentity{Package: "dev.oneo.nsgo.test", VersionCode: 10000}); err != nil || identity != want {
		t.Fatalf("bundle identity = %+v, %v; want %+v", identity, err, want)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	var names []string
	for _, f := range r.File {
		rc, _ := f.Open()
		files[f.Name], _ = io.ReadAll(rc)
		names = append(names, f.Name)
	}
	want := "META-INF/MANIFEST.MF,META-INF/NSGO.SF,META-INF/NSGO.RSA,base/manifest/AndroidManifest.xml,base/dex/classes.dex," +
		"base/lib/arm64-v8a/libnsgo.so,base/lib/x86_64/libnsgo.so,base/assets/a.txt,base/res/mipmap-xxxhdpi-v4/ic_launcher.png," +
		"base/native.pb,base/resources.pb,BundleConfig.pb"
	if got := strings.Join(names, ","); got != want {
		t.Fatalf("the bundle holds %s", got)
	}

	// NativeLibraries names the two directories with arm64-v8a and x86_64 targeting.
	const native = "\x0a\x15\x0a\x0dlib/arm64-v8a\x12\x04\x0a\x02\x08\x03" +
		"\x0a\x12\x0a\x0alib/x86_64\x12\x04\x0a\x02\x08\x05"
	if got := files["base/native.pb"]; !bytes.Equal(got, []byte(native)) {
		t.Errorf("native.pb = %x, want %x", got, native)
	}

	// The manifest's strings, the target SDK among them, and the
	// resource table's names.
	manifest := string(files["base/manifest/AndroidManifest.xml"])
	for _, s := range []string{"dev.oneo.nsgo.test", "targetSdkVersion", "35", "dev.oneo.nsgo.test.nsgo.files", androidNS} {
		if !strings.Contains(manifest, s) {
			t.Errorf("the manifest lacks %q", s)
		}
	}
	// supportsRtl must carry its framework ID and a compiled true boolean.
	rtl := pbString(pbString(pbString(nil, 1, androidNS), 2, "supportsRtl"), 3, "true")
	rtl = pbVarint(rtl, 5, 0x010103af)
	rtl = pbBytes(rtl, 6, pbBytes(nil, 7, pbVarint(nil, 8, 1)))
	if !bytes.Contains([]byte(manifest), rtl) {
		t.Fatal("bundle manifest does not enable native RTL layout")
	}
	table := string(files["base/resources.pb"])
	for _, s := range []string{"ic_launcher", "app_name", "测试", "zh-Hans", iconFile} {
		if !strings.Contains(table, s) {
			t.Errorf("the resource table lacks %q", s)
		}
	}
	if !bytes.Equal(files["BundleConfig.pb"], []byte("\x0a\x08\x12\x06"+BundletoolVersion+"\x12\x02\x12\x00")) {
		t.Errorf("BundleConfig.pb is %x", files["BundleConfig.pb"])
	}

	// Each file's digest is in the manifest, the manifest's in the
	// signature file, and the key signed that.
	mf := strings.ReplaceAll(string(files["META-INF/MANIFEST.MF"]), "\r\n ", "")
	for _, name := range names[3:] {
		sum := sha256.Sum256(files[name])
		if !strings.Contains(mf, "Name: "+name+"\r\nSHA-256-Digest: "+base64.StdEncoding.EncodeToString(sum[:])) {
			t.Errorf("the manifest lacks %s's digest", name)
		}
	}
	sf := files["META-INF/NSGO.SF"]
	sum := sha256.Sum256(files["META-INF/MANIFEST.MF"])
	if !bytes.Contains(sf, []byte("SHA-256-Digest-Manifest: "+base64.StdEncoding.EncodeToString(sum[:]))) {
		t.Error("the signature file lacks the manifest's digest")
	}
	var outer struct {
		Type    asn1.ObjectIdentifier
		Content asn1.RawValue
	}
	if _, err := asn1.Unmarshal(files["META-INF/NSGO.RSA"], &outer); err != nil || !outer.Type.Equal(oidSignedData) {
		t.Fatalf("NSGO.RSA is no signed data: %v", err)
	}
	var sd struct {
		Version      int
		Digests      asn1.RawValue
		Content      asn1.RawValue
		Certificates asn1.RawValue
		Signers      []struct {
			Version         int
			IssuerAndSerial asn1.RawValue
			Digest          asn1.RawValue
			Encryption      asn1.RawValue
			Signature       []byte
		} `asn1:"set"`
	}
	if _, err := asn1.Unmarshal(outer.Content.Bytes, &sd); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(sd.Certificates.Bytes, key.Cert.Raw) || len(sd.Signers) != 1 {
		t.Fatal("NSGO.RSA does not carry the key's certificate and one signer")
	}
	digest := sha256.Sum256(sf)
	if err := rsa.VerifyPKCS1v15(&key.Private.PublicKey, crypto.SHA256, digest[:], sd.Signers[0].Signature); err != nil {
		t.Errorf("the signature does not verify: %v", err)
	}
}

func TestReadBundleIdentityRejectsIncompleteManifest(t *testing.T) {
	for _, manifest := range [][]byte{[]byte("\x0a\xff"), pbBytes(nil, 1, pbString(nil, 3, "manifest"))} {
		var data bytes.Buffer
		w := zip.NewWriter(&data)
		entry, err := w.Create("base/manifest/AndroidManifest.xml")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := entry.Write(manifest); err != nil {
			t.Fatal(err)
		}
		if err := w.Close(); err != nil {
			t.Fatal(err)
		}
		if _, err := ReadBundleIdentity(bytes.NewReader(data.Bytes()), int64(data.Len())); err == nil {
			t.Fatalf("accepted incomplete manifest %x", manifest)
		}
	}
}

func TestWriteBundleAdaptiveIcon(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	err = WriteBundle(&buf, androidruntime.Manifest, Manifest{
		Package: "dev.oneo.nsgo.test", Label: "Test", VersionName: "1.0.0", VersionCode: 1,
		Icon: []byte("legacy"), Adaptive: &AdaptiveIcon{Foreground: []byte("fg"), Background: []byte("bg"), Monochrome: []byte("mono")},
	}, []File{{Name: "classes.dex", Data: []byte("dex")}}, key)
	if err != nil {
		t.Fatal(err)
	}
	r, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range r.File {
		read, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		files[f.Name], err = io.ReadAll(read)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{"base/" + adaptiveIconFile, "base/" + foregroundFile, "base/" + backgroundFile, "base/" + monochromeFile} {
		if _, ok := files[name]; !ok {
			t.Errorf("bundle lacks %s", name)
		}
	}
	if bytes.HasPrefix(files["base/"+adaptiveIconFile], []byte{3, 0, 8, 0}) {
		t.Error("the bundle's adaptive icon is binary XML, not a protobuf XmlNode")
	}
	aapt2, err := tool("aapt2")
	if err != nil {
		t.Skip("aapt2 is not installed")
	}
	// The base module as a proto APK, which aapt2 converts as bundletool does.
	var proto bytes.Buffer
	zw := zip.NewWriter(&proto)
	for name, data := range files {
		rel, ok := strings.CutPrefix(name, "base/")
		if !ok || strings.HasPrefix(rel, "dex/") {
			continue
		}
		rel = strings.TrimPrefix(rel, "manifest/")
		w, err := zw.Create(rel)
		if err != nil {
			t.Fatal(err)
		}
		w.Write(data)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	in, out := filepath.Join(dir, "proto.apk"), filepath.Join(dir, "binary.apk")
	if err := os.WriteFile(in, proto.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if b, err := exec.Command(aapt2, "convert", "--output-format", "binary", "-o", out, in).CombinedOutput(); err != nil {
		t.Fatalf("aapt2 convert: %v\n%s", err, b)
	}
	dump, err := exec.Command(aapt2, "dump", "resources", out).CombinedOutput()
	if err != nil {
		t.Fatalf("aapt2 dump resources: %v\n%s", err, dump)
	}
	for _, want := range []string{"0x7f010000 mipmap/ic_launcher\n", "mipmap/ic_launcher_foreground", "mipmap/ic_launcher_background", "mipmap/ic_launcher_monochrome", adaptiveIconFile} {
		if !strings.Contains(string(dump), want) {
			t.Errorf("resource dump lacks %q:\n%s", want, dump)
		}
	}
	tree, err := exec.Command(aapt2, "dump", "xmltree", out, "--file", adaptiveIconFile).CombinedOutput()
	if err != nil || !strings.Contains(string(tree), "monochrome") || !strings.Contains(string(tree), "@0x7f010003") {
		t.Errorf("adaptive icon XML (%v):\n%s", err, tree)
	}
}

func TestManifestLinesWrap(t *testing.T) {
	line := manifestLine("Name: " + strings.Repeat("a", 100))
	for i, l := range strings.Split(strings.TrimSuffix(line, "\r\n"), "\r\n") {
		if len(l) > 72 || i > 0 && l[0] != ' ' {
			t.Errorf("line %d is %q", i, l)
		}
	}
	if strings.ReplaceAll(line, "\r\n ", "") != "Name: "+strings.Repeat("a", 100)+"\r\n" {
		t.Errorf("the line wraps into %q", line)
	}
}

func TestProtoWire(t *testing.T) {
	if got := pbVarint(nil, 5, 300); !bytes.Equal(got, []byte{0x28, 0xac, 0x02}) {
		t.Errorf("a varint field is %x", got)
	}
	if got := pbString(nil, 2, "hi"); !bytes.Equal(got, []byte{0x12, 2, 'h', 'i'}) {
		t.Errorf("a string field is %x", got)
	}
}

// TestBundleManifestLibraries checks that a bundle's manifest compiles the device's libraries the app uses.
func TestBundleManifestLibraries(t *testing.T) {
	root, err := appManifest(androidruntime.Manifest, Manifest{Package: "dev.oneo.nsgo.test", Libraries: []string{"androidx.window.extensions"}})
	if err != nil {
		t.Fatal(err)
	}
	pb, err := protoElement(root, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(pb, []byte("uses-library")) || !bytes.Contains(pb, []byte("androidx.window.extensions")) {
		t.Error("the bundle's manifest lacks the library")
	}
}
