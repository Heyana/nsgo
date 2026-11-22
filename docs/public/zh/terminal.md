---
title: 终端
description: 运行程序或在终端网格中显示远程会话。
group: 核心概念
order: 63
---

`base/terminal` 提供会话 `Terminal` 及其 `View`。桌面运行本地程序；iOS 使用 `Options.Conn`，即连接远程程序的 `io.ReadWriteCloser`。应用持有会话，并在结束时关闭它。

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
