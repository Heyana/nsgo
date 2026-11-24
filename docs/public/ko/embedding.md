---
title: 임베딩
description: 기존 iOS, Android, 데스크톱 또는 웹 앱 안에 NSGo 앱을 넣습니다.
group: 툴체인
order: 23
---

## 임베딩 가능한 앱 빌드

임베딩 가능한 앱은 `Root`와 `main`을 가진 일반 앱 패키지이며, 호스트 대상에 맞게 `-embed` 옵션을 주어 빌드합니다. 각 뷰는 앱을 한 번 실행하며 독립된 엔진이므로 호스트는 각기 자체 상태를 가진 뷰를 원하는 만큼 표시할 수 있습니다.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

iOS 빌드는 `<Name>.xcframework`와 `<Name>Resources`를, Android 빌드는 `<Name>.aar`을, 웹 빌드는 `nsgo.json`과 로더가 담긴 디렉터리를 생성합니다.

## iOS

앱이 `base/terminal`을 임포트하는 경우, 빌드 시 앱 프레임워크 옆에 `GhosttyVT.xcframework`도 함께 생성됩니다. 정적 앱 프레임워크와 `-dynamic` 앱 프레임워크 모두에서 이 동반 프레임워크를 **Embed & Sign**으로 호스트 대상에 추가하세요. 여기에는 기기, Intel 및 Apple Silicon 시뮬레이터, Mac Catalyst 라이브러리가 들어 있으며 `GhosttyVT.framework` 이름을 그대로 유지해야 합니다.

프레임워크를 정적 라이브러리로 앱 대상에 추가하고(임베드하지 않음), 리소스 폴더를 폴더 참조(folder reference)로 추가한 뒤 `import <Name>`을 작성하세요. 첫 번째 뷰를 만들기 전, 앱이 시작될 때 `NSGoStart()`를 한 번 호출합니다. 정적 라이브러리의 코드는 호스트 앱의 일부가 되므로 호스트의 개인정보 보호 매니페스트(privacy manifest)에 NSGo가 사용하는 필수 사유 API인 user defaults(`CA92.1`), file timestamps(`C617.1`), system boot time(`35F9.1`)을 선언해야 합니다. `-dynamic` 프레임워크는 자체 `PrivacyInfo.xcprivacy`에 이를 포함합니다.

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView`는 창에 추가되면 앱을 시작하고, 할당 해제되거나 `stop()`을 받으면 중지합니다. 시작 값은 창에 추가되기 전에 설정하고 나머지는 언제든 변경할 수 있습니다. 핸들러, 컴포넌트, 콜백은 메인 스레드에서 실행되며 응답은 나중에 어느 스레드에서든 보낼 수 있습니다. 뷰 컨트롤러는 시스템 막대 설정을 따르도록 뷰의 `preferredStatusBarStyle`, `prefersStatusBarHidden`, `prefersHomeIndicatorAutoHidden`을 전달할 수 있습니다. 또한 `mobile.DeferSystemGestures`를 전달하기 위해 컨트롤러에서 뷰의 `preferredScreenEdgesDeferringSystemGestures`를 반환할 수도 있습니다.

## Android

`base/terminal`을 임포트하는 arm64 앱의 경우, AAR에는 `libnsgo.so` 옆에 `jni/arm64-v8a/libghostty-vt.so`가 포함되므로 호스트는 네이티브 라이브러리 추출(extraction)을 활성화 상태로 유지해야 합니다. Android 사전 빌드 및 터미널 로더에는 [#1452](https://github.com/nsgo-dev/nsgo/issues/1452)가 필요하며 x86_64 터미널 라이브러리는 아직 제공되지 않습니다.

앱 모듈에 AAR을 추가하고 런타임이 네이티브 라이브러리를 찾을 수 있도록 추출 상태로 패키징하세요.

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

코드나 레이아웃에서 `dev.oneo.nsgo.NsgoView`를 생성합니다. 각 뷰는 처음 크기를 얻을 때 자체 프로세스에서 앱을 시작하고, `stop()`은 앱을 종료합니다. 액티비티의 결과를 모든 뷰로 전달하고, 앱이 재시작되지 않도록 액티비티에서 구성 변경(configuration changes)을 직접 처리하세요.

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Go 앱을 살려 둔 채 네이티브 뷰 트리만 분리하려면 호스트에서 `NsgoEngine`을 유지하고 뷰를 명시적으로 연결하세요.

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

분리하면 네이티브 자식 뷰가 해제되고, 다시 연결하면 컴포넌트를 재실행하지 않고 현재 시그널, 입력, 포커스, 스크롤 위치, 드로잉이 복원됩니다. 웹 페이지와 플레이어는 앱 설정에서 다시 생성됩니다. `stop()`은 명시적으로 전달된 엔진만 분리합니다. 각 엔진은 여전히 첫 번째 뷰와 그 Activity를 유지하며 다른 뷰를 거부하므로, 해당 Activity에서 구성 변경을 처리하세요. Activity를 교체하거나 한 프로세스 안에 독립된 여러 뷰를 표시하는 것은 아직 지원되지 않습니다.

Google Play 업데이트 확인을 사용하려면 Android 호스트의 Gradle 의존성에 `com.google.android.play:app-update:2.1.0`을 추가하세요. [런타임 축소 규칙](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro)을 사용해 NSGo가 이름으로 로드하는 AppUpdateManager, AppUpdateInfo, AppUpdateOptions, Tasks API가 유지되도록 하세요. 위의 액티비티 결과 전달 코드는 업데이트 취소 이벤트도 전달합니다.

임베디드 호스트에서 Android 인증 세션을 사용하려면 호스팅 Activity에 비공개 콜백 스킴을 선언하고 `onNewIntent`(및 새로 실행된 `onCreate`)에서 해당 Activity가 수락한 링크를 `view.openLink(uri.toString())`으로 전달하세요. 일치하는 활성 세션이 콜백을 소비하며, 다른 링크는 `OnOpenLinks`로 전달됩니다. 단독 실행 `NsgoActivity`는 이를 자동으로 전달합니다.

## 데스크톱

Go 호스트는 플랫폼의 이벤트 루프를 실행하고 각 뷰의 `Handle`을 자신의 창에 넣습니다. `nsgo.NewView`는 핸들이 창에 들어가면 앱을 시작하므로 그전에 한 설정이 첫 프레임에 반영됩니다.

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()`는 앱에 이벤트를 보내고 앱의 호출을 받으며, `nsgo.Component`는 호스트 플랫폼용 네이티브 컴포넌트를 등록합니다.

