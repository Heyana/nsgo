// Text-to-speech in mini programs: core wx, qq, tt and consumer my APIs
// provide no built-in speech synthesis. On WeChat, an app that enables
// WeChat's official WechatSI plugin (wx069ba97219f66d99) in the WeChat MP
// console and sets miniprogram.wechat_speech_plugin in nsgo.yaml can speak
// zh-CN and en-US text through plugin.textToSpeech and InnerAudioContext.
// Everywhere else (unconfigured WeChat, QQ, Alipay, Douyin), available is
// false and requests answer synthesis-unavailable.

'use strict'

const WECHAT_VOICES = [
  { id: 'WechatSI:zh_CN', name: 'WechatSI (zh-CN)', lang: 'zh-CN' },
  { id: 'WechatSI:en_US', name: 'WechatSI (en-US)', lang: 'en-US' },
]

function loadWeChatSpeechPlugin(pluginName, loader) {
  if (!pluginName) return null
  /* global requirePlugin */
  const fn = typeof loader === 'function' ? loader : typeof requirePlugin === 'function' ? requirePlugin : null
  if (!fn) return null
  try {
    const plugin = fn(pluginName)
    return plugin && typeof plugin.textToSpeech === 'function' ? plugin : null
  } catch (_) {
    return null
  }
}

function resolveWeChatLang(lang, voice) {
  if (voice) {
    if (voice === 'WechatSI:zh_CN') return { lang: 'zh_CN', error: '' }
    if (voice === 'WechatSI:en_US') return { lang: 'en_US', error: '' }
    return { lang: '', error: 'voice-unavailable' }
  }
  const tag = String(lang || '')
    .trim()
    .replace(/_/g, '-')
    .toLowerCase()
  if (!tag || tag === 'zh' || tag === 'zh-cn' || tag === 'zh-hans' || tag === 'zh-hans-cn') {
    return { lang: 'zh_CN', error: '' }
  }
  if (tag === 'en' || tag === 'en-us') {
    return { lang: 'en_US', error: '' }
  }
  return { lang: '', error: 'language-unavailable' }
}

function mapPluginError(res) {
  const code = res && res.retcode !== undefined ? Number(res.retcode) : NaN
  if (code === -20001) return 'language-unavailable'
  if (code === -20002 && res && typeof res.msg === 'string' && /len|long|limit|超长|长度/i.test(res.msg)) {
    return 'text-too-long'
  }
  return 'synthesis-failed'
}

function destroyAudio(ctx, stopFirst) {
  if (!ctx) return
  try {
    if (stopFirst && typeof ctx.stop === 'function') ctx.stop()
  } catch (_) {
    // Ignore host errors during teardown.
  }
  try {
    if (typeof ctx.destroy === 'function') ctx.destroy()
  } catch (_) {
    // Ignore host errors during teardown.
  }
}

function createSpeaker(platform, api, options = {}) {
  const plugin =
    platform === 'wechat' && api && typeof api.createInnerAudioContext === 'function'
      ? loadWeChatSpeechPlugin(options.pluginName, options.requirePlugin)
      : null
  const available = plugin !== null
  let active = null

  const finish = (state, stopped, error) => {
    if (!state || state.ended) return
    state.ended = true
    const ctx = state.audio
    state.audio = null
    if (active === state) active = null
    destroyAudio(ctx, stopped)
    if (typeof state.complete === 'function') {
      state.complete({ stopped: !!stopped && !error, error: error || '' })
    }
  }

  const stop = (id) => {
    if (!active) return
    if (id != null && active.id !== id) return
    finish(active, true, '')
  }

  const start = (req, complete) => {
    stop(null)
    if (!available) {
      if (typeof complete === 'function') complete({ stopped: false, error: 'synthesis-unavailable' })
      return
    }
    const resolved = resolveWeChatLang(req && req.lang, req && req.voice)
    if (resolved.error) {
      if (typeof complete === 'function') complete({ stopped: false, error: resolved.error })
      return
    }
    const state = { id: req && req.id, audio: null, ended: false, complete }
    active = state
    const text = String((req && req.text) || '')
    const rawRate = req && typeof req.rate === 'number' && isFinite(req.rate) && req.rate > 0 ? req.rate : 1
    const rate = Math.min(2, Math.max(0.5, rawRate))
    try {
      plugin.textToSpeech({
        lang: resolved.lang,
        tts: true,
        content: text,
        success: (res) => {
          if (state.ended || active !== state) return
          if (!res || (res.retcode !== undefined && Number(res.retcode) !== 0)) {
            finish(state, false, mapPluginError(res))
            return
          }
          if (typeof res.filename !== 'string' || res.filename.length === 0) {
            finish(state, false, 'synthesis-failed')
            return
          }
          let ctx
          try {
            ctx = api.createInnerAudioContext()
            if (!ctx || typeof ctx.play !== 'function') {
              finish(state, false, 'synthesis-failed')
              return
            }
            state.audio = ctx
            if (typeof ctx.onEnded === 'function') ctx.onEnded(() => finish(state, false, ''))
            if (typeof ctx.onStop === 'function') ctx.onStop(() => finish(state, true, ''))
            if (typeof ctx.onError === 'function') ctx.onError(() => finish(state, false, 'synthesis-failed'))
            ctx.src = res.filename
            ctx.playbackRate = rate
            ctx.play()
          } catch (_) {
            finish(state, false, 'synthesis-failed')
          }
        },
        fail: (res) => {
          if (state.ended || active !== state) return
          finish(state, false, mapPluginError(res))
        },
      })
    } catch (_) {
      finish(state, false, 'synthesis-failed')
    }
  }

  return {
    available,
    voices: () => (available ? WECHAT_VOICES.map((v) => Object.assign({}, v)) : []),
    start,
    stop,
    close: () => stop(null),
  }
}

module.exports = { createSpeaker, resolveWeChatLang, WECHAT_VOICES }
