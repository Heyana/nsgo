---
title: 运行与调试
description: 在任意目标上运行应用并热重载，查看它的错误、更新、布局、性能和其他语言下的样子。
group: 工具链
order: 20
---

## 运行

```sh
go tool nsgo dev
```

`dev` 为本机构建应用并打开它，或者为 `nsgo.yaml` 中 `dev.target` 指定的目标构建。每次保存 Go 文件，它都会重新构建并重启应用；应用导入的包所在的 module 不在 `go.mod` 或 `go.sum` 中时，会先执行 `go mod tidy`；只改了 TSX 时，会在运行中的应用里原地更新，并保留状态。应用退出时运行结束。

在 macOS 和 Windows 上，同一次 `dev` 会话中的原生开发构建会按主视图窗口上次的普通位置和大小重新打开该窗口。Windows 还会恢复全屏状态。

`-target` 在别处运行，`-device` 按名称、UDID 或序列号选择 iPhone、Apple TV、Apple Watch、模拟器或 Android 设备：

```sh
go tool nsgo dev -target web                # 在浏览器中，打开它打印的地址，端口为 -port（默认 8080）
go tool nsgo dev -target ios_simulator      # 在 iOS 模拟器中
go tool nsgo dev -target ios                # 在连接的 iPhone 上，由 Xcode 签名
go tool nsgo dev -target tvos_simulator     # 在 Apple TV 模拟器中
go tool nsgo dev -target watchos_simulator  # 在 Apple Watch 模拟器中
go tool nsgo dev -target android            # 通过 adb 在连接的设备或模拟器上
```

开发服务器默认监听所有网络接口。用 `-host 127.0.0.1` 只监听回环地址，Android 仍可通过 `adb reverse` 连接；也可以指定某个本机网卡的 IP 地址，只在该地址监听。要在 Apple 真机上使用开发功能，请选择设备能通过局域网访问的地址。

`-h` 列出全部参数。

## Go 调试器

把与你的 Go 版本兼容的 [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) 放到 `PATH` 上，命令名为 `dlv`。用下列命令运行原生桌面应用，保留调试信息并关闭 Go 优化。

```sh
go tool nsgo dev -target native -debug
```

从 Delve 的 `API server listening at` 输出中复制端口。在装有 Go 扩展的 VS Code 中添加用于连接调试器的配置，把 `40000` 换成这个端口。

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

在 GoLand 中添加 **Go Remote** 运行配置，主机填 `127.0.0.1`，端口填同一个值。在应用的 Go 代码中设置断点，连接调试器，再执行会运行到该断点的操作。Go 重编译成功后，连接同一地址，让调试器把断点发给新进程。TSX 改动仍走热重载。

macOS、Windows 和 Linux 可用 `-target native -debug`。用开发构建调试，用 `-release` 测量发布构建。

## 组件预览

在组件所在的可导入 Go 包中定义一个或多个 `PreviewXxx` 函数。在文件顶部用构建标签标记这些预览。

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

勾选要同时查看的预览，然后选择语言区域、主题、浅色或深色外观、视口尺寸和布局方向。预览使用最近的应用的 `nsgo.yaml`、翻译目录、配置的 `theme` 和资源。也可在参数后指定同一个 Go module 内的应用目录。这时预览路径相对于该目录。

保存文件会重新构建预览，保留已选的组件和显示设置。`nsgopreview` 标签将预览排除在普通发布构建之外。在使用同一标签的 `_test.go` 文件中用 `nsgotest.Start(t, PreviewEmpty)` 复用预览，然后用 `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` 运行测试以生成原生截图。

使用 `-target ios_simulator` 首次安装主应用后，只改 Go 代码会替换已安装的可执行文件并重新启动应用。修改 `nsgo.yaml`、打包资源或应用图标会重新安装。
## 错误

