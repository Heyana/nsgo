---
title: Comparação com outros frameworks
description: Onde o NSGo se encaixa ao lado do NSH, MyGo e outros frameworks de interface, e do que ele abre mão.
order: 2
---

O lugar do NSGo entre os frameworks de interface vem de três escolhas: os aplicativos são escritos em Go (ou TSX sobre ele), cada plataforma desenha com suas próprias views, e uma única base de código cobre celulares, desktops e a web. Outros frameworks fazem outras escolhas, e cada escolha tem um custo.

## Visão geral

| Framework | Linguagem | Desenha com | Alvos |
| --- | --- | --- | --- |
| NSGo | Go, TSX | As views de cada plataforma | iOS, App Clips, tvOS, watchOS, Android, macOS, Windows, Linux, web; miniprogramas, experimentalmente |
| [NSH](https://nsh.dev) | Rust, TSX | As views de cada plataforma | iOS, Android, macOS, Windows, Linux, web, aplicativos GPUI, terminais; HarmonyOS e miniprogramas do WeChat por meio de plataformas da comunidade |
| [MyGo](https://github.com/egoist/mygo) | Go, páginas web | Seu próprio renderizador em GPU ou a webview do sistema | macOS, Windows, Linux, web |
| [Fyne](https://fyne.io), [Gio](https://gioui.org) | Go | Seu próprio renderizador | Desktops, celulares, web |
| [Wails](https://wails.io) | Go, páginas web | A webview do sistema | Desktops |
| [Flutter](https://flutter.dev) | Dart | Seu próprio renderizador | Celulares, desktops, web |
| [Compose Multiplatform](https://www.jetbrains.com/compose-multiplatform/) | Kotlin | Seu próprio renderizador; o Compose do Android | Celulares, desktops, web |
| [React Native](https://reactnative.dev) | JavaScript, TSX | As views de cada plataforma | iOS, Android; outros por meio de plataformas da comunidade |
| [.NET MAUI](https://dotnet.microsoft.com/apps/maui) | C# | Os controles de cada plataforma | iOS, Android, macOS, Windows |
| [Avalonia](https://avaloniaui.net) | C# | Seu próprio renderizador | Desktops, celulares, web |
| [Qt](https://www.qt.io) | C++, QML | Seu próprio renderizador | Desktops, celulares, dispositivos embarcados |
| [Electron](https://www.electronjs.org), [Tauri](https://tauri.app) | Páginas web; Rust para Tauri | Um Chromium empacotado; a webview do sistema para Tauri | Desktops; celulares para Tauri |

## Pontos fortes

- **Views nativas.** Texto, métodos de entrada, seleção, rolagem, acessibilidade e menus são os da própria plataforma, de modo que um aplicativo se comporta como os usuários esperam nela, enquanto um framework que desenha por conta própria precisa reconstruir cada um desses recursos. O que apenas uma plataforma possui, como o Liquid Glass do iOS, as outras imitam da mesma forma em vez de descartar.
- **Atualizações granulares.** Os componentes são executados uma única vez e os signals atualizam apenas o que os lê, sem DOM virtual ou função de visualização por quadro para comparar diferenças; a animação roda no relógio de quadros da plataforma.
- **Os menores pacotes e a inicialização mais rápida.** Nos [benchmarks](https://nsgo.dev/benchmarks/), o mesmo aplicativo de contador gera pacotes de cerca de 8 MB no macOS, Windows e Linux, menores que os do MyGo, GPUI e NSH (33 a 45 MB para o NSH); ele também inicia mais rápido e consome menos memória do que frameworks que desenham na GPU, como MyGo e GPUI.
- **Somente Go.** O motor, o layout e os backends são em Go, sem C, sem cgo e sem motor JavaScript para aplicativos em Go.
- **Todas as plataformas a partir de um Mac.** Um Mac com Go e Xcode compila de forma cruzada para todos os alvos: sem NDK, Android SDK, JDK, GTK ou toolchain do Windows.
- **Todos os arquivos de lançamento com um comando.** `nsgo build` gera uma imagem de disco assinada e notarizada para macOS, um instalador assinado para Windows, pacotes deb e Arch e um AppImage, pacotes para a Mac App Store, Microsoft Store, Google Play e Flathub, além de scripts que instalam a partir de lançamentos no GitHub ([compilação](building.md)).
- **As atualizações próprias de cada plataforma.** No macOS o Sparkle, e no Windows o WinSparkle, baixam, verificam e instalam a próxima versão; no Linux e no Android o aplicativo a encontra e abre sua página de lançamento; no iOS a App Store cuida disso. Um aplicativo instalado por uma loja fica sob responsabilidade dessa loja ([atualizações](building.md#updates)).
- **Todas as plataformas.** Celulares, tablets, TVs, relógios, desktops e a web compartilham um único motor e uma única API, além de iOS App Clips, widgets para iPhone e Mac e, experimentalmente, miniprogramas do WeChat, QQ, Alipay e Douyin.
- **Incorporação.** Um aplicativo NSGo pode funcionar como uma view dentro de um aplicativo existente de iOS, Android, desktop em Go ou web, permitindo migrar uma tela por vez ([incorporação](embedding.md)).
- **Um ciclo de desenvolvimento rápido.** Salvar um arquivo Go recompila e reinicia o aplicativo; uma alteração em TSX atualiza o aplicativo em execução no local e preserva seu estado; erros de compilação e panics durante a renderização aparecem sobre o aplicativo ([execução e depuração](debugging.md)).
- **Um único conjunto de testes em qualquer lugar.** Os testes encontram e usam controles por texto ou papel como os usuários fazem, rapidamente sem janela, ou sem alterações em janelas reais em cada alvo usando a própria entrada da plataforma e salvando capturas de tela ([testes](testing.md)).
- **Localização integrada.** Os textos de um aplicativo e de seus componentes ficam em catálogos, e pseudo-locales identificam o que não foi traduzido, o que foi cortado ou o que não está pronto para escrita da direita para a esquerda ([localização](localization.md)).
- **Shaders em Go.** `core.Shader` executa shaders de GPU escritos em sintaxe Go ou WGSL, em todas as plataformas ([canvas](canvas.md#shaders)).
- **TSX quando desejado.** As telas podem ser escritas em TSX com classes Tailwind, sobre o mesmo motor que os componentes em Go.

## Pontos fracos

- **Estágio inicial.** 0.1.0 é a primeira versão, e a API ainda não é estável; o [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md) lista o que falta, como um limite de heap para TSX entregue em tempo de execução.
- **Um ecossistema pequeno.** Poucos componentes de terceiros, exemplos ou respostas online, em comparação com Flutter, React Native ou Qt.
- **As plataformas diferem.** Views nativas implicam pequenas diferenças entre plataformas, então um aplicativo precisa ser verificado em cada uma. Um framework que desenha por conta própria tem a mesma aparência em todo lugar.
- **Pontes próprias.** No Android o programa Go roda ao lado do runtime Java e se comunica com ele por pipes, e no Linux os usuários precisam do GTK 4.14 ou mais recente.
- **TSX é interpretado.** TSX roda no goja, um interpretador JavaScript escrito em Go, mais lento que os motores usados pelo React Native ou Electron.

## NSH

O NSGo porta o design do NSH: a arquitetura, os backends, os nomes da API e os componentes sem estilo de `base` são do NSH ([status](index.md#status)).

- **Escolha o NSGo** para escrever aplicativos em Go em vez de Rust ou TSX, para compilar para todos os alvos a partir de um Mac com Go e Xcode, ou pelo que o NSGo acrescenta: shaders de GPU escritos em Go, ícones vetoriais, um terminal, aplicativos que abrem suas próprias janelas, painéis e ícones na bandeja do sistema, e componentes estilizados em `ui`.
- **Escolha o NSH** para um projeto mais maduro, TSX executado pelo QuickJS com bytecode e limites de heap, o backend GPUI ou Android de 32 bits.

## MyGo

Ambos são interfaces escritas em Go sem cgo, mas o MyGo desenha sua interface nativa por conta própria na GPU e a reconstrói a partir de uma função de visualização a cada quadro, enquanto o NSGo passa às views nativas apenas o que mudou.

- **Escolha o MyGo** para aplicativos desktop com aparência idêntica em todos os desktops, janelas baseadas em páginas web com chamadas tipadas para Go e suas APIs de desktop e empacotamento: ícones na bandeja, associações de arquivos, instaladores, notarização e atualizações delta assinadas.
- **Escolha o NSGo** para alcançar celulares também, usar o texto, a entrada e a acessibilidade próprios de cada plataforma, ou manter o estado em signals em vez de redesenhar a partir dos dados do aplicativo.

## Outros toolkits em Go

- **Fyne e Gio** desenham tudo por conta própria, como faz o MyGo, e ambos alcançam celulares e a web. O Fyne oferece um conjunto completo de widgets com visual próprio; o Gio é em modo imediato, de baixo nível e flexível. Ambos têm versões lançadas e usuários; nenhum dos dois tem aparência ou comportamento nativos.
- **Wails** coloca um frontend web na webview do sistema com Go por trás: qualquer biblioteca web funciona, mas a interface é uma página web, apenas para desktop.

## Frameworks em outras linguagens

- **Flutter e Compose Multiplatform** desenham cada pixel por conta própria: idênticos em todas as plataformas e maduros, com grandes ecossistemas, mas com texto, rolagem e entrada reconstruídos em vez de nativos, além de exigir aprender Dart ou Kotlin.
- **React Native** compartilha o modelo do NSGo de um núcleo reativo controlando views nativas e tem o maior ecossistema do tipo. Ele é centrado em iOS e Android e executa a lógica do aplicativo em JavaScript; o NSGo a executa em Go e trata desktops e a web como plataformas de primeira classe.
- **.NET MAUI** envolve os controles de cada plataforma em C#, sem Linux ou web. **Avalonia** desenha por conta própria, próximo ao WPF, em todos os desktops, celulares e web.
- **Qt** é maduro, rápido e alcança dispositivos embarcados, mas desenha seus próprios widgets, exige C++ ou QML e é licenciado sob LGPL ou comercialmente.
- **Electron e Tauri** exibem páginas web: todas as ferramentas e bibliotecas da web estão disponíveis, ao custo da memória de um navegador, especialmente no Electron, e de uma interface com sensação de página web.

## Escolhendo

Escolha o NSGo para um aplicativo que deva parecer nativo em celulares, desktops e na web a partir de uma única base de código Go, quando um projeto em estágio inicial for aceitável. Para um aplicativo que precisa ser entregue em breve sobre uma API estável, escolha um framework maduro da tabela acima: Flutter ou React Native para celulares, MyGo, Wails ou Tauri apenas para desktops.
