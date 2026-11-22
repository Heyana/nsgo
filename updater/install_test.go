//go:build !nsgo_steam

package updater_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/updater"
)

// signedNewer is a newer version whose signed package the platform can install itself.
var signedNewer = core.UpdateCheck{App: "Hello", Current: "1.0", Version: "1.1", URL: "https://example.com/hello/1.1",
	Download: "https://example.com/hello-1.1-linux-x64.deb", Signature: "c2ln", Key: "a2V5"}

// stagedDownload models the platform's downloader. It waits for release
// when set, then stages the package or reports the download error.
type stagedDownload struct {
	release    chan struct{}
	onDownload func(progress func(driver.UpdateDownloadProgress))
	err        error
	downloaded []string
}

func fakeDownload(t *testing.T) *stagedDownload {
	// The updater stages under the system's temporary directory when the app has no cache.
	t.Setenv("TMPDIR", t.TempDir())
	in := &stagedDownload{}
	download := driver.UpdateDownload
	t.Cleanup(func() { driver.UpdateDownload = download })
	driver.UpdateDownload = func(_ context.Context, u driver.Update, dir string, progress func(driver.UpdateDownloadProgress)) (string, error) {
		if in.onDownload != nil {
			in.onDownload(progress)
		}
		if in.release != nil {
			<-in.release
		}
		if in.err != nil {
			return "", in.err
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return "", err
		}
		path := filepath.Join(dir, filepath.Base(u.Download))
		in.downloaded = append(in.downloaded, path)
		return path, os.WriteFile(path, []byte("package"), 0o600)
	}
	return in
}

// startOn runs an app on os whose Check button checks for updates, with the platform's updater answering found.
func startOn(t *testing.T, os driver.OS, found core.UpdateCheck) *nsgotest.App {
	t.Helper()
	return startIn(t, driver.Environment{OS: os}, found)
}

// startIn is startOn in the environment env.
func startIn(t *testing.T, env driver.Environment, found core.UpdateCheck) *nsgotest.App {
	t.Helper()
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		return base.Button{OnPress: func(core.PressEvent) { updater.Check(cx) },
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)}, Children: []core.Node{core.Text{Content: "Check"}}}
	}, nsgotest.Size(300, 120), nsgotest.Environment(env))
	app.Update = func(bool) core.UpdateCheck { return found }
	return app
}

// A web build that runs as Linux installs nothing: Update opens the release's page.
func TestUpdateOpensThePageInASimulatedLinux(t *testing.T) {
	in := fakeDownload(t)
	app := startIn(t, driver.Environment{OS: driver.OSLinux, Simulated: true}, signedNewer)
	app.Press("text:Check")
	app.Answer(0)
	if !slices.Equal(app.OpenedURLs, []string{signedNewer.URL}) || len(in.downloaded) != 0 || app.Restarts != 0 {
		t.Fatalf("opened %v, downloaded %v, restarts %d", app.OpenedURLs, in.downloaded, app.Restarts)
	}
}

func TestUpdateInstallsAndRestartsOnLinux(t *testing.T) {
	in := fakeDownload(t)
	app := startOn(t, driver.OSLinux, signedNewer)
	app.Press("text:Check")
	app.Answer(0)
	app.WaitUntil(5*time.Second, func() bool { return app.Restarts == 1 })
	if len(in.downloaded) != 1 || len(app.Installs) != 1 || app.Installs[0].Path != in.downloaded[0] {
		t.Fatalf("downloaded %v, installed %+v", in.downloaded, app.Installs)
	}
	if _, err := os.Stat(app.Installs[0].Path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the installed package stayed: %v", err)
	}
	if len(app.OpenedURLs) != 0 || len(app.Alerts()) != 0 {
		t.Errorf("an install opened %v and showed %+v", app.OpenedURLs, app.Alerts())
	}
}

