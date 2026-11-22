---
title: 平台服务
description: 警告框、剪贴板、文件、照片、分享、存储、机密、语音、朗读、网络、登录、App 内购买和应用生命周期。
group: 平台功能
order: 44
---

## 警告框与剪贴板

`cx.ShowAlert` 显示平台的警告框或操作表。`Actions` 按下标返回，角色会让平台用对应方式绘制取消和危险按钮。

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "删除这个文件？",
	Message: "此操作无法撤销。",
	Actions: []core.AlertAction{
		{Label: "取消", Role: core.AlertCancel},
		{Label: "删除", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` 写入文本，`SetClipboardImage` 写入图片。`ReadClipboard`、`ReadClipboardImage` 和 `ReadClipboardFiles` 异步回答，`HasClipboardText` 和 `HasClipboardImage` 只读取剪贴板内容，不会触发粘贴提示。`core.FileFromURL` 把 `ReadClipboardFiles` 和 `View.OnFileDrop` 给出的 URL 打开为 `core.File`：`file:` URL，以及 Web 上的 `blob:` URL。

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

## 打开链接与文件

`cx.OpenURL` 用系统的浏览器或应用打开链接，`core.CheckURL` 说明它会拒绝什么：没有 scheme、超过 16 KiB、包含空格、Web 地址没有 host，以及 `javascript:`、`data:`、`blob:`、`file:`、`about:` 和 `asset:`。

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("没有应用能打开这个链接。")
		}
	})
}
```

`cx.OpenFile` 用系统关联的应用打开应用持有的文件，`cx.Share` 在 macOS、iOS、Android、Windows、Linux（通过桌面门户或文件管理器）与 Web（`navigator.share`）上显示系统分享面板，可以分享文件、文本或两者。`core.SharedFile` 是磁盘上的路径——在 Web 上或应用持有内存字节时也可以直接通过 `Data` 传入——以及另一个应用看到的名称和类型。在桌面上，`desktop.RevealFile` 在访达、资源管理器或文件管理器中选中显示一个文件或文件夹，`desktop.TrashFile` 把它移到废纸篓或回收站；它从不永久删除，卷没有废纸篓或废纸篓容不下该项目（例如比回收站还大的文件）时回答 `core.ErrNoTrash`，项目留在原处。

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "来自 NSGo 的笔记",
}, func(shown bool, err error) {})
```

在 Android 上，`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` 接收 [`android.share_types`](building.md#接收-android-分享) 声明的文本和文件。启动应用的分享会等到第一个处理函数注册。文本放在 `Text`，文件会复制到缓存，并带有 `Path`、`Name` 和 `MIME`。如需长期保留文件，请移到或复制到持久存储。

`desktop.LookUp` 为一段文本显示系统词典面板，如 macOS 上三指轻点所做，文本绘制在某个视图中的一个框上：通常是选区的文本和边界框，加上其 `Selectable` 视图的 `Ref`。`desktop.CanLookUp` 说明能否显示，即在 macOS 的窗口中；Windows、Linux 和 Web 没有这样的面板。测试中，在 `nsgotest.Environment` 设置了 `CanLookUp` 时，`nsgotest.App.LookUps` 列出显示过的面板。

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// 在菜单项或快捷键中：
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` 请求从指定序号开始预览文件，面板打开时再次调用会更新文件。`desktop.CanQuickLook` 在 macOS 和 Windows 的应用自有窗口中可用；Linux 需要会话中存在已运行或可激活的 GNOME Sushi D-Bus 服务。Windows 使用文件类型注册的预览处理程序，支持的格式由已安装的处理程序或预览器决定。返回 true 仅表示已向平台发出请求，不代表文件已经显示。

空格或 Escape 关闭面板。Windows 上可用方向键切换请求中的文件，X11 上能报告选择和所属窗口的 Sushi 版本也支持。Wayland 上目前打开无父窗口的预览，无法随所属窗口自动关闭或用方向键切换文件。KDE 专用预览器和 Web 暂不支持。

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## 照片与文件

`cx.PickImages` 显示系统照片选择器，返回 `core.PickedFile`，其中包含字节、名称、类型和像素尺寸。`cx.TakePhoto` 在平台有相机时打开相机；`cx.Env().CanTakePhoto` 说明是否有。`cx.ScanCode` 打开相机预览，返回读到的第一个码，默认只读二维码，`ScanOptions.Kinds` 可以指定其他种类；用户关闭时返回 nil。`cx.Env().CanScanCode` 说明平台是否能扫码，支持带相机的 macOS、iOS、Android 和 Windows 设备、具备 Camera 门户和 GStreamer `pipewiresrc` 的 Linux 桌面，以及支持 `getUserMedia` 和 `BarcodeDetector` 的浏览器。macOS 在发起请求的可见窗口上展示附属面板，Windows 和 Linux 在窗口上展示模态预览，Web 在页面上展示浮层。macOS 和 iOS 需要在 `apple.info` 中设置 `NSCameraUsageDescription`，Android 需要在 `android.permissions` 中加入 `CAMERA`。Android、Windows 和 Linux 除 PDF417 外都能识别。码里的文本是不可信的输入。

