---
title: Dialog
description: Uma superfície modal, uma sheet e um alerta.
group: Componentes
order: 41
---

`ui.Dialog` é uma superfície modal sobre o aplicativo escurecido, uma sheet vinda de baixo em telas sensíveis ao toque. `ui.Sheet` é um painel vindo de baixo, e `ui.AlertDialog` pede ao usuário que confirme.

```go preview=dialog/basic height=360
open := core.NewSignal(cx, false)
ui.Dialog{
	Open:         open.Get,
	OnOpenChange: func(v bool, _ base.Reason) { open.Set(v) },
	Title:        "Rename the file",
	Description:  "The new name shows everywhere.",
	Children:     []core.Node{ui.Input{Value: name.Get, OnChange: name.Set}},
	Footer: []core.Node{
		ui.Button{Text: "Cancel", Variant: ui.ButtonSecondary, OnPress: func() { open.Set(false) }},
		ui.Button{Text: "Save", OnPress: save},
	},
}
```

## Abrir e fechar

O aplicativo guarda se o diálogo está aberto. `OnOpenChange` ouve cada mudança que o diálogo pede, com um `base.Reason` que distingue um toque fora de Escape ou voltar; `OnClose` roda assim que ele fecha. `NoCloseOnEscape` e `NoCloseOnBackdropPress` impedem que Escape e um toque fora o fechem.

`Title` e `Description` encabeçam a superfície, `TitleFunc` e `DescriptionFunc` quando mudam enquanto ela está aberta, e `Footer` guarda seus botões. `Bare` deixa a superfície inteira para `Children`, para um diálogo com o layout próprio do aplicativo, que os desktops recortam dentro da borda da superfície nos cantos que `Style` e `StyleFunc` lhe dão, e `Inset` define sua distância mínima das bordas da janela nos desktops.

Em sheets de toque, `Bare` agora remove o grabber padrão e o padding do conteúdo para que `Children` preencham o painel. `NoGrabber` oculta apenas o grabber, `ContentStyle` e `ContentStyleFunc` personalizam o contêiner do conteúdo, e `TitleStyle` e `TitleStyleFunc` personalizam o título. `Leading` e `Trailing` colocam nós, como os botões Cancelar e Concluído, nas extremidades da linha do título, com o título centralizado entre eles. `StartingStyle` e `EndingStyle` personalizam a transição da sheet; inclua uma `Transition` neles quando precisar de um tempo personalizado.

`Style` é mesclado sobre a aparência da superfície. `StyleFunc` é mesclado depois dele e segue os signals que lê, incluindo a aparência, enquanto o diálogo está aberto e quando é reaberto. Por exemplo, `StyleFunc: func() core.Style { return core.Style{Background: colors.Surface()} }` segue o tema, onde `colors := ui.ColorsOf(cx)`. Ambos os campos também funcionam em `ui.AlertDialog`. Uma cor zero mantém a cor existente; use uma cor transparente não zero, como `core.RGBA(0xFFFFFF, 0)`, para limpá-la.

## Sheets

`ui.Sheet` é um painel na parte de baixo sobre um fundo escurecido; um toque no fundo, Escape ou voltar chama `OnClose`.

Defina `Label` ou `LabelFunc` para o nome acessível do seu diálogo, e `TestID` para encontrar o painel nos testes. `LabelFunc` segue seus signals sem substituir o conteúdo aberto. `CloseOnSwipe: true` também chama `OnClose` depois que um deslize para baixo passa o limite de fechamento; atualize `Open` nesse callback para fechá-la.

Defina `Style` ou `StyleFunc` para personalizar o fundo, os raios dos cantos e o padding da superfície, mantendo o grabber e o tratamento da área segura. `StyleFunc` segue os signals que lê. Um `CornerRadius` sem `CornerRadii` arredonda todos os cantos no lugar do topo arredondado padrão.

```go preview=dialog/sheet height=320
ui.Sheet{Open: filters.Get, OnClose: func() { filters.Set(false) }, Children: filterControls}
```

## Alertas

`ui.AlertDialog` pede ao usuário que confirme ou reconheça algo. Cada ação o fecha, e um toque fora não.

```go preview=dialog/alert height=320
ui.AlertDialog{
	Open:         confirm.Get,
	OnOpenChange: func(v bool, _ base.Reason) { confirm.Set(v) },
	Title:        "Delete the note?",
	Description:  "This cannot be undone.",
	Actions: []ui.AlertAction{
		{Text: "Cancel", Variant: ui.ButtonSecondary, Role: core.AlertCancel},
		{Text: "Delete", Color: colors.Danger, Role: core.AlertDestructive, Default: true, OnPress: remove},
	},
}
```

Uma ação com `Role: core.AlertCancel` roda com Escape ou voltar, seguida de `OnOpenChange(false, base.ReasonCancel)`. `Default: true` dá a uma ação o foco do teclado quando o alerta abre nos desktops, na web e na Apple TV, e faz Return escolhê-la; o toque em uma ação roda `OnPress` e depois `OnOpenChange(false, base.ReasonSelect)`. Return roda a ação padrão enquanto o foco do teclado não está em nenhuma outra ação ou campo: com o foco movido para outra ação, Return pressiona essa, e um diálogo sobre o alerta fica com o Return. Depois que uma ação roda, o alerta fecha, faça o que fizer `OnOpenChange`, e não roda nenhuma outra até que `Open` passe a falso e a verdadeiro de novo. Se várias ações tiverem o mesmo marcador, a primeira vence. Sem marcadores, Escape fecha sem rodar nenhuma ação e Return não tem ação padrão. `Role: core.AlertDestructive` descreve uma ação; `Variant` ou `Color` escolhe sua aparência.

Para o alerta ou a action sheet do próprio sistema, use `cx.ShowAlert` ([Serviços da plataforma](../services.md#alerts-and-the-clipboard)).

## Comportamento

Um modal bloqueia o aplicativo atrás dele, toma o foco do teclado e fecha com Escape e voltar, a menos que seja instruído do contrário. Em telas sensíveis ao toque, um diálogo é uma sheet que segue um arrasto para baixo e fecha após um limite.

## Acessibilidade

`ui.Dialog` relata `RoleDialog`, ou seu `Role` explícito, e `ui.AlertDialog` relata `RoleAlertDialog` nos desktops e telas sensíveis ao toque. Ambos leem seu título, ou `Label` quando o título não basta, seguido da descrição. O foco entra em um diálogo quando ele abre e volta quando ele fecha; o fundo não é uma parada. Os botões do rodapé mantêm seus papéis e rótulos.

## Plataforma

No Android e em telas sensíveis ao toque, um diálogo é uma sheet vinda de baixo; nos desktops, é uma superfície centralizada com um fundo. O conteúdo é o mesmo; só a apresentação muda.

## Relacionados

- [Navegação](../navigation.md): páginas, abas e modais juntos.
- [Menu](menu.md): comandos ancorados e menus de contexto.
- [Acessibilidade](../accessibility.md): foco e papéis de diálogo.
- As páginas Dialog, Sheet e AlertDialog da galeria.
- [Componentes sem estilo](../authoring/base.md#dialog): `base.Dialog` e `base.Sheet`, com as apresentações do iOS e um movimento próprio.