func TestUpdateHandsTheAPKToAndroidsInstaller(t *testing.T) {
	in := fakeDownload(t)
	apk := signedNewer
	apk.Download = "https://example.com/hello-1.1-android-arm64.apk"
	app := startOn(t, driver.OSAndroid, apk)
	app.Press("text:Check")
	app.Answer(0)
	app.WaitUntil(5*time.Second, func() bool { return len(app.Installs) == 1 })
	if len(in.downloaded) != 1 || app.Installs[0].Path != in.downloaded[0] {
		t.Fatalf("installs %+v, downloaded %v", app.Installs, in.downloaded)
	}
	// The installer reads the package after it starts, and Android restarts the app itself.
	if _, err := os.Stat(in.downloaded[0]); err != nil || app.Restarts != 0 {
		t.Errorf("package %v, restarts %d", err, app.Restarts)
	}
	// A check finding the app up to date discards it.
	app.Update = func(bool) core.UpdateCheck { return core.UpdateCheck{App: "Hello", Current: "1.1"} }
	app.Press("text:Check")
	if _, err := os.Stat(in.downloaded[0]); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the staged package stayed once the app was up to date: %v", err)
	}
}

// A user who sees nothing happen while the package downloads may choose Update again, and a check may find the app up to date meanwhile; neither touches the package on its way.
func TestUpdateRunsOnceAtATime(t *testing.T) {
	in := fakeDownload(t)
	in.release = make(chan struct{})
	apk := signedNewer
	apk.Download = "https://example.com/hello-1.1-android-arm64.apk"
	app := startOn(t, driver.OSAndroid, apk)
	app.Press("text:Check")
	app.Answer(0)
	app.Press("text:Check")
	app.Answer(0)
	app.Update = func(bool) core.UpdateCheck { return core.UpdateCheck{App: "Hello", Current: "1.1"} }
	app.Press("text:Check")
	app.Answer(0)
	close(in.release)
	app.WaitUntil(5*time.Second, func() bool { return len(app.Installs) == 1 })
	if len(in.downloaded) != 1 || app.Installs[0].Path != in.downloaded[0] {
		t.Fatalf("installs %+v, downloaded %v", app.Installs, in.downloaded)
	}
	if _, err := os.Stat(in.downloaded[0]); err != nil {
		t.Errorf("the package handed to the installer is gone: %v", err)
	}
	// Once the installer started, Update installs again, as after the user cancelled it.
	app.Update = func(bool) core.UpdateCheck { return apk }
	app.Press("text:Check")
	app.Answer(0)
	app.WaitUntil(5*time.Second, func() bool { return len(app.Installs) == 2 })
}

func TestFailedInstallsOfferThePage(t *testing.T) {
	for _, c := range []struct {
		name                    string
		downloadErr, installErr error
	}{
		{"download", errors.New("updater: update signature is invalid"), nil},
		{"install", nil, errors.New("exit status 126")},
	} {
		t.Run(c.name, func(t *testing.T) {
			in := fakeDownload(t)
			in.err = c.downloadErr
			app := startOn(t, driver.OSLinux, signedNewer)
			app.InstallPackage = func(string) error { return c.installErr }
			app.Press("text:Check")
			app.Answer(0)
			app.WaitUntil(5*time.Second, func() bool { return len(app.Alerts()) == 1 })
			a := app.Alerts()[0]
			if a.Title != "Can’t install the update" || a.Message != "Hello 1.1 couldn’t be downloaded or installed. You can get it from its page instead." ||
				len(a.Actions) != 2 || a.Actions[0].Label != "Open Page" {
				t.Fatalf("alert %+v", a)
			}
			app.Answer(0)
			if !slices.Equal(app.OpenedURLs, []string{signedNewer.URL}) || app.Restarts != 0 {
				t.Errorf("opened %v, restarts %d", app.OpenedURLs, app.Restarts)
			}
			if app.TaskProgress.State != core.TaskProgressNone {
				t.Errorf("failed install left task progress %+v", app.TaskProgress)
			}
		})
	}
}

// A package the platform cannot install itself, as an unsigned appcast's or an unmanaged binary's, opens the page.
func TestUpdateWithoutAPackageOpensThePage(t *testing.T) {
	in := fakeDownload(t)
	app := startOn(t, driver.OSLinux, newer)
	app.Press("text:Check")
	app.Answer(0)
	if !slices.Equal(app.OpenedURLs, []string{newer.URL}) || len(in.downloaded) != 0 {
		t.Errorf("opened %v, downloaded %v", app.OpenedURLs, in.downloaded)
	}
}

