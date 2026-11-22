//go:build darwin

package apple

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
	"github.com/nsgo-dev/nsgo/internal/driver"
	"github.com/nsgo-dev/nsgo/internal/platform/objcblock"
)

// downloadHost and its delegates belong to the main queue. URLSession owns
// the transfer; workers publish finished files without holding that queue.
type downloadHost struct {
	directory, identifier string
	home                  string
	background            bool
	session, delegate     objc.ID
	tasks                 map[string]*downloadTask
	ready                 bool
	waiting               []driver.DownloadCommand
	publishing            int
	finishedEvents        bool
	completion            objc.ID
}

type downloadTask struct {
	driver.DownloadProgress
	Request    driver.DownloadCommand
	ResumeData []byte
	Staged     string
	Home       string
	native     objc.ID
	nativeID   uint
	stopping   string
	publishing bool
	lastReport time.Time
}

var (
	configuredDownloads   *downloadManager
	downloadDelegateClass objc.Class
	downloadDelegates     = map[objc.ID]*downloadHost{}
)

// Each exact pin has its own URLSession. A TLS connection trusted for a
// pinned task must never carry a task that requires system trust or another
// certificate, because URLSession can reuse an authenticated connection.
type downloadManager struct {
	directory, identifier string
	background            bool
	hosts                 map[string]*downloadHost
}

// ConfigureDownloads connects the app-wide transfer channel. Background
// sessions reconnect at launch when a previous transfer left a journal.
// Call on the main thread before starting the app's engine.
func ConfigureDownloads(dataDir, identifier string, background bool) {
	manager := &downloadManager{
		directory: filepath.Join(dataDir, "nsgo-downloads"), identifier: identifier + ".nsgo.downloads",
		background: background, hosts: make(map[string]*downloadHost),
	}
	configuredDownloads = manager
	driver.ConnectDownloads(func(command driver.DownloadCommand) error {
		command.Headers = maps.Clone(command.Headers)
		OnMain(func() { manager.command(command) })
		return nil
	})
	entries, _ := os.ReadDir(manager.directory)
	for _, entry := range entries {
		if entry.IsDir() && downloadPolicyValid(entry.Name()) {
			manager.host(entry.Name()).open()
		}
	}
}

func downloadPolicy(pin string) string {
	if pin == "" {
		return "system"
	}
	return fmt.Sprintf("pin-%x", sha256.Sum256([]byte(pin)))
}

func downloadPolicyValid(policy string) bool {
	if policy == "system" {
		return true
	}
	digest, ok := strings.CutPrefix(policy, "pin-")
	if !ok || len(digest) != 64 {
		return false
	}
	_, err := hex.DecodeString(digest)
	return err == nil
}

func (m *downloadManager) host(policy string) *downloadHost {
	if host := m.hosts[policy]; host != nil {
		return host
	}
	host := &downloadHost{directory: filepath.Join(m.directory, policy), identifier: m.identifier + "." + policy, background: m.background}
	m.hosts[policy] = host
	return host
}

func (m *downloadManager) command(command driver.DownloadCommand) {
	if command.Op == "start" {
		host := m.host(downloadPolicy(command.CertificatePEM))
		if err := host.open(); err != nil {
			driver.ReportDownload(driver.DownloadProgress{ID: command.ID, Path: command.Path, State: "failed", Total: -1, Error: "cannot open the download journal"})
			return
		}
		host.command(command)
		return
	}
	for _, host := range m.hosts {
		if err := host.open(); err == nil {
			host.command(command)
		}
	}
}

// HandleDownloadEvents retains UIKit's background-session completion block
// until the session's events and file publications have finished. It reports
// whether the session identifier belongs to NSGo's transfer host.
func HandleDownloadEvents(identifier string, completion objc.ID) bool {
	manager := configuredDownloads
	if manager == nil || !manager.background {
		return false
	}
	policy, ok := strings.CutPrefix(identifier, manager.identifier+".")
	if !ok || !downloadPolicyValid(policy) {
		return false
	}
	host := manager.host(policy)
	host.finishedEvents = false
	if err := host.open(); err != nil {
		CallBlock(completion)
		return true
	}
	host.completion = completion.Send(sel("copy"))
	return true
}

