---
title: Componentes e estado
description: Elementos, componentes, signals e estilos.
group: Conceitos principais
order: 50
---

## Elementos

A tela é uma árvore de elementos, que são valores de struct em Go. `core` tem as primitivas: `View`, `Text`, `Image`, `Scroll`, `Input` e mais. `ui` tem os controles que um aplicativo usa, como `Button`, `Select`, `DatePicker` e `EmojiPicker`, com uma aparência que segue o tema. (`base` guarda os mesmos controles sem aparência definida, para [autores de componentes](authoring/index.md).)

```go
core.View{
	Style: core.Style{Padding: core.All(24), Gap: 12},
	Children: []core.Node{
		core.Text{Content: "Hello"},
	},
}
```

`core.WebView` exibe conteúdo web; um `WebSource` vazio carrega `about:blank` e relata os eventos usuais de carregamento e navegação. Defina `SourceFunc` como uma fonte vazia ou chame `Ref.Load(core.WebSource{})` para limpar a página anterior. Defina `InjectedJavaScriptBeforeContentFunc` quando o script executado antes dos scripts da própria página seguir um signal: quando ele muda, a view carrega novamente sua fonte com o novo script, no lugar da página exibida, mesmo uma página para a qual o usuário ou `Ref.Load` tenha navegado. Defina `UserAgentSuffix` (TSX `userAgentSuffix`) para adicionar um token, como `Version/26.0 Safari/605.1.15`, após o user agent da própria plataforma, ou após `UserAgent` quando estiver definido; no macOS e iOS, um sufixo adicionado ao da própria plataforma só tem efeito quando a view é criada pela primeira vez, e o HarmonyOS e a web mantêm seus próprios user agents. Anexe um `core.Ref` para chamar `Reload`, `GoBack` e `GoForward`. Em alvos nativos, esses comandos mantêm as fontes HTML e de dados do próprio aplicativo isentas de `core.AccessWebSchemes`; páginas que criam novas navegações ainda seguem a política de acesso.

No tvOS, `cx.Env().HasWebView()` é falso. Montar uma `WebView` relata um `*core.WebError` por meio de `OnError` sem carregar o WebKit; forneça conteúdo do aplicativo para esse alvo.

No macOS, iOS, Windows e Linux, `OnNavigationStateChange` também relata alterações de URL, histórico e título no mesmo documento.

Defina `WebView.Media` como `core.WebMicrophone`, `core.WebCamera` ou ambos para permitir solicitações `getUserMedia` de uma página. Declare as descrições de uso em `apple.info` e `RECORD_AUDIO` ou `CAMERA` em `android.permissions`; o sistema ainda perguntará ao usuário. No Android, uma solicitação pendente é recusada se o aplicativo remover um dos dispositivos solicitados, se a página sair ou se a view fechar enquanto o prompt do sistema estiver aberto. A página precisa solicitar o acesso novamente após tal mudança.

`WebView.Muted` e `MutedFunc` (TSX `muted`) silenciam todo o áudio da página no Windows e Linux, incluindo Web Audio e quadros incorporados. O macOS 12 e o iOS 15 ou posteriores suspendem a reprodução de mídia enquanto silenciados. Outros hosts mantêm sua reprodução normal. As atualizações se aplicam à página atual e persistem entre navegações.

Em TSX, uma alteração de `injectedJavaScriptBeforeContentLoaded` carrega a fonte novamente com o novo script na mesma web view, como faz `InjectedJavaScriptBeforeContentFunc`; uma alteração de `injectedJavaScript` cria uma nova web view, que carrega a fonte do zero, assim como uma alteração de `userAgent`, `userAgentSuffix` ou `mobile`. No HarmonyOS, uma alteração em qualquer um dos scripts injetados cria uma nova web view e inicia um novo histórico nativo. No Windows e no HarmonyOS, cada script injetado roda no quadro principal dentro de um bloco, portanto suas declarações de nível superior `let`, `const` e `class` permanecem locais a ele; atribua em `window` para criar um global que a página deva enxergar.