## 웹

웹 빌드는 독립된 페이지이지만, 페이지 안의 특정 엘리먼트에 마운트해 그 페이지를 호스트로 삼을 수도 있습니다. `mount(element, url, options)`는 플레이어를 반환하며, `nsgo.json`은 자기 자신을 기준으로 로더, wasm, 리소스 경로를 지정합니다.

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

`host` 옵션이 없으면 로더는 `nsgo.yaml`의 `web.host`가 지정한 모듈을 임포트하고, 그 모듈이 `createHost(player)`를 내보낸 경우 이를 await합니다. 엘리먼트나 프레임 안에 마운트된 앱은 브라우저의 히스토리와 제목을 페이지에 맡깁니다.

`appearance`(`"light"`, `"dark"` 또는 `"system"`), `locale`, `fontScale`은 네이티브 호스트의 설정과 마찬가지로 브라우저 설정 대신 사용되며, 플레이어의 `setAppearance`, `setLocale`, `setFontScale`로 나중에 변경할 수 있습니다. 컴파일된 `WebAssembly.Module`이나 그 프로미스인 `module`을 전달하면 앱을 여러 번 마운트하는 페이지가 Wasm을 한 번만 다운로드하고 컴파일할 수 있습니다.

로더는 앱이 시작되기 전에 `progress.nsgo-load-progress` 엘리먼트를 표시합니다. 이를 CSS로 스타일링하거나, mount 옵션에 `onProgress`를 전달하거나, 앱 자체 페이지의 경우 `web.host`에서 `onProgress`를 내보내세요. 콜백은 기본 진행 막대를 대체하며 디코딩된 바이트 단위의 `{ loaded, total }`을 받고, 전체 크기를 알 수 없을 때는 `total`이 `null`이 됩니다. 로더는 시작이 끝나면 UI를 정리할 수 있도록 `null`을 전달해 콜백을 호출합니다. 컴파일된 `module`을 사용하면 다운로드 진행 단계를 건너뜁니다.

