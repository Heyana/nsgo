---
title: "Exemplo: um app de tarefas"
description: Crie passo a passo um pequeno app de tarefas, com signals, uma lista e um teste.
group: Primeiros passos
order: 11
---

Este tutorial cria um pequeno app de tarefas: adicione itens, marque-os como concluídos e exclua-os. Leva cerca de dez minutos e pressupõe a [configuração](../getting-started.md).

## Crie o app

```sh
mkdir todo && cd todo
go mod init example.com/todo
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
go tool nsgo dev
```

## Construa o app de tarefas

O app gerado é um contador. Substitua todo o conteúdo de `app.go` por este app:

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

Salve o arquivo e o app reinicia. Digite uma tarefa, pressione **Add** e marque-a como concluída.

## Como funciona

**O estado é feito de signals.** `items`, `draft` e `nextID` são signals. Um signal guarda um valor e rastreia o que o lê: quando `items` muda, apenas o texto e as linhas que o leem são atualizados. `core.NewSignal` compara valores com `==`, então uma slice usa `core.NewSignalFunc` sem função de igualdade.

**As atualizações substituem a slice.** `items.Update` retorna uma nova slice, então a identidade da lista e as chaves das linhas decidem o que permanece na tela. `nextID` dá um ID a cada tarefa, e o `Key` de `core.List` o usa; sem uma chave estável, as linhas reaproveitariam suas células para outros itens.

**A tela é uma árvore de caixas.** `Root` retorna um `core.View` com padding, um espaçamento e as margens da área segura. `FlexGrow: 1` e `FlexShrink: 1` fazem com que ele preencha a janela e nada além disso, de modo que uma lista longa rola dentro dele em vez de esticá-lo além da borda da janela. A linha de entrada define `Direction: core.Row`; todo o resto usa a coluna padrão.

**A entrada é controlada.** `draft.Get` é o valor do campo e `draft.Set` recebe cada alteração. `OnSubmit` adiciona uma tarefa quando Return é pressionado, e `add` limpa o rascunho, de modo que o campo se esvazia sozinho.

**A lista só constrói o que aparece.** `core.List` pede `Count` linhas e constrói uma linha apenas quando ela se aproxima da tela. `Empty` aparece enquanto não há itens, e a caixa de seleção e o botão de excluir de cada linha carregam o ID de sua tarefa, de modo que um toque ainda encontra o item certo depois que a lista muda.

**A aparência é a do `ui`.** `ui.Text`, `ui.Input`, `ui.Button`, `ui.Checkbox` e `ui.IconButton` obtêm suas cores do tema e seguem o modo claro e o escuro automaticamente. `ui.ColorsOf(cx)` fornece as cores para as partes que o próprio app desenha, e um `StyleFunc` acompanha uma troca de tema no lugar.

## Teste

O teste gerado pressiona o botão do contador, então substitua `app_test.go` por um teste do app de tarefas:

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

O teste inicia o app sem interface gráfica, encontra elementos pelo texto ou pelo ID de teste e age como um usuário agiria. Execute-o com:

```sh
go tool nsgo test
```

`TestMain` entrega a thread principal à plataforma quando os testes rodam em um alvo, e `go tool nsgo test -target ios_simulator` executa o mesmo teste no simulador com sua entrada nativa.

## Distribua

```sh
go tool nsgo build
```

`build` grava um pacote de release para cada alvo listado em `nsgo.yaml`, em `dist/<name>-<version>-<target>`: um `.app`, um `.ipa`, um `.apk`, os diretórios do Windows e do Linux, ou um site. Tudo o que é para distribuir também vai para `dist/releases`: a imagem de disco, o programa de instalação, os pacotes Debian e Arch, IPAs, APKs, o site, mini programas e scripts que instalam o app a partir das suas releases no GitHub. [Compilação e distribuição](../building.md) aborda instaladores, assinatura e notarização, publicação, ícones, widgets, App Clips e a App Store.

## Próximos passos

- [Exemplo: um app de notas](notes.md) e [Exemplo: repositórios populares do GitHub](repos.md): mais dois apps, passo a passo.
- [Componentes e estado](../components.md): elementos, componentes, signals e regiões com mais profundidade.
- [Layout](../layout.md) e [Estilos e temas](../styles.md): a estrutura e a aparência da tela.
- [Listas e rolagem](../lists.md): listas longas, cabeçalhos fixos e carregamento de mais itens.
- [Testes](../testing.md): encontre controles, pressione-os e capture telas.
- [Incorporação](../embedding.md): coloque o app dentro de outro app.
