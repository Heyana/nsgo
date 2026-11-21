package android

import (
	"errors"
	"testing"
	"time"
)

func TestTextRPCIgnoresDuplicateAndUnknownReplies(t *testing.T) {
	writes := make(chan uint64, 1)
	rpc := newTextRPC(func(data []byte, _ time.Time) error {
		r := reader{data}
		writes <- r.u64()
		return nil
	})
	defer rpc.close()
	request := func() (<-chan textReply, uint64) {
		t.Helper()
		result := make(chan textReply, 1)
		go func() {
			_, reply := rpc.call(textMeasure, time.Second, func(*writer) {})
			result <- reply
		}()
		select {
		case id := <-writes:
			return result, id
		case <-time.After(time.Second):
			t.Fatal("the text request was not sent")
			return nil, 0
		}
	}
	first, id := request()
	if rpc.complete(id+1, textMeasure, textReply{data: []byte{8}}) {
		t.Fatal("an unknown ID was accepted")
	}
	if !rpc.complete(id, textMeasure, textReply{data: []byte{1}}) {
		t.Fatal("the matching reply was dropped")
	}
	if rpc.complete(id, textMeasure, textReply{data: []byte{2}}) {
		t.Fatal("the duplicate reply was accepted")
	}
	if got := <-first; got.err != nil || string(got.data) != "\x01" {
		t.Fatalf("first reply = %v, %v", got.data, got.err)
	}
	second, next := request()
	if next == id || rpc.complete(id, textMeasure, textReply{}) {
		t.Fatal("a stale reply could reach a new request")
	}
	rpc.complete(next, textLines, textReply{data: []byte{3}})
	if got := <-second; got.err == nil || len(got.data) != 0 {
		t.Fatalf("a reply for another operation was returned: %+v", got)
	}
}

func TestTextRPCDeadlinesCoverAWriteAndAMissingReply(t *testing.T) {
	for _, blockWrite := range []bool{false, true} {
		name := "missing reply"
		if blockWrite {
			name = "blocked write"
		}
		t.Run(name, func(t *testing.T) {
			entered := make(chan struct{})
			resume := make(chan struct{})
			defer close(resume)
			rpc := newTextRPC(func([]byte, time.Time) error {
				close(entered)
				if blockWrite {
					<-resume
				}
				return nil
			})
			defer rpc.close()
			result := make(chan textReply, 1)
			go func() {
				_, reply := rpc.call(textMeasure, 100*time.Millisecond, func(*writer) {})
				result <- reply
			}()
			select {
			case <-entered:
			case <-time.After(time.Second):
				t.Fatal("the writer was not called")
			}
			select {
			case got := <-result:
				if !errors.Is(got.err, errTextTimeout) {
					t.Fatalf("text request returned %v", got.err)
				}
			case <-time.After(time.Second):
				t.Fatal("the text request exceeded its deadline")
			}
			if rpc.complete(1, textMeasure, textReply{}) {
				t.Fatal("a reply after the deadline was accepted")
			}
		})
	}
}
