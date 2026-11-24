---
title: Execução e depuração
description: Execute um aplicativo em qualquer alvo com hot reload e veja seus erros, atualizações, layout, velocidade e outros idiomas.
group: Ferramentas
order: 20
---

## Executar

```sh
go tool nsgo dev
```

`dev` compila o aplicativo para esta máquina e o abre, ou para o alvo que `dev.target` do `nsgo.yaml` indicar. A cada salvamento de um arquivo Go, ele recompila e reinicia o aplicativo, executando `go mod tidy` antes quando o aplicativo importa um pacote cujo módulo falta em `go.mod` ou `go.sum`; uma alteração apenas em TSX atualiza o aplicativo em execução no local, preservando seu estado. A execução termina junto com o aplicativo.

No macOS e Windows, uma recompilação nativa de desenvolvimento reabre a janela da view principal na sua última posição e tamanho normais dentro da sessão atual. O Windows também restaura a tela cheia. No Linux, o GTK restaura o tamanho normal e solicita seu último modo de tela cheia. O gerenciador de janelas escolhe a posição.

`-target` o executa em outro ambiente, e `-device` seleciona o iPhone, Apple TV, Apple Watch, simulador ou dispositivo Android por nome, UDID ou número de série:

```sh
go tool nsgo dev -target tui                # terminal cells; diagnostics in .nsgo/tui.log
go tool nsgo dev -target web                # in the browser, at the address it prints, on -port (8080 by default)
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target ios                # on the connected iPhone, signed by Xcode
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator, over adb
go tool nsgo dev -target miniprogram_wechat # in WeChat DevTools (also _qq, _alipay, _douyin)
```

O servidor de desenvolvimento escuta em todas as interfaces por padrão. Use `-host 127.0.0.1` para mantê-lo em loopback, inclusive quando o Android se conecta via `adb reverse`, ou informe o endereço IP de uma interface local para escutar apenas nela. Use um endereço acessível pela rede local para recursos de desenvolvimento em dispositivos Apple físicos.

`-h` lista todas as flags.

## Depurador Go

