---
title: Endereços da aplicação
description: Leia e atualize o endereço de um aplicativo web através do histórico do navegador.
group: Conceitos principais
order: 59
---

`cx.AppURL()` lê o endereço da aplicação como um valor rastreado. Na web, a URL inicial da página fica disponível antes da renderização da raiz, para que um recarregamento ou link compartilhado possa escolher a mesma visualização. O aplicativo decide como a URL mapeia para suas visualizações.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` atualiza o endereço rastreado imediatamente sem requisitar uma página ou navegar em uma pilha. Por padrão, ele substitui a entrada atual do navegador; `HistoryPush` solicita uma nova entrada. Uma navegação ou empilhamento de modal no mesmo commit fornece essa entrada em seu lugar, para que uma única ação não crie dois passos para voltar. Várias escritas em um commit mantêm o último endereço e preservam qualquer solicitação de empilhamento.

Endereços relativos são resolvidos em relação ao endereço atual ou a uma base HTML explícita. Um endereço de navegador deve permanecer na origem da página. Endereços inválidos, espaços em branco, caracteres de controle, barras invertidas e endereços com mais de 16 KiB retornam um erro sem alterá-lo.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` observa navegações pelo histórico do navegador e edições de hash. `change.Reason` é `"back"`, `"forward"` ou `"address"`; a função retornada cancela a inscrição, e o descarte do escopo remove o ouvinte. Leia `AppURL` para obter o endereço inicial. As próprias escritas do aplicativo atualizam os vínculos, mas não enviam uma notificação de alteração. Uma navegação que mantém o mesmo endereço não envia alteração.

O botão voltar do navegador ainda dispensa um modal ou desempilha uma tela de navegação. Um retorno recusado pelo aplicativo restaura a entrada existente e seu endereço. Um manipulador de dispensa pode definir seu próprio endereço substituto. Um desempilhamento programático atualiza o endereço após a travessia assíncrona do histórico, e escritas que chegam durante essa travessia aguardam por ela. Voltar além das entradas deste player sai do aplicativo, incluindo entradas retidas de um carregamento anterior.

Para caminhos como `/films/42`, defina o diretório de implantação do aplicativo e faça o servidor servir seu `index.html` para as rotas da aplicação:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` escreve uma base HTML para que scripts, estilos, Wasm e recursos continuem carregando quando um endereço profundo for aberto diretamente. Sem ela, os caminhos relativos de ativos existentes são mantidos; endereços com query ou hash podem usá-los sem alterações. Regras de fallback do servidor devem deixar requisições reais de ativos intactas. Uma Política de Segurança de Conteúdo (CSP) estrita deve permitir essa base de mesma origem (`base-uri 'self'`).

Um aplicativo incorporado em outra página mantém seu endereço internamente e deixa o histórico da página contêiner intacto. Um iframe de mesma origem pode optar explicitamente por participar com `data-nsgo-history`. Subárvores confinadas não podem ler nem alterar o endereço da aplicação.

Em outros alvos, `AppURL` mantém o último link aberto com o aplicativo ou o valor que `SetAppURL` armazenou; defini-lo não altera nenhuma janela nativa nem o navegador do sistema. Miniprogramas também mantêm esse valor, sem alterar as rotas de seu host.

TSX usa o mesmo estado:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
