---
title: 小组件
description: 在 iPhone 和 iPad 的主屏幕与锁屏、Mac 的桌面上显示应用的视图。
group: 平台功能
order: 47
---

## 一种小组件

无论应用是否在运行，小组件都展示它的摘要：iPhone 和 iPad 的主屏幕与锁屏、Mac 的桌面和通知中心，以及 Android 启动器。一种小组件是系统小组件库中的一项，有它支持的尺寸：

```yaml
widgets:
  - kind: counter
    name: Counter                 # 显示在系统的小组件库中
    description: Shows the count.
    families: [small, medium, circular]  # 另有 large、extra_large、rectangular 和 inline
# apple: { app_group: group.dev.example.hello }  # 共享 App Group，默认为 group.<id>
```

Apple 平台的小组件由扩展绘制，`build` 和 `dev` 用 Xcode 的 Swift 编译器把它编进应用（[构建与发布](building.md#小组件与实时活动)）。Android 由预编译的 Java 运行时将同一时间线绘制为 App Widget。应用停止后，两者仍显示已发布的内容。

Mac App Store 的沙盒构建使用 `apple.app_group`，并通过 `sign.macos_profile` 和 `sign.widgets_profile` 提供 macOS 描述文件。应用和 `<id>.widgets` 必须属于同一团队，并获准使用该组。时间线与图片写入共享容器；直接分发和开发构建继续使用原有主目录路径。[商店签名](building.md#商店包)介绍构建选项。

`name` 和 `description` 可以使用应用目录中的消息键。这样小组件库和占位内容会使用用户的语言，缺少翻译时回退到源语言目录中的文本。不是消息键的值会按原文显示。

## 时间线

`widget.Update` 写入一种小组件从现在起显示的内容，由若干条目组成时间线：系统显示日期已过的最后一条，之后的更新在各自的日期显示，不需要应用运行。`Date` 为零表示现在。没有条目时，小组件显示这种小组件的名称和描述。Apple 平台的应用在前台时可以任意多次更新小组件，在后台时每天有几十次。

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` 为某个尺寸显示它自己的视图，代替 `Entry.View`；`Entry.Background` 填充小组件在系统留出的边距之外的背景；`Entry.URL` 在点击小组件时用应用打开。没有编入小组件时 `widget.Update` 返回 `widget.ErrNoWidgets`，在 macOS、iOS 和 Android 之外返回 `widget.ErrUnsupported`，`widget.Available` 可以在写入前判断属于哪种情况。它可以在任意 goroutine 上调用，在 `nsgo.Run` 之前也可以。

## 视图

小组件的视图不是 core 的元素：它们按 SwiftUI 的方式布局，视图默认和内容一样大，除非 `Style` 指定尺寸或让它撑满；容器默认把内容居中，除非 `AlignItems` 和 `Justify` 另有指定。`Text`、`Symbol` 和 `Image`（来自字节，或来自 `Update` 读取的文件）显示内容，`Column`、`Row`、`Stack`、`Spacer` 和 `Divider` 排列内容，`Rectangle`、`Progress` 和 `Gauge` 负责绘制，`Date` 用系统的格式显示时间，其中 `Relative`、`Offset` 和 `Timer` 会自己持续计时，不需要为每一秒写入条目。

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` 提供间距、内边距、尺寸或撑满、对齐、带圆角的背景、系统的文字样式或字号、字重、字形和变体、行数限制，以及用应用打开的 `Link` 和把视图列入系统着色的 `Accented`。`widget.Primary`、`Secondary` 和 `Accent` 跟随外观，`RGB` 和 `RGBA` 可以用 `Dark` 指定深色模式下的另一种颜色。

## 按钮

从 iOS 17 和 macOS 14 开始，`widget.Button` 会运行 `widget.OnAction`，不打开应用；旧系统只显示它的子视图。在 `main` 中、`nsgo.Run` 之前注册处理函数：系统可能在后台启动应用，不带任何视图。小组件按钮的 `Activity` 为 `nil`，`Action` 是按钮设置的值。

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

在处理函数返回前处理动作并调用 `widget.Update`，系统自动重载时间线时才能读取更新。处理函数不在视图的 goroutine 上运行，改变视图时使用 `core.Scope.Post`。它与[实时活动](live-activities.md) 使用同一处理函数和意图，`nsgo.yaml` 只需声明 `widgets`，无需启用 `live_activities`。

## Android

Android 启动器中的 App Widget 使用与 Apple 相同的时间线，由预编译的 Java 运行时绘制。应用停止后，已发布的内容仍然显示。`widget.Update` 和 `widget.Available` 支持 Android；应用未配置组件时返回 `widget.ErrNoWidgets`，其他不支持的平台返回 `widget.ErrUnsupported`。

从启动器的组件选择器添加并调整大小。渲染器从声明的系列中选择：宽高均低于 240 点时为 small，宽度从 240 点起为 medium，高度从 240 点起为 large，宽度从 500 点起为 extra-large；未声明该系列时使用声明的第一个系列。`Entry.URL` 和 `Style.Link` 通过 `url_schemes` 声明的协议打开应用，内层视图的链接优先于容器链接。

未来的时间线条目通过 Android 的非精确闹钟更新，系统可能延迟后台更新。计时器使用原生 Chronometer，无需应用运行也能计时；目前支持字号和颜色，自定义字重、字体设计和斜体仍是目标。相对日期随系统调度刷新。通用渲染器支持文本、图片、布局、形状、进度条、环和一组常用符号。需要跨平台一致的图形请使用 `Image`；完整的 SF Symbols 名称支持仍是 Android 的目标。组件提供程序只为独立 APK 和 AAB 生成，嵌入式宿主不包含它们。

## 实时活动

同一个扩展还负责绘制应用的实时活动，显示在锁屏和灵动岛里（[实时活动](live-activities.md)）。

## 下一步

- [实时活动](live-activities.md)：应用在事情发生过程中跟随的内容。
- [通知](notifications.md)：与小组件并用的通知和图标角标。
- [构建与发布](building.md)：扩展共享的 App Group。
