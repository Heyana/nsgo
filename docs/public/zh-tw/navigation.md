---
title: 導覽
description: 原生導覽堆疊、系統標題列、強制回應視窗、選單與分頁列。
group: 核心概念
order: 56
---

## 堆疊

`base.NewNav` 維護單一型別路由的堆疊：`Push` 會新增頁面、`Pop` 會移除最上層頁面、`PopToRoot` 會返回第一頁，而 `Replace` 則會替換最上層頁面。其 `Stack` 會將每個路由渲染為一個頁面，在顯示最上層頁面的同時，保持下方頁面及其狀態處於掛載狀態。`ui.NavPage` 會為每個頁面提供 `ui` 的背景以及包含標題、返回按鈕與操作按鈕的 `ui.NavHeader`。

watchOS 沒有系統導覽列、分頁列或開關，因此在該平台上 `ui.NavPage` 會在頁面頂端自行繪製 `ui.NavHeader`（包含返回按鈕），`ui.TabShell` 會繪製自己的分頁列，而 `ui.SwitchSystem` 則是 `ui` 自身的開關。

```go
nav := base.NewNav(cx, "inbox")

return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, route string, depth func() int) base.NavPage {
	if route == "inbox" {
		return ui.NavPage(cx, base.NavPage{Title: "Inbox", Children: []core.Node{
			ui.Button{Text: "Open a message", OnPress: func() { nav.Push("message") }},
		}}, ui.NavHeader{})
	}
	return ui.NavPage(cx, base.NavPage{Title: "Message"}, ui.NavHeader{
		OnBack: func() { nav.Pop(1) }, BackLabel: "Inbox",
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Archive, Label: "Archive", Variant: ui.IconButtonGhost, OnPress: archive}},
	})
})
```

在 iOS 與 Android 上，平台會依據頁面的標題與標題列按鈕繪製系統標題列，並附帶原生的返回按鈕與轉場效果；在 macOS、Windows、Linux 與網頁上則由 `ui` 自行繪製。

Android 使用 Material 3 頂部應用程式列（top app bar）。一般標題列高度為 64 dp、標題大小為 22 sp。`HeaderLargeTitle` 則提供高度 152 dp、標題大小 28 sp 的大型標題列，當頁面頂端的捲動檢視將內容捲動至其下方時，會收合為一般標題列；當內容回到頂端時則重新展開。

設定 `HeaderTitleStyle` 與 `HeaderLargeTitleStyle` 可自訂 iOS 與 Android 上原生精簡標題與展開大型標題的字型樣式，包含 `FontSize`、`FontWeight`、`FontFamily` 與 `FontStyle`。未設定的欄位將保留各平台的預設值；`HeaderTitleStyleFunc` 與 `HeaderLargeTitleStyleFunc` 會追蹤變更。`HeaderStyle` 仍負責提供背景、標題顏色與按鈕色調。在 TSX 中，這些對應為 `headerTitleStyle` 與 `headerLargeTitleStyle` 樣式物件或陣列。

自訂的 `HeaderTitle` 在 Android 上僅會取代精簡標題；`Title` 仍作為展開後的大型標題。設定 `Android: core.AndroidScreenProps{HeaderRightWithLargeTitle: true}` 可將 `HeaderRight` 置於該大型標題旁。當標題收合時，其中的控制項會移入固定工具列中，且在整個過程中皆可點擊。TSX 使用 `android={{ headerRightWithLargeTitle: true }}`。當字型需要更多空間時，大型標題區塊會隨之增高。獨立的 Android 建置會啟用原生從右到左（RTL）版面配置；嵌入式宿主則必須在其 application 上設定 `android:supportsRtl="true"`，原生標題列才會跟隨應用程式的方向。

