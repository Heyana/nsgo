---
title: "예제: 인기 GitHub 저장소"
description: GitHub API에서 스타가 가장 많은 Go 저장소 목록을 가져와 고루틴에서 로딩하고 스피너, 오류 처리, 재시도 기능을 구현합니다.
group: 시작하기
order: 13
---

이 튜토리얼에서는 GitHub에서 스타가 가장 많은 Go 저장소 목록을 보여 주는 앱을 만듭니다. 서버와 통신하는 모든 앱에 필요한 요소인 화면을 멈추지 않는 요청, 로딩 표시기, 재시도 기능이 포함된 오류 처리, 그리고 네트워크 없이 실행되는 테스트를 다룹니다. 먼저 [설치 및 준비](../getting-started.md)를 완료했다고 가정합니다.

## 앱 만들기

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## 앱 작성하기

`app.go`의 전체 내용을 다음 코드로 바꿉니다:

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

요청이 실행되는 동안 스피너가 표시되고, 완료되면 목록이 채워집니다.

## 동작 방식

**Go 자체 네트워킹.** `net/http`는 시스템의 TLS 및 프록시 설정과 함께 모든 대상에서 동작하므로, `fetch`는 순수 Go 코드로 작성할 수 있습니다.

**엔진 고루틴 밖에서 실행.** 컴포넌트와 시그널은 엔진의 고루틴에 속하므로 절대 네트워크 응답을 기다리며 블로킹되어서는 안 됩니다. `core.NewResource`는 별도의 고루틴에서 `fetch`를 실행하고 결과를 엔진으로 다시 전달합니다. 소스가 nil이므로 한 번만 로드되며, **Try again**을 누르면 `Refetch`를 호출해 다시 로드합니다. 가장 최근의 로드 결과만 유효합니다. 화면을 벗어나면 `ctx`를 통해 요청이 취소되며, 뒤늦게 도착한 응답이 더 새로운 응답을 덮어쓰지 않습니다.

**시그널로 상태 관리.** `repos.Get`, `repos.Err`, `repos.State`, `repos.Loading`은 모두 시그널이므로, 이를 읽는 바인딩은 각 로드의 상태를 자동으로 따릅니다. `core.Show`는 로딩 중일 때 스피너를 표시하고, 결과에 따라 **Try again** 버튼이 있는 오류 화면이나 목록을 표시하며, 조건이 바뀔 때 해당 부분만 다시 빌드합니다. 재시도할 때는 새 목록이 도착할 때까지 이전 목록을 유지합니다.

**길이에 제한 없는 리스트.** `core.List`는 저장소 이름을 키로 사용하여 화면 근처의 행만 빌드하므로, 새 결과가 도착해도 동일한 저장소의 행을 재사용합니다.

## 테스트하기

요청 URL이 변수로 되어 있으므로, 테스트에서는 `httptest`로 이를 로컬 서버로 가리키게 하여 "네트워크"가 어떤 응답을 반환할지 제어할 수 있습니다:

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

`WaitFor`는 응답이 도착하여 화면에 표시될 때까지 기다립니다.

## 다음 단계

- [플랫폼 서비스](../services.md#networking): 네트워킹, 저장소 및 파일에 대한 자세한 내용.
- [리스트와 스크롤](../lists.md): 당겨서 새로고침, 스냅 및 긴 리스트.
- [실행 및 디버깅](../debugging.md): `-trace`로 각 업데이트 추적하기.
