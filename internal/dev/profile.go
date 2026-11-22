//go:build nsgodev && !nsgominiprogram

package dev

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"time"

	"github.com/nsgo-dev/nsgo/internal/devtools"
)

// devProfile is "1" in nsgo dev -profile's builds, set with -ldflags -X.
var devProfile = ""

type runtimeProfileRequest struct {
	Kind     string        `json:"kind"`
	Duration time.Duration `json:"duration"`
}

func receiveRuntimeProfile(server string) {
	url := server + "/_nsgo/profile"
	request, err := requestRuntimeProfile(url)
	if err == nil && request.Kind != "" {
		err = sendRuntimeProfile(url, request)
	}
	if err != nil {
		if r, sendErr := devtools.Fetch(http.MethodPost, url, "text/plain", strings.NewReader(err.Error())); sendErr == nil {
			r.Body.Close()
			if r.StatusCode == http.StatusNoContent {
				return
			}
		}
		log.Printf("nsgo runtime profile: %v", err)
	}
}

func requestRuntimeProfile(url string) (runtimeProfileRequest, error) {
	var request runtimeProfileRequest
	var resp *devtools.Response
	var err error
	for attempt := 0; ; attempt++ {
		resp, err = devtools.Fetch(http.MethodGet, url, "", nil)
		if err == nil {
			break
		}
		// An iOS app can reach the server once its first Local Network
		// prompt is answered. Like the dev event stream, wait to connect.
		if attempt == 0 {
			log.Printf("nsgo runtime profile: waiting for the dev server: %v", err)
		}
		time.Sleep(time.Second)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK:
		err = json.NewDecoder(resp.Body).Decode(&request)
	case http.StatusNoContent:
	default:
		err = fmt.Errorf("requesting runtime profile: %s", resp.Status)
	}
	return request, err
}

func sendRuntimeProfile(url string, request runtimeProfileRequest) error {
	if request.Kind != "trace" {
		// pprof serializes its sample after collection. Start the request
		// once those bytes are ready, so a device's network does not hold
		// an idle upload connection open during CPU sampling.
		var sample bytes.Buffer
		if err := recordRuntimeProfile(context.Background(), &sample, request); err != nil {
			return err
		}
		return uploadRuntimeProfile(url, &sample)
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader, writer := io.Pipe()
	result := make(chan error, 1)
	go func() {
		err := recordRuntimeProfile(ctx, writer, request)
		writer.CloseWithError(err)
		result <- err
	}()
	uploadErr := uploadRuntimeProfile(url, reader)
	reader.Close()
	cancel()
	if recordErr := <-result; recordErr != nil && !errors.Is(recordErr, context.Canceled) {
		return recordErr
	}
	return uploadErr
}

func uploadRuntimeProfile(url string, body io.Reader) error {
	resp, err := devtools.Fetch(http.MethodPost, url, "application/octet-stream", body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("uploading runtime profile: %s", resp.Status)
	}
	return nil
}

func recordRuntimeProfile(ctx context.Context, w io.Writer, request runtimeProfileRequest) error {
	var stop func()
	switch request.Kind {
	case "heap":
		runtime.GC()
		return pprof.WriteHeapProfile(w)
	case "cpu":
		if runtime.GOARCH == "wasm" {
			return errors.New("Go's WebAssembly runtime has no CPU sampling")
		}
		if err := pprof.StartCPUProfile(w); err != nil {
			return err
		}
		stop = pprof.StopCPUProfile
	case "trace":
		if err := trace.Start(w); err != nil {
			return err
		}
		stop = trace.Stop
	default:
		return fmt.Errorf("unknown runtime profile %q", request.Kind)
	}
	defer stop()
	timer := time.NewTimer(request.Duration)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