构建失败时，错误显示在运行中的应用上方，带有复制错误和重新构建的按钮；修好文件后，下一次保存就会替换它。组件渲染时 panic 也以同样方式显示。在应用里按 Command+R（非 Apple 平台为 Control+R）随时可以重新构建并重启应用，错误显示时也可以，除非应用自己绑定了这个键，无论是用 `cx.OnKey` 还是作为应用菜单里一个启用的菜单项；在浏览器里这个键照常刷新页面。应用自己的输出（比如 `log.Print`）出现在终端里工具输出的下方。

运行前，`go tool nsgo check` 会检查应用并对它的 TSX 做类型检查；加上 `-target all` 则改为为每个目标检查，这样在 Mac 上也能检查到其他平台构建标签下的代码。

### Apple 应用包的日志

在 macOS、iOS、tvOS 和 watchOS 上，默认 Go 日志器会写入 Apple 统一日志，包括 `log.Print`、NSGo 诊断和 TSX 控制台输出。在 Console 中，以运行应用的包标识符筛选 subsystem，以 `nsgo` 筛选 category。macOS 也可以查询日志存储：

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

记录使用 default 日志类型，保留原有的警告或错误前缀。日志文本为公开内容，请勿写入凭据或其他秘密。长消息会拆为多条记录。开发时仍可在终端查看输出；自定义日志 writer 或 `slog` handler 自行管理输出。

### Linux 网页视图

