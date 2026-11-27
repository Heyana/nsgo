---
title: 组件参考
description: 最常用的组件，以及它们的变体、状态和无障碍。
group: 组件
order: 30
---

这些是 `ui` 的组件，带有 NSGo 的外观并跟随主题。这份参考先覆盖应用最常用的组件，每页先给最短示例，再说明部件、状态和无障碍。gallery 覆盖所有组件，包括还没有页面的那些。

[gallery](https://nsgo.dev/gallery/) 可以在浏览器中运行每个组件的演示。

## 操作

- [按钮](ui/button.md)：普通按钮、强调色按钮，以及图标按钮。

## 文本与输入

- [文本](ui/text.md)：段落、span、链接、字体和文档文本。
- [输入框](ui/input.md)：平台的文本框，受控或非受控。
- [复选框](ui/checkbox.md)：选中、未选中或不确定的复选控件。
- [开关](ui/switch.md)：打开或关闭的设置。
- [选择器](ui/select.md)：从列表中选一个值，以弹出列表或 sheet 显示。
- [滑块](ui/slider.md)：沿轨道选择一个值或一个范围。

## 集合

- [列表](ui/list.md)：任意数量的行，只构建显示中的部分，支持选择与激活。
- [滚动](ui/scroll.md)：平台的滚动视图，支持分页、吸附、缩放和下拉刷新。

## 导航

- [标签页](ui/tabs.md)：标签列表、分段控件和系统标签栏。

## 覆盖层

- [对话框](ui/dialog.md)：模态表面、sheet 和系统警告框。
- [菜单](ui/menu.md)：从触发器打开的菜单、上下文菜单及其条目。

## 更多

gallery 覆盖其余组件：表格、图表、代码编辑器、富文本、终端、Dock、命令面板、颜色、日期与表情选择器（`ui.EmojiPicker`）、Toast 等。每个组件那里都有演示。

`base/terminal.View` 支持 Command+单击（Windows 和 Linux 上为 Control+单击）打开已有本地路径，与 URL 链接一样。相对路径按 shell 通过 OSC 7 报告的工作目录解析；本地 shell 尚未报告时使用 `terminal.Options.Dir`，未设置则使用主目录。含空格的文件名需加引号。远程目录主机和未报告本地目录的连接不会映射到本地文件。文件及文件夹通过 `Scope.OpenFile` 打开；URL 仍通过 `Scope.OpenURL` 和嵌入宿主的 URL 处理器。

`base/terminal.View` 默认拒绝程序读取或写入剪贴板。应用允许后，再设置 `AllowClipboardRead` 和 `AllowClipboardWrite`；仍需通过请求视图的 `AccessClipboard` 权限检查。TSX 的 `Terminal` 也有同名属性，修改它们不会重启会话。授权界面由应用提供。用户主动粘贴时，可独立向使用 Kitty 剪贴板协议（模式 5522）的程序提供文本/PNG 快照；一次性密码只能读取这份快照。未启用该模式的程序仍收到普通括号粘贴文本。剪贴板图像能力取决于平台服务；不提供 X11 主选择区。

## 可复制组件

顶层 `ui` 组件直接从 `github.com/nsgo-dev/nsgo/ui` 导入使用。`ui` 下的八个子包组件已声明在 NSGo 根目录的 `nsgo-module.yaml` 中，并收录在 `nsgo add -list` 索引里，应用既可以直接导入它们，也可以把它们复制到自己的目录中按需修改：

- `chat`（`ui/chat`）：对话视图、输入栏、消息操作与查找栏。
- `code-editor`（`ui/editor`）：基于 `base/editor` 的 `ui` 风格代码编辑器。
- `commit-graph`（`ui/commitgraph`）：基于 `base/commitgraph` 的提交列表与修订关系图。
- `diff-view`（`ui/diff`）：基于 `base/diff` 的 `ui` 风格差异视图。
- `file-picker`（`ui/filepicker`）：带侧边栏和面包屑的模态文件与文件夹选择器。
- `flow-diagram`（`ui/flow`）：基于 `base/flow` 的有向流程图与卡片节点。
- `rich-editor`（`ui/richeditor`）：基于 `base/richeditor` 的 `ui` 风格富文本编辑器。
- `terminal`（`ui/terminal`）：基于 `base/terminal` 的 `ui` 风格终端视图。

关于 `nsgo add` 如何复制组件、重写导入路径、生成多语言目录并记录来源，见[复制组件](copying-components.md)。

## 下一步

- [复制组件](copying-components.md)：将 `ui` 的子包组件或第三方组件复制到应用中。

- [组件与状态](components.md)：组件如何返回元素，signal 如何更新它们。
- [样式与主题](styles.md)：组件取用颜色的主题。
- [无样式组件](authoring/base.md)：同一批控件的无外观形式，用于自己设计外观。
- [无障碍](accessibility.md)：每个控件的标签、角色和值。
