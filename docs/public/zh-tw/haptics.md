---
title: 觸覺回饋
description: 播放預設的觸覺回饋或定時的脈衝序列。
group: 平台功能
order: 46
---

## 播放單次觸覺回饋

`mobile.PlayHaptic` 會在 iOS 與 Android 上於使用者指尖處播放觸覺回饋，在其他平台上則不執行任何動作，因此請將它與畫面上的視覺變化搭配使用。`HapticSelection` 標記數值改變，例如選擇器、分段控制項或滑桿的步進；`HapticLight` 到 `HapticRigid` 依照元素的重量與彈性標記碰撞感；而 `HapticSuccess`、`HapticWarning` 與 `HapticError` 則標記任務以對應結果結束。

```go
mobile.PlayHaptic(cx, mobile.HapticSelection)
```

## 播放觸覺模式

`cx.Env().CanPlayHapticPattern()` 會回報是否支援定時觸覺回饋。一個模式（pattern）包含多個脈衝，每個脈衝具有相對於模式起始點的開始時間 `At`。`Duration` 為零時會產生瞬態脈衝（transient），正值的持續時間則會維持脈衝最多 30 秒。`Intensity` 與 `Sharpness` 的範圍為 0 到 1。新的模式會取代先前的模式；傳入空模式可停止播放。當應用程式移至背景時，模式也會停止播放。

```go
mobile.PlayHapticPattern(cx, mobile.HapticPattern{
    {Intensity: 0.25, Sharpness: 0.4},
    {At: 100 * time.Millisecond, Intensity: 0.55, Sharpness: 0.6},
    {At: 200 * time.Millisecond, Intensity: 1, Sharpness: 0.8},
})
```

iOS 在支援的硬體上會透過 Core Haptics 播放這些事件。Android 會依照時間軸執行，並在震動馬達支援振幅控制時套用強度。支援 `navigator.vibrate` 的 Android 瀏覽器會在發生使用者互動後依照開啟與關閉時間執行。Android 與網頁會將瞬態脈衝作為 20 毫秒的脈衝播放，而銳利度（sharpness）僅在 iOS 上提供。在不支援的平台上，此呼叫不會產生任何效果。

在測試中，透過 `nsgotest.Environment` 設定 `HapticPatterns: true`，並讀取 `app.HapticPatterns`。當應用程式取消播放時，歷史紀錄中會包含一筆 nil 項目。

## 使用時機

請在手勢發生的當下播放觸覺回饋，而非在結果傳回時才播放：數值經過時觸發選取回饋、檢視落定位時觸發撞擊回饋、任務結束時觸發結果回饋。請為使用者觸發的變更播放回饋，不要為應用程式自行執行的動作播放。

## 下一步

- [通知](notifications.md)：傳達相同訊息的通知與角標。
- [平台服務](services.md)：系統提供的其他功能。
- [無障礙設計](accessibility.md)：無法感受觸覺的使用者所能聽到的播報。
