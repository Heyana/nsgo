---
title: 终端
description: 运行程序或在终端网格中显示远程会话。
group: 核心概念
order: 63
---

`base/terminal` 提供会话 `Terminal` 及其无样式的 `View`；`ui/terminal` 以 `ui` 的外观包装 `View`，为默认上下文菜单和触控按键栏着色。桌面运行本地程序；iOS 和浏览器使用 `Options.Conn`，即连接远程程序的 `io.ReadWriteCloser`。应用持有会话，并在结束时关闭它。

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` 选择默认方块、竖线或下划线光标。`CursorBlink` 启用默认闪烁，零值仍为常亮。程序通过 DECSCUSR 或 DEC 模式 12 覆盖形状和闪烁状态。未聚焦时光标为空心且常亮；输入法预编辑和减少动态效果时也常亮。平台动画时钟驱动闪烁，不需要引擎逐帧重画终端文本。

`View.Font`、`Theme` 和 `DarkTheme` 设置字体和调色板。网格自行绘制 Unicode 制表符、块元素和 U+E0B0–U+E0B7 的 Powerline 分隔符，将相邻单元格对齐到设备像素。其他文本使用平台字体及其后备字体。

`MinimumContrast` 根据单元格实际背景提高普通文本的对比度，包括反色、暗淡文本和选区。1–21 使用 WCAG 对比度；零值保留颜色。图形和表情符号保持原色。设置 `Transparent` 后，对比度仍以终端配置的背景为准。

`Padding` 以点为单位设置网格外的留白。`PaddingColor` 选择其背景：

| 值 | 行为 |
| --- | --- |
| `PaddingBackground` | 终端背景，包括 OSC 11 的修改 |
| `PaddingExtend` | 最近单元格的背景；主屏幕中，提示符行、Powerline 行或含默认背景单元格的行不会向上、下延伸 |
| `PaddingExtendAlways` | 延伸所有边缘，不应用上述例外 |

TSX 应用通过副作用导入 `tsx/terminal`，将 `Terminal` 注册到 `nsgo/base`。对应属性为 `cursorBlink`、`minimumContrast` 和 `paddingColor`；后者接受 `background`、`extend` 或 `extend-always`。外观属性变化时，会话仍继续运行。

## 在浏览器中

应用内嵌固定版本的 libghostty-vt WebAssembly 模块，使用相同的终端视图和输入路径。构建应用无需 Zig、额外安装原生库，也不需要在运行时下载库。

将浏览器的 WebSocket 适配为 `io.ReadWriteCloser`：发送二进制字节，并把收到的消息合并或拆分为连续字节流。认证、重连以及尺寸和控制消息由应用与服务器决定；NSGo 只处理终端字节。[浏览器验收用例](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) 提供了包含关闭处理的完整字节流适配器。

## 动作与缩放

`View.Keybinds` 用 `Keybind{Key, Action, Amount}` 覆盖默认快捷键。键名与 `Scope.OnKey` 相同，例如 `mod+=` 或 `ctrl+shift+c`；后面的绑定优先。只有当前终端输入框获得焦点且输入法未在组词时，绑定才生效。

动作包括 `CopyToClipboard`、`PasteFromClipboard`、`SelectAll`、`ClearScreen`、`IncreaseFontSize`、`DecreaseFontSize`、`ResetFontSize`、`SetFontSize` 和 `Ignore`。窗口和标签页动作放在应用自己的键位表中；`Ignore` 消费按键而不向程序发送输入。

默认 `mod+=` 和 `mod+shift+=` 增大一磅，`mod+-` 缩小一磅，`mod+0` 重置为 `View.Font.Size`（未设置时为 13）。增大和缩小时 `Amount` 表示步长，零表示一磅；`SetFontSize` 将其作为字号。字号限制在 1–255 磅，忽略非有限值。缩放保留会话和外层视图，更新网格行列并调整程序的终端尺寸。

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` 可被控件响应式读取。`Ref.SetFontSize(size)` 和 `Ref.Perform(action, amount)` 与事件处理器一样在应用引擎上执行；`Perform` 返回动作是否可用。卸载后的 ref 字号为零，拒绝执行动作。

## 上下文菜单

`base/terminal.View` 右键默认打开无样式的 `base.ContextMenu`，包含本地化的复制、粘贴、全选和清空。`ui/terminal.View` 以 `ui` 主题颜色为该菜单及触控按键栏提供样式，并保留应用自定义样式的覆盖。复制需要已有选区；清空移除提示符上方内容，在备用屏幕禁用。触摸屏沿用上下文菜单的长按展示。用 `MenuClass`/`MenuStyle`/`MenuStyleFunc` 设置菜单表面，用 `MenuItemClass`/`MenuItemStyle`/`MenuItemStyleFunc` 设置菜单项，并用 `MenuItemIcon` 为菜单项提供前导图标。

`RightClickAction` 还可设置为 `RightClickPaste`、`RightClickCopy`、`RightClickCopyOrPaste`（有选区时复制，否则粘贴）或 `RightClickIgnore`。Ignore 把事件留给外层应用菜单；自定义带样式的 `ui.ContextMenu` 可通过 ref 调用相同动作。

导入 `tsx/terminal` 后可在 `nsgo/base` 使用 `<Terminal>`。响应式 `keybinds` 和 `rightClickAction` 属性更新视图并保留会话。动作使用 Ghostty 名称，例如 `increase_font_size` 和 `copy_to_clipboard`；右键策略为 `context-menu`、`paste`、`copy`、`copy-or-paste` 和 `ignore`。菜单样式属性使用 Go 名称对应的小驼峰写法。

## 鼠标上报

程序启用鼠标跟踪后，会按所请求的模式接收按下、释放、拖动和悬停事件，由 libghostty-vt 按它选择的协议编码。按住 Shift 可绕过点击和拖动捕获以选择文字。`View.MouseReporting` 是可选的响应式 `func() bool`，nil 表示 true；返回 false 后，即使程序请求捕获，也会保留视图的文字选择和原生回滚区滚动。启用上报时，程序的右键事件优先于外层上下文菜单。

备用屏幕启用 alternate scroll 且未请求鼠标跟踪时，纵向滚轮输入会变成方向键，并跟随普通或应用方向键模式。`MouseScrollMultiplier{Precision: 1, Discrete: 3}` 为触控板和滚轮设置默认上报倍数；零值采用这些默认值，正数限制在 0.01 到 10,000。小数输入会积累到完整步数后发出。普通回滚区继续使用平台原生滚动和惯性。

导入 `tsx/terminal` 后可使用 `nsgo/base` 的 `<Terminal>`。`mouseReporting` 默认为 true；`mouseScrollPrecision` 和 `mouseScrollDiscrete` 设置倍数。

