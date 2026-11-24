---
title: Switch
description: Uma configuração que está ligada ou desligada, em três tamanhos.
group: Componentes
order: 35
---

`ui.Switch` é uma configuração que entra em vigor imediatamente. Ele não guarda nenhum valor: o aplicativo é dono do valor e o atualiza em `OnChange`.

```go preview=switch/basic
notifications := core.NewSignal(cx, true)
ui.Switch{Checked: notifications.Get, OnChange: notifications.Set, Label: "Notifications"}
```

## Tamanhos

`ui.Switch` aceita `Checked`, `OnChange`, `Label` e `Size` (`SizeDefault`, `SizeSmall` ou `SizeLarge`). O botão desliza e a trilha muda de cor no relógio de quadros da plataforma.

```go preview=switch/small
ui.Switch{Checked: sync.Get, OnChange: sync.Set, Label: "Sync automatically", Size: ui.SizeSmall}
```

Um switch serve para uma configuração que se aplica imediatamente. Para uma escolha dentro de um formulário, use um [checkbox](checkbox.md); para uma entre várias opções, use `ui.Radio`s em um `ui.RadioGroup` ou um [select](select.md).

## Acessibilidade

O switch relata `RoleSwitch` e seu estado ligado ou desligado. `Label` é o que a tecnologia assistiva lê, e o texto ao lado de um switch geralmente é seu rótulo. Espaço ou um toque o alterna quando ele tem o foco do teclado.

## Plataforma

O controle padrão é desenhado pelo aplicativo, então tem a mesma aparência em todo lugar. `Variant: ui.SwitchSystem` pede o controle da plataforma: NSSwitch no macOS, UISwitch no iOS e o switch de 52 × 32 dp do Material 3 no Android. Os outros alvos usam a aparência padrão. Os toques, o foco e o leitor de tela são os da plataforma. O switch padrão de um telefone tem, por design, o mesmo tamanho que o de um desktop; `SizeSmall` é para linhas densas, não para um dispositivo.

## Relacionados

- [Checkbox](checkbox.md): uma ou mais escolhas.
- [Formulários](../forms.md): configurações em um formulário.
- [Acessibilidade](../accessibility.md): rótulos e foco.
- A página Switch da galeria.
- [Componentes sem estilo](../authoring/base.md#switch): `base.Switch`, para um switch com uma aparência própria.
