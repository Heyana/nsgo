package devtools

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"sync/atomic"
	"testing"
)

func TestMiniProgramRequestStreamsSSEChunks(t *testing.T) {
	var gotReq miniProgramRequest
	chunkCh := make(chan []byte, 4)
	doneCh := make(chan struct{})
	var aborted atomic.Bool

	call := func(req miniProgramRequest) (func(), error) {
		gotReq = req
		go func() {
			req.OnHeaders(200)
			for chunk := range chunkCh {
				req.OnChunk(chunk)
			}
			req.OnSuccess(200, nil)
			close(doneCh)
		}()
		return func() { aborted.Store(true) }, nil
	}

	resp, err := doMiniProgramRequest(call, "GET", "http://127.0.0.1:8080/_nsgo/events?devtools", "", "secret-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, want 200", resp.StatusCode)
	}
	if !gotReq.EnableChunked {
		t.Error("GET without body did not set EnableChunked")
	}
	if gotReq.Headers["X-NSGo-Token"] != "secret-token" {
		t.Errorf("X-NSGo-Token = %q, want secret-token", gotReq.Headers["X-NSGo-Token"])
	}

	chunkCh <- []byte("data: hello\n\n")
	chunkCh <- []byte("data: devtools {\"op\":\"elements\"}\n\n")
	close(chunkCh)

	var lines []string
	sc := bufio.NewScanner(resp.Body)
	for sc.Scan() {
		if line := sc.Text(); line != "" {
			lines = append(lines, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scanner error: %v", err)
	}
	<-doneCh
	resp.Body.Close()
	if !aborted.Load() {
		t.Error("Body.Close did not invoke abort")
	}
	if want := []string{"data: hello", `data: devtools {"op":"elements"}`}; len(lines) != len(want) || lines[0] != want[0] || lines[1] != want[1] {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
}

func TestMiniProgramRequestResolvesOnChunkWithoutHeaders(t *testing.T) {
	call := func(req miniProgramRequest) (func(), error) {
		go func() {
			req.OnChunk([]byte("data: hello\n\n"))
			req.OnSuccess(0, nil)
		}()
		return nil, nil
	}
	resp, err := doMiniProgramRequest(call, "GET", "http://127.0.0.1:8080/_nsgo/events", "", "tok", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("StatusCode = %d, want 200", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil || string(data) != "data: hello\n\n" {
		t.Fatalf("body = %q, err = %v", data, err)
	}
}

func TestMiniProgramRequestPostsPayloadAndHeaders(t *testing.T) {
	var gotReq miniProgramRequest
	call := func(req miniProgramRequest) (func(), error) {
		gotReq = req
		req.OnSuccess(200, []byte("ok"))
		return nil, nil
	}
	payload := "{\"type\":\"commit\"}\n"
	resp, err := doMiniProgramRequest(call, "POST", "http://127.0.0.1:8080/_nsgo/devtools/out", "application/x-ndjson", "run-token", strings.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if gotReq.EnableChunked {
		t.Error("POST request set EnableChunked")
	}
	if gotReq.Method != "POST" || gotReq.Body != payload {
		t.Fatalf("req = %+v", gotReq)
	}
	if gotReq.Headers["X-NSGo-Token"] != "run-token" || gotReq.Headers["Content-Type"] != "application/x-ndjson" {
		t.Fatalf("headers = %v", gotReq.Headers)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "ok" {
		t.Fatalf("body = %q, err = %v", body, err)
	}
}

func TestMiniProgramRequestErrorsAndClose(t *testing.T) {
	t.Run("non-200", func(t *testing.T) {
		call := func(req miniProgramRequest) (func(), error) {
			req.OnSuccess(401, []byte("unauthorized"))
			return nil, nil
		}
		resp, err := doMiniProgramRequest(call, "GET", "http://127.0.0.1:8080/_nsgo/events?devtools", "", "", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != 401 {
			t.Fatalf("StatusCode = %d, want 401", resp.StatusCode)
		}
		data, _ := io.ReadAll(resp.Body)
		if string(data) != "unauthorized" {
			t.Fatalf("body = %q, want unauthorized", data)
		}
	})

	t.Run("fail before headers", func(t *testing.T) {
		wantErr := errors.New("request:fail connection refused")
		call := func(req miniProgramRequest) (func(), error) {
			req.OnFail(wantErr)
			return nil, nil
		}
		if _, err := doMiniProgramRequest(call, "GET", "http://127.0.0.1:8080/_nsgo/events", "", "tok", nil); !errors.Is(err, wantErr) {
			t.Fatalf("err = %v, want %v", err, wantErr)
		}
	})

	t.Run("fail mid-stream", func(t *testing.T) {
		wantErr := errors.New("request:fail timeout")
		call := func(req miniProgramRequest) (func(), error) {
			go func() {
				req.OnHeaders(200)
				req.OnChunk([]byte("data: hello\n\n"))
				req.OnFail(wantErr)
			}()
			return nil, nil
		}
		resp, err := doMiniProgramRequest(call, "GET", "http://127.0.0.1:8080/_nsgo/events", "", "tok", nil)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if string(data) != "data: hello\n\n" || !errors.Is(err, wantErr) {
			t.Fatalf("data = %q, err = %v, want %v", data, err, wantErr)
		}
	})

	t.Run("close unblocks read and aborts", func(t *testing.T) {
		var aborted atomic.Bool
		call := func(req miniProgramRequest) (func(), error) {
			req.OnHeaders(200)
			return func() { aborted.Store(true) }, nil
		}
		resp, err := doMiniProgramRequest(call, "GET", "http://127.0.0.1:8080/_nsgo/events", "", "tok", nil)
		if err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			buf := make([]byte, 16)
			_, err := resp.Body.Read(buf)
			done <- err
		}()
		resp.Body.Close()
		if err := <-done; !errors.Is(err, io.EOF) {
			t.Fatalf("Read after Close = %v, want io.EOF", err)
		}
		if !aborted.Load() {
			t.Error("Close did not abort the underlying request")
		}
	})
}