测试中，`nsgotest.App.PickImages`、`nsgotest.App.TakePhoto` 和 `nsgotest.App.ScanCode` 接收照片选择或拍摄记录 `nsgotest.ImagePick`，或扫码记录 `nsgotest.CodeScan`，并代替系统界面回答。它们可用于无头测试和支持这些钩子的 `nsgo test -target` 宿主。目标没有相机时，用 `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` 启动即可得到。

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` 选择任意类型的文件，`cx.PickDirectories` 把文件夹选成 `fs.FS`，`cx.SaveFile` 通过系统保存对话框保存文件或字节。两种选择器都可以设置 `Title`（标题）、`Button`（按钮文字）和 `Start`（起始文件夹），`PickFilesOptions.TypesName` 还能命名类型菜单，便于用应用自己的语言显示；这些只是提示，macOS、Windows 和 Linux 会遵循，iOS 只支持 `Start`，Android 和 Web 都会忽略。`cx.PickScreenColor` 在 `cx.Env().CanPickScreenColor()` 为真时从屏幕任意位置取色：macOS、Windows、提供桌面 Screenshot portal 的 Linux，以及支持 `EyeDropper` 的浏览器。


在 macOS、Windows、Linux 和 iOS 上，将 `core.SaveOptions.Start` 设为现有文件夹的绝对路径，保存对话框就会从该文件夹打开。路径为空、不可用或在其他平台上时，由系统选择起始文件夹。

设置 `core.SaveOptions.Accept` 可以在写入前校验用户选定的目标。回调在引擎的 goroutine 上运行。返回错误时，所选目标保持原样。macOS、Windows 和 Linux 上进程内的 GTK 对话框会显示拒绝原因，并允许用户选择其他路径。如果用户在拒绝后取消，保存回调会收到 `saved == false`、被拒绝的路径和原始错误。在 Android 和 Linux 桌面门户中，拒绝时保存回调直接收到上述结果。Android 让用户选择文件夹，传给校验回调的是树 URI，其中 `name` 查询参数为应用请求的文件名。校验通过后才创建文件，成功时返回新文档的 URI。校验超过五秒未答复，平台会放弃写入。iOS 和 HarmonyOS 对要求校验的保存返回错误。Web 没有目标路径，沿用其常规保存流程。

在无头测试中，`App.PickFiles`、`App.PickDirectories` 和 `App.SaveFile` 分别接收 `nsgotest.FilePick`、`nsgotest.DirectoryPick` 和 `nsgotest.FileSave` 记录。`app.FilePicks`、`app.DirectoryPicks` 和 `app.Saves` 保存这些记录。

## 拉取式 PCM 音频

用 `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` 播放应用解码或合成的交错 Float32 PCM。`Start` 开始拉取采样，`Pause` 停止拉取，所属作用域销毁时会关闭输出。macOS、iOS 和 Android 支持此输出，其他目标返回 `audio.ErrUnsupported`。

`Render` 返回填入的音频帧数。它在音频线程上运行，须避免阻塞、分配内存或访问响应式图。填入的帧数不足时，剩余部分播放静音并增加一次 `Underruns`。`Position` 返回单调递增的帧数及该帧对应的主机时间。`Flush` 丢弃待播放的帧，`Latency` 返回缓冲延迟。音频输出线路变化或会话被中断时，输出暂停并在引擎中调用 `OnInterrupt`。应用可调用 `Start` 恢复。

## 存储

`cx.Storage()` 是应用的键值存储，保存在设备上，应用的每个响应式图共享它。读取是同步的，在计算中读取会让它订阅变化；超过配额时 `SetItem` 返回 `core.StorageFullError`（默认 `core.StorageQuota` 为 5 MiB）。它是一个用户的任何程序都能读取的明文文件，令牌和密码请放进[机密](#机密)。

需要创建另一个 root 的宿主（例如在独立 goroutine 上运行插件）可以用 `cx.Store()` 获取底层 store，并将它传给 `core.RootOptions.Storage`，两个 root 会共享应用的条目和保存在设备上的数据。它是整个 store，而不是 `cx.ProvideStorage` 分给非应用自身代码的分区。

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` 列出键，`RemoveItem` 删除一个，`Clear` 清空存储。`cx.DataDir()` 和 `cx.CacheDir()` 给出应用存放键值存储放不下的文件的目录；`cx.LogsDir()` 给出平台存放日志的目录；`Scope.ProvideStorage` 给子树一个独立分区。

在 HarmonyOS 上，`os.ReadFile`、`os.WriteFile` 和 `os.Rename` 等 Go 文件操作可用于这些目录及系统的临时目录。相对路径从应用的文件目录开始解析。暂不支持建议性文件锁和符号链接。

### 崩溃报告

原生发布构建将 Go 崩溃堆栈保存在 `cx.LogsDir()/crashes` 下。Apple 平台记录未捕获的 Objective-C 异常，Android 记录未捕获的 Java 异常，Windows 记录到达顶层过滤器的 SEH 异常。下次启动时调用 `cx.CrashReports()` 列出待处理报告。报告含崩溃应用的版本、构建号和平台。调用 `Open()` 读取诊断文本并关闭读取器，处理完成后调用 `Acknowledge()` 删除报告。在 goroutine 中执行这些文件操作。

## 磁盘空间

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` 异步查询 `cx.DataDir()` 所在的卷，在引擎的 goroutine 上回调。`Total` 是系统报告的容量，`Available` 是应用可以用于用户请求操作的空间，`Opportunistic` 是可以用于缓存和预取的空间，单位都是字节。系统报告可按需回收的空间时，`Available` 也会计入它。平台只提供一个可用空间值时，两个字段取同一值。在 Web 上，`Total` 是浏览器估算的当前来源存储配额，`Available` 是剩余配额。服务不可用时返回 `core.ErrDiskSpaceUnavailable`。

Apple 构建需要在 `nsgo.yaml` 的 `apple.privacy.accessed_apis` 中声明 `NSPrivacyAccessedAPICategoryDiskSpace`。写入之前检查空间或在空间不足时清理文件，且用户可感知到这样的行为变化，使用 `E174.1`。向用户显示可用空间使用 `85F4.1`。应用必须遵守 Apple 对数据使用和共享的限制。未声明该类别的构建返回 `core.ErrNotDeclared`。[Apple 批准的用途](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

测试可以用 `nsgotest.DiskSpace(space, err)` 指定返回值。

## 机密

`cx.Secrets()` 是应用的安全存储，用来保存刷新令牌、API 密钥和密码：按账户 `Get`、`Set`、`Delete` 一个字符串，由平台保护。不存在的账户读出 `("", false, nil)`；平台没有安全存储时（例如 Web），每次调用都返回 `core.ErrSecretsUnavailable`，NSGo 不会退回明文存储。系统可能让调用等待，例如钥匙串向用户请求授权，所以请在引擎的 goroutine 之外调用，再用 `cx.Post` 把结果交回。

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| 平台 | 机密保存在 |
| --- | --- |
| macOS | 登录钥匙串中的通用密码：`kSecAttrService` 为服务，`kSecAttrAccount` 为账户 |
| iOS、tvOS、watchOS | 钥匙串中的通用密码，方式相同 |
| Windows | 凭据管理器中目标为 `service/account` 的普通凭据，最多 2560 字节 |
| Linux | Secret Service（GNOME Keyring、KWallet）中带 `service` 和 `account` 属性的条目；会话中没有运行 Secret Service 时不可用 |
| Android | 用 Android Keystore 中的 AES 密钥加密，保存在应用的私有文件中，不进入备份；读取时也会回退读取 `expo-secure-store` 加密保存在 `SecureStore` `SharedPreferences` 中的值 |
| 微信小程序 | 平台加密存储（`wx.setStorage` / `wx.getStorage` 的 `encrypt: true`，需基础库 2.21.3+）；旧版基础库不可用 |
| Web、其他小程序 | 不可用（`core.ErrSecretsUnavailable`）：浏览器没有受操作系统保护的客户端机密存储（Web 会话应保存在服务端下发的 `HttpOnly` Cookie 中），QQ、支付宝和抖音小程序也没有加密存储 |

服务默认是应用的 ID（`core.App().ID`）。从其他框架迁移来的应用，只要指定那个框架写入的条目，用户就不会被登出：`Service`，以及 Apple 平台上的 `AccessGroup` 和 `Accessible`。在 Android 上，将 `Service` 设为 `expo-secure-store` 使用的 `keychainService`（默认为 `"key_v1"`）即可读取它加密写入 `SharedPreferences` 的值；写入或删除会清理对应的迁移条目。在 iOS 上，`core.SecretAfterFirstUnlock` 让后台任务在手机锁定时也能读取条目，`ThisDeviceOnly` 类的可访问性让条目不进入备份。在 macOS 上，设置 `AccessGroup` 会把条目移到数据保护钥匙串，这需要由团队签名、且 `keychain-access-groups` 权利列出该组的构建；在 `nsgo.yaml` 的 `macos.entitlements` 中配置即可同时用于 Mac App Store 和 Developer ID 构建。

```go
s := cx.Secrets()
s.Service = "app:no-auth" // 早先版本使用的服务
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` 把应用的机密保存在内存中（`App.Secrets`），`nsgotest.Secrets` 让测试带着一些机密启动，所以测试不会碰到本机的钥匙串。TSX 包凭 `secrets` 能力访问安全存储，使用它自己的服务。

## 网络

`cx.Env().Network()` 报告系统是否有可用的网络路由（`Online`），以及系统是否认为该路由按流量计费或费用较高（`Expensive`）。绑定会随网络状态变化。使用 `cx.OnNetworkChange(func(core.Network))` 可在作用域存活期间，让处理函数在引擎的 goroutine 上接收每次变化。系统的路由状态无法判断某个服务器是否会响应。无法报告连通性的目标默认视为在线，费用未知时 `Expensive` 为 false。

通过 `nsgotest.Environment` 或 `App.SetEnvironment` 设置 `core.Environment{Network: &core.Network{Online: false}}`，即可测试离线行为。

Go 自带的 `net/http` 在每个目标上都能发起请求，使用系统的 TLS 和代理设置。signal 属于引擎的 goroutine，因此其他 goroutine 得到的响应要通过 `cx.Post` 交给它。

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` 在响应式图的 goroutine 上运行函数，因此可以在那里设置 signal。Android 的清单默认申请网络权限，图片下载会缓存在磁盘上。