func (h *downloadHost) open() error {
	if h.home == "" {
		h.home = os.Getenv("HOME")
	}
	if h.session != 0 {
		return nil
	}
	if err := os.MkdirAll(h.directory, 0o700); err != nil {
		return err
	}
	h.tasks = make(map[string]*downloadTask)
	if err := h.restore(); err != nil {
		return err
	}
	if h.background {
		// Resume data can contain credentials. Its private directory remains
		// available to a background relaunch after the first device unlock.
		attrs := JSONObject(map[string]string{"NSFileProtectionKey": "NSFileProtectionCompleteUntilFirstUserAuthentication"})
		manager := objc.ID(Class("NSFileManager")).Send(sel("defaultManager"))
		var failure objc.ID
		if !objc.Send[bool](manager, sel("setAttributes:ofItemAtPath:error:"), attrs, NSString(h.directory), &failure) {
			return errors.New("cannot protect the download journal")
		}
	}
	registerDownloadDelegate()
	h.delegate = objc.ID(downloadDelegateClass).Send(sel("new"))
	downloadDelegates[h.delegate] = h
	var configuration objc.ID
	if h.background {
		configuration = objc.ID(Class("NSURLSessionConfiguration")).Send(sel("backgroundSessionConfigurationWithIdentifier:"), NSString(h.identifier))
		configuration.Send(sel("setSessionSendsLaunchEvents:"), true)
		configuration.Send(sel("setDiscretionary:"), false)
	} else {
		configuration = objc.ID(Class("NSURLSessionConfiguration")).Send(sel("ephemeralSessionConfiguration"))
	}
	configuration.Send(sel("setURLCache:"), objc.ID(0))
	queue := objc.ID(Class("NSOperationQueue")).Send(selMainQueue)
	h.session = objc.ID(Class("NSURLSession")).Send(sel("sessionWithConfiguration:delegate:delegateQueue:"), configuration, h.delegate, queue).Send(sel("retain"))
	if h.session == 0 {
		return errors.New("cannot create the download session")
	}
	snapshot := objcblock.New(func(_ objc.Block, tasks objc.ID) {
		tasks.Send(sel("retain"))
		OnMain(func() {
			defer tasks.Send(sel("release"))
			ForEach(tasks, func(native objc.ID) {
				task := h.task(native)
				if task == nil || downloadTerminal(task.State) || task.State == "paused" {
					native.Send(sel("cancel"))
					return
				}
				if task.State == "publishing" {
					return
				}
				h.retainTask(task, native)
				task.State = "running"
				task.Loaded = objc.Send[int64](native, sel("countOfBytesReceived"))
				task.Total = objc.Send[int64](native, sel("countOfBytesExpectedToReceive"))
			})
			for _, task := range h.tasks {
				if task.State == "publishing" {
					h.publish(task)
				} else if (task.State == "queued" || task.State == "running") && task.native == 0 {
					h.update(task, "interrupted", "the transfer was absent from the restored URL session")
				}
			}
			h.ready = true
			waiting := h.waiting
			h.waiting = nil
			for _, command := range waiting {
				h.command(command)
			}
		})
	})
	h.session.Send(sel("getAllTasksWithCompletionHandler:"), snapshot)
	snapshot.Release()
	return nil
}

func registerDownloadDelegate() {
	if downloadDelegateClass != 0 {
		return
	}
	downloadDelegateClass = RegisterClass("NsgoBackgroundDownloadDelegate", Class("NSObject"), []*objc.Protocol{objc.GetProtocol("NSURLSessionDownloadDelegate")}, []objc.MethodDef{
		{Cmd: sel("URLSession:downloadTask:didWriteData:totalBytesWritten:totalBytesExpectedToWrite:"), Fn: func(self objc.ID, _ objc.SEL, _, native objc.ID, _, loaded, total int64) {
			if h := downloadDelegates[self]; h != nil {
				if task := h.task(native); task != nil && !downloadTerminal(task.State) && task.State != "publishing" {
					h.retainTask(task, native)
					task.Loaded, task.Total = loaded, total
					if time.Since(task.lastReport) >= 250*time.Millisecond || total >= 0 && loaded >= total {
						h.report(task)
					}
				}
			}
		}},
		{Cmd: sel("URLSession:downloadTask:didFinishDownloadingToURL:"), Fn: func(self objc.ID, _ objc.SEL, _, native, location objc.ID) {
			if h := downloadDelegates[self]; h != nil {
				h.downloaded(native, location)
			}
		}},
		{Cmd: sel("URLSession:task:didCompleteWithError:"), Fn: func(self objc.ID, _ objc.SEL, _, native, failure objc.ID) {
			if h := downloadDelegates[self]; h != nil {
				if task := h.task(native); task != nil {
					if task.stopping == "paused" {
						return
					}
					if task.stopping == "canceled" {
						h.releaseTask(task)
						h.update(task, "canceled", "")
					} else if failure != 0 && !downloadTerminal(task.State) && task.State != "publishing" {
						h.releaseTask(task)
						// NSError descriptions may include the URL's credentials.
						h.update(task, "failed", fmt.Sprintf("URL session failed (%d)", objc.Send[int](failure, sel("code"))))
					} else {
						h.releaseTask(task)
					}
				}
			}
		}},
		{Cmd: sel("URLSession:task:didReceiveChallenge:completionHandler:"), Fn: downloadChallenge},
		{Cmd: sel("URLSessionDidFinishEventsForBackgroundURLSession:"), Fn: func(self objc.ID, _ objc.SEL, _ objc.ID) {
			if h := downloadDelegates[self]; h != nil {
				h.finishedEvents = true
				h.finishEvents()
			}
		}},
	})
}

