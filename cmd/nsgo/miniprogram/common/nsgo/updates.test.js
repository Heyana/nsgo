'use strict'

const assert = require('assert')
const { updateManager } = require('./updates.js')

function fakeManager() {
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
assert.strictEqual(unsupported.supported, false)
assert.strictEqual(unsupported.apply(), false)

const manager = fakeManager()
const updates = updateManager({ getUpdateManager: () => manager })
assert.strictEqual(updates.supported, true)
assert.strictEqual(updates.apply(), false)
manager.listeners.check({ hasUpdate: true })
assert.strictEqual(updates.checked, true)
assert.strictEqual(updates.available, true)
manager.listeners.ready()
assert.strictEqual(updates.ready, true)
assert.strictEqual(updates.apply(), true)
assert.strictEqual(manager.applied(), 1)

const failed = fakeManager()
const errorState = updateManager({ getUpdateManager: () => failed })
failed.listeners.failed({ errMsg: 'vendor failure' })
assert.strictEqual(errorState.error, 'vendor failure')

console.log('updates.js lifecycle tests passed')
