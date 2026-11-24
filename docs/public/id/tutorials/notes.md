---
title: "Contoh: Aplikasi catatan"
description: Bangun aplikasi catatan dengan tumpukan navigasi, editor multibaris, dan catatan yang disimpan di perangkat.
group: Mulai
order: 12
---

Tutorial ini membangun aplikasi catatan: daftar catatan, halaman untuk menyunting satu catatan, dan catatan yang masih ada saat aplikasi dijalankan lagi. Tutorial ini mengasumsikan [penyiapan](../getting-started.md) sudah dilakukan.

## Membuat aplikasi

```sh
mkdir notes && cd notes
go mod init example.com/notes
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
	"encoding/json"
	"strings"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

type note struct {
	ID   int    `json:"id"`
	Text string `json:"text"`
}

// title is a note's first line.
func (n note) title() string {
	line, _, _ := strings.Cut(strings.TrimSpace(n.Text), "\n")
	if line == "" {
		return "New note"
	}
	return line
}

func main() { nsgo.Run(Root) }

func Root(cx *core.Scope) core.Node {
	store := cx.Storage()
	var saved []note
	if data, ok := store.GetItem("notes"); ok {
		json.Unmarshal([]byte(data), &saved)
	}
	notes := core.NewSignalFunc(cx, saved, nil)
	save := func(list []note) {
		notes.Set(list)
		data, _ := json.Marshal(list)
		store.SetItem("notes", string(data))
	}

	// A route is the ID of the note it edits, and 0 the list.
	nav := base.NewNav(cx, 0)

	find := func(id int) note {
		for _, n := range notes.Get() {
			if n.ID == id {
				return n
			}
		}
		return note{ID: id}
	}
	add := func() {
		id := 1
		for _, n := range notes.Get() {
			id = max(id, n.ID+1)
		}
		save(append([]note{{ID: id}}, notes.Get()...))
		nav.Push(id)
	}
	edit := func(id int, text string) {
		list := append([]note(nil), notes.Get()...)
		for i := range list {
			if list[i].ID == id {
				list[i].Text = text
			}
		}
		save(list)
	}
	remove := func(id int) {
		var list []note
		for _, n := range notes.Get() {
			if n.ID != id {
				list = append(list, n)
			}
		}
		save(list)
		nav.Pop(1)
	}

	return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, id int, _ func() int) base.NavPage {
		if id == 0 {
			return listPage(cx, notes.Get, nav.Push, add)
		}
		return ui.NavPage(cx, base.NavPage{
			TitleFunc: func() string { return find(id).title() },
			Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, Padding: core.All(16)}, Children: []core.Node{
				ui.Input{
					Multiline:   true,
					Value:       func() string { return find(id).Text },
					OnChange:    func(text string) { edit(id, text) },
					Placeholder: "Write something",
					TestID:      "editor",
					Style:       core.Style{FlexGrow: 1, FlexShrink: 1},
				},
			}}},
		}, ui.NavHeader{
			OnBack:    func() { nav.Pop(1) },
			BackLabel: "Notes",
			Trailing: []core.Node{ui.IconButton{Icon: lucide.Trash, Label: "Delete", Variant: ui.IconButtonGhost,
				TestID: "delete", OnPress: func() { remove(id) }}},
		})
	})
}

func listPage(cx *core.Scope, notes func() []note, open func(int), add func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	return ui.NavPage(cx, base.NavPage{
		Title: "Notes",
		Children: []core.Node{core.List{
			Count:         func() int { return len(notes()) },
			Key:           func(i int) any { return notes()[i].ID },
			EstimatedSize: 48,
			Style:         core.Style{FlexGrow: 1},
			Empty: core.View{Style: core.Style{Padding: core.All(16)}, Children: []core.Node{
				ui.Text("No notes yet.", 13, ui.Regular, colors.Muted),
			}},
			Row: func(cx *core.Scope, row core.ListRow) core.Node {
				n := notes()[row.Index()]
				return core.View{
					Role:    core.RoleButton,
					OnPress: func(core.PressEvent) { open(n.ID) },
					Style:   core.Style{Padding: core.Symmetric(12, 16), Borders: core.Edges{Bottom: 1}},
					StyleFunc: func() core.Style {
						return core.Style{BorderColor: colors.Border()}
					},
					Children: []core.Node{ui.Text(n.title(), 15, ui.Regular, colors.Text)},
				}
			},
		}},
	}, ui.NavHeader{
		Trailing: []core.Node{ui.IconButton{Icon: lucide.Plus, Label: "New note", Variant: ui.IconButtonGhost,
			TestID: "new", OnPress: add}},
	})
}
```

Tekan **+** untuk menambahkan catatan lalu ketik; baris pertamanya menjadi judulnya di daftar. Keluar dari aplikasi lalu jalankan lagi: catatannya masih ada.

## Cara kerjanya

**Tumpukan halaman.** `base.NewNav(cx, 0)` menyimpan tumpukan rute, di sini berupa ID catatan dengan `0` untuk daftar. `nav.Stack` membangun halaman untuk setiap rute dan tetap memasang halaman-halaman di bawahnya, sehingga daftar mempertahankan posisi gulirnya selama sebuah catatan terbuka. `Push` membuka catatan dan `Pop` kembali.

**Halaman dengan header.** `ui.NavPage` memberi halaman latar belakang `ui` dan sebuah `ui.NavHeader` dengan judul, tombol kembali, dan tombol-tombolnya. Di iOS dan Android, sistem yang menggambar header, dengan gestur kembali dan transisinya sendiri; di tempat lain `ui` yang menggambarnya.

**Penyimpanan di perangkat.** `cx.Storage()` adalah penyimpanan key-value milik aplikasi. `Root` membaca catatan satu kali saat mulai, dan `save` menuliskannya sebagai JSON setiap kali berubah. Penyimpanan ini menyimpan string; file yang ditulis aplikasi masuk ke `cx.DataDir()`.

**Satu sumber kebenaran.** Setiap halaman membaca signal `notes` yang sama. `Value` milik editor membaca catatan, `OnChange` menyimpannya, dan `TitleFunc` milik header serta baris di daftar mengikuti signal yang sama, sehingga semuanya diperbarui selagi Anda mengetik.

## Mengujinya

Ganti `app_test.go`. `nsgotest.Storage` memulai aplikasi dengan penyimpanan yang sudah berisi data, seolah-olah aplikasi pernah dijalankan sebelumnya:

```go
package main

import (
	"strings"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestWriteANote(t *testing.T) {
	app := nsgotest.Start(t, Root)
	app.Find("text:No notes yet.")
	app.Press("id:new")
	app.Type("id:editor", "Groceries\nMilk")
	app.WaitFor("role:header Groceries", time.Second)
	if data, _ := app.Storage().GetItem("notes"); !strings.Contains(data, "Milk") {
		t.Fatalf("saved %q", data)
	}
}

func TestSavedNotes(t *testing.T) {
	app := nsgotest.Start(t, Root, nsgotest.Storage(map[string]string{
		"notes": `[{"id":1,"text":"Ideas\nA notes app"}]`,
	}))
	app.Press("text:Ideas")
	app.Press("id:delete")
	app.WaitFor("text:No notes yet.", time.Second)
}
```

```sh
go tool nsgo test
```

## Selanjutnya

- [Contoh: Repositori GitHub populer](repos.md): mengambil data dari jaringan tanpa memblokir layar.
- [Navigasi](../navigation.md): modal, menu, dan tab bar.
- [Layanan platform](../services.md): file, berbagi, clipboard, dan lainnya.
