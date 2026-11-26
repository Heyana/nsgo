---
title: Bibliotecas nativas
description: Distribua bibliotecas compartilhadas pré-compiladas e frameworks da Apple com um pacote Go usando as etapas normais de empacotamento, licença e assinatura do NSGo.
group: Ferramentas
order: 23
---

Um pacote Go pode distribuir as bibliotecas compartilhadas que carrega via purego colocando `nsgo-natives.yaml` no diretório do pacote e, a partir da versão 2, seus frameworks para iOS e tvOS. Quando um app importa o pacote, o NSGo seleciona as declarações para o alvo, verifica os arquivos e os inclui antes da assinatura e da criação dos instaladores. O app não precisa de nenhuma configuração ou comando de empacotamento extra.

As compilações de apps de desktop e os testes nos alvos desativam o cgo para o código do app. O código nativo deve usar purego e uma biblioteca declarada em `nsgo-natives.yaml`. O NSGo rejeita pacotes do app que contêm arquivos cgo antes de a compilação começar.

## Declarar bibliotecas

```yaml
version: 2
libraries:
  macos_arm64:
    - name: libexample.dylib
      path: lib/macos-arm64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  macos_x64:
    - name: libexample.dylib
      url: https://downloads.example.org/v1.0/macos-x64/libexample.dylib
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  windows_arm64: []
  windows_x64: []
  linux_arm64: []
  linux_x64: []
  ios:
    - name: Example.framework
      url: https://downloads.example.org/v1.0/ios/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  ios_simulator:
    - name: Example.framework
      path: lib/ios-simulator/Example
      sha256: "<64 hexadecimal digits for this file>"
      licenses: [licenses/LICENSE, licenses/NOTICE]
  tvos: []
  tvos_simulator: []
```

A versão 2 suporta as chaves `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64`, `linux_x64`, `ios`, `ios_simulator`, `tvos` e `tvos_simulator`. A versão 1 suporta apenas as seis chaves de desktop; um arquivo é lido apenas para as chaves da sua versão, então compilações de iOS e tvOS não empacotam nada de um arquivo da versão 1. Declare todos os alvos em que o pacote Go é selecionado; `[]` informa explicitamente que aquele alvo não precisa de biblioteca empacotada. Como alternativa, build tags do Go podem manter o pacote fora dos alvos que ele não suporta. `macos_universal` utiliza as duas entradas de macOS, e um alvo de terminal as do seu desktop: `tui_macos_arm64` utiliza `macos_arm64`.

Cada biblioteca possui:

