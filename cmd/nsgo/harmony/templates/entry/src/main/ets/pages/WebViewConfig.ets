// Pure ArkTS helpers for WebView configuration, user agent composition,
// reload/recreation decisions, navigation restrictions, media permissions,
// and downloads. Kept free of @kit.* imports so host tests execute this exact
// source directly.

export interface WebHeaderPair {
  k: string;
  v: string;
}

// WebConfigOp mirrors harmonyWebConfig in internal/platform/harmonyos/web.go.
export interface WebConfigOp {
  src?: string;
  html?: string;
  base?: string;
  before?: string;
  after?: string;
  ua?: string;
  uas?: string;
  headers?: WebHeaderPair[];
  incognito?: boolean;
  noScroll?: boolean;
  noJS?: boolean;
  muted?: boolean;
  pinchZoom?: boolean;
  downloadDir?: string;
  restricted?: boolean;
  hosts?: string[];
  schemes?: string[];
  appSchemes?: string[];
  media?: number;
}

export interface WebLoadOp {
  src?: string;
  html?: string;
  base?: string;
}

export interface WebMediaRequest {
  want: number;
  resources: string[];
  perms: string[];
}

export const WEB_MEDIA_MICROPHONE: number = 1;
export const WEB_MEDIA_CAMERA: number = 2;

export function parseWebConfigOp(raw: string): WebConfigOp {
  if (raw === '') {
    return {};
  }
  try {
    return JSON.parse(raw) as WebConfigOp;
  } catch (_e) {
    return {};
  }
}

export function parseWebLoadOp(raw: string): WebLoadOp {
  if (raw === '') {
    return {};
  }
  try {
    return JSON.parse(raw) as WebLoadOp;
  } catch (_e) {
    return {};
  }
}

// resolveWebUserAgent appends suffix to customUA (or defaultUA when customUA is
// empty) with one separating space, matching driver.WebUserAgent without
// duplicating the suffix across live config updates.
export function resolveWebUserAgent(defaultUA: string, customUA: string, suffix: string): string {
  const base = customUA !== '' ? customUA : defaultUA;
  if (suffix === '') {
    return base;
  }
  if (base === '') {
    return suffix;
  }
  return base + ' ' + suffix;
}

// webHooksKey captures the ArkWeb settings that require recreating the Web
// component: JavaScript access and document-start/document-end scripts.
export function webHooksKey(noJS: boolean, before: string, after: string): string {
  return JSON.stringify([noJS, before, after]);
}

// webReloadKey captures the settings whose change reloads the configured source
// in place: source URL/HTML/base URL and initial request headers. Unrelated
// settings (muted, scroll, pinch zoom, user agent, media, downloads, policy)
// apply to the live view without reloading.
export function webReloadKey(src: string, html: string, baseURL: string, headers: WebHeaderPair[]): string {
  return JSON.stringify([src, html, baseURL, headers]);
}

// webBridgeScript defines window.nsgo.postMessage in the top frame only,
// carrying a per-WebView token so subframes and stale views cannot spoof posts.
export function webBridgeScript(token: string): string {
  return 'if (window === window.top) {\n' +
    '(function(b, t) {\n' +
    '  if (!b || !b.post) return;\n' +
    '  window.nsgo = { postMessage: function(data) { b.post(t, String(data)); } };\n' +
    '})(window.__nsgoWebBridge, ' + JSON.stringify(token) + ');\n' +
    '}';
}

// webDocumentStartScripts builds the main-frame scripts run before content:
// the window.nsgo bridge first, followed by InjectedJavaScriptBeforeContent.
export function webDocumentStartScripts(noJS: boolean, token: string, before: string): string[] {
  if (noJS) {
    return [];
  }
  const out: string[] = [webBridgeScript(token)];
  if (before !== '') {
    out.push('if (window === window.top) {\n' + before + '\n}');
  }
  return out;
}

// webDocumentEndScripts builds the main-frame script run after content.
export function webDocumentEndScripts(noJS: boolean, after: string): string[] {
  if (noJS || after === '') {
    return [];
  }
  return ['if (window === window.top) {\n' + after + '\n}'];
}

// webSourceNeedsFileAccess reports whether the source itself is a file: URL.
export function webSourceNeedsFileAccess(src: string, html: string, baseURL: string): boolean {
  if (html !== '') {
    return baseURL.toLowerCase().startsWith('file:');
  }
  return src.toLowerCase().startsWith('file:');
}

// schemeOf returns the lowercase RFC 3986 scheme of rawURL, or '' when invalid.
function schemeOf(rawURL: string): string {
  const colon = rawURL.indexOf(':');
  if (colon <= 0) {
    return '';
  }
  for (let i = 0; i < colon; i++) {
    const c = rawURL.charCodeAt(i);
    const alpha = (c >= 65 && c <= 90) || (c >= 97 && c <= 122);
    const digit = c >= 48 && c <= 57;
    const extra = c === 43 || c === 45 || c === 46; // + - .
    if (i === 0 ? !alpha : !(alpha || digit || extra)) {
      return '';
    }
  }
  return rawURL.slice(0, colon).toLowerCase();
}

