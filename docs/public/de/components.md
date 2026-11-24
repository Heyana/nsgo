---
title: Komponenten und Zustand
description: Elemente, Komponenten, Signals und Styles.
group: Kernkonzepte
order: 50
---

## Elemente

Der Bildschirm ist ein Baum aus Elementen, die Go-Struct-Werte sind. `core` enthält die Grundbausteine: `View`, `Text`, `Image`, `Scroll`, `Input` und weitere. `ui` bietet die Steuerelemente, die eine App direkt einsetzt – etwa `Button`, `Select`, `DatePicker` und `EmojiPicker` –, in einem Erscheinungsbild passend zum Theme. (`base` enthält dieselben Steuerelemente ohne vorgegebenes Aussehen für [Komponentenautoren](authoring/index.md).)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` zeigt Webinhalte an; eine leere `WebSource` lädt `about:blank` und meldet die üblichen Lade- und Navigationsereignisse. Setzen Sie `SourceFunc` auf eine leere Quelle oder rufen Sie `Ref.Load(core.WebSource{})` auf, um die vorherige Seite zu leeren. Setzen Sie `InjectedJavaScriptBeforeContentFunc`, wenn das Skript vor den eigenen Skripten der Seite einem Signal folgt: Sobald es sich ändert, lädt der View seine Quelle mit dem neuen Skript erneut – anstelle der aktuell gezeigten Seite, selbst wenn die Nutzerin, der Nutzer oder `Ref.Load` dorthin navigiert hat. Setzen Sie `UserAgentSuffix` (TSX `userAgentSuffix`), um ein Token wie `Version/26.0 Safari/605.1.15` an den plattformeigenen User-Agent – oder an `UserAgent`, falls gesetzt – anzuhängen; unter macOS und iOS wirkt ein Suffix am plattformeigenen User-Agent nur beim ersten Erstellen des Views, während HarmonyOS und das Web ihren eigenen User-Agent behalten. Hängen Sie ein `core.Ref` an, um `Reload`, `GoBack` und `GoForward` aufzurufen. Auf nativen Zielplattformen bleiben bei diesen Befehlen die eigenen HTML- und Datenquellen der App von `core.AccessWebSchemes` ausgenommen; Seiten, die neue Navigationen auslösen, unterliegen weiterhin der Zugriffsrichtlinie.

Unter tvOS ist `cx.Env().HasWebView()` false. Das Mounten eines `WebView` meldet einen `*core.WebError` über `OnError`, ohne WebKit zu laden; stellen Sie für diese Zielplattform eigene App-Inhalte bereit.

Unter macOS, iOS, Windows und Linux meldet `OnNavigationStateChange` auch URL-, Verlaufs- und Titeländerungen innerhalb desselben Dokuments.

Setzen Sie `WebView.Media` auf `core.WebMicrophone`, `core.WebCamera` oder beides, um `getUserMedia`-Anfragen einer Seite zuzulassen. Deklarieren Sie die Nutzungsbeschreibungen in `apple.info` sowie `RECORD_AUDIO` oder `CAMERA` in `android.permissions`; das System fragt die Nutzerin oder den Nutzer dennoch selbst um Erlaubnis. Unter Android wird eine ausstehende Anfrage abgelehnt, wenn die App eines der angeforderten Geräte entfernt, die Seite verlassen wird oder der View geschlossen wird, während die Systemabfrage offen ist. Die Seite muss den Zugriff nach einer solchen Änderung erneut anfordern.

`WebView.Muted` und `MutedFunc` (TSX `muted`) schalten unter Windows und Linux die gesamte Tonausgabe der Seite stumm, einschließlich Web Audio und eingebetteter Frames. macOS 12 und iOS 15 oder neuer pausieren die Medienwiedergabe, solange sie stummgeschaltet sind. Andere Hosts behalten ihre gewohnte Wiedergabe bei. Aktualisierungen gelten für die aktuelle Seite und bleiben über Navigationen hinweg bestehen.

In TSX lädt eine Änderung von `injectedJavaScriptBeforeContentLoaded` die Quelle mit dem neuen Skript im selben WebView erneut, genau wie `InjectedJavaScriptBeforeContentFunc`; eine Änderung von `injectedJavaScript` erzeugt ein neues WebView, das die Quelle neu lädt – ebenso wie eine Änderung von `userAgent`, `userAgentSuffix` oder `mobile`. Unter HarmonyOS erzeugt eine Änderung beider injizierten Skripte ein neues WebView und startet einen neuen nativen Verlauf. Unter Windows und HarmonyOS läuft jedes injizierte Skript im Haupt-Frame innerhalb eines eigenen Blocks, sodass seine Top-Level-Deklarationen mit `let`, `const` und `class` lokal bleiben; weisen Sie Eigenschaften `window` zu, damit die Seite sie global sieht.

Setzen Sie `Mobile` (TSX `mobile`), um eine Seite unter Windows, macOS und Linux so anzuzeigen, wie ein Smartphone sie sieht – etwa für Websites wie YouTube Shorts, die Videos nur auf Smartphones per vertikalem Wischen wechseln: Die Seite erhält einen Smartphone-User-Agent (sofern `UserAgent` nicht gesetzt ist) sowie einen Touchscreen ohne Hover, und ein Ziehen mit der Maus oder ein Mausrad- beziehungsweise Trackpad-Scrollen über nicht scrollbaren Bereichen erreicht die Seite als Fingerwischgeste. Windows emuliert ein Smartphone wie im Gerätemodus von Chrome. macOS und Linux setzen dies per Skript um, sodass Stylesheets dort weiterhin eine Maus sehen und die Touch-Events als nicht vertrauenswürdig gelten, was eine Seite ignorieren kann. iOS, Android, HarmonyOS, das Web und Mini-Programme zeigen Seiten unverändert an.

Unter macOS, iOS, Windows und Linux empfängt `OnOpenWindow` Anfragen einer Webseite zum Öffnen eines neuen Fensters. Der View erzeugt kein Popup. Nutzen Sie `URL` und `UserInitiated` der Anfrage, um zu entscheiden, ob derselbe View mit `Ref.Load` navigieren, die URL anderswo öffnen oder die Anfrage ignorieren soll. Ein eingeschränkter View leitet abgelehnte Anfragen an `OnError` weiter.

Setzen Sie `OnDownload`, um Dateien zu speichern, die Seiten in einem WebView unter macOS, iOS, Linux, Windows und Android herunterladen. Downloads werden unter `downloads/<id>/<file name>` im Datenverzeichnis der App gespeichert und melden ihren Start, ihren Abschluss mit Byte-Anzahl oder einen Fehler; Views ohne `OnDownload` sowie eingeschränkte Views brechen Downloads ab.

`core.Video` spielt eine Datei oder URL im Player der jeweiligen Plattform ab. Unter macOS, iOS und tvOS können Sie `Video.AudioSource` auf eine separate Datei oder URL mit Ton für einen endlichen Videoclip setzen. Der Ton des Players stammt dann aus dieser Quelle. Bild und Ton starten gemeinsam, und `Ref.Seek` verschiebt beide auf derselben Zeitleiste. Das Video bestimmt die Wiedergabedauer; ein kürzerer Ton verstummt, sobald er endet.

Ein transparenter Clip zeigt den Hintergrund als HEVC mit Alpha in einer `.mov`-Datei unter macOS (iOS ist noch nicht geprüft) sowie als VP9 mit Alpha in WebM unter Linux mit dem GStreamer-Plug-in `codecalpha` (`gstreamer1.0-plugins-bad`, das ein Linux-Paket in `nsgo.yaml` empfehlen kann). Unter macOS, iOS, Windows und Linux meldet ein lokaler Clip, dessen Alpha-Kanal nicht erhalten werden kann (WebM auf Apple-Plattformen, HEVC unter Linux und vorerst jeder Clip unter Windows, bis die Alpha-Ausgabe von Media Foundation auf einem Windows-Rechner geprüft wurde), dies über `OnError`, statt das Video in einem deckenden Kasten anzuzeigen: Der Fehler ist ein `*core.MediaError`, dessen `Reason` gleich `core.MediaErrorAlphaUnsupported` ist, sodass die App diesen Clip stattdessen selbst dekodieren kann. Android und das Web sowie entfernte URLs auf allen Plattformen spielen solche Clips so ab, wie ihr Decoder sie ausgibt – was deckend sein kann.

`OnPause` und `OnPlay` folgen den integrierten Steuerelementen auf Apple-Plattformen, Android, Windows, Linux und im Web. `OnPause` liefert `VideoPauseControls` bei einer Pause durch ein Steuerelement. `Paused` greift, wenn eine Quelle geöffnet wird oder sich ihr Wert ändert. Änderungen an Lautstärke, Wiedergabegeschwindigkeit oder Layout behalten ein natives Pausieren oder Abspielen bei, einschließlich eines Starts über die Steuerelemente, während `Paused` weiterhin true bleibt. Binden Sie `PausedFunc` an Ihren Zustand und aktualisieren Sie ihn in diesen Callbacks, wenn Ihre App einen eigenen Play-Button anzeigt.

`Rate` legt die Wiedergabegeschwindigkeit fest, und `RateFunc` kann sie während der Wiedergabe ändern. Nutzen Sie `VideoInfo.RateChangeable` aus `OnLoad`, um ein Geschwindigkeitssteuerelement anzuzeigen. Linux kann die Geschwindigkeit suchbarer Videos ändern, wenn GstPlay und der GStreamer-Filter `scaletempo` installiert sind. Der Fallback-Player verwendet 1×.

Setzen Sie `Video.Headers`, um HTTP-Anfrage-Header wie `Referer`, `User-Agent` oder `Cookie` unter Android sowie für direkte HTTP-Dateien wie MP4 oder MOV unter Linux, macOS, iOS und tvOS mitzusenden.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

Das Video kopiert die Map beim Mounten und verwendet dieselben Header, wenn `SourceFunc` eine neue URL liefert. Verwenden Sie auf Apple-Plattformen zum Spulen eine Quelle mit bekannter Byte-Länge und Unterstützung für Range-Anfragen. Der Player sendet die konfigurierten Header auch über Weiterleitungen mit demselben Ursprung (Same-Origin) hinweg mit. Dieselben Header gelten auf Apple-Plattformen auch für eine direkte HTTP-Datei in `AudioSource`. Playlist-URLs mit der Endung `.m3u` oder `.m3u8` verwenden den nativen Loader.

Installieren Sie unter Linux GstPlay sowie die GStreamer-Plug-ins `scaletempo` und `souphttpsrc`. Verwenden Sie die endgültige Medien-URL, wenn Sie `User-Agent` setzen oder Ihrem `souphttpsrc` die Eigenschaft `location-trusted` fehlt. Mit dieser Eigenschaft und dem Standard-User-Agent behält GStreamer die App-Header bei Same-Origin-Weiterleitungen bei. Die Header der App gelten für die Quelldatei; adaptive Mediensegmente verwenden ihre eigenen Anfragen.

## Komponenten

Eine Komponente ist eine Funktion, die einen Scope entgegennimmt und ein Element zurückgibt. Sie läuft einmal; was sich später ändert, wird über Signals gesteuert.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Eine Komponente mit Props ist ein Typ mit einer `Render`-Methode:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` kann seinen Schließen-Button mit `NoClose` ausblenden, einen `Trailing`-Node hinzufügen und den Toast über `OnPress` schließen. `Toasts.NoClose` blendet Schließen-Buttons im gesamten Stack aus.

