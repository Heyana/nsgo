---
title: Arrastar e soltar
description: Transfira valores da aplicação entre views em uma mesma janela.
group: Conceitos principais
order: 57
---

`base/dragdrop` conecta uma origem e um destino na mesma `Area`, em uma única janela. O aplicativo mantém suas views, estilos, seleção e a ação que trata o valor. Uma origem começa após quatro pontos de movimento por padrão, deixando um toque comum intacto.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` é executado uma vez quando o arrasto começa. Retorne um slice para carregar vários itens selecionados juntos; copie dados mutáveis ali caso precisem permanecer um instantâneo. Um `Data` nulo desativa o arrasto. `Threshold` define o movimento necessário. `Activation: dragdrop.AfterLongPress` aguarda primeiro `View.DelayLongPress` (500 ms por padrão) e depois o movimento; escolha-o explicitamente onde um deslize deva rolar primeiro.

`View` mantém suas props comuns, incluindo manipuladores de toque e acessibilidade. Uma origem é dona de `View.Responder`; deixe-o sem definição. Seus filhos podem incluir um botão com sua ação normal. `Preview` acompanha o ponteiro sem capturar entrada nem remontar a origem; sem ele, o rótulo da origem é exibido. Dê à origem um rótulo ou uma pré-visualização. Áreas aninhadas operam de forma independente.

Os destinos precisam de `OnDrop` para aceitar um valor. O destino mais interno tem a primeira chance; quando `Accept` o rejeita, um ancestral que aceite pode recebê-lo. `OnEnter`, `OnMove` e `OnLeave` podem atualizar a aparência do destino. Cada evento carrega `Data`, `Position` no destino e `PagePosition` na superfície. Desativar ou remover o destino ativo limpa seu estado de entrada; soltar verifica a aceitação novamente. Um destino atrás de outra view, recortado, desativado ou abaixo de um modal não pode receber a soltura.

`OnStart` e `OnEnd` descrevem a sessão da origem. `OnEnd` recebe `Result{Data, Dropped, Target}`. Escape, cancelamento nativo de toque, desativar ou remover a origem, ou soltar fora de um destino que aceite encerram sem soltar. O posicionamento do destino usa a última geometria confirmada; animações nativas entre commits não são amostradas. Isso transfere valores dentro da janela; arquivos do sistema continuam usando `OnFileDrop`.

Mantenha um menu, comando de teclado ou ação acessível que realize a mesma operação. Arrastar é um atalho para essa ação. Os testes usam `app.DragTo("id:poster", "id:playlist")`, ou `DragTo` de uma superfície selecionada; testes TSX usam `await app.dragTo("id:poster", "id:playlist")`. Ambas as views já precisam caber na janela visível; o passo envia uma sequência de toques através do responder sem sintetizar uma soltura nem rolar durante o arrasto. Veja [Testes](testing.md).
