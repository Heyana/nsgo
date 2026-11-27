// One view of the app at the frame NSGo laid it out at, as WeChat's
// nsgo-node is, for Alipay: what the user does with it goes to the page
// through the onNsgo prop each parent hands its children, [name, id,
// ...]. Every view binds every handler, as Alipay names them statically;
// an event bubbling up is kept by the innermost view that takes it.
// Keyframes run through render-layer CSS transitions stepped across the
// keyframe offsets, and scroll views report drags once touch movement or
// scrolling starts along their scroll axis.

// The phases of a touch event, as Go numbers them.
const video = require('../../nsgo/video.js')
const PHASES = { touchstart: 0, touchmove: 1, touchend: 2, touchcancel: 3 }
const DRAG_SLOP = 4

const fingers = (list) =>
  Array.prototype.map.call(list || [], (t) => [
    t.identifier,
    t.pageX != null ? t.pageX : t.clientX,
    t.pageY != null ? t.pageY : t.clientY,
  ])

// The time of the event of each kind a view kept last, so the views it
// bubbles through further out leave it.
const kept = {}

const keep = (kind, e) => {
  if (kept[kind] === e.timeStamp) return false
  kept[kind] = e.timeStamp
  return true
}

const point = (e) => {
  const d = e.detail || {}
  const t = (e.changedTouches && e.changedTouches[0]) || (e.touches && e.touches[0]) || {}
  return [d.pageX != null ? d.pageX : d.x != null ? d.x : t.pageX, d.pageY != null ? d.pageY : d.y != null ? d.y : t.pageY]
}

const touchPoint = (e) => {
  const t = (e && e.touches && e.touches[0]) || (e && e.changedTouches && e.changedTouches[0])
  if (!t) return null
  const x = t.pageX != null ? t.pageX : t.clientX
  const y = t.pageY != null ? t.pageY : t.clientY
  return x != null && y != null ? [x, y] : null
}

function frameStyle(f) {
  let css = ''
  if (f.opacity != null) css += 'opacity:' + f.opacity + ';'
  const tf = []
  if (f.translateY != null) {
    tf.push('translateY(' + (typeof f.translateY === 'number' ? f.translateY + 'px' : f.translateY) + ')')
  }
  if (f.rotate != null) tf.push('rotate(' + f.rotate + 'deg)')
  if (f.scale != null) {
    tf.push(Array.isArray(f.scale) ? 'scale(' + f.scale[0] + ',' + f.scale[1] + ')' : 'scale(' + f.scale + ',' + f.scale + ')')
  }
  if (tf.length > 0) css += 'transform:' + tf.join(' ') + ';'
  return css
}

