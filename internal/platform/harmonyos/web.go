package harmonyos

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

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

// harmonyWebCookieEntry is one cookie to set, serialized for Bridge.webcookies.
// Every key must match WebCookieSetItem in WebViewConfig.ets.
type harmonyWebCookieEntry struct {
	URL       string `json:"u"`
	SetCookie string `json:"v"`
	Name      string `json:"n"`
}

// harmonyWebCookiesRequest is the wire payload of Bridge.webcookies.
// Every key must match WebCookiesReq in WebViewConfig.ets.
type harmonyWebCookiesRequest struct {
	Op      uint8                   `json:"op"`
	URL     string                  `json:"url,omitempty"`
	Domain  string                  `json:"domain,omitempty"`
	Cookies []harmonyWebCookieEntry `json:"cookies,omitempty"`
}

// harmonyWebCookieWire is one cookie returned by ArkWeb's WebCookieManager.fetchAllCookies.
// Every key must match WebCookieWire in WebViewConfig.ets.
type harmonyWebCookieWire struct {
	Name     string `json:"n"`
	Value    string `json:"v"`
	Domain   string `json:"d"`
	Path     string `json:"p"`
	Expires  string `json:"e,omitempty"`
	Session  bool   `json:"s,omitempty"`
	Secure   bool   `json:"sec,omitempty"`
	HTTPOnly bool   `json:"http,omitempty"`
	SameSite uint8  `json:"ss,omitempty"`
}

const (
	harmonyWebCookiesModeStatus = 0
	harmonyWebCookiesModeAll    = 1
	harmonyWebCookiesModeHeader = 2
)

func encodeWebCookiesRequest(r *driver.WebCookiesRequest) (string, error) {
	if r == nil {
		return "", errors.New("harmonyos: nil web cookies request")
	}
	req := harmonyWebCookiesRequest{Op: uint8(r.Op)}
	switch r.Op {
	case driver.WebCookiesSet:
		req.Cookies = make([]harmonyWebCookieEntry, 0, len(r.Cookies))
		for _, c := range r.Cookies {
			req.Cookies = append(req.Cookies, harmonyWebCookieEntry{
				URL:       c.URL(),
				SetCookie: c.SetCookie(),
				Name:      c.Name,
			})
		}
	case driver.WebCookiesList:
		req.URL = r.Filter.URL
		req.Domain = r.Filter.Domain
	case driver.WebCookiesClear:
	default:
		return "", fmt.Errorf("harmonyos: unsupported web cookies op %d", r.Op)
	}
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func decodeWebCookiesEvent(count uint32, mode int, payload, errText string, filter driver.WebCookieFilter, now time.Time) driver.Event {
	if errText != "" {
		return driver.Event{Kind: driver.EventWebCookies, Count: count, Text: errText}
	}
	switch mode {
	case harmonyWebCookiesModeStatus:
		return driver.Event{Kind: driver.EventWebCookies, Count: count}
	case harmonyWebCookiesModeAll:
		var wire []harmonyWebCookieWire
		if payload != "" {
			if err := json.Unmarshal([]byte(payload), &wire); err != nil {
				return driver.Event{Kind: driver.EventWebCookies, Count: count, Text: fmt.Sprintf("harmonyos: invalid web cookies payload: %v", err)}
			}
		}
		all := make([]driver.WebCookie, 0, len(wire))
		for _, w := range wire {
			ss := driver.WebSameSite(w.SameSite)
			if ss > driver.WebSameSiteNone {
				ss = driver.WebSameSiteDefault
			}
			c := driver.WebCookie{
				Name:     w.Name,
				Value:    w.Value,
				Domain:   strings.ToLower(strings.TrimSpace(w.Domain)),
				Path:     w.Path,
				Secure:   w.Secure,
				HTTPOnly: w.HTTPOnly,
				SameSite: ss,
			}
			if c.Path == "" {
				c.Path = "/"
			}
			if !w.Session && w.Expires != "" {
				c.Expires = parseHarmonyCookieExpires(w.Expires)
			}
			all = append(all, c)
		}
		return driver.Event{
			Kind:  driver.EventWebCookies,
			Count: count,
			Data:  driver.FilterWebCookies(all, filter, now),
		}
	case harmonyWebCookiesModeHeader:
		host := ""
		if u, err := url.Parse(filter.URL); err == nil {
			host = strings.ToLower(u.Hostname())
		}
		parsed := make([]driver.WebCookie, 0)
		for part := range strings.SplitSeq(payload, ";") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			name, value, hasEq := strings.Cut(part, "=")
			if !hasEq {
				name, value = "", part
			}
			parsed = append(parsed, driver.WebCookie{Name: name, Value: value, Domain: host})
		}
		return driver.Event{
			Kind:  driver.EventWebCookies,
			Count: count,
			Data:  driver.FilterWebCookies(parsed, driver.WebCookieFilter{}, now),
		}
	default:
		return driver.Event{Kind: driver.EventWebCookies, Count: count, Text: fmt.Sprintf("harmonyos: unknown web cookies reply mode %d", mode)}
	}
}

var harmonyCookieTimeLayouts = []string{
	http.TimeFormat,
	time.RFC1123,
	time.RFC1123Z,
	"Mon, 02-Jan-2006 15:04:05 GMT",
	"Mon, 02-Jan-06 15:04:05 GMT",
	time.RFC850,
	time.RFC822,
	time.RFC822Z,
	time.ANSIC,
	time.RFC3339Nano,
	time.RFC3339,
	"2006-01-02 15:04:05",
	"2006-01-02T15:04:05",
}

func parseHarmonyCookieExpires(raw string) time.Time {
	s := strings.TrimSpace(raw)
	if s == "" {
		return time.Time{}
	}
	for _, layout := range harmonyCookieTimeLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC()
		}
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		switch {
		case n >= 1e15 || n <= -1e15:
			return time.UnixMicro(n).UTC()
		case n >= 1e12 || n <= -1e12:
			return time.UnixMilli(n).UTC()
		default:
			return time.Unix(n, 0).UTC()
		}
	}
	return time.Time{}
}
