---
title: Introduzione
description: Che cos'è NSGo, su cosa gira e come funziona.
order: 1
---

NSGo è un framework di UI nativa multipiattaforma scritto in Go. Scrivi un'app una volta sola, in Go o in TSX, e questa gira su iOS e sulle sue App Clip, tvOS, watchOS, Android, macOS, Windows, Linux e il web, disegnata con le viste proprie di ciascuna piattaforma. Le app Go girano anche, in via sperimentale, nei mini program di WeChat, QQ, Alipay e Douyin: queste piattaforme limitano un'app a 2 MB, che le app più grandi superano. HarmonyOS esegue le app Go in anteprima, disegnate da ArkUI.

## Cosa scrivi

Un'app è un package main di Go. I suoi componenti restituiscono elementi: viste, testo, immagini, campi di input e i controlli di `ui`. Lo stato risiede nei signal, e quando un signal cambia si aggiorna solo ciò che lo legge.

```go
func Root(cx *core.Scope) core.Node {
	count := core.NewSignal(cx, 0)
	return core.View{Style: core.Style{Padding: core.All(24), Gap: 12}, Children: []core.Node{
		core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }},
		ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }},
	}}
}
```

La stessa app può essere scritta in TSX e stilizzata con le classi di Tailwind ([App TSX](tsx.md)).

## Come funziona

Un motore Go gestisce l'app: il grafo reattivo, gli stili, il layout flexbox e grid e la preparazione del testo. Ogni aggiornamento produce un unico lotto di modifiche, e un sottile backend lo applica alle viste native della piattaforma. Le animazioni girano sul clock dei frame della piattaforma, quindi nulla viene confermato a ogni frame.

| Piattaforma | Disegna con |
| --- | --- |
| iOS | UIKit e CoreText |
| Android | Android Views |
| macOS | AppKit e CoreText |
| Windows | Direct2D e DirectWrite |
| Linux | GTK 4, Pango e Cairo |
| Web | WebAssembly ed elementi DOM |

Ogni target si compila su un Mac con Go; iOS richiede anche Xcode. Lo strumento `nsgo` crea, esegue, testa e impacchetta le app.

## Stato del progetto

NSGo è agli inizi: 0.1.0 è la sua prima release, la sua API non è ancora stabile e ciò che manca è nella [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Porta in Go il design di [Native Shell](https://github.com/longbridge/nsh).

## Come leggere la documentazione

Ogni pagina risponde a una domanda. Un esempio di codice è un frammento di un componente, salvo diversa indicazione della pagina: `cx` è lo `*core.Scope` che un componente riceve. Le app usano i componenti di `ui`, che hanno un aspetto definito; `base` contiene gli stessi controlli senza stile, per gli autori di componenti.

- **Per iniziare**: la [configurazione](getting-started.md), poi app di esempio costruite passo dopo passo, [un'app di attività](tutorials/todo.md), [un'app di note](tutorials/notes.md) e [repository GitHub popolari](tutorials/repos.md).
- **Strumenti**: [esecuzione e debug](debugging.md), [test](testing.md), [compilazione e distribuzione](building.md) e [integrazione](embedding.md).
- **Componenti**: [i componenti più usati](ui.md), con le loro varianti, i loro stati e la loro accessibilità.
- **Concetti chiave**: [componenti e stato](components.md), [layout](layout.md), [stili](styles.md), [testo](text.md), [liste](lists.md), [navigazione](navigation.md), [moduli](forms.md), [localizzazione](localization.md), [TSX](tsx.md) e il resto.
- **Funzionalità di piattaforma**: [servizi di piattaforma](services.md), [notifiche](notifications.md), [feedback aptico](haptics.md), [widget](widgets.md), [Live Activities](live-activities.md) e [permessi](permissions.md).
- **Scrivere componenti**: [componenti per le app di altri](authoring/index.md), senza stile e con stile.
- **Contribuire**: [compila NSGo dai sorgenti](contributing.md), orientati al suo interno e invia una modifica.
- La [demo dal vivo](https://nsgo.dev/gallery/): ogni componente, nel tuo browser.
