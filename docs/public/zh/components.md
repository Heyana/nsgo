---
title: 组件与状态
description: 元素、组件、signal 与样式。
group: 核心概念
order: 50
---

## 元素

屏幕是一棵元素树，元素是 Go 结构体值。`core` 提供基本元素：`View`、`Text`、`Image`、`Scroll`、`Input` 等。`ui` 提供应用使用的控件，例如 `Button`、`Select` 和 `DatePicker`，外观跟随主题。（`base` 提供同一批控件的无外观形式，供[组件作者](authoring/index.md)使用。）

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` 显示网页内容；空的 `WebSource` 会加载 `about:blank`，并报告通常的加载和导航事件。让 `SourceFunc` 返回空来源，或调用 `Ref.Load(core.WebSource{})`，即可清空之前的页面。若页面自身脚本之前的注入脚本跟随 signal，请设置 `InjectedJavaScriptBeforeContentFunc`：脚本变化时，web view 会带着新脚本重新加载它的来源，替换当前显示的页面，即使那是用户或 `Ref.Load` 导航到的页面。设置 `UserAgentSuffix`（TSX 中为 `userAgentSuffix`）可在平台自己的 user agent 之后（设置了 `UserAgent` 时则在它之后）追加一段标识，例如 `Version/26.0 Safari/605.1.15`；在 macOS 和 iOS 上，追加到平台自己的 user agent 只在 web view 首次创建时生效，HarmonyOS 和 Web 保持各自的 user agent。给它绑定一个 `core.Ref` 即可调用 `Reload`、`GoBack` 和 `GoForward`。在原生目标上，这些命令让应用自己的 HTML 和 data 来源继续不受 `core.AccessWebSchemes` 限制；页面自己发起的新导航仍遵循访问策略。

在 macOS、iOS、Windows 和 Linux 上，`OnNavigationStateChange` 还会报告同一文档内的 URL、历史记录和标题变化。

在 TSX 中，`injectedJavaScriptBeforeContentLoaded` 变化时，同一个 web view 会像 `InjectedJavaScriptBeforeContentFunc` 那样带着新脚本重新加载来源；`injectedJavaScript` 变化时，则会像 `userAgent`、`userAgentSuffix` 或 `mobile` 变化时一样，换成一个新的 web view，重新加载来源。

设置 `Mobile`（TSX 中为 `mobile`）后，Windows、macOS 和 Linux 上的页面按手机所见的样子显示，适合 YouTube Shorts 这类只在手机上用上下滑动切换视频的网站：页面得到手机的 user agent（设置了 `UserAgent` 时以它为准）和没有悬停的触摸屏，鼠标拖动、以及在不可滚动处的滚轮或触控板滚动，都会作为手指滑动送达页面。Windows 像 Chrome 的设备模式那样模拟手机。macOS 和 Linux 用脚本实现，因此页面的样式表仍看到鼠标，触摸事件也不是可信事件，页面可能忽略它们。iOS、Android、HarmonyOS、Web 和小程序按原样显示页面。

在 macOS、iOS、Windows 和 Linux 上，`OnOpenWindow` 接收网页的新窗口请求。WebView 不会弹出窗口。应用可根据请求的 `URL` 和 `UserInitiated` 决定通过 `Ref.Load` 在同一视图导航、在其他位置打开 URL，或者忽略请求。受限 WebView 会通过 `OnError` 报告被拒绝的请求。

`core.Video` 用平台的播放器播放文件或 URL。在 macOS、iOS 和 tvOS 上，`Video.AudioSource` 可以为有固定时长的视频指定单独的音频文件或 URL。视频使用该来源的音轨，画面和声音同时开始，`Ref.Seek` 在同一条时间轴上移动两者。播放时长由画面决定，较短的音轨播完后保持静音。

带透明通道的视频会透出它后面的内容：macOS 上用 `.mov` 里带 alpha 的 HEVC（iOS 尚未验证），Linux 上用 WebM 里带 alpha 的 VP9，并需要 GStreamer 的 codecalpha 插件（`gstreamer1.0-plugins-bad`，Linux 安装包可以在 `nsgo.yaml` 里把它列为推荐依赖）。在 macOS、iOS、Windows 和 Linux 上，本地视频的透明通道无法保留时（Apple 平台上的 WebM、Linux 上的 HEVC，以及在 Windows 机器上验证 Media Foundation 的透明通道输出之前，Windows 上的任何格式），`OnError` 会说明原因，而不是把视频显示在不透明的方框里：这个错误是 `*core.MediaError`，其 `Reason` 为 `core.MediaErrorAlphaUnsupported`，应用可以据此改用自己的解码器播放这段视频。Android 和 Web 平台，以及各平台上的远程 URL，会按解码器的输出播放这类视频，可能是不透明的。

`Rate` 设置播放速度，`RateFunc` 可在播放中调整速度。根据 `OnLoad` 返回的 `VideoInfo.RateChangeable` 决定是否显示倍速控件。在 Linux 上，安装 GstPlay 和 GStreamer 的 `scaletempo` 滤镜后，可调整支持跳转的视频的播放速度。备用播放器以 1 倍速播放。

## 组件

组件是一个接收 scope、返回元素的函数。它只运行一次，之后的变化由 signal 驱动。

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

带属性的组件是一个有 `Render` 方法的类型：

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` 可以用 `NoClose` 隐藏关闭按钮，用 `Trailing` 添加尾部节点，并用 `OnPress` 让整条 toast 响应按压并关闭。`Toasts.NoClose` 隐藏整个堆栈的关闭按钮。