// hostnameOf extracts the lowercase hostname from an http(s) URL without port
// or trailing dot, matching Go's url.URL.Hostname().
function hostnameOf(rawURL: string, schemeLen: number): string {
  if (rawURL.slice(schemeLen, schemeLen + 3) !== '://') {
    return '';
  }
  let rest = rawURL.slice(schemeLen + 3);
  let end = rest.length;
  for (let i = 0; i < rest.length; i++) {
    const ch = rest.charAt(i);
    if (ch === '/' || ch === '?' || ch === '#') {
      end = i;
      break;
    }
  }
  let authority = rest.slice(0, end);
  const at = authority.lastIndexOf('@');
  if (at >= 0) {
    authority = authority.slice(at + 1);
  }
  let host = authority;
  if (authority.startsWith('[')) {
    const close = authority.indexOf(']');
    if (close < 0) {
      return '';
    }
    host = authority.slice(1, close);
  } else {
    const port = authority.indexOf(':');
    if (port >= 0) {
      host = authority.slice(0, port);
    }
  }
  host = host.toLowerCase();
  if (host.endsWith('.')) {
    host = host.slice(0, host.length - 1);
  }
  return host;
}

// allowsHost matches driver.AllowsHost: '*', '*.example.com', or exact host.
function allowsHost(patterns: string[], host: string): boolean {
  if (host === '') {
    return false;
  }
  for (let i = 0; i < patterns.length; i++) {
    const p = patterns[i].toLowerCase();
    if (p === '*') {
      return true;
    }
    if (p.startsWith('*.')) {
      const suffix = p.slice(1); // ".example.com"
      if (host.endsWith(suffix) && host.length > suffix.length) {
        return true;
      }
    } else if (p === host) {
      return true;
    }
  }
  return false;
}

// webNavigable mirrors driver.WebConfig.Navigable (source=false) and
// driver.WebConfig.SourceNavigable (source=true).
export function webNavigable(
  restricted: boolean,
  hosts: string[],
  schemes: string[],
  appSchemes: string[],
  rawURL: string,
  source: boolean
): boolean {
  if (!restricted) {
    return true;
  }
  const scheme = schemeOf(rawURL);
  if (scheme === '') {
    return false;
  }
  if (scheme === 'about' || scheme === 'data' || scheme === 'blob') {
    return source || schemes.indexOf(scheme) >= 0;
  }
  if (scheme === 'http' || scheme === 'https') {
    return allowsHost(hosts, hostnameOf(rawURL, scheme.length));
  }
  return appSchemes.indexOf(scheme) >= 0;
}

// refusedNavigationMessage matches driver.RefusedNavigation.
export function refusedNavigationMessage(url: string): string {
  return 'nsgo: the web view may not navigate to ' + url + ' (the webview capability\'s hosts or schemes)';
}

// webMediaWant maps ArkWeb ProtectedResourceType strings to driver.WebMedia
// bits and OpenHarmony permission names.
export function webMediaWant(requested: string[]): WebMediaRequest {
  let want = 0;
  const resources: string[] = [];
  const perms: string[] = [];
  for (let i = 0; i < requested.length; i++) {
    const r = requested[i];
    if (r === 'TYPE_AUDIO_CAPTURE') {
      want |= WEB_MEDIA_MICROPHONE;
      resources.push(r);
      if (perms.indexOf('ohos.permission.MICROPHONE') < 0) {
        perms.push('ohos.permission.MICROPHONE');
      }
    } else if (r === 'TYPE_VIDEO_CAPTURE') {
      want |= WEB_MEDIA_CAMERA;
      resources.push(r);
      if (perms.indexOf('ohos.permission.CAMERA') < 0) {
        perms.push('ohos.permission.CAMERA');
      }
    }
  }
  return { want: want, resources: resources, perms: perms };
}

// sanitizeDownloadName keeps a suggested filename inside its download directory.
export function sanitizeDownloadName(raw: string): string {
  if (raw === '') {
    return 'download';
  }
  const slash = Math.max(raw.lastIndexOf('/'), raw.lastIndexOf('\\'));
  const name = slash >= 0 ? raw.slice(slash + 1) : raw;
  if (name === '' || name === '.' || name === '..') {
    return 'download';
  }
  return name;
}

// normalizeReportedWebURL maps ArkWeb's internal data: URL for an HTML source
// without a BaseURL to about:blank, matching driver.WebSource.Shows.
export function normalizeReportedWebURL(url: string, loadedHTMLNoBase: boolean): string {
  if (loadedHTMLNoBase && url.toLowerCase().startsWith('data:')) {
    return 'about:blank';
  }
  return url;
}

// webShouldReportError filters out subframe errors and cancelled loads
// (WebNetErrorList.ERR_ABORTED = -3).
export function webShouldReportError(isMainFrame: boolean, errorCode: number): boolean {
  return isMainFrame && errorCode !== -3;
}
