'use strict'

// The hosts issue one-use codes, not OAuth callback URLs. Keep their basic
// login separate from AuthSession, and never treat a device code as a user.
function available(platform, api) {
  if (!api) return false
  switch (platform) {
    case 'wechat':
    case 'qq':
    case 'douyin':
      return typeof api.login === 'function'
    case 'alipay':
      return typeof api.getAuthCode === 'function'
  }
  return false
}

function signIn(platform, api, options) {
  let ended = false
  const finish = (code, error) => {
    if (ended) return
    ended = true
    options.complete({ platform, code: error ? '' : code, error })
  }
  if (!available(platform, api)) return finish('', 'unavailable')
  const success = (r) => {
    r = r || {}
    if (platform === 'douyin' && r.isLogin === false) return finish('', 'notSignedIn')
    const code = platform === 'alipay' ? r.authCode : r.code
    if (typeof code !== 'string' || code.length === 0) return finish('', 'failed')
    finish(code, '')
  }
  const fail = (r) => {
    // Douyin explicitly documents 113001 as cancellation of host login.
    finish('', platform === 'douyin' && r && Number(r.errorCode) === 113001 ? 'cancelled' : 'failed')
  }
  try {
    if (platform === 'alipay') api.getAuthCode({ scopes: ['auth_base'], success, fail })
    else if (platform === 'douyin') api.login({ force: options.prompt === true, success, fail })
    else api.login({ success, fail })
  } catch (_) {
    finish('', 'failed')
  }
}

module.exports = { available, signIn }
