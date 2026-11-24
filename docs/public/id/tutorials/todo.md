---
title: "Contoh: Aplikasi todo"
description: Bangun aplikasi todo kecil langkah demi langkah, dengan signal, daftar, dan pengujian.
group: Mulai
order: 11
---

Tutorial ini membangun aplikasi todo kecil: tambahkan item, centang, dan hapus. Tutorial ini memakan waktu sekitar sepuluh menit dan mengasumsikan [penyiapan](../getting-started.md) sudah dilakukan.

## Membuat aplikasi

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Membangun aplikasi todo

Aplikasi yang dihasilkan adalah penghitung. Ganti seluruh isi `app.go` dengan aplikasi ini:

```go program height=480
package main

import (
	"fmt"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type todo struct {
	id   int
	text string
	done bool
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	items := core.NewSignalFunc(cx, []todo{}, nil)
	draft := core.NewSignal(cx, "")
	nextID := core.NewSignal(cx, 1)
	colors := ui.ColorsOf(cx)

	add := func() {
		text := strings.TrimSpace(draft.Get())
		if text == "" {
			return
		}
		id := nextID.Get()
		nextID.Set(id + 1)
		items.Update(func(list []todo) []todo {
			return append(list, todo{id: id, text: text})
		})
		draft.Set("")
	}
	setDone := func(id int, done bool) {
		items.Update(func(list []todo) []todo {
			next := append([]todo(nil), list...)
			for i := range next {
				if next[i].id == id {
					next[i].done = done
				}
			}
			return next
		})
	}
	remove := func(id int) {
		items.Update(func(list []todo) []todo {
			out := list[:0:0]
			for _, it := range list {
				if it.id != id {
					out = append(out, it)
				}
			}
			return out
		})
	}
	left := func() int {
		n := 0
		for _, it := range items.Get() {
			if !it.done {
				n++
			}
		}
		return n
	}

	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(24), PaddingEnv: core.EnvAll(core.EnvSafeArea), Gap: 16},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background()}
		},
		Children: []core.Node{
			ui.Text("Todos", 28, ui.Bold, colors.Text),
			core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Input{Value: draft.Get, OnChange: draft.Set, Placeholder: "What needs doing?",
					OnSubmit: func(string) { add() }, TestID: "draft", Style: core.Style{FlexGrow: 1}},
				ui.Button{Text: "Add", Icon: lucide.Plus, OnPress: add, TestID: "add"},
			}},
			ui.Textf(func() string { return fmt.Sprintf("%d left", left()) }, 13, ui.Regular, colors.Muted),
			core.List{
				Count:         func() int { return len(items.Get()) },
				Key:           func(i int) any { return items.Get()[i].id },
				EstimatedSize: 44,
				Style:         core.Style{FlexGrow: 1},
				Empty:         ui.Text("Nothing yet. Add one above.", 13, ui.Regular, colors.Muted),
				Row: func(cx *core.Scope, row core.ListRow) core.Node {
					index := row.Index()
					it := items.Get()[index]
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, Padding: core.Symmetric(8, 0)},
						Children: []core.Node{
							ui.Checkbox{
								Checked: func() bool {
									for _, x := range items.Get() {
										if x.id == it.id {
											return x.done
										}
									}
									return false
								},
								OnChange: func(done bool) { setDone(it.id, done) },
								Text:     it.text,
								TestID:   fmt.Sprintf("todo-%d", it.id),
								Style:    core.Style{FlexGrow: 1, FlexShrink: 1},
							},
							ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
								TestID: fmt.Sprintf("delete-%d", it.id), OnPress: func() { remove(it.id) }},
						}}
				},
			},
		},
	}
}
```

Simpan file tersebut dan aplikasi akan dimulai ulang. Ketik sebuah tugas, tekan **Add**, lalu centang tugas itu.

## Cara kerjanya

**State adalah signal.** `items`, `draft`, dan `nextID` adalah signal. Signal menyimpan sebuah nilai dan melacak apa saja yang membacanya: saat `items` berubah, hanya teks dan baris yang membacanya yang diperbarui. `core.NewSignal` membandingkan nilai dengan `==`, sehingga slice menggunakan `core.NewSignalFunc` tanpa fungsi kesetaraan.

