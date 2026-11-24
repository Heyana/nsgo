---
title: Permissões
description: Verifique e peça as permissões do sistema, abra suas configurações e guie o usuário até os Ajustes do Sistema no macOS.
group: Recursos de plataforma
order: 49
---

## Verificando e pedindo

O sistema deixa para o usuário conceder parte do que um aplicativo pode fazer: o microfone, a câmera, os contatos, os calendários, os lembretes, as fotos, o Bluetooth, a biblioteca de mídia, a localização e as notificações em telefones e Macs, o movimento e a localização em segundo plano em telefones e, no macOS, também Acessibilidade, Gravação de Tela, Monitoramento de Entrada, Acesso Total ao Disco, Gerenciamento de Apps, Ferramentas de Desenvolvedor e Automação de outro aplicativo ([localização e movimento](services.md#localização-e-movimento), [notificações](notifications.md)). `core.Permissions()` os lista como chaves `core.Permission`. `cx.PermissionStatus` responde se o aplicativo tem uma sem perguntar, e `cx.RequestPermission` pergunta com o aviso do sistema onde houver um e ainda não tiver sido perguntado. Peça quando o usuário fizer algo que precise da permissão. Face ID e Touch ID não estão entre elas: `cx.BiometricAvailability` informa sobre eles e `cx.AuthenticateBiometric` pergunta com seu próprio aviso ([biometria](services.md#biometria)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

No macOS, a Automação é concedida por aplicativo de destino: passe `core.PermissionAutomationOf("com.apple.finder")` para verificar ou pedir para um aplicativo de destino em execução pelo seu bundle ID, ou `core.PermissionAutomation` para abrir o painel Automação nos Ajustes do Sistema. Um aplicativo de destino que não está em execução responde `PermissionUnknown`.

| Status | `String()` | Significado |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Ainda não perguntado: um pedido mostra o aviso do sistema |
| `PermissionGranted` | `granted` | O aplicativo a tem |
| `PermissionLimited` | `limited` | Acesso parcial, como fotos ou contatos selecionados |
| `PermissionDenied` | `denied` | Recusada; só as configurações do sistema a mudam |
| `PermissionUnknown` | `unknown` | Controlada, mas o sistema ou a plataforma não informa |
| `PermissionNotGated` | `not-gated` | Nada a impede para o aplicativo, como no Linux |
| `PermissionUnavailable` | `unavailable` | A plataforma não tem tal coisa |

`Allowed` é true para concedida, limitada e não controlada. O macOS responde negado para Acessibilidade, Gravação de Tela e Monitoramento de Entrada até que o usuário os ative, já que pedir apenas lista o aplicativo no painel deles, e informa uma concessão de Gravação de Tela só quando o aplicativo inicia novamente.

## Descrições de uso

No macOS e no iOS, pedir requer o motivo da permissão no Info.plist do aplicativo, que `apple.info` do `nsgo.yaml` guarda; sem ele o pedido responde indisponível e registra a chave no log, já que o sistema encerraria o aplicativo. Uma mensagem dos [catálogos](localization.md) do aplicativo com a mesma chave, como `NSCameraUsageDescription: Scans the codes you point it at.` em `locales/en.yaml`, traduz o motivo, que o sistema então pergunta no idioma do usuário; um idioma que não o traduz usa o da origem.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

No Android, cada permissão deve estar listada em `android.permissions` do `nsgo.yaml`; uma deixada fora do manifesto responde `PermissionUnavailable`. Sob `nsgo dev`, o macOS atribui o que o aplicativo faz ao terminal que o iniciou, então é o terminal que recebe o pedido e aparece na lista; compile o aplicativo para testar suas próprias permissões.

## Abrindo as configurações

`cx.OpenPermissionSettings` abre a página que concede uma permissão e responde se alguma foi aberta: Privacidade e Segurança no painel da permissão (incluindo Automação, Contatos, Fotos e Serviços de Localização) ou Notificações no macOS, a página do aplicativo nos Ajustes (ou sua página de Notificações a partir do iOS 16) no iOS, a página do aplicativo nas Configurações no Android e a página do microfone, da câmera ou das notificações no Windows.

Para `PermissionLocation` e `PermissionBackgroundLocation`, o macOS abre Serviços de Localização; `PermissionMotion` abre Movimento e Condicionamento, embora o macOS não forneça amostras de movimento. O iOS e o Android abrem a página do aplicativo para as três. Abrir as configurações não pede nem concede uma permissão.

## Guiando o usuário

`desktop.GuidePermission` abre essa página e espera o usuário conceder a permissão. No macOS, um painel acompanha a janela dos Ajustes do Sistema sem tomar o foco e, para os painéis cuja lista aceita um aplicativo arrastado para ela, segura o aplicativo para o usuário arrastá-lo até lá. Ele termina, e sua função ouve o status, quando o aplicativo tem a permissão, o usuário fecha o painel ou os Ajustes do Sistema, `stop` é chamado ou o componente desaparece; um novo guia encerra o anterior.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

O aplicativo oferecido é o seu bundle, senão o seu executável; `GuideOptions.App` nomeia outro e `Name` como o painel o chama. O Windows abre a página e espera as Configurações fecharem. Em outros lugares, a função ouve o status imediatamente.

## Plataformas

| Plataforma | O que informa e pede |
| --- | --- |
| macOS | Todas as permissões (Automação por aplicativo de destino em execução via `PermissionAutomationOf`, além de notificações para aplicativos em bundle); Automação sem destino, Gerenciamento de Apps, Ferramentas de Desenvolvedor e Mídia e Apple Music são desconhecidas; movimento é indisponível |
| iOS | O microfone, a câmera, os contatos, os calendários, os lembretes, as fotos (com `PermissionLimited`), o Bluetooth, a biblioteca de mídia, a localização, o movimento e as notificações |
| Windows | O microfone, a câmera e as notificações, para as quais ele nunca mostra aviso; o restante não é controlado |
| Linux | Não controlado fora de sandboxes; as notificações refletem se um servidor de notificações responde; no Flatpak e no Snap, a câmera pelo portal do desktop e o microfone pelo portal e pelo PipeWire |
| Web | O microfone e a câmera (desconhecidos no Firefox), as notificações, a localização (não a localização em segundo plano) e o movimento em navegadores móveis |
| Android | O microfone, a câmera, os calendários, o Bluetooth e a biblioteca de mídia, onde `android.permissions` os declara |
| Apple Watch, mini programas | Notificações, localização e movimento por meio de seus serviços; o restante controlado pelo sistema é desconhecido por enquanto |

As permissões de localização e movimento seguem os [sensores de cada plataforma](services.md#localização-e-movimento), e `PermissionNotifications` segue as [notificações](notifications.md). Atividade física usa `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` no Android, `NSMotionUsageDescription` na Apple), incluindo a pressão da Apple; o movimento bruto do Android continua separado. No Safari móvel, peça `PermissionMotion` a partir de um handler de toque.

## Testes

`nsgotest` responde a partir do seu modelo: `app.Permissions` guarda cada status, não perguntado no início, e o pedido de uma não perguntada a torna o valor de `app.PermissionAnswers` para ela, senão concedida. `PermissionNotifications` fica sincronizada com `app.NotificationPermission` e `app.NotificationAnswer`. `app.PermissionSettings` lista as configurações abertas e `app.Guide` o guia exibido, que `app.GrantPermission` e `app.CloseGuide` encerram.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Próximos passos

- [Notificações](notifications.md): sua própria permissão, badges e toques.
- [Compilação e distribuição](building.md): os entitlements e as capabilities que um pacote declara.
