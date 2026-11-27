package web

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/nsgo-dev/nsgo/internal/driver"
)

// sameWebFrame reports whether a and b load the same frame on the web,
// comparing every setting the browser host applies.
func sameWebFrame(a, b *driver.WebConfig) bool {
	if a.Source != b.Source || a.InjectedJavaScript != b.InjectedJavaScript ||
		a.InjectedJavaScriptBeforeContent != b.InjectedJavaScriptBeforeContent ||
		a.NoScroll != b.NoScroll || a.NoJavaScript != b.NoJavaScript || a.Media != b.Media ||
		a.Restricted != b.Restricted || a.Hosts != b.Hosts || a.Schemes != b.Schemes {
		return false
	}
	if len(a.SchemeHandlers) != len(b.SchemeHandlers) {
		return false
	}
	for k := range a.SchemeHandlers {
		if _, ok := b.SchemeHandlers[k]; !ok {
			return false
		}
	}
	return true
}

// webFrameSandbox returns the iframe sandbox attribute tokens for c and src.
// Restricted views omit allow-popups, allow-popups-to-escape-sandbox and
// allow-downloads, and no view grants allow-top-navigation.
func webFrameSandbox(c *driver.WebConfig, src driver.WebSource, pageOrigin string, isSrcdoc bool) string {
	var sandbox []string
	if c.Restricted {
		sandbox = []string{"allow-forms", "allow-modals"}
	} else {
		sandbox = []string{"allow-forms", "allow-popups", "allow-popups-to-escape-sandbox", "allow-modals", "allow-downloads"}
	}
	if !c.NoJavaScript {
		sandbox = append(sandbox, "allow-scripts")
	}
	if !isSrcdoc && src.HTML == "" && src.Src != "" {
		// Pages of other sites keep their origin, so their storage works; one of the app's own must not reach the app, nor an about: or data: page, such as an empty source's about:blank, which takes the app's origin unless the sandbox keeps it opaque, as it keeps a srcdoc page.
		if !isSameOrOpaqueOrigin(src.Src, pageOrigin) {
			sandbox = append(sandbox, "allow-same-origin")
		}
	}
	return strings.Join(sandbox, " ")
}

func isSameOrOpaqueOrigin(rawURL, pageOrigin string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	scheme := strings.ToLower(u.Scheme)
	switch scheme {
	case "", "about", "data":
		return true
	}
	origin := driver.WebOrigin(rawURL)
	if origin == "" || origin == "null" {
		return true
	}
	return pageOrigin != "" && strings.EqualFold(origin, pageOrigin)
}

// decodeRestrictedDataHTML decodes a data: URL carrying HTML so a restricted
// web view can load it through its guarded srcdoc pipeline.
func decodeRestrictedDataHTML(rawURL string) (string, bool) {
	if len(rawURL) < 5 || !strings.EqualFold(rawURL[:5], "data:") {
		return "", false
	}
	meta, payload, ok := strings.Cut(rawURL[5:], ",")
	if !ok {
		return "", false
	}
	parts := strings.Split(meta, ";")
	mediaType := strings.ToLower(strings.TrimSpace(parts[0]))
	if mediaType != "text/html" && mediaType != "application/xhtml+xml" {
		return "", false
	}
	isBase64 := false
	for _, p := range parts[1:] {
		if strings.EqualFold(strings.TrimSpace(p), "base64") {
			isBase64 = true
		}
	}
	if isBase64 {
		clean := payload
		if unescaped, err := url.PathUnescape(clean); err == nil {
			clean = unescaped
		}
		// Strip ASCII whitespace per the Fetch data: URL processor.
		clean = strings.Map(func(r rune) rune {
			switch r {
			case ' ', '\t', '\r', '\n', '\f':
				return -1
			}
			return r
		}, clean)
		if decoded, err := base64.StdEncoding.DecodeString(clean); err == nil {
			return string(decoded), true
		}
		if decoded, err := base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "=")); err == nil {
			return string(decoded), true
		}
		return "", false
	}
	decoded, err := url.PathUnescape(payload)
	if err != nil {
		return payload, true
	}
	return decoded, true
}

