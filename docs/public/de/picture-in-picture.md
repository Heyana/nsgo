---
title: Bild-in-Bild
description: "Video in Bild-in-Bild auf iOS, tvOS, macOS, Android und im Browser abspielen oder auf iOS einen eigenen Baum anzeigen."
group: Plattformfunktionen
order: 45
---

Deklarieren Sie auf iOS die Hintergrundwiedergabe, bevor Sie Bild-in-Bild verwenden; macOS benötigt keinen Hintergrundmodus:

```yaml
apple:
  info:
    UIBackgroundModes: [audio]
```

Aktivieren Sie für ein Video `PictureInPicture`, halten Sie eine `core.Ref` und starten oder beenden Sie es über ein Steuerelement der App. Verfolgen Sie den Status-Callback, um dieses Steuerelement zu aktivieren, sobald `Possible` true wird:

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

`AutoPictureInPicture` aktiviert den automatischen Eintritt, wenn eine iOS-App in den Hintergrund wechselt; tvOS und macOS ignorieren diese Option. `Controls` zeigt weiterhin die nativen Wiedergabesteuerelemente; der PiP-Button der App verwendet für ein gesteuertes Video dieselbe verwaltete Sitzung. Mit `OnPictureInPictureRestore` kann die App ihre Player-Seite anzeigen, wenn der Nutzer zurückkehrt. Lassen Sie das Video eingehängt, solange PiP aktiv ist: Das Löschen seines Scopes beendet Wiedergabe und PiP.

Auf Android deklariert die paketierte Activity bereits PiP-Unterstützung. Ein Video wechselt in das PiP-Fenster der Activity, wobei die übrige Oberfläche der App ausgeblendet wird, und behält bei der Rückkehr seinen Player und seine Position. `StopPictureInPicture` vergrößert dieselbe Activity wieder. Die Wiedergabe-/Pause-Steuerelemente des Fensters melden `OnPlay` und `OnPause`, mit `VideoPauseControls`; `Ref.Seek` verwendet denselben Player. Der automatische Eintritt erfordert die explizite Einstellung und ein vorbereitetes, laufendes Video. Ein Start kann aufgrund des aktuellen Systemzustands dennoch fehlschlagen; behandeln Sie `Error` auch dann, wenn `Possible` true war.

Ein eingebetteter Android-Host muss `android:supportsPictureInPicture="true"` deklarieren und Konfigurationsänderungen für `orientation|screenSize|screenLayout|smallestScreenSize` selbst behandeln. Leiten Sie `onPictureInPictureModeChanged` und `onUserLeaveHint` der Activity an die gleichnamigen Methoden von `NsgoView` weiter; leiten Sie ab Android 15 zusätzlich `PictureInPictureUiState.isTransitioningToPip()` an `NsgoView.onPictureInPictureTransition` weiter. Ein Host oder Gerät ohne Unterstützung meldet `Supported: false`.

Starten Sie im Web aus einem Tippen bzw. Klick des Nutzers in einem Browser, der die standardisierte Video-PiP-API anbietet. `Supported` berücksichtigt auch die Permissions Policy der Seite, und `Possible` erfordert geladene Video-Metadaten. Die eigenen PiP-Steuerelemente des Browsers melden denselben aktiven Zustand. Das Beenden, Ersetzen oder Leeren der Quelle oder das Entfernen des Videos beendet dessen Sitzung; ein ausstehender Start kann die vorherige Quelle nicht erneut öffnen. Web-PiP zeigt nur das Video, ohne eigene NSGo-Inhalte, automatischen Eintritt im Hintergrund oder `OnPictureInPictureRestore`. Nicht unterstützte Browser melden `Supported: false`.

Auf iOS nimmt `mobile.PictureInPicture` `Children` mit einem eigenen Layout entgegen, standardmäßig 320 × 180 Punkte. Es belegt in der App keinen Platz. Seine `Ref` hat dieselben Start- und Stopp-Methoden, während `OnState`, `OnRestore`, `OnPlay`, `OnPause` und `OnSkip` Systemaktionen liefern. Binden Sie `PausedFunc` und `PositionFunc` an den Wiedergabezustand, den diese Callbacks aktualisieren; `Duration: 0` beschreibt Live-Inhalte. Eine angehaltene App behält das zuletzt gerenderte Frame.

