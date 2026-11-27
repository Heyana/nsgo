package main

import (
	"archive/zip"
	"bytes"
	"encoding/binary"
	"encoding/pem"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/authenticode"
	projectcfg "github.com/nsgo-dev/nsgo/project"
)

// TestSignsMSIXWithCertificate goes through the same PKCS#12 certificate loading and file signing as a store build. The block map and payload must survive byte for byte.
func TestSignsMSIXWithCertificate(t *testing.T) {
	dir := t.TempDir()
	key := testWindowsKey(t)
	certificate := filepath.Join(dir, "fixture.pfx")
	writeTestCertificate(t, certificate, key)
	p := &project{Config: projectcfg.Config{Dir: dir, Name: "MSIXFixture", ID: "dev.nsgo.test.msix1414", Version: "1.0.0"}, Release: true}
	p.Sign.Certificate = certificate
	p.Sign.TimestampURL = "none"
	p.Windows.Store.Identity = "NSGoMSIX1414"
	p.Windows.Store.Publisher = "CN=NSGo Test"
	stage := filepath.Join(dir, "package")
	image := buildTestPE(t)
	writeFiles(t, stage, map[string]string{"MSIXFixture.exe": string(image), "notes/测试.txt": "a package asset\n"})
	out, err := buildMSIX(t.Context(), p, stage, "amd64")
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	signer := windowsSignerOf(p)
	if signer == nil {
		t.Fatal("certificate did not select a Windows signer")
	}
	if err := signer.load(p); err != nil {
		t.Fatal(err)
	}
	if err := signer.signFile(t.Context(), out, false); err != nil {
		t.Fatal(err)
	}
	signed, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	read := func(data []byte) map[string][]byte {
		t.Helper()
		r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			t.Fatal(err)
		}
		parts := map[string][]byte{}
		for _, f := range r.File {
			offset, err := f.DataOffset()
			if err != nil {
				t.Fatal(err)
			}
			local := int(offset) - 30 - len(f.Name) - len(f.Extra)
			if got := binary.LittleEndian.Uint16(data[local+4:]); got != 20 || f.ReaderVersion != 20 || f.CreatorVersion != 45 {
				t.Errorf("%s ZIP versions: local=%d central=%d creator=%d, want reader 20 and creator 45", f.Name, got, f.ReaderVersion, f.CreatorVersion)
			}
			v, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			parts[f.Name], err = io.ReadAll(v)
			v.Close()
			if err != nil {
				t.Fatal(err)
			}
		}
		return parts
	}
	before, after := read(unsigned), read(signed)
	signature := after["AppxSignature.p7x"]
	if !bytes.HasPrefix(signature, []byte("PKCX")) {
		t.Fatal("MSIX has no PKCX signature")
	}
	for name, data := range before {
		if name != "[Content_Types].xml" && !bytes.Equal(after[name], data) {
			t.Errorf("signing changed %s", name)
		}
	}
	if !bytes.Contains(after["[Content_Types].xml"], []byte(`ContentType="application/vnd.ms-appx.signature"`)) {
		t.Fatal("signature content type is absent")
	}
	again, err := authenticode.SignMSIX(signed, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read(again)) != len(after) {
		t.Fatal("re-signing duplicated a package part")
	}
	// Export only public verification material when explicitly requested; the private key and PKCS#12 remain in the automatically removed test directory.
	if export := os.Getenv("NSGO_MSIX_SIGN_FIXTURE"); export != "" {
		if err := os.MkdirAll(export, 0o755); err != nil {
			t.Fatal(err)
		}
		pe, err := authenticode.Sign(image, key, nil)
		if err != nil {
			t.Fatal(err)
		}
		for name, data := range map[string][]byte{"signed-pe.exe": pe, "signed.msix": signed, "unsigned.msix": unsigned, "signer.cer": key.Cert.Raw, "signer.pem": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: key.Cert.Raw})} {
			if err := os.WriteFile(filepath.Join(export, name), data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
		// Alter a payload byte without touching the signature or directory, so native verifiers must reject its package digest.
		r, err := zip.NewReader(bytes.NewReader(signed), int64(len(signed)))
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range r.File {
			if f.Name == "MSIXFixture.exe" {
				offset, err := f.DataOffset()
				if err != nil {
					t.Fatal(err)
				}
				tampered := bytes.Clone(signed)
				tampered[offset] ^= 1
				if err := os.WriteFile(filepath.Join(export, "tampered.msix"), tampered, 0o644); err != nil {
					t.Fatal(err)
				}
			}
		}
	}
}
