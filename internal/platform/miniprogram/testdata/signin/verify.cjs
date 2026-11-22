// Run against this fixture's own DevTools automation port. Never print the
// host's short-lived code: only the app's public result text leaves the page.
const automator = require('miniprogram-automator')

;(async () => {
  const port = Number(process.argv[2] || 19402)
  const mini = await automator.connect({ wsEndpoint: `ws://127.0.0.1:${port}` })
  try {
    const page = await mini.currentPage()
    if (!page || page.path !== 'pages/index/index') throw new Error('open the owned sign-in fixture first')
    const initial = await mini.evaluate(() => getCurrentPages().slice(-1)[0].data.root.c[0].c[0].t.r.map((run) => run[0]).join(''))
    if (initial !== 'Ready') throw new Error('reload the fixture before each verification')
    const button = await mini.evaluate(() => {
      const page = getCurrentPages().slice(-1)[0]
      return page.data.root.c[0].c.find((node) => node.p)
    })
    if (!button) throw new Error('the fixture login control is not mounted')
    // Use the render layer's actual event shape and the mounted control's
    // coordinates, rather than inventing an authentication result in Go.
    await mini.evaluate((button) => {
      getCurrentPages().slice(-1)[0].onNsgo({
        detail: ['tap', button.id, button.f[0] + button.f[2] / 2, button.f[1] + button.f[3] / 2],
      })
      return true
    }, button)
    let result = ''
    for (let attempt = 0; attempt < 150; attempt++) {
      result = await mini.evaluate(() => {
        const root = getCurrentPages().slice(-1)[0].data.root
        return root.c[0].c[0].t.r.map((run) => run[0]).join('')
      })
      if (result !== 'Ready') break
      await new Promise((resolve) => setTimeout(resolve, 100))
    }
    if (result !== 'Host wechat returned a nonempty login code') throw new Error(`host login failed: ${result}`)
    console.log('PASS: WeChat host → production runtime → Go/Wasm → app returned a nonempty code')
  } finally {
    await mini.disconnect()
  }
})().catch((error) => {
  console.error(error.message)
  process.exitCode = 1
})