func (h *downloadHost) command(command driver.DownloadCommand) {
	if !h.ready {
		h.waiting = append(h.waiting, command)
		return
	}
	if command.Op == "list" {
		for _, task := range h.tasks {
			h.report(task)
		}
		return
	}
	if !downloadID(command.ID) {
		return
	}
	task := h.tasks[command.ID]
	if command.Op == "start" {
		if task != nil {
			h.report(task)
			return
		}
		task = &downloadTask{DownloadProgress: driver.DownloadProgress{ID: command.ID, Path: command.Path, State: "queued", Total: -1}, Request: command, Home: h.home}
		h.tasks[command.ID] = task
		if err := h.save(task); err != nil {
			task.State, task.Error = "failed", "cannot save the download request"
			h.report(task)
			return
		}
		h.start(task)
		return
	}
	if task == nil {
		return
	}
	switch command.Op {
	case "pause":
		if task.native == 0 || task.stopping != "" || downloadTerminal(task.State) || task.State == "publishing" {
			return
		}
		task.stopping = "paused"
		paused := objcblock.New(func(_ objc.Block, data objc.ID) {
			resume := bytes.Clone(Bytes(data))
			OnMain(func() {
				if task.stopping != "paused" {
					return
				}
				h.releaseTask(task)
				task.ResumeData = resume
				task.stopping = ""
				h.update(task, "paused", "")
			})
		})
		task.native.Send(sel("cancelByProducingResumeData:"), paused)
		paused.Release()
	case "resume":
		if task.State == "paused" && task.stopping == "" {
			h.start(task)
		}
	case "cancel":
		if downloadTerminal(task.State) || task.State == "publishing" {
			return
		}
		task.stopping = "canceled"
		if task.native != 0 {
			task.native.Send(sel("cancel"))
		} else {
			h.update(task, "canceled", "")
		}
	case "forget":
		if downloadTerminal(task.State) {
			if os.Remove(h.record(task.ID)) == nil {
				task.native.Send(sel("cancel"))
				h.releaseTask(task)
				cleanDownloadStage(task)
				delete(h.tasks, task.ID)
			}
		}
	}
}

func (h *downloadHost) start(task *downloadTask) {
	if err := downloadURL(task.Request); err != nil {
		h.update(task, "failed", err.Error())
		return
	}
	var native objc.ID
	if len(task.ResumeData) > 0 {
		native = h.session.Send(sel("downloadTaskWithResumeData:"), NSData(task.ResumeData))
	} else {
		location := objc.ID(Class("NSURL")).Send(sel("URLWithString:"), NSString(task.Request.URL))
		request := objc.ID(Class("NSMutableURLRequest")).Send(sel("requestWithURL:"), location)
		for name, value := range task.Request.Headers {
			request.Send(sel("setValue:forHTTPHeaderField:"), NSString(value), NSString(name))
		}
		native = h.session.Send(sel("downloadTaskWithRequest:"), request)
	}
	if native == 0 {
		h.update(task, "failed", "cannot create the URL session task")
		return
	}
	native.Send(sel("setTaskDescription:"), NSString(task.ID))
	h.retainTask(task, native)
	task.ResumeData = nil
	task.State, task.Error = "running", ""
	if err := h.save(task); err != nil {
		native.Send(sel("cancel"))
		h.releaseTask(task)
		task.State, task.Error = "failed", "cannot save the URL session task"
		h.report(task)
		return
	}
	h.report(task)
	native.Send(sel("resume"))
}