為頁面提供自訂的 `Header` 即可取代 `ui.NavHeader`。在 iOS 與 Android 上，它會成為系統標題列的標題檢視（取代 `HeaderTitle`），位於頁面的 `HeaderLeft`、`HeaderRight` 與返回按鈕之間；當其中的檢視延展或採用百分比寬度（例如搜尋欄位）時，它會填滿兩端之間的空間，否則就依其內容決定尺寸。在其他平台上則會繪製於頁面內容上方。`ui.NavHeader.Scroll` 仍可將 iOS 或 Android 頁面的捲動連結至其標題列的模糊效果。

`NoPopOnBack` 可在按下返回時保留目前頁面，`OnBackPress` 會接收該按下事件，而 `NoPopOnSwipe` 則可停用 iOS 的側滑返回手勢。`cx.OnBackPress(func() bool)` 讓應用程式能在導覽堆疊之前先行處理返回事件，例如提示尚未儲存的變更。`OnDismiss` 會回報使用者已退出該頁面，隨後堆疊便會將其移除。

## 沉浸式全螢幕

呼叫 `mobile.Immersive(cx, true)` 可為視窗請求全螢幕顯示，呼叫 `mobile.Immersive(cx, false)` 則可恢復為一般系統列設定。針對個別頁面或對話框，可設定 `StatusBar: core.StatusBarSetting{Immersive: core.ToggleOn}` 以在該層顯示期間請求全螢幕。在 TSX 中，可在頁面或對話框上使用 `statusBar={{ immersive: true }}`，或在 `nsgo.yaml` 中設定 `status_bar: { immersive: true }` 作為視窗預設值。

在 Android 上，從螢幕邊緣滑動會短暫顯示狀態列與導覽列。在 HarmonyOS 上，視窗會請求系統隱藏狀態列、三鍵導覽列與導覽指示條。在 iOS 上，狀態列會隱藏，並由 UIKit 決定何時隱藏或顯示主畫面指示條。`cx.Env().SafeArea()` 仍會持續回報系統提供的安全區域內縮，包含螢幕挖孔或瀏海區域。在 HarmonyOS 橫向模式下使用三鍵導覽時，系統會將按鈕區域回報在底部邊緣，但實際上卻將按鈕繪製於側邊，因此安全區域可能會在底部預留空間而按鈕卻覆蓋在側邊。

## 強制回應視窗

`ui.Dialog` 會在調暗的應用程式畫面上方顯示一個浮層介面（在觸控螢幕上為從底部彈出的工作表），而 `ui.Sheet` 則從底部顯示一個面板。除非另有設定，否則兩者皆會在按下 Escape 鍵或點擊背景遮罩時關閉。

