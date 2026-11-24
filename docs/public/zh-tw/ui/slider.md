---
title: 滑桿
description: 透過鍵盤與指標沿著軌道選取單一數值或範圍。
group: 元件
order: 37
---

`ui.Slider` 可透過拖曳、步階與鍵盤沿著軌道選取數值。單一數值使用單個滑塊；設定 `High` 則可建立雙滑塊的範圍。

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## 範圍

設定 `High` 與 `OnHighChange` 即可選取兩個數值。`Value` 為下限值。`LowLabel` 與 `HighLabel` 用於為輔助技術命名兩個滑塊。

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## 步階與數值

`Step` 會將數值貼齊步階；步階為 0 時則接受任意數值。`ValueText` 可變更輔助技術朗讀的數值內容，而 `Disabled` 會停用此控制項。

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

當範圍或步階會變動時（例如影片回報其總長度），請使用 `MinFunc`、`MaxFunc` 與 `StepFunc`。它們會覆蓋對應的靜態欄位並即時更新已掛載的滑桿，同時保留其焦點與任何進行中的拖曳操作。明確傳回 0 的 `MaxFunc` 允許在總長度取得前維持收合的範圍；應用程式的數值會被保留，並在目前的邊界內顯示。

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

在 TSX 中，`min={minimum()}`、`max={duration()}` 與 `step={step()}` 會原地追蹤其訊號；`max={0}` 會將範圍收合，如同傳回 0 的 `MaxFunc` 一樣。

## 方向

`Axis: base.Vertical` 會將滑桿直立放置，最小值位於底部；請在 `Style` 中為其指定高度。`Inverted` 會將最小值移至另一端：垂直滑桿的頂端，或水平滑桿的末端。沿著軌道移動的方向鍵也會跟隨此方向。`ui.Progress` 接受相同的 `Axis` 與 `Inverted`。

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## 行為

拖曳會跟隨指標移動，並在移動過程中持續透過 `OnChange` 回報數值。方向鍵會將取得焦點的滑塊移動一個步階，Home 與 End 鍵會移至兩端，而 Page Up 與 Page Down 鍵則會移動較大的幅度。當 `Step` 大於 0 時，`ui.Slider` 會在軌道下方標記步階刻度，最多顯示 60 個刻度。

## 無障礙設計

滑桿會回報 `RoleAdjustable`、其 `Label` 以及其數值；範圍滑桿則會讓兩個滑塊各自帶有自己的標籤。朗讀時會以 `ValueText` 取代數字。鍵盤以及螢幕閱讀器的遞增與遞減動作皆可變更其數值。

## 平台行為

軌道由應用程式自行繪製，因此在所有平台上尺寸皆相同。按壓、焦點與螢幕閱讀器交由平台處理；觸控拖曳的行為與指標拖曳相同，並套用相同的步階。

## 相關資源

- [表單](../forms.md)：表單中的滑桿。
- [無障礙設計](../accessibility.md)：數值與可調整角色。
- 展示畫廊的 Slider、RangeSlider 與 Rating 頁面。
- [無樣式元件](../authoring/base.md#slider)：`base.Slider`，支援垂直方向與自訂刻度標記。