- `name`: o nome do arquivo instalado, terminado em `.dylib` no macOS, `.dll` no Windows ou `.so` (ou `.so.*` versionado) no Linux. No iOS e no tvOS, é o de um framework, `<Name>.framework`, em que `Name` é formado por letras ASCII, dígitos, `.` e `-`, começando e terminando com uma letra ou um dígito.
- Exatamente um entre `path` (um arquivo relativo ao diretório deste pacote) ou `url` (uma URL HTTP(S) para a biblioteca em si). A versão 2 aceita apenas URLs https, e um download falha quando um redirecionamento sai do https. Publique bibliotecas já extraídas em vez de URLs de arquivos compactados e, para um framework, apenas o seu binário. URLs não devem conter nomes de usuário e senhas embutidos.
- `sha256`: o SHA-256 do arquivo original, obrigatório para qualquer uma das origens. Por exemplo, `shasum -a 256 lib/macos-arm64/libexample.dylib` o imprime no macOS.
- `licenses`: caminhos não vazios para os arquivos originais de licença e aviso legal, relativos ao diretório deste pacote. Esses textos vão para `THIRD_PARTY_NOTICES.txt` e chegam a `core.ThirdPartyNotices()` em compilações de release. A [política de licenças](building.md#avisos-de-licença) existente do app, incluindo exceções exatas por módulo, aplica-se de forma independente às licenças nativas.

Caminhos locais e links simbólicos devem permanecer dentro do pacote declarante. O NSGo rejeita campos desconhecidos, divergências de checksum, arquivos destinados a outro sistema operacional ou arquitetura e nomes instalados conflitantes. Dois pacotes podem compartilhar um arquivo com nome idêntico caso tenha o mesmo checksum. O limite de tamanho por arquivo é 1 GiB.

## Empacotar e carregar

| Alvo | Diretório da biblioteca |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows, Linux e alvos de terminal | Ao lado do executável |
| iOS e tvOS | `<app>.app/Frameworks/<Name>.framework/<Name>` |

O pacote carrega sua biblioteca a partir desse diretório, usando a localização do executável em vez do diretório de trabalho. Pré-compile as dependências de forma que bibliotecas dependentes carreguem umas às outras a partir do diretório em que são distribuídas. Declare cada arquivo auxiliar necessário e cada nome de arquivo de que o carregador precisa.

O NSGo verifica novamente os downloads em cache antes de reutilizá-los e coordena compilações concorrentes por meio de uma trava de arquivo. O cache de conteúdo fica sob o diretório de cache de usuário do sistema operacional, em `nsgo/natives`. Arquivos locais são verificados à medida que são copiados.

Um pacote universal para macOS exige os mesmos nomes de biblioteca para ambas as arquiteturas. O NSGo une as fatias com `lipo`; uma biblioteca já universal fornecida pelo publicador pode servir para ambas as entradas. As bibliotecas são assinadas antes do app e depois passam pelas etapas normais de imagem de disco, assinatura e notarização do app. A assinatura no Windows inclui as DLLs declaradas mesmo com `build.installers: false`. Os instaladores no Linux incluem os mesmos arquivos e avisos que o diretório do executável. Os alvos de terminal assinam as bibliotecas de macOS de forma ad hoc.

O modo de desenvolvimento, `-run` e os testes nos alvos usam as mesmas etapas de seleção e cópia de pacotes, tanto no desktop quanto no iOS e no tvOS. As declarações são selecionadas com o alvo, as tags e o overlay Go gerado da compilação, incluindo substituições locais (`replace`) e pacotes em `vendor`. Android, HarmonyOS, watchOS, a web e os miniprogramas não empacotam nada a partir deste arquivo.

## Frameworks de iOS e tvOS

O NSGo verifica o binário de cada framework e interrompe a compilação informando o motivo, a menos que ele seja:

- uma biblioteca dinâmica Mach-O arm64 compilada para a plataforma do alvo, dispositivos ou simulador, como indica o seu `LC_BUILD_VERSION`; um arquivo universal fornece a sua fatia arm64;
- para um sistema operacional mínimo não mais recente que o do app, `apple.info.MinimumOSVersion` ou, na falta dele, iOS 15.0 e tvOS 17.0 ([Compilação e distribuição](building.md#nsgoyaml));
- com o nome de instalação `@rpath/<Name>.framework/<Name>`;
- e que carregue via `@rpath` apenas frameworks que os pacotes do app declaram para o mesmo alvo.

O NSGo grava o `Info.plist` do framework, cujo `CFBundleIdentifier` é o ID do app, um ponto e `Name` em minúsculas, com a plataforma e o sistema operacional mínimo do app. Compilações para o simulador assinam o framework de forma ad hoc. Uma compilação para dispositivos o deixa para a assinatura do app: `sign.profile` o assina com a identidade do app antes do próprio app, como o Xcode faz para `nsgo build -run` e `nsgo dev` em um dispositivo. Dê a um framework que carrega seus pares o caminho de execução `@loader_path/..`, para que ele os encontre em `Frameworks`.

`nsgo build -embed` para, citando o pacote, quando o app vincula um pacote que declara frameworks de iOS: o XCFramework para um app hospedeiro ainda não consegue levá-los.

## Preservar arquivos ao usar vendor

A operação `vendor` do Go precisa de dados incorporados (`embed`) para manter arquivos dentro do diretório de um pacote. Um pacote pode marcar seu manifesto e suas licenças para inclusão sem usar seus bytes em tempo de execução:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Inclua também caminhos de bibliotecas locais nessa diretiva `embed` ao usar `path`, por exemplo `lib/*`. Downloads por URL precisam apenas dos arquivos de manifesto e licença no pacote Go.