// webRestrictedCSP builds the Content-Security-Policy meta content for a
// restricted srcdoc page, constraining form submissions and <base> URIs to
// hosts and schemes WebConfig.Navigable permits.
func webRestrictedCSP(c *driver.WebConfig) string {
	var sources []string
	seen := map[string]bool{}
	add := func(s string) {
		if s != "" && !seen[s] {
			seen[s] = true
			sources = append(sources, s)
		}
	}
	hosts := strings.Fields(c.Hosts)
	anyHost := false
	for _, h := range hosts {
		if h == "*" {
			anyHost = true
			break
		}
	}
	if anyHost {
		add("http:")
		add("https:")
	} else {
		for _, h := range hosts {
			h = strings.ToLower(strings.TrimSuffix(h, "."))
			if isValidCSPHost(h) {
				add("http://" + h + ":*")
				add("https://" + h + ":*")
			}
		}
	}
	for _, scheme := range strings.Fields(c.Schemes) {
		switch s := strings.ToLower(scheme); s {
		case "about", "blob", "data":
			add(s + ":")
		}
	}
	for _, scheme := range sortedSchemeNames(c.SchemeHandlers) {
		if isValidSchemeName(scheme) {
			add(scheme + ":")
		}
	}
	if len(sources) == 0 {
		return "form-action 'none'; base-uri 'none'"
	}
	joined := strings.Join(sources, " ")
	return "form-action " + joined + "; base-uri " + joined
}

func isValidCSPHost(h string) bool {
	if h == "" {
		return false
	}
	rest := h
	if cut, ok := strings.CutPrefix(rest, "*."); ok {
		rest = cut
		if rest == "" {
			return false
		}
	}
	for i := 0; i < len(rest); i++ {
		ch := rest[i]
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '-' || ch == '.') {
			return false
		}
	}
	return true
}

func isValidSchemeName(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if i == 0 {
			if !(ch >= 'a' && ch <= 'z') {
				return false
			}
			continue
		}
		if !((ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '+' || ch == '-' || ch == '.') {
			return false
		}
	}
	return true
}

func sortedSchemeNames(m map[string]driver.WebSchemeHandler) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, strings.ToLower(k))
	}
	slices.Sort(out)
	return out
}

type webMetaRefresh struct {
	DelayMS int    `json:"delayMs"`
	URL     string `json:"url"`
}

