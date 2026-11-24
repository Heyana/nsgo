---
title: 嵌入
description: 將 NSGo 應用程式嵌入你自己的 iOS、Android、桌面或網頁應用程式中。
group: 工具鏈
order: 23
---

## 建置可嵌入的應用程式

可嵌入的應用程式是一個帶有 `Root` 與 `main` 的一般應用程式套件，只需加上 `-embed` 針對宿主目標平台建置即可。每個檢視會執行一次該應用程式並各自擁有獨立的引擎，因此宿主可以依需求顯示任意數量的檢視，每個檢視都保有自己的狀態。

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

iOS 建置會產出 `<Name>.xcframework` 與 `<Name>Resources`；Android 建置會產出 `<Name>.aar`；網頁建置則會產出包含 `nsgo.json` 與載入器的目錄。

## iOS

當應用程式匯入 `base/terminal` 時，建置也會在應用程式 framework 旁產出 `GhosttyVT.xcframework`。無論使用靜態或 `-dynamic` 應用程式 framework，都請將此相依項目以 **Embed & Sign** 加入宿主目標。它包含實機、Intel 與 Apple Silicon 模擬器以及 Mac Catalyst 函式庫；請保留其 `GhosttyVT.framework` 名稱。

將該 framework 以靜態函式庫形式加入應用程式目標（請勿嵌入），並將資源資料夾以 folder reference 形式加入，接著 `import <Name>`。在應用程式啟動時、建立第一個檢視之前呼叫一次 `NSGoStart()`。由於靜態函式庫的程式碼會併入宿主應用程式中，宿主的隱私權清單（privacy manifest）須宣告 NSGo 所使用的必要理由 API：使用者預設設定（`CA92.1`）、檔案時間戳記（`C617.1`）以及系統開機時間（`35F9.1`）；若使用 `-dynamic` framework，這些宣告已包含在其自身的 `PrivacyInfo.xcprivacy` 內。

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

`NSGoView` 會在進入視窗後啟動應用程式，並在遭到釋放或收到 `stop()` 時停止它。啟動參數請在進入視窗前設定完成；其餘屬性隨時都可以變更。處理常式、元件與回呼會在主執行緒上執行，而回覆可於稍後從任意執行緒送出。檢視控制器可轉送該檢視的 `preferredStatusBarStyle`、`prefersStatusBarHidden` 與 `prefersHomeIndicatorAutoHidden`，以跟隨其系統列設定。宿主也可從控制器回傳該檢視的 `preferredScreenEdgesDeferringSystemGestures`，以轉送 `mobile.DeferSystemGestures`。

## Android

