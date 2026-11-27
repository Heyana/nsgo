---
title: 复制组件
description: 使用 nsgo add 将声明的 Go 组件复制到应用中，重写导入路径并记录其来源模块、版本与许可证声明。
group: 组件
order: 35
---

当应用需要直接修改某个组件而不是只配置它时，`nsgo add` 可以把模块在 `nsgo-module.yaml` 的 `components:` 中声明的包复制到应用中：

```sh
nsgo add github.com/acme/kit#field                  # 复制到 components/field
nsgo add github.com/acme/kit@v1.2.0#field           # 复制指定版本
nsgo add -dir internal/components/field github.com/acme/kit#field
nsgo add -tests=false github.com/acme/kit#field     # 跳过 _test.go 与 testdata/
nsgo add -dry-run -diff github.com/acme/kit#field   # 预览计划及每个文件的完整内容
```

复制完成后，这些文件即成为应用自己的 Go 代码。

## 复制目标目录

默认情况下，组件会复制到相对于 `nsgo.yaml`（没有 `nsgo.yaml` 时相对于模块根目录）的 `components/<包名>`。可在 `nsgo.yaml` 中用 `components.dir` 修改基础目录：

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
3. **重写导入与注释。** 组件自身、其 `locales/` 包以及同模块内被导入的其他组件（会按需一并复制）的导入路径都会重写为副本的新导入路径，并保留原有导入别名；注释中完整出现的旧路径也会同步重写，字符串字面量中出现的旧路径会在计划中列出但保持不变，最后以 `go/format` 格式化。
4. **生成多语言目录。** 若组件包含 `locales/*.yaml`，`nsgo add` 会以副本的最终导入路径在内存中用当前 `nsgo` 生成 `locales/catalog_gen.go`，使 `nsgo check` 立即通过。若应用自身目录为原 `locales` 路径提供了翻译，计划会提示导入副本后应改为新路径。
5. **规划依赖并执行 vet。** 外部模块以及同模块的非组件包导入会作为直接依赖加入 `go.mod`（不低于源模块所需版本，也绝不降低应用已有版本）。规划阶段会在临时环境中对 `build.targets`（无 `nsgo.yaml` 时为宿主机）以 `go vet -mod=readonly` 校验副本；宿主机缺少工具链的目标列在 `not vetted` 中且不阻断 `-y`。
6. **原子写入。** 写入时持有模块的 check 锁，先把文件暂存到目标旁的隐藏目录 `.nsgo-copy-*`，再连同 `go.mod` 与 `go.sum` 原子改名到位；任何错误或中断都会完整回滚。

## 许可证声明与 `nsgo-copy.yaml`

源模块根目录及路径中间目录中的许可证与声明文件会一并复制到组件根目录（重名会被拒绝）。`nsgo add` 会使用构建时的分类器识别组件携带的全部许可证文件（包括 `icons/OFL.txt` 等资源许可证），并立即执行应用的 `licenses` 策略。

在 `nsgo build` 中，带有 `nsgo-copy.yaml` 的目录会在 `THIRD_PARTY_NOTICES.txt` 中获得独立章节 `<导入路径>, copied from <模块> <版本>`，不会混入应用自身的许可证章节。对于 Apache-2.0 许可的组件，请按第 4(b) 条要求标明修改过的文件。

每个复制目录都包含 `nsgo-copy.yaml`，以确定性格式记录来源模块、版本、组件名、测试选择、已复制文件和生成文件（不含时间戳和哈希）：

```yaml
# Written by nsgo add. nsgo diff compares this directory with the copy of this version.
version: 1
module: github.com/acme/kit
module_version: v1.0.0
component: field
tests: true
files:
  - LICENSE
  - field.go
  - locales/en.yaml
generated:
  - locales/catalog_gen.go
```

以相同版本再次执行 `nsgo add` 会直接报告已有副本的版本和目录而不做修改。移动副本目录无需额外命令，`nsgo-copy.yaml` 会随之移动。
