---
title: 嵌入
description: 把 NSGo 应用放进你自己的 iOS、Android、桌面或网页应用。
group: 工具链
order: 23
---

## 构建可嵌入的应用

可嵌入的应用就是普通的应用包，带 `Root` 和 `main`，用 `-embed` 按宿主的平台构建。每个视图运行一次应用，并且是独立的引擎，因此宿主可以显示任意多个视图，每个都有自己的状态。

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

iOS 构建写出 `<Name>.xcframework` 和 `<Name>Resources`；Android 构建写出 `<Name>.aar`；Web 构建写出包含 `nsgo.json` 和加载器的目录。

## iOS

把框架作为静态库加入应用目标（不要嵌入），把资源目录作为文件夹引用加入，然后 `import <Name>`。在应用启动时、第一个视图之前调用一次 `NSGoStart()`。静态库的代码属于宿主应用，因此由宿主应用的隐私清单声明 NSGo 用到的需要理由的 API：用户默认设置（`CA92.1`）、文件时间戳（`C617.1`）和系统启动时间（`35F9.1`）；`-dynamic` 框架则在自己的 `PrivacyInfo.xcprivacy` 中声明它们。

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // 存储所在的位置
view.localeIdentifier = "de-CH"            // 替换系统语言
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // 覆盖应用主题
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` 在进入窗口后启动应用，在释放或收到 `stop()` 时停止。启动值要在进入窗口前设置，其余值可以随时改变。处理器、组件和回调在主线程运行，回复可以从任意线程稍后到达。视图控制器可以转发视图的 `preferredStatusBarStyle`、`prefersStatusBarHidden` 和 `prefersHomeIndicatorAutoHidden`，以跟随它的系统栏设置。

## Android

把 AAR 加入应用模块，并让原生库被解压，以便运行时找到它们：

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

在代码或布局中创建 `dev.oneo.nsgo.NsgoView`。每个视图在第一次获得尺寸时启动一个独立进程的应用，`stop()` 结束它。把 Activity 的结果转发给每个视图，并在 Activity 中自行处理配置变化，以免应用重启。

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

要在释放原生视图树时保留 Go 应用，由宿主持有 `NsgoEngine`，并显式附加视图：

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// 移走视图前：
app.detachFromEngine();
// 稍后复用同一个视图：
app.attachToEngine(engine);
// 宿主结束应用时：
engine.destroy();
```

分离会释放原生子视图；重新附加会恢复当前信号、输入、焦点、滚动位置和绘图，不重新运行组件。网页和播放器按应用配置重新创建。对显式传入的引擎，`stop()` 只分离视图。每个引擎仍保留最初的视图及其 Activity，并拒绝其他视图，因此仍需由该 Activity 处理配置变化；尚不支持替换 Activity 或在一个进程中显示独立视图。

要检查 Google Play 更新，请把 `com.google.android.play:app-update:2.1.0` 加入 Android 宿主的 Gradle 依赖，并用[运行时的压缩规则](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro)保留 NSGo 按名称加载的 AppUpdateManager、AppUpdateInfo、AppUpdateOptions 和 Tasks API。上面的 Activity 结果转发也会传递用户取消更新的结果。

## 桌面

Go 宿主运行平台的事件循环，把每个视图的 `Handle` 放进自己的窗口。`nsgo.NewView` 在 handle 进入窗口后启动应用，因此之前设置的选项会出现在第一帧。

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // NSView、GtkWidget 或子窗口
```

`view.Host()` 向应用发送事件并接收应用的调用，`nsgo.Component` 为宿主平台注册原生组件。

## Web

Web 构建本身是一个页面，但页面也可以把它挂载到一个元素里，并成为它的宿主。`mount(element, url, options)` 返回一个 player，`nsgo.json` 给出相对于自身的加载器、wasm 和资源。

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "来自页面的问候" });
</script>
```