Defina `Mobile` (TSX `mobile`) para exibir uma página como um celular a vê no Windows, macOS e Linux, para sites como YouTube Shorts que trocam vídeos com deslizes verticais apenas em celulares: a página recebe o user agent de um celular, a menos que `UserAgent` esteja definido, e uma tela sensível ao toque sem hover, e um arrasto com o mouse, ou uma rolagem com a roda ou trackpad sobre o que não rola, chega até ela como o deslize de um dedo. O Windows emula um celular como faz o modo de dispositivo do Chrome. O macOS e o Linux fazem isso com um script, então as folhas de estilo de suas páginas ainda veem um mouse e os eventos de toque não são confiáveis, podendo ser ignorados por uma página. iOS, Android, HarmonyOS, web e miniprogramas exibem as páginas como elas são.

No macOS, iOS, Windows e Linux, `OnOpenWindow` recebe solicitações de nova janela de uma página web. A view não cria nenhum popup. Use `URL` e `UserInitiated` da solicitação para decidir se navega na mesma view com `Ref.Load`, abre a URL em outro lugar ou a ignora. Uma view restrita envia solicitações recusadas para `OnError`.

Defina `OnDownload` para salvar arquivos que as páginas de uma web view baixam no macOS, iOS, Linux, Windows e Android. Os downloads são salvos em `downloads/<id>/<file name>` dentro do diretório de dados do aplicativo e relatam seu início, conclusão com contagem de bytes ou falha; views sem `OnDownload`, ou views restritas, cancelam os downloads.

`core.Video` reproduz um arquivo ou URL no reprodutor da plataforma. No macOS, iOS e tvOS, defina `Video.AudioSource` para um arquivo ou URL separado contendo som para um clipe de vídeo finito. O áudio do reprodutor vem dessa fonte. A imagem e o som começam juntos, e `Ref.Seek` move ambos na mesma linha do tempo. O vídeo define a duração da reprodução, e um áudio mais curto fica em silêncio quando termina.

Um clipe transparente exibe o que está atrás dele como HEVC com alfa em um `.mov` no macOS (iOS ainda não foi verificado) e como VP9 com alfa em WebM no Linux com o plug-in `codecalpha` do GStreamer (`gstreamer1.0-plugins-bad`, que um pacote Linux pode recomendar em `nsgo.yaml`). No macOS, iOS, Windows e Linux, um clipe local cujo alfa não pode ser preservado (WebM em plataformas Apple, HEVC no Linux e qualquer um no Windows até que a saída com alfa do Media Foundation seja verificada em uma máquina Windows) faz com que `OnError` informe isso em vez de exibir o vídeo em uma caixa opaca: o erro é um `*core.MediaError` cujo `Reason` é `core.MediaErrorAlphaUnsupported`, permitindo que o aplicativo decodifique esse clipe por conta própria. Android e web, e URLs remotas em todas as plataformas, reproduzem tais clipes conforme seus decodificadores os entregam, o que pode ser opaco.

`OnPause` e `OnPlay` seguem os controles integrados na Apple, Android, Windows, Linux e web. `OnPause` fornece `VideoPauseControls` para uma pausa vinda de um controle. `Paused` se aplica quando uma fonte abre ou seu valor muda. Alterações de volume, velocidade e layout mantêm uma pausa ou reprodução nativa, incluindo uma reprodução pelos controles enquanto `Paused` permanece verdadeiro. Vincule `PausedFunc` ao seu estado e atualize-o nesses callbacks quando seu aplicativo exibir seu próprio botão de reproduzir.

`Rate` define a velocidade de reprodução e `RateFunc` pode alterá-la durante a reprodução. Use `VideoInfo.RateChangeable` vindo de `OnLoad` para exibir um controle de velocidade. O Linux consegue alterar a velocidade de vídeos com suporte a busca quando o GstPlay e o filtro `scaletempo` do GStreamer estão instalados. O reprodutor de fallback usa 1×.

Defina `Video.Headers` para enviar cabeçalhos de requisição HTTP como `Referer`, `User-Agent` ou `Cookie` no Android e para arquivos HTTP diretos como MP4 ou MOV no Linux, macOS, iOS e tvOS.

```go
core.Video{
    Source:  "https://video.example/clip.mp4",
    Headers: map[string]string{"Referer": "https://video.example/player"},
}
```