// The scope that offered the update, such as a settings page's, may be gone before its package arrives.
func TestInstallOutlivesTheScopeThatOffered(t *testing.T) {
	in := fakeDownload(t)
	in.release = make(chan struct{})
	var page *core.Scope
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		page = cx.Child()
		return base.Button{OnPress: func(core.PressEvent) { updater.Check(page) },
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)}, Children: []core.Node{core.Text{Content: "Check"}}}
	}, nsgotest.Size(300, 120), nsgotest.Environment(driver.Environment{OS: driver.OSLinux}))
	app.Update = func(bool) core.UpdateCheck { return signedNewer }
	app.Press("text:Check")
	app.Answer(0)
	page.Dispose()
	close(in.release)
	app.WaitUntil(5*time.Second, func() bool { return app.Restarts == 1 })
}

func TestUpdateReportsStreamingProgressAndResets(t *testing.T) {
	in := fakeDownload(t)
	in.release = make(chan struct{})
	var closeOnce sync.Once
	release := func() { closeOnce.Do(func() { close(in.release) }) }
	t.Cleanup(release)
	halfway := make(chan struct{})
	in.onDownload = func(progress func(driver.UpdateDownloadProgress)) {
		progress(driver.UpdateDownloadProgress{Loaded: 400, Total: 1000})
		close(halfway)
	}

	var mu sync.Mutex
	var observed updater.State
	pkg := signedNewer
	pkg.Size = 1000
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		cx.Effect(func() {
			s := updater.Status(cx)
			mu.Lock()
			observed = s
			mu.Unlock()
		})
		return base.Button{OnPress: func(core.PressEvent) { updater.Check(cx) },
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(30)}, Children: []core.Node{core.Text{Content: "Check"}}}
	}, nsgotest.Size(300, 120), nsgotest.Environment(driver.Environment{OS: driver.OSLinux}))
	app.Update = func(bool) core.UpdateCheck { return pkg }

	current := func() updater.State {
		mu.Lock()
		defer mu.Unlock()
		return observed
	}
	if s := current(); s.Phase != updater.PhaseIdle {
		t.Fatalf("initial status = %+v, want idle", s)
	}
	app.Press("text:Check")
	app.Answer(0)
	<-halfway
	app.WaitUntil(5*time.Second, func() bool {
		s := current()
		return s.Phase == updater.PhaseDownloading && s.Loaded == 400 && s.Total == 1000
	})
	if s := current(); s.Available != "1.1" || s.Fraction() != 0.4 {
		t.Fatalf("mid-download status = %+v", s)
	}
	if app.TaskProgress.State != core.TaskProgressNormal || app.TaskProgress.Value != 0.4 {
		t.Fatalf("mid-download task progress = %+v, want Normal 0.4", app.TaskProgress)
	}
	release()
	app.WaitUntil(5*time.Second, func() bool { return app.Restarts == 1 })
	if s := current(); s.Phase != updater.PhaseIdle {
		t.Fatalf("final status = %+v, want idle", s)
	}
	if app.TaskProgress.State != core.TaskProgressNone {
		t.Fatalf("final task progress = %+v, want None", app.TaskProgress)
	}
}

func TestUpdateInstallsArchAndAppImagePackagesOnLinux(t *testing.T) {
	for _, download := range []string{
		"https://example.com/hello-1.1-linux-x64.pkg.tar.gz",
		"https://example.com/hello-1.1-linux-x64.pkg.tar.zst",
		"https://example.com/hello-1.1-linux-x64.AppImage",
	} {
		t.Run(filepath.Base(download), func(t *testing.T) {
			fakeDownload(t)
			pkg := signedNewer
			pkg.Download = download
			app := startOn(t, driver.OSLinux, pkg)
			app.Press("text:Check")
			app.Answer(0)
			app.WaitUntil(5*time.Second, func() bool { return app.Restarts == 1 })
			if len(app.Installs) != 1 || filepath.Base(app.Installs[0].Path) != filepath.Base(download) {
				t.Fatalf("installs = %+v, want %s", app.Installs, filepath.Base(download))
			}
			if _, err := os.Stat(app.Installs[0].Path); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("staged package %s stayed: %v", app.Installs[0].Path, err)
			}
		})
	}
}
