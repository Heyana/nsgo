---
title: Publier sur le web
description: Publier une application NSGo avec une sémantique de document, des métadonnées de page et des ressources sur CDN.
group: Chaîne d’outils
order: 24
---

## Compiler l’application

```sh
go tool nsgo build -target web
```

Servez le répertoire généré en HTTP. Les ressources de publication comportent des empreintes de contenu dans leur nom, mettez-les donc en cache comme immuables ; revalidez `index.html`, `nsgo.json` et `manifest.webmanifest`. L’application web conserve la mise en page, le défilement, le focus et le comportement du pointeur de NSGo.

## Titres et liens

Un `core.View` ou un `core.Text` avec `Role: core.RoleHeader` devient une balise `h1` à `h6`, sélectionnée par `AccessibilityLevel` ; sans niveau défini, c’est `h2`. Les titres Markdown issus de `base/textview` conservent leur niveau. Les marges et les polices par défaut du navigateur pour les titres ne modifient pas le style de l’application.

`base.Link` et `ui.Link` publient leur `Href` sous forme de lien d’ancrage. Un clic ordinaire exécute `OnOpen` et `OnPress` de l’application ; un clic du milieu ou un clic avec une touche de modification utilise l’action de lien du navigateur. Le navigateur fournit son menu contextuel de lien, sauf si l’application gère `OnContextMenu`. Un lien désactivé n’a pas d’adresse active. Pour une vue personnalisée ayant le rôle de lien, définissez `core.View.Web` sur `core.WebViewProps{Href: "/notes"}` ou liez `HrefFunc`. En TSX, utilisez `web={{href: "/notes"}}` ; pour les titres, utilisez `accessibilityLevel={1}`. Les schémas d’URL exécutables non sûrs sont refusés. Le contenu confiné garde la navigation dans ses gestionnaires d’appui à portée restreinte et n’annonce aucune adresse au navigateur.

## Métadonnées de page

Importez `github.com/nsgo-dev/nsgo/web` et placez un `web.Head` à côté du contenu d’une page :

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` et `DescriptionFunc` suivent les signaux. Les éléments d’en-tête visibles ultérieurs remplacent les champs qu’ils définissent et héritent des autres. Masquer ou supprimer une page rétablit les en-têtes précédents ; supprimer tous les en-têtes rétablit le document d’origine. Un champ statique vide hérite ; une fonction qui renvoie une chaîne vide efface le champ. Un en-tête n’ajoute aucun nœud de mise en page. Les cibles natives, les applications intégrées et le contenu confiné ne modifient pas le document de leur hôte.

En TSX, importez `Head` depuis `nsgo/web` et écrivez `<Head title={title()} description="Your saved notes" />`. Pour les aperçus HTML statiques, pré-rendez chaque URL lors du déploiement du site ; `Head` met à jour le document en cours d’exécution.

## Servir les ressources depuis un CDN

Le chargeur résout tous les fichiers de l’application relativement à `nsgo.json`. Une page peut charger l’application depuis une autre origine :

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Autorisez l’origine de la page à récupérer les fichiers du CDN avec CORS, et servez WebAssembly avec le type `application/wasm`. Servez le HTML d’entrée de l’application pour toutes les routes profondes utilisées par votre application. [Intégration](embedding.md#web) montre comment effectuer le montage dans un élément à la place.
