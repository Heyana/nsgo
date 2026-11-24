---
title: "Ví dụ: Các kho GitHub phổ biến"
description: Liệt kê các kho Go có nhiều sao nhất từ API của GitHub, tải trên một goroutine kèm biểu tượng đang tải, lỗi và nút thử lại.
group: Bắt đầu
order: 13
---

Hướng dẫn này xây dựng một ứng dụng liệt kê các kho Go có nhiều sao nhất trên GitHub. Nó bao quát những gì mọi ứng dụng giao tiếp với máy chủ đều cần: một request không chặn màn hình, một chỉ báo đang tải, một thông báo lỗi kèm nút thử lại, và một bài kiểm thử chạy được mà không cần mạng. Hướng dẫn giả định bạn đã hoàn tất phần [cài đặt](../getting-started.md).

## Tạo ứng dụng

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Viết ứng dụng

Thay toàn bộ nội dung `app.go`:

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

Biểu tượng đang tải hiển thị trong lúc request chạy, sau đó danh sách được điền vào.

## Cách hoạt động

**Mạng của chính Go.** `net/http` hoạt động trên mọi nền tảng đích, với thiết lập TLS và proxy của hệ thống, nên `fetch` chỉ là Go thuần.

**Ngoài goroutine của engine.** Các thành phần và signal thuộc về goroutine của engine, vốn không bao giờ được phép chờ mạng. `core.NewResource` chạy `fetch` trên một goroutine riêng và đưa kết quả trở lại engine. Nguồn của nó là nil, nên nó chỉ tải một lần, và **Try again** gọi `Refetch` để tải lại. Chỉ lần tải mới nhất được tính: rời khỏi màn hình sẽ hủy request thông qua `ctx`, và một câu trả lời đến muộn không bao giờ thay thế câu trả lời mới hơn.

**Trạng thái là signal.** `repos.Get`, `repos.Err`, `repos.State` và `repos.Loading` là các signal, nên các binding đọc chúng sẽ bám theo từng lần tải. `core.Show` hiển thị biểu tượng đang tải trong khi tải, rồi hiển thị hoặc lỗi kèm **Try again**, hoặc danh sách, và chỉ dựng lại phần đó khi điều kiện của nó thay đổi. Một lần thử lại giữ danh sách cũ cho đến khi danh sách mới đến.

**Danh sách có độ dài bất kỳ.** `core.List` chỉ dựng các hàng ở gần màn hình, với khóa là tên kho, nên một kết quả mới sẽ dùng lại các hàng của cùng các kho đó.

## Kiểm thử

URL của request là một biến, nên bài kiểm thử trỏ nó tới một máy chủ cục bộ bằng `httptest` và kiểm soát những gì "mạng" trả lời:

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

// serve answers the app's requests with body instead of GitHub.
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

`WaitFor` chờ cho đến khi câu trả lời đã đến và màn hình hiển thị nó.

## Tiếp theo

- [Dịch vụ nền tảng](../services.md#networking): thêm về mạng, lưu trữ và tệp.
- [Danh sách và cuộn](../lists.md): kéo để làm mới, căn khớp (snapping) và danh sách dài.
- [Chạy và gỡ lỗi](../debugging.md): theo dõi từng lần cập nhật với `-trace`.
