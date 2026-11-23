package apk

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

func TestLaunchScreenPackages(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		Package: "dev.nsgo.launchtest", Label: "Launch", VersionCode: 1,
		Icon: []byte("launcher"), Names: map[string]string{"": "Launch"},
		LoopbackCleartext: true,
		Launch: &LaunchScreen{
			Light: LaunchAppearance{Background: 0xff14a7cd, Image: []byte("light")},
			Dark:  LaunchAppearance{Background: 0xff3a125c, Image: []byte("dark")},
		},
	}
	const styleID = 0x7f040000
	var buf bytes.Buffer
	for _, format := range []struct {
		name, prefix string
		write        func(io.Writer, []byte, Manifest, []File, *Key) error
	}{{"apk", "", Write}, {"bundle", "base/", WriteBundle}} {
		t.Run(format.name, func(t *testing.T) {
			buf.Reset()
			if err := format.write(&buf, androidruntime.Manifest, m, nil, key); err != nil {
				t.Fatal(err)
			}
			archive, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatal(err)
			}
			read := func(name string) []byte {
				t.Helper()
				f, err := archive.Open(format.prefix + name)
				if err != nil {
					t.Fatal(err)
				}
				defer f.Close()
				data, err := io.ReadAll(f)
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			for path, want := range map[string]string{launchImageFile: "light", launchNightImageFile: "dark"} {
				if got := string(read(path)); got != want {
					t.Errorf("%s = %q, want %q", path, got, want)
				}
			}
			if format.name == "bundle" {
				manifest := read("manifest/AndroidManifest.xml")
				ref := pbBytes(nil, 1, pbVarint(nil, 2, styleID))
				if !bytes.Contains(manifest, pbBytes(pbVarint(nil, 5, 0x01010000), 6, ref)) {
					t.Fatal("bundle manifest lacks the splash style reference")
				}
				res := read("resources.pb")
				for _, elem := range [][]byte{
					pbString(nil, 2, "NsgoLaunchScreen"),
					pbBytes(nil, 1, pbVarint(pbVarint(nil, 17, 1), 24, 31)),
					pbVarint(nil, 10, 0xff14a7cd),
					pbVarint(nil, 10, 0xff3a125c),
				} {
					if !bytes.Contains(res, elem) {
						t.Errorf("bundle resources lack encoded splash field %x", elem)
					}
				}
				return
			}
			root := decodeXML(t, read("AndroidManifest.xml")).root
			activity := root.child(t, "application").child(t, "activity")
			if attr := activity.attr(t, "theme"); attr.kind != typeReference || attr.data != styleID {
				t.Errorf("launcher theme = %+v, want reference to %#x", attr, styleID)
			}
			aapt2, err := tool("aapt2")
			if err != nil {
				return
			}
			path := filepath.Join(t.TempDir(), "launch.apk")
			if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
				t.Fatal(err)
			}
			out, err := exec.CommandContext(t.Context(), aapt2, "dump", "resources", path).CombinedOutput()
			if err != nil {
				t.Fatalf("aapt2 dump resources: %v\n%s", err, out)
			}
			for _, want := range []string{"0x7f040000 style/NsgoLaunchScreen", "(night-v31)", "(night-v33)",
				"0x0101062c=#ff14a7cd", "0x0101062c=#ff3a125c", "0x0101062d=@drawable/nsgo_launch_image"} {
				if !strings.Contains(string(out), want) {
					t.Errorf("aapt2 could not find %q in the resource table:\n%s", want, out)
				}
			}
		})
	}
}

func TestLaunchScreenPinnedAppearance(t *testing.T) {
	for _, tc := range []struct {
		theme string
		color uint32
		image string
	}{
		{"@android:style/Theme.DeviceDefault.NoActionBar", 0xff000000, "dark"},
		{"@android:style/Theme.DeviceDefault.Light.NoActionBar", 0xffffffff, "light"},
	} {
		m := Manifest{
			Theme: tc.theme,
			Launch: &LaunchScreen{
				Light: LaunchAppearance{Background: 0xffffffff, Image: []byte("light")},
				Dark:  LaunchAppearance{Background: 0xff000000, Image: []byte("dark")},
			},
		}
		variants, drawables := launchValues(m)
		if len(variants) != 3 || len(drawables) != 1 || variants[1].style.items[0].data != tc.color {
			t.Errorf("%s selected variants %+v and drawables %+v", tc.theme, variants, drawables)
		}
		files := launchFiles(m)
		if len(files) != 1 || string(files[0].Data) != tc.image || files[0].Name != launchImageFile {
			t.Errorf("%s selected files %+v", tc.theme, files)
		}
	}
}

// With widgets, the launch screen's types follow their XML and layout types, and the launcher activity's theme names the style there.
func TestLaunchScreenFollowsWidgetLayouts(t *testing.T) {
	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	m := Manifest{
		Package: "dev.nsgo.launchwidgets", Label: "Launch", VersionCode: 1,
		Icon: []byte("launcher"), Names: map[string]string{"": "Launch"},
		Widgets: []Widget{{Kind: "counter", Names: map[string]string{"": "Counter"}, Families: []string{"small"}}},
		Launch:  &LaunchScreen{Light: LaunchAppearance{Background: 0xff14a7cd, Image: []byte("light")}},
	}
	// mipmap 1, string 2, xml 3, layout 4, then style 5 and drawable 6.
	const styleID = 0x7f050000
	if got := launchStyleID(m); got != styleID || got != layoutResourceID(m)+1<<16 {
		t.Fatalf("launch style %#x, want %#x after the layouts at %#x", got, styleID, layoutResourceID(m))
	}
	var buf bytes.Buffer
	if err := Write(&buf, androidruntime.Manifest, m, nil, key); err != nil {
		t.Fatal(err)
	}
	archive, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	f, err := archive.Open("AndroidManifest.xml")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	activity := decodeXML(t, data).root.child(t, "application").child(t, "activity")
	if attr := activity.attr(t, "theme"); attr.kind != typeReference || attr.data != styleID {
		t.Errorf("launcher theme = %+v, want reference to %#x", attr, styleID)
	}
	aapt2, err := tool("aapt2")
	if err != nil {
		return
	}
	path := filepath.Join(t.TempDir(), "launch.apk")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.CommandContext(t.Context(), aapt2, "dump", "resources", path).CombinedOutput()
	if err != nil {
		t.Fatalf("aapt2 dump resources: %v\n%s", err, out)
	}
	for _, want := range []string{"0x7f040000 layout/nsgo_widget", "0x7f050000 style/NsgoLaunchScreen", "0x7f060000 drawable/nsgo_launch_image"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("aapt2 could not find %q in the resource table:\n%s", want, out)
		}
	}
}
