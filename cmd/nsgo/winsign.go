package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/cmd/nsgo/apk"
	"github.com/nsgo-dev/nsgo/cmd/nsgo/authenticode"
)

// Windows code signing: sign.certificate, a .pfx, signs the packages'
// programs and libraries with Authenticode in Go, as signtool does, so a
// Mac signs them without the Windows SDK; sign.sign_command signs them
// with a tool of its own instead, for a certificate on a hardware token or
// in a cloud service. SmartScreen warns of a download that is not signed
// by a certificate it knows, so every release signs them.

// defaultTimestampURL is the RFC 3161 server signatures are timestamped
// by without sign.timestamp_url.
const defaultTimestampURL = "http://timestamp.digicert.com"

// windowsSigner signs Windows files, or is nil when the project signs
// none.
type windowsSigner struct {
	// command signs a file in place, %1 standing for its path.
	command string
	// key and stamp sign PE images and MSIX packages in Go.
	key   *authenticode.Key
	stamp authenticode.Stamper
}

// windowsSignerOf is what the project signs Windows packages with: nil
// when it signs none.
func windowsSignerOf(p *project) *windowsSigner {
	if p.Sign.SignCommand != "" {
		return &windowsSigner{command: p.Sign.SignCommand}
	}
	if p.Sign.Certificate == "" || !p.Release {
		return nil
	}
	s := &windowsSigner{}
	// "none" leaves the signatures untimestamped, for a build without the
	// server at hand.
	if url := p.timestampURL(); url != "none" {
		s.stamp = &authenticode.Timestamp{URL: url}
	}
	return s
}

// timestampURL is the server that timestamps signatures.
func (p *project) timestampURL() string {
	if p.Sign.TimestampURL != "" {
		return p.Sign.TimestampURL
	}
	return defaultTimestampURL
}

// signingWindows reports whether the project signs Windows packages.
func (p *project) signingWindows() bool {
	return p.Sign.Certificate != "" || p.Sign.SignCommand != ""
}

// load reads the certificate the signatures use, once, before anything
// builds.
func (s *windowsSigner) load(p *project) error {
	if s.command != "" {
		return nil
	}
	data, err := os.ReadFile(p.Sign.Certificate)
	if err != nil {
		return err
	}
	key, err := apk.ReadKeystore(data, os.Getenv("NSGO_CERTIFICATE_PASSWORD"), "", "")
	if err != nil {
		return fmt.Errorf("%s: %w", rel(p.Sign.Certificate), err)
	}
	s.key = &authenticode.Key{Private: key.Private, Cert: key.Cert}
	return nil
}

// signFile signs a PE image or MSIX package in place with the certificate, or asks the command to sign another format.
func (s *windowsSigner) signFile(ctx context.Context, path string, pe bool) error {
	if s.command != "" {
		if err := s.run(ctx, path); err != nil {
			return err
		}
		if pe {
			data, err := os.ReadFile(path)
			if err != nil {
				return fmt.Errorf("sign.sign_command: %w", err)
			}
			if !authenticode.Signed(data) {
				return fmt.Errorf("sign.sign_command left %s without an Authenticode signature", filepath.Base(path))
			}
		}
		return nil
	}
	if !pe && !strings.EqualFold(filepath.Ext(path), ".msix") {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var signed []byte
	if pe {
		signed, err = authenticode.Sign(data, s.key, s.stamp)
	} else {
		signed, err = authenticode.SignMSIX(data, s.key, s.stamp)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", filepath.Base(path), err)
	}
	return os.WriteFile(path, signed, 0o644)
}

// run invokes sign.sign_command for path.
func (s *windowsSigner) run(ctx context.Context, path string) error {
	line := strings.ReplaceAll(s.command, "%1", quoteArg(path))
	cmd := shellCommand(line)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sign.sign_command: %w\n%s", err, out)
	}
	return nil
}

// shellCommand runs line through the shell, which sign.sign_command's
// pipelines and redirections may need.
func shellCommand(line string) *exec.Cmd {
	if isWindows() {
		return exec.Command("cmd", "/c", line)
	}
	return exec.Command("sh", "-c", line)
}

// quoteArg quotes a path for the shell sign.sign_command runs in.
func quoteArg(path string) string {
	if isWindows() {
		return `"` + path + `"`
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}

// signWindowsPackage signs what a Windows package hands out: the
// executable and its command companion, then the programs and libraries
// among its resources that carry no signature of their own. The installer
// is signed once makensis has made it.
func (s *windowsSigner) signWindowsPackage(ctx context.Context, p *project, dir string) error {
	if err := s.load(p); err != nil {
		return err
	}
	stage(ctx, "signing")
	names := []string{p.Name + ".exe", p.Name + ".com"}
	for _, c := range p.Commands {
		names = append(names, commandFile(c.Name, "windows"))
	}
	for _, name := range names {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			continue
		}
		if err := s.signFile(ctx, path, true); err != nil {
			return err
		}
		detailIn(ctx, "signed %s", name)
	}
	return s.signWindowsResources(ctx, dir, names)
}

