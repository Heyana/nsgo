---
title: 無障礙設計
description: 為螢幕閱讀器與鍵盤提供標籤、角色、數值、動作與焦點支援。
group: 核心概念
order: 61
---

## 螢幕閱讀器會朗讀的內容

檢視的 `Label` 是輔助技術會唸出的名稱，`AccessibilityDescription` 會在標籤之後朗讀更多說明，`Hint` 說明啟用後的動作，而 `Role` 則代表元件的類型：`core.RoleButton`、`RoleLink`、`RoleHeader`、`RoleImage`、`RoleCheckbox`、`RoleSwitch`、`RoleRadio`、`RoleTab`、`RoleTextField`、`RoleSearchField`、`RoleStatus`、`RoleMenu`、`RoleMenuItem`、`RoleDialog`、`RoleAlertDialog`、`RoleAlert`、`RoleAdjustable`、`RoleProgressBar`、`RoleTable` 等。狀態預設為禮貌式即時區域（polite live region）；搜尋欄位則會作為搜尋方塊朗讀的文字欄位。對話框開啟時會朗讀其角色與標籤，隨後朗讀描述，正如 `ui.Dialog` 的標題與描述一樣。沒有標籤的文字元件會直接朗讀其內容。`LabelFunc`、`HintFunc` 與 `AccessibilityDescriptionFunc` 會在同一個檢視上繫結動態變化的無障礙文字，讓控制項在切換語言時仍保留焦點與狀態。所有具備標籤的核心元素皆接受 `LabelFunc`，而 `View` 與 `Text` 皆接受 `HintFunc`。

若要讓 `ui.Button`、`ui.Checkbox`、`ui.MenuItem` 與 `ui.AlertAction` 的文字動態變化，請使用 `TextFunc`，其無障礙名稱會跟隨該文字更新。按鈕與核取方塊也另外接受獨立的 `LabelFunc`。滑桿、分頁、開關、進度條與彈出觸發器接受 `LabelFunc` 以提供動態無障礙名稱。`ui.Option.LabelFunc` 則為分頁、下拉選單與複合方塊提供選項文字。數字與驗證碼輸入框、進度環及兩種旋轉載入指示器也同樣接受 `LabelFunc`。

```go
core.View{
	Role:  core.RoleButton,
	Label: "Add to cart",
	Hint:  "Adds the item and opens the cart",
	OnPress: add,
	Children: []core.Node{core.Icon{SVG: lucide.ShoppingCart}},
}
```

`AccessibilityValue` 提供範圍或文字數值：例如滑桿的最小值、最大值與目前數值，或是像「3 items」這樣的文字。`Accessible` 會將一個檢視合併為單一無障礙元素，使卡片的名稱與價格能被一併朗讀而不需停留兩次；若未設定標籤，該檢視會朗讀其子孫節點的文字。`AccessibilityElementsHidden` 用於隱藏裝飾性元素（例如箭頭符號或分隔線），而 `ImportantForAccessibility` 則會隱藏整個子樹。

```go
core.View{Accessible: true, Label: "Apples, $1.20 a kilo", Children: []core.Node{
	core.Text{Content: "Apples"},
	core.Text{Content: "$1.20 a kilo"},
}}
```

## 可見標籤

將控制項的 `Ref` 指定給 `core.Text` 或 `core.View` 的 `LabelFor`。點擊該標籤會將焦點移至控制項並觸發其點擊動作，例如切換核取方塊。標籤目前的可見文字會成為控制項的名稱，無需重複設定 `Label` 字串，且其無障礙狀態與樣式會跟隨控制項的 `Disabled` 與 `Invalid` 狀態。標籤本身不會獲得按鈕角色或 Tab 停駐點。

```go
field := new(core.Ref)
return core.View{Children: []core.Node{
	core.Text{Content: "Email", LabelFor: field},
	base.Input{Ref: field},
}}
```

