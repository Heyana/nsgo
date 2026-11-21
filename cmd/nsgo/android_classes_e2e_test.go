package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAndroidNativePackagedView(t *testing.T) {
	if os.Getenv("NSGO_ANDROID_E2E") != "1" {
		t.Skip("set NSGO_ANDROID_E2E=1 to run tests on an Android device")
	}
	ctx := t.Context()
	d, err := adbDevice(ctx, os.Getenv("NSGO_ANDROID_DEVICE"))
	if err != nil {
		t.Fatal(err)
	}
	if err := d.prepareTestScreen(ctx); err != nil {
		t.Fatal(err)
	}
	arch, err := d.goarch(ctx)
	if err != nil {
		t.Fatal(err)
	}
	p, err := load(filepath.Join("..", "..", "examples", "android-native"))
	if err != nil {
		t.Fatal(err)
	}
	p = testProject(p)
	apk, err := buildAndroidAPK(ctx, p, t.TempDir(), arch)
	if err != nil {
		t.Fatal(err)
	}
	if err := d.run(ctx, "install", "-r", apk); err != nil {
		t.Fatal(err)
	}
	tree := "/data/local/tmp/" + p.ID + ".xml"
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		d.run(ctx, "shell", "rm", tree)
		d.run(ctx, "uninstall", p.ID)
	})
	if err := d.run(ctx, "shell", "am", "start", "-W", "-n", p.ID+androidComponent); err != nil {
		t.Fatal(err)
	}
	wait, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	for wait.Err() == nil {
		if _, err := d.output(wait, "shell", "uiautomator", "dump", tree); err == nil {
			ui, err := d.output(wait, "shell", "cat", tree)
			if err == nil && strings.Contains(ui, `package="`+p.ID+`"`) &&
				strings.Contains(ui, `text="Android view: Packaged DEX"`) {
				return
			}
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatal("the packaged Android view did not show the caption from its Java setter")
}