```yaml
web:
  host: host.js
```

## 호스트 채널

`core.Host`는 모든 호스트가 구현하는 단일 계약으로, 시작 매개변수, 호스트가 보내는 이벤트, 앱이 호출하는 호스트 함수로 이루어집니다. 값은 JSON처럼 전달되므로 맵은 `map[string]any`, 숫자는 `float64`가 되며, 동일한 코드가 Swift, Java, Go, JavaScript에서 그대로 동작합니다.

- `HostParams`는 시작 시 한 번 시작 매개변수를 읽습니다.
- `cx.OnHostEvent`는 `Host.Send`와 페이지의 `hostEvent`를 수신합니다.
- `cx.CallHost`는 지정한 핸들러를 호출해 응답으로 해결(resolve)되고, `cx.PostHost`는 응답 없이 호출합니다.
- `Host.HandleOpenURL`은 플랫폼보다 먼저 `cx.OpenURL`을 확인하며, 그 응답에 따라 플랫폼에서도 URL을 열지 결정됩니다.
- `Host.SetParams`, `Send`, `Handle`, `HandleDefault`는 어느 고루틴에서나 안전하게 호출할 수 있습니다.

iOS에서는 호스트가 `params`를 설정하고 `handle`로 핸들러를 등록하며 `send`로 이벤트를 보내고 `onOpenURL`에 응답합니다. Android에서는 `setParams`, `handle`, `send`, `setOpenUrlHandler`로 동일하게 처리합니다. 데스크톱에서는 `core.Host` 메서드를 사용하고, 웹에서는 페이지의 `host` 객체가 응답합니다.

## 네이티브 컴포넌트

`core.Native`는 호스트가 만든 뷰를 표시합니다. `Props`는 호스트 컴포넌트의 `SetProp`로 전달되고, 컴포넌트가 발생시킨 이벤트는 `OnEvent`로 전달되며, 값이 애니메이션 노드인 prop은 엔진 작업 없이 프레임 클록에 맞춰 움직입니다.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

호스트에 컴포넌트를 등록하세요. iOS에서는 `app.register(component:)`와 `NSGoComponent`, Android에서는 `registerComponent`와 `NsgoComponent`, 데스크톱에서는 `nsgo.Component`를 사용합니다. 웹에는 네이티브 컴포넌트가 없으므로 알 수 없는 이름은 빈 상자로 표시됩니다.

## 설정

`tokens`는 양쪽 모양 모두 또는 한쪽 모양에서 역할 이름(`accent`, `inset`, `raised`)별로 앱의 테마를 재정의하며 `cx.Palette()`가 이를 반영합니다. 호스트는 로캘, 텍스트 크기, 모양, 데이터 디렉터리도 설정하며 앱의 오류를 전달받습니다. `nsgo build -embed`는 창, 메뉴, 트레이 아이콘을 만들지 않으며, 이들은 호스트의 몫입니다.

뷰의 엔진 고루틴에서 내부 패닉이 발생하면 해당 엔진이 중지되고 연결된 표면들이 제거되며 호스트의 오류 리스너(Go에서는 `nsgo.OnError`)로 패닉과 스택이 보고됩니다. 다른 엔진은 계속 실행되며, 실패한 앱을 다시 시작하려면 새 뷰를 생성하세요. 일반적인 앱 콜백 패닉은 여전히 가장 가까운 `core.Catch`로 먼저 전달됩니다.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop`, `examples/embed-web`은 각 호스트 종류별로 구현한 동일한 앱 예제입니다.

## 다음 단계

- [플랫폼 서비스](services.md): 앱이 플랫폼에 요청하는 기능.
- [빌드 및 배포](building.md): `-embed`가 생성하는 패키지와 서명.
- [예제: 할 일 앱](tutorials/todo.md): 호스트가 임베딩하는 앱.