`LabelledBy: []*core.Ref{label, requirement}` 會依指定順序以多個可見元素為控制項命名，其優先權高於 `Label`；它不會轉發點擊事件。在其他情況下，`Label` 優先於隱含的 `LabelFor` 名稱，而多個 `LabelFor` 的順序則依元件樹順序排列。文字更新、移動、隱藏、移除與重新掛載都會更新此關聯。遺失、重複、循環及隱藏的參照會被忽略；關聯範圍僅限於同一介面與限制範圍（confinement scope）內。標籤無法啟用位於強制回應視窗背後的控制項。

`base` 與 `ui` 的按鈕、核取方塊、單選按鈕、開關、下拉選單與數字輸入框會將 `Ref` 與 `LabelledBy` 傳遞給其實際控制項；滑桿則公開了 `ThumbRef` 與 `StartThumbRef`。在 TSX 中，請搭配透過 `ref={node => field.set(node)}` 填入的節點訊號，使用 `labelFor={field()}` 與 `labelledBy={[label()]}`，如此一來，位於欄位前方的標籤就能在該欄位掛載時更新。

## 動作

`AccessibilityActions` 列出螢幕閱讀器在元素上提供的動作：包含標準動作（`core.ActionActivate`、`ActionIncrement`、`ActionDecrement`、`ActionEscape`、`ActionMagicTap`）以及應用程式自訂的動作，每個動作皆具備 `Name` 與 `Label`。`OnAccessibilityAction` 會接收該動作名稱。在 `core.View` 或 `core.Input` 上使用 `AccessibilityActionsFunc`，即可在元素保持掛載的同時變更可用動作或翻譯其標籤。

```go
core.View{
	Role: core.RoleAdjustable,
	Label: "Quantity",
	AccessibilityValue: func() core.AccessibilityValue {
		return core.AccessibilityValue{Max: 10, Now: float32(n.Get()), Text: fmt.Sprintf("%d items", n.Get())}
	},
	AccessibilityActions: []core.AccessibilityAction{{Name: core.ActionIncrement}, {Name: core.ActionDecrement}},
	OnAccessibilityAction: func(name string) {
		if name == core.ActionIncrement {
			n.Update(func(v int) int { return v + 1 })
		} else {
			n.Update(func(v int) int { return v - 1 })
		}
	},
	Children: quantityControls,
}
```

`MultiSelectable` 標記允許選取多個項目的格線、清單方塊或樹狀結構。`base.DataTable` 會在多列選取時設定此屬性；網頁、Windows 與 Linux 會公開對應的無障礙旗標。

`Expanded` 表示展開項目或選單按鈕是否處於開啟狀態，`Busy` 表示內容正在載入中，`Invalid` 表示應用程式拒絕了欄位內容或控制項數值（可使用 `aria-invalid:` 設定樣式），而 `Checked`、`Selected` 與 `Pressed` 則回報控制項的狀態。控制項的各個部分可透過 `PartStates` 取得其狀態，讓指示器能跟隨核取方塊的狀態而無需成為獨立的焦點停駐點。

`SelectionItem` 會將檢視標記為可選取集合中的項目，不受其目前 `Selected` 數值影響。`base.DataTable` 會在支援選取的列或儲存格上設定此屬性，`base.Tree` 則會在每一列上設定；Windows 會保留 SelectionItem 模式，而網頁則會寫入 `aria-selected="true"` 或 `"false"`。未啟用選取功能的表格不會在列上加上這兩種標記。

## 即時變更

`LiveRegion` 會讓螢幕閱讀器在檢視的標籤及其內部文字變更時進行播報：`core.LiveRegionPolite` 會等待使用者空閒時才播報，`LiveRegionAssertive` 則會立即中斷目前語音進行播報。`cx.Announce` 則用於單次播報一則訊息。

```go
core.Text{ContentFunc: func() string { return fmt.Sprintf("%d items in the basket", count.Get()) },
	LiveRegion: core.LiveRegionPolite}
```

## 焦點與鍵盤

