---
title: 알림
description: 알림 표시 권한 요청, 알림 표시·예약·닫기, 배지 카운트 및 앱 아이콘의 작업 진행률 표시, 알림 누름 감지를 다룹니다.
group: 플랫폼 기능
order: 45
---

## 권한

`cx.RequestNotificationPermission`은 앱이 알림을 표시할 수 있도록 사용자에게 요청하며, 처음 호출할 때 시스템 프롬프트를 띄우고 `core.NotificationPermission` 값으로 응답합니다. 이 값은 `NotificationsGranted`, `NotificationsDenied`, 또는 플랫폼이나 빌드에서 알림을 표시할 수 없는 경우 `NotificationsUnavailable`입니다. 사용자가 알림이 필요한 작업을 수행할 때 요청하세요. iOS와 Android는 한 번만 묻고, 브라우저는 사용자 동작으로 발생하지 않은 프롬프트를 거부합니다. `cx.NotificationPermission`은 묻지 않고 현재 응답 상태를 읽습니다.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## 알림 표시

`cx.ShowNotification`은 앱이 전면에 있는 동안에도 알림을 표시하며, 같은 `ID`를 가진 기존 알림을 대체합니다. `Silent`를 설정하면 시스템 소리와 진동을 생략합니다. 시스템이 알림을 수락했는지 여부를 `done` 콜백으로 전달하며 권한이 없으면 false를 전달합니다. ID나 제목이 없는 경우에는 `done`을 호출하지 않고 `core.ErrNotification`을 반환합니다. `cx.CloseNotification`은 지정한 ID의 알림을 제거합니다.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications`는 앱이 표시한 모든 알림을 제거하고 예약된 알림을 취소합니다. macOS, iOS, watchOS, Windows, Android에서는 이전 실행에서 알림 센터, 알림 창(Action Center), 또는 알림 창(shade)에 남겨진 알림까지 포함되며, Linux와 웹에서는 현재 실행에서 표시하거나 예약한 알림에만 적용됩니다. 앱의 창이 알림이 전하는 내용을 이미 보여줄 때 앱이 전면으로 올라오면 이 함수를 호출하세요:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## 알림 예약

`At`이 현재 시각보다 나중인 알림은 예약되며, 같은 `ID`로 `cx.ShowNotification`을 다시 호출하면 예약 시각이 이동하고, 표시되기 전에 `cx.CloseNotification`을 호출하면 예약이 취소됩니다. `Repeat`는 기기의 시간대를 기준으로 다음 해당 시각부터 매일 `At`과 같은 시각에, 또는 매주 같은 요일과 시각에 알림을 표시합니다. `cx.Env().CanScheduleNotifications()`는 플랫폼이 알림 예약을 지원하는지 알려줍니다. 번들된 macOS, iOS, watchOS 앱과 Android, Windows는 앱의 실행 여부와 무관하게 네이티브로 알림을 예약하며(Windows는 앱이 실행되는 동안 일간 및 주간 반복을 연쇄 등록하고 앱 시작 시 다시 설정합니다), Linux와 웹(`Notification`을 사용할 수 있는 경우)은 앱이나 페이지가 실행되는 동안에만 예약을 유지합니다. 그 외 플랫폼(알림 센터가 배지만 표시하는 tvOS 및 미니 프로그램)에서는 `done`이 false로 응답합니다.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android는 기기가 잠자기(doze) 상태일 때 예약된 알림을 지정된 시각보다 몇 분 늦게 표시할 수 있습니다. 알람 시계처럼 정각에 맞춰 표시해야 하는 앱은 `android.permissions`에 `android.permission.SCHEDULE_EXACT_ALARM`(Android 12부터 사용자가 시스템 설정에서 허용) 또는 `USE_EXACT_ALARM`(Google Play에서는 알람 및 캘린더 앱에만 허용)을 나열해야 합니다. 강제 종료된 Android 앱의 알림은 앱이 다음에 시작될 때까지 대기합니다. 프로세스 종료 후 알림 전달을 테스트하려면 `ShowNotification`이 true로 응답할 때까지 기다린 다음, 패키지를 강제 종료(force-stop)하지 않고 프로세스만 종료하세요.

## 알림 누름

`cx.OnNotificationPress`는 핸들러를 설정한 컴포넌트가 살아 있는 동안 사용자가 누른 알림의 `ID`를 전달해 함수를 실행하고 앱을 전면으로 가져옵니다. 앱을 새로 시작한 알림 누름은 가장 먼저 등록된 핸들러로 전달됩니다. 알림을 눌러 앱이 새로 시작될 수도 있으므로, 눌렀을 때 열어야 할 대상을 `ID`에 담아 두세요(`"chat:42"`). Windows에서는 인스톨러가 시작 메뉴에 등록한 앱에 한해 앱을 새로 시작하며, 그렇지 않은 실행에서는 앱이 실행 중일 때만 누름 이벤트를 받습니다.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Android 13 이상에서 알림 권한을 요청하는 Android 앱은 `nsgo.yaml`의 `android.permissions`에 `android.permission.POST_NOTIFICATIONS`를 나열해야 합니다.

## 배지

`cx.SetBadge`는 앱 아이콘에 숫자를 표시합니다. macOS에서는 Dock 아이콘, iOS에서는 홈 화면 아이콘(알림 권한 필요), Windows에서는 작업 표시줄 버튼, Unity 호환 Linux 런처 및 설치된 웹 앱의 아이콘에 표시됩니다. 0 이하의 값을 전달하면 배지가 지워집니다. macOS 앱은 알림 권한을 확인하거나 요청하거나 알림을 표시하기 전까지는 Dock 배지에 권한이 필요하지 않지만, 그 이후부터는 현재 실행과 이후 실행 모두에서 사용자가 시스템 설정에서 앱의 배지를 허용한 경우에만 Dock에 배지가 표시됩니다(`cx.RequestNotificationPermission`을 호출할 때 경고 알림과 함께 배지 권한도 요청합니다). Linux 배지 인터페이스가 없는 런처와 Android에는 설정할 숫자가 없으며, Android 런처는 표시 중인 알림이 있는 앱에 표식을 보여줍니다.

```go
cx.SetBadge(len(unread.Get()))
```

## 작업 진행률

`cx.SetTaskProgress`는 오래 걸리는 작업의 진행 정도를 앱 아이콘 위에 표시합니다. macOS에서는 Dock 타일 위, Windows에서는 작업 표시줄 버튼 위, 그리고 Unity 호환 Linux 런처에 막대로 나타납니다. 별도의 권한이 필요 없으며 배지 옆에 함께 표시됩니다. `TaskProgressNormal`은 0에서 1 사이의 비율인 `Value`를 표시하고, `TaskProgressPaused`와 `TaskProgressError`는 Windows와 macOS에서 막대에 해당 상태의 색상을 입히며, `TaskProgressIndeterminate`는 Windows에서는 움직이는 막대로, macOS와 Linux에서는 절반이 채워진 정지 막대로 표시됩니다. 제로 값 `TaskProgress`는 진행률 표시를 지웁니다. Windows에서는 해당 스코프 창의 작업 표시줄 버튼에 표시되며, 앱 자체 스코프나 트레이 패널처럼 자체 버튼이 없는 창에서 설정된 진행률은 별도의 진행률을 설정하지 않은 앱의 모든 창에 표시됩니다. iOS, Android, 웹에서는 아무것도 표시되지 않습니다.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## 다음 단계

- [권한](permissions.md): 마이크와 손쉬운 사용 같은 시스템의 다른 권한.
- [햅틱](haptics.md): 사용자의 손가락 아래에서 기기가 재생하는 촉각 피드백.
- [플랫폼 서비스](services.md): 알림창, 클립보드, 파일, 저장소 및 네트워킹.
- [Live Activities](live-activities.md): 앱이 잠금 화면과 Dynamic Island에서 추적하는 항목.
- [빌드 및 배포](building.md): 패키지에 필요한 권한.
