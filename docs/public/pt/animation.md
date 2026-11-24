---
title: Animação
description: Transições, valores animados, animação de layout e predefinições de movimento que rodam no relógio de quadros da plataforma.
group: Conceitos principais
order: 58
---

## Valores animados

Uma animação roda na thread principal da plataforma, conduzida pelo próprio relógio de quadros dela, de modo que não custa nada à engine por quadro. Um `AnimatedValue` é um número que a plataforma move; `Style.Animated` vincula a ele a opacidade, translação, escala, rotação, fundo, cor da borda, sombra e cor do texto de uma visualização.

```go
fade := core.NewAnimatedValue(cx, 0)
cx.StartAnimation(core.Timing(fade, 1, core.TimingConfig{
	Duration: 300 * time.Millisecond,
	Easing:   core.EaseOut,
}), nil)

core.View{Style: core.Style{Animated: &core.AnimatedStyle{Opacity: fade}}}
```

`Timing`, `Spring` e `Decay` movem um valor, e `Sequence`, `Parallel`, `Stagger`, `Delay` e `Loop` os compõem. `cx.StartAnimation` retorna um `AnimationRun` cujo `Stop` encerra uma execução onde ela está e `Reset` recoloca seus valores no ponto inicial. Uma execução começa de onde seu valor está, e uma nova animação sobre um valor encerra a anterior.

```go
scale := core.NewAnimatedValue(cx, 1)
ui.Spring(cx, scale, 1.1, core.SpringConfig{Stiffness: 220, Damping: 14})
```

Nós derivados acompanham um valor: `core.Interpolate` mapeia números ou cores com seu próprio easing e extrapolação, e `core.Add`, `Sub`, `Mul`, `Div`, `Mod`, `DiffClamp` e `SnappingDiffClamp` calculam outros valores a partir dele. `ui.Lerp` é uma interpolação curta.

```go
angle := core.Interpolate(scroll, core.Interpolation{
	Input:  []float64{0, 240},
	Output: []float64{0, 90},
	Easing: core.EaseOut,
})
```

`Scope.RequestAnimationFrame` executa uma função uma vez no próximo quadro da tela, e `Scope.Animate` avança por uma duração nesse relógio. Ambos rodam na goroutine do grafo, então podem definir sinais. A função recebe o tempo do quadro — o da própria tela no macOS, iOS, Android, web, Linux, Windows, HarmonyOS e watchOS (no HarmonyOS quando a máquina de compilação tem o SDK do OpenHarmony; um aplicativo empacotado sem ele recebe o relógio da engine) —, de modo que um loop que move elementos pelo tempo entre dois quadros acompanha o ritmo da tela; um teste `nsgotest` fornece o tempo do relógio de teste.

Defina a taxa de callbacks de uma janela com `cx.SetPreferredFrameRate(core.FrameRateRange{Preferred: 30})`. Passe `Minimum` e `Maximum` para fornecer um intervalo ao display link, como `{Minimum: 80, Maximum: 120, Preferred: 120}` em uma tela ProMotion. Chame-o a partir do próprio escopo da janela. A preferência se aplica a `RequestAnimationFrame` e `Animate` até ser alterada. Um intervalo zerado restaura o ritmo normal da tela. O tempo do callback continua sendo o tempo real da tela. A tela e o sistema operacional escolhem a taxa de atualização disponível, e o NSGo pode pular quadros da tela para cadenciar os callbacks.


`Scope.Tween` avança um número nesse relógio para aquilo que um valor animado não alcança, como um número subindo em um texto: ele chama uma função com cada número de um valor a outro, assim como `Timing` moveria um valor, e mais uma vez com o valor final. Um passo de `nsgotest` que o inicia aguarda o valor final, como ocorre em uma animação, e com redução de movimento ativa entrega o valor final imediatamente. `Animated.tween` em TSX funciona da mesma forma.

```go
freed := core.NewSignal(cx, 0.0)
cx.Tween(0, 18.7, core.TimingConfig{Duration: 600 * time.Millisecond, Easing: core.BezierEasing(core.EaseOut)}, freed.Set, nil)

core.Text{ContentFunc: func() string { return fmt.Sprintf("%.1f GB can be freed", freed.Get()) }}
```

## Transições

