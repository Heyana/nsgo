//go:build darwin

package apple

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/url"
	"path"
	"strings"
	"sync"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A videoResource supplies the byte ranges AVFoundation needs for a remote file with app-supplied headers. The loader and requests belong to the main queue. Each transfer runs in the resource's URLSession, the system's HTTP client, so no app links Go's (TestMinimalAppLinks).
type videoResource struct {
	source   *url.URL
	headers  [][2]string
	session  objc.ID
	delegate objc.ID
	loader   objc.ID
	requests map[objc.ID]context.CancelFunc
}

var (
	videoResourceClass objc.Class
	videoResources     = map[objc.ID]*videoResource{}
)

// videoAssetWithHeaders opens one media source and returns its resource
// loader, which the player keeps until the source closes.
func videoAssetWithHeaders(source string, headers *driver.VideoHeaders) (objc.ID, *videoResource) {
	if remote := videoResourceURL(source); remote != nil && headers.Len() > 0 {
		resource := newVideoResource(remote, headers)
		return resource.asset(), resource
	}
	return videoAsset(source), nil
}

func newVideoResource(source *url.URL, fields *driver.VideoHeaders) *videoResource {
	r := &videoResource{source: source, requests: make(map[objc.ID]context.CancelFunc)}
	for name, value := range fields.All {
		// The transfer sets its own byte range and encoding.
		if !strings.EqualFold(name, "Range") && !strings.EqualFold(name, "Accept-Encoding") {
			r.headers = append(r.headers, [2]string{name, value})
		}
	}
	withPool(func() {
		config := objc.ID(Class("NSURLSessionConfiguration")).Send(sel("ephemeralSessionConfiguration"))
		config.Send(sel("setURLCache:"), objc.ID(0))
		config.Send(sel("setRequestCachePolicy:"), uint(1)) // NSURLRequestReloadIgnoringLocalCacheData
		// Only the app's headers carry cookies.
		config.Send(sel("setHTTPCookieStorage:"), objc.ID(0))
		config.Send(sel("setHTTPShouldSetCookies:"), false)
		// A nil queue is a serial one the session makes.
		r.session = objc.ID(Class("NSURLSession")).Send(sel("sessionWithConfiguration:delegate:delegateQueue:"),
			config, videoTransferDelegate(), objc.ID(0)).Send(sel("retain"))
	})
	return r
}

// redirect prepares a redirected request: the app's headers go only to the source's origin, and a Referer, which may carry an access token, nowhere else.
func (t *videoTransfer) redirect(proposed objc.ID) objc.ID {
	t.redirects++
	if t.redirects > 10 {
		return 0 // The redirect response arrives as the response, and fails.
	}
	req := proposed.Send(sel("mutableCopy")).Send(sel("autorelease"))
	target, err := url.Parse(GoString(req.Send(sel("URL")).Send(sel("absoluteString"))))
	sameOrigin := err == nil && sameVideoOrigin(t.resource.source, target)
	if !sameOrigin {
		req.Send(sel("setValue:forHTTPHeaderField:"), objc.ID(0), NSString("Referer"))
	}
	for _, h := range t.resource.headers {
		if sameOrigin {
			req.Send(sel("setValue:forHTTPHeaderField:"), NSString(h[1]), NSString(h[0]))
		} else {
			req.Send(sel("setValue:forHTTPHeaderField:"), objc.ID(0), NSString(h[0]))
		}
	}
	req.Send(sel("setValue:forHTTPHeaderField:"), NSString("identity"), NSString("Accept-Encoding"))
	req.Send(sel("setValue:forHTTPHeaderField:"), NSString(t.byteRange), NSString("Range"))
	return req
}

func sameVideoOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if port := u.Port(); port != "" {
			return port
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return a.Scheme == b.Scheme && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

// videoResourceURL identifies HTTP file URLs the delegate can serve. Known
// playlist URLs keep the native path for loads of their media segments.
func videoResourceURL(source string) *url.URL {
	u, err := url.Parse(source)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil
	}
	switch strings.ToLower(path.Ext(u.Path)) {
	case ".m3u", ".m3u8":
		return nil
	default:
		return u
	}
}