`ui.Toasts.Style` und `StyleFunc` passen jede Toast-Oberfläche an, einschließlich Hintergrund, Rahmen, Ecken und Innenabstand. Der Style des Stacks wird über das Theme gelegt, gefolgt vom jeweiligen `ToastContent.Style` und `StyleFunc`. Jede `StyleFunc` läuft nach dem zugehörigen `Style` und aktualisiert sich, sobald sich ihre Signals ändern. Eine `Color` in diesen Styles färbt den Titel ein sowie die Beschreibung mit reduzierter Deckkraft, sodass Text auf einem dunklen oder farbigen Untergrund gut lesbar bleibt. Die Einblende-, Ausblende- und Wischphasen wenden weiterhin ihre Bewegungs-Styles an. In TSX setzen Sie das `style`-Prop von `Toasts` und übergeben `style` oder `styleFunc` an `show` für einen einzelnen Toast.

## Signals

Ein Signal hält einen Wert. Wird es innerhalb einer Funktion gelesen, die ein Element aufruft – etwa `ContentFunc` –, abonniert dieser Teil des Bildschirms das Signal, und bei einer Wertänderung wird nur genau dieser Teil aktualisiert.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Ein Signal über ein ganzes Slice aktualisiert jede Zeile, die es liest. `core.NewCollection` verwaltet Einträge nach Schlüssel, jeweils in einem eigenen Signal, sodass jede Zeile ihrem eigenen Eintrag folgt und das Ändern eines Eintrags nur eine Zeile neu ausführt:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Weitere Zustände bauen auf die gleiche Weise auf Signals auf:

- `core.NewSelector` meldet, ob ein Schlüssel gerade ausgewählt ist, und führt nur die Zeilen neu aus, die eine Auswahl verlässt oder neu erreicht.
- `core.NewHistory` verwaltet einen Wert mit Rückgängig und Wiederherstellen samt `CanUndo`- und `CanRedo`-Signals für ein Bearbeiten-Menü.
- `core.NewPersisted` speichert einen typisierten Wert im Speicher der App als versioniertes JSON und migriert Daten älterer Versionen.
- `core.From` folgt einer Quelle außerhalb des Graphen, etwa einer Subscription, die auf einer anderen Goroutine liefert.

## Styles

`core.Style` folgt CSS: Flexbox und Grid, Innen- und Außenabstände, abgerundete Ecken, Rahmen, Schatten, Farbverläufe, Transformationen und Zustände wie `Active`. Elemente akzeptieren über `Class` auch Tailwind-Klassen:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Themes verleihen den Komponenten aus `ui` ihr helles und dunkles Erscheinungsbild.

## Desktop-Apps

`nsgo.Run(Root)` zeigt einen einzelnen Root-View an. Eine Desktop-App kann stattdessen selbst öffnen, was sie anzeigt: `nsgo.Run(App)` führt `func App(app *core.Scope)` einmal aus, und diese Funktion öffnet über `desktop` Fenster, Panels, Tray-Icons und globale Tastenkürzel, die sich ihre Signals teilen. Das funktioniert unter macOS, Windows und Linux; unter macOS bleibt `examples/tray` aus dem Dock heraus und lebt in der Menüleiste. `nsgo.Run(App, nsgo.StartHidden())` öffnet das Hauptfenster, ohne es anzuzeigen, damit eine Tray-App – oder eine App, die das System bei der Anmeldung gestartet hat (`desktop.LaunchedAtLogin`) – nicht kurz aufblitzt; die App zeigt es später mit `app.MainWindow().Show()`. Smartphones und das Web zeigen die App unmittelbar beim Start an.

