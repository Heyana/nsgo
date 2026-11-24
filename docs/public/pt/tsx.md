---
title: Apps TSX
description: Escreva um app em TSX com classes Tailwind, compilado antecipadamente sem Node.
group: Conceitos principais
order: 63
---

## Um app TSX

Um app TSX é um pacote Go que incorpora seus fontes e executa sua entrada:

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

`go tool nsgo init -template tsx` cria um, e `examples/tsx` é outro. `nsgo dev` e `nsgo build` compilam o TSX antecipadamente, de modo que os pacotes não incluem nenhum compilador; `go run` e `go test` o compilam quando o app inicia. Em nenhum dos casos é preciso Node.

## Componentes e módulos

O TSX importa as primitivas de `nsgo`, os componentes de `base` de `nsgo/base`, os controles de jogo de `nsgo/gamepad` (sob a capacidade `gamepad` em pacotes assinados), os sons de `nsgo/audio` (sob `audio`) e os de `ui` de `nsgo/ui`, desde que o app importe `tsx/ui`; `examples/gallery-tsx` os demonstra.

Para diagramas Mermaid, importe `github.com/nsgo-dev/nsgo/tsx/diagram` pelo seu efeito no host Go, junto com `tsx/textview` ou `tsx/ui`. Isso adiciona `Diagram` a `nsgo/base` sem vincular o analisador de diagramas a apps que não o usam. Ele aceita `source`, `fontSize`, um `theme` parcial, `accessibilityLabel` e as props de estilo habituais. Use-o diretamente ou forneça um renderizador de TextView:

```tsx
import { Diagram, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderDiagram={(code) => <Diagram source={code.code} errorClass="font-mono" />} />
```

A mesma prop `renderDiagram` funciona na TextView de `nsgo/ui`. Um bloco de código que não é Mermaid mantém seu renderizador habitual; um diagrama inválido exibe sua fonte.

Para matemática tipografada, importe `github.com/nsgo-dev/nsgo/tsx/formula` pelo seu efeito no host Go, junto com `tsx/textview` ou `tsx/ui`. Isso adiciona `Formula` a `nsgo/base`; apps que omitem o pacote deixam o mecanismo de matemática e a fonte fora do seu binário. Ele aceita `source`, `dialect` (`tex` por padrão, ou `typst`), `display`, `accessibilityLabel` e props de estilo. O tamanho e a cor da fonte seguem o texto ao redor. Use-o diretamente ou forneça um renderizador de TextView:

```tsx
import { Formula, TextView } from "nsgo/base";

<TextView markdown={markdown}
  renderMath={(math) => <Formula source={math.tex} display={math.display} />} />
```

A mesma `renderMath` funciona na TextView de `nsgo/ui`. As fórmulas mantêm seu TeX quando copiadas. O `Renderer` Go continua sendo uma configuração do host.

`UrlScheme.url()` lê o endereço da aplicação rastreado; `UrlScheme.setUrl(url, { history: "push" })` adiciona uma entrada de endereço, ou a substitui quando as opções são omitidas. `UrlScheme.on("change", fn)` observa as ações de voltar e avançar do navegador e as mudanças de hash, e retorna uma função para cancelar a inscrição. Veja [Endereços da aplicação](addresses.md) para roteamento, validação de mesma origem e hospedagem de deep links.

## Go a partir do TSX

Registre Go para o TSX importar:

```go
tsx.Register("app/native", map[string]core.Node{"Badge": Badge{}})
tsx.RegisterFunctions("app/native", map[string]any{
	"greet": func(name string) string { return "Hello, " + name },
})
```

`import { Badge, greet } from "app/native"` então recebe os campos de `Badge` como props e chama `greet`. Funções que recebem um `context.Context` tornam-se promises, e `tsx.RegisterClass` transforma tipos Go em classes que o TSX cria com `new`.

Uma prop opcional repassada como `undefined` usa seu padrão. Ao registrar um componente Go com um getter cujo padrão difere do zero do seu tipo, adicione uma opção como `tsx.Default("Max", float64(100))`; o mesmo padrão se aplica enquanto uma prop reativa retorna undefined, sem substituir o componente.

Campos de dados com padrões Go documentados também podem ser omitidos: `width` e `height` de um nó `Flow` medem seu conteúdo JSX quando ausentes, o `width` omitido de uma aresta usa o traço padrão, e o `pace` omitido de uma linha de cota deixa sua marca de ritmo oculta.

