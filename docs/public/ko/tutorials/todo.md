---
title: "예제: 할 일 앱"
description: 시그널, 리스트, 테스트를 사용해 작은 할 일 앱을 단계별로 만들어 봅니다.
group: 시작하기
order: 11
---

이 튜토리얼에서는 항목을 추가하고, 완료 체크하고, 삭제할 수 있는 작은 할 일 앱을 만듭니다. 약 10분 정도 소요되며 [설치 및 환경 설정](../getting-started.md)이 완료되어 있다고 가정합니다.

## 앱 만들기

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## 할 일 앱 구현하기

생성된 앱은 카운터입니다. `app.go` 전체를 다음 앱 코드로 교체하세요:

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

파일을 저장하면 앱이 다시 시작됩니다. 할 일을 입력하고 **Add**를 누른 뒤 체크 표시로 완료해 보세요.

## 동작 방식

**상태는 시그널입니다.** `items`, `draft`, `nextID`는 시그널입니다. 시그널은 값을 보관하고 이를 읽는 대상을 추적하므로 `items`가 변경될 때 이를 읽는 텍스트와 행만 업데이트됩니다. `core.NewSignal`은 `==`로 값을 비교하므로 슬라이스에는 동등 비교 함수 없이 `core.NewSignalFunc`를 사용합니다.

**업데이트는 슬라이스를 교체합니다.** `items.Update`는 새 슬라이스를 반환하므로 리스트의 식별성과 행 키가 화면에 무엇이 유지될지를 결정합니다. `nextID`는 각 할 일에 ID를 부여하고 `core.List`의 `Key`가 이를 사용합니다. 안정적인 키가 없으면 행이 다른 항목에 대해 기존 셀을 재사용하게 됩니다.

**화면은 상자들의 트리입니다.** `Root`는 패딩, 간격(gap), 안전 영역(safe area) 인셋이 지정된 `core.View`를 반환합니다. `FlexGrow: 1`과 `FlexShrink: 1`은 뷰가 창을 넘치지 않고 딱 맞게 채우도록 하므로, 긴 리스트가 창 가장자리를 넘어 늘어나지 않고 내부에서 스크롤됩니다. 입력 행은 `Direction: core.Row`를 설정하며, 그 외의 모든 요소는 기본값인 세로 방향(column)입니다.

**입력 필드는 제어(controlled) 모드입니다.** `draft.Get`이 필드의 값이며 `draft.Set`이 각 변경 사항을 수신합니다. Return 키를 누르면 `OnSubmit`이 할 일을 추가하고, `add`가 `draft`를 비우므로 입력 필드가 자동으로 비워집니다.

**리스트는 보이는 부분만 빌드합니다.** `core.List`는 `Count`만큼의 행 개수를 요청하고 화면 근처에 올 때만 행을 빌드합니다. 항목이 없을 때는 `Empty`가 표시되며, 각 행의 체크박스와 삭제 버튼이 해당 할 일의 ID를 가지고 있으므로 리스트가 변경된 뒤에도 누를 때 올바른 항목을 찾을 수 있습니다.

**외관은 `ui`가 담당합니다.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox`, `ui.IconButton`은 테마에서 색상을 가져오며 라이트 모드와 다크 모드를 자동으로 따릅니다. `ui.ColorsOf(cx)`는 앱이 직접 그리는 부분의 색상을 제공하며, `StyleFunc`는 테마 변경을 그 자리에서 바로 반영합니다.

## 테스트하기

생성된 테스트는 카운터 버튼을 누르므로, `app_test.go`를 할 일 앱 테스트로 교체하세요:

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

이 테스트는 헤드리스로 앱을 시작하고 텍스트나 테스트 ID로 엘리먼트를 찾은 뒤 사용자처럼 동작합니다. 다음 명령으로 실행하세요:

```sh
go tool nsgo test
```

`TestMain`은 테스트가 대상 플랫폼에서 실행될 때 플랫폼에 메인 스레드를 제공하며, `go tool nsgo test -target ios_simulator`는 시뮬레이터에서 네이티브 입력과 함께 동일한 테스트를 실행합니다.

## 배포하기

```sh
go tool nsgo build
```

`build`는 `nsgo.yaml`에 나열된 각 대상 플랫폼에 대한 릴리스 패키지를 `dist/<name>-<version>-<target>`에 생성합니다(`.app`, `.ipa`, `.apk`, Windows 및 Linux 디렉터리, 또는 웹 사이트). 배포용 산출물 일체는 `dist/releases`에도 저장되며, 여기에는 디스크 이미지, 설치 프로그램, Debian 및 Arch 패키지, IPA, APK, 웹 사이트, 미니 프로그램, 그리고 GitHub 릴리스에서 앱을 설치하는 스크립트가 포함됩니다. 설치 프로그램, 서명 및 공증(notarization), 게시, 아이콘, 위젯, App Clips, App Store에 대한 내용은 [빌드 및 배포](../building.md)에서 다룹니다.

## 다음 단계

- [예제: 메모 앱](notes.md) 및 [예제: 인기 GitHub 저장소](repos.md): 단계별로 살펴보는 두 개의 추가 앱.
- [컴포넌트와 상태](../components.md): 엘리먼트, 컴포넌트, 시그널, 리전(region)에 대한 심화 설명.
- [레이아웃](../layout.md) 및 [스타일과 테마](../styles.md): 화면의 구조와 외관.
- [리스트와 스크롤](../lists.md): 긴 리스트, 고정 헤더(sticky header), 추가 로딩.
- [테스팅](../testing.md): 컨트롤 찾기, 누르기, 스크린샷 찍기.
- [임베딩](../embedding.md): 다른 앱 안에 앱 넣기.