**Pembaruan mengganti slice.** `items.Update` mengembalikan slice baru, sehingga identitas daftar dan key baris yang menentukan apa yang tetap di layar. `nextID` memberi setiap todo sebuah ID, dan `Key` milik `core.List` menggunakannya; tanpa key yang stabil, baris akan menggunakan kembali sel-selnya untuk item lain.

**Layar adalah pohon kotak.** `Root` mengembalikan sebuah `core.View` dengan padding, gap, dan inset safe area. `FlexGrow: 1` dan `FlexShrink: 1` membuatnya mengisi jendela dan tidak lebih, sehingga daftar yang panjang bergulir di dalamnya alih-alih melebarkannya melewati tepi jendela. Baris input mengatur `Direction: core.Row`; semua yang lain menggunakan kolom bawaan.

**Input dikendalikan.** `draft.Get` adalah nilai field dan `draft.Set` menerima setiap perubahan. `OnSubmit` menambahkan tugas saat Return ditekan, dan `add` mengosongkan draft, sehingga field mengosongkan dirinya sendiri.

**Daftar hanya membangun yang tampil.** `core.List` meminta `Count` baris dan membangun sebuah baris hanya saat baris itu mendekati layar. `Empty` tampil selama belum ada item, dan checkbox serta tombol hapus di setiap baris membawa ID todo-nya, sehingga sebuah tekanan tetap menemukan item yang benar setelah daftar berubah.

**Tampilannya milik `ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox`, dan `ui.IconButton` mengambil warnanya dari tema dan mengikuti mode terang dan gelap secara otomatis. `ui.ColorsOf(cx)` memberikan warna untuk bagian-bagian yang digambar aplikasi sendiri, dan `StyleFunc` mengikuti perubahan tema di tempat.

## Mengujinya

Pengujian yang dihasilkan menekan tombol penghitung, jadi ganti `app_test.go` dengan pengujian untuk aplikasi todo:

```go
package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestAddCompleteAndDelete(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:Nothing yet. Add one above.")
	app.Type("id:draft", "Buy milk")
	app.Press("id:add")
	app.WaitFor("text:Buy milk", time.Second)
	app.Press("id:todo-1")
	app.WaitFor("text:0 left", time.Second)
	app.Press("id:delete-1")
	app.WaitFor("text:Nothing yet. Add one above.", time.Second)
}
```

Pengujian memulai aplikasi tanpa tampilan (*headless*), menemukan elemen berdasarkan teks atau test ID-nya, dan bertindak seperti yang dilakukan pengguna. Jalankan dengan:

```sh
go tool nsgo test
```

`TestMain` memberikan thread utama kepada platform saat pengujian berjalan di sebuah target, dan `go tool nsgo test -target ios_simulator` menjalankan pengujian yang sama di simulator dengan input native-nya.

## Merilisnya

```sh
go tool nsgo build
```

`build` menulis paket rilis untuk setiap target yang tercantum di `nsgo.yaml`, ke dalam `dist/<name>-<version>-<target>`: sebuah `.app`, sebuah `.ipa`, sebuah `.apk`, direktori Windows dan Linux, atau sebuah situs web. Semua yang akan dibagikan juga ditempatkan di `dist/releases`: image disk, program setup, paket Debian dan Arch, IPA, APK, situs, mini program, dan skrip yang memasang aplikasi dari rilis GitHub Anda. [Membangun dan mendistribusikan](../building.md) membahas installer, penandatanganan dan notarisasi, publikasi, ikon, widget, App Clip, dan App Store.

## Selanjutnya

- [Contoh: Aplikasi catatan](notes.md) dan [Contoh: Repositori GitHub populer](repos.md): dua aplikasi lagi, langkah demi langkah.
- [Komponen dan state](../components.md): elemen, komponen, signal, dan region secara lebih mendalam.
- [Tata letak](../layout.md) dan [Gaya dan tema](../styles.md): struktur dan tampilan layar.
- [Daftar dan pengguliran](../lists.md): daftar panjang, header lengket, dan memuat lebih banyak.
- [Pengujian](../testing.md): temukan kontrol, tekan, dan ambil tangkapan layar.
- [Penyematan](../embedding.md): tempatkan aplikasi di dalam aplikasi lain.
