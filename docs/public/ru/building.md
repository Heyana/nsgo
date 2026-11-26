---
title: Сборка и распространение
description: Упаковка приложения для каждой платформы, создание установщиков, подпись и нотаризация, публикация на GitHub и встраивание в другие приложения.
group: Инструменты
order: 22
---

## Сборка

```sh
go tool nsgo build
```

Команда `build` создаёт очищенный от отладочной информации релизный пакет для каждой целевой платформы из списка в `nsgo.yaml` в каталоге `dist/<name>-<version>-<target>`: `.app`, `.ipa`, `.apk`, каталоги для Windows и Linux или веб-сайт, страница которого подгружает каждый TSX-модуль из `import()` в момент выполнения импорта. Флаг `-target` переопределяет список, причём `all` означает все целевые платформы, для которых достаточно Go и Xcode; добавьте `android_x64`, если установлен Android NDK, и `miniprogram` (или отдельно `miniprogram_wechat`, `miniprogram_qq`, `miniprogram_alipay`, `miniprogram_douyin`) для [мини-приложений](#мини-приложения), а также `harmonyos` для предварительной версии [HarmonyOS](#harmonyos). Флаг `-run` запускает пакет, собранный для текущей машины, либо устанавливает его на iPhone, симулятор или Android-устройство, выбранное через `-device`.

Всё готовое к распространению попадает в `dist/releases` — это единственный каталог, куда нужно заглядывать:

| В `dist/releases` | Назначение |
| --- | --- |
| `*.dmg`, `*-setup.exe`, `*.deb`, `*.pkg.tar.gz`, `*.AppImage` | [Установщики](#установщики) для macOS, Windows и Linux |
| `*.AppImage.zsync` | Данные для дельта-обновлений в AppImageUpdate и совместимых запусках |
| `*-ios.ipa`, `*-tvos.ipa`, `*.apk` | Пакеты iOS, tvOS и Android для магазинов или устройств |
| `*.pkg`, `*.msix`, `*.aab`, `*.flatpak`, `*.tar.gz` и `flatpak/` | [Пакеты для магазинов](#пакеты-для-магазинов) при сборке канала `store` |
| `install.sh`, `install.ps1` | Скрипты установки приложения из [релизов GitHub](#публикация-на-github) |
| `appcast-<target>.xml` | Ленты, которые считывает механизм [обновлений](#обновления) |
| `homebrew/<name>.rb`, `winget/` | [Манифесты Homebrew cask и winget](#каналы) для прямых релизов |
| `*-web.zip` | Готовый сайт — распакуйте и раздавайте как есть |
| `miniprogram/<platform>/` | Проекты мини-приложений для загрузки из инструментов разработки каждой платформы |

Каждый файл именуется по шаблону `<name>-<version>-<system>-<arch>`, например `hello-1.2.0-windows-x64-setup.exe`, чтобы пользователям было легко найти нужный файл на странице релиза, а рядом лежит его файл `.sha256`. Сборка другой версии сначала очищает этот каталог; сборки той же версии, в том числе на других машинах, добавляются в него. Пакеты для симуляторов и watchOS остаются в своих каталогах `dist/<name>-<version>-<target>`.

Веб-сайт включает `.wasm.br` и `.wasm.gz`, если они меньше исходного модуля. Включите раздачу предварительно сжатых файлов на хостингах, которые это поддерживают. Отдавайте их по исходному URL `.wasm` с заголовком `Content-Type: application/wasm`, соответствующим `Content-Encoding` и `Vary: Accept-Encoding`. Исходный модуль работает на любом статическом хостинге. Страница [Публикация в вебе](web.md) описывает семантические заголовки, браузерные ссылки, метаданные для отдельных страниц и загрузку через CDN.

### Офлайн-пакеты для веба

Укажите `web.offline: true` в `nsgo.yaml`, чтобы веб-релиз кэшировался при первом онлайн-запуске.

```yaml
web:
  offline: true
```

Раздавайте опубликованный каталог по HTTPS либо используйте `go tool nsgo build -target web -run` на localhost. Service worker кэширует страницу, WebAssembly, скрипты и упакованные ресурсы. При наличии сети открывается опубликованная версия. После успешной установки и закрытия вкладок предыдущего релиза свежий релиз сможет запускаться из кэша. Запросы к онлайн-сервисам самого приложения по-прежнему требуют подключения к сети.

## Терминал

Запустите приложение в интерактивном терминале командой `go tool nsgo dev -target tui`. Команда `go tool nsgo test -target tui` запускает его тесты там же через `nsgotest.Main` — по одному приложению на тестовый процесс. Размер задаётся терминалом: одна ячейка соответствует 8 на 16 пунктам макета. Выбирайте стили приложения явно; компоненты сохраняют свои обычные формы и размеры. В `examples/tui` показаны поля ввода, кнопки, горячие клавиши и изменение размера.

Соберите консольный исполняемый файл командой `go tool nsgo build -target tui_macos_arm64` или соответствующей `tui_macos_x64`, `tui_windows_arm64`, `tui_windows_x64`, `tui_linux_arm64` или `tui_linux_x64`. Группа сборки и проверки `tui` включает все шесть целей, как и `all`. Каждый каталог пакета содержит исполняемый файл и ресурсы без графического установщика. Все шесть вариантов собираются силами одного лишь Go.

Бэкенд поддерживает ячейки текста, фон, сплошные границы, обрезку, прокрутку, фокус и редактирование текста. Он использует шрифт и цвета терминала; на Mac клавиша `mod` также соответствует Control. Ctrl+C копирует текст внутри редактора и завершает приложение, если ни одна привязка и ни один редактор не перехватили сочетание. Вставка в скобках (`bracketed paste`) вставляет текст без запуска горячих клавиш, а буфер обмена является локальным для процесса. Холст, изображения, медиа, нативные встроенные компоненты, системные меню/оповещения, анимации, масштабирование, двунаправленное формирование текста, усечение в начале/середине строки и мост к системному буферу обмена пока не реализованы. `base/terminal` остаётся элементом управления терминалом, встраиваемым в GUI-приложение.

Пока `nsgo dev` владеет терминалом, диагностика инструмента и приложения записывается в `.nsgo/tui.log`. При выходе перед возвратом в оболочку восстанавливаются экран, курсор и режимы терминала. Скриншоты терминальных тестов растеризуют ячейки фиксированным переносимым шрифтом вместо захвата окна эмулятора; для символов Unicode, отсутствующих в этом шрифте, используйте текстовые проверки.

## nsgo.yaml

```yaml
name: Hello
id: dev.example.hello   # the bundle ID; without it, dev.nsgo. and the name's letters and digits
version: 0.1.0
build_number: 1         # CFBundleVersion and Android's versionCode; else from version
# copyright: © 2026 Example  # the Windows executable's details and installer, and the macOS About panel
# appearance: dark      # light or dark whatever the system's
# theme: theme.json     # a TSX app's theme: NSGo's or a shadcn/ui one
# i18n: { dir: locales, source: en }  # the app's catalogs and the language they are written in
icon:
  apple: assets/AppIcon.icon  # macOS, iOS and watchOS, or an .icns for macOS
  windows: assets/IconWindows.png  # optional Windows artwork
  linux: assets/IconLinux.png      # optional Linux artwork
  png: assets/Icon.png        # every other target
  tvos: assets/AppIconTV.brandassets  # the Apple TV's layered icon and top shelf art, or a wide PNG
  watchos: assets/AppIconWatch.icon   # the Apple Watch's own, drawn in circles, or a square PNG
  android:
    png: assets/IconAndroid.png  # optional bitmap; valid without adaptive layers
    foreground: assets/IconForeground.png  # Android's adaptive icon: 432 px, the logo within the center 264 px
    background: "#1F1F1F"                 # a color or a square PNG
    monochrome: assets/IconMonochrome.png  # optional: the shape Android 13's themed icons tint
dev:
  target: native        # native, web, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android, harmonyos or miniprogram_<platform>
  hot_reload: true      # false builds once, watching no files
  # icon: { png: assets/Icon-dev.png }  # replaces icon's files in dev's builds; defaults to assets/*-dev.*
build:
  output: dist
  tags: []              # Go build tags every build, test and check of the app takes, such as timetzdata
  ldflags: []           # the app's own linker flags, such as -X main.releaseDate=2026-11-17
  open: false           # true opens the output directory after building
  installers: true      # false leaves out the disk image, the Windows installer and the Linux packages
  jobs: 0               # how many targets build at once; 0 is half the CPUs, 1 one after another
  targets: [macos_arm64, macos_x64, windows_arm64, windows_x64, linux_arm64, linux_x64, ios, ios_simulator, tvos, tvos_simulator, watchos, watchos_simulator, android_arm64, web]
# apple: { sign_in_with_apple: true }  # the Sign in with Apple entitlement, for iOS, tvOS and Mac App Store builds
# apple: { purchases: true }  # StoreKit's library, which in-app purchases go through, in macOS, iOS and tvOS packages
# apple: { privacy: { accessed_apis: { NSPrivacyAccessedAPICategoryDiskSpace: [E174.1] }, tracking: false } }  # add required-reason APIs and tracking declarations to PrivacyInfo.xcprivacy
# apple: { requires_full_screen: false }  # let an iPad run the app in Split View, Slide Over and Stage Manager, in all four orientations
# apple: { orientations: [portrait, landscape_left] }  # the orientations a full-screen app takes on an iPad
# apple: { multiple_scenes: true }  # let an iPad open more than one window of the app, each showing the main view with its own state
# apple: { info: { UIBackgroundModes: [audio] } }  # background playback for iOS Picture in Picture
# android: { permissions: [REQUEST_INSTALL_PACKAGES] }  # manifest permissions beyond the runtime's
# android: { recording: true }  # the microphone service microphone.RecordWAV records in, with RECORD_AUDIO, the foreground-service permissions and POST_NOTIFICATIONS, which RecordWAV asks for so its notification shows from Android 13; a refusal still records
# android: { uses_cleartext_traffic: true }  # allow plain HTTP to any host in the APK and App Bundle
# android: { background_downloads: true }  # dataSync foreground service for cx.Download
# android: { background_motion: true }  # explicit finite motion shortService; streams also need a notification title
# android: { loopback_cleartext: true }  # allow plain HTTP to loopback in the APK and App Bundle
# commands: [{ name: hello-cli, package: ./cmd/hello-cli }]  # command-line programs the desktop packages ship beside the app: see Commands
# windows: { command_line: true }  # add <name>.com, a console companion that forwards arguments and streams to <name>.exe; not in store builds
# windows: { path: true }  # the installer puts the app's folder on the user's Path, so consoles find its commands
# windows: { publisher: Example Ltd }  # the company the Windows executable's details, its installer and the uninstall entry name; else windows.store.publisher_name
# linux: { maintainer: "Jane Doe <jane@example.com>" }  # the Linux packages' maintainer, else the app's name
# linux: { categories: [Office, Dictionary] }  # the freedesktop menu categories of the app's desktop entry, at least one a main category such as Office, Utility or Game, else menus file it under Other; your own start with X-
# linux: { share: packaging/share }  # files the .deb and Arch package install under /usr/share and the AppImage in its usr/share, such as metainfo/<id>.metainfo.xml or bash-completion/completions/<command>
# linux: { depends: { deb: [gstreamer1.0-plugins-good], arch: [gst-plugins-good] }, recommends: { deb: [gstreamer1.0-libav], arch: ["gst-libav: H.264 and AAC video"] } }  # packages the .deb and Arch package also depend on and recommend, after GTK 4 and WebKitGTK 6, each as dpkg or pacman writes it
# release: { github: me/hello }  # the GitHub repository the install scripts and updates install from, else the Git remote origin's; tag_prefix: v by default
# updates: { key: <public key> }  # how the app updates, by platform and channel: see Updates
# store: { dir: store }           # store listings directory (YAML and Markdown per language)
# miniprogram: { appid: { wechat: wx0123456789abcdef, alipay: "2021000000000000" }, custom_navigation: false }  # the mini programs'
```

Голое имя ОС в `targets` — или в `nsgo build -target` и `nsgo check -target` — означает её архитектуры (`windows` соответствует `windows_arm64` и `windows_x64`; `android` — только `android_arm64`, так как для x86_64 требуется NDK). Цель `macos_universal`, которую не включают `macos` и `all`, собирает обе архитектуры macOS и объединяет их в один пакет приложения, включая его библиотеки и виджеты. Канал Steam использует универсальный пакет приложения. Сборки с установщиками упаковывают его в один образ диска с одной лентой Sparkle в прямом канале. `nsgo test -target` также принимает имя архитектуры, для которой выполняются тесты: для настольной ОС — архитектуру текущей машины, а для Android — `arm64`, так что `windows_x64` запускает цель `windows` на 64-разрядной машине Windows, тогда как неподдерживаемая архитектура отклоняется с указанием правильного имени.

Флаги командной строки заменяют одноимённые значения из конфигурации, например `-target`, `-version` или `-output`; флаг `-h` выводит полный список.

`build.ldflags` содержит собственные флаги компоновщика для приложения, записанные так же, как их принимает `go build -ldflags`, например `-X main.releaseDate=2026-11-17` или `-X 'main.channel=beta builds'`. Их получает каждая сборка, компонующая приложение: `dev` с его перезагрузками, `build` (включая `-embed`) и `test` с `-target` на любой платформе. Флаг `-ldflags` в командах `build` и `test` дополняет этот список, а не заменяет его, и его можно указывать несколько раз. Пользовательские флаги идут перед собственными флагами `-X` инструмента, а компоновщик сохраняет последнее значение переменной, поэтому имя, идентификатор и версия в `core.App` остаются такими, как в `nsgo.yaml`. Инструмент передаёт `go` собственный `-ldflags`, который заменяет значение из `GOFLAGS`, поэтому флаги компоновщика следует указывать здесь; в headless-тесте `test` флаг `-ldflags` после имени каталога относится к `go test` и заменяет все остальные.

Каждая команда `go`, которую инструмент запускает для приложения, получает `build.tags`: в `dev` и его перезагрузках, `build` (включая `-embed`), `test` с `-target` и `check` на всех целевых платформах. К ним добавляются собственные теги NSGo, такие как `nsgodev` в `dev` или `nsgotest` в тестах на целевых платформах, а также теги из `-tags` в `GOFLAGS`, которые иначе были бы заменены; остальная часть `GOFLAGS` применяется без изменений.

`apple.info.LSMinimumSystemVersion` повышает минимальную версию ОС для пакета macOS, а `apple.info.MinimumOSVersion` — для выбранного пакета iOS, tvOS или watchOS. Указывайте версии в виде строк в кавычках, например `"14.2"` или `"18.0"`, из 1–3 числовых частей. Минимальные требования среды выполнения: macOS 13.0 (Go 1.27), iOS 15.0, tvOS 17.0 и watchOS 26.0; более низкие или некорректные значения вызывают ошибку. Мобильный ключ должен удовлетворять минимальной версии каждой выбранной мобильной платформы. Для App Clips сохраняется собственный параметр `app_clip.minimum`. Эти значения попадают в метаданные пакета, компиляцию ресурсов, проекты подписи и флаги компоновщика; встраиваемые фреймворки iOS также используют их (с Mac Catalyst не ниже 16.0 для среды выполнения macOS 13 в Go).

`icon.windows` и `icon.linux` выбирают квадратные PNG-файлы для пакетов приложений, оформления установщиков и логотипов в магазинах. По умолчанию используются `assets/IconWindows.png` и `assets/IconLinux.png`, а при их отсутствии — `icon.png`; `dev.icon` принимает те же настройки и файлы по умолчанию с суффиксом `-dev`. `icon.android.png` задаёт растровый значок Android независимо, в том числе когда адаптивные слои не настроены. Остальные платформы сохраняют общий запасной вариант `icon.png`.

Значок Apple TV представляет собой многослойную карточку, а не квадрат: `tvos` принимает `.brandassets` из Xcode со стеками `App Icon` и изображениями верхней полки, либо широкое изображение, например 1280×768, которое инструмент масштабирует в карточку поверх цвета её краёв и центрирует на изображениях верхней полки. Без обоих вариантов PNG размещается на три четверти высоты карточки. Apple Watch обрезает значок по кругу, поэтому `watchos` принимает `.icon`, нарисованный для кругов watchOS, либо квадратный PNG; без него приложение использует `apple`, а затем `png`. Оба варианта при отсутствии используют `assets/AppIconTV.*` и `assets/AppIconWatch.*`, как и `apple` с `png`.

На Android поля `icon.android.foreground` и `background` формируют адаптивный значок запуска, чтобы лаунчеры обрезали его под свою форму и анимировали вместо уменьшения `png` на подложке. Каждый слой представляет собой квадрат 108 dp (PNG размером 432 px); лаунчер отображает центральные 72 dp (288 px) и может обрезать всё за пределами центральных 66 dp (264 px), поэтому размещайте логотип внутри этого круга, а фон растягивайте на весь квадрат. `background` может быть PNG-файлом или цветом вроде `"#1F1F1F"`. `monochrome` — фигура, в которой учитывается только альфа-канал — окрашивается системой Android 13 и новее при включении тематических значков; без неё значок сохраняет свои исходные цвета. Пакет сохраняет `icon.android.png` (или `icon.png`) в качестве обычного растрового значка, который ни один лаунчер Android 9 или новее (минимальная версия для NSGo) не предпочтёт адаптивному; без `png` инструмент рисует его из переднего плана поверх фона. Без адаптивных слоёв лаунчер показывает растровый значок Android или общий `png`.

`android.uses_cleartext_traffic: true` разрешает незашифрованный HTTP к любому хосту в APK и App Bundle, устанавливая как флаг манифеста, так и политику сетевой безопасности. Без этого флага `android.loopback_cleartext: true` разрешает обычный HTTP только для `localhost`, `127.0.0.1` и `::1` — например, для видео, раздаваемого самим приложением. Политика только для loopback запрещает незашифрованный трафик к другим хостам; разрешение для всех хостов имеет приоритет, если оба параметра равны `true`. Для встраиваемого AAR политику сетевой безопасности Android настраивает хост-приложение.

Каждая сборка также принимает собственные флаги NSGo при запуске, например `--nsgo-fps` ([Запуск и отладка](debugging.md#флаги-любой-сборки)). В `nsgo dev` сочетание Command/Control+R перезагружает приложение из любого открытого окна и остаётся доступным после закрытия главного представления, если другое окно поддерживает работу функции приложения.

### Экран запуска

Задайте `launch_screen`, чтобы показывать непрозрачный фон, центрированный PNG или и то, и другое на iOS и Android 12 и новее. Пути к изображениям указываются относительно `nsgo.yaml`. Изображение пропорционально вписывается в область 128 × 128 пунктов или dp. Прозрачный PNG со стороной не менее 512 пикселей обеспечит чёткость на экранах высокой плотности.

```yaml
launch_screen:
  background: "#F5F1EA"
  image: assets/Launch.png
  dark:
    background: "#201C26"
    image: assets/LaunchDark.png
```

Система выбирает оформление сама, если оно не зафиксировано параметром `appearance`. Пропустите любое из полей `dark`, чтобы повторно использовать соответствующее значение светлой темы. Если фон или изображение не заданы, сохраняются стандартные значения платформы. Встроенные приложения получают экран запуска от хоста.

## Нативные классы Android

Перечислите предварительно скомпилированные классы Android в `nsgo.yaml` приложения, чтобы использовать собственный `View` через `core.Native`.

```yaml
android:
  classes: [java/BadgeView.jar, java/Helpers.dex]
```

Пути указываются относительно `nsgo.yaml`, если они не абсолютные. Каждый путь указывает на файл `.dex` или JAR-архив, содержащий корневой `classes.dex`, `classes2.dex` и последующие шарды от D8. Подготавливайте эти файлы с помощью D8 с флагом `--min-api 28`. На архитектуре arm64 NSGo собирает APK или App Bundle из этих предварительно скомпилированных файлов силами одного лишь Go. Сеансы разработки Android пересобираются при изменении любого из перечисленных файлов. Для встраиваемого AAR Java-зависимости предоставляет сборка Android хост-приложения. [Документация D8](https://developer.android.com/tools/d8)

В [примере нативного Android](https://github.com/nsgo-dev/nsgo/tree/main/examples/android-native) класс регистрируется с публичным конструктором `Context` и настраивается через Go-свойство.

## Игры

Установите `game: true` в `nsgo.yaml` для игры. Android классифицирует приложение как игру и применяет системные оптимизации Game Mode. macOS и iOS включают Game Mode, macOS по умолчанию выбирает категорию Games, а Linux добавляет `Game` в `.desktop`-файл. Явно заданные `macos.category` или ключ Game Mode в `apple.info` имеют приоритет.

Установите `gamepad: true`, если приложение поддерживает аппаратный геймпад. Android объявляет необязательными как геймпад, так и сенсорный экран. macOS, iOS и tvOS объявляют профиль Extended Gamepad, а tvOS также профиль Micro Gamepad для пульта Siri Remote; ключи контроллера в `apple.info` имеют приоритет.

## Уведомления о лицензиях

`nsgo build` помещает файл `THIRD_PARTY_NOTICES.txt` в ресурсы автономного пакета. Он содержит тексты лицензий модулей Go, используемых для данной целевой платформы, среды выполнения Go и встроенной среды выполнения Java или JavaScript в NSGo. В веб-сборках файл именуется по хешу содержимого, как и остальные ресурсы. Вызовите `core.ThirdPartyNotices()`, чтобы показать этот же текст на экране сведений о лицензиях внутри приложения. В режиме разработки, тестах и сборках встраиваемых библиотек эта функция возвращает пустую строку. [Объявления нативных библиотек](native-libraries.md) добавляют в эти же уведомления свои исходные тексты лицензий.

По умолчанию неизвестная или отсутствующая лицензия стороннего пакета прерывает сборку, а обнаруженная копилефт-лицензия вызывает предупреждение. Модули, указанные в рабочем пространстве Go приложения (`go.work`), рассматриваются как собственный код наряду с основным модулем: найденные в них файлы лицензий включаются в отчёт, а проверки сторонних зависимостей к ним не применяются. Установите для любого из этих правил значение `error` или `warn`. Список `allow` освобождает точно указанные пути модулей от обоих правил, сохраняя найденный при сборке текст лицензии.

```yaml
licenses:
  unknown: error
  copyleft: warn
  allow: [example.org/reviewed-module]
```

## Добавление библиотек

Добавьте библиотеку, то есть модуль Go, командой `nsgo add` — это `go get`, который сначала показывает свой план:

```sh
nsgo add github.com/acme/chart           # его новейший релиз, если go.mod его ещё не содержит
nsgo add github.com/acme/chart@v1.3.0    # эта версия, даже ниже той, что в go.mod
nsgo add -dry-run github.com/acme/chart  # вывести план и ничего не записывать
```

```text
github.com/acme/chart
  module     github.com/acme/chart v1.3.0, published 2026-11-20
  owner      github.com/acme, the first module from it in go.mod
  next       import "github.com/acme/chart"

example.com/hello
  go.mod     + require github.com/acme/chart v1.3.0 // indirect
             + require example.com/units v1.0.0 // indirect
             ~ require github.com/nsgo-dev/nsgo v0.4.0 => v0.4.2, a release
  go.sum     + 6 lines
  tools      go tool nsgo then runs v0.4.2, and so does an installed nsgo here
Apply these changes? [y/N]
```

Для каждого модуля план указывает его версию и дату публикации, а также владельца: хост, а на хостингах кода вроде github.com — ещё и учётную запись; отмечается, если в `go.mod` пока нет ни одного модуля этого владельца. Далее, если применимо, идут уведомления об отзыве (retract) и устаревании, версия без тега релиза, `replace` модуля и строка import его корневого пакета. Затем — изменяющиеся строки `go.mod`, включая `go` и `toolchain`, команды `go tool`, которые стали бы запускать другую версию, модули, загружаемые с их собственных хостов, и модули, чьи хеши `go.sum` принимает без проверки базой контрольных сумм. `go get` помечает модуль, который ещё не импортирует ни один пакет, как `// indirect`, а `go mod tidy` снимает пометку, как только какой-нибудь пакет его импортирует.

В терминале `add` спрашивает перед записью, а `-y` применяет план без вопроса; без терминала и без `-y` он ничего не записывает и завершается с кодом 1. Прерывание, например Ctrl+C, оставляет `go.mod` и `go.sum` как были и завершается с кодом 130. Модуль, который `go.mod` уже требует, остаётся как есть, а его версия выводится, если версия не указана явно.

Планирование не запускает ничего, кроме команды go, и работает с копиями `go.mod` и `go.sum`:

- с тулчейном, который модуль уже использует, и `GOTOOLCHAIN=local`, поэтому никакой другой тулчейн не загружается и не запускается;
- через прокси модулей, которые `GOPROXY` перечисляет до первого `direct`, а тот становится `off`, поэтому с собственных хостов загружаются только модули, помеченные `GOPRIVATE` или `GONOPROXY`, а `GONOSUMDB` и `GOAUTH` остаются как есть;
- с `GOWORK=off`, поэтому в рабочем пространстве `go.mod` планируется только для самого модуля, как его записывает `GOWORK=off go get`, и план об этом сообщает.

С `-y` или без него `add` отказывается:

- переводить требование NSGo на псевдоверсию или предварительный выпуск, как сделала бы библиотека, требующая коммит NSGo; `go get github.com/nsgo-dev/nsgo@<версия>` сначала осознанно переводит его туда;
- добавлять модуль, которому нужен более новый Go, чем тулчейн модуля; `go get go@<версия>` сначала поднимает его у модуля;
- понижать модуль через запрос вроде `@latest`; явно указанная версия понижает его, и план называет это понижением версии.

Он записывает `go.mod` и `go.sum`, только если ни один из них не изменился после планирования, каждый — через временный файл, переименовываемый поверх него, и восстанавливает оба при ошибке или прерывании. Для модуля с каталогом `vendor` план называет следующим шагом `go mod vendor`.

`add` появился после NSGo 0.3.4. В модуле, закрепившем 0.3.4 или более ранний релиз, установленный `nsgo` не запускает для этой команды `nsgo` модуля, а выводит `go get github.com/nsgo-dev/nsgo@<версия>`, который приносит `add`; там `go tool nsgo add` запускает закреплённый `nsgo`, который не знает `add`.

## Библиотеки из пакетов Go

Приложения, импортирующие `base/terminal`, также поставляются с `libghostty-vt`: пакеты Windows и Linux размещают библиотеку рядом с исполняемым файлом, а пакеты macOS и iOS подписывают её в своих фреймворках. На странице [Встраивание](embedding.md) описан сопутствующий XCFramework для хостов iOS.

Зависимость может объявить готовые динамические библиотеки для настольных платформ в `nsgo-natives.yaml`. NSGo проверяет их SHA-256, копирует в приложение перед подписью и включает в установщики; автору приложения не нужно выполнять никаких дополнительных шагов. На странице [Нативные библиотеки](native-libraries.md) описаны формат файла для автора пакета и пути загрузки.

## Команды

Приложение может поставлять программы командной строки рядом со своим настольным исполняемым файлом — например, CLI для работы с данными приложения. Перечислите их в разделе `commands`, каждую со своим главным пакетом Go:

```yaml
commands:
  - name: hello-cli             # the file's name, hello-cli.exe on Windows
    package: ./cmd/hello-cli    # a main package, built for each desktop target with CGO_ENABLED=0 and build.tags
    ldflags: [-X main.channel=beta]  # its own linker flags, after build.ldflags
windows:
  path: true                    # the installer puts the app's folder on the user's Path
```

Имя состоит из букв, цифр, `-` и `_` и не может совпадать без учёта регистра с исполняемым файлом приложения, его командой в Linux, `Uninstall`, `AppRun` или другой командой. `macos_universal` объединяет две сборки macOS каждой команды так же, как и сборки самого приложения.

Пакет компонуется с `build.ldflags` и `-ldflags`, затем с собственными `ldflags`, чьи значения `-X` побеждают, а затем с идентификационными данными приложения, поэтому `core.App()` передаёт команде те же имя, ID, версию и номер сборки из `nsgo.yaml`, что и самому приложению. Команда, не импортирующая `core`, принимает версию через `-X`, например `-ldflags "-X main.version=1.2.0"` в команде `build`, который применяется и к приложению, и к его командам.

| Пакет | Команды |
| --- | --- |
| macOS | В каталоге `Contents/MacOS`, подписываются до самого приложения в текущем виде как `<id>.<command>`: ad hoc или с `sign.identity`, усиленной защитой среды выполнения (`hardened runtime`) и меткой времени, чтобы приложение [проходило нотаризацию](#нотаризация), но без прав доступа (`entitlements`) приложения. Образ диска ничего не добавляет в `PATH`; [Homebrew cask](#каналы) создаёт ссылки на каждую команду, а пользователь может самостоятельно создать ссылку на `/Applications/<name>.app/Contents/MacOS/<command>` |
| Windows | `<command>.exe` рядом с `<name>.exe`, [подписанный](#подпись-пакетов-windows) тем же способом; установщик устанавливает его, а деинсталлятор удаляет; при `windows.path` установщик также добавляет папку приложения в пользовательский `Path`, как и `windows.command_line`, поэтому открытые после установки консоли находят эти команды. Команда, переименованная или удалённая в обновлении, остаётся в папке (как и любой файл, который приложение больше не поставляет), так как деинсталлятор удаляет только то, что установила его версия |
| Linux | В каталоге `/opt/<slug>` со ссылкой `/usr/bin/<command>`, принадлежащей пакету `.deb` или Arch, поэтому удаление пакета удаляет и её, а `dpkg` и `pacman` отказываются устанавливать поверх команды другого пакета; AppImage хранит команды рядом с приложением и ничего не добавляет в `PATH` |

Установщики, их контрольные суммы и ленты appcast [обновлений](#обновления) включают команды так же, как остальные файлы приложения, поэтому обновление заменяет их вместе с приложением. [Пакеты для магазинов](#пакеты-для-магазинов) исключают команды, поскольку песочница Mac App Store, MSIX и Flatpak запускают только собственный исполняемый файл приложения.

## Установщики

Для настольных целевых платформ команда `build` записывает в `dist/releases` пакеты для установки пользователями, каждый со своей контрольной суммой `.sha256`:

| Платформа | Установщик |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.dmg` — образ диска с приложением, стрелкой и ссылкой на «Программы» для перетаскивания, подписанный через `sign.identity` и прошедший [нотаризацию](#нотаризация) |
| Windows | `<name>-<version>-windows-<arch>-setup.exe` — нативный установщик Windows, устанавливающий приложение для текущего пользователя (или повторно в ранее выбранную им папку), закрывающий запущенное приложение, с ярлыком в меню «Пуск», необязательным ярлыком на рабочем столе и деинсталлятором, удаляющим только установленные им файлы, [подписанный с помощью `sign.certificate`](#подпись-пакетов-windows), где приложение подписывает пакеты Windows; `windows.command_line` также устанавливает компаньон `.com` и добавляет папку приложения в пользовательский `Path`, чтобы консоли, открытые после установки, запускали команду приложения, тогда как Win+R (также находящий компаньон) запускает приложение и закрывает открытую им консоль; деинсталлятор убирает папку из `Path` только если её добавил установщик, никогда не трогая уже существовавшую запись |
| Linux | `<name>-<version>-linux-<arch>.deb` для Debian и Ubuntu и `<name>-<version>-linux-<arch>.pkg.tar.gz` для Arch Linux, которые устанавливают приложение в `/opt/<slug>` с командой, ссылками на его [команды](#команды), файлом `.desktop` и значком (пакет Arch включает `.MTREE` для `pacman -Qkk`), а также `<name>-<version>-linux-<arch>.AppImage`, запускаемый в любом дистрибутиве без установки |

Установщик и деинсталлятор Windows собираются одним Go на любой системе и используют нативные диалоги Windows. Они общаются на тех языках из каталогов приложения, для которых у них есть переводы, включая упрощённый и традиционный китайский, автоматически выбирая язык интерфейса системы; `/LANG=<LCID>` выбирает язык для одного запуска, например `/LANG=2052` для упрощённого китайского. `/S` выполняет тихую установку или удаление, завершающий `/D=<folder>` задаёт папку (пробелы допускаются), а `/RELAUNCH` открывает приложение после обновления. Без `/D` обновление устанавливается в папку заменяемой установки. Флаг `-installers=false` или параметр `build.installers: false` собирает только сами пакеты без установщиков.

Установщики поддерживают собственные изображения и значки приложения; в противном случае используется значок приложения или встроенная картинка:

```yaml
macos:
  dmg_background: assets/DMGBackground.png  # the disk image's window, which takes its size in points
  dmg_icon: assets/DMGIcon.png              # the disk image's icon: an .icns or a square PNG
windows:
  installer_icon: assets/Setup.png          # the setup program's icon: an .ico or a square PNG
  uninstaller_icon: assets/Uninstall.png    # the uninstaller's
```

Фоновый PNG с парным файлом удвоенного размера, например `DMGBackground@2x.png`, остаётся чётким на экранах Retina. Значок приложения располагается на четверть ширины от левого края, папка Applications — на четверть от правого, оба с подписями, отцентрированными по вертикали, где встроенная картинка рисует стрелку; оставляйте для них место на собственном фоне. Пакеты Linux не имеют собственных картинок и устанавливают значок приложения.

Пакеты Linux также устанавливают метаданные AppStream `/usr/share/metainfo/<id>.metainfo.xml`, по которым центры приложений отображают программу, формируемые из `linux.summary`, `linux.license` (если задана) и названий приложения, с категорией `Utility`, если `linux.categories` не задаёт ни одной; AppImage хранит их в `usr/share` своего `AppDir` рядом со ссылкой на `.desktop`-файл в `usr/share/applications`, указанный в метаданных. Файлы, которые система читает из `/usr/share` (например, расширенные метаданные, дополнительные `.desktop`-файлы, автодополнение оболочки или страницы руководства `man`), помещаются в каталог, указанный в `linux.share` относительно `nsgo.yaml`: пакеты `.deb` и Arch устанавливают это дерево в `/usr/share`, а AppImage — в `usr/share` своего `AppDir`. Его файл `metainfo/<id>.metainfo.xml` заменяет тот, что формирует NSGo, а файл по пути, который пакет записывает сам (например, `.desktop`-файл или значок приложения), приводит к ошибке сборки. Файлы в `resources/` с любыми именами остаются рядом с исполняемым файлом в `/opt/<slug>`, а пакеты Flatpak для сборок в магазины используют только метаданные NSGo.

Для работы AppImage требуется FUSE, который есть в большинстве дистрибутивов, и системный GTK 4, как и для обычных пакетов; его среда выполнения однократно загружается в пользовательский кэш.

AppImage встраивает локатор `gh-releases-zsync`, используя `release.github` или удалённый репозиторий GitHub `origin`, выбирая последний стабильный релиз и архитектуру этого приложения. Загружайте каждый `.AppImage` вместе со сгенерированным для него файлом `.AppImage.zsync`. Для другого хостинга укажите в `linux.appimage_update` значение `zsync|https://example.com/latest.AppImage.zsync` — постоянный URL с поддержкой HTTP Range; также принимается формат `gh-releases-zsync|owner|repo|latest-pre|name-*-linux-x64.AppImage.zsync` для канала предварительных версий. Явная пустая строка отключает встроенное обнаружение; без репозитория GitHub или настроенного локатора оно также остаётся пустым. Файл `.zsync` при этом всё равно создаётся отдельно от подписанного appcast.

## Пакеты для магазинов

Флаг `-channel store` или параметр `updates.channel: store` собирает настольные и Android-приложения для соответствующих магазинов, которые сами занимаются обновлениями, вместо создания установщиков и установочных скриптов:

| Платформа | Пакет для магазина |
| --- | --- |
| macOS | `<name>-<version>-macos-<arch>.pkg` для Mac App Store: приложение в App Sandbox, подписанное удостоверением профиля подготовки (`provisioning profile`) и упакованное с удостоверением установщика вашей команды |
| Windows | `<name>-<version>-windows-<arch>.msix` для Microsoft Store, который сам подписывает пакет при публикации |
| Linux | Устанавливаемый `<name>-<version>-linux-<arch>.flatpak`, а также `.tar.gz` с AppStream и `flatpak/<id>.yml` для Flathub или `flatpak-builder` на базе среды выполнения GNOME |
| Android | `<name>-<version>-android-<arch>.aab` — App Bundle для Google Play, подписанный ключом загрузки `sign.keystore` |

MSIX считывает отображаемое имя для каждого языка из `appName` в каталогах локализации, с откатом к исходному языку. Логотипы для меню «Пуск», панели задач и Store включают варианты 100%, 125%, 150%, 200% и 400%. Инструмент формирует их индекс ресурсов прямо на macOS — Windows SDK не требуется.

В канале `store` флаг `-target macos` выбирает обе архитектуры macOS и создаёт один `<name>-<version>-macos-universal.pkg`, как и `-target macos_universal`. Выберите `-target android_arm64,android_x64`, чтобы получить один `<name>-<version>-android-universal.aab`, содержащий `arm64-v8a` и `x86_64`. Для среза `x86_64` требуется Android NDK.

```yaml
macos:
  category: public.app-category.productivity  # LSApplicationCategoryType, which the Mac App Store requires
  # entitlements: { com.apple.security.files.downloads.read-write: true }  # beyond the network, the files the user picks and what apple.info explains
windows:
  store:                               # as Partner Center's Product identity lists them
    identity: 12345Example.Hello
    publisher: CN=00000000-0000-0000-0000-000000000000
    publisher_name: Example
  # device_capabilities: [microphone, webcam]  # the devices the app uses, which Windows asks the user for
linux:
  summary: Says hello                  # one line for software centers
  license: MIT                         # an SPDX expression; LicenseRef-proprietary by default
  # flatpak_runtime: "51"              # GNOME's runtime version
android:
  target_sdk: 36                       # the targetSdkVersion Google Play requires
sign:
  macos_profile: assets/MacAppStore.provisionprofile  # the Mac App Store's profile
  # installer: "3rd Party Mac Developer Installer: Example (ABCDE12345)"  # by default the keychain's of the profile's team
```

Среда выполнения ориентируется на API 36, если `android.target_sdk` не задан. Сборка Android App Bundle завершается ошибкой при значении ниже API 36, который [требует Google Play](https://support.google.com/googleplay/android-developer/answer/11926878) для новых мобильных приложений и обновлений начиная с 31 августа 2026 года. Сборка отдельного APK выводит предупреждение и продолжается.

Задайте `sign.macos_profile`, чтобы подписать пакет Mac для загрузки. Без виджетов неподписанный пакет всё равно запускается в песочнице, что позволяет проверить доступ к контейнеру приложения, сети и файлам, выбираемым пользователем. Сборка для магазина с виджетами требует как `sign.macos_profile` для `<id>`, так и профиль macOS `sign.widgets_profile` для `<id>.widgets`, выданные одной командой и предоставляющие `apple.app_group` (по умолчанию `group.<id>`). Оба бандла используют этот общий контейнер; исключение для домашнего каталога не добавляется. `-installers=false` сохраняет песочницу и подписи профиля, пропуская `.pkg` и удостоверение установщика. Загрузите `.msix` в Partner Center, а `.aab` — в Play Console. Отправьте `flatpak/<id>.yml` в Flathub после того, как в релизе будут размещены перечисленные там архивы. `flatpak-builder` также собирает его локально. Установите пакет командой `flatpak install --user ./<name>-<version>-linux-<arch>.flatpak`; соответствующая среда выполнения GNOME должна быть установлена или доступна в настроенном удалённом репозитории. Сборки формируют пакет на Go без набора инструментов Linux. При отсутствии графики пакеты для магазинов используют прозрачный значок и категорию `Utility`, если не задан `linux.categories`. Для установки вне Microsoft Store укажите [`sign.certificate`](#подпись-пакетов-windows), чтобы подписать MSIX; субъект сертификата должен совпадать с `windows.store.publisher`, а принимающая машина должна доверять этому сертификату. Без настроек подписи загруженный пакет подписывает сам Store.

На macOS и iOS краевые эффекты области прокрутки (`Scroll.EdgeEffects`), размытие фона на macOS без `IOS.BackdropMaterial` и цветовые фильтры фона (самостоятельно или поверх размытия без `IOS.BackdropMaterial`) отрисовываются с использованием приватных классов Core Animation `CABackdropLayer` и `CAFilter` — так же, как собственные материалы и краевые эффекты AppKit и UIKit, поскольку публичного API для размытия по радиусу, переменного размытия по площади или цветовой матрицы фона не существует. Проверка App Store может отклонять приложения, использующие приватные API: NSGo вызывает их только для областей прокрутки с краевыми полосами и указанных фоновых фильтров, однако их имена присутствуют в каждой сборке macOS и iOS.

Каждый бандл Apple, создаваемый командой `build` — приложение, расширение виджетов, App Clip, приложение Apple Watch, а также динамический фреймворк `-dynamic` `-embed` — содержит манифест конфиденциальности `PrivacyInfo.xcprivacy` с объявлением API, требующих указания причины использования в NSGo и среде выполнения Go: user defaults (`CA92.1`), временные метки файлов (`C617.1`) и время загрузки системы (`35F9.1`). Поле `apple.privacy` добавляет собственные записи приложения: `accessed_apis`, причины для каждой категории, `tracking`, `tracking_domains` и `collected_data` — записи `NSPrivacyCollectedDataTypes` в терминологии Apple; файл `PrivacyInfo.xcprivacy` в `resources/` объединяется с ними, а не заменяется.

Файл `Info.plist` каждого бандла iOS, tvOS и watchOS, собираемого командой `build` для реальных устройств — приложения, его расширения виджетов и App Clip — содержит ключи SDK и Xcode `DT`, проверяемые App Store Connect (считываются из используемого для сборки Xcode), а также `UIRequiredDeviceCapabilities: [arm64]`; бандлы симуляторов опускают их, а ключи, заданные в `apple.info`, сохраняют свои значения. Параметр `apple.multiple_scenes: true` устанавливает `UIApplicationSupportsMultipleScenes` в манифесте сцен приложения iOS (по умолчанию `false`): на iPad пользователь сможет открывать несколько окон приложения, каждое из которых заново отображает главное представление с собственным состоянием и общими для всего приложения сигналами и хранилищем, пока пользователь его не закроет.

### Публикация в Google Play

Включите Google Play Developer API и выдайте сервисному аккаунту права на выпуск приложения в нужный канал (трек). Задайте переменной `NSGO_PLAY_SERVICE_ACCOUNT` JSON-ключ аккаунта или путь к файлу ключа. [Настройка Google Play API](https://developers.google.com/android-publisher/getting_started)

```sh
go tool nsgo build -target android -channel store
go tool nsgo publish -target android -track internal
```

Команда находит App Bundle для версии проекта в `build.output/releases`. При наличии нескольких сборок передайте путь к нужному `.aab` после флагов. Используйте `internal`, `production` или идентификатор трека закрытого тестирования. Команда считывает идентификатор пакета и код версии из бандла, требует, чтобы код был выше всех сборок на данном треке, затем загружает бандл и публикует релиз.

## Страницы в магазинах приложений

Метаданные для магазинов (заголовки, подзаголовки, описания, ключевые слова, заметки к выпуску, URL, скриншоты и превью) хранятся в репозитории в виде обычных текстовых файлов YAML и Markdown в каталоге `store/` (или `store.dir` в `nsgo.yaml`):

```text
store/
  listing.yaml              # version: 1, default_locale, app_store and google_play settings
  en-US/
    listing.yaml            # version: 1, title, subtitle, short_description, keywords, URLs, media
    description.md          # full description (optional file in place of YAML description)
    release_notes.md        # what's new (optional file in place of YAML release_notes)
    promotional_text.md     # App Store promotional text (optional file)
    screenshots/
  zh-Hans/
    listing.yaml
    description.md
    release_notes.md
    screenshots/
```

Оба файла `listing.yaml` требуют `version: 1` и отклоняют неизвестные ключи. Канонические каталоги локалей, такие как `zh-Hans`, автоматически сопоставляются с кодами каждого магазина (`zh-Hans` в App Store Connect, `zh-CN` в Google Play), если не переопределены для конкретного магазина через `app_store.locale` или `google_play.locale`.

```sh
go tool nsgo publish -store all -dry-run                  # validate lengths, locales and media without uploading
go tool nsgo publish -store appstore,play                 # validate and upload to App Store Connect and Google Play
go tool nsgo publish -store play -track production        # also update release notes on the named Play track
```

Проверка выполняется до чтения учётных данных и любых HTTP-запросов: проверяются обязательные поля, коды локалей, схемы URL, медиафайлы и их количество (до 10 скриншотов и 3 превью на каждый тип экрана App Store; до 8 скриншотов на каждый тип изображений Google Play), а также ограничения на длину строк (App Store: заголовок 30, подзаголовок 30, ключевые слова 100, рекламный текст 170, описание 4000, заметки к выпуску 4000; Google Play: заголовок 30, краткое описание 80, описание 4000, заметки к выпуску 500).

Учётные данные для загрузки берутся только из переменных окружения: `NSGO_APP_STORE_KEY`, `NSGO_APP_STORE_KEY_ID` и `NSGO_APP_STORE_ISSUER` (с откатом к ключу нотаризации `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` и `NSGO_NOTARY_ISSUER`) для App Store Connect и `NSGO_PLAY_SERVICE_ACCOUNT` (путь к JSON-файлу сервисного аккаунта или сам текст JSON) для Google Play.

### Соответствие требованиям магазинов

Используйте `nsgo.yaml` для отслеживания статуса каждой анкеты выпуска.

```yaml
compliance:
  app_store: { age_rating: false }
  google_play: { data_safety: false }
  steam: { content_survey: false }
```

После заполнения анкеты установите соответствующую запись в `true`. Соответствующая сборка `-channel store` или `-channel steam` напомнит о пунктах со значением `false`. Записи, которые вы не отслеживаете в YAML, можно опустить.

- Заполните [анкету возрастного рейтинга](https://developer.apple.com/help/app-store-connect/manage-app-information/set-an-app-age-rating) и [сведения о конфиденциальности приложения](https://developer.apple.com/help/app-store-connect/manage-app-information/manage-app-privacy) в App Store Connect.
- Для [анкеты по шифрованию](https://developer.apple.com/help/app-store-connect/manage-app-information/determine-and-upload-app-encryption-documentation) от Apple задайте в `apple.info.ITSAppUsesNonExemptEncryption` ответ вашего приложения в виде булева значения YAML. Если Apple выдала код одобрения, запишите его в `apple.info.ITSEncryptionExportComplianceCode` в виде строки. Можно пропустить эти ключи и ответить на вопросы при загрузке. Сборки для магазина Apple проверяют типы этих ключей и структуру манифеста конфиденциальности бандла.
- Заполните раздел [Безопасность данных](https://support.google.com/googleplay/android-developer/answer/10787469) в Play Console. Приложения, распространяемые исключительно на треке внутреннего тестирования, освобождаются от этого требования.
- Заполните [анкету о содержимом](https://partner.steamgames.com/doc/gettingstarted/contentsurvey) в Steamworks перед отправкой игры на проверку.
## App Store Connect

Создайте приложение в App Store Connect и подпишите IPA для устройства iOS профилем распространения App Store либо PKG для магазина macOS удостоверением установщика вашей команды. На Mac с Xcode задайте `NSGO_APP_STORE_KEY` (путь к файлу `.p8` ключа команды App Store Connect или его текст), `NSGO_APP_STORE_KEY_ID` (ID ключа) и `NSGO_APP_STORE_ISSUER` (ID издателя); без них используется ключ нотаризации (`NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID`, `NSGO_NOTARY_ISSUER`), как и для страниц в магазинах. Выдайте этому ключу доступ к приложению и право на загрузку сборок.

```sh
go tool nsgo publish ios path/to/Hello.ipa
go tool nsgo publish macos path/to/Hello.pkg
```

Перед загрузкой инструмент проверяет номер сборки бандла, а затем сообщает результат обработки Apple для TestFlight. Для следующей загрузки увеличивайте `build_number`. На macOS номера сборок монотонно растут между всеми версиями приложения, тогда как на iOS нумерацию можно начинать заново для новой версии. Отправку обработанной сборки на проверку App Review выполняйте отдельно в App Store Connect. [Руководство Apple по распространению](https://developer.apple.com/documentation/xcode/preparing-your-app-for-distribution)

## Пакеты ресурсов для магазинов

Объявляйте каталоги крупных ресурсов в `mobile.asset_packs`. Файлы внутри этих каталогов доставляются отдельно от базового Android App Bundle и приложения iOS. Имена пакетов начинаются со строчной буквы и могут содержать строчные буквы, цифры и символы подчёркивания.

```yaml
mobile:
  asset_packs:
    - {name: levels, dir: resources/levels, delivery: install-time}
    - {name: voices, dir: large-assets/voices, delivery: fast-follow}
```

Собирайте пакеты для Android командой `go tool nsgo build -channel store -target android_arm64`. Для тестирования бандла на устройстве используйте `bundletool build-apks --local-testing` и `bundletool install-apks`. NSGo открывает установленные пакеты `install-time`. Модули `fast-follow` включаются для доставки через Play, а чтение этих пакетов через Play Asset Delivery SDK пока находится в разработке.

На iOS команда `go tool nsgo build -target ios` отображает `install-time` на политику `essential` от Apple, а `fast-follow` — на `prefetch`, как при первой установке, так и при обновлениях. Xcode 26 или новее собирает загрузчик Background Assets. Используйте общую группу `apple.app_group` или значение по умолчанию `group.<id>`. Профиль приложения `sign.profile` и профиль расширения `<id>.assetdownloader` `sign.asset_packs_profile` оба должны включать эту группу. Загрузите файлы `.aar` из `dist/releases` вместе с IPA через процесс загрузки пакетов ресурсов в App Store Connect. [Создание управляемых пакетов ресурсов](https://developer.apple.com/documentation/backgroundassets/creating-managed-asset-packs)

Из горутины вызывайте `mobile.AssetPackReady(ctx, "levels")` для проверки готовности и `mobile.OpenAssetPackFile("levels", "intro.map")` для открытия файла относительно объявленного каталога. После использования закройте возвращённый ридер. [Локальное тестирование пакетов ресурсов Apple](https://developer.apple.com/documentation/backgroundassets/testing-asset-packs-locally)

## Депо Steam

```sh
go tool nsgo build -channel steam -target macos_universal,windows_x64,linux_x64
```

Задайте `updates.channel: steam`, чтобы использовать этот канал по умолчанию. Выберите целевые платформы macOS, Windows и Linux. Загрузите каждый каталог `dist/<name>-<version>-<target>` в соответствующее депо Steam, указав в качестве параметра запуска бандл приложения на macOS, исполняемый файл на Windows и Linux arm64 и `steam-launch.sh` на Linux x64. Обновлениями управляет сам Steam. Эти каталоги не содержат установщиков, системы самообновления или `steam_appid.txt`.

Депо для Linux x64 включают GTK 4 и его зависимости для Steam Linux Runtime 3.0 и 4.0. Скрипт запуска использует их только в том случае, если в системе нет GTK 4.14 или новее. Храните `nsgo-gtk/` рядом со скриптом запуска: в его подкаталоге `licenses/` находятся лицензии этих библиотек, исходные коды, из которых они были собраны, и скрипт сборки, что требуется по условиям LGPL для GLib, Pango и GTK. На Linux arm64 требуется наличие GTK 4 в среде выполнения.

Для локального тестирования вне клиента Steam задайте `steam: {app_id: 480}` в `nsgo.yaml`, заменив `480` на ваш Steamworks App ID. Команда `go tool nsgo dev` записывает этот ID в `steam_appid.txt` в своём каталоге нативной сборки и запускает приложение с этим каталогом в качестве рабочего. [Steamworks API](https://partner.steamgames.com/doc/api/steam_api), [SteamPipe](https://partner.steamgames.com/doc/sdk/uploading).

## Публикация на GitHub

Если сборка включает целевую платформу macOS, Windows или Linux, в `dist/releases` также появляются `install.sh` и `install.ps1`, которые устанавливают приложение из релизов GitHub-репозитория, указанного в `release.github`, либо репозитория из Git remote `origin` приложения; без обоих `build` не создаёт скрипты и сообщает об этом. Загрузите его файлы (верхний уровень без `miniprogram/`) в релиз с тегом `v<version>` (`release.tag_prefix` заменяет `v`; скрипты экранируют зарезервированные URL-символы в этом префиксе):

```sh
gh release create v1.2.0 dist/releases/*.*
```

После этого пользователи устанавливают или обновляют приложение одной строкой, которую выводит команда сборки:

```sh
curl -fsSL https://github.com/me/hello/releases/latest/download/install.sh | sh    # macOS and Linux
```

```powershell
irm https://github.com/me/hello/releases/latest/download/install.ps1 | iex         # Windows
```

На macOS скрипт копирует приложение из образа диска в `/Applications`, либо в `~/Applications`, если туда нет прав на запись; на Linux устанавливает пакет Arch через `pacman`, иначе пакет Debian через `apt-get` или `dpkg` с помощью `sudo`; на Windows тихо запускает установщик, который устанавливает приложение туда, где оно уже находилось. Перед установкой каждый скрипт сверяет файл с его контрольной суммой. Переменная `<NAME>_VERSION` (имя приложения заглавными буквами, например `HELLO_VERSION=1.1.0`) устанавливает другую версию, а `<NAME>_DOWNLOAD_URL` загружает файлы с зеркала.

## Публикация на itch.io

Укажите в `nsgo.yaml` поле `itch.project` со слагом `owner/game` вашей игры на itch.io. Чтобы переопределить названия каналов, задайте `itch.channels` по целевым платформам сборки. Значения по умолчанию: `html5` для веба, `osx-<arch>` для macOS, `windows-<arch>` для Windows и `linux-<arch>` для Linux.

```yaml
itch:
  project: example/my-game
  channels:
    macos_arm64: osx-arm64-beta
```

Задав секрет `BUTLER_API_KEY` в CI, соберите и загрузите целевые платформы.

```sh
go tool nsgo build -channel none -installers=false -target web,macos_arm64
go tool nsgo publish itch -target web,macos_arm64
```

Используйте одинаковые `-output` и `-version` в обеих командах, если переопределяете их. Загрузка берёт каталог пакета каждой целевой платформы и устанавливает её версию на itch.io равной версии сборки. NSGo использует `butler` из `PATH` либо скачивает закреплённую версию с itch.io. После первой загрузки веб-сборки выберите на itch.io тип игры HTML и отметьте загруженный файл как запускаемый в браузере. [Документация Butler](https://itch.io/docs/butler/pushing.html)

## Обновления

Параметр `updates` настраивает автоматическое обновление приложения так, как это принято на его платформе и в канале, из которого пользователи его получают:

```yaml
updates:
  key: zE0hx2FZwwHy6X7LbEP3GjTTE3KWfCAp5gsaBysC+vg=  # the public key nsgo keygen prints
  # channel: store                    # direct, store, steam or none; direct by default with a key, none without
  # feed: https://example.com/hello/  # where the appcasts and installers are; by default the latest GitHub release
  # automatic: false                  # check only when asked; true by default
  # automatic_downloads: true          # Sparkle downloads automatically; false by default, macOS only
```

`nsgo keygen` выводит пару ключей и не создаёт файлов: поместите открытый ключ в `updates.key` и закоммитьте его, а закрытый ключ храните вне репозитория — в менеджере паролей и в переменной `NSGO_UPDATES_KEY` там, где собираются релизы, например в секрете GitHub Actions. Каждая прямая сборка подписывает установщики закрытым ключом и записывает рядом `appcast-<target>.xml` — ленту в формате [Sparkle](https://sparkle-project.org), которую приложение читает по адресу `feed`: загружайте appcast вместе с установщиками. Без закрытого ключа `build` не создаёт appcast для macOS и Windows, поскольку те устанавливают только подписанные установщики, а приложения Linux и Android при чтении неподписанного appcast открывают страницу релиза вместо установки пакета.

Флаг `-release-notes notes.md` добавляет в каждый appcast файл Markdown с заметками к релизу, например раздел из списка изменений, отрендеренный в HTML без исходного сырого HTML из файла. Sparkle и WinSparkle показывают его в своих окнах обновления, а запрос на Linux и Android отображает его в виде текста под версией (до 1500 символов); приложения считывают его через `core.UpdateCheck.Notes`.

| Платформа | Как обновляется прямая сборка |
| --- | --- |
| macOS | Sparkle, который `build` встраивает в приложение, ежедневно проверяет обновления, предлагает новую версию, скачивает образ диска, проверяет его подпись, устанавливает и перезапускает приложение; стандартное меню приложения содержит пункт «Проверить обновления…», а приложение со своим меню добавляет его через `updater.Check` |
| Windows | WinSparkle, расположенный рядом с исполняемым файлом, делает то же самое на языке, который приложение отображает при запуске (если этот язык есть в WinSparkle), и тихо запускает установщик, который запускает новую версию |
| Linux, Android | Приложение читает свой appcast, и после того как пользователь выбирает «Обновить», поточно скачивает и проверяет подписанный пакет прямо на диск с отображением прогресса (`updater.Status` и индикатор выполнения на панели задач платформы), затем устанавливает его через `pkexec dpkg`, `pkexec pacman` (`.pkg.tar.gz`/`.pkg.tar.zst`), атомарную замену AppImage (с перезапуском на Linux) или установщик пакетов Android; для других форматов установки или при неподписанном appcast вместо этого открывается страница релиза |
| iOS | Приложение запрашивает App Store, который обновляет его, и предлагает открыть страницу приложения там |

Для опубликованного автономного веб-приложения `updater.Check(cx)` сравнивает запущенный релиз со свежим `nsgo.json` и после подтверждения предлагает перезагрузить страницу. Он также замечает изменённые файлы сборки под тем же номером версии. Ему не нужен ключ appcast; явные каналы `none`/`steam` по-прежнему отключают проверки, а встраивающие хосты сами управляют перезагрузкой. Мини-приложения используют менеджер обновлений своей платформы и перезапускаются только после того, как загруженное обновление готово. Эти целевые платформы выполняют проверку при вызове updater приложением; время проверки и загрузки мини-приложений определяет хост. Они не предоставляют сеттеров настройки автоматической проверки. Пользовательский обработчик `cx.CheckForUpdates` использует `UpdateCheck.Restart` для подготовленного обновления, метка версии которого может отсутствовать.

### Каналы

Канал определяет, кто обновляет приложение. Флаг `-channel` переопределяет `updates.channel` для одной сборки, поэтому один проект можно собирать для каждого канала:

| Канал | Для чего | Итоговые пакеты |
| --- | --- | --- |
| `direct` | GitHub, веб-сайт, Homebrew casks и winget, устанавливающие собственные файлы релиза | Обновляют сами себя, как описано выше |
| `store` | App Store, Mac App Store, Microsoft Store, Google Play, Flathub и другие магазины | Являются [пакетами для магазинов](#пакеты-для-магазинов) и не содержат встроенного средства обновления; iOS и macOS запрашивают свой App Store без ключа, а установка из Google Play может использовать нативный процесс обновления Play |
| `steam` | Steam | Создают [каталоги депо](#депо-steam), которые обновляет Steam |
| `none` | Дистрибутивы Linux и все, кто перепаковывает приложение | Ничего не проверяют |

Магазины отклоняют приложения, которые обновляют себя сами, поэтому загружайте сборку `store`, а не `direct`. Каждый прямой релиз с установщиками macOS или Windows также записывает Homebrew cask (`dist/releases/homebrew/<name>.rb`, объявляющий `auto_updates true`, контрольные суммы и URL загрузки для собранных образов диска, а также ссылки `binary` на каждую из [команд](#команды) приложения) и многофайловый манифест winget (`dist/releases/winget/<id>.yaml`, `<id>.installer.yaml` и `<id>.locale.en-US.yaml` для собранных установщиков NSIS). Везде, где система сообщает, кто установил приложение, приложение также проверяет это: сборка Android из другого магазина, Flatpak или Snap и сборка TestFlight передают обновления соответствующему магазину. Ручной вызов `updater.Check` в приложении из Google Play запрашивает у Play In-App Updates немедленный сценарий обновления, если он разрешён; согласие, загрузку, установку и перезапуск обрабатывает Play. Отладочные сборки никогда не попадают в этот сценарий. Автономные пакеты Android включают SDK в себя; [встраивающие хосты](embedding.md#android) подключают его через Gradle.

### Проверка из приложения

`updater.Check(cx)` проверяет обновления так же, как пункт меню *Check for Updates…*, и показывает найденное; `updater.CheckInBackground(cx)` показывает диалог только при наличии более новой версии, а `updater.CheckDaily(cx)` делает это не чаще раза в день. `updater.Status(cx)` считывает текущий прогресс загрузки и установки на Linux и Android. Когда `automatic` включён, приложение проверяет обновления таким образом на Linux, в прямых сборках Android и в сборках Apple App Store при запуске и каждом возвращении пользователя к приложению; неудачная проверка (например, без сети) повторяется через час. Отладочные сборки никогда не обновляются. В сборках Steam эти три функции ничего не делают.

`updater.CapabilityOf(cx)` сообщает без сетевого запроса, как обновляется данная копия: `ManagedSelf`, `ManagedPage`, `ManagedDevelopment`, `ManagedChannel`, `ManagedStore` (с именем магазина в `Store`) или `ManagedUnavailable`. Отладочная сборка, отключённый канал или установка из магазина выдают при проверке пояснение с конкретной причиной; поле `core.UpdateCheck.Err` зарезервировано для ошибок вроде повреждённой ленты или сбоя загрузки встроенного модуля обновлений.

Считывайте `updater.Status(cx)` внутри привязки, чтобы отслеживать `Capability`, `AutomaticChecks`, `AutomaticDownloads`, `Available`, `Notes` и `LastCheck`, а на Linux и Android — также `Phase`, `Loaded` и `Total` (`Fraction()` для индикатора выполнения). `updater.SetAutomaticChecks(cx, on)` сохраняет выбор пользователя и переключает ежедневный планировщик во время работы. Sparkle и WinSparkle сами хранят свои настройки; Linux, Android и iOS сохраняют выбор в приватном разделе хранилища. `updater.SetAutomaticDownloads(cx, on)` поддерживается только на macOS и по умолчанию берёт значение `updates.automatic_downloads`; остальные платформы возвращают `false` и игнорируют сеттер. Неподдерживаемые и управляемые извне сборки сообщают `false` для обеих настроек. WinSparkle передаёт метаданные найденного обновления, считывая appcast после срабатывания колбэка обнаружения; установкой по-прежнему управляет сам WinSparkle. Linux при запуске проверяет принадлежность к пакету Debian или Arch и статус AppImage; такие копии сообщают `ManagedSelf`, тогда как прочие неупакованные копии сообщают `ManagedPage` ещё до первой проверки.

## Запуск при входе в систему

`desktop.SetLaunchAtLogin(cx, on)` регистрирует приложение в механизме автозапуска платформы, а `desktop.LaunchAtLogin(cx, done)` считывает это состояние. MSIX, собранный текущим инструментом, автоматически включает отключённую задачу автозапуска; её включение учитывает переключатель «Приложения автозагрузки» пользователя и политики администратора. Обычные установки Windows и Linux используют параметр в ветке `Run` и запись автозапуска XDG соответственно. macOS использует нативную службу входа из бандла приложения.

Flatpak обращается к порталу Background, который может запросить согласие пользователя, не предоставляя доступа к файлам конфигурации хоста. Последующее чтение дожидается нативного ответа. У этого портала нет запроса статуса только для чтения: в Flatpak чтение возвращает только последний подтверждённый ответ за текущий запуск (`false` до получения ответа), не может обнаружить внешние изменения и не восстанавливает настройку при следующем запуске.

Упакованный внешним инструментом Snap объявляет имя файла автозапуска в описании приложения:

```yaml
apps:
  app:
    command: bin/app
    autostart: app.desktop
```

NSGo записывает файл с этим именем в `$SNAP_USER_DATA/.config/autostart`, откуда snapd запускает его через обёртку приложения. `desktop.LaunchedAtLogin(cx)` определяет запуск при входе в систему ещё до построения интерфейса, включая активацию `StartupTask` в MSIX.

## Типы файлов

`file_types` регистрирует документы, которые открывает приложение: из Finder, Dock и «Открыть с помощью» на macOS, из меню «Открыть с помощью» Проводника через установщик Windows или MSIX на Windows, через `.desktop`-файл пакетов Debian, Arch, AppImage и Flatpak на Linux и через «Открыть в» на iOS:

```yaml
file_types:
  - name: Flow File
    extensions: [flow]
    # id: dev.example.hello.flow   # <id>.<first extension> by default; another's ID imports its type
    # icon: assets/Flow.icns       # macOS document icon; or a square PNG
    # role: viewer                 # editor by default
    # conforms: [public.json]      # public.data by default
    # mime: [application/json]     # existing MIME types; otherwise inferred on Linux
```

Установщик Windows добавляет приложение в «Открыть с помощью», не заменяя выбор пользователя по умолчанию, и удаляет свои записи при обновлении и удалении. Linux определяет MIME-типы распространённых расширений по фиксированной таблице, а неизвестному расширению собственного типа приложения назначает `application/x-<type id>` с определением `shared-mime-info`. Вместо этого в `mime` можно указать существующие типы; для импортируемого типа, расширения которого нет в таблице, это необходимо. Сам запуск AppImage ничего не регистрирует: его запись и определения устанавливает инструмент интеграции с рабочим столом.

Приложение получает открываемые файлы через `desktop.OnOpenFiles`, включая последующий запуск на Windows или Linux при уже работающем приложении и открытие документов в сценах iOS. Файлы, которыми было запущено приложение, ожидают первый обработчик:

```go
desktop.OnOpenFiles(cx, func(paths []string) { open(paths[0]) })
```

На iOS обычные документы импортируются во временные файлы, принадлежащие приложению, с сохранением исходных имён. Чтение из провайдеров файлов координируется вне потока UI, а их области безопасности закрываются до запуска обработчика. Чтобы сохранить импортированный файл, переместите его в `cx.DataDir()`; изменения затрагивают только копию. На iOS используется значок документа приложения; параметр `icon` применяется на macOS. App Clips и пакеты tvOS не регистрируют типы документов. Открытие файлов остаётся отделённым от `OnOpenLinks`.

`desktop.AddRecentFile(cx, path)` запоминает документ, открытый пользователем: в меню Dock и «Файл › Открыть недавние» на macOS, в категории «Недавние» списка переходов (Jump List) на Windows и в недавно использованных файлах рабочего стола на Linux. Выбор элемента из этого списка передаётся тому же обработчику. `desktop.ClearRecentFiles` очищает записи приложения, сохраняя историю общих файлов других приложений на Linux; Linux также учитывает настройку истории в GTK.

`desktop.SetDockMenu(cx, items)` задаёт действия на значке приложения: меню Dock в macOS, задачи в Jump List в Windows и быстрый список (`quicklist`) в доке Ubuntu, Plasma и Dash to Dock в Linux, где поддерживаются быстрые списки. На Linux требуется установленный `.desktop`-файл, совпадающий с `id` приложения; поддерживаются подменю, разделители, отмеченные и отключённые пункты. Меню принадлежит области (`Scope`), которая его установила, и последний вызов в любом из окон приложения заменяет предыдущее меню.

## Ссылки

`url_schemes` добавляет приложению собственные схемы ссылок, например `myapp://settings`, которые открывают его из браузера, другого приложения или команд `open`, `start` и `xdg-open`. Приложения macOS, iOS и Android регистрируют их, как и установщик Windows, пакет MSIX и `.desktop`-файл Linux в пакетах Debian, Arch, AppImage и Flatpak. При удалении приложения эти регистрации удаляются.

```yaml
url_schemes: [myapp]
# app_links: [links.example.com]  # HTTPS hosts for the full app
```

Приложение получает такие ссылки через `desktop.OnOpenLinks`, в том числе во время работы: на Windows и Linux повторный запуск передаёт свою ссылку уже работающему приложению и завершается. Ссылка, запустившая приложение, ждёт регистрации первого обработчика. Составить такую ссылку может кто угодно, а на Windows и Linux обработчик получает любой URL, с которым было запущено приложение, поэтому проверяйте схему и запрашивайте подтверждение у пользователя, прежде чем ссылка изменит какие-либо данные:

```go
desktop.OnOpenLinks(cx, func(links []string) { show(links[0]) })
```

На iOS и Android ссылки при запуске и во время работы принимает `mobile.OnOpenLinks`. Открыть ссылку можно командой `xcrun simctl openurl booted myapp://settings` на симуляторе iOS или `adb shell am start -a android.intent.action.VIEW -d myapp://settings` на Android. В тестах ссылку доставляет `app.OpenLinks("myapp://settings")`.

Для универсальных ссылок iOS укажите явные HTTPS-хосты в `app_links: [links.example.com]`. Каждый хост должен быть доменным именем DNS без схемы, пути, порта, IP-адреса или подстановочных знаков; имена приводятся к нижнему регистру, а дубликаты отклоняются. Инструмент записывает `applinks:<host>` в право доступа Associated Domains основного приложения, в том числе при подписи под управлением Xcode; профиль IPA должен разрешать это право. App Clips сохраняют отдельные объявления `appclips:` из `app_clip.domains`.

Каждый хост должен отдавать файл `apple-app-site-association`, указывающий полное приложение по адресу `https://<host>/.well-known/apple-app-site-association`; одних лишь метаданных пакета недостаточно для авторизации универсальной ссылки. [Требования Apple к ассоциации доменов](https://developer.apple.com/documentation/xcode/supporting-associated-domains) описывают настройку сервера. Ad hoc подписи симулятора опускают ограниченные права разработчика, поэтому тест делегата не подтверждает серверную ассоциацию.

На iOS `mobile.OpenedURL(cx)` отслеживает последний вызов App Clip, универсальную ссылку или URL пользовательской схемы, включая исходное значение до монтирования корня. Доставка пользовательских схем по-прежнему поступает в `mobile.OnOpenLinks`; на момент запуска этого обработчика значение вызова уже актуально. Открытия файлов остаются в `OnOpenFiles` и не перезаписывают URL вызова. Поддержка вызова для Android и веба отслеживается отдельно в #1475.

## Входящие данные «Поделиться» на Android

Укажите MIME-типы, которые приложение принимает из системного меню «Поделиться» Android:

```yaml
android:
  share_types: [text/plain, 'image/*', application/pdf]
```

Сборки APK и App Bundle регистрируют эти типы как для одиночных, так и для множественных элементов. Зарегистрируйте [`mobile.OnShared`](services.md#открытие-ссылок-и-файлов), чтобы получать каждую отправку.

Для веб-приложения с адресами на основе путей задайте в `web.base` его каталог развёртывания (`/` или `/my-app/`) и настройте продакшен-сервер так, чтобы для маршрутов приложения отдавался `index.html`. Сервер разработки уже обеспечивает такой резервный ответ для HTML-запросов навигации. При перезагрузке глубокой ссылки пути к ресурсам остаются относительными к этому фиксированному HTML-элементу `base`. См. [Адреса приложения](addresses.md).

## Входящие данные «Поделиться» на iOS

Объявите универсальные идентификаторы типов (UTI), которые принимает расширение Share, указывая URL и текстовые типы перед более общими файловыми типами:

```yaml
apple:
  share_types: [public.url, public.plain-text, public.image, com.adobe.pdf]
  # app_group: group.com.example.app  # defaults to group.<id>
sign:
  # share_profile: assets/Share.mobileprovision
```

Команды `build` и `dev` упаковывают в приложение написанное на Go расширение Share `<id>.share`. Xcode подписывает его и регистрирует его App Group для запусков на устройстве. Для IPA включите ту же группу App Group у `<id>` и `<id>.share`; профили `sign.profile` и `sign.share_profile` должны предоставлять её. App Clips, tvOS и watchOS не имеют расширения Share.

Расширение копирует до 128 элементов и 512 МиБ файлов в контейнер App Group, после чего завершает работу листа «Поделиться». Обработчик `mobile.OnShared` получает эти пакеты данных, пока приложение активно или при следующем его открытии пользователем; они сохраняются, даже если расширение и приложение завершат работу до доставки. В iOS нет поддерживаемого API, позволяющего расширению Share вывести основное приложение на передний план или запустить его, поэтому такая передача не открывает приложение автоматически.

## Мини-приложения

```sh
go tool nsgo build -target miniprogram_wechat -run
go tool nsgo dev -target miniprogram_alipay
```

Каждая целевая платформа создаёт проект для инструмента разработки своей платформы (также в `dist/releases/miniprogram/<platform>`), который открывают флаги `-run` и `dev`; команда `dev` пересобирает проект при сохранении файлов, и инструмент автоматически перезагружает его:

| Цель | Платформа | Инструмент |
| --- | --- | --- |
| `miniprogram_wechat` | WeChat | [WeChat DevTools](https://developers.weixin.qq.com/miniprogram/dev/devtools/download.html) со включённым сервисным портом (Settings → Security) |
| `miniprogram_qq` | QQ | [Инструмент разработки QQ](https://q.qq.com/wiki/tools/devtool/) на macOS |
| `miniprogram_alipay` | Alipay | [Mini Program Studio от Alipay](https://opendocs.alipay.com/mini/ide/download) на macOS |
| `miniprogram_douyin` | Douyin и другие приложения ByteDance | [Инструмент разработки Douyin](https://developer.open-douyin.com/docs/resource/zh-CN/mini-app/develop/developer-instrument/download/developer-instrument-update-and-download) на macOS |

`miniprogram.appid` задаёт AppID мини-приложения для каждой платформы (`wechat`, `qq`, `alipay`, `douyin`); без него WeChat DevTools открывает предпросмотр с гостевым AppID, а остальные инструменты запрашивают его. В WeChat `-run` и каждая пересборка `dev` закрывают и заново открывают проект через CLI, чтобы DevTools скомпилировал свежие файлы. Это поведение проверено на DevTools 2.01.2510290. Навигационная панель платформы показывает заголовок приложения; с `custom_navigation` приложение занимает всю страницу, а его безопасная область удерживает интерфейс в стороне от кнопки меню. Приложение считывает ОС телефона с `Env.Simulated == true`, как и в вебе с `--nsgo-os`: страницы стека отображают собственный `Header`, так как системный заголовок их не заменяет. QQ, Alipay и Douyin пока не проверялись в своих инструментах, а Alipay ещё не передаёт приложению действие «Назад», ключевые кадры и высоту клавиатуры.

Мини-приложения находятся в экспериментальной стадии. Мини-приложение запускает только Go-приложения, но не TSX, а платформы ограничивают размер сжатого модуля приложения 2 МБ: приложение из нескольких экранов на `base` укладывается в лимит (как `examples/gallery-miniprogram`), а приложение с элементами управления `ui` — пока нет, и `build` сообщает о превышении. Шейдеры там ничего не рисуют, а веб-представления, видео, нативные компоненты и файлы шрифтов приложения пока не поддерживаются.

## HarmonyOS

```sh
go tool nsgo build -target harmonyos -run
```

`harmonyos` записывает `<name>-<version>-harmonyos.hap` в `dist/releases`. Движок работает как WebAssembly внутри ArkWeb, а хост на ArkTS отрисовывает монтируемое дерево с помощью представлений ArkUI, поэтому текст, поля ввода, прокрутка и диалоги являются нативными системными компонентами. Когда `hdc` доступен в `PATH`, `-run` устанавливает HAP на подключённое устройство или эмулятор и запускает его, а `nsgo test -target harmonyos` запускает тесты приложения из неподписанного HAP на эмуляторе под отдельным идентификатором бандла `.nsgotest`, удаляя тестовый HAP после завершения прогона, чтобы установленное приложение оставалось на месте.

Чтобы компилировать хост ArkTS из шаблонов и использовать `id` приложения в качестве имени бандла, добавьте `hvigorw` в `PATH` и задайте в `OHOS_BASE_SDK_HOME` путь к OpenHarmony SDK с API 26 (`26.0.0`) — целевой и сборочной версии SDK для шаблонов. `build.harmonyos_bundles` выбирает режим: `auto` (по умолчанию), `exact` (требует SDK) или `padded` (дополняет идентификатор до фиксированной длины закоммиченного хоста и не требует SDK). Тесты на целевой платформе используют ту же настройку и выводят, какой хост они упаковали. `harmonyos.permissions` перечисляет запрашиваемые приложением имена `ohos.permission`, такие как `MICROPHONE` и `CAMERA`, а `widget.title` задаёт подпись на карточке домашнего экрана.

Поддержка HarmonyOS находится в статусе предварительной версии. Эмуляторы умеют устанавливать неподписанные HAP. В [архитектурных заметках](https://github.com/nsgo-dev/nsgo/blob/main/docs/architecture/harmonyos.md) перечислены возможности ArkUI, которые уже отрисовывает предварительная версия.

### Подписанные пакеты HAP

Настройте `harmonyos.signing`, чтобы подписывать HAP материалами из AppGallery Connect. Для прямой установки на телефон Huawei требуются отладочный сертификат и профиль для этого устройства. Для распространения используйте релизные материалы. [Руководство Huawei по подписи](https://developer.huawei.com/consumer/en/doc/harmonyos-guides-V2/command-line-building-app-hap-hvigor-0000001553061909-V2/)

```yaml
harmonyos:
  signing:
    keystore: signing/app.p12
    key_alias: app
    certificate: signing/app.cer
    profile: signing/app.p7b
```

Пути указываются относительно `nsgo.yaml`. Задайте в окружении переменные `NSGO_HARMONY_KEYSTORE_PASSWORD` и `NSGO_HARMONY_KEY_PASSWORD`. Для подписи используются `hap-sign-tool.jar` из SDK и Java из `JAVA_HOME` или `PATH`. Сборка сохраняет точный `id` приложения для соответствия профилю, поэтому для подписи требуются SDK и `hvigorw` даже в режиме `auto`. Команда `go tool nsgo doctor`, запущенная в каталоге приложения, проверяет конфигурацию подписи и имя бандла в профиле без ввода паролей. `build -run` передаёт утилите `hdc` подписанный HAP.

## Подпись

```yaml
sign:
  identity: "Apple Development: you@example.com (ABCDE12345)"  # signs the macOS app, else ad hoc
  profile: assets/AppStore.mobileprovision  # signs the IPA
  keystore: assets/release.p12              # signs APKs, with key_alias
  certificate: assets/codesign.pfx          # signs the Windows app and its installer
  # widgets_profile: assets/Widgets.mobileprovision  # the iOS extension's profile; use a macOS profile for a Mac App Store build
  # share_profile: assets/Share.mobileprovision      # signs the IPA's share extension
  # clip_profile: assets/Clip.mobileprovision        # signs the IPA's App Clip
```

Пароли хранилища ключей передаются через `NSGO_KEYSTORE_PASSWORD` и `NSGO_KEY_PASSWORD`. Флаг `-team` подписывает пакет для iPhone. Флаги `-profile`, `-widgets-profile`, `-share-profile`, `-clip-profile` и `-macos-profile` переопределяют из командной строки профили подготовки для приложения, расширения виджетов, расширения Share, App Clip и Mac App Store. Флаг `-installer` переопределяет удостоверение Mac Installer Distribution.

### Подпись пакетов Windows

Authenticode подтверждает издателя загружаемого файла Windows; [SmartScreen проверяет его репутацию отдельно](https://learn.microsoft.com/windows/apps/package-and-deploy/smartscreen-reputation). Файл `.pfx`, указанный в `sign.certificate`, подписывает исполняемый файл, консольный компаньон, программы и библиотеки из ресурсов приложения, установщик вместе с деинсталлятором, а также MSIX, собранный для канала Store; пароль к нему берётся из `NSGO_CERTIFICATE_PASSWORD`. Подписи формируются средствами самого Go так же, как это делает `signtool`, поэтому Mac подписывает их без Windows SDK:

```yaml
sign:
  certificate: assets/codesign.pfx
  timestamp_url: http://timestamp.digicert.com  # the RFC 3161 server that timestamps them
```

`sign.timestamp_url` по умолчанию использует сервер DigiCert, а значение `none` оставляет подписи без метки времени, из-за чего они теряют силу по истечении срока действия сертификата. Если сертификат находится на аппаратном токене или в облачном сервисе, подписание выполняется вашей собственной командой, где `%1` обозначает подписываемый файл, и запускаемой для каждого из перечисленных выше файлов:

```yaml
sign:
  sign_command: "signtool sign /fd sha256 /tr http://timestamp.digicert.com /td sha256 /a %1"
```

Команда `build` останавливается с ошибкой, если команда подписи завершилась успешно, но оставила исполняемый файл приложения, консольный компаньон, ресурс PE или установщик без встроенной подписи Authenticode.

Деинсталлятор внутри установщика подписывается перед встраиванием тем же `sign.certificate` или `sign.sign_command`, что и приложение и установщик. Флаги `-certificate`, `-sign-command` и `-timestamp-url` задают эти параметры из командной строки.

### Нотаризация

Приложения, скачанные на Mac, открываются без предупреждения Gatekeeper после прохождения нотаризации в Apple. Подписывайте их сертификатом Developer ID и укажите профиль учётных данных, сохранённый `notarytool` в связке ключей:

```sh
xcrun notarytool store-credentials hello --apple-id you@example.com --team-id ABCDE12345
```

```yaml
sign:
  identity: "Developer ID Application: Example Inc (ABCDE12345)"
  notarize: hello   # the keychain profile above; or -notarize hello
```

После этого `build` подписывает приложение с `hardened runtime`, отправляет образ диска, ожидает вердикт Apple (несколько минут) и прикрепляет квитанцию (staple) к образу и приложению; при отказе выводится журнал Apple. В CI вместо профиля связки ключей используются переменные `NSGO_NOTARY_KEY`, `NSGO_NOTARY_KEY_ID` и `NSGO_NOTARY_ISSUER` — файл `.p8`, ID ключа и ID издателя API App Store Connect. Усиленная защита среды выполнения запрещает доступ к микрофону, камере, календарям и геолокации, если приложение явно не запрашивает эти права — `build` добавляет их автоматически, когда в `apple.info` присутствуют `NSMicrophoneUsageDescription`, `NSCameraUsageDescription`, описание использования календарей или напоминаний либо описание использования геолокации.

## Виджеты и Live Activities

`build` и `dev` автоматически компилируют расширение, отрисовывающее виджеты и Live Activities приложения, в пакеты macOS и iOS с помощью компилятора Swift из Xcode без отдельного шага. На iOS приложение и его виджеты используют общую группу приложения (`app group`): при `-run` на iPhone Xcode подписывает оба таргета и регистрирует группу; для IPA включите эту группу для обоих идентификаторов (`<id>` и `<id>.widgets`) и подпишите расширение профилем `sign.widgets_profile`. Автономные APK и AAB для Android включают провайдеры и ресурсы App Widget из `widgets`; предварительно собранная среда выполнения Java отрисовывает их временные шкалы без необходимости в Android SDK или компиляторе Java при сборке приложения. Android 16 отображает Live Activities как Live Updates, которым не требуется отдельное расширение. Подробности конфигурации и API описаны на страницах [Виджеты](widgets.md) и [Live Activities](live-activities.md).

## Android App Links

`app_links` перечисляет явные HTTPS-хосты, ссылки на которые открывают полное приложение Android:

```yaml
app_links: [example.com, shop.example.com]
```

APK и App Bundle объявляют для каждого хоста фильтр `ACTION_VIEW` с категорией `BROWSABLE` и флагом `autoVerify`. Используйте DNS-имена хостов без схемы, пути, порта, IP-адреса или подстановочных знаков. Разместите по адресу `https://<host>/.well-known/assetlinks.json` файл с идентификатором пакета приложения и отпечатком сертификата подписи; Android проверяет эту ассоциацию перед маршрутизацией ссылок в приложение. Инструмент не публикует файлы ассоциации; их настройка описана в [руководстве Android по проверке ссылок](https://developer.android.com/training/app-links/verify-applinks). Встраивающий Android-хост объявляет собственные фильтры намерений и перенаправляет принятые ссылки в свой `NsgoView`.

`mobile.OpenedURL(cx)` считывает принятый вызов ещё до первого рендеринга корня и обновляется при открытии новой ссылки уже запущенным приложением. При этом `mobile.OnOpenLinks` по-прежнему получает каждую принятую ссылку. Полностраничное веб-приложение считывает текущий адрес страницы через `mobile.OpenedURL`, включая переходы по истории браузера и изменения хеша; приложение, смонтированное внутри другой страницы, не читает адрес содержащей его страницы. Используйте [`cx.AppURL`](addresses.md), если приложению также требуется изменять свой адрес.

## App Clips

App Clip — это компактная часть iOS-приложения, запускаемая по ссылке, коду или метке NFC без установки основного приложения. Параметр `app_clip` собирает его внутри пакета iOS из собственного главного пакета, чтобы в него попадало только то, что он отображает:

```yaml
app_clip:
  main: ./clip              # the clip's main package, the app's own by default
  domains: [example.com]    # whose links open it
  # name: Hello Clip        # "<name> Clip" by default
  # id: dev.example.hello.Clip  # <id>.Clip by default
  # minimum: "17.0"         # 16.0 by default; 17.0 allows 100 MB when only links open it
  # notifications: true     # ask for notifications for a day
  # location: true          # let the clip confirm where it was opened
```

Клип считывает открывшую его ссылку через `mobile.OpenedURL` (значение обновляется при повторном открытии другой ссылкой), а `mobile.IsAppClip` сообщает, выполняется ли код внутри App Clip. Система загружает клип размером до 15 МБ (или до 100 МБ начиная с iOS 17, если он открывается только по ссылкам), и `build` отклоняет сборку большего размера; клип, использующий `ui`, занимает около 11 МБ. Команда `dev -target ios_simulator -clip https://example.com/order?table=12` запускает сам клип в симуляторе, открытый по указанной ссылке; `-target ios` запускает его на iPhone и выводит QR-код ссылки: добавьте ссылку в «Настройки» > «Разработчик» > Local Experiences и отсканируйте её камерой. Используйте `-release`, если отладочные символы выводят клип за предел размера для QR. Apple подписывает App Clips только для команд программы Apple Developer Program, а не для личных команд.

Команды `dev -target ios` и `build -run -target ios` также сохраняют клип внутри основного приложения: Xcode подписывает его как отдельный таргет и встраивает до подписи приложения. Автоматическая подпись требует авторизованной платной учётной записи команды в Xcode с профилями для приложения и клипа. Для App Store включите App Clips для `<id>`, подпишите клип профилем `sign.clip_profile` и раздавайте файл `apple-app-site-association` с указанных доменов. В репозитории пример `examples/appclip` моделирует кафе, чей клип оформляет заказ за столиком.

Чтобы передать заказ или другие локальные данные в основное приложение, задайте явную группу вместе с `app_clip`:

```yaml
apple:
  app_group: group.dev.example.hello
```

После этого клип и основное приложение делят `cx.DataDir()` и `cx.Storage()` в контейнере этой группы. Файлы в каталоге данных и записи «ключ–значение», сохранённые клипом, доступны при запуске основного приложения; `cx.LogsDir()` следует за этим каталогом, тогда как каждый бандл сохраняет собственный кэш. Подпись Xcode запрашивает группу для обоих таргетов; при использовании профилей вручную включите ту же группу для обоих App ID и выдайте её в `sign.profile` и `sign.clip_profile`. Отсутствие права доступа не даст подписать сборку, а недоступная группа остановит запуск. Явный каталог данных, заданный хостом, по-прежнему имеет приоритет.

Без `apple.app_group` каждый бандл использует собственное хранилище песочницы. Включение этой настройки не переносит данные, сохранённые в прежних песочницах, а приложения только с виджетами сохраняют прежнее расположение хранилища. Храните общие секреты в связке ключей, а не в обычном хранилище. Пример с кафе сохраняет заказ клипа и отображает его в основном приложении.

По явному действию пользователя внутри клипа вызовите `mobile.ShowAppClipOverlay(cx, mobile.AppClipOverlayBottom)`, чтобы попросить StoreKit порекомендовать соответствующее полное приложение. Значение `AppClipOverlayBottomRaised` оставляет место под нижнюю панель. Возвращённая функция скрывает этот запрос; очистка области (`Scope`) также скрывает его, причём более старый запрос не может закрыть более новый. Вызовы вне нативного App Clip на iOS ничего не делают. Решение о том, можно ли загрузить и показать рекомендацию, принимает App Store, поэтому вызов не сообщает ни о показе баннера, ни об установке. Сохраняйте сценарий клипа полностью работоспособным и без установки основного приложения. [Рекомендации Apple по баннеру App Clip](https://developer.apple.com/documentation/appclip/recommending-your-app-to-app-clip-users)

## Встраивание в другое приложение

Флаг `-embed` упаковывает приложение для вашего собственного приложения на iOS или Android: в XCFramework (с динамическими фреймворками при `-dynamic`) или AAR, отображаемый через `NSGoView` или `NsgoView`. Настольные Go-программы отображают приложения через `nsgo.NewView`, а веб-страницы — с помощью файла `nsgo.js` с сайта, хостом которого является текущая страница или хост, указанный в `web.host` в `nsgo.yaml`. Примеры `examples/embed-*` в репозитории демонстрируют каждый из вариантов; сторона хоста подробно описана на странице [Встраивание](embedding.md).