func (r *videoResource) asset() objc.ID {
	// Only the delegate sees the original URL, including any query token.
	location := videoURL("nsgo-video://media"+r.source.EscapedPath(), "")
	if location == 0 {
		return 0
	}
	asset := objc.ID(Class("AVURLAsset")).Send(sel("URLAssetWithURL:options:"), location, objc.ID(0))
	if asset == 0 {
		return 0
	}
	if videoResourceClass == 0 {
		videoResourceClass = RegisterClass("NsgoVideoResourceDelegate", Class("NSObject"),
			[]*objc.Protocol{objc.GetProtocol("AVAssetResourceLoaderDelegate")}, []objc.MethodDef{
				{Cmd: sel("resourceLoader:shouldWaitForLoadingOfRequestedResource:"), Fn: startVideoResourceRequest},
				{Cmd: sel("resourceLoader:didCancelLoadingRequest:"), Fn: cancelVideoResourceRequest},
			})
	}
	r.delegate = objc.ID(videoResourceClass).Send(sel("new"))
	r.loader = asset.Send(sel("resourceLoader")).Send(sel("retain"))
	videoResources[r.delegate] = r
	r.loader.Send(sel("setDelegate:queue:"), r.delegate, MainQueue())
	return asset
}

func (r *videoResource) close() {
	for _, cancel := range r.requests {
		cancel()
	}
	clear(r.requests)
	delete(videoResources, r.delegate)
	r.loader.Send(sel("setDelegate:queue:"), objc.ID(0), objc.ID(0))
	r.loader.Send(sel("release"))
	r.delegate.Send(sel("release"))
	r.session.Send(sel("invalidateAndCancel"))
	r.session.Send(sel("release"))
}

func cancelVideoResourceRequest(self objc.ID, _ objc.SEL, _ objc.ID, request objc.ID) {
	if r := videoResources[self]; r != nil {
		if cancel := r.requests[request]; cancel != nil {
			delete(r.requests, request)
			cancel()
		}
	}
}

type videoResourceRange struct {
	offset, length int64
	toEnd          bool
}

func startVideoResourceRequest(self objc.ID, _ objc.SEL, _ objc.ID, request objc.ID) bool {
	r := videoResources[self]
	if r == nil {
		return false
	}
	info := request.Send(sel("contentInformationRequest"))
	data := request.Send(sel("dataRequest"))
	if info == 0 && data == 0 {
		return false
	}
	span := videoResourceRange{length: 2} // Metadata probing requires only the response headers.
	if data != 0 {
		requested := objc.Send[int64](data, sel("requestedOffset"))
		span.offset = max(requested, objc.Send[int64](data, sel("currentOffset")))
		span.length = max(int64(objc.Send[int](data, sel("requestedLength")))-(span.offset-requested), 0)
		span.toEnd = objc.Send[bool](data, sel("requestsAllDataToEndOfResource"))
	}
	ctx, cancel := context.WithCancel(context.Background())
	r.requests[request] = cancel
	request.Send(sel("retain"))
	go func() {
		err := r.load(ctx, span, info, data)
		OnMain(func() {
			defer request.Send(sel("release"))
			if r.requests[request] == nil {
				return
			}
			delete(r.requests, request)
			cancel()
			if err == nil {
				request.Send(sel("finishLoading"))
				return
			}
			details := objc.ID(Class("NSDictionary")).Send(sel("dictionaryWithObject:forKey:"),
				NSString(err.Error()), NSString("NSLocalizedDescription"))
			failure := objc.ID(Class("NSError")).Send(sel("errorWithDomain:code:userInfo:"), NSString("NSGoVideoHTTP"), 1, details)
			request.Send(sel("finishLoadingWithError:"), failure)
		})
	}()
	return true
}

// A videoTransfer is one byte range's data task. Its delegate's callbacks arrive in order on the session's queue, and each chunk waits there until the reader takes it, so a slow player holds back the download.
type videoTransfer struct {
	resource  *videoResource
	ctx       context.Context
	task      objc.ID
	byteRange string
	redirects int // on the session's queue

	head    chan videoResponseHead
	data    chan []byte
	done    chan error
	stop    chan struct{}
	stopped sync.Once

	pending []byte
	err     error
}

type videoResponseHead struct {
	status        int
	contentRange  string
	contentType   string
	contentLength int64
}

var videoTransfers struct {
	sync.Mutex
	byTask   map[objc.ID]*videoTransfer
	delegate objc.ID
}

