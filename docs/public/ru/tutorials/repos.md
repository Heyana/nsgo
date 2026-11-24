---
title: "Пример: популярные репозитории GitHub"
description: Выведите список Go-репозиториев с наибольшим числом звёзд из API GitHub, загружая данные в горутине с индикатором загрузки, ошибками и повтором.
group: Начало работы
order: 13
---

В этом руководстве создаётся приложение, которое выводит список Go-репозиториев с наибольшим числом звёзд на GitHub. Оно охватывает всё, что нужно любому приложению, работающему с сервером: запрос, не блокирующий экран, индикатор загрузки, ошибку с возможностью повтора и тест, который выполняется без сети. Предполагается, что вы выполнили [установку](../getting-started.md).

## Создание приложения

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Написание приложения

Замените всё содержимое `app.go`:

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

Пока выполняется запрос, отображается индикатор загрузки, затем заполняется список.

## Как это работает

**Собственная сетевая библиотека Go.** `net/http` работает на всех целевых платформах с системными настройками TLS и прокси, поэтому `fetch` — это обычный Go.

**Вне горутины движка.** Компоненты и сигналы принадлежат горутине движка, которая никогда не должна ждать сеть. `core.NewResource` выполняет `fetch` в отдельной горутине и возвращает результат движку. Его источник равен nil, поэтому он загружает данные один раз, а **Try again** вызывает `Refetch`, чтобы загрузить их снова. Учитывается только последняя загрузка: уход с экрана отменяет запрос через `ctx`, а запоздавший ответ никогда не заменяет более новый.

**Состояние в виде сигналов.** `repos.Get`, `repos.Err`, `repos.State` и `repos.Loading` являются сигналами, поэтому привязки, которые их читают, следуют за каждой загрузкой. `core.Show` показывает индикатор во время загрузки, а затем либо ошибку с **Try again**, либо список, перестраивая только эту часть при изменении её условия. При повторе последний список сохраняется до прихода нового.

**Список любой длины.** `core.List` создаёт только строки рядом с видимой областью, с ключом по имени репозитория, поэтому новый результат повторно использует строки тех же репозиториев.

## Тестирование

URL запроса хранится в переменной, поэтому тест направляет его на локальный сервер с помощью `httptest` и управляет тем, что отвечает «сеть»:

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

`WaitFor` ждёт, пока придёт ответ и экран его отобразит.

## Далее

- [Сервисы платформы](../services.md#networking): подробнее о сети, хранилище и файлах.
- [Списки и прокрутка](../lists.md): обновление жестом «потянуть», привязка прокрутки и длинные списки.
- [Запуск и отладка](../debugging.md): отслеживайте каждое обновление с помощью `-trace`.
