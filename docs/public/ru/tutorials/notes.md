---
title: "Пример: приложение для заметок"
description: Создайте приложение для заметок со стеком навигации, многострочным редактором и заметками, сохраняемыми на устройстве.
group: Начало работы
order: 12
---

В этом руководстве создаётся приложение для заметок: список заметок, страница для редактирования одной из них и заметки, которые остаются на месте при повторном запуске приложения. Предполагается, что вы выполнили [установку](../getting-started.md).

## Создание приложения

```sh
mkdir notes && cd notes
go mod init example.com/notes
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

Нажмите **+**, чтобы добавить заметку, и начните печатать; её первая строка становится заголовком в списке. Закройте приложение и запустите его снова: заметки по-прежнему на месте.

## Как это работает

**Стек страниц.** `base.NewNav(cx, 0)` хранит стек маршрутов — здесь это ID заметок, где `0` обозначает список. `nav.Stack` создаёт страницу для каждого маршрута и оставляет нижележащие страницы смонтированными, поэтому список сохраняет позицию прокрутки, пока открыта заметка. `Push` открывает заметку, а `Pop` возвращает назад.

**Страницы с заголовком.** `ui.NavPage` даёт странице фон `ui` и `ui.NavHeader` с заголовком, кнопкой «Назад» и другими кнопками. На iOS и Android заголовок рисует система, со своим жестом возврата и переходом; на остальных платформах его рисует `ui`.

**Хранилище на устройстве.** `cx.Storage()` — это хранилище «ключ — значение» приложения. `Root` читает заметки один раз при запуске, а `save` записывает их в виде JSON при каждом изменении. Хранилище содержит строки; файлы, которые записывает приложение, помещаются в `cx.DataDir()`.

**Единый источник истины.** Все страницы читают один и тот же сигнал `notes`. `Value` редактора читает заметку, `OnChange` сохраняет её, а `TitleFunc` заголовка и строка списка следуют за тем же сигналом, поэтому они обновляются по мере ввода.

## Тестирование

Замените `app_test.go`. `nsgotest.Storage` запускает приложение с уже записанным хранилищем, как будто оно уже запускалось раньше:

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

## Далее

- [Пример: популярные репозитории GitHub](repos.md): загрузка данных из сети без блокировки экрана.
- [Навигация](../navigation.md): модальные окна, меню и панели вкладок.
- [Сервисы платформы](../services.md): файлы, отправка через «Поделиться», буфер обмена и многое другое.