Coloque uma versão do [Delve](https://github.com/go-delve/delve/blob/master/Documentation/installation/README.md) compatível com a sua versão do Go no `PATH` como `dlv`. Inicie um aplicativo desktop nativo com suas informações de depuração e com a otimização do Go desabilitada.

```sh
go tool nsgo dev -target native -debug
```

Copie a porta da linha `API server listening at` do Delve. No VS Code com a extensão Go, adicione uma configuração de anexo (`attach`) e substitua `40000` por essa porta.

```json
{
  "name": "Attach to NSGo",
  "type": "go",
  "request": "attach",
  "mode": "remote",
  "debugAdapter": "dlv-dap",
  "host": "127.0.0.1",
  "port": 40000
}
```

No GoLand, adicione uma configuração de execução **Go Remote** com host `127.0.0.1` e a mesma porta. Defina um breakpoint no código Go do seu aplicativo, conecte o depurador e execute a ação que chega até ele. Após uma recompilação Go bem-sucedida, reconecte o depurador no mesmo endereço para que ele envie os breakpoints ao novo processo. Alterações em TSX continuam usando hot reload.

`-debug` funciona no macOS, Windows e Linux com `-target native`. Use uma compilação de desenvolvimento para depurar e reserve `-release` para medir uma compilação de lançamento.

## Pré-visualizações de componentes

Coloque uma ou mais funções `PreviewXxx` ao lado dos componentes em um pacote Go importável. Salve-as em um arquivo Go protegido pela build tag de pré-visualização.

```go
//go:build nsgopreview

package components

import "github.com/nsgo-dev/nsgo/core"

func PreviewEmpty(cx *core.Scope) core.Node {
    return core.Text{Content: "No items"}
}
```

```sh
go tool nsgo dev -preview ./components
go tool nsgo dev -target web -preview ./components
```

Marque os fixtures que deseja exibir juntos e escolha um locale, tema, aparência clara ou escura, tamanho de viewport e direção de layout. A pré-visualização usa o `nsgo.yaml` do aplicativo mais próximo, seus catálogos de tradução, `theme` configurado e recursos. Passe um diretório de aplicativo após as flags para selecionar uma configuração diferente no mesmo módulo. Nesse caso, o caminho da pré-visualização é relativo ao diretório do aplicativo.

Salvar um arquivo recompila a pré-visualização e mantém sua seleção e configurações de exibição. A tag `nsgopreview` exclui os fixtures das compilações normais de lançamento. Use o mesmo fixture em um arquivo `_test.go` marcado com a tag usando `nsgotest.Start(t, PreviewEmpty)` e execute-o com `GOFLAGS=-tags=nsgopreview go tool nsgo test -target macos ./components` para tirar uma captura de tela nativa.

Após a primeira instalação do aplicativo principal com `-target ios_simulator`, uma recompilação apenas em Go substitui o executável instalado e reinicia o aplicativo. Editar `nsgo.yaml`, recursos empacotados ou o ícone do aplicativo aciona uma reinstalação.

Use `go tool nsgo dev -target android -android-host` para trabalhar na interface Android com um ciclo de recompilação Go mais curto. A ferramenta instala o runtime nativo uma vez e executa o Go na máquina de desenvolvimento. Cada recompilação Go se reconecta à mesma Activity via adb. Alterações em recursos empacotados, classes pré-compiladas, ícones do iniciador ou `nsgo.yaml` instalam o APK de runtime novamente. O processo Go usa as restrições de compilação, arquivos e rede do host. `Scope.DiskSpace` relata o volume que contém o `DataDir` do host. Use o comando dev normal do Android para exercitar arquivos `_android.go` e o comportamento do Go no dispositivo.

## Erros

Uma falha de compilação aparece sobre o aplicativo em execução, com botões para copiar o erro e compilar novamente; corrija o arquivo e o próximo salvamento o substitui. Um componente que entra em panic enquanto renderiza aparece da mesma forma. Command+R no aplicativo, ou Control+R fora das plataformas Apple, compila e o inicia novamente a qualquer momento, inclusive sobre o erro, a menos que o aplicativo vincule a própria tecla, com `cx.OnKey` ou como um item habilitado do menu do aplicativo; no navegador, a tecla recarrega a página normalmente. A saída do próprio aplicativo, como `log.Print`, aparece no terminal abaixo das linhas da ferramenta.

Antes de executar, `go tool nsgo check` verifica o aplicativo e faz checagem de tipos no TSX; `-target all` o verifica para todos os alvos, de modo que código atrás de build tags de outra plataforma também é verificado a partir de um Mac.

### Logs de aplicativos Apple empacotados

No macOS, iOS, tvOS e watchOS, o logger padrão do Go grava no log unificado da Apple, incluindo `log.Print`, diagnósticos do NSGo e saída de console do TSX. No Console, filtre pelo identificador de pacote (bundle identifier) do aplicativo em execução como subsistema e `nsgo` como categoria. No macOS você também pode consultar o histórico:

```sh
log show --last 10m --predicate 'subsystem == "dev.example.hello"'
```

Os registros usam o tipo de log padrão e mantêm seus prefixos de aviso ou erro. O texto deles é público, portanto mantenha credenciais e outros segredos fora dos logs. Mensagens longas viram várias entradas. A saída no terminal continua disponível durante o desenvolvimento; um gravador de log personalizado ou manipulador `slog` controla sua própria saída.

### Web views no Linux

O Ubuntu 24.04 e versões posteriores podem restringir namespaces de usuário sem privilégios por meio do AppArmor. Quando o bubblewrap não consegue iniciar o sandbox do WebKitGTK, o NSGo registra o erro e exibe um espaço reservado. Algumas versões do WebKitGTK, caso contrário, encerram com `SIGTRAP` ao abrir a primeira web view. [Documentação do Ubuntu AppArmor](https://documentation.ubuntu.com/security/security-features/privilege-restriction/apparmor/)

Use o perfil do AppArmor para bubblewrap da sua distribuição se ela fornecer um, como `bwrap-userns-restrict`. Caso contrário, um administrador pode carregar um perfil concedendo `userns` a `/usr/bin/bwrap` usando [o perfil do CI Linux do NSGo](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/test.yml) como ponto de partida. O administrador gerencia o perfil porque ele se aplica a todos os aplicativos que usam o bubblewrap do sistema.

`WEBKIT_DISABLE_SANDBOX_THIS_IS_DANGEROUS=1` desativa o sandbox de subprocessos do WebKit. Use-o apenas para desenvolvimento local com conteúdo confiável.

## DevTools

```sh
go tool nsgo dev -devtools
```

`-devtools` abre o DevTools no navegador, servido por `nsgo dev` para todos os alvos, inclusive aplicativos de celular. Ele mostra o aplicativo em execução enquanto roda:

- **Elements**: as views em uma árvore. Cada uma mostra o componente que a construiu, um link para sua linha no seu editor, o seletor pelo qual um teste a encontra, suas classes, seus limites, suas margens, bordas e preenchimentos conforme o layout aplicado e o estilo definido. Passar o mouse sobre uma linha desenha sua margem, preenchimento e conteúdo no aplicativo, como faz o DevTools de um navegador, e **Pick** permite pressionar uma view em qualquer janela, painel, sheet ou view incorporada para encontrá-la na árvore.
- **State**: os signals que seu código criou, sob os componentes e computações que os possuem, com seus valores, os valores dos memos e quantas computações leem cada um. Um valor que nenhuma computação lê não muda nada na tela. Strings, booleanos e números podem ser editados, permitindo testar um estado sem alterar o código. Valores que o próprio NSGo criou ficam sob **NSGo's own**.
- **Profiler**: o tempo de cada commit, dividido entre seu código antes dele, o commit com seu layout e a aplicação na thread principal da plataforma. Ele também mostra as linhas cujas escritas causaram o commit e abre nas computações executadas, da mais demorada para a mais rápida. Uma segunda lista nomeia as computações que mais rodaram no último segundo, pela linha que as criou. O runtime do Android aplica lotes no seu próprio código Java, portanto commits no Android não têm tempo de plataforma.
- **Recorder**: o que você pressiona, digita, limpa e rola no aplicativo, escrito como um arquivo de teste `nsgotest` para copiar em um arquivo `_test.go`. Em Elements, **Assert shown** e **Assert text** adicionam uma verificação da view selecionada. Uma verificação de texto guarda o texto no idioma em que o aplicativo o exibiu, portanto grave no idioma em que seus testes rodam.
- **Highlight updates** contorna cada view que um commit altera, como faz `-highlight`.
- **Layout** exibe e oculta a [auditoria de layout](#layout) no dispositivo, como `-layout` a inicia.

O estado, as computações e as escritas de um aplicativo TSX são nomeados por suas linhas TSX, por meio dos source maps dos pacotes, e apontam diretamente para os arquivos do seu aplicativo.

O DevTools lê o aplicativo através do servidor do `nsgo dev`, que escuta na sua rede para que os celulares o alcancem. Cada execução gera um token, que os endereços impressos e abertos pelo `nsgo dev` para o DevTools e para o alvo web carregam após `#`, e o servidor responde apenas a requisições que o carregam, então abra o DevTools e o alvo web a partir desses endereços. Qualquer pessoa a quem você der um desses endereços pode alterar o estado do aplicativo enquanto ele roda e adicionar etapas ao teste que o Recorder escreve, portanto leia um teste gravado antes de executá-lo. Outras pessoas na sua rede e páginas web de outros sites que você visita não alcançam nada disso.

## Atualizações

```sh
go tool nsgo dev -highlight
```

`-highlight` contorna por meio segundo cada view que um commit alterou, de modo que uma atualização que redesenha mais do que deveria fica visível na tela conforme você usa o aplicativo. Os contornos aparecem sobre toda janela, painel, modal, sheet do iOS e view incorporada (`nsgo.NewView`). O DevTools os ativa e desativa enquanto o aplicativo roda.

```sh
go tool nsgo dev -trace
```

`-trace` imprime cada commit do aplicativo em execução: seu número, seu tempo e os itens de montagem enviados, para que você veja quais views uma atualização criou, alterou ou removeu. Acima dos itens, ele nomeia do que o commit decorreu: as linhas do seu código que escreveram em signals desde o último commit, as que mais escreveram primeiro, e quantos bindings, effects e memos foram executados.

```text
── commit 12: 40 items, commit 1.65 ms
   after app/market.go:473 ×4, app/market.go:487 ×4, app/market.go:384
   ran 118 bindings, 18 effects, 5 memos
```

Uma linha que escreve muitas vezes, ou muitos bindings executados para uma única escrita, indica onde agrupar escritas (`cx.Batch`) ou restringir o que um binding lê. `go tool nsgo test -trace` imprime o mesmo abaixo de cada passo de um teste.

## Perfis de runtime

Capture a atividade de CPU Go, o heap ou o trace de execução de um aplicativo em execução com `nsgo dev`. O aplicativo envia a captura para o servidor dev, que a salva na máquina que executa o comando.

```sh
go tool nsgo dev -target android -profile cpu -profile-duration 10s
go tool pprof -top .nsgo/profiles/cpu.pprof
```

O exemplo de CPU também funciona em um desktop, em um iPhone ou em um simulador iOS. Em um iPhone, permita o acesso à Rede Local se o sistema solicitar. O heap é um instantâneo e funciona também na web. Para eventos do escalonador e de goroutines, execute `go tool nsgo dev -profile trace` e abra `.nsgo/profiles/execution.trace` com `go tool trace`. Capturas de CPU e traces de execução têm padrão de 10 segundos. `-profile-output` define o caminho do arquivo local. Para pré-visualizações de componentes, os caminhos padrão e relativos usam o diretório do aplicativo. Uma execução dev salva sua primeira captura completa e depois continua executando o aplicativo.

```sh
go tool nsgo dev -target web -profile heap
go tool pprof -top .nsgo/profiles/heap.pprof
```

O runtime de navegador do Go suporta perfis de heap e traces de execução. Para amostras de CPU no navegador, use as ferramentas de Performance do navegador. Escolha `-profile` ou `-release`. Empacotar com `nsgo build` deixa o cliente de captura dev de fora. [Diagnósticos de runtime do Go](https://go.dev/doc/diagnostics)

## Layout

```sh
go tool nsgo dev -layout
go tool nsgo dev -layout=bounds,grid=4
```

`-layout` inicia o aplicativo com a auditoria de layout do DevTools sobre cada janela, painel, sheet do iOS e view incorporada, que contorna cada view: sua caixa de borda em azul, a borda interna de seu preenchimento em verde e a borda externa de sua margem em laranja, cada uma cortada onde uma scroll view ou uma view com recorte a oculta. `grid` desenha uma grade de 8 pontos, e `grid=N` uma de N pontos, no mínimo 4. Informe o valor após `=`: a flag sozinha significa `bounds`.

Uma leitura no canto inferior mostra o tamanho da janela em pontos, com botões que ligam e desligam as caixas, a grade e a régua. Na Apple TV e em outros dispositivos somente com controle remoto, o foco alcança os botões da leitura para que você possa alterná-los com o controle sem sair do aplicativo. Enquanto a régua está ligada, os toques escolhem dois pontos em vez de chegar ao aplicativo, e a leitura mostra a distância horizontal e vertical entre eles; um terceiro toque começa novamente. Na Apple TV ou com um teclado, as setas direcionais (ou deslizar o touchpad do Siri Remote) movem uma mira pela tela e Select, Return ou Espaço escolhem os pontos, atualizando a distância em tempo real até que o segundo ponto a trave.

| Tecla | Efeito |
| --- | --- |
| `Cmd+Option+Shift+L`, `Ctrl+Alt+Shift+L` | Exibe ou oculta a auditoria, em uma compilação com `-layout` |
| Setas, D-pad do controle / deslize | Move a mira da régua pelo passo da grade (8 pt por padrão) |
| `Shift` + Setas | Move a mira da régua em 1 pt |
| Select, `Return`, `Space` | Escolhe o ponto da mira durante a medição |
| `Esc`, voltar, Menu | Para a medição |

Se o seu aplicativo vincular a mesma tecla, a vinculação do aplicativo vence. Sem a tecla, como em um celular, execute também com `-devtools` e exiba ou oculte a auditoria usando **Layout** no DevTools; oculte-a para alcançar um controle sob a leitura. Os backends nativos e a web leem matrizes de apresentação para que os limites, o Pick e a auditoria acompanhem transformações exibidas, animações e rolagem; o watchOS acompanha seu desenho afim. HarmonyOS e miniprogramas mantêm o rastreador do lado Go até que suas camadas de renderização relatem a geometria. Redesenhar esses elementos custa tempo em cada atualização, portanto meça a velocidade sem eles.

## Velocidade

```sh
go tool nsgo dev -fps
go tool nsgo dev -release -fps
```

`-fps` exibe o monitor de desempenho sobre o aplicativo no macOS, iOS, Windows, Linux e Android. Ele mede quadros e commits, e o Android também mostra o tempo de ida e volta de texto nativo. Compilações de desenvolvimento carregam o cliente dev; meça com `-release`, que compila como faz `nsgo build`.

## Idiomas e aparência

```sh
go tool nsgo dev -lang zh-Hans
go tool nsgo dev -lang en-XA   # accented and longer, to find untranslated and cut text
go tool nsgo dev -lang ar-XB   # right to left
```

`-lang` inicia o aplicativo em um locale, inclusive um pseudo-locale ([Localização](localization.md)).

## Agentes

```sh
go tool nsgo dev -automation
go tool nsgo dev -target android -automation
go tool nsgo dev -target web -automation
```

`-automation` permite que um agente de IA ou um executor de ponta a ponta use o aplicativo em execução como uma pessoa faz, no macOS, Windows, Linux (`-target native`), Android (`-target android`), web (`-target web`), miniprogramas nos simuladores de suas ferramentas de desenvolvedor (`-target miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` ou `miniprogram_douyin`) e dispositivos e simuladores Apple (`-target ios`, `tvos`, `watchos`, `ios_simulator`, `tvos_simulator` ou `watchos_simulator`) até o momento: listar suas janelas, descrever o que uma exibe, pressionar, digitar, enviar teclas, rolar, arrastar, passar o cursor, escolher itens de menu, responder a alertas, aguardar pelo que aparece e tirar capturas de tela. Os agentes acessam via MCP por meio de `go tool nsgo mcp`, que usa o `dev -automation` em execução para o diretório, ou inicia um com as mesmas flags (como `-target android`) e o encerra quando o cliente sai:

```sh
claude mcp add nsgo -- go tool nsgo mcp
```

Outros clientes aceitam o mesmo comando, `go` com os argumentos `tool nsgo mcp`, executado no diretório do aplicativo. As views são nomeadas com os seletores dos [testes](testing.md#um-teste), ou com as referências `view:` que um instantâneo imprime, as quais um salvamento invalida ao reiniciar o aplicativo.

O agente atua nos dados reais do aplicativo, conforme `dev` o executa. Apenas compilações `dev -automation` incluem esse recurso, e apenas ferramentas nesta máquina que possuam o token em `.nsgo/automation.yaml`, que somente você pode ler, o alcançam.

### Ferramentas MCP definidas pelo aplicativo

Chame `automation.Register` a partir de um aplicativo ou biblioteca Go na goroutine do motor. Por exemplo, esta função informa aos agentes o ID da cena exibida no momento.

```go
import (
    "context"
    "encoding/json"

    "github.com/nsgo-dev/nsgo/automation"
    "github.com/nsgo-dev/nsgo/core"
)

func registerSceneTool(cx *core.Scope, sceneID *core.Signal[string]) error {
    return automation.Register(cx, automation.Tool{
        Name:        "current_scene",
        Description: "Read the ID of the active scene",
        InputSchema: json.RawMessage(`{"type":"object"}`),
        Handler: func(_ context.Context, _ json.RawMessage) (any, error) {
            return map[string]string{"id": sceneID.Get()}, nil
        },
    })
}
```

O nome deve ser único no grafo do aplicativo e distinto dos nomes integrados. `nsgo mcp` lista a ferramenta até que o escopo seja limpo. O manipulador é executado na goroutine do motor, podendo ler ou definir signals. Ele recebe um objeto JSON, valida os argumentos necessários e retorna um valor serializável em JSON ou um erro.

Em um teste, capture o escopo quando o aplicativo iniciar, chame `automation.Call(cx, ctx, name, args)` dentro de `app.Post` e use `app.Settle()` antes de verificar o resultado ou a view. O registro fica ativo em `dev -automation` e nos testes, e não faz nada em compilações de lançamento.

## Flags de toda compilação

Toda compilação de um aplicativo, pacotes incluídos, aceita as próprias flags do NSGo ao iniciar, que substituem o `nsgo.yaml` e o código do aplicativo e nunca chegam às flags do próprio aplicativo:

| Flag | Efeito |
| --- | --- |
| `--nsgo-fps` | Exibe o monitor de desempenho no macOS, iOS, Windows, Linux e Android |
| `--nsgo-lang=zh-Hans` | Inicia o aplicativo em um locale, inclusive um pseudo-locale |
| `--nsgo-appearance=dark` | Inicia o aplicativo em `light`, `dark` ou seguindo `system` |
| `--nsgo-os=ios` | Executa uma compilação web como `macos`, `windows`, `linux`, `ios` ou `android`, sendo `web` ela mesma |

```sh
./Hello --nsgo-fps                                   # Windows and Linux
open Hello.app --args --nsgo-fps                     # macOS
xcrun simctl launch booted dev.example.hello --nsgo-fps
adb shell am start -n dev.example.hello/dev.oneo.nsgo.NsgoActivity --ez nsgo-fps true --es nsgo-lang de
```

Na web, elas vão na query da página: `?nsgo-fps&nsgo-lang=de`.

`--nsgo-os` pré-visualiza o layout de outra plataforma em um navegador: o aplicativo lê aquele sistema operacional e seu idioma de interface (idiom), um celular para iOS e Android e um desktop nos demais casos, e `Env.Simulated` retorna verdadeiro. O navegador não possui nenhuma das partes nativas daquela plataforma: `NavHeader` e a barra de abas desenhada tomam o lugar das do sistema, não há menu do aplicativo, não há menu popup nativo (`desktop.PopupMenu`) nem funções de janela, e `mobile.IsAvailable` e `desktop.IsAvailable` são falsos. Menus desenhados como `ui.ContextMenu` e `ui.Menu` continuam funcionando. Os atalhos seguem o teclado do visitante. Outras compilações ignoram a flag e registram um aviso.
