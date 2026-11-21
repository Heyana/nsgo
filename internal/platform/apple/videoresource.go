//go:build darwin

package apple

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/ebitengine/purego/objc"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// A videoResource supplies the byte ranges AVFoundation needs for a remote
// file with app-supplied headers. The loader and requests belong to the
// main queue. Each HTTP transfer runs off that queue.
type videoResource struct {
	source   *url.URL
	headers  http.Header
	client   http.Client
	delegate objc.ID
	loader   objc.ID
	requests map[objc.ID]context.CancelFunc
}

var (
	videoResourceClass objc.Class
	videoResources     = map[objc.ID]*videoResource{}
)

func newVideoResource(source *url.URL, fields *driver.VideoHeaders) *videoResource {
	r := &videoResource{
		source: source, headers: make(http.Header), requests: make(map[objc.ID]context.CancelFunc),
	}
	for name, value := range fields.All {
		r.headers.Set(name, value)
	}
	r.client.CheckRedirect = r.redirect
	return r
}

func (r *videoResource) redirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return errors.New("too many media redirects")
	}
	sameOrigin := sameVideoOrigin(r.source, req.URL)
	if !sameOrigin {
		// The URL used to generate Referer may contain an access token.
		req.Header.Del("Referer")
	}
	for name, values := range r.headers {
		if name == "Range" || name == "Accept-Encoding" {
			continue
		}
		if sameOrigin {
			// net/http generates its own Referer for redirects.
			req.Header[name] = values
		} else {
			req.Header.Del(name)
		}
	}
	return nil
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

// response opens the requested range as an HTTP stream. A 200 response is
// also usable from offset zero, with seeking left to AVFoundation.
func (r *videoResource) response(ctx context.Context, span videoResourceRange) (*http.Response, int64, error) {
	if span.offset < 0 || span.length < 0 || !span.toEnd && span.length > math.MaxInt64-span.offset {
		return nil, 0, errors.New("invalid media byte range")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.source.String(), nil)
	if err != nil {
		return nil, 0, errors.New("invalid media URL")
	}
	req.Header = r.headers.Clone()
	req.Header.Set("Accept-Encoding", "identity")
	header := fmt.Sprintf("bytes=%d-", span.offset)
	if !span.toEnd && span.length > 0 {
		header += fmt.Sprint(span.offset + span.length - 1)
	}
	req.Header.Set("Range", header)
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, 0, errors.New("cannot request media bytes")
	}
	var length int64
	switch resp.StatusCode {
	case http.StatusPartialContent:
		var first, last int64
		n, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &first, &last, &length)
		if err == nil && n == 3 && first == span.offset && last >= first && last < length {
			return resp, length, nil
		}
	case http.StatusOK:
		if span.offset == 0 && resp.ContentLength >= 0 {
			return resp, resp.ContentLength, nil
		}
	}
	resp.Body.Close()
	return nil, 0, fmt.Errorf("cannot read media byte range from HTTP %d", resp.StatusCode)
}

func (r *videoResource) load(ctx context.Context, span videoResourceRange, info, data objc.ID) error {
	resp, length, err := r.response(ctx, span)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if info != 0 {
		contentType, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
		if !onVideoResourceMain(ctx, func() {
			if kind := mediaResourceType(contentType, path.Ext(r.source.Path)); kind != 0 {
				info.Send(sel("setContentType:"), kind)
			}
			info.Send(sel("setContentLength:"), length)
			info.Send(sel("setByteRangeAccessSupported:"), resp.StatusCode == http.StatusPartialContent)
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
		n, err := io.ReadFull(resp.Body, buf[:min(int64(len(buf)), remaining)])
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
