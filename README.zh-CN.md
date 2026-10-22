<p align="center">
  <img width="128" height="128" alt="NSGo" src="docs/images/logo.png">
</p>

# NSGo

[English](README.md) | **简体中文**

NSGo 是一个以 Go 为后端的跨平台原生 UI 引擎。应用逻辑、响应式图和布局在 Go 中运行，各平台使用自己的原生 UI 绘制。

NSGo 参考 [Native Shell（NSH）](https://nsh.dev) 的设计，NSH 用 Rust 实现了同样的模型。NSGo 面向想要 NSH 的模型、但不想写 Rust 的团队。

## 状态

NSGo 处于早期阶段。引擎已具备 NSH 的响应式模型、block/flexbox/grid 布局、列表、图片、画布、Markdown 和 HTML 文本视图、浮层、对话框、导航、无样式基础组件和带主题的 `ui` 组件，可在 macOS、iOS、Windows、Linux 和 Web 上运行；Android 正在开发。目前还没有发布版本或稳定的 API。各平台进度见 [ARCHITECTURE.md](ARCHITECTURE.md#platforms)。

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
	"github.com/oneo-me/nsgo/core"
	"github.com/oneo-me/nsgo/ui"
)

func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return ui.ThemeView{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Label: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
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
dev:
  target: native        # native, web 或 ios_simulator
  hot_reload: false     # 只运行一次，应用退出后 dev 也随之退出；true 则在保存后重新编译并重启
build:
  output: dist          # 构建产物位于 dist/<name>-<version>-<target>
  open: true            # 构建后打开输出目录
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, web]
```

在你的模块中添加一次该工具，之后用 `go tool nsgo` 运行，无需安装：

```sh
go get -tool github.com/oneo-me/nsgo/cmd/nsgo
```

`go tool nsgo dev` 运行一次应用，应用退出后随之退出。将 `dev.hot_reload` 设为 `true`，源文件变化时会重新编译并重启。`go tool nsgo build` 为每个目标输出一个去除符号的发布包，并打开输出目录，除非 `build.open` 为 `false`。

dev 和 build 命令的参数会覆盖它们所对应的 `nsgo.yaml` 值：两者都接受 `-name`、`-id`、`-version`、`-width` 和 `-height`；dev 另有 `-target` 和 `-hot-reload`，build 另有 `-target`、`-output` 和 `-open`。`-h` 会列出全部参数。

## 开发

要求：装有 Xcode 的 macOS，以及 Go 1.25 或更高版本。构建任何目标平台都不需要其他工具。在 Windows 上，只需 Go 1.25 或更高版本即可运行原生应用，并构建 Windows 和 Web 包。在 Linux 上，安装 GTK 4.14 或更高版本、Pango 和 Cairo 后即可用 Go 运行原生应用。

在仓库根目录运行示例：

```sh
go tool nsgo dev examples/hello                     # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev -target web examples/hello         # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/hello
go tool nsgo build examples/hello                   # release packages in examples/hello/dist
go test ./...
```

在 Windows 上运行 `go test ./platform/windows`，或在 Linux 上运行 `go test ./platform/linux`，会打开真实窗口检查输入和绘制；没有桌面时会跳过。

NSGo 以 NSH 作为参考实现。将它克隆到仓库根目录的 `nsh/`，Git 会忽略该目录：

```sh
git clone https://github.com/longbridge/nsh.git nsh
```

修改代码前请先阅读 [ARCHITECTURE.md](ARCHITECTURE.md) 和 [AGENTS.md](AGENTS.md)。

官方网站是 `website/` 下的独立 Bun 项目，命令见 [website/README.md](website/README.md)。

## 许可证

本项目以 [Apache License 2.0](LICENSE) 授权。NSGo 是 NSH 的移植，[NOTICE](NOTICE) 记录了移植所需的署名。

除非你明确声明，否则你提交给 NSGo 的任何贡献，按 Apache-2.0 许可证的定义，均以 Apache-2.0 授权，不附加任何额外条款或条件。

## 作者

[博客](https://oneo.me) · [X @oneo666](https://x.com/oneo666)
