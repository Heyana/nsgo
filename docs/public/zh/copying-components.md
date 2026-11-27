---
title: 复制组件
description: 使用 nsgo add 将 chat、file-picker 等声明的 Go 组件复制到应用中，重写导入路径，使用 nsgo diff 查看修改，并记录其来源模块、版本与许可证声明。
group: 组件
order: 35
---

当应用需要直接修改某个组件而不是只配置它时，`nsgo add` 可以把模块在 `nsgo-module.yaml` 的 `components:` 中声明的包复制到应用中。NSGo 已将 `ui` 下的八个子包（`chat`、`code-editor`、`commit-graph`、`diff-view`、`file-picker`、`flow-diagram`、`rich-editor` 和 `terminal`）声明为可复制组件，并收录在内置索引中：

```sh
nsgo add -list chat                              # 在 NSGo 内置索引中查找 chat
nsgo add chat                                    # 将 ui/chat 复制到 components/chat
nsgo add file-picker                             # 将 ui/filepicker 复制到 components/filepicker
nsgo add -dir internal/components/chat chat      # 将 chat 复制到模块内的其他路径
nsgo add -tests=false file-picker                # 跳过 _test.go 与 testdata/
nsgo add -dry-run -diff chat                     # 预览计划及每个文件的完整内容
```

也可以直接用模块路径复制任意模块声明的组件，例如 `nsgo add github.com/nsgo-dev/nsgo#file-picker`。

复制完成后，这些文件即成为应用自己的 Go 代码。

## 副本位置

默认情况下，组件会复制到相对于 `nsgo.yaml`（没有 `nsgo.yaml` 时相对于模块根目录）的 `components/<包名>`，因此 `chat` 会复制到 `components/chat`，`file-picker` 会复制到 `components/filepicker`。可在 `nsgo.yaml` 中用 `components.dir` 修改基础目录：

```yaml
components:
  dir: ui/components
```

使用 `-dir <路径>` 可为命令行中指定的单个组件指定模块内的自定义目录，其最后一段必须等于该组件的 Go 包名。`nsgo add` 会拒绝模块之外的路径、`vendor`、`testdata`、嵌套模块、以 `.` 或 `_` 开头的目录、经由符号链接父目录的路径、已存在的路径以及仅大小写不同的冲突路径。

## 版本选择

未输入 `@version` 时，`nsgo add` 按以下顺序决定版本：

1. 应用 `go.mod` 中已依赖的该模块版本（该规则为最终选择：若该版本未声明该组件，`nsgo add` 会拒绝并给出引入声明版本的 `nsgo add <名称>@<版本>` 命令）；
2. 否则使用索引条目中声明的版本（通过短名称添加时）；
3. 否则使用 `@upgrade`（最新正式版）。

若输入的版本高于 `go.mod` 中当前依赖版本，计划的第一行会直接说明该版本升级；若低于当前依赖版本，`go.mod` 保持不变并在计划中说明副本版本较旧。`go` 指令低于 `1.22` 或高于应用规划后 `go` 版本的模块会被拒绝。

## 复制流程

1. **校验归档。** `nsgo add` 在隔离目录中使用与 `add` 相同的仅代理、`GOTOOLCHAIN=local` 环境下载模块 zip，通过 `zip.CheckZip` 校验归档结构，并校验 `dirhash.HashZip` 是否同时匹配下载返回的 `Sum` 以及 `go.sum` 中的记录，随后直接从 zip 中读取文件。
2. **检查规则与标记项。** 所有文件都会经过组件复制规则检查：文件白名单、路径限制、数量与体积上限（最多 256 个文件、单文件 1 MiB、总计 4 MiB）、无 BOM 的 UTF-8 以及 Go AST 规则（拒绝 `import "C"`、`internal` 导入、测试外的 `tsx` 导入、`//go:build ignore`、非 `nsgo i18n` 的 `//go:generate` 与违规 `//go:embed`）。控制/格式字符、非 ASCII 标识符、敏感导入（`unsafe`、`syscall`、`golang.org/x/sys/...`、`purego`、`os/exec`、`plugin`）、敏感编译指令（`//go:linkname`、`//go:wasm*`、`//go:cgo_*`、`//go:debug`）及 `go vet` 失败会附带位置显示并在 `-y` 下被拒绝。
3. **重写导入与注释。** 组件自身、其 `locales/` 包（如 `ui/chat/locales`）以及同模块内被导入的其他组件（会按需一并复制）的导入路径都会重写为副本的新导入路径，并保留原有导入别名；注释中完整出现的旧路径也会同步重写，字符串字面量中出现的旧路径会在计划中列出但保持不变，最后以 `go/format` 格式化。
4. **生成多语言目录。** 若组件包含 `locales/*.yaml`（例如 `chat`），`nsgo add` 会以副本的最终导入路径在内存中用当前 `nsgo` 生成 `locales/catalog_gen.go`，使 `nsgo check` 立即通过。若应用自身目录为原 `locales` 路径提供了翻译，计划会提示导入副本后应改为新路径。
5. **规划依赖并执行 vet。** 外部模块以及同模块的非组件包导入会作为直接依赖加入 `go.mod`（不低于源模块所需版本，也绝不降低应用已有版本）。规划阶段会在临时环境中对 `build.targets`（无 `nsgo.yaml` 时为宿主机）以 `go vet -mod=readonly` 校验副本；宿主机缺少工具链的目标列在 `not vetted` 中且不阻断 `-y`。
6. **原子写入。** 写入时持有模块的 check 锁，先把文件暂存到目标旁的隐藏目录 `.nsgo-copy-*`，再连同 `go.mod` 与 `go.sum` 原子改名到位；任何错误或中断都会完整回滚。

