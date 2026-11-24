---
title: 햅틱
description: 사전 설정된 햅틱 피드백이나 시간 지정 펄스 시퀀스를 재생합니다.
group: 플랫폼 기능
order: 46
---

## 하나 재생하기

`mobile.PlayHaptic`은 iOS와 Android에서 사용자의 손가락 아래에 피드백을 재생하며, 다른 플랫폼에서는 아무 동작도 하지 않으므로 화면에 표시되는 내용과 함께 사용하세요. `HapticSelection`은 값이 바뀌거나 피커, 세그먼트, 슬라이더의 단계가 이동할 때를 표시합니다. `HapticLight`부터 `HapticRigid`까지는 요소가 충돌할 때 무게감과 탄성에 따라 사용합니다. `HapticSuccess`, `HapticWarning`, `HapticError`는 작업이 해당 상태로 끝날 때를 표시합니다.

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## 패턴 재생하기

`cx.Env().CanPlayHapticPattern()`은 시간 지정 햅틱을 사용할 수 있는지 보고합니다. 패턴에는 패턴 시작 시점을 기준으로 한 시작 시간 `At`을 가진 펄스들이 포함됩니다. `Duration`이 0이면 일시적인(transient) 펄스가 되고, 양수인 지속 시간은 최대 30초까지 펄스를 유지합니다. `Intensity`와 `Sharpness`의 범위는 0에서 1까지입니다. 새 패턴은 이전 패턴을 대체합니다. 재생을 중지하려면 빈 패턴을 전달하세요. 앱이 백그라운드로 이동하면 패턴 재생이 중지됩니다.

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS는 지원되는 하드웨어에서 Core Haptics를 통해 이벤트를 재생합니다. Android는 타이밍을 따르며 진동기가 진폭 제어를 지원하는 경우 강도(`Intensity`)를 사용합니다. `navigator.vibrate`를 지원하는 Android 브라우저는 사용자 상호 작용 이후에 켜짐/꺼짐 타이밍을 따릅니다. Android와 웹은 일시적인(transient) 펄스를 20ms 펄스로 재생하며, 선명도(`Sharpness`)는 iOS에서만 사용할 수 있습니다. 지원되지 않는 플랫폼에서는 이 호출이 아무런 영향을 미치지 않습니다.

테스트에서는 `nsgotest.Environment`를 통해 `HapticPatterns: true`를 설정하고 `app.HapticPatterns`를 읽으세요. 앱이 재생을 취소하면 기록에 nil 항목이 포함됩니다.

## 재생 시점

피드백은 응답이 도착할 때가 아니라 제스처가 일어나는 순간에 재생하세요. 값이 지나갈 때 선택(selection) 피드백을, 뷰가 안착할 때 충돌(impact) 피드백을, 작업이 끝날 때 결과(outcome) 피드백을 재생합니다. 사용자가 일으킨 변경에 대해서만 재생하고, 앱이 자체적으로 수행한 변경에는 재생하지 마세요.

## 다음 단계

- [알림](notifications.md): 동일한 소식을 전달하는 알림과 배지.
- [플랫폼 서비스](services.md): 시스템이 제공하는 나머지 기능.
- [접근성](accessibility.md): 촉각 피드백을 느끼지 못하는 사용자가 듣게 되는 안내.
