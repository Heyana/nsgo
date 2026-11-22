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
  equal(updateManager({ getUpdateManager: () => ({}) }).supported, false)
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
  equal(updates.apply(), false)
  equal(manager.applied(), 1)
  manager.listeners.check({ hasUpdate: false })
  equal(updates.ready, false)
  equal(updates.available, false)
  assert(updates.checkedAt > 0, 'missing real check timestamp')
  const failed = fakeManager()
  const errorState = updateManager({ getUpdateManager: () => failed })
  failed.listeners.failed({ errMsg: 'vendor failure' })
  equal(errorState.error, 'vendor failure')
  failed.listeners.ready()
  equal(errorState.error, '')
  equal(errorState.ready, true)
  assert(true, 'updates lifecycle')
}
