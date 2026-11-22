---
title: 开发环境
description: 从源码构建 NSGo，运行 gallery 并测试改动。
group: 参与项目
order: 80
---

## 环境要求

装有 Xcode 的 macOS 和 Go 1.27.3 或更新版本即可构建 arm64 的所有目标，包括 Android 的 APK。在设备上运行所需的条件见[环境准备](getting-started.md#环境要求)；x86_64 Android 模拟器目标 `android_x64` 还需要 Android NDK，因为 Go 只能通过 cgo 链接 android/amd64。Windows 安装程序需要 NSIS（`brew install makensis`），没有时 `build` 只留下它的脚本。

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

修改代码前请先阅读 [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) 和 [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md)。

## Gallery

gallery 为每个元素、`base` 和 `ui` 组件及服务各提供一个演示。宽窗口在侧边栏按选项卡列出演示，窄窗口在底部栏显示选项卡和设置，演示页面推入其上。在仓库根目录运行：

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml 的 dev 目标，应用退出后随之退出
go tool nsgo dev examples/gallery-tsx               # 用 TSX 编写的 gallery 组件和应用
go tool nsgo dev -target web examples/gallery       # 在浏览器中运行，直到中断
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # 在已连接的 iPhone 上运行，由 Xcode 签名
go tool nsgo dev -target android examples/gallery   # 在已连接的设备或模拟器上运行，通过 adb
go tool nsgo build -target all examples/gallery     # 在 examples/gallery/dist 中生成所有平台的包，交付物在 dist/releases
```

## 检查与测试

```sh
go tool nsgo check                 # 检查所有包
go tool nsgo check -target all     # 改为检查所有可用的 Go/Xcode 目标
go tool nsgo test                  # 运行所有包的测试
go tool nsgo test -bench . ./core  # core 的基准测试
(cd benchmark && go run ./runner)  # 与其他框架对比测量
```

`-target` 只检查它指定的目标，不再检查宿主系统，在 NSGo 自己的仓库里也不检查 dev client，这些由不带参数的 `check` 检查。要检查一项改动涉及的平台，把它们都写进同一个检查，例如 `check -target web,android -jobs 2`，不要同时启动多个：`-jobs n` 最多同时检查 n 个目标，每次 `go vet` 使用 n 个编译进程，默认一次检查一个目标并使用 Go 的默认值；同一模块里已有检查在运行时，后启动的检查会提示并等待它结束。CI 的 `check.yml` 会检查所有目标。

在仓库根目录或 `internal/dev` 中运行无头测试命令时，它会带上 `nsgodev` 构建标签再测试一次 `internal/dev`（dev client 的文件需要这个标签），让本地和 CI 都运行 dev client 的测试。两次运行使用相同的筛选条件和参数，结果合并到同一份统计和 JUnit 报告中，第二次运行的包名为 `internal/dev [nsgodev]`。

`check -target all` 在任何宿主系统上都会检查 macOS、Windows、Linux、Android arm64 和 Web 目标。有对应 Xcode SDK 时，还会检查 iOS、tvOS 和 watchOS，并集中列出跳过的目标。显式指定的目标（例如 `check -target ios`）在所需工具不可用时仍会失败，与 `all` 一起指定时也一样。一个目标失败不会阻止后续目标的检查，任何必需检查失败都会让命令失败。`build -target all` 仍需要装有 Xcode 的 macOS。在 NSGo 自己的仓库里，每个指定的目标还会带 `nsgotest` 检查平台的测试宿主；`harmonyos` 预览目标不在 `all` 里，CI 会单独指定它。

Linux CI 通过 Node.js 和 Go 的 Wasm 启动器运行 HarmonyOS Go 后端的 `js/wasm` 测试。Gallery 测试需通过 `hdc` 连接 ArkUI 模拟器后单独运行。编译 [HarmonyOS 宿主模板](building.md#harmonyos) 需要 OpenHarmony SDK API 26。

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

在 Linux 桌面的 Wayland 会话中，用 `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<已安装的桌面应用 ID> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux` 验收真实的全局快捷键门户。该 ID 必须有对应的 `.desktop` 条目。在合成器的提示中允许 Ctrl+Alt+K，再让另一个应用获得焦点并按下该快捷键；检查会等待注册成功和快捷键触发。未设置该环境变量时会跳过此项。

## 发布版本

下游项目需要带标签的版本，或维护者有可以发布的改动时，NSGo 会按需发布。为下游库请求发布时，请链接依赖的工作，并注明包含该库所需最后一项 NSGo 改动的提交。第一个包含该提交的发布标签，就是该库可以依赖的版本。

要在打标签前测试已合入的改动，可以在下游模块中运行 `go get github.com/nsgo-dev/nsgo@main`。也可以用提交 ID 代替 `main`，选定已验证的版本。Go 会把生成的伪版本记录在 `go.mod` 中。

维护者在 `CHANGELOG.md` 中为改动写上版本和日期，留出新的 Unreleased 小节，并为 `main` 上的对应提交打标签。[发布工作流](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) 检查标签，并在测试工作流通过后发布。

## NSH

NSGo 把 [Native Shell](https://github.com/longbridge/nsh) 作为设计参考，测试都不依赖它。需要阅读时，将它克隆到仓库旁边，并检出它的最新发行版标签，不要用 `main`：

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## 下一步

- [代码结构](contributing/codebase.md)：NSGo 的各部分在哪里。
- [提交改动](contributing/changes.md)：一个 pull request 需要什么。