Setzen Sie ein `core.Video` in den eigenen Baum, um seine dekodierten Frames mit von der App gezeichneten Untertiteln oder anderen Overlays zu kombinieren. Starten Sie die Sitzung über die äußere `Ref`. Teilen Sie ein Pause-Signal mit dem Video, speisen Sie die `PositionFunc` der eigenen Sitzung aus `Video.OnProgress` und leiten Sie `OnSkip` an `Ref.Seek` des Videos weiter. Der Ton des Videos wird über dessen eigenen Player wiedergegeben.

Der eigene Baum rendert auch Text, Views, Bilder und Canvas. WebView, Metal-Shader und Backdrop-Effekte sind in den aufgenommenen Frames nicht verfügbar. Animierte Bilder und native Layer-Übergänge zeigen ihren Modellzustand. Verwenden Sie für Bewegung explizite animierte Werte. Die App-Store-Prüfung kann Verwendungen ablehnen, die nichts mit Video zu tun haben.

`Supported: false` unterscheidet eine nicht verfügbare Plattform von einer unterstützten Sitzung, die noch nicht bereit ist oder deren `Error` einen Fehler erklärt. Video unterstützt iOS und tvOS, wenn AVKit meldet, dass das Gerät PiP unterstützt, sowie macOS, Android und Webbrowser, die die standardisierte API anbieten; eigene Bäume unterstützen nur iOS. Auf iOS und tvOS bleibt die gemeinsame Audiositzung in der Wiedergabekategorie, solange PiP aktiviert ist, auch im pausierten Zustand, und mischt sich mit dem Ton anderer Apps, wenn die `AudioSession` des Videos `AudioSessionMixWithOthers` ist.

In Headless-Tests legt `nsgotest.PictureInPictureSupport(supported, possible)` fest, was das System unter einem iOS-, macOS- oder Android-`nsgotest.Environment` anbietet, `PiPState`, `PiPStarts` und `PiPStops` der View zeichnen die Sitzung auf, `app.ReportPictureInPicture` meldet einen Zustand, wie es das System tun würde, und `app.PictureInPictureControl` sendet `play`, `pause`, `skip` und `restore` des Fensters. Das PiP-Fenster des Videos spielt und pausiert den eigenen Player des Videos, was `Video.OnPlay` und `Video.OnPause` melden, mit `VideoPauseControls` für eine Pause; `app.PlayVideo` und `app.PauseVideo` melden diese in Tests.

Führen Sie `go tool nsgo dev -target ios examples/pip` für beide Wege aus oder `-target macos` bzw. `-target android` für Video-PiP. Native Mac-Tests prüfen Eintritt, Austritt und wiederholte Befehle mit und ohne native Steuerelemente sowie das Ersetzen einer aktiven gesteuerten Quelle. Die Abnahme auf dem iPhone, einschließlich Vollbild für gesteuerte Videos und automatischem Eintritt, steht noch aus; die Implementierung hat derzeit native Delegate-/Frame-Tests und Headless-Abdeckung der App.

Führen Sie für tvOS-Video `go tool nsgo dev -target tvos_simulator examples/pip` aus; NSGo verknüpft den Player mit einer nativen Now-Playing-Sitzung. Der tvOS-27-Simulator lädt Videos mit und ohne native Steuerelemente, doch AVKit meldet dort PiP als nicht unterstützt. Tatsächlicher PiP-Eintritt, das Beenden und die Wiederherstellung müssen noch auf einem physischen Apple TV geprüft werden.

Der Regressionstest für das Web-Ziel verwendet einen aufgezeichneten lokalen Clip und vertrauenswürdige Button-Klicks in der App, um die tatsächliche PiP-Sitzung des Browsers über Eintritt, Austritt, Ersetzen der Quelle und Löschen in Chromium zu prüfen. Andere Browser-Engines benötigen noch eine Abnahme.