从某个来源加载值时，`core.NewResource` 会替你完成这些：它的 fetcher 在自己的 goroutine 上运行，来源变化时重新加载：

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`、`items.Err()` 和 `items.Loading()` 都是 signal，绑定会跟着它们更新。新的查询会通过 `ctx` 取消仍在进行的请求，迟到的响应不会覆盖较新的结果。`items.Refetch()` 用于重试，加载期间保留上一次的列表；`items.Mutate` 立即设置一个值，用于乐观更新。

`core.Suspense` 会在其内容读取的资源加载首个值时显示占位内容，页面无需自己维护加载状态：

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

等待期间内容保持挂载但处于隐藏状态，值一到就能立即显示。重新加载时会继续显示上一次的值。

### 发现局域网服务

在 macOS、iOS、Android 和 Linux 上，`cx.AdvertiseService` 发布监听端口和 TXT 属性。`cx.BrowseServices` 报告解析后的服务，包括主机名或 IP 地址、端口和 TXT 属性。服务离开时，回调收到设置了 `Removed` 的事件。通过 `cx.Env().CanDiscoverServices()` 检查平台支持。两个调用都会返回停止函数，作用域释放时自动停止。平台错误会通过引擎 goroutine 异步报告。Linux 使用 Avahi，守护进程不可用时会报告错误。

Apple 平台使用 Bonjour。请在 `nsgo.yaml` 中声明服务类型和访问局域网的用途。权限提示由系统显示，用户的选择也由系统保存。

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: 查找附近可同步的设备。
    NSBonjourServices:
      - _example._tcp
```

Android 使用 `NsdManager`。在 Android 12 及更早版本以及低于 T Extensions 7 的 Android 13 上，通过 Wi-Fi 发现服务需要 `CHANGE_WIFI_MULTICAST_STATE` 权限。独立应用已声明该权限。如果把 NSGo 嵌入 Android 宿主，请在宿主的清单中为这些设备声明该权限。在缺少原始 TXT 接口（API 37 或 T Extensions 22）的设备上，发布的 TXT 值必须是有效的 UTF-8。浏览仍会保留 TXT 值的原始字节。目标 API 为 37 或更高的 Android 宿主需要在 Android 17 或更高版本上声明并请求 `ACCESS_LOCAL_NETWORK` 权限。

可用 `nsgotest.LocalServices` 为无头测试提供浏览结果。

## 后台下载

在独立的 iOS、macOS 应用及 Android 上使用 `cx.Download` 和 `cx.OnDownloadUpdate`。嵌入式 Apple 视图尚无传输宿主。Android 应用需在 `nsgo.yaml` 中启用 `android.background_downloads: true`。应用可见时，通过用户操作启动下载。目标路径必须是绝对路径，且父目录已存在并可由应用写入。

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

下载使用 GET，可附加请求头。HTTPS 默认信任系统证书。填写 `CertificatePEM` 可以固定服务端的叶证书，并继续检查主机名和证书有效期。请传入最终 URL，以便直接取得成功响应。

