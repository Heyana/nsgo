---
title: Localisation
description: Traduire les messages d’une application et suivre la langue qu’elle affiche.
group: Concepts clés
order: 62
---

## Messages

`locales/` à côté de `nsgo.yaml` contient les messages de l’application, un fichier YAML par langue (`en.yaml`, `de.yaml`) au format ICU MessageFormat :

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Un message est une chaîne, ou un objet `message` accompagné d’une `description` et de `placeholders` pour les traducteurs.

Le paquet importe `locales/`. `dev`, `build` et `test` vérifient les catalogues et génèrent une fonction Go par message à côté d’eux, comme `locales.Greeting(cx, "Ada")`, qui l’énonce dans la langue affichée par l’application et s’adapte lorsqu’elle change ; `go tool nsgo i18n` génère uniquement les fonctions. Les messages mettent en forme les nombres et les dates (`{d, date, medium}`), et `appName` nomme l’application dans chaque langue au sein de ses paquets, tout comme un message doté d’une clé `apple.info`, telle que `NSCameraUsageDescription`, traduit cette entrée Info.plist ([autorisations](permissions.md#descriptions-dutilisation)).

## La langue

`cx.SetLocale("de")` choisit la langue, et la mise en page s’effectue dans son sens d’écriture. Dans une application autonome, ce que le système affiche lui-même — comme les menus de sélection de texte, les boîtes de dialogue de fichiers et les widgets de GTK — suit également la langue forcée (immédiatement sous Android 13+, Windows et Linux, et dès le lancement suivant sous macOS et iOS) ; `cx.SetLocale("")` suit de nouveau la langue de l’utilisateur.

```sh
go tool nsgo dev -lang en-XA
```

lance l’application dans une pseudo-locale qui met en évidence les textes non traduits ou tronqués. Toute compilation l’accepte au démarrage : `--nsgo-lang=en-XA`.

## Semaines, heures et mesures

`i18n.Regional(cx)` indique la manière dont l’utilisateur lit les semaines, les heures et les mesures, indépendamment de la langue : le premier jour de la semaine, le format sur 12 ou 24 heures, le système de mesure et l’unité de température, tels qu’il les a configurés sur la plateforme, ou à défaut selon sa région. Les calendriers et sélecteurs de date commencent leur semaine ce jour-là et les champs d’heure affichent ce format horaire, sauf si l’application définit `FirstDayOfWeek` ou `HourCycle`, et les heures écrites par les messages d’un catalogue suivent les préférences horaires de l’utilisateur.

## Les textes propres à NSGo

Les composants de NSGo s’expriment dans les langues vers lesquelles NSGo est traduit. Pour une autre langue, une clé nommant un paquet, comme `github.com/nsgo-dev/nsgo/base/locales`, fournit les messages de ce paquet dans la langue du fichier. Les menus des plateformes, ainsi que les rôles et états annoncés par les lecteurs d’écran d’Android et de Windows lorsque le système ne possède pas son propre terme, comme un onglet ou un champ de recherche, utilisent les termes de `github.com/nsgo-dev/nsgo/desktop/locales`.
