---
title: 選單
description: 由觸發器開啟的選單、情境選單及其項目。
group: 元件
order: 42
---

`ui.Menu` 會從按鈕開啟 `ui.MenuItem`：無論在任何裝置上，皆可顯示在按鈕旁，或透過 `Presentation: base.PresentSheet` 以從底部滑出的工作表形式呈現。`ui.ContextMenu` 則會在指標位置開啟相同的選單項目。

```go preview=menu/basic height=280
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", Icon: lucide.Pencil, OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", Shortcut: "mod+d", OnSelect: duplicate},
	ui.MenuSeparator{},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

選單會自行開啟與關閉；`Open` 與 `OnOpenChange` 可讓應用程式掌控其開關狀態。`Trigger` 決定按鈕的外觀與內容，並由選單接手處理其點擊事件。彈出選單永遠不會超出觸發器旁的可用空間：它會開啟在能夠完整容納的一側，而當項目超出該空間或超過 `MaxHeight` 時便會捲動。

## 選單項目

`ui.MenuItem` 會顯示 `Text`、可選的 `Icon` 以及 `Shortcut`（例如 `"mod+c"`，在彈出選單中會依平台慣例格式顯示），並在選取時回報 `OnSelect`；除非設定了 `NoCloseOnSelect`，否則選取後會關閉選單。`Disabled` 會停用項目，`Checked` 會在項目旁顯示勾選標記，而 `Variant: ui.MenuItemDanger` 則標示具破壞性的操作。`ui.MenuLabel` 作為分組的標題，`ui.MenuSeparator` 用於分隔各組。`ui.Submenu` 可容納更多項目，並會開啟在該項目旁，或在工作表模式中取代周圍的項目顯示。

```go preview=menu/checked height=320
ui.Menu{Trigger: ui.Button{Text: "View", Variant: ui.ButtonGhost}, Children: []core.Node{
	ui.MenuLabel{Text: "Sort by"},
	ui.MenuItem{Text: "Name", Checked: func() bool { return sort.Get() == "name" }, OnSelect: func() { sort.Set("name") }},
	ui.MenuItem{Text: "Date", Checked: func() bool { return sort.Get() == "date" }, OnSelect: func() { sort.Set("date") }},
	ui.MenuSeparator{},
	ui.Submenu{Text: "Share", Children: []core.Node{
		ui.MenuItem{Text: "Copy link", OnSelect: copyLink},
		ui.MenuItem{Text: "Email", OnSelect: emailLink},
	}},
}}
```

## 情境選單

`ui.ContextMenu` 會在對其 `Trigger` 進行次要點擊，或在觸控螢幕上長按時，於指標位置開啟選單項目。

```go preview=menu/context height=220
ui.ContextMenu{Trigger: fileRow, Children: []core.Node{
	ui.MenuItem{Text: "Open", OnSelect: open},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## 行為

按下 Escape、返回鍵或點擊選單外部皆會關閉選單。選單會依 `Placement` 與 `Align` 定位為彈出選單，或在指定時作為從底部開啟的工作表。在桌面平台上，Down、Up、Home、End、Page Down 與 Page Up 可在已啟用的項目間移動，並從指標懸停的項目開始；Right 開啟子選單、Left 關閉子選單，而 Return 則確認選取。透過鍵盤開啟選單時會聚焦於第一個項目，並在選單關閉時將焦點歸還給觸發器。

## 無障礙支援

選單會回報 `RoleMenu`，每個項目則回報 `RoleMenuItem`。當觸發器的內容未提供名稱時，可由 `Label` 為其命名。螢幕閱讀器會朗讀項目的文字以及是否處於停用狀態。

## 平台特性

觸發器與項目由應用程式提供；工作表、彈出選單、焦點管理、Escape 與返回鍵則由平台處理。要使用彈出選單或工作表由應用程式自行決定，NSGo 不會依據裝置類型自動選擇。

## 相關資源

- [選擇器](select.md)：從清單中選取單一值。
- [對話框](dialog.md)：強制回應（modal）介面。
- [分頁](tabs.md)：一次顯示多個區段。
- 展示畫廊（gallery）的 Menu、ContextMenu 與 Command 頁面。
- [無樣式元件](../authoring/base.md#menu)：`base.Menu`，用於建置自訂外觀的選單。
