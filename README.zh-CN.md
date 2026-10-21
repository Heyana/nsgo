# NSGo

[English](README.md) | **简体中文**

NSGo 是一个以 Go 为后端的跨平台原生 UI 引擎。应用逻辑、响应式图和布局在 Go 中运行，各平台使用自己的原生 UI 绘制。

NSGo 参考 [Native Shell（NSH）](https://nsh.dev) 的设计，NSH 用 Rust 实现了同样的模型。NSGo 面向想要 NSH 的模型、但不想写 Rust 的团队。

## 状态

NSGo 处于早期阶段。最小引擎已能在 macOS、iOS 和 Web 上渲染文本、输入框和按钮；Windows 可以编译但未测试，Android 正在开发，Linux 尚未开始。目前还没有发布版本或稳定的 API。各平台进度见 [ARCHITECTURE.md](ARCHITECTURE.md#platforms)。

## 目标

- 用 Go 编写应用和组件，使用 signal、memo 和 effect。
- 使用 flexbox 和 grid 布局，并用各平台自己的视图和文本渲染。
- 与 NSH 一样支持 iOS、Android、macOS、Windows、Linux 和 Web。
- 可嵌入现有原生应用，也可打包为独立应用。

## 用法

一个应用是调用 `nsgo.Run` 的 Go main 包，旁边放一个 `nsgo.yaml`：

```yaml
name: Hello
id: dev.oneo.nsgo.hello
version: 0.1.0
window: { width: 420, height: 560 }
dev:
  target: native        # native, web or ios_simulator
build:
  output: dist          # packages go to dist/<name>-<version>-<target>
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, ios, ios_simulator, web]
```

在你的模块中添加一次该工具，之后用 `go tool nsgo` 运行，无需安装：

```sh
go get -tool github.com/oneo-me/nsgo/cmd/nsgo
```

`go tool nsgo dev` 运行应用，并在源文件变化时重新编译。`go tool nsgo build` 为每个目标输出一个去除符号的发布包。

## 开发

要求：装有 Xcode 的 macOS，以及 Go 1.25 或更高版本。任何目标平台都不需要其他工具。

在仓库根目录运行示例：

```sh
go tool nsgo dev examples/hello                     # nsgo.yaml's dev target, rebuilt on save
go tool nsgo dev -target web examples/hello         # in the browser, reloaded on save
go tool nsgo dev -target ios_simulator examples/hello
go tool nsgo build examples/hello                   # release packages in examples/hello/dist
go test ./...
```

NSGo 以 NSH 作为参考实现。将它克隆到仓库根目录的 `nsh/`，Git 会忽略该目录：

```sh
git clone https://github.com/longbridge/nsh.git nsh
```

修改代码前请先阅读 [ARCHITECTURE.md](ARCHITECTURE.md) 和 [AGENTS.md](AGENTS.md)。

## 作者

[博客](https://oneo.me) · [X @oneo666](https://x.com/oneo666)
