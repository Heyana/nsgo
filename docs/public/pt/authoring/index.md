---
title: Escrevendo componentes
description: Crie componentes para que outras pessoas usem em seus aplicativos, da mesma forma que os pacotes base e ui do NSGo são feitos.
group: Escrevendo componentes
order: 70
---

Estas páginas são para autores de componentes: uma biblioteca de controles, um design system ou uma visualização complexa que aplicativos importam como um pacote Go. Elas pressupõem o conhecimento de [Componentes e estado](../components.md) e [Estilos e temas](../styles.md).

## Um componente

Um componente é um tipo com um método `Render`, como em um aplicativo. Seus campos são o que o aplicativo define; tudo o que o aplicativo não define assume o valor zero, portanto o valor zero de cada campo deve ser o caso mais comum.

```go
package chips

import "github.com/nsgo-dev/nsgo/core"

// Chip is a short label in a pill.
type Chip struct {
	Text string
}

// Render implements core.Node.
func (c Chip) Render(*core.Scope) core.Node {
	return core.View{Class: "self-start rounded-full px-3 py-1", Children: []core.Node{
		core.Text{Content: c.Text, Class: "text-sm"},
	}}
}
```

Um aplicativo o utiliza como qualquer elemento: `chips.Chip{Text: "New"}`.

## Comportamento e visual

O NSGo mantém cada controle em duas versões. `base` guarda seu comportamento, estado e acessibilidade sem visual e recebe todos os estilos do aplicativo parte por parte; `ui` coloca um visual sobre a forma de `base`, de modo que o comportamento reside em um só lugar. Uma biblioteca pode seguir a mesma divisão:

- Um componente **sem estilo** controla os pressionamentos, o teclado, o foco e o que os leitores de tela ouvem, e expõe o estilo de cada parte, como faz `base.Switch`. Aplicativos e outras bibliotecas dão a ele o seu próprio visual ([Partes e estados](parts.md)).
- Um componente **estilizado** envolve um componente sem estilo e apenas escolhe estilos a partir do tema em vigor, como faz `ui.Switch` ([Visuais e temas](looks.md)).

Um componente sem comportamento que valha a pena compartilhar, como um badge, pode existir apenas na forma estilizada.

## Um pacote

- Mantenha um componente por arquivo, nomeado como ele (`chip.go`), com os tipos que apenas ele usa; o que vários compartilham ganha um arquivo próprio.
- Dê a um componente complexo um pacote próprio, cuja API omite o nome do componente, como `dock.Area` em `base/dock`.
- Coloque os textos que um componente exibe no `locales/` do pacote ([Textos, testes e publicação](shipping.md)).
- Não faça nada implicitamente por dispositivo: um tamanho ou formato que muda em um telefone ou desktop deve ser um campo definido pelo aplicativo.

## Próximos passos

- [Partes e estados](parts.md): valores controlados, partes estilizadas e acessibilidade.
- [Visuais e temas](looks.md): cores do tema, variantes e tamanhos.
- [Componentes sem estilo](base.md): os controles próprios do NSGo sem visual, para estilizar ou usar como base.
- [Textos, testes e publicação](shipping.md): traduções, testes e TSX.
