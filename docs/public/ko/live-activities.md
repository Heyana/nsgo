---
title: 라이브 액티비티
description: 잠금 화면과 Dynamic Island, 그리고 Android의 라이브 업데이트로 진행 상황을 실시간으로 추적합니다.
group: 플랫폼 기능
order: 48
---

## 활성화하기

라이브 액티비티(Live Activity)는 배달이나 타이머처럼 실시간으로 진행되는 작업을 추적합니다. iOS 16.1 이상의 iPhone에서는 잠금 화면과 Dynamic Island에 표시되며, Android 16에서는 알림창 상단과 상태 표시줄 칩의 라이브 업데이트(Live Update)로 표시되고, 이전 Android 버전에서는 진행 중 알림으로 표시됩니다. `nsgo.yaml`에서 활성화하세요:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## 시작, 업데이트 및 종료

`widget.StartActivity`는 추적 대상을 지정하고 위젯의 종류(kind)와 같은 방식으로 작성되는 특정 종류의 액티비티를 시작하여 `widget.ActivityContent`를 표시합니다. `Activity.Update`는 표시 내용을 교체하고, `Activity.Alert`도 내용을 교체하면서 사용자에게 알림을 보내며, `Activity.End`는 nil이 아닌 경우 최종 콘텐츠를 표시한 뒤 액티비티를 종료하고 `Dismissal` 설정에 따라 잠금 화면에서 제거합니다. `widget.Activities`는 앱의 이전 실행에서 시작된 것을 포함해 현재 실행 중인 액티비티 목록을 반환하며, `widget.ActivitiesEnabled`는 사용자와 플랫폼이 이를 허용하는지 여부를 알려 줍니다.

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

`StartActivity`는 해당 기능 없이 빌드된 앱에서는 `widget.ErrNoActivities`를, iOS 16.1 및 Android 외의 환경에서는 `widget.ErrActivitiesUnsupported`를, 사용자가 비활성화한 경우에는 `widget.ErrActivitiesDisabled`를 반환합니다. 액티비티가 종료된 후에는 `Update`와 `End`가 `widget.ErrActivityEnded`를 반환합니다. 액티비티의 콘텐츠 크기는 ActivityKit 및 APNs 제한에 따라 최대 4 KB입니다.

## 표시되는 내용

두 플랫폼 모두 제목과 본문, 상태 또는 시스템이 초를 세는 타이머, 진행률 표시줄, 최대 세 개의 동작, 강조 색상을 표시합니다. iOS는 위젯 익스텐션으로 이를 그리며, 잠금 화면과 Dynamic Island의 각 영역을 위해 그 대신 `widget`의 뷰를 받을 수 있습니다. 여기에는 `LockScreen`, 확장된 상태의 `Leading`, `Trailing`, `Center`, `Bottom`, 축소된 상태의 `CompactLeading`과 `CompactTrailing`, 여러 액티비티가 공간을 공유할 때의 `Minimal`이 있습니다. Android는 라이브 업데이트가 앱의 뷰를 받지 않으므로 앱 아이콘과 함께 해당 필드만 표시합니다.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL`은 액티비티를 탭했을 때 앱과 함께 열리며, `Stale`은 iOS가 콘텐츠를 만료된 것으로 표시하는 시각이고, `Background`는 잠금 화면에서 액티비티의 배경을 채웁니다.

## 누르기 동작

`widget.OnAction`은 액션이나 `widget.Button`을 눌렀을 때 액티비티와 액션 ID를 전달하여 핸들러를 실행합니다([위젯 버튼](widgets.md#버튼)의 경우 `Activity`는 nil입니다). 이는 `main`에서 `nsgo.Run` 이전에 설정하세요. iOS에서는 누르기가 발생하면 시스템이 뷰 없이 백그라운드에서 앱을 시작하고 핸들러가 반환될 때까지 액티비티를 유지하므로 핸들러 안에서 액티비티를 업데이트할 수 있습니다. Android에서는 누르기가 Activity를 열지 않고 런타임 리시버를 통해 실행 중인 Go 프로세스로 전달됩니다. 액션이 동작하려면 이를 게시한 엔진이 계속 실행 중이어야 하며, 해당 엔진이 종료된 뒤의 오래된 액션은 아무 동작도 하지 않습니다. 알림 자체를 탭하면 여전히 앱이 열립니다. 핸들러는 앱 뷰의 고루틴이 아닌 시스템 고루틴에서 실행되므로, 뷰에는 `core.Scope`의 `Post`를 통해 접근해야 합니다.

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

iOS 17 이상에서는 액티비티의 뷰에서도 동일하게 동작합니다. `widget.Button{Action: "pause"}`는 앱을 열지 않고 같은 핸들러를 실행하며, 누르기가 앱을 열기만 하는 환경에서는 자식 요소만 표시합니다.

## 푸시 업데이트

백그라운드에 있는 앱은 실행 중인 동안에만 액티비티를 업데이트할 수 있으므로, 외부에서 일어나는 상황을 추적하는 앱은 `widget.ActivityOptions{Push: true}`로 액티비티를 시작하고 `widget.OnPushToken`이 수신한 토큰을 서버로 보낸 뒤, 서버가 `widget.ActivityState`가 해당 콘텐츠에 대해 생성한 값을 `content-state`로 담아 APNs `liveactivity` 업데이트를 푸시하도록 합니다. 푸시에는 이미지를 포함할 수 없습니다. iOS 17.2부터는 푸시로 액티비티를 시작할 수 있는 토큰도 받을 수 있으며, 이때 어트리뷰트 타입 `widget.ActivityAttributesType`과 종류 및 ID에 해당하는 어트리뷰트를 사용합니다. 푸시를 사용하려면 앱 프로파일에 푸시 알림 entitlement가 필요합니다.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## 제한 사항

시스템은 8시간이 지나면 액티비티를 종료합니다. `widget.DismissDefault`는 종료된 액티비티를 iOS의 경우 최대 4시간 동안 잠금 화면에 남겨 두고 Android에서는 즉시 제거하며, `widget.DismissNow`는 즉시 제거하고, `widget.DismissAt`은 4시간 이내의 지정된 시각에 제거합니다. Android에서는 `POST_NOTIFICATIONS` 및 `POST_PROMOTED_NOTIFICATIONS` 권한이 필요하므로, 먼저 `cx.RequestNotificationPermission`으로 사용자에게 권한을 요청하세요([알림](notifications.md)). 갤러리의 위젯 데모와 `examples/widgets`에서 실행 예제를 확인할 수 있습니다.

## 다음 단계

- [위젯](widgets.md): 같은 익스텐션이 그리는 홈 화면과 잠금 화면의 타임라인.
- [알림](notifications.md): Android에서 먼저 요청해야 하는 권한.
- [빌드 및 배포](building.md): 익스텐션과 필요한 entitlement 설정.
