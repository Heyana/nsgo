---
title: A base de código
description: Onde fica cada parte do NSGo e os documentos que explicam o porquê.
group: Contribuir
order: 81
---

O NSGo é um único módulo Go. Os aplicativos importam seus pacotes de cima; o backend de cada plataforma fica na base e recebe o que mudou como views nativas para criar, atualizar e remover.

## Pacotes

| Pacote | O que ele contém |
| --- | --- |
| `nsgo` (a raiz) | `nsgo.Run` e `nsgo.NewView`, escolhendo um backend por build tags |
| `core` | A API do aplicativo: elementos, estilos, classes, temas, signals e serviços; layout e preparação de texto |
| `base` | Componentes sem estilo, um arquivo por componente, e os mais complexos como pacotes (`base/dock`, `base/editor`) |
| `ui` | Os componentes de `base` com uma aparência definida e pacotes como `ui/chat` |
| `mobile`, `desktop` | O que apenas celulares ou apenas desktops possuem |
| `tsx` | Aplicativos TSX, seu empacotador e compilador, e os módulos que importam |
| `i18n`, `*/locales` | Catálogos de mensagens e o código gerado a partir deles |
| `nsgotest` | Testes que agem como um usuário, em modo headless ou em uma plataforma |
| `internal/engine`, `internal/driver`, `internal/layout` | A goroutine do motor, o protocolo de montagem e o layout |
| `internal/platform/<target>` | Um backend: views nativas, texto e entrada para um alvo |
| `cmd/nsgo` | A ferramenta `nsgo` |
| `examples/` | A galeria e os outros aplicativos de exemplo |

Os pacotes importam apenas para baixo; `internal/moduletest/layering_test.go` verifica o grafo em todos os alvos.

## Documentos de design

[ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) mapeia os pacotes, as threads e os fluxos de um commit até a tela, e lista os assuntos em `docs/architecture/`: um por parte, como layout, texto, rolagem, temas ou a ferramenta. Leia o assunto do que você vai alterar antes de modificá-lo; ele registra como a parte funciona e onde diverge do [Native Shell](https://github.com/longbridge/nsh), cujo design o NSGo porta.

[AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md) contém as regras que toda alteração segue, tanto para pessoas quanto para agentes de código. Algumas moldam a maioria das alterações:

- Cada API tem um único lar: `core` para elementos e o ambiente, `base` para controles, `ui` para suas aparências, `mobile` e `desktop` para um tipo de dispositivo.
- `base` não tem aparência definida; um componente de `ui` é sua forma de `base` com uma.
- Nada muda implicitamente por dispositivo: nada de controles maiores em desktops ou menores em celulares dentro de `core` ou `base`.
- Views nativas em todo lugar, iguais em todas as plataformas: o que uma plataforma tem nativamente, as outras imitam.
- Nenhum C escrito à mão, e nada além de macOS, Xcode e Go para compilar todos os alvos exceto o emulador x86_64 do Android.

O que falta, plataforma por plataforma, está no [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md).
