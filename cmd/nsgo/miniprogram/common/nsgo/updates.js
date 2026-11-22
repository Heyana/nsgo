'use strict'

// The native update manager is created before WebAssembly starts. Mini
// program hosts may emit an event during startup, so state is retained here
// rather than forwarded through Go's bridge as a one-shot notification.
function updateManager(api) {
  const state = {
    supported: false,
    checked: false,
    available: false,
    ready: false,
    error: '',
    checkedAt: 0,
    changed: null,
    apply() { return false },
  }
  const callbacks = []
  const notify = () => callbacks.slice().forEach((f) => { try { f(state) } catch (_) {} })
  const onChanged = (f) => {
    if (typeof f !== 'function') return () => {}
    callbacks.push(f)
    return () => {
      const i = callbacks.indexOf(f)
      if (i >= 0) callbacks.splice(i, 1)
    }
  }
  state.changed = onChanged

  let manager
  try {
    manager = api && typeof api.getUpdateManager === 'function' ? api.getUpdateManager() : null
  } catch (e) {
    state.error = String(e && (e.errMsg || e.message) || e)
    return state
  }
  if (!manager || ['onCheckForUpdate', 'onUpdateReady', 'onUpdateFailed', 'applyUpdate']
    .some((name) => typeof manager[name] !== 'function')) return state
  state.supported = true
  const listen = (name, f) => {
    try { if (typeof manager[name] === 'function') manager[name](f) } catch (e) {
      state.error = String(e && (e.errMsg || e.message) || e)
      notify()
    }
  }
  listen('onCheckForUpdate', (r) => {
    state.checked = true
    state.checkedAt = Date.now()
    state.available = !!(r && r.hasUpdate)
    state.ready = false
    state.error = ''
    notify()
  })
  listen('onUpdateReady', () => {
    state.ready = true
    state.available = true
    state.error = ''
    notify()
  })
  listen('onUpdateFailed', (r) => {
    state.ready = false
    state.available = false
    state.error = String(r && (r.errMsg || r.message) || 'update failed')
    notify()
  })
  state.apply = () => {
    if (!state.ready) return false
    try {
      if (typeof manager.applyUpdate !== 'function') return false
      state.ready = false
      manager.applyUpdate()
      return true
    } catch (e) {
      state.error = String(e && (e.errMsg || e.message) || e)
      notify()
      return false
    }
  }
  return state
}

module.exports = { updateManager }
