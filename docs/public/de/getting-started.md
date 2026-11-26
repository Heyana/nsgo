---
title: Einrichtung
description: Installieren Sie, was NSGo benötigt, erstellen Sie eine App und führen Sie sie auf jeder Zielplattform aus.
group: Erste Schritte
order: 10
---

## Voraussetzungen

- [Go](https://go.dev/dl/) 1.27.3 oder neuer.
- Für Builds für iOS, tvOS und watchOS ein Mac mit Xcode. Jede andere Zielplattform lässt sich unter macOS allein mit Go bauen; die x86_64-Android-Emulatoren benötigen zusätzlich ein Android NDK.
- Zur Ausführung auf einem iPhone eine Apple-ID in den Accounts von Xcode sowie ein als vertrauenswürdig eingestuftes iPhone mit aktiviertem Entwicklermodus.
- Zur Ausführung unter Android `adb` aus den platform-tools des Android SDK.
- Zur Ausführung unter Windows allein Go, das auch die Windows- und Web-Pakete baut.
- Unter Linux GTK 4.14 oder neuer, Pango und Cairo. Netzwerkbilder verwenden libsoup 3, Web-Views verwenden WebKitGTK 6.0 und Audio sowie Video verwenden GStreamer-Plug-ins. Auf Systemen mit AppArmor-Einschränkungen für User-Namespaces [benötigt WebKitGTK ein bubblewrap-Profil](debugging.md#webviews-unter-linux).

## Eine App erstellen

Legen Sie ein Modul an, fügen Sie NSGo und sein Tool hinzu und lassen Sie anschließend das Tool eine App schreiben:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` fügt `nsgo` einmalig zum Modul hinzu, sodass `go tool nsgo` es ausführt, ohne etwas zu installieren. `init` schreibt:

| Datei | Beschreibung |
| --- | --- |
| `app.go` | Die App, ein Zähler: eine `Root`-Komponente und `main`, die `nsgo.Run(Root)` aufruft |
| `app_test.go` | Ein Test, der den Button drückt |
| `nsgo.yaml` | Name, ID, Version und Zielplattformen der App |

`go tool nsgo init -template tsx` schreibt stattdessen eine TSX-App, und `-template mixed` eine Go-App, deren TSX-Einstiegspunkt eigene Go-Komponenten verwendet.

Fügen Sie Bibliotheken mit `nsgo add <modul>` statt mit `go get` hinzu: Es zeigt vor dem Schreiben, was `go.mod` und `go.sum` erhalten würden, und verschiebt das NSGo des Projekts nie weg von seinen Releases ([Bibliotheken hinzufügen](building.md#bibliotheken-hinzufügen)).

### Vorlagen aus Go-Modulen

Übergeben Sie den Modulpfad der Vorlage mit einer optionalen Version. Eine weggelassene Version wird auf die neueste Version aufgelöst, die der Modul-Proxy von Go finden kann.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Veröffentlichen Sie das Starter-Projekt im Verzeichnis `template/` des Moduls. Fügen Sie `nsgo.yaml` und `main.go` hinzu. Das Tool rendert beide sowie jede Datei mit der Endung `.tmpl` über Gos `text/template` und entfernt `.tmpl` aus den Ausgabenamen. Andere Dateien, einschließlich binärer Assets, kopiert es unverändert.

Vorlagen erhalten `.Name` (den Anzeigenamen der App), `.ID` (die generierte App-ID), `.Slug` (den kleingeschriebenen Namen mit Bindestrichen) und `.Module` (den Wert von `-module` oder den Slug). Liefern Sie eine `go.mod.tmpl` mit, um das Modul und die Abhängigkeiten der generierten App zu deklarieren, da Go verschachtelte `go.mod`-Dateien von Modul-Downloads ausschließt. Ohne diese Datei schreibt `-module` dieselbe minimale `go.mod` wie bei einer integrierten Vorlage.

## Ausführen

```sh
go tool nsgo dev
```

`dev` baut die App für diesen Rechner und öffnet sie. Bei jedem Speichern wird die App neu gebaut und neu gestartet, und eine reine TSX-Änderung aktualisiert die laufende App direkt an Ort und Stelle. Ein fehlgeschlagener Build wird über der App angezeigt, mit Buttons zum Kopieren des Fehlers und zum erneuten Bauen.

Führen Sie sie mit `-target` woanders aus:

```sh
go tool nsgo dev -target web                # im Browser
go tool nsgo dev -target ios_simulator      # im iOS-Simulator
go tool nsgo dev -target tvos_simulator     # im Apple-TV-Simulator
go tool nsgo dev -target watchos_simulator  # im Apple-Watch-Simulator
go tool nsgo dev -target android            # auf einem verbundenen Gerät oder Emulator
```

`go tool nsgo doctor` listet jede Zielplattform mit der Angabe auf, ob dieser Rechner sie baut und ausführt, und was für jene fehlt, bei denen dies nicht möglich ist – etwa Xcode, `adb`, ein verbundenes Gerät oder GTK.

[Ausführen und Debuggen](debugging.md) behandelt Geräte, das Nachverfolgen von Updates, den Performance-Monitor und andere Sprachen.

## Mit einem Coding-Agenten

Der Skill von NSGo vermittelt Coding-Agenten wie Claude Code, Codex und Cursor, wie eine NSGo-App geschrieben, getestet und ausgeliefert wird. Fügen Sie ihn dem Verzeichnis der App hinzu – oder mit `-g` für jedes Projekt:

```sh
bunx skills add nsgo-dev/nsgo
```

Agenten, die die Dokumentation selbst lesen, beginnen bei <https://nsgo.dev/llms.txt>, die jede Seite als Markdown unter `https://nsgo.dev/docs/<page>.md` auflistet; <https://nsgo.dev/llms-full.txt> enthält alle Seiten. Ein Agent kann auch die laufende App nutzen ([Ausführen und Debuggen](debugging.md#agents)).

## Weiter

- [Beispiel: Eine Todo-App](tutorials/todo.md): Zustand, eine Liste und ein Test in etwa zehn Minuten.
- [Beispiel: Eine Notizen-App](tutorials/notes.md): ein Navigations-Stack und Notizen, die auf dem Gerät gespeichert werden.
- [Beispiel: Beliebte GitHub-Repositories](tutorials/repos.md): Laden aus dem Netzwerk mit Fehlern und einem erneuten Versuch.
