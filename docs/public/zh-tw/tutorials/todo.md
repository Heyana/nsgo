---
title: "範例：待辦事項應用程式"
description: 一步一步建置一個小型待辦事項應用程式，包含訊號、清單與測試。
group: 快速開始
order: 11
---

本教學將建置一個小型的待辦事項應用程式：新增項目、勾選完成與刪除。大約需要十分鐘，並假設已完成[環境設定](../getting-started.md)。

## 建立應用程式

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## 建置待辦事項應用程式

產生的應用程式是一個計數器。請將 `app.go` 的全部內容替換為以下程式碼：

```go program height=480
package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type todo struct {
	id   int
	text string
	done bool
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	items := core.NewSignalFunc(cx, []todo{}, nil)
	draft := core.NewSignal(cx, "")
	nextID := core.NewSignal(cx, 1)
	colors := ui.ColorsOf(cx)

	add := func() {
		text := strings.TrimSpace(draft.Get())
		if text == "" {
			return
		}
		id := nextID.Get()
		nextID.Set(id + 1)
		items.Update(func(list []todo) []todo {
			return append(list, todo{id: id, text: text})
		})
		draft.Set("")
	}
	setDone := func(id int, done bool) {
		items.Update(func(list []todo) []todo {
			next := append([]todo(nil), list...)
			for i := range next {
				if next[i].id == id {
					next[i].done = done
				}
			}
			return next
		})
	}
	remove := func(id int) {
		items.Update(func(list []todo) []todo {
			out := list[:0:0]
			for _, it := range list {
				if it.id != id {
					out = append(out, it)
				}
			}
			return out
		})
	}
	left := func() int {
		n := 0
		for _, it := range items.Get() {
			if !it.done {
				n++
			}
		}
		return n
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background()}
		},
		Children: []core.Node{
			ui.Text("Todos", 28, ui.Bold, colors.Text),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "What needs doing?",
					OnSubmit: func(string) { add() }, TestID: "draft", Style: core.Style{FlexGrow: 1}},
				ui.Button{Text: "Add", Icon: lucide.Plus, OnPress: add, TestID: "add"},
			}},
			ui.Textf(func() string { return fmt.Sprintf("%d left", left()) }, 13, ui.Regular, colors.Muted),
			core.List{
				Count:         func() int { return len(items.Get()) },
				Key:           func(i int) any { return items.Get()[i].id },
				EstimatedSize: 44,
				Style:         core.Style{FlexGrow: 1},
				Empty:         ui.Text("Nothing yet. Add one above.", 13, ui.Regular, colors.Muted),
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					index := row.Index()
					it := items.Get()[index]
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 0)},
						Children: []core.Node{
							ui.Checkbox{
								Checked: func() bool {
									for _, x := range items.Get() {
										if x.id == it.id {
											return x.done
										}
									}
									return false
								},
								OnChange: func(done bool) { setDone(it.id, done) },
								Text:     it.text,
								TestID:   fmt.Sprintf("todo-%d", it.id),
								Style:    core.Style{FlexGrow: 1, FlexShrink: 1},
							},
							ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
								TestID: fmt.Sprintf("delete-%d", it.id), OnPress: func() { remove(it.id) }},
						}}
				},
			},
		},
	}
}
```

儲存檔案後應用程式便會重啟。輸入一項任務，按下 **Add**，然後將它勾選完成。

## 運作方式

**狀態即訊號。**`items`、`draft` 與 `nextID` 都是訊號。訊號會保存一個值並追蹤讀取它的內容：當 `items` 變更時，只有讀取它的文字與列位會更新。`core.NewSignal` 使用 `==` 比較值，因此切片（slice）會改用不帶相等比較函式的 `core.NewSignalFunc`。

**更新會替換切片。**`items.Update` 會回傳一個新的切片，因此由清單的識別與列鍵（row key）決定畫面上保留哪些內容。`nextID` 會為每個待辦事項賦予一個 ID，並供 `core.List` 的 `Key` 使用；若沒有穩定的鍵值，列位會將其儲存格重複使用於其他項目。

**畫面是一棵由方框組成的樹。**`Root` 會回傳一個帶有內距（padding）、間距（gap）與安全區域內縮（inset）的 `core.View`。`FlexGrow: 1` 與 `FlexShrink: 1` 使其剛好填滿視窗而不會超出，因此長清單會在其中捲動，而不會將視窗撐出邊緣。輸入列設定了 `Direction: core.Row`；其餘部分皆為預設的垂直欄（column）排列。

**輸入框是受控的。**`draft.Get` 是欄位的值，而 `draft.Set` 會接收每次變更。按下 Return 時 `OnSubmit` 會新增任務，接著 `add` 會清除草稿，讓欄位自動清空。

**清單只會建置顯示的內容。**`core.List` 會查詢 `Count` 列數量，且僅在列位接近畫面時才建置該列。當沒有項目時會顯示 `Empty`，且每一列的核取方塊與刪除按鈕都帶有對應待辦事項的 ID，因此在清單變更後點擊時仍能找到正確的項目。

**外觀由 `ui` 提供。**`ui.Text`、`ui.Input`、`ui.Button`、`ui.Checkbox` 與 `ui.IconButton` 會從主題取得顏色，並自動跟隨淺色與深色模式。`ui.ColorsOf(cx)` 會為應用程式自行繪製的部分提供顏色，而 `StyleFunc` 會在主題切換時就地更新。

## 進行測試

產生的測試會按下計數器的按鈕，因此請將 `app_test.go` 替換為待辦事項應用程式的測試：

```go
package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAddCompleteAndDelete(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:Nothing yet. Add one above.")
	app.Type("id:draft", "Buy milk")
	app.Press("id:add")
	app.WaitFor("text:Buy milk", time.Second)
	app.Press("id:todo-1")
	app.WaitFor("text:0 left", time.Second)
	app.Press("id:delete-1")
	app.WaitFor("text:Nothing yet. Add one above.", time.Second)
}
```

該測試會以無頭模式啟動應用程式，依文字或測試 ID 尋找元素，並如同使用者般進行操作。執行指令如下：

```sh
go tool nsgo test
```

當測試在目標平台上執行時，`TestMain` 會將主執行緒交給平台，而 `go tool nsgo test -target ios_simulator` 則會在模擬器中使用原生輸入執行相同的測試。

## 發佈應用程式

```sh
go tool nsgo build
```

`build` 會針對 `nsgo.yaml` 中列出的每個目標平台，將發佈套件寫入 `dist/<name>-<version>-<target>`：包含 `.app`、`.ipa`、`.apk`、Windows 與 Linux 的目錄，或是網站。所有供分發的檔案也會輸出至 `dist/releases`：磁碟映像檔、安裝程式、Debian 與 Arch 套件、IPA、APK、網站、小程式，以及從 GitHub Releases 安裝應用程式的腳本。[建置與發佈](../building.md)涵蓋了安裝程式、簽署與公證、發佈、圖示、小工具（widget）、App Clips 與 App Store。

## 下一步

- [範例：筆記應用程式](notes.md)與[範例：熱門 GitHub 儲存庫](repos.md)：另外兩個應用程式的逐步教學。
- [元件與狀態](../components.md)：深入瞭解元素、元件、訊號與區域（region）。
- [版面配置](../layout.md)與[樣式與主題](../styles.md)：畫面的結構與外觀。
- [清單與捲動](../lists.md)：長清單、固定標頭（sticky header）與載入更多項目。
- [測試](../testing.md)：尋找控制項、按下它們並擷取畫面。
- [嵌入](../embedding.md)：將應用程式嵌入至另一個應用程式中。
