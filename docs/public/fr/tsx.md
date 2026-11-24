---
title: Applications TSX
description: Écrire une application en TSX avec des classes Tailwind, compilée en amont sans Node.
group: Concepts clés
order: 63
---

## Une application TSX

Une application TSX est un paquet Go qui intègre ses sources et exécute son point d’entrée :

```go
//go:embed *.tsx nsgo.yaml
var src embed.FS

var Root = compiler.App(src, "App.tsx")

func main() { nsgo.Run(Root) }
```

```tsx
import { mount, signal, Text, View } from "nsgo";

function App() {
  const count = signal(0);
  return (
    <View class="gap-3 p-6" onPress={() => count.update((n) => n + 1)}>
      <Text class="text-lg font-bold">Pressed {count()} times</Text>
    </View>
  );
}

mount(App);
```

`go tool nsgo init -template tsx` en génère une, et `examples/tsx` en est une autre. `nsgo dev` et `nsgo build` compilent le TSX en amont, de sorte que les paquets n’embarquent aucun compilateur ; `go run` et `go test` le compilent au démarrage de l’application. Node n’est nécessaire dans aucun des deux cas.

## Composants et modules

TSX importe les primitives depuis `nsgo`, les composants de `base` depuis `nsgo/base`, les manettes de jeu depuis `nsgo/gamepad` (sous la capacité `gamepad` dans les paquets signés), les sons depuis `nsgo/audio` (sous `audio`) et ceux de `ui` depuis `nsgo/ui`, une fois que l’application importe `tsx/ui` ; `examples/gallery-tsx` les illustre.

Pour les diagrammes Mermaid, importez `github.com/nsgo-dev/nsgo/tsx/diagram` pour son effet dans l’hôte Go, aux côtés de `tsx/textview` ou `tsx/ui`. Cela ajoute `Diagram` à `nsgo/base` sans lier l’analyseur de diagrammes aux applications qui ne l’utilisent pas. Il accepte `source`, `fontSize`, un `theme` partiel, `accessibilityLabel` et les propriétés de style habituelles. Utilisez-le directement ou fournissez un moteur de rendu TextView :

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

La même propriété `renderDiagram` fonctionne sur le TextView de `nsgo/ui`. Un bloc de code non Mermaid conserve son moteur de rendu habituel ; un diagramme invalide affiche sa source.

Pour les formules mathématiques composées, importez `github.com/nsgo-dev/nsgo/tsx/formula` pour son effet dans l’hôte Go, aux côtés de `tsx/textview` ou `tsx/ui`. Cela ajoute `Formula` à `nsgo/base` ; les applications qui omettent ce paquet gardent le moteur mathématique et sa police hors de leur binaire. Il accepte `source`, `dialect` (`tex` par défaut, ou `typst`), `display`, `accessibilityLabel` et les propriétés de style. La taille de police et la couleur suivent le texte environnant. Utilisez-le directement ou fournissez un moteur de rendu TextView :

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

La même propriété `renderMath` fonctionne sur le TextView de `nsgo/ui`. Les formules conservent leur code TeX lorsqu’elles sont copiées. Le `Renderer` Go reste une configuration de l’hôte.

`UrlScheme.url()` lit l’adresse suivie de l’application ; `UrlScheme.setUrl(url, { history: "push" })` ajoute une entrée d’adresse, ou la remplace lorsque les options sont omises. `UrlScheme.on("change", fn)` observe les navigations arrière et avant du navigateur ainsi que les changements de fragment (`#`), et renvoie une fonction de résiliation d’abonnement. Voir [Adresses d’application](addresses.md) pour le routage, la validation de même origine et l’hébergement de liens profonds.

## Go depuis TSX

Enregistrez des éléments Go pour que TSX puisse les importer :

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

`import { Badge, greet } from "app/native"` accepte alors les champs de `Badge` en tant que propriétés (*props*) et appelle `greet`. Les fonctions qui prennent un `context.Context` deviennent des promesses, et `tsx.RegisterClass` transforme des types Go en classes que TSX instancie avec `new`.

Une propriété optionnelle transmise avec la valeur `undefined` utilise sa valeur par défaut. Lors de l’enregistrement d’un composant Go doté d’un accesseur dont la valeur par défaut diffère de la valeur nulle de son type, ajoutez une option telle que `tsx.Default("Max", float64(100))` ; la même valeur par défaut s’applique tant qu’une propriété réactive renvoie undefined, sans remplacer le composant.

Les champs de données dotés de valeurs par défaut Go documentées peuvent également être omis : `width` et `height` sur un nœud `Flow` mesurent son contenu JSX lorsqu’ils sont absents, l’omission de `width` sur une arête utilise le trait par défaut, et l’omission de `pace` sur une ligne de quota laisse son indicateur de rythme masqué.

