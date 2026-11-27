package harmonyos

import (
	"encoding/json"
	"maps"
	"slices"
	"strings"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// harmonyWebHeader is one custom request header serialized to the ArkTS host.
type harmonyWebHeader struct {
	Key   string `json:"k"`
	Value string `json:"v"`
}

// harmonyWebConfig is the wire format of ItemWeb sent to ArkWeb in Index.ets.
// Every key must match WebConfigOp in WebViewConfig.ets.
type harmonyWebConfig struct {
	Src             string             `json:"src,omitempty"`
	HTML            string             `json:"html,omitempty"`
	BaseURL         string             `json:"base,omitempty"`
	Before          string             `json:"before,omitempty"`
	After           string             `json:"after,omitempty"`
	UserAgent       string             `json:"ua,omitempty"`
	UserAgentSuffix string             `json:"uas,omitempty"`
	Headers         []harmonyWebHeader `json:"headers,omitempty"`
	Incognito       bool               `json:"incognito,omitempty"`
	NoScroll        bool               `json:"noScroll,omitempty"`
	NoJavaScript    bool               `json:"noJS,omitempty"`
	Muted           bool               `json:"muted,omitempty"`
	PinchZoom       bool               `json:"pinchZoom,omitempty"`
	DownloadDir     string             `json:"downloadDir,omitempty"`
	Restricted      bool               `json:"restricted,omitempty"`
	Hosts           []string           `json:"hosts,omitempty"`
	Schemes         []string           `json:"schemes,omitempty"`
	AppSchemes      []string           `json:"appSchemes,omitempty"`
	Media           uint8              `json:"media,omitempty"`
}

// harmonyWebLoad is the JSON payload carried by wcmd for driver.WebLoad.
type harmonyWebLoad struct {
	Src     string `json:"src,omitempty"`
	HTML    string `json:"html,omitempty"`
	BaseURL string `json:"base,omitempty"`
}

// harmonyWebDownload is the JSON payload reported by ArkWeb's download delegate.
type harmonyWebDownload struct {
	State    uint8  `json:"state"`
	ID       string `json:"id"`
	URL      string `json:"url"`
	Filename string `json:"filename"`
	Path     string `json:"path"`
	Bytes    int64  `json:"bytes"`
	Message  string `json:"message"`
}

func cloneWebConfig(c *driver.WebConfig) *driver.WebConfig {
	if c == nil {
		return nil
	}
	cp := *c
	if len(c.Headers) > 0 {
		cp.Headers = maps.Clone(c.Headers)
	}
	if len(c.SchemeHandlers) > 0 {
		cp.SchemeHandlers = maps.Clone(c.SchemeHandlers)
	}
	return &cp
}

func encodeWebConfig(c *driver.WebConfig) (string, error) {
	if c == nil {
		return "{}", nil
	}
	src := driver.ResolveWebSource(c.Source)
	out := harmonyWebConfig{
		Src:             src.Src,
		HTML:            src.HTML,
		BaseURL:         src.BaseURL,
		Before:          c.InjectedJavaScriptBeforeContent,
		After:           c.InjectedJavaScript,
		UserAgent:       c.UserAgent,
		UserAgentSuffix: c.UserAgentSuffix,
		Incognito:       c.Incognito,
		NoScroll:        c.NoScroll,
		NoJavaScript:    c.NoJavaScript,
		Muted:           c.Muted,
		PinchZoom:       c.PinchZoom,
		DownloadDir:     c.DownloadDir,
		Restricted:      c.Restricted,
		Media:           uint8(c.Media),
	}
	if len(c.Headers) > 0 {
		keys := make([]string, 0, len(c.Headers))
		for k := range c.Headers {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		out.Headers = make([]harmonyWebHeader, 0, len(keys))
		for _, k := range keys {
			out.Headers = append(out.Headers, harmonyWebHeader{Key: k, Value: c.Headers[k]})
		}
	}
	if fields := strings.Fields(c.Hosts); len(fields) > 0 {
		out.Hosts = fields
	}
	if fields := strings.Fields(c.Schemes); len(fields) > 0 {
		out.Schemes = make([]string, 0, len(fields))
		for _, s := range fields {
			out.Schemes = append(out.Schemes, strings.ToLower(s))
		}
	}
	if len(c.SchemeHandlers) > 0 {
		schemes := make([]string, 0, len(c.SchemeHandlers))
		for k := range c.SchemeHandlers {
			schemes = append(schemes, strings.ToLower(k))
		}
		slices.Sort(schemes)
		out.AppSchemes = schemes
	}
	data, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func encodeWebLoad(src *driver.WebSource) (string, error) {
	var s driver.WebSource
	if src != nil {
		s = *src
	}
	s = driver.ResolveWebSource(s)
	data, err := json.Marshal(harmonyWebLoad{
		Src:     s.Src,
		HTML:    s.HTML,
		BaseURL: s.BaseURL,
	})
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeWebNavigation(url, title string, flags int, loading bool) *driver.WebNavigation {
	return &driver.WebNavigation{
		URL:          url,
		Title:        title,
		Loading:      loading,
		CanGoBack:    flags&1 != 0,
		CanGoForward: flags&2 != 0,
	}
}

func decodeWebDownload(raw string) (*driver.WebDownload, bool) {
	if raw == "" {
		return nil, false
	}
	var in harmonyWebDownload
	if err := json.Unmarshal([]byte(raw), &in); err != nil {
		return nil, false
	}
	return &driver.WebDownload{
		State:    driver.WebDownloadState(in.State),
		ID:       in.ID,
		URL:      in.URL,
		Filename: in.Filename,
		Path:     in.Path,
		Bytes:    in.Bytes,
		Message:  in.Message,
	}, true
}
