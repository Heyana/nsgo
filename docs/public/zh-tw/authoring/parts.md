---
title: 部分與狀態
description: 不持有應用程式擁有的狀態，逐部分設定樣式，並向輔助技術說明元件的語意。
group: 撰寫元件
order: 71
---

無樣式元件負責決定控制項的行為，並將外觀交由使用者決定。本頁將比照 `base` 建立控制項的方式，實作一個可切換開關狀態的膠囊元件 `Toggle`。

## 受控數值

控制項不應保存應用程式所擁有的數值：由應用程式透過函式傳入目前數值，並在回呼中更新它。元件只會在顯示該數值的位置讀取它，如此一來數值變更時只會更新該部分。

```go
// Toggle is a pill that is on or off. Its parts are the pill and a dot
// inside it, then its children, the label.
type Toggle struct {
	On       func() bool
	Disabled func() bool
	// OnChange receives the value a press asks for, the opposite of On's.
	OnChange func(on bool)

	Class        string
	Style        core.Style
	StyleFunc    func() core.Style
	DotClass     string
	DotStyle     core.Style
	DotStyleFunc func() core.Style
	Children     []core.Node
	Label        string
}
```

應用程式會將該數值保存在訊號中：`chips.Toggle{On: bold.Get, OnChange: bold.Set, Label: "Bold"}`。

## 部分

元件繪製的每一個部分都應接受 `Class`、`Style` 與 `StyleFunc`，並依該部分命名（`DotClass`、`DotStyle`、`DotStyleFunc`）。`Style` 為固定樣式，`StyleFunc` 會跟隨其讀取的訊號更新，而類別則套用於兩者之下，讓應用程式能自由選擇熟悉的寫法。

每個部分透過 `PartStates` 取得控制項的狀態，而非各自回報狀態，如此一來 `checked:` 類別與 `Checked` 樣式都能套用至該部分，同時輔助技術只會看到單一個控制項：

```go
func (t Toggle) Render(*core.Scope) core.Node {
	on := func() bool { return t.On != nil && t.On() }
	off := func() bool { return t.Disabled != nil && t.Disabled() }
	shows := func() core.PartStates { return core.PartStates{Checked: on(), Disabled: off()} }
	dot := core.View{Class: t.DotClass, Style: t.DotStyle, StyleFunc: t.DotStyleFunc, PartStates: shows}
	return core.View{
		Class: t.Class, Style: t.Style, StyleFunc: t.StyleFunc,
		Role: core.RoleSwitch, Label: t.Label, Checked: on, Disabled: t.Disabled,
		OnPress: func(core.PressEvent) {
			if t.OnChange != nil {
				t.OnChange(!on())
			}
		},
		Children: append([]core.Node{dot}, t.Children...),
	}
}
```

應用程式可以僅透過類別來設定其樣式：

```go
chips.Toggle{
	On: bold.Get, OnChange: bold.Set, Label: "Bold",
	Class:    "flex-row items-center gap-2 rounded-full border border-zinc-300 px-3 py-1 checked:bg-blue-50",
	DotClass: "size-2 rounded-full bg-zinc-300 checked:bg-blue-500",
	Children: []core.Node{core.Text{Content: "Bold"}},
}
```

同樣地，`PartStates.Invalid` 會將 `aria-invalid:` 類別與 `Style.AriaInvalid` 套用至裝飾性部分（包含其內部的 `group-aria-invalid:` 樣式），而不會額外回報一個無效控制項。請將 `Invalid` 設定在實際的控制項或原生輸入欄位上，以確保輔助技術能接收其驗證狀態。

## 無障礙設計

控制項的根節點負責宣告其身分：`Role`、在文字未說明時提供的 `Label`、說明啟用後動作的 `Hint`，以及適用的 `Checked`、`Selected`、`Disabled`、`Invalid` 或 `Expanded`。內部各部分則保持靜默。以方向鍵在項目間移動的控制項（例如分頁列）應只佔用一個 Tab 停駐點。[無障礙設計](../accessibility.md)列出了所有角色與動作。

## 應透傳的屬性

若元件的根節點本身是一個檢視，應接收該檢視的 `OnLayout`、`Tooltip` 與 `TestID` 並向下傳遞，使應用程式能像對待任何檢視一樣對其進行量測、加上工具提示與測試。回呼函式應為選填：當 `OnChange` 為 nil 時，控制項應僅顯示數值而不變更它。
