package main

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
)

// appleUpload identifies the main bundle from the actual store package.
type appleUpload struct {
	bundle, version, build, platform string
}

func publishApple(ctx context.Context, target string, args []string) error {
	flags := flag.NewFlagSet("publish "+target, flag.ExitOnError)
	ext := ".ipa"
	if target == "macos" {
		ext = ".pkg"
	}
	if err := parseCommand(flags, args, 1); err != nil {
		return err
	}
	if file := flags.Arg(0); !strings.EqualFold(filepath.Ext(file), ext) {
		return fmt.Errorf("%s: give the signed %s file from a store build", flags.Name(), ext)
	}
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("%s runs on macOS with Xcode", flags.Name())
	}
	file, err := filepath.Abs(flags.Arg(0))
	if err != nil {
		return err
	}
	uploader, err := newAppleUploader()
	if err != nil {
		return err
	}
	defer uploader.close()
	var build appleUpload
	if target == "ios" {
		build, err = readAppleIPA(ctx, file)
	} else {
		build, err = readApplePkg(ctx, file)
	}
	if err != nil {
		return fmt.Errorf("%s: %w", rel(file), err)
	}
	return uploader.publish(ctx, file, build)
}

func readAppleIPA(ctx context.Context, file string) (appleUpload, error) {
	z, err := zip.OpenReader(file)
	if err != nil {
		return appleUpload{}, err
	}
	defer z.Close()
	var info *zip.File
	for _, f := range z.File {
		parts := strings.SplitN(f.Name, "/", 4)
		if len(parts) != 3 || parts[0] != "Payload" || !strings.HasSuffix(parts[1], ".app") || parts[2] != "Info.plist" {
			continue
		}
		if info != nil {
			return appleUpload{}, errors.New("IPA contains multiple app bundles")
		}
		info = f
	}
	if info == nil {
		return appleUpload{}, errors.New("IPA must contain Payload/*.app/Info.plist")
	}
	r, err := info.Open()
	if err != nil {
		return appleUpload{}, err
	}
	defer r.Close()
	return readAppleUploadPlist(ctx, r, "IOS")
}

func readApplePkg(ctx context.Context, file string) (appleUpload, error) {
	dir, err := os.MkdirTemp("", "nsgo-app-store-*")
	if err != nil {
		return appleUpload{}, err
	}
	defer os.RemoveAll(dir)
	dst := filepath.Join(dir, "package")
	if err := run(ctx, "pkgutil", "--expand-full", file, dst); err != nil {
		return appleUpload{}, err
	}
	var info string
	err = filepath.WalkDir(dst, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() || !strings.HasSuffix(d.Name(), ".app") {
			return nil
		}
		if info != "" {
			return errors.New("PKG contains multiple app bundles")
		}
		info = filepath.Join(path, "Contents", "Info.plist")
		return filepath.SkipDir
	})
	if err != nil {
		return appleUpload{}, err
	}
	if info == "" {
		return appleUpload{}, errors.New("PKG contains no macOS app bundle")
	}
	r, err := os.OpenInRoot(dst, strings.TrimPrefix(info, dst+string(filepath.Separator)))
	if err != nil {
		return appleUpload{}, err
	}
	defer r.Close()
	return readAppleUploadPlist(ctx, r, "MAC_OS")
}

func readAppleUploadPlist(ctx context.Context, r io.Reader, platform string) (appleUpload, error) {
	const maxPlist = 1 << 20
	data, err := io.ReadAll(io.LimitReader(r, maxPlist+1))
	if err != nil {
		return appleUpload{}, err
	}
	if len(data) > maxPlist {
		return appleUpload{}, errors.New("app Info.plist exceeds 1 MiB")
	}
	if bytes.HasPrefix(data, []byte("bplist00")) {
		cmd := exec.CommandContext(ctx, "plutil", "-convert", "xml1", "-o", "-", "-")
		cmd.Stdin = bytes.NewReader(data)
		data, err = cmd.Output()
		if err != nil {
			return appleUpload{}, fmt.Errorf("convert binary Info.plist: %w", err)
		}
	}
	v, err := decodePlist(data)
	if err != nil {
		return appleUpload{}, fmt.Errorf("read app Info.plist: %w", err)
	}
	dict, _ := v.(map[string]any)
	bundle, _ := dict["CFBundleIdentifier"].(string)
	version, _ := dict["CFBundleShortVersionString"].(string)
	build, _ := dict["CFBundleVersion"].(string)
	if bundle == "" || version == "" || build == "" {
		return appleUpload{}, errors.New("app Info.plist needs CFBundleIdentifier, CFBundleShortVersionString and CFBundleVersion")
	}
	if platform == "IOS" {
		platforms, _ := dict["CFBundleSupportedPlatforms"].([]any)
		if !slices.Contains(platforms, any("iPhoneOS")) {
			return appleUpload{}, errors.New("IPA must target iOS devices")
		}
	}
	return appleUpload{bundle: bundle, version: version, build: build, platform: platform}, nil
}
