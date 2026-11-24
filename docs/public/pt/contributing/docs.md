---
title: Documentação e o site
description: Escreva uma página desta documentação, traduza-a, execute o site e mantenha atualizada a skill para agentes.
group: Contribuir
order: 83
---

## Uma página

Esta documentação é composta pelos arquivos Markdown em `docs/public/`, legíveis no GitHub como estão. `en/` e `zh/` espelham um ao outro caminho por caminho; uma página ausente em chinês exibe a versão em inglês com um aviso. Cada página começa com front matter:

```yaml
---
title: Testing
description: Test an app the way a user uses it, headless or in a native window.
group: Toolchain
order: 21
---
```

- `group` é o grupo da página na barra lateral, no idioma da página, e `order` define sua posição. Os grupos seguem o caminho de entrada de um leitor: tutoriais, ferramentas, componentes, conceitos principais e, em seguida, escrevendo componentes e contribuindo. Uma página sem grupo, como a introdução, vem antes deles.
- O layout exibe o título e a descrição, portanto o corpo começa em `##`.

Faça links entre páginas com links `.md` relativos (`[Testes](testing.md#run-on-a-platform)`); o site os transforma em suas URLs. Cada página responde a uma pergunta; como o NSGo funciona internamente fica em `docs/architecture/`.

## O site

O site em `website/` é compilado pelo [nsgo-website](https://github.com/nsgo-dev/nsgo-website), uma integração do Astro, a partir das configurações em `website/site.config.ts`. Ele precisa do [Bun](https://bun.com) 1.4 ou posterior:

```sh
cd website
bun install
bun run dev    # at http://localhost:4321/
bun run build  # type-check and build, as the deployment does
```

`website/README.md` explica como o site é compilado e publicado.

## Para agentes

O site também publica as páginas em inglês para modelos de linguagem: `/llms.txt` as lista, `/llms-full.txt` contém todas elas e `/docs/<page>.md` contém uma página individual, com links absolutos e sem as marcas dos blocos de código (`src/lib/llms.ts`). Elas acompanham as páginas sem nenhuma etapa extra.

`skills/nsgo/` é a skill que agentes instalam com `bunx skills add nsgo-dev/nsgo`: `SKILL.md` com as regras que um aplicativo segue, e `references/` lidos quando uma tarefa precisa deles. Altere-a na mesma modificação que a documentação quando mudar o que um aplicativo escreve, os comandos da ferramenta, os testes ou o empacotamento; mantenha-a restrita ao que um agente deve seguir e aponte para a documentação para o restante. `go tool nsgo test -run TestSkills .` verifica seu front matter, seus links e cada nome do NSGo que ela menciona.