在 Apple 平台上，公共主机名使用自签名证书时，需要在 `apple.info` 中为该主机配置 App Transport Security 例外。传输仍会验证证书固定值、主机名和有效期。连接本地 NAS 时，还应通过 `NSLocalNetworkUsageDescription` 说明用途，无限定主机名或 `.local` 主机使用 `NSAllowsLocalNetworking`。[Apple 的 ATS 配置](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

Android 前台服务在界面退出前台后继续下载，并在获得通知权限时显示进度。进度以字节计，未知总长度时 `Total` 为 -1。下载成功后原子替换目标文件。`task.Pause()` 保留分片文件，服务器提供校验标识时 `task.Resume()` 使用 HTTP Range 续传，`task.Cancel()` 删除分片文件。

iOS 使用系统的后台 URLSession，在应用挂起时继续传输。应用启动时重新连接该会话，并在确认后台处理结束前发布文件。macOS 的传输在进程内运行。Apple 平台使用原生续传数据暂停和恢复下载，并将请求元数据保存在应用私有存储中，直到终态写入完成。不支持续传的服务器会从头开始传输。

启动应用后重新注册 `OnDownloadUpdate`，即可查看活动任务和保存的结果。处理完终态后调用 `p.Task.Forget()`。Android 停止下载服务时（例如前台传输超时），任务报告 `DownloadInterrupted`。在 Windows、Linux、tvOS、watchOS 和 Web 上，`Download` 返回 `ErrDownloadUnavailable`。

## 语音与生命周期

`cx.StartSpeech` 在 `cx.Env().CanRecognizeSpeech()` 为真时用系统识别器识别语音（支持 macOS、iOS、Android、Windows 以及提供 `SpeechRecognition` 或 `webkitSpeechRecognition` 的浏览器），并随着转写结果变化返回文本。在 Apple 平台上需在 `apple.info` 中声明 `NSMicrophoneUsageDescription` 和 `NSSpeechRecognitionUsageDescription`；在 Android 上需在 `android.permissions` 中添加 `RECORD_AUDIO`。在无头测试中，识别期间 `app.Speech` 保存一个 `nsgotest.SpeechSession`。使用 `app.HearSpeech` 和 `app.EndSpeech` 交付结果。

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "zh-CN",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` 说明应用处于前台、非活动还是后台：在桌面上是 `cx` 所在窗口的状态，所以一个窗口最小化时其内容可以暂停，而其他窗口照常显示；在应用自身的作用域里则是所有窗口合起来的状态。`desktop.Minimized(cx)` 说明该窗口是否最小化，`desktop.FrontmostApp(cx)` 说明桌面上最前面的是哪个应用（包括应用自身）以及它的窗口是否全屏，桌面宠物或计时工具可以据此跟随用户正在用的应用；`desktop.CanReadFrontmostApp(cx)` 说明能否读到它，在 macOS、Windows 和 X11 上可以。`cx.OnQuit` 在应用退出时做清理，无论以何种方式退出，最多有 200 毫秒完成工作。`desktop.OnQuitRequest` 可以取消或推迟桌面菜单的退出操作或 `desktop.RequestQuit(cx)` 发出的请求。在 Windows 和 Linux 上，如果关闭窗口会结束应用，`OnClosing` 通过后也会询问 `OnQuitRequest`。系统报告注销、重新启动或关机时，`q.Forced()` 为真，应用会执行 `OnQuit` 后退出。询问用户前先检查 `Forced()`。`cx.SetAppearance` 强制浅色或深色，`cx.SetLocale` 选择语言，`cx.OnKey` 绑定键盘快捷键，它的所有者显示的内容被隐藏或被对话框盖住时不生效，除非用 `core.KeyOptions{OverModals: true}` 保留；它先于应用菜单拿到按键，除非用 `YieldsToMenu` 把按键让给当前焦点下接收相同快捷键的已启用菜单项（文本框或网页视图获得焦点时，`MenuNotEditing` 菜单项把按键留给该绑定，也不让后面相同快捷键的菜单项接收；此时 Linux 的菜单不接收未按 Control、Alt 或徽标键的按键，因此绑定会保留这类按键）；它只在 `cx` 所在的窗口里生效，除非用 `AllSurfaces` 把它绑定到应用的每个窗口和面板，包括之后打开的，作为全应用的快捷键，绑定它的窗口里所有者显示的内容被隐藏时，它在所有窗口都不生效；获得焦点的控件自己要用的键也留给控件，按钮的 Return 和 Space，滑块、标签栏和单选组的方向键，除非用 `core.KeyAlways` 或 `core.KeyNotEditing` 绑定；`cx.OnOpenFiles` 接收 macOS、Windows、Linux 和 iOS 打开的文档。iOS 把普通文件导入应用自有的临时目录；要保留，请移到 `cx.DataDir()`。 在 macOS 上，提供应用菜单的窗口关闭后，会恢复最近仍打开的窗口菜单；没有其他菜单时恢复默认菜单。

应用菜单中的编辑角色会先交给获得焦点的编辑器，再考虑 `OnSelect` 回退。在 macOS 上，撤销和重做按该编辑器的历史启用，剪切按其选区启用，应用处于非活动状态时也如此；没有编辑器时，回退保持启用。

在 iOS 上，可以通过 `cx.OnKeyWith` 的 `core.KeyOptions.Title` 函数为快捷键提供标题。绑定生效时，快捷键会出现在系统的键盘菜单里。`Group` 为 iPadOS 26 及以上版本的菜单组命名。这两个函数可以读取应用的语言目录，让标题随语言切换。测试可以用 `app.KeyCommands()` 查看当前列表。

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` 是用户给设备起的名字，`cx.Env().DeviceModel()` 是它的硬件型号，例如 `Mac15,7`、`iPhone15,2` 或 `Google Pixel 8`，应用可以据此区分两台配对的设备。macOS、Android、Windows 和 Linux 两者都提供；iOS 提供型号，名字只提供给拥有 Apple 用户自定义设备名权限（user-assigned device name entitlement）的应用；Web 两者都不提供。测试可用 `nsgotest.Environment` 设置它们。

`cx.Env().ThermalState()` 在 Apple 平台和 Android API 29 及以上报告 `core.ThermalNominal`、`ThermalFair`、`ThermalSerious` 或 `ThermalCritical`。`cx.Env().LowPowerMode()` 在 Apple 平台、Android、Windows 和运行 power-profiles-daemon 的 Linux 会话中报告 `core.LowPowerOn` 或 `LowPowerOff`。系统不提供值时，分别返回 `ThermalUnknown` 和 `LowPowerUnknown`。两者各自响应变化，应用可以在运行时调整负载。测试可以通过 `nsgotest.Environment` 或 `app.SetEnvironment` 设置 `core.Environment` 的 `ThermalState` 和 `LowPowerMode`。

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` 在 macOS、Windows 和 X11 上可以读取应用窗口外的指针位置，每次调用读取一次。位置与 `desktop.Monitors(cx)` 和 `Window.ShowAt` 使用同一屏幕坐标系。在 Wayland 等不支持的目标上，或作用域缺少 `core.AccessCursorPoint` 权限时，`available` 为 false。

`desktop.SetWindowConstraints` 限制内容尺寸，并控制调整大小、最小化和最大化；打开窗口时可设置 `WindowOptions.Constraints`，主窗口可用 `nsgo.WindowConstraints`。Linux 在 X11 下支持非固定的最大尺寸，并在 X11 和 Wayland 下从 GTK 原生标题栏移除被禁止的最小化、最大化按钮。Wayland 目前只有固定尺寸能限制上界。

`desktop.SetWindowLevel` 让窗口浮在其他窗口之上；`desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` 或 `WindowOptions.Workspaces` 让它出现在每个工作区，桌宠、HUD、计时器都是这样：macOS 上出现在每个空间（Space）和其他应用的全屏空间之上，不参与 Cmd-` 切换；Linux 上出现在 X11 窗口管理器的每个虚拟桌面。Windows 和 Wayland 上它留在打开时所在的那个。`desktop.CanSetWindowWorkspaces` 说明它是否生效：macOS 上以及支持 `_NET_WM_DESKTOP` 的 X11 窗口管理器下为 true。主窗口在自己的内容里调用 `desktop.SetWindowWorkspaces` 即可出现在每个工作区。测试中 `nsgotest.App.Window` 列出这些请求，`nsgotest.Environment` 可以设置 `CanSetWindowWorkspaces`。

`desktop.RegisterHotKey` 绑定全局快捷键，无论哪个应用在前台都生效，支持 macOS、Linux，以及用 `nsgo.Run(App)` 运行的 Windows 应用。它返回的句柄的 `State` 说明系统是否接受了它：系统答复前是 `HotKeyPending`，之后是 `HotKeyRegistered`；组合键已被其他注册占用时是 `HotKeyTaken`；按键或平台不支持全局快捷键时是 `HotKeyUnsupported`。快捷键录制器据此可以请用户换一个组合。在 macOS 上，只有应用自己已注册、或其他应用以独占方式注册的组合键才算被占用。在 Wayland 上由合成器请用户绑定快捷键，用户绑定后才是已注册；用户或合成器没有绑定的是已占用。测试中用 `nsgotest.HotKeyTaken` 和 `nsgotest.HotKeyUnsupported` 让组合键这样答复。

使用 `desktop.TitleBarHidden` 时，Windows 和 Linux 会在单独按下并松开 Alt 或按下 F10 时弹出应用菜单。Alt 加顶层标签的第一个字母或数字会打开该子菜单或选择该命令，也支持非拉丁文字。应用绑定仍优先接收自己的按键；Shift+F10 仍用于上下文菜单，其他 Alt 组合键留给系统。Escape 关闭弹出菜单并恢复键盘焦点。

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Alt+Space 已被其他应用使用"
	}
	return ""
}}
```

macOS、Windows 和 Linux 上调整桌面窗口大小时，绘制前最多等待 16 毫秒的新布局。让响应式计算和事件处理保持轻快，布局才能赶上这个期限；耗时的 I/O 放到 goroutine 中，再通过 `cx.Post` 交回结果。应用繁忙时窗口仍可调整大小，赶上后再显示新布局。

将 `desktop.WindowOptions.Parent` 设为同一应用中打开的窗口，父窗口关闭时子窗口也会关闭。在桌面平台上，子窗口位于父窗口上方，并随父窗口隐藏或最小化；主动隐藏的子窗口不会随父窗口恢复而重新显示。在 macOS 上，未设置 `Frame` 时子窗口还会在父窗口上居中打开，并随父窗口进入全屏。

`desktop.SetWindowPointerEvents` 让指针穿过窗口，落到它下面任何应用的窗口上：`WindowPointerNone` 处处穿过，像抬头显示那样；`WindowPointerPainted` 只在没有视图绘制内容、也没有视图接收指针的地方穿过，像桌面宠物周围那样，宠物本身仍接收按下、拖动和右键；系统绘制的标题栏算作什么都没绘制。`WindowOptions.PointerEvents` 和 `PanelOptions.PointerEvents` 让窗口一打开就这样。`Desktop.WindowDrag` 区域位于可按祖先内部时仍能拖动窗口；区域内部更近的可按或编辑控件保留自己的输入，不会拖动窗口。完全不接收指针的窗口收不到任何按下，所以应用要从快捷键、托盘图标或另一个窗口把指针交还给它。受 `Access` 约束的代码需要 `core.AccessOverlay`。

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

在 macOS 和 Windows 上，绘制区域接收指针的窗口可为图片设置 `Image.HitTestAlpha`（TSX 中为 `hitTestAlpha`），取值大于 0 且不超过 1。例如，0.5 表示图片像素的不透明度至少为一半时才接收指针。这个阈值也适用于图片的 `OnPress`，而视图的背景或边框仍算作绘制区域。设为 0 时使用整个图片视图，Linux 上的图片以及桌面平台上的视频也都按整个视图判断。

在 macOS 和 Windows 上，`desktop.NewTray` 可通过 `TrayOptions.View` 绘制实时图标。Windows 将视图的透明绘制结果缩放到通知区域的图标大小，并在视图变化或图片加载完成后更新；用 `TrayOptions.OnClick`、`Panel` 和 `Menu` 处理图标的操作。Linux 目前显示 PNG 图标，绘制托盘视图仍是目标。

`desktop.OpenPanel` 打开一个面板，供这样的快捷键或托盘图标显示：浮在其他窗口之上的无边框窗口。与 `desktop.OpenWindow`、`desktop.NewTray`、`mobile.OpenOverlay`、`audio.NewPlayer`、`audio.LoadSound` 和 `core.NewHolder` 一样，它直接归你显式传入的 `cx` 所有，而不被当前运行中的回调所有者覆盖，因此在按钮的 `OnPress` 里传入应用作用域即可让打开的窗口、面板或播放器在按钮作用域销毁后继续存活；`Effect`、`Cleanup`、`AfterFunc`、`OnKey` 等响应式原语则归当前运行中的所有者，需要切换时用 `cx.Run`。弹出卡片画在带 `Transparent` 的面板上，面板自己不画背景、边框和阴影，只显示卡片的圆角和 `BoxShadow`，阴影要在面板内留出空间；`FitContent` 让面板在首次显示前、以及内容变化时取内容的大小，最大到 `Width` 和 `Height`。`desktop.Transparent` 说明面板此刻是否不绘制任何自身内容，对透明窗口也一样；`desktop.CanMakeWindowsTransparent` 只说明窗口能否透明。为 false 时，例如没有合成器的 Linux，要给内容加上背景。

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`PanelOptions.Dock` 让面板在 `Show`、`Focus`、`Toggle` 和内容尺寸变化时，始终居中贴在显示器可用区域的一侧。`Edge` 取 `core.PlaceTop`、`PlaceBottom`、`PlaceLeft` 或 `PlaceRight`，`Display` 是 `Env.Displays` 的 ID（空字符串表示主显示器），`Margin` 是离该边缘的点数。停靠面板不能调用 `ShowAt` 或 `MoveTo`；`SetWindowFrame` 只改变其尺寸。托盘图标显示它时仍保持停靠位置。

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` 单独报告停靠能力。macOS、Windows 和 X11 支持它；Wayland 需要可选运行库 `libgtk4-layer-shell.so.0`，以及实现 `zwlr_layer_shell_v1` 的合成器。NSGo 在 GTK 之前加载该库；若嵌入宿主已加载 Wayland，就必须在启动宿主前预加载 gtk4-layer-shell。缺少库、协议或指定显示器时，面板保持隐藏并调用 `OnPlacementError`。Wayland 停靠面板位于 top 层，不占用工作区：`NoActivate` 让键盘继续归当前窗口，否则面板独占键盘焦点，直到隐藏。普通 Wayland 窗口仍由合成器按原有方式放置。

