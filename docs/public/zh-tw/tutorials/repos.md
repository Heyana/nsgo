---
title: "範例：熱門 GitHub 儲存庫"
description: 從 GitHub 的 API 列出最多星號的 Go 儲存庫，在 goroutine 中載入並包含載入指示器、錯誤處理與重試功能。
group: 快速開始
order: 13
---

本教學將建置一個應用程式，列出 GitHub 上最多星號的 Go 儲存庫。內容涵蓋所有與伺服器通訊的應用程式都需要的功能：不阻塞畫面的請求、載入指示器、附帶重試的錯誤處理，以及無須網路即可執行的測試。本教學假設你已完成[環境設定](../getting-started.md)。

## 建立應用程式

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## 撰寫應用程式

將 `app.go` 的全部內容替換為：

```go program height=480
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// api lists the most-starred Go repositories on GitHub.
var api = "https://api.github.com/search/repositories?q=language:go&sort=stars&per_page=30"

type repo struct {
	Name        string `json:"full_name"`
	Description string `json:"description"`
	Stars       int    `json:"stargazers_count"`
}

// fetch runs on its own goroutine, never the engine's. ctx is cancelled
// once its answer no longer counts.
func fetch(ctx context.Context) ([]repo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, api, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub answered %s", resp.Status)
	}
	var body struct {
		Items []repo `json:"items"`
	}
	err = json.NewDecoder(resp.Body).Decode(&body)
	return body.Items, err
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	repos := core.NewResource(cx, nil, func(ctx context.Context, _ struct{}, _ core.ResourceInfo) ([]repo, error) {
		return fetch(ctx)
	})
	list := func() []repo {
		list, _ := repos.Get()
		return list
	}
	failure := func() string {
		if err := repos.Err(); err != nil {
			return err.Error()
		}
		return ""
	}
	colors := ui.ColorsOf(cx)

	return core.View{
		Style:     core.Style{FlexGrow: 1, FlexShrink: 1, PaddingEnv: core.EnvAll(core.EnvSafeArea)},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Background()} },
		Children: []core.Node{
			core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.All(16)}, Children: []core.Node{
				ui.Text("Popular Go repositories", 22, ui.Bold, colors.Text),
				core.Show{When: repos.Loading, Then: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} }},
			}},
			core.Show{
				When: func() bool { return repos.State() == core.ResourceErrored },
				Then: func(*core.Scope) core.Node {
					return core.View{Style: core.Style{Padding: core.All(16), Gap: 12, AlignItems: core.AlignStart}, Children: []core.Node{
						ui.Textf(failure, 14, ui.Regular, colors.Muted),
						ui.Button{Text: "Try again", OnPress: repos.Refetch},
					}}
				},
				Else: func(*core.Scope) core.Node {
					return core.List{
						Count:         func() int { return len(list()) },
						Key:           func(i int) any { return list()[i].Name },
						EstimatedSize: 64,
						Style:         core.Style{FlexGrow: 1},
						Row: func(cx *core.Scope, row core.ListRow) core.Node {
							r := list()[row.Index()]
							return core.View{Style: core.Style{Padding: core.Symmetric(10, 16), Gap: 2}, Children: []core.Node{
								core.View{Style: core.Style{Direction: core.Row, JustifyContent: core.JustifySpaceBetween, Gap: 8}, Children: []core.Node{
									ui.Text(r.Name, 15, ui.Medium, colors.Text),
									ui.Text(fmt.Sprintf("★ %d", r.Stars), 13, ui.Regular, colors.Muted),
								}},
								ui.Text(r.Description, 13, ui.Regular, colors.Muted),
							}}
						},
					}
				},
			},
		},
	}
}
```

請求執行期間會顯示載入指示器，隨後清單便會填入資料。

## 運作原理

**使用 Go 原生的網路功能。** `net/http` 可在每個目標平台上運作，並自動沿用系統的 TLS 與代理伺服器設定，因此 `fetch` 就是標準的 Go 程式碼。

**在引擎的 goroutine 之外執行。** 元件與訊號屬於引擎的 goroutine，絕對不能在其中等待網路回應。`core.NewResource` 會在獨立的 goroutine 上執行 `fetch`，並將結果帶回引擎。它的來源（source）為 nil，因此只會載入一次，而點擊 **Try again** 則會呼叫 `Refetch` 重新載入。只有最新一次的載入結果有效：離開畫面時會透過 `ctx` 取消請求，且較晚抵達的回應永遠不會覆蓋較新的結果。

**將狀態視為訊號。** `repos.Get`、`repos.Err`、`repos.State` 與 `repos.Loading` 都是訊號，因此讀取它們的繫結會自動跟隨每次載入更新。`core.Show` 會在載入期間顯示載入指示器，並在結果出現時顯示附帶 **Try again** 按鈕的錯誤訊息或儲存庫清單，且僅在條件變更時重建該部分介面。重試時會保留上一次的清單，直到新資料抵達為止。

**支援任意長度的清單。** `core.List` 只會建構螢幕附近的列，並以儲存庫名稱作為鍵值（key），因此新的載入結果會重複使用相同儲存庫的列。

## 執行測試

請求的 URL 是一個變數，因此測試可以透過 `httptest` 將其指向本機伺服器，進而控制「網路」所回傳的內容：

```go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// serve 會以指定的 body 回應應用程式的請求，藉此取代 GitHub。
func serve(t *testing.T, status int, body string) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		w.Write([]byte(body))
	}))
	t.Cleanup(server.Close)
	api = server.URL
}

func TestShowsRepositories(t *testing.T) {
	serve(t, http.StatusOK, `{"items":[{"full_name":"golang/go","description":"The Go programming language","stargazers_count":130000}]}`)
	app := nsgotest.Start(t, Root)
	app.WaitFor("text:golang/go", 5*time.Second)
	app.Find("text:★ 130000")
}

func TestRetriesAfterAFailure(t *testing.T) {
	serve(t, http.StatusServiceUnavailable, "")
	app := nsgotest.Start(t, Root)
	app.WaitFor("text:Try again", 5*time.Second)
}
```

```sh
go tool nsgo test
```

`WaitFor` 會等待回應抵達並顯示於畫面上。

## 下一步

- [平台服務](../services.md#網路連線)：深入了解網路連線、儲存空間與檔案。
- [清單與捲動](../lists.md)：下拉重新整理、對齊吸附與長清單。
- [執行與除錯](../debugging.md)：使用 `-trace` 觀察每一次更新。
