---
title: 子母畫面
description: 在 iOS、tvOS、macOS、Android 與瀏覽器中以子母畫面播放影片，或在 iOS 上顯示自訂元件樹。
group: 平台功能
order: 45
---

在 iOS 上，使用子母畫面之前必須先宣告背景播放；macOS 則不需要背景模式：

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

若要讓影片使用子母畫面，請啟用 `PictureInPicture`、保留一個 `core.Ref`，並透過應用程式的控制項來啟動或停止。請追蹤狀態回呼，在 `Possible` 變為 true 時啟用該控制項：

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

`AutoPictureInPicture` 可選擇在 iOS 應用程式進入背景時自動進入子母畫面；tvOS 與 macOS 會忽略此選項。`Controls` 仍會顯示原生播放控制項；對於受控管的影片，應用程式的子母畫面（PiP）按鈕會共用同一個受管工作階段。`OnPictureInPictureRestore` 可讓應用程式在使用者返回時顯示其播放器頁面。子母畫面啟用期間請保持影片掛載：刪除其作用域會停止播放並結束子母畫面。

在 Android 上，打包後的 activity 已宣告支援子母畫面。影片會進入該 activity 的 PiP 視窗並隱藏應用程式其餘介面，返回時則會保留原本的播放器與播放位置。`StopPictureInPicture` 會重新展開同一個 activity。該視窗的播放/暫停控制項會回報 `OnPlay` 與 `OnPause`（附帶 `VideoPauseControls`）；`Ref.Seek` 也使用同一個播放器。自動進入需要明確設定該選項，且影片必須已準備就緒並正在播放。即使 `Possible` 為 true，啟動仍可能因系統目前狀態而失敗；請務必處理 `Error`。

嵌入式的 Android 宿主必須宣告 `android:supportsPictureInPicture="true"`，並處理 `orientation|screenSize|screenLayout|smallestScreenSize` 組態變更。請將 activity 的 `onPictureInPictureModeChanged` 與 `onUserLeaveHint` 轉發給 `NsgoView` 上的同名方法；在 Android 15 以上版本，也請將 `PictureInPictureUiState.isTransitioningToPip()` 轉發給 `NsgoView.onPictureInPictureTransition`。不支援的宿主或裝置會回報 `Supported: false`。

在網頁平台上，需在提供標準影片 PiP API 的瀏覽器中由使用者點擊啟動。`Supported` 同時反映頁面的 Permissions Policy，而 `Possible` 則需要已載入影片中繼資料。瀏覽器自身的 PiP 控制項會回報相同的啟用狀態。停止、替換或清除來源，或是移除影片都會結束該工作階段；尚未完成的啟動請求無法重新開啟先前的來源。網頁版 PiP 僅顯示影片本身，不包含自訂 NSGo 內容、背景自動進入或 `OnPictureInPictureRestore`。不支援的瀏覽器會回報 `Supported: false`。

在 iOS 上，`mobile.PictureInPicture` 接收具備自身版面配置的 `Children`，預設大小為 320 × 180 點。它在應用程式中不佔據任何空間。其 `Ref` 具備相同的啟動與停止方法，而 `OnState`、`OnRestore`、`OnPlay`、`OnPause` 與 `OnSkip` 則負責傳遞系統動作。請將 `PausedFunc` 與 `PositionFunc` 繫結至這些回呼所更新的播放狀態；`Duration: 0` 代表直播內容。暫停執行的應用程式會保留最後一次算繪的影格。

在自訂元件樹中放入 `core.Video`，即可將其解碼後的影格與應用程式繪製的字幕或其他疊加層進行合成。請使用外層的 `Ref` 啟動工作階段。與影片共用一個暫停訊號，透過 `Video.OnProgress` 提供自訂工作階段的 `PositionFunc`，並將 `OnSkip` 轉發至影片的 `Ref.Seek`。影片的音訊會透過其自身的播放器播放。

自訂元件樹也能算繪文字、檢視、圖片與 Canvas。在擷取的影格中無法使用 WebView、Metal 著色器與背景效果。動態圖片與原生圖層轉場只會顯示其模型狀態。如需動態效果，請使用明確的動畫數值。與影片無關的用途可能會遭到 App Store 審核拒絕。

`Supported: false` 用於區分不支援的平台，以及雖支援但尚未就緒、或是透過 `Error` 說明失敗原因的工作階段。影片子母畫面支援在 AVKit 回報裝置支援 PiP 時的 iOS 與 tvOS、macOS、Android，以及提供標準 API 的網頁瀏覽器；自訂元件樹僅支援 iOS。在 iOS 與 tvOS 上，啟用子母畫面期間（包括暫停時），共用音訊工作階段會維持在 playback 類別；當影片的 `AudioSession` 設為 `AudioSessionMixWithOthers` 時，會與其他應用程式的音訊混音。

在無頭測試中，`nsgotest.PictureInPictureSupport(supported, possible)` 可設定系統在 iOS、macOS 或 Android `nsgotest.Environment` 下提供的支援狀態，檢視的 `PiPState`、`PiPStarts` 與 `PiPStops` 會記錄該工作階段，`app.ReportPictureInPicture` 會回報模擬系統的狀態，而 `app.PictureInPictureControl` 則會傳送視窗的 `play`、`pause`、`skip` 與 `restore` 指令。影片的子母畫面視窗會播放與暫停影片自身的播放器，這會透過 `Video.OnPlay` 與 `Video.OnPause` 回報，並在暫停時附帶 `VideoPauseControls`；在測試中，`app.PlayVideo` 與 `app.PauseVideo` 也會觸發這些回報。

執行 `go tool nsgo dev -target ios examples/pip` 可測試兩種路徑，或使用 `-target macos` 或 `-target android` 測試影片子母畫面。原生 Mac 測試已驗證在有無原生控制項情況下的進入、退出、重複指令，以及替換使用中的受控管來源。iPhone 的驗收測試（包含受控管影片的全螢幕與自動進入）仍待完成；目前實作具備原生委派（delegate）/影格測試與無頭應用程式測試覆蓋。

針對 tvOS 影片，請執行 `go tool nsgo dev -target tvos_simulator examples/pip`；NSGo 會將播放器與原生的「播放中」（Now Playing）工作階段關聯。tvOS 27 模擬器能載入有無原生控制項的影片，但 AVKit 在該環境回報不支援 PiP。實際的 PiP 進入、停止與恢復仍須在實體 Apple TV 上驗證。

網頁目標的迴歸測試使用錄製的本機影片片段與受信任的應用程式按鈕點擊，在 Chromium 中驗證瀏覽器實際的子母畫面工作階段，涵蓋進入、退出、替換來源與刪除。其他瀏覽器引擎仍需進行驗收測試。