Registrar componentes e classes Go encontra seus campos e métodos exportados por reflexão enquanto o app é executado. Essa busca impede que o linker Go descarte métodos exportados que nada chama, no app inteiro e não apenas no que o TSX registra, de modo que vincular `tsx` adiciona cerca de 20 MB a um binário desktop sem símbolos, mais quanto mais pacotes o app vincular.

## Pacotes assinados

Um app pode executar TSX que baixa, como suas próprias telas entregues novamente ou plugins, depois de verificar o pacote. Declare o que o código do pacote pode acessar no seu `nsgo.yaml`; todo o resto lhe é recusado:

```yaml
capabilities:
  network:
    hosts: [api.example.com, "*.cdn.example.com"]
  open_url: true # or { schemes: [https] }
  host: [search, "player.*"] # the app's handlers it may call
  # webview: { hosts: [...], input: true, media: [microphone, camera] }, clipboard, picker, speech, text_to_speech, status_bar, notifications, biometrics, sign_in_with_apple, purchases, secrets, audio, gamepad, auth_session
```

Mantenha uma chave raiz offline e certifique chaves de release com ela, depois assine o app nesse diretório. `go tool nsgo keygen` imprime um par de chaves e não grava nenhum arquivo; execute-o uma vez para a raiz e uma vez para cada chave de release. As chaves públicas vão em flags e no código do app; as chaves secretas ficam fora do repositório e chegam à ferramenta apenas por `NSGO_ROOT_KEY` e `NSGO_RELEASE_KEY`:

```sh
go tool nsgo keygen # once for the root, once for the release key
NSGO_ROOT_KEY=<root secret> go tool nsgo certify -key <release public> -apps 'com.example.*'
NSGO_RELEASE_KEY=<release secret> go tool nsgo publish -cert release.cert plugin
go tool nsgo verify -root <root public> plugin/dist/Plugin-1.0.0.nsgopkg
```

O app confia na chave pública da raiz, escrita no seu código, verifica o pacote fora da goroutine do engine e o exibe em `tsx.View`, em sandbox ao lado das suas próprias visualizações. O pacote alcança o app apenas pelo canal do host que a visualização lhe dá, e mantém sua própria parte do armazenamento e dos segredos do app:

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

`Sandbox.Grant` lhe concede menos do que ele declara, `Sandbox.Limits` define outros orçamentos de tempo e cotas, e `Sandbox.Modules` fornece funções e componentes Go além dos do próprio NSGo. Seus atalhos de teclado, manipuladores de voltar e pilhas de navegação só têm efeito enquanto o foco do teclado está na sua visualização, e ele só move o foco por conta própria (`focus()`, `autoFocus`, `Keyboard.dismiss`, `setAccessibilityFocus`) enquanto o foco já está lá ou em resposta a um toque do usuário nela. Ele é exibido em uma visualização própria que preenche o espaço que seu pai lhe dá, nada mais, e é recortado por ela, incluindo menus, popovers e toasts; em uma visualização de rolagem, coloque-o em uma visualização para deixá-lo assumir a altura do seu conteúdo. Um diálogo, um alerta, um menu de contexto ou um anúncio dele só aparece da mesma forma, e voltar fecha um diálogo dele mesmo que ele não o remova. `View.Export` nomeia um componente exportado pelo módulo de entrada do pacote, exibido no lugar da raiz que a entrada monta, cujo `mount` então nem renderiza nem executa seu componente, e que recebe `View.Props`, um mapa codificável em JSON copiado para cada visualização, de modo que um pacote fornece várias visualizações, cada uma com seu próprio estado, sob as mesmas capacidades e limites; uma exportação ausente gera panic com `tsx.ErrNoViewExport` e uma que não é função com `tsx.ErrViewExportNotComponent`. Um pacote cuja entrada exporta funções é um plugin de funções: `tsx.Load(cx, p, tsx.Sandbox{})` o executa de forma invisível e síncrona na goroutine de quem chama, sob um `cx` que pertence a uma árvore de visualizações (sem uma, retorna `tsx.ErrNoTree`), ou retorna o que seu módulo de entrada lançou ao carregar, e `Plugin.Call` as chama. `go tool nsgo dev -limits` executa um app como uma visualização dessas o faria, e `go tool nsgo revoke` assina uma lista de chaves de release e pacotes revogados para `Trust.UpdateRevocations`. `examples/plugins` pede ao usuário que conceda as capacidades de um pacote, instala-o e o executa. `Limits.Memory`, 64 MiB por padrão, orça o armazenamento nativo medido para arrays, strings, buffers e typed arrays. Isso inclui cópias de `concat`, `flat`, `slice`, spread de array, concatenação de strings, templates, `replace`, `replaceAll`, `TextEncoder` e `Blob`. `memory=<bytes>` em `-limits` altera o orçamento, e `memory=0` o desativa. As verificações ocorrem antes da alocação. Um pacote que excede o orçamento para com `ERR_NSH_QUOTA_EXCEEDED` e recurso `memory` depois que o Go verificou se há armazenamento coletável. Objetos comuns, closures, pilhas, alocações pequenas e built-ins e APIs do host não medidos também podem alocar. O navegador mantém seu próprio heap. Um app que assina pacotes ele mesmo com `pkg.Build`, ou com `go tool nsgo publish -extra <file>` com os bytes desse arquivo, pode colocar metadados próprios em `pkg.Contents.Extra`, como o que um plugin adiciona aos seus menus: eles são assinados com o pacote, e `Verify` os retorna inalterados como `p.Extra` para o app analisar, de modo que o app os lê sem executar o código do pacote.

