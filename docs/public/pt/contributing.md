---
title: Configuração de desenvolvimento
description: Compile o NSGo a partir do código-fonte, execute a galeria e teste uma alteração.
group: Contribuir
order: 80
---

## Requisitos

Um macOS com Xcode e Go 1.27.3 ou posterior compila todos os alvos para arm64, incluindo o APK do Android. Executar em dispositivos exige o que [Configuração](getting-started.md#requirements) lista, e o alvo do emulador x86_64 do Android, `android_x64`, exige um Android NDK, já que o Go vincula `android/amd64` apenas com cgo. Os programas de instalação e desinstalação do Windows são compilados apenas com Go.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Leia [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) e [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) antes de alterar o código.

## A galeria

A galeria tem uma demonstração de cada elemento, componente de `base` e `ui` e serviço. Uma janela larga mostra as demonstrações sob abas em uma barra lateral; uma janela estreita mostra as abas e as configurações em uma barra inferior, com as demonstrações empurradas sobre elas. Execute-a a partir da raiz do repositório:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Verificação e testes

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` verifica os alvos nomeados em vez do host e, no repositório do NSGo, o cliente de desenvolvimento, que o `check` simples verifica. Para verificar as plataformas que uma alteração toca, nomeie todas elas em uma única verificação, como `check -target web,android -jobs 2`, em vez de iniciar várias: `-jobs n` verifica até n alvos ao mesmo tempo, cada `go vet` com n workers de compilação, onde o padrão é um alvo por vez com o padrão do Go, e uma verificação iniciada enquanto outra roda no mesmo módulo aguarda por ela e avisa. O `check.yml` do CI verifica todos os alvos.

Executado na raiz do repositório ou em `internal/dev`, o comando headless testa `internal/dev` uma segunda vez com a build tag `nsgodev`, de que os arquivos do cliente de desenvolvimento precisam, para que seus testes rodem localmente e no CI. Ambas as execuções aceitam os mesmos filtros e flags e compartilham uma contagem e um relatório JUnit, onde o pacote da segunda execução se chama `internal/dev [nsgodev]`.

`check -target all` verifica os alvos macOS, Windows, Linux, Android arm64 e web em qualquer host. Ele também verifica iOS, tvOS e watchOS quando seus SDKs do Xcode estão disponíveis e lista os que pula. Um alvo nomeado explicitamente, como `check -target ios`, falha se suas ferramentas não estiverem disponíveis, inclusive quando combinado com `all`. Um alvo com falha não impede a verificação dos alvos restantes, e o comando falha se qualquer verificação obrigatória falhar. `build -target all` ainda precisa do macOS com Xcode. No próprio repositório do NSGo, cada alvo nomeado também verifica seus hosts de teste de plataforma com `nsgotest`; o CI nomeia o alvo preliminar `harmonyos` explicitamente, já que ele está fora de `all`.

O job de CI no Linux executa os testes `js/wasm` do backend Go do HarmonyOS com Node.js e o lançador Wasm do Go. Execute o teste da galeria localmente com um emulador do HarmonyOS conectado via `hdc`. Compilar os [templates de host do HarmonyOS](building.md#harmonyos) exige a API 26 do SDK OpenHarmony.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

Em uma sessão Wayland em um desktop Linux, execute a verificação real de aceitação do portal de atalhos globais com `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux`. O ID deve ter uma entrada `.desktop`. Conceda Ctrl+Alt+K no prompt do compositor e depois pressione o atalho com outro aplicativo em foco; a verificação aguarda tanto o registro quanto a ativação. Ela é pulada a menos que a variável de ambiente esteja definida.

Com o Docker disponível em um host Linux, `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` verifica instalações reais em contêineres descartáveis Debian, Ubuntu e Arch. Ele baixa suas imagens oficiais e instala dependências do GTK dentro deles, verifica cada banco de dados de pacotes e os arquivos instalados e então remove o pacote. O banco de dados de pacotes do host nunca é usado.

## Lançamentos

O NSGo faz lançamentos sob demanda quando um projeto dependente precisa de uma versão com tag ou os mantenedores têm alterações prontas para publicar. Ao solicitar um lançamento para uma biblioteca, vincule o trabalho dependente e nomeie o commit que contém a última alteração do NSGo de que ele precisa. A primeira tag de lançamento que contém esse commit é a versão que a biblioteca pode exigir.

Para testar uma alteração mesclada antes de ela receber uma tag, execute `go get github.com/nsgo-dev/nsgo@main` no módulo dependente. Você pode usar um ID de commit no lugar de `main` para selecionar a revisão verificada. O Go registra a pseudoversão resultante em `go.mod`.

Um mantenedor atribui às alterações sua versão e data em `CHANGELOG.md`, deixa uma nova seção Unreleased e cria uma tag no commit resultante em `main`. O [workflow de lançamento](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) verifica a tag e a publica após o workflow de testes passar; as notas publicadas então saem do `CHANGELOG.md`, que mantém apenas alterações não lançadas, para [Releases](https://github.com/nsgo-dev/nsgo/releases).

## NSH

O NSGo usa o [Native Shell](https://github.com/longbridge/nsh) como referência de design; nenhum dos testes dele precisa dele. Para lê-lo, clone-o ao lado do repositório e faça checkout da sua tag de lançamento mais recente, nunca de `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Próximos passos

- [A base de código](contributing/codebase.md): onde cada parte do NSGo vive.
- [Fazendo uma alteração](contributing/changes.md): o que um pull request precisa.
