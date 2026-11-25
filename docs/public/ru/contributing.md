---
title: Настройка окружения для разработки
description: Сборка NSGo из исходников, запуск галереи и тестирование изменений.
group: Участие в разработке
order: 80
---

## Требования

macOS с Xcode и Go 1.27.3 или новее собирает все целевые платформы под arm64, включая APK для Android. Для запуска на устройствах требуется то, что перечислено в разделе [Установка](getting-started.md#требования), а для целевой платформы эмулятора x86_64 Android, `android_x64`, нужен Android NDK, поскольку Go линкует android/amd64 только с cgo. Программы установки и удаления для Windows собираются одним Go.

```sh
git clone https://github.com/nsgo-dev/nsgo.git
cd nsgo
```

Перед изменением кода прочитайте [ARCHITECTURE.md](https://github.com/nsgo-dev/nsgo/blob/main/ARCHITECTURE.md) и [AGENTS.md](https://github.com/nsgo-dev/nsgo/blob/main/AGENTS.md).

## Галерея

В галерее есть демонстрация каждого элемента, компонента `base` и `ui`, а также каждого сервиса. В широком окне демонстрации сгруппированы по вкладкам в боковой панели; в узком вкладки и настройки отображаются на нижней панели, а демонстрации открываются поверх них. Запускайте галерею из корня репозитория:

```sh
go tool nsgo dev examples/gallery                   # nsgo.yaml's dev target, exiting with the app
go tool nsgo dev examples/gallery-tsx               # the gallery's components and apps, written in TSX
go tool nsgo dev -target web examples/gallery       # in the browser, serving until interrupted
go tool nsgo dev -target ios_simulator examples/gallery
go tool nsgo dev -target ios examples/gallery       # on the connected iPhone, signed by Xcode
go tool nsgo dev -target android examples/gallery   # on the connected device or emulator, over adb
go tool nsgo build -target all examples/gallery     # every platform's packages in examples/gallery/dist, what to hand out in dist/releases
```

## Проверка и тестирование

```sh
go tool nsgo check                 # vet every package
go tool nsgo check -target all     # vet every available Go/Xcode target instead
go tool nsgo test                  # every package's tests
go tool nsgo test -bench . ./core  # core's benchmarks
(cd benchmark && go run ./runner)  # measure against other frameworks
# capacity curve for NSGo's 600-cell grid, without rebuilding it
(cd benchmark && go run ./runner -frameworks nsgo -cases grid -grid-cols 20 -grid-rows 15 -out /tmp/grid.json)
```

`-target` проверяет с помощью `vet` указанные целевые платформы вместо хоста, а в репозитории NSGo — ещё и dev-клиент, который проверяет обычный `check`. Чтобы проверить платформы, затронутые изменением, перечислите их все в одной команде, например `check -target web,android -jobs 2`, вместо запуска нескольких команд подряд: `-jobs n` проверяет до n целевых платформ одновременно, каждая команда `go vet` использует n рабочих компиляции, тогда как по умолчанию проверяется по одной платформе за раз со стандартными настройками Go; если запустить `check`, пока в том же модуле выполняется другой, он дождётся завершения первого и сообщит об этом. В CI рабочий процесс `check.yml` проверяет все платформы.

При запуске в корне репозитория или в `internal/dev` безголовая команда тестирует `internal/dev` повторно с тегом сборки `nsgodev`, который нужен файлам dev-клиента, поэтому его тесты выполняются и локально, и в CI. Оба запуска принимают одинаковые фильтры и флаги и формируют общий итог и JUnit-отчёт, где пакет второго запуска называется `internal/dev [nsgodev]`.

`check -target all` проверяет платформы macOS, Windows, Linux, Android arm64 и веб на любом хосте. Она также проверяет iOS, tvOS и watchOS при наличии соответствующих SDK Xcode и перечисляет пропущенные. Явно указанная платформа, например `check -target ios`, завершается ошибкой, если её инструменты недоступны — в том числе в сочетании с `all`. Сбой на одной платформе не останавливает проверку остальных, а сама команда завершается с ошибкой, если не прошла любая из обязательных проверок. Для `build -target all` по-прежнему требуется macOS с Xcode. В самом репозитории NSGo для каждой указанной платформы также проверяются её тестовые хосты с `nsgotest`; в CI предварительная платформа `harmonyos` указывается явно, поскольку она не входит в `all`.

Задача Linux в CI запускает тесты Go-бэкенда HarmonyOS для `js/wasm` с помощью Node.js и загрузчика Go Wasm. Локально тест галереи запускается с подключённым через `hdc` эмулятором HarmonyOS. Для компиляции [шаблонов хоста HarmonyOS](building.md#harmonyos) требуется OpenHarmony SDK API 26.

```sh
go tool nsgo test -env GOOS=js -env GOARCH=wasm ./internal/platform/harmonyos \
  -- "-exec=env -i \"PATH=$PATH\" \"$(go env GOROOT)/lib/wasm/go_js_wasm_exec\"" -count=1
go tool nsgo test -target harmonyos -run TestHarmonyOSPages -timeout 10m ./examples/gallery
```

В сессии Wayland на десктопе Linux можно выполнить реальную приёмочную проверку портала глобальных горячих клавиш: `NSGO_TEST_GLOBAL_SHORTCUT_PORTAL=<installed-desktop-id> go tool nsgo test -run TestGlobalShortcutPortalWithPermission ./internal/platform/linux`. Указанный ID должен иметь `.desktop`-файл. Разрешите Ctrl+Alt+K в запросе композитора, а затем нажмите эту комбинацию, когда фокус находится в другом приложении; проверка ждёт как регистрации, так и активации. Без этой переменной окружения тест пропускается.

При наличии Docker на хосте Linux команда `go tool nsgo test -run '^TestLinuxPackagesNative$' -env NSGO_TEST_LINUX_PACKAGES=1 ./cmd/nsgo -- -timeout=20m` проверяет реальную установку пакетов в одноразовых контейнерах Debian, Ubuntu и Arch. Она скачивает их официальные образы, устанавливает внутри них зависимости GTK, проверяет базу пакетов и установленные файлы, после чего удаляет пакет. База пакетов самого хоста не затрагивается.

## Релизы

Релизы NSGo выпускаются по мере необходимости — когда зависимому проекту нужна версия с тегом или у мейнтейнеров готовы изменения для публикации. Запрашивая релиз для библиотеки, дайте ссылку на зависимую работу и назовите коммит, содержащий последнее нужное изменение NSGo. Первый релизный тег, включающий этот коммит, станет версией, которую сможет указать библиотека.

Чтобы протестировать смерженное изменение до выпуска тега, выполните `go get github.com/nsgo-dev/nsgo@main` в зависимом модуле. Вместо `main` можно указать идентификатор конкретного проверенного коммита. Go запишет полученную псевдоверсию в `go.mod`.

Мейнтейнер запускает `go run ./tools/changelog release X.Y.Z`, который присваивает изменениям версию и дату в `CHANGELOG.md` и оставляет новый раздел Unreleased, сливает это в `main`, а затем запускает [рабочий процесс релиза](https://github.com/nsgo-dev/nsgo/blob/main/.github/workflows/release.yml) с этой версией: `gh workflow run release.yml -f version=X.Y.Z`. Рабочий процесс выполняет все проверки и тесты на последнем коммите `main` и только после их прохождения ставит на этот коммит тег `vX.Y.Z` и публикует релиз, поэтому неудача не расходует номер версии; опубликованные примечания затем переносятся из `CHANGELOG.md` (где остаются только невыпущенные изменения) в раздел [Релизы](https://github.com/nsgo-dev/nsgo/releases).

## NSH

NSGo использует [Native Shell](https://github.com/longbridge/nsh) в качестве ориентира архитектуры; ни одному из его тестов этот репозиторий не нужен. Чтобы изучить его код, клонируйте его рядом с репозиторием и переключитесь на последний релизный тег, но не на `main`:

```sh
git clone https://github.com/longbridge/nsh.git ../nsh
git -C ../nsh checkout "$(git -C ../nsh describe --tags --abbrev=0)"
```

## Далее

- [Кодовая база](contributing/codebase.md): где расположена каждая часть NSGo.
- [Внесение изменений](contributing/changes.md): требования к pull request.
