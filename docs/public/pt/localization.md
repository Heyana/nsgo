---
title: Localização
description: Traduza as mensagens de um app e acompanhe o idioma exibido por ele.
group: Conceitos principais
order: 62
---

## Mensagens

`locales/` ao lado de `nsgo.yaml` armazena as mensagens do app, em um arquivo YAML por idioma (`en.yaml`, `de.yaml`) no formato ICU MessageFormat:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Uma mensagem é uma string, ou `message` com `description` e `placeholders` para os tradutores.

O pacote importa `locales/`. `dev`, `build` e `test` validam os catálogos e geram uma função Go por mensagem ao lado deles, como `locales.Greeting(cx, "Ada")`, que a exibe no idioma mostrado pelo app e se atualiza quando ele muda; `go tool nsgo i18n` gera apenas as funções. Mensagens formatam números e datas (`{d, date, medium}`), e `appName` nomeia o app em cada idioma em seus pacotes, enquanto uma mensagem com uma chave `apple.info`, como `NSCameraUsageDescription`, traduz essa entrada do Info.plist ([permissões](permissions.md#usage-descriptions)).

## O idioma

`cx.SetLocale("de")` escolhe o idioma, e o layout passa a seguir a direção em que ele é escrito. Em um app independente, o que o próprio sistema exibe — como menus de seleção de texto, diálogos de arquivo e widgets do GTK — também segue o idioma forçado (imediatamente no Android 13+, Windows e Linux, e a partir da próxima inicialização no macOS e iOS); `cx.SetLocale("")` volta a seguir o idioma do usuário.

```sh
go tool nsgo dev -lang en-XA
```

executa o app em uma pseudo-localidade que evidencia textos não traduzidos e cortados. Qualquer compilação aceita o parâmetro na inicialização: `--nsgo-lang=en-XA`.

## Semanas, horários e medidas

`i18n.Regional(cx)` informa como o usuário lê semanas, horários e medidas, independentemente do idioma: o dia em que a semana começa, o formato de 12 ou 24 horas, o sistema de medidas e a unidade de temperatura, conforme definidos na plataforma ou, na ausência disso, pela região. Calendários e seletores de data iniciam a semana nesse dia e campos de hora exibem esse formato, a menos que o app defina `FirstDayOfWeek` ou `HourCycle`; os horários formatados pelas mensagens de um catálogo também seguem o formato de horas do usuário.

## As palavras do próprio NSGo

Os componentes do NSGo exibem seus textos nos idiomas para os quais o NSGo está traduzido. Para outro idioma, uma chave que nomeia um pacote, como `github.com/nsgo-dev/nsgo/base/locales`, fornece as mensagens daquele pacote no idioma do arquivo. Os menus das plataformas, bem como os papéis e estados que os leitores de tela do Android e do Windows anunciam quando o sistema não possui termo próprio — como uma aba ou uma caixa de pesquisa — utilizam os textos de `github.com/nsgo-dev/nsgo/desktop/locales`.
