// Video playback control for <video> elements across WeChat, QQ, Douyin and
// Alipay via each platform's createVideoContext API (normalized by api.js).
'use strict'

const config = require('./config.js')
const { globalOf, apiOf } = require('./api.js')

let cachedAPI = null

function resolveAPI(api) {
  if (api) return api
  if (!cachedAPI) {
    const platform = config.platform || 'wechat'
    const base = globalOf(platform)
    if (base) cachedAPI = apiOf(platform, base)
  }
  return cachedAPI
}

/** Runs queued VideoContext commands [['play'], ['pause'], ['seek', s], ['rate', r]] for view id. */
function run(id, component, cmds, api) {
  if (!Array.isArray(cmds) || cmds.length === 0) return false
  const a = resolveAPI(api)
  if (!a || typeof a.createVideoContext !== 'function') return false
  const ctx = a.createVideoContext('nsgo-vd-' + id, component)
  if (!ctx) return false
  for (let i = 0; i < cmds.length; i++) {
    const cmd = cmds[i]
    switch (cmd[0]) {
      case 'play':
        if (typeof ctx.play === 'function') ctx.play()
        break
      case 'pause':
        if (typeof ctx.pause === 'function') ctx.pause()
        break
      case 'seek':
        if (typeof ctx.seek === 'function') ctx.seek(cmd[1])
        break
      case 'rate':
        if (typeof ctx.playbackRate === 'function') ctx.playbackRate(cmd[1])
        break
    }
  }
  return true
}

module.exports = { run }