func (h *downloadHost) task(native objc.ID) *downloadTask {
	task := h.tasks[GoString(native.Send(sel("taskDescription")))]
	if task == nil || task.native != 0 && task.nativeID != objc.Send[uint](native, sel("taskIdentifier")) {
		return nil
	}
	if task.native == 0 && h.ready && (task.State == "paused" || downloadTerminal(task.State)) {
		return nil
	}
	return task
}

func (h *downloadHost) retainTask(task *downloadTask, native objc.ID) {
	if task.native == 0 {
		task.native = native.Send(sel("retain"))
		task.nativeID = objc.Send[uint](native, sel("taskIdentifier"))
	}
}

func (h *downloadHost) releaseTask(task *downloadTask) {
	task.native.Send(sel("release"))
	task.native, task.nativeID = 0, 0
}

func (h *downloadHost) downloaded(native, location objc.ID) {
	task := h.task(native)
	if task == nil || downloadTerminal(task.State) || task.State == "publishing" {
		return
	}
	if task.stopping != "" {
		return
	}
	response := native.Send(sel("response"))
	status := objc.Send[int](response, sel("statusCode"))
	if status < 200 || status >= 300 {
		h.update(task, "failed", fmt.Sprintf("the server returned HTTP %d", status))
		return
	}
	source := GoString(location.Send(sel("path")))
	holding, err := os.MkdirTemp(filepath.Dir(source), ".nsgo-download-"+task.ID+"-")
	if err == nil {
		task.Staged = filepath.Join(holding, "payload")
		err = os.Rename(source, task.Staged)
	}
	if err != nil {
		h.update(task, "failed", "cannot retain the downloaded file")
		return
	}
	if info, err := os.Stat(task.Staged); err == nil {
		task.Loaded, task.Total = info.Size(), info.Size()
	}
	task.State = "publishing"
	if err := h.save(task); err != nil {
		h.update(task, "failed", "cannot save the downloaded file's state")
		return
	}
	h.publish(task)
}

func (h *downloadHost) publish(task *downloadTask) {
	if task.publishing {
		return
	}
	task.publishing = true
	source, destination, id := task.Staged, task.Path, task.ID
	h.publishing++
	go func() {
		err := publishDownload(source, destination, id)
		OnMain(func() {
			h.publishing--
			task.publishing = false
			if err != nil {
				h.update(task, "failed", "cannot publish the downloaded file")
			} else {
				h.update(task, "completed", "")
			}
			h.finishEvents()
		})
	}()
}

// Keep the staged original until the terminal journal is durable. Repeating
// this publication after a crash replaces the destination with the same file.
func publishDownload(source, destination, id string) error {
	partial := filepath.Join(filepath.Dir(destination), ".nsgo-download-"+id+".part")
	if err := os.Remove(partial); err != nil && !os.IsNotExist(err) {
		return err
	}
	defer os.Remove(partial)
	if err := os.Link(source, partial); err != nil {
		input, err := os.Open(source)
		if err != nil {
			return err
		}
		defer input.Close()
		output, err := os.OpenFile(partial, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(output, input)
		syncErr := output.Sync()
		closeErr := output.Close()
		if err = errors.Join(copyErr, syncErr, closeErr); err != nil {
			return err
		}
	}
	return os.Rename(partial, destination)
}

func (h *downloadHost) update(task *downloadTask, state, message string) {
	task.State, task.Error = state, message
	if downloadTerminal(state) {
		task.ResumeData = nil
		task.Request.URL = ""
		task.Request.Headers = nil
		task.Request.CertificatePEM = ""
	}
	if err := h.save(task); err != nil {
		task.State, task.Error = "failed", "cannot save the download result"
		h.report(task)
		return
	}
	if downloadTerminal(state) {
		cleanDownloadStage(task)
	}
	h.report(task)
}

// Only the temporary holding directory created by downloaded is removed.
func cleanDownloadStage(task *downloadTask) {
	parent := filepath.Dir(task.Staged)
	if filepath.Base(task.Staged) != "payload" || !strings.HasPrefix(filepath.Base(parent), ".nsgo-download-"+task.ID+"-") {
		return
	}
	os.Remove(task.Staged)
	os.Remove(parent)
	task.Staged = ""
}

func (h *downloadHost) report(task *downloadTask) {
	task.lastReport = time.Now()
	progress := task.DownloadProgress
	if progress.State == "publishing" {
		progress.State = "running"
	}
	driver.ReportDownload(progress)
}

func downloadTerminal(state string) bool {
	switch state {
	case "completed", "failed", "canceled", "interrupted":
		return true
	}
	return false
}

func downloadID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, c := range id {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9') {
			return false
		}
	}
	return true
}

