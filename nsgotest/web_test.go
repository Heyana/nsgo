package nsgotest

import (
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

func TestWebConfigSnapshotOwnsItsMaps(t *testing.T) {
	source := core.WebSrc("https://ok.example/home")
	cfg := driver.WebConfig{
		Source: source, Headers: map[string]string{"X-Test": "original"},
		SchemeHandlers: map[string]core.WebSchemeHandler{
			"app": func(*core.WebSchemeRequest) *core.WebSchemeResponse {
				return &core.WebSchemeResponse{Status: 200}
			},
		},
		Restricted: true, Hosts: "ok.example", Schemes: "about", Media: core.WebMicrophone,
	}
	snapshot := webConfig(cfg)
	cfg.Headers["X-Test"] = "changed"
	delete(cfg.SchemeHandlers, "app")
	if snapshot.Source != source || snapshot.Headers["X-Test"] != "original" || snapshot.Media != core.WebMicrophone {
		t.Fatalf("snapshot lost the original configuration: %+v", snapshot)
	}
	if handler := snapshot.SchemeHandlers["app"]; handler == nil || handler(nil).Status != 200 {
		t.Fatal("snapshot lost its scheme handler")
	}
	if !snapshot.Navigable(source.Src) || !snapshot.Navigable("app://article") || snapshot.Navigable("data:,page") {
		t.Fatal("snapshot did not preserve the page navigation policy")
	}
	if !snapshot.SourceNavigable("data:,source") || snapshot.SourceNavigable("https://other.example/") {
		t.Fatal("snapshot did not preserve the app's source navigation policy")
	}
	allowed := core.WebWindowRequest{URL: source.Src, UserInitiated: true}
	if got := snapshot.OpenWindowEvent(allowed); got.Kind != driver.EventWebOpenWindow || got.Data != allowed {
		t.Fatalf("allowed popup gave %+v", got)
	}
	refused := core.WebWindowRequest{URL: "https://other.example/"}
	if got := snapshot.OpenWindowEvent(refused); got.Kind != EventError || got.Navigation == nil || got.Navigation.URL != refused.URL {
		t.Fatalf("refused popup gave %+v", got)
	}
}
