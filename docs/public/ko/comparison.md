---
title: 다른 프레임워크와의 비교
description: NSH, MyGo 및 다른 UI 프레임워크와 비교한 NSGo의 위치와 트레이드오프.
order: 2
---

UI 프레임워크 가운데 NSGo가 자리한 위치는 세 가지 선택에서 비롯됩니다. 앱은 Go(또는 그 위의 TSX)로 작성하고, 각 플랫폼은 자체 네이티브 뷰로 그리며, 하나의 코드베이스로 휴대폰, 데스크톱, 웹을 모두 다룹니다. 다른 프레임워크는 다른 선택을 하며, 모든 선택에는 그에 따른 대가가 따릅니다.

## 한눈에 보기

| 프레임워크 | 언어 | 렌더링 방식 | 대상 플랫폼 |
| --- | --- | --- | --- |
| NSGo | Go, TSX | 각 플랫폼의 네이티브 뷰 | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, 웹; 미니 프로그램(실험적) |
| [NSH](https://nsh.dev) | Rust, TSX | 각 플랫폼의 네이티브 뷰 | iOS, Android, macOS, Windows, Linux, 웹, GPUI 앱, 터미널; 커뮤니티 플랫폼을 통한 HarmonyOS 및 위챗 미니 프로그램 |
| [MyGo](https://github.com/egoist/mygo) | Go, 웹 페이지 | 자체 GPU 렌더러 또는 시스템 웹뷰 | macOS, Windows, Linux, 웹 |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | 자체 렌더러 | 데스크톱, 휴대폰, 웹 |
| [Wails](https://wails.io) | Go, 웹 페이지 | 시스템 웹뷰 | 데스크톱 |
| [Flutter](https://flutter.dev) | Dart | 자체 렌더러 | 휴대폰, 데스크톱, 웹 |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | 자체 렌더러; Android의 Compose | 휴대폰, 데스크톱, 웹 |
| [React Native](https://reactnative.dev) | JavaScript, TSX | 각 플랫폼의 네이티브 뷰 | iOS, Android; 커뮤니티 플랫폼을 통한 기타 플랫폼 |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | 각 플랫폼의 컨트롤 | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | 자체 렌더러 | 데스크톱, 휴대폰, 웹 |
| [Qt](https://www.qt.io) | C++, QML | 자체 렌더러 | 데스크톱, 휴대폰, 임베디드 기기 |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | 웹 페이지; Tauri는 Rust | 번들된 Chromium; Tauri는 시스템 웹뷰 | 데스크톱; Tauri는 휴대폰 포함 |

## 강점

- **네이티브 뷰.** 텍스트, 입력기(IME), 선택, 스크롤, 접근성, 메뉴가 모두 플랫폼 자체의 것이므로 앱이 해당 플랫폼에서 사용자가 기대하는 방식 그대로 동작합니다. 직접 그리는 프레임워크는 이 모든 것을 각각 다시 구현해야 합니다. iOS의 Liquid Glass처럼 한 플랫폼에만 있는 기능도 버리는 대신 다른 플랫폼이 같은 방식으로 모방합니다.
- **세밀한 업데이트.** 컴포넌트는 한 번만 실행되고 시그널이 이를 읽는 부분만 갱신하므로, 가상 DOM이나 매 프레임마다 비교할 뷰 함수가 필요 없습니다. 애니메이션은 플랫폼의 프레임 클록에서 실행됩니다.
- **가장 작은 패키지와 가장 빠른 시작 속도.** [벤치마크](https://nsgo.dev/benchmarks/)에서 동일한 카운터 앱을 패키징하면 macOS, Windows, Linux에서 약 8 MB로 MyGo, GPUI, NSH(NSH는 33~45 MB)보다 작습니다. 시작 속도 역시 가장 빠르며, MyGo나 GPUI처럼 GPU에 직접 그리는 프레임워크보다 메모리를 적게 사용합니다.
- **Go 단일 언어.** 엔진, 레이아웃, 백엔드가 모두 Go로 작성되어 있으며, Go 앱에는 C, cgo, JavaScript 엔진이 들어가지 않습니다.
- **Mac 한 대로 모든 플랫폼 빌드.** Go와 Xcode가 설치된 Mac 한 대면 NDK, Android SDK, JDK, GTK, Windows 툴체인 없이도 모든 대상을 크로스 컴파일할 수 있습니다.
- **명령 한 번으로 모든 배포 파일 생성.** `nsgo build`는 서명 및 공증된 macOS 디스크 이미지, 서명된 Windows 설치 프로그램, deb 및 Arch 패키지, AppImage, 그리고 Mac App Store, Microsoft Store, Google Play, Flathub용 패키지와 GitHub 릴리스에서 설치하는 스크립트까지 생성합니다([빌드 및 배포](building.md)).
- **각 플랫폼 고유의 업데이트 방식.** macOS에서는 Sparkle이, Windows에서는 WinSparkle이 다음 버전을 다운로드, 검증, 설치합니다. Linux와 Android에서는 앱이 새 버전을 찾아 릴리스 페이지를 열고, iOS에서는 App Store가 처리합니다. 스토어를 통해 설치된 앱은 해당 스토어가 업데이트를 담당합니다([업데이트](building.md#updates)).
- **모든 플랫폼 지원.** 휴대폰, 태블릿, TV, 워치, 데스크톱, 웹이 하나의 엔진과 하나의 API를 공유하며, iOS App Clips, iPhone 및 Mac 위젯, 그리고 실험적으로 위챗, QQ, 알리페이, 도우인 미니 프로그램도 지원합니다.
- **임베딩.** 기존 iOS, Android, Go 데스크톱 또는 웹 앱 안에 NSGo 앱을 하나의 뷰로 배치해 화면 단위로 점진적인 전환이 가능합니다([임베딩](embedding.md)).
- **빠른 개발 루프.** Go 파일을 저장하면 앱이 다시 빌드되어 재시작되며, TSX를 변경하면 실행 중인 앱이 상태를 유지한 채 제자리에서 업데이트됩니다. 빌드 오류와 렌더링 중 발생한 패닉은 앱 위에 바로 표시됩니다([실행 및 디버깅](debugging.md)).
- **어디서나 동일한 테스트.** 테스트는 실제 사용자처럼 텍스트나 역할로 컨트롤을 찾아 조작합니다. 창 없이 빠르게 실행하거나, 각 플랫폼의 실제 창에서 변경 없이 그대로 플랫폼 자체의 입력으로 실행하며 스크린샷을 저장할 수 있습니다([테스트](testing.md)).
- **내장된 지역화.** 앱과 컴포넌트의 문구는 카탈로그에 저장되며, 의사 로캘(pseudo-locale)을 통해 번역되지 않았거나 잘리거나 오른쪽에서 왼쪽으로 쓰기(RTL)에 대비되지 않은 부분을 찾을 수 있습니다([지역화](localization.md)).
- **Go로 작성하는 셰이더.** `core.Shader`는 Go 문법 또는 WGSL로 작성한 GPU 셰이더를 모든 플랫폼에서 실행합니다([캔버스](canvas.md#shaders)).
- **원할 때는 TSX 사용.** Go 컴포넌트와 동일한 엔진 위에서 Tailwind 클래스를 적용한 TSX로 화면을 작성할 수 있습니다.

## 약점

- **초기 단계.** 0.1.0이 첫 릴리스이며 API가 아직 안정화되지 않았습니다. 런타임에 전달되는 TSX의 힙 제한 등 부족한 기능은 [로드맵](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md)에 정리되어 있습니다.
- **작은 생태계.** Flutter, React Native, Qt에 비해 서드파티 컴포넌트, 예제, 온라인 자료가 적습니다.
- **플랫폼 간 차이.** 네이티브 뷰를 사용하므로 플랫폼 간에 작은 차이가 있어 각 플랫폼에서 확인해야 합니다. 직접 그리는 프레임워크는 어디서나 똑같이 보입니다.
- **자체 브리지 구조.** Android에서는 Go 프로그램이 Java 런타임 옆에서 실행되며 파이프로 통신하고, Linux에서는 사용자가 GTK 4.14 이상을 설치해야 합니다.
- **인터프리터 기반 TSX.** TSX는 Go로 작성된 JavaScript 인터프리터인 goja에서 실행되므로 React Native나 Electron이 사용하는 엔진보다 느립니다.

## NSH

NSGo는 NSH의 설계를 이식한 프로젝트입니다. 아키텍처, 백엔드, API 이름, 그리고 `base`의 스타일 없는 컴포넌트들은 NSH에서 비롯되었습니다([상태](index.md#status)).

- **NSGo를 선택해야 하는 경우:** Rust나 TSX 대신 Go로 앱을 작성하거나, Go와 Xcode가 있는 Mac 한 대로 모든 대상을 빌드하거나, NSGo가 추가로 제공하는 기능(Go로 작성하는 GPU 셰이더, 벡터 아이콘, 터미널, 앱 자체 창·패널·트레이 아이콘 열기, `ui`의 스타일 적용 컴포넌트)이 필요할 때.
- **NSH를 선택해야 하는 경우:** 더 성숙한 프로젝트, 바이트코드와 힙 제한이 있는 QuickJS 기반 TSX 실행, GPUI 백엔드, 또는 32비트 Android 지원이 필요할 때.

## MyGo

두 프레임워크 모두 cgo 없이 Go로 UI를 작성하지만, MyGo는 네이티브 UI를 GPU에 직접 그리고 매 프레임 뷰 함수에서 다시 그리는 반면, NSGo는 변경된 부분만 네이티브 뷰에 전달합니다.

- **MyGo를 선택해야 하는 경우:** 모든 데스크톱에서 동일하게 보이는 데스크톱 앱, Go로의 타입 지정 호출이 가능한 웹 페이지 창, 그리고 트레이 아이콘, 파일 연결, 설치 프로그램, 공증, 서명된 델타 업데이트 같은 데스크톱 API 및 배포 기능이 필요할 때.
- **NSGo를 선택해야 하는 경우:** 휴대폰까지 지원하거나, 각 플랫폼 고유의 텍스트·입력·접근성을 사용하거나, 앱 데이터에서 매번 다시 그리는 대신 시그널로 상태를 관리하려 할 때.

## 기타 Go 툴킷

- **Fyne과 Gio**는 MyGo처럼 모든 것을 직접 그리며 둘 다 휴대폰과 웹을 지원합니다. Fyne은 고유한 룩을 갖춘 전체 위젯 세트를 제공하고, Gio는 즉각 모드(immediate mode)로 저수준이며 유연합니다. 둘 다 정식 버전과 사용자층을 보유하고 있으나 네이티브처럼 보이거나 동작하지는 않습니다.
- **Wails**는 시스템 웹뷰 안에 웹 프론트엔드를 넣고 뒤에서 Go를 실행합니다. 모든 웹 라이브러리를 사용할 수 있지만 인터페이스가 웹 페이지이며 데스크톱 전용입니다.

## 다른 언어의 프레임워크

- **Flutter와 Compose Multiplatform**은 모든 픽셀을 직접 그립니다. 모든 플랫폼에서 완전히 동일하게 보이고 성숙하며 거대한 생태계를 갖추고 있지만, 텍스트·스크롤·입력이 네이티브가 아니라 재구현된 것이고 Dart나 Kotlin을 배워야 합니다.
- **React Native**는 반응형 코어가 네이티브 뷰를 구동한다는 점에서 NSGo와 모델을 공유하며, 이 계열 중 가장 큰 생태계를 가지고 있습니다. iOS와 Android 중심이며 앱 로직을 JavaScript로 실행하는 반면, NSGo는 Go로 실행하고 데스크톱과 웹을 일급 대상으로 다룹니다.
- **.NET MAUI**는 C#으로 각 플랫폼의 컨트롤을 감싸지만 Linux와 웹을 지원하지 않습니다. **Avalonia**는 WPF와 비슷하게 자체 렌더링하며 모든 데스크톱, 휴대폰, 웹을 지원합니다.
- **Qt**는 성숙하고 빠르며 임베디드 기기까지 지원하지만, 자체 위젯을 그리고 C++ 또는 QML이 필요하며 LGPL 또는 상용 라이선스를 따릅니다.
- **Electron과 Tauri**는 웹 페이지를 보여줍니다. 웹의 도구와 라이브러리를 온전히 활용할 수 있는 대신 브라우저만큼의 메모리(특히 Electron)가 필요하고 인터페이스가 웹 페이지처럼 느껴집니다.

## 선택 가이드

초기 단계 프로젝트를 감수할 수 있고 하나의 Go 코드베이스로 휴대폰, 데스크톱, 웹에서 네이티브처럼 느껴지는 앱을 만들고 싶다면 NSGo를 선택하세요. 안정적인 API 위에서 곧 출시해야 하는 앱이라면 위 표에서 성숙한 프레임워크를 고르세요. 휴대폰용으로는 Flutter나 React Native, 데스크톱 전용으로는 MyGo, Wails, Tauri가 적합합니다.
