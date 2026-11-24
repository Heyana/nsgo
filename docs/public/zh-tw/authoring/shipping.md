---
title: 文字、測試與發佈
description: 翻譯元件顯示的文字，以使用者的操作方式測試元件，並提供給 Go 與 TSX 應用程式使用。
group: 撰寫元件
order: 74
---

## 文字

元件所顯示的文字（例如螢幕閱讀器為按鈕朗讀的名稱）應透過套件的語系目錄提供，而非寫死字串常值。在函式庫中建立一個 `locales/` 套件，放入來源語言的語系目錄，以及用於產生程式碼的檔案：

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

執行 `go generate ./locales` 會為每則訊息產生對應函式，元件可傳入作用域呼叫：`locales.Clear(cx)` 便會以應用程式目前顯示的語言輸出該文字。請將產生的 `catalog_gen.go` 提交至版本庫，讓應用程式僅需 Go 工具鏈即可建置。為每個需要翻譯的語言新增對應檔案（例如 `zh-Hans.yaml`）；應用程式也可在自己的語系目錄中，以命名貴套件匯入路徑的鍵值提供其他語言翻譯，方式與 [NSGo 內建文字](../localization.md#nsgos-own-words)相同。

## 測試

`nsgotest` 能以無頭（headless）模式模擬真實使用者的操作方式來測試元件。將元件掛載於一個小型根元件中，並透過角色與文字進行操作：

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

在函式庫根目錄執行 `go tool nsgo test` 會測試所有套件，加上 `-target macos` 或其他目標平台則會在原生視窗中執行相同的測試。請務必測試淺色與深色兩種外觀以及偽語系（`-lang en-XA`），以抓出寫死的顏色與文字。其餘細節請參閱[測試](../testing.md)。

## 發佈

元件函式庫是一個 Go 模組：應用程式透過 `go get` 下載並匯入其套件。當同時提供無樣式與有樣式版本時，請將兩者置於不同的套件中，讓自訂外觀的應用程式不會匯入多餘的樣式。

網站的[元件](https://nsgo.dev/components/)頁面收錄了可供他人使用的函式庫；只要在 `docs/public/components/` 中新增說明文件與螢幕截圖即可登錄你的函式庫。

只要在 `init` 函式中註冊元件，TSX 應用程式也能使用 Go 元件：

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

之後透過 `import { Toggle } from "example.com/chips"` 即可將元件欄位作為 props 使用（[TSX 應用程式](../tsx.md#go-from-tsx)）。
