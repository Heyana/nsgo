---
title: 로컬라이제이션
description: 앱의 메시지를 번역하고 현재 표시 중인 언어를 따릅니다.
group: 핵심 개념
order: 62
---

## 메시지

`nsgo.yaml` 옆의 `locales/` 디렉터리는 앱의 메시지를 보관하며, 언어별로 ICU MessageFormat 형식의 YAML 파일(`en.yaml`, `de.yaml`)을 둡니다:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

메시지는 문자열이거나, 번역가를 위한 `description`과 `placeholders`를 포함한 `message` 객체입니다.

패키지는 `locales/`를 임포트합니다. `dev`, `build`, `test` 명령은 카탈로그를 검사하고 그 옆에 메시지마다 Go 함수(예: `locales.Greeting(cx, "Ada")`)를 생성합니다. 이 함수는 앱이 표시하는 언어로 문구를 반환하며 언어가 바뀌면 함께 갱신됩니다. `go tool nsgo i18n`은 함수 생성만 단독으로 실행합니다. 메시지는 숫자와 날짜를 서식화할 수 있으며(`{d, date, medium}`), `appName`은 패키징 시 각 언어에 맞는 앱 이름을 지정합니다. 마찬가지로 `NSCameraUsageDescription` 같은 `apple.info` 키를 가진 메시지는 해당 Info.plist 항목을 번역합니다([권한](permissions.md#usage-descriptions)).

## 언어

`cx.SetLocale("de")`는 언어를 선택하며, 레이아웃은 해당 언어가 쓰이는 방향을 따릅니다. 독립 실행형 앱에서는 텍스트 선택 메뉴, 파일 대화상자, GTK 위젯처럼 시스템 자체가 표시하는 문구도 지정된 언어를 따릅니다(Android 13+, Windows, Linux에서는 즉시 적용되며 macOS 및 iOS에서는 다음 실행부터 적용됩니다). `cx.SetLocale("")`를 호출하면 다시 사용자의 시스템 언어를 따릅니다.

```sh
go tool nsgo dev -lang en-XA
```

위 명령은 번역되지 않았거나 잘린 텍스트를 확인할 수 있는 의사(pseudo) 로케일로 앱을 실행합니다. 모든 빌드에서 시작 시 `--nsgo-lang=en-XA` 플래그로도 전달할 수 있습니다.

## 요일, 시간 및 단위

`i18n.Regional(cx)`는 언어와는 별개로 사용자가 주, 시간, 단위를 읽는 방식을 알려 줍니다. 즉, 플랫폼에 설정되어 있거나 사용자의 지역 설정에 따른 한 주의 시작 요일, 12시간제 또는 24시간제, 측정 단위계, 온도 단위를 반환합니다. 앱에서 `FirstDayOfWeek`나 `HourCycle`을 별도로 지정하지 않는 한 달력과 날짜 선택기는 해당 요일부터 주를 시작하고 시간 필드는 해당 시간 형식을 표시하며, 카탈로그의 메시지가 출력하는 시각도 사용자의 시간 형식을 따릅니다.

## NSGo 기본 문구

NSGo의 컴포넌트는 NSGo가 번역된 언어로 기본 문구를 표시합니다. 지원되지 않는 다른 언어의 경우, `github.com/nsgo-dev/nsgo/base/locales`처럼 패키지 이름을 키로 지정하면 해당 파일의 언어로 그 패키지의 메시지를 제공할 수 있습니다. 플랫폼의 메뉴와, 탭이나 검색 상자처럼 시스템 자체 문구가 없는 곳에서 Android 및 Windows 스크린 리더가 읽어 주는 역할 및 상태는 `github.com/nsgo-dev/nsgo/desktop/locales`의 문구를 사용합니다.
