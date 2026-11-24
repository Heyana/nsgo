---
title: Compilação e distribuição
description: Empacote um aplicativo para cada plataforma, crie instaladores, assine e autentique (notarize), publique no GitHub e incorpore em outros aplicativos.
group: Ferramentas
order: 22
---

## Compilação

```sh
go tool nsgo build
```

`build` grava um pacote de lançamento (release) com símbolos removidos para cada alvo listado em `nsgo.yaml`, em `dist/<name>-<version>-<target>`: um `.app`, um `.ipa`, um `.apk`, os diretórios de Windows e Linux ou um site web cuja página busca cada módulo TSX nomeado por um `import()` quando a importação é executada. `-target` substitui a lista, e `all` significa todos os alvos que precisam apenas de Go e Xcode; adicione `android_x64` quando um Android NDK estiver instalado, `miniprogram` — ou apenas `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay` ou `miniprogram_douyin` — para [miniprogramas](#mini-programs), e `harmonyos` para uma prévia de [HarmonyOS](#harmonyos). `-run` inicia o pacote compilado para esta máquina ou o instala no iPhone, simulador ou dispositivo Android escolhido por `-device`.

Tudo o que deve ser distribuído vai parar em `dist/releases`, o único diretório que você precisa consultar:

| Em `dist/releases` | O quê |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Instaladores](#installers) para macOS, Windows e Linux |
| `*.AppImage.zsync` | Controles de atualização delta para AppImageUpdate e inicializadores compatíveis |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Os pacotes de iOS, tvOS e Android, para as lojas ou dispositivos |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` e `flatpak/` | Os [pacotes de loja](#store-packages) de uma compilação `store` |
| `install.sh`, `install.ps1` | Scripts que instalam o aplicativo a partir dos [lançamentos do GitHub](#publish-on-github) |
| `appcast-<target>.xml` | Os feeds de [atualização](#updates) que os aplicativos leem |
| `homebrew/<name>.rb`, `winget/` | [Manifestos de cask do Homebrew e winget](#channels) para distribuição direta |
| `*-web.zip` | O site, para descompactar e servir como está |
| `miniprogram/<platform>/` | O projeto de cada miniprograma, para enviar pela respectiva ferramenta de desenvolvedor |

Cada arquivo é nomeado `<name>-<version>-<system>-<arch>`, como `hello-1.2.0-windows-x64-setup.exe`, para que os usuários encontrem o do seu sistema em uma página de lançamento, e vem acompanhado de seu `.sha256`. Uma compilação de outra versão esvazia o diretório primeiro; compilações da mesma versão, inclusive em outras máquinas, são adicionadas a ele. Os pacotes de simuladores e do watchOS permanecem em seus diretórios `dist/<name>-<version>-<target>`.

O site web inclui `.wasm.br` e `.wasm.gz` quando são menores que o módulo original. Habilite arquivos pré-comprimidos em hospedagens que os suportem. Sirva-os na URL original `.wasm` com `Content-Type: application/wasm`, o `Content-Encoding` correspondente e `Vary: Accept-Encoding`. O módulo original funciona em qualquer hospedagem estática. [Publicando na web](web.md) aborda títulos semânticos, links do navegador, metadados por página e carregamento por CDN.

### Pacotes web offline

Defina `web.offline: true` em `nsgo.yaml` para armazenar em cache um lançamento web na sua primeira inicialização online.

```yaml
web:
  offline: true
```

Sirva o diretório publicado via HTTPS ou use `go tool nsgo build -target web -run` em localhost. O service worker armazena em cache a página, o WebAssembly, os scripts e os recursos empacotados. Visitas online carregam a versão publicada. Após a instalação ser bem-sucedida e as abas da versão anterior serem fechadas, a versão mais recente pode iniciar a partir do cache. Requisições aos serviços online de um aplicativo ainda precisam de conexão.

## Terminal

Execute o aplicativo em um terminal interativo com `go tool nsgo dev -target tui`. `go tool nsgo test -target tui` executa seus testes ali por meio de `nsgotest.Main`, um aplicativo por processo de teste. O terminal controla o tamanho: uma célula representa 8 por 16 pontos de layout. Escolha os estilos do seu aplicativo explicitamente; os componentes mantêm suas formas e tamanhos comuns. `examples/tui` demonstra campos, botões, atalhos e redimensionamento.

Compile um executável de console com `go tool nsgo build -target tui_macos_arm64`, ou o correspondente `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` ou `tui_linux_x64`. O grupo de compilação/verificação `tui` inclui todos os seis, e `all` também os inclui. Cada diretório de pacote contém o executável e os recursos; ele não possui instalador de interface gráfica. Todos os seis compilam apenas com Go.

O backend suporta texto em células, planos de fundo, bordas sólidas, recorte, rolagem, foco e edição de texto. Ele usa a fonte e as cores do terminal; Control é `mod`, inclusive em um Mac. Ctrl+C copia dentro de um editor e encerra quando nenhum vínculo ou editor o consome. A colagem delimitada (bracketed paste) insere texto sem executar atalhos, e a área de transferência é local ao processo. Canvas, imagens, mídia, componentes nativos hospedados, menus/alertas do sistema, animações, zoom, modelagem bidirecional, reticências no início/meio e ponte com a área de transferência do sistema ainda não estão implementados. `base/terminal` continua sendo o controle de terminal incorporado em um aplicativo GUI.

Enquanto `nsgo dev` controla o terminal, os diagnósticos da ferramenta e do aplicativo vão para `.nsgo/tui.log`. Ao sair, a tela, o cursor e os modos do terminal são restaurados antes de retornar ao shell. Capturas de tela de testes de terminal rasterizam as células com uma fonte portátil fixa em vez de capturar o emulador; use asserções de texto para glifos Unicode ausentes nessa fonte.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Um nome simples de sistema operacional em `targets` — ou em `nsgo build -target` e `nsgo check -target` — representa suas arquiteturas (`windows` é `windows_arm64` e `windows_x64`; `android` é apenas `android_arm64`, pois a compilação x86_64 requer um NDK). `macos_universal`, que `macos` e `all` omitem, compila ambas as arquiteturas do macOS e as une em um único aplicativo, incluindo suas bibliotecas e widgets. O canal Steam usa o pacote de aplicativo universal. Compilações de instalador o envolvem em uma única imagem de disco, com um único feed Sparkle no canal direto. `nsgo test -target` também aceita o nome da arquitetura para a qual seus testes rodam: a de um sistema operacional desktop na arquitetura desta máquina e a do Android em `arm64`, portanto `windows_x64` executa o alvo `windows` em uma máquina Windows x64, enquanto uma arquitetura para a qual o alvo não roda é recusada informando o nome a usar.

Flags substituem os valores que nomeiam, como `-target`, `-version` ou `-output`; `-h` lista todas elas.

`build.ldflags` contém as próprias flags de vinculador do aplicativo, cada uma escrita como `-ldflags` de `go build` a aceita, como `-X main.releaseDate=2026-11-17` ou `-X 'main.channel=beta builds'`. Toda compilação que vincula o aplicativo as utiliza: `dev` e suas recargas, `build` inclusive com `-embed` e `test` inclusive com `-target`, em todos os alvos. `-ldflags` em `build` e `test` adiciona a elas em vez de substituí-las, podendo ser repetida para mais flags. Elas vêm antes das próprias flags `-X` da ferramenta, e o vinculador mantém a última definição de uma variável, de modo que o nome, o ID e a versão em `core.App` continuam sendo os de `nsgo.yaml`. A ferramenta passa ao Go seu próprio `-ldflags`, o que substitui o que houver em `GOFLAGS`, portanto flags de vinculador pertencem a este campo; o `-ldflags` após o diretório em um `test` headless é o de `go test` e substitui todos eles.

Todo comando `go` que a ferramenta executa para o aplicativo utiliza `build.tags`, em `dev` e suas recargas, `build` inclusive com `-embed`, `test` inclusive com `-target` e `check`, em todos os alvos. As próprias tags do NSGo, como `nsgodev` em `dev` ou `nsgotest` em testes de alvo, são adicionadas a elas, assim como as tags de `-tags` em `GOFLAGS`, que de outra forma seriam substituídas; o restante de `GOFLAGS` se aplica como está.

`apple.info.LSMinimumSystemVersion` eleva o mínimo do pacote macOS, e `apple.info.MinimumOSVersion` eleva o do pacote iOS, tvOS ou watchOS selecionado. Escreva strings de versão entre aspas, como `"14.2"` ou `"18.0"`, com uma a três partes numéricas. Os pisos do runtime são macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 e watchOS 26.0; valores menores ou malformados falham. A chave móvel deve atender ao piso de cada alvo móvel selecionado. App Clips mantêm seu próprio `app_clip.minimum`. Os valores chegam aos metadados do pacote, compilação de ativos, projetos de assinatura e vinculador do aplicativo; frameworks iOS incorporados também os usam, com Mac Catalyst em pelo menos 16.0 para o runtime macOS 13 do Go.

`icon.windows` e `icon.linux` selecionam PNGs quadrados para seus pacotes de aplicativo, artes de instalador e logotipos de loja. Eles usam por padrão `assets/IconWindows.png` e `assets/IconLinux.png` e, na ausência deles, recorrem a `icon.png`; `dev.icon` aceita as mesmas configurações e arquivos padrão `-dev`. `icon.android.png` seleciona um bitmap Android de forma independente, inclusive quando nenhuma camada adaptativa está configurada. Outros alvos mantêm o fallback compartilhado `icon.png`.

O ícone de uma Apple TV é um cartão em camadas, não um quadrado: `tvos` aceita o `.brandassets` do Xcode, suas pilhas de `App Icon` e imagens de top shelf, ou uma imagem larga como 1280×768, que a ferramenta desenha ajustada ao cartão sobre sua cor de borda e centraliza nas imagens de top shelf. Sem nenhum dos dois, o PNG ocupa três quartos da altura do cartão. Um Apple Watch mascara seu ícone em um círculo, portanto `watchos` aceita um `.icon` desenhado para os círculos do watchOS ou um PNG quadrado; sem ele, o aplicativo usa `apple` e depois `png`. Ambos recorrem a `assets/AppIconTV.*` e `assets/AppIconWatch.*`, assim como `apple` e `png`.

No Android, `icon.android.foreground` e `background` tornam o ícone do inicializador adaptativo, para que os inicializadores o mascarem no formato deles e o movam como quiserem em vez de encolher `png` em uma placa própria. Cada camada tem 108 dp quadrados, um PNG escalado para 432 px; o inicializador exibe os 72 dp centrais, 288 px, e os mascara em uma forma que pode cortar tudo fora dos 66 dp centrais, 264 px, portanto mantenha o logotipo dentro desse círculo e deixe o fundo preencher todo o quadrado. `background` é um PNG ou uma cor como `"#1F1F1F"`. `monochrome`, uma forma cujo alfa é tudo o que importa, é o que o Android 13 e posteriores tingem quando o usuário ativa ícones temáticos; sem ela, o ícone mantém suas cores ali. O pacote mantém `icon.android.png`, ou então `icon.png`, como o ícone bitmap simples, que nenhum inicializador do Android 9 ou posterior — o mais antigo em que o NSGo roda — escolhe acima do adaptativo; sem `png`, a ferramenta o desenha a partir do primeiro plano sobre o fundo. Sem camadas adaptativas, o inicializador exibe o bitmap do Android ou o `png` compartilhado.

`android.uses_cleartext_traffic: true` habilita um APK ou App Bundle para HTTP simples para qualquer host, definindo tanto a flag do manifesto quanto sua política de segurança de rede. Sem essa habilitação, `android.loopback_cleartext: true` permite HTTP simples apenas de `localhost`, `127.0.0.1` e `::1`, como vídeos servidos pelo próprio aplicativo. A política somente de loopback nega tráfego em texto claro para outros hosts; a habilitação para todos os hosts tem precedência quando ambas as configurações são verdadeiras. Para um AAR incorporado, configure a política de segurança de rede do Android do aplicativo host.

Toda compilação também aceita as próprias flags do NSGo ao iniciar, como `--nsgo-fps` ([Execução e depuração](debugging.md#flags-of-every-build)). Em `nsgo dev`, Command/Control+R recarrega a partir de qualquer janela aberta e permanece disponível após o fechamento da visualização principal quando outra janela mantém uma função do aplicativo em execução.

### Tela de inicialização

Defina `launch_screen` para exibir um fundo opaco, um PNG centralizado ou ambos no iOS e no Android 12 ou posterior. Os caminhos de imagem são relativos a `nsgo.yaml`. A imagem se ajusta proporcionalmente dentro de 128 × 128 pontos ou dp. Um PNG transparente com o maior lado tendo pelo menos 512 pixels a mantém nítida em telas de alta densidade.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

O sistema seleciona a aparência, a menos que `appearance` a fixe. Omita qualquer um dos campos de `dark` para reutilizar o valor correspondente. Um fundo ou imagem omitido mantém o padrão da plataforma. Aplicativos incorporados obtêm sua tela de inicialização do host.

## Classes nativas do Android

Liste classes Android pré-compiladas no `nsgo.yaml` do aplicativo para usar uma View personalizada a partir de `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Os caminhos são relativos a `nsgo.yaml`, a menos que sejam absolutos. Cada caminho nomeia um arquivo `.dex` ou um JAR contendo os `classes.dex`, `classes2.dex` e shards posteriores na raiz do D8. Prepare esses arquivos com D8 usando `--min-api 28`. Em arm64, o NSGo compila o APK ou App Bundle a partir desses arquivos pré-compilados apenas com Go. Sessões de desenvolvimento Android recompilam quando um arquivo listado muda. Para um AAR incorporado, o build Android do host fornece suas dependências Java. [Documentação do D8](https://developer.android.com/tools/d8)

O [exemplo nativo do Android](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) registra uma classe com um construtor público de `Context` e a configura com uma prop Go.

## Jogos

Defina `game: true` em `nsgo.yaml` para um jogo. O Android classifica o aplicativo como jogo e usa as otimizações do Modo de Jogo do sistema. macOS e iOS habilitam o Modo de Jogo, o macOS usa por padrão a categoria Games e o Linux adiciona `Game` à entrada de desktop. Um `macos.category` explícito ou chave de Modo de Jogo em `apple.info` tem precedência.

Defina `gamepad: true` quando seu aplicativo lidar com um gamepad de hardware. O Android declara tanto gamepad quanto touchscreen como opcionais. macOS, iOS e tvOS declaram o perfil Extended Gamepad, e o tvOS também declara o Micro Gamepad do Siri Remote, com chaves de controle em `apple.info` tendo precedência.

## Avisos de licença

`nsgo build` coloca `THIRD_PARTY_NOTICES.txt` nos recursos do pacote independente. Ele inclui os textos de licença dos módulos Go usados por aquele alvo, do runtime Go e do runtime Java ou JavaScript empacotado pelo NSGo. Compilações web nomeiam o arquivo pelo seu conteúdo, como ocorre com outros recursos. Use `core.ThirdPartyNotices()` para exibir o mesmo texto em uma tela de créditos dentro do aplicativo. A função retorna uma string vazia em desenvolvimento, testes e compilações de bibliotecas incorporadas. [Declarações de bibliotecas nativas](native-libraries.md) adicionam seus textos de licença originais aos mesmos avisos.

Por padrão, uma licença de terceiros desconhecida ou ausente falha a compilação e uma licença copyleft detectada emite um aviso. Módulos listados no workspace Go do aplicativo (`go.work`) são tratados como código próprio junto ao módulo do aplicativo: quaisquer arquivos de licença que contenham são incluídos e não disparam verificações de terceiros. Defina qualquer uma das regras como `error` ou `warn`. A lista `allow` isenta caminhos de módulo exatos de ambas as regras e mantém qualquer texto de licença que a compilação encontrou.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Bibliotecas de pacotes Go

Aplicativos que importam `base/terminal` também distribuem `libghostty-vt`: pacotes de Windows e Linux colocam a biblioteca ao lado do executável; pacotes de macOS e iOS a assinam dentro de seus frameworks. [Incorporação](embedding.md) explica o XCFramework complementar para hosts iOS.

Uma dependência pode declarar bibliotecas compartilhadas de desktop pré-compiladas em `nsgo-natives.yaml`. O NSGo verifica seu SHA-256, as copia para o aplicativo antes da assinatura e as inclui nos instaladores; o autor do aplicativo não executa nenhuma etapa extra. [Bibliotecas nativas](native-libraries.md) descreve o formato do autor do pacote e os caminhos de carregamento.

## Comandos

Um aplicativo pode distribuir programas de linha de comando ao lado de seu executável de desktop, como uma CLI que opera sobre os dados do aplicativo. Liste-os em `commands`, cada um compilado a partir de um pacote `main` Go:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Um nome contém letras, dígitos, `-` e `_`, e não pode ser igual ao executável do aplicativo, ao seu comando Linux, `Uninstall`, `AppRun` ou ao de outro comando em qualquer caixa (maiúsculas/minúsculas). `macos_universal` une as duas compilações macOS de cada comando, assim como une as do aplicativo.

Um pacote vincula com `build.ldflags` e `-ldflags`, depois seus próprios `ldflags`, cujos valores `-X` vencem, e por fim a identidade do aplicativo, de modo que `core.App()` fornece ao comando o nome, ID, versão e número de compilação de `nsgo.yaml`, assim como fornece ao aplicativo. Um comando que não importa `core` recebe a versão com `-X`, como `-ldflags "-X main.version=1.2.0"` em `build`, que tanto o aplicativo quanto seus comandos recebem.

| Pacote | Comandos |
| --- | --- |
| macOS | Em `Contents/MacOS`, assinados antes do aplicativo como estão, como `<id>.<command>`: ad hoc ou com `sign.identity`, hardened runtime e carimbo de data/hora, para que o aplicativo seja [autenticado (notarize)](#notarize), mas sem os entitlements do aplicativo. A imagem de disco não coloca nada no `PATH`; o [cask do Homebrew](#channels) vincula cada comando, e o usuário pode vincular `/Applications/<name>.app/Contents/MacOS/<command>` por conta própria |
| Windows | `<command>.exe` ao lado de `<name>.exe`, [assinado](#sign-windows-packages) como está, que o instalador instala e seu desinstalador remove; com `windows.path` o instalador também coloca a pasta no `Path` do usuário, como faz `windows.command_line`, para que consoles abertos após a instalação encontrem os comandos. Um comando que uma atualização renomeia ou remove permanece na pasta, assim como um arquivo que o aplicativo não distribui mais, pois o desinstalador remove apenas o que sua versão instalou |
| Linux | Em `/opt/<slug>` com um link `/usr/bin/<command>` de propriedade do `.deb` e do pacote Arch, de modo que remover o pacote o remove, e dpkg e pacman se recusam a instalar sobre o de outro pacote; o AppImage os mantém ao lado do aplicativo e não coloca nada no `PATH` |

Os instaladores, seus checksums e os appcasts de [atualização](#updates) carregam os comandos assim como carregam os outros arquivos do aplicativo, de modo que uma atualização os substitui junto com o aplicativo. [Pacotes de loja](#store-packages) os omitem, pois o sandbox da Mac App Store, o MSIX e o Flatpak executam apenas o próprio executável do aplicativo.

## Instaladores

Para os alvos de desktop, `build` grava em `dist/releases` aquilo a partir do qual os usuários instalam o aplicativo, cada um com um checksum `.sha256`:

| Alvo | Instalador |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg`, uma imagem de disco exibindo o aplicativo, uma seta e um link para Applications para arrastá-lo, assinada com `sign.identity` e [autenticada (notarize)](#notarize) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe`, um instalador [NSIS](https://nsis.sourceforge.io) que instala para o usuário — novamente na pasta que o usuário escolheu antes, fechando o aplicativo se estiver em execução — com um atalho no menu Iniciar, um atalho opcional na área de trabalho e um desinstalador que remove apenas o que instalou, [assinado com `sign.certificate`](#sign-windows-packages) onde o aplicativo assina pacotes Windows; `windows.command_line` também instala o companheiro `.com` e coloca a pasta do aplicativo no `Path` do usuário, para que consoles abertos após a instalação executem o comando do aplicativo, enquanto Win+R, que também encontra o companheiro, inicia o aplicativo e fecha o console que abriu; o desinstalador retira a pasta do `Path` somente se um instalador a adicionou, nunca uma entrada que você já tinha |
| Linux | `<name>-<version>-linux-<arch>.deb` para Debian e Ubuntu, e `<name>-<version>-linux-<arch>.pkg.tar.gz` para Arch Linux, que instalam o aplicativo em `/opt/<slug>` com um comando, links para seus [comandos](#commands), sua entrada de desktop e ícone (o pacote Arch inclui `.MTREE` para `pacman -Qkk`), e `<name>-<version>-linux-<arch>.AppImage`, que roda em qualquer distribuição sem instalação |

O instalador Windows precisa do compilador do NSIS, 3.06 ou posterior; em um Mac `brew install makensis`, de modo que um Mac compila todos os instaladores; sem ele, `build` deixa o script `<name>.nsi` no pacote, que compila em qualquer lugar como está. O instalador fala os idiomas dos catálogos do aplicativo que o NSIS traduz, incluindo chinês simplificado e tradicional, escolhendo o do sistema. `-installers=false` ou `build.installers: false` compila apenas os pacotes.

Os instaladores aceitam imagens e ícones próprios do aplicativo; caso contrário, cada um usa o ícone do aplicativo ou a imagem embutida:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

Um PNG de fundo com um gêmeo com o dobro do tamanho, como `DMGBackground@2x.png`, permanece nítido em telas Retina. O ícone do aplicativo fica a um quarto da esquerda, Applications a um quarto da direita, cada um com seu nome centralizado de cima a baixo, onde a imagem embutida desenha sua seta; deixe espaço para eles em um fundo próprio. Os pacotes Linux não têm imagens próprias; eles instalam o ícone do aplicativo.

Os pacotes Linux também instalam os metadados AppStream do aplicativo, `/usr/share/metainfo/<id>.metainfo.xml`, pelos quais as centrais de software o listam, escritos a partir de `linux.summary`, `linux.license`, se definido, e os nomes do aplicativo, com `Utility` como categoria quando `linux.categories` não nomeia nenhuma; o AppImage os mantém em `usr/share` do seu AppDir, ao lado de um link para sua entrada de desktop em `usr/share/applications`, que os metadados nomeiam. Arquivos que o sistema lê de `/usr/share`, como metadados mais ricos, mais entradas de desktop, autocompletar de shell ou páginas de manual, vão no diretório que `linux.share` nomeia, relativo a `nsgo.yaml`: o `.deb` e o pacote Arch instalam sua árvore sob `/usr/share`, e o AppImage sob `usr/share` do seu AppDir. Seu `metainfo/<id>.metainfo.xml` substitui o que o NSGo escreve, e um arquivo em um caminho que o próprio pacote escreve, como a entrada de desktop ou o ícone do aplicativo, falha a compilação. Arquivos em `resources/`, qualquer que seja o nome, permanecem ao lado do executável em `/opt/<slug>`, e os Flatpaks de compilações de loja usam apenas os metadados do NSGo.

O AppImage precisa de FUSE, que a maioria das distribuições possui, e obtém o GTK 4 do sistema, como fazem os pacotes; seu runtime é baixado uma única vez para o cache do usuário.

AppImages incorporam um localizador `gh-releases-zsync` usando `release.github` ou o remoto `origin` do GitHub, selecionando o lançamento estável mais recente e a arquitetura deste aplicativo. Envie cada `.AppImage` com seu arquivo `.AppImage.zsync` gerado. Para outro host, defina `linux.appimage_update` como `zsync|https://example.com/latest.AppImage.zsync`, uma URL estável servida com suporte a intervalos HTTP (HTTP range); ele também aceita `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` para um canal de pré-lançamento. Uma string vazia explícita desabilita a descoberta incorporada; sem um repositório GitHub ou um localizador configurado, ela também fica vazia. O arquivo `.zsync` ainda é gerado, separadamente do appcast assinado.

## Pacotes de loja

`-channel store`, ou `updates.channel: store`, compila os alvos de desktop e Android para suas lojas, que os atualizam, no lugar dos instaladores e scripts de instalação:

| Alvo | Pacote de loja |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` para a Mac App Store: o aplicativo no App Sandbox, assinado com a identidade do perfil de provisionamento e empacotado com a identidade de instalador da sua equipe |
| Windows | `<name>-<version>-windows-<arch>.msix` para a Microsoft Store, que o assina na publicação |
| Linux | `<name>-<version>-linux-<arch>.flatpak` instalável, além de um `.tar.gz` com AppStream e `flatpak/<id>.yml` para Flathub ou flatpak-builder no runtime do GNOME |
| Android | `<name>-<version>-android-<arch>.aab`, um App Bundle para o Google Play assinado com `sign.keystore`, a chave de upload |

O MSIX lê o nome de exibição de cada idioma a partir de `appName` nos catálogos, recorrendo ao idioma de origem. Seus logotipos de Iniciar, barra de tarefas e Loja incluem variantes de 100%, 125%, 150%, 200% e 400%. A ferramenta grava seu índice de recursos no macOS; nenhum SDK do Windows é necessário.

`-target macos` seleciona ambas as arquiteturas do macOS no canal `store` e grava um único `<name>-<version>-macos-universal.pkg`, assim como `-target macos_universal`. Selecione `-target android_arm64,android_x64` para um único `<name>-<version>-android-universal.aab` contendo `arm64-v8a` e `x86_64`. A fatia `x86_64` requer um Android NDK.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

O runtime tem como alvo a API 36 quando `android.target_sdk` não está definido. Uma compilação de Android App Bundle falha abaixo da API 36, o que o [Google Play exige](https://support.google.com/googleplay/android-developer/answer/11926878) para novos aplicativos móveis e atualizações a partir de 31 de agosto de 2026. Uma compilação de APK independente emite um aviso e continua.

Defina `sign.macos_profile` para assinar o pacote Mac para upload. Sem widgets, o pacote não assinado ainda roda no sandbox, o que permite testar o acesso ao contêiner do aplicativo, à rede e aos arquivos que o usuário escolhe. Uma compilação de loja com widgets requer tanto `sign.macos_profile` para `<id>` quanto um `sign.widgets_profile` do macOS para `<id>.widgets`, da mesma equipe e concedendo `apple.app_group` (padrão `group.<id>`). Ambos os pacotes usam esse contêiner compartilhado; nenhuma exceção de diretório inicial é adicionada. `-installers=false` mantém as assinaturas de sandbox e perfil enquanto omite o `.pkg` e sua identidade de instalador. Envie o `.msix` no Partner Center e o `.aab` no Play Console. Submeta `flatpak/<id>.yml` ao Flathub assim que o lançamento contiver os arquivos compactados que ele nomeia. `flatpak-builder` também o compila localmente. Instale um pacote com `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; o runtime correspondente do GNOME precisa estar instalado ou disponível em um remoto configurado. As compilações gravam o pacote em Go sem uma toolchain Linux. Pacotes de loja usam um ícone transparente quando a arte está ausente e `Utility` quando `linux.categories` não está definido. Para instalação fora da Store, defina [`sign.certificate`](#sign-windows-packages) para assinar o MSIX; o assunto do certificado deve corresponder a `windows.store.publisher`, e a máquina receptora deve confiar no certificado. Sem configurações de assinatura, a Store assina o pacote enviado.

No macOS e iOS, os efeitos de borda de uma visualização de rolagem (`Scroll.EdgeEffects`), desfoques de fundo no macOS sem `IOS.BackdropMaterial` e filtros de cor de fundo (sozinhos ou sobre um desfoque sem `IOS.BackdropMaterial`) desenham por meio das APIs privadas `CABackdropLayer` e `CAFilter` do Core Animation, assim como fazem os próprios materiais e efeitos de borda do AppKit e UIKit, já que nenhuma API pública desfoca por um raio, varia um desfoque ao longo de uma visualização ou aplica uma matriz de cores de fundo. A revisão da App Store pode rejeitar aplicativos que usam APIs privadas: o NSGo só as chama para visualizações de rolagem com barras de borda e esses filtros de fundo, mas toda compilação de macOS e iOS contém seus nomes.

Todo pacote Apple que `build` grava — o aplicativo, sua extensão de widget, App Clip e aplicativo Apple Watch, e um framework `-dynamic` `-embed` — carrega um manifesto de privacidade, `PrivacyInfo.xcprivacy`, declarando as APIs de motivo obrigatório que o NSGo e o runtime do Go usam: padrões do usuário (`CA92.1`), carimbos de data/hora de arquivo (`C617.1`) e tempo de inicialização do sistema (`35F9.1`). `apple.privacy` adiciona os do próprio aplicativo: `accessed_apis`, os motivos de cada categoria, `tracking`, `tracking_domains` e `collected_data`, entradas de `NSPrivacyCollectedDataTypes` como a Apple as nomeia; um `PrivacyInfo.xcprivacy` em `resources/` é mesclado a eles em vez de substituído.

O Info.plist de cada pacote de iOS, tvOS e watchOS que `build` grava para dispositivos — o do aplicativo, de sua extensão de widget e do App Clip — carrega as chaves de SDK e `DT` do Xcode que o App Store Connect verifica, lidas do Xcode que o compila, e `UIRequiredDeviceCapabilities: [arm64]`; pacotes de simulador as omitem, e uma chave definida por `apple.info` mantém seu valor. `apple.multiple_scenes: true` define `UIApplicationSupportsMultipleScenes` no manifesto de cenas de um aplicativo iOS, que é falso por padrão: em um iPad, o usuário pode então abrir mais de uma janela do aplicativo, e cada uma exibe a visualização principal do aplicativo novamente, com estado próprio e os sinais e armazenamento compartilhados do aplicativo, até que o usuário a feche.

### Publicar no Google Play

Habilite a Google Play Developer API e dê a uma conta de serviço permissão para lançar o aplicativo na faixa escolhida. Defina `NSGO_PLAY_SERVICE_ACCOUNT` com a chave JSON da conta ou o caminho para seu arquivo de chave. [Configuração da API do Google Play](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

O comando encontra o App Bundle para a versão do projeto em `build.output/releases`. Passe um caminho `.aab` após as flags para selecionar um arquivo quando houver várias compilações. Use `internal`, `production` ou o ID de faixa de um teste fechado. O comando lê o pacote e o código de versão do bundle, exige um código maior do que qualquer lançamento naquela faixa, depois faz o upload do bundle e confirma o lançamento.

## Listagens nas lojas

Metadados das lojas (títulos, subtítulos, descrições, palavras-chave, notas de lançamento, URLs, capturas de tela e prévias) ficam no repositório como YAML e Markdown em texto puro sob `store/` (ou `store.dir` em `nsgo.yaml`):

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Ambos os arquivos `listing.yaml` exigem `version: 1` e recusam chaves desconhecidas. Diretórios de localidade canônicos como `zh-Hans` são mapeados automaticamente para o código de cada loja (`zh-Hans` no App Store Connect, `zh-CN` no Google Play), a menos que sejam sobrescritos por loja com `app_store.locale` ou `google_play.locale`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

A validação é executada antes de ler credenciais ou fazer qualquer requisição HTTP, verificando campos obrigatórios, códigos de localidade, esquemas de URL, arquivos de mídia e quantidades (até 10 capturas de tela e 3 prévias por tipo de tela da App Store; até 8 capturas de tela por tipo de imagem do Google Play), e limites de caracteres (App Store: título 30, subtítulo 30, palavras-chave 100, texto promocional 170, descrição 4000, notas de lançamento 4000; Google Play: título 30, descrição curta 80, descrição 4000, notas de lançamento 500).

As credenciais de upload vêm apenas do ambiente: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` e `NSGO_APP_STORE_ISSUER` (recorrendo à chave de autenticação `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` e `NSGO_NOTARY_ISSUER`) para o App Store Connect, e `NSGO_PLAY_SERVICE_ACCOUNT` (caminho do arquivo JSON da conta de serviço ou texto JSON) para o Google Play.

### Conformidade nas lojas

Use `nsgo.yaml` para acompanhar o status de cada questionário de lançamento.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

Defina uma entrada como `true` após concluir seu questionário. Uma compilação `-channel store` ou `-channel steam` correspondente lembra sobre entradas definidas como `false`. Omita qualquer entrada que você não acompanhe no YAML.

- Preencha o [questionário de classificação etária](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) e os [detalhes de Privacidade do App](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) no App Store Connect.
- Para o [questionário de criptografia](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) da Apple, defina `apple.info.ITSAppUsesNonExemptEncryption` com a resposta do seu aplicativo como um booleano YAML. Se a Apple enviar um código de aprovação, coloque-o em `apple.info.ITSEncryptionExportComplianceCode` como uma string. Você pode omitir as chaves e responder no momento do upload. Compilações de loja da Apple verificam os tipos dessas chaves e a estrutura do manifesto de privacidade do pacote.
- Preencha a seção [Segurança dos dados](https://support.google.com/googleplay/android-developer/answer/10787469) no Play Console. Aplicativos distribuídos exclusivamente na faixa de teste interno são isentos.
- Preencha a [Pesquisa de Conteúdo](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) no Steamworks antes de enviar o jogo para revisão.
## App Store Connect

Crie um aplicativo no App Store Connect e assine um IPA de dispositivo iOS com um perfil de distribuição da App Store ou um PKG de loja macOS com a identidade de instalador da sua equipe. Em um Mac com Xcode, defina `NSGO_APP_STORE_KEY` com o arquivo `.p8` da chave da sua equipe no App Store Connect ou com seu texto, `NSGO_APP_STORE_KEY_ID` com o ID da chave e `NSGO_APP_STORE_ISSUER` com o ID do emissor; sem eles, é usada a chave de autenticação (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`), como ocorre para listagens nas lojas. Dê a essa chave acesso ao aplicativo e permissão para enviar compilações.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

A ferramenta verifica o número de compilação do pacote antes de enviar e, em seguida, informa o resultado do processamento da Apple para o TestFlight. Aumente `build_number` para o próximo envio. Números de compilação do macOS crescem em todas as versões do aplicativo, enquanto o iOS pode reiniciar a sequência para uma nova versão. Envie uma compilação processada para a Revisão de Aplicativos separadamente no App Store Connect. [Guia de distribuição da Apple](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Pacotes de ativos de loja

Declare diretórios de ativos grandes em `mobile.asset_packs`. Os arquivos dentro desses diretórios são entregues separadamente do Android App Bundle base e do aplicativo iOS. Os nomes dos pacotes começam com uma letra minúscula e podem conter letras minúsculas, dígitos e sublinhados.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Compile pacotes Android com `go tool nsgo build -channel store -target android_arm64`. Use `bundletool build-apks --local-testing` e `bundletool install-apks` para testar o bundle em um dispositivo. O NSGo abre pacotes `install-time` instalados. Os módulos `fast-follow` são incluídos para entrega pelo Play, e a leitura desses pacotes a partir do Play Asset Delivery SDK ainda está pendente.

No iOS, `go tool nsgo build -target ios` mapeia `install-time` para a política essencial da Apple e `fast-follow` para pré-busca (prefetch), tanto na primeira instalação quanto em atualizações. O Xcode 26 ou posterior compila o downloader de Background Assets. Use um `apple.app_group` compartilhado ou o padrão `group.<id>`. Tanto o `sign.profile` do aplicativo quanto o `sign.asset_packs_profile` da extensão `<id>.assetdownloader` precisam desse grupo. Envie os arquivos `.aar` de `dist/releases` junto com o IPA usando o fluxo de upload de pacotes de ativos do App Store Connect. [Criando pacotes de ativos gerenciados](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

A partir de uma goroutine, chame `mobile.AssetPackReady(ctx, "levels")` para verificar a prontidão e `mobile.OpenAssetPackFile("levels", "intro.map")` para abrir um arquivo relativo ao diretório declarado. Feche o leitor retornado após o uso. [Testando pacotes de ativos da Apple localmente](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Depósitos do Steam

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Defina `updates.channel: steam` para usar o canal por padrão. Selecione os alvos macOS, Windows e Linux. Envie cada diretório `dist/<name>-<version>-<target>` para o respectivo depósito do Steam, usando o pacote do aplicativo no macOS, o executável no Windows e Linux arm64 e `steam-launch.sh` no Linux x64 como opção de inicialização. O Steam cuida das atualizações. Os diretórios não contêm instaladores, atualizador próprio nem `steam_appid.txt`.

Depósitos Linux x64 incluem o GTK 4 e suas dependências para o Steam Linux Runtime 3.0 e 4.0. O inicializador os utiliza apenas quando o sistema não possui GTK 4.14 ou mais recente. Mantenha `nsgo-gtk/` ao lado do inicializador: seu diretório `licenses/` contém as licenças das bibliotecas, os códigos-fonte a partir dos quais foram compiladas e o script de compilação, o que a LGPL exige para GLib, Pango e GTK. O Linux arm64 precisa do GTK 4 disponível em seu runtime.

Para testes locais fora do cliente Steam, defina `steam: {app_id: 480}` em `nsgo.yaml`, substituindo 480 pelo seu Steamworks App ID. `go tool nsgo dev` grava o ID em `steam_appid.txt` em seu diretório de compilação nativo e inicia o aplicativo com esse diretório como diretório de trabalho. [API do Steamworks](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Publicar no GitHub

Quando a compilação possui um alvo macOS, Windows ou Linux, `dist/releases` também contém `install.sh` e `install.ps1`, que instalam o aplicativo a partir dos lançamentos do repositório GitHub nomeado por `release.github`, ou então daquele em que está o remoto `origin` do Git do aplicativo; sem nenhum dos dois, `build` não grava nenhum e avisa. Envie seus arquivos — o nível superior sem `miniprogram/` — para o lançamento com a tag `v<version>` (`release.tag_prefix` substitui `v`; os scripts codificam caracteres de URL reservados nesse prefixo):

```sh
gh release create v1.2.0 dist/releases/*.*
```

Os usuários então instalam ou atualizam o aplicativo com uma linha, que a compilação imprime:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

No macOS, o script copia o aplicativo da imagem de disco para `/Applications`, ou `~/Applications` quando não pode escrever lá; no Linux, instala o pacote Arch com `pacman`, ou o pacote Debian com `apt-get` ou `dpkg`, via `sudo`; no Windows, executa o instalador silenciosamente, o qual instala onde o aplicativo já está. Cada um verifica o arquivo contra seu checksum primeiro. `<NAME>_VERSION`, o nome do aplicativo em maiúsculas, como `HELLO_VERSION=1.1.0`, instala outro lançamento, e `<NAME>_DOWNLOAD_URL` baixa de um espelho.

## Publicar no itch.io

Defina `itch.project` com o slug `owner/game` do seu jogo no itch.io em `nsgo.yaml`. Para sobrescrever nomes de canais, defina `itch.channels` por alvo de compilação. Os padrões são `html5` para web, `osx-<arch>` para macOS, `windows-<arch>` para Windows e `linux-<arch>` para Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Com `BUTLER_API_KEY` definida como segredo no CI, compile e envie os alvos.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Use os mesmos `-output` e `-version` em ambos os comandos se você os sobrescrever. O upload usa o diretório de pacote de cada alvo e define a versão no itch.io para a versão da compilação. O NSGo usa `butler` no `PATH` ou baixa uma cópia fixada do itch.io. Após o primeiro upload web, defina o tipo do jogo no itch.io como HTML e marque o upload como jogável no navegador. [Documentação do Butler](https://itch.io/docs/butler/pushing.html)

## Atualizações

`updates` faz o aplicativo se manter atualizado da maneira que sua plataforma e o canal pelo qual os usuários o obtêm funcionam:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` imprime um par de chaves e não grava nenhum arquivo: coloque a chave pública em `updates.key` e faça commit dela, e mantenha a chave secreta fora do repositório, em um gerenciador de senhas e em `NSGO_UPDATES_KEY` onde os lançamentos são compilados, como um segredo do GitHub Actions. Cada compilação direta assina os instaladores com a chave secreta e grava `appcast-<target>.xml` ao lado deles, um feed no formato do [Sparkle](https://sparkle-project.org) que o aplicativo lê em `feed`: envie os appcasts junto com os instaladores. Sem a chave secreta, `build` não grava appcast de macOS nem de Windows, pois estes instalam apenas instaladores assinados, e aplicativos Linux e Android que leem um appcast não assinado abrem a página do lançamento em vez de instalar seu pacote.

`-release-notes notes.md` coloca um arquivo Markdown com as notas do lançamento, como sua seção do changelog, dentro de cada appcast, renderizado como HTML sem o HTML bruto do próprio arquivo. Sparkle e WinSparkle o exibem em seus diálogos de atualização, e o aviso no Linux e no Android o exibe como texto abaixo da versão, com até 1.500 caracteres; os aplicativos o leem como `core.UpdateCheck.Notes`.

| Alvo | Como uma compilação direta atualiza |
| --- | --- |
| macOS | O Sparkle, que `build` coloca no aplicativo, verifica diariamente, oferece a nova versão, baixa a imagem de disco, verifica sua assinatura, a instala e reinicia o aplicativo; o menu padrão do aplicativo tem Verificar Atualizações…, que um aplicativo que define seu próprio menu adiciona com `updater.Check` |
| Windows | O WinSparkle, ao lado do executável, faz o mesmo no idioma que o aplicativo exibe ao iniciar, quando o WinSparkle o possui, e executa o instalador silenciosamente, o qual inicia a nova versão |
| Linux, Android | O aplicativo lê seu appcast e, assim que o usuário escolhe Atualizar, faz streaming e verifica o pacote assinado diretamente no disco com progresso informado (`updater.Status` e o indicador de progresso de tarefa da plataforma), depois o instala via `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), substituição atômica de AppImage (reiniciando no Linux) ou o instalador de pacotes do Android; instalações fora desses formatos ou um appcast não assinado oferecem a página do lançamento |
| iOS | O aplicativo consulta a App Store, que o atualiza, e oferece sua página lá |

Para um aplicativo web independente publicado, `updater.Check(cx)` compara o lançamento em execução com um `nsgo.json` novo e oferece recarregar após confirmação. Ele também detecta arquivos de compilação alterados sob o mesmo rótulo de versão. Não precisa de chave de appcast; canais explícitos `none`/`steam` ainda desabilitam verificações, e hosts incorporados gerenciam suas próprias recargas. Miniprogramas usam o gerenciador de atualizações de sua plataforma e reiniciam apenas quando a atualização baixada está pronta. Esses alvos verificam quando o aplicativo chama o atualizador; o momento de verificação/download em miniprogramas pertence ao host. Eles não expõem setters de preferência de verificação automática. Um manipulador `cx.CheckForUpdates` personalizado usa `UpdateCheck.Restart` para uma atualização preparada cujo rótulo de versão pode estar ausente.

### Canais

O canal decide quem atualiza o aplicativo. `-channel` substitui `updates.channel` para uma compilação, de modo que um único projeto compila para cada um:

| Canal | Para | Os pacotes |
| --- | --- | --- |
| `direct` | GitHub, um site, casks do Homebrew e winget, que instalam os próprios arquivos do lançamento | Atualizam a si mesmos como acima |
| `store` | App Store, Mac App Store, Microsoft Store, Google Play, Flathub e outras lojas | São os [pacotes de loja](#store-packages) e não carregam autoatualizador; iOS e macOS consultam sua App Store sem chave, e uma instalação do Google Play pode usar o fluxo nativo de atualização do Play |
| `steam` | Steam | Produzem [diretórios de depósito](#steam-depots) que o Steam atualiza |
| `none` | Distribuições Linux e qualquer pessoa que reempacote o aplicativo | Não verificam nada |

As lojas rejeitam aplicativos que se autoatualizam, portanto envie uma compilação `store`, nunca uma direta. Cada lançamento direto com instaladores de macOS ou Windows também grava um cask do Homebrew (`dist/releases/homebrew/<name>.rb`, declarando `auto_updates true`, checksums e URLs de download para as imagens de disco compiladas e um link `binary` para cada um dos [comandos](#commands) do aplicativo) e um manifesto multiarquivo do winget (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` e `<id>.locale.en-US.yaml` para os instaladores NSIS compilados). Onde quer que o sistema informe quem instalou o aplicativo, o aplicativo também verifica isso: um aplicativo Android de outra loja, um Flatpak ou Snap e uma compilação do TestFlight deixam as atualizações para essa loja. Um `updater.Check` manual em uma instalação do Google Play solicita ao Play In-App Updates que ofereça seu fluxo imediato quando permitido; o Play cuida do consentimento, download, instalação e reinicialização. Compilações de desenvolvimento nunca entram nesse fluxo. Pacotes Android independentes empacotam o SDK; [hosts de incorporação](embedding.md#android) o fornecem via Gradle.

### Verificando pelo aplicativo

`updater.Check(cx)` verifica como um item de menu *Verificar Atualizações…* faz e exibe o que encontra; `updater.CheckInBackground(cx)` exibe apenas uma versão mais nova, e `updater.CheckDaily(cx)` faz isso no máximo uma vez por dia. `updater.Status(cx)` lê o progresso ao vivo de download e instalação no Linux e Android. Enquanto `automatic` estiver ligado, o aplicativo verifica dessa forma no Linux, em compilações Android diretas e em compilações da Apple App Store ao iniciar e sempre que o usuário volta a ele; uma verificação que falha, como uma feita offline, é tentada novamente uma hora depois. Compilações de desenvolvimento nunca atualizam. Em compilações Steam, essas três funções não fazem nada.

`updater.CapabilityOf(cx)` informa como esta cópia atualiza sem fazer uma requisição de rede: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (com seu nome `Store`) ou `ManagedUnavailable`. Uma compilação de desenvolvimento, canal desabilitado ou instalação de loja produz uma explicação específica do motivo quando verificada; `core.UpdateCheck.Err` é reservado para falhas como um feed inválido ou um atualizador empacotado que não pôde ser carregado.

Leia `updater.Status(cx)` dentro de um vínculo para acompanhar `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` e `LastCheck`, e no Linux e Android a `Phase`, `Loaded` e `Total` de uma atualização (`Fraction()` para uma barra de progresso). `updater.SetAutomaticChecks(cx, on)` persiste a escolha do usuário e altera o agendador diário em tempo de execução. Sparkle e WinSparkle mantêm suas próprias preferências; Linux, Android e iOS guardam a escolha em uma partição de armazenamento privada. `updater.SetAutomaticDownloads(cx, on)` é suportado apenas no macOS, usando `updates.automatic_downloads` por padrão; outras plataformas informam falso e ignoram o setter. Compilações sem suporte e gerenciadas externamente informam ambas as configurações como falsas. O WinSparkle informa metadados de atualização encontrada lendo o appcast após seu callback de encontrado; ele ainda controla a instalação. O Linux resolve a propriedade de pacote Debian ou Arch e a identidade do AppImage na inicialização; essas cópias informam `ManagedSelf`, enquanto outras cópias não empacotadas informam `ManagedPage` antes de qualquer verificação.

## Iniciar no login

`desktop.SetLaunchAtLogin(cx, on)` registra o aplicativo no mecanismo de login da plataforma, e `desktop.LaunchAtLogin(cx, done)` o lê. Um MSIX compilado pela ferramenta atual inclui automaticamente uma tarefa de inicialização desabilitada; habilitá-la respeita a opção de aplicativos de inicialização do usuário e a política do administrador. Instalações comuns de Windows e Linux mantêm seu valor Run e entrada de inicialização automática XDG. O macOS usa o serviço de login nativo do pacote do aplicativo.

O Flatpak solicita o portal Background, que pode pedir consentimento, sem conceder acesso aos arquivos de configuração do host. Uma leitura subsequente aguarda a resposta nativa. O portal não possui consulta de inicialização somente leitura: leituras no Flatpak retornam apenas a última resposta confirmada nesta execução, falso antes de uma, e não conseguem detectar uma alteração fora do aplicativo nem recuperar a configuração na próxima execução.

Um Snap empacotado externamente declara o nome do arquivo de autostart em seu aplicativo:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

O NSGo grava esse nome de arquivo em `$SNAP_USER_DATA/.config/autostart`, que o snapd inicia por meio do wrapper do aplicativo. `desktop.LaunchedAtLogin(cx)` distingue uma inicialização por login antes de o aplicativo ser construído, incluindo uma ativação de StartupTask de MSIX.

## Tipos de arquivo

`file_types` registra os documentos que um aplicativo abre: pelo Finder, pelo Dock e por Abrir Com no macOS, pelo Abrir Com do Explorer via instalador NSIS ou MSIX no Windows, pela entrada de desktop dos pacotes Debian, Arch, AppImage e Flatpak no Linux e por Abrir Em no iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

O instalador Windows adiciona o aplicativo ao Abrir Com sem substituir o padrão do usuário, e remove seus registros na atualização e desinstalação. O Linux nomeia os tipos MIME de extensões comuns a partir de uma tabela fixa e atribui a uma extensão desconhecida do próprio tipo do aplicativo `application/x-<type id>` com uma definição shared-mime-info. `mime` nomeia tipos existentes em vez disso; um tipo importado cuja extensão falta na tabela precisa dele. Executar um AppImage não registra nada: uma ferramenta de integração ao desktop instala sua entrada e definições.

O aplicativo recebe os arquivos com `desktop.OnOpenFiles`, incluindo uma inicialização posterior no Windows ou Linux enquanto já está rodando e aberturas de documentos em cenas do iOS. Arquivos que iniciaram o aplicativo aguardam o primeiro manipulador:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

No iOS, documentos regulares são importados para arquivos temporários de propriedade do aplicativo com seus nomes originais. Leituras de provedores de arquivo são coordenadas fora da thread de UI, e seus escopos de segurança terminam antes de o manipulador ser executado. Mova uma importação para `cx.DataDir()` para mantê-la; edições afetam a cópia. O iOS usa o ícone de documento do aplicativo; `icon` se aplica no macOS. App Clips e pacotes tvOS não registram tipos de documentos. Aberturas de arquivos permanecem separadas de `OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` registra um documento que o usuário abriu: no menu do Dock e em Arquivo › Abrir Recente no macOS, na categoria Recentes da Jump List no Windows e nos arquivos usados recentemente do desktop no Linux. Uma escolha chega ao mesmo manipulador. `desktop.ClearRecentFiles` esquece os registros do aplicativo, mantendo o histórico de outros aplicativos para arquivos compartilhados no Linux; o Linux também respeita a configuração de histórico do GTK.

`desktop.SetDockMenu(cx, items)` define ações no ícone do aplicativo: o menu do Dock do macOS, tarefas da Jump List do Windows e a quicklist no dock do Ubuntu, Plasma e Dash to Dock no Linux onde quicklists são suportadas. O Linux precisa de uma entrada de desktop instalada correspondente ao `id` do aplicativo, e preserva submenus, separadores, itens marcados e desabilitados. O menu pertence ao escopo que o definiu, e o último definidor entre as janelas do aplicativo o substitui.

## Links

`url_schemes` fornece ao aplicativo links próprios, como `myapp://settings`, que o abrem a partir de um navegador, outro aplicativo, `open`, `start` ou `xdg-open`. Os aplicativos macOS, iOS e Android os registram, assim como o instalador Windows e o pacote MSIX e a entrada de desktop Linux, que os pacotes Debian, Arch, AppImage e Flatpak carregam. A desinstalação os remove.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

O aplicativo os recebe com `desktop.OnOpenLinks`, também enquanto está em execução: no Windows e no Linux, uma segunda execução entrega seu link ao aplicativo em execução e encerra. Um link que iniciou o aplicativo aguarda o primeiro manipulador. Qualquer pessoa pode escrever um link assim, e no Windows e Linux o manipulador recebe qualquer URL com a qual o aplicativo é iniciado, portanto verifique o esquema e pergunte ao usuário antes que um link altere algo:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

No iOS e Android, `mobile.OnOpenLinks` escuta links na inicialização e enquanto o aplicativo está rodando. Abra um com `xcrun simctl openurl booted myapp://settings` no simulador iOS ou `adb shell am start -a android.intent.action.VIEW -d myapp://settings` no Android. `app.OpenLinks("myapp://settings")` entrega um em um teste.

Para links universais do iOS, declare hosts HTTPS explícitos com `app_links: [links.example.com]`. Cada host é um nome DNS sem esquema, caminho, porta, endereço IP ou curinga; os nomes são convertidos para minúsculas e duplicatas são rejeitadas. A ferramenta escreve `applinks:<host>` no entitlement Associated Domains do aplicativo completo, inclusive em assinaturas gerenciadas pelo Xcode; um perfil de IPA deve autorizá-lo. App Clips mantêm as declarações separadas `appclips:` de `app_clip.domains`.

Cada host deve servir um arquivo `apple-app-site-association` que nomeie o aplicativo completo em `https://<host>/.well-known/apple-app-site-association`; os metadados do pacote por si sós não podem autorizar um link universal. Os [requisitos de associação da Apple](https://developer.apple.com/documentation/xcode/supporting-associated-domains) descrevem a configuração do servidor. Assinaturas ad hoc de simulador omitem entitlements restritos de desenvolvedor, portanto um teste de delegate não comprova a associação com o servidor.

No iOS, `mobile.OpenedURL(cx)` rastreia a invocação de App Clip, link universal ou URL de esquema personalizado mais recente, incluindo o valor inicial antes de a raiz ser montada. A entrega de esquema personalizado ainda chega a `mobile.OnOpenLinks`; o valor da invocação está atualizado quando esse manipulador é executado. Aberturas de arquivos permanecem em `OnOpenFiles` e não substituem a URL de invocação. O suporte a invocações no Android/Web é acompanhado separadamente na issue #1475.

## Compartilhamentos recebidos no Android

Escolha os tipos MIME que o aplicativo trata a partir da folha de compartilhamento do Android:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

Compilações de APK e App Bundle registram esses tipos tanto para itens únicos quanto múltiplos. Registre [`mobile.OnShared`](services.md#opening-links-and-files) para receber cada compartilhamento.

Para um aplicativo web com endereços baseados em caminho, defina `web.base` para seu diretório de implantação (`/` ou `/my-app/`) e configure seu servidor de produção para servir `index.html` para as rotas da aplicação. O servidor de desenvolvimento já fornece esse fallback para requisições de navegação HTML. Os ativos permanecem relativos a essa base HTML fixa quando um link profundo é recarregado. Veja [Endereços da aplicação](addresses.md).

## Compartilhamentos recebidos no iOS

Declare os Uniform Type Identifiers que a extensão de compartilhamento aceita, com tipos de URL e texto antes de tipos de arquivo mais amplos:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

`build` e `dev` empacotam uma extensão de compartilhamento Go, `<id>.share`, com o aplicativo. O Xcode a assina e registra seu App Group para execuções em dispositivo. Para um IPA, habilite o mesmo App Group para `<id>` e `<id>.share`; `sign.profile` e `sign.share_profile` devem concedê-lo. App Clips, tvOS e watchOS não possuem extensão de compartilhamento.

A extensão copia até 128 itens e 512 MiB de arquivos para o App Group e, em seguida, conclui a folha de compartilhamento. `mobile.OnShared` recebe esses lotes enquanto o aplicativo está ativo ou quando o usuário o abre em seguida; eles sobrevivem ao encerramento da extensão e do aplicativo antes da entrega. O iOS não fornece nenhuma API suportada para que uma extensão de compartilhamento traga para o primeiro plano ou inicie seu aplicativo contêiner. Esse repasse, portanto, não abre o aplicativo automaticamente.

## Miniprogramas

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Cada alvo grava um projeto para a ferramenta de desenvolvedor de sua plataforma, também em `dist/releases/miniprogram/<platform>`, que `-run` e `dev` abrem; `dev` recompila quando você salva, e a ferramenta recarrega:

| Alvo | Plataforma | Ferramenta |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html), com sua porta de serviço ligada (Settings, Security) |
| `miniprogram_qq` | QQ | [Ferramenta de desenvolvedor do QQ](https://q.qq.com/wiki/tools/devtool/), no macOS |
| `miniprogram_alipay` | Alipay | [Mini Program Studio da Alipay](https://opendocs.alipay.com/mini/ide/download), no macOS |
| `miniprogram_douyin` | Douyin e outros aplicativos da ByteDance | [Ferramenta de desenvolvedor do Douyin](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download), no macOS |

`miniprogram.appid` fornece o AppID do miniprograma em cada plataforma (`wechat`, `qq`, `alipay`, `douyin`); sem um, o WeChat DevTools o exibe com seu AppID de visitante e as outras ferramentas pedem um. No WeChat, `-run` e cada recompilação de `dev` fecham e reabrem automaticamente o projeto pela CLI, para que o DevTools compile os arquivos novos. Isso foi verificado com o DevTools 2.01.2510290. A barra de navegação da plataforma exibe o título do aplicativo; com `custom_navigation`, o aplicativo preenche a página e sua área segura o mantém livre do botão de menu. O aplicativo lê o sistema operacional do telefone com `Env.Simulated` verdadeiro, como na web com `--nsgo-os`: as páginas de uma pilha exibem seu próprio `Header`, pois nenhum cabeçalho do sistema o substitui. QQ, Alipay e Douyin ainda não foram testados em suas ferramentas, e o Alipay ainda não recebe voltar pelo aplicativo, quadros-chave ou a altura do teclado.

Miniprogramas são experimentais. Um miniprograma executa apenas aplicativos Go, não TSX, e as plataformas limitam o módulo comprimido do aplicativo a 2 MB: um aplicativo de poucas telas em `base` cabe, como faz `examples/gallery-miniprogram`, enquanto um dos controles de `ui` ainda não cabe, e `build` avisa quando não couber. Shaders não desenham nada ali, e web views, vídeos, componentes nativos e arquivos de fonte do aplicativo ainda não são exibidos.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` grava `<name>-<version>-harmonyos.hap` em `dist/releases`. A engine roda como WebAssembly no ArkWeb, e um host ArkTS desenha o que monta com visualizações ArkUI, de modo que texto, campos, rolagem e diálogos são os do próprio sistema. Com `hdc` no `PATH`, `-run` instala o HAP no dispositivo ou emulador conectado e o inicia, e `nsgo test -target harmonyos` executa os testes de um aplicativo a partir de um HAP não assinado em um emulador sob uma identidade de pacote `.nsgotest` separada, removendo esse HAP de teste após a execução para que o aplicativo instalado permaneça disponível.

Para compilar o host ArkTS a partir dos templates e usar o `id` do aplicativo como nome do pacote, coloque `hvigorw` no `PATH` e defina `OHOS_BASE_SDK_HOME` para um SDK do OpenHarmony com API 26 (`26.0.0`), a versão de SDK de compilação e alvo dos templates. `build.harmonyos_bundles` escolhe `auto` (o padrão), `exact`, que exige o SDK, ou `padded`, que preenche o id até o comprimento fixo do host comitado e não precisa de SDK. Testes de alvo usam a mesma configuração e imprimem qual host empacotaram. `harmonyos.permissions` lista os nomes `ohos.permission` que o aplicativo solicita, como `MICROPHONE` e `CAMERA`, e `widget.title` é o que seu cartão da tela inicial exibe.

HarmonyOS é uma prévia. Emuladores podem instalar HAPs não assinados. As [notas de arquitetura](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) listam os recursos do ArkUI que a prévia desenha.

### HAPs assinados

Defina `harmonyos.signing` para assinar um HAP com seus materiais do AppGallery Connect. Uma instalação direta em um telefone Huawei precisa de um certificado de depuração e perfil para aquele dispositivo. Use os materiais de lançamento para distribuição. [Guia de assinatura da Huawei](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Os caminhos são relativos a `nsgo.yaml`. Defina `NSGO_HARMONY_KEYSTORE_PASSWORD` e `NSGO_HARMONY_KEY_PASSWORD` no ambiente. A assinatura usa o `hap-sign-tool.jar` do SDK e o Java de `JAVA_HOME` ou `PATH`. A compilação mantém o `id` exato do aplicativo para corresponder ao perfil, portanto a assinatura precisa do SDK e do `hvigorw` mesmo no modo `auto`. `go tool nsgo doctor`, executado no diretório do aplicativo, verifica a configuração de assinatura e o nome do pacote do perfil sem senhas. `build -run` passa o HAP assinado ao `hdc`.

## Assinatura

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

As senhas do keystore vêm de `NSGO_KEYSTORE_PASSWORD` e `NSGO_KEY_PASSWORD`. `-team` assina o pacote do iPhone. `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` e `-macos-profile` substituem os perfis de provisionamento do aplicativo, da extensão de widget, da extensão de compartilhamento, do App Clip e da Mac App Store pela linha de comando. `-installer` substitui a identidade Mac Installer Distribution.

### Assinar pacotes Windows

O Authenticode identifica o publicador de um download Windows; o [SmartScreen verifica sua reputação separadamente](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). `sign.certificate`, um `.pfx`, assina o executável, o companheiro de comando, os programas e bibliotecas que os recursos do aplicativo trazem, o instalador com seu desinstalador e o MSIX compilado para o canal Store; sua senha vem de `NSGO_CERTIFICATE_PASSWORD`. As assinaturas são feitas em Go, como o `signtool` as faz, de modo que um Mac as assina sem o Windows SDK:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` usa por padrão o da DigiCert, e `none` deixa as assinaturas sem carimbo de data/hora, fazendo com que percam a validade quando o certificado expirar. Um certificado em um token de hardware ou em um serviço na nuvem assina por meio de um comando próprio, no qual `%1` é o arquivo a assinar e que a compilação executa para cada arquivo acima:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

`build` para se o comando tiver sucesso mas deixar o executável do aplicativo, o companheiro de comando, um recurso PE ou o executável de instalação sem uma assinatura Authenticode incorporada.

Assinar o desinstalador dentro do instalador precisa do NSIS 3.08 ou posterior, e um certificado que não seja `.pfx` não pode assiná-lo via `sign.certificate`. `-certificate`, `-sign-command` e `-timestamp-url` definem essas opções pela linha de comando.

### Autenticar (Notarize)

Aplicativos baixados em um Mac abrem sem o aviso do Gatekeeper depois que a Apple os autentica. Assine com um Developer ID e nomeie as credenciais que o `notarytool` guarda no seu keychain:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

`build` então assina com o hardened runtime, envia a imagem de disco, aguarda o veredito da Apple — alguns minutos — e grampeia (staples) o tíquete na imagem e no aplicativo; uma rejeição imprime o log da Apple. No CI, `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` e `NSGO_NOTARY_ISSUER`, o arquivo `.p8`, o ID e o emissor de uma chave de API do App Store Connect, assumem o lugar do perfil. O hardened runtime nega o microfone, a câmera, os calendários e a localização a menos que o aplicativo os reivindique, o que `build` faz quando `apple.info` possui `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, uma descrição de uso de calendários ou lembretes ou uma descrição de uso de localização.

## Widgets e Live Activities

`build` e `dev` compilam a extensão que desenha os widgets e Live Activities do aplicativo nos pacotes macOS e iOS com o compilador Swift do Xcode, sem nenhuma etapa separada. No iOS, o aplicativo e seus widgets compartilham um app group: `-run` em um iPhone faz o Xcode assinar ambos e registrar o grupo; para um IPA, habilite-o para ambos `<id>` e `<id>.widgets`, e assine a extensão com `sign.widgets_profile`. APKs e AABs independentes do Android incluem provedores de App Widget e recursos de `widgets`; o runtime Java pré-compilado desenha suas linhas do tempo sem necessidade de SDK nem compilador Java nas compilações do aplicativo. O Android 16 exibe Live Activities como Live Updates, que não precisam de extensão. [Widgets](widgets.md) e [Live Activities](live-activities.md) cobrem sua configuração e API.

## Android App Links

`app_links` nomeia hosts HTTPS explícitos cujos links abrem o aplicativo Android completo:

```yaml
app_links: [example.com, shop.example.com]
```

O APK e o App Bundle declaram um filtro `ACTION_VIEW` navegável com `autoVerify` para cada host. Use hosts DNS sem esquema, caminho, porta, curinga ou endereço IP. Sirva `https://<host>/.well-known/assetlinks.json` com o ID de pacote do aplicativo e a impressão digital do certificado de assinatura; o Android verifica essa associação antes de rotear links para o aplicativo. A ferramenta não publica arquivos de associação; o [guia de verificação do Android](https://developer.android.com/training/app-links/verify-applinks) descreve sua configuração. Um host Android incorporado declara seus próprios filtros de intent e encaminha links aceitos para sua `NsgoView`.

`mobile.OpenedURL(cx)` lê a invocação aceita antes da primeira renderização da raiz e muda quando outro link abre o aplicativo em execução. `mobile.OnOpenLinks` ainda escuta cada link aceito. Um aplicativo web de página inteira lê o endereço da página atual por meio de `mobile.OpenedURL`, incluindo o histórico do navegador e alterações de hash; um aplicativo montado dentro de outra página não lê nenhum endereço da página contêiner. Use [`cx.AppURL`](addresses.md) quando o aplicativo também escreve seu endereço.

## App Clips

Um App Clip é uma pequena parte de um aplicativo iOS que roda a partir de um link, um código ou uma tag NFC sem instalar o aplicativo. `app_clip` compila um dentro do pacote iOS, a partir de um pacote `main` próprio para que vincule apenas o que exibe:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

O clip lê o link que o abriu com `mobile.OpenedURL`, que muda quando outro link o abre novamente, e `mobile.IsAppClip` informa se o código está rodando como um clip. O sistema baixa um clip de até 15 MB, ou 100 MB a partir do iOS 17 quando apenas links o abrem, e `build` recusa um maior; um clip que vincula `ui` ocupa cerca de 11 MB. `dev -target ios_simulator -clip https://example.com/order?table=12` executa apenas o clip no simulador, aberto a partir daquele link; `-target ios` o executa em um iPhone e imprime o código QR do link: adicione o link em Ajustes > Desenvolvedor > Experiências Locais e depois escaneie-o com a câmera. Use `-release` se os símbolos de desenvolvimento fizerem o clip ultrapassar o limite de tamanho do QR. A Apple assina App Clips apenas para equipes do Apple Developer Program, não para equipes pessoais.

`dev -target ios` e `build -run -target ios` também mantêm o clip dentro do aplicativo completo: o Xcode o assina como um alvo separado e o incorpora antes de assinar o aplicativo. A assinatura automática precisa de uma conta de equipe paga autenticada no Xcode, com provisionamento tanto para o aplicativo quanto para o clip. Para a App Store, habilite App Clips para `<id>`, assine o clip com `sign.clip_profile` e sirva o arquivo `apple-app-site-association` dos domínios. O `examples/appclip` do repositório é uma cafeteria cujo clip faz pedidos em uma mesa.

Para repassar um pedido ou outros dados locais ao aplicativo completo, defina um grupo explícito ao lado de `app_clip`:

```yaml
apple:
  app_group: group.dev.example.hello
```

O clip e o aplicativo completo passam então a compartilhar `cx.DataDir()` e `cx.Storage()` no contêiner do grupo. Arquivos sob o diretório de dados e entradas de chave-valor que o clip grava ficam disponíveis quando o aplicativo completo inicia; `cx.LogsDir()` acompanha esse diretório, enquanto cada pacote mantém seu próprio cache. A assinatura do Xcode reivindica o grupo para ambos os alvos; ao usar perfis, habilite o mesmo grupo para ambos os App IDs e conceda-o em `sign.profile` e `sign.clip_profile`. Uma concessão ausente recusa a assinatura, e um grupo inacessível interrompe a inicialização. Um diretório de dados explícito de um host ainda tem precedência.

Sem `apple.app_group`, cada pacote mantém seu próprio armazenamento em sandbox. Habilitar a configuração não migra dados armazenados anteriormente nesses sandboxes, e aplicativos somente com widgets mantêm sua localização de armazenamento existente. Mantenha segredos compartilhados no keychain em vez de armazenamento simples. O exemplo da cafeteria persiste o pedido de seu clip e o exibe no aplicativo completo.

A partir de uma ação explícita do usuário dentro do clip, chame `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)` para solicitar ao StoreKit que recomende seu aplicativo completo correspondente. `AppClipOverlayBottomRaised` deixa espaço para uma barra inferior. A função retornada dispensa essa solicitação; a limpeza do escopo também a dispensa, e uma solicitação mais antiga não pode dispensar uma mais nova. Chamadas fora de um App Clip nativo do iOS não fazem nada. A App Store decide se pode carregar e exibir a recomendação, portanto a chamada não informa nem uma oferta exibida nem uma instalação. Mantenha a tarefa do clip utilizável sem instalar o aplicativo completo. [Orientação da Apple sobre o overlay](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Incorporar em outro aplicativo

`-embed` empacota o aplicativo para um aplicativo iOS ou Android próprio: um XCFramework, de frameworks dinâmicos com `-dynamic`, ou um AAR cuja `NSGoView` ou `NsgoView` o exibe. Programas desktop em Go exibem aplicativos com `nsgo.NewView`, e páginas web com o `nsgo.js` do site, cujo host é o da página ou o nomeado por `web.host` em `nsgo.yaml`. Os `examples/embed-*` do repositório mostram cada caso; [Incorporação](embedding.md) aborda o lado do host.