// webRestrictedBridge returns the script prepended to restricted srcdoc pages:
// it connects the private MessagePort to the host, enforces WebConfig.Navigable
// on top-level navigations (Navigation API, links, forms, window.open), and
// optionally exposes window.nsgo.postMessage when exposeBridge is true.
func webRestrictedBridge(token string, exposeBridge bool, c *driver.WebConfig, refreshes []webMetaRefresh) string {
	hosts := make([]string, 0)
	for _, h := range strings.Fields(c.Hosts) {
		hosts = append(hosts, strings.ToLower(strings.TrimSuffix(h, ".")))
	}
	schemes := make([]string, 0)
	for _, s := range strings.Fields(c.Schemes) {
		schemes = append(schemes, strings.ToLower(s))
	}
	customSchemes := sortedSchemeNames(c.SchemeHandlers)
	if customSchemes == nil {
		customSchemes = []string{}
	}
	if refreshes == nil {
		refreshes = []webMetaRefresh{}
	}
	hostsJSON, _ := json.Marshal(hosts)
	schemesJSON, _ := json.Marshal(schemes)
	customJSON, _ := json.Marshal(customSchemes)
	refreshesJSON, _ := json.Marshal(refreshes)
	expose := "false"
	if exposeBridge {
		expose = "true"
	}
	return `(() => {
  const channel = new MessageChannel(), port = channel.port1;
  const post = port.postMessage.bind(port);
  const URLCtor = window.URL;
  const setTimer = window.setTimeout.bind(window);
  const getAttr = Element.prototype.getAttribute;
  const hasAttr = Element.prototype.hasAttribute;
  const toLower = String.prototype.toLowerCase;
  const trim = String.prototype.trim;
  const startsWith = String.prototype.startsWith;
  const endsWith = String.prototype.endsWith;
  const slice = String.prototype.slice;
  const prevent = Event.prototype.preventDefault;
  const stopImm = Event.prototype.stopImmediatePropagation;
  const addWinListener = window.addEventListener.bind(window);
  const hosts = ` + string(hostsJSON) + `;
  const schemes = ` + string(schemesJSON) + `;
  const customSchemes = ` + string(customJSON) + `;
  const allowedRefreshes = ` + string(refreshesJSON) + `;
  const resolveURL = (raw) => {
    try { return new URLCtor(String(raw), document.baseURI).href; }
    catch (_) { return String(raw); }
  };
  const navigable = (rawURL) => {
    let u;
    try { u = new URLCtor(rawURL); } catch (_) { return false; }
    const proto = toLower.call(u.protocol);
    const scheme = endsWith.call(proto, ":") ? slice.call(proto, 0, -1) : proto;
    if (scheme === "about" || scheme === "data" || scheme === "blob") {
      for (let i = 0; i < schemes.length; i++) {
        if (schemes[i] === scheme) return true;
      }
      return false;
    }
    if (scheme === "http" || scheme === "https") {
      let host = toLower.call(u.hostname);
      if (startsWith.call(host, "[") && endsWith.call(host, "]")) host = slice.call(host, 1, -1);
      if (endsWith.call(host, ".")) host = slice.call(host, 0, -1);
      for (let i = 0; i < hosts.length; i++) {
        const p = hosts[i];
        if (p === "*") return true;
        if (startsWith.call(p, "*.")) {
          const suffix = slice.call(p, 1);
          if (endsWith.call(host, suffix) && host.length > suffix.length) return true;
        } else if (p === host) {
          return true;
        }
      }
      return false;
    }
    for (let i = 0; i < customSchemes.length; i++) {
      if (customSchemes[i] === scheme) return true;
    }
    return false;
  };
  const reportRefused = (url) => { post({ refused: String(url) }); };
  const reportOpenWindow = (url, userInitiated) => { post({ openWindow: String(url), userInitiated: Boolean(userInitiated) }); };
  const reportNavigate = (url) => { post({ navigate: String(url) }); };
  let approvedUnload = false;
  let reportedUnchecked = false;
  const approveUnloadBriefly = () => {
    approvedUnload = true;
    setTimer(() => { approvedUnload = false; }, 100);
  };
  const hasChildFrame = (name) => {
    if (!name) return false;
    const frames = document.getElementsByTagName("iframe");
    for (let i = 0; i < frames.length; i++) {
      if (toLower.call(getAttr.call(frames[i], "name") || "") === name) return true;
    }
    return false;
  };
  const baseTarget = () => {
    const base = document.querySelector("base[target]");
    return base ? toLower.call(trim.call(getAttr.call(base, "target") || "")) : "";
  };
  const findLink = (e) => {
    const path = typeof e.composedPath === "function" ? e.composedPath() : [e.target];
    for (let i = 0; i < path.length; i++) {
      const node = path[i];
      if (node && node.nodeType === 1) {
        const tag = toLower.call(node.localName || node.tagName || "");
        if ((tag === "a" || tag === "area") && (hasAttr.call(node, "href") || hasAttr.call(node, "xlink:href"))) {
          return node;
        }
      }
    }
    return null;
  };
  if (window.navigation && typeof window.navigation.addEventListener === "function") {
    window.navigation.addEventListener("navigate", (e) => {
      if (!e || !e.destination || typeof e.destination.url !== "string") return;
      if (e.hashChange) return;
      const dest = e.destination.url;
      if (!navigable(dest)) {
        if (e.cancelable) {
          prevent.call(e);
          reportRefused(dest);
        }
      } else {
        approveUnloadBriefly();
      }
    });
  }
  const guardLinkCapture = (e) => {
    if (e.defaultPrevented) return;
    const el = findLink(e);
    if (!el) return;
    const raw = hasAttr.call(el, "href") ? getAttr.call(el, "href") : getAttr.call(el, "xlink:href");
    if (raw == null) return;
    const trimmed = trim.call(raw);
    const target = toLower.call(trim.call(getAttr.call(el, "target") || "")) || baseTarget();
    if (startsWith.call(trimmed, "#") && (!target || target === "_self")) return;
    const dest = resolveURL(trimmed);
    if (e.type === "auxclick" || (target && target !== "_self" && target !== "_parent" && target !== "_top")) {
      if (e.type !== "auxclick" && hasChildFrame(target)) return;
      prevent.call(e);
      stopImm.call(e);
      reportOpenWindow(dest, e.isTrusted);
      return;
    }
    if (target === "_top" || target === "_parent" || !navigable(dest)) {
      prevent.call(e);
      stopImm.call(e);
      reportRefused(dest);
    }
  };
  addWinListener("click", guardLinkCapture, true);
  addWinListener("auxclick", guardLinkCapture, true);
  addWinListener("click", (e) => {
    if (e.defaultPrevented) return;
    const el = findLink(e);
    if (!el) return;
    const raw = hasAttr.call(el, "href") ? getAttr.call(el, "href") : getAttr.call(el, "xlink:href");
    if (raw == null) return;
    const trimmed = trim.call(raw);
    const target = toLower.call(trim.call(getAttr.call(el, "target") || "")) || baseTarget();
    if (startsWith.call(trimmed, "#") && (!target || target === "_self")) return;
    if (target && target !== "_self") return;
    const dest = resolveURL(trimmed);
    prevent.call(e);
    reportNavigate(dest);
  }, false);
  const checkForm = (form, submitter, trusted, approve) => {
    const rawAction = (submitter && hasAttr.call(submitter, "formaction") ? getAttr.call(submitter, "formaction") : null) ??
      (hasAttr.call(form, "action") ? getAttr.call(form, "action") : "") ?? "";
    const trimmed = trim.call(rawAction);
    const target = toLower.call(trim.call(
      (submitter && hasAttr.call(submitter, "formtarget") ? getAttr.call(submitter, "formtarget") : "") ||
      getAttr.call(form, "target") || ""
    )) || baseTarget();
    const dest = resolveURL(trimmed);
    if (target && target !== "_self" && target !== "_parent" && target !== "_top") {
      if (hasChildFrame(target)) return true;
      reportOpenWindow(dest, trusted);
      return false;
    }
    if (target === "_top" || target === "_parent" || !navigable(dest)) {
      reportRefused(dest);
      return false;
    }
    if (approve) approveUnloadBriefly();
    return true;
  };
  addWinListener("submit", (e) => {
    if (e.defaultPrevented) return;
    const form = e.target;
    if (!form || form.nodeType !== 1) return;
    if (!checkForm(form, e.submitter || null, e.isTrusted, false)) {
      prevent.call(e);
      stopImm.call(e);
    }
  }, true);
  addWinListener("submit", (e) => {
    if (e.defaultPrevented) return;
    const form = e.target;
    if (!form || form.nodeType !== 1) return;
    if (checkForm(form, e.submitter || null, e.isTrusted, true)) return;
    prevent.call(e);
  }, false);
  if (window.HTMLFormElement && HTMLFormElement.prototype) {
    const origSubmit = HTMLFormElement.prototype.submit;
    if (typeof origSubmit === "function") {
      HTMLFormElement.prototype.submit = function () {
        if (!checkForm(this, null, false, true)) return;
        return origSubmit.apply(this, arguments);
      };
    }
    const origRequestSubmit = HTMLFormElement.prototype.requestSubmit;
    if (typeof origRequestSubmit === "function") {
      HTMLFormElement.prototype.requestSubmit = function (submitter) {
        if (!checkForm(this, submitter || null, false, false)) return;
        return origRequestSubmit.apply(this, arguments);
      };
    }
  }
  const openBlocked = function (url) {
    const raw = url === undefined || url === null || url === "" ? "about:blank" : String(url);
    const dest = resolveURL(raw);
    const active = Boolean(navigator.userActivation && navigator.userActivation.isActive);
    reportOpenWindow(dest, active);
    return null;
  };
  try {
    Object.defineProperty(window, "open", { value: openBlocked, writable: false, configurable: false });
  } catch (_) {
    try { window.open = openBlocked; } catch (_) {}
  }
  for (let i = 0; i < allowedRefreshes.length; i++) {
    const r = allowedRefreshes[i];
    setTimer(() => { reportNavigate(r.url); }, r.delayMs);
  }
  const onUncheckedLeave = () => {
    if (approvedUnload || reportedUnchecked) return;
    reportedUnchecked = true;
    post({ uncheckedNav: true });
    try { window.stop(); } catch (_) {}
  };
  addWinListener("beforeunload", onUncheckedLeave, true);
  if (` + expose + `) {
    window.nsgo = { postMessage(data) { post(String(data)); } };
    port.onmessage = (m) => {
      const d = m.data;
      if (d && d.eval != null) { try { (0, eval)(String(d.eval)); } catch (err) { console.error(err); } }
      else if (d && d.go) history.go(d.go);
    };
  }
  addWinListener("pagehide", (e) => {
    if (!e.persisted) {
      if (!approvedUnload && !reportedUnchecked) {
        reportedUnchecked = true;
        post({ uncheckedNav: true });
      } else {
        post(null);
      }
    }
  });
  parent.postMessage(` + strconv.Quote(token) + `, "*", [channel.port2]);
})();`
}

