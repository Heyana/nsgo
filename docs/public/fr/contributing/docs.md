---
title: La documentation et le site web
description: Rédiger une page de cette documentation, la traduire, lancer le site web et maintenir la skill des agents à jour.
group: Contribuer
order: 83
---

## Une page

Cette documentation correspond aux fichiers Markdown dans `docs/public/`, lisibles directement sur GitHub tels quels. `en/` et `zh/` se reflètent chemin pour chemin ; une page manquante en chinois affiche la version anglaise accompagnée d'un avertissement. Chaque page commence par un front matter :

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` est le groupe de la page dans la barre latérale, dans la langue de la page, et `order` détermine sa position. Les groupes suivent le parcours d'un lecteur : tutoriels, chaîne d'outils, composants, concepts clés, puis écriture de composants et contribution. Une page sans groupe, comme l'introduction, apparaît avant eux.
- La mise en page affiche déjà le titre et la description ; le corps commence donc à `##`.

Liez les pages entre elles à l'aide de liens `.md` relatifs (`[Tests](testing.md#run-on-a-platform)`) ; le site les transforme en ses propres URL. Chaque page répond à une seule question ; le fonctionnement interne de NSGo reste dans `docs/architecture/`.

## Le site web

Le site web dans `website/` est généré par [nsgo-website](https://github.com/nsgo-dev/nsgo-website), une intégration Astro, à partir des paramètres de `website/site.config.ts`. Il nécessite [Bun](https://bun.com) 1.4 ou plus récent :

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` indique comment le site est compilé et publié.

## Pour les agents

Le site web publie également les pages anglaises pour les modèles de langage : `/llms.txt` les liste, `/llms-full.txt` les contient toutes et `/docs/<page>.md` en fournit une seule, avec des liens absolus et sans les marqueurs des blocs de code (`src/lib/llms.ts`). Elles suivent automatiquement les pages sans étape supplémentaire.

`skills/nsgo/` est la skill que les agents installent avec `bunx skills add nsgo-dev/nsgo` : `SKILL.md` avec les règles qu'une application suit, et `references/` lues lorsqu'une tâche l'exige. Mettez-la à jour dans la même modification que la documentation lorsque ce qu'écrit une application, les commandes de l'outil, les tests ou l'empaquetage changent ; limitez-la à ce qu'un agent doit respecter et renvoyez vers la documentation pour le reste. `go tool nsgo test -run TestSkills .` vérifie son front matter, ses liens et chaque nom de NSGo qu'elle mentionne.
