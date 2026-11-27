// The app's page: starts the app (nsgo/runtime.js), which keeps root and
// ov up to date, and hands it the nsgo events of the views.
const { start } = require('../../nsgo/runtime.js')

Page({
  data: { root: null, ov: [], back: false },

  onLoad() {
    start(this)
      .then((app) => {
        this.app = app
      })
      .catch((error) => console.error('nsgo: the app did not start', error))
  },

  onUnload() {
    if (this.app) this.app.stop()
  },

  // An nsgo event: WeChat, QQ and Douyin's component event, whose detail
  // it is, or what Alipay's component hands its onNsgo prop.
  onNsgo(e) {
    if (this.app) this.app.event(...(Array.isArray(e) ? e : e.detail))
  },

  // The page container left with the back: the app goes back, and shows
  // it again while it has more to go back through.
  onBack() {
    if (this.data && !this.data.back) return
    this.setData({ back: false })
    if (this.app) this.app.event('back', 0)
  },

  // Alipay page events: intercept navbar/hardware back when page-container
  // does not take it (or is unavailable), and forward soft keyboard height
  // changes to the environment.
  events: {
    onBack(e) {
      if (!this.data || !this.data.back) return
      if (e && typeof e.preventDefault === 'function') e.preventDefault()
      this.onBack()
    },
    onKeyboardHeight(e) {
      const h = e && (e.height != null ? e.height : e.detail && e.detail.height)
      const d = e && (e.duration != null ? e.duration : e.detail && e.detail.duration)
      if (this.app) this.app.event('keyboard', 0, Math.max(0, Number(h) || 0), Number(d) || 0.25)
    },
  },
})
