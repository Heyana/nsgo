---
title: Localization
description: Translate an app's messages and follow the language it shows.
group: Core concepts
order: 62
---

## Messages

`locales/` beside `nsgo.yaml` holds the app's messages, a YAML file per language (`en.yaml`, `de.yaml`) in ICU MessageFormat:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

A message is a string, or `message` with a `description` and `placeholders` for translators.

The package imports `locales/`. `dev`, `build` and `test` check the catalogs and write a Go function per message beside them, such as `locales.Greeting(cx, "Ada")`, which says it in the language the app shows and follows it as it changes; `nsgo i18n` writes the functions alone. Messages format numbers and dates (`{d, date, medium}`), and `appName` names the app in each language in its packages, as a message with an `apple.info` key, such as `NSCameraUsageDescription`, translates that Info.plist entry ([permissions](permissions.md#usage-descriptions)).

## The language

`cx.SetLocale("de")` chooses the language, and the layout runs in the direction it is written in. In a standalone app, what the system says itself—such as text selection menus, file dialogs and GTK's widgets—follows the forced language too (immediately on Android 13+, Windows and Linux, and from the next launch on macOS and iOS); `cx.SetLocale("")` follows the user's language again.

```sh
nsgo dev -lang en-XA
```

runs the app in a pseudo locale that shows untranslated and cut text. Any build takes it as it starts: `--nsgo-lang=en-XA`.

## Weeks, times and measures

`i18n.Regional(cx)` says how the user reads weeks, times and measures, apart from the language: the day weeks start on, 12- or 24-hour time, the measurement system and the temperature unit, as they set them on the platform, else as their region does. Calendars and date pickers start their weeks there and time fields show those hours unless the app sets `FirstDayOfWeek` or `HourCycle`, and the times a catalog's messages write follow the user's hours.

## NSGo's own words

NSGo's components say their words in the languages NSGo is translated to. For another language, a key that names a package, such as `github.com/nsgo-dev/nsgo/base/locales`, gives that package's messages in the file's language. The platforms' menus, and the roles and states Android's and Windows' screen readers announce where the system has no word of its own, such as a tab or a search box, say the words of `github.com/nsgo-dev/nsgo/desktop/locales`.

`nsgo` warns when a dependency is missing an app language and lists which keys are missing if that language is only partly translated. To accept fallback to the dependency's source language, set `i18n: { coverage: off }` in `nsgo.yaml`. This leaves all other catalog warnings and errors enabled.
