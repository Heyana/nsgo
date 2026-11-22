package nsgotest

import (
	"maps"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/internal/driver"
)

// WebConfig records the source, settings and permissions of a mounted web view.
type WebConfig struct {
	Source                          WebSource
	InjectedJavaScript              string
	InjectedJavaScriptBeforeContent string
	UserAgent                       string
	UserAgentSuffix                 string
	// Headers accompany the initial URL request where custom headers are supported.
	Headers map[string]string
	// Incognito keeps website data in memory for this view.
	Incognito bool
	// SchemeHandlers are the app's responders for its URL schemes.
	SchemeHandlers      map[string]core.WebSchemeHandler
	NoScroll            bool
	NoJavaScript        bool
	Muted               bool
	PinchZoom           bool
	BackForwardGestures bool
	Mobile              bool
	// DownloadDir is where downloads go. An empty path disables saving them.
	DownloadDir string
	// Restricted applies the navigation rules below.
	Restricted bool
	// Hosts and Schemes are the space-separated host patterns and page schemes
	// allowed by a restricted view.
	Hosts, Schemes string
	Media          core.WebMedia
}

func webConfig(c driver.WebConfig) WebConfig {
	return WebConfig{
		Source:                          c.Source,
		InjectedJavaScript:              c.InjectedJavaScript,
		InjectedJavaScriptBeforeContent: c.InjectedJavaScriptBeforeContent,
		UserAgent:                       c.UserAgent,
		UserAgentSuffix:                 c.UserAgentSuffix,
		Headers:                         maps.Clone(c.Headers),
		Incognito:                       c.Incognito,
		SchemeHandlers:                  maps.Clone(c.SchemeHandlers),
		NoScroll:                        c.NoScroll,
		NoJavaScript:                    c.NoJavaScript,
		Muted:                           c.Muted,
		PinchZoom:                       c.PinchZoom,
		BackForwardGestures:             c.BackForwardGestures,
		Mobile:                          c.Mobile,
		DownloadDir:                     c.DownloadDir,
		Restricted:                      c.Restricted,
		Hosts:                           c.Hosts,
		Schemes:                         c.Schemes,
		Media:                           c.Media,
	}
}

// Navigable reports whether the page may navigate to rawURL in its top frame.
func (c *WebConfig) Navigable(rawURL string) bool {
	return c.navigationPolicy().Navigable(rawURL)
}

// SourceNavigable reports whether the app may load rawURL in the web view.
func (c *WebConfig) SourceNavigable(rawURL string) bool {
	return c.navigationPolicy().SourceNavigable(rawURL)
}

// OpenWindowEvent returns the platform report for the page's request to open
// another window, using the recorded navigation policy.
func (c *WebConfig) OpenWindowEvent(r core.WebWindowRequest) Event {
	ev := c.navigationPolicy().OpenWindowEvent(r)
	return Event{Kind: ev.Kind, Text: ev.Text, Navigation: ev.Navigation, Data: ev.Data}
}

func (c *WebConfig) navigationPolicy() *driver.WebConfig {
	return &driver.WebConfig{
		Restricted:     c.Restricted,
		Hosts:          c.Hosts,
		Schemes:        c.Schemes,
		SchemeHandlers: c.SchemeHandlers,
	}
}