Ubuntu 24.04 及更新版本可通过 AppArmor 限制非特权用户的命名空间。bubblewrap 无法启动 WebKitGTK 沙箱时，NSGo 会记录错误并显示占位内容。一些 WebKitGTK 版本遇到这种情况时会在打开第一个网页视图时以 `SIGTRAP` 退出。[Ubuntu AppArmor 文档](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

优先使用发行版提供的 bubblewrap AppArmor 配置，例如 `bwrap-userns-restrict`。否则管理员可以参考 [NSGo Linux CI 使用的配置](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml)，为 `/usr/bin/bwrap` 加载允许 `userns` 的配置。该配置适用于所有调用系统 bubblewrap 的应用，由管理员管理。

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` 会关闭 WebKit 的子进程沙箱，只适用于使用可信内容的本地开发。

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` 会在浏览器中打开 DevTools。它由 `nsgo dev` 提供，适用于每个目标平台，手机上的应用也可以。它会实时展示运行中的应用：

- **Elements**：以树形展示视图。每个视图都标出构建它的组件、在编辑器中打开对应代码行的链接、测试用来查找它的选择器、它的 classes、位置和尺寸、布局后的外边距、边框和内边距，以及它设置的样式。鼠标悬停在某一行上，会像浏览器的 DevTools 一样在应用里画出它的外边距、内边距和内容；按下 **Pick** 后在应用里点一个视图，就能在树中找到它。
- **State**：按拥有它们的组件和计算列出代码创建的 signal 及其当前值、memo 的值，以及读取每个值的计算个数。没有计算读取的值，改动它不会改变屏幕上的任何内容。字符串、布尔值和数字可以直接修改，不用改代码就能试一种状态。NSGo 自己创建的值放在 **NSGo's own** 开关后面。
- **Profiler**：每次提交的耗时，拆成提交前你的代码、提交本身（含布局）和在平台主线程上应用这三部分。它还列出写入导致这次提交的代码行，展开后可以看到这次提交运行的计算，耗时最长的排在前面。另一个列表按创建它们的代码行，列出过去一秒中运行最多的计算。Android 的运行时在自己的 Java 代码里应用批次，因此 Android 的提交没有平台耗时。
- **Recorder**：把你在应用里的按压、输入、清空和滚动记录成一个 `nsgotest` 测试文件，复制到 `_test.go` 文件即可使用。在 Elements 中，**Assert shown** 和 **Assert text** 会为选中的视图添加检查。文字检查记录的是应用当时显示的语言下的文字，因此录制时请使用测试运行时的语言。
- **Highlight updates** 会像 `-highlight` 一样框出每次提交改动的视图。
- **Layout** 在设备上显示或隐藏[布局审计](#布局)，与 `-layout` 启动时显示的相同。

TSX 应用的状态、计算和写入会通过打包产物的 source map 以 TSX 代码行命名，并链接到应用中的文件。

DevTools 通过 `nsgo dev` 的服务器读取应用，这个服务器监听你所在的网络，以便手机连接。每次运行都会生成一个令牌，`nsgo dev` 为 DevTools 和 web 目标打印并打开的地址在 `#` 之后带着它，服务器只回答带着令牌的请求，所以请从这些地址打开 DevTools 和 web 目标。拿到地址的人都能在应用运行时修改它的状态，还能往 Recorder 生成的测试里添加步骤，所以运行录制的测试前请先读一遍。同一网络中的其他人和你浏览的其他网站的网页都做不到这些。

## 更新

```sh
go tool nsgo dev -highlight
```

`-highlight` 会把每次提交改动过的视图框出来半秒，使用应用时就能在屏幕上看到哪次更新重绘了超出预期的内容。这些框画在应用及其模态视图之上。应用运行时可以在 DevTools 中开关它们。

```sh
go tool nsgo dev -trace
```

`-trace` 打印运行中应用的每次提交：序号、耗时，以及它发出的挂载项，这样能看到一次更新创建、修改或移除了哪些视图。挂载项上方会列出这次提交的起因：自上次提交以来写入 signal 的代码行（写得最多的排在前面），以及运行了多少个绑定、effect 和 memo。

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

某一行写了很多次，或者一次写入触发了大量绑定重算，就是该合并写入（`cx.Batch`）或缩小绑定读取范围的地方。`go tool nsgo test -trace` 会在测试的每一步下打印同样的内容。

## 运行时性能分析

使用 `nsgo dev` 采集运行中应用的 Go CPU 活动、堆快照或执行跟踪。应用将采集结果发给开发服务器，由运行命令的机器保存。

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

桌面、iPhone 和 iOS 模拟器也可以采集 CPU。iPhone 提示本地网络权限时请允许。也可以用 `go tool nsgo dev -profile trace` 记录调度器和 goroutine 事件，再用 `go tool trace` 打开 `.nsgo/profiles/execution.trace`。CPU 和执行跟踪默认采集 10 秒，`-profile-output` 指定本机输出路径。组件预览的默认路径和相对路径基于应用目录。开发命令保存第一份完整采集结果后，应用会继续运行。

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

Go 在浏览器中支持堆快照和执行跟踪。浏览器 CPU 采样可使用其性能工具。`-profile` 和 `-release` 二选一。`nsgo build` 的发布产物不包含开发采集客户端。[Go 运行时诊断](https://go.dev/doc/diagnostics)

## 布局

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` 启动应用时在它上方显示 DevTools 的布局审计层，勾出每个视图的框：边框盒为蓝色，内边距的内缘为绿色，外边距的外缘为橙色；被滚动视图或裁剪内容的视图遮住的部分不画。`grid` 画出间距 8 点的网格，`grid=N` 画出间距 N 点的网格，N 至少为 4。取值要写在 `=` 之后：只写参数本身表示 `bounds`。

底部角落的读数显示窗口尺寸（单位为点），旁边的按钮用来开关这些框、网格和测距尺。在 Apple TV 等仅遥控器的设备上，焦点可以到达读数里的按钮，用遥控器即可直接切换。测距尺打开时，点按不会传给应用，而是用来选取两个点，读数显示两点之间的水平和垂直距离；第三次点按重新开始。在 Apple TV 上或使用键盘时，方向键（或滑动 Siri Remote 触控板）会移动屏幕上的十字准星，按选择键、回车或空格即可选点，选取第一个点后会实时显示距离直到第二个点锁定。

| 按键 | 作用 |
| --- | --- |
| `Cmd+Option+Shift+L`、`Ctrl+Alt+Shift+L` | 显示或隐藏审计层，仅限带 `-layout` 的构建 |
| 方向键、遥控器方向键 / 滑动 | 按网格步长移动测距尺准星（默认 8 点） |
| `Shift` + 方向键 | 以 1 点精细移动测距尺准星 |
| 选择键、`Return`、`Space` | 测距时选取准星所在点 |
| `Esc`、返回、Menu | 退出测距 |

应用自己也绑定了这个按键时，以应用的绑定为准。用不了这个按键时（例如在手机上），同时加上 `-devtools`，用 DevTools 的 **Layout** 显示或隐藏审计层；读数挡住角落里的控件时，先隐藏审计层。框会跟随 transform、布局动画、过渡和粘性偏移移动，滚动时会比内容晚一帧跟上。每次更新都要重画这些框，会增加耗时，所以测量性能时请把它们关掉。

## 性能

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` 在 macOS、iOS、Windows、Linux 和 Android 上于应用上方显示性能监视器。它测量帧和提交的开销，Android 还显示原生文本请求的往返耗时。开发构建带有开发客户端；测量时请加 `-release`，它与 `nsgo build` 的构建方式相同。

## 语言与外观

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # 加重音并拉长，找出未翻译和被截断的文字
go tool nsgo dev -lang ar-XB   # 从右到左
```

`-lang` 以某个语言区域启动应用，也可以是伪语言区域（[本地化](localization.md)）。

## 智能体

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
```

`-automation` 让 AI 智能体或端到端测试工具像人一样使用运行中的应用（目前支持 macOS、Android（`-target android`）以及 iOS、tvOS 和 watchOS 的真机与模拟器，通过 `-target ios`、`tvos`、`watchos`、`ios_simulator`、`tvos_simulator` 或 `watchos_simulator`）：列出窗口、概括某个窗口显示的内容、点击、输入、按键、滚动、拖动、悬停、选择菜单项、回答提示框、等待内容出现以及截图。智能体通过 `go tool nsgo mcp` 以 MCP 连接：它使用当前目录正在运行的 `dev -automation`，没有时就用相同的参数（如 `-target android`）启动一个，并在客户端断开时停止它：

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

其他客户端使用同样的命令：在应用目录中运行 `go`，参数为 `tool nsgo mcp`。视图用[测试](testing.md#一个测试)的选择器命名，或者用快照打印的 `view:` 引用；保存文件会重启应用，之前的引用随之失效。

智能体操作的是应用的真实数据，与 `dev` 运行时相同。只有 `dev -automation` 的构建带有它，也只有本机上持有 `.nsgo/automation.yaml` 中令牌的工具能连接它，而这个文件只有你能读取。

## 所有构建都接受的参数

应用的每个构建（包括发布包）在启动时都接受 NSGo 自己的参数。它们优先于 `nsgo.yaml` 和应用代码中的设置，也不会传到应用自己的参数解析中：

| 参数 | 作用 |
| --- | --- |
| `--nsgo-fps` | 在 macOS、iOS、Windows、Linux 和 Android 上显示性能监视器 |
| `--nsgo-lang=zh-Hans` | 以某个语言区域启动应用，也可以是伪语言区域 |
| `--nsgo-appearance=dark` | 以 `light`、`dark` 或跟随系统（`system`）的外观启动应用 |
| `--nsgo-os=ios` | 让 Web 构建以 `macos`、`windows`、`linux`、`ios` 或 `android` 运行，`web` 表示按本身运行 |

```sh
./Hello --nsgo-fps                                   # Windows 和 Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

在 Web 上，这些参数写在页面的查询字符串中：`?nsgo-fps&nsgo-lang=de`。

`--nsgo-os` 用来在浏览器里预览别的平台的布局：应用读到的是那个系统和对应的设备类型（iOS 和 Android 是手机，其余是桌面），`Env.Simulated` 返回 true。浏览器里没有那个平台的原生部件：`NavHeader` 和自绘的标签栏代替系统的，没有应用菜单、原生弹出菜单（`desktop.PopupMenu`）和窗口功能，`mobile.IsAvailable` 和 `desktop.IsAvailable` 都是 false。`ui.ContextMenu`、`ui.Menu` 这类自绘菜单照常可用。快捷键跟随访问者自己的键盘。其他构建忽略这个参数，并打印一条警告。
