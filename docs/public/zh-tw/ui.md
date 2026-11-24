---
title: 元件參考
description: 最常用的元件、其變體、狀態與無障礙設計。
group: 元件
order: 30
---

這些是 `ui` 的元件，採用 NSGo 的外觀並跟隨主題。本參考文件從應用程式最先會用到的元件開始介紹；每個頁面都會先提供最短的範例，接著說明其組成部分、狀態與無障礙設計。元件藝廊（gallery）收錄了所有元件，包括尚未在此建立獨立頁面的元件。

[元件藝廊](https://nsgo.dev/gallery/)會在瀏覽器中執行每個元件的示範。

## 動作

- [按鈕（Button）](ui/button.md)：一般或強調色的按鈕，以及圖示按鈕。

## 文字與輸入

- [文字（Text）](ui/text.md)：段落、片段（span）、連結、字型與文件文字。
- [輸入框（Input）](ui/input.md)：平台的文字欄位，支援受控或非受控模式。
- [核取方塊（Checkbox）](ui/checkbox.md)：核取方塊，支援已勾選、未勾選與不確定狀態。
- [開關（Switch）](ui/switch.md)：開啟或關閉的設定開關。
- [選單選擇器（Select）](ui/select.md)：從清單中選取單一值，以下拉選單或工作表（sheet）形式呈現。
- [滑桿（Slider）](ui/slider.md)：沿著軌道選取單一數值或範圍。

## 集合

- [清單（List）](ui/list.md)：任意列數的清單，僅建置進入視野的列位，支援選取與啟用（activation）。
- [捲動檢視（Scroll）](ui/scroll.md)：平台的捲動檢視，支援分頁、貼齊（snapping）、縮放與下拉重新整理。

## 導覽

- [分頁（Tabs）](ui/tabs.md)：分頁清單、分段控制項（segmented control）與系統分頁列。

## 覆蓋層

- [對話框（Dialog）](ui/dialog.md)：強制回應（modal）介面、工作表（sheet）與系統提示框。
- [選單（Menu）](ui/menu.md)：從觸發器開啟的選單、右鍵內容選單及其項目。

## 更多

其餘元件皆收錄於元件藝廊中：表格、圖表、程式碼編輯器、RTF 富文字、終端機、Dock、命令面板、顏色／日期／表情符號選擇器（`ui.EmojiPicker`）、快訊通知（toast）等。每個元件在那裡都有互動示範。

`base/terminal.View` 可透過 Command+點擊（Windows 與 Linux 上為 Control+點擊）開啟既有的本機路徑，就如同開啟 URL 一樣。相對路徑會使用 shell 的 OSC 7 工作目錄；尚未回報工作目錄的本機 shell 則會使用 `terminal.Options.Dir`，若未設定則使用家目錄（home directory）。包含空白的檔名請加上引號。遠端目錄權限（authority）以及未回報本機目錄的連線不會解析為本機檔案。檔案與資料夾會透過 `Scope.OpenFile` 開啟；URL 連結則繼續透過 `Scope.OpenURL` 與嵌入宿主的 URL 處理常式開啟。

`base/terminal.View` 預設會拒絕程式對剪貼簿的讀取與寫入。僅在應用程式允許時才設定 `AllowClipboardRead` 與 `AllowClipboardWrite`；請求該檢視的 `AccessClipboard` 權限仍會繼續套用。TSX 的 `Terminal` 也提供相同的屬性，且可在不重新啟動工作階段的情況下變更。應用程式需自行提供同意介面。使用者明確執行的貼上動作，會獨立向使用 Kitty 剪貼簿協定（mode 5522）的程式提供文字／PNG 快照；其一次性密碼僅能讀取該次快照。未使用該模式的程式會收到一般的括號貼上（bracketed）文字。剪貼簿圖片的可用性取決於平台的剪貼簿服務；不會公開 X11 的 primary selection。

## 下一步

- [元件與狀態](components.md)：元件如何回傳元素以及訊號如何更新它們。
- [樣式與主題](styles.md)：元件從中取得顏色的主題系統。
- [無樣式元件](authoring/base.md)：不含外觀的相同控制項，供你打造專屬外觀。
- [無障礙設計](accessibility.md)：每個控制項的標籤、角色與數值。
