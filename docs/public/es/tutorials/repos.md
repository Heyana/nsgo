---
title: "Ejemplo: repositorios populares de GitHub"
description: Lista los repositorios de Go con más estrellas desde la API de GitHub, cargando en una goroutine con un indicador de carga, errores y reintento.
group: Primeros pasos
order: 13
---

Este tutorial construye una aplicación que lista los repositorios de Go con más estrellas en GitHub. Cubre lo que toda aplicación que se comunica con un servidor necesita: una solicitud que no bloquea la pantalla, un indicador de carga, un error con reintento y una prueba que se ejecuta sin red. Asume la [configuración](../getting-started.md).

## Crea la aplicación

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Escribe la aplicación

Reemplaza todo el contenido de `app.go`:

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

El indicador de carga se muestra mientras la solicitud está en curso, y luego la lista se rellena.

## Cómo funciona

**La propia red de Go.** `net/http` funciona en todos los destinos, con la configuración de TLS y proxy del sistema, por lo que `fetch` es Go puro.

**Fuera de la goroutine del motor.** Los componentes y las señales pertenecen a la goroutine del motor, que nunca debe esperar a la red. `core.NewResource` ejecuta `fetch` en una goroutine propia y trae el resultado de vuelta al motor. Su origen es nil, por lo que carga una vez, y **Try again** llama a `Refetch` para cargar de nuevo. Solo cuenta la última carga: salir de la pantalla cancela la solicitud mediante `ctx`, y una respuesta tardía nunca reemplaza a una más reciente.

**Estado como señales.** `repos.Get`, `repos.Err`, `repos.State` y `repos.Loading` son señales, por lo que los enlaces que las leen siguen cada carga. `core.Show` muestra el indicador de carga mientras carga, y o bien el error con **Try again** o la lista, reconstruyendo solo esa parte cuando cambia su condición. Un reintento mantiene la última lista hasta que llega la nueva.

**Una lista de cualquier longitud.** `core.List` construye solo las filas cercanas a la pantalla, con clave por el nombre del repositorio, de modo que un nuevo resultado reutiliza las filas de los mismos repositorios.

## Pruébala

La URL de la solicitud es una variable, por lo que una prueba la apunta a un servidor local con `httptest` y controla lo que responde la "red":

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

// serve responde a las solicitudes de la aplicación con body en lugar de GitHub.
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

`WaitFor` espera hasta que la respuesta haya llegado y la pantalla la muestre.

## Siguiente

- [Servicios de plataforma](../services.md#redes): más sobre red, almacenamiento y archivos.
- [Listas y desplazamiento](../lists.md): arrastrar para actualizar, ajuste (snapping) y listas largas.
- [Ejecución y depuración](../debugging.md): observa cada actualización con `-trace`.
