---
title: Textos, testes e publicação
description: Traduza o que um componente diz, teste-o como um usuário faria e ofereça-o para aplicativos Go e TSX.
group: Escrevendo componentes
order: 74
---

## Textos

Um componente expressa seus próprios textos, como o nome de um botão para leitores de tela, por meio dos catálogos do seu pacote em vez de strings literais. Coloque um pacote `locales/` na biblioteca com o catálogo do idioma de origem e um arquivo que gera o código:

```yaml
# locales/en.yaml
clear:
  message: Clear
  description: What screen readers say of the button that empties a chip input.
```

```go
// Package locales says the chips' words in the language the app shows.
package locales

//go:generate go tool nsgo i18n
```

`go generate ./locales` escreve uma função por mensagem, que o componente chama com seu escopo: `locales.Clear(cx)` a pronuncia no idioma que o aplicativo exibe. Faça commit do arquivo `catalog_gen.go` gerado, para que os aplicativos compilem apenas com Go. Adicione um arquivo por idioma para o qual traduzir (`zh-Hans.yaml`); um aplicativo fornece outros em seus próprios catálogos, sob uma chave que nomeia o caminho de importação do seu pacote, como em [Textos próprios do NSGo](../localization.md#as-palavras-do-próprio-nsgo).

## Testes

`nsgotest` executa um componente da forma como um usuário o utiliza, em modo headless. Monte-o em uma pequena raiz e aja sobre ele por papel e texto:

```go
func TestMain(m *testing.M) { nsgotest.Main(m) }

func TestToggle(t *testing.T) {
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		on := core.NewSignal(cx, false)
		return chips.Toggle{On: on.Get, OnChange: on.Set, Label: "Bold"}
	})
	app.Press("role:switch Bold")
	if !app.Find("role:switch Bold").Props.Checked {
		t.Fatal("the toggle did not turn on")
	}
}
```

A partir da raiz da biblioteca, `go tool nsgo test` executa todos os pacotes, e `-target macos` ou outro alvo executa os mesmos testes em uma janela nativa. Teste ambas as aparências e uma pseudolocalidade (`-lang en-XA`) para detectar cores e textos fixos no código. [Testes](../testing.md) cobre o restante.

## Publicação

Uma biblioteca de componentes é um módulo Go: os aplicativos usam `go get` e importam seus pacotes. Mantenha suas formas sem estilo e estilizada em pacotes separados quando ambas existirem, para que um aplicativo com visual próprio não importe estilos que não usa.

A página [Componentes](https://nsgo.dev/components/) do site lista bibliotecas que outras pessoas podem usar; adicione um documento e uma captura de tela em `docs/public/components/` para disponibilizar a sua.

Aplicativos TSX acessam componentes Go depois que eles são registrados, em uma função `init`:

```go
func init() {
	tsx.Register("example.com/chips", map[string]core.Node{"Toggle": Toggle{}})
}
```

`import { Toggle } from "example.com/chips"` passa então a aceitar os campos como props ([Aplicativos TSX](../tsx.md#go-a-partir-do-tsx)).
