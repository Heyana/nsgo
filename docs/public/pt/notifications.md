---
title: Notificações
description: Peça para mostrar notificações, mostre, agende e feche-as, conte-as, mostre o progresso de uma tarefa no ícone do aplicativo e ouça os toques.
group: Recursos de plataforma
order: 45
---

## Permissão

`cx.RequestNotificationPermission` pede ao usuário que deixe o aplicativo mostrar notificações, com o aviso do sistema na primeira vez, e responde com um `core.NotificationPermission`: `NotificationsGranted`, `NotificationsDenied` ou `NotificationsUnavailable` quando a plataforma ou a build não mostra nenhuma. Peça quando o usuário fizer algo que precise de notificações: o iOS e o Android perguntam apenas uma vez, e os navegadores recusam um aviso que o usuário não provocou. `cx.NotificationPermission` lê a resposta sem perguntar.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Mostrando uma

`cx.ShowNotification` mostra uma notificação, também enquanto o aplicativo está em primeiro plano, e substitui uma com o mesmo `ID`; `Silent` deixa de fora o som e a vibração do sistema. Ela responde a `done` se o sistema aceitou a notificação, false sem a permissão, e retorna `core.ErrNotification`, sem chamar `done`, para uma sem ID ou sem título. `cx.CloseNotification` remove aquela que um ID nomeia.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` remove todas as notificações que o aplicativo mostrou e cancela as que ele agendou. No macOS, iOS, watchOS, Windows e Android isso inclui as que execuções anteriores deixaram na Central de Notificações, na Central de Ações ou na aba de notificações; no Linux e na web alcança apenas as que esta execução mostrou ou agendou. Chame-a quando o aplicativo vier para o primeiro plano e sua janela mostrar aquilo que elas informavam:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Agendando uma

Uma notificação cujo `At` é posterior a agora é agendada, e chamar `cx.ShowNotification` novamente com seu `ID` a move, enquanto `cx.CloseNotification` a cancela antes que apareça. `Repeat` a mostra todos os dias no horário do dia de `At`, ou também toda semana no seu dia da semana, a partir do próximo horário desse tipo, no fuso horário do dispositivo. `cx.Env().CanScheduleNotifications()` diz se a plataforma as agenda: aplicativos de macOS, iOS e watchOS em um bundle, o Android e o Windows as agendam nativamente, esteja o aplicativo em execução ou não (com o Windows encadeando as repetições diárias e semanais enquanto o aplicativo roda e as rearmando na inicialização), enquanto o Linux e a web (onde `Notification` está disponível) as mantêm enquanto o aplicativo ou a página roda. Em outros lugares (o tvOS, cuja central mostra apenas badges, e os mini programas) `done` responde false.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

O Android pode mostrar uma notificação agendada alguns minutos depois do horário enquanto o telefone cochila; um aplicativo que precisa dela no minuto exato, como um despertador, lista `android.permission.SCHEDULE_EXACT_ALARM` em `android.permissions`, que o usuário concede nas configurações do sistema a partir do Android 12, ou `USE_EXACT_ALARM`, que o Google Play permite apenas a aplicativos de alarme e de calendário. As notificações de um aplicativo Android forçado a parar esperam até a próxima vez que ele iniciar. Para testar a entrega após a morte do processo, espere `ShowNotification` responder true e depois mate seus processos sem forçar a parada do pacote.

## Toques

`cx.OnNotificationPress` executa sua função com o `ID` de uma notificação que o usuário tocou, enquanto o componente que a definiu existir, e traz o aplicativo para o primeiro plano. Um toque que iniciou o aplicativo chega ao primeiro handler registrado. Guarde em `ID` o que o toque abre (`"chat:42"`), já que um toque pode iniciar o aplicativo do zero. No Windows ele faz isso para um aplicativo que seu instalador colocou no menu Iniciar; caso contrário, uma execução só ouve os toques enquanto está rodando.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Um aplicativo Android que pede a partir do Android 13 lista `android.permission.POST_NOTIFICATIONS` em `android.permissions` do `nsgo.yaml`.

## Badge

`cx.SetBadge` coloca uma contagem no ícone do aplicativo: o do Dock no macOS, o da Tela de Início no iOS, que precisa da permissão de notificação, o do botão da barra de tarefas no Windows, o de lançadores Linux compatíveis com Unity e o de um aplicativo web instalado. Uma contagem de 0 ou menos o limpa. Um aplicativo macOS não precisa de permissão para o badge do Dock até verificar ou pedir a permissão de notificação ou mostrar uma notificação; a partir daí, nessa execução e nas seguintes, o Dock só mostra o badge enquanto o usuário permitir os badges do aplicativo nos Ajustes do Sistema, que `cx.RequestNotificationPermission` pede junto com os alertas. Lançadores sem uma interface de badge do Linux e o Android não têm contagem a definir; os lançadores do Android marcam um aplicativo cujas notificações estão visíveis.

```go
cx.SetBadge(len(unread.Get()))
```

## Progresso de tarefa

`cx.SetTaskProgress` mostra no ícone do aplicativo o quanto uma tarefa longa já avançou: uma barra sobre o bloco do Dock no macOS, sobre o botão da barra de tarefas no Windows e em lançadores Linux compatíveis com Unity. Ele não precisa de permissão e aparece ao lado do badge. `TaskProgressNormal` mostra `Value`, uma fração de 0 a 1; `TaskProgressPaused` e `TaskProgressError` a colorem no Windows e no macOS; `TaskProgressIndeterminate` se move no Windows e é uma barra parada, preenchida pela metade, no macOS e no Linux; o `TaskProgress` zero a limpa. No Windows ela aparece no botão da janela do escopo, e o progresso definido onde nenhuma janela tem um botão próprio, como o próprio escopo do aplicativo ou um painel da bandeja, aparece em todas as janelas do aplicativo que não definem nenhum. O iOS, o Android e a web não mostram nada.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## Próximos passos

- [Permissões](permissions.md): as outras permissões do sistema, como o microfone e a Acessibilidade.
- [Resposta tátil](haptics.md): o feedback que o dispositivo reproduz sob o dedo do usuário.
- [Serviços da plataforma](services.md): alertas, a área de transferência, arquivos, armazenamento e rede.
- [Live Activities](live-activities.md): o que o aplicativo acompanha, na Tela Bloqueada e na Dynamic Island.
- [Compilação e distribuição](building.md): as permissões de que um pacote precisa.
