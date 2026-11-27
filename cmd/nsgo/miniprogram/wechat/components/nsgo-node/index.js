// One view of the app at the frame NSGo laid it out at, with its style,
// what it draws (its text's runs, its picture, its image or field) and
// its children: this component again. What the user does with it goes to
// the page as nsgo events, [name, id, ...], which bubble across the
// components' boundaries (composed) to the page's one listener.

const pixels = require('../../nsgo/pixels.js')
const video = require('../../nsgo/video.js')

// The phases of a touch event, as Go numbers them.
const PHASES = { touchstart: 0, touchmove: 1, touchend: 2, touchcancel: 3 }

const fingers = (list) =>
  Array.prototype.map.call(list || [], (t) => [
    t.identifier,
    t.pageX != null ? t.pageX : t.clientX,
    t.pageY != null ? t.pageY : t.clientY,
  ])

Component({
  options: { addGlobalClass: false },
  properties: { node: Object },
  observers: {
    'node.ka': function (ka) {
      this.keyframes(ka)
    },
    'node.px': function () {
      if (this.pixelReady) this.bindPixels()
    },
    'node.vc': function (vc) {
      if (this.pixelReady) this.videoCommand(vc)
    },
  },
  lifetimes: {
    ready() {
      this.pixelReady = true
      this.bindPixels()
      const n = this.data.node
      if (n && n.vc) this.videoCommand(n.vc)
    },
    detached() {
      this.pixelReady = false
      this.unbindPixels()
      this.running = null
      if (this.scrollTimer) clearTimeout(this.scrollTimer)
    },
  },
  methods: {
    bindPixels() {
      const { id, px } = this.data.node || {}
      if (px && this.pixelBinding && this.pixelBinding.id === id) return
      this.unbindPixels()
      if (!px || !this.createSelectorQuery) return
      const binding = { id }
      this.pixelBinding = binding
      this.createSelectorQuery()
        .select('#pixels')
        .fields({ node: true })
        .exec(results => {
          if (this.pixelBinding !== binding) return
          const canvas = results && results[0] && results[0].node
          if (canvas && pixels.attach(id, canvas)) binding.canvas = canvas
          else this.pixelBinding = null
        })
    },
    unbindPixels() {
      const binding = this.pixelBinding
      if (binding && binding.canvas) pixels.detach(binding.id, binding.canvas)
      this.pixelBinding = null
    },
    send(...detail) {
      this.triggerEvent('nsgo', detail, { bubbles: true, composed: true })
    },
    id() {
      return this.data.node.id
    },
    onTap(e) {
      this.send('tap', this.id(), e.detail.x, e.detail.y)
    },
    onLongPress(e) {
      const t = (e.touches && e.touches[0]) || { pageX: e.detail.x, pageY: e.detail.y }
      this.send('longpress', this.id(), t.pageX, t.pageY)
    },
    onPressIn(e) {
      const t = e.touches[0]
      this.send('pressin', this.id(), t.pageX, t.pageY)
    },
    onPressOut(e) {
      const t = e.changedTouches[0]
      this.send('pressout', this.id(), t.pageX, t.pageY)
    },
    onTouch(e) {
      const phase = PHASES[e.type]
      // The views the event bubbles through send it again: Go keeps the
      // first, the innermost, by the event's time and phase.
      this.send('touch', this.id(), Math.round(e.timeStamp * 4) + phase, phase,
        fingers(e.changedTouches), fingers(e.touches), e.timeStamp)
    },
    onScroll(e) {
      const id = this.id()
      this.send('scroll', id, e.detail.scrollLeft, e.detail.scrollTop)
      // Scroll views report no end: one comes 150 ms after the last move.
      if (this.scrollTimer) clearTimeout(this.scrollTimer)
      this.scrollTimer = setTimeout(() => {
        this.scrollTimer = null
        this.send('scrollend', id)
      }, 150)
    },
    onDragStart() {
      this.send('dragstart', this.id())
    },
    onDragEnd() {
      this.send('dragend', this.id())
    },
    onRefresh() {
      this.send('refresh', this.id())
    },
    // A read-only field is an enabled input, so it focuses, selects and copies, and bindinput returns the value Go holds, which WeChat puts back in the input in place of the edit (a textarea ignores the return, so the template disables a read-only one instead). Go hears no input, and a confirm carries the held value.
    onInput(e) {
      const node = this.data.node
      if (node && node.fd && node.fd.ro) return node.v || ''
      this.send('input', this.id(), e.detail.value)
    },
    onConfirm(e) {
      const node = this.data.node
      this.send('confirm', this.id(), node && node.fd && node.fd.ro ? node.v || '' : e.detail.value)
    },
    onFocus() {
      this.send('focus', this.id())
    },
    onBlur() {
      this.send('blur', this.id())
    },
    onKeyboard(e) {
      this.send('keyboard', 0, e.detail.height || 0, e.detail.duration || 0.25)
    },
    onLoad(e) {
      this.send('imgload', this.id(), e.detail.width, e.detail.height)
    },
    onError(e) {
      this.send('imgerror', this.id(), e.detail.errMsg || 'the image did not load')
    },
    onSelHandle(e) {
      const phase = PHASES[String(e.type).toLowerCase()]
      if (phase == null) return
      const t = (e.changedTouches && e.changedTouches[0]) || (e.touches && e.touches[0]) || {}
      const x = t.pageX != null ? t.pageX : t.clientX || 0
      const y = t.pageY != null ? t.pageY : t.clientY || 0
      const h = Number((e.currentTarget && e.currentTarget.dataset && e.currentTarget.dataset.h) || 0)
      this.send('selhandle', this.id(), h, phase, x, y)
    },
    videoCommand(vc) {
      if (!Array.isArray(vc) || vc.length < 2 || vc[0] === this.lastVideoCmd) return
      this.lastVideoCmd = vc[0]
      video.run(this.id(), this, vc[1])
    },
    onVideoMeta(e) {
      const d = e.detail || {}
      this.send('vdmeta', this.id(), d.duration || 0, d.width || 0, d.height || 0)
    },
    onVideoPlay() {
      this.send('vdplay', this.id())
    },
    onVideoPause() {
      this.send('vdpause', this.id())
    },
    onVideoEnd() {
      this.send('vdend', this.id())
    },
    onVideoTime(e) {
      const d = e.detail || {}
      this.send('vdtime', this.id(), d.currentTime || 0, d.duration || 0)
    },
    onVideoWait() {
      this.send('vdwait', this.id())
    },
    onVideoError(e) {
      const d = e.detail || {}
      this.send('vderror', this.id(), d.errMsg || 'the video could not play')
    },
    // A keyframe animation (op ka): {d, n, delay, f}, its duration, its
    // iterations (0 for ever) and its frames, run in the render layer and
    // started again at each iteration's end.
    keyframes(ka) {
      this.running = ka
      if (!this.animate) return
      this.clearAnimation('#n', () => {
        if (!ka || this.running !== ka) return
        const frames = ka.f.map((f) => {
          const out = { offset: f.offset }
          if (f.ease) out.ease = f.ease
          if (f.opacity != null) out.opacity = f.opacity
          if (f.rotate != null) out.rotate = f.rotate
          if (f.scale != null) out.scale = Array.isArray(f.scale) ? f.scale : [f.scale, f.scale]
          if (f.translateY != null) out.translateY = f.translateY
          return out
        })
        let left = ka.n || Infinity
        const run = () => {
          if (this.running !== ka) return
          this.animate('#n', frames, ka.d, () => {
            if (--left > 0 && this.running === ka) this.clearAnimation('#n', run)
            else if (ka.n === 1) this.clearAnimation('#n')
          })
        }
        if (ka.delay > 0) setTimeout(run, ka.delay)
        else run()
      })
    },
  },
})
