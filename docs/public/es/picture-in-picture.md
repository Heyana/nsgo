---
title: Imagen en imagen
description: Reproduce video en Picture in Picture en iOS, tvOS, macOS, Android y navegadores, o muestra un árbol personalizado en iOS.
group: Funciones de plataforma
order: 45
---

En iOS, declara la reproducción en segundo plano antes de usar Picture in Picture; macOS no necesita modo en segundo plano:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Para un video, habilita `PictureInPicture`, conserva un `core.Ref` e inícialo o detenlo desde un control de la aplicación. Sigue la devolución de llamada de estado para habilitar ese control cuando `Possible` pase a ser verdadero:

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

`AutoPictureInPicture` activa la entrada automática cuando una aplicación de iOS pasa a segundo plano; tvOS y macOS ignoran esta opción. `Controls` sigue mostrando los controles de reproducción nativos; el botón de PiP de la aplicación usa la misma sesión administrada para un video con controles. `OnPictureInPictureRestore` permite que la aplicación muestre su página del reproductor cuando el usuario regresa. Mantén el video montado mientras PiP esté activo: eliminar su ámbito detiene la reproducción y PiP.

En Android, la actividad empaquetada ya declara compatibilidad con PiP. Un video entra en la ventana de PiP de la actividad, con el resto de la interfaz de la aplicación oculta, y conserva su reproductor y posición cuando regresa. `StopPictureInPicture` expande esa misma actividad. Los controles de reproducción/pausa de la ventana informan `OnPlay` y `OnPause`, con `VideoPauseControls`; `Ref.Seek` usa el mismo reproductor. La entrada automática requiere la configuración explícita y un video preparado y en reproducción. Aun así, un inicio puede fallar debido al estado actual del sistema; gestiona `Error` incluso cuando `Possible` era verdadero.

Un host de Android incrustado debe declarar `android:supportsPictureInPicture="true"` y gestionar los cambios de configuración `orientation|screenSize|screenLayout|smallestScreenSize`. Reenvía `onPictureInPictureModeChanged` y `onUserLeaveHint` de la actividad a los mismos métodos en `NsgoView`; en Android 15+, reenvía también `PictureInPictureUiState.isTransitioningToPip()` a `NsgoView.onPictureInPictureTransition`. Un host o dispositivo sin compatibilidad informa `Supported: false`.

En la web, inicia desde una pulsación del usuario en un navegador que ofrezca la API estándar de PiP para video. `Supported` también refleja la Permissions Policy de la página, y `Possible` requiere metadatos de video cargados. Los controles de PiP propios del navegador informan el mismo estado activo. Detener, reemplazar o limpiar el origen, o eliminar el video finaliza su sesión; un inicio pendiente no puede volver a abrir el origen anterior. PiP en la web muestra solo el video, sin contenido personalizado de NSGo, entrada automática en segundo plano ni `OnPictureInPictureRestore`. Los navegadores no compatibles informan `Supported: false`.

En iOS, `mobile.PictureInPicture` recibe `Children` con una disposición propia, con un valor predeterminado de 320 × 180 puntos. No ocupa espacio en la aplicación. Su `Ref` tiene los mismos métodos de inicio y detención, mientras que `OnState`, `OnRestore`, `OnPlay`, `OnPause` y `OnSkip` entregan acciones del sistema. Enlaza `PausedFunc` y `PositionFunc` al estado de reproducción que actualizan esas devoluciones de llamada; `Duration: 0` describe contenido en directo. Una aplicación suspendida conserva el último fotograma renderizado.

Coloca un `core.Video` en el árbol personalizado para componer sus fotogramas decodificados con subtítulos dibujados por la aplicación u otras superposiciones. Inicia la sesión usando el `Ref` exterior. Comparte una señal de pausa con el video, alimenta `PositionFunc` de la sesión personalizada desde `Video.OnProgress` y reenvía `OnSkip` a `Ref.Seek` del video. El audio del video se reproduce a través de su propio reproductor.

El árbol personalizado también renderiza texto, vistas, imágenes y Canvas. WebView, los shaders de Metal y los efectos de fondo no están disponibles en los fotogramas capturados. Las imágenes animadas y las transiciones de capas nativas muestran el estado de su modelo. Usa valores animados explícitos para el movimiento. La revisión del App Store puede rechazar usos no relacionados con video.

`Supported: false` distingue una plataforma no disponible de una sesión compatible que no está lista o cuyo `Error` explica un fallo. El video es compatible con iOS y tvOS cuando AVKit informa que el dispositivo admite PiP, macOS, Android y navegadores web que ofrecen la API estándar; los árboles personalizados solo son compatibles con iOS. En iOS y tvOS, la sesión de audio compartida permanece en la categoría de reproducción mientras PiP está habilitado, incluso en pausa, mezclándose con el audio de otras aplicaciones cuando `AudioSession` del video es `AudioSessionMixWithOthers`.

En las pruebas sin interfaz gráfica, `nsgotest.PictureInPictureSupport(supported, possible)` establece lo que el sistema ofrece bajo un `nsgotest.Environment` de iOS, macOS o Android; `PiPState`, `PiPStarts` y `PiPStops` de la vista registran la sesión; `app.ReportPictureInPicture` informa un estado como lo haría el sistema; y `app.PictureInPictureControl` envía `play`, `pause`, `skip` y `restore` de la ventana. La ventana de PiP del video reproduce y pausa el propio reproductor del video, lo que informan `Video.OnPlay` y `Video.OnPause`, con `VideoPauseControls` para una pausa; `app.PlayVideo` y `app.PauseVideo` los informan en las pruebas.

Ejecuta `go tool nsgo dev -target ios examples/pip` para ambas rutas, o `-target macos` o `-target android` para PiP de video. Las pruebas nativas en Mac verifican la entrada, la salida y los comandos repetidos con y sin controles nativos, además del reemplazo de un origen activo con controles. La aceptación en iPhone, incluidos la pantalla completa de video con controles y la entrada automática, sigue pendiente; la implementación cuenta actualmente con pruebas nativas de delegado y fotogramas, así como cobertura de aplicación sin interfaz gráfica.

Para video en tvOS, ejecuta `go tool nsgo dev -target tvos_simulator examples/pip`; NSGo asocia el reproductor con una sesión nativa de Now Playing. El simulador de tvOS 27 carga videos con y sin controles nativos, pero AVKit informa allí que PiP no es compatible. La entrada, detención y restauración reales de PiP aún requieren verificación en un Apple TV físico.

La regresión del destino web usa un clip local grabado y pulsaciones confiables de botones de la aplicación para verificar la sesión real de PiP del navegador mediante la entrada, la salida, el reemplazo de origen y la eliminación en Chromium. Otros motores de navegación aún necesitan aceptación.
