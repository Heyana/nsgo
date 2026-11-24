---
title: Estilos e temas
description: Estilos tipados, classes do Tailwind, estilos de estado e temas claro e escuro.
group: Conceitos principais
order: 52
---

## Um estilo

`core.Style` é um único valor Go para o layout, a aparência e o texto de uma caixa. O valor zero de cada campo significa não definido, então os estilos se compõem: `Merge` substitui apenas o que o estilo posterior define, e um filho herda os campos de texto que seus pais definem. Para zerar um comprimento sobre um estilo que o define, use `core.Zero` em `Padding`, `Margin`, `Borders`, `BorderWidth`, `CornerRadius` ou em um gap, assim como `core.Transparent` faz para uma cor: `Padding: core.All(core.Zero)`.

```go
card := core.Style{
	Background: core.RGB(0xffffff),
	CornerRadius: 12,
	Padding: core.All(16),
	Gap: 12,
	BorderWidth: 1,
	BorderColor: core.RGBA(0x000000, 0.1),
}
```

Os estilos aceitam as cores, cantos, bordas, contornos, sombras, gradientes, filtros, desfoques de fundo, máscaras, modos de mesclagem, transformações 3D e transições do CSS. `core.RGB`, `core.RGBA` e `core.ParseColor` criam cores, `core.LinearGradient` e seus semelhantes criam gradientes, `core.Ring` cria o ring do Tailwind, e `core.AllCorners` dá a cada canto seu raio. `BorderCurveContinuous` arredonda os cantos fundindo-os às bordas, como o iOS faz. As sombras e o contorno de uma caixa ficam fora do recorte de seu `Overflow`, exceto onde [Layout](layout.md#position) indica. Os filtros de fundo alteram o que está atrás de uma visualização, como o `backdrop-filter` do CSS: um `Blur` desfoca pelo seu raio e os filtros de cor alteram suas cores. O iOS desenha o material de sistema que `View.IOS.BackdropMaterial` nomeia para um desfoque sem filtros de cor; um desfoque que nomeia um material diferente de `core.BackdropUltraThin` desenha esse material no iOS e o mais próximo do AppKit no macOS, deixando de fora os filtros de cor. Linux e Android não desenham filtros de fundo. `Style.GlassEffect` (`core.GlassRegular` ou `core.GlassClear`, `glassEffect` em TSX) desenha Liquid Glass no lugar de um desfoque de fundo no macOS 26 e no iOS 26 (e o shader de vidro do NSGo no Windows; dê a uma visualização de vidro um desfoque como alternativa nas demais plataformas). Definir `Style.GlassInteractive` (`glassInteractive` em TSX) ativa a resposta interativa de toque do sistema para Liquid Glass no macOS 27 (`NSGlassEffectView.effectIsInteractive`) e no iOS 26 (`UIGlassEffect.isInteractive`), mantendo `OnPress` e os filhos pressionáveis funcionando.

`Transform` move o alvo do ponteiro junto com a visualização: toques, hover e gestos começam onde ela é desenhada, e as posições do ponteiro informadas desfazem a transformação. Um filho transformado fora de seu pai continua alcançável, a menos que um recorte ou uma área de rolagem o esconda. Os mini programas ainda não desfazem keyframes que sua camada de renderização executa ([roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md#mini-programs)).

```go
core.View{Style: core.Style{
	CornerRadii: core.Corners{TopLeft: 28, BottomRight: 28},
	BorderCurve: core.BorderCurveContinuous,
	Shadows:     []core.BoxShadow{core.Ring(1, core.RGBA(0x000000, 0.08)), ui.Shadow(0, 8, 24, -6, ui.Black(0.18))},
}}
```

## Estados

A aparência de uma visualização pode mudar com seu estado. `Hover`, `Focus`, `FocusWithin`, `Active`, `Selected`, `Checked`, `Disabled` e os estados `aria-*` são valores `*Style` que se mesclam sobre o restante enquanto o estado vale. Um app define `Selected`, `Checked`, `Disabled` e `Expanded` por meio de funções, e a plataforma informa toques e foco.

`Active` e `group-active:` acompanham uma visualização enquanto ela está pressionada, o que exige um handler de toque ou um `Responder` que retenha o toque. Estilizar uma visualização simples com `Active`, `active:` ou `group` não a torna pressionável nem uma parada da tecla Tab; defina `Focusable` quando ela mesma deve receber o foco.

```go
core.View{
	OnPress: func(core.PressEvent) { open() },
	Style: core.Style{
		Background: core.RGB(0x007aff),
		Active:     &core.Style{Opacity: core.Alpha(0.85)},
		Hover:      &core.Style{Background: core.RGB(0x006be0)},
	},
	Disabled: func() bool { return saving },
}
```

Uma parte de um componente pode assumir os estados do controle ao seu redor sem informá-los ela mesma: `PartStates` fornece `Checked`, `Selected`, `Disabled` e os demais, de modo que a marca de uma caixa de seleção acompanha a caixa de seleção também para leitores de tela.

## Classes

`Class` em `View`, `Text`, `Span`, `Input`, `Image`, `Scroll` e `Canvas` aceita classes do Tailwind CSS 4, e cada parte de um componente `base` tem uma classe ao lado de seu estilo. Utilitários, variantes e valores arbitrários significam o que significam no Tailwind, com seu tema e paleta padrão.

```go
core.View{Class: "flex items-center gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"}
```

Uma classe se mescla por baixo do estilo, então os campos do estilo prevalecem. As classes podem acompanhar o que muda com `ClassFunc`, e uma variante de estado (`hover:bg-zinc-100`, `checked:opacity-100`) se torna o estado do estilo. `group` e `group-hover:` estilizam filhos a partir do estado de um pai, `first:` e `last:` acompanham a posição de um filho, e `aria-checked:`, `data-open:` e `focus-within:` acompanham os estados que um controle exibe. `pointer-fine:` e `pointer-coarse:` estilizam uma subárvore para mouse ou tela sensível ao toque, correspondendo à media feature `pointer` do CSS via `cx.Env().Pointer()` (a consulta `pointer` do navegador na web, fine em desktops e Macs, coarse em celulares, tablets e relógios, e nenhuma das duas em TVs), e se combinam com outras variantes como `dark:` e `hover:`, para que o app escolha explicitamente o dimensionamento para toque ou ponteiro em vez de o `core` mudá-lo implicitamente.

Marque um controle com `peer` para estilizar irmãos posteriores a partir de seu estado, por exemplo uma caixa de seleção com `Class: "peer"` seguida de um rótulo com `Class: "peer-checked:text-accent peer-disabled:opacity-50"`. Um peer estiliza apenas os irmãos após ele sob o mesmo pai, incluindo `Show` e `Each`, nunca irmãos anteriores nem as visualizações dentro dele. Use `peer/field` com `peer-checked/field:` para nomear um conjunto, e `peer-data-invalid:` para o estado de dados booleano `invalid` de um controle. Marcadores e estados são atualizados reativamente. Marcar uma visualização não a torna uma parada da tecla Tab nem a faz receber toques, então `peer-active:` acompanha apenas um peer que recebe toques por si só, como um botão.

`has-[>svg]:px-3` altera o padding de um pai quando ele tem um ícone como filho direto; `[&_svg]:size-4 [&_svg]:shrink-0` dimensiona seus ícones descendentes. `svg` seleciona `core.Icon` e `SVGView`, inclusive através de regiões condicionais. Esses seletores são atualizados quando filhos, classes ou estados mudam. `hover:[&_svg]` acompanha o hover do pai; `[&_svg]:hover` acompanha o de cada ícone. O `active` de um seletor acompanha apenas visualizações que recebem toques, como um botão, assim como `peer-active:`. Seletores correspondentes competem por especificidade antes da ordem das variantes; estilos inline prevalecem, a menos que a classe seja important.

Os seletores correspondem a elementos do NSGo em todos os alvos. Eles suportam listas, os combinadores de filho (`>`), descendente (`_`), irmão adjacente (`+`) e irmão seguinte (`~`), classes, presença/igualdade de `data-*`, papéis, estados booleanos de acessibilidade suportados e `:is`, `:where`, `:not`, `:has` e posições de filhos. Os tipos são `view`/`div`, `text`/`span` (nós de texto inteiros), `scroll`, `input`, `textarea`, `image`/`img`, `canvas`, `svg`, `video`, `webview` e `native`. Use um único `&` de nível superior em cada alternativa de seletor arbitrário; sem ele, o seletor testa o próprio elemento. Seletores exclusivos do navegador, pseudoelementos, at-rules, escapes e `&` explícito dentro de uma função são desconhecidos; esses seletores não alcançam as visualizações de implementação do backend nem spans de texto individuais.

Variantes `peer-*` em `Span.Class` ou `Screen.HeaderClass` não têm irmãos para acompanhar. Elas são ignoradas e registradas em log uma vez por classe e contexto; coloque a variante peer de um span no `Text` que o contém.

O campo `Tailwind` do tema estende o tema padrão com cores, breakpoints e utilitários, e uma cor que a paleta não tem nomeia um papel de `Palette` (`bg-accent`, `text-text-muted/70`) ou uma cor do tema, lida do tema do nó na aparência que ele exibe.

## Temas

Um `core.Theme` é um visual nas duas aparências: uma `Palette` para o modo claro e outra para o escuro, um raio de canto e famílias de fontes. Os papéis da paleta são `Background`, `Surface`, `Text`, `TextMuted`, `Border`, `Accent`, `AccentText`, `Danger`, `Warning`, `Success`, `Selection`, `Focus` e `Scrollbar`; `Colors` guarda os que uma biblioteca adiciona, como `inset` e `raised`. `core.DefaultTheme()`, que um app exibe até definir outro, é o tema neutro com um destaque azul e os papéis do `ui`, e `ui.WarmDuskTheme()` é uma paleta quente.

```go
theme := core.DefaultTheme()
theme.Light.Accent, theme.Dark.Accent = core.RGB(0xbd481b), core.RGB(0xed986e)
theme.Radius = 10
cx.SetTheme(theme)
```

`cx.SetTheme` dá um tema ao escopo e a tudo o que é construído sob ele, de modo que uma barra lateral ou um painel pode ter um tema próprio; um tema nil assume o de cima. `core.ParseTheme` lê um arquivo de tema YAML ou JSON, ou as variáveis CSS de um arquivo de tema do shadcn/ui, com cores de papéis, um raio e famílias de fontes. `core.DefaultTheme`, o tema que um app exibe até definir um, e `ui.WarmDuskTheme` retornam temas prontos para modificar.

`cx.SetAppearance` força o app inteiro para claro ou escuro, `cx.SetSubtreeAppearance` faz isso para uma subárvore, e `core.AppearanceSystem` volta a seguir o sistema. `cx.Dark()` informa qual aparência uma subárvore exibe, e `cx.Palette()` fornece suas cores com as substituições do host de incorporação aplicadas.

Quando nenhum ancestral define `Style.Color`, o texto e os ícones `currentColor` usam o `Palette.Text` da subárvore, acompanhando no lugar as mudanças de tema, aparência e tokens do host. Uma cor explícita, incluindo `core.Transparent`, continua sendo herdada.

## Cores em componentes

Um componente lê suas cores onde é construído e em seus `StyleFunc`s, de modo que uma mudança de tema ou aparência é acompanhada no lugar, sem construir o componente novamente. `ui.ColorsOf(cx)` nomeia os papéis que o `ui` usa, `ui.TintsOf(cx)` fornece seus preenchimentos translúcidos, e `ui.FinOf(cx)` fornece os acabamentos em camadas do `ui`.

```go
colors := ui.ColorsOf(cx)
core.Text{Content: "Hello", StyleFunc: func() core.Style {
	return core.Style{Color: colors.Text(), FontSize: 15}
}}
```

`StyleFunc` é executado novamente quando o que ele leu muda, então `cx.Dark()` e `cx.Palette()` dentro dele acompanham o tema. O mesmo vale para `ClassFunc`, o `StyleFunc` de um span e os campos `StyleFunc` de um componente.

## Cursores

`Style.Cursor` define um cursor de ponteiro nomeado (`core.CursorText`, `core.CursorColResize` e assim por diante); deixe os elementos pressionáveis em `core.CursorAuto` para que mantenham a seta da plataforma. Para ferramentas de desenho, jogos ou ponteiros personalizados de canvas em desktops e na web, `Style.CursorImage` aceita um `*core.CustomCursor` com uma `Image` 1x, uma `Image2x` HiDPI opcional (escolhida automaticamente quando `PixelRatio >= 1.5`), um `Hotspot` em pontos lógicos limitado às dimensões da imagem e um cursor nomeado de `Fallback`. Consulte `cx.Env().SupportsCustomCursor()` para saber se o alvo atual exibe imagens de cursor personalizadas; plataformas de toque as ignoram.

## Próximos passos

- [Layout](layout.md): onde as caixas ficam.
- [Texto](text.md): fontes, spans e decorações.
- [Animação](animation.md): transições entre estilos.