Uma `Transition` anima o estilo ou as classes de uma visualização quando eles mudam, na plataforma, sem trabalho para a engine por quadro. `Properties` define o que se move; 0 significa todas as propriedades, exceto o quadro (frame).

```go
core.View{Style: core.Style{
	Background:  core.RGB(0xffffff),
	CornerRadius: 12,
	Transition:  core.Transition{Duration: 200 * time.Millisecond, Properties: core.TransitionBackground | core.TransitionTransform},
	Transform:   core.Transform{Scale: 1},
	Hover:       &core.Style{Transform: core.Transform{Scale: 1.02}, Background: core.RGB(0xf5f5f5)},
}}
```

`TransitionLayout` move e redimensiona uma visualização a partir de onde ela aparece quando seu próprio estilo lhe dá um novo quadro. Em vez disso, uma animação de layout anima todo um commit, como faz o `LayoutAnimation` do React Native: `cx.AnimateLayout(core.LayoutEaseInEaseOutPreset)` anima as movimentações, inserções e remoções do próximo lote, e as predefinições cobrem easing, linear e mola (spring). Uma transição `LayoutSpring` recebe `Response` (o período natural não amortecido da mola) e `DampingRatio` (`< 1` subamortecido, `1` criticamente amortecido, `> 1` superamortecido), roda até estabilizar em vez de ter uma duração fixa e carrega sua posição e velocidade para um novo alvo quando redirecionada em pleno movimento.

```go
cx.AnimateLayout(core.LayoutAnimation{
	Update: &core.LayoutTransition{
		Curve:        core.LayoutSpring,
		Response:     280 * time.Millisecond,
		DampingRatio: 0.85,
	},
})
open.Set(!open.Get())
```

`View.OnTransitionEnd` é executado assim que uma alteração conclui sua transição, tenha algo se movido ou não. A redução de movimento desativa transições e animações de layout, portanto respeite a configuração da plataforma mantendo o estado final acessível.

## Movimento

`core.Motion` move juntos a opacidade, translação, escala e rotação de uma visualização por meio de quadros-chave, a partir de um `MotionSpec` do aplicativo ou de um `MotionPreset` pronto: entradas, saídas, destaques de atenção e loops. Vincular seu estilo posiciona a visualização em repouso; `Play` executa uma especificação e `Exit` permite que uma visualização saia com seu movimento dentro de um `core.Presence`.

```go
enter := core.SlideInUp.Spec()
motion := core.NewMotion(cx, &enter)
motion.Exit(core.FadeOut.Spec())

core.View{Style: core.Style{Animated: motion.Style()}, Children: content}
```

`core.Presence{When, Then, Else}` mantém um ramo até que todas as saídas registradas por ele tenham terminado, para que uma folha (sheet) ou um toast saia com seu movimento qualquer que seja o motivo de sua remoção. `core.PresenceOf` aplica o mesmo comportamento a uma chave comparável e uma função de conteúdo, e `MaxLeaving` limita ramos antigos quando um valor muda rapidamente. `MotionPresets()` lista todas as predefinições e `Kind()` informa se ela entra, sai, chama atenção ou faz loop.

## Quadros-chave

`KeyframeAnimation` repete quadros-chave em uma plataforma: opacidade, escala, rotação e translação vertical em cada deslocamento, com duração, atraso, easing e iterações. `KeyframesSpin`, `KeyframesPing`, `KeyframesPulse` e `KeyframesBounce` são os do Tailwind.

```go
core.View{Style: core.Style{Animation: core.KeyframeAnimation{
	Keyframes: core.KeyframesPulse,
	Duration:  2 * time.Second,
	Iterations: 0, // forever
}}}
```

`ValueKeyframes` move um valor animado por paradas como faz Web Animations, com deslocamentos, um easing por segmento, atraso, iterações e direção. Gestos também podem dirigir valores: `View.PanValues`, `PinchValue` e `RotationValue` os movem na thread principal a partir de onde estavam quando o gesto começou.

`core.PanEvent` inclui `Position` na visualização e `PagePosition` na página em cada fase do pan, de modo que ferramentas de arraste não precisam de um evento de pressionamento separado para recuperar a localização do dedo.

## Próximos passos

- [Estilos e temas](styles.md): as propriedades que uma transição move.
- [Listas e rolagem](lists.md): valores que acompanham uma visualização de rolagem.
- [Layout](layout.md): o que são quadros animados.