// buildRestrictedSrcdoc prepares rawHTML for a restricted srcdoc iframe:
// it strips disallowed <meta http-equiv="refresh"> redirects, neutralizes
// disallowed static links/forms when JavaScript is disabled, prepends the CSP
// meta tag, base tag, navigation guard/bridge script, NoScroll style and
// InjectedJavaScriptBeforeContent at the start of <head>, and appends
// InjectedJavaScript at the end of <body>.
func buildRestrictedSrcdoc(rawHTML, baseURL, token string, exposeBridge bool, c *driver.WebConfig) (docHTML, title string, refusedMeta []string) {
	doc, err := html.Parse(strings.NewReader(rawHTML))
	if err != nil {
		// Fallback to minimal document if parsing fails.
		doc = &html.Node{Type: html.DocumentNode}
	}
	htmlEl, headEl, bodyEl := ensureHTMLStructure(doc)
	_ = htmlEl

	effectiveBase := baseURL
	var docBaseTarget string
	var walkBase func(*html.Node)
	walkBase = func(n *html.Node) {
		if n.Type == html.ElementNode && n.DataAtom == atom.Base {
			if effectiveBase == "" {
				if href, ok := htmlAttr(n, "href"); ok && strings.TrimSpace(href) != "" {
					effectiveBase = strings.TrimSpace(href)
				}
			}
			if docBaseTarget == "" {
				if target, ok := htmlAttr(n, "target"); ok && strings.TrimSpace(target) != "" {
					docBaseTarget = strings.ToLower(strings.TrimSpace(target))
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walkBase(ch)
		}
	}
	walkBase(doc)

	title = extractHTMLTitle(doc)

	// Walk and sanitize disallowed meta refreshes (always) and static links/forms
	// (when NoJavaScript disables the in-document navigation guard).
	var allowedRefreshes []webMetaRefresh
	var sanitize func(*html.Node)
	sanitize = func(n *html.Node) {
		for ch := n.FirstChild; ch != nil; {
			next := ch.NextSibling
			if ch.Type == html.ElementNode {
				switch ch.DataAtom {
				case atom.Meta:
					if eq, ok := htmlAttr(ch, "http-equiv"); ok && strings.EqualFold(strings.TrimSpace(eq), "refresh") {
						if content, ok := htmlAttr(ch, "content"); ok {
							if delayMS, refreshURL, hasURL := parseMetaRefresh(content); hasURL {
								resolved := resolveHTMLURL(refreshURL, effectiveBase)
								if !isSameDocFragment(refreshURL) {
									if !c.Navigable(resolved) {
										refusedMeta = append(refusedMeta, resolved)
										n.RemoveChild(ch)
										ch = next
										continue
									}
									if !c.NoJavaScript && token != "" {
										allowedRefreshes = append(allowedRefreshes, webMetaRefresh{DelayMS: delayMS, URL: resolved})
										n.RemoveChild(ch)
										ch = next
										continue
									}
								}
							}
						}
					}
				case atom.Base:
					if target, ok := htmlAttr(ch, "target"); ok {
						t := strings.ToLower(strings.TrimSpace(target))
						if t == "_top" || t == "_parent" || (c.NoJavaScript && t != "" && t != "_self") {
							removeHTMLAttr(ch, "target")
						}
					}
				case atom.A, atom.Area:
					removeHTMLAttr(ch, "download")
					if c.NoJavaScript {
						sanitizeStaticLink(ch, effectiveBase, docBaseTarget, c)
					}
				case atom.Form:
					if c.NoJavaScript {
						sanitizeStaticForm(ch, effectiveBase, docBaseTarget, c)
					}
				case atom.Button, atom.Input:
					if c.NoJavaScript {
						sanitizeStaticFormButton(ch, effectiveBase, docBaseTarget, c)
					}
				}
			}
			sanitize(ch)
			ch = next
		}
	}
	sanitize(doc)

	// Build leading head elements in order: base, CSP meta, bridge/guard,
	// NoScroll style, InjectedJavaScriptBeforeContent.
	var prepend []*html.Node
	if baseURL != "" {
		prepend = append(prepend, &html.Node{
			Type:     html.ElementNode,
			DataAtom: atom.Base,
			Data:     "base",
			Attr:     []html.Attribute{{Key: "href", Val: baseURL}},
		})
	}
	prepend = append(prepend, &html.Node{
		Type:     html.ElementNode,
		DataAtom: atom.Meta,
		Data:     "meta",
		Attr: []html.Attribute{
			{Key: "http-equiv", Val: "Content-Security-Policy"},
			{Key: "content", Val: webRestrictedCSP(c)},
		},
	})
	if token != "" {
		prepend = append(prepend, newScriptNode(webRestrictedBridge(token, exposeBridge, c, allowedRefreshes)))
	}
	if c.NoScroll {
		prepend = append(prepend, &html.Node{
			Type:       html.ElementNode,
			DataAtom:   atom.Style,
			Data:       "style",
			FirstChild: &html.Node{Type: html.TextNode, Data: "html,body{overflow:hidden!important}"},
		})
		prepend[len(prepend)-1].LastChild = prepend[len(prepend)-1].FirstChild
		prepend[len(prepend)-1].FirstChild.Parent = prepend[len(prepend)-1]
	}
	if c.InjectedJavaScriptBeforeContent != "" && !c.NoJavaScript {
		prepend = append(prepend, newScriptNode(c.InjectedJavaScriptBeforeContent))
	}
	first := headEl.FirstChild
	for _, node := range prepend {
		headEl.InsertBefore(node, first)
	}
	if c.InjectedJavaScript != "" && !c.NoJavaScript {
		bodyEl.AppendChild(newScriptNode(c.InjectedJavaScript))
	}

	var buf bytes.Buffer
	_ = html.Render(&buf, doc)
	return buf.String(), title, refusedMeta
}

func sanitizeStaticLink(el *html.Node, effectiveBase, docBaseTarget string, c *driver.WebConfig) {
	target := docBaseTarget
	if attr, ok := htmlAttr(el, "target"); ok {
		target = strings.ToLower(strings.TrimSpace(attr))
	}
	if target != "" && target != "_self" {
		removeHTMLAttr(el, "target")
		if target == "_top" || target == "_parent" {
			removeHTMLAttr(el, "href")
			removeHTMLAttr(el, "xlink:href")
			return
		}
	}
	for _, key := range []string{"href", "xlink:href"} {
		raw, ok := htmlAttr(el, key)
		if !ok {
			continue
		}
		trimmed := strings.TrimSpace(raw)
		if isSameDocFragment(trimmed) {
			continue
		}
		resolved := resolveHTMLURL(trimmed, effectiveBase)
		if !c.Navigable(resolved) {
			removeHTMLAttr(el, key)
		}
	}
}

func sanitizeStaticForm(el *html.Node, effectiveBase, docBaseTarget string, c *driver.WebConfig) {
	target := docBaseTarget
	if attr, ok := htmlAttr(el, "target"); ok {
		target = strings.ToLower(strings.TrimSpace(attr))
	}
	if target != "" && target != "_self" {
		removeHTMLAttr(el, "target")
	}
	raw, _ := htmlAttr(el, "action")
	resolved := resolveHTMLURL(strings.TrimSpace(raw), effectiveBase)
	if !c.Navigable(resolved) {
		removeHTMLAttr(el, "action")
	}
}

func sanitizeStaticFormButton(el *html.Node, effectiveBase, docBaseTarget string, c *driver.WebConfig) {
	target := docBaseTarget
	if attr, ok := htmlAttr(el, "formtarget"); ok {
		target = strings.ToLower(strings.TrimSpace(attr))
	}
	if target != "" && target != "_self" {
		removeHTMLAttr(el, "formtarget")
	}
	if raw, ok := htmlAttr(el, "formaction"); ok {
		resolved := resolveHTMLURL(strings.TrimSpace(raw), effectiveBase)
		if !c.Navigable(resolved) {
			removeHTMLAttr(el, "formaction")
		}
	}
}

func ensureHTMLStructure(doc *html.Node) (htmlEl, headEl, bodyEl *html.Node) {
	for ch := doc.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode && ch.DataAtom == atom.Html {
			htmlEl = ch
			break
		}
	}
	if htmlEl == nil {
		htmlEl = &html.Node{Type: html.ElementNode, DataAtom: atom.Html, Data: "html"}
		doc.AppendChild(htmlEl)
	}
	for ch := htmlEl.FirstChild; ch != nil; ch = ch.NextSibling {
		if ch.Type == html.ElementNode {
			switch ch.DataAtom {
			case atom.Head:
				if headEl == nil {
					headEl = ch
				}
			case atom.Body:
				if bodyEl == nil {
					bodyEl = ch
				}
			}
		}
	}
	if headEl == nil {
		headEl = &html.Node{Type: html.ElementNode, DataAtom: atom.Head, Data: "head"}
		htmlEl.InsertBefore(headEl, htmlEl.FirstChild)
	}
	if bodyEl == nil {
		bodyEl = &html.Node{Type: html.ElementNode, DataAtom: atom.Body, Data: "body"}
		htmlEl.AppendChild(bodyEl)
	}
	return htmlEl, headEl, bodyEl
}

func newScriptNode(code string) *html.Node {
	safe := strings.ReplaceAll(code, "</script", `<\/script`)
	el := &html.Node{Type: html.ElementNode, DataAtom: atom.Script, Data: "script"}
	el.AppendChild(&html.Node{Type: html.TextNode, Data: safe})
	return el
}

func extractHTMLTitle(doc *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.DataAtom == atom.Title {
			var b strings.Builder
			for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
				if ch.Type == html.TextNode {
					b.WriteString(ch.Data)
				}
			}
			title = strings.TrimSpace(b.String())
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(doc)
	return title
}

func htmlAttr(n *html.Node, name string) (string, bool) {
	for _, a := range n.Attr {
		key := a.Key
		if a.Namespace != "" {
			key = a.Namespace + ":" + a.Key
		}
		if strings.EqualFold(key, name) {
			return a.Val, true
		}
	}
	return "", false
}

func removeHTMLAttr(n *html.Node, name string) {
	out := n.Attr[:0]
	for _, a := range n.Attr {
		key := a.Key
		if a.Namespace != "" {
			key = a.Namespace + ":" + a.Key
		}
		if strings.EqualFold(key, name) {
			continue
		}
		out = append(out, a)
	}
	n.Attr = out
}

func isSameDocFragment(raw string) bool {
	return strings.HasPrefix(strings.TrimSpace(raw), "#")
}

func resolveHTMLURL(raw, base string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if base != "" {
			return base
		}
		return "about:srcdoc"
	}
	ref, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if ref.Scheme != "" {
		return ref.String()
	}
	if base != "" {
		if b, err := url.Parse(base); err == nil {
			return b.ResolveReference(ref).String()
		}
	}
	if strings.HasPrefix(raw, "#") {
		return "about:srcdoc" + raw
	}
	return raw
}

