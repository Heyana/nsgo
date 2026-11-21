---
title: 触感反馈
description: 播放预设触感反馈或定时的脉冲序列。
group: 平台功能
order: 46
---

## 播放

`mobile.PlayHaptic` 在 iOS 和 Android 上播放用户手指下的触感反馈，其他平台不做任何事，因此要配合用户看得见的反馈。`HapticSelection` 表示值发生变化，如选择器、分段控件或滑块的步进；`HapticLight` 到 `HapticRigid` 表示元素按其重量和弹性碰撞；`HapticSuccess`、`HapticWarning` 和 `HapticError` 表示任务以相应结果结束。

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## 播放序列

`cx.Env().CanPlayHapticPattern()` 报告是否支持定时触感反馈。序列由脉冲组成，`At` 是相对于序列开始的时间。`Duration` 为零时播放瞬态脉冲，正值表示持续播放，最长 30 秒。`Intensity` 和 `Sharpness` 的范围是 0 到 1。新序列会替换上一个序列，传入空序列可停止播放。应用进入后台时，序列也会停止。

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS 在支持的硬件上通过 Core Haptics 播放事件。Android 按时间播放，振动器支持振幅控制时也使用强度。支持 `navigator.vibrate` 的 Android 浏览器会在用户交互后按启停时间播放。Android 和 Web 将瞬态播放为 20 毫秒的脉冲，锐度由 iOS 支持。不支持的平台不执行播放。

测试通过 `nsgotest.Environment` 设置 `HapticPatterns: true`，并读取 `app.HapticPatterns`。取消播放会在播放记录中留下一条 nil。

## 何时播放

要在手势发生时播放，而不是结果返回时：值经过时给一次选择，视图落位时给一次碰撞，任务结束时给一次结果。只为用户造成的变化播放，应用自己发生的事不播放。

## 下一步

- [通知](notifications.md)：传达同样消息的通知和图标角标。
- [平台服务](services.md)：系统提供的其余能力。
- [无障碍](accessibility.md)：感受不到反馈的用户听到的播报。
