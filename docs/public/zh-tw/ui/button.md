---
title: 按鈕（Button）
description: 按鈕、其變體與尺寸，以及圖示按鈕。
group: 元件
order: 31
---

`ui.Button` 是採用 `ui` 外觀的按鈕，具備視窗所需的各種變體與尺寸。`OnPress` 會在完成按下動作時執行。

```go preview=button/basic
ui.Button{Text: "Save", OnPress: save}
ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: cancel}
```

## 變體與尺寸

`ui.Button.Variant` 可設為 `ButtonPrimary`（預設）、`ButtonSecondary`、`ButtonOutline`、`ButtonGhost`、`ButtonTinted` 或 `ButtonDanger`。`Tinted` 與 `Danger` 會在柔和填色上使用主題的強調色或危險色。在桌面與電視介面模式下，`Size` 為 `SizeDefault`（28 點）、`SizeSmall`（24 點）或 `SizeLarge`（32 點）。在觸控介面模式下，相同的尺寸對應為 44、40 與 48 點，因此無須重新設定樣式即可直接使用該按鈕。

```go preview=button/variants
ui.Button{Text: "Delete", Variant: ui.ButtonOutline, OnPress: remove}
ui.Button{Text: "Continue", Size: ui.SizeLarge, OnPress: next}
ui.Button{Text: "Save", Icon: lucide.Check, Flex: 1, OnPress: save}
```

`Color` 會使用應用程式指定的顏色填滿按鈕，而非變體預設的顏色；`Tint` 則會為其圖示著色。`Flex` 會為它在水平列中分配比例空間，而 `Style` 負責版面配置：它可以讓按鈕填滿容器、設定應用程式自訂的大小、加入外距（margin）或設定圓角；`TestID` 則用於為測試命名該按鈕。

```go
ui.Button{Text: "Continue", Size: ui.SizeLarge,
    Style: core.Style{Width: core.Pct(100), Height: core.Pt(50)}, OnPress: next}
```

僅包含圖示的按鈕為 `ui.IconButton`，其 `Variant` 可設為 `IconButtonGhost`、`IconButtonRaised` 或 `IconButtonFilled`，外觀為使用主題內縮色（inset color）或 `Fill` 的扁平圓形，並具有以點為單位的 `Size` 以及供輔助技術使用的 `Label`。在桌面上，當游標停留在按鈕上時，該標籤也會顯示為工具提示（tooltip）；`Tooltip` 可在該處顯示不同的文字，若其回傳 `""` 則不顯示提示。僅有 `Icon` 而沒有 `Text` 的 `ui.Button` 也會呈現為正方形，同樣需要設定 `Label`。

```go preview=button/icon
ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost, OnPress: remove}
```

## 內容與狀態

`ui.Button` 會顯示 `Text`、選用的前置 `Icon`，或兩者同時顯示。使用 `TextStyle` 可自訂標籤的字體大小、字重或其他文字樣式。

```go
ui.Button{Text: "Saving…", Disabled: saving.Get, OnPress: save}
```

`Disabled` 接收一個函式，讓按鈕能夠跟隨訊號變化：停用的按鈕既不會接收點擊，也不會取得鍵盤焦點，且其外觀會呈現停用狀態，而非由應用程式自行挑選的顏色。

`Busy` 用於仍在執行的動作。當它回傳 true 時，內容上方會顯示一個與按鈕文字同色的旋轉指示器（spinner），因此即使在主要按鈕的強調色上也能清楚顯示；此時點擊不會有任何反應，且輔助技術會接收到忙碌狀態的通知。預設情況下，原本的內容會在旋轉指示器下方保留其空間，按鈕也會維持鍵盤焦點。`BusyLabel` 或 `BusyLabelFunc` 會在旋轉指示器旁顯示訊息，並在忙碌期間將其回報為按鈕的無障礙名稱。按鈕至少會維持其原本靜止時的寬度。

```go preview=button/busy
ui.Button{Text: "Save", Busy: saving.Get, BusyLabel: "Saving…", OnPress: save}
```

`ui.Spinner` 與 `ui.ProgressRing` 皆可自行接受 `Tint` 屬性，以應對強調色會變得不明顯的其他背景表面。

## 連結

`ui.Link{Text: "Docs", Href: "/docs"}` 會以連結角色開啟目標位置。除非 `Style.Color` 設定了其他顏色，否則其文字與懸停底線皆使用強調色；`Style.Hover.Color` 則可設定懸停時的顏色。

## 無障礙設計

按鈕會回報 `RoleButton`，並以其文字作為名稱。`Label` 會替換提供給輔助技術的文字。沒有設定 `Label` 的純圖示按鈕無法被讀取，因此請務必為其命名。

在桌面、網頁與 Apple TV 上，對著取得焦點的按鈕按下 Return 或 Space 鍵即可觸發點擊；手機的螢幕閱讀器則會將其當作點擊動作來觸發。

## 平台行為

該按鈕是在各處皆以相同方式繪製的檢視，而非平台原生的按鈕控制項；點擊、焦點與無障礙功能則來自平台本身。如前所述，觸控介面模式會使用較大的版面配置點擊目標。

## 相關文件

- [表單](../forms.md)：表單中的按鈕與驗證。
- [樣式與主題](../styles.md)：按鈕從中取得顏色的主題系統。
- [無障礙設計](../accessibility.md)：角色、標籤與焦點。
- 元件藝廊的 Button 頁面。
- [無樣式元件](../authoring/base.md#button)：`base.Button`，用於打造專屬外觀的按鈕。
