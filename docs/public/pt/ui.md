---
title: Referência de componentes
description: Os componentes mais usados, suas variantes, estados e acessibilidade.
group: Componentes
order: 30
---

Estes são os componentes do `ui`, com a aparência do NSGo e seguindo o tema. Esta referência começa pelos componentes que um app usa primeiro; cada página começa pelo exemplo mais curto e depois aborda suas partes, estados e acessibilidade. A galeria abrange todos os componentes, incluindo aqueles que ainda não têm uma página aqui.

A [galeria](https://nsgo.dev/gallery/) executa no navegador uma demonstração de cada componente.

## Ações

- [Button](ui/button.md): um botão de ação, simples ou na cor de destaque, e o botão de ícone.

## Texto e entrada

- [Text](ui/text.md): parágrafos, trechos, links, fontes e texto de documento.
- [Input](ui/input.md): o campo de texto da plataforma, controlado ou não.
- [Checkbox](ui/checkbox.md): uma caixa de seleção, marcada, desmarcada ou indeterminada.
- [Switch](ui/switch.md): uma configuração que está ligada ou desligada.
- [Select](ui/select.md): um valor de uma lista, como pop-up ou como folha.
- [Slider](ui/slider.md): um valor ou um intervalo ao longo de uma trilha.

## Coleções

- [List](ui/list.md): linhas em qualquer quantidade, construídas apenas quando visíveis, com seleção e ativação.
- [Scroll](ui/scroll.md): a view de rolagem da plataforma, com paginação, encaixe, zoom e atualização.

## Navegação

- [Tabs](ui/tabs.md): uma lista de abas, um controle segmentado e a barra de abas do sistema.

## Sobreposições

- [Dialog](ui/dialog.md): uma superfície modal, uma folha e o alerta do sistema.
- [Menu](ui/menu.md): um menu a partir de um gatilho, um menu de contexto e seus itens.

## Mais

A galeria abrange o restante: tabelas, gráficos, o editor de código, texto rico, o terminal, o dock, a paleta de comandos, seletores de cor, data e emoji (`ui.EmojiPicker`), toasts e mais. Cada um tem uma demonstração lá.

`base/terminal.View` abre caminhos locais existentes com Command+clique (Control+clique no Windows e no Linux), assim como abre URLs. Caminhos relativos usam o diretório de trabalho OSC 7 do shell; um shell local que não informou nenhum usa `terminal.Options.Dir` ou, quando não definido, o diretório pessoal. Coloque entre aspas nomes de arquivo que contenham espaços. Autoridades de diretórios remotos e conexões sem informação de diretório local não são resolvidas em relação a arquivos locais. Arquivos e pastas usam `Scope.OpenFile`; links de URL continuam passando por `Scope.OpenURL` e pelo manipulador de URLs do host de incorporação.

`base/terminal.View` nega por padrão leituras e gravações da área de transferência feitas por programas. Defina `AllowClipboardRead` e `AllowClipboardWrite` somente quando o app as permitir; o `AccessClipboard` da view solicitante continua valendo. As mesmas props estão disponíveis no `Terminal` do TSX e podem mudar sem reiniciar sua sessão. Os apps fornecem a interface de consentimento. Uma colagem explícita do usuário oferece, de forma independente, um instantâneo de texto/PNG aos programas que usam o protocolo de área de transferência do Kitty (modo 5522); sua senha de uso único só consegue ler esse instantâneo. Programas sem esse modo recebem texto comum entre colchetes (bracketed paste). A disponibilidade de imagens na área de transferência segue os serviços de área de transferência da plataforma; a seleção primária do X11 não é exposta.

## Próximos passos

- [Componentes e estado](components.md): como um componente retorna elementos e como os signals os atualizam.
- [Estilos e temas](styles.md): o tema do qual os componentes obtêm suas cores.
- [Componentes sem estilo](authoring/base.md): os mesmos controles sem aparência, para uma aparência própria.
- [Acessibilidade](accessibility.md): rótulos, papéis e valores para cada controle.
