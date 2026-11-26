---
title: 画中画
description: 在 iOS、tvOS、macOS、Android 和浏览器画中画中播放视频，或在 iOS 中显示自定义元素树。
group: 平台功能
order: 45
---

在 iOS 上使用画中画前，先声明后台播放模式；macOS 不需要后台模式：

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

`AutoPictureInPicture` 允许 iOS 应用进入后台时自动开启画中画；tvOS 和 macOS 忽略此选项。`Controls` 仍显示系统播放控件；带控件的视频也通过应用按钮使用同一个受管理的画中画会话。用户返回应用时，`OnPictureInPictureRestore` 可重新显示播放器页面。画中画活动期间应保留视频：删除其作用域会停止播放和画中画。

Android 打包应用的 Activity 已声明画中画支持。进入画中画后，窗口只显示视频，应用其余界面保留但不显示；返回时继续使用同一播放器和播放位置。`StopPictureInPicture` 展开同一个 Activity。窗口的播放、暂停控件通过 `OnPlay`、`OnPause`（原因是 `VideoPauseControls`）报告操作，`Ref.Seek` 仍控制同一个播放器。自动进入需要显式启用，且视频已就绪并正在播放。系统当前状态仍可能拒绝启动，因此即使 `Possible` 为 true，也应处理 `Error`。

嵌入的 Android 宿主需要声明 `android:supportsPictureInPicture="true"`，并处理 `orientation|screenSize|screenLayout|smallestScreenSize` 配置变化。将 Activity 的 `onPictureInPictureModeChanged` 和 `onUserLeaveHint` 转发给 `NsgoView` 的同名方法；Android 15 及以后，还应将 `PictureInPictureUiState.isTransitioningToPip()` 转发给 `NsgoView.onPictureInPictureTransition`。宿主或设备不支持时，状态为 `Supported: false`。

网页视频应在支持标准视频画中画 API 的浏览器中从用户点击启动。`Supported` 也反映页面的 Permissions Policy，`Possible` 要求视频元数据已加载。浏览器自带画中画控件报告相同的活动状态。停止、更换或清空来源、删除视频会结束其会话；未完成的启动请求不能重新打开旧来源。网页画中画仅显示视频，不支持自定义 NSGo 内容、自动进入或 `OnPictureInPictureRestore`。不支持的浏览器报告 `Supported: false`。

iOS 上 `mobile.PictureInPicture` 的 `Children` 使用独立布局，默认为 320 × 180 点，在应用中不占空间。其 `Ref` 使用相同的启动和停止方法，`OnState`、`OnRestore`、`OnPlay`、`OnPause` 和 `OnSkip` 接收系统操作。用 `PausedFunc`、`PositionFunc` 绑定这些回调更新的播放状态；`Duration: 0` 表示直播内容。应用挂起时保留最后一帧。

在自定义树中放置 `core.Video`，可将其解码画面与应用绘制的字幕或其他叠层合成。使用外层 `Ref` 启动画中画，让视频与会话共用暂停信号，由 `Video.OnProgress` 为 `PositionFunc` 更新进度，并将 `OnSkip` 转发给视频的 `Ref.Seek`。声音仍由视频播放器播放。

自定义树也可显示文字、视图、图片和 Canvas。捕获画面不支持 WebView、Metal 着色器和背景效果。动图和原生图层过渡显示其模型状态。需要动态内容时使用显式动画值。App Store 审核可能拒绝与视频无关的用途。

`Supported: false` 表示平台不可用，与尚未就绪或通过 `Error` 报告失败的受支持会话不同。视频支持 AVKit 报告具备画中画能力的 iOS 和 tvOS 设备、macOS、Android，以及支持标准 API 的网页浏览器；自定义元素树仅支持 iOS。在 iOS 和 tvOS 上启用画中画期间，共享音频会话保持在播放类别，暂停时也一样；视频的 `AudioSession` 为 `AudioSessionMixWithOthers` 时与其他应用的音频混音。

在 headless 测试中，`nsgotest.PictureInPictureSupport(supported, possible)` 在 iOS、macOS 或 Android 的 `nsgotest.Environment` 下设定系统提供的支持，视图的 `PiPState`、`PiPStarts` 和 `PiPStops` 记录会话，`app.ReportPictureInPicture` 报告系统会给出的状态，`app.PictureInPictureControl` 发送窗口的 `play`、`pause`、`skip` 和 `restore`。视频画中画窗口的播放和暂停作用于视频自身的播放器，由 `Video.OnPlay` 和 `Video.OnPause` 报告，暂停的原因是 `VideoPauseControls`；测试中用 `app.PlayVideo` 和 `app.PauseVideo` 报告。

运行 `nsgo dev -target ios examples/pip` 可试用两条路径，使用 `-target macos` 或 `-target android` 可试用视频画中画。Mac 原生测试覆盖有无系统控件时的进入、退出和重复命令，以及带控件的活动会话中的源替换。iOS 模拟器测试在 iPad 模拟器上验证了视频和自定义内容的进入、持续帧渲染与退出，并在 iPhone 模拟器上验证了明确的不支持状态；实体 iPhone 已验证手动与自动视频画中画、全屏恢复、自定义内容重绘与后台最后一帧保留，剩余待真机验证项为关闭自动进入时不触发画中画、与其他应用音频混音策略以及后台持续字幕视频合成。

运行 `nsgo dev -target tvos_simulator examples/pip` 可试用 tvOS 视频路径；NSGo 将播放器关联到系统 Now Playing 会话。tvOS 27 模拟器可加载带系统控件和不带控件的视频，但 AVKit 在此报告不支持画中画。实际进入、停止和恢复画中画仍需在实体 Apple TV 上验证。

网页目标回归测试在 Chromium 使用本地录制的视频和真实应用按钮点击，验证浏览器实际会话的进入、退出、来源更换与删除。其他浏览器引擎仍待验收。
