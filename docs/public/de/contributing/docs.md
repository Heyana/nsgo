---
title: Dokumentation und Website
description: Eine Seite dieser Dokumentation schreiben, übersetzen, die Website betreiben und den Agent-Skill aktuell halten.
group: Mitwirken
order: 83
---

## Eine Seite

Diese Dokumentation besteht aus dem Markdown unter `docs/public/` und ist direkt auf GitHub lesbar. `en/` und `zh/` spiegeln sich Pfad für Pfad; fehlt eine Seite auf Chinesisch, wird die englische Version mit einem Hinweis angezeigt. Jede Seite beginnt mit Frontmatter:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` ist die Gruppe der Seite in der Seitenleiste in der jeweiligen Sprache der Seite, und `order` legt die Reihenfolge fest. Die Gruppen folgen dem Einstieg der Leserschaft: Tutorials, Toolchain, Komponenten, Kernkonzepte sowie Komponenten schreiben und Mitwirken. Eine Seite ohne Gruppe – wie die Einführung – steht davor.
- Das Layout zeigt Titel und Beschreibung automatisch an, daher beginnt der Textkörper mit `##`.

Verlinken Sie Seiten untereinander mit relativen `.md`-Links (`[Tests](testing.md#run-on-a-platform)`); die Website wandelt diese in ihre URLs um. Jede Seite beantwortet genau eine Frage; wie NSGo intern arbeitet, bleibt in `docs/architecture/`.

## Die Website

Die Website unter `website/` wird mit [nsgo-website](https://github.com/nsgo-dev/nsgo-website), einer Astro-Integration, anhand der Einstellungen in `website/site.config.ts` gebaut. Sie benötigt [Bun](https://bun.com) 1.4 oder neuer:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` beschreibt, wie die Seite gebaut und veröffentlicht wird.

## Für Agents

Die Website veröffentlicht die englischen Seiten außerdem für Sprachmodelle: `/llms.txt` listet sie auf, `/llms-full.txt` enthält alle zusammen, und `/docs/<page>.md` liefert eine einzelne Seite mit absoluten Links und ohne die Markierungen der Codeblöcke (`src/lib/llms.ts`). Sie werden automatisch aus den Seiten erzeugt, ohne eigenen Arbeitsschritt.

`skills/nsgo/` ist der Skill, den Agents über `bunx skills add nsgo-dev/nsgo` installieren: `SKILL.md` mit den Regeln, denen eine App folgt, sowie `references/`, die gelesen werden, wenn eine Aufgabe sie benötigt. Passen Sie ihn in derselben Änderung wie die Dokumentation an, sobald sich ändert, wie eine App geschrieben wird oder wie sich Befehle des Tools, Tests oder Paketierung verhalten; beschränken Sie ihn auf das, was ein Agent befolgen muss, und verlinken Sie für alles Übrige auf die Dokumentation. `go tool nsgo test -run TestSkills .` prüft das Frontmatter, die Links und jeden erwähnten NSGo-Bezeichner.
