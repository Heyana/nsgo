package apk

import (
	"os"
	"strings"
	"testing"
)

// The keystores in testdata were made by OpenSSL (legacy: RC2 and 3DES;
// modern: PBES2 with AES-256) and keytool (PKCS#12, and JKS with a key
// password of its own), all with the store password secret123.
func TestKeystoresGiveTheirSigningKey(t *testing.T) {
	for _, c := range []struct {
		file, alias, keyPassword, subject string
	}{
		{"legacy.p12", "release", "", "NSGo Test"},
		{"modern.p12", "", "", "NSGo Test"},
		{"keytool.p12", "upload", "", "NSGo P12"},
		{"keytool.jks", "upload", "keypass9", "NSGo JKS"},
	} {
		data, err := os.ReadFile("testdata/" + c.file)
		if err != nil {
			t.Fatal(err)
		}
		key, err := ReadKeystore(data, "secret123", c.alias, c.keyPassword)
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if !key.Private.PublicKey.Equal(key.Cert.PublicKey) {
			t.Errorf("%s: the certificate is not the key's", c.file)
		}
		if key.Cert.Subject.CommonName != c.subject {
			t.Errorf("%s: certificate for %q", c.file, key.Cert.Subject.CommonName)
		}
	}
	data, _ := os.ReadFile("testdata/modern.p12")
	if _, err := ReadKeystore(data, "wrong", "", ""); err == nil || !strings.Contains(err.Error(), "password") {
		t.Errorf("a wrong password gives %v", err)
	}
	data, _ = os.ReadFile("testdata/keytool.jks")
	if _, err := ReadKeystore(data, "secret123", "other", "keypass9"); err == nil || !strings.Contains(err.Error(), "upload") {
		t.Errorf("a missing alias gives %v", err)
	}
}
