---
title: "Örnek: Bir not uygulaması"
description: Bir gezinme yığını, çok satırlı bir düzenleyici ve cihazda saklanan notlarla bir not uygulaması oluşturun.
group: Başlarken
order: 12
---

Bu eğitim bir not uygulaması oluşturur: bir not listesi, bir notu düzenleyen bir sayfa ve uygulama yeniden başladığında hâlâ yerinde duran notlar. [Kurulum](../getting-started.md) adımlarının tamamlandığını varsayar.

## Uygulamayı oluşturun

```sh
mkdir notes && cd notes
go mod init example.com/notes
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

Bir not eklemek için **+** düğmesine basın ve yazın; notun ilk satırı listedeki başlığı olur. Uygulamadan çıkıp yeniden başlatın: notlar hâlâ oradadır.

## Nasıl çalışır

**Bir sayfa yığını.** `base.NewNav(cx, 0)` bir rota yığını tutar; burada rotalar not kimlikleridir ve `0` listeyi temsil eder. `nav.Stack` her rota için bir sayfa oluşturur ve alttaki sayfaları bağlı tutar; böylece bir not açıkken liste kaydırma konumunu korur. `Push` bir notu açar, `Pop` ise geri döner.

**Başlıklı sayfalar.** `ui.NavPage` bir sayfaya `ui`'ın arka planını ve başlığı, geri düğmesi ve düğmeleriyle bir `ui.NavHeader` verir. iOS ve Android'de başlığı sistem kendi geri hareketi ve geçişiyle çizer; diğer yerlerde onu `ui` çizer.

**Cihazda depolama.** `cx.Storage()` uygulamanın anahtar-değer deposudur. `Root` notları başlarken bir kez okur, `save` ise notlar her değiştiğinde onları JSON olarak yazar. Depolama dizeleri tutar; uygulamanın yazdığı dosyalar `cx.DataDir()` içine gider.

**Tek doğruluk kaynağı.** Her sayfa aynı `notes` sinyalini okur. Düzenleyicinin `Value` alanı notu okur, `OnChange` onu kaydeder; başlığın `TitleFunc`'ı ve listenin satırı da aynı sinyali izler, bu yüzden siz yazdıkça güncellenirler.

## Test edin

`app_test.go` dosyasını değiştirin. `nsgotest.Storage`, uygulamayı daha önce çalışmış gibi önceden yazılmış bir depolamayla başlatır:

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

## Sonraki adımlar

- [Örnek: Popüler GitHub depoları](repos.md): ekranı engellemeden ağdan veri çekin.
- [Gezinme](../navigation.md): kipli pencereler, menüler ve sekme çubukları.
- [Platform hizmetleri](../services.md): dosyalar, paylaşım, pano ve daha fazlası.
