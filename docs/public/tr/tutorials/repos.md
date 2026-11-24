---
title: "Örnek: Popüler GitHub depoları"
description: GitHub API'sinden en çok yıldız alan Go depolarını listeleyin; bir goroutine üzerinde yükleme göstergesi, hatalar ve yeniden deneme ile yükleyin.
group: Başlarken
order: 13
---

Bu eğitim, GitHub'da en çok yıldız alan Go depolarını listeleyen bir uygulama oluşturur. Bir sunucuyla konuşan her uygulamanın ihtiyaç duyduklarını kapsar: ekranı engellemeyen bir istek, bir yükleme göstergesi, yeniden denemeli bir hata ve ağ olmadan çalışan bir test. [Kurulum](../getting-started.md) adımlarının tamamlandığını varsayar.

## Uygulamayı oluşturun

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Uygulamayı yazın

`app.go` dosyasının tamamını şununla değiştirin:

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

İstek sürerken yükleme göstergesi görünür, ardından liste dolar.

## Nasıl çalışır

**Go'nun kendi ağ yetenekleri.** `net/http` her hedefte sistemin TLS ve proxy ayarlarıyla çalışır; bu yüzden `fetch` sıradan Go kodudur.

**Motorun goroutine'inin dışında.** Bileşenler ve sinyaller motorun goroutine'ine aittir ve bu goroutine asla ağı beklememelidir. `core.NewResource`, `fetch`'i kendi goroutine'inde çalıştırır ve sonucu motora geri getirir. Kaynağı nil olduğundan bir kez yükler; **Try again** ise yeniden yüklemek için `Refetch` çağırır. Yalnızca en son yükleme geçerlidir: ekrandan ayrılmak isteği `ctx` üzerinden iptal eder ve geç gelen bir yanıt asla daha yenisinin yerini almaz.

**Sinyal olarak durum.** `repos.Get`, `repos.Err`, `repos.State` ve `repos.Loading` sinyaldir; bu yüzden onları okuyan bağlamalar her yüklemeyi izler. `core.Show` yükleme sürerken yükleme göstergesini, ardından ya **Try again** ile birlikte hatayı ya da listeyi gösterir ve koşulu değiştiğinde yalnızca o kısmı yeniden oluşturur. Bir yeniden deneme, yenisi gelene kadar son listeyi korur.

**Her uzunlukta bir liste.** `core.List` yalnızca ekrana yakın satırları, deponun adıyla anahtarlanmış olarak oluşturur; böylece yeni bir sonuç aynı depoların satırlarını yeniden kullanır.

## Test edin

İsteğin URL'si bir değişkendir; bu yüzden bir test onu `httptest` ile yerel bir sunucuya yönlendirir ve "ağın" ne yanıt vereceğini denetler:

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

`WaitFor`, yanıt gelip ekran onu gösterene kadar bekler.

## Sonraki adımlar

- [Platform hizmetleri](../services.md#networking): ağ, depolama ve dosyalar hakkında daha fazlası.
- [Listeler ve kaydırma](../lists.md): çekerek yenileme, yaslanma ve uzun listeler.
- [Çalıştırma ve hata ayıklama](../debugging.md): her güncellemeyi `-trace` ile izleyin.
