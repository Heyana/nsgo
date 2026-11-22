package windows

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"slices"
	"testing"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWindowsSpeechStatusAndHResultMapping(t *testing.T) {
	for _, tc := range []struct {
		status        int32
		hasText       bool
		stopRequested bool
		want          string
	}{
		{speechStatusSuccess, false, false, ""},
		{speechStatusUserCanceled, false, false, ""},
		{speechStatusTopicLanguageNotSupported, false, false, driver.SpeechLanguageUnsupported},
		{speechStatusGrammarLanguageMismatch, false, false, driver.SpeechLanguageUnsupported},
		{speechStatusAudioQualityFailure, false, false, driver.SpeechAudioCapture},
		{speechStatusMicrophoneUnavailable, false, false, driver.SpeechAudioCapture},
		{speechStatusNetworkFailure, false, false, driver.SpeechNetwork},
		{speechStatusTimeoutExceeded, false, false, driver.SpeechNoSpeech},
		{speechStatusTimeoutExceeded, true, false, ""},
		{speechStatusPauseLimitExceeded, false, true, ""},
		{speechStatusGrammarCompilationFailure, false, false, driver.SpeechFailed},
		{speechStatusUnknown, false, false, driver.SpeechFailed},
	} {
		if got := speechResultError(tc.status, tc.hasText, tc.stopRequested); got != tc.want {
			t.Errorf("speechResultError(%d, %v, %v) = %q, want %q", tc.status, tc.hasText, tc.stopRequested, got, tc.want)
		}
	}

	for hr, want := range map[uintptr]string{
		0:                             "",
		uintptr(hrSpeechAccessDenied): driver.SpeechNotAllowed,
		uintptr(hrSpeechPrivacyPolicyNotAccepted): driver.SpeechNotAllowed,
		uintptr(hrSpeechMicNotAuthorized):         driver.SpeechNotAllowed,
		uintptr(hrSpeechRecognizerNotFound):       driver.SpeechLanguageUnsupported,
		uintptr(hrSpeechLanguagePackMissing):      driver.SpeechLanguageUnsupported,
		uintptr(hrSpeechAudioFailure):             driver.SpeechAudioCapture,
		uintptr(hrAudclntDeviceInvalidated):       driver.SpeechAudioCapture,
		uintptr(hrMFNoAudioHardware):              driver.SpeechAudioCapture,
		uintptr(hrSpeechClassNotReg):              driver.SpeechUnavailable,
		uintptr(hrSpeechClassNotAvailable):        driver.SpeechUnavailable,
		uintptr(hrInetNameNotResolved):            driver.SpeechNetwork,
		0x80004005:                                driver.SpeechFailed,
	} {
		if got := speechHResultError(hr); got != want {
			t.Errorf("speechHResultError(0x%x) = %q, want %q", hr, got, want)
		}
	}
}

func TestWindowsSpeechLanguageMatching(t *testing.T) {
	supported := []string{"en-US", "en-GB", "ja-JP", "zh-Hans-CN"}
	for want, expected := range map[string]string{
		"en-US":      "en-US",
		"en_us":      "en-US",
		"en":         "en-US",
		"ja":         "ja-JP",
		"zh-Hans-CN": "zh-Hans-CN",
		"fr-FR":      "",
		"":           "",
	} {
		if got := matchSpeechLanguage(want, supported); got != expected {
			t.Errorf("matchSpeechLanguage(%q) = %q, want %q", want, got, expected)
		}
	}
}