快捷键录制控件让用户自己选这样的按键。`Focusable` 视图的 `OnKeyDown` 在它有焦点时、应用的绑定之前，收到每个按下的键和当时按住的修饰键，单独按下的修饰键也算；返回 `true` 让这个键不再交给绑定，所以录制 Command+S 时不会触发保存。`KeyEvent.Shortcut` 把按键写成 `OnKey` 和 `RegisterHotKey` 接受的形式，每个修饰键用自己的名字（"ctrl+shift+k"、"alt+space"，Command+Shift+K 是 "shift+meta+k"），单独的修饰键为 ""。它的 `OnKeyUp` 收到这些键各自抬起，即使 `OnKeyDown` 已消费该键或焦点已经移走，同时带着仍按住的修饰键。两者在 macOS、Windows、Linux、Web（浏览器不交给页面的快捷键仍归浏览器）、Apple TV 和 `nsgotest` 中可用；Android 异步向有焦点的视图报告硬件键盘事件，`OnKeyDown` 的返回值无法消费按键。iPhone 和 iPad 支持轮询硬件按键。应用自己注册的全局快捷键会先于视图拿到按键，所以录制期间要先注销它。

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

游戏或画布则轮询按键。`core.KeysDown(cx)` 列出窗口上按住的键，`core.KeyHeld(cx, "left")` 判断某个键是否按住：二者都是被追踪的读取，`RequestAnimationFrame` 循环也能读取。第一次读取后，平台从此报告每个键，所以读取前已经按住的键要再按一次才会知道；在文本框或网页视图中、或输入法组字时按下的键永远不算按住，窗口失去焦点时仍按住的键会在那时抬起。它们在 `OnKeyUp` 可用的地方都可用，在接了硬件键盘的 iPhone 和 iPad 上也可用。

