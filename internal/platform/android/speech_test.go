package android

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/testhost"
)

func TestSpeechProtocol(t *testing.T) {
	// 1. ItemSpeech wire encoding matches NsgoView.apply's read order.
	requests := []struct {
		req     driver.SpeechRequest
		interim byte
	}{
		{driver.SpeechRequest{Command: driver.SpeechStart, Lang: "en-US", Interim: true}, 1},
		{driver.SpeechRequest{Command: driver.SpeechStop}, 0},
		{driver.SpeechRequest{Command: driver.SpeechAbort}, 0},
	}
	var w writer
	for _, tc := range requests {
		writeSpeechRequest(&w, 42, tc.req)
	}
	r := reader{w.buf}
	for _, want := range requests {
		count := r.u32()
		cmd := driver.SpeechCommand(r.byte())
		lang := r.str()
		flags := r.byte()
		if count != 42 || cmd != want.req.Command || lang != want.req.Lang || flags != want.interim {
			t.Fatalf("item = count:%d cmd:%v lang:%q flags:%d, want %+v", count, cmd, lang, flags, want)
		}
	}
	if len(r.buf) != 0 {
		t.Fatalf("%d bytes left unread", len(r.buf))
	}

	// 2. EventSpeech decoding through readSpeech.
	for _, tc := range []struct {
		ev   driver.Event
		data string
		want driver.SpeechEvent
	}{
		{
			ev:   driver.Event{Index: int(driver.SpeechStarted)},
			want: driver.SpeechEvent{Phase: driver.SpeechStarted},
		},
		{
			ev:   driver.Event{Index: int(driver.SpeechResult), Text: "hello", Finished: false},
			want: driver.SpeechEvent{Phase: driver.SpeechResult, Text: "hello", Final: false},
		},
		{
			ev:   driver.Event{Index: int(driver.SpeechResult), Text: "hello world", Finished: true},
			want: driver.SpeechEvent{Phase: driver.SpeechResult, Text: "hello world", Final: true},
		},
		{
			ev:   driver.Event{Index: int(driver.SpeechEnded), Text: "hello world"},
			data: driver.SpeechNotAllowed,
			want: driver.SpeechEvent{Phase: driver.SpeechEnded, Text: "hello world", Error: driver.SpeechNotAllowed},
		},
	} {
		got := readSpeech(tc.ev, tc.data)
		if *got != tc.want {
			t.Errorf("readSpeech(%+v, %q) = %+v, want %+v", tc.ev, tc.data, *got, tc.want)
		}
	}

	// 3. The environment's speech bit is capSpeech in the third capability byte, the second's being full: bit 8 of the second (the network) says nothing of speech, and a runtime built before the third byte reads as having none.
	for _, c := range []struct {
		more   byte
		third  []byte
		speech bool
	}{
		{0, nil, false},
		{8, nil, false},
		{8, []byte{capExtendedMessages}, false},
		{0, []byte{capSpeech}, true},
		{0, []byte{capExtendedMessages | capAudio | capSpeech}, true},
	} {
		var ew writer
		ew.floats(0, 0, 0, 0, 0, 1, 2)
		ew.byte(0)
		ew.byte(c.more)
		ew.byte(byte(driver.IdiomPhone))
		ew.str("en-US")
		ew.str("Pixel")
		ew.str("Google Pixel")
		ew.floats(56, 56)
		ew.u32(34)
		ew.byte(0) // thermal state
		ew.byte(0) // low-power mode
		if c.third != nil {
			ew.byte(1)
			ew.str("h23")
			ew.str("metric")
			ew.str("celsius")
			ew.str("US")
			ew.buf = append(ew.buf, c.third...)
		}
		env := readEnvironment(&reader{ew.buf})
		if env.Speech != c.speech {
			t.Errorf("readEnvironment with capability bytes %d and %v: Speech %v, want %v", c.more, c.third, env.Speech, c.speech)
		}
	}

	// Protocol.java's CAP_SPEECH is capSpeech, and NsgoView sets it in the third capability byte.
	testhost.RequireSource(t)
	java, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Protocol.java"))
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`static final int CAP_SPEECH = (\d+);`).FindStringSubmatch(string(java)); m == nil || m[1] != strconv.Itoa(capSpeech) {
		t.Errorf("Protocol.java's CAP_SPEECH is %v, protocol.go's capSpeech %d", m, capSpeech)
	}
	view, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "NsgoView.java"))
	if err != nil {
		t.Fatal(err)
	}
	if m := regexp.MustCompile(`int third = ([^;]*);`).FindStringSubmatch(string(view)); m == nil || !strings.Contains(m[1], "Protocol.CAP_SPEECH") {
		t.Errorf("NsgoView.environment's third capability byte is %v, without Protocol.CAP_SPEECH", m)
	}
}

func TestSpeechJavaConstants(t *testing.T) {
	testhost.RequireSource(t)
	src, err := os.ReadFile(filepath.Join("java", "dev", "oneo", "nsgo", "Speech.java"))
	if err != nil {
		t.Fatal(err)
	}
	values := map[string]int{}
	for _, m := range regexp.MustCompile(`\b(START|STOP|ABORT|STARTED|RESULT|ENDED)\s*=\s*(\d+)`).FindAllStringSubmatch(string(src), -1) {
		values[m[1]], _ = strconv.Atoi(m[2])
	}
	for name, want := range map[string]int{
		"START":   int(driver.SpeechStart),
		"STOP":    int(driver.SpeechStop),
		"ABORT":   int(driver.SpeechAbort),
		"STARTED": int(driver.SpeechStarted),
		"RESULT":  int(driver.SpeechResult),
		"ENDED":   int(driver.SpeechEnded),
	} {
		if got, ok := values[name]; !ok || got != want {
			t.Errorf("Speech.java %s = %d (found %v), want %d", name, got, ok, want)
		}
	}
}

func TestSpeechJava(t *testing.T) {
	javac, java := androidJDK(t)
	dir := t.TempDir()
	stubs := filepath.Join("testdata", "speech")
	var sources []string
	err := filepath.Walk(stubs, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Ext(path) == ".java" {
			sources = append(sources, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sources = append(sources, filepath.Join("java", "dev", "oneo", "nsgo", "Speech.java"))
	args := append([]string{"-d", dir}, sources...)
	if out, err := exec.CommandContext(t.Context(), javac, args...).CombinedOutput(); err != nil {
		t.Fatalf("javac: %v\n%s", err, out)
	}
	if out, err := exec.CommandContext(t.Context(), java, "-cp", dir, "dev.oneo.nsgo.SpeechCheck").CombinedOutput(); err != nil {
		t.Fatalf("SpeechCheck: %v\n%s", err, out)
	}
}
