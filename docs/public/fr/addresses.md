---
title: Adresses d'application
description: Lire et mettre à jour l'adresse d'une application web via l'historique du navigateur.
group: Concepts clés
order: 59
---

`cx.AppURL()` lit l'adresse de l'application comme une valeur suivie (*tracked value*). Sur le web, l'URL initiale de la page est disponible avant le rendu de la racine, ce qui permet à un rechargement ou à un lien partagé de choisir la même vue. L'application décide de la manière dont l'URL correspond à ses vues.

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

`SetAppURL` met immédiatement à jour l'adresse suivie sans demander de page ni naviguer dans une pile. Elle remplace par défaut l'entrée courante du navigateur ; `HistoryPush` demande une nouvelle entrée. Une navigation ou l'ouverture d'une fenêtre modale dans le même *commit* fournit cette entrée à la place, afin qu'une seule action ne crée pas deux retours arrière. Plusieurs écritures dans un même *commit* conservent la dernière adresse et préservent toute demande d'ajout à l'historique.

Les adresses relatives sont résolues par rapport à l'adresse courante, ou à une base HTML explicite. Une adresse navigateur doit rester sur l'origine de la page. Les adresses invalides, les espaces blancs, les caractères de contrôle, les barres obliques inverses et les adresses dépassant 16 Kio renvoient une erreur sans modifier l'adresse.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` observe les parcours d'historique du navigateur et les modifications de fragment (`#`). `change.Reason` vaut `"back"`, `"forward"` ou `"address"` ; la fonction retournée résilie l'abonnement, et la destruction de la portée (*scope*) supprime l'écouteur. Lisez `AppURL` pour connaître l'adresse initiale. Les écritures propres à l'application mettent à jour les liaisons mais n'envoient pas de notification de changement. Un parcours qui conserve la même adresse n'envoie aucun changement.

Le bouton retour du navigateur ferme toujours une fenêtre modale ou dépile un écran de navigation. Un retour refusé par l'application restaure l'entrée existante et son adresse. Un gestionnaire de fermeture peut définir sa propre adresse de remplacement. Un dépilage programmatique met à jour l'adresse après le parcours asynchrone de l'historique, et les écritures survenant pendant ce parcours attendent sa fin. Revenir en arrière au-delà des entrées de ce lecteur quitte l'application, y compris les entrées conservées depuis un chargement précédent.

Pour des chemins tels que `/films/42`, définissez le répertoire d'application déployé et configurez le serveur pour qu'il serve son `index.html` pour les routes de l'application :

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` écrit une balise de base HTML afin que les scripts, styles, Wasm et ressources continuent de se charger lorsqu'une adresse profonde est ouverte directement. Sans cela, les chemins relatifs d'actifs existants sont conservés ; les adresses avec requête ou fragment peuvent les utiliser telles quelles. Les règles de repli du serveur doivent laisser les requêtes de véritables ressources intactes. Une politique stricte de sécurité du contenu (CSP) doit autoriser cette base de même origine (`base-uri 'self'`).

Une application intégrée dans une autre page conserve son adresse en interne et laisse l'historique de la page englobante intact. Une `iframe` de même origine peut explicitement s'y inscrire avec `data-nsgo-history`. Les sous-arbres confinés ne peuvent ni lire ni modifier l'adresse de l'application.

Sur les autres cibles, `AppURL` conserve le dernier lien ouvert avec l'application ou la valeur stockée par `SetAppURL` ; la modifier ne change aucune fenêtre native ni aucun navigateur système. Les mini-programmes conservent aussi cette valeur, sans modifier les routes de leur hôte.

TSX utilise le même état :

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
