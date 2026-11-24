---
title: Apporter une modification
description: Ce que requiert une pull request sur NSGo, de la cohérence entre tous les backends aux tests, à la documentation et à l'entrée du changelog.
group: Contribuer
order: 82
---

## Avant de commencer

Ouvrez une issue ou une discussion pour tout ce qui dépasse un simple correctif, afin que la conception soit approuvée avant l'écriture du code. Les questions que la conception laisse ouvertes sont listées dans [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions) ; leurs réponses y sont tranchées en premier lieu.

## Chaque backend

Un correctif ou une fonctionnalité dans un backend a généralement sa place dans les autres. Apportez la même modification sur chaque cible qui en a besoin, ou consignez ce qui manque dans la [feuille de route](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Les composants ont la même apparence et le même comportement sur toutes les plateformes.

## Vérifications

Utilisez l'outil `nsgo` plutôt que `go vet`, `go test` ou `go build` :

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Ajoutez un test pour ce que vous modifiez : `nsgotest` pour ce que voit l'utilisateur, les propres tests du paquet pour le reste. Exécutez la galerie sur les plateformes que vous avez touchées ([Environnement de développement](../contributing.md#la-galerie)).

Formatez les fichiers Go avec `gofmt` de la version de Go indiquée dans `go.mod` ; `gofmt -l .` liste les fichiers qui en ont besoin. La CI vérifie chaque fichier Go suivi et échoue en affichant les noms de ceux qui ne sont pas formatés.

## La pull request

Donnez-lui un titre au format Conventional Commit, par exemple `fix(linux): keep the caret after a paste` : une fusion par squash fait du titre le sujet du commit, et une vérification échoue tant qu'il ne correspond pas. Le modèle de PR demande les cibles concernées par la modification, des captures d'écran avant et après pour tout changement visible, et les vérifications que vous avez exécutées.

## Documentation et changelog

- Mettez à jour le document dont les faits changent : un sujet dans `docs/architecture/` pour le fonctionnement interne de NSGo, une page dans `docs/public/` pour la manière dont les applications et les composants l'utilisent ([La documentation et le site web](docs.md)).
- Gardez `README.md` et `README.zh-CN.md` équivalents ; tout le reste est en anglais, avec des pages en chinois dans `docs/public/zh/`.
- Consignez tout changement visible par les utilisateurs en une ligne, avec une référence telle que `(#12 by @you)`, dans un fichier dédié dans `changelog.d/` sous l'en-tête de son type, tel que `### Fixed`, plutôt que dans `CHANGELOG.md`, qu'une publication remplit à partir de ces fichiers ([format](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
