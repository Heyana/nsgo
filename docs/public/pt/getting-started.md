---
title: Configuração
description: Instale o que o NSGo precisa, crie um app e execute-o em cada plataforma de destino.
group: Primeiros passos
order: 10
---

## Requisitos

- [Go](https://go.dev/dl/) 1.27.3 ou superior.
- Para compilar para iOS, tvOS e watchOS, um Mac com Xcode. Todos os outros alvos compilam no macOS apenas com Go; o emulador de Android x86_64 também precisa de um Android NDK.
- Para executar em um iPhone, uma conta Apple ID configurada nas Contas do Xcode e o iPhone marcado como confiável, com o Modo de Desenvolvedor ativado.
- Para executar no Android, `adb` do pacote platform-tools do Android SDK.
- Para executar no Windows, apenas Go, que também compila os pacotes para Windows e web.
- No Linux, GTK 4.14 ou superior, Pango e Cairo. Imagens de rede usam libsoup 3, web views usam WebKitGTK 6.0 e áudio e vídeo usam plug-ins do GStreamer. Em sistemas com restrições de namespace de usuário do AppArmor, o [WebKitGTK precisa de um perfil bubblewrap](debugging.md#web-views-no-linux).

## Crie um app

Crie um módulo, adicione o NSGo e sua ferramenta, e deixe a ferramenta gerar um app:

```sh
mkdir hello && cd hello
go mod init example.com/hello
go get github.com/nsgo-dev/nsgo
go get -tool github.com/nsgo-dev/nsgo/cmd/nsgo
go tool nsgo init
```

`go get -tool` adiciona `nsgo` ao módulo uma única vez, para que `go tool nsgo` o execute sem instalar nada globalmente. `init` cria:

| Arquivo | O que é |
| --- | --- |
| `app.go` | O app, um contador: um componente `Root` e `main`, que chama `nsgo.Run(Root)` |
| `app_test.go` | Um teste que pressiona o botão |
| `nsgo.yaml` | O nome, id, versão e alvos do app |

`go tool nsgo init -template tsx` cria um app em TSX, e `-template mixed` cria um app em Go cuja entrada em TSX utiliza componentes próprios escritos em Go.

Adicione bibliotecas com `nsgo add <módulo>` em vez de `go get`: ele mostra o que `go.mod` e `go.sum` receberiam antes de gravá-los e nunca tira o NSGo do projeto de suas versões publicadas ([Adicionar bibliotecas](building.md#adicionar-bibliotecas)).

### Modelos a partir de módulos Go

Passe o caminho do módulo do modelo com uma versão opcional. Se a versão for omitida, será resolvida a versão mais recente encontrada pelo proxy de módulos do Go.

```sh
go tool nsgo init -template example.com/my-template@v1.2.0 -module example.com/hello hello
```

Publique o projeto inicial dentro do diretório `template/` do módulo. Inclua `nsgo.yaml` e `main.go`. A ferramenta processa ambos, além de qualquer arquivo terminado em `.tmpl`, usando `text/template` do Go e remove `.tmpl` dos nomes de saída. Outros arquivos, incluindo assets binários, são copiados sem alterações.

Os modelos recebem `.Name` (o nome de exibição do app), `.ID` (o ID gerado para o app), `.Slug` (o nome em minúsculas separado por hifens) e `.Module` (o valor de `-module`, ou o slug). Distribua um `go.mod.tmpl` para declarar o módulo e as dependências do app gerado, já que o Go exclui arquivos `go.mod` aninhados ao baixar módulos. Sem ele, `-module` escreve o mesmo `go.mod` mínimo que usa para um modelo integrado.

## Execute

```sh
go tool nsgo dev
```

`dev` compila o app para esta máquina e o abre. A cada salvamento, ele recompila e reinicia o app; uma alteração apenas em TSX atualiza o app em execução no próprio lugar. Se a compilação falhar, o erro é exibido sobre o app com botões para copiar a mensagem e compilar novamente.

Execute em outro alvo com `-target`:

```sh
go tool nsgo dev -target web                # in the browser
go tool nsgo dev -target ios_simulator      # in the iOS simulator
go tool nsgo dev -target tvos_simulator     # in the Apple TV simulator
go tool nsgo dev -target watchos_simulator  # in the Apple Watch simulator
go tool nsgo dev -target android            # on a connected device or emulator
```

`go tool nsgo doctor` lista cada alvo indicando se esta máquina consegue compilá-lo e executá-lo, e o que falta para os que não consegue — como Xcode, `adb`, um dispositivo conectado ou GTK.

[Execução e depuração](debugging.md) aborda dispositivos, rastreamento de atualizações, o monitor de desempenho e outras linguagens.

## Com um agente de codificação

A skill do NSGo ensina agentes de codificação, como Claude Code, Codex e Cursor, como um app NSGo é escrito, testado e distribuído. Adicione-a ao diretório do app, ou com `-g` para todos os projetos:

```sh
bunx skills add nsgo-dev/nsgo
```

Agentes que leem a documentação por conta própria começam por <https://nsgo.dev/llms.txt>, que lista cada página em Markdown em `https://nsgo.dev/docs/<page>.md`; <https://nsgo.dev/llms-full.txt> contém todas elas. Um agente também pode usar o app em execução ([Execução e depuração](debugging.md#agentes)).

## Próximos passos

- [Exemplo: um app de tarefas](tutorials/todo.md): estado, uma lista e um teste, em cerca de dez minutos.
- [Exemplo: um app de notas](tutorials/notes.md): uma pilha de navegação e notas salvas no dispositivo.
- [Exemplo: repositórios populares do GitHub](tutorials/repos.md): carregamento pela rede com tratamento de erros e nova tentativa.
