---
title: Glisser-déposer
description: Transférer des valeurs applicatives entre des vues dans une même fenêtre.
group: Concepts clés
order: 57
---

`base/dragdrop` relie une source et une cible dans une même `Area`, au sein d'une fenêtre. L'application conserve ses vues, ses styles, sa sélection et l'action qui traite la valeur. Une source démarre après quatre points de déplacement par défaut, ce qui laisse intact un appui ordinaire.

```go
import "github.com/nsgo-dev/nsgo/base/dragdrop"

dragdrop.Area{Children: []core.Node{
	dragdrop.Source{
		View: core.View{
			Label: "Film", TestID: "poster",
			Style: core.Style{Width: core.Pt(80), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Poster"}},
		},
		Data: func() any { return filmID },
		Preview: func(any) core.Node { return core.Text{Content: "Film"} },
	},
	dragdrop.Target{
		View: core.View{
			TestID: "playlist",
			Style: core.Style{Width: core.Pt(160), Height: core.Pt(80)},
			Children: []core.Node{core.Text{Content: "Playlist"}},
		},
		Accept: func(value any) bool { _, ok := value.(string); return ok },
		OnDrop: func(e dragdrop.Event) { addToPlaylist(e.Data.(string)) },
	},
}}
```

`Data` s'exécute une fois lorsque le glissement démarre. Retournez une tranche pour transporter plusieurs éléments sélectionnés ensemble ; copiez-y les données mutables si elles doivent rester un instantané. Une fonction `Data` nil désactive le glissement. `Threshold` définit le déplacement requis. `Activation: dragdrop.AfterLongPress` attend d'abord `View.DelayLongPress` (500 ms par défaut), puis le déplacement ; choisissez-le explicitement lorsqu'un balayage doit d'abord faire défiler.

`View` conserve ses propriétés habituelles, y compris les gestionnaires d'appui et l'accessibilité. Une source possède `View.Responder` ; laissez-le non défini. Ses enfants peuvent inclure un bouton avec son action normale. `Preview` suit le pointeur sans capter les entrées ni remonter la source ; sans aperçu, le libellé de la source est affiché. Donnez un libellé ou un aperçu à toute source. Les zones imbriquées fonctionnent indépendamment.

Les cibles ont besoin de `OnDrop` pour accepter une valeur. La cible la plus imbriquée a la priorité ; lorsque `Accept` la refuse, un ancêtre qui l'accepte peut la recevoir. `OnEnter`, `OnMove` et `OnLeave` permettent de mettre à jour l'apparence de la cible. Chaque événement contient `Data`, `Position` dans la cible et `PagePosition` dans la surface. Désactiver ou retirer la cible active efface son état d'entrée ; le relâchement vérifie à nouveau l'acceptation. Une cible placée derrière une autre vue, rognée, désactivée ou sous une modale ne peut pas recevoir le dépôt.

`OnStart` et `OnEnd` décrivent la session de la source. `OnEnd` reçoit `Result{Data, Dropped, Target}`. Échap, l'annulation tactile native, la désactivation ou le retrait de la source, ou le relâchement hors d'une cible qui accepte mettent fin à l'opération sans dépôt. Le placement des cibles utilise la dernière géométrie validée ; les animations natives entre commits ne sont pas échantillonnées. Ce mécanisme transfère des valeurs au sein de la fenêtre ; les fichiers système utilisent toujours `OnFileDrop`.

Prévoyez toujours un menu, une commande clavier ou une action accessible effectuant la même opération. Le glissement est un raccourci vers cette action. Les tests utilisent `app.DragTo("id:poster", "id:playlist")`, ou `DragTo` sur une surface sélectionnée ; les tests TSX utilisent `await app.dragTo("id:poster", "id:playlist")`. Les deux vues doivent déjà tenir dans la fenêtre visible ; l'étape envoie une séquence tactile à travers le répondeur sans synthétiser de dépôt ni faire défiler pendant le glissement. Voir [Tests](testing.md).
