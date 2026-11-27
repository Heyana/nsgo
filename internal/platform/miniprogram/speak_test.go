package miniprogram

import (
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestBuildEnvironmentSpeakCapability(t *testing.T) {
	for _, tc := range []struct {
		name string
		info deviceInfo
		want bool
	}{
		{name: "unavailable by default", info: deviceInfo{OS: "ios"}, want: false},
		{name: "available when host exposes speaker", info: deviceInfo{OS: "ios", CanSpeak: true}, want: true},
		{name: "available on android when host exposes speaker", info: deviceInfo{OS: "android", CanSpeak: true}, want: true},
	} {
		env := buildEnvironment(driver.Environment{}, tc.info)
		if env.Speak != tc.want {
			t.Errorf("%s: Speak = %v, want %v", tc.name, env.Speak, tc.want)
		}
		if env.Speech || env.Audio {
			t.Errorf("%s: unexpected Speech=%v Audio=%v", tc.name, env.Speech, env.Audio)
		}
	}
}

type speakRecordHost struct {
	fakeHost
	items []driver.MountItem
}

func (h *speakRecordHost) service(it *driver.MountItem) {
	h.items = append(h.items, *it)
}

func TestMounterRoutesItemSpeak(t *testing.T) {
	h := &speakRecordHost{}
	m := newMounter(h)
	start := driver.SpeakRequest{Command: driver.SpeakStart, Text: "Hello", Lang: "en-US", Voice: "WechatSI:en_US", Rate: 1.25, Pitch: 1}
	stop := driver.SpeakRequest{Command: driver.SpeakStop}
	voices := driver.SpeakRequest{Command: driver.SpeakVoices}
	m.mount(&driver.MountBatch{Items: []driver.MountItem{
		{Kind: driver.ItemSpeak, Count: 11, Speak: &start},
		{Kind: driver.ItemSpeak, Count: 11, Speak: &stop},
		{Kind: driver.ItemSpeak, Count: 12, Speak: &voices},
	}})
	if len(h.items) != 3 {
		t.Fatalf("items = %d, want 3", len(h.items))
	}
	if h.items[0].Kind != driver.ItemSpeak || h.items[0].Count != 11 || h.items[0].Speak == nil || *h.items[0].Speak != start {
		t.Fatalf("first service = %+v", h.items[0])
	}
	if h.items[1].Kind != driver.ItemSpeak || h.items[1].Count != 11 || h.items[1].Speak == nil || h.items[1].Speak.Command != driver.SpeakStop {
		t.Fatalf("second service = %+v", h.items[1])
	}
	if h.items[2].Kind != driver.ItemSpeak || h.items[2].Count != 12 || h.items[2].Speak == nil || h.items[2].Speak.Command != driver.SpeakVoices {
		t.Fatalf("third service = %+v", h.items[2])
	}
}