O vídeo copia o mapa quando é montado e usa os mesmos cabeçalhos se `SourceFunc` fornecer uma nova URL. Em plataformas Apple, use uma fonte com tamanho em bytes conhecido e suporte a range para permitir busca. O reprodutor envia os cabeçalhos configurados através de redirecionamentos de mesma origem. Os mesmos cabeçalhos também se aplicam a um arquivo HTTP direto em `AudioSource` nas plataformas Apple. URLs de playlist terminadas em `.m3u` ou `.m3u8` usam o carregador nativo.

No Linux, instale o GstPlay e os plug-ins `scaletempo` e `souphttpsrc` do GStreamer. Use a URL final da mídia se você definir `User-Agent` ou se seu `souphttpsrc` não possuir a propriedade `location-trusted`. Com essa propriedade e o user agent padrão, o GStreamer mantém os cabeçalhos do aplicativo em redirecionamentos de mesma origem. Os cabeçalhos do aplicativo se aplicam ao arquivo de origem, e segmentos de mídia adaptativa usam suas próprias requisições.

## Componentes

Um componente é uma função que recebe um escopo e retorna um elemento. Ele é executado uma única vez; o que muda depois é controlado por signals.

```go
func Greeting(cx *core.Scope) core.Node {
	return core.Text{Content: "Hello from NSGo"}
}
```

Um componente com props é um tipo com um método `Render`:

```go
type Badge struct{ Text string }

func (b Badge) Render(*core.Scope) core.Node {
	return core.Text{Content: b.Text}
}
```

`ui.ToastContent` pode omitir seu botão de fechar com `NoClose`, adicionar um nó `Trailing` e dispensar o toast por meio de `OnPress`. `Toasts.NoClose` oculta os botões de fechar em toda a pilha.

`ui.Toasts.Style` e `StyleFunc` personalizam cada superfície de toast, incluindo fundo, borda, cantos e preenchimento interno. O estilo da pilha é mesclado sobre o tema, seguido por `Style` e `StyleFunc` de cada `ToastContent`. Cada `StyleFunc` é executado após seu respectivo `Style` e é atualizado quando seus signals mudam. Um `Color` nesses estilos colore o título e a descrição com opacidade reduzida, para que o texto permaneça legível sobre uma superfície escura ou colorida. As fases de entrada, saída e deslize continuam aplicando seus estilos de movimento. Em TSX, defina a prop `style` de `Toasts` e passe `style` ou `styleFunc` para `show` para um toast individual.

## Signals

Um signal guarda um valor. Lê-lo dentro de uma função chamada por um elemento, como `ContentFunc`, inscreve aquela parte da tela, e apenas aquela parte é atualizada quando o valor muda.

```go
count := core.NewSignal(cx, 0)

core.Text{ContentFunc: func() string { return fmt.Sprintf("Pressed %d times", count.Get()) }}

ui.Button{Text: "Press me", OnPress: func() { count.Update(func(n int) int { return n + 1 }) }}
```

Um signal de um slice inteiro atualiza todas as linhas que o leem. `core.NewCollection` armazena itens por chave, cada um em seu próprio signal, de modo que cada linha acompanha seu próprio item e alterar um item executa apenas uma linha:

```go
todos := core.NewCollection(cx, func(t Todo) int { return t.ID }, nil)

core.List{Count: todos.Len, Key: func(i int) any { return todos.KeyAt(i) },
	Row: func(cx *core.Scope, row core.ListRow) core.Node {
		return core.Text{ContentFunc: func() string { t, _ := todos.At(row.Index()); return t.Title }}
	}}

todos.Update(id, func(t Todo) Todo { t.Done = true; return t })
```

Outros estados são construídos sobre signals da mesma maneira:

- `core.NewSelector` informa se uma chave é a selecionada e executa apenas as linhas que a seleção deixa e alcança.
- `core.NewHistory` mantém um valor com desfazer e refazer, com seus signals `CanUndo` e `CanRedo` para um menu Editar.
- `core.NewPersisted` mantém um valor tipado no armazenamento do aplicativo como JSON versionado, migrando o que versões anteriores gravaram.
- `core.From` acompanha uma fonte fora do grafo, como uma assinatura que entrega eventos em outra goroutine.