Um app pode assinar seus próprios documentos JSON, como um catálogo dos seus plugins, com `pkg.SignDocument` e verificá-los com `Signed.Open`, usando as chaves que `nsgo keygen` imprime. Dê a cada tipo de documento um contexto `app:` próprio com uma versão, como `app:catalog-v1`: um documento nunca passa por outro tipo, nem por um pacote, certificado ou lista de revogação. `Open` confia exatamente nas chaves públicas que recebe, sem certificado nem lista de revogação, e recusa campos do corpo que o tipo em que decodifica não possui. O envelope não contém tempo nem sequência, então um documento mais antigo é verificado tão bem quanto um mais novo: coloque uma versão ou validade no corpo e verifique-a.
Em uma sandbox nativa, `JSON.stringify`, `JSON.parse` com um reviver, `Array.prototype.join`, `toString`, `toLocaleString` e `flat`, e portanto `String()` de um array, lançam um `RangeError` capturável quando ultrapassam 1.000 níveis de profundidade, contando um dentro do outro, já que o `toString` de um array e o `join` que ele chama contam cada um; dados mais profundos travariam a chamada ou encerrariam o app. Na web são usados os built-ins do navegador.

Em alvos nativos, `DefaultLimits.Stack` é de 1.000 frames. Defina `Limits.Stack` ou `-limits stack=<frames>` para escolher uma profundidade diferente de até 10.000 frames. Valores Go menores ou iguais a zero e entradas acima do teto usam 10.000. Na web é usado o limite de pilha do navegador.

Funções nativas que chamam umas às outras contam para `Limits.Stack` como as chamadas JS: um getter, `toString` ou `valueOf` que é um built-in, uma expressão regular cujo `exec` é outro dos seus métodos, `Reflect.apply` e `Reflect.construct` um dentro do outro, e cadeias de funções vinculadas ou proxies lançam um `RangeError` capturável ao ultrapassá-lo em vez de encerrar o app, e um valor de plugin aninhado mais fundo do que isso faz falhar com um `RangeError` a chamada que o converte para Go. Isso vem do fork do goja que o NSGo exige, `github.com/nsgo-dev/goja`, que seu app recebe com o NSGo sem nenhuma alteração no seu `go.mod`.

Em alvos nativos, `Limits.Call` limita cada fatia de JavaScript, incluindo callbacks de timers e continuações de promises após `await`. Quando uma fatia é cortada, um `Plugin.Call` pendente informa `tsx.ErrBudget` uma vez, para seus timers restantes e ignora respostas tardias do host. Chamadas independentes continuam sendo executadas, incluindo reações de promises enfileiradas na fatia interrompida. Uma chamada que resolve a promise de outra ainda informa seu próprio resultado ou erro. Expressões regulares com backtracking usam o menor orçamento entre o de carregamento e o de chamada como tempo limite de correspondência; um tempo esgotado não tratado também se torna `tsx.ErrBudget`.

