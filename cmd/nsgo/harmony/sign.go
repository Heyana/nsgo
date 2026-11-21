package harmony

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	keystorePasswordEnv = "NSGO_HARMONY_KEYSTORE_PASSWORD"
	keyPasswordEnv      = "NSGO_HARMONY_KEY_PASSWORD"
)

// Signing names the AppGallery Connect materials for a HarmonyOS HAP.
// Paths are relative to the app's nsgo.yaml unless absolute.
type Signing struct {
	Keystore    string `yaml:"keystore"`
	KeyAlias    string `yaml:"key_alias"`
	Certificate string `yaml:"certificate"`
	Profile     string `yaml:"profile"`
}

// Signer uses the SDK's HAP signing tool with the app's own material.
type Signer struct {
	config    Signing
	java, jar string
}

// OpenSigner checks the signing configuration and locates the SDK tool.
// It reads no passwords, so doctor can check the profile without them.
func OpenSigner(dir string, config Signing) (*Signer, error) {
	if strings.TrimSpace(config.KeyAlias) == "" {
		return nil, fmt.Errorf("harmonyos.signing.key_alias is required")
	}
	for _, item := range []struct {
		name string
		path *string
	}{
		{"keystore", &config.Keystore},
		{"certificate", &config.Certificate},
		{"profile", &config.Profile},
	} {
		if strings.TrimSpace(*item.path) == "" {
			return nil, fmt.Errorf("harmonyos.signing.%s is required", item.name)
		}
		if !filepath.IsAbs(*item.path) {
			*item.path = filepath.Join(dir, *item.path)
		}
		if info, err := os.Stat(*item.path); err != nil {
			return nil, fmt.Errorf("harmonyos.signing.%s: %w", item.name, err)
		} else if !info.Mode().IsRegular() {
			return nil, fmt.Errorf("harmonyos.signing.%s must name a file", item.name)
		}
	}
	jar, err := findSigningTool()
	if err != nil {
		return nil, err
	}
	javaPath := "java"
	if home := os.Getenv("JAVA_HOME"); home != "" {
		javaPath = filepath.Join(home, "bin", "java")
	}
	java, err := exec.LookPath(javaPath)
	if err != nil {
		return nil, fmt.Errorf("the HarmonyOS SDK signing tool needs Java: %w", err)
	}
	return &Signer{config: config, java: java, jar: jar}, nil
}

func findSigningTool() (string, error) {
	for _, env := range []string{"OHOS_BASE_SDK_HOME", "DEVECO_SDK_HOME", "HOS_SDK_HOME"} {
		root := os.Getenv(env)
		if root == "" {
			continue
		}
		openharmony := filepath.Join(root, "openharmony")
		dirs := []string{root, filepath.Join(root, "default", "openharmony"), openharmony}
		for _, parent := range []string{root, openharmony} {
			entries, _ := os.ReadDir(parent)
			for _, entry := range entries {
				if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
					dirs = append(dirs, filepath.Join(parent, entry.Name()))
				}
			}
		}
		for _, dir := range dirs {
			for _, name := range []string{"hap-sign-tool.jar", "hap_sign_tool.jar"} {
				path := filepath.Join(dir, "toolchains", "lib", name)
				if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
					return path, nil
				}
			}
		}
	}
	return "", fmt.Errorf("%w signing tool: set OHOS_BASE_SDK_HOME or DEVECO_SDK_HOME to an SDK with toolchains/lib/hap-sign-tool.jar", ErrNoSDK)
}

// CheckProfile verifies the signed profile with the SDK tool and checks its
// bundle name. It leaves device trust and provisioning checks to HarmonyOS.
func (s *Signer) CheckProfile(ctx context.Context, bundle string) error {
	dir, err := os.MkdirTemp("", "nsgo-hap-profile-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	result := filepath.Join(dir, "profile.json")
	if err := s.run(ctx, nil, "verify-profile", "-inFile", s.config.Profile, "-outFile", result); err != nil {
		return err
	}
	data, err := os.ReadFile(result)
	if err != nil {
		return fmt.Errorf("read the HarmonyOS profile verification: %w", err)
	}
	var profile struct {
		Verified bool `json:"verifiedPassed"`
		Content  struct {
			BundleInfo struct {
				Name string `json:"bundle-name"`
			} `json:"bundle-info"`
		} `json:"content"`
	}
	if err := json.Unmarshal(data, &profile); err != nil {
		return fmt.Errorf("read the HarmonyOS profile verification: %w", err)
	}
	if !profile.Verified {
		return fmt.Errorf("hap-sign-tool could not verify the HarmonyOS profile")
	}
	name := profile.Content.BundleInfo.Name
	if name == "" {
		return fmt.Errorf("the HarmonyOS profile has no bundle name")
	}
	if name != bundle {
		return fmt.Errorf("HarmonyOS profile bundle name %q does not match app id %q", name, bundle)
	}
	return nil
}

// Sign writes a signed HAP to output, which must differ from the input.
func (s *Signer) Sign(ctx context.Context, input, output string) error {
	store, key := os.Getenv(keystorePasswordEnv), os.Getenv(keyPasswordEnv)
	if store == "" {
		return fmt.Errorf("set %s to sign the HarmonyOS HAP", keystorePasswordEnv)
	}
	if key == "" {
		return fmt.Errorf("set %s to sign the HarmonyOS HAP", keyPasswordEnv)
	}
	if err := s.run(ctx, []string{store, key}, "sign-app", "-mode", "localSign", "-signAlg", "SHA256withECDSA",
		"-keystoreFile", s.config.Keystore, "-keystorePwd", store, "-keyAlias", s.config.KeyAlias, "-keyPwd", key,
		"-appCertFile", s.config.Certificate, "-profileFile", s.config.Profile, "-profileSigned", "1",
		"-inFile", input, "-outFile", output); err != nil {
		return err
	}
	info, err := os.Stat(output)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return fmt.Errorf("hap-sign-tool produced no signed HAP")
	}
	return nil
}

func (s *Signer) run(ctx context.Context, secrets []string, args ...string) error {
	out, err := runSigningTool(ctx, s.java, append([]string{"-jar", s.jar}, args...)...)
	if err == nil {
		return nil
	}
	message := strings.TrimSpace(string(out))
	if len(secrets) == 2 && len(secrets[0]) < len(secrets[1]) {
		secrets[0], secrets[1] = secrets[1], secrets[0]
	}
	for _, secret := range secrets {
		message = strings.ReplaceAll(message, secret, "[redacted]")
	}
	if message == "" {
		return fmt.Errorf("hap-sign-tool %s: %w", args[0], err)
	}
	return fmt.Errorf("hap-sign-tool %s: %w\n%s", args[0], err, message)
}

var runSigningTool = func(ctx context.Context, java string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, java, args...).CombinedOutput()
}
