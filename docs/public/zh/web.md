---
title: 发布 Web 应用
description: 使用文档语义、页面元数据和 CDN 资源发布 NSGo 应用。
group: 工具链
order: 24
---

## 构建应用

运行 `go tool nsgo build -target web`，通过 HTTP 提供生成的目录。带内容哈希的资源可长期缓存；`index.html`、`nsgo.json` 和 `manifest.webmanifest` 应重新验证。应用仍保持 NSGo 的布局、滚动、焦点和指针行为。

## 标题与链接

`core.View` 和 `core.Text` 的 `Role: core.RoleHeader` 在浏览器中生成 `h1` 到 `h6`，由 `AccessibilityLevel` 选择，未指定时使用 `h2`。`base/textview` 保留 Markdown 标题的级别。浏览器默认的标题字体和外边距不会改变应用样式。

`base.Link` 和 `ui.Link` 把 `Href` 发布为锚点地址。普通点击运行应用的 `OnOpen` 和 `OnPress`；中键或带修饰键的点击使用浏览器的链接操作。除非应用处理 `OnContextMenu`，链接使用浏览器的右键菜单。停用的链接没有可用地址。自定义链接视图设置 `core.View.Web: core.WebViewProps{Href: "/notes"}`，或使用 `HrefFunc`。TSX 使用 `web={{href: "/notes"}}`，标题级别使用 `accessibilityLevel={1}`。可执行的不安全 URL 协议会被拒绝；受限内容通过自身点击处理器导航，不发布浏览器地址。

## 页面元数据

导入 `github.com/nsgo-dev/nsgo/web`，在页面内容旁放置 `web.Head{Title: "笔记", Description: "你保存的笔记"}`。`TitleFunc` 和 `DescriptionFunc` 跟随信号。后挂载且可见的 Head 按字段覆盖较早的设置；隐藏或移除页面恢复之前的 Head，移除全部 Head 则恢复原始文档。空的静态字段继承，函数返回空字符串则清除该字段。Head 不产生布局节点；原生目标、嵌入应用和受限内容不改变宿主文档。

TSX 从 `nsgo/web` 导入 `Head`，使用 `<Head title={title()} description="你保存的笔记" />`。需要静态 HTML 预览时，在站点部署中预渲染各个 URL；Head 更新运行中的文档。

## 从 CDN 加载资源

加载器相对于 `nsgo.json` 解析应用文件。页面可从另一个源获取配置，导入配置中的加载器，再挂载整个应用：

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

CDN 应通过 CORS 允许页面源读取文件，并以 `application/wasm` 提供 WebAssembly。应用使用深层路由时，由服务器为相应路径提供入口 HTML。[嵌入](embedding.md#web)介绍如何挂载到页面中的一个元素。
