package windows

import (
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Windows.Media.SpeechRecognition.SpeechRecognitionResultStatus values.
const (
	speechStatusSuccess                   int32 = 0
	speechStatusTopicLanguageNotSupported int32 = 1
	speechStatusGrammarLanguageMismatch   int32 = 2
	speechStatusGrammarCompilationFailure int32 = 3
	speechStatusAudioQualityFailure       int32 = 4
	speechStatusUserCanceled              int32 = 5
	speechStatusUnknown                   int32 = 6
	speechStatusTimeoutExceeded           int32 = 7
	speechStatusPauseLimitExceeded        int32 = 8
	speechStatusNetworkFailure            int32 = 9
	speechStatusMicrophoneUnavailable     int32 = 10
)

// WinRT HRESULT values relevant to Windows.Media.SpeechRecognition.
const (
	hrSpeechAccessDenied               uint32 = 0x80070005
	hrSpeechPrivacyPolicyNotAccepted   uint32 = 0x80045509
	hrSpeechMicNotAuthorized           uint32 = 0x800455A6
	hrSpeechRecognizerNotFound         uint32 = 0x8004503A
	hrSpeechLanguagePackMissing        uint32 = 0x800455A5
	hrSpeechAudioFailure               uint32 = 0x80045510
	hrSpeechAudioStopped               uint32 = 0x8004503E
	hrSpeechAudioNotFound              uint32 = 0x80045077
	hrAudclntDeviceInvalidated         uint32 = 0x88890004
	hrAudclntEndpointCreateFailed      uint32 = 0x88890008
	hrMFNoAudioHardware                uint32 = 0xC00D36B4
	hrMFDeviceUnavailable              uint32 = 0xC00D4E85
	hrSpeechClassNotReg                uint32 = 0x80040154
	hrSpeechClassNotAvailable          uint32 = 0x80040111
	hrInetNameNotResolved              uint32 = 0x80072EE7
	hrInetCannotConnect                uint32 = 0x80072EFD
	hrInetTimeout                      uint32 = 0x80072EE2
	speechCompileCompletedIIDString           = "78c859bd-14d4-5c40-abff-490616d5e92d"
	speechHypothesisGeneratedIIDString        = "4cb45aba-7573-545a-b29a-e9be35bd4682"
	speechContinuousResultIIDString           = "26192073-a2c9-527d-9bd3-911c05e0011e"
	speechContinuousCompletedIIDString        = "8103c018-7952-59f9-9f41-23b17d6e452d"
)

// speechResultError maps a SpeechRecognitionResultStatus to a driver Speech error, returning "" when the session ended normally or was stopped/cancelled.
func speechResultError(status int32, hasText, stopRequested bool) string {
	switch status {
	case speechStatusSuccess, speechStatusUserCanceled:
		return ""
	case speechStatusTopicLanguageNotSupported, speechStatusGrammarLanguageMismatch:
		return driver.SpeechLanguageUnsupported
	case speechStatusAudioQualityFailure, speechStatusMicrophoneUnavailable:
		return driver.SpeechAudioCapture
	case speechStatusNetworkFailure:
		return driver.SpeechNetwork
	case speechStatusTimeoutExceeded, speechStatusPauseLimitExceeded:
		if hasText || stopRequested {
			return ""
		}
		return driver.SpeechNoSpeech
	default:
		return driver.SpeechFailed
	}
}

// speechHResultError maps a WinRT SpeechRecognizer HRESULT to a driver Speech error.
func speechHResultError(hr uintptr) string {
	code := uint32(hr)
	if code == 0 {
		return ""
	}
	switch code {
	case hrSpeechAccessDenied, hrSpeechPrivacyPolicyNotAccepted, hrSpeechMicNotAuthorized:
		return driver.SpeechNotAllowed
	case hrSpeechRecognizerNotFound, hrSpeechLanguagePackMissing:
		return driver.SpeechLanguageUnsupported
	case hrSpeechAudioFailure, hrSpeechAudioStopped, hrSpeechAudioNotFound,
		hrAudclntDeviceInvalidated, hrAudclntEndpointCreateFailed,
		hrMFNoAudioHardware, hrMFDeviceUnavailable:
		return driver.SpeechAudioCapture
	case hrSpeechClassNotReg, hrSpeechClassNotAvailable:
		return driver.SpeechUnavailable
	case hrInetNameNotResolved, hrInetCannotConnect, hrInetTimeout:
		return driver.SpeechNetwork
	default:
		return driver.SpeechFailed
	}
}

// matchSpeechLanguage finds the installed speech topic language matching want:
// exact case-insensitive match first, then a subtag match (e.g. "en" -> "en-US").
func matchSpeechLanguage(want string, supported []string) string {
	want = strings.ToLower(driver.LanguageTag(strings.TrimSpace(want)))
	if want == "" {
		return ""
	}
	var prefix string
	for _, tag := range supported {
		norm := strings.ToLower(driver.LanguageTag(strings.TrimSpace(tag)))
		if norm == want {
			return tag
		}
		if prefix == "" && strings.HasPrefix(norm, want+"-") {
			prefix = tag
		}
	}
	return prefix
}

// joinSpeechText appends next to committed with a single space separator when both are non-empty.
func joinSpeechText(committed, next string) string {
	next = strings.TrimSpace(next)
	if committed == "" {
		return next
	}
	if next == "" {
		return committed
	}
	return committed + " " + next
}

// speechTranscript manages a single Windows speech recognition session's emitted driver.SpeechEvent transitions on the UI thread.
type speechTranscript struct {
	interim       bool
	committed     string
	lastText      string
	started       bool
	stopRequested bool
	finalSent     bool
	done          bool
}

func (s *speechTranscript) onStart() (driver.SpeechEvent, bool) {
	if s.done || s.started {
		return driver.SpeechEvent{}, false
	}
	s.started = true
	return driver.SpeechEvent{Phase: driver.SpeechStarted}, true
}

func (s *speechTranscript) onHypothesis(text string) (driver.SpeechEvent, bool) {
	if s.done || !s.interim {
		return driver.SpeechEvent{}, false
	}
	full := joinSpeechText(s.committed, text)
	if full == "" || full == s.lastText {
		return driver.SpeechEvent{}, false
	}
	s.lastText = full
	return driver.SpeechEvent{Phase: driver.SpeechResult, Text: full, Final: false}, true
}

func (s *speechTranscript) onResult(status int32, text string) (driver.SpeechEvent, bool) {
	if s.done || status != speechStatusSuccess {
		return driver.SpeechEvent{}, false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return driver.SpeechEvent{}, false
	}
	s.committed = joinSpeechText(s.committed, text)
	s.lastText = s.committed
	if !s.interim {
		return driver.SpeechEvent{}, false
	}
	return driver.SpeechEvent{Phase: driver.SpeechResult, Text: s.committed, Final: false}, true
}

func (s *speechTranscript) onComplete(status int32) []driver.SpeechEvent {
	if s.done {
		return nil
	}
	s.done = true
	finalText := s.committed
	if finalText == "" {
		finalText = s.lastText
	}
	errCode := speechResultError(status, finalText != "", s.stopRequested)
	var out []driver.SpeechEvent
	if errCode == "" && finalText != "" && !s.finalSent {
		s.finalSent = true
		out = append(out, driver.SpeechEvent{Phase: driver.SpeechResult, Text: finalText, Final: true})
	}
	out = append(out, driver.SpeechEvent{Phase: driver.SpeechEnded, Text: finalText, Error: errCode})
	return out
}

func (s *speechTranscript) onFail(errCode string) []driver.SpeechEvent {
	if s.done {
		return nil
	}
	s.done = true
	finalText := s.committed
	if finalText == "" {
		finalText = s.lastText
	}
	return []driver.SpeechEvent{{Phase: driver.SpeechEnded, Text: finalText, Error: errCode}}
}
