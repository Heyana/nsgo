---
title: Lokalisierung
description: Die Meldungen einer App übersetzen und der Sprache folgen, die sie anzeigt.
group: Kernkonzepte
order: 62
---

## Meldungen

`locales/` neben `nsgo.yaml` enthält die Meldungen der App, eine YAML-Datei pro Sprache (`en.yaml`, `de.yaml`) im ICU-MessageFormat:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Eine Meldung ist ein String oder eine `message` mit einer `description` und `placeholders` für Übersetzerinnen und Übersetzer.

Das Package importiert `locales/`. `dev`, `build` und `test` prüfen die Kataloge und schreiben daneben eine Go-Funktion pro Meldung, etwa `locales.Greeting(cx, "Ada")`, die sie in der Sprache ausgibt, die die App anzeigt, und dieser folgt, wenn sie sich ändert; `go tool nsgo i18n` schreibt nur die Funktionen. Meldungen formatieren Zahlen und Datumsangaben (`{d, date, medium}`), und `appName` benennt die App in jeder Sprache in ihren Paketen, so wie eine Meldung mit einem `apple.info`-Schlüssel, etwa `NSCameraUsageDescription`, diesen Info.plist-Eintrag übersetzt ([Berechtigungen](permissions.md#usage-descriptions)).

## Die Sprache

`cx.SetLocale("de")` wählt die Sprache, und das Layout verläuft in der Richtung, in der sie geschrieben wird. In einer eigenständigen App folgt auch das, was das System selbst anzeigt – etwa Textauswahlmenüs, Dateidialoge und die Widgets von GTK –, der erzwungenen Sprache (sofort unter Android 13+, Windows und Linux, und ab dem nächsten Start unter macOS und iOS); `cx.SetLocale("")` folgt wieder der Sprache der Nutzerin oder des Nutzers.

```sh
go tool nsgo dev -lang en-XA
```

führt die App in einem Pseudo-Gebietsschema aus, das unübersetzten und abgeschnittenen Text sichtbar macht. Jeder Build nimmt es beim Start an: `--nsgo-lang=en-XA`.

## Wochen, Uhrzeiten und Maßeinheiten

`i18n.Regional(cx)` gibt an, wie die Nutzerin oder der Nutzer Wochen, Uhrzeiten und Maßeinheiten liest, unabhängig von der Sprache: den Tag, mit dem Wochen beginnen, 12- oder 24-Stunden-Format, das Maßsystem und die Temperatureinheit, so wie sie auf der Plattform eingestellt sind, andernfalls so, wie es ihre Region vorgibt. Kalender und Datumsauswahlen beginnen ihre Wochen dort, und Zeitfelder zeigen dieses Stundenformat, sofern die App nicht `FirstDayOfWeek` oder `HourCycle` setzt, und die Uhrzeiten, die die Meldungen eines Katalogs ausgeben, folgen dem Stundenformat der Nutzerin oder des Nutzers.

## Die eigenen Texte von NSGo

Die Komponenten von NSGo geben ihre Texte in den Sprachen aus, in die NSGo übersetzt ist. Für eine andere Sprache liefert ein Schlüssel, der ein Package benennt, etwa `github.com/nsgo-dev/nsgo/base/locales`, die Meldungen dieses Packages in der Sprache der Datei. Die Menüs der Plattformen sowie die Rollen und Zustände, die die Screenreader von Android und Windows ansagen, wenn das System kein eigenes Wort dafür hat – etwa für einen Tab oder ein Suchfeld –, verwenden die Texte aus `github.com/nsgo-dev/nsgo/desktop/locales`.
