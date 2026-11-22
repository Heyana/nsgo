---
title: 通知
description: 请求显示通知的权限，显示、定时和关闭通知，在应用图标上显示数字和任务进度并接收点按。
group: 平台功能
order: 45
---

## 权限

`cx.RequestNotificationPermission` 在用户做需要通知的操作时请求显示通知的权限，第一次会显示系统弹窗，并以 `core.NotificationPermission` 回答：`NotificationsGranted`、`NotificationsDenied`，或在平台或构建版本不显示通知时为 `NotificationsUnavailable`。iOS 和 Android 只会询问一次，浏览器也会拒绝用户没有触发的弹窗，所以要在用户做需要通知的操作时再请求。`cx.NotificationPermission` 不打扰用户地读取结果。

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "一起吃午饭？"}, nil)
	}
})
```

## 显示通知

`cx.ShowNotification` 显示一条通知，应用在前台时也显示，并替换同一 `ID` 的通知；`Silent` 让通知不带系统的声音和振动。它通过 `done` 回答系统是否接受了这条通知，没有权限时为 false；对于没有 ID 或标题的通知，它返回 `core.ErrNotification`，且不调用 `done`。`cx.CloseNotification` 移除 ID 指定的通知。

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "已同步", Body: "12 条笔记"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` 移除应用显示过的所有通知，并取消它定时的通知。在 macOS、iOS、Windows 和 Android 上，这也包括之前运行留在通知中心、操作中心或通知栏里的通知；在 Linux 和 Web 上，它只能移除本次运行显示的通知。应用的窗口已经显示了通知所说的内容时，在应用回到前台时调用它：

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## 定时通知

`At` 晚于当前时间的通知会被定时：系统到时显示它，无论应用是否在运行；用同一 `ID` 再次调用 `cx.ShowNotification` 会改动它的时间，在它显示之前调用 `cx.CloseNotification` 会取消它。`Repeat` 让它每天在 `At` 的时刻显示，或者每周在 `At` 的星期几的那个时刻显示，从下一个这样的时刻开始，按设备的时区计算。`cx.Env().CanScheduleNotifications()` 说明平台是否能定时：在应用包中运行的 macOS 和 iOS 应用、Android 和 Windows，其中 Windows 不支持 `Repeat`。在其他平台上，`done` 回答 false。

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "有卡片要复习", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "练习",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

手机处于低电耗模式时，Android 可能在定时通知的时间过后几分钟才显示它；需要准时显示的应用（如闹钟）要在 `android.permissions` 中列出 `android.permission.SCHEDULE_EXACT_ALARM`，从 Android 12 起由用户在系统设置中授予；或者列出 `USE_EXACT_ALARM`，Google Play 只允许闹钟和日历应用使用它。被强行停止的 Android 应用，它的定时通知要等到下次启动时才会显示。验证进程结束后的投递时，先等 `ShowNotification` 回调返回 true，再结束应用进程，不要强行停止整个应用包。

## 点按

`cx.OnNotificationPress` 在设置它的组件存活期间，以用户点按的通知的 `ID` 调用函数，并把应用带到前台。启动应用的那一次点按会交给第一个注册的处理函数。把点按要打开的内容记在 `ID` 里（如 `"chat:42"`），因为点按也可能重新启动应用。在 Windows 上，只有由安装程序放进开始菜单的应用才会被点按重新启动；以其他方式运行的应用只在运行期间收到点按。

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

从 Android 13 起请求权限的 Android 应用要在 `nsgo.yaml` 的 `android.permissions` 中列出 `android.permission.POST_NOTIFICATIONS`。

## 图标角标

`cx.SetBadge` 在应用图标上显示数字：macOS 是程序坞图标，iOS 是主屏幕图标（需要通知权限），Windows 是任务栏按钮，兼容 Unity 的 Linux 启动器，以及已安装的 Web 应用。0 或更小的数字会清除它。macOS 应用在查询或请求通知权限、或显示通知之前，程序坞角标不需要任何权限；此后，在当次及之后的运行中，只有用户在系统设置中允许该应用的标记时程序坞才显示角标，`cx.RequestNotificationPermission` 会连同提醒一起请求它。没有 Linux 角标接口的启动器和 Android 没有可以设置的数字；Android 启动器会标记有通知显示的应用。

```go
cx.SetBadge(len(unread.Get()))
```

## 任务进度

`cx.SetTaskProgress` 在应用图标上显示一个耗时任务的进度：macOS 是程序坞图标上的进度条，Windows 是任务栏按钮，以及兼容 Unity 的 Linux 启动器。它不需要权限，并与角标同时显示。`TaskProgressNormal` 显示 `Value`，即 0 到 1 之间的完成比例；`TaskProgressPaused` 和 `TaskProgressError` 在 Windows 和 macOS 上用颜色区分；`TaskProgressIndeterminate` 在 Windows 上会滚动，在 macOS 和 Linux 上是静止的半满进度条；零值 `TaskProgress` 会清除它。在 Windows 上，进度显示在该作用域所在窗口的按钮上；在没有自己任务栏按钮的地方设置的进度，例如应用自身的作用域或托盘面板，会显示在应用中所有没有自己进度的窗口上。iOS、Android 和 Web 不显示任何内容。

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // 还没有完成比例
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // 完成
```

## 下一步

- [系统权限](permissions.md)：系统的其他权限，例如麦克风和辅助功能。
- [触感反馈](haptics.md)：设备在用户手指下播放的反馈。
- [平台服务](services.md)：警告框、剪贴板、文件、存储和网络。
- [实时活动](live-activities.md)：应用跟随的事，显示在锁屏和灵动岛里。
- [构建与发布](building.md)：应用包需要的权限。
