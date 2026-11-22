//go:build windows

package windows

import (
	"log"
	"sync"
	"unsafe"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// Speech recognition through WinRT's Windows.Media.SpeechRecognition.SpeechRecognizer and its ContinuousRecognitionSession: one session in the process at a time, a new one aborting the one before. Its state is the UI thread's; WinRT completion and session callbacks run on worker threads and post extracted values back to the window.

var (
	iidLanguageFactory                 = parseGUID("9b0252ac-0c27-44f8-b792-9793fb66c63e")
	iidLanguage                        = parseGUID("ea79a752-f7c2-4265-b1bd-c4dec4e4f080")
	iidSpeechRecognizerStatics         = parseGUID("87a35eac-a7dc-4b0b-bcc9-24f47c0b7ebf")
	iidSpeechRecognizerFactory         = parseGUID("60c488dd-7fb8-4033-ac70-d046f64818e1")
	iidSpeechRecognizer                = parseGUID("0bc3c9cb-c26a-40f2-aeb5-8096b2e48073")
	iidSpeechRecognizer2               = parseGUID("63c9baf1-91e3-4ea4-86a1-7c3867d084a6")
	iidSpeechContinuousSession         = parseGUID("6a213c04-6614-49f8-99a2-b5e9b3a085c8")
	iidSpeechCompilationResult         = parseGUID("407e6c5d-6ac7-4da4-9cc1-2fce32cf7489")
	iidSpeechRecognitionResult         = parseGUID("4e303157-034e-4652-857e-d0454cc4beec")
	iidSpeechHypothesisArgs            = parseGUID("55161a7a-8023-5866-411d-1213bb271476")
	iidSpeechHypothesis                = parseGUID("7a7b25b0-99c5-4f7d-bf84-10aa1302b634")
	iidSpeechContinuousResultArgs      = parseGUID("19091e1e-6e7e-5a46-40fb-76594f786504")
	iidSpeechContinuousCompletedArgs   = parseGUID("e3d069bb-e30c-5e18-424b-7fbe81f8fbd0")
	iidSpeechCompileCompleted          = parseGUID(speechCompileCompletedIIDString)
	iidSpeechHypothesisGenerated       = parseGUID(speechHypothesisGeneratedIIDString)
	iidSpeechContinuousResultGenerated = parseGUID(speechContinuousResultIIDString)
	iidSpeechContinuousCompleted       = parseGUID(speechContinuousCompletedIIDString)
)

// WinRT vtable slot indices after IInspectable's six methods.
const (
	languageFactoryCreate         = 6
	languageTagSlot               = 6
	speechStaticsSupportedTopic   = 7
	speechFactoryCreate           = 6
	speechCompileConstraintsAsync = 10
	speech2ContinuousSession      = 6
	speech2AddHypothesis          = 9
	speech2RemoveHypothesis       = 10
	continuousStartAsync          = 8
	continuousStopAsync           = 10
	continuousCancelAsync         = 11
	continuousAddCompleted        = 14
	continuousRemoveCompleted     = 15
	continuousAddResult           = 16
	continuousRemoveResult        = 17
	compilationStatusSlot         = 6
	resultStatusSlot              = 6
	resultTextSlot                = 7
	hypothesisArgsHypothesisSlot  = 6
	hypothesisTextSlot            = 6
	continuousResultArgsSlot      = 6
	continuousCompletedStatusSlot = 6
	asyncInfoErrorCodeSlot        = 8
	closableCloseSlot             = 6

	speechLanguageClass   = "Windows.Globalization.Language"
	speechRecognizerClass = "Windows.Media.SpeechRecognition.SpeechRecognizer"
)

type speechRun struct {
	w          *window
	id         uint32
	state      speechTranscript
	listening  bool
	recognizer *comObject
	rec2       *comObject
	session    *comObject

	hasHypothesis bool
	hypothesisTok eventToken
	hasResult     bool
	resultTok     eventToken
	hasCompleted  bool
	completedTok  eventToken
}

var speechSession struct {
	run *speechRun
}

// speechAvailable is Environment.Speech: Windows 10 and later register SpeechRecognizer.
var speechAvailable = sync.OnceValue(func() bool {
	comApartment()
	statics := activationFactory(speechRecognizerClass, &iidSpeechRecognizerStatics)
	statics.release()
	return statics != nil
})

// speech carries out an ItemSpeech for session id on the UI thread.
func (w *window) speech(id uint32, r driver.SpeechRequest) {
	switch r.Command {
	case driver.SpeechStart:
		if speechSession.run != nil {
			speechSession.run.teardown(true)
		}
		run := &speechRun{
			w:     w,
			id:    id,
			state: speechTranscript{interim: r.Interim},
		}
		speechSession.run = run
		run.begin(r.Lang)
	case driver.SpeechStop:
		run := speechSession.run
		if run == nil || run.w != w || run.id != id || run.state.done {
			return
		}
		run.state.stopRequested = true
		if !run.listening || run.session == nil {
			run.complete(speechStatusUserCanceled)
			return
		}
		var stopOp *comObject
		hr := run.session.call(continuousStopAsync, uintptr(unsafe.Pointer(&stopOp)))
		if failed(hr) || stopOp == nil {
			run.complete(speechStatusUserCanceled)
			return
		}
		// If StopAsync fails asynchronously (for instance if the session already stopped), ensure the session finishes rather than hanging.
		stopDone := newHandler(&iidAsyncActionCompleted, func(_ uintptr, status unsafe.Pointer) {
			ok := uintptr(status) == asyncStatusCompleted
			if ok && failed(stopOp.call(asyncGetResults)) {
				ok = false
			}
			stopOp.release()
			if !ok {
				run.w.post(func() {
					if speechSession.run == run && !run.state.done {
						run.complete(speechStatusUserCanceled)
					}
				})
			}
		})
		stopDone.agile = true
		if failed(stopOp.call(asyncPutCompleted, stopDone.ptr())) {
			stopOp.release()
		}
		stopDone.release()
	case driver.SpeechAbort:
		if run := speechSession.run; run != nil && run.w == w && run.id == id {
			run.teardown(true)
		}
	}
}

// closeSpeech tears down any active speech session owned by window w as it closes.
func (w *window) closeSpeech() {
	if run := speechSession.run; run != nil && run.w == w {
		run.teardown(true)
	}
}

func (run *speechRun) begin(lang string) {
	comApartment()
	rec, errCode := createWindowsSpeechRecognizer(lang)
	if rec == nil {
		run.fail(errCode)
		return
	}
	run.recognizer = rec
	rec2 := rec.query(&iidSpeechRecognizer2)
	if rec2 == nil {
		run.fail(driver.SpeechUnavailable)
		return
	}
	run.rec2 = rec2
	var session *comObject
	if hr := rec2.call(speech2ContinuousSession, uintptr(unsafe.Pointer(&session))); failed(hr) || session == nil {
		run.fail(speechHResultError(hr))
		return
	}
	run.session = session

	if run.state.interim {
		hyp := newHandler(&iidSpeechHypothesisGenerated, func(_ uintptr, rawArgs unsafe.Pointer) {
			if rawArgs == nil {
				return
			}
			args := (*comObject)(rawArgs)
			var hypothesis *comObject
			if failed(args.call(hypothesisArgsHypothesisSlot, uintptr(unsafe.Pointer(&hypothesis)))) || hypothesis == nil {
				return
			}
			text := winrtStringProp(hypothesis, hypothesisTextSlot)
			hypothesis.release()
			run.w.post(func() {
				if speechSession.run != run || run.state.done {
					return
				}
				if ev, ok := run.state.onHypothesis(text); ok {
					run.dispatch(ev)
				}
			})
		})
		hyp.agile = true
		var tok eventToken
		hr := rec2.call(speech2AddHypothesis, hyp.ptr(), uintptr(unsafe.Pointer(&tok)))
		hyp.release()
		if !failed(hr) {
			run.hasHypothesis = true
			run.hypothesisTok = tok
		}
	}

	resHandler := newHandler(&iidSpeechContinuousResultGenerated, func(_ uintptr, rawArgs unsafe.Pointer) {
		if rawArgs == nil {
			return
		}
		args := (*comObject)(rawArgs)
		var result *comObject
		if failed(args.call(continuousResultArgsSlot, uintptr(unsafe.Pointer(&result)))) || result == nil {
			return
		}
		var status int32
		if failed(result.call(resultStatusSlot, uintptr(unsafe.Pointer(&status)))) {
			status = speechStatusUnknown
		}
		text := winrtStringProp(result, resultTextSlot)
		result.release()
		run.w.post(func() {
			if speechSession.run != run || run.state.done {
				return
			}
			if ev, ok := run.state.onResult(status, text); ok {
				run.dispatch(ev)
			}
		})
	})
	resHandler.agile = true
	var resTok eventToken
	hr := session.call(continuousAddResult, resHandler.ptr(), uintptr(unsafe.Pointer(&resTok)))
	resHandler.release()
	if failed(hr) {
		run.fail(speechHResultError(hr))
		return
	}
	run.hasResult = true
	run.resultTok = resTok

	doneHandler := newHandler(&iidSpeechContinuousCompleted, func(_ uintptr, rawArgs unsafe.Pointer) {
		status := speechStatusUnknown
		if rawArgs != nil {
			args := (*comObject)(rawArgs)
			var s int32
			if !failed(args.call(continuousCompletedStatusSlot, uintptr(unsafe.Pointer(&s)))) {
				status = s
			}
		}
		run.w.post(func() {
			if speechSession.run != run || run.state.done {
				return
			}
			run.complete(status)
		})
	})
	doneHandler.agile = true
	var doneTok eventToken
	hr = session.call(continuousAddCompleted, doneHandler.ptr(), uintptr(unsafe.Pointer(&doneTok)))
	doneHandler.release()
	if failed(hr) {
		run.fail(speechHResultError(hr))
		return
	}
	run.hasCompleted = true
	run.completedTok = doneTok

	var compileOp *comObject
	if hr := rec.call(speechCompileConstraintsAsync, uintptr(unsafe.Pointer(&compileOp))); failed(hr) || compileOp == nil {
		log.Print(hresultError("nsgo: speech: CompileConstraintsAsync", hr))
		run.fail(speechHResultError(hr))
		return
	}
	compileDone := newHandler(&iidSpeechCompileCompleted, func(_ uintptr, status unsafe.Pointer) {
		var compStatus int32 = speechStatusUnknown
		var errHR uintptr
		if uintptr(status) == asyncStatusCompleted {
			var comp *comObject
			errHR = compileOp.call(asyncGetResults, uintptr(unsafe.Pointer(&comp)))
			if !failed(errHR) && comp != nil {
				if failed(comp.call(compilationStatusSlot, uintptr(unsafe.Pointer(&compStatus)))) {
					compStatus = speechStatusUnknown
				}
				comp.release()
			}
		} else {
			errHR = asyncErrorCode(compileOp)
		}
		compileOp.release()
		run.w.post(func() {
			if speechSession.run != run || run.state.done {
				return
			}
			if failed(errHR) {
				run.fail(speechHResultError(errHR))
				return
			}
			if compStatus != speechStatusSuccess {
				run.fail(speechResultError(compStatus, false, false))
				return
			}
			run.startListening()
		})
	})
	compileDone.agile = true
	hr = compileOp.call(asyncPutCompleted, compileDone.ptr())
	compileDone.release()
	if failed(hr) {
		compileOp.release()
		run.fail(speechHResultError(hr))
	}
}

func (run *speechRun) startListening() {
	if speechSession.run != run || run.state.done || run.session == nil {
		return
	}
	var startOp *comObject
	hr := run.session.call(continuousStartAsync, uintptr(unsafe.Pointer(&startOp)))
	if failed(hr) || startOp == nil {
		log.Print(hresultError("nsgo: speech: ContinuousRecognitionSession.StartAsync", hr))
		run.fail(speechHResultError(hr))
		return
	}
	startDone := newHandler(&iidAsyncActionCompleted, func(_ uintptr, status unsafe.Pointer) {
		var errHR uintptr
		if uintptr(status) == asyncStatusCompleted {
			errHR = startOp.call(asyncGetResults)
		} else {
			errHR = asyncErrorCode(startOp)
		}
		startOp.release()
		run.w.post(func() {
			if speechSession.run != run || run.state.done {
				return
			}
			if failed(errHR) {
				run.fail(speechHResultError(errHR))
				return
			}
			run.listening = true
			if ev, ok := run.state.onStart(); ok {
				run.dispatch(ev)
			}
		})
	})
	startDone.agile = true
	hr = startOp.call(asyncPutCompleted, startDone.ptr())
	startDone.release()
	if failed(hr) {
		startOp.release()
		run.fail(speechHResultError(hr))
	}
}

func (run *speechRun) dispatch(ev driver.SpeechEvent) {
	run.w.engine.Dispatch(driver.Event{Kind: driver.EventSpeech, Count: run.id, Data: &ev})
}

func (run *speechRun) complete(status int32) {
	events := run.state.onComplete(status)
	run.teardown(false)
	for _, ev := range events {
		run.dispatch(ev)
	}
}

func (run *speechRun) fail(errCode string) {
	if errCode == "" {
		errCode = driver.SpeechFailed
	}
	events := run.state.onFail(errCode)
	run.teardown(false)
	for _, ev := range events {
		run.dispatch(ev)
	}
}

func (run *speechRun) teardown(cancel bool) {
	run.state.done = true
	run.listening = false
	if speechSession.run == run {
		speechSession.run = nil
	}
	if run.rec2 != nil && run.hasHypothesis {
		run.rec2.call(speech2RemoveHypothesis, u64Args(uint64(run.hypothesisTok.value))...)
		run.hasHypothesis = false
	}
	if run.session != nil {
		if run.hasResult {
			run.session.call(continuousRemoveResult, u64Args(uint64(run.resultTok.value))...)
			run.hasResult = false
		}
		if run.hasCompleted {
			run.session.call(continuousRemoveCompleted, u64Args(uint64(run.completedTok.value))...)
			run.hasCompleted = false
		}
		if cancel {
			var cancelOp *comObject
			if !failed(run.session.call(continuousCancelAsync, uintptr(unsafe.Pointer(&cancelOp)))) && cancelOp != nil {
				cancelOp.release()
			}
		}
		run.session.release()
		run.session = nil
	}
	if run.rec2 != nil {
		run.rec2.release()
		run.rec2 = nil
	}
	if run.recognizer != nil {
		if closable := run.recognizer.query(&iidClosable); closable != nil {
			closable.call(closableCloseSlot)
			closable.release()
		}
		run.recognizer.release()
		run.recognizer = nil
	}
}

// createWindowsSpeechRecognizer creates a SpeechRecognizer for lang (or the system speech language when lang is empty).
func createWindowsSpeechRecognizer(lang string) (*comObject, string) {
	if lang == "" {
		rec := activate(speechRecognizerClass, &iidSpeechRecognizer)
		if rec == nil {
			return nil, driver.SpeechUnavailable
		}
		return rec, ""
	}
	factory := activationFactory(speechRecognizerClass, &iidSpeechRecognizerFactory)
	if factory == nil {
		return nil, driver.SpeechUnavailable
	}
	defer factory.release()

	resolved := lang
	if supported := windowsSpeechTopicLanguages(); len(supported) > 0 {
		if matched := matchSpeechLanguage(lang, supported); matched != "" {
			resolved = matched
		}
	}

	langObj := createWinRTLanguage(resolved)
	if langObj == nil {
		return nil, driver.SpeechLanguageUnsupported
	}
	defer langObj.release()

	var rec *comObject
	hr := factory.call(speechFactoryCreate, langObj.ptr(), uintptr(unsafe.Pointer(&rec)))
	if failed(hr) || rec == nil {
		code := speechHResultError(hr)
		if code == driver.SpeechFailed {
			code = driver.SpeechLanguageUnsupported
		}
		return nil, code
	}
	return rec, ""
}

func createWinRTLanguage(tag string) *comObject {
	factory := activationFactory(speechLanguageClass, &iidLanguageFactory)
	if factory == nil {
		return nil
	}
	defer factory.release()
	h := hstring(tag)
	defer deleteHString(h)
	var lang *comObject
	if failed(factory.call(languageFactoryCreate, h, uintptr(unsafe.Pointer(&lang)))) {
		return nil
	}
	return lang
}

func windowsSpeechTopicLanguages() []string {
	statics := activationFactory(speechRecognizerClass, &iidSpeechRecognizerStatics)
	if statics == nil {
		return nil
	}
	defer statics.release()
	var list *comObject
	if failed(statics.call(speechStaticsSupportedTopic, uintptr(unsafe.Pointer(&list)))) || list == nil {
		return nil
	}
	defer list.release()
	var n uint32
	if failed(list.call(vectorViewSize, uintptr(unsafe.Pointer(&n)))) || n == 0 {
		return nil
	}
	out := make([]string, 0, n)
	for i := range n {
		var item *comObject
		if failed(list.call(vectorViewGetAt, uintptr(i), uintptr(unsafe.Pointer(&item)))) || item == nil {
			continue
		}
		if tag := winrtStringProp(item, languageTagSlot); tag != "" {
			out = append(out, tag)
		}
		item.release()
	}
	return out
}

func winrtStringProp(obj *comObject, slot int) string {
	var h uintptr
	if failed(obj.call(slot, uintptr(unsafe.Pointer(&h)))) {
		return ""
	}
	defer deleteHString(h)
	return goHString(h)
}

func asyncErrorCode(op *comObject) uintptr {
	info := op.query(&iidAsyncInfo)
	if info == nil {
		return 0x80004005
	}
	defer info.release()
	var errHR int32
	if failed(info.call(asyncInfoErrorCodeSlot, uintptr(unsafe.Pointer(&errHR)))) || errHR == 0 {
		return 0x80004005
	}
	return uintptr(uint32(errHR))
}