// signWindowsResources signs the PE images among a package's files, such
// as helper programs and native libraries, that carry no signature of
// their own, the app's executables, which names lists, aside.
func (s *windowsSigner) signWindowsResources(ctx context.Context, dir string, names []string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".exe", ".dll", ".com", ".sys", ".ocx":
		default:
			return nil
		}
		if slices.ContainsFunc(names, func(name string) bool { return path == filepath.Join(dir, name) }) {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if authenticode.Signed(data) {
			return nil
		}
		if err := s.signFile(ctx, path, true); err != nil {
			return err
		}
		detailIn(ctx, "signed %s", rel(path))
		return nil
	})
}

// The environment makensis runs this tool with to sign the uninstaller it
// writes: the certificate and the timestamp server, since NSIS reads "$"
// and quotes in the script as its own.
const (
	signerEnv     = "NSGO_SIGNER"
	signCertEnv   = "NSGO_SIGN_CERTIFICATE"
	signStampEnv  = "NSGO_SIGN_TIMESTAMP"
	signerCommand = "sign-uninstaller"
)

// uninstallerSignCommand is makensis's hook that signs the uninstaller it
// writes: this tool signs it with the app's certificate, as it signs the
// app. sign.sign_command's own command signs it instead, with makensis's
// %1 standing for the file. NSIS 3.08 is the first with the hook; without
// it the uninstaller stays unsigned, and the installer with it.
func uninstallerSignCommand(ctx context.Context, p *project) (string, []string, error) {
	if p.Sign.SignCommand != "" {
		return strings.ReplaceAll(p.Sign.SignCommand, "'", `$\'`), nil, nil
	}
	compiler := nsisCompiler()
	if compiler == "" {
		// Without makensis the build leaves the script for the user to
		// compile, which signs nothing.
		return "", nil, nil
	}
	if major, minor := nsisCompilerVersion(compiler); major < 3 || major == 3 && minor < 8 {
		detailIn(ctx, "not signing the uninstaller: that needs NSIS 3.08 or later")
		return "", nil, nil
	}
	self, err := os.Executable()
	if err != nil {
		return "", nil, err
	}
	// The path goes through the environment, since NSIS reads "$" and
	// "%" in the script as its own syntax: each system's shell expands
	// its own spelling of a variable.
	line := `"$` + signerEnv + `" ` + signerCommand + ` "%1"`
	if isWindows() {
		line = `"%` + signerEnv + `%" ` + signerCommand + ` "%1"`
	}
	return line, []string{
		signerEnv + "=" + self,
		signCertEnv + "=" + p.Sign.Certificate,
		signStampEnv + "=" + p.timestampURL(),
	}, nil
}

// signUninstaller signs the file makensis names, the uninstaller it made,
// with the certificate the build's environment gave: the hook
// uninstallerSignCommand installs. It is the tool's own command, run by
// makensis alone.
func signUninstaller(path string) error {
	certificate := os.Getenv(signCertEnv)
	if certificate == "" {
		return fmt.Errorf("%s is not set: sign-uninstaller is for makensis, which nsgo build runs", signCertEnv)
	}
	p := &project{Sign: signing{Certificate: certificate, TimestampURL: os.Getenv(signStampEnv)}}
	s := &windowsSigner{}
	if url := p.timestampURL(); url != "none" {
		s.stamp = &authenticode.Timestamp{URL: url}
	}
	if err := s.load(p); err != nil {
		return err
	}
	return s.signFile(context.Background(), path, true)
}

// nsisVersion reads makensis's version, "v3.11-1" or a distribution's
// "v3.08-3".
func nsisCompilerVersion(makensis string) (major, minor int) {
	out, err := exec.Command(makensis, "-VERSION").Output()
	if err != nil {
		return 0, 0
	}
	text, _, _ := strings.Cut(strings.TrimSpace(string(out)), "-")
	fmt.Sscanf(strings.TrimPrefix(text, "v"), "%d.%d", &major, &minor)
	return major, minor
}

// checkSigningWindows refuses, before anything builds, what signing
// Windows packages cannot do: two ways of signing at once, a certificate
// without a password where one is needed, or a build target it cannot
// sign.
func (p *project) checkSigningWindows(targets []string) error {
	if !p.signingWindows() {
		return nil
	}
	if p.Sign.Certificate != "" && p.Sign.SignCommand != "" {
		return errors.New("sign.certificate and sign.sign_command exclude each other: set one")
	}
	if p.Sign.Certificate != "" && !p.Release {
		return errors.New("sign.certificate signs release builds alone; nsgo build signs it")
	}
	if !slices.ContainsFunc(targets, func(t string) bool { return strings.HasPrefix(t, "windows_") }) {
		return errors.New("sign.certificate and sign.sign_command sign Windows packages: add a windows target to build.targets, or leave them out")
	}
	return nil
}

// isWindows reports whether the tool runs on Windows.
func isWindows() bool { return runtime.GOOS == "windows" }
