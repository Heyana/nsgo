<p align="center">
  <img width="128" height="128" alt="NSGo" src="examples/gallery/assets/Icon.png">
</p>

# NSGo

[English](README.md) | **简体中文**

NSGo 是一个以 Go 为后端的跨平台原生 UI 引擎。应用逻辑、响应式图和布局在 Go 中运行，各平台使用自己的原生 UI 绘制。

NSGo 参考 [Native Shell（NSH）](https://nsh.dev) 的设计，NSH 用 Rust 实现了同样的模型。NSGo 面向想要 NSH 的模型、但不想写 Rust 的团队。

## 状态

NSGo 处于早期阶段。引擎已具备 NSH 的响应式模型、block/flexbox/grid 布局、列表、应用自带字体、动图与矢量图片、画布、用 Go 编写的 GPU 着色器、自带 Lucide 图标包的 SVG 图标、网页视图、视频、应用自己的原生视图、动画值与布局动画、类 CSS 的渐变、阴影、滤镜与 3D 变换。

文件拖放、Markdown 和 HTML 文本视图、浮层、对话框、导航和无样式基础组件可在 macOS、iOS、Windows、Linux、Web 和 Android 设备及 x86_64 模拟器上运行。目前还没有发布版本或稳定的 API。各平台进度见 [ARCHITECTURE.md](ARCHITECTURE.md#platforms)。

## 目标

- 用 Go 编写应用和组件，使用 signal、memo 和 effect。
- 使用 flexbox 和 grid 布局，并用各平台自己的视图和文本渲染。
- 与 NSH 一样支持 iOS、Android、macOS、Windows、Linux 和 Web。
- 可嵌入现有原生应用，也可打包为独立应用。

## 用法

一个应用是调用 `nsgo.Run` 的 Go main 包。组件是返回元素的函数或类型；signal 变化时，读取它的地方随之更新：

```go
package main

import (
	"fmt"

	"github.com/oneo-me/nsgo"
	"github.com/oneo-me/nsgo/base"
	"github.com/oneo-me/nsgo/core"
)

func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		base.Button{
			Label:    "Press me",
			OnPress:  func(core.PressEvent) { count.Update(func(n int) int { return n + 1 }) },
			Style:    core.Style{Padding: core.Symmetric(6, 14), CornerRadius: 8, Background: core.RGB(0xbd481b), Color: core.RGB(0xffffff)},
			Children: []core.Node{core.Text{Content: "Press me"}},
		},
	}}
}

func main() { nsgo.Run(Root) }
```

用 `nsgotest` 无窗口地测试它：

```go
func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("text:Press me")
	app.Find("text:Pressed 1 times")
}
```

在包旁边放一个 `nsgo.yaml`，写明应用名称和目标平台：

```yaml
name: Hello
id: dev.oneo.nsgo.hello
version: 0.1.0
window: { width: 420, height: 560 }
icon:
  apple: assets/AppIcon.icon  # macOS 和 iOS；macOS 也接受 .icns
  png: assets/Icon.png        # 其他所有目标，以及没有上述图标的 Apple 目标
dev:
  target: native        # native、web、ios（连接的 iPhone）、ios_simulator 或 android（真机或模拟器）
  hot_reload: false     # 只运行一次，应用退出后 dev 也随之退出；true 则在保存后重新编译并重启
build:
  output: dist          # 构建产物位于 dist/<name>-<version>-<target>
  open: false           # 设为 true 时在构建后打开输出目录
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, android_arm64, android_x64, web]
```

`nsgo.yaml` 旁边有两个目录。`resources/` 存放随应用发布的文件：每个包把它们放进自己的资源根目录，`asset:` 图片从这里加载，`nsgo.Fonts` 也从这里注册字体，例如 `nsgo.Run(Root, nsgo.Fonts("fonts/Brand.ttf"))`。资源根目录在 macOS 上是 `Contents/Resources`，在 iOS 上是应用包本身，在 Android 上是 APK 的 assets，其他平台是可执行文件或页面旁的 `resources/`。`assets/` 存放不随应用发布的项目文件，例如 `icon` 默认使用的图标。

在你的模块中添加一次该工具，之后用 `go tool nsgo` 运行，无需安装：

```sh
go get -tool github.com/oneo-me/nsgo/cmd/nsgo
```

`go tool nsgo dev` 运行一次应用，应用退出后随之退出。将 `dev.hot_reload` 设为 `true`，源文件变化时会重新编译并重启。它构建的应用图标带有 DEV 标签；要改用其他文件，可在 `dev.icon` 中指定（与 `icon` 一样接受 `apple` 和 `png`），或在 `assets/` 中放入 `AppIcon-dev.icon`、`AppIcon-dev.icns` 或 `Icon-dev.png`。在 macOS 上，开发运行还会在应用上方显示性能监视器；发布构建不包含它。

`go tool nsgo build` 为每个目标输出一个去除符号的发布包；将 `build.open` 设为 `true`，构建完成后会打开输出目录。`build.targets` 和 `-target` 都接受 `all`，表示仅需 Go 和 Xcode 的所有平台；安装 NDK 后可再加上 `android_x64`。

dev 和 build 命令的参数会覆盖它们所对应的 `nsgo.yaml` 值：两者都接受 `-name`、`-id`、`-version`、`-width` 和 `-height`；dev 另有 `-target` 和 `-hot-reload`，build 另有 `-target`、`-output` 和 `-open`。dev 还接受 `-device` 以选择 iPhone，以及 `-team` 以选择 Apple 开发者团队。`-h` 会列出全部参数。

## 开发

要求：装有 Xcode 的 macOS，以及 Go 1.25 或更高版本。构建 arm64 的所有目标（包括 Android 的 APK）都不需要其他工具。

Android 真机和模拟器开发还需要 Android SDK platform-tools 中的 `adb`；iOS 真机开发需要在 Xcode 的 Accounts 中登录 Apple ID，并连接一台已信任、已开启开发者模式的 iPhone。

x86_64 模拟器目标 `android_x64` 还需要 Android NDK：Go 只能通过 cgo 链接 android/amd64。在 Windows 上，只需 Go 1.25 或更高版本即可运行原生应用，并构建 Windows 和 Web 包。在 Linux 上，安装 GTK 4.14 或更高版本、Pango 和 Cairo 后即可用 Go 运行原生应用；网页视图还需要 WebKitGTK 6.0，视频还需要 GStreamer 插件。

在仓库根目录运行 gallery，它在标准窗口中展示所有元素、`base` 组件和服务。

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml 的 dev 目标，应用退出后随之退出
go tool nsgo dev -target web examples/gallery       # 在浏览器中运行，直到中断
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # 在已连接的 iPhone 上运行，由 Xcode 签名
go tool nsgo dev -target android examples/gallery   # 在已连接的设备或模拟器上运行，通过 adb
go tool nsgo build examples/gallery                 # 在 examples/gallery/dist 中生成发布包
go tool nsgo build -target all examples/gallery     # 生成所有平台的发布包
go test ./...
bench/run.sh                                       # 测量行情表基准（见 bench/README.md）
```

在 Windows 上运行 `go test ./internal/platform/windows`，或在 Linux 上运行 `go test ./internal/platform/linux`，会打开真实窗口检查输入和绘制；没有桌面时会跳过。

NSGo 以 NSH 作为参考实现。将它克隆到仓库根目录的 `nsh/`，Git 会忽略该目录：

```sh
git clone https://github.com/longbridge/nsh.git nsh
```

修改代码前请先阅读 [ARCHITECTURE.md](ARCHITECTURE.md) 和 [AGENTS.md](AGENTS.md)。

官方网站就是编译到 web 的 gallery，地址为 <https://oneo-me.github.io/nsgo/>。运行方式为 `go tool nsgo dev -target web examples/gallery`；[Website 工作流](.github/workflows/website.yml) 将它发布到 GitHub Pages。

## 许可证

本项目以 [Apache License 2.0](LICENSE) 授权。NSGo 是 NSH 的移植，[NOTICE](NOTICE) 记录了移植所需的署名。

除非你明确声明，否则你提交给 NSGo 的任何贡献，按 Apache-2.0 许可证的定义，均以 Apache-2.0 授权，不附加任何额外条款或条件。

## 作者

[博客](https://oneo.me) · [X @oneo666](https://x.com/oneo666)