L’enregistrement de composants et de classes Go recherche leurs champs et méthodes exportés par réflexion à l’exécution de l’application. Cette recherche empêche l’éditeur de liens Go de supprimer des méthodes exportées que rien n’appelle, dans toute l’application et pas uniquement dans ce que TSX enregistre ; ainsi, lier `tsx` ajoute environ 20 Mo à un binaire de bureau dépouillé (*stripped*), et davantage à mesure que l’application lie plus de paquets.

## Paquets signés

Une application peut exécuter du code TSX qu’elle télécharge, comme ses propres écrans livrés à nouveau ou des greffons (*plugins*), une fois qu’elle a vérifié le paquet. Déclarez ce à quoi le code du paquet peut accéder dans son fichier `nsgo.yaml` ; tout le reste lui est refusé :

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # ou { schemes: [https] }
  host: [search, "player.*"] # les gestionnaires de l’application qu’il peut appeler
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Conservez une clé racine hors ligne et certifiez des clés de publication avec celle-ci, puis signez l’application dans ce répertoire. `go tool nsgo keygen` affiche une paire de clés et n’écrit aucun fichier ; exécutez-le une fois pour la clé racine et une fois pour chaque clé de publication. Les clés publiques vont dans les drapeaux et le code de l’application ; les clés secrètes restent hors du dépôt et ne parviennent à l’outil que via `NSGO_ROOT_KEY` et `NSGO_RELEASE_KEY` :