## Estilos

`core.Style` segue o CSS: flexbox e grid, paddings e margens, cantos, bordas, sombras, gradientes, transformações e estados como `Active`. Elementos também aceitam classes do Tailwind por meio de `Class`:

```go
core.Text{Content: "Done", Class: "self-start rounded-full bg-zinc-100 px-3 py-1 text-sm"}
```

Temas dão aos componentes de `ui` sua aparência nos modos claro e escuro.

## Aplicativos desktop

`nsgo.Run(Root)` exibe uma view raiz. Um aplicativo desktop pode abrir por conta própria o que exibe: `nsgo.Run(App)` executa `func App(app *core.Scope)` uma vez, e ela abre janelas, painéis, ícones na bandeja e atalhos globais com `desktop`, que compartilham seus signals. Isso funciona no macOS, Windows e Linux; no macOS, `examples/tray` fica fora do Dock e vive na barra de menus. `nsgo.Run(App, nsgo.StartHidden())` abre a janela principal sem exibi-la, para que um aplicativo de bandeja, ou um iniciado pelo sistema no login (`desktop.LaunchedAtLogin`), não a faça piscar na tela; o aplicativo a exibe com `app.MainWindow().Show()`. Celulares e a web exibem o aplicativo assim que ele é iniciado.

`desktop.SetWindowLevel` mantém uma janela acima das outras, como faz um timer ou um mascote de área de trabalho, ou abaixo delas, e `desktop.SetWindowFrame` a move e redimensiona. `desktop.CanSetWindowLevel` e `desktop.CanPlaceWindows` informam se eles têm efeito no escopo de uma janela: ambos têm no macOS, Windows e Linux sob X11, e nenhum em uma janela toplevel comum do Wayland, cujo compositor controla o empilhamento e posicionamento.

Uma janela aberta com `desktop.WindowOptions{Transparent: true}`, ou a janela principal com `nsgo.WindowTransparent()`, não tem fundo nem moldura, de modo que a área de trabalho aparece onde o aplicativo não desenha nada, como em um mascote de área de trabalho ou widget; uma view com `Desktop.WindowDrag` a move. `NoShadow` remove a sombra do sistema e `SkipTaskbar` mantém a janela fora da barra de tarefas do Windows e do Alt+Tab e da lista de tarefas do X11, enquanto `desktop.SetWindowTransparent`, `SetWindowShadow` e `SetSkipTaskbar` os alteram posteriormente. O Windows altera a transparência apenas em uma janela aberta como transparente, que não tem sombra, e o Wayland não tem dica para manter uma janela fora da lista de tarefas. `desktop.CanMakeWindowsTransparent` e `desktop.CanSkipTaskbar` informam no escopo de uma janela se isso tem efeito: transparência exige DirectComposition no Windows e um compositor no Linux, pois X11 sem compositor não a possui, e no macOS `desktop.SetDockVisible` remove o aplicativo do Dock.

## Terminal

Importe `base/terminal` e crie uma sessão com `terminal.New(terminal.Options{Conn: conn})` para um fluxo remoto, ou defina `Command` para um programa local no macOS, Linux ou Windows. Renderize `terminal.View{Terminal: session}` e chame `Close` quando a sessão terminar. `Options.ImageLimit` limita o armazenamento gráfico Kitty em bytes: zero usa 64 MiB e um valor negativo desativa imagens. Programas podem enviar imagens RGB, RGBA ou PNG e posicionamentos por placeholder Unicode.

Chame `Search(text)` para destacar correspondências na tela e no histórico de rolagem, `SearchNext` e `SearchPrevious` para selecioná-las e `EndSearch` para limpá-las. `Options.OnSearch` relata `SearchStatus`; `Theme.Search`, `SearchText`, `SearchSelected` e `SearchSelectedText` definem as cores das correspondências. Shells que emitem marcadores OSC 133 suportam Alt/Option-clique dentro de sua entrada de comando ativa para mover o cursor. O NSGo não instala scripts de integração de shell.
