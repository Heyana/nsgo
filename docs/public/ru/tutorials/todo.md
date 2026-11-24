---
title: "Пример: список дел"
description: Создайте небольшое приложение со списком дел шаг за шагом — с сигналами, списком и тестом.
group: Начало работы
order: 11
---

В этом руководстве создаётся небольшое приложение со списком дел: добавляйте задачи, отмечайте их выполненными и удаляйте. Это займёт около десяти минут; предполагается, что вы выполнили [установку](../getting-started.md).

## Создание приложения

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Сборка списка дел

Сгенерированное приложение — это счётчик. Замените всё содержимое `app.go` следующим приложением:

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

Сохраните файл, и приложение перезапустится. Введите задачу, нажмите **Add** и отметьте её выполненной.

## Как это работает

**Состояние — это сигналы.** `items`, `draft` и `nextID` являются сигналами. Сигнал хранит значение и отслеживает, кто его читает: при изменении `items` обновляются только текст и строки, которые его читают. `core.NewSignal` сравнивает значения с помощью `==`, поэтому для среза используется `core.NewSignalFunc` без функции сравнения.

**Обновления заменяют срез.** `items.Update` возвращает новый срез, поэтому то, что остаётся на экране, определяют идентичность списка и ключи строк. `nextID` выдаёт каждой задаче ID, и `Key` у `core.List` использует его; без стабильного ключа строки повторно использовали бы свои ячейки для других элементов.

**Экран — это дерево блоков.** `Root` возвращает `core.View` с внутренними отступами, промежутком и отступами безопасной области. `FlexGrow: 1` и `FlexShrink: 1` заставляют его заполнять окно, но не больше, поэтому длинный список прокручивается внутри него, а не растягивает его за край окна. Строка ввода задаёт `Direction: core.Row`; всё остальное — это колонка по умолчанию.

**Поле ввода контролируемое.** `draft.Get` — значение поля, а `draft.Set` получает каждое изменение. `OnSubmit` добавляет задачу при нажатии Return, а `add` очищает черновик, так что поле очищается само.

**Список создаёт только видимое.** `core.List` запрашивает `Count` строк и создаёт строку, только когда она приближается к экрану. `Empty` отображается, пока элементов нет, а флажок и кнопка удаления каждой строки несут ID своей задачи, поэтому нажатие находит нужный элемент и после изменения списка.

**Внешний вид — от `ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` и `ui.IconButton` берут цвета из темы и автоматически следуют светлому и тёмному режиму. `ui.ColorsOf(cx)` даёт цвета для элементов, которые приложение рисует само, а `StyleFunc` на месте следует за сменой темы.

## Тестирование

Сгенерированный тест нажимает кнопку счётчика, поэтому замените `app_test.go` тестом для списка дел:

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

Тест запускает приложение без интерфейса (headless), находит элементы по тексту или тестовому ID и действует так, как действовал бы пользователь. Запустите его командой:

```sh
go tool nsgo test
```

`TestMain` отдаёт платформе главный поток, когда тесты выполняются на целевой платформе, а `go tool nsgo test -target ios_simulator` запускает тот же тест в симуляторе с его нативным вводом.

## Публикация

```sh
go tool nsgo build
```

`build` записывает релизный пакет для каждой целевой платформы, перечисленной в `nsgo.yaml`, в `dist/<name>-<version>-<target>`: `.app`, `.ipa`, `.apk`, каталоги для Windows и Linux или веб-сайт. Всё, что предназначено для распространения, также попадает в `dist/releases`: образ диска, программа установки, пакеты Debian и Arch, IPA, APK, сайт, мини-программы и скрипты, устанавливающие приложение из ваших релизов на GitHub. В разделе [Сборка и распространение](../building.md) описаны установщики, подпись и нотаризация, публикация, значки, виджеты, App Clips и App Store.

## Далее

- [Пример: приложение для заметок](notes.md) и [Пример: популярные репозитории GitHub](repos.md): ещё два приложения, шаг за шагом.
- [Компоненты и состояние](../components.md): подробнее об элементах, компонентах, сигналах и регионах.
- [Раскладка](../layout.md) и [Стили и темы](../styles.md): структура и внешний вид экрана.
- [Списки и прокрутка](../lists.md): длинные списки, закреплённые заголовки и догрузка.
- [Тестирование](../testing.md): поиск элементов управления, нажатие на них и снимки экрана.
- [Встраивание](../embedding.md): размещение приложения внутри другого приложения.
