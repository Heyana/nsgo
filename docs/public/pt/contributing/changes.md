---
title: Fazendo uma alteração
description: O que um pull request para o NSGo precisa, desde a mesma alteração em cada backend até testes, documentação e uma linha no changelog.
group: Contribuir
order: 82
---

## Antes de começar

Abra uma issue ou uma discussão para qualquer coisa maior do que uma correção simples, para que o design seja acordado antes do código. Questões que o design deixa em aberto estão listadas em [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md#open-questions); suas respostas são decididas lá primeiro.

## Cada backend

Uma correção ou um recurso em um backend normalmente pertence aos outros também. Faça a mesma alteração em cada alvo que precisa dela, ou registre o que está faltando no [roadmap](https://github.com/nsgo-dev/nsgo/blob/main/docs/roadmap.md). Os componentes têm aparência e comportamento iguais em todas as plataformas.

## Verificações

Use a ferramenta `nsgo` em vez de `go vet`, `go test` ou `go build`:

```sh
go tool nsgo check   # vet every package
go tool nsgo test    # every package's tests
```

Adicione um teste para o que você alterar: `nsgotest` para o que um usuário vê, e os testes do próprio pacote para o restante. Execute a galeria nas plataformas que você tocou ([Configuração de desenvolvimento](../contributing.md#the-gallery)).

Formate os arquivos Go com `gofmt` da versão do Go definida em `go.mod`; `gofmt -l .` lista os arquivos que precisam de formatação. O CI verifica cada arquivo Go rastreado e falha informando os nomes daqueles que não estão formatados.

## O pull request

Dê a ele um título no formato Conventional Commit, como `fix(linux): keep the caret after a paste`: um squash merge torna o título o assunto do commit, e uma verificação falha até que ele se encaixe. O template solicita os alvos que a alteração toca, capturas de tela de antes e depois de qualquer coisa visível e as verificações que você executou.

## Documentação e o changelog

- Atualize o documento cujos fatos mudaram: um assunto em `docs/architecture/` para como o NSGo funciona, ou uma página em `docs/public/` para como aplicativos e componentes o usam ([Documentação e o site](docs.md)).
- Mantenha `README.md` e `README.zh-CN.md` equivalentes; todo o resto é em inglês, com páginas em chinês em `docs/public/zh/`.
- Registre uma alteração visível aos usuários em uma única linha, com uma referência como `(#12 by @you)`, em um arquivo próprio dentro de `changelog.d/` sob o título do seu tipo, como `### Fixed`, em vez de editar `CHANGELOG.md`, que um lançamento preenche a partir desses arquivos ([formato](https://github.com/nsgo-dev/nsgo/blob/main/changelog.d/README.md)).