func (h *downloadHost) record(id string) string { return filepath.Join(h.directory, id+".json") }

func (h *downloadHost) save(task *downloadTask) error {
	data, err := json.Marshal(task)
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(h.directory, ".record-")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.Write(data)
	syncErr := file.Sync()
	closeErr := file.Close()
	if err = errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return os.Rename(file.Name(), h.record(task.ID))
}

func (h *downloadHost) restore() error {
	entries, err := os.ReadDir(h.directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".record-") {
			os.Remove(filepath.Join(h.directory, entry.Name()))
			continue
		}
		id, ok := strings.CutSuffix(entry.Name(), ".json")
		if !ok || !downloadID(id) {
			continue
		}
		data, err := os.ReadFile(h.record(id))
		if err != nil {
			return err
		}
		var task downloadTask
		if json.Unmarshal(data, &task) != nil || task.ID != id || !filepath.IsAbs(task.Path) {
			continue
		}
		previousHome := task.Home
		if previousHome == "" {
			previousHome = downloadContainerHome(task.Path)
		}
		if previousHome != "" && h.home != "" {
			task.Path = relocateDownloadPath(task.Path, previousHome, h.home)
			task.Request.Path = relocateDownloadPath(task.Request.Path, previousHome, h.home)
			task.Staged = relocateDownloadPath(task.Staged, previousHome, h.home)
			task.Home = h.home
		}
		if downloadTerminal(task.State) {
			cleanDownloadStage(&task)
		}
		h.tasks[id] = &task
	}
	return nil
}

// iOS changes an app's absolute container path during some installations.
// Older journals can identify their former container from a destination.
func downloadContainerHome(path string) string {
	prefix, rest, ok := strings.Cut(path, "/Containers/Data/Application/")
	if !ok {
		return ""
	}
	identifier, _, _ := strings.Cut(rest, "/")
	if len(identifier) != 36 || strings.Count(identifier, "-") != 4 {
		return ""
	}
	return prefix + "/Containers/Data/Application/" + identifier
}

func relocateDownloadPath(path, previousHome, currentHome string) string {
	if path == "" {
		return ""
	}
	normalize := func(path string) string {
		if strings.HasPrefix(path, "/private/var/") {
			path = strings.TrimPrefix(path, "/private")
		}
		return filepath.Clean(path)
	}
	relative, err := filepath.Rel(normalize(previousHome), normalize(path))
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.Join(currentHome, relative)
}

func (h *downloadHost) finishEvents() {
	if !h.finishedEvents || h.publishing != 0 || h.completion == 0 {
		return
	}
	completion := h.completion
	h.completion = 0
	h.finishedEvents = false
	CallBlock(completion)
	completion.Send(sel("release"))
}

type downloadSecurityAPI struct {
	certificate, data uintptr
	createCertificate func(uintptr, objc.ID) uintptr
	createPolicy      func(bool, objc.ID) uintptr
	createTrust       func(uintptr, uintptr, *uintptr) int32
	setAnchors        func(uintptr, objc.ID) int32
	setAnchorsOnly    func(uintptr, bool) int32
	setNetwork        func(uintptr, bool) int32
	setDate           func(uintptr, objc.ID) int32
	evaluate          func(uintptr, uintptr) bool
}

