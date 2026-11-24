---
title: 輸入框
description: 平台原生的文字欄位，支援受控或非受控模式、焦點與螢幕鍵盤。
group: 元件
order: 33
---

`ui.Input` 是具備 `ui` 外觀的平台文字欄位，除非設定了 `Multiline`，否則預設為單行。與所有套用樣式的欄位一樣，它具備 `base.Input` 的所有欄位；`ui.Field` 則是相同但沒有預設尺寸的欄位，其外框不會隨指標懸停或焦點而改變；將 `Background` 設為 `core.Transparent` 並將 `BorderWidth` 設為 `core.Zero` 可隱藏外框，呈現出外觀如同純文字但可編輯的效果。

```go preview=input/basic
name := core.NewSignal(cx, "")
ui.Input{
	Value:       name.Get,
	OnChange:    name.Set,
	Placeholder: "Your name",
	TestID:      "name",
}
```

## 受控與非受控

使用 `Value` 時，欄位為受控模式：由應用程式持有數值並在 `OnChange` 中更新。使用 `DefaultValue` 時，欄位會自行管理其數值。受控欄位可透過將數值設為 `""` 來清空內容。

```go preview=input/submit
draft := core.NewSignal(cx, "")
ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "New task",
	OnSubmit: func(string) { add(); draft.Set("") }}
```

## 欄位屬性

`Multiline` 會讓文字自動換行，起始高度為三行並隨內容增長，除非在樣式中設定了 `Height` 或 `MinHeight`。`Masked` 可遮蔽字元，`Readonly` 會禁止編輯但仍允許選取與複製文字，`Disabled` 會停用欄位，而 `MaxLength` 則限制使用者可輸入的 UTF-16 單元數上限。`Placeholder` 亦可透過 `PlaceholderFunc` 指定為函式。

```go
ui.Input{Masked: true, Value: password.Get, OnChange: password.Set,
	AutoComplete: core.AutoCompletePassword}
```

`KeyboardType` 用於選擇螢幕鍵盤類型（`KeyboardEmail`、`KeyboardNumber`、`KeyboardURL` 等），`ReturnKeyType` 設定 Return 鍵的標籤文字，而 `SubmitBehavior` 則決定按下 Return 鍵時是要送出還是為多行欄位換行。`AutoCapitalize`、`NoAutoCorrect` 與 `AutoComplete` 可調整系統提供的輸入輔助功能。

`InputSource` 會在欄位取得焦點時切換至指定的鍵盤輸入來源，並在失去焦點時恢復使用者原本的設定，除非同時設定了 `KeepInputSource`：例如在 macOS 上使用 `"com.apple.keylayout.ABC"`，可讓輸入程式碼或指令的欄位維持英文輸入。該 ID 為各平台原生識別碼，在 Windows 上為鍵盤配置 ID（如 `"00000409"`），在 Linux 上則為 Fcitx 5 或 IBus 輸入法；若指定了尚未啟用的來源，或在手機與網頁平台上，則不會有任何變更。

```go
ui.Input{Placeholder: "Bundle ID", InputSource: "com.apple.keylayout.ABC"}
```

`Style` 會合併覆蓋於欄位的預設外觀之上，而 `StyleFunc` 則會進一步覆蓋於其上：它們所設定的屬性（例如高度、內距、字型或顏色）會取代 `ui` 的預設值，其餘屬性則維持不變。只要兩者皆未設定 `BorderColor` 或 `Hover`，外框在指標懸停時會變深，並在取得焦點時套用強調色。`Padding` 為零值時會保留 `ui` 的預設內距；若要移除內距，請使用代表「已設定為零」的長度值 `core.Zero`（`Padding: core.All(core.Zero)`）。

```go
ui.Input{Placeholder: "Filter", Style: core.Style{Height: core.Pt(ui.SizeSmall.Height()), FontSize: 12}}
ui.Field{DefaultValue: "Untitled", Style: core.Style{FontSize: 24, FontWeight: ui.Bold,
	Background: core.Transparent, BorderWidth: core.Zero}}
```

## 焦點與事件

`Ref` 可讓程式主動對欄位進行聚焦與失焦操作。`OnFocus` 與 `OnBlur` 會追蹤焦點變化，`OnSelectionChange` 與 `OnCaretMove` 追蹤選取範圍與游標移動，`OnKeyPress` 接收每次按鍵輸入，`OnPaste` 接收貼上的內容，而 `OnCompositionChange` 則追蹤輸入法的組字狀態。

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

螢幕鍵盤會遮擋手機畫面底部。將 `core.EnvKeyboard` 套用於內容的底部內距或外距，可讓內容保持在鍵盤上方，且移動過程會與鍵盤動畫同步。`cx.DismissKeyboard()` 可關閉鍵盤，而 `cx.Env().KeyboardHeight()` 可讀取鍵盤高度。

## 驗證

驗證規則由應用程式自行掌控。透過一個訊號保存錯誤訊息，在 `OnBlur` 或 `OnSubmit` 時設定它，並於欄位下方顯示該訊息（[表單](../forms.md#validation)）。

```go preview=input/validation
err := core.NewSignal(cx, "")
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{Value: email.Get, OnChange: email.Set, Label: "Email",
		OnSubmit: func(string) {
			if !strings.Contains(email.Get(), "@") {
				err.Set("Enter an email address.")
			} else {
				err.Set("")
			}
		}},
	ui.Textf(err.Get, 12, ui.Regular, colors.Danger),
}}
```

## 無障礙設計

欄位會回報 `RoleTextField`（或在 `Role` 指定為搜尋框時回報 `RoleSearchField`），並朗讀其 `Label`；`Placeholder` 並非標籤，因此請為沒有可見標籤的欄位設定 `Label`。系統會回報 `Readonly` 與 `Disabled` 狀態，並將 `OnSubmit` 公開為鍵盤的送出動作。在 macOS、iOS 與 Linux 上，單行、遮蔽及多行欄位都會將目前的 `Placeholder` 公開為原生的無障礙預留位置文字或提示，並在更新與清除時不會為多行欄位所繪製的預留位置標籤額外產生第二個焦點停駐點。

## 平台行為

iOS、Android、Windows 與 Linux 皆使用各自平台的原生文字欄位；網頁則使用 `<input>` 或 `<textarea>`。螢幕鍵盤、自動填入、輸入法與文字方向均由平台提供。焦點與文字輸入在所有目標平台上皆採用相同的方式進行測試。

## 相關主題

- [表單](../forms.md)：欄位、驗證、選項控制項與鍵盤。
- [無障礙設計](../accessibility.md)：標籤與焦點。
- [版面配置](../layout.md)：讓內容保持在鍵盤上方。
- 元件展覽館中的 Input 與 Form 頁面。
