package main

import (
	"strings"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// capabilities reads nsgo.yaml's capabilities strictly, nil when it has
// none. YAML's keys are snake_case (open_url, status_bar, path_prefixes),
// the package's camelCase.
func (p *project) capabilities() (*pkg.Capabilities, error) {
	return p.Config.ParseCapabilities()
}

// describeCapabilities lists what c grants, a line each.
func describeCapabilities(c pkg.Capabilities) []string {
	var out []string
	if n := c.Network; n != nil {
		for _, h := range n.Hosts {
			out = append(out, "network: "+h)
		}
		for _, r := range n.HTTP {
			line := "network: " + r.Host
			if len(r.Methods) > 0 {
				line += " " + strings.Join(r.Methods, ",")
			}
			if len(r.PathPrefixes)+len(r.Paths) > 0 {
				line += " " + strings.Join(append(append([]string(nil), r.Paths...), r.PathPrefixes...), ",")
			}
			out = append(out, line)
		}
	}
	if f := c.FS; f != nil && len(f.Read) > 0 {
		out = append(out, "fs.read: "+strings.Join(f.Read, ", "))
	}
	if w := c.WebView; w != nil {
		line := "webview: " + strings.Join(w.Hosts, ", ")
		if w.Input {
			line += ", with trusted input"
		}
		if len(w.Media) > 0 {
			line += ", capturing " + strings.Join(w.Media, " and ")
		}
		out = append(out, line)
	}
	if o := c.OpenURL; o != nil {
		schemes := "any URL"
		if len(o.Schemes) > 0 {
			schemes = strings.Join(o.Schemes, ", ")
		}
		out = append(out, "open_url: "+schemes)
	}
	for _, b := range []struct {
		on   bool
		name string
	}{{c.Clipboard, "clipboard"}, {c.Picker, "picker"}, {c.Speech, "speech"}, {c.TextToSpeech, "text_to_speech"}, {c.StatusBar, "status_bar"},
		{c.Notifications, "notifications"}, {c.Biometrics, "biometrics"}, {c.SignInWithApple, "sign_in_with_apple"}, {c.Purchases, "purchases"}, {c.Secrets, "secrets"}, {c.Audio, "audio"}, {c.Gamepad, "gamepad"}, {c.AuthSession, "auth_session"}} {
		if b.on {
			out = append(out, b.name)
		}
	}
	if len(c.Host) > 0 {
		out = append(out, "host: "+strings.Join(c.Host, ", "))
	}
	return out
}
