'use strict'

module.exports = function (updateManager) {
  const assert = (value, message) => { if (!value) throw new Error(message || 'assertion failed') }
  const equal = (got, want, message) => { if (got !== want) throw new Error((message || 'values differ') + ': ' + got + ' !== ' + want) }
  const fakeManager = () => {
    const listeners = {}
    let applied = 0
    return {
      onCheckForUpdate(f) { listeners.check = f },
      onUpdateReady(f) { listeners.ready = f },
      onUpdateFailed(f) { listeners.failed = f },
      applyUpdate() { applied++ },
      listeners,
      applied() { return applied },
    }
  }
  const unsupported = updateManager({})
  equal(unsupported.supported, false)
  equal(unsupported.apply(), false)
  const manager = fakeManager()
  const updates = updateManager({ getUpdateManager: () => manager })
  equal(updates.supported, true)
  equal(updates.apply(), false)
  manager.listeners.check({ hasUpdate: true })
  equal(updates.checked, true)
  equal(updates.available, true)
  manager.listeners.ready()
  equal(updates.ready, true)
  equal(updates.apply(), true)
  equal(manager.applied(), 1)
  const failed = fakeManager()
  const errorState = updateManager({ getUpdateManager: () => failed })
  failed.listeners.failed({ errMsg: 'vendor failure' })
  equal(errorState.error, 'vendor failure')
  assert(true, 'updates lifecycle')
}