对于鼠标按键，`View.OnMouseDown` 和 `View.OnMouseUp` 在 macOS、Windows、Linux、Web 和 `nsgotest` 上为五个按键（`core.MouseButtonLeft`、`core.MouseButtonRight`、`core.MouseButtonMiddle`、`core.MouseButtonBack`、`core.MouseButtonForward`）报告 `core.MouseButtonEvent`，且不会把视图变成键盘焦点停靠点。`core.MouseButtonsDown(cx)` 和 `core.MouseButtonHeld(cx, core.MouseButtonRight)` 是对窗口上当前按住的鼠标按键的被追踪读取，窗口失焦时会自动清空。

如果操作绑定到物理键位，读取 `KeyEvent.Code`、`core.KeyCodesDown(cx)` 或 `core.KeyCodeHeld(cx, "KeyW")`。键位采用 `KeyboardEvent.code` 命名。在 AZERTY 布局下，QWERTY 键盘 W 所在位置的 `Code` 为 `"KeyW"`，`Key` 为 `"z"`。`core.KeyCodeLabel(cx, "KeyW")` 返回当前布局的显示标签，可用于按键提示。在 iOS、Android 和不支持布局映射的浏览器上，它使用该键位最近一次报告的按键名称，在收到事件之前返回空字符串。

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## 保持屏幕常亮

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` 会让屏幕保持常亮，直到调用返回的释放函数或当前作用域被销毁。在 Android 上，应用可见时保持常亮。在 Web 上，它需要支持 Screen Wake Lock 的安全上下文，页面恢复可见时会重新请求锁。Safari 首次授予锁前可能需要点击或按键。这两个平台上的仅系统保活请求均无效。

## 全屏

在 macOS、Windows 和 Linux 上，`desktop.SetFullScreen(cx, true, displayID)` 会铺满 `cx.Env().Displays()` 中指定的显示器。不传 ID 时使用窗口所在的显示器。未知 ID 不改变窗口。重复同一请求没有影响。`desktop.SetFullScreen(cx, false)` 退出全屏并恢复之前的窗口位置和大小。在 Web 上请从点击处理函数进入全屏，由浏览器选择显示器，`cx.Env().FullScreen()` 跟踪进入和退出，包括按 Esc 退出。

## 显示器

`cx.Env().Display()` 返回窗口当前所在的 `core.DisplayInfo`，窗口移到另一台显示器时会更新绑定。`cx.Env().Displays()` 列出平台报告的显示器。`RefreshRate` 和 `MaxRefreshRate` 的单位为 Hz，无法获取时为零。iOS 提供面板最高刷新率。Web 提供当前屏幕，并用 `RefreshRateEstimated` 标记通过帧回调测得的估算值。测试中可通过 `nsgotest.Environment` 设置 `Displays` 和 `DisplayID`。

## 登录

`cx.AuthSession` 在系统的认证会话里用 OAuth 服务登录，而不是在网页视图里：macOS 和 iOS 用 `ASWebAuthenticationSession`，Windows、Linux 以及没有设置 scheme 的 Mac 用默认浏览器加回环地址重定向，Web 用弹出窗口。协议由应用负责：它生成 PKCE 校验码和 state，为传给它的重定向地址拼出授权 URL，并在用 code 换令牌前检查回调。取消不算失败：`errors.Is(err, core.AuthCancelled)` 表示用户关闭了会话，或返回的 `cancel` 结束了它。

```go
verifier, challenge, state := newPKCE() // 应用自己的实现
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

系统会话能捕获 scheme 的平台上，重定向地址是 `com.example.app:/oauth2redirect`，应用不必声明这个 scheme；桌面的回环地址上是 `http://127.0.0.1:<端口>/oauth2redirect`，所以两者都要在服务方登记，通常各是一个客户端。`Scheme` 留空则只用回环地址；设为 `https` 并给出 `Host` 则用应用的关联域名，iOS 17.4 和 macOS 14.4 起能捕获。浏览器标签页被关闭时回环会话无从得知，所以要给它 `Timeout` 或一个取消按钮。`Ephemeral` 让 macOS 和 iOS 不共享浏览器的 Cookie。Android、tvOS、watchOS 和小程序目前回答 `core.AuthUnavailable`。在 TSX 中，`AuthSession.start(url, options)` 需要 `auth_session` 能力，返回的会话的 `result` 解析为回调地址，取消时为 null。

授权 URL 函数应原样使用收到的重定向地址。Web 和桌面回环重定向会先编码 Unicode 路径并解析点路径段，再把地址交给该函数；回环 `Path` 是未编码文本，所以字面上的 `%` 也会被转义。

应用的 web view 要以用户在别处（例如自己的浏览器里）已有的登录状态打开页面时，用 `cx.SetWebCookies` 写入这次登录的 Cookie，包括页面脚本写不了的 HttpOnly Cookie，再加载页面：存储收下它们后 done 才会运行。写入的是应用所有 web view 共用的持久存储，带 `Expires` 的 Cookie 在应用退出后仍然保留；无痕（Incognito）web view 看不到它们，但 Android 每个应用只有一个存储，那里也看得到。Web、tvOS 和 watchOS 回答 `Unavailable`。

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

要删除单个 Cookie，请使用相同的名称、域名和路径，将 `Expires` 设为过去的时间；零值过期时间表示会话 Cookie。等待回调后再加载页面，页面才能看到删除后的状态。

