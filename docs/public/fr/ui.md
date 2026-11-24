---
title: Référence des composants
description: Les composants les plus utilisés, leurs variantes, états et accessibilité.
group: Composants
order: 30
---

Voici les composants d’`ui`, dans l’apparence propre à NSGo et en suivant le thème. Cette référence commence par les composants auxquels une application a recours en premier ; chaque page débute par l’exemple le plus court, puis aborde ses parties, ses états et son accessibilité. La galerie couvre tous les composants, y compris ceux qui n’ont pas encore de page ici.

La [galerie](https://nsgo.dev/gallery/) exécute une démonstration de chaque composant dans le navigateur.

## Actions

- [Button](ui/button.md) : un bouton-poussoir, simple ou dans la couleur d’accentuation, et le bouton icône.

## Texte et saisie

- [Text](ui/text.md) : paragraphes, segments (*spans*), liens, polices et texte de document.
- [Input](ui/input.md) : le champ texte de la plateforme, contrôlé ou non.
- [Checkbox](ui/checkbox.md) : une case à cocher, cochée, décochée ou indéterminée.
- [Switch](ui/switch.md) : un réglage activé ou désactivé.
- [Select](ui/select.md) : une valeur parmi une liste, sous forme de menu contextuel ou de feuille.
- [Slider](ui/slider.md) : une valeur ou une plage le long d’une piste.

## Collections

- [List](ui/list.md) : des lignes en nombre quelconque, construites uniquement à l’affichage, avec sélection et activation.
- [Scroll](ui/scroll.md) : la vue défilante de la plateforme, avec pagination, aimantation (*snapping*), zoom et rafraîchissement.

## Navigation

- [Tabs](ui/tabs.md) : une liste d’onglets, un contrôle segmenté et la barre d’onglets système.

## Superpositions

- [Dialog](ui/dialog.md) : une surface modale, une feuille et l’alerte système.
- [Menu](ui/menu.md) : un menu depuis un déclencheur, un menu contextuel et leurs éléments.

## Plus

La galerie couvre le reste : tableaux, graphiques, éditeur de code, texte enrichi, terminal, dock, palette de commandes, sélecteurs de couleur, de date et d’émojis (`ui.EmojiPicker`), toasts et bien plus encore. Chacun y dispose d’une démonstration.

`base/terminal.View` ouvre les chemins locaux existants avec Commande+clic (Contrôle+clic sous Windows et Linux), tout comme il ouvre les URL. Les chemins relatifs utilisent le répertoire de travail OSC 7 du shell ; un shell local qui n’en a pas signalé utilise `terminal.Options.Dir`, ou le répertoire personnel s’il n’est pas défini. Placez entre guillemets les noms de fichiers contenant des espaces. Les autorités de répertoires distants et les connexions ne signalant pas de répertoire local ne sont pas résolus vers des fichiers locaux. Les fichiers et dossiers utilisent `Scope.OpenFile` ; les liens URL continuent de passer par `Scope.OpenURL` et le gestionnaire d’URL de l’hôte intégrateur.

`base/terminal.View` refuse par défaut les lectures et écritures dans le presse-papiers par les programmes. Ne définissez `AllowClipboardRead` et `AllowClipboardWrite` que lorsque l’application les autorise ; le paramètre `AccessClipboard` de la vue appelante reste applicable. Les mêmes propriétés sont disponibles sur `Terminal` en TSX et peuvent changer sans redémarrer sa session. Les applications fournissent l’interface de consentement. Un collage explicite de l’utilisateur propose indépendamment un instantané texte/PNG aux programmes utilisant le protocole presse-papiers Kitty (mode 5522) ; son mot de passe à usage unique ne permet de lire que cet instantané. Les programmes sans ce mode reçoivent du texte ordinaire entre séquences encadrées (*bracketed paste*). La disponibilité des images dans le presse-papiers suit les services presse-papiers de la plateforme ; la sélection primaire X11 n’est pas exposée.

## Suite

- [Composants et état](components.md) : comment un composant retourne des éléments et comment les signaux les mettent à jour.
- [Styles et thèmes](styles.md) : le thème dont les composants tirent leurs couleurs.
- [Composants sans style](authoring/base.md) : les mêmes contrôles sans apparence, pour une apparence qui vous est propre.
- [Accessibilité](accessibility.md) : libellés, rôles et valeurs pour chaque contrôle.
