---
title: Slider
description: Um valor, ou um intervalo, ao longo de uma trilha, com o teclado e o ponteiro.
group: Componentes
order: 37
---

`ui.Slider` escolhe um valor ao longo de uma trilha, com o arrasto, o passo e o teclado. Um valor é um único botão deslizante; `High` cria um intervalo de dois.

```go preview=slider/basic
volume := core.NewSignal(cx, 40.0)
ui.Slider{Value: volume.Get, OnChange: volume.Set, Min: 0, Max: 100, Step: 5, Label: "Volume"}
```

## Um intervalo

Defina `High` e `OnHighChange` para escolher dois valores. `Value` é a extremidade inferior. `LowLabel` e `HighLabel` nomeiam os botões deslizantes para a tecnologia assistiva.

```go preview=slider/range
price := core.NewSignal(cx, 20.0)
ceiling := core.NewSignal(cx, 80.0)
ui.Slider{
	Value: price.Get, OnChange: price.Set,
	High: ceiling.Get, OnHighChange: ceiling.Set,
	Min: 0, Max: 100, Step: 1,
	Label: "Price", LowLabel: "Minimum", HighLabel: "Maximum",
}
```

## Passos e valores

`Step` ajusta o valor ao passo; um passo de 0 aceita qualquer valor. `ValueText` muda o que a tecnologia assistiva lê como valor, e `Disabled` o desativa.

```go preview=slider/format
ui.Slider{Value: speed.Get, OnChange: speed.Set, Min: 0.5, Max: 2, Step: 0.25, Label: "Speed",
	ValueText: func(v float64) string { return fmt.Sprintf("%g×", v) }}
```

Use `MinFunc`, `MaxFunc` e `StepFunc` quando o intervalo ou o passo mudar, como quando um vídeo informa sua duração. Eles substituem seus campos estáticos e atualizam o slider montado, mantendo seu foco e qualquer arrasto em andamento. Um `MaxFunc` explícito que retorna 0 permite um intervalo colapsado até que a duração chegue; o valor do aplicativo é preservado e exibido dentro dos limites atuais.

```go
ui.Slider{Value: position.Get, OnChange: position.Set, MaxFunc: duration.Get, Label: "Position"}
```

Em TSX, `min={minimum()}`, `max={duration()}` e `step={step()}` seguem seus signals no lugar; `max={0}` colapsa o intervalo, como faz um `MaxFunc` que retorna 0.

## Direção

`Axis: base.Vertical` coloca o slider em pé, com seu mínimo embaixo; dê a ele uma altura em `Style`. `Inverted` coloca o mínimo na outra extremidade: o topo de um slider vertical, o fim de um horizontal. As teclas de seta ao longo da trilha seguem a direção. `ui.Progress` aceita os mesmos `Axis` e `Inverted`.

```go
ui.Slider{Value: level.Get, OnChange: level.Set, Axis: base.Vertical, Label: "Level", Style: core.Style{Height: core.Pt(160)}}
```

## Comportamento

Um arrasto segue o ponteiro e relata `OnChange` enquanto se move. As teclas de seta movem um botão deslizante focado em um passo, Home e End vão até as extremidades, e Page Up e Page Down movem mais longe. `ui.Slider` marca os passos sob a trilha quando `Step` é maior que 0, até 60 marcas.

## Acessibilidade

O slider relata `RoleAdjustable`, seu `Label` e seu valor; um intervalo tem ambos os botões deslizantes com seus próprios rótulos. `ValueText` é lido no lugar do número. O teclado e as ações de incremento e decremento de um leitor de tela o alteram.

## Plataforma

A trilha é desenhada pelo aplicativo e tem o mesmo tamanho em todo lugar. Os toques, o foco e o leitor de tela são os da plataforma; um arrasto por toque se comporta como um arrasto de ponteiro, com o mesmo passo.

## Relacionados

- [Formulários](../forms.md): sliders em um formulário.
- [Acessibilidade](../accessibility.md): valores e papéis ajustáveis.
- As páginas Slider, RangeSlider e Rating da galeria.
- [Componentes sem estilo](../authoring/base.md#slider): `base.Slider`, vertical e com marcas próprias.
