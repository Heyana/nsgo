---
title: Introdução
description: O que é o NSGo, onde ele roda e como funciona.
order: 1
---

O NSGo é um framework de UI nativa multiplataforma escrito em Go. Você escreve um aplicativo uma vez, em Go ou em TSX, e ele roda no iOS e em seus App Clips, tvOS, watchOS, Android, macOS, Windows, Linux e na web, desenhado pelas visualizações próprias de cada plataforma. Aplicativos em Go também rodam, de forma experimental, nos mini programas do WeChat, QQ, Alipay e Douyin: as plataformas limitam um aplicativo a 2 MB, o que aplicativos maiores ultrapassam. O HarmonyOS roda aplicativos em Go como prévia, desenhados pelo ArkUI.

## O que você escreve

Um aplicativo é um pacote main em Go. Seus componentes retornam elementos: visualizações, texto, imagens, entradas e os controles de `ui`. O estado vive em signals, e só o que lê um signal é atualizado quando ele muda.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

O mesmo aplicativo pode ser escrito em TSX e estilizado com classes do Tailwind ([aplicativos TSX](tsx.md)).

## Como funciona

Um motor em Go é dono do aplicativo: o grafo reativo, os estilos, o layout flexbox e grid e a preparação do texto. Cada atualização produz um lote de mudanças, e um backend fino o aplica às visualizações nativas da plataforma. A animação roda no relógio de quadros da plataforma, então nada é confirmado a cada quadro.

| Plataforma | Desenha com |
| --- | --- |
| iOS | UIKit e CoreText |
| Android | Android Views |
| macOS | AppKit e CoreText |
| Windows | Direct2D e DirectWrite |
| Linux | GTK 4, Pango e Cairo |
| Web | WebAssembly e elementos DOM |

Todos os alvos compilam em um Mac com Go; o iOS também precisa do Xcode. A ferramenta `nsgo` cria, executa, testa e empacota aplicativos.

## Status

O NSGo está no início: a 0.1.0 é sua primeira versão, sua API ainda não é estável, e o que falta está no [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Ele porta o design do [Native Shell](https://github.com/longbridge/nsh) para Go.

## Como ler a documentação

Cada página responde a uma pergunta. Um exemplo de código é um fragmento de um componente, a menos que a página diga o contrário: `cx` é o `*core.Scope` que um componente recebe. Os aplicativos usam os componentes de `ui`, que têm uma aparência; `base` guarda os mesmos controles sem aparência, para autores de componentes.

- **Primeiros passos**: [configuração](getting-started.md) e, em seguida, aplicativos de exemplo construídos passo a passo, [um aplicativo de tarefas](tutorials/todo.md), [um aplicativo de notas](tutorials/notes.md) e [repositórios populares do GitHub](tutorials/repos.md).
- **Ferramentas**: [execução e depuração](debugging.md), [testes](testing.md), [compilação e distribuição](building.md) e [incorporação](embedding.md).
- **Componentes**: [os componentes mais usados](ui.md), suas variantes, estados e acessibilidade.
- **Conceitos principais**: [componentes e estado](components.md), [layout](layout.md), [estilos](styles.md), [texto](text.md), [listas](lists.md), [navegação](navigation.md), [formulários](forms.md), [localização](localization.md), [TSX](tsx.md) e o restante.
- **Recursos de plataforma**: [serviços da plataforma](services.md), [notificações](notifications.md), [resposta tátil](haptics.md), [widgets](widgets.md), [Live Activities](live-activities.md) e [permissões](permissions.md).
- **Escrevendo componentes**: [componentes para os aplicativos de outras pessoas](authoring/index.md), sem estilo e com estilo.
- **Contribuir**: [compile o NSGo a partir do código-fonte](contributing.md), oriente-se nele e envie uma alteração.
- A [demonstração ao vivo](https://nsgo.dev/gallery/): todos os componentes, no seu navegador.
