---
title: 测试
description: 像用户一样测试应用，可以无窗口运行，也可以在原生窗口中运行。
group: 工具链
order: 21
---

## 一个测试

`nsgotest` 无需窗口即可启动应用。测试按文本或角色找到控件，并像用户一样操作它们：

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestPress(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Press("text:Press me")
	app.Find("text:Pressed 1 times")
}
```

当测试在某个目标上运行时，`TestMain` 会把主线程交给平台。

需要进程级初始化和清理时，把它们放在 `MainFunc` 中 `m.Run` 的前后；测试进程退出前会执行延迟清理：

```go
func TestMain(m *testing.M) {
	nsgotest.MainFunc(func() int {
		setup()
		defer teardown()
		return m.Run()
	})
}
```

`Main(m)` 是 `MainFunc(m.Run)` 的简写。目标测试中的每个测试都在独立进程运行，因此这组初始化和清理会在每个测试进程中执行一次。


用应用函数运行的应用（`nsgo.Run(func(app *core.Scope) {...})`）用 `nsgotest.StartApp(t, fn)` 测试：它像 `Run` 一样在应用自己的作用域中运行该函数一次，因此应用菜单和退出处理器都会保留。使用桌面环境的 `nsgotest.Environment` 时，凡是 `nsgo.Run` 会打开窗口的地方（macOS，以及 Windows 和 Linux 上的应用函数），应用的主视图都显示在主窗口中。此时 `desktop.CurrentWindow` 和 `OnClosing` 都能工作；`app.CloseWindow()` 发出窗口关闭按钮的请求，并返回窗口是否关闭。单独测试一个处理器时，直接调用它并读取 `WindowClosing.Canceled()`，`OnQuitRequest` 的处理器则读取 `QuitRequest.Canceled()`。`app.RequestQuit(false)` 模拟用户发起的退出，`app.RequestQuit(true)` 模拟注销等强制退出。两者均可用于无头测试和 macOS、Windows、Linux 目标，获准的请求计入 `app.Quits`。`app` 上的步骤作用于主窗口。`app.Opened` 按请求的样子列出应用打开的窗口、面板和托盘视图（包括 `Transparent` 和 `SkipTaskbar`），`app.Window` 列出发给主窗口的命令。`app.DropFiles` 可以指定放置区域中的任意视图：外层最近的带 `OnFileDrop` 的视图接收文件。`app.OpenLinks("myapp://settings")` 在无头测试以及 iOS、Android 目标测试中，将应用自己的链接交给 `desktop.OnOpenLinks` 或 `mobile.OnOpenLinks`。`app.ReceiveShare(mobile.SharedItem{Text: "https://example.com", MIME: "text/plain"})` 在无头测试和 Android 目标测试中将分享交给 `mobile.OnShared`。视图的 `Bounds()` 是它在窗口坐标（点）中的框，与 `Ref.Bounds` 报告的一致。图片视图的 `Image.Source` 是它正在显示的来源，因此 `app.Find("id:video").Image.Source.Version` 和它的 `Pixels.Rect` 能说明哪一帧像素以什么尺寸到达了视图。无头运行时，`app.Press` 会像在目标上一样切换系统开关（`ui.SwitchSystem`），`app.Emit("id:switch", "change", true)` 则直接报告一个值。`app.Navigate` 和 `app.PostMessage` 能到达已挂载但隐藏的 Web 视图，因为它仍会继续加载。`View.WebCommands` 记录应用发送给 Web 视图的命令。可将 `Command` 与 `nsgotest.WebLoad` 或 `nsgotest.WebSendInput` 等值比较。

`app.Menu`、`app.DockMenu` 和 `app.PopupMenu` 提供 `nsgotest.MenuItem` 快照，其中包含标签、角色、勾选状态和子菜单项。调用对应的选择步骤并传入标签路径，即可选择菜单项。

`app.Surfaces()` 列出已打开的窗口、面板和托盘视图，主窗口排在第一位，其余按打开顺序排列。`app.Surface("title:Settings")`、`app.Surface("id:settings-root")` 和 `app.Surface("index:1")` 分别按当前标题、根视图的测试 ID 和列表位置选取一个界面。选择器必须恰好匹配一个打开的界面。

界面的 `Press`、`Type`、`Key`、`Find` 和 `Query` 在对应窗口执行，步骤会等待整个应用完成更新。`Config` 返回打开时的 `nsgotest.SurfaceConfig`，`app.Opened` 也记录了这份配置。`Screenshot` 在原生目标上截取该界面。`Close` 通过 `OnClosing` 请求关闭窗口，并返回窗口是否关闭。已关闭的句柄会使后续步骤失败，并从列表中移除。这些步骤适用于无头测试和 macOS、Windows、Linux 测试宿主。单界面的目标只列出主界面。

`Drag` 在指针从按下位置移动超过 8 点后会取消指针下的按压。Web 也遵循此规则，因此拖动回到原来的视图不会触发 `OnPress`。

`app.KeyCommands()` 列出当前焦点和可见视图下生效的带标题快捷键。在无界面测试和目标平台测试中，它都会返回按键、标题和分组。

`app.PressWith("id:row", core.Modifiers{Shift: true})` 为一次点击按住修饰键；`PressAtWith(selector, core.Point{X: 20, Y: 10}, mods)` 点击视图中的指定点。`Shift`、`Control`、`Alt` 和 `Platform`（Apple 上为 Command，其他平台为 Windows/Super 键）会通过无头模型和目标的原生输入传给 `OnPressDown`、`OnPressUp` 和 `OnPress`。仍遵循平台的原生约定：例如 macOS 上的 Chrome 会将 Control-click 转为上下文菜单。之后的 `Press` 或 `PressAt` 不带修饰键。watchOS 没有键盘修饰键输入，因此会拒绝带修饰键的点击。

点击可以直接指定文本框。在无头测试以及 macOS、Windows 和 Linux 上，可以在点击返回后立即读取 `app.Find("id:field").Focused`，包括点击处理函数聚焦文本框的情况。

`app.SelectTab("id:tabs", 1)` 选择已挂载的 `mobile.TabBar` 的第二项。无界面测试使用 iOS 或 Android 的 `Environment`，iOS 和 Android 目标使用原生标签栏。索引从零开始，必须对应现有选项。

`app.Focus(selector)` 像点击一样聚焦文本框；在桌面或 Web 的 `Environment` 中，或在这些目标上，它用 Tab 走到可按压或 `Focusable` 的视图，之后的 `app.Key("Return")` 会按下它。应用调用的 `Ref.Focus()` 遵循相同的停靠点：文本框在所有平台上都能聚焦，其他视图只在桌面、Web 和 Apple TV 上能聚焦，因此无头测试按钮的焦点需要使用这些环境之一。带有 `OnPress`、`OnPressDown`、`OnPressUp` 或 `OnLongPress` 任意一个的视图都算可按压，与各平台一致；在它上面按 Return 或 Space 会发送它处理的按下、抬起和按压事件，所以只有 `OnLongPress` 的视图能获得焦点，但没有按键能按下它。

无头运行默认使用手机 idiom，与项目的目标平台无关，因此 `cx.Env().Desktop()` 为 false，悬停提示不会出现。传入 `nsgotest.Idiom(core.IdiomDesktop)` 可以测试桌面上的呈现，环境的其余部分保持不变，包括 `Environment` 选项设置的值。键盘焦点取决于环境的操作系统，而不是 idiom。

在 macOS 目标测试中，测试窗口会忽略 AppKit 的屏幕尺寸限制，因此即使请求的内容高度超过屏幕可见区域，`nsgotest.Size` 和 `Resize` 仍保持请求的尺寸。

在真实目标上，`nsgotest.Environment` 固定外观、语言环境、文字大小、减少动画、增强对比度、热状态、低电量模式、登录时启动的原因、位于前台的应用及平台是否跟踪它，以及设备的名称和型号。其余部分保持目标自身的值：操作系统、idiom、安全区域和系统标题栏高度。因此，`Environment` 设置为 `core.OSWindows` 并不会让 `go tool nsgo test -target macos` 按 Windows 布局应用；这样的测试要么无头运行，此时整个 `Environment` 都会生效，要么使用 `-target windows`。`nsgotest.Idiom` 同样只在无头运行时生效。`nsgotest.Size` 在 macOS、Windows、Linux 和 Web 上设置测试窗口的大小；Android、iOS、tvOS 和 watchOS 保持设备屏幕的大小。在 `-target macos` 上，应用像 `nsgo.Run` 那样自己打开窗口，不需要 `Environment`：根视图或 `SetMainView` 设置的视图显示在主窗口中，`desktop.CurrentWindow` 和 `OnClosing` 在其中可用；主窗口默认为 480×640 点，除非 `nsgotest.Size` 另行指定；各个步骤作用于主窗口，与无头运行一致。通过关闭按钮或“关闭”菜单项的快捷键关闭主窗口时，会询问它的 `OnClosing` 处理函数；主窗口关闭后不再有可操作的视图，应用在最后一个窗口关闭时请求的退出计入 `app.Quits`，而不会结束测试。

没有绑定消费按键、也没有应用菜单项以该键为快捷键时，`app.Key("mod+a")` 会全选当前文本框的内容，`mod+c`、`mod+x` 和 `mod+v` 会通过 `app.Clipboard` 复制、剪切和粘贴。在 macOS、Windows 或 Linux 的 `Environment` 中，没有被绑定、聚焦视图或文本框消费的按键会选择以它为快捷键的已启用应用菜单项，因此 `app.Key("mod+r")` 可以测试菜单快捷键能否到达对应的菜单项：已禁用的菜单项不响应，文本框获得焦点时，`NotInText` 菜单项不响应，也不让后面同快捷键的菜单项响应；在 Linux 上，没有按住 Control、Alt 或徽标键的按键不会选择任何菜单项。在无头测试中，带有标准角色但没有 `OnSelect` 的菜单项，无论由 `app.ChooseMenu` 还是快捷键选择，都会像桌面平台那样执行：Close 经由主窗口的 `OnClosing` 处理函数关闭主窗口；Quit 询问 `OnQuitRequest` 处理函数，获准的请求计入 `app.Quits`；Minimize、Zoom 和 Toggle Full Screen 只改变主窗口环境里报告的窗口状态，应用的其他窗口保持不变（`app.SetWindowEnvironment` 可以单独改任意一个窗口的环境）；编辑角色作用于已聚焦的文本框。其他角色，以及文本框中的 Undo 或 Redo，会让该步骤失败，而不是悄悄吞掉按键；在 `-target` 下，这样的菜单项会跳过测试。在无头测试、桌面目标和 Web 上，`KeyAlways` 绑定会在文本框编辑之前消费 Return、Backspace 和全选；`KeyAuto` 和 `KeyNotEditing` 则把这些键留给已聚焦的文本框。Android 和 iOS 的测试宿主还不会先把这些键交给键位绑定。已聚焦视图的 `OnKeyDown` 会最先收到 `app.Key`，单独的修饰键（`app.Key("shift")`）也算；它消费的键不会再交给任何绑定，无头测试以及桌面和 Web 目标上都是如此。`app.Key` 是一次完整的按键，所以视图的 `OnKeyUp` 随后会收到该键抬起。`app.KeyDown` 和 `app.KeyUp` 分别按下和松开一个键，不接受组合键：`KeyDown` 按住的修饰键会加入之后的每个键，直到对应的 `KeyUp`，所以先 `app.KeyDown("shift")` 再 `app.Key("a")` 按下的是 Shift+A；`KeyUp` 一个没有按住的键会让该步骤失败。测试结束时仍按住的键会在那时松开，在 `-target` 下经由平台松开，所以收到按下的视图也会收到抬起，不会有键留到下一个测试。`core.KeysDown` 和 `KeyHeld` 读取它们按住的键。在 `-target` 下，Android、HarmonyOS 和 watchOS 会跳过用到它们的测试。`app.MouseDown`、`app.MouseDownAt`、`app.MouseDownWith`、`app.MouseDownAtWith` 以及对应的 `MouseUp` 方法可以分别按下和松开全部五个鼠标按键（从 `core.MouseButtonLeft` 到 `core.MouseButtonForward`），并更新 `core.MouseButtonsDown` 与 `core.MouseButtonHeld`；`app.BlurWindow()` 模拟窗口失去焦点并清空所有按住的键盘和鼠标按键。

在无界面测试中，`app.KeyCode("KeyW")` 按一次物理键位，`KeyCodeDown` 和 `KeyCodeUp` 分别按住和释放。设置 `core.Environment.KeyCodeNames`（例如 `map[string]string{"KeyW": "z"}`）可以测试其他布局。未指定的键位使用美式布局。

在 macOS、Windows、Linux 和 Web 上以 `-target` 运行时，`app.Select(selector, start, end)` 用原生文本框的编辑键（全选和方向键）移动它的选区，之后的输入、复制或粘贴都作用于测试指定的范围；消费这些键的绑定或应用菜单项会把它们拦下。手机目标还不能移动原生选区：在那里它只移动测试模型中的选区，文本框设置了 `OnSelectionChange` 时则跳过该测试。

在 Linux 和 Android 的无头测试中，可以用 `app.Installs` 检查交给安装器的软件包路径。设置 `app.InstallPackage` 可让模拟安装器返回错误。

`app.Orientation` 保存当前生效的 `Scope.RequestOrientation` 方向集合。使用应用原有的方向策略时，该值为零。

`app.Find(selector).Kind` 的类型是 `nsgotest.ViewKind`。需要检查挂载视图的形式时，可将它与 `nsgotest.KindText`、`KindInput`、`KindCanvas` 或 `KindGPU` 等常量比较。

无头 GPU 测试可用 `app.Surface("title:Inspector").GPUReadPixels("id:viewport")` 读取对应窗口的软件 GPU 像素。`Surface.TriggerGPUDeviceLost` 和 `Surface.RestoreGPUDevice` 模拟该窗口的设备丢失与恢复，并使用该窗口的像素比例。即使不同窗口中的视图 ID 相同，设备和回读回调也各自独立；`app.GPUFrames()` 记录整个应用的提交。这些辅助方法作用于无头软件 GPU 模型。

## 小组件和实时活动

`nsgotest.Widgets(true)` 记录应用通过 `widget.Update` 发布的内容。用 `app.Widgets()` 读取每类小组件的最新时间线。条目没有指定日期时，使用应用的墙钟时间。`nsgotest.Widgets(false)` 让 `widget.Available()` 返回 false，并让更新返回 `widget.ErrNoWidgets`。

`nsgotest.LiveActivities(true)` 记录实时活动的开始、更新、提醒和结束。`app.LiveActivities()` 按开始顺序返回活动，包含内容变化和最终内容。`nsgotest.LiveActivities(false)` 让开始活动的调用返回 `widget.ErrActivitiesDisabled`。这些选项在测试期间接管相应的进程级服务，在原生目标上也一样。调用这些服务的测试应按顺序运行。

应用锁定指针后，`app.PointerDelta(dx, dy)` 向锁定的视图发送相对位移。无头测试用 `nsgotest.Environment(core.Environment{CanLockPointer: true})` 启用此功能。无头测试和目标测试都可用 Escape 解锁。

## 运行测试

```sh
go tool nsgo test
```

`test` 每个测试输出一行。`-run` 像 `go test` 一样筛选测试。在目标上，`-timeout` 设置每个测试的截止时间（默认三分钟，`0` 表示不限制），到达时结果会注明。在模拟器或手机上，`TestMain` 调用 `nsgotest.Main` 的测试从测试宿主启动时开始计时，应用启动得慢不计入其中；启动另有五分钟的上限，超过时测试以启动超时失败。子测试全部跳过的测试计为跳过。`-junit report.xml` 写出保留每个跳过原因的 JUnit 报告，`-trace` 打印每一步的挂载项和提交耗时，`-bench` 改为运行基准测试，每个结果输出一行。`--` 之后的参数交给 `go test`。

传入没有 `nsgo.yaml` 的目录（例如库或模块根目录）时，`test` 会处理其下的所有包；目录也可以像 Go 的包模式那样以 `/...` 结尾。它只接受一个目录，遇到第二个会报错退出：要测试多个目录，就传入它们共同的上级目录，或逐个运行。`go tool nsgo check` 以同样的方式选取并检查包。

测试会检查响应式图的 goroutine 归属：信号和作用域应在应用的 goroutine 上访问，工作 goroutine 通过 `cx.Post` 调度。在应用空闲时从其他 goroutine 发起的调用会 panic，并提示使用 `Scope.Post`；其余情况由 `-race` 检出。引擎内部 panic 仍会使测试失败；发布的应用则停止受影响的引擎，并把堆栈报告给宿主。开发构建也检查归属，发布构建不包含这项检查；WebAssembly 的开发构建和测试构建不使用 goroutine ID 检查，因为浏览器回调可能在浏览器管理的 goroutine 上恢复；其他测试检查照常进行。

`app.Post(func() { state.Set(value) })` 把工作排到应用的响应式图所在的 goroutine 上，在下一个步骤或等待稳定时运行：无头运行时是测试 goroutine，在目标平台上是引擎 goroutine。可以从测试或桩出来的工作 goroutine 调用它，再用 `app.Settle()` 或等待读取结果视图。函数可以继续投递，同一个步骤会一并运行；它不执行步骤，也不调用 `t.Fatal`，因为在目标平台上它不在测试 goroutine 上运行。

无头运行时，每个步骤随后会逐帧推进时钟，最多 2 秒，直到有限动画结束、每个样式过渡都执行了它的 `OnTransitionEnd`：经 `EndingStyle` 关闭的对话框或弹出框在 `app.Press` 返回时已经消失。要观察中途的状态，用 `nsgotest.ManualFrames()` 启动；这时它们只随 `app.Advance` 推进的时钟移动。

## 时间

无论无头运行还是在目标平台上，应用都运行在 `app.Clock` 上，它是一个从 2026-01-01 00:00 UTC 开始的 `core.ManualClock`，只在测试推进它时才走动。`app.Advance(d)` 将它推进 `d`，每个计时器在到期时运行；应用等待显示帧时，每隔一个帧间隔运行一帧，和显示器一样。`app.FastForward(d)` 一次跳到前面，就像笔记本合上一段时间后再打开：途中到期的每个计时器在末尾各运行一次，显示帧也运行一次。

应用用 `cx.Now()` 读取这个时钟，用 `cx.WallTime()` 读取它显示的日期，所以应用自己测量的时间（例如手势速度、双击的时间窗口）要用它们来计时。`time.Now()` 和 `time.Sleep` 跟随系统时钟，`Advance` 不会推进它：用 `time.Now()` 计时的应用察觉不到测试推进的时间，测试里的 `time.Sleep` 只会让测试变慢。等待来自其他 goroutine 的工作（例如收到回复后的 `cx.Post`）时，用 `app.WaitFor`、`app.WaitForGone` 或 `app.WaitUntil`，它们的超时按真实时间计算。

无头运行时，这些等待在每次检查之间逐帧推进时钟。用 `nsgotest.ManualFrames()` 启动可在等待时保持时钟不动。在目标平台上，等待会让应用稳定下来，动画则由平台自行运行。

首屏依赖日期的界面（例如今天到期的内容）用 `nsgotest.WallTime(t0)` 启动：首次渲染时 `cx.WallTime()` 就是 `t0`，保留它的时区，计时器仍按时钟自身的时间运行。`nsgotest.Clock(c)` 让应用运行在测试自己的时钟上，供应用之外的代码共用；`c.Advance` 推进它时，应用的计时器随之触发。在目标平台上用 `nsgotest.Measure()` 测量时，应用改为运行在系统时钟上，见下文。

## 在平台上运行

测试可以在真实窗口中、用平台自己的输入运行。测试发现遵循目标平台的构建标签，即使包中也有跨平台文件，仍会包含仅适用于该目标的测试文件：

```sh
go tool nsgo test -target macos
```

在 macOS 上，测试窗口会忽略测试宿主以外的鼠标、键盘和触控板输入，因此测试运行时使用电脑不会改变测试场景。测试自身的操作仍然有效。

测试窗口无需激活应用就能收到测试的按键，因此你正在使用的应用会保持在前台。当其他应用处于活动状态时，AppKit 可能拒绝文本框获得焦点，`Focus` 和 `Type` 会说明原因。`go tool nsgo test -target macos -activate` 让测试应用在点击文本框或按键前激活并获得键盘。它需要未锁定的桌面会话，并要求测试导入 `github.com/nsgo-dev/nsgo`（空白导入即可），并在 `TestMain` 中启动 `nsgotest.Main` 或 `nsgotest.MainFunc`。

清理测试时会关闭仍打开的原生上下文菜单，避免吞掉下一个子测试的第一次点击或按键。在 macOS 上，WindowServer 截图包含测试窗口中打开的原生弹出菜单，裁剪到窗口范围内，不包含其他窗口。

可用的目标有 `macos`、`windows`、`linux`、`web`（在 Chrome、Chromium 或 Edge 中）、`ios`（在连接的 iPhone 上运行，连接多台时用 `-device` 指定，用 Xcode 上次使用的团队或 `-team` 指定的团队签名）、`ios_simulator`、`tvos_simulator`（方向键、作为选择键的 Return 和作为 Menu 键的 Escape 模拟遥控器）、`watchos_simulator` 和 `android`，或 `all`。每个测试在独立的进程中运行。`app.Capture()` 返回当前像素的 `image.Image`，`app.CapturePNG()` 返回 PNG 字节，无头运行时两者都会跳过测试。在 macOS 上，截图是 WindowServer 合成后的窗口，包含 Core Animation 的阴影、蒙版、滤镜、动画和插入符；无法截取窗口时（例如窗口已最小化或在其他空间中），改由视图通过 AppKit 自行绘制，不含这些效果，并在第一次这样做时记录日志。macOS 上的两种截图路径都会在编码前转换为 sRGB。iOS 和 tvOS 也使用 sRGB 截图，因此这些目标的像素值不会因显示器使用 Display P3 而变化。`app.Screenshot("en/library")` 会把截图保存为 `.nsgo/test/screenshots/<target>/en/library.png`，在 `macos` 上即 `.nsgo/test/screenshots/macos/en/library.png`；名称可以包含空格，但不能是绝对路径或包含 `..`。`-screenshots dir` 改为保存到 `dir` 下，`dir` 是相对于当前工作目录的路径。设备日志丢失或截断了部分内容的截图会让测试失败且不会写入文件，路径与之前的截图重复也会让该测试失败。`-fps` 会在 `macos`、`windows`、`linux` 和 `ios_simulator` 上每秒记录一次性能监视器的读数，在 `web` 和 `android` 上只记录引擎的读数。`-env KEY=VALUE` 为测试设置环境变量，手机上的测试只能这样拿到变量。在模拟器或手机上，没有 `nsgo.yaml` 的目录的测试会安装成以该目录命名的应用，ID 是 `dev.nsgo.` 加上目录名中的字母和数字，例如 `base/formula` 的是 `dev.nsgo.formula`。在 `android` 和各模拟器上，测试以应用 ID 加 `.nsgotest` 的标识安装（App Clip 也移到这个 ID 之下），与已安装的应用并存而不覆盖它，运行结束后卸载。在 `ios` 上，测试以应用自己的 ID 安装，因为描述文件只认这个 ID，会替换 iPhone 上已安装的版本及其数据。

在 `android` 和 `ios` 上，测试只能在 NSGo 的测试宿主中运行：Android 运行时从测试进程的标准输出读取协议，工具也要靠宿主写下的标记才能拿到测试的输出和退出码。要在这两个目标上运行的包，其测试必须链接宿主，即在某个测试文件中导入 `github.com/nsgo-dev/nsgo`（空导入即可），并在 `TestMain` 中用 `nsgotest.Main` 或 `nsgotest.MainFunc` 启动它。构建或安装之前，`nsgo test` 会检查是否链接了宿主、是否声明了 `TestMain`。如果该函数绕过宿主，Android 会报告 stdout 协议错误并提示调用 `nsgotest.Main`；宿主启动之前，日志和初始化输出应写到 stderr。像 `ui/chat` 这样的库包，在测试文件中补上这两项后即可在手机上运行，无头运行或在其他目标上运行则无需改动。Android 目标测试达到超时时，结果还会说明宿主是否写出了启动标记，并包含运行器清理日志前该测试的 `logcat` 尾部。

没有链接平台后端的包会在目标的运行时中无头运行。使用 `-target web` 时，库中的控件测试可通过这种方式在浏览器中执行。存储保留在内存中；浏览器没有文件系统，因此 `DataDir`、`CacheDir` 和 `LogsDir` 均为空。`nsgotest.MicrophoneInput` 的模拟录音器需要写入真实 WAV 文件，所以会在浏览器中跳过测试。

在 `web` 上，测试运行在工具找到的无头 Chrome、Chromium 或 Edge 中，也可以用 `NSGO_BROWSER` 指定浏览器的可执行文件。在 macOS 上，工具会优先使用已安装的 Chrome for Testing。工具最多等待 30 秒让浏览器启动，命令被取消时停止等待启动；浏览器提前退出或一直不响应时，会报告它的错误输出。

每个步骤和 `app.Settle()` 都会先等引擎完成待提交的更新，再等待连续 100 ms 没有新批次或有限动画。即使应用持续变化，整个等待也会在五秒后结束；首次构建较慢的对话框不需要测试额外投递任务或休眠。

在 Android 上，工具会在构建前和每个测试前唤醒休眠的屏幕，并关闭没有凭据的锁屏。设有 PIN、密码或图案的设备必须先解锁才能继续；错误消息会指出设备。临时的 `adb` 状态读取失败会自动重试；设备持续不可达时，目标测试会只报告一次设备丢失，而不会让其余测试全部失败。

要测量而不是检查时，用 `nsgotest.Measure()` 启动：应用运行在系统时钟上，`Advance` 会真实等待相应时长，各步骤只等待正在进行的动画停下。`app.Mark("name")` 标出此后的读数测的是什么，`app.Mark("-")` 结束标记；无论是否加 `-fps`，标记都会打印出来。

NSGo 的参数要写在目录之前：`go tool nsgo test -target web ./app`，而不是写在目录之后。无头运行时，`--` 之后的参数交给 `go test`。在目标上运行时，`--` 之后只支持 `-short`、`-count`、`-cpu`、`-parallel`、`-failfast` 和 `-fullpath`，例如 `go tool nsgo test -target web ./app -- -short -count=3`；其他 Go 测试参数会明确报错，包括此时应写在目录之前的 `-run` 和 `-timeout`。`-count=0` 不运行任何测试。链接 NSGo 原生宿主的测试按每个次数和 CPU 设置在新进程中重复运行，结果带 `[run N]`，截图保存在 `run-N` 子目录中，每次调用各有超时；库包保留 Go 在进程内的重复运行。

## TSX 测试

TSX 应用中用 `nsgo/test` 编写的 `*.test.tsx` 文件会随 Go 测试一起运行。

`open("App.tsx", { clock: {} })` 让应用运行在手动时钟上，由 `app.clock` 推进：`app.clock.runFor(ms)` 和 `app.wait(ms)` 一样逐帧推进，如同 `Advance`；`app.clock.fastForward(ms)` 一次跳到前面，如同 `FastForward`。`clock: { time }` 让 `Date.now()` 和 `new Date()` 从 `time`（一个 `Date` 或自 1970 年起的毫秒数）开始，之后用 `app.clock.setSystemTime(time)` 修改；两者都不移动计时器。不传 `clock` 时，无头应用的时间还会在 `waitFor` 和 `waitUntil` 等待期间逐帧流逝，和在设备上一样。

`open("App.tsx", { camera: true })` 为测试环境提供相机，使 `Picker.canTakePhoto()` 返回 `true`，并让 `Picker.takePhoto` 记录到 `app.picks()`（带有 `camera: true`）并由 `app.onPick` 回答：返回单个 `PickFile` 或 `PickFile[]` 作为拍下的照片，返回 `null` 或 `[]` 表示取消，抛出错误则让 `Picker.takePhoto` 拒绝。不传 `camera: true` 时，`Picker.canTakePhoto()` 为 `false`，`Picker.takePhoto()` 会拒绝。

不写测试文件也能驱动 TSX 应用：传入它的入口，并从标准输入提供步骤，写法与 `*.test.tsx` 测试的函数体相同，`app` 即该应用：

```sh
go tool nsgo test App.tsx < steps.ts
```
