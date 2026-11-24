---
title: 核取方塊
description: 支援勾選、未勾選或半選狀態的核取方塊。
group: 元件
order: 34
---

`ui.Checkbox` 是具備 `ui` 外觀的核取方塊，旁側附有標籤。它本身不保存數值：由應用程式決定其是否處於勾選狀態，並在 `OnChange` 中更新該狀態。

```go preview=checkbox/basic
done := core.NewSignal(cx, false)
ui.Checkbox{Checked: done.Get, OnChange: done.Set, Text: "Remember me"}
```

## 內容

`ui.Checkbox` 會在核取方塊旁顯示 `Text` 或應用程式傳入的 `Children`。對輔助技術而言，`Label` 會取代這兩者作為無障礙名稱，而 `Hint` 則說明其用途。

## 狀態

`Indeterminate` 代表既非勾選也非未勾選的半選狀態，適用於「全選」控制項在部分項目被選取時；此時點擊會請求切換為勾選狀態。`Disabled` 會停用點擊與焦點。

```go preview=checkbox/indeterminate height=220
ui.Checkbox{
	Checked:       func() bool { return chosen.Get() == len(files) },
	Indeterminate: func() bool { return chosen.Get() > 0 && chosen.Get() < len(files) },
	OnChange:      chooseAll,
	Text:          "Select all",
}
```

## 點擊修飾鍵

當清單需要搭配 Shift 鍵點擊來進行範圍選取時，請改用 `OnChangeEvent` 而非 `OnChange`：

```go
ui.Checkbox{Checked: done.Get, Text: "Remember me", OnChangeEvent: func(next bool, press core.PressEvent) {
	if press.Modifiers.Shift {
		chooseRange(next)
		return
	}
	done.Set(next)
}}
```

設定此回呼後會取代 `OnChange`；應用程式仍需負責更新數值。在桌面平台與網頁上，透過鍵盤啟用時會在應用程式快捷鍵之後保留修飾鍵資訊；而在 Windows 上，Alt 搭配 Space 或 Return 則會開啟視窗選單。TSX 的 `onChangeEvent` 會接收 `{ checked, press }`。

## 無障礙設計

核取方塊會回報 `RoleCheckbox` 及其勾選或半選狀態。它的名稱來自 `Text`、`Label` 或子元件的文字。當它取得鍵盤焦點時，按下 Space 可切換狀態，且螢幕閱讀器會播報新的狀態。

## 平台行為

核取方塊是由應用程式自行繪製，而非使用平台原生控制項，因此在每個平台上的外觀都一致。點擊互動、焦點外框與螢幕閱讀器則由平台提供。它在所有目標平台上皆透過角色與標籤進行測試。

## 相關主題

- [開關（Switch）](switch.md)：立即生效的設定。
- [表單](../forms.md)：表單中的選項控制項。
- [清單（List）](list.md)：列中的核取方塊。
- 元件展覽館中的 Checkbox 頁面。
- [無樣式元件](../authoring/base.md#checkbox)：`base.Checkbox`，用於打造專屬外觀的核取方塊。
