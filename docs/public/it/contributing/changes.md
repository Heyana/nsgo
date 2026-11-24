---
title: Apportare una modifica
description: Cosa serve in una pull request per NSGo, dalla stessa modifica su ogni backend fino a test, documentazione e changelog.
group: Contribuire
order: 82
---

## Prima di iniziare

Apri una issue o una discussione per qualsiasi intervento più ampio di una semplice correzione, così da concordare il design prima di scrivere il codice. Le questioni lasciate aperte dal design sono elencate in [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions); le relative risposte vanno decise lì prima di tutto.

## Ogni backend

Una correzione o una funzionalità in un backend riguarda solitamente anche gli altri. Apporta la stessa modifica su ogni target che la richiede, oppure annota ciò che manca nella [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). I componenti hanno lo stesso aspetto e si comportano allo stesso modo su ogni piattaforma.

## Verifiche

Usa lo strumento `nsgo` invece di `go vet`, `go test` o `go build`:

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Aggiungi un test per ciò che modifichi: `nsgotest` per ciò che vede l'utente, i test propri del pacchetto per il resto. Esegui la galleria sulle piattaforme interessate ([Configurazione dell'ambiente di sviluppo](../contributing.md#the-gallery)).

Formatta i file Go con `gofmt` della versione di Go indicata in `go.mod`; `gofmt -l .` elenca i file che ne hanno bisogno. La CI controlla ogni file Go tracciato e fallisce riportando i nomi di quelli non formattati.

## La pull request

Dai al titolo il formato di un Conventional Commit, come `fix(linux): keep the caret after a paste`: uno squash merge usa il titolo come oggetto del commit, e un controllo automatico fallisce finché non rispetta il formato. Il template chiede i target toccati dalla modifica, gli screenshot prima e dopo per qualsiasi cambiamento visibile e le verifiche eseguite.

## Documentazione e changelog

- Aggiorna il documento i cui fatti cambiano: un argomento in `docs/architecture/` per il funzionamento interno di NSGo, una pagina in `docs/public/` per il modo in cui app e componenti lo usano ([Documentazione e sito web](docs.md)).
- Mantieni `README.md` e `README.zh-CN.md` equivalenti; tutto il resto è in inglese, con le pagine in cinese in `docs/public/zh/`.
- Registra una modifica visibile agli utenti in una sola riga, con un riferimento come `(#12 by @you)`, in un file dedicato dentro `changelog.d/` sotto l'intestazione del tipo appropriato, come `### Fixed`, anziché in `CHANGELOG.md`, che viene compilato al momento del rilascio a partire da quei file ([formato](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