func videoTransferOf(task objc.ID) *videoTransfer {
	videoTransfers.Lock()
	defer videoTransfers.Unlock()
	return videoTransfers.byTask[task]
}

// videoTransferDelegate is the sessions' delegate, which finds each task's transfer.
func videoTransferDelegate() objc.ID {
	videoTransfers.Lock()
	defer videoTransfers.Unlock()
	if videoTransfers.delegate != 0 {
		return videoTransfers.delegate
	}
	videoTransfers.byTask = map[objc.ID]*videoTransfer{}
	class := RegisterClass("NsgoVideoTransferDelegate", Class("NSObject"),
		[]*objc.Protocol{objc.GetProtocol("NSURLSessionDataDelegate")}, []objc.MethodDef{
			{Cmd: sel("URLSession:task:willPerformHTTPRedirection:newRequest:completionHandler:"),
				Fn: func(_ objc.ID, _ objc.SEL, _, task, _, proposed, handler objc.ID) {
					var next objc.ID
					if t := videoTransferOf(task); t != nil {
						next = t.redirect(proposed)
					}
					CallBlock(handler, uintptr(next))
				}},
			{Cmd: sel("URLSession:dataTask:didReceiveResponse:completionHandler:"),
				Fn: func(_ objc.ID, _ objc.SEL, _, task, response, handler objc.ID) {
					const cancel, allow = 0, 1 // NSURLSessionResponseDisposition
					t := videoTransferOf(task)
					if t == nil {
						CallBlock(handler, cancel)
						return
					}
					field := func(name string) string {
						return GoString(response.Send(sel("valueForHTTPHeaderField:"), NSString(name)))
					}
					t.head <- videoResponseHead{
						status: objc.Send[int](response, sel("statusCode")), contentRange: field("Content-Range"),
						contentType: field("Content-Type"), contentLength: objc.Send[int64](response, sel("expectedContentLength")),
					}
					CallBlock(handler, allow)
				}},
			{Cmd: sel("URLSession:dataTask:didReceiveData:"), Fn: func(_ objc.ID, _ objc.SEL, _, task, data objc.ID) {
				if t := videoTransferOf(task); t != nil {
					select {
					case t.data <- Bytes(data):
					case <-t.stop:
					}
				}
			}},
			{Cmd: sel("URLSession:task:didCompleteWithError:"), Fn: func(_ objc.ID, _ objc.SEL, _, task, failure objc.ID) {
				videoTransfers.Lock()
				t := videoTransfers.byTask[task]
				delete(videoTransfers.byTask, task)
				videoTransfers.Unlock()
				if t == nil {
					return
				}
				var err error
				if failure != 0 {
					err = errors.New(GoString(failure.Send(sel("localizedDescription"))))
				}
				t.done <- err
			}},
		})
	videoTransfers.delegate = objc.ID(class).Send(sel("new"))
	return videoTransfers.delegate
}

