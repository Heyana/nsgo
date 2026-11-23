// Package menuspeech holds what the desktop backends' Speech menus share to read a focused web view.
package menuspeech

// WebSelection is the expression a backend evaluates in a focused page for Start Speaking to read, which is always a string. It reads the focused text control as a native field is read, its selection or its whole value without one, and else the document's selection, following focus into same-origin frames. A password field, a control with no text selection, and a frame of another origin read as nothing.
const WebSelection = `String((function () {
  var page = window;
  for (;;) {
    var el = page.document.activeElement;
    if (el && (el.tagName === "IFRAME" || el.tagName === "FRAME")) {
      var child;
      try { child = el.contentWindow; if (!child || child === page || !child.document) return ""; } catch (_) { return ""; }
      page = child;
      continue;
    }
    if (el && (el.tagName === "INPUT" || el.tagName === "TEXTAREA")) {
      if (el.type === "password") return "";
      var start, end;
      try { start = el.selectionStart; end = el.selectionEnd; } catch (_) { return ""; }
      if (typeof start !== "number" || typeof end !== "number") return "";
      var value = String(el.value);
      return start === end ? value : value.slice(Math.min(start, end), Math.max(start, end));
    }
    var selection = page.getSelection();
    return selection ? selection.toString() : "";
  }
})())`
