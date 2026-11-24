---
title: 위젯
description: iPhone과 iPad의 홈 화면 및 잠금 화면, 그리고 Mac 데스크톱에 앱의 뷰를 표시합니다.
group: 플랫폼 기능
order: 47
---

## 위젯 종류

위젯은 앱의 실행 여부와 상관없이 iPhone과 iPad의 홈 화면과 잠금 화면, Mac의 데스크톱과 알림 센터, 그리고 Android 런처에 앱의 요약 정보를 표시합니다. 종류(kind)는 시스템의 위젯 갤러리가 제공하는 하나의 위젯과 그 위젯이 지원하는 크기 패밀리(family)를 뜻합니다:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

Apple 플랫폼에서는 위젯 익스텐션이 이를 그리며, `build`와 `dev`가 Xcode의 Swift 컴파일러로 이 익스텐션을 컴파일합니다([빌드 및 배포](building.md#위젯과-실시간-현황)). Android에서는 사전 빌드된 Java 런타임이 동일한 타임라인을 App Widget으로 그립니다. 두 플랫폼 모두 앱이 종료된 뒤에도 게시된 콘텐츠를 계속 표시합니다.

샌드박스된 Mac App Store 빌드의 경우 `apple.app_group`을 설정하고 `sign.macos_profile`과 `sign.widgets_profile`을 통해 macOS 프로비저닝 프로필을 제공하세요. 앱과 `<id>.widgets` 모두 동일한 팀에 속해야 하며 해당 그룹의 entitlement를 가져야 합니다. 타임라인과 이미지는 이 공유 컨테이너에 저장되며, 직접 배포(direct) 빌드와 개발용 macOS 빌드는 기존의 홈 디렉터리 위치를 그대로 사용합니다. 빌드 옵션은 [스토어 서명](building.md#스토어-패키지)에서 설명합니다.

`name`과 `description`에는 앱 카탈로그의 메시지 키를 지정할 수 있습니다. 키를 지정하면 위젯 갤러리와 플레이스홀더는 사용자의 언어를 사용하며, 번역이 없는 경우 원본 카탈로그의 텍스트로 폴백합니다. 카탈로그 키가 아닌 값은 입력된 그대로 표시됩니다.

## 타임라인

`widget.Update`는 특정 종류의 위젯이 앞으로 표시할 내용을 엔트리(entry)의 타임라인으로 기록합니다. 시스템은 앱이 실행되지 않아도 시각이 가장 최근에 지난 엔트리를 먼저 보여 주고, 이후 각 엔트리를 해당 시각에 맞춰 표시합니다. 제로 값 `Date`는 현재 시각을 의미합니다. 엔트리가 없으면 위젯은 해당 종류의 이름과 설명을 표시합니다. Apple 플랫폼에서는 포그라운드에 있는 앱이 원하는 만큼 자주 위젯을 업데이트할 수 있으며, 백그라운드에 있는 앱은 하루에 수십 번 정도 업데이트할 수 있습니다.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families`는 `Entry.View` 대신 특정 패밀리 전용 뷰를 표시하고, `Entry.Background`는 시스템이 유지하는 여백 바깥까지 위젯의 배경 전체를 채우며, `Entry.URL`은 위젯을 탭했을 때 앱과 함께 열립니다. `widget.Update`는 위젯 없이 빌드된 앱에서는 `widget.ErrNoWidgets`를, macOS, iOS, Android 외의 플랫폼에서는 `widget.ErrUnsupported`를 반환하며, 기록하기 전에 `widget.Available`로 지원 여부를 확인할 수 있습니다. 이 함수는 어느 고루틴에서든, 심지어 `nsgo.Run` 이전에도 실행할 수 있습니다.

## 뷰

위젯의 뷰는 `core`의 엘리먼트가 아닙니다. 이들은 SwiftUI와 같은 방식으로 레이아웃되며, `Style`로 크기를 지정하거나 채우도록 설정하지 않으면 뷰는 자신의 콘텐츠 크기만큼만 차지하고, 컨테이너는 `AlignItems`와 `Justify`로 별도 지정하지 않는 한 내부 요소를 가운데에 배치합니다. `Text`, `Symbol`, 그리고 바이트 데이터나 `Update`가 읽는 파일 기반의 `Image`는 콘텐츠를 표시하고, `Column`, `Row`, `Stack`, `Spacer`, `Divider`는 콘텐츠를 배치하며, `Rectangle`, `Progress`, `Gauge`는 도형과 진행 상태를 그립니다. `Date`는 시스템 형식으로 시각을 표시하며, 이 중 `Relative`, `Offset`, `Timer`는 매초마다 새 엔트리를 만들지 않고도 스스로 시간을 계속 계산해 표시합니다.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style`에는 간격(gap), 안쪽 여백(padding), 크기 또는 채우기(fill), 정렬, 둥근 모서리가 있는 배경, 시스템 텍스트 스타일 또는 글꼴 크기·굵기·디자인·변형, 줄 수 제한, 앱과 함께 열리는 `Link`, 그리고 시스템이 색조(tint)를 입히는 뷰에 포함시키는 `Accented`를 지정할 수 있습니다. `widget.Primary`, `Secondary`, `Accent`는 화면 모드(appearance)를 따르며, `RGB`와 `RGBA`는 `Dark`로 다크 모드용 색상을 따로 지정할 수 있습니다.

## 버튼

`widget.Button`은 iOS 17 및 macOS 14 이상에서 앱을 열지 않고 `widget.OnAction`을 실행합니다. 이전 버전의 시스템에서는 자식 요소만 표시합니다. 시스템이 뷰 없이 백그라운드에서 앱을 시작할 수 있으므로, 핸들러는 `main`에서 `nsgo.Run` 이전에 등록하세요. 위젯을 눌렀을 때는 `Activity == nil`이며 `Action`에 버튼의 값이 전달됩니다.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

시스템의 자동 타임라인 리로드가 결과를 반영할 수 있도록 액션을 처리하고 핸들러가 반환되기 전에 `widget.Update`를 호출하세요. 핸들러는 뷰의 고루틴 바깥에서 실행되므로 뷰를 변경하려면 `core.Scope.Post`를 사용해야 합니다. 이는 [라이브 액티비티](live-activities.md)와 동일한 핸들러와 인텐트를 사용하며, `nsgo.yaml`에 `live_activities` 없이 `widgets`만 있으면 됩니다.

## Android

런처의 위젯 선택기에서 위젯을 추가하고 크기를 조절할 수 있습니다. 렌더러는 위젯의 경계 크기에 따라 허용된 패밀리를 선택합니다. 가로와 세로 모두 240포인트 미만이면 small, 가로 240포인트 이상이면 medium, 세로 240포인트 이상이면 large, 가로 500포인트 이상이면 extra-large를 선택하며, 해당 패밀리가 목록에 없으면 목록의 첫 번째 패밀리를 사용합니다. `Entry.URL`과 `Style.Link`는 `url_schemes`에 선언된 스킴을 통해 앱을 엽니다. 중첩된 뷰에 지정된 링크가 컨테이너의 링크보다 우선합니다.

미래의 타임라인 엔트리는 Android의 부정확한 알람(inexact alarm)을 사용하므로 앱이 백그라운드에 있을 때 시스템에 의해 지연될 수 있습니다. 타이머 뷰는 네이티브 크로노미터(chronometer)를 사용해 앱 없이도 초를 세며, 현재 크기와 색상은 반영되지만 커스텀 글꼴 굵기, 디자인, 기울임꼴 스타일은 아직 향후 지원 목표입니다. 상대 날짜는 시스템 일정에 따라 새로고침됩니다. 범용 렌더러는 텍스트, 이미지, 레이아웃, 도형, 막대, 링 및 자주 쓰이는 심볼 글리프 모음을 지원하므로, 두 플랫폼에서 정확히 일치해야 하는 아트워크에는 `Image`를 사용하세요. 임의의 SF Symbol 이름 지원은 Android에서 아직 과제로 남아 있습니다. Android 위젯 프로바이더는 독립 실행형 APK와 AAB에 대해서만 생성되며, 임베디드 호스트에는 생성되지 않습니다.

## 라이브 액티비티

동일한 익스텐션이 잠금 화면과 Dynamic Island에 앱의 라이브 액티비티도 함께 그립니다([라이브 액티비티](live-activities.md)).

## 다음 단계

- [라이브 액티비티](live-activities.md): 진행 중인 상황을 실시간으로 추적하는 기능.
- [알림](notifications.md): 위젯과 함께 사용하는 알림 및 배지.
- [빌드 및 배포](building.md): 익스텐션이 공유하는 앱 그룹 설정.