反过来，`cx.WebCookies` 列出存储里的 Cookie，包括 `document.cookie` 读不到的 HttpOnly Cookie 及其域名、路径、过期时间和各项标志，不管这个站点的页面是否正在显示：`core.WebCookieFilter` 选出向某个 `URL` 发请求时会带上的 Cookie，或某个 `Domain` 及其下所有主机的 Cookie，零值过滤器选出全部。应用要用用户的登录状态从 Go 调用这个站点，或显示用户登录了哪些站点时，就这样读取。`cx.ClearWebsiteData` 让用户退出所有站点：它删除存储里的 Cookie、本地存储和会话存储、IndexedDB、Service Worker 和缓存，全部删除后 done 才会运行。Android 的 `CookieManager` 只回答某个 URL 会带上的名称和值，所以那里只有 `URL` 过滤器能列出，`Domain` 填为它的主机，其他字段留空，`Domain` 过滤器和零值过滤器回答 `Unavailable`；在那里清除不包括 HTTP 缓存。Web、tvOS 和 watchOS 对两者都回答 `Unavailable`。

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## 通过 Apple 登录

`cx.SignInWithApple` 在 macOS、iOS 和 tvOS 上从应用自己的按钮弹出系统的「通过 Apple 登录」面板，这些平台上 `cx.Env().CanSignInWithApple()` 为 true。它只回答一次：给出凭据，或给出错误。用户关闭面板时是 `core.AppleSignInCancelled`，这不算失败；其余是 `AppleSignInFailed`，其他平台上是 `AppleSignInUnavailable`。Apple 只在用户第一次登录这个应用时给出邮箱和姓名，要在那时保存。`Nonce` 以其 SHA-256 的小写十六进制放进请求，身份令牌的 `nonce` 声明里就是这个值：服务器对它交给应用的 nonce 做哈希后比较。`cx.AppleCredentialState` 告诉你用户之后是否撤销了授权。

在无头测试中，`app.AppleSignIn` 接收 `nsgotest.AppleSignInRequest`，并提供凭据或错误。

```go
nonce := rand.Text() // 也交给服务器，服务器用它的 SHA-256 核对令牌的声明
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

应用必须带着「通过 Apple 登录」的授权签名，`nsgo.yaml` 要求时 `build` 会加上；没有它面板会失败：

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin，用于 iOS 和 tvOS 包以及 Mac App Store 构建
```

`nsgo dev` 的设备构建由 Xcode 注册这项能力；`sign.profile` 或 `sign.macos_profile` 中的描述文件必须来自开启了「通过 Apple 登录」的 App ID。macOS 只在有描述文件时授予它，所以磁盘映像和 Mac 上的 `nsgo dev` 都无法登录，模拟器也不行，它的构建不声明任何开发者授权。

## App 内购买

`cx.Products`、`cx.Purchase` 及其旁边的调用在 macOS、iOS 和 tvOS 上通过 App Store 销售，这些平台上 `cx.Env().CanPurchase()` 为 true，前提是构建的 `nsgo.yaml` 要求了 StoreKit：

```yaml
apple:
  purchases: true  # StoreKit 的库，放进 macOS、iOS 和 tvOS 包；不需要任何授权
```

