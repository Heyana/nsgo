// Styles of a view, a line of its text and its field, at their frames.
export default {
  box: function (n, ka) {
    return 'left:' + n.f[0] + 'px;top:' + n.f[1] + 'px;width:' + n.f[2] + 'px;height:' + n.f[3] + 'px;' +
      n.s + n.a + (n.tr ? 'transition:' + n.tr + ';' : '') + (ka || '')
  },
  run: function (t, r) {
    return 'left:' + (t.x + r[1]) + 'px;top:' + (t.y + r[2]) + 'px;line-height:' + r[3] + 'px;' + t.s[r[4]] +
      (t.sh ? 'text-shadow:' + t.sh + ';' : '')
  },
  background: function (t, b) {
    return 'left:' + (t.x + b[0]) + 'px;top:' + (t.y + b[1]) + 'px;width:' + b[2] + 'px;height:' + b[3] +
      'px;background-color:' + b[4] + ';border-radius:' + b[5] + 'px;'
  },
  field: function (n) {
    var i = n.fd
    return 'left:' + i.x + 'px;top:' + i.y + 'px;width:' + i.w + 'px;height:' + i.h + 'px;' + i.css
  },
  selRect: function (hl, r) {
    return 'left:' + r[0] + 'px;top:' + r[1] + 'px;width:' + r[2] + 'px;height:' + r[3] + 'px;background-color:' + hl.c + ';'
  },
  selHandle: function (h) {
    return 'left:' + (h[0] - 14) + 'px;top:' + (h[1] - 14) + 'px;width:28px;height:' + (h[2] + 28) + 'px;'
  },
  selBar: function (sh, h) {
    return 'height:' + h[2] + 'px;background-color:' + sh.c + ';'
  },
  selKnob: function (sh) {
    return 'background-color:' + sh.c + ';'
  },
}
