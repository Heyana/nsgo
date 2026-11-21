---
title: TSX 应用
description: 用 TSX 和 Tailwind 类名编写应用，提前编译，不需要 Node。
group: 核心概念
order: 63
---

## TSX 应用

TSX 应用是一个嵌入自身源码并运行入口的 Go 包：

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` 会写出这样一个应用，`examples/tsx` 也是一个例子。`nsgo dev` 和 `nsgo build` 会提前编译 TSX，所以发布的包里不带编译器；`go run` 和 `go test` 则在应用启动时编译。两种方式都不需要 Node。

## 组件与模块

TSX 从 `nsgo` 导入基本元素，从 `nsgo/base` 导入 `base` 的组件；应用导入 `tsx/ui` 后，还能从 `nsgo/ui` 导入 `ui` 的组件；`examples/gallery-tsx` 展示了这些组件。

## 在 TSX 中使用 Go

注册 Go 代码供 TSX 导入：

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

之后 `import { Badge, greet } from "app/native"` 就会把 `Badge` 的字段当作属性，并能调用 `greet`。接收 `context.Context` 的函数会变成 promise，`tsx.RegisterClass` 则把 Go 类型变成 TSX 可以用 `new` 创建的类。

以 `undefined` 转发的可选属性使用它的默认值。注册的 Go 组件若有默认值不是其类型零值的 getter，加上 `tsx.Default("Max", float64(100))` 这样的选项；响应式属性返回 undefined 时同样使用这个默认值，而且不会替换组件。

注册 Go 组件和类时，会在运行时通过反射查找它们导出的字段和方法。这种查找让 Go 链接器无法删除没有被调用的导出方法，而且影响整个应用，不只是 TSX 注册的部分，因此链接 `tsx` 会让去除调试信息的桌面二进制增加约 20 MB，应用链接的包越多，增加得越多。

## 签名包

应用可以运行它下载的 TSX，例如重新下发的自身页面或插件，前提是先验证包。在包的 `nsgo.yaml` 中声明它的代码可以访问什么，其余一律拒绝：

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # 或 { schemes: [https] }
  host: [search, "player.*"] # 它可以调用的应用处理函数
  # webview: { hosts: [...], input: true, media: [microphone, camera] }、clipboard、picker、speech、text_to_speech、status_bar、notifications、biometrics、sign_in_with_apple、purchases、secrets、audio、auth_session
```

根密钥离线保存，用它为发布密钥签发证书，然后在该目录中签名应用。`go tool nsgo keygen` 打印一对密钥，不写任何文件；根密钥和每个发布密钥各运行一次。公钥写在参数和应用代码里；私钥不要放进仓库，只通过 `NSGO_ROOT_KEY` 和 `NSGO_RELEASE_KEY` 交给工具：

```sh
go tool nsgo keygen # 根密钥一次，发布密钥一次
NSGO_ROOT_KEY=<根私钥> go tool nsgo certify -key <发布公钥> -apps 'com.example.*'
NSGO_RELEASE_KEY=<发布私钥> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <根公钥> plugin/dist/Plugin-1.0.0.nsgopkg
```

