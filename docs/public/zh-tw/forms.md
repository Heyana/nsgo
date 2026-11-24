---
title: 表單
description: 文字欄位、焦點、螢幕鍵盤、開關、選擇器與驗證。
group: 核心概念
order: 57
---

## 文字欄位

`ui.Input` 是套用了 `ui` 外觀的平台文字欄位，並具備 `base.Input` 的所有欄位。使用 `Value` 時為受控模式：由應用程式持有數值並在 `OnChange` 中更新；使用 `DefaultValue` 時則由欄位自行管理數值。

```go
name, email := core.NewSignal(cx, ""), &core.Ref{}

ui.Input{
	Value:         name.Get,
	OnChange:      name.Set,
	Placeholder:   "Your name",
	ReturnKeyType: core.ReturnKeyNext,
	OnSubmit:      func(string) { email.Focus() },
}
```

`Masked` 可遮蔽字元；`Readonly` 會禁止編輯但仍允許選取與複製文字（在小程式中僅微信單行欄位支援此行為：微信的多行欄位與支付寶的所有欄位都會像 `Disabled` 一樣停用，因為這些宿主無法復原編輯）；`Disabled` 會停用欄位；`Multiline` 會讓文字換行並隨內容增長高度，除非樣式明確指定尺寸。多行欄位的每一行（包含預留位置文字）高度皆等同於樣式的 `LineHeight`，且字型如同 `core.Text` 般置中於行內；單行欄位則將該行文字置中於欄位高度內。`KeyboardType` 可選擇螢幕鍵盤類型（`core.KeyboardEmail`、`core.KeyboardNumber`、`core.KeyboardURL` 等），`AutoComplete` 向系統說明欄位內容以利自動填入，而 `MaxLength` 則限制使用者可輸入的 UTF-16 單元數上限。`AutoCapitalize` 與 `NoAutoCorrect` 可調整鍵盤行為。

`OnSelectionChange` 與 `OnCaretMove` 會回報選取範圍與插入點游標位置，方便在游標處開啟選單。`Suggest`（即 `core.Suggest`）可追蹤如 `@` 等觸發字元後的查詢字串，欄位旁的 `ui.SuggestList` 則會在游標處提供候選列，並可透過方向鍵上、下、Return 與點擊選取，就像 `chat.Composer` 的 `Suggest` 用於提及使用者一樣。`OnKeyPress` 會接收每個按鍵事件、`OnPaste` 接收使用者貼上的內容，而 `OnCompositionChange` 則追蹤輸入法組字過程。

在 iOS 與 Android 上，`cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, handler)` 會在欄位執行送出、失焦或插入換行之前先行攔截螢幕鍵盤的 Return 鍵。`KeyAuto` 與 `KeyNotEditing` 則交由 `SubmitBehavior` 決定；在 iOS 上，用於確認組字文字的 Return 鍵仍會保留給輸入法處理。

## 焦點與鍵盤

`Ref.Focus()` 會將焦點移至文字欄位，或在桌面、網頁與 Apple TV 上移至已啟用的可點擊或 `Focusable` 檢視。`Ref.Blur()` 僅清除自身檢視的焦點，而 `cx.DismissKeyboard()` 會關閉螢幕鍵盤。`OnFocus` 與 `OnBlur` 會追蹤焦點變化，`View.OnFocus`/`OnBlur` 對任何可聚焦的檢視亦同。當已聚焦的檢視離開檢視樹時（例如其 `Show` 關閉或該列被移除），會在移除它的提交之後收到一次 `OnBlur`——或者當平台先回報下一個焦點時，在該檢視的 `OnFocus` 之前收到——而周圍仍留在樹上的檢視則會收到 `OnFocusOut`。

當元素的內容位於尚未在任何位置顯示的 `Holder` 中時，透過 `Ref` 發出的聚焦、失焦、捲動與縮放請求不會產生任何效果，而當元素離開檢視樹時仍在佇列中的請求也會被捨棄。重新顯示該內容時並不會重播這些請求：請於重新放置後再次呼叫。

```go
email := &core.Ref{}
ui.Input{Ref: email, Placeholder: "you@example.com"}
email.Focus()
```

螢幕鍵盤會遮擋手機畫面底部。將 `core.EnvKeyboard` 設為底部內距或外距，可讓內容保持在鍵盤上方，其移動過程也會隨鍵盤動畫同步：

```go
core.View{Style: core.Style{
	Padding:    core.All(16),
	PaddingEnv: core.EnvEdges{Bottom: core.EnvKeyboard},
	Gap:        12,
}, Children: form}
```

包覆表單的捲動檢視可在拖曳時收起鍵盤，並決定鍵盤顯示時輕觸畫面的行為：

```go
core.Scroll{Behavior: core.ScrollBehavior{
	KeyboardDismissMode:       core.KeyboardDismissOnDrag,
	KeyboardShouldPersistTaps: core.KeyboardTapsHandled,
}, Children: []core.Node{form}}
```

在程式碼中可使用 `cx.Env().KeyboardHeight()` 與 `cx.Env().KeyboardDuration()` 讀取鍵盤高度與動畫時間，並以 `cx.Env().SafeArea()` 讀取螢幕周圍的安全區域內縮。

## 選擇控制項

