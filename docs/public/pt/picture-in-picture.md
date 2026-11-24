---
title: Picture in Picture
description: Reproduza vídeo no Picture in Picture do iOS, tvOS, macOS, Android e do navegador, ou mostre uma árvore personalizada no iOS.
group: Recursos de plataforma
order: 45
---

No iOS, declare a reprodução em segundo plano antes de usar o Picture in Picture; o macOS não precisa de modo de segundo plano:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Para um vídeo, ative `PictureInPicture`, mantenha um `core.Ref` e inicie ou pare a sessão a partir de um controle do app. Acompanhe o callback de estado para ativar esse controle quando `Possible` se tornar verdadeiro:

```go
ref := &core.Ref{}
state := core.NewSignal(cx, core.PictureInPictureState{})
video := core.Video{
    Source: "asset:clip.mp4", Ref: ref,
    PictureInPicture: true, AutoPictureInPicture: true,
    OnPictureInPicture: state.Set,
}
start := ui.Button{
    Text: "Picture in Picture", OnPress: ref.StartPictureInPicture,
    Disabled: func() bool { return !state.Get().Possible },
}
```

`AutoPictureInPicture` ativa a entrada automática quando um app iOS vai para segundo plano; o tvOS e o macOS ignoram essa opção. `Controls` continua mostrando os controles de reprodução nativos; o botão de PiP do app usa a mesma sessão gerenciada para um vídeo controlado. `OnPictureInPictureRestore` permite que o app revele a página do seu player quando o usuário volta. Mantenha o vídeo montado enquanto o PiP estiver ativo: excluir seu escopo interrompe a reprodução e o PiP.

No Android, a activity empacotada já declara suporte a PiP. Um vídeo entra na janela de PiP da activity, com o restante da interface do app oculto, e mantém seu player e sua posição ao retornar. `StopPictureInPicture` expande essa mesma activity. Os controles de reproduzir/pausar da janela reportam `OnPlay` e `OnPause`, com `VideoPauseControls`; `Ref.Seek` usa o mesmo player. A entrada automática exige a configuração explícita e um vídeo preparado e em reprodução. Um início ainda pode falhar por causa do estado atual do sistema; trate `Error` mesmo quando `Possible` era verdadeiro.

Um host Android incorporado deve declarar `android:supportsPictureInPicture="true"` e tratar as mudanças de configuração `orientation|screenSize|screenLayout|smallestScreenSize`. Encaminhe `onPictureInPictureModeChanged` e `onUserLeaveHint` da activity para os métodos de mesmo nome em `NsgoView`; no Android 15+, encaminhe também `PictureInPictureUiState.isTransitioningToPip()` para `NsgoView.onPictureInPictureTransition`. Um host ou dispositivo sem suporte reporta `Supported: false`.

Na web, inicie a partir de um toque do usuário em um navegador que ofereça a API padrão de PiP de vídeo. `Supported` também reflete a Permissions Policy da página, e `Possible` exige que os metadados do vídeo estejam carregados. Os próprios controles de PiP do navegador reportam o mesmo estado ativo. Parar, substituir ou limpar a fonte, ou remover o vídeo, encerra sua sessão; um início pendente não pode reabrir a fonte anterior. O PiP na web mostra apenas o vídeo, sem conteúdo NSGo personalizado, entrada automática em segundo plano ou `OnPictureInPictureRestore`. Navegadores sem suporte reportam `Supported: false`.

No iOS, `mobile.PictureInPicture` recebe `Children` com um layout próprio, com padrão de 320 × 180 pontos. Ele não ocupa espaço no app. Seu `Ref` tem os mesmos métodos de iniciar e parar, enquanto `OnState`, `OnRestore`, `OnPlay`, `OnPause` e `OnSkip` entregam as ações do sistema. Vincule `PausedFunc` e `PositionFunc` ao estado de reprodução que esses callbacks atualizam; `Duration: 0` descreve conteúdo ao vivo. Um app suspenso mantém o último quadro renderizado.

Coloque um `core.Video` na árvore personalizada para compor seus quadros decodificados com legendas desenhadas pelo app ou outras sobreposições. Inicie a sessão usando o `Ref` externo. Compartilhe um sinal de pausa com o vídeo, alimente o `PositionFunc` da sessão personalizada a partir de `Video.OnProgress` e encaminhe `OnSkip` para o `Ref.Seek` do vídeo. O áudio do vídeo é reproduzido pelo seu próprio player.

A árvore personalizada também renderiza texto, visualizações, imagens e Canvas. WebView, shaders Metal e efeitos de fundo (backdrop) não estão disponíveis nos quadros capturados. Imagens animadas e transições de camadas nativas mostram o estado do seu modelo. Use valores animados explícitos para movimento. A revisão da App Store pode rejeitar usos não relacionados a vídeo.

`Supported: false` distingue uma plataforma indisponível de uma sessão suportada que não está pronta ou cujo `Error` explica uma falha. O vídeo é suportado no iOS e no tvOS quando o AVKit informa que o dispositivo suporta PiP, no macOS, no Android e em navegadores que oferecem a API padrão; árvores personalizadas são suportadas apenas no iOS. No iOS e no tvOS, a sessão de áudio compartilhada permanece na categoria de reprodução enquanto o PiP estiver ativado, inclusive quando pausado, mixando com o áudio de outros apps quando o `AudioSession` do vídeo é `AudioSessionMixWithOthers`.

Em testes headless, `nsgotest.PictureInPictureSupport(supported, possible)` define o que o sistema oferece em um `nsgotest.Environment` de iOS, macOS ou Android, `PiPState`, `PiPStarts` e `PiPStops` da visualização registram a sessão, `app.ReportPictureInPicture` reporta um estado como o sistema faria, e `app.PictureInPictureControl` envia `play`, `pause`, `skip` e `restore` da janela. A janela de PiP do vídeo reproduz e pausa o próprio player do vídeo, o que `Video.OnPlay` e `Video.OnPause` reportam, com `VideoPauseControls` para uma pausa; `app.PlayVideo` e `app.PauseVideo` os reportam nos testes.

Execute `go tool nsgo dev -target ios examples/pip` para os dois caminhos, ou `-target macos` ou `-target android` para o PiP de vídeo. Testes nativos no Mac verificam entrada, saída e comandos repetidos com e sem controles nativos, além da substituição de uma fonte controlada ativa. A validação no iPhone, incluindo tela cheia de vídeo controlado e entrada automática, ainda está pendente; a implementação atualmente tem testes nativos de delegate/quadro e cobertura headless do app.

Para vídeo no tvOS, execute `go tool nsgo dev -target tvos_simulator examples/pip`; o NSGo associa o player a uma sessão nativa de Now Playing. O simulador do tvOS 27 carrega vídeos com e sem controles nativos, mas lá o AVKit informa que o PiP não é suportado. A entrada, a parada e a restauração reais do PiP ainda exigem verificação em uma Apple TV física.

O teste de regressão do alvo web usa um clipe local gravado e toques confiáveis em botões do app para verificar a sessão real de PiP do navegador durante a entrada, a saída, a substituição da fonte e a exclusão no Chromium. Outros motores de navegador ainda precisam de validação.
