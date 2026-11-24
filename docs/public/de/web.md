---
title: Im Web veröffentlichen
description: Eine NSGo-App mit Dokumentsemantik, Seitenmetadaten und CDN-Assets veröffentlichen.
group: Toolchain
order: 24
---

## Die App bauen

```sh
go tool nsgo build -target web
```

Stellen Sie das erzeugte Verzeichnis über HTTP bereit. Release-Assets tragen Inhalts-Hashes in ihren Namen, cachen Sie sie also als unveränderlich; `index.html`, `nsgo.json` und `manifest.webmanifest` sollten erneut validiert werden. Die Web-App behält das Layout-, Scroll-, Fokus- und Zeigerverhalten von NSGo bei.

## Überschriften und Links

Eine `core.View` oder ein `core.Text` mit `Role: core.RoleHeader` wird zu `h1` bis `h6`, ausgewählt über `AccessibilityLevel`; ohne Ebene wird daraus `h2`. Markdown-Überschriften aus `base/textview` tragen ihre Ebenen mit. Die Überschriftenabstände und Schriften des Browsers ändern das Styling der App nicht.

`base.Link` und `ui.Link` veröffentlichen ihr `Href` als Anker. Ein gewöhnlicher Klick führt `OnOpen` und `OnPress` der App aus; ein Mittelklick oder ein Klick mit Modifikatortaste verwendet die Link-Aktion des Browsers. Der Browser stellt sein Link-Kontextmenü bereit, sofern die App `OnContextMenu` nicht behandelt. Ein deaktivierter Link hat keine aktive Adresse. Für eine eigene View mit Link-Rolle setzen Sie `core.View.Web` auf `core.WebViewProps{Href: "/notes"}` oder binden `HrefFunc`. TSX verwendet `web={{href: "/notes"}}`; Überschriften verwenden `accessibilityLevel={1}`. Unsichere ausführbare URL-Schemata werden abgelehnt. Eingegrenzte Inhalte halten die Navigation in ihren eigenen Press-Handlern und geben keine Browser-Adresse bekannt.

## Seitenmetadaten

Importieren Sie `github.com/nsgo-dev/nsgo/web` und platzieren Sie einen `web.Head` neben dem Inhalt einer Seite:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` und `DescriptionFunc` folgen Signals. Später sichtbare Heads überschreiben die Felder, die sie setzen, und erben die übrigen. Das Ausblenden oder Entfernen einer Seite stellt frühere Heads wieder her; das Entfernen aller Heads stellt das ursprüngliche Dokument wieder her. Ein leeres statisches Feld erbt; eine Funktion, die einen leeren String zurückgibt, leert es. Ein Head fügt keinen Layout-Knoten hinzu. Native Zielplattformen, eingebettete Apps und eingegrenzte Inhalte lassen das Dokument ihres Hosts unverändert.

Importieren Sie in TSX `Head` aus `nsgo/web` und schreiben Sie `<Head title={title()} description="Your saved notes" />`. Für statische HTML-Vorschauen rendern Sie jede URL im Rahmen des Deployments der Website vorab; `Head` aktualisiert das laufende Dokument.

## Assets von einem CDN ausliefern

Der Loader löst alle App-Dateien relativ zu `nsgo.json` auf. Eine Seite kann die App von einem anderen Origin laden:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Erlauben Sie dem Origin der Seite per CORS, die Dateien des CDN abzurufen, und liefern Sie WebAssembly als `application/wasm` aus. Liefern Sie das Einstiegs-HTML der App für alle tiefen Routen aus, die Ihre App verwendet. [Einbetten](embedding.md#web) zeigt, wie Sie die App stattdessen in ein Element einhängen.
