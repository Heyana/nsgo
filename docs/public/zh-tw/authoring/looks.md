---
title: 外觀與主題
description: 從生效的主題取得顏色，並為元件提供變體與尺寸。
group: 撰寫元件
order: 72
---

有樣式元件只負責選擇樣式而不處理行為邏輯：它包裝一個無樣式元件（通常放在獨立套件中，例如 `chipsui`），並將行為留在無樣式元件內。它的顏色來自主題，因此能自動跟隨應用程式的主題、面板專屬的主題以及淺色或深色外觀，無需重新建置。

## 從主題取得顏色

在 `StyleFunc` 內讀取顏色：當其讀取的訊號變更時，該函式會重新執行，因此在切換主題或外觀時會原地重新套用元件樣式。`cx.Palette()` 會傳回元件所在位置顯示的調色盤，而 `ui.ColorsOf(cx)` 則列出 `ui` 使用的顏色角色。

```go
// Toggle is chips.Toggle in the theme's colors.
type Toggle struct {
	On       func() bool
	OnChange func(bool)
	Label    string
}

func (t Toggle) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	on := func() bool { return t.On != nil && t.On() }
	return chips.Toggle{On: t.On, OnChange: t.OnChange, Label: t.Label,
		Class: "flex-row items-center gap-2 rounded-full px-3 py-1",
		StyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.Accent()}
			}
			return core.Style{Background: colors.Inset()}
		},
		DotClass: "size-2 rounded-full",
		DotStyleFunc: func() core.Style {
			if on() {
				return core.Style{Background: colors.OnAccent()}
			}
			return core.Style{Background: colors.Faint()}
		},
		Children: []core.Node{ui.Text(t.Label, 13, ui.Medium, colors.Text)},
	}
}
```

寫死的顏色會在深色模式與應用程式自訂主題下失效；請務必從顏色角色取得每一個顏色。

## 自訂顏色角色

若調色盤中沒有對應的顏色角色，可將其以自訂名稱存入 `Palette.Colors`，並同時在淺色與深色外觀中設定。應用程式可在程式碼中覆寫，或在主題檔案中的 `light` 與 `dark` 區段下覆寫：

```go
theme := core.DefaultTheme()
theme.Light.SetColor("chip", core.RGB(0xeef5ff))
theme.Dark.SetColor("chip", core.RGB(0x1b2a3d))
cx.SetTheme(theme)
```

元件可透過 `cx.Palette().Color("chip")` 讀取該顏色，此方法同時會回報主題是否定義了該顏色，讓元件在未定義時能退回使用標準角色。漸層、陰影及其他裝飾效果並非單純顏色：請將它們保留在程式碼中，並依據 `cx.Dark()` 選擇。

## 變體

具有多種外觀的元件可接受 `Variant`，這是一個獨立型別，其零值應對應預設外觀。它只應命名元件實際繪製的外觀（包含顏色），而非使用多個布林欄位或共用的顏色型別：

```go
// ToggleVariant is a toggle's look.
type ToggleVariant uint8

const (
	// ToggleSolid fills the pill with the accent when on.
	ToggleSolid ToggleVariant = iota
	// ToggleOutline draws its edge in the accent instead.
	ToggleOutline
)
```

## 尺寸

大多數元件只需要單一尺寸，這樣無需應用程式額外指定即可讓控制項整齊對齊。若元件確實需要多種尺寸，請接受 `Size`，使用包含預設、小、大三種規格的 `ui.Size`，以便與 `ui.Button` 的 28、24 與 32 點高度一致；`Size.Height()` 會傳回可與其他控制項對齊的高度。至於在任意比例下皆維持單一形狀的元件（例如頭像），則應直接接受以點為單位的 `Size`。