`ui.Toasts.Style` 和 `StyleFunc` 可设置每条 toast 的表面样式，包括背景、边框、圆角和内边距。堆栈样式覆盖主题，随后合并单条 `ToastContent.Style` 和 `StyleFunc`。每一级的 `StyleFunc` 都在对应的 `Style` 之后合并，并随它读取的 signal 更新。样式中的 `Color` 会用于标题，并以较低不透明度用于描述，使文字在深色或彩色表面上保持清晰。进入、退出和滑动阶段继续应用各自的动效样式。在 TSX 中，通过 `Toasts` 的 `style` 属性设置堆栈样式，调用 `show` 时可用 `style` 或 `styleFunc` 设置单条 toast。

## Signal

signal 保存一个值。在元素调用的函数（例如 `ContentFunc`）中读取它，就会让屏幕上的这一部分订阅它；值变化时，只有这一部分更新。

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

整个切片放在一个 signal 里时，读它的每一行都会更新。`core.NewCollection` 按 key 保存条目，每个条目有自己的 signal，所以每一行只跟随自己的条目，改一个条目只会重算一行：

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

其他状态也同样建立在 signal 之上：

- `core.NewSelector` 判断某个 key 是否被选中，选中项变化时只重算离开和到达的那两行。
- `core.NewHistory` 保存一个可撤销、重做的值，它的 `CanUndo` 和 `CanRedo` 是 signal，可以直接用于编辑菜单。
- `core.NewPersisted` 把带类型的值以带版本号的 JSON 保存在应用的存储里，并迁移旧版本写下的数据。
- `core.From` 跟随响应式图之外的数据源，例如在其他 goroutine 上推送数据的订阅。

## 样式

`core.Style` 遵循 CSS：flexbox 与 grid、内外边距、圆角、边框、阴影、渐变、变换，以及 `Active` 等状态样式。元素也可以通过 `Class` 使用 Tailwind 类名：

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

主题为 `ui` 的组件提供浅色与深色外观。

## 桌面应用

`nsgo.Run(Root)` 显示一个根视图。桌面应用也可以自己决定显示什么：`nsgo.Run(App)` 运行一次 `func App(app *core.Scope)`，由它通过 `desktop` 打开窗口、面板、托盘图标和全局快捷键，它们共享它的 signal。目前支持 macOS、Windows 和 Linux；在 macOS 上 `examples/tray` 不出现在程序坞中，只驻留在菜单栏。`nsgo.Run(App, nsgo.StartHidden())` 打开主窗口但不显示它，托盘应用或随登录启动的应用（`desktop.LaunchedAtLogin`）因此不会闪出窗口；应用用 `app.MainWindow().Show()` 显示它。手机和 Web 照常在启动时显示应用。

`desktop.SetWindowLevel` 让窗口保持在其他窗口之上（比如计时器或桌面宠物），或置于其下；`desktop.SetWindowFrame` 移动窗口并调整大小。`desktop.CanSetWindowLevel` 和 `desktop.CanPlaceWindows` 说明它们在窗口的 scope 中是否生效：macOS 和 Windows 上都生效，Linux 在 X11 下只有层级生效，在 Wayland 下都不生效，因为窗口的层叠和位置由合成器决定。

用 `desktop.WindowOptions{Transparent: true}` 打开的窗口，或用 `nsgo.WindowTransparent()` 打开的主窗口，没有背景和边框，应用不绘制的地方都会露出桌面，适合桌面宠物或小组件；带 `Desktop.WindowDrag` 的视图可以拖动它。`NoShadow` 去掉系统阴影，`SkipTaskbar` 让窗口不出现在 Windows 任务栏和 Alt+Tab 以及 X11 的任务列表中；之后可以用 `desktop.SetWindowTransparent`、`SetWindowShadow` 和 `SetSkipTaskbar` 修改它们。Windows 只能在以透明方式打开的窗口中切换透明，这种窗口没有阴影；Wayland 没有让窗口不出现在任务列表中的提示。`desktop.CanMakeWindowsTransparent` 和 `desktop.CanSkipTaskbar` 说明它们在窗口的 scope 中是否生效：Windows 上透明需要 DirectComposition，Linux 上需要合成器（没有合成器的 X11 不支持）；在 macOS 上用 `desktop.SetDockVisible` 让应用不出现在程序坞中。
