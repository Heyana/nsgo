package devtools

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"sync"
)

// Response is nsgo dev's server's answer to Fetch. Mini program dev builds answer through the platform's request API without linking net/http, so callers read Status, StatusCode and Body alone.
type Response struct {
	Status     string
	StatusCode int
	Body       io.ReadCloser
}

// miniProgramRequest is one HTTP request Fetch makes through a mini program's request API (nsgoMP.api.request). Only mini program dev builds call doMiniProgramRequest, so release builds link none of it; it is untagged so that nsgo test checks it without nsgodev or a browser.
type miniProgramRequest struct {
	Method        string
	URL           string
	Headers       map[string]string
	Body          string
	EnableChunked bool
	OnHeaders     func(statusCode int)
	OnChunk       func(data []byte)
	OnSuccess     func(statusCode int, data []byte)
	OnFail        func(err error)
}

type miniProgramRequester func(req miniProgramRequest) (abort func(), err error)

type streamBody struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	eof    bool
	closed bool
	err    error
	notify chan struct{}
	abort  func()
}

func newStreamBody() *streamBody {
	return &streamBody{notify: make(chan struct{}, 1)}
}

func (b *streamBody) signal() {
	select {
	case b.notify <- struct{}{}:
	default:
	}
}

func (b *streamBody) setAbort(abort func()) {
	b.mu.Lock()
	closed := b.closed
	if !closed {
		b.abort = abort
	}
	b.mu.Unlock()
	if closed && abort != nil {
		abort()
	}
}

func (b *streamBody) push(p []byte) {
	if len(p) == 0 {
		return
	}
	b.mu.Lock()
	if !b.closed && !b.eof {
		b.buf.Write(p)
		b.signal()
	}
	b.mu.Unlock()
}

func (b *streamBody) finish(err error) {
	b.mu.Lock()
	if !b.eof {
		b.eof = true
		if err != nil && b.err == nil {
			b.err = err
		}
		b.signal()
	}
	b.mu.Unlock()
}

func (b *streamBody) Read(p []byte) (int, error) {
	for {
		b.mu.Lock()
		if b.buf.Len() > 0 {
			n, _ := b.buf.Read(p)
			b.mu.Unlock()
			return n, nil
		}
		if b.closed {
			b.mu.Unlock()
			return 0, io.EOF
		}
		if b.eof {
			err := b.err
			b.mu.Unlock()
			if err != nil {
				return 0, err
			}
			return 0, io.EOF
		}
		b.mu.Unlock()
		<-b.notify
	}
}

func (b *streamBody) Close() error {
	b.mu.Lock()
	b.closed = true
	abort := b.abort
	b.abort = nil
	b.signal()
	b.mu.Unlock()
	if abort != nil {
		abort()
	}
	return nil
}

// doMiniProgramRequest runs one HTTP request over call, streaming GET chunks as they arrive and delivering the response once headers, a chunk or completion arrive.
func doMiniProgramRequest(call miniProgramRequester, method, url, contentType, token string, body io.Reader) (*Response, error) {
	var payload string
	if body != nil {
		data, err := io.ReadAll(body)
		if err != nil {
			return nil, err
		}
		payload = string(data)
	}
	headers := map[string]string{}
	if token != "" {
		headers["X-NSGo-Token"] = token
	}
	if contentType != "" {
		headers["Content-Type"] = contentType
	}
	type readyResult struct {
		status int
		err    error
	}
	sb := newStreamBody()
	ready := make(chan readyResult, 1)
	var (
		mu      sync.Mutex
		chunked bool
		once    sync.Once
	)
	resolve := func(status int, err error) {
		once.Do(func() { ready <- readyResult{status: status, err: err} })
	}
	req := miniProgramRequest{
		Method:        method,
		URL:           url,
		Headers:       headers,
		Body:          payload,
		EnableChunked: method == "GET" && body == nil,
		OnHeaders: func(statusCode int) {
			if statusCode > 0 {
				resolve(statusCode, nil)
			}
		},
		OnChunk: func(data []byte) {
			mu.Lock()
			chunked = true
			mu.Unlock()
			sb.push(data)
			resolve(200, nil)
		},
		OnSuccess: func(statusCode int, data []byte) {
			mu.Lock()
			wasChunked := chunked
			mu.Unlock()
			if !wasChunked && len(data) > 0 {
				sb.push(data)
			}
			if statusCode <= 0 {
				statusCode = 200
			}
			resolve(statusCode, nil)
			sb.finish(nil)
		},
		OnFail: func(err error) {
			if err == nil {
				err = errors.New("devtools: request failed")
			}
			resolve(0, err)
			sb.finish(err)
		},
	}
	abort, err := call(req)
	if err != nil {
		return nil, err
	}
	sb.setAbort(abort)
	res := <-ready
	if res.err != nil {
		return nil, res.err
	}
	return &Response{Status: strconv.Itoa(res.status), StatusCode: res.status, Body: sb}, nil
}
