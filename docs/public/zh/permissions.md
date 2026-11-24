---
title: 系统权限
description: 检查和请求系统权限，打开对应的设置页面，并在 macOS 上引导用户到系统设置中授权。
group: 平台功能
order: 49
---

## 检查和请求

系统替用户保管着应用的一些能力，由用户授予：手机和 Mac 上的麦克风、相机、通讯录、日历、提醒事项、照片、蓝牙、媒体资料库、本地网络、定位和通知，手机上的运动与后台定位，以及 macOS 上的辅助功能、录屏、输入监控、完全磁盘访问权限、App 管理、开发者工具和控制其他应用的自动化权限（[定位与运动](services.md#定位与运动)、[通知](notifications.md)）。`core.Permissions()` 以 `core.Permission` 键列出它们。`cx.PermissionStatus` 不打扰用户地回答应用是否已有某项权限，`cx.RequestPermission` 在系统有弹窗且尚未询问过时用系统弹窗请求。请在用户做需要该权限的操作时再请求。面容 ID 和触控 ID 不在其中：`cx.BiometricAvailability` 回答它们是否可用，`cx.AuthenticateBiometric` 用它们自己的弹窗请求。

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

在 macOS 上，自动化权限按目标应用分别授予：传入 `core.PermissionAutomationOf("com.apple.finder")` 可按 bundle ID 检查或请求控制正在运行的目标应用；传入 `core.PermissionAutomation` 则用于打开系统设置的自动化面板。目标应用未运行时回答 `PermissionUnknown`。

| 状态 | `String()` | 含义 |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | 尚未询问：请求会显示系统弹窗 |
| `PermissionGranted` | `granted` | 应用已有该权限 |
| `PermissionLimited` | `limited` | 部分访问权限，例如选定的照片或联系人 |
| `PermissionDenied` | `denied` | 已被拒绝，只能在系统设置中更改 |
| `PermissionUnknown` | `unknown` | 系统保管该权限，但系统或平台不告知 |
| `PermissionNotGated` | `not-gated` | 没有什么限制应用使用它，例如在 Linux 上 |
| `PermissionUnavailable` | `unavailable` | 平台没有这项权限 |

`Allowed` 对已授予、部分访问和不受限都为真。macOS 在用户打开之前对辅助功能、录屏和输入监控都回答已拒绝，因为请求只会把应用列进对应面板；录屏的授权要等应用重新启动后才能读到。

## 用途说明

在 macOS 和 iOS 上，请求权限需要应用的 Info.plist 写明用途，由 `nsgo.yaml` 的 `apple.info` 提供；缺少时请求回答不可用并在日志中写出所需的键，因为系统会在询问时终止应用。应用[文案](localization.md)中与之同键的文案（例如 `locales/en.yaml` 中的 `NSCameraUsageDescription: Scans the codes you point it at.`）会翻译这段用途说明，系统随后用用户的语言询问；没有翻译它的语言使用源语言的文案。

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: 录制你的语音笔记。
    NSCameraUsageDescription: 扫描你对准的二维码。
    NSContactsUsageDescription: 查找已在使用此应用的朋友。
    NSCalendarsFullAccessUsageDescription: 把你的预订加入日历。   # macOS 14 和 iOS 17 起；之前为 NSCalendarsUsageDescription
    NSRemindersFullAccessUsageDescription: 把任务变成提醒事项。   # 之前为 NSRemindersUsageDescription
    NSPhotoLibraryUsageDescription: 从图库导入照片。
    NSBluetoothAlwaysUsageDescription: 查找你的耳机。
    NSAppleMusicUsageDescription: 播放你的资料库。                # iOS
    NSLocalNetworkUsageDescription: 查找要同步的电脑。            # iOS、macOS 15
    NSAppleEventsUsageDescription: 在访达中显示文件。             # macOS 自动化
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+；Android 9–11 为 BLUETOOTH
    - READ_MEDIA_AUDIO       # Android 13+；Android 9–12 为 READ_EXTERNAL_STORAGE
windows:
  device_capabilities: [microphone, webcam]  # store 构建的 MSIX 声明的设备
```

在 Android 上，每项权限都必须在 `nsgo.yaml` 的 `android.permissions` 中声明；清单中未声明的权限回答 `PermissionUnavailable`。在 `nsgo dev` 下，macOS 把应用的行为记在启动它的终端名下，询问和列出的都是终端；要测试应用自己的权限，请先构建应用。

## 本地网络

从 iOS 14 和 macOS 15 起，在用户允许应用查找并连接本地网络上的设备之前，连接本地网络设备会失败，而系统既不告诉应用是否允许，也不告诉它是否询问过。请在第一次连接之前请求 `core.PermissionLocalNetwork`，例如在用户开始配对时：请求会发布并浏览一个 NSGo 自己的 Bonjour 服务，第一次时由此显示系统弹窗，等用户回答后回答已授予或已拒绝，两分钟内没有回答则为未知。`cx.PermissionStatus` 回答本次运行中最近一次请求得知的状态，此前为未知。Android、Windows 和 Linux 回答不受限，macOS 15 之前以及从终端启动的程序（例如 `nsgo dev` 下）也是如此；Web 回答不可用。

```go
cx.RequestPermission(core.PermissionLocalNetwork, func(s core.PermissionStatus) {
	if s.Allowed() {
		pair()
	} else if s == core.PermissionDenied {
		showLocalNetworkOff()
	}
})
```

请求需要在 `apple.info` 中写 `NSLocalNetworkUsageDescription`。写了之后，`nsgo build` 会把这个服务的类型 `_nsgo-lnp._tcp` 加在应用自己的类型之后列进 `NSBonjourServices`，因为 iOS 拒绝浏览 Info.plist 没有列出的类型；把 NSGo 嵌入自己宿主的应用要在宿主的 Info.plist 中列出这个类型。

## 打开设置

`cx.OpenPermissionSettings` 打开授予某项权限的页面，并回答是否打开：macOS 上是“隐私与安全性”中该权限的面板（包括自动化、通讯录、照片和定位服务，macOS 15 起还有本地网络）或“通知”，iOS 上是“设置”中本应用的页面（通知从 iOS 16 起直接打开通知设置页），Android 上是“设置”中本应用的页面，Windows 上是麦克风、相机或通知的页面。

对于 `PermissionLocation` 和 `PermissionBackgroundLocation`，macOS 打开“定位服务”；`PermissionMotion` 打开“运动与健身”，但 macOS 尚不提供运动采样。iOS 和 Android 对这三项权限都打开本应用的设置页面。打开设置不会请求或授予权限。

## 引导用户

`desktop.GuidePermission` 打开该页面并等待用户授予权限。在 macOS 上，一个面板跟随系统设置的窗口且不抢焦点；对于列表接受拖入应用的面板，它会展示应用，让用户把应用拖进列表。应用获得权限、用户关闭面板或系统设置、调用 `stop` 或组件销毁时，引导结束，回调收到当时的状态；新的引导会结束之前的引导。

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

展示的应用是它的应用包，没有应用包时是可执行文件；`GuideOptions.App` 可以指定其他应用，`Name` 指定面板中显示的名称。Windows 打开页面并等待“设置”关闭。其他平台上回调立即收到当前状态。

## 各平台

| 平台 | 能读取和请求的权限 |
| --- | --- |
| macOS | 全部权限（自动化通过 `PermissionAutomationOf` 按运行中的目标应用检查与请求，打包应用还支持通知）；不含目标的自动化、App 管理、开发者工具和媒体与 Apple Music 为未知；运动为不可用；macOS 15 起的本地网络由请求得知 |
| iOS | 麦克风、相机、通讯录、日历、提醒事项、照片（支持 `PermissionLimited`）、蓝牙、媒体资料库、由请求得知的本地网络、定位、运动和通知 |
| Windows | 麦克风、相机和通知，但从不弹窗请求；其余不受限 |
| Linux | 沙盒外不受限；通知反映通知服务是否响应；在 Flatpak 和 Snap 中通过桌面 Portal 读取和请求相机，通过 Portal 与 PipeWire 读取和请求麦克风 |
| Web | 麦克风和相机（在 Firefox 中为未知）、通知、位置（不含后台位置）以及移动浏览器上的运动 |
| Android | 在 `android.permissions` 中声明了的麦克风、相机、日历、蓝牙和媒体资料库；本地网络不受限 |
| Apple Watch、小程序 | 通知、定位和运动走各自服务；其余系统保管的权限目前为未知 |

定位和运动权限在有对应传感器的平台上都能读取和请求，`PermissionNotifications` 与[通知](notifications.md)一致。在移动 Safari 上，请在点按处理函数中请求 `PermissionMotion`。

## 测试

`nsgotest` 用它的模型回答：`app.Permissions` 保存每项权限的状态，初始为尚未询问；请求尚未询问的权限时，状态变为 `app.PermissionAnswers` 中给它的值，没有时为已授予。`PermissionNotifications` 与 `app.NotificationPermission` 和 `app.NotificationAnswer` 保持同步。`app.PermissionSettings` 列出打开过的设置，`app.Guide` 是正在显示的引导，`app.GrantPermission` 和 `app.CloseGuide` 会结束它。

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## 下一步

- [通知](notifications.md)：通知自己的权限、角标和点按。
- [构建与发布](building.md)：安装包声明的授权和能力。

健身服务使用 `PermissionActivityRecognition`（Android 的 `ACTIVITY_RECOGNITION`，Apple 的 `NSMotionUsageDescription`），Apple 气压也需这一授权；Android 原始运动传感器权限保持独立。