工具提示會在觸發元素取得焦點前先行說明該元素。`View.Tooltip`、`base.Tooltip` 與 `ui.Tooltip` 會將其目前的文字作為控制項的無障礙描述，除非該文字與控制項名稱重複、應用程式已提供 `AccessibilityDescription`，或工具提示處於 `Disabled` 狀態。包裝元件會將該描述傳遞給其內部的控制項。氣泡框及其子元件對螢幕閱讀器隱藏，且永遠不會觸發警告播報；因此 `ui.IconButton` 的預設工具提示不會重複朗讀其標籤。

`base.DataTable` 會將其具名格線公開為鍵盤與無障礙焦點停駐點；指標點擊層則對輔助技術隱藏。

在桌面平台、網頁與 Apple TV 上，除了文字欄位外，接受點擊事件的檢視也會成為 Tab 停駐點；`NoFocus` 可將檢視排除在 Tab 順序之外，而 `Focusable` 則可讓不接收點擊的檢視成為焦點停駐點。`Style.FocusVisible` 僅在使用鍵盤移動焦點時繪製焦點外框（`focus-visible:`），`Style.Focus` 適用於任何焦點，而 `Style.FocusWithin` 則偵測檢視內部的焦點；`OnFocus` 與 `OnBlur` 會接收焦點事件（當取得焦點的檢視離開元件樹時也會觸發 `OnBlur`），而 `Ref.Focus()` 與 `Ref.SetAccessibilityFocus()` 可移動焦點。`core.FocusedRef(cx)` 是指向目前持有焦點之檢視的參照，適合用於非觸發器開啟的介面（例如透過快捷鍵開啟的尋找列），以便在關閉時將焦點歸還原處。按下 Return 與 Space 會觸發目前取得焦點的檢視，方向鍵則可在分頁列、單選按鈕群組與選單之間移動。當鍵盤將焦點移至檢視上時，其 `Tooltip` 會立即顯示，直到焦點離開或按下 Escape 鍵為止。彈出視窗的 `OnOpen` 會在內容就緒後執行，因此在此呼叫 `Focus` 可將焦點移入其中（非強制回應對話框應如此實作）；彈出氣泡框的內容在 Tab 順序中緊接於其觸發器之後，且彈出視窗關閉時焦點會回到觸發器上。

在 Android、iPhone 與 iPad 上，針對外接硬體鍵盤，`Ref.Focus()` 與按鈕的 `AutoFocus` 可以將焦點移至已啟用的可點擊檢視或 `Focusable` 檢視。觸控點擊控制項不會移動該焦點。在按鍵對應之後，Space 與 Return 會啟用取得焦點的可點擊檢視，並在每個按壓階段保留 Control、Alt、Shift 與平台修飾鍵。`NoFocus`、停用或隱藏的控制項，以及位於強制回應視窗背後的控制項無法取得此焦點。將焦點移至檢視上不會開啟軟體鍵盤；原生輸入欄位則維持原有的編輯行為。watchOS 與小程式沒有實體鍵盤的啟用路徑。

`cx.Env().ScreenReader()` 回報螢幕閱讀器是否正在執行，`cx.Env().ReduceMotion()` 回報使用者是否要求減少動態效果，`cx.Env().IncreaseContrast()` 回報使用者是否要求提高對比度（macOS、iOS 與 watchOS 上的增加對比、Windows 高對比佈景主題、GNOME 高對比、GTK 4.20 對比設定或高對比 GTK 主題、Android 14 高於標準的對比設定，以及網頁上的 `prefers-contrast: more` 或 `forced-colors: active`），`cx.Env().FontScale()` 回報文字縮放比例，而 `cx.Env().RTL()` 則回報版面配置方向。除非設定了 `NoFontScaling`，否則文字會跟隨文字大小縮放；而使用邏輯邊界的版面配置在由右至左模式時會自動鏡像其物理邊界。

## 下一步

- [樣式與主題](styles.md)：焦點與停用狀態。
- [表單](forms.md)：欄位的標籤與數值。
- [畫布與著色器](canvas.md)：公開繪圖中的各個部分。
