---
title: 開關
description: 開啟或關閉的設定控制項，提供三種尺寸。
group: 元件
order: 35
---

`ui.Switch` 用於立即生效的設定。它本身不保存數值：由應用程式持有該數值並在 `OnChange` 中更新。

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## 尺寸

`ui.Switch` 接受 `Checked`、`OnChange`、`Label` 與 `Size`（`SizeDefault`、`SizeSmall` 或 `SizeLarge`）。滑塊滑動與軌道變色會跟隨平台的影格時脈執行動畫。

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

開關適用於立即套用的設定。若為表單內的核取項目，請使用[核取方塊](checkbox.md)；若需從多個選項擇一，請在 `ui.RadioGroup` 中使用 `ui.Radio`，或使用[下拉選單](select.md)。

## 無障礙設計

開關會回報 `RoleSwitch` 及其開啟或關閉狀態。`Label` 為輔助技術朗讀的內容，而開關旁的文字通常即為其標籤。當具有鍵盤焦點時，按下 Space 鍵或點擊皆可切換其狀態。

## 平台行為

預設控制項由應用程式自行繪製，因此在各個平台上外觀一致。`Variant: ui.SwitchSystem` 會請求使用平台的原生控制項：macOS 上的 NSSwitch、iOS 上的 UISwitch，以及 Android 上 Material 3 的 52 × 32 dp 開關。其他目標平台則使用預設外觀。按壓、焦點與螢幕閱讀器交由平台處理。手機上的預設開關刻意設計為與桌面相同尺寸；`SizeSmall` 適用於密集排列的列表列，而非針對特定裝置。

## 相關資源

- [核取方塊](checkbox.md)：單一或多個選擇。
- [表單](../forms.md)：表單中的設定控制項。
- [無障礙設計](../accessibility.md)：標籤與焦點。
- 展示畫廊的 Switch 頁面。
- [無樣式元件](../authoring/base.md#switch)：`base.Switch`，用於自訂開關外觀。
