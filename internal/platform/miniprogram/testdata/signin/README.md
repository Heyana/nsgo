# Mini program login acceptance

Build this owned fixture from the repository root:

```sh
go tool nsgo build -target miniprogram_wechat -output /tmp/nsgo-signin1402 ./internal/platform/miniprogram/testdata/signin
```

Open the resulting `releases/miniprogram/wechat` directory in WeChat DevTools, or use its CLI `auto --project <that-directory> --auto-port 19402`. Approve only this generated project's code if DevTools asks. With `miniprogram-automator` 0.13 installed in a temporary directory, run `verify.cjs 19402` with that directory's `node_modules` on `NODE_PATH`.

The controller sends the rendered login control's normal page event with its mounted coordinates. The production runtime calls `wx.login`, and the app displays only whether it received a nonempty code and which host issued it. No code, app secret or identity is logged, stored or sent to a server. This verifies the real DevTools API and Go/Wasm bridge, rather than physical tap routing.

The default generated project uses `touristappid`. DevTools supplies a development login code there, which does not prove a registered app's server exchange or a real account login. Registered-app exchanges and physical WeChat, QQ, Alipay and Douyin acceptance remain separate checks. The portable `TestMiniProgramHostSignIn` covers all four adapters, failures, duplicate callbacks, explicit prompt policy and rejection of anonymous device codes; `TestMiniProgramSignInWasmBridge` runs the production Wasm adapter in a browser.
