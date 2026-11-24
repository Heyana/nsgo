---
title: 화면 속 화면
description: iOS, tvOS, macOS, Android 및 브라우저의 화면 속 화면(Picture in Picture)에서 비디오를 재생하거나 iOS에서 커스텀 트리를 표시합니다.
group: 플랫폼 기능
order: 45
---

iOS에서는 화면 속 화면(Picture in Picture)을 사용하기 전에 백그라운드 재생을 선언해야 하며, macOS에서는 백그라운드 모드가 필요하지 않습니다:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

비디오의 경우 `PictureInPicture`를 활성화하고 `core.Ref`를 보관한 뒤 앱 컨트롤에서 시작하거나 중지합니다. `Possible`이 true가 될 때 해당 컨트롤을 활성화하려면 상태 콜백을 따르세요:

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture`는 iOS 앱이 백그라운드로 전환될 때 자동으로 진입하도록 선택하며, tvOS와 macOS에서는 이 옵션을 무시합니다. `Controls`는 여전히 네이티브 재생 컨트롤을 표시하며, 앱의 PiP 버튼은 컨트롤이 있는 비디오에 대해 동일한 관리형 세션을 사용합니다. `OnPictureInPictureRestore`는 사용자가 돌아왔을 때 앱이 플레이어 페이지를 다시 표시할 수 있게 합니다. PiP가 활성화되어 있는 동안 비디오를 마운트된 상태로 유지하세요. 스코프를 삭제하면 재생과 PiP가 중지됩니다.

Android에서는 패키징된 액티비티가 이미 PiP 지원을 선언합니다. 비디오는 액티비티의 PiP 창으로 진입하며 앱 인터페이스의 나머지 부분은 숨겨지고, 돌아왔을 때 플레이어와 재생 위치를 유지합니다. `StopPictureInPicture`는 해당 액티비티를 다시 확장합니다. 창의 재생/일시 정지 컨트롤은 `VideoPauseControls`와 함께 `OnPlay` 및 `OnPause`를 보고하며, `Ref.Seek`는 동일한 플레이어를 사용합니다. 자동 진입에는 명시적인 설정과 준비되어 재생 중인 비디오가 필요합니다. 시스템의 현재 상태로 인해 시작이 실패할 수도 있으므로 `Possible`이 true였더라도 `Error`를 처리하세요.

임베딩된 Android 호스트는 `android:supportsPictureInPicture="true"`를 선언하고 `orientation|screenSize|screenLayout|smallestScreenSize` 구성 변경을 처리해야 합니다. 액티비티의 `onPictureInPictureModeChanged`와 `onUserLeaveHint`를 `NsgoView`의 동일한 메서드로 전달하고, Android 15 이상에서는 `PictureInPictureUiState.isTransitioningToPip()`도 `NsgoView.onPictureInPictureTransition`으로 전달하세요. 지원하지 않는 호스트나 기기는 `Supported: false`를 보고합니다.

웹에서는 표준 비디오 PiP API를 제공하는 브라우저에서 사용자의 누르기 동작으로 시작합니다. `Supported`는 페이지의 Permissions Policy도 반영하며, `Possible`에는 로드된 비디오 메타데이터가 필요합니다. 브라우저 자체의 PiP 컨트롤도 동일한 활성 상태를 보고합니다. 소스를 중지, 교체 또는 지우거나 비디오를 제거하면 세션이 종료되며, 대기 중인 시작 요청은 이전 소스를 다시 열 수 없습니다. 웹 PiP는 커스텀 NSGo 콘텐츠, 자동 백그라운드 진입 또는 `OnPictureInPictureRestore` 없이 비디오만 표시합니다. 지원하지 않는 브라우저는 `Supported: false`를 보고합니다.

iOS에서 `mobile.PictureInPicture`는 자체 레이아웃을 가진 `Children`을 받으며 기본 크기는 320 × 180포인트입니다. 앱 내에서 공간을 차지하지 않습니다. `Ref`에는 동일한 시작 및 중지 메서드가 있으며, `OnState`, `OnRestore`, `OnPlay`, `OnPause`, `OnSkip`은 시스템 액션을 전달합니다. 이러한 콜백이 업데이트하는 재생 상태에 `PausedFunc`와 `PositionFunc`를 바인딩하세요. `Duration: 0`은 라이브 콘텐츠를 나타냅니다. 일시 중단된 앱은 마지막으로 렌더링된 프레임을 유지합니다.

커스텀 트리에 `core.Video`를 배치하면 디코딩된 프레임을 앱에서 그린 자막이나 다른 오버레이와 합성할 수 있습니다. 바깥쪽 `Ref`를 사용하여 세션을 시작하세요. 비디오와 일시 정지 시그널을 공유하고, `Video.OnProgress`에서 커스텀 세션의 `PositionFunc`로 값을 전달하며, `OnSkip`을 비디오의 `Ref.Seek`로 전달하세요. 비디오의 오디오는 자체 플레이어를 통해 재생됩니다.

커스텀 트리는 텍스트, 뷰, 이미지 및 Canvas도 렌더링합니다. WebView, Metal 셰이더 및 백드롭 효과는 캡처된 프레임에서 사용할 수 없습니다. 애니메이션 이미지와 네이티브 레이어 트랜지션은 모델 상태를 표시합니다. 모션에는 명시적인 애니메이션 값을 사용하세요. App Store 심사에서는 비디오와 관련 없는 용도의 사용을 거부할 수 있습니다.

`Supported: false`는 사용할 수 없는 플랫폼을, 준비되지 않았거나 `Error`로 실패 원인을 설명하는 지원되는 세션과 구분합니다. 비디오는 AVKit이 기기의 PiP 지원을 보고하는 경우 iOS와 tvOS를 지원하며, macOS, Android 및 표준 API를 제공하는 웹 브라우저를 지원합니다. 커스텀 트리는 iOS만 지원합니다. iOS와 tvOS에서 공유 오디오 세션은 일시 정지 상태를 포함해 PiP가 활성화된 동안 재생 카테고리로 유지되며, 비디오의 `AudioSession`이 `AudioSessionMixWithOthers`인 경우 다른 앱의 오디오와 믹싱됩니다.

헤드리스 테스트에서 `nsgotest.PictureInPictureSupport(supported, possible)`은 iOS, macOS 또는 Android `nsgotest.Environment`에서 시스템이 제공하는 상태를 설정합니다. 뷰의 `PiPState`, `PiPStarts` 및 `PiPStops`는 세션을 기록하고, `app.ReportPictureInPicture`는 시스템이 보고할 상태를 보고하며, `app.PictureInPictureControl`은 창의 `play`, `pause`, `skip` 및 `restore`를 전송합니다. 비디오의 PiP 창은 비디오 자체 플레이어를 재생 및 일시 정지하며, 이는 `Video.OnPlay` 및 `Video.OnPause`가 일시 정지에 대한 `VideoPauseControls`와 함께 보고합니다. 테스트에서는 `app.PlayVideo` 및 `app.PauseVideo`가 이를 보고합니다.

두 경로 모두에 대해 `go tool nsgo dev -target ios examples/pip`를 실행하거나, 비디오 PiP의 경우 `-target macos` 또는 `-target android`를 실행하세요. 네이티브 Mac 테스트는 네이티브 컨트롤이 있는 경우와 없는 경우 모두에서 진입, 종료, 반복 명령 및 활성화된 컨트롤 있는 소스 교체를 검증합니다. 컨트롤이 있는 비디오의 전체 화면과 자동 진입을 포함한 iPhone 검증은 아직 대기 중이며, 현재 구현에는 네이티브 델리게이트/프레임 테스트와 헤드리스 앱 테스트 커버리지가 포함되어 있습니다.

tvOS 비디오의 경우 `go tool nsgo dev -target tvos_simulator examples/pip`를 실행하세요. NSGo는 플레이어를 네이티브 Now Playing 세션과 연결합니다. tvOS 27 시뮬레이터는 네이티브 컨트롤이 있는 비디오와 없는 비디오를 모두 로드하지만, AVKit은 해당 환경에서 PiP가 지원되지 않는다고 보고합니다. 실제 PiP 진입, 중지 및 복원은 여전히 실제 Apple TV 기기에서 검증해야 합니다.

웹 대상 회귀 테스트는 녹화된 로컬 클립과 신뢰할 수 있는 앱 버튼 누르기 동작을 사용하여 Chromium에서 진입, 종료, 소스 교체 및 삭제 전반에 걸쳐 브라우저의 실제 PiP 세션을 검증합니다. 다른 브라우저 엔진은 아직 검증이 필요합니다.