```go
ui.Dialog{
	Open:         dialog.Get,
	OnOpenChange: func(open bool, _ base.Reason) { dialog.Set(open) },
	Title:        "Rename",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { dialog.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

`ui.AlertDialog` 用於要求使用者確認操作，而 `cx.ShowAlert` 則顯示系統原生的警告視窗或動作工作表（[Dialog](ui/dialog.md)）。

## 選單

`ui.Menu` 會從按鈕開啟一組 `ui.MenuItem`：預設顯示於按鈕旁，或當其 `Presentation` 如此指定時，在任何裝置上皆以底部工作表形式顯示。`ui.ContextMenu` 針對右鍵點擊或長按提供相同行為，而 `ui.Popover`、`ui.HoverCard` 與 `ui.Tooltip` 則用於顯示其他錨定於元件旁的內容。

```go
ui.Menu{Trigger: ui.Button{Text: "Actions", Variant: ui.ButtonSecondary}, Children: []core.Node{
	ui.MenuItem{Text: "Rename", OnSelect: rename},
	ui.MenuItem{Text: "Duplicate", OnSelect: duplicate},
	ui.MenuItem{Text: "Delete", Variant: ui.MenuItemDanger, OnSelect: remove},
}}
```

## 分頁

`ui.Tabs[T]` 為分段控制項或分頁清單，附帶一個會滑動至所選選項的指示器；`TabStyle` 會合併套用至每個分頁的樣式上，其 `Selected` 會套用至已選取的分頁，而 `IndicatorStyle` 則套用至指示器上。底線式分頁可設定 `IndicatorColor`，並透過 `IndicatorFit: TabsIndicatorContent` 讓底線貼齊內容寬度。`ui.Sidebar` 可容納桌面側邊欄所需的區段與列，而 `ui.TabShell` 則在任何視窗寬度下將頂層區段維持在分頁列旁（預設位於底部，或可透過 `Edge` / `EdgeFunc` 設為前緣或後緣的垂直排列，包含使用 `mobile.TabBarAutomatic` 以跟隨 `cx.Env().VerticalBarEdge()`）。當每個分頁皆設有 `SystemIcon` 或 SVG `Icon` 時，iOS 的任意邊緣與 Android 的底部邊緣皆會採用系統分頁列，並在 iOS 上 `SystemIcon` 未指定 SF Symbol 或 Android 上未指定 drawable 時改為顯示 SVG；否則將改以 SVG 圖示自行繪製分頁列。當推入的詳細頁面顯示時，設定 `BarHidden` 可隱藏任一種分頁列並還原該區段的安全區域；當分頁列隱藏或垂直放置時，`TabBarInset` 會變為 0。

`mobile.TabBar` 是 iOS 與 Android 上的系統分頁列，具備 Liquid Glass 或 Material 外觀；它接收由 `mobile.TabBarItem{Title, Icon, SVG}` 組成的 `Items`、目前選取的索引、`OnChange`，以及選填的 `Edge`（`mobile.TabBarBottom`、`mobile.TabBarLeading`、`mobile.TabBarTrailing` 或 `mobile.TabBarAutomatic`——在 iOS 27.1 上會跟隨 `cx.Env().VerticalBarEdge()`，在其他情況下則退回底部邊緣）或響應式的 `EdgeFunc`。Android 的調色盤角色 `tabBarBackground`、`tabBarIdle`、`tabBarSelected` 與 `tabBarIndicator` 分別設定分頁列的背景、未選取時的圖示與標籤、選取時的圖示與標籤，以及選取膠囊底色。自訂調色盤會依序退回使用 `Surface`、`TextMuted`、`Accent` 與 `accentContainer`；當未設定 `accentContainer` 時，則使用 12% 透明度的強調色作為選取膠囊底色。預設調色盤保留 Material 的背景與未選取顏色，並以強調色為已選取分頁著色。在其他平台上該元件不佔任何空間，因此應用程式可在該處顯示 `ui.Tabs`。`mobile.IsAvailable(cx)` 可用來檢查是否會顯示系統分頁列。`Icon` 用於指定平台圖示名稱；當 iOS 上 `Icon` 未指定 SF Symbol 或 Android 上未指定 drawable 時，`SVG`（例如 `lucide.House()`）會顯示於 iOS 與 Android 上，並如系統圖示般由分頁列著色。

將 `TabBarItem.Badge` 設為 `mobile.TabBadge{Text: "3"}` 可顯示文字徽章，設為 `mobile.TabBadge{Dot: true}` 則可顯示圓點徽章。可透過 `ItemsFunc` 更新徽章，回傳零值即可清除。無論是系統分頁列或自行繪製的分頁列，`ui.ShellTab.Badge` 皆接收 `func() mobile.TabBadge`。當兩個欄位同時設定時，文字會優先顯示。UIKit 會為圓點徽章顯示空白的原生徽章。輔助技術會朗讀徽章文字，或是代表新內容的在地化詞彙。TSX 在原生項目上接收 `badge: { text: "3" }` 或 `badge: { dot: true }`，在 `TabShell` 項目上則接收回傳該值的函式。

```go
mobile.TabBar{
	Items: []mobile.TabBarItem{{Title: "Home", Icon: "house"}, {Title: "Search", Icon: "magnifyingglass"}},
	Selected: func() int { return tab.Get() },
	OnChange: func(i int) { tab.Set(i) },
}
```

## 下一步

- [表單](forms.md)：堆疊頁面中可放置的內容。
- [動畫](animation.md)：頁面與強制回應視窗的動態轉場方式。
- [版面配置](layout.md)：標題列周圍的安全區域。