没有 `host` 选项时，加载器导入 `nsgo.yaml` 中 `web.host` 指定的模块。如果模块导出了 `createHost(player)`，加载器会等待它返回。元素或框架中的应用把浏览器的历史记录和标题交给页面管理。

`appearance`（`"light"`、`"dark"` 或 `"system"`）、`locale` 和 `fontScale` 代替浏览器的设置，与原生宿主的设置相同；之后可以用 player 的 `setAppearance`、`setLocale` 和 `setFontScale` 修改。`module` 是编译好的 `WebAssembly.Module` 或它的 promise，让多次挂载同一应用的页面只下载和编译一次。

加载器在应用启动前显示 `progress.nsgo-load-progress` 元素。可以用 CSS 设置样式，在挂载选项中传入 `onProgress`，也可以为应用自己的页面从 `web.host` 导出 `onProgress`。回调替换默认进度条，接收解码后的字节数 `{ loaded, total }`，总大小未知时 `total` 为 `null`。启动结束时，加载器以 `null` 调用回调，以便清除界面。传入已编译的 `module` 会跳过下载进度。

```yaml
web:
  host: host.js
```

## 宿主通道

`core.Host` 是每个宿主都实现的同一份约定：启动参数、宿主发送的事件，以及应用调用的宿主函数。值按 JSON 传递，因此 map 变成 `map[string]any`，数字变成 `float64`，同一份代码在 Swift、Java、Go 和 JavaScript 中都能工作。

- `HostParams` 在启动时读取一次启动参数。
- `cx.OnHostEvent` 听到 `Host.Send` 和页面的 `hostEvent`。
- `cx.CallHost` 调用命名处理器并等待结果；`cx.PostHost` 调用但不等待结果。
- `Host.HandleOpenURL` 在平台之前看到 `cx.OpenURL`，它的回答决定平台是否也打开。
- `Host.SetParams`、`Send`、`Handle` 和 `HandleDefault` 可以从任意 goroutine 调用。

iOS 宿主设置 `params`，用 `handle` 安装处理器，用 `send` 发送事件，并用 `onOpenURL` 回答打开链接的请求。Android 宿主用 `setParams`、`handle`、`send` 和 `setOpenUrlHandler` 做同样的事。桌面宿使用 `core.Host` 的方法，Web 由页面的 `host` 对象回答。

## 原生组件

`core.Native` 显示宿主创建的视图：它的 `Props` 到达宿主组件的 `SetProp`，组件发出的事件到达 `OnEvent`，值为动画节点的 prop 会在帧时钟上移动，不消耗引擎。

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

在宿主中注册组件：iOS 用 `app.register(component:)` 和 `NSGoComponent`，Android 用 `registerComponent` 和 `NsgoComponent`，桌面用 `nsgo.Component`。Web 没有原生组件，因此未知名称在那里显示空盒子。

## 设置

`tokens` 按角色名覆盖应用主题（`accent`、`inset`、`raised`），可以只覆盖一种外观；`cx.Palette()` 会看到它们。宿主还设置语言、文本大小、外观和数据目录，并听到应用的失败。`nsgo build -embed` 不生成窗口、菜单或托盘图标：这些属于宿主。

视图的引擎 goroutine 发生内部 panic 时，该引擎会停止，销毁它连接的所有界面，并向宿主的错误监听器（Go 中的 `nsgo.OnError`）报告 panic 和堆栈。其他引擎继续运行；要重启失败的应用，创建新的视图。普通应用回调的 panic 仍优先交给最近的 `core.Catch`。

`examples/embed-ios`、`examples/embed-android`、`examples/embed-desktop` 和 `examples/embed-web` 是同一个应用配四种宿主。

## 下一步

- [平台服务](services.md)：应用向平台请求什么。
- [构建与发布](building.md)：`-embed` 生成什么包，以及如何签名。
- [示例：待办应用](tutorials/todo.md)：宿主嵌入的应用。
