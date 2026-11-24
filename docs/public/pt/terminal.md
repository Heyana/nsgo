---
title: Terminal
description: Execute um programa ou exiba uma sessão remota em uma grade de terminal.
group: Conceitos principais
order: 63
---

`base/terminal` fornece uma sessão (`Terminal`) e sua `View`. Desktops executam programas locais; iOS e navegadores usam `Options.Conn`, um `io.ReadWriteCloser` conectado a um programa remoto. O app é dono da sessão e a fecha ao terminar.

```go
t, err := terminal.New(terminal.Options{CursorBlink: true})
if err != nil {
	return core.Text{Content: err.Error()}
}
cx.Cleanup(func() { t.Close() })
return terminal.View{
	Terminal: t, AutoFocus: true,
	MinimumContrast: 4.5, PaddingColor: terminal.PaddingExtend,
	Style: core.Style{FlexGrow: 1},
}
```

`Options.Cursor` escolhe o padrão entre bloco, barra ou sublinhado. `CursorBlink` ativa a piscada padrão; zero mantém o cursor fixo. Os programas substituem a forma e a piscada por meio de DECSCUSR ou do modo DEC 12. Um cursor sem foco é vazado e fixo; a composição e o movimento reduzido também o mantêm fixo. O relógio de animação da plataforma conduz a piscada sem que o motor redesenhe o texto do terminal.

`View.Font`, `Theme` e `DarkTheme` definem a fonte e a paleta. A grade desenha ela mesma os caracteres Unicode de desenho de caixas e de blocos e os separadores Powerline U+E0B0–U+E0B7, alinhando as células adjacentes aos pixels do dispositivo. O restante do texto usa as fontes da plataforma e seus fallbacks.

`MinimumContrast` aumenta o contraste do texto comum em relação ao fundo real de sua célula, incluindo cores invertidas, texto esmaecido e seleção. Valores de 1 a 21 usam a razão de contraste da WCAG; zero preserva as cores. Gráficos e emoji mantêm suas cores. Com `Transparent`, o contraste ainda usa o fundo configurado do terminal.

`Padding` deixa espaço fora da grade, em pontos. `PaddingColor` escolhe seu fundo:

| Valor | Comportamento |
| --- | --- |
| `PaddingBackground` | Fundo do terminal, incluindo mudanças via OSC 11 |
| `PaddingExtend` | Fundo da célula mais próxima; na tela principal, suprime a extensão superior/inferior ao lado de linhas de prompt, linhas Powerline ou linhas que contenham uma célula com o fundo padrão |
| `PaddingExtendAlways` | Estende todas as bordas sem essas exceções |

Apps TSX importam `tsx/terminal` pelo seu efeito de registrar `Terminal` em `nsgo/base`. As props são `cursorBlink`, `minimumContrast` e `paddingColor`; esta última aceita `background`, `extend` e `extend-always`. As props de aparência podem mudar enquanto a sessão continua.

## Em um navegador

O app incorpora o módulo WebAssembly fixado do libghostty-vt e executa a mesma visualização de terminal e o mesmo caminho de entrada. Compilar o app não exige Zig, instalação de biblioteca nativa nem download em tempo de execução.

