---
title: "Contoh: Repositori GitHub populer"
description: Tampilkan repositori Go dengan bintang terbanyak dari API GitHub, dimuat di goroutine dengan spinner, error, dan coba lagi.
group: Mulai
order: 13
---

Tutorial ini membangun aplikasi yang menampilkan repositori Go dengan bintang terbanyak di GitHub. Tutorial ini mencakup hal yang dibutuhkan setiap aplikasi yang berkomunikasi dengan server: permintaan yang tidak memblokir layar, indikator pemuatan, error dengan tombol coba lagi, dan pengujian yang berjalan tanpa jaringan. Tutorial ini mengasumsikan [penyiapan](../getting-started.md) sudah dilakukan.

## Membuat aplikasi

```sh
mkdir repos && cd repos
go mod init example.com/repos
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Menulis aplikasi

Ganti seluruh isi `app.go`:

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

Spinner tampil selama permintaan berjalan, lalu daftar terisi.

## Cara kerjanya

**Jaringan bawaan Go.** `net/http` bekerja di setiap target, dengan pengaturan TLS dan proxy milik sistem, sehingga `fetch` cukup berupa Go biasa.

**Di luar goroutine engine.** Komponen dan signal milik goroutine engine, yang tidak boleh menunggu jaringan. `core.NewResource` menjalankan `fetch` di goroutine tersendiri dan membawa hasilnya kembali ke engine. Sumbernya nil, sehingga ia memuat satu kali, dan **Try again** memanggil `Refetch` untuk memuat lagi. Hanya pemuatan terakhir yang berlaku: meninggalkan layar membatalkan permintaan melalui `ctx`, dan jawaban yang terlambat tidak pernah menggantikan jawaban yang lebih baru.

**State sebagai signal.** `repos.Get`, `repos.Err`, `repos.State`, dan `repos.Loading` adalah signal, sehingga binding yang membacanya mengikuti setiap pemuatan. `core.Show` menampilkan spinner selama pemuatan, lalu error dengan **Try again** atau daftarnya, dan hanya membangun ulang bagian itu saat kondisinya berubah. Saat mencoba lagi, daftar terakhir tetap ditampilkan sampai daftar baru tiba.

**Daftar dengan panjang berapa pun.** `core.List` hanya membangun baris yang dekat dengan layar, dengan key berupa nama repositori, sehingga hasil baru menggunakan kembali baris dari repositori yang sama.

## Mengujinya

URL permintaan adalah sebuah variabel, sehingga pengujian dapat mengarahkannya ke server lokal dengan `httptest` dan mengendalikan apa yang dijawab oleh "jaringan":

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

`WaitFor` menunggu sampai jawaban tiba dan layar menampilkannya.

## Selanjutnya

- [Layanan platform](../services.md#jaringan): lebih lanjut tentang jaringan, penyimpanan, dan file.
- [Daftar dan pengguliran](../lists.md): tarik untuk menyegarkan, snapping, dan daftar panjang.
- [Menjalankan dan debugging](../debugging.md): pantau setiap pembaruan dengan `-trace`.