```sh
go tool nsgo keygen # une fois pour la clé racine, une fois pour la clé de publication
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

L’application fait confiance à la clé publique de la racine, inscrite dans son code, vérifie le paquet en dehors de la goroutine du moteur et l’affiche dans `tsx.View`, isolé dans un bac à sable aux côtés de ses propres vues. Le paquet n’atteint l’application que par le canal hôte que la vue lui fournit, et conserve sa propre partie du stockage et des secrets de l’application :

```go
trust, err := pkg.OpenTrust(filepath.Join(cx.DataDir(), "trust"))
trust.AddRoot(root)
p, err := trust.Verify(data) // refuses tampered, revoked and older builds
channel := core.NewHost()
channel.Handle("search", func(c *core.HostCall) { c.Resolve(results(c.Data)) })
view := tsx.View{Package: p, Sandbox: tsx.Sandbox{Host: channel}}
core.Catch{Try: view.Render, Fallback: failed}
trust.Accept(p) // once it ran, older builds are refused
```

`Sandbox.Grant` lui accorde moins que ce qu’il déclare, `Sandbox.Limits` d’autres budgets de temps et quotas, et `Sandbox.Modules` des fonctions et composants Go au-delà de ceux de NSGo. Ses raccourcis clavier, gestionnaires de retour et piles de navigation ne prennent effet que lorsque le focus clavier se trouve dans sa vue, et il ne déplace lui-même le focus (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) que lorsque le focus s’y trouve déjà ou en réponse à un appui de l’utilisateur dans celle-ci. Il s’affiche dans une vue qui lui est propre, qui remplit l’espace que son parent lui accorde, sans plus, et le découpe, menus, popovers et toasts compris ; dans une vue défilante, placez-le dans une vue pour le laisser prendre la hauteur de son contenu. Une boîte de dialogue, une alerte, un menu contextuel ou une annonce qui en émane ne s’affiche que de la même manière, et le retour ferme sa boîte de dialogue même s’il ne la supprime pas. `View.Export` nomme un composant exporté par le module d’entrée du paquet, affiché à la place de la racine que l’entrée monte, dont le `mount` n’effectue alors ni rendu ni exécution de son composant, et reçoit `View.Props`, une table encodable en JSON copiée pour chaque vue, afin qu’un même paquet fournisse plusieurs vues, chacune avec son propre état, sous les mêmes capacités et limites ; un export manquant provoque une panique avec `tsx.ErrNoViewExport` et un export qui n’est pas une fonction avec `tsx.ErrViewExportNotComponent`. Un paquet dont l’entrée exporte des fonctions est un greffon de fonctions : `tsx.Load(cx, p, tsx.Sandbox{})` l’exécute de manière invisible et synchrone sur la goroutine de l’appelant, sous un `cx` appartenant à un arbre de vues (sans cela, il renvoie `tsx.ErrNoTree`), ou renvoie ce que son module d’entrée a levé lors de son chargement, et `Plugin.Call` appelle ces fonctions. `go tool nsgo dev -limits` exécute une application comme le ferait une telle vue, et `go tool nsgo revoke` signe une liste de clés de publication et de paquets révoqués pour `Trust.UpdateRevocations`. `examples/plugins` demande à l’utilisateur d’accorder les capacités d’un paquet, l’installe et l’exécute. `Limits.Memory`, fixé à 64 Mio par défaut, alloue un budget de stockage natif mesuré pour les tableaux, les chaînes, les tampons et les tableaux typés. Cela inclut `concat`, `flat`, `slice`, la décomposition de tableaux (*spread*), la concaténation de chaînes, les gabarits (*templates*), `replace`, `replaceAll`, `TextEncoder` et les copies de `Blob`. `memory=<bytes>` dans `-limits` modifie ce budget, et `memory=0` le désactive. Les vérifications s’exécutent avant l’allocation. Un paquet dépassant le budget s’arrête avec `ERR_NSH_QUOTA_EXCEEDED` et la ressource `memory` après que Go a vérifié s’il existe de la mémoire récupérable. Les objets ordinaires, les fermetures (*closures*), les piles, les petites allocations ainsi que les fonctions intégrées et API hôtes non mesurées peuvent également allouer de la mémoire. Le navigateur conserve son propre tas (*heap*). Une application qui signe elle-même des paquets avec `pkg.Build`, ou via `go tool nsgo publish -extra <file>` avec les octets de ce fichier, peut placer ses propres métadonnées dans `pkg.Contents.Extra`, comme ce qu’un greffon ajoute à ses menus : elles sont signées avec le paquet, et `Verify` les renvoie inchangées sous `p.Extra` pour que l’application les analyse, ce qui lui permet de les lire sans exécuter le code du paquet.

Une application peut signer ses propres documents JSON, tels qu’un catalogue de ses greffons, avec `pkg.SignDocument` et les vérifier avec `Signed.Open`, à l’aide des clés que `nsgo keygen` affiche. Donnez à chaque type de document un contexte `app:` qui lui est propre accompagné d’une version, comme `app:catalog-v1` : un document ne passe jamais pour un autre type, ni pour un paquet, un certificat ou une liste de révocation. `Open` fait confiance exactement aux clés publiques qui lui sont fournies, sans certificat ni liste de révocation, et refuse les champs du corps absents du type dans lequel il décode. L’enveloppe ne contient ni horodatage ni numéro de séquence, si bien qu’un document plus ancien passe la vérification tout aussi bien qu’un plus récent : placez une version ou une date d’expiration dans le corps et vérifiez-la.
Dans un bac à sable natif, `JSON.stringify`, `JSON.parse` avec un *reviver*, `Array.prototype.join`, `toString`, `toLocaleString` et `flat`, et par conséquent `String()` sur un tableau, lèvent une `RangeError` interceptable dès qu’ils dépassent 1 000 niveaux de profondeur, en comptant les appels imbriqués les uns dans les autres, car `toString` d’un tableau et le `join` qu’il appelle comptent chacun ; des données plus profondes bloqueraient l’appel ou mettraient fin à l’application. Le web utilise les fonctions intégrées du navigateur.

Sur les cibles natives, `DefaultLimits.Stack` est de 1 000 trames (*frames*). Définissez `Limits.Stack` ou `-limits stack=<frames>` pour choisir une autre profondeur jusqu’à 10 000 trames. Les valeurs Go inférieures ou égales à zéro et les entrées au-delà du plafond utilisent 10 000. Le web utilise la limite de pile du navigateur.

Les fonctions natives qui s’appellent entre elles sont décomptées de `Limits.Stack` au même titre que les appels JS : un accesseur, `toString` ou `valueOf` qui est une fonction intégrée, une expression régulière dont `exec` est une autre de ses méthodes, `Reflect.apply` et `Reflect.construct` imbriqués, ainsi que les chaînes de fonctions liées ou de proxies lèvent une `RangeError` interceptable au-delà de cette limite plutôt que de mettre fin à l’application, et une valeur d’un greffon imbriquée plus profondément fait échouer l’appel qui la convertit en Go avec une `RangeError`. Cela provient du fork de goja requis par NSGo, `github.com/nsgo-dev/goja`, que votre application obtient avec NSGo sans aucune modification de son `go.mod`.

Sur les cibles natives, `Limits.Call` borne chaque tranche d’exécution JavaScript, y compris les rappels de minuteurs et les continuations de promesses après `await`. Lorsqu’une tranche est interrompue, un `Plugin.Call` en attente signale `tsx.ErrBudget` une seule fois, arrête ses minuteurs restants et ignore les réponses tardives de l’hôte. Les appels indépendants continuent de s’exécuter, y compris les réactions de promesses mises en file dans la tranche interrompue. Un appel qui résout ou rejette la promesse d’un autre appel signale toujours son propre résultat ou sa propre erreur. Les expressions régulières à retour sur trace (*backtracking*) utilisent le plus court entre le budget de chargement et celui d’appel comme délai d’expiration de correspondance ; un délai dépassé non géré devient également `tsx.ErrBudget`.

`Limits.CallTotal` ajoute une échéance en temps réel (*wall-clock*) pour chaque appel de greffon et événement de vue en bac à sable. Elle inclut le temps passé en attente et couvre ses minuteurs, intervalles, trames, continuations de promesses et réponses de l’hôte même après la résolution de l’appel. À l’échéance, le travail en attente est abandonné et un appel toujours en attente échoue avec `tsx.ErrBudget`. Le travail du module d’entrée est soumis à `Limits.Load`, ou à `CallTotal` si `Load` vaut zéro. Les événements hôtes autonomes utilisent `CallTotal` à partir de leur remise. Utilisez `Plugin.CallWithBudget` pour obtenir une poignée vers l’échéance d’un appel en attente. Dans un `Scope.Post` de l’hôte, appelez `budget.Extend(duration)` avant cette échéance pour accorder plus de temps uniquement à cet appel.

`Plugin.Interrupt` arrête l’environnement d’exécution depuis n’importe quelle goroutine, supprime ses minuteurs en attente et fait échouer les appels en cours et ultérieurs. `Plugin.Stopped()` et `Plugin.Err()` peuvent également être lus depuis n’importe quelle goroutine. `Err()` vaut nil tant que l’environnement d’exécution tourne et renvoie sa première raison d’arrêt une fois arrêté. Fermer le greffon ou libérer normalement son propriétaire exécute ses nettoyages et l’arrête avec `tsx.ErrPluginClosed`. Définissez `Sandbox.OnStop` lors de l’appel à `tsx.Load` pour recevoir cette raison une fois sur la goroutine du moteur, après l’échec des appels en attente.

Définissez `Sandbox.OnError` lors du chargement d’un greffon de fonctions pour recevoir les erreurs non interceptées de minuteurs, d’animation-frame, de `queueMicrotask` et de `reportError` en dehors d’un `Plugin.Call` en attente. Le gestionnaire de chaque greffon s’exécute sur la goroutine du moteur, afin qu’un hôte puisse attribuer les défaillances et appeler `Plugin.Interrupt` lorsque son disjoncteur s’ouvre. Les coupures natives de budget de tranche signalent également `tsx.ErrBudget`, y compris pour une tâche de promesse dont l’appel est déjà terminé. Les erreurs de rappel ordinaires laissent le greffon en cours d’exécution. Les erreurs de chargement et de résultat d’appel sont toujours renvoyées à leurs appelants, et les vues continuent d’utiliser `core.Catch` ; sans gestionnaire, le comportement existant des frontières d’erreur et des rapports d’application est conservé.

Définissez `Sandbox.OnViewRuntime` pour recevoir le `*tsx.ViewRuntime` de chaque vue sur la goroutine du moteur avant l’exécution de son module d’entrée. Appelez `Interrupt` depuis n’importe quelle goroutine pour arrêter son travail et laisser la dernière vue rendue en place. `Stopped` et `Err` indiquent son état terminal, avec `tsx.ErrViewClosed` lors d’une libération normale. `Sandbox.OnStop` signale également une fois la raison terminale d’une vue.

`Plugin.JSTime` et `ViewRuntime.JSTime` indiquent le temps écoulé dans les entrées JavaScript de l’environnement d’exécution natif, y compris l’entrée en cours. L’attente entre les entrées et le temps passé dans les fonctions Go synchrones enregistrées sont exclus. `ViewRuntime.EngineTime` inclut également les appels Go synchrones et le travail réactif, de montage, de *commit* et de mise en page lui appartenant, y compris le travail toujours en cours. Il exclut l’attente entre les entrées du moteur, le travail appartenant à l’hôte ou à une autre vue, la comptabilité partagée des *commits* et le dessin natif. Les deux compteurs peuvent être lus en toute sécurité depuis n’importe quelle goroutine. Les tours JavaScript du navigateur en dehors de NSGo ne peuvent pas être mesurés ni préemptés : sur le web, les deux compteurs sont indiqués comme indisponibles et `Interrupt` arrête les futurs rappels NSGo lorsque l’hôte reprend le contrôle.

Le web ne peut pas encore isoler un paquet de la page : son code atteindrait le réseau, le presse-papiers et le stockage propres à la page, y compris celui du magasin de confiance, au-delà de ses capacités. Sur cette cible, `tsx.View` et `tsx.Load` refusent un paquet avec `tsx.ErrUnisolated`, à moins de définir `Sandbox.WebUnisolated` pour l’exécuter avec l’autorité de la page, ce qui ne convient qu’aux paquets auxquels vous accorderiez la même confiance qu’à l’application elle-même.

## Éditeurs et tests

Un fichier `tsconfig.json` étendant `.nsgo/tsconfig.json` fournit aux éditeurs les types du TSX, que `go tool nsgo dev` et `go tool nsgo check` génèrent. Les fichiers `*.test.tsx` placés à côté de l’application, écrits avec `nsgo/test`, s’exécutent avec ses tests Go ([Tests](testing.md)).
