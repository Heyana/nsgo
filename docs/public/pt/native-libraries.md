---
title: Bibliotecas nativas
description: Distribua bibliotecas compartilhadas pré-compiladas com um pacote Go usando as etapas normais de empacotamento, licença e assinatura do NSGo.
group: Ferramentas
order: 23
---

Um pacote Go pode distribuir as bibliotecas compartilhadas que carrega via purego colocando `nsgo-natives.yaml` no diretório do pacote. Quando um app importa o pacote, o NSGo seleciona as declarações para o alvo, verifica os arquivos e os inclui antes da assinatura e da criação dos instaladores. O app não precisa de nenhuma configuração ou comando de empacotamento extra.

## Declarar bibliotecas

```yaml
version: 1
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
```

As chaves suportadas são `macos_arm64`, `macos_x64`, `windows_arm64`, `windows_x64`, `linux_arm64` e `linux_x64`. Declare todos os alvos em que o pacote Go é selecionado; `[]` informa explicitamente que aquele alvo não precisa de biblioteca empacotada. Como alternativa, build tags do Go podem manter o pacote fora dos alvos que ele não suporta. `macos_universal` utiliza as duas entradas de macOS.

Cada biblioteca possui:

- `name`: o nome do arquivo instalado, terminado em `.dylib` no macOS, `.dll` no Windows ou `.so` (ou `.so.*` versionado) no Linux.
- Exatamente um entre `path` (um arquivo relativo ao diretório deste pacote) ou `url` (uma URL HTTP(S) para a biblioteca em si). Publique bibliotecas já extraídas em vez de URLs de arquivos compactados. URLs não devem conter nomes de usuário e senhas embutidos.
- `sha256`: o SHA-256 do arquivo original, obrigatório para qualquer uma das origens. Por exemplo, `shasum -a 256 lib/macos-arm64/libexample.dylib` o imprime no macOS.
- `licenses`: caminhos não vazios para os arquivos originais de licença e aviso legal, relativos ao diretório deste pacote. Esses textos vão para `THIRD_PARTY_NOTICES.txt` e chegam a `core.ThirdPartyNotices()` em compilações de release. A [política de licenças](building.md#avisos-de-licença) existente do app, incluindo exceções exatas por módulo, aplica-se de forma independente às licenças nativas.

Caminhos locais e links simbólicos devem permanecer dentro do pacote declarante. O NSGo rejeita campos desconhecidos, divergências de checksum, arquivos destinados a outro sistema operacional ou arquitetura e nomes instalados conflitantes. Dois pacotes podem compartilhar um arquivo com nome idêntico caso tenha o mesmo checksum. O limite de tamanho por arquivo é 1 GiB.

## Empacotar e carregar

| Alvo | Diretório da biblioteca |
| --- | --- |
| macOS | `<app>.app/Contents/Frameworks` |
| Windows e Linux | Ao lado do executável |

O pacote carrega sua biblioteca a partir desse diretório, usando a localização do executável em vez do diretório de trabalho. Pré-compile as dependências de forma que bibliotecas dependentes carreguem umas às outras a partir do diretório em que são distribuídas. Declare cada arquivo auxiliar necessário e cada nome de arquivo de que o carregador precisa.

O NSGo verifica novamente os downloads em cache antes de reutilizá-los e coordena compilações concorrentes por meio de uma trava de arquivo. O cache de conteúdo fica sob o diretório de cache de usuário do sistema operacional, em `nsgo/natives`. Arquivos locais são verificados à medida que são copiados.

Um pacote universal para macOS exige os mesmos nomes de biblioteca para ambas as arquiteturas. O NSGo une as fatias com `lipo`; uma biblioteca já universal fornecida pelo publicador pode servir para ambas as entradas. As bibliotecas são assinadas antes do app e depois passam pelas etapas normais de imagem de disco, assinatura e notarização do app. A assinatura no Windows inclui as DLLs declaradas mesmo com `build.installers: false`. Os instaladores no Linux incluem os mesmos arquivos e avisos que o diretório do executável.

O modo de desenvolvimento e os testes nativos para desktop usam as mesmas etapas de seleção e cópia de pacotes. As declarações são selecionadas com o alvo, as tags e o overlay Go gerado da compilação, incluindo substituições locais (`replace`) e pacotes em `vendor`. Outros alvos atualmente não empacotam bibliotecas a partir deste arquivo.

## Preservar arquivos ao usar vendor

A operação `vendor` do Go precisa de dados incorporados (`embed`) para manter arquivos dentro do diretório de um pacote. Um pacote pode marcar seu manifesto e suas licenças para inclusão sem usar seus bytes em tempo de execução:

```go
import "embed"

//go:embed nsgo-natives.yaml licenses/*
var _ embed.FS
```

Inclua também caminhos de bibliotecas locais nessa diretiva `embed` ao usar `path`, por exemplo `lib/*`. Downloads por URL precisam apenas dos arquivos de manifesto e licença no pacote Go.
