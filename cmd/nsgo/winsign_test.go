package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"math/big"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/authenticode"
)

// buildTestPE builds a Windows executable, as the tool builds an app's.
func buildTestPE(t *testing.T) []byte {
	t.Helper()
	dir := t.TempDir()
	source := filepath.Join(dir, "main.go")
	if err := os.WriteFile(source, []byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "app.exe")
	cmd := exec.Command("go", "build", "-o", out, source)
	cmd.Env = append(os.Environ(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	if msg, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("no Go build for Windows: %v\n%s", err, msg)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// testWindowsKey makes a self-signed code signing certificate and its key.
func testWindowsKey(t *testing.T) *authenticode.Key {
	t.Helper()
	private, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 64))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "NSGo Test"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.AddDate(1, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageCodeSigning},
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &private.PublicKey, private)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	return &authenticode.Key{Private: private, Cert: cert}
}

// writeTestCertificate writes key as a PKCS#12 file with the password the
// tool reads from NSGO_CERTIFICATE_PASSWORD, as a release certificate is.
func writeTestCertificate(t *testing.T, path string, key *authenticode.Key) {
	t.Helper()
	keytool, err := exec.LookPath("openssl")
	if err != nil {
		t.Skip("no openssl")
	}
	pemKey := filepath.Join(t.TempDir(), "key.pem")
	pemCert := filepath.Join(t.TempDir(), "cert.pem")
	write := func(path, typ string, der []byte) {
		var b strings.Builder
		b.WriteString("-----BEGIN " + typ + "-----\n")
		b.WriteString(base64Lines(der))
		b.WriteString("-----END " + typ + "-----\n")
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(pemKey, "RSA PRIVATE KEY", x509.MarshalPKCS1PrivateKey(key.Private))
	write(pemCert, "CERTIFICATE", key.Cert.Raw)
	t.Setenv("NSGO_CERTIFICATE_PASSWORD", "secret")
	cmd := exec.Command(keytool, "pkcs12", "-export", "-out", path, "-inkey", pemKey, "-in", pemCert, "-passout", "pass:secret")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("openssl pkcs12: %v\n%s", err, out)
	}
}

// base64Lines wraps der in base64 lines of 64 characters, as PEM holds
// them.
func base64Lines(der []byte) string {
	text := base64.StdEncoding.EncodeToString(der)
	var b strings.Builder
	for len(text) > 64 {
		b.WriteString(text[:64] + "\n")
		text = text[64:]
	}
	b.WriteString(text + "\n")
	return b.String()
}

func TestSignsWindowsPackage(t *testing.T) {
	image := buildTestPE(t)
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"App.exe":        string(image),
		"App.com":        string(image),
		"helper.exe":     string(image),
		"lib/native.dll": string(image),
		"tools/tool.exe": string(image),
		"notes.txt":      "not a program",
	})
	// A library signed by its publisher keeps its signature.
	signed := filepath.Join(dir, "lib", "signed.dll")
	key := testWindowsKey(t)
	data, err := authenticode.Sign(image, key, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(signed, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cert := filepath.Join(dir, "release.p12")
	writeTestCertificate(t, cert, key)
	p := &project{Dir: dir, Name: "App", ID: "dev.example.app", Version: "1.0.0", Release: true}
	p.Sign.Certificate = cert
	p.Sign.TimestampURL = "none"
	s := windowsSignerOf(p)
	if s == nil {
		t.Fatal("no signer for a certificate")
	}
	if err := s.signWindowsPackage(t.Context(), p, dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"App.exe", "App.com", "helper.exe", "lib/native.dll", "tools/tool.exe"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if !authenticode.Signed(data) {
			t.Errorf("%s stays unsigned", name)
		}
	}
	// The installer is signed once made, not before.
	data, _ = os.ReadFile(filepath.Join(dir, "setup.exe"))
	if authenticode.Signed(data) {
		t.Error("the setup program was signed before its payload was appended")
	}
	// Signing again keeps one signature.
	before, _ := os.Stat(filepath.Join(dir, "App.exe"))
	if err := s.signWindowsPackage(t.Context(), p, dir); err != nil {
		t.Fatal(err)
	}
	after, _ := os.Stat(filepath.Join(dir, "App.exe"))
	if after.Size() > before.Size() {
		t.Errorf("signing twice grew the executable: %d bytes, then %d", before.Size(), after.Size())
	}
}

// A command signs the files instead, with %1 standing for each path, for a
// certificate on a hardware token or in a cloud service.
func TestSignsWithACommand(t *testing.T) {
	image := buildTestPE(t)
	signed, err := authenticode.Sign(image, testWindowsKey(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "release with spaces")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	fixture := filepath.Join(dir, "signed fixture.exe")
	if err := os.WriteFile(fixture, signed, 0o644); err != nil {
		t.Fatal(err)
	}
	command := "cp " + quoteArg(fixture) + " %1"
	if isWindows() {
		command = "copy /Y " + quoteArg(fixture) + " %1 >NUL"
	}
	p := &project{Dir: dir, Name: "App", Release: true}
	p.Sign.SignCommand = command
	s := windowsSignerOf(p)
	if s == nil {
		t.Fatal("no signer for a command")
	}
	for _, name := range []string{"App.exe", "App-setup.exe"} {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, image, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := s.signFile(t.Context(), path, true); err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, signed) {
			t.Errorf("the command did not write the signed %s", name)
		}
	}
}

func TestSigningCommandMustLeavePESignature(t *testing.T) {
	command := "true"
	if isWindows() {
		command = "exit /b 0"
	}
	s := &windowsSigner{command: command}
	t.Run("PE", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unsigned app.exe")
		if err := os.WriteFile(path, buildTestPE(t), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := s.signFile(t.Context(), path, true); err == nil || !strings.Contains(err.Error(), "Authenticode") {
			t.Fatalf("a successful command left the PE unsigned: %v", err)
		}
	})
	t.Run("non-PE container", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "package.msix")
		if err := os.WriteFile(path, []byte("container fixture"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := s.signFile(t.Context(), path, false); err != nil {
			t.Fatalf("a non-PE signing command was checked as Authenticode: %v", err)
		}
	})
}

func TestSigningWindowsIsChecked(t *testing.T) {
	for _, c := range []struct {
		name   string
		mutate func(*project)
		want   string
	}{
		{"both", func(p *project) { p.Sign.SignCommand = "sign %1" }, "exclude each other"},
		{"no targets", func(p *project) { p.Build.Targets = []string{"macos_arm64"} }, "windows target"},
	} {
		p := &project{Dir: t.TempDir(), Name: "App", Release: true}
		p.Sign.Certificate = "release.p12"
		p.Build.Targets = []string{"windows_x64"}
		c.mutate(p)
		err := p.checkSigningWindows(p.Build.Targets)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: %v, want %q", c.name, err, c.want)
		}
	}
	p := &project{Dir: t.TempDir(), Name: "App", Release: true}
	p.Build.Targets = []string{"windows_x64"}
	if err := p.checkSigningWindows(p.Build.Targets); err != nil {
		t.Errorf("a project that signs nothing: %v", err)
	}
}
