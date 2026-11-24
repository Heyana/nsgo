---
title: Anwendungsadressen
description: Die Adresse einer Web-App über den Browserverlauf auslesen und aktualisieren.
group: Kernkonzepte
order: 59
---

`cx.AppURL()` liest die Adresse der Anwendung als verfolgten Wert (`tracked value`). Im Web steht die anfängliche Seiten-URL bereits vor dem Rendern des Wurzelelements zur Verfügung, sodass ein Neuladen oder ein geteilter Link dieselbe Ansicht auswählen kann. Die App entscheidet selbst, wie sie die URL auf ihre Ansichten abbildet.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` aktualisiert die verfolgte Adresse sofort, ohne eine Seite neu anzufordern oder einen Navigationsstapel zu verschieben. Standardmäßig ersetzt es den aktuellen Verlaufseintrag des Browsers; `HistoryPush` fordert einen neuen Eintrag an. Ein Navigations- oder Modal-Push im selben Commit liefert stattdessen diesen Eintrag, sodass eine Aktion keine zwei Zurück-Schritte erzeugt. Mehrere Schreibvorgänge in einem Commit behalten die letzte Adresse und bewahren jede Push-Anforderung.

Relative Adressen werden gegen die aktuelle Adresse oder eine explizite HTML-Basis aufgelöst. Eine Browser-Adresse muss auf dem Origin der Seite bleiben. Ungültige Adressen, Leerzeichen, Steuerzeichen, Backslashes und Adressen über 16 KiB geben einen Fehler zurück, ohne die Adresse zu ändern.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` beobachtet Verlaufsnavigationen des Browsers und Änderungen am Hash. `change.Reason` ist `"back"`, `"forward"` oder `"address"`; die zurückgegebene Funktion meldet den Listener ab, und beim Verwerfen des Scopes wird der Listener ebenfalls entfernt. Lesen Sie `AppURL` für die Anfangsadresse. Eigene Schreibvorgänge der App aktualisieren Bindings, lösen aber keine Änderungsbenachrichtigung aus. Eine Verlaufsnavigation, bei der die Adresse gleich bleibt, sendet keine Änderung.

Die Zurück-Schaltfläche des Browsers schließt weiterhin einen modalen Dialog oder entfernt den obersten Bildschirm aus einem Navigationsstapel. Ein von der App abgelehntes Zurück stellt den bestehenden Eintrag samt seiner Adresse wieder her. Ein Handler beim Schließen kann eine eigene Ersatzadresse setzen. Ein programmatisches Pop aktualisiert die Adresse nach dem asynchronen Durchlaufen des Verlaufs, und Schreibvorgänge, die während dieses Vorgangs eintreffen, warten darauf. Ein Zurückgehen über die Einträge dieses Players hinaus verlässt die App – einschließlich Einträgen, die aus einem früheren Ladevorgang erhalten blieben.

Für Pfade wie `/films/42` legen Sie das Verzeichnis der bereitgestellten App fest und konfigurieren den Server so, dass er für Anwendungsrouten deren `index.html` ausliefert:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` schreibt eine HTML-Basis, sodass Skripte, Styles, Wasm und Ressourcen auch dann noch geladen werden, wenn eine tiefe Adresse direkt geöffnet wird. Ohne diese Einstellung bleiben die vorhandenen relativen Asset-Pfade erhalten; Adressen mit Query-Parametern oder Hashes können diese unverändert nutzen. Fallback-Regeln des Servers müssen echte Asset-Anfragen unangetastet lassen. Eine strikte Content Security Policy muss diese Same-Origin-Basis erlauben (`base-uri 'self'`).

Eine in eine andere Seite eingebettete App verwaltet ihre Adresse intern und lässt den Verlauf der einbettenden Seite unangetastet. Ein Same-Origin-Iframe kann sich über `data-nsgo-history` explizit dafür anmelden. Eingegrenzte Teilbäume (`confined subtrees`) können die Adresse der Anwendung weder lesen noch ändern.

Auf anderen Zielplattformen speichert `AppURL` den zuletzt mit der App geöffneten Link oder den von `SetAppURL` abgelegten Wert; das Setzen verändert weder ein natives Fenster noch einen Systembrowser. Mini-Programme behalten diesen Wert ebenfalls, ohne die Routen ihres Hosts zu ändern.

TSX nutzt denselben Zustand:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
