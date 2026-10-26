package main

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

// archiveDir writes dir as an archive at dst, its files under a top
// directory named top: a .tar.gz keeps the executable bit, as Linux
// needs, and a .zip is what Windows opens.
func archiveDir(dir, dst, top string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	var add func(rel string, info fs.FileInfo, data []byte) error
	var finish func() error
	if strings.HasSuffix(dst, ".zip") {
		zw := zip.NewWriter(f)
		add = func(rel string, info fs.FileInfo, data []byte) error {
			h, err := zip.FileInfoHeader(info)
			if err != nil {
				return err
			}
			h.Name, h.Method = top+"/"+rel, zip.Deflate
			w, err := zw.CreateHeader(h)
			if err != nil {
				return err
			}
			_, err = w.Write(data)
			return err
		}
		finish = zw.Close
	} else {
		gz := gzip.NewWriter(f)
		tw := tar.NewWriter(gz)
		add = func(rel string, info fs.FileInfo, data []byte) error {
			h, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return err
			}
			h.Name, h.Uname, h.Gname, h.Uid, h.Gid = top+"/"+rel, "", "", 0, 0
			if err := tw.WriteHeader(h); err != nil {
				return err
			}
			_, err = tw.Write(data)
			return err
		}
		finish = func() error {
			if err := tw.Close(); err != nil {
				return err
			}
			return gz.Close()
		}
	}
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		return add(filepath.ToSlash(rel), info, data)
	})
	if err != nil {
		return err
	}
	if err := finish(); err != nil {
		return err
	}
	return f.Close()
}

// writeIPA packs app, a built .app, as the IPA at dst: its bundle under
// Payload/, as App Store Connect and installers take it.
func writeIPA(app, dst string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	base := filepath.Dir(app)
	err = filepath.WalkDir(app, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(base, p)
		name := "Payload/" + filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		h, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		h.Name = name
		if d.IsDir() {
			h.Name += "/"
			_, err = zw.CreateHeader(h)
			return err
		}
		h.Method = zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// hashedName names data by its content, as a published site names every
// file but its page and manifest, so hosts cache them for good:
// app.wasm becomes app.<10 hex digits>.wasm.
func hashedName(name string, data []byte) string {
	sum := sha256.Sum256(data)
	ext := path.Ext(name)
	return strings.TrimSuffix(name, ext) + "." + hex.EncodeToString(sum[:])[:10] + ext
}

// hashed matches the names hashedName gives, and the directory a site's
// resources go in.
var hashed = regexp.MustCompile(`\.[0-9a-f]{10}(\.[A-Za-z0-9]+|/)`)

// webManifest is a site's manifest.webmanifest, which lets browsers
// install it as an app.
type webManifest struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	ShortName       string    `json:"short_name"`
	StartURL        string    `json:"start_url"`
	Display         string    `json:"display"`
	BackgroundColor string    `json:"background_color,omitempty"`
	ThemeColor      string    `json:"theme_color,omitempty"`
	Icons           []webIcon `json:"icons,omitempty"`
}

type webIcon struct {
	Src   string `json:"src"`
	Sizes string `json:"sizes"`
	Type  string `json:"type"`
}

// appearanceColor is the color a site's page and manifest start in.
func appearanceColor(appearance string) string {
	switch appearance {
	case "dark":
		return "#000000"
	case "light":
		return "#ffffff"
	}
	return ""
}

// writeWebManifest writes dir/manifest.webmanifest for p with its icons.
func writeWebManifest(p *project, dir string, icons []webIcon) error {
	color := appearanceColor(p.Appearance)
	m := webManifest{ID: p.ID, Name: p.Name, ShortName: p.Name, StartURL: ".", Display: "standalone",
		BackgroundColor: color, ThemeColor: color, Icons: icons}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "manifest.webmanifest"), append(data, '\n'), 0o644)
}