Adapte o WebSocket do navegador a `io.ReadWriteCloser`: envie bytes binários e junte ou divida as mensagens recebidas como um fluxo. O app e seu servidor são responsáveis pela autenticação, pela reconexão e por quaisquer mensagens de redimensionamento/controle; o NSGo consome os bytes do terminal. O [fixture de aceitação do navegador](https://github.com/nsgo-dev/nsgo/blob/main/internal/testhost/testdata/terminalweb/socket_js_test.go) mostra um adaptador de fluxo de bytes completo com tratamento de fechamento.

## Ações e zoom

`View.Keybinds` substitui os padrões com `Keybind{Key, Action, Amount}`. As teclas usam os mesmos nomes de `Scope.OnKey`, como `mod+=` ou `ctrl+shift+c`. As entradas posteriores prevalecem. Os atalhos são executados enquanto a entrada deste terminal tem o foco e nenhum método de entrada está compondo.

As ações são `CopyToClipboard`, `PasteFromClipboard`, `SelectAll`, `ClearScreen`, `IncreaseFontSize`, `DecreaseFontSize`, `ResetFontSize`, `SetFontSize` e `Ignore`. Ações de janela e de aba pertencem ao mapa de teclas do app. `Ignore` consome uma tecla sem enviá-la ao programa.

Por padrão, `mod+=` e `mod+shift+=` aumentam a fonte em um ponto, `mod+-` a diminui, e `mod+0` a redefine para `View.Font.Size` (13 quando não definido). `Amount` define o passo para aumentar/diminuir, um quando zero, ou o tamanho para `SetFontSize`. Os tamanhos são limitados a 1–255 pontos; solicitações não finitas são ignoradas. O zoom mantém a sessão e a visualização externa, atualiza as linhas e colunas da grade e redimensiona o programa.

```go
ref := &terminal.Ref{}
view := terminal.View{
	Terminal: session, Ref: ref,
	Keybinds: []terminal.Keybind{
		{Key: "ctrl+shift+k", Action: terminal.ClearScreen},
		{Key: "mod+=", Action: terminal.IncreaseFontSize, Amount: 2},
	},
}
```

`Ref.FontSize()` é rastreado para controles que o exibem; `Ref.SetFontSize(size)` e `Ref.Perform(action, amount)` são executados no motor do app, como os handlers de eventos. `Perform` informa se uma ação está disponível. Uma ref desmontada retorna tamanho zero e recusa ações.

## Menu de contexto

Um clique secundário abre um `base.ContextMenu` sem estilo com os itens localizados Copiar, Colar, Selecionar tudo e Limpar. Copiar exige uma seleção. Limpar remove o conteúdo acima do prompt e fica desativado na tela alternativa. Telas sensíveis ao toque usam a apresentação por toque longo do menu de contexto. Estilize a superfície com `MenuClass`/`MenuStyle` e os itens com `MenuItemClass`/`MenuItemStyle`.

Defina `RightClickAction` como `RightClickPaste`, `RightClickCopy`, `RightClickCopyOrPaste` (copia uma seleção, senão cola) ou `RightClickIgnore`. Ignore deixa o evento para um menu do app que o envolva; use-o ao fornecer um `ui.ContextMenu` estilizado que chama as ações da ref.

Importe `tsx/terminal` para registrar `<Terminal>` em `nsgo/base`. Suas props reativas `keybinds` e `rightClickAction` atualizam a visualização mantendo a sessão. Os valores de ação usam nomes do Ghostty, como `increase_font_size` e `copy_to_clipboard`; os valores de clique direito são `context-menu`, `paste`, `copy`, `copy-or-paste` e `ignore`. As props de estilo do menu usam os nomes Go em lower camel case.

## Relatórios de mouse

Programas que ativam o rastreamento do mouse recebem os toques, liberações, arrastos e hover solicitados, codificados pelo libghostty-vt no protocolo que selecionaram. Shift ignora a captura de cliques e arrastos para permitir a seleção. `View.MouseReporting` é um `func() bool` reativo opcional, verdadeiro quando nil; retorne false para manter a seleção com o mouse e a rolagem nativa do histórico mesmo enquanto um programa solicita a captura. O clique direito de um programa tem prioridade sobre um menu de contexto envolvente enquanto os relatórios estão ativados.

Na tela alternativa, com o modo de rolagem alternativa ativado e nenhum rastreamento de mouse solicitado, a entrada vertical da roda vira teclas de cursor, seguindo o modo de teclas de cursor normal ou de aplicação. `MouseScrollMultiplier{Precision: 1, Discrete: 3}` define as contagens de relatório padrão para touchpads e rodas; zero usa esses padrões, e valores positivos são limitados a 0,01–10.000. A entrada fracionária se acumula até que um passo completo esteja disponível. O histórico normal mantém a rolagem nativa e a inércia da plataforma.

Importe `tsx/terminal` para registrar `<Terminal>` em `nsgo/base`. Sua prop `mouseReporting` tem padrão true; `mouseScrollPrecision` e `mouseScrollDiscrete` definem os multiplicadores.
