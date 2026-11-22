---
title: 画中画
description: 在 iOS 和 tvOS 画中画中播放视频，或在 iOS 中显示自定义元素树。
group: 平台功能
order: 45
---

使用画中画前，先声明后台播放模式：

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

视频启用 `PictureInPicture`，用 `core.Ref` 启动或停止，并通过状态回调在 `Possible` 为 true 时启用应用的按钮：

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "画中画", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture` 允许 iOS 应用进入后台时自动开启画中画；tvOS 忽略此选项。`Controls` 仍显示系统播放控件；带控件的视频也通过应用按钮使用同一个受管理的画中画会话。用户返回应用时，`OnPictureInPictureRestore` 可重新显示播放器页面。画中画活动期间应保留视频：删除其作用域会停止播放和画中画。

iOS 上 `mobile.PictureInPicture` 的 `Children` 使用独立布局，默认为 320 × 180 点，在应用中不占空间。其 `Ref` 使用相同的启动和停止方法，`OnState`、`OnRestore`、`OnPlay`、`OnPause` 和 `OnSkip` 接收系统操作。用 `PausedFunc`、`PositionFunc` 绑定这些回调更新的播放状态；`Duration: 0` 表示直播内容。应用挂起时保留最后一帧。

在自定义树中放置 `core.Video`，可将其解码画面与应用绘制的字幕或其他叠层合成。使用外层 `Ref` 启动画中画，让视频与会话共用暂停信号，由 `Video.OnProgress` 为 `PositionFunc` 更新进度，并将 `OnSkip` 转发给视频的 `Ref.Seek`。声音仍由视频播放器播放。

自定义树也可显示文字、视图、图片和 Canvas。捕获画面不支持 WebView、Metal 着色器和背景效果。动图和原生图层过渡显示其模型状态。需要动态内容时使用显式动画值。App Store 审核可能拒绝与视频无关的用途。

`Supported: false` 表示平台不可用，与尚未就绪或通过 `Error` 报告失败的受支持会话不同。视频支持 AVKit 报告具备画中画能力的 iOS 和 tvOS 设备，自定义元素树仅支持 iOS。macOS、Android 和网页的画中画仍不可用。启用画中画期间，共享音频会话保持在播放类别，暂停时也一样；视频的 `AudioSession` 为 `AudioSessionMixWithOthers` 时与其他应用的音频混音。

在 headless 测试中，`nsgotest.PictureInPictureSupport(supported, possible)` 在 iOS 的 `nsgotest.Environment` 下设定系统提供的支持，视图的 `PiPState`、`PiPStarts` 和 `PiPStops` 记录会话，`app.ReportPictureInPicture` 报告系统会给出的状态，`app.PictureInPictureControl` 发送窗口的 `play`、`pause`、`skip` 和 `restore`。视频画中画窗口的播放和暂停作用于视频自身的播放器，由 `Video.OnPlay` 和 `Video.OnPause` 报告，暂停的原因是 `VideoPauseControls`；测试中用 `app.PlayVideo` 和 `app.PauseVideo` 报告。

运行 `go tool nsgo dev -target ios examples/pip` 可试用两条路径。iPhone 验收仍待完成，包括带控件视频的全屏和自动进入；目前已覆盖原生委托、视频帧及 headless 应用测试。

运行 `go tool nsgo dev -target tvos_simulator examples/pip` 可试用 tvOS 视频路径；NSGo 将播放器关联到系统 Now Playing 会话。tvOS 27 模拟器可加载带系统控件和不带控件的视频，但 AVKit 在此报告不支持画中画。实际进入、停止和恢复画中画仍需在实体 Apple TV 上验证。
