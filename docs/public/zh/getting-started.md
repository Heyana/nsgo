---
title: 环境准备
description: 安装 NSGo 所需的环境，创建一个应用并在各个目标上运行。
group: 入门
order: 10
---

## 环境要求

- [Go](https://go.dev/dl/) 1.27.3 或更新版本。
- 为 iOS、tvOS 和 watchOS 构建需要装有 Xcode 的 Mac。其他目标只需 macOS 上的 Go；x86_64 Android 模拟器还需要 Android NDK。
- 在 iPhone 上运行需要 Xcode 的 Accounts 中有 Apple ID，iPhone 已信任该电脑，并开启开发者模式。
- 在 Android 上运行需要 Android SDK platform-tools 中的 `adb`。
- 在 Windows 上运行只需要 Go，它也能构建 Windows 和 Web 包。
- 在 Linux 上运行需要 GTK 4.14 或更新版本、Pango 和 Cairo。网络图片使用 libsoup 3，网页视图使用 WebKitGTK 6.0，音频和视频使用 GStreamer 插件。AppArmor 限制用户命名空间的系统上，[WebKitGTK 需要 bubblewrap 配置](debugging.md#linux-网页视图)。

## 安装 nsgo

安装一次 `nsgo` 命令，可以用 Homebrew：

```sh
brew install nsgo-dev/tap/nsgo
```

也可以用安装脚本，它在 macOS 和 Linux 上把命令放进 `~/.local/bin`，或 `$NSGO_INSTALL_DIR`；`NSGO_VERSION=vX.Y.Z` 指定版本：

```sh
curl -fsSL https://nsgo.dev/install.sh | sh
```

或者用 Go 安装，适用于任何系统，包括 Windows：

```sh
go install github.com/nsgo-dev/nsgo/cmd/nsgo@latest
```

`nsgo version` 打印安装的版本。

## 创建应用

让 `nsgo` 生成应用和它的模块：

```sh
nsgo init -module example.com/hello hello
cd hello
```

`init` 会写出：

| 文件 | 内容 |
| --- | --- |
| `app.go` | 应用，一个计数器：一个 `Root` 组件，以及调用 `nsgo.Run(Root)` 的 `main` |
| `app_test.go` | 一个按下按钮的测试 |
| `nsgo.yaml` | 应用的名称、id、版本和目标 |
| `go.mod` | 模块，名称取自 `-module`，未指定时取目录名；依赖运行 `init` 的那个 NSGo，并把它作为工具运行 |
| `go.sum` | 应用所需依赖的校验和，由 `init` 用 `go mod tidy` 填好 |

在已有的模块里，不带 `-module` 的 `nsgo init` 把应用写进当前目录；`go.mod` 缺少 NSGo 的 `require` 和 `tool` 行时会补上，其余内容不变。`nsgo` 的开发构建没有可依赖的版本，写出的 `go.mod` 不含这两行，并打印补上它们的 `go get` 命令。`nsgo init -template tsx` 会生成 TSX 应用，`-template mixed` 则生成一个入口用 TSX、内部使用 Go 组件的应用。

### 项目的 NSGo

`go.mod` 里的 `tool` 行把项目固定在它依赖的 NSGo 版本上；缺少这一行的模块可以用 `go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo` 写入。在项目里，版本不同的已安装 `nsgo` 会用同样的参数运行固定的那个，即 `go tool nsgo`，所以无论各人安装的是哪个版本，项目都用同一个 NSGo 构建；`nsgo version` 会说明运行的是哪一个（[模块的 nsgo](building.md#模块的-nsgo)）。`go tool nsgo` 也能单独使用，无需安装任何东西，比如在 CI 里，或者给没有安装 `nsgo` 的队友：

```sh
go tool nsgo test
```

添加库时用 `nsgo add <模块>` 代替 `go get`：它在写入之前先列出 `go.mod` 和 `go.sum` 将发生的变化，并且绝不会把项目的 NSGo 移到正式版本之外（[添加库](building.md#添加库)）。

### 来自 Go 模块的模板

传入模板的模块路径，可以附带 `@version` 和 `#name`。省略版本时，使用 Go 模块代理能找到的最新版本。

```sh
nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
nsgo init -template 'example.com/my-templates@v1.2.0#platformer' -module example.com/hello hello
nsgo init -template example.com/my-templates -list
```

发布时二选一：把单个起始项目放在模块的 `template/` 目录里，或者把多个起始项目分别放在 `template/<name>/` 下。`template/` 本身就是起始项目时，工具会把它连同子目录整个复制。用 `#<name>` 选择具名起始项目；有些 shell 会特殊处理 `#`，所以要给参数加引号。`-list` 每行打印一个名字，`template/` 本身是起始项目的模块则打印 `(default)`。每个起始项目里都要有 `nsgo.yaml` 和 `main.go`。工具用 Go 的 `text/template` 渲染这两个文件以及所有以 `.tmpl` 结尾的文件，并去掉输出文件名里的 `.tmpl`；其他文件（包括二进制资源）原样复制。

模板可以使用 `.Name`（应用的显示名称）、`.ID`（生成的应用 id）、`.Slug`（小写、以连字符连接的名称）和 `.Module`（`-module` 的值，未指定时为 slug；若 slug 与标准库包同名，如 `time`，则改用 `example.com/<slug>`）。`-module` 若给出标准库包路径会被拒绝。Go 下载模块时会排除含有 `go.mod` 的子目录，所以要用 `go.mod.tmpl` 声明生成的应用的模块和依赖。没有它时，工具会像内置模板一样写出 `go.mod`。无论哪种情况，工具都会补上 `go.mod` 缺少的 NSGo `require` 和 `tool` 行（保留模板依赖的 NSGo 版本），再运行 `go mod tidy`。

## 运行应用

```sh
nsgo dev
```

`dev` 为当前机器构建应用并打开它。每次保存都会重新构建并重启应用，只修改 TSX 时会原地更新正在运行的应用。构建失败会显示在应用上方，并提供复制错误和重新构建的按钮。

用 `-target` 在其他地方运行：

```sh
nsgo dev -target web                # 在浏览器中
nsgo dev -target ios_simulator      # 在 iOS 模拟器中
nsgo dev -target tvos_simulator     # 在 Apple TV 模拟器中
nsgo dev -target watchos_simulator  # 在 Apple Watch 模拟器中
nsgo dev -target android            # 在已连接的设备或模拟器上
```

`nsgo doctor` 列出每个目标在这台机器上能否构建、能否运行，以及不能时缺少什么，例如 Xcode、`adb`、已连接的设备或 GTK。

设备选择、追踪更新、性能监视器和其他语言见[运行与调试](debugging.md)。

## 使用编程智能体

NSGo 的技能（skill）教 Claude Code、Codex、Cursor 等编程智能体如何编写、测试和发布 NSGo 应用。在应用目录中添加它，或加上 `-g` 用于所有项目：

```sh
bunx skills add nsgo-dev/nsgo
```

自行阅读文档的智能体可以从 <https://nsgo.dev/llms.txt> 开始，它以 Markdown 列出每个页面，地址为 `https://nsgo.dev/docs/<page>.md`；<https://nsgo.dev/llms-full.txt> 包含全部页面。智能体还可以操作运行中的应用（[运行与调试](debugging.md#智能体)）。

## 下一步

- [示例：待办应用](tutorials/todo.md)：状态、列表和测试，大约十分钟。
- [示例：笔记应用](tutorials/notes.md)：导航栈和保存在设备上的笔记。
- [示例：GitHub 热门仓库](tutorials/repos.md)：从网络加载，带错误和重试。
