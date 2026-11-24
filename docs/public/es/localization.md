---
title: Localización
description: Traduce los mensajes de una aplicación y sigue el idioma que muestra.
group: Conceptos básicos
order: 62
---

## Mensajes

`locales/` junto a `nsgo.yaml` contiene los mensajes de la aplicación, un archivo YAML por idioma (`en.yaml`, `de.yaml`) en formato ICU MessageFormat:

```yaml
greeting: Hello, {name}!
appName:
  message: Hello
  description: The app's name, for translators.
```

Un mensaje es una cadena, o `message` con una `description` y `placeholders` para los traductores.

El paquete importa `locales/`. `dev`, `build` y `test` comprueban los catálogos y generan una función en Go por mensaje junto a ellos, como `locales.Greeting(cx, "Ada")`, que lo expresa en el idioma que muestra la aplicación y lo sigue conforme cambia; `go tool nsgo i18n` genera únicamente las funciones. Los mensajes dan formato a números y fechas (`{d, date, medium}`), y `appName` nombra la aplicación en cada idioma en sus paquetes, mientras que un mensaje con una clave `apple.info`, como `NSCameraUsageDescription`, traduce esa entrada de `Info.plist` ([permisos](permissions.md#descripciones-de-uso)).

## El idioma

`cx.SetLocale("de")` selecciona el idioma, y la disposición se ejecuta en la dirección en la que se escribe. En una aplicación independiente, lo que dice el propio sistema —como menús de selección de texto, cuadros de diálogo de archivos y los widgets de GTK— también sigue el idioma forzado (de inmediato en Android 13+, Windows y Linux, y a partir del próximo inicio en macOS e iOS); `cx.SetLocale("")` vuelve a seguir el idioma del usuario.

```sh
go tool nsgo dev -lang en-XA
```

ejecuta la aplicación en una pseudolocalización que muestra texto sin traducir y texto recortado. Cualquier compilación la acepta al iniciar: `--nsgo-lang=en-XA`.

## Semanas, horas y medidas

`i18n.Regional(cx)` indica cómo lee el usuario las semanas, las horas y las medidas, independientemente del idioma: el día en que comienzan las semanas, el formato de 12 o 24 horas, el sistema de medición y la unidad de temperatura, tal como los haya configurado en la plataforma o, de lo contrario, según su región. Los calendarios y selectores de fecha comienzan sus semanas allí y los campos de hora muestran esas horas a menos que la aplicación establezca `FirstDayOfWeek` o `HourCycle`, y las horas que escriben los mensajes de un catálogo siguen el formato horario del usuario.

## Las palabras propias de NSGo

Los componentes de NSGo expresan sus textos en los idiomas a los que NSGo está traducido. Para otro idioma, una clave que nombra un paquete, como `github.com/nsgo-dev/nsgo/base/locales`, proporciona los mensajes de ese paquete en el idioma del archivo. Los menús de las plataformas, y los roles y estados que anuncian los lectores de pantalla de Android y Windows donde el sistema no tiene una palabra propia, como una pestaña o un cuadro de búsqueda, usan las palabras de `github.com/nsgo-dev/nsgo/desktop/locales`.