显示商店给出的价格 `DisplayPrice`，不要显示应用自己定的价格：商店按各地区自行定价。一次购买只回答一次，给出交易或错误：用户关闭面板时是 `core.PurchaseCancelled`，等待批准（如「询问购买」）时是 `core.PurchasePending`，其余是附带商店原话的 `core.PurchaseFailed`，其他平台上是 `PurchasesUnavailable`。把交易的 `Signed`（商店签名的 JWS）交给应用的服务器验证，交付所购内容，再完成这笔交易；未完成的交易每次启动时商店都会再次交给应用。

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // 续订、获批的购买、其他设备上的购买、退款，以及启动时未完成的交易
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // 它的交易稍后经 OnPurchaseUpdate 到达
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` 对已完成的交易返回 `PurchaseNotFound`，即使它仍由 `CurrentEntitlements` 返回；已完成的非消耗型商品仍归用户所有。

在应用启动时注册 `OnPurchaseUpdate`：在任何处理函数之前到达的更新会留给第一个注册的。`cx.CurrentEntitlements` 回答用户当前拥有的内容，供启动时解锁；「恢复购买」按钮调用 `cx.SyncPurchases`，它可能要求用户登录。`AccountToken` 是应用自己的 UUID，交易和 App Store 发给服务器的通知都会带上它。macOS 只有 Mac App Store 构建能销售，App Clip 不销售任何东西；Android、watchOS、Windows、Linux、Web 和小程序目前回答 `PurchasesUnavailable`。用 `nsgotest` 测试购买，它的 `Products`、`PurchaseAnswer`、`UpdatePurchase` 和 `UnfinishedTransactions` 代替商店。`app.PurchaseRequests` 以 `nsgotest.PurchaseRequest` 记录商店操作。在 TSX 中，`Purchases` 模块需要 `purchases` 能力：`purchase` 解析为 `{ status: "purchased", transaction }`、`"pending"` 或 `"cancelled"`，`on("update", listener)` 接收更新。

## 指针锁定

游戏镜头或需要越过屏幕边缘继续拖动时，可在持有该 ref 的视图被按下时调用 `ref.LockPointer()`。锁定期间指针停在原处并隐藏，视图通过 `OnPointerDelta(dx, dy)` 收到相对位移，正值表示向右和向下。

调用返回的函数可以解锁。按 Escape、窗口失去焦点，或者隐藏或移除视图，也会解除锁定。用 `cx.Env().CanLockPointer()` 查看当前窗口是否支持。Web 需要点击等用户操作。Linux 的 Wayland 会话需要合成器支持指针约束和相对位移。

## 音频播放

用 `audio.LoadSound` 加载短音效，再用 `sound.Play(audio.Voice{})` 播放。多次调用可以重叠播放。在 macOS、iOS、Windows、Linux、Android 和 Web 上，可以用返回的 `audio.Playback` 单独调整音量和倍速，或停止本次播放。播放结束、停止、超过并发上限被淘汰，或无法启动时，响应式的 `Ended()` 变为 true。无法单独控制每次播放的后端会返回一个已结束的句柄。

用 `audio.Voice{Muted: true}` 静音启动音效，也可用 `playback.SetVolume(0)` 将正在播放的音效静音。长音频用 `audio.NewPlayer` 流式播放，`player.SetMuted(true)` 可在保持播放进度的同时静音。取消静音会恢复播放器设定的音量。`Voice.Volume` 和 `Player.SetVolume` 以零表示默认满音量。

## 音效限制

加载音效前，可以调用 `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` 调整引擎的限制。也可以设置 `MaxEncodedBytes`、`MaxDecodedBytes` 和 `MaxPlaybacks`。零值使用默认限制：单个音效编码后最多 8 MiB、时长最多 5 秒，引擎最多 64 个音源、解码后总计最多 16 MiB，每个输出最多同时播放 32 路。已加载的音效会继续计入新的限制。受限作用域在引擎预算内继续使用默认限制。

## 朗读文本

`cx.Speak` 在 `cx.Env().CanSpeak()` 为真时用系统中某种语言的语音朗读文本：macOS、iOS、tvOS、Android、Windows、Web，以及装有 speech-dispatcher 的 Linux。同一时间只朗读一段：新的 `Speak` 会打断正在朗读的那段，`cx.StopSpeaking` 和发起朗读的作用域结束也会停止它。`done` 只会收到一次结束方式：读完、被停止，或因 `core.SpeakError` 失败，例如没有安装该语言的语音时返回 `core.SpeakLanguageUnavailable`。

```go
cx.Speak("おはようございます", core.Voice{Lang: "ja-JP", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` 和 `Pitch` 的范围是 0.5 到 2，1 为语音本身的语速和音高。`cx.Voices("ja", ...)` 列出某种语言已安装的语音，应用可以提示用户安装缺少的语音，或让用户挑选，把选中语音的 `ID` 填进 `core.Voice`。浏览器要等用户在页面上按过某个控件后才会朗读，在此之前返回 `core.SpeakNotAllowed`。TSX 包里的 Go 组件需要 `text_to_speech` 能力才能朗读。

## 循环播放音频

设置 `audio.Voice.Loop` 可让单次音效持续重复，直到调用 `Stop` 或 `Close`。音乐可设置 `audio.PlayerOptions.LoopStart`，再调用 `SetLoop(true)`。第一遍包含前奏，之后从指定位置重复。偏移量为零时重复整条音轨。

在 macOS 14 和 iOS 17 或更新版本上，时长已知的音轨使用原生队列，在当前一遍结束前准备下一遍。更早的 Apple 系统在当前一遍结束时跳到循环起点。在 Windows、Linux、Android 和 Web 上，带起点的循环在结尾跳回起点，会有短暂间隙。

## 生物识别

`cx.BiometricAvailability` 回答设备是否录入了面容 ID、触控 ID 或指纹；Android 不说明录入的是哪一种，所以在 Android 上录入了指纹时回答指纹，否则在设备有面部识别硬件时回答面部。`cx.AuthenticateBiometric` 带着一个不能为空的原因请系统确认用户身份，回答成功、失败、取消或不可用。它不会替应用选择密码作为后备方式。

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("确认删除这个账户", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

使用面容 ID 的应用要在 `apple.info` 下声明 `NSFaceIDUsageDescription`；Android 应用要在 `android.permissions` 中列出 `USE_BIOMETRIC`。

## 界面方向

在全屏播放器的作用域内调用 `cx.RequestOrientation(core.OrientationLandscape)`，可请求 iOS、Android 或浏览器将界面切换到横屏。仍存活的作用域中，最新的请求生效。作用域结束后，恢复上一个请求，或者回到应用原有的方向策略。

在 iOS 16 及更新版本上，请求可立即旋转界面。在较早的 iOS 版本上，请求限制下次系统旋转可选的方向。已显示的模态视图的 `IOS.SupportedOrientations` 优先。Android 无法表达某些横竖屏的组合，此时会固定到其中一个方向，优先保留当前方向。在大屏幕或多窗口模式下，Android 可能忽略方向请求。

在 Web 应用中，从打开播放器作用域的按压回调调用 `desktop.ToggleFullScreen(cx)`。浏览器在这次按压后进入全屏，NSGo 随后应用播放器的方向请求。离开全屏会释放方向锁。浏览器控制台会报告不支持和被拒绝的操作。

`nsgotest.App.Orientation` 记录最新请求的方向集合，零表示使用应用原有的方向策略。

## 应用内评分

在 macOS 和 iOS 上，`cx.RequestReview()` 向 App Store 请求显示应用内评分弹窗。在 Android 上，它向 Google Play 发起请求。`cx.Env().CanRequestReview()` 表示此服务是否可用。商店自行决定是否显示以及何时显示，且不返回结果。请用开发构建验证 Apple 的评分弹窗，TestFlight 构建会隐藏它。

独立 Android 安装包包含 Play Review SDK、弹窗 Activity 和 Play 商店可见性查询，构建时无需 Android SDK 或 JDK。设备没有启用的 Play 商店时，此服务不可用。通过[内部应用分享](https://developer.android.com/guide/playcore/in-app-review/test)验证发行构建。

嵌入 NSGo 时，将 `com.google.android.play:review:2.0.2` 加入 Gradle 宿主，并在宿主清单的 `<queries>` 下添加 `<package android:name="com.android.vending" />`，使应用能够查询 Play 商店。如果宿主会压缩代码，请加入以下规则保留 NSGo 按名称加载的 Play Review 和 Tasks API。

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

测试时，在 `nsgotest.Environment` 中设置 `CanRequestReview`，并检查 `nsgotest.App.ReviewRequests`。

## Android 上的悬浮窗

`mobile.OpenOverlay` 在 Android 上把应用的一个视图显示在其他应用之上：它是一棵独立的树，与应用共享信号，显示期间由前台服务让应用保持运行。应用要在 `android.permissions` 中列出 `SYSTEM_ALERT_WINDOW`，并由用户在系统设置中授权，`mobile.RequestOverlayPermission` 会打开这个设置；没有授权时悬浮窗立即结束，`OnEnd` 会得知原因。它接收触摸，但按键和窗口以外的触摸仍交给前台应用，除非设为 `Focusable` 或 `Passive`；在内容里用 `desktop.SetWindowFrame` 移动它。其他平台不会打开悬浮窗。

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## 音频播放

短音效用 `audio.LoadSound` 加载，可多次重叠播放。需要暂停、跳转、调速或循环的流式音频用 `audio.NewPlayer`。`cx.Env().CanPlayAudio()` 报告平台是否支持。macOS、iOS、Windows、Linux、Android 和 Web 已能播放音频：Apple、Windows 和浏览器后端支持 WAV、AAC 和 MP3，Linux 使用 GStreamer 解码。Android 会在视图离开屏幕时暂停播放器，每个音效解码后最多 1 MiB。tvOS、watchOS、HarmonyOS 和小程序仍将 `CanPlayAudio` 报告为 false。

浏览器需要用户操作才能播放有声内容。可提前加载短音效，在点击、轻触或按键后调用 `Sound.Play`。用户操作之前播放的音效无声。如果浏览器拒绝播放，播放器的 `Play` 回调会收到 `audio.NotAllowed`。可在下一次用户操作中重试。

## 音频时序

用 `audio.Now(cx)` 读取预约播放的时钟。`audio.CanSchedule(cx)` 为 true 时，可将 `audio.Voice.StartAt` 设置为将来的时刻。调用 `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` 可读取当前音频输出路径的延迟。macOS、iOS、Linux 和 Web 支持预约播放，回调中的 `known` 表示当前路径是否报告了延迟。

## 下一步

- [通知](notifications.md)：权限、图标角标和点按。
- [表单](forms.md)：文本框和键盘。
- [嵌入](embedding.md)：宿主应用提供给嵌入应用的服务。
- [构建与发布](building.md)：应用包需要的权限和文件。
