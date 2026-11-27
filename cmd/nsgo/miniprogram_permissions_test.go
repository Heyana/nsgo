package main

import "testing"

func TestMiniProgramPermissionAPIAdapters(t *testing.T) {
	vm := runMiniProgramModule(t, "api.js")
	_, err := vm.RunString(`
;(() => {
const { apiOf } = module.exports
function check(cond, msg) {
  if (!cond) throw new Error(msg)
}

// WeChat, QQ and Douyin expose WeChat's scope.record, scope.camera and scope.bluetooth directly.
for (const platform of ['wechat', 'qq', 'douyin']) {
  const scopes = []
  let opened = 0
  const base = {
    getSetting(o) {
      o.success({ authSetting: { 'scope.record': true, 'scope.camera': false } })
    },
    authorize(o) {
      scopes.push(o.scope)
      if (o.scope === 'scope.bluetooth') o.success({})
      else o.fail({ errMsg: 'authorize:fail auth deny' })
    },
    openSetting(o) {
      opened++
      o.success({ authSetting: { 'scope.camera': true } })
    },
  }
  const api = apiOf(platform, base)
  let setting
  api.getSetting({ success: (r) => (setting = r.authSetting) })
  check(setting['scope.record'] === true, platform + ' record')
  check(setting['scope.camera'] === false, platform + ' camera')
  check(setting['scope.bluetooth'] === undefined, platform + ' bluetooth')

  let authOK = false
  api.authorize({ scope: 'scope.bluetooth', success: () => (authOK = true) })
  check(authOK && scopes[0] === 'scope.bluetooth', platform + ' authorize bluetooth')

  let deniedMsg = ''
  api.authorize({ scope: 'scope.camera', fail: (e) => (deniedMsg = e.errMsg) })
  check(deniedMsg === 'authorize:fail auth deny', platform + ' authorize deny')

  let openSetting
  api.openSetting({ success: (r) => (openSetting = r.authSetting) })
  check(opened === 1 && openSetting['scope.camera'] === true, platform + ' openSetting')
}

// Douyin hosts that omit tt.authorize keep api.authorize undefined.
{
  const api = apiOf('douyin', { getSetting() {}, openSetting() {} })
  check(typeof api.getSetting === 'function', 'douyin getSetting')
  check(typeof api.openSetting === 'function', 'douyin openSetting')
  check(api.authorize === undefined, 'douyin missing authorize')
}

// Alipay translates audioRecord, camera and bluetooth to WeChat's scope.* keys,
// leaves missing authorize undefined, and translates scope when authorize is present.
{
  let openCalls = 0
  let settingFail = false
  const myBase = {
    getSetting(o) {
      if (settingFail) return o.fail({ errorMessage: 'denied' })
      o.success({ authSetting: { audioRecord: true, camera: false } })
    },
    openSetting(o) {
      openCalls++
      o.success({ authSetting: { bluetooth: true, camera: true } })
    },
  }
  const api = apiOf('alipay', myBase)
  check(api.authorize === undefined, 'alipay omits authorize when my.authorize is absent')
  let setting
  api.getSetting({ success: (r) => (setting = r.authSetting) })
  check(setting['scope.record'] === true, 'alipay audioRecord -> scope.record')
  check(setting['scope.camera'] === false, 'alipay camera -> scope.camera')
  check(setting['scope.bluetooth'] === undefined, 'alipay absent bluetooth stays undefined')

  let opened
  api.openSetting({ success: (r) => (opened = r.authSetting) })
  check(openCalls === 1 && opened['scope.bluetooth'] === true && opened['scope.camera'] === true, 'alipay openSetting')

  settingFail = true
  let failMsg = ''
  api.getSetting({ fail: (e) => (failMsg = e.errorMessage) })
  check(failMsg === 'denied', 'alipay getSetting fail propagated')

  // Malformed Alipay response without authSetting does not fabricate an empty object.
  const malformed = apiOf('alipay', { getSetting(o) { o.success({}) } })
  let res
  malformed.getSetting({ success: (r) => (res = r) })
  check(res.authSetting === undefined, 'malformed alipay getSetting keeps authSetting undefined')

  // Missing Alipay setting APIs stay undefined.
  const empty = apiOf('alipay', {})
  check(empty.getSetting === undefined && empty.openSetting === undefined && empty.authorize === undefined, 'empty alipay')

  // When a host supplies my.authorize, WeChat scope names map to Alipay keys.
  const seen = []
  const withAuth = apiOf('alipay', {
    authorize(o) {
      seen.push(o.scope)
      o.success({})
    },
  })
  withAuth.authorize({ scope: 'scope.record', success() {} })
  withAuth.authorize({ scope: 'scope.camera', success() {} })
  withAuth.authorize({ scope: 'scope.bluetooth', success() {} })
  check(seen.join(',') === 'audioRecord,camera,bluetooth', 'alipay authorize scope mapping: ' + seen.join(','))
}
})()
`)
	if err != nil {
		t.Fatal(err)
	}
}