应用信任写在代码里的根公钥，在引擎协程之外验证包，再用 `tsx.View` 在沙箱中把它显示在自己的视图旁。包只能通过视图给它的 host 通道访问应用，并拥有应用存储和机密中属于自己的一部分：

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // 拒绝被篡改、已吊销和更旧的构建
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // 运行之后，更旧的构建会被拒绝
```

`Sandbox.Grant` 让它得到比声明更少的权限，`Sandbox.Limits` 设置其他时间预算和配额，`Sandbox.Modules` 给它 NSGo 自带之外的 Go 函数和组件。它的快捷键、返回处理器和导航栈只在键盘焦点位于它的视图内时生效；它自己移动焦点（`focus()`、`autoFocus`、`Keyboard.dismiss`、`setAccessibilityFocus`）也只在焦点已在其中、或响应用户在其中的按压时才生效。它显示在自己的视图中，这个视图占满且不超出父视图给它的空间，并裁剪它的内容，菜单、弹出框和提示也在其中；放在滚动视图中时，用一个视图包住它，它才会按内容的高度显示。它的对话框、警告框、上下文菜单和读屏播报也只按同样的规则显示；即使它不移除自己的对话框，返回也会关闭它。`View.Export` 指定包的入口模块导出的一个组件，显示它而不是入口挂载的根，此时入口的 `mount` 既不渲染也不运行它的组件，并把 `View.Props` 作为它的 props；`View.Props` 是可以 JSON 编码的 map，每个视图各得一份副本。这样一个包能提供多个视图，各有自己的状态，权限和限额都相同；导出不存在时以 `tsx.ErrNoViewExport` panic，导出的不是函数时以 `tsx.ErrViewExportNotComponent` panic。入口导出函数的包是函数型插件：`tsx.Load(cx, p, tsx.Sandbox{})` 在调用方的 goroutine 上同步运行它而不显示，要求 `cx` 属于一个视图树，否则返回 `tsx.ErrNoTree`；入口模块加载时抛出错误则返回这个错误，`Plugin.Call` 调用这些函数。`go tool nsgo dev -limits` 像这样的视图一样运行应用，`go tool nsgo revoke` 签署吊销发布密钥和包的列表，供 `Trust.UpdateRevocations` 使用。`examples/plugins` 会请用户授予包声明的权限，再安装并运行它。`Limits.Memory` 默认为 64 MiB，在 `-limits` 中写作 `memory=<bytes>`（`memory=0` 关闭它），它在原生目标上限制按大小分配的内置函数为包占用的内存：`ArrayBuffer` 和各种类型化数组、`repeat`、`padStart`、`padEnd`、`Array.from`、`join`、`fill`、`toSorted` 等数组副本、`apply` 的参数列表，以及 `JSON.stringify`。它们在分配前检查，超出时包以 `ERR_NSH_QUOTA_EXCEEDED` 停止，而不会让应用退出；分配出的内存一直计入，直到被回收；停止包之前会先运行 Go 的垃圾回收，所以仅仅是垃圾不会让它停止。对象、拼接出的字符串、逐个元素增长的数组、`concat`、`flat` 和展开语法产生的副本，以及 `replace` 生成的字符串都不计入，Web 上也不限制，所以它防的是失控的分配，而不是有意耗尽内存的包。自己用 `pkg.Build` 签名包的应用，或用 `go tool nsgo publish -extra <文件>` 放入该文件内容的应用，可以把自己的元数据放进 `pkg.Contents.Extra`，例如插件要加到菜单里的项：它和包一起签名，`Verify` 原样以 `p.Extra` 返回，由应用自己解析，因此应用无需运行包的代码就能读到它。

应用可以用 `pkg.SignDocument` 为自己的 JSON 文档（例如插件目录）签名，再用 `Signed.Open` 验证，密钥使用 `nsgo keygen` 生成的即可。每种文档使用一个带版本的独立 `app:` 上下文，例如 `app:catalog-v1`：一种文档不会被当作另一种文档，也不会被当作包、证书或吊销列表。`Open` 只信任传给它的公钥，不使用证书和吊销列表，并拒绝解码目标类型中没有的字段。信封中没有时间和序号，旧文档与新文档同样能通过验证：请在正文中放入版本或过期时间并加以检查。
在原生沙箱中，`JSON.stringify`、带 reviver 的 `JSON.parse`、`Array.prototype.join`、`toString`、`toLocaleString` 和 `flat`，以及数组的 `String()`，深入超过 1,000 层时会抛出可捕获的 `RangeError`；一个在另一个内部调用时层数累计，数组的 `toString` 和它调用的 `join` 各算一层。更深的数据会让调用卡住或让应用退出。Web 使用浏览器的内置函数。

在原生平台上，`DefaultLimits.Stack` 为 1,000 帧。可以通过 `Limits.Stack` 或 `-limits stack=<frames>` 调整深度，上限为 10,000 帧。Go 中小于或等于零的值以及超过上限的设置都按 10,000 帧处理。Web 使用浏览器的调用栈上限。

原生函数之间的相互调用与 JS 调用一样计入 `Limits.Stack`：作为内置函数的 getter、`toString` 或 `valueOf`，`exec` 被换成自身其他方法的正则表达式，相互嵌套的 `Reflect.apply` 与 `Reflect.construct`，以及绑定函数或代理组成的链，超过该深度时抛出可捕获的 `RangeError`，不会让应用退出；嵌套深度超过它的插件值在转换为 Go 值时，让该调用以 `RangeError` 失败。这来自 NSGo 依赖的 goja 分支 `github.com/nsgo-dev/goja`，应用随 NSGo 一同获得，无需修改自己的 `go.mod`。

在原生平台上，`Limits.Call` 限制每一段 JavaScript 的连续执行时间，包括定时器回调和 `await` 后恢复的执行。若其中一段被中断，仍在等待结果的 `Plugin.Call` 只回调一次并返回 `tsx.ErrBudget`，同时停止该调用剩余的定时器、忽略迟到的宿主应答。其他独立调用继续运行，包括在被中断的执行段中排队的 Promise 后续回调。结算其他调用的 Promise 时，当前调用仍会报告自己的结果或错误。回溯正则表达式使用加载与调用预算中较短的值作为匹配超时；未处理的超时也会变成 `tsx.ErrBudget`。

`Limits.CallTotal` 为每次插件调用设置从调用开始算起的截止时间，等待时间也计入。即使调用已完成，它启动的定时器、interval、动画帧、Promise 后续回调和宿主应答仍受这个截止时间限制。到期后，待执行的工作被丢弃，仍在等待结果的调用以 `tsx.ErrBudget` 失败。入口模块启动的工作受 `Limits.Load` 限制，宿主主动送达的事件从送达时起受 `CallTotal` 限制。`Plugin.Interrupt` 可从任意 goroutine 停止插件，丢弃待处理的定时器，让进行中和之后的调用失败。`Plugin.Close` 会运行插件的清理函数，并让仍在等待的调用以“插件已关闭”的错误结束。

Web 上暂时无法把包与页面隔离：包的代码能绕过它的权限，直接访问页面自己的网络、剪贴板和存储，信任库所在的存储也包括在内。因此在 Web 上 `tsx.View` 和 `tsx.Load` 会以 `tsx.ErrUnisolated` 拒绝运行包，除非设置 `Sandbox.WebUnisolated`，让它以页面的权限运行；这只适合你愿意像对待应用自身代码一样信任的包。

## 编辑器与测试

继承 `.nsgo/tsconfig.json` 的 `tsconfig.json` 为编辑器提供 TSX 的类型，这些类型由 `go tool nsgo dev` 和 `go tool nsgo check` 写出。应用旁边用 `nsgo/test` 编写的 `*.test.tsx` 文件会随 Go 测试一起运行（[测试](testing.md)）。