## 许可证声明与 `nsgo-copy.yaml`

源模块根目录及路径中间目录中的许可证与声明文件（如 NSGo 的 `LICENSE` 和 `NOTICE`）会一并复制到组件根目录（重名会被拒绝）。`nsgo add` 会使用构建时的分类器识别组件携带的全部许可证文件（包括 `icons/OFL.txt` 等资源许可证），并立即执行应用的 `licenses` 策略。

在 `nsgo build` 中，带有 `nsgo-copy.yaml` 的目录会在 `THIRD_PARTY_NOTICES.txt` 中获得独立章节 `<导入路径>, copied from <模块> <版本>`，不会混入应用自身的许可证章节。对于 `chat` 和 `file-picker` 这类 Apache-2.0 许可的组件，请按第 4(b) 条要求标明修改过的文件；`nsgo diff -stat` 会列出所有改动文件。

每个复制目录都包含 `nsgo-copy.yaml`，以确定性格式记录来源模块、版本、组件名、测试选择、已复制文件和生成文件（不含时间戳和哈希）：

```yaml
# Written by nsgo add. nsgo diff compares this directory with the copy of this version.
version: 1
module: github.com/nsgo-dev/nsgo
module_version: v0.4.0
component: chat
tests: true
files:
  - LICENSE
  - NOTICE
  - actions.go
  - actions_test.go
  - chat.go
  - chat_test.go
  - composer.go
  - controls.go
  - conversation.go
  - effort.go
  - find.go
  - locales/en.yaml
  - locales/locales.go
  - locales/zh-Hans.yaml
  - native_test.go
  - reasoning.go
  - reasoning_test.go
generated:
  - locales/catalog_gen.go
```

以相同版本再次执行 `nsgo add` 会直接报告已有副本的版本和目录而不做修改。移动副本目录无需额外命令，`nsgo-copy.yaml` 会随之移动。

## 使用 `nsgo diff` 对比副本

使用 `nsgo diff` 查看应用对副本的本地修改、另一个上游版本会带来的变更，或列出模块中所有副本的修改状态：

```sh
nsgo diff                              # 列出每个副本、版本以及是否被修改
nsgo diff chat                         # 输出本地修改相对记录版本的统一 diff
nsgo diff components/chat              # 按目录路径指定副本
nsgo diff chat@v0.5.0                  # 输出从记录版本到 v0.5.0 的统一 diff
nsgo diff chat@upgrade                 # 输出从记录版本到最新正式版的统一 diff
nsgo diff -stat chat                   # 按文件输出增删行数（也用于 Apache-2.0 第 4(b) 条标明修改文件）
nsgo diff -exit-code chat              # 存在差异时以状态码 1 退出
```

- **目标解析。** 包含 `nsgo-copy.yaml` 的目录可直接匹配。短名称优先匹配模块自身的 `nsgo-copy.yaml` 记录，因此离线也可用；若来自不同模块的两个副本同名，`nsgo diff` 会提示传入目录（或 `module#component`）。否则按内置索引或已配置的 `@registry/name` 解析并按模块与组件匹配记录。
- **干净基准副本。** `nsgo diff` 直接从经校验的模块 zip（通过 `zip.CheckZip` 与 `dirhash.HashZip` 核验并比对 `go.sum`）重建记录版本（或目标版本），应用记录中的 `tests` 选择、祖先许可证复制、Go 导入与注释重写（其他已复制组件按其当前记录所在目录定位）以及 `go/format` 格式化；`locales/catalog_gen.go` 等生成文件不参与对比。
- **兼容 Git 的输出。** 统一 diff 使用 `diff --git` 头和相对模块根目录的路径，附带三行上下文，可直接交给 `git apply` 和 `git apply -R`。二进制文件输出 `Binary files ... differ`。
- **行尾与格式。** 对比时将 CRLF 视作 LF，且经 `go/format` 格式化后与记录版本一致的 Go 文件视为未修改。
- **离线与不可用版本。** 记录版本优先从 `GOMODCACHE` 读取，支持在 `GOPROXY=off` 下离线使用；未命中缓存时在 `GOTOOLCHAIN=local` 的仅代理环境中下载。若记录版本无法再下载，副本中所有非生成文件均视为已修改，并输出不可用原因。
- **安全约束。** 每次读取都会重新校验 `nsgo-copy.yaml` 中的路径（包含 `../form/form.go` 等越界路径会被拒绝并指出行号），所有文件读取均限制在副本目录的 `os.Root` 内，并拒绝符号链接。