var downloadSecurity = sync.OnceValue(func() downloadSecurityAPI {
	lib := load("/System/Library/Frameworks/Security.framework/Security")
	api := downloadSecurityAPI{certificate: symbol(lib, "SecTrustGetCertificateAtIndex"), data: symbol(lib, "SecCertificateCopyData")}
	purego.RegisterLibFunc(&api.createCertificate, lib, "SecCertificateCreateWithData")
	purego.RegisterLibFunc(&api.createPolicy, lib, "SecPolicyCreateSSL")
	purego.RegisterLibFunc(&api.createTrust, lib, "SecTrustCreateWithCertificates")
	purego.RegisterLibFunc(&api.setAnchors, lib, "SecTrustSetAnchorCertificates")
	purego.RegisterLibFunc(&api.setAnchorsOnly, lib, "SecTrustSetAnchorCertificatesOnly")
	purego.RegisterLibFunc(&api.setNetwork, lib, "SecTrustSetNetworkFetchAllowed")
	purego.RegisterLibFunc(&api.setDate, lib, "SecTrustSetVerifyDate")
	purego.RegisterLibFunc(&api.evaluate, lib, "SecTrustEvaluateWithError")
	return api
})

func downloadChallenge(self objc.ID, _ objc.SEL, _, native, challenge, completion objc.ID) {
	h := downloadDelegates[self]
	var task *downloadTask
	if h != nil {
		task = h.task(native)
	}
	const useCredential, defaultHandling, cancel = 0, 1, 2
	if task == nil || task.Request.CertificatePEM == "" {
		CallBlock(completion, defaultHandling, 0)
		return
	}
	protection := challenge.Send(sel("protectionSpace"))
	if GoString(protection.Send(sel("authenticationMethod"))) != "NSURLAuthenticationMethodServerTrust" {
		CallBlock(completion, defaultHandling, 0)
		return
	}
	trust := protection.Send(sel("serverTrust"))
	api := downloadSecurity()
	certificate, _, _ := purego.SyscallN(api.certificate, uintptr(trust), 0)
	if certificate == 0 {
		CallBlock(completion, cancel, 0)
		return
	}
	data, _, _ := purego.SyscallN(api.data, certificate)
	if data == 0 {
		CallBlock(completion, cancel, 0)
		return
	}
	defer Release(data)
	if !downloadCertificateMatches(task.Request.CertificatePEM, Bytes(objc.ID(data)), GoString(protection.Send(sel("host"))), time.Now()) {
		CallBlock(completion, cancel, 0)
		return
	}
	credential := objc.ID(Class("NSURLCredential")).Send(sel("credentialForTrust:"), trust)
	CallBlock(completion, useCredential, uintptr(credential))
}

func downloadCertificateMatches(pin string, der []byte, host string, now time.Time) bool {
	block, _ := pem.Decode([]byte(pin))
	if block == nil || block.Type != "CERTIFICATE" || !bytes.Equal(block.Bytes, der) {
		return false
	}
	api := downloadSecurity()
	certificate := api.createCertificate(0, NSData(der))
	if certificate == 0 {
		return false
	}
	defer Release(certificate)
	policy := api.createPolicy(true, NSString(host))
	if policy == 0 {
		return false
	}
	defer Release(policy)
	var trust uintptr
	if api.createTrust(certificate, policy, &trust) != 0 || trust == 0 {
		return false
	}
	defer Release(trust)
	anchors := objc.ID(Class("NSArray")).Send(sel("arrayWithObject:"), objc.ID(certificate))
	date := objc.ID(Class("NSDate")).Send(sel("dateWithTimeIntervalSince1970:"), float64(now.Unix())+float64(now.Nanosecond())/1e9)
	// The exact leaf is the sole anchor. Keep the SSL hostname and date
	// policies while avoiding intermediate downloads and Go's TLS stack.
	return api.setAnchors(trust, anchors) == 0 && api.setAnchorsOnly(trust, true) == 0 &&
		api.setNetwork(trust, false) == 0 && api.setDate(trust, date) == 0 && api.evaluate(trust, 0)
}

// A malformed pinned URL is refused before any native network operation.
func downloadURL(command driver.DownloadCommand) error {
	location, err := url.Parse(command.URL)
	if err != nil || location.Hostname() == "" || location.User != nil || location.Scheme != "http" && location.Scheme != "https" {
		return errors.New("invalid download URL")
	}
	if command.CertificatePEM != "" {
		if location.Scheme != "https" {
			return errors.New("a pinned download requires HTTPS")
		}
		block, _ := pem.Decode([]byte(command.CertificatePEM))
		if block == nil || !downloadCertificateMatches(command.CertificatePEM, block.Bytes, location.Hostname(), time.Now()) {
			return errors.New("the pinned certificate is invalid for the download host")
		}
	}
	return nil
}
