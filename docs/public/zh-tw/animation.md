---
title: 動畫
description: 在平台畫面時脈上執行的轉場、動畫數值、版面配置動畫與動態預設效果。
group: 核心概念
order: 58
---

## 動畫數值

動畫會在平台的主執行緒上執行，並由平台自身的畫面時脈步進驅動，因此每影格不會對引擎造成任何負擔。`AnimatedValue` 是由平台移動的數值；`Style.Animated` 會將檢視的不透明度、位移、縮放、旋轉、背景、邊框顏色、陰影與文字顏色繫結至該數值。

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`、`Spring` 與 `Decay` 用於移動單一數值，而 `Sequence`、`Parallel`、`Stagger`、`Delay` 與 `Loop` 則將其組合起來。`cx.StartAnimation` 會回傳一個 `AnimationRun`，呼叫其 `Stop` 可讓執行中的動畫停在目前位置，`Reset` 則可將數值重設回原點。動畫會從數值目前所在的位置開始執行，且對同一個數值啟動新動畫時會結束舊的動畫。

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

衍生節點會跟隨數值變化：`core.Interpolate` 會以自訂的緩動與外插方式映射數字或顏色，而 `core.Add`、`Sub`、`Mul`、`Div`、`Mod`、`DiffClamp` 與 `SnappingDiffClamp` 則從中計算出其他數值。`ui.Lerp` 則是簡短的線性內插函式。

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` 會在下一個顯示畫面執行一次函式，而 `Scope.Animate` 則會在該時脈上依持續時間步進執行。兩者皆在元件圖（graph）的 goroutine 上執行，因此可以設定訊號。函式會收到該畫面的時間戳記，在 macOS、iOS、Android、網頁、Linux、Windows、HarmonyOS 與 watchOS 上為顯示器本身的時間（在 HarmonyOS 上，當建置機器裝有 OpenHarmony SDK 時為顯示器時間；未裝 SDK 打包的應用程式則使用引擎時脈），因此依據兩次畫面之間的時間差移動物件的迴圈能與顯示器的步調同步；在 `nsgotest` 測試中則會提供測試時脈的時間。

使用 `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})` 設定視窗的回呼頻率。傳入 `Minimum` 與 `Maximum` 可為顯示連結（display link）指定更新率範圍，例如在 ProMotion 螢幕上設定 `{Minimum: 80, Maximum: 120, Preferred: 120}`。請從視窗自身的作用域呼叫此方法。該偏好設定會套用於 `RequestAnimationFrame` 與 `Animate`，直到再次變更為止。傳入零值範圍可恢復顯示器的正常步調。回呼傳入的時間仍維持為實際的顯示時間。可用的更新率由顯示器與作業系統決定，NSGo 亦可能會跳過部分顯示影格以配合回呼步調。

`Scope.Tween` 會在該時脈上針對動畫數值無法直接觸及的對象（例如文字中遞增計數的數字）進行數值步進：它會從起點到終點依序以每個中間數值呼叫函式（如同 `Timing` 移動數值一般），並在最後再次以終點值呼叫一次。在 `nsgotest` 中，啟動此動畫的測試步驟會等待其完成至最後一個值（與一般動畫相同）；而在減少動態效果模式下，則會立即給出最終數值。TSX 的 `Animated.tween` 行為相同。

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## 轉場

`Transition` 會在檢視的樣式或類別變更時於平台端自動執行動畫，每影格無需引擎額外介入。`Properties` 指定要產生動畫的屬性；0 代表除框架（frame）之外的所有屬性。

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` 會在檢視自身的樣式為其指定新框架時，將其從目前顯示的位置移動並調整大小至目標位置。相對地，版面配置動畫則會為整次提交（commit）執行動畫，就像 React Native 的 `LayoutAnimation` 一樣：`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` 會為下一批次的移動、插入與移除執行動畫，內建預設涵蓋緩動、線性與彈簧效果。`LayoutSpring` 轉場接受 `Response`（彈簧的無阻尼自然週期）與 `DampingRatio`（`< 1` 為欠阻尼、`1` 為臨界阻尼、`> 1` 為過阻尼），它會執行至穩定為止而非固定持續時間，且在半途重新指定目標時會延續原有的位置與速度。

```go
cx.AnimateLayout(core.LayoutAnimation{
	Update: &core.LayoutTransition{
		Curve:        core.LayoutSpring,
		Response:     280 * time.Millisecond,
		DampingRatio: 0.85,
	},
})
open.Set(!open.Get())
```

`View.OnTransitionEnd` 會在變更完成其轉場後執行，無論是否有任何屬性實際移動。減少動態效果會關閉轉場與版面配置動畫，因此請確保最終狀態在無動畫時依然可達，以尊重平台設定。

## 動態效果

`core.Motion` 會依據應用程式自訂的 `MotionSpec` 或現成的 `MotionPreset`（進場、退場、注意力吸引與循環動畫），透過關鍵影格同步移動檢視的不透明度、位移、縮放與旋轉。繫結其樣式可讓檢視保持靜止狀態；`Play` 用於播放規格，而 `Exit` 則可讓檢視在 `core.Presence` 內以動態效果退場。

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` 會保留分支直到其註冊的所有退場動畫完成為止，因此無論是什麼條件移除了工作表或吐司通知，它們都能以動態效果退場。`core.PresenceOf` 會將相同行為套用至可比較的鍵值與內容函式，而 `MaxLeaving` 則會在數值快速變化時限制同時退場的舊分支數量。`MotionPresets()` 列出所有內建預設效果，而 `Kind()` 則說明該效果屬於進場、退場、吸引注意或循環類型。

## 關鍵影格

`KeyframeAnimation` 會在平台上重複播放關鍵影格：在每個偏移量指定不透明度、縮放、旋轉與垂直位移，並搭配持續時間、延遲、緩動與重複次數。`KeyframesSpin`、`KeyframesPing`、`KeyframesPulse` 與 `KeyframesBounce` 對應自 Tailwind 的預設動畫。

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` 如同 Web Animations 一般，透過多個停駐點移動動畫數值，支援偏移量、分段緩動、延遲、重複次數與方向設定。手勢也能直接驅動數值：`View.PanValues`、`PinchValue` 與 `RotationValue` 會在主執行緒上從手勢開始時的位置直接移動這些數值。

`core.PanEvent` 在每個平移階段皆包含檢視內的 `Position` 與頁面內的 `PagePosition`，因此拖曳工具無需額外透過按壓事件來取得手指位置。

## 下一步

- [樣式與主題](styles.md)：轉場能夠移動的屬性。
- [清單與捲動](lists.md)：跟隨捲動檢視變化的數值。
- [版面配置](layout.md)：動畫框架的運作原理。
