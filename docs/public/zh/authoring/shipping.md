---
title: 文案、测试与发布
description: 翻译组件说的话，像用户一样测试它，并提供给 Go 和 TSX 应用。
group: 编写组件
order: 74
---

## 文案

组件自己要说的话，比如按钮给读屏器的名称，通过所在包的目录文件提供，而不是写成字符串字面量。在组件库中放一个 `locales/` 包，包含源语言的目录文件，以及一个生成代码的文件：

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales` 为每条消息生成一个函数，组件用自己的 scope 调用：`locales.Clear(cx)` 以应用显示的语言说出它。把生成的 `catalog_gen.go` 一并提交，应用只用 Go 就能构建。为每种已翻译的语言加一个文件（`zh-Hans.yaml`）；其他语言由应用在自己的目录文件中提供，键名是你的包的导入路径，与 [NSGo 自己的文案](../localization.md#nsgo-自己的文案)相同。

## 测试

`nsgotest` 以用户的方式无头运行组件。把它挂在一个小的根组件里，按角色和文字操作它：

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

在组件库根目录运行 `nsgo test` 会测试所有包，加上 `-target macos` 或其他目标则在原生窗口中运行同样的测试。在两种外观和伪语言（`-lang en-XA`）下都测一遍，可以发现写死的颜色和文案。其余内容见[测试](../testing.md)。

## 发布

组件库就是一个 Go module：应用用 `nsgo add <模块>`（或 `go get`）添加它并导入其中的包。同时有无样式和带样式两种形式时，把它们放在不同的包里，这样有自己外观的应用不会导入用不到的样式。

收录在 NSGo 内置索引（[`cmd/nsgo/registry.yaml`](https://github.com/nsgo-dev/nsgo/blob/main/cmd/nsgo/registry.yaml)）中的库会出现在 `nsgo add -list` 中，并可用短名称添加（`nsgo add <名称>`）。要收录你的库，提交一个 Pull Request 按规范格式（按名称排序、字段按模式顺序、两空格缩进）向 `cmd/nsgo/registry.yaml` 添加条目。收录的库需要满足：

- 至少有一个正式发布标签（不能只有未打标签的提交）；
- 模块路径无需重定向即可直接解析；
- 解析后的依赖图使应用停留在已发布的 NSGo 版本上；
- 使用合法的 SPDX 许可证表达式。

团队也可以发布自己的 `version: 1` YAML 注册表索引，并在 `nsgo.yaml` 的 `registries:` 中配置，让应用通过 `nsgo add @team/name` 添加并用 `nsgo add -list` 浏览（[自定义注册表](../building.md#自定义注册表)）。

### 索引检查

NSGo 的[索引检查工作流](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/registry.yml)每周运行一次，并在修改 `cmd/nsgo/registry.yaml` 的 Pull Request 上触发。它对收录的每一项检查：

- `@upgrade`（或条目指定的 `version`）解析到正式语义化版本标签（不能是伪版本或预发布版本），且模块的 `go.mod` 声明的模块路径与索引一致；
- 模块路径对应的仓库 URL 直接响应，不发生重定向；
- 在依赖 NSGo 最新正式版本的临时模块中添加该条目后，NSGo 仍停留在正式版本上；
- `nsgo-module.yaml`（若存在）及所有关联包的 `nsgo-natives.yaml` 都能正常解析；
- 对于库，在临时包中导入其声明的 `import` 包时 `go tool nsgo check -target linux_x64` 通过（只编译和 vet，不运行或测试第三方代码）；
- 对于组件，模块在 `@upgrade` 处的 `nsgo-module.yaml` 仍声明该组件，且（除 NSGo 自身外，其组件由 `check.yml` 就地检查）复制到依赖 NSGo 最新正式版本的临时应用后通过 `go tool nsgo check -target linux_x64`。

TSX 应用在 Go 组件注册后就能使用它们，注册写在 `init` 函数中：

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

之后 `import { Toggle } from "example.com/chips"` 即可把字段作为 props 使用（[TSX 应用](../tsx.md#在-tsx-中使用-go)）。

### 模块清单

组件库可以在模块根目录的 `go.mod` 旁放一个可选的 `nsgo-module.yaml`，向 `nsgo add` 和 `nsgo check` 说明应用需要知道的信息：

```yaml
version: 1
import: [player]
targets: [macos, windows_x64, linux_x64, ios, ios_simulator]
notes: |
  FFmpeg's libraries are LGPL 2.1: offer their source beside your downloads,
  credit FFmpeg and allow replacing them.
components:
  chat: ui/chat
  file-picker: ui/filepicker
