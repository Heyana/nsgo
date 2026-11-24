---
title: La codebase
description: Dove si trova ogni parte di NSGo e i documenti che spiegano il perché.
group: Contribuire
order: 81
---

NSGo è un unico modulo Go. Le app importano i suoi pacchetti dall'alto; il backend di ciascuna piattaforma si trova in basso e riceve ciò che è cambiato sotto forma di viste native da creare, aggiornare e rimuovere.

## Pacchetti

| Pacchetto | Cosa contiene |
| --- | --- |
| `nsgo` (la radice) | `nsgo.Run` e `nsgo.NewView`, con selezione del backend tramite build tag |
| `core` | L'API dell'app: elementi, stili, classi, temi, signal e servizi; layout e preparazione del testo |
| `base` | Componenti privi di stile, un file ciascuno, e quelli complessi come pacchetti (`base/dock`, `base/editor`) |
| `ui` | I componenti di `base` dotati di stile e pacchetti come `ui/chat` |
| `mobile`, `desktop` | Ciò che riguarda solo i telefoni o solo i desktop |
| `tsx` | App TSX, il relativo bundler e compilatore e i moduli che importano |
| `i18n`, `*/locales` | Cataloghi dei messaggi e codice generato a partire da essi |
| `nsgotest` | Test che agiscono come un utente, in modalità headless o su una piattaforma |
| `internal/engine`, `internal/driver`, `internal/layout` | La goroutine del motore, il protocollo di montaggio e il layout |
| `internal/platform/<target>` | Un backend: viste native, testo e input per un singolo target |
| `cmd/nsgo` | Lo strumento `nsgo` |
| `examples/` | La galleria e le altre app di esempio |

I pacchetti importano solo verso il basso; `internal/moduletest/layering_test.go` verifica il grafo delle dipendenze su ogni target.

## Documenti di design

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) mappa i pacchetti, i thread e i flussi da un commit fino allo schermo, ed elenca gli argomenti in `docs/architecture/`: uno per ogni parte, come layout, testo, scorrimento, temi o lo strumento. Leggi l'argomento relativo a ciò che modifichi prima di intervenire; documenta come funziona quella parte e dove si discosta da [Native Shell](https://github.com/longbridge/nsh), di cui NSGo effettua il porting progettuale.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) raccoglie le regole seguite da ogni modifica, tanto per le persone quanto per gli agenti di coding. Alcune guidano la maggior parte degli interventi:

- Ogni API ha una sola sede: `core` per elementi e ambiente, `base` per i controlli, `ui` per il loro aspetto, `mobile` e `desktop` per una singola categoria di dispositivi.
- `base` non ha aspetto grafico; un componente `ui` è la sua forma `base` dotata di stile.
- Nulla cambia implicitamente in base al dispositivo: niente controlli più grandi su desktop o più piccoli su telefono in `core` o `base`.
- Viste native ovunque, identiche su ogni piattaforma: ciò che una piattaforma ha nativamente, le altre lo imitano.
- Nessun codice C scritto a mano e nulla oltre a macOS, Xcode e Go per compilare ogni target, fatta eccezione per l'emulatore Android x86_64.

Ciò che manca, piattaforma per piattaforma, è riportato nella [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md).