`Limits.CallTotal` adiciona um prazo de tempo de relógio para cada chamada de plugin e evento de visualização em sandbox. Ele inclui o tempo gasto esperando e abrange seus timers, intervalos, frames, continuações de promises e respostas do host mesmo depois que a chamada se resolve. No prazo, o trabalho pendente é descartado e uma chamada ainda em espera falha com `tsx.ErrBudget`. O trabalho do módulo de entrada fica sujeito a `Limits.Load`, usando `CallTotal` se `Load` for zero. Eventos de host independentes usam `CallTotal` a partir da entrega. Use `Plugin.CallWithBudget` para obter um handle para o prazo de uma chamada em espera. No `Scope.Post` de um host, chame `budget.Extend(duration)` antes desse prazo para dar mais tempo apenas a essa chamada.

`Plugin.Interrupt` para o runtime a partir de qualquer goroutine, descarta seus timers pendentes e faz falhar as chamadas atuais e posteriores. `Plugin.Stopped()` e `Plugin.Err()` também podem ser lidos de qualquer goroutine. `Err()` é nil enquanto o runtime está em execução e retorna seu primeiro motivo de parada depois que ele para. Fechar o plugin ou descartar seu dono normalmente executa suas rotinas de limpeza e o para com `tsx.ErrPluginClosed`. Defina `Sandbox.OnStop` ao chamar `tsx.Load` para receber esse motivo uma vez na goroutine do engine, depois que as chamadas em espera falham.

Defina `Sandbox.OnError` ao carregar um plugin de funções para receber falhas não capturadas de timers, animation frames, `queueMicrotask` e `reportError` fora de um `Plugin.Call` em espera. O manipulador de cada plugin é executado na goroutine do engine, de modo que um host pode atribuir falhas e chamar `Plugin.Interrupt` quando seu disjuntor abre. Cortes de orçamento de fatia nativos também informam `tsx.ErrBudget`, incluindo um job de promise cuja chamada já se resolveu. Erros comuns de callback deixam o plugin em execução. Erros de carregamento e de resultado de chamada ainda retornam a quem chamou, e as visualizações ainda usam `core.Catch`; sem um manipulador, o comportamento existente de limites e de relatórios do app permanece.

Defina `Sandbox.OnViewRuntime` para receber o `*tsx.ViewRuntime` de cada visualização na goroutine do engine antes que seu módulo de entrada seja executado. Chame `Interrupt` de qualquer goroutine para parar seu trabalho e deixar a última visualização renderizada no lugar. `Stopped` e `Err` informam seu estado terminal, com `tsx.ErrViewClosed` no descarte normal. `Sandbox.OnStop` também informa o motivo terminal de uma visualização uma vez.

`Plugin.JSTime` e `ViewRuntime.JSTime` informam o tempo decorrido nas entradas JavaScript do runtime nativo, incluindo a entrada atual. A espera entre entradas e o tempo dentro de funções Go síncronas registradas são excluídos. `ViewRuntime.EngineTime` também inclui chamadas Go síncronas e o trabalho reativo, de montagem, de commit e de layout que lhe pertence, incluindo trabalho ainda em execução. Ele exclui a espera entre entradas do engine, o trabalho pertencente ao host ou a outra visualização, a contabilidade compartilhada de commit e o desenho nativo. Ambos os contadores podem ser lidos com segurança de qualquer goroutine. Os turnos de JavaScript do navegador fora do NSGo não podem ser medidos nem interrompidos preemptivamente. Lá, ambos os contadores informam indisponível e `Interrupt` para futuros callbacks do NSGo quando o host recupera o controle.

A web ainda não consegue isolar um pacote da página: seu código alcançaria a rede, a área de transferência e o armazenamento da própria página, incluindo os do repositório de confiança, além das suas capacidades. Lá, `tsx.View` e `tsx.Load` recusam um pacote com `tsx.ErrUnisolated`, a menos que você defina `Sandbox.WebUnisolated` para executá-lo com a autoridade da página, o que serve apenas para pacotes aos quais você confiaria a autoridade do próprio app.

## Editores e testes

Um `tsconfig.json` que estende `.nsgo/tsconfig.json` dá aos editores os tipos do TSX, que `go tool nsgo dev` e `go tool nsgo check` geram. Arquivos `*.test.tsx` ao lado do app, escritos com `nsgo/test`, são executados com seus testes Go ([Testes](testing.md)).
