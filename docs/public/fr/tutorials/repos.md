---
title: "Exemple : dépôts GitHub populaires"
description: Lister les dépôts Go les plus étoilés depuis l’API de GitHub, avec chargement dans une goroutine, indicateur d’activité, gestion des erreurs et nouvel essai.
group: Démarrer
order: 13
---

Ce tutoriel permet de créer une application qui liste les dépôts Go les plus étoilés sur GitHub. Il couvre tout ce dont a besoin une application qui dialogue avec un serveur : une requête qui ne bloque pas l’écran, un indicateur de chargement, une gestion des erreurs avec nouvel essai et un test qui s’exécute sans réseau. Il suppose que l’[installation](../getting-started.md) est effectuée.

## Créer l’application

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Écrire l’application

Remplacez tout le contenu de `app.go` :

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

L’indicateur de chargement s’affiche pendant l’exécution de la requête, puis la liste se remplit.

## Comment ça fonctionne

**La pile réseau propre à Go.** `net/http` fonctionne sur chaque cible, avec les paramètres TLS et proxy du système ; `fetch` est donc écrit en Go standard.

**Hors de la goroutine du moteur.** Les composants et les signaux appartiennent à la goroutine du moteur, qui ne doit jamais attendre le réseau. `core.NewResource` exécute `fetch` dans sa propre goroutine et rapporte le résultat au moteur. Sa source est nil, il se charge donc une seule fois, et **Try again** appelle `Refetch` pour relancer le chargement. Seul le dernier chargement compte : quitter l’écran annule la requête via `ctx`, et une réponse tardive ne remplace jamais une réponse plus récente.

**L’état sous forme de signaux.** `repos.Get`, `repos.Err`, `repos.State` et `repos.Loading` sont des signaux, de sorte que les liaisons qui les lisent suivent chaque chargement. `core.Show` affiche l’indicateur de chargement pendant la requête, puis soit l’erreur avec **Try again**, soit la liste, en ne reconstruisant que cette partie lorsque sa condition change. Un nouvel essai conserve la dernière liste jusqu’à l’arrivée de la nouvelle.

**Une liste de n’importe quelle longueur.** `core.List` ne construit que les lignes proches de l’écran, indexées par le nom du dépôt, de sorte qu’un nouveau résultat réutilise les lignes des mêmes dépôts.

## La tester

L’URL de la requête est une variable ; un test peut donc la rediriger vers un serveur local avec `httptest` et contrôler ce que répond le « réseau » :

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

`WaitFor` attend que la réponse soit arrivée et qu’elle s’affiche à l’écran.

## Suite

- [Services plateforme](../services.md#networking) : plus d’informations sur le réseau, le stockage et les fichiers.
- [Listes et défilement](../lists.md) : actualisation par glissement, aimantation et longues listes.
- [Exécution et débogage](../debugging.md) : observer chaque mise à jour avec `-trace`.
