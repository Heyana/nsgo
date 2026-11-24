---
title: Publicando na web
description: Publique um aplicativo NSGo com semântica de documento, metadados de página e assets em CDN.
group: Ferramentas
order: 24
---

## Compile o aplicativo

```sh
go tool nsgo build -target web
```

Sirva o diretório gerado por HTTP. Os assets de release têm hashes de conteúdo em seus nomes, então armazene-os em cache como imutáveis; revalide `index.html`, `nsgo.json` e `manifest.webmanifest`. O aplicativo web mantém o comportamento de layout, rolagem, foco e ponteiro do NSGo.

## Títulos e links

Uma `core.View` ou um `core.Text` com `Role: core.RoleHeader` se torna `h1` a `h6`, selecionado por `AccessibilityLevel`; sem um nível, é `h2`. Os títulos Markdown de `base/textview` levam seus níveis. As margens e fontes de título do navegador não alteram o estilo do aplicativo.

`base.Link` e `ui.Link` publicam seu `Href` como uma âncora. Um clique comum executa `OnOpen` e `OnPress` do aplicativo; um clique do meio ou um clique com um modificador usa a ação de link do navegador. O navegador fornece seu menu de contexto de link, a menos que o aplicativo trate `OnContextMenu`. Um link desabilitado não tem endereço ativo. Para uma view personalizada com papel de link, defina `core.View.Web` como `core.WebViewProps{Href: "/notes"}` ou vincule `HrefFunc`. TSX usa `web={{href: "/notes"}}`; títulos usam `accessibilityLevel={1}`. Esquemas de URL executáveis inseguros são recusados. Conteúdo confinado mantém a navegação nos seus manipuladores de toque com escopo e não anuncia nenhum endereço ao navegador.

## Metadados de página

Importe `github.com/nsgo-dev/nsgo/web` e coloque um `web.Head` ao lado do conteúdo de uma página:

```go
core.Group{
	web.Head{Title: "Notes", Description: "Your saved notes"},
	pageContent,
}
```

`TitleFunc` e `DescriptionFunc` seguem signals. Heads visíveis posteriores substituem os campos que definem e herdam os demais. Ocultar ou remover uma página restaura os heads anteriores; remover todos os heads restaura o documento original. Um campo estático vazio é herdado; uma função que retorna uma string vazia o limpa. Um head não adiciona nenhum nó de layout. Alvos nativos, aplicativos incorporados e conteúdo confinado não alteram o documento do seu host.

Em TSX, importe `Head` de `nsgo/web` e escreva `<Head title={title()} description="Your saved notes" />`. Para pré-visualizações em HTML estático, pré-renderize cada URL como parte da implantação do site; `Head` atualiza o documento em execução.

## Sirva assets de uma CDN

O loader resolve todos os arquivos do aplicativo em relação a `nsgo.json`. Uma página pode carregar o aplicativo de outra origem:

```js
const url = new URL("https://cdn.example.com/notes/nsgo.json");
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url).href);
await mount(null, url.href, { config });
```

Permita que a origem da página busque os arquivos da CDN com CORS e sirva WebAssembly como `application/wasm`. Sirva o HTML de entrada do aplicativo para todas as rotas profundas que seu aplicativo usa. [Incorporação](embedding.md#web) mostra como montar em um elemento.