Component({
  data: { kaStyle: '' },
  props: { node: null, onNsgo: () => {} },
  didMount() {
    const n = this.props.node
    if (n && n.ka) this.keyframes(n.ka)
    if (n && n.vc) this.videoCommand(n.vc)
  },
  didUpdate() {
    const n = this.props.node
    this.keyframes(n ? n.ka : null)
    if (n && n.vc) this.videoCommand(n.vc)
  },
  didUnmount() {
    this.stopKeyframes(false)
    this.touchDown = false
    this.dragOrigin = null
    this.dragging = false
    if (this.scrollTimer) clearTimeout(this.scrollTimer)
    if (this.keyboardHeight > 0) {
      this.keyboardHeight = 0
      this.send('keyboard', 0, 0, 0.25)
    }
  },
  methods: {
    send(...detail) {
      this.props.onNsgo(detail)
    },
    // A child's event, on its way to the page.
    onNsgo(detail) {
      this.props.onNsgo(detail)
    },
    id() {
      return this.props.node.id
    },
    onTap(e) {
      if (this.props.node.p && keep('tap', e)) this.send('tap', this.id(), ...point(e))
    },
    onLongPress(e) {
      if (this.props.node.lp && keep('longpress', e)) this.send('longpress', this.id(), ...point(e))
    },
    onTouch(e) {
      const n = this.props.node
      const phase = PHASES[String(e.type).toLowerCase()]
      if (n.io && (phase === 0 || phase >= 2) && keep('press' + phase, e)) {
        this.send(phase === 0 ? 'pressin' : 'pressout', this.id(), ...point(e))
      }
      if (!n.tf) return
      // The views the event bubbles through send it again: Go keeps the
      // first, the innermost, by the event's time and phase.
      this.send('touch', this.id(), Math.round(e.timeStamp * 4) + phase, phase,
        fingers(e.changedTouches), fingers(e.touches), e.timeStamp)
    },
    onScroll(e) {
      const id = this.id()
      const n = this.props.node
      if (this.touchDown && !this.dragging && !(n && n.sc && n.sc.off)) {
        this.dragging = true
        this.send('dragstart', id)
      }
      this.send('scroll', id, e.detail.scrollLeft, e.detail.scrollTop)
      // Scroll views report no end: one comes 150 ms after the last move.
      if (this.scrollTimer) clearTimeout(this.scrollTimer)
      this.scrollTimer = setTimeout(() => {
        this.scrollTimer = null
        this.send('scrollend', id)
      }, 150)
    },
    // Alipay's scroll views have no enhanced dragstart/dragend events:
    // track touch down/move/end against the scroll axis and onScroll so
    // taps, disabled scroll views and cross-axis drags do not report drags.
    onScrollTouchStart(e) {
      const n = this.props.node
      if (!(n && n.sc && n.sc.off) && !this.touchDown) {
        this.touchDown = true
        this.dragging = false
        this.dragOrigin = touchPoint(e)
      }
      this.onTouch(e)
    },
    onScrollTouchMove(e) {
      const n = this.props.node
      if (this.dragging) {
        keep('scrollDrag', e)
      } else if (this.touchDown && !(n && n.sc && n.sc.off)) {
        const pt = touchPoint(e)
        if (!this.dragOrigin) this.dragOrigin = pt
        if (pt && this.dragOrigin) {
          const dx = Math.abs(pt[0] - this.dragOrigin[0])
          const dy = Math.abs(pt[1] - this.dragOrigin[1])
          const horiz = !!(n && n.sc && n.sc.h)
          const along = horiz ? dx >= DRAG_SLOP && dx >= dy : dy >= DRAG_SLOP && dy >= dx
          if (along && keep('scrollDrag', e)) {
            this.dragging = true
            this.send('dragstart', this.id())
          }
        }
      }
      this.onTouch(e)
    },
    onScrollTouchEnd(e) {
      const cancelled = String(e && e.type).toLowerCase() === 'touchcancel'
      const remaining = e && e.touches ? e.touches.length : 0
      if (cancelled || remaining === 0) {
        this.touchDown = false
        this.dragOrigin = null
        if (this.dragging) {
          this.dragging = false
          this.send('dragend', this.id())
        }
      }
      this.onTouch(e)
    },
    onRefresh() {
      this.send('refresh', this.id())
    },
    // The template disables a read-only field, as Alipay's onInput cannot put the value back and node is a prop the component does not own; should an input arrive anyway, Go does not hear it.
    onInput(e) {
      const node = this.props.node
      if (node && node.fd && node.fd.ro) return
      this.send('input', this.id(), e.detail.value)
    },
    onConfirm(e) {
      this.send('confirm', this.id(), e.detail.value)
    },
    onFocus(e) {
      this.send('focus', this.id())
      const d = e && e.detail
      if (d && d.height > 0) {
        this.keyboardHeight = d.height
        this.send('keyboard', 0, d.height, d.duration || 0.25)
      }
    },
    onBlur() {
      this.send('blur', this.id())
      if (this.keyboardHeight > 0) {
        this.keyboardHeight = 0
        this.send('keyboard', 0, 0, 0.25)
      }
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
      const [x, y] = point(e)
      const h = Number((e.currentTarget && e.currentTarget.dataset && e.currentTarget.dataset.h) || 0)
      this.send('selhandle', this.id(), h, phase, x || 0, y || 0)
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
    setKaStyle(style) {
      if (!style && !this.kaStyled) return
      this.kaStyled = !!style
      this.setData({ kaStyle: style })
    },
    stopKeyframes(clear) {
      this.running = null
      this.kaKey = ''
      if (this.kaTimer) {
        clearTimeout(this.kaTimer)
        this.kaTimer = null
      }
      if (clear) this.setKaStyle('')
    },
    // A keyframe animation (op ka): {d, n, delay, f}, stepped across its
    // offsets as render-layer CSS transitions and started again at each
    // iteration's end.
    keyframes(ka) {
      const key = ka ? JSON.stringify(ka) : ''
      if (key === (this.kaKey || '')) return
      this.stopKeyframes(!ka)
      if (!ka || !Array.isArray(ka.f) || ka.f.length === 0) return
      this.kaKey = key
      this.running = ka
      const frames = ka.f
      const duration = Number(ka.d) || 0
      let left = ka.n || Infinity
      const step = (i) => {
        if (this.running !== ka) return
        if (i >= frames.length) {
          if (--left > 0 && this.running === ka) {
            run()
          } else if (ka.n === 1) {
            this.setKaStyle('')
          }
          return
        }
        const prev = frames[i - 1]
        const curr = frames[i]
        const o0 = prev.offset != null ? prev.offset : (i - 1) / (frames.length - 1)
        const o1 = curr.offset != null ? curr.offset : i / (frames.length - 1)
        const segMs = Math.max(0, Math.round((o1 - o0) * duration))
        const ease = prev.ease || 'linear'
        this.setKaStyle(
          frameStyle(curr) + 'transition:opacity ' + segMs + 'ms ' + ease + ',transform ' + segMs + 'ms ' + ease + ';'
        )
        this.kaTimer = setTimeout(() => {
          this.kaTimer = null
          step(i + 1)
        }, segMs)
      }
      const run = () => {
        if (this.running !== ka) return
        this.setKaStyle(frameStyle(frames[0]) + 'transition:none;')
        if (frames.length === 1) {
          this.kaTimer = setTimeout(() => {
            this.kaTimer = null
            step(1)
          }, Math.max(0, duration))
          return
        }
        this.kaTimer = setTimeout(() => {
          this.kaTimer = null
          step(1)
        }, 16)
      }
      if (ka.delay > 0) {
        this.kaTimer = setTimeout(() => {
          this.kaTimer = null
          run()
        }, ka.delay)
      } else {
        run()
      }
    },
  },
})
