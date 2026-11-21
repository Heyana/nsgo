package harmony

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
)

func TestHAPSigningChecksTheProfileAndUsesSDK(t *testing.T) {
	dir := t.TempDir()
	cfg := Signing{Keystore: "signing/test.p12", KeyAlias: "test-key", Certificate: "signing/test.cer", Profile: "signing/test.p7b"}
	write := func(path string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, name := range []string{cfg.Keystore, cfg.Certificate, cfg.Profile} {
		write(filepath.Join(dir, name))
	}
	sdk := filepath.Join(dir, "sdk")
	jar := filepath.Join(sdk, "default", "openharmony", "toolchains", "lib", "hap-sign-tool.jar")
	javaHome := filepath.Join(dir, "jdk")
	java := filepath.Join(javaHome, "bin", "java")
	if runtime.GOOS == "windows" {
		java += ".exe"
	}
	write(java)
	for _, env := range []string{"OHOS_BASE_SDK_HOME", "DEVECO_SDK_HOME", "HOS_SDK_HOME", keystorePasswordEnv, keyPasswordEnv} {
		t.Setenv(env, "")
	}
	t.Setenv("JAVA_HOME", javaHome)
	if _, err := OpenSigner(dir, cfg); !errors.Is(err, ErrNoSDK) {
		t.Fatalf("missing tool error = %v", err)
	}
	write(jar)
	t.Setenv("DEVECO_SDK_HOME", sdk)
	if _, err := OpenSigner(dir, Signing{KeyAlias: "test-key"}); err == nil || !strings.Contains(err.Error(), "harmonyos.signing.keystore") {
		t.Fatalf("incomplete configuration error = %v", err)
	}
	signer, err := OpenSigner(dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	old := runSigningTool
	t.Cleanup(func() { runSigningTool = old })
	arg := func(args []string, flag string) string {
		t.Helper()
		i := slices.Index(args, flag)
		if i < 0 || i+1 == len(args) {
			t.Fatalf("signing tool is missing %s", flag)
		}
		return args[i+1]
	}
	profile := `{"verifiedPassed":true,"content":{"bundle-info":{"bundle-name":"dev.nsgo.signing"}}}`
	const storePassword = "synthetic-store"
	const keyPassword = "synthetic-store-key"
	unsigned, signed := filepath.Join(dir, "unsigned.hap"), filepath.Join(dir, "signed.hap")
	var fail bool
	runSigningTool = func(_ context.Context, path string, args ...string) ([]byte, error) {
		if path != java || !slices.Equal(args[:2], []string{"-jar", jar}) {
			t.Fatal("the configured SDK signing tool was not used")
		}
		switch args[2] {
		case "verify-profile":
			if arg(args, "-inFile") != filepath.Join(dir, cfg.Profile) || slices.Contains(args, "-keystorePwd") {
				t.Fatal("profile verification should use the profile and no passwords")
			}
			return nil, os.WriteFile(arg(args, "-outFile"), []byte(profile), 0o600)
		case "sign-app":
			for field, want := range map[string]string{
				"-inFile": unsigned, "-outFile": signed, "-mode": "localSign", "-signAlg": "SHA256withECDSA",
				"-keyAlias": cfg.KeyAlias, "-keystorePwd": storePassword, "-keyPwd": keyPassword, "-profileSigned": "1",
				"-keystoreFile": filepath.Join(dir, cfg.Keystore), "-appCertFile": filepath.Join(dir, cfg.Certificate),
				"-profileFile": filepath.Join(dir, cfg.Profile),
			} {
				if arg(args, field) != want {
					t.Fatalf("incorrect %s argument", field)
				}
			}
			if fail {
				return []byte("signing failure: " + keyPassword + " and " + storePassword), errors.New("exit status 1")
			}
			return nil, os.WriteFile(signed, []byte("signed fixture"), 0o600)
		default:
			t.Fatalf("unexpected signing operation %s", args[2])
			return nil, nil
		}
	}
	if err := signer.CheckProfile(t.Context(), "dev.nsgo.signing"); err != nil {
		t.Fatal(err)
	}
	if err := signer.CheckProfile(t.Context(), "dev.nsgo.different"); err == nil || !strings.Contains(err.Error(), "dev.nsgo.signing") {
		t.Fatalf("mismatched bundle error = %v", err)
	}
	profile = `{"verifiedPassed":false}`
	if err := signer.CheckProfile(t.Context(), "dev.nsgo.signing"); err == nil {
		t.Fatal("an unverified profile was accepted")
	}
	if err := signer.Sign(t.Context(), unsigned, signed); err == nil || !strings.Contains(err.Error(), keystorePasswordEnv) {
		t.Fatalf("missing password error = %v", err)
	}
	t.Setenv(keystorePasswordEnv, storePassword)
	t.Setenv(keyPasswordEnv, keyPassword)
	if err := signer.Sign(t.Context(), unsigned, signed); err != nil {
		t.Fatal(err)
	}
	fail = true
	if err := signer.Sign(t.Context(), unsigned, signed); err == nil || strings.Contains(err.Error(), storePassword) || strings.Contains(err.Error(), keyPassword) {
		t.Fatal("the signing error was missing or disclosed a password")
	}
}
