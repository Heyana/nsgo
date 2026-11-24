---
title: 应用地址
description: 通过浏览器历史记录读取和更新 Web 应用的地址。
group: 核心概念
order: 59
---

`cx.AppURL()` 将应用的地址作为响应式跟踪值读取。在 Web 上，根视图渲染之前即可获取初始页面 URL，因此刷新页面或打开分享链接时可以选择相同的视图。应用自行决定如何将 URL 映射到各个视图。

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // 按应用常规方式显示错误。
            }
        }},
    }}
}
```

`SetAppURL` 会立即更新被跟踪的地址，而不会请求新页面或对导航栈执行跳转。默认情况下，它会替换当前的浏览器历史条目；`HistoryPush` 则请求新增一个条目。同一次提交中的导航或模态框推入操作会代为提供该条目，从而避免单次操作产生两个后退步骤。同一次提交中的多次写入会保留最后一个地址，并保留任何推入请求。

相对地址会基于当前地址或显式设置的 HTML base 进行解析。浏览器地址必须与页面保持同源。无效地址、空白字符、控制字符、反斜杠以及超过 16 KiB 的地址都会返回错误，且不会修改当前地址。

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` 用于监听浏览器的历史记录遍历和 hash 编辑。`change.Reason` 为 `"back"`、`"forward"` 或 `"address"`；返回的函数用于取消订阅，作用域销毁时也会移除该监听器。初始地址请通过 `AppURL` 读取。应用自身的写入会更新绑定，但不会发送变更通知。保持相同地址的遍历也不会发送变更通知。

浏览器的后退操作仍然可以关闭模态框或弹出导航页面。如果应用拒绝后退，则会恢复现有条目及其地址。关闭处理器可以自行设置替代地址。以编程方式弹出页面时，地址会在异步历史记录遍历完成后更新，而在该遍历期间到达的写入操作会等待其完成。后退越过当前播放器的历史条目（包括从上一次加载保留下来的条目）将会离开应用。

对于 `/films/42` 这类路径，请设置应用的部署目录，并让服务器为应用路由提供其 `index.html`：

```yaml
web:
  base: /        # 部署在该子目录下时设为 /my-app/
```

`web.base` 会写入一个 HTML base，使直接打开深层地址时仍能加载脚本、样式、Wasm 和其他资源。未设置该项时，将保留原有的相对资源路径；使用查询参数或 hash 的地址可直接沿用这些相对路径。服务器的回退规则必须放行真实的资源请求。严格的内容安全策略（Content Security Policy）必须允许这个同源 base（`base-uri 'self'`）。

嵌入在其他页面中的应用会在内部维护其地址，不会影响宿主页面的历史记录。同源 iframe 可以通过 `data-nsgo-history` 显式启用该功能。受限子树无法读取或修改应用的地址。

在其他目标平台上，`AppURL` 会保留最近一次用于打开应用的链接或由 `SetAppURL` 存储的值；设置该值不会改变任何原生窗口或系统浏览器。小程序也会保留这个值，但不会改变宿主路由。

TSX 使用相同的状态：

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // 根据浏览器的选择更新应用路由。
});
```