// parseMetaRefresh extracts the timeout in milliseconds and destination URL
// from a <meta http-equiv="refresh"> content attribute following the HTML
// Living Standard refresh parser.
func parseMetaRefresh(content string) (delayMS int, dest string, ok bool) {
	s := strings.TrimSpace(content)
	if s == "" {
		return 0, "", false
	}
	// Parse leading timeout digits (and optional decimal part).
	i := 0
	for i < len(s) && ((s[i] >= '0' && s[i] <= '9') || s[i] == '.') {
		i++
	}
	if i > 0 {
		if sec, err := strconv.ParseFloat(s[:i], 64); err == nil && sec > 0 {
			delayMS = int(sec * 1000)
		}
	}
	// Skip optional characters until delimiter ';' or ',' or whitespace.
	for i < len(s) && s[i] != ';' && s[i] != ',' && s[i] != ' ' && s[i] != '\t' && s[i] != '\r' && s[i] != '\n' {
		i++
	}
	s = strings.TrimSpace(s[i:])
	if s == "" {
		return 0, "", false
	}
	if s[0] == ';' || s[0] == ',' {
		s = strings.TrimSpace(s[1:])
	}
	if s == "" {
		return 0, "", false
	}
	// Optional "url=" prefix (case-insensitive, with optional spaces before '=').
	if len(s) >= 3 && strings.EqualFold(s[:3], "url") {
		rest := strings.TrimSpace(s[3:])
		if strings.HasPrefix(rest, "=") {
			s = strings.TrimSpace(rest[1:])
		}
	}
	if s == "" {
		return 0, "", false
	}
	if q := s[0]; q == '\'' || q == '"' {
		s = s[1:]
		if end := strings.IndexByte(s, q); end >= 0 {
			s = s[:end]
		}
	}
	s = strings.TrimSpace(s)
	return delayMS, s, s != ""
}
