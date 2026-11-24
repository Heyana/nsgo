---
title: Разрешения
description: Проверка и запрос системных разрешений, открытие их настроек и подсказки пользователю в Системных настройках macOS.
group: Возможности платформ
order: 49
---

## Проверка и запрос

Часть того, что может делать приложение, система оставляет на усмотрение пользователя: микрофон, камера, контакты, календари, напоминания, фотографии, Bluetooth, медиатека, геопозиция и уведомления на телефонах и Mac, движение и геопозиция в фоне на телефонах, а в macOS также Универсальный доступ, Запись экрана, Мониторинг ввода, Полный доступ к диску, Управление приложениями, Инструменты разработчика и Автоматизация другого приложения ([геопозиция и движение](services.md#location-and-motion), [уведомления](notifications.md)). `core.Permissions()` перечисляет их как ключи `core.Permission`. `cx.PermissionStatus` сообщает, есть ли у приложения разрешение, ничего не запрашивая, а `cx.RequestPermission` запрашивает его системным запросом, если такой запрос существует и ещё не показывался. Запрашивайте разрешение в момент, когда пользователь делает что-то, для чего оно нужно. Face ID и Touch ID к ним не относятся: о них сообщает `cx.BiometricAvailability`, а `cx.AuthenticateBiometric` запрашивает их собственным запросом ([биометрия](services.md#biometrics)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

В macOS Автоматизация выдаётся для каждого целевого приложения отдельно: передайте `core.PermissionAutomationOf("com.apple.finder")`, чтобы проверить или запросить её для запущенного целевого приложения по его bundle ID, или `core.PermissionAutomation`, чтобы открыть панель «Автоматизация» в Системных настройках. Для незапущенного целевого приложения ответом будет `PermissionUnknown`.

| Статус | `String()` | Значение |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Ещё не запрашивалось: запрос покажет системный диалог |
| `PermissionGranted` | `granted` | У приложения есть разрешение |
| `PermissionLimited` | `limited` | Частичный доступ, например к выбранным фотографиям или контактам |
| `PermissionDenied` | `denied` | Отклонено; изменить можно только в системных настройках |
| `PermissionUnknown` | `unknown` | Ограничено, но система или платформа не сообщает статус |
| `PermissionNotGated` | `not-gated` | Ничто не ограничивает приложение, как в Linux |
| `PermissionUnavailable` | `unavailable` | На платформе такого нет |

`Allowed` истинно для granted, limited и not gated. macOS сообщает, что Универсальный доступ, Запись экрана и Мониторинг ввода отклонены, пока пользователь их не включит, поскольку запрос лишь добавляет приложение в список на соответствующей панели, а о выдаче Записи экрана сообщает только после перезапуска приложения.

## Описания использования

В macOS и iOS для запроса нужно указать причину в Info.plist приложения, содержимое которого задаётся в `apple.info` файла `nsgo.yaml`; без неё запрос отвечает unavailable и записывает ключ в журнал, поскольку система завершила бы приложение. Сообщение из [каталогов](localization.md) приложения с тем же ключом, например `NSCameraUsageDescription: Scans the codes you point it at.` в `locales/en.yaml`, переводит причину, и система показывает её на языке пользователя; для языка без перевода используется исходный текст.

```yaml
apple:
  info:
    NSMicrophoneUsageDescription: Records your voice notes.
    NSCameraUsageDescription: Scans the codes you point it at.
    NSContactsUsageDescription: Finds friends who already use the app.
    NSCalendarsFullAccessUsageDescription: Adds your bookings to your calendar.  # macOS 14 and iOS 17; NSCalendarsUsageDescription before
    NSRemindersFullAccessUsageDescription: Turns tasks into reminders.            # NSRemindersUsageDescription before
    NSPhotoLibraryUsageDescription: Imports photos from your library.
    NSBluetoothAlwaysUsageDescription: Finds your headphones.
    NSAppleMusicUsageDescription: Plays your library.                            # iOS
    NSAppleEventsUsageDescription: Reveals files in Finder.                      # macOS Automation
android:
  permissions:
    - RECORD_AUDIO
    - CAMERA
    - READ_CALENDAR
    - WRITE_CALENDAR
    - BLUETOOTH_CONNECT      # Android 12+; BLUETOOTH on Android 9–11
    - READ_MEDIA_AUDIO       # Android 13+; READ_EXTERNAL_STORAGE on Android 9–12
windows:
  device_capabilities: [microphone, webcam]  # what a store build's MSIX declares
```

В Android каждое разрешение нужно указать в `android.permissions` файла `nsgo.yaml`; разрешение, отсутствующее в манифесте, отвечает `PermissionUnavailable`. В режиме `nsgo dev` macOS приписывает действия приложения терминалу, который его запустил, поэтому разрешение запрашивается у терминала и в списке оказывается он; чтобы проверить собственные разрешения приложения, соберите его.

## Открытие настроек

`cx.OpenPermissionSettings` открывает страницу, где выдаётся разрешение, и сообщает, открылась ли она: в macOS — раздел «Конфиденциальность и безопасность» на панели разрешения (включая Автоматизацию, Контакты, Фото и Службы геолокации) или «Уведомления»; в iOS — страницу приложения в Настройках (или его страницу уведомлений начиная с iOS 16); в Android — страницу приложения в Настройках; в Windows — страницу микрофона, камеры или уведомлений.

Для `PermissionLocation` и `PermissionBackgroundLocation` macOS открывает Службы геолокации; `PermissionMotion` открывает «Движение и фитнес», хотя macOS не поставляет данные о движении. iOS и Android для всех трёх открывают страницу приложения. Открытие настроек не запрашивает и не выдаёт разрешение.

## Подсказки пользователю

`desktop.GuidePermission` открывает эту страницу и ждёт, пока пользователь выдаст разрешение. В macOS за окном Системных настроек следует панель, не забирающая фокус, и для панелей, в список которых приложение перетаскивается, она держит приложение, чтобы пользователь перетащил его туда. Подсказка завершается, и её функция получает статус, когда приложение получает разрешение, пользователь закрывает панель или Системные настройки, вызывается `stop` или исчезает компонент; новая подсказка завершает предыдущую.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

Предлагаемое приложение — это его бандл, а если его нет, исполняемый файл; `GuideOptions.App` указывает другое приложение, а `Name` — как его называет панель. Windows открывает страницу и ждёт закрытия Параметров. На остальных платформах функция получает статус сразу.

## Платформы

| Платформа | Что сообщает и запрашивает |
| --- | --- |
| macOS | Все разрешения (Автоматизацию — для каждого запущенного целевого приложения через `PermissionAutomationOf`, а также уведомления для приложений в бандле); Автоматизация без цели, Управление приложениями, Инструменты разработчика и «Медиа и Apple Music» имеют статус unknown; движение недоступно |
| iOS | Микрофон, камера, контакты, календари, напоминания, фотографии (с `PermissionLimited`), Bluetooth, медиатека, геопозиция, движение и уведомления |
| Windows | Микрофон, камера и уведомления, для которых она никогда не показывает запрос; остальное не ограничено |
| Linux | Вне песочниц не ограничено; уведомления отражают, отвечает ли сервер уведомлений; во Flatpak и Snap камера — через портал рабочего стола, а микрофон — через портал и PipeWire |
| Веб | Микрофон и камера (unknown в Firefox), уведомления, геопозиция (без фоновой геопозиции) и движение в мобильных браузерах |
| Android | Микрофон, камера, календари, Bluetooth и медиатека, если они объявлены в `android.permissions` |
| Apple Watch, мини-программы | Уведомления, геопозиция и движение через их сервисы; остальное, что ограничивает система, пока имеет статус unknown |

Разрешения на геопозицию и движение следуют за [датчиками платформы](services.md#location-and-motion), а `PermissionNotifications` — за [уведомлениями](notifications.md). Фитнес использует `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` в Android, `NSMotionUsageDescription` на платформах Apple), в том числе для давления на Apple; необработанные данные движения в Android остаются отдельными. В мобильном Safari запрашивайте `PermissionMotion` из обработчика нажатия.

## Тестирование

`nsgotest` отвечает по своей модели: `app.Permissions` хранит статус каждого разрешения (изначально «не запрашивалось»), а запрос незапрошенного разрешения устанавливает для него значение из `app.PermissionAnswers`, а если его там нет — granted. `PermissionNotifications` синхронизировано с `app.NotificationPermission` и `app.NotificationAnswer`. `app.PermissionSettings` перечисляет открытые настройки, а `app.Guide` — показываемую подсказку, которую завершают `app.GrantPermission` и `app.CloseGuide`.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Далее

- [Уведомления](notifications.md): их собственное разрешение, счётчики и нажатия.
- [Сборка и распространение](building.md): entitlements и возможности, которые объявляет пакет.