// copyHashedResources copies the project's resources into dir under a
// directory named by their content, and returns its name with a slash.
func copyHashedResources(p *project, dir string) (string, error) {
	type file struct {
		rel  string
		data []byte
	}
	var files []file
	if err := walkResources(p, func(rel string, data []byte) error {
		files = append(files, file{rel, data})
		return nil
	}); err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", nil
	}
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	h := sha256.New()
	for _, f := range files {
		fmt.Fprintf(h, "%s\x00%d\x00", f.rel, len(f.data))
		h.Write(f.data)
	}
	name := resourcesDir + "." + hex.EncodeToString(h.Sum(nil))[:10]
	for _, f := range files {
		dst := filepath.Join(dir, name, filepath.FromSlash(f.rel))
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return "", err
		}
		if err := os.WriteFile(dst, f.data, 0o644); err != nil {
			return "", err
		}
	}
	return name + "/", nil
}

// serveSite serves a published site from dir on this machine, with the
// cache headers a CDN should send: named files for good, the page and
// manifest checked each time.
func serveSite(ctx context.Context, dir string) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	files := http.FileServer(http.Dir(dir))
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hashed.MatchString(r.URL.Path) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})}
	go func() {
		<-ctx.Done()
		srv.Close()
	}()
	url := fmt.Sprintf("http://%s/", ln.Addr())
	step("serving %s (interrupt to stop)", url)
	name, args := openCommand(url)
	exec.Command(name, args...).Start()
	if err := srv.Serve(ln); err != http.ErrServerClosed {
		return err
	}
	return nil
}

// runnable reports whether this machine runs target's package with -run.
func runnable(target string) bool {
	arch := goArchName(runtime.GOARCH)
	switch target {
	case "web", "android_arm64", "android_x64":
		return true
	case "ios", "ios_simulator":
		return runtime.GOOS == "darwin"
	case "macos_" + arch:
		return runtime.GOOS == "darwin"
	case "linux_" + arch:
		return runtime.GOOS == "linux"
	case "windows_" + arch:
		return runtime.GOOS == "windows"
	}
	return false
}

// runPackage starts the package built for target in dir, as -run asks:
// the app opens here, in the simulator or on the device -device names
// or the only one connected, or the site is served. An iPhone's package
// is signed again by Xcode, as nsgo dev signs it, since devices run only
// development builds.
func runPackage(ctx context.Context, p *project, target, dir string) error {
	step("run %s", target)
	switch {
	case target == "web":
		return serveSite(ctx, dir)
	case strings.HasPrefix(target, "macos_"):
		return run(ctx, "open", "-n", filepath.Join(dir, p.Name+".app"))
	case target == "ios":
		device, err := selectIOSDevice(ctx, p.Build.device)
		if err != nil {
			return err
		}
		team, err := iosTeam(ctx, p.Build.team)
		if err != nil {
			return err
		}
		if err := installOnIOSDevice(ctx, p, filepath.Join(dir, p.Name+".app"), p.cache("run", "ios"), team, device); err != nil {
			return err
		}
		return run(ctx, "xcrun", "devicectl", "device", "process", "launch", "--terminate-existing", "--device", device.UDID, p.ID)
	case target == "ios_simulator":
		udid, err := bootSimulator(ctx, p.Build.device)
		if err != nil {
			return err
		}
		if err := run(ctx, "xcrun", "simctl", "install", udid, filepath.Join(dir, p.Name+".app")); err != nil {
			return err
		}
		return run(ctx, "xcrun", "simctl", "launch", "--terminate-running-process", udid, p.ID)
	case strings.HasPrefix(target, "android_"):
		d, err := adbDevice(ctx, p.Build.device)
		if err != nil {
			return err
		}
		if err := d.run(ctx, "install", "-r", filepath.Join(dir, p.Name+".apk")); err != nil {
			return err
		}
		return d.run(ctx, "shell", "am", "start", "-S", "-n", p.ID+androidComponent)
	case strings.HasPrefix(target, "linux_"):
		return startDetached(filepath.Join(dir, p.Name))
	case strings.HasPrefix(target, "windows_"):
		return startDetached(filepath.Join(dir, p.Name+".exe"))
	}
	return fmt.Errorf("-run cannot run %s here", target)
}

// startDetached starts bin, which keeps running after nsgo exits.
func startDetached(bin string) error {
	cmd := exec.Command(bin)
	cmd.Dir = filepath.Dir(bin)
	return cmd.Start()
}
