---
title: Live Activities
description: Acompanhe algo enquanto acontece na Tela Bloqueada e na Dynamic Island, e como Live Updates no Android.
group: Recursos de plataforma
order: 48
---

## Ativando

Uma Live Activity acompanha algo em andamento, como uma entrega ou um cronômetro: na Tela Bloqueada e na Dynamic Island de iPhones a partir do iOS 16.1, e como um Live Update no Android 16, no topo da central de notificações e em um chip na barra de status; versões anteriores do Android exibem uma notificação contínua. Ative no arquivo `nsgo.yaml`:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Iniciar, atualizar e encerrar

`widget.StartActivity` inicia uma atividade de um tipo (`kind`), que identifica o que ela acompanha e é escrita da mesma forma que os tipos de widgets, exibindo um `widget.ActivityContent`. `Activity.Update` substitui o que é exibido, `Activity.Alert` faz o mesmo e alerta o usuário, e `Activity.End` a encerra — com conteúdo final quando não for `nil` — deixando-a na Tela Bloqueada conforme definido em `Dismissal`. `widget.Activities` lista as atividades em execução do app, incluindo as de execuções anteriores do app, e `widget.ActivitiesEnabled` informa se o usuário e a plataforma as permitem.

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

`StartActivity` retorna `widget.ErrNoActivities` em um app compilado sem suporte a elas, `widget.ErrActivitiesUnsupported` fora do iOS 16.1+ e Android, e `widget.ErrActivitiesDisabled` quando o usuário as desativou; `Update` e `End` retornam `widget.ErrActivityEnded` após o encerramento. O conteúdo de uma atividade tem limite máximo de 4 KB, conforme aceito pelo ActivityKit e pelo APNs.

## O que ela exibe

Ambas as plataformas exibem o título e o corpo, o status ou um cronômetro atualizado pelo sistema, uma barra de progresso, até três ações e a cor de destaque. O iOS as desenha com a extensão de widget e aceita views de `widget` no lugar delas para a Tela Bloqueada e cada parte da Dynamic Island: `LockScreen`, `Leading`, `Trailing`, `Center` e `Bottom` no modo expandido, `CompactLeading` e `CompactTrailing` na ilha compacta e `Minimal` quando várias atividades a compartilham. O Android exibe apenas os campos, com o ícone do app, já que Live Updates não aceitam views do app.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` abre junto com o app quando a atividade é tocada, `Stale` indica quando o iOS exibe o conteúdo como desatualizado e `Background` preenche o fundo da atividade na Tela Bloqueada.

## Toques

`widget.OnAction` executa o manipulador ao pressionar uma ação ou um `widget.Button`, recebendo a atividade e o ID da ação (`Activity` é `nil` para um [botão de widget](widgets.md#buttons)). Defina-o em `main`, antes de `nsgo.Run`: no iOS, o sistema inicia o app em segundo plano ao receber um toque, sem suas views, e mantém a atividade aguardando até o manipulador retornar, para que ele possa atualizá-la ali; no Android, o toque chega ao processo Go em execução por meio do receptor de runtime sem abrir sua Activity. As ações exigem que o motor que as publicou continue em execução; uma ação obsoleta não faz nada após o encerramento desse motor. A notificação em si continua abrindo o app quando tocada. O manipulador é executado em uma goroutine do sistema, não na das views do app — que ele alcança por meio do `Post` de um `core.Scope`.

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

Uma view da própria atividade também faz isso, no iOS 17 e superior: `widget.Button{Action: "pause"}` executa o mesmo manipulador sem abrir o app e exibe apenas seus filhos onde um toque normalmente abriria o app.

## Atualizações via push

Um app em segundo plano só atualiza suas atividades enquanto está em execução; portanto, um app que acompanha eventos externos as inicia com `widget.ActivityOptions{Push: true}`, envia o token recebido por `widget.OnPushToken` para seu servidor e faz com que ele envie atualizações `liveactivity` do APNs cujo `content-state` é o resultado de `widget.ActivityState` para o conteúdo: nenhuma imagem vai em um push. A partir do iOS 17.2, o app também recebe um token para iniciar atividades via push, com o tipo de atributos `widget.ActivityAttributesType` e atributos de `kind` e `id`. Os pushes exigem o entitlement de notificações push no perfil do app.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Limites

O sistema encerra uma atividade após oito horas. `widget.DismissDefault` mantém uma atividade encerrada na Tela Bloqueada por até quatro horas no iOS e a remove imediatamente no Android, `widget.DismissNow` a remove imediatamente e `widget.DismissAt` a remove em um horário dentro de quatro horas. O Android solicita `POST_NOTIFICATIONS` e `POST_PROMOTED_NOTIFICATIONS`: peça permissão ao usuário antes com `cx.RequestNotificationPermission` ([Notificações](notifications.md)). A demo Widgets da galeria e `examples/widgets` executam uma Live Activity.

## Próximos passos

- [Widgets](widgets.md): a Tela Inicial e as linhas do tempo da Tela Bloqueada, desenhadas pela mesma extensão.
- [Notificações](notifications.md): a permissão que o Android pede primeiro.
- [Compilação e distribuição](building.md): a extensão e os entitlements necessários.
