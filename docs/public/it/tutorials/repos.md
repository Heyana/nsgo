---
title: "Esempio: repository GitHub popolari"
description: Elenca i repository Go con più stelle dall'API di GitHub, caricandoli su una goroutine con uno spinner, la gestione degli errori e un nuovo tentativo.
group: Per iniziare
order: 13
---

Questo tutorial costruisce un'app che elenca i repository Go con più stelle su GitHub. Copre ciò di cui ha bisogno ogni app che comunica con un server: una richiesta che non blocca lo schermo, un indicatore di caricamento, un errore con un nuovo tentativo e un test che gira senza rete. Presuppone la [configurazione](../getting-started.md).

## Creare l'app

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Scrivere l'app

Sostituisci l'intero contenuto di `app.go`:

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

Lo spinner compare mentre la richiesta è in corso, poi l'elenco si riempie.

## Come funziona

**Il networking di Go.** `net/http` funziona su ogni target, con le impostazioni TLS e proxy del sistema, quindi `fetch` è Go puro e semplice.

**Fuori dalla goroutine del motore.** Componenti e signal appartengono alla goroutine del motore, che non deve mai restare in attesa della rete. `core.NewResource` esegue `fetch` su una goroutine propria e riporta il risultato al motore. La sua sorgente è nil, quindi carica una sola volta, e **Try again** chiama `Refetch` per caricare di nuovo. Conta solo l'ultimo caricamento: lasciare la schermata annulla la richiesta tramite `ctx`, e una risposta tardiva non sostituisce mai una più recente.

**Lo stato come signal.** `repos.Get`, `repos.Err`, `repos.State` e `repos.Loading` sono signal, quindi i binding che li leggono seguono ogni caricamento. `core.Show` mostra lo spinner durante il caricamento, e poi l'errore con **Try again** oppure l'elenco, ricostruendo solo quella parte quando la sua condizione cambia. Un nuovo tentativo mantiene l'ultimo elenco finché non arriva quello nuovo.

**Un elenco di qualsiasi lunghezza.** `core.List` costruisce solo le righe vicine allo schermo, identificate dal nome del repository, così un nuovo risultato riutilizza le righe degli stessi repository.

## Testarla

L'URL della richiesta è una variabile, quindi un test la fa puntare a un server locale con `httptest` e controlla cosa risponde la "rete":

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

`WaitFor` attende che la risposta sia arrivata e che lo schermo la mostri.

## Prossimi passi

- [Servizi di piattaforma](../services.md#networking): altro su networking, archiviazione e file.
- [Liste e scorrimento](../lists.md): trascinamento per aggiornare, aggancio e liste lunghe.
- [Esecuzione e debug](../debugging.md): osserva ogni aggiornamento con `-trace`.
