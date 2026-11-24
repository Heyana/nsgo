---
title: Terminal
description: Exécuter un programme ou afficher une session distante dans une grille de terminal.
group: Concepts clés
order: 63
---

`base/terminal` fournit une session (`Terminal`) et sa `View`. Les ordinateurs de bureau exécutent des programmes locaux ; iOS et les navigateurs utilisent `Options.Conn`, un `io.ReadWriteCloser` connecté à un programme distant. L’application possède la session et la ferme une fois terminée.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` choisit la forme par défaut : bloc, barre ou soulignement. `CursorBlink` active le clignotement par défaut ; la valeur zéro le laisse fixe. Les programmes remplacent la forme et le clignotement via DECSCUSR ou le mode DEC 12. Un curseur sans focus est creux et fixe ; la saisie par composition et la réduction des animations le laissent également fixe. L’horloge d’animation de la plateforme pilote le clignotement sans que le moteur ait à redessiner le texte du terminal.

`View.Font`, `Theme` et `DarkTheme` définissent la police et la palette. La grille dessine elle-même les caractères Unicode semi-graphiques et blocs ainsi que les séparateurs Powerline U+E0B0–U+E0B7, en alignant les cellules adjacentes sur les pixels physiques de l’appareil. Le reste du texte utilise les polices de la plateforme et leurs polices de repli.

`MinimumContrast` augmente le contraste du texte ordinaire par rapport à l’arrière-plan réel de sa cellule, y compris pour les couleurs inversées, le texte atténué et la sélection. Les valeurs de 1 à 21 utilisent le ratio de contraste WCAG ; zéro préserve les couleurs. Les graphismes et les emoji conservent leurs couleurs. Avec `Transparent`, le contraste continue d’utiliser l’arrière-plan configuré du terminal.

`Padding` laisse de l’espace à l’extérieur de la grille, en points. `PaddingColor` choisit son arrière-plan :

| Valeur | Comportement |
| --- | --- |
| `PaddingBackground` | Arrière-plan du terminal, y compris les changements OSC 11 |
| `PaddingExtend` | Arrière-plan de la cellule la plus proche ; sur l’écran principal, supprime l’extension en haut et en bas à côté des lignes d’invite, des lignes Powerline ou des lignes contenant une cellule avec l’arrière-plan par défaut |
| `PaddingExtendAlways` | Étend chaque bord sans ces exceptions |

Les applications TSX importent `tsx/terminal` pour son effet d’enregistrement de `Terminal` dans `nsgo/base`. Les props sont `cursorBlink`, `minimumContrast` et `paddingColor` ; cette dernière accepte `background`, `extend` et `extend-always`. Les props d’apparence peuvent changer pendant que la session se poursuit.

## Dans un navigateur

L’application intègre le module WebAssembly libghostty-vt épinglé et exécute la même vue de terminal et le même chemin de saisie. La compilation de l’application ne nécessite ni Zig, ni installation de bibliothèque native, ni téléchargement à l’exécution.

Adaptez la WebSocket du navigateur vers `io.ReadWriteCloser` : envoyez des octets binaires et assemblez ou découpez les messages entrants comme un flux. L’application et son serveur gèrent l’authentification, la reconnexion ainsi que tout message de redimensionnement ou de contrôle ; NSGo consomme les octets du terminal. La [fixture d’acceptation navigateur](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) montre un adaptateur complet de flux d’octets avec gestion de la fermeture.

## Actions et zoom

`View.Keybinds` surcharge les raccourcis par défaut avec `Keybind{Key, Action, Amount}`. Les touches utilisent les mêmes noms que `Scope.OnKey`, tels que `mod+=` ou `ctrl+shift+c`. Les dernières entrées l’emportent. Les raccourcis s’exécutent tant que la saisie de ce terminal a le focus et qu’aucune méthode de saisie n’est en cours de composition.

Les actions sont `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` et `Ignore`. Les actions de fenêtre et d’onglet appartiennent à la table de raccourcis de l’application. `Ignore` consomme une touche sans l’envoyer au programme.

Par défaut, `mod+=` et `mod+shift+=` augmentent la police d’un point, `mod+-` la réduit, et `mod+0` la réinitialise à `View.Font.Size` (13 lorsqu’elle n’est pas définie). `Amount` définit le pas pour l’augmentation ou la réduction (un lorsqu’il vaut zéro) ou la taille pour `SetFontSize`. Les tailles sont bornées entre 1 et 255 points ; les demandes non finies sont ignorées. Le zoom conserve la session et la vue englobante, met à jour les lignes et colonnes de la grille et redimensionne le programme.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` est suivi pour les contrôles qui l’affichent ; `Ref.SetFontSize(size)` et `Ref.Perform(action, amount)` s’exécutent sur le moteur de l’application, comme le font les gestionnaires d’événements. `Perform` indique si une action est disponible. Une référence non montée renvoie une taille nulle et refuse les actions.

## Menu contextuel

Un clic secondaire ouvre un `base.ContextMenu` sans style doté des éléments localisés Copier, Coller, Tout sélectionner et Effacer. Copier nécessite une sélection. Effacer supprime le contenu au-dessus de l’invite et est désactivé sur l’écran alternatif. Les écrans tactiles utilisent la présentation par appui long du menu contextuel. Stylisez la surface avec `MenuClass`/`MenuStyle` et les éléments avec `MenuItemClass`/`MenuItemStyle`.

Définissez `RightClickAction` sur `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (copie la sélection, sinon colle) ou `RightClickIgnore`. L’option ignorer laisse l’événement à un menu englobant de l’application ; utilisez-la lorsque vous fournissez un `ui.ContextMenu` stylisé qui appelle les actions de la référence.

Importez `tsx/terminal` pour enregistrer `<Terminal>` dans `nsgo/base`. Ses props réactives `keybinds` et `rightClickAction` mettent à jour la vue tout en conservant la session. Les valeurs d’action utilisent les noms Ghostty tels que `increase_font_size` et `copy_to_clipboard` ; les valeurs de clic droit sont `context-menu`, `paste`, `copy`, `copy-or-paste` et `ignore`. Les props de style du menu reprennent les noms Go en *lower camel case*.

## Rapports souris

Les programmes qui activent le suivi de la souris reçoivent les appuis, relâchements, glissements et survols demandés, encodés par libghostty-vt dans le protocole qu’ils ont sélectionné. La touche Maj contourne la capture des clics et glissements pour permettre la sélection. `View.MouseReporting` est un `func() bool` réactif optionnel, vrai lorsqu’il vaut nil ; renvoyez faux pour conserver la sélection à la souris et le défilement natif de l’historique même lorsqu’un programme demande la capture. Le clic droit d’un programme a priorité sur un menu contextuel englobant tant que le rapport est activé.

Sur l’écran alternatif, avec le mode de défilement alternatif activé et sans suivi de souris demandé, le défilement vertical à la molette est converti en touches fléchées, selon le mode normal ou applicatif des touches de curseur. `MouseScrollMultiplier{Precision: 1, Discrete: 3}` définit le nombre de rapports par défaut pour les pavés tactiles et les molettes ; zéro utilise ces valeurs par défaut, et les valeurs positives sont bornées entre 0,01 et 10 000. Les entrées fractionnaires s’accumulent jusqu’à ce qu’un pas complet soit disponible. L’historique normal conserve le défilement natif et l’inertie de la plateforme.

Importez `tsx/terminal` pour enregistrer `<Terminal>` dans `nsgo/base`. Sa prop `mouseReporting` vaut vrai par défaut ; `mouseScrollPrecision` et `mouseScrollDiscrete` définissent les multiplicateurs.
