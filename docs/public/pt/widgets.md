---
title: Widgets
description: Mostre as views do aplicativo na Tela Inicial e na Tela Bloqueada de iPhones e iPads, e na mesa do Mac.
group: Recursos de plataforma
order: 47
---

## Um tipo de widget

Widgets mostram um resumo do aplicativo, esteja ele em execução ou não: na Tela Inicial e na Tela Bloqueada de iPhones e iPads, na mesa do Mac e na Central de Notificações, e no launcher do Android. Um tipo é um widget que a galeria do sistema oferece, com as famílias em que ele vem:

```yaml
widgets:
  - kind: counter
    name: Counter                 # shown in the system's gallery of widgets
    description: Shows the count.
    families: [small, medium, circular]  # also large, extra_large, rectangular and inline
# apple: { app_group: group.dev.example.hello }  # the shared App Group, group.<id> by default
```

Nas plataformas Apple, uma extensão de widget o desenha, que `build` e `dev` compilam com o compilador Swift do Xcode ([Compilação e distribuição](building.md#widgets-and-live-activities)). No Android, o runtime Java pré-compilado desenha a mesma linha do tempo como App Widgets. Ambos continuam mostrando o conteúdo publicado quando o aplicativo para.

Para uma compilação da Mac App Store com sandbox, defina `apple.app_group` e forneça perfis de provisionamento do macOS por meio de `sign.macos_profile` e `sign.widgets_profile`. Tanto o aplicativo quanto `<id>.widgets` devem pertencer à mesma equipe e ter o entitlement desse grupo. As linhas do tempo e as imagens ficam no contêiner compartilhado dele; compilações diretas e de desenvolvimento do macOS mantêm sua localização existente no diretório pessoal. [Assinatura para lojas](building.md#store-packages) descreve as opções de compilação.

`name` e `description` podem nomear mensagens nos catálogos do aplicativo. Quando o fazem, a galeria de widgets e o placeholder usam o idioma do usuário, recorrendo ao texto do catálogo de origem quando falta uma tradução. Um valor que não é uma chave de catálogo é mostrado literalmente.

## Uma linha do tempo

`widget.Update` grava o que os widgets de um tipo mostram de agora em diante, como uma linha do tempo de entradas: o sistema mostra a entrada cuja data passou por último e, depois, cada uma das seguintes na sua data, sem o aplicativo. Uma `Date` zero é agora. Sem entradas, os widgets mostram o nome e a descrição do tipo. Nas plataformas Apple, um aplicativo em primeiro plano atualiza seus widgets com a frequência que quiser, e um em segundo plano algumas dezenas de vezes por dia.

```go
err := widget.Update("counter", widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Count", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Text{Content: strconv.Itoa(n), Style: widget.Style{FontSize: 44, FontWeight: core.FontWeightBold}},
	}},
})
```

`Entry.Families` mostra a uma família uma view própria no lugar de `Entry.View`, `Entry.Background` preenche o widget atrás dela, além das margens que o sistema mantém, e `Entry.URL` é aberta com o aplicativo quando o widget é tocado. `widget.Update` relata `widget.ErrNoWidgets` em um aplicativo compilado sem eles, `widget.ErrUnsupported` fora de macOS, iOS e Android, e `widget.Available` diz qual antes de uma gravação. Ele pode ser executado em qualquer goroutine, inclusive antes de `nsgo.Run`.

## Views

As views de um widget não são os elementos de core: elas fazem layout como o SwiftUI faz, uma view tão grande quanto seu conteúdo, a menos que `Style` a dimensione ou a faça preencher, e um contêiner centralizando o que contém, a menos que `AlignItems` e `Justify` digam o contrário. `Text`, `Symbol` e `Image`, de bytes ou de um arquivo que `Update` lê, mostram conteúdo; `Column`, `Row`, `Stack`, `Spacer` e `Divider` o organizam; `Rectangle`, `Progress` e `Gauge` desenham; e `Date` mostra um horário com os formatos do sistema, que `Relative`, `Offset` e `Timer` continuam contando por conta própria, sem uma entrada para cada segundo.

```go
widget.Entry{
	View: widget.Column{Style: widget.Style{Gap: 4}, Children: []widget.View{
		widget.Text{Content: "Next hour in", Style: widget.Style{TextStyle: widget.Caption, Color: widget.Secondary}},
		widget.Date{Time: hour, Format: widget.Timer, Style: widget.Style{TextStyle: widget.Title2, FontVariant: core.FontTabularNums}},
		widget.Progress{Value: 0.4, Style: widget.Style{Color: widget.Accent}},
	}},
}
```

`Style` aceita um espaçamento, padding, um tamanho ou preenchimento, alinhamento, um fundo com cantos arredondados, os estilos de texto do sistema ou um tamanho, peso, design e variante de fonte, limites de linhas, um `Link` que é aberto com o aplicativo e `Accented`, que coloca uma view entre as que o sistema colore. `widget.Primary`, `Secondary` e `Accent` seguem a aparência, e `RGB` e `RGBA` aceitam outra cor no modo escuro com `Dark`.

## Botões

`widget.Button` executa `widget.OnAction` a partir do iOS 17 e do macOS 14, sem abrir o aplicativo. Sistemas mais antigos mostram apenas seus filhos. Registre o handler em `main`, antes de `nsgo.Run`: o sistema pode iniciar o aplicativo em segundo plano sem suas views. Um toque em widget tem `Activity == nil`; seu `Action` é o valor do botão.

```go
widget.Button{Action: "refresh", Children: []widget.View{
    widget.Text{Content: "Refresh"},
}}
```

Trate a ação e chame `widget.Update` antes que o handler retorne, para que o recarregamento automático da linha do tempo pelo sistema veja o resultado. O handler é executado fora da goroutine das views; use `core.Scope.Post` para alterar as views. Isso usa o mesmo handler e intent que as [Live Activities](live-activities.md) e precisa apenas de `widgets` em `nsgo.yaml`, sem `live_activities`.

## Android

Adicione o widget pelo seletor de widgets do launcher e redimensione-o ali. O renderizador seleciona uma família permitida a partir de seus limites: small abaixo de 240 pontos em ambas as dimensões, medium a partir de 240 na horizontal, large a partir de 240 na vertical e extra-large a partir de 500 na horizontal; se essa família não estiver listada, ele usa a primeira família listada. `Entry.URL` e `Style.Link` abrem o aplicativo por meio de um esquema que ele declara em `url_schemes`. Um link em uma view aninhada tem precedência sobre o link do seu contêiner.

Entradas futuras da linha do tempo usam os alarmes inexatos do Android, que o sistema pode atrasar enquanto o aplicativo está em segundo plano. As views de timer usam cronômetros nativos e avançam sem o aplicativo; atualmente elas respeitam tamanho e cor, e peso de fonte, design e itálico personalizados ainda são objetivos. As datas relativas são atualizadas no cronograma do sistema. O renderizador genérico oferece suporte a texto, imagens, layout, formas, barras, anéis e um conjunto de glifos de símbolos comuns; use `Image` para artes que precisam ser iguais nas duas plataformas. Nomes arbitrários de SF Symbols continuam sendo um objetivo em aberto no Android. Os provedores de widgets do Android são gerados para APKs e AABs independentes; hosts incorporados não recebem nenhum.

## Live Activities

A mesma extensão desenha as Live Activities do aplicativo na Tela Bloqueada e na Dynamic Island ([Live Activities](live-activities.md)).

## Próximos passos

- [Live Activities](live-activities.md): o que o aplicativo acompanha enquanto acontece.
- [Notificações](notifications.md): as notificações e os badges ao lado de um widget.
- [Compilação e distribuição](building.md): o app group que a extensão compartilha.
