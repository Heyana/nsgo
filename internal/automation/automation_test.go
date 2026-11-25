package automation

import (
	"encoding/json"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	// Window 0, the app's own, survives as a window named.
	answer, window := 1, uint32(0)
	params, _ := json.Marshal(AlertParams{Window: &window, Answer: &answer})
	data, err := json.Marshal(Request{ID: 7, Method: Alert, Params: params})
	if err != nil {
		t.Fatal(err)
	}
	var req Request
	if err := json.Unmarshal(data, &req); err != nil {
		t.Fatal(err)
	}
	var got AlertParams
	if err := json.Unmarshal(req.Params, &got); err != nil {
		t.Fatal(err)
	}
	if req.ID != 7 || req.Method != Alert || got.Window == nil || *got.Window != 0 || got.Answer == nil || *got.Answer != 1 {
		t.Errorf("round trip: %+v %+v", req, got)
	}
}

func TestReply(t *testing.T) {
	a := Reply(3, StepResult{Settled: true}, nil)
	var r StepResult
	if a.ID != 3 || a.Error != nil || json.Unmarshal(a.Result, &r) != nil || !r.Settled {
		t.Errorf("reply: %+v", a)
	}
	a = Reply(4, nil, Errorf(NoMatch, "no view matches %q", "id:x"))
	if a.Error == nil || a.Error.Error() != `no_match: no view matches "id:x"` || a.Result != nil {
		t.Errorf("error reply: %+v", a)
	}
}

func TestEveryMethodHasParams(t *testing.T) {
	for _, m := range []string{Windows, Snapshot, Find, Press, Type, Key, Scroll, Drag, Hover, Menu, Alert, Wait, Screenshot, Status, Logs, Reload} {
		if Params[m] == nil {
			t.Errorf("%s has no params type", m)
		}
	}
	if len(Params) != 16 {
		t.Errorf("%d params types for 16 methods", len(Params))
	}
}

func TestWindowGraphRoundTrip(t *testing.T) {
	data, err := json.Marshal(WindowsResult{Windows: []Window{
		{ID: 1, Graph: 2, Title: "First", Kind: "window", Width: 300, Height: 200},
		{ID: 2, Title: "Plain", Kind: "window", Width: 300, Height: 200, Default: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var got WindowsResult
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Windows) != 2 || got.Windows[0].Graph != 2 || got.Windows[1].Graph != 0 {
		t.Fatalf("round trip: %+v (%s)", got, data)
	}
}

func TestAppToolResultRoundTrip(t *testing.T) {
	want := AppToolResult{
		Text: "rendered",
		Images: []AppToolImage{
			{Data: []byte("png-bytes"), MIMEType: "image/png"},
			{Data: []byte("jpeg-bytes"), MIMEType: "image/jpeg"},
		},
	}
	encoded, err := EncodeAppToolResult(want)
	if err != nil {
		t.Fatal(err)
	}
	got, ok, err := DecodeAppToolResult(encoded)
	if err != nil || !ok {
		t.Fatalf("DecodeAppToolResult(%s) = %+v, %v, %v", encoded, got, ok, err)
	}
	if got.Text != want.Text || len(got.Images) != 2 || string(got.Images[0].Data) != "png-bytes" || got.Images[1].MIMEType != "image/jpeg" {
		t.Fatalf("decoded = %+v, want %+v", got, want)
	}

	emptyEncoded, err := EncodeAppToolResult(AppToolResult{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := DecodeAppToolResult(emptyEncoded); err != nil || !ok {
		t.Fatalf("empty AppToolResult: ok=%v err=%v", ok, err)
	}

	for _, raw := range []string{
		`{"png":"cG5n"}`,
		`{"_nsgoAppResult":{},"extra":1}`,
		`{"_nsgoAppResult":null}`,
		`{"_nsgoAppResult":"text"}`,
		`{"_nsgoAppResult":{"unknown":1}}`,
		`[1,2]`,
		`"plain"`,
		`null`,
	} {
		if _, ok, err := DecodeAppToolResult([]byte(raw)); ok || err != nil {
			t.Errorf("DecodeAppToolResult(%s) = ok:%v err:%v, want false, nil", raw, ok, err)
		}
	}

	if _, ok, err := DecodeAppToolResult([]byte(`{"_nsgoAppResult":{"images":[{"data":123}]}}`)); ok || err == nil {
		t.Errorf("invalid image data: ok=%v err=%v, want false, non-nil", ok, err)
	}
}
