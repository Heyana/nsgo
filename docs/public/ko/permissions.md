---
title: 권한
description: 시스템 권한을 확인 및 요청하고, 해당 설정을 열며, macOS에서 시스템 설정으로 사용자를 안내합니다.
group: 플랫폼 기능
order: 49
---

## 확인 및 요청

시스템은 앱이 수행할 수 있는 일부 기능을 사용자가 직접 허용하도록 보호합니다. 휴대폰과 Mac에서는 마이크, 카메라, 연락처, 캘린더, 미리 알림, 사진, Bluetooth, 미디어 보관함, 위치, 알림이 여기에 해당하고, 휴대폰에서는 동작 및 백그라운드 위치가 포함되며, macOS에서는 손쉬운 사용(Accessibility), 화면 기록(Screen Recording), 입력 모니터링(Input Monitoring), 전체 디스크 접근 권한(Full Disk Access), 앱 관리(App Management), 개발자 도구(Developer Tools), 다른 앱 자동화(Automation)도 포함됩니다([위치 및 동작](services.md#위치와-모션), [알림](notifications.md)). `core.Permissions()`는 이들을 `core.Permission` 키로 나열합니다. `cx.PermissionStatus`는 묻지 않고 앱에 해당 권한이 있는지 응답하며, `cx.RequestPermission`은 시스템 프롬프트가 있고 아직 묻지 않은 경우 시스템 프롬프트로 권한을 요청합니다. 사용자가 해당 권한이 필요한 작업을 수행할 때 요청하세요. Face ID와 Touch ID는 이 목록에 포함되지 않습니다. 이들의 사용 가능 여부는 `cx.BiometricAvailability`로 확인하고, 자체 프롬프트로 인증을 요청할 때는 `cx.AuthenticateBiometric`을 사용합니다([생체 인증](services.md#생체-인증)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

macOS에서 자동화(Automation)는 대상 앱별로 부여됩니다. 실행 중인 대상 앱의 번들 ID로 확인하거나 요청하려면 `core.PermissionAutomationOf("com.apple.finder")`를 전달하고, 시스템 설정의 자동화 패널을 열려면 `core.PermissionAutomation`을 전달하세요. 실행 중이지 않은 대상 앱은 `PermissionUnknown`으로 응답합니다.

| 상태 | `String()` | 의미 |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | 아직 묻지 않음: 요청 시 시스템 프롬프트가 표시됨 |
| `PermissionGranted` | `granted` | 앱에 권한이 있음 |
| `PermissionLimited` | `limited` | 선택한 사진이나 연락처 등 제한된 접근만 허용됨 |
| `PermissionDenied` | `denied` | 거부됨; 시스템 설정에서만 변경할 수 있음 |
| `PermissionUnknown` | `unknown` | 시스템이 보호하고 있지만 시스템이나 플랫폼이 상태를 알려주지 않음 |
| `PermissionNotGated` | `not-gated` | Linux처럼 앱의 접근을 막는 제한이 없음 |
| `PermissionUnavailable` | `unavailable` | 플랫폼에 해당 기능이 없음 |

`Allowed`는 granted, limited, not gated일 때 true입니다. macOS에서는 요청 시 해당 패널의 목록에 앱을 올리기만 하므로 사용자가 직접 켜기 전까지 손쉬운 사용, 화면 기록, 입력 모니터링에 대해 denied로 응답하며, 화면 기록 권한의 허용 여부는 앱이 다시 시작된 후에 반영됩니다.

## 사용 목적 설명

macOS와 iOS에서는 권한을 요청하려면 앱의 `Info.plist`에 해당 권한의 사유가 있어야 하며, 이는 `nsgo.yaml`의 `apple.info`에 작성합니다. 이 항목이 없으면 시스템이 앱을 종료시켜 버리므로 요청은 unavailable로 응답하고 해당 키를 로그에 남깁니다. 앱의 [카탈로그](localization.md)에 `locales/en.yaml`의 `NSCameraUsageDescription: Scans the codes you point it at.`처럼 같은 키로 메시지를 두면 사유가 번역되어 시스템이 사용자의 언어로 묻습니다. 번역되지 않은 언어에서는 원본 언어의 문구가 표시됩니다.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

Android에서는 각 권한을 `nsgo.yaml`의 `android.permissions`에 나열해야 하며, 매니페스트에서 빠진 권한은 `PermissionUnavailable`로 응답합니다. `nsgo dev`로 실행할 때는 macOS가 앱의 동작을 앱을 시작한 터미널에 귀속시키므로 권한 요청과 목록 등록 대상이 터미널이 됩니다. 앱 자체의 권한을 테스트하려면 앱을 빌드하세요.

## 설정 열기

`cx.OpenPermissionSettings`는 권한을 부여하는 설정 페이지를 열고 페이지가 열렸는지 여부를 응답합니다. macOS에서는 개인정보 보호 및 보안의 해당 권한 패널(자동화, 연락처, 사진, 위치 서비스 포함) 또는 알림 패널을 열고, iOS에서는 설정의 앱 페이지(iOS 16부터는 앱의 알림 페이지), Android에서는 설정의 앱 페이지, Windows에서는 마이크·카메라·알림 설정 페이지를 엽니다.

`PermissionLocation`과 `PermissionBackgroundLocation`의 경우 macOS는 위치 서비스를 열고, `PermissionMotion`은 macOS가 동작 샘플을 제공하지 않더라도 동작 및 피트니스를 엽니다. iOS와 Android는 세 경우 모두 앱의 설정 페이지를 엽니다. 설정을 연다고 해서 권한이 요청되거나 부여되지는 않습니다.

## 사용자 안내

`desktop.GuidePermission`은 해당 설정 페이지를 열고 사용자가 권한을 부여할 때까지 기다립니다. macOS에서는 포커스를 빼앗지 않고 시스템 설정 창을 따라다니는 패널을 표시하며, 목록에 앱을 끌어다 놓아야 하는 패널의 경우 사용자가 거기로 끌어다 놓을 수 있도록 앱을 제시합니다. 앱이 권한을 얻거나, 사용자가 패널이나 시스템 설정을 닫거나, `stop`이 호출되거나, 컴포넌트가 사라지면 안내가 종료되고 콜백 함수가 최종 상태를 전달받습니다. 새 안내를 시작하면 이전 안내는 종료됩니다.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

패널이 제시하는 대상은 앱의 번들이며 번들이 없으면 실행 파일입니다. `GuideOptions.App`으로 다른 앱을 지정하고 `Name`으로 패널에 표시될 이름을 지정할 수 있습니다. Windows는 해당 페이지를 열고 설정 창이 닫힐 때까지 기다립니다. 그 외의 플랫폼에서는 콜백 함수가 즉시 상태를 전달받습니다.

## 플랫폼

| 플랫폼 | 확인 및 요청 가능한 항목 |
| --- | --- |
| macOS | 모든 권한(`PermissionAutomationOf`를 통한 실행 중인 대상 앱별 자동화 및 번들 앱의 알림 포함); 대상이 없는 `PermissionAutomation`, 앱 관리, 개발자 도구, 미디어 및 Apple Music은 unknown이며 동작은 unavailable |
| iOS | 마이크, 카메라, 연락처, 캘린더, 미리 알림, 사진(`PermissionLimited` 포함), Bluetooth, 미디어 보관함, 위치, 동작, 알림 |
| Windows | 마이크, 카메라, 알림(시스템 프롬프트는 표시하지 않음); 나머지는 제한 없음(`not-gated`) |
| Linux | 샌드박스 외부에서는 제한 없음(`not-gated`); 알림은 알림 서버의 응답 여부를 반영; Flatpak과 Snap에서는 데스크톱 포털을 통해 카메라를, 포털과 PipeWire를 통해 마이크를 처리 |
| 웹 | 마이크와 카메라(Firefox에서는 unknown), 알림, 위치(백그라운드 위치 제외), 모바일 브라우저에서의 동작 |
| Android | `android.permissions`에 선언된 경우 마이크, 카메라, 캘린더, Bluetooth, 미디어 보관함 |
| Apple Watch, 미니 프로그램 | 각 서비스를 통한 알림, 위치, 동작; 시스템이 보호하는 나머지 항목은 현재 unknown |

위치 및 동작 권한은 각 [플랫폼의 센서](services.md#위치와-모션)를 따르며, `PermissionNotifications`는 [알림](notifications.md)을 따릅니다. 피트니스는 Apple 기압 데이터를 포함해 `PermissionActivityRecognition`(Android에서는 `ACTIVITY_RECOGNITION`, Apple에서는 `NSMotionUsageDescription`)을 사용하며, Android의 원시 동작 데이터는 이와 별개로 유지됩니다. 모바일 Safari에서는 누르기 핸들러 안에서 `PermissionMotion`을 요청하세요.

## 테스트

`nsgotest`는 자체 모델의 상태를 바탕으로 응답합니다. `app.Permissions`는 각 권한의 상태를 보관하며 처음에는 요청 전(`PermissionNotAsked`) 상태이고, 아직 묻지 않은 권한을 요청하면 `app.PermissionAnswers`에 해당 값이 있을 경우 그 값으로, 없으면 허용됨(`PermissionGranted`)으로 바뀝니다. `PermissionNotifications`는 `app.NotificationPermission` 및 `app.NotificationAnswer`와 동기화됩니다. `app.PermissionSettings`는 열린 설정 목록을 기록하고 `app.Guide`는 현재 표시 중인 안내를 나타내며, `app.GrantPermission`과 `app.CloseGuide`가 이를 종료합니다.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## 다음 단계

- [알림](notifications.md): 알림 자체 권한, 배지 및 알림 누름.
- [빌드 및 배포](building.md): 패키지가 선언하는 인타이틀먼트와 기능(capability).
