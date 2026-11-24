---
title: Documentazione e sito web
description: Scrivere una pagina di questa documentazione, tradurla, eseguire il sito web e mantenere aggiornata la skill per gli agenti.
group: Contribuire
order: 83
---

## Una pagina

Questa documentazione è composta dai file Markdown in `docs/public/`, leggibili direttamente su GitHub così come sono. `en/` e `zh/` si rispecchiano percorso per percorso; una pagina mancante in cinese mostra quella in inglese con un avviso. Ogni pagina inizia con il front matter:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` è il gruppo della pagina nella barra laterale, nella lingua della pagina, e `order` la posiziona. I gruppi seguono il percorso del lettore: tutorial, strumenti, componenti, concetti chiave, poi scrittura dei componenti e contribuzione. Una pagina senza gruppo, come l'introduzione, precede gli altri.
- Il layout mostra il titolo e la descrizione, quindi il corpo inizia da `##`.

Collega le pagine tra loro con link `.md` relativi (`[Test](testing.md#run-on-a-platform)`); il sito li converte nei propri URL. Ogni pagina risponde a una singola domanda; i dettagli sul funzionamento interno di NSGo restano in `docs/architecture/`.

## Il sito web

Il sito web in `website/` è compilato tramite [nsgo-website](https://github.com/nsgo-dev/nsgo-website), un'integrazione Astro, a partire dalle impostazioni in `website/site.config.ts`. Richiede [Bun](https://bun.com) 1.4 o successivo:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` spiega come il sito viene compilato e pubblicato.

## Per gli agenti

Il sito web pubblica anche le pagine in inglese per i modelli linguistici: `/llms.txt` le elenca, `/llms-full.txt` le contiene tutte e `/docs/<page>.md` contiene una singola pagina, con link assoluti e senza i marcatori dei blocchi di codice (`src/lib/llms.ts`). Seguono automaticamente le pagine senza alcun passaggio separato.

`skills/nsgo/` è la skill che gli agenti installano con `bunx skills add nsgo-dev/nsgo`: `SKILL.md` contiene le regole seguite da un'app, e `references/` viene letto quando un'attività lo richiede. Aggiornala nello stesso intervento della documentazione quando cambiano ciò che un'app scrive, i comandi dello strumento, i test o il packaging; limitati a ciò che un agente deve seguire e rimanda alla documentazione per il resto. `go tool nsgo test -run TestSkills .` ne verifica il front matter, i link e ogni nome di NSGo menzionato.