## 更新副本

为已复制的组件指定其他版本运行 `nsgo add`，可逐文件更新副本并保留应用自己的修改：

```sh
nsgo add chat@v0.5.0                  # 按 nsgo-copy.yaml 中的组件名更新
nsgo add chat@upgrade                 # 更新到最新正式版
nsgo add chat@v0.4.0                  # 降级到更早版本（计划中标为 downgrade）
nsgo add components/chat@v0.5.0       # 当模块内存在同名副本时按目录指定
nsgo add -dry-run chat@v0.5.0         # 预览逐文件变更计划而不写入磁盘
```

短名称会优先在当前模块的 `nsgo-copy.yaml` 记录中离线匹配；如果不同模块的两个副本同名，请传入副本目录或 `module@version#component`。

`nsgo add` 会重建记录版本（基准）与目标版本（作者版本），并将 CRLF 视为 LF、对 Go 文件做格式化后与目录中的当前文件逐一比较：

- **应用未修改该文件：** 采用目标版本；若目录中原文件使用 CRLF 换行，写入时保留 CRLF。
- **作者未修改，或双方改成了相同内容：** 保持原样不改写。
- **双方做了不同修改：** 拒绝更新，列出所有冲突文件，且不写入任何文件。
- **作者删除了该文件：** 应用未修改时自动删除；若应用修改过则标为标记项（`-y` 下拒绝，交互确认后才删除）。
- **作者新增了文件，且目录中已存在同名文件（不区分大小写）：** 拒绝更新，且不写入任何文件。

更新同时会依据最终的目录内容重新生成 `locales/catalog_gen.go`，把当前副本从同一模块导入的其他组件在同一计划中同步更新到同一版本，按新复制的规则处理 `go.mod` 依赖、许可证策略和目标 vet，并将 `nsgo-copy.yaml` 更新为新版本及其文件清单。若目标版本的模块不再声明该组件，更新会被拒绝并指明该版本号。

### 手动迁移应用修改

当应用与作者同时修改了同一个文件时，`nsgo add` 不会自动做行级合并，而是拒绝写入以防覆盖。可借助 `nsgo diff` 手动把应用修改迁移到新版本：

1. 先保存应用相对于记录版本的修改：
   ```sh
   nsgo diff chat > chat.patch
   ```
2. 查看目标版本中作者的改动：
   ```sh
   nsgo diff chat@v0.5.0
   ```
3. 将冲突文件临时恢复到记录版本（例如用 `git apply -R` 还原该文件在 `chat.patch` 中的改动），使 `nsgo add` 将其识别为未修改。
4. 运行 `nsgo add chat@v0.5.0` 干净地升级副本并更新 `nsgo-copy.yaml`。
5. 对照 `chat.patch` 将应用修改重新合入更新后的文件，再运行 `nsgo diff chat` 与 `nsgo check` 确认只保留预期修改且编译检查通过。

## 在包含副本的应用中升级 NSGo

NSGo 或组件模块在新版本中可能引入不兼容变更。副本在与复制时匹配的模块版本下能够正常编译，升级后则可能出现编译失败。`nsgo` 会在升级前后指出受影响的副本，同时避免在没有破坏编译的版本差异上令 CI 失败：

1. **升级前（`nsgo add`）。** 当 `nsgo add` 的计划移动了已有副本所导入的模块（尤其是 NSGo）时，`nsgo add` 会在不改写模块文件的前提下，用规划后的 `go.mod` 与 `go.sum` 对这些副本执行宿主机 `go vet`。凡会停止编译的副本都会在计划的 `copies` 栏中列出，附带第一条编译或 vet 错误以及 `nsgo diff <名称>@<版本>`、`nsgo add <名称>@<版本>` 命令。在 `-y` 下，会破坏副本的计划会被拒绝，防止自动化流程静默破坏副本；在终端中交互审阅计划后仍可手动确认。
2. **外部升级后（`nsgo check`）。** 当 `go.mod` 通过 `go get` 或 Dependabot 等方式在 `nsgo` 之外更新后，`nsgo check` 会在执行 vet 之前读取所有 `nsgo-copy.yaml`、校验路径与边界，并对来源模块在 `go.mod` 中已处于不同版本的副本输出警告：

   ```text
   warn components/chat is chat from github.com/nsgo-dev/nsgo v0.4.2; this module requires v0.5.0: nsgo diff chat@v0.5.0, nsgo add chat@v0.5.0
   ```

   已是最新版本的副本不会输出任何信息。如果版本变动未导致副本编译失败，`nsgo check` 仅输出该警告并正常退出（退出码 0）。
3. **编译或 vet 失败时（`nsgo check`、`nsgo build`、`nsgo dev`）。** 每当 `go vet` 或 `go build` 在某个副本包内失败（依据 Go 输出的 `# <导入路径>` 行匹配），`nsgo` 会在 `go` 命令输出之后追加同一行诊断信息，直接标明失败包对应的副本目录、来源模块与版本以及查看和更新命令。如果模块中存在两个同名副本，命令提示会自动使用副本的相对目录路径以消除歧义。