`desktop.SetWindowLevel` hält ein Fenster über den anderen – wie bei einem Timer oder einem Desktop-Pet – oder darunter, und `desktop.SetWindowFrame` verschiebt und skaliert es. `desktop.CanSetWindowLevel` und `desktop.CanPlaceWindows` geben im Scope eines Fensters an, ob dies greift: Beide gelten unter macOS, Windows und Linux unter X11, nicht jedoch für ein gewöhnliches Wayland-Toplevel, dessen Compositor die Fenster stapelt und platziert.

Ein Fenster, das mit `desktop.WindowOptions{Transparent: true}` geöffnet wurde – oder das Hauptfenster mit `nsgo.WindowTransparent()` –, hat weder Hintergrund noch Rahmen, sodass der Desktop überall dort durchscheint, wo die App nichts zeichnet, etwa bei einem Desktop-Pet oder einem Widget; ein View mit `Desktop.WindowDrag` bewegt das Fenster. `NoShadow` entfernt den Systemschatten und `SkipTaskbar` hält das Fenster aus der Windows-Taskleiste, Alt+Tab und der X11-Fensterliste heraus; `desktop.SetWindowTransparent`, `SetWindowShadow` und `SetSkipTaskbar` ändern diese Einstellungen später. Windows ändert die Transparenz nur bei einem transparent geöffneten Fenster, das keinen Schatten hat, und Wayland besitzt keinen Hinweis, um ein Fenster aus seiner Taskliste fernzuhalten. `desktop.CanMakeWindowsTransparent` und `desktop.CanSkipTaskbar` geben im Scope eines Fensters an, ob dies wirksam ist: Transparenz erfordert DirectComposition unter Windows und einen Compositor unter Linux, da X11 ohne Compositor keine bietet, und unter macOS nimmt `desktop.SetDockVisible` die App aus dem Dock.

## Terminal

Importieren Sie `base/terminal` und erstellen Sie eine Sitzung mit `terminal.New(terminal.Options{Conn: conn})` für einen entfernten Datenstrom, oder setzen Sie `Command` für ein lokales Programm unter macOS, Linux oder Windows. Rendern Sie `terminal.View{Terminal: session}` und rufen Sie `Close` auf, wenn die Sitzung endet. `Options.ImageLimit` begrenzt den Kitty-Grafikspeicher in Bytes: Null verwendet 64 MiB, ein negativer Wert deaktiviert Bilder. Programme können RGB-, RGBA- oder PNG-Bilder sowie Unicode-Platzhalterpositionierungen senden.

Rufen Sie `Search(text)` auf, um Treffer auf dem Bildschirm und im Scrollback hervorzuheben, `SearchNext` und `SearchPrevious`, um sie auszuwählen, und `EndSearch`, um sie zu löschen. `Options.OnSearch` meldet `SearchStatus`; `Theme.Search`, `SearchText`, `SearchSelected` und `SearchSelectedText` legen die Farben der Treffer fest. Shells, die OSC-133-Markierungen ausgeben, unterstützen Alt/Option-Klick innerhalb ihrer aktiven Befehlseingabe, um den Cursor zu versetzen. NSGo installiert keine Shell-Integrationsskripte.
