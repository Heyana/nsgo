NSGo 是一个以 Go 为后端的跨平台原生 UI 引擎。应用的逻辑、响应式图和布局都用 Go 编写，每个平台用自己的原生视图绘制。

本站就是 gallery：由 NSGo 自己编译到 web。侧边栏中的每个页面都在这里实时运行，这份指南也由应用使用的同一个文本视图绘制。在 web 上，地址栏会记录当前页面，因此链接可以直接打开它。

## 开始

一个应用就是一个调用 `nsgo.Run` 的 Go main 包：

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
		core.Text{ContentFunc: func() string {
			return fmt.Sprintf("已按 %d 次", count.Get())
		}},
		ui.Button{Label: "按我", OnPress: func(core.PressEvent) {
			count.Update(func(n int) int { return n + 1 })
		}},
	}}
}

func main() { nsgo.Run(Root) }
```

在浏览器中运行，或在本机上用原生后端运行：

```sh
go tool nsgo dev -target web .
go tool nsgo dev .
```

`nsgo.yaml` 指定应用名称和 `nsgo build` 输出的目标。[README](https://github.com/oneo-me/nsgo/blob/main/README.zh-CN.md#用法) 介绍了最小的应用和工具，[ARCHITECTURE.md](https://github.com/oneo-me/nsgo/blob/main/ARCHITECTURE.md) 说明了引擎的包结构、平台与边界。

## 浏览

侧边栏把页面分组：概览包含欢迎页和这份指南，图标库包含 Lucide，NSGo 包含引擎拥有的无样式 **Base** 部件，然后是元素页面和由它们构建的主题 **ui** 组件。搜索可以从键盘找到任何页面，页面上的每个控件都可以操作。

在 Theme 页面可以一次改变整个 gallery 的主题，跟随系统外观或手动选择。

## 状态

NSGo 仍处于早期。[CHANGELOG.md](https://github.com/oneo-me/nsgo/blob/main/CHANGELOG.md) 记录每次变化新增的内容，[路线图](https://github.com/oneo-me/nsgo/blob/main/docs/roadmap.md) 排列接下来要做的事。
