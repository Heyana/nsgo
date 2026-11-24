---
title: "예제: 메모 앱"
description: 내비게이션 스택, 여러 줄 편집기, 기기에 저장되는 메모를 갖춘 메모 앱을 만듭니다.
group: 시작하기
order: 12
---

이 튜토리얼에서는 메모 목록, 메모를 편집하는 페이지, 그리고 앱을 다시 시작해도 유지되는 메모를 갖춘 메모 앱을 만듭니다. 먼저 [설치 및 준비](../getting-started.md)를 완료했다고 가정합니다.

## 앱 만들기

```sh
mkdir notes && cd notes
go mod init example.com/notes
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## 앱 작성하기

`app.go`의 전체 내용을 다음 코드로 바꿉니다:

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

**+** 버튼을 눌러 메모를 추가하고 내용을 입력해 보세요. 첫 번째 줄이 목록의 제목이 됩니다. 앱을 종료한 뒤 다시 시작해도 메모가 그대로 남아 있습니다.

## 동작 방식

**페이지 스택.** `base.NewNav(cx, 0)`은 라우트 스택을 유지하며, 여기서는 메모 ID를 사용하고 `0`은 목록을 나타냅니다. `nav.Stack`은 각 라우트에 대한 페이지를 생성하고 아래에 있는 페이지들을 마운트된 상태로 유지하므로, 메모가 열려 있는 동안에도 목록의 스크롤 위치가 유지됩니다. `Push`는 메모를 열고 `Pop`은 이전 화면으로 돌아갑니다.

**헤더가 있는 페이지.** `ui.NavPage`는 페이지에 `ui`의 배경을 적용하고 제목, 뒤로 가기 버튼, 추가 버튼들이 포함된 `ui.NavHeader`를 제공합니다. iOS와 Android에서는 시스템이 자체 뒤로 가기 제스처 및 화면 전환 효과와 함께 헤더를 그리며, 그 외의 플랫폼에서는 `ui`가 직접 그립니다.

**기기에 저장.** `cx.Storage()`는 앱의 키-값 저장소입니다. `Root`는 시작할 때 메모를 한 번 읽어 오고, `save`는 메모가 변경될 때마다 JSON 형식으로 저장합니다. 저장소는 문자열을 보관하며, 앱이 직접 쓰는 파일은 `cx.DataDir()`에 저장됩니다.

**단일 진실 공급원.** 모든 페이지는 동일한 `notes` 시그널을 읽습니다. 편집기의 `Value`는 메모를 읽고 `OnChange`는 이를 저장하며, 헤더의 `TitleFunc`와 목록의 행도 같은 시그널을 따르므로 입력하는 즉시 함께 업데이트됩니다.

## 테스트하기

`app_test.go`를 다음 코드로 바꿉니다. `nsgotest.Storage`는 이전에 실행된 적이 있는 것처럼 저장소에 이미 데이터가 기록된 상태로 앱을 시작합니다:

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

## 다음 단계

- [예제: 인기 GitHub 저장소](repos.md): 화면을 멈추지 않고 네트워크에서 데이터 가져오기.
- [내비게이션](../navigation.md): 모달, 메뉴 및 탭 바.
- [플랫폼 서비스](../services.md): 파일, 공유, 클립보드 등.