func TestWindowsSpeechTranscriptLifecycle(t *testing.T) {
	// Interim enabled: emits hypotheses and intermediate results, then final + end on completion.
	s := speechTranscript{interim: true}
	var got []driver.SpeechEvent
	if ev, ok := s.onStart(); ok {
		got = append(got, ev)
	}
	if _, ok := s.onStart(); ok {
		t.Fatal("duplicate start emitted")
	}
	if ev, ok := s.onHypothesis("hel"); ok {
		got = append(got, ev)
	}
	if ev, ok := s.onResult(speechStatusSuccess, "hello"); ok {
		got = append(got, ev)
	}
	if ev, ok := s.onHypothesis("wor"); ok {
		got = append(got, ev)
	}
	if ev, ok := s.onResult(speechStatusSuccess, "world"); ok {
		got = append(got, ev)
	}
	s.stopRequested = true
	got = append(got, s.onComplete(speechStatusSuccess)...)
	// Late events after completion are ignored.
	if _, ok := s.onHypothesis("late"); ok {
		t.Fatal("late hypothesis emitted")
	}
	if len(s.onComplete(speechStatusSuccess)) != 0 {
		t.Fatal("duplicate completion emitted")
	}

	want := []driver.SpeechEvent{
		{Phase: driver.SpeechStarted},
		{Phase: driver.SpeechResult, Text: "hel", Final: false},
		{Phase: driver.SpeechResult, Text: "hello", Final: false},
		{Phase: driver.SpeechResult, Text: "hello wor", Final: false},
		{Phase: driver.SpeechResult, Text: "hello world", Final: false},
		{Phase: driver.SpeechResult, Text: "hello world", Final: true},
		{Phase: driver.SpeechEnded, Text: "hello world"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("interim events:\ngot  %+v\nwant %+v", got, want)
	}

	// Interim disabled: suppresses partial results until final completion.
	s2 := speechTranscript{interim: false}
	var got2 []driver.SpeechEvent
	if ev, ok := s2.onStart(); ok {
		got2 = append(got2, ev)
	}
	if _, ok := s2.onHypothesis("ignored"); ok {
		t.Fatal("hypothesis emitted with interim=false")
	}
	if _, ok := s2.onResult(speechStatusSuccess, "quiet"); ok {
		t.Fatal("non-final result emitted with interim=false")
	}
	got2 = append(got2, s2.onComplete(speechStatusSuccess)...)
	want2 := []driver.SpeechEvent{
		{Phase: driver.SpeechStarted},
		{Phase: driver.SpeechResult, Text: "quiet", Final: true},
		{Phase: driver.SpeechEnded, Text: "quiet"},
	}
	if !slices.Equal(got2, want2) {
		t.Fatalf("non-interim events:\ngot  %+v\nwant %+v", got2, want2)
	}

	// Timeout with no speech ends with SpeechNoSpeech.
	s3 := speechTranscript{interim: true}
	end3 := s3.onComplete(speechStatusTimeoutExceeded)
	if len(end3) != 1 || end3[0] != (driver.SpeechEvent{Phase: driver.SpeechEnded, Error: driver.SpeechNoSpeech}) {
		t.Fatalf("no-speech completion = %+v", end3)
	}

	// Failure preserves last text and ends once.
	s4 := speechTranscript{interim: true}
	_, _ = s4.onResult(speechStatusSuccess, "partial")
	fail4 := s4.onFail(driver.SpeechNetwork)
	if len(fail4) != 1 || fail4[0] != (driver.SpeechEvent{Phase: driver.SpeechEnded, Text: "partial", Error: driver.SpeechNetwork}) {
		t.Fatalf("failure = %+v", fail4)
	}
	if len(s4.onFail(driver.SpeechFailed)) != 0 {
		t.Fatal("duplicate failure emitted")
	}
}

func TestWindowsSpeechWinRTParameterizedIIDs(t *testing.T) {
	// WinRT parameterized interface IID namespace: 11f47ad5-7b73-42c0-abae-878b1e16adee (RFC 4122 UUIDv5).
	ns, err := hex.DecodeString("11f47ad57b7342c0abae878b1e16adee")
	if err != nil {
		t.Fatal(err)
	}
	derive := func(sig string) string {
		h := sha1.New()
		h.Write(ns)
		h.Write([]byte(sig))
		sum := h.Sum(nil)
		sum[6] = (sum[6] & 0x0f) | 0x50
		sum[8] = (sum[8] & 0x3f) | 0x80
		return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", sum[0:4], sum[4:6], sum[6:8], sum[8:10], sum[10:16])
	}
	for _, tc := range []struct {
		name, sig, want string
	}{
		{
			"IAsyncOperationCompletedHandler<SpeechRecognitionCompilationResult>",
			"pinterface({fcdcf02c-e5d8-4478-915a-4d90b74b83a5};rc(Windows.Media.SpeechRecognition.SpeechRecognitionCompilationResult;{407e6c5d-6ac7-4da4-9cc1-2fce32cf7489}))",
			speechCompileCompletedIIDString,
		},
		{
			"ITypedEventHandler<SpeechRecognizer, SpeechRecognitionHypothesisGeneratedEventArgs>",
			"pinterface({9de1c534-6ae1-11e0-84e1-18a905bcc53f};rc(Windows.Media.SpeechRecognition.SpeechRecognizer;{0bc3c9cb-c26a-40f2-aeb5-8096b2e48073});rc(Windows.Media.SpeechRecognition.SpeechRecognitionHypothesisGeneratedEventArgs;{55161a7a-8023-5866-411d-1213bb271476}))",
			speechHypothesisGeneratedIIDString,
		},
		{
			"ITypedEventHandler<SpeechContinuousRecognitionSession, SpeechContinuousRecognitionResultGeneratedEventArgs>",
			"pinterface({9de1c534-6ae1-11e0-84e1-18a905bcc53f};rc(Windows.Media.SpeechRecognition.SpeechContinuousRecognitionSession;{6a213c04-6614-49f8-99a2-b5e9b3a085c8});rc(Windows.Media.SpeechRecognition.SpeechContinuousRecognitionResultGeneratedEventArgs;{19091e1e-6e7e-5a46-40fb-76594f786504}))",
			speechContinuousResultIIDString,
		},
		{
			"ITypedEventHandler<SpeechContinuousRecognitionSession, SpeechContinuousRecognitionCompletedEventArgs>",
			"pinterface({9de1c534-6ae1-11e0-84e1-18a905bcc53f};rc(Windows.Media.SpeechRecognition.SpeechContinuousRecognitionSession;{6a213c04-6614-49f8-99a2-b5e9b3a085c8});rc(Windows.Media.SpeechRecognition.SpeechContinuousRecognitionCompletedEventArgs;{e3d069bb-e30c-5e18-424b-7fbe81f8fbd0}))",
			speechContinuousCompletedIIDString,
		},
	} {
		if got := derive(tc.sig); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.name, got, tc.want)
		}
	}
}