`ui.Switch` 用於立即生效的設定切換、`ui.Checkbox` 支援勾選、未勾選或半選（indeterminate）狀態，而 `ui.Radio` 則是 `ui.RadioGroup` 中的其中一項。`ui.Select[T]` 會在觸發器下方開啟選項清單，或依其 `Presentation` 設定以工作表形式顯示；`ui.Tabs[T]` 則一次顯示數個選項。

```go
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}

size := core.NewSignal(cx, "m")
ui.Select[string]{
	Options:  []ui.Option[string]{{Value: "s", Label: "Small"}, {Value: "m", Label: "Medium"}, {Value: "l", Label: "Large"}},
	Value:    size.Get,
	OnChange: size.Set,
	Label:    "Size",
}
```

`ui.NumberInput` 與 `base.NumberInput` 接受 `LabelFunc` 以提供動態變化的無障礙名稱。在欄位非編輯狀態時，自訂的 `Format` 會追蹤它所讀取的訊號；切換語言時會保留目前編輯中的草稿，直到編輯結束為止。`ui.NumberInput` 可在邊界內微調及拖曳調整數值，`ui.DatePicker` 與 `ui.TimeField` 用於編輯日期與時間，`ui.ColorPicker` 用於編輯顏色，`ui.TagInput` 則用於編輯標籤清單。

`ui.OtpInput` 以格狀欄位輸入一次性驗證碼。它與 `base.OtpInput` 皆接受 `LabelFunc`，且不會取代目前聚焦的欄位或其輸入碼。`Size` 設定預設的高度與寬度，在桌面與電視上為 36 點，在觸控裝置上為 44 點。`CellWidth` 與 `CellHeight` 可分別覆蓋寬度或高度。`TextStyle` 覆蓋欄位字型排版、`CornerRadius` 變更圓角半徑，而 `CaretHeight` 則覆蓋由行高推算出的游標高度。

```go
quantity := core.NewSignal(cx, 1.0)
ui.NumberInput{Value: quantity.Get, OnChange: quantity.Set, Min: 1, Max: 99, Step: 1, Label: "Quantity"}
```

## 分組列

`ui.FormSection` 將各列放置於卡片上，上方為標題、下方為頁尾。設定 `CardStyle` 可自訂卡片的背景、邊框或圓角；`CardStyleFunc` 會合併覆蓋於其上並追蹤所讀取的訊號。`DividerInset` 會接收每一個子列（包含由 `core.Each` 建立的列）：回傳內縮值與 `true` 可覆蓋其分隔線內縮，或回傳 `false` 以保留自動與標題對齊的預設行為（其他節點為 12 點）。內縮設為 0 時會繪製全寬分隔線。

`ui.FormRow` 可在標題前放置 `Leading` 或圖示方塊。設定 `LeadingWidth` 可為你的節點保留寬度，並使下方的分隔線與標題對齊，該插槽預設寬度為 24 點。設定 `TileSize` 可同時縮放圖示方塊、圖示字形與圓角。`TitleStyle` 與 `DetailStyle` 會覆蓋標題與詳細文字的排版樣式。`TitleMaxLines` 限制標題行數（0 為不限制），`MinHeight` 以點為單位覆蓋列的最小高度，而 `Below` 可在標題與詳細文字所屬的文字欄下方放入額外節點。`MinHeight` 為 0 時維持預設值：一般裝置為 40 點，觸控裝置為 48 點。

針對單選群組，請以 `base.RadioGroup` 包覆該區段，並為每一列設定 `Role: core.RoleRadio`、`Checked` 與 `OnPress`。該列會使用 `base.Radio` 來管理其勾選與選取的無障礙狀態、單一 Tab 停駐點與方向鍵選取；點擊已勾選的列不會有任何反應。選取的值由應用程式自行管理。

`ui.FormSection.TitleFunc` 與 `FooterFunc`，以及 `ui.FormRow.TitleFunc`、`DetailFunc` 與 `LabelFunc` 可在不重建該列的情況下更新文字內容。空的詳細文字、區段標題或頁尾會移除該選填文字；空的列標籤則會採用其目前的標題。可搜尋的 `ui.Select` 透過 `SearchPlaceholderFunc` 與 `EmptyFunc` 進行繫結，同時保留開啟中的搜尋文字與選取項目。

## 驗證

驗證規則由應用程式自行掌控：以訊號保存錯誤訊息，透過 `OnBlur` 或 `OnSubmit` 設定它，並使用 `core.Show` 在有錯誤時於欄位下方顯示訊息。

```go
address := core.NewSignal(cx, "")
err := core.NewSignal(cx, "")
colors := ui.ColorsOf(cx)
core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
	ui.Input{
		Value:    address.Get,
		OnChange: func(v string) { address.Set(v); err.Set("") },
		Label:    "Email",
		OnSubmit: func(string) {
			if !strings.Contains(address.Get(), "@") {
				err.Set("Enter an email address.")
			}
		},
	},
	core.Show{
		When: func() bool { return err.Get() != "" },
		Then: func(*core.Scope) core.Node { return ui.Textf(err.Get, 12, ui.Regular, colors.Danger) },
	},
}}
```

`cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` 可為出現的訊息加上動畫，而將 `OnSubmit` 搭配 `ReturnKeyNext` 則可將鍵盤焦點移至下一個欄位。

## 下一步

- [導覽](navigation.md)：表單所在的頁面結構。
- [文字](text.md)：欄位文字的樣式設定。
- [無障礙設計](accessibility.md)：螢幕閱讀器所需的標籤與數值。