```

- 该文件严格校验：单个 YAML 文档，最大 1 MiB，拒绝未知字段、锚点、别名和合并键。`nsgo build` 从不读取它。
- `import`：相对于模块根目录的包目录（根目录用 `.`）。每一项都必须是本模块的非 `main` 包，不能属于嵌套模块。`nsgo add` 会为每一项打印 import 行；没有该文件（或省略 `import`）时打印根包的 import 行。
- `targets`：`targets.Expand` 接受的目标名和目标组（如 `macos`、`windows_x64`、`all`、`miniprogram`、`tui`）。`nsgo add` 的计划会列出应用 `build.targets` 中该列表缺少的目标；`nsgo check` 在检查每个目标时，会对任何被链接且 `targets` 缺少该目标的模块发出警告。
- `notes`：最多 4 KiB 的纯文本，除换行外不能包含控制字符或 Unicode 格式字符。`nsgo add` 会把它打印在计划最后，标题为 `From <module>, the author's notes:`，并标记每一行。
- `components`：从组件名（1 到 64 个 `[a-z0-9-]` 字符，不以 `-` 开头或结尾）到模块根目录之下相对包目录的映射。每个目录都必须是本模块的非 `main` 包，且目录路径的最后一段必须与 Go 包名一致。`nsgo add` 的计划会连同 `nsgo add <module>#<name>` 命令一并列出。
- 新字段在 `version: 1` 内演进。当模块的 `go.mod` 要求比当前工具更新的 NSGo 且使用了当前工具不认识的字段时，`nsgo` 会报错并给出 `go get github.com/nsgo-dev/nsgo@<version>`。

### 可复制组件

当模块根目录的 `nsgo-module.yaml` 声明了 `components` 时，`nsgo check` 会在运行 `go vet` 之前，针对 `golang.org/x/mod/zip.CheckDir` 会打入模块 zip 的文件，按 `nsgo add` 复制组件相同的规则逐一检查：

- **文件**（不区分大小写比较）：组件根目录和 `locales/` 下的 `.go` 文件；仅在 `locales/` 和 `testdata/` 中允许 YAML；图片（`.svg`、`.png`、`.jpg`、`.jpeg`、`.webp`、`.gif`、`.avif`、`.ico`）、字体（`.ttf`、`.otf`、`.woff2`）、音频（`.wav`、`.mp3`、`.ogg`）和 `.txt`；仅组件根目录允许 `README.md`；仅 `testdata/` 和资源子目录允许 `.json`（任何位置都不允许 `package.json`、`tsconfig.json`、`jsconfig.json`、`deno.json` 或 `composer.json`）；以及许可证文档（`LICENSE`、`NOTICE`、`COPYING` 等）。其余文件一律拒绝——包括 `.s`、`.syso`、C/C++/Objective-C/Fortran/SWIG 源码、`.wasm`、`go.mod`、`go.work`、`nsgo.yaml`、`nsgo-module.yaml`、`nsgo-natives.yaml`、`nsgo-copy.yaml`、`AGENTS.md`、`CLAUDE.md` 和 `SKILL.md`。
- **路径**：每段路径只能包含 ASCII 字母、数字、`-`、`_` 和 `.`，不能以 `.` 或 `_` 开头；不能存在仅大小写不同的路径；除 `locales/` 外任何子目录都不能包含 `.go` 文件。
- **限额**：单个组件最多 256 个文件，单文件不超过 1 MiB，总大小不超过 4 MiB。
- **文本**：文本文件必须是不带 BOM 的合法 UTF-8。
- **Go 规则**：每个 `.go` 文件（不论构建标签）都必须能正常解析并声明对应包名；禁止 `import "C"`；禁止导入外部副本无法访问的 `internal` 包；禁止在 `_test.go` 之外导入 `github.com/nsgo-dev/nsgo/tsx`；禁止 `//go:build ignore`（及 `+build ignore`）；除 `locales/` 中的 `//go:generate go tool nsgo i18n`（可带 `-source <tag>`）外禁止其他 `//go:generate`；禁止 `//go:embed` 模式匹配 `nsgo-copy.yaml`、逃出包目录或匹配被拒绝的文件。
- **警示项**（发出警告并说明 `nsgo add -y` 会拒绝复制）：文本文件中的控制字符与格式字符（除制表符、LF 和 CRLF 外的 `Cc`、`Cf`、`U+2028`、`U+2029`，其中 `locales/` 下从右到左语言目录消息内容里的 `U+200E`、`U+200F`、`U+061C` 与 `U+2066`–`U+2069` 例外）；含非 ASCII 字符的 Go 标识符；导入 `unsafe`、`syscall`、`golang.org/x/sys/...`、`github.com/ebitengine/purego`、`os/exec` 或 `plugin`；以及 `//go:linkname`、`//go:wasmimport`、`//go:wasmexport`、`//go:cgo_*` 和 `//go:debug` 指令。

`nsgo check` 还会注明组件导入的、属于同一模块但本身不是组件的包，因为复制该组件的应用会因此依赖本模块。未声明 `components` 的模块检查行为保持不变。