// response opens the requested range as an HTTP stream. A 200 response is also usable from offset zero, with seeking left to AVFoundation.
func (r *videoResource) response(ctx context.Context, span videoResourceRange) (*videoTransfer, videoResponseHead, int64, error) {
	if span.offset < 0 || span.length < 0 || !span.toEnd && span.length > math.MaxInt64-span.offset {
		return nil, videoResponseHead{}, 0, errors.New("invalid media byte range")
	}
	t := &videoTransfer{
		resource: r, ctx: ctx, byteRange: fmt.Sprintf("bytes=%d-", span.offset),
		head: make(chan videoResponseHead, 1), data: make(chan []byte), done: make(chan error, 1), stop: make(chan struct{}),
	}
	if !span.toEnd && span.length > 0 {
		t.byteRange += fmt.Sprint(span.offset + span.length - 1)
	}
	withPool(func() {
		u := objc.ID(Class("NSURL")).Send(sel("URLWithString:"), NSString(r.source.String()))
		if u == 0 {
			return
		}
		req := objc.ID(Class("NSMutableURLRequest")).Send(sel("requestWithURL:"), u)
		for _, h := range r.headers {
			req.Send(sel("setValue:forHTTPHeaderField:"), NSString(h[1]), NSString(h[0]))
		}
		req.Send(sel("setValue:forHTTPHeaderField:"), NSString("identity"), NSString("Accept-Encoding"))
		req.Send(sel("setValue:forHTTPHeaderField:"), NSString(t.byteRange), NSString("Range"))
		t.task = r.session.Send(sel("dataTaskWithRequest:"), req).Send(sel("retain"))
		if t.task == 0 {
			return
		}
		videoTransfers.Lock()
		videoTransfers.byTask[t.task] = t
		videoTransfers.Unlock()
		t.task.Send(sel("resume"))
	})
	if t.task == 0 {
		return nil, videoResponseHead{}, 0, errors.New("invalid media URL")
	}
	var head videoResponseHead
	select {
	case head = <-t.head:
	case <-t.done:
		t.Close()
		return nil, head, 0, errors.New("cannot request media bytes")
	case <-ctx.Done():
		t.Close()
		return nil, head, 0, ctx.Err()
	}
	var length int64
	switch head.status {
	case 206:
		var first, last int64
		n, err := fmt.Sscanf(head.contentRange, "bytes %d-%d/%d", &first, &last, &length)
		if err == nil && n == 3 && first == span.offset && last >= first && last < length {
			return t, head, length, nil
		}
	case 200:
		if span.offset == 0 && head.contentLength >= 0 {
			return t, head, head.contentLength, nil
		}
	}
	t.Close()
	return nil, head, 0, fmt.Errorf("cannot read media byte range from HTTP %d", head.status)
}

// Read reads the response's body.
func (t *videoTransfer) Read(p []byte) (int, error) {
	for len(t.pending) == 0 {
		if t.err != nil {
			return 0, t.err
		}
		select {
		case t.pending = <-t.data:
		case err := <-t.done:
			t.err = err
			if err == nil {
				t.err = io.EOF
			}
		case <-t.ctx.Done():
			t.err = t.ctx.Err()
		}
	}
	n := copy(p, t.pending)
	t.pending = t.pending[n:]
	return n, nil
}

// Close cancels the task if it is still running.
func (t *videoTransfer) Close() error {
	t.stopped.Do(func() {
		close(t.stop)
		t.task.Send(sel("cancel"))
		t.task.Send(sel("release"))
	})
	return nil
}

func (r *videoResource) load(ctx context.Context, span videoResourceRange, info, data objc.ID) error {
	body, head, length, err := r.response(ctx, span)
	if err != nil {
		return err
	}
	defer body.Close()
	if info != 0 {
		contentType, _, _ := mime.ParseMediaType(head.contentType)
		if !onVideoResourceMain(ctx, func() {
			if kind := mediaResourceType(contentType, path.Ext(r.source.Path)); kind != 0 {
				info.Send(sel("setContentType:"), kind)
			}
			info.Send(sel("setContentLength:"), length)
			info.Send(sel("setByteRangeAccessSupported:"), head.status == 206)
		}) {
			return ctx.Err()
		}
	}
	if data == 0 {
		return nil
	}

	remaining := length - span.offset
	if !span.toEnd {
		remaining = min(remaining, span.length)
	}
	buf := make([]byte, 64<<10)
	for remaining > 0 {
		n, err := io.ReadFull(body, buf[:min(int64(len(buf)), remaining)])
		if err != nil {
			return errors.New("media response ended before the requested byte range")
		}
		if !onVideoResourceMain(ctx, func() { data.Send(sel("respondWithData:"), NSData(buf[:n])) }) {
			return ctx.Err()
		}
		remaining -= int64(n)
	}
	return nil
}

func mediaResourceType(contentType, extension string) objc.ID {
	class := UTType()
	if contentType != "" && contentType != "application/octet-stream" {
		if typ := class.Send(sel("typeWithMIMEType:"), NSString(contentType)); typ != 0 {
			return typ.Send(sel("identifier"))
		}
	}
	return class.Send(sel("typeWithFilenameExtension:"), NSString(strings.TrimPrefix(extension, "."))).Send(sel("identifier"))
}

// Wait for AVFoundation to consume one chunk before reusing its buffer.
func onVideoResourceMain(ctx context.Context, fn func()) bool {
	done := make(chan struct{})
	OnMain(func() {
		defer close(done)
		if ctx.Err() == nil {
			fn()
		}
	})
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return ctx.Err() == nil
	}
}
