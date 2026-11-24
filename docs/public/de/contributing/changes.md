---
title: Eine Änderung vornehmen
description: Was ein Pull Request für NSGo braucht – von derselben Änderung in jedem Backend bis zu Tests, Dokumentation und einem Changelog-Eintrag.
group: Mitwirken
order: 82
---

## Bevor Sie beginnen

Eröffnen Sie bei allem, was über eine kleine Fehlerbehebung hinausgeht, zunächst ein Issue oder eine Discussion, damit das Design vor dem Code abgestimmt ist. Offene Designfragen sind in [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions) aufgeführt; ihre Antworten werden dort zuerst festgelegt.

## Jedes Backend

Eine Fehlerbehebung oder Funktion in einem Backend gehört in aller Regel auch in die anderen. Nehmen Sie dieselbe Änderung auf jeder Zielplattform vor, die sie benötigt, oder dokumentieren Sie, was fehlt, in der [Roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Komponenten sehen auf jeder Plattform gleich aus und verhalten sich gleich.

## Prüfungen

Verwenden Sie das Werkzeug `nsgo` statt `go vet`, `go test` oder `go build`:

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Fügen Sie für jede Änderung einen Test hinzu: `nsgotest` für alles, was Nutzerinnen und Nutzer sehen, und die eigenen Tests des Pakets für alles Übrige. Starten Sie die Gallery auf den Plattformen, die Sie geändert haben ([Entwicklungsumgebung einrichten](../contributing.md#die-gallery)).

Formatieren Sie Go-Dateien mit `gofmt` aus der Go-Version in `go.mod`; `gofmt -l .` listet die Dateien auf, die formatiert werden müssen. Die CI prüft jede getrackte Go-Datei und schlägt mit den Namen der nicht formatierten Dateien fehl.

## Der Pull Request

Benennen Sie den Titel als Conventional Commit, etwa `fix(linux): keep the caret after a paste`: Ein Squash-Merge macht den Titel zur Commit-Betreffzeile, und eine Prüfung schlägt fehl, bis er passt. Die Vorlage fragt nach den berührten Zielplattformen, Vorher-Nachher-Screenshots bei sichtbaren Änderungen sowie den ausgeführten Prüfungen.

## Dokumentation und Changelog

- Aktualisieren Sie das Dokument, dessen Fakten sich ändern: ein Thema unter `docs/architecture/` für die innere Funktionsweise von NSGo und eine Seite unter `docs/public/` dafür, wie Apps und Komponenten es verwenden ([Dokumentation und Website](docs.md)).
- Halten Sie `README.md` und `README.zh-CN.md` gleichwertig; alles Weitere liegt auf Englisch vor, mit chinesischen Seiten unter `docs/public/zh/`.
- Dokumentieren Sie eine für Nutzerinnen und Nutzer sichtbare Änderung als eine Zeile mit einer Referenz wie `(#12 by @you)` in einer eigenen Datei unter `changelog.d/` unter der passenden Überschrift wie `### Fixed` – statt direkt in `CHANGELOG.md`, die bei einem Release aus diesen Dateien befüllt wird ([Format](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
