package apk

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	androidruntime "github.com/nsgo-dev/nsgo/internal/platform/android/runtime"
)

func TestGameMetadata(t *testing.T) {
	m := Manifest{Package: "dev.nsgo.game", Label: "Game"}
	readManifest := func() *xmlNode {
		t.Helper()
		data, err := EncodeManifest(androidruntime.Manifest, m)
		if err != nil {
			t.Fatal(err)
		}
		return decodeXML(t, data).root
	}
	application := readManifest().child(t, "application")
	for _, a := range application.attrs {
		if a.id == 0x01010545 || a.id == 0x010103f4 {
			t.Fatal("an unclassified app has game attributes")
		}
	}

	m.Game, m.Gamepad = true, true
	m.Icon = []byte("icon")
	m.Names = map[string]string{"": "Game", "zh-Hans": "游戏"}
	root := readManifest()
	app := root.child(t, "application")
	if app.value(t, "appCategory") != 0 || app.value(t, "isGame") != ^uint32(0) {
		t.Fatal("the application is missing its game classification")
	}
	meta := app.child(t, "meta-data")
	if meta.string(t, "name") != "android.game_mode_config" || meta.value(t, "resource") != 0x7f030000 {
		t.Fatal("the application has no reference to its Game Mode config")
	}
	features := map[string]bool{}
	for _, kid := range root.kids {
		if kid.name == "uses-feature" {
			features[kid.string(t, "name")] = kid.value(t, "required") == 0
		}
	}
	for _, feature := range []string{"android.hardware.gamepad", "android.hardware.touchscreen"} {
		if !features[feature] {
			t.Errorf("%s is not declared optional", feature)
		}
	}

	m.LoopbackCleartext = true
	app = readManifest().child(t, "application")
	if app.value(t, "networkSecurityConfig") != 0x7f030000 || app.child(t, "meta-data").value(t, "resource") != 0x7f030001 {
		t.Fatal("the game config and network policy do not have separate XML resource IDs")
	}

	key, err := NewKey()
	if err != nil {
		t.Fatal(err)
	}
	for _, format := range []struct {
		name, config, table string
		write               func(io.Writer, []byte, Manifest, []File, *Key) error
	}{
		{"apk", gameModeFile, "resources.arsc", Write},
		{"bundle", "base/" + gameModeFile, "base/resources.pb", WriteBundle},
	} {
		t.Run(format.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := format.write(&buf, androidruntime.Manifest, m, nil, key); err != nil {
				t.Fatal(err)
			}
			z, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			if err != nil {
				t.Fatal(err)
			}
			config, err := z.Open(format.config)
			if err != nil {
				t.Fatal(err)
			}
			mode, err := io.ReadAll(config)
			config.Close()
			if err != nil {
				t.Fatal(err)
			}
			if format.name == "apk" {
				if got := decodeXML(t, mode).root.name; got != "game-mode-config" {
					t.Errorf("game mode XML root = %q", got)
				}
			} else if !bytes.Contains(mode, []byte("game-mode-config")) {
				t.Errorf("bundle's protobuf XML lacks the Game Mode root element")
			}
			table, err := z.Open(format.table)
			if err != nil {
				t.Fatal(err)
			}
			resources, err := io.ReadAll(table)
			table.Close()
			if err != nil || !bytes.Contains(resources, []byte(gameModeFile)) {
				t.Errorf("resource table does not include %s: %v", gameModeFile, err)
			}
		})
	}
}