對於匯入 `base/terminal` 的 arm64 應用程式，AAR 會在 `libnsgo.so` 旁包含 `jni/arm64-v8a/libghostty-vt.so`；宿主需維持啟用原生函式庫解壓縮。Android 預先建置與終端機載入器需要 [#1452](https://github.com/nsgo-dev/nsgo/issues/1452)；目前尚未提供 x86_64 終端機函式庫。

將 AAR 加入應用程式模組，並設定打包時解壓縮原生函式庫，讓執行環境能夠找到它們：

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

在程式碼或版面配置中建立 `dev.oneo.nsgo.NsgoView`。每個檢視會在首次取得尺寸時於專屬程序中啟動應用程式，並在呼叫 `stop()` 時結束它。請將 Activity 的結果轉送給每個檢視，並在 Activity 中處理組態變更，以免應用程式遭到重啟。

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

若要在卸離原生檢視樹的同時讓 Go 應用程式保持存活，請在宿主中保留一個 `NsgoEngine` 並明確地將檢視附加至該引擎：

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

卸離會釋放原生子檢視；重新附加時則會還原目前的訊號、輸入狀態、焦點、捲動位置與繪圖內容，而不必重新執行元件。網頁檢視與媒體播放器會依應用程式設定重新建立。`stop()` 僅會卸離明確提供的引擎。每個引擎仍會保留其第一個檢視與對應的 Activity，並拒絕其他檢視，因此請在該 Activity 中處理組態變更；目前尚未支援更換 Activity 或在同一程序中顯示多個獨立檢視。

若要支援 Google Play 更新檢查，請將 `com.google.android.play:app-update:2.1.0` 加入 Android 宿主的 Gradle 相依套件，並使用[執行環境縮減規則](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro)保留 NSGo 依名稱載入的 AppUpdateManager、AppUpdateInfo、AppUpdateOptions 與 Tasks API。上述的 activity-result 轉送同樣會傳遞更新取消事件。

若要在嵌入宿主中使用 Android 驗證工作階段，請在宿主 Activity 上宣告私有回呼 scheme，並在 `onNewIntent`（以及全新的 `onCreate`）中將該 Activity 接收到的連結轉送至 `view.openLink(uri.toString())`。符合且作用中的驗證工作階段會取用其回呼；其他連結則會送往 `OnOpenLinks`。獨立運作的 `NsgoActivity` 會自行轉送這些連結。

## 桌面

Go 宿主會執行平台的事件迴圈，並將每個檢視的 `Handle` 放入其視窗中。`nsgo.NewView` 會在 handle 進入視窗時啟動應用程式，因此在此之前所做的設定都會反映在第一幀上。

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

`view.Host()` 可向應用程式傳送事件並接收其呼叫，而 `nsgo.Component` 則可為宿主所在平台註冊原生元件。

## 網頁

網頁建置本身即為一個獨立頁面，但其他網頁也可將它掛載於某個元素中作為其宿主。`mount(element, url, options)` 會回傳一個 player 物件，而 `nsgo.json` 則會以相對於自身的位置指定載入器、wasm 與資源路徑。

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

若未提供 `host` 選項，載入器會匯入 `nsgo.yaml` 中 `web.host` 指定的模組，並在該模組匯出 `createHost(player)` 時等待其執行完成。掛載於元素或框架中的應用程式會將瀏覽器的歷史紀錄與頁面標題交由外部網頁管理。

`appearance`（`"light"`、`"dark"` 或 `"system"`）、`locale` 與 `fontScale` 可取代瀏覽器的預設設定，就像原生宿主的設定一樣，且之後可透過 player 的 `setAppearance`、`setLocale` 與 `setFontScale` 進行變更。`module` 參數（已編譯的 `WebAssembly.Module` 或其 Promise）可讓多次掛載同一個應用程式的頁面只下載並編譯一次。

在應用程式啟動前，載入器會顯示一個 `progress.nsgo-load-progress` 元素。你可以用 CSS 為其設計樣式、在 mount 選項中傳入 `onProgress`，或針對應用程式自身的頁面從 `web.host` 匯出 `onProgress`。該回呼會取代預設進度條，並接收解碼後位元組數的 `{ loaded, total }`，當總大小未知時 `total` 為 `null`。載入器會在啟動結束時傳入 `null` 呼叫它，以便清除載入畫面 UI。使用已編譯的 `module` 時會略過下載進度。

```yaml
web:
  host: host.js
```

## 宿主通道

`core.Host` 是所有宿主共同實作的唯一介面契約：啟動參數、宿主發送的事件，以及供應用程式呼叫的宿主函式。數值以與 JSON 相同的方式跨界傳遞，因此 map 會成為 `map[string]any`、數字成為 `float64`，同一份程式碼在 Swift、Java、Go 與 JavaScript 中都能運作。

- `HostParams` 會在啟動時讀取一次啟動參數。
- `cx.OnHostEvent` 會接收 `Host.Send` 與網頁的 `hostEvent`。
- `cx.CallHost` 呼叫指定名稱的處理常式並取得其回應；`cx.PostHost` 則發起無須回應的呼叫。
- `Host.HandleOpenURL` 會在平台之前攔截 `cx.OpenURL`，其回傳結果決定平台是否也要開啟該網址。
- `Host.SetParams`、`Send`、`Handle` 與 `HandleDefault` 可安全地從任何 goroutine 呼叫。

在 iOS 上，宿主透過 `params` 設定參數、`handle` 安裝處理常式、`send` 發送事件，並透過 `onOpenURL` 回應開啟連結請求。在 Android 上則分別對應 `setParams`、`handle`、`send` 與 `setOpenUrlHandler`。在桌面平台上使用 `core.Host` 的方法，而在網頁上則由頁面的 `host` 物件回應。

## 原生元件

`core.Native` 用於顯示宿主建立的檢視：其 `Props` 會傳遞給宿主元件的 `SetProp`，元件送出的事件會抵達 `OnEvent`，且數值為動畫節點的屬性會在影格時脈上自行移動，不增加引擎負擔。

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

在宿主中註冊元件：iOS 使用 `app.register(component:)` 與 `NSGoComponent`，Android 使用 `registerComponent` 與 `NsgoComponent`，桌面則使用 `nsgo.Component`。網頁沒有原生元件，因此未知的名稱在網頁上只會顯示空白方框。

## 設定

`tokens` 可依角色名稱（`accent`、`inset`、`raised`）覆蓋應用程式的主題，可同時套用於兩種外觀或單一外觀；`cx.Palette()` 會讀到這些覆蓋值。宿主同時也負責設定語系、文字大小、外觀與資料目錄，並接收應用程式的錯誤回報。`nsgo build -embed` 不會產生任何視窗、選單或系統列圖示：這些都屬於宿主的職責。

若檢視的引擎 goroutine 上發生內部 panic，該引擎將會停止、銷毀其附掛的表面，並將 panic 與堆疊追蹤回報給宿主的錯誤監聽器（在 Go 中為 `nsgo.OnError`）。其他引擎會繼續運作；建立新檢視即可重新啟動失敗的應用程式。一般應用程式回呼中的 panic 仍會優先交給最近的 `core.Catch` 處理。

`examples/embed-ios`、`examples/embed-android`、`examples/embed-desktop` 與 `examples/embed-web` 展示了同一支應用程式搭配各種宿主的範例。

## 下一步

- [平台服務](services.md)：應用程式可向平台請求的功能。
- [建置與發佈](building.md)：`-embed` 產生的套件與簽署方式。
- [範例：待辦事項應用程式](tutorials/todo.md)：供宿主嵌入的範例應用程式。
