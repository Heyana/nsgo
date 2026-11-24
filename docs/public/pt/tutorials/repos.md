---
title: "Exemplo: repositórios populares do GitHub"
description: Liste os repositórios Go com mais estrelas a partir da API do GitHub, carregando em uma goroutine com um spinner, erros e uma nova tentativa.
group: Primeiros passos
order: 13
---

Este tutorial cria um app que lista os repositórios Go com mais estrelas no GitHub. Ele cobre o que todo app que conversa com um servidor precisa: uma requisição que não bloqueia a tela, um indicador de carregamento, um erro com nova tentativa e um teste que roda sem a rede. Ele pressupõe a [configuração](../getting-started.md).

## Crie o app

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Escreva o app

Substitua todo o conteúdo de `app.go`:

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

O spinner aparece enquanto a requisição é executada e, em seguida, a lista é preenchida.

## Como funciona

**A rede do próprio Go.** `net/http` funciona em todos os alvos, com as configurações de TLS e de proxy do sistema, então `fetch` é Go puro.

**Fora da goroutine do engine.** Componentes e signals pertencem à goroutine do engine, que nunca deve esperar pela rede. `core.NewResource` executa `fetch` em uma goroutine própria e traz o resultado de volta ao engine. Sua fonte é nil, então ele carrega uma vez, e **Try again** chama `Refetch` para carregar de novo. Só o carregamento mais recente vale: sair da tela cancela a requisição por meio de `ctx`, e uma resposta atrasada nunca substitui uma mais nova.

**Estado como signals.** `repos.Get`, `repos.Err`, `repos.State` e `repos.Loading` são signals, então os vínculos que os leem acompanham cada carregamento. `core.Show` mostra o spinner enquanto carrega e, depois, o erro com **Try again** ou a lista, reconstruindo apenas essa parte quando sua condição muda. Uma nova tentativa mantém a última lista até a nova chegar.

**Uma lista de qualquer tamanho.** `core.List` constrói apenas as linhas próximas da tela, identificadas pelo nome do repositório, de modo que um novo resultado reaproveita as linhas dos mesmos repositórios.

## Teste

A URL da requisição é uma variável, então um teste a aponta para um servidor local com `httptest` e controla o que a "rede" responde:

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

`WaitFor` espera até que a resposta chegue e a tela a mostre.

## Próximos passos

- [Serviços da plataforma](../services.md#networking): mais sobre rede, armazenamento e arquivos.
- [Listas e rolagem](../lists.md): puxar para atualizar, encaixe e listas longas.
- [Execução e depuração](../debugging.md): acompanhe cada atualização com `-trace`.
