---
title: Встраивание
description: Встраивание приложения NSGo в собственное приложение для iOS, Android, десктопа или веба.
group: Инструменты
order: 23
---

## Сборка встраиваемого приложения

Встраиваемое приложение — это обычный пакет приложения с `Root` и `main`, собранный с флагом `-embed` для целевой платформы хоста. Каждое представление запускает приложение один раз и представляет собой собственный движок, поэтому хост может отображать сколько угодно представлений, каждое со своим состоянием.

```sh
go tool nsgo build -embed -target ios examples/embedded
go tool nsgo build -embed -target android_arm64 examples/embedded
go tool nsgo build -target web examples/embedded
```

Сборка для iOS создаёт `<Name>.xcframework` и `<Name>Resources`; сборка для Android создаёт `<Name>.aar`; веб-сборка создаёт каталог с `nsgo.json` и загрузчиком.

## iOS

Когда приложение импортирует `base/terminal`, сборка также создаёт `GhosttyVT.xcframework` рядом с фреймворком приложения. Добавьте этот сопутствующий фреймворк в целевой таргет хоста с параметром **Embed & Sign** как для статического фреймворка, так и для фреймворка с `-dynamic`. Он содержит библиотеки для устройства, симуляторов Intel и Apple silicon, а также Mac Catalyst; сохраняйте его имя `GhosttyVT.framework`.

Добавьте фреймворк приложения в таргет как статическую библиотеку (не встраивайте её, Do Not Embed), а каталог ресурсов — как ссылку на папку (folder reference), после чего выполните `import <Name>`. Вызовите `NSGoStart()` один раз при запуске приложения до создания первого представления. Код статической библиотеки является частью хост-приложения, поэтому манифест конфиденциальности хоста должен декларировать API с обязательным указанием причины (required-reason APIs), используемые NSGo: user defaults (`CA92.1`), метки времени файлов (`C617.1`) и время загрузки системы (`35F9.1`); фреймворк `-dynamic` несёт их в собственном `PrivacyInfo.xcprivacy`.

```swift
import Embedded
import UIKit

@main
final class AppDelegate: UIResponder, UIApplicationDelegate {
	func application(_ app: UIApplication, didFinishLaunchingWithOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
		NSGoStart()
		return true
	}
}

let view = NSGoView(frame: .zero)
view.params = ["user": "Ada"]              // cx.HostParams()
view.dataDirectory = documents.path        // where storage lives
view.localeIdentifier = "de-CH"            // in place of the system's
view.setFontScale(1.3)
view.overrideUserInterfaceStyle = .dark
view.tokens = ["accent": "#cf222e"]        // over the app's theme
view.handle("time") { _, reply in reply(Date().description, nil) }
view.send("message", ["text": "Hello"])    // cx.OnHostEvent("message", …)
view.onOpenURL = { url in router.open(url) }
view.onError = { message in log(message) }
view.register(component: "host.badge") { context in Badge(context) }
```

`NSGoView` запускает приложение при попадании в окно и останавливает его при освобождении памяти или вызове `stop()`. Параметры запуска задавайте до добавления в окно; остальные свойства можно менять в любой момент. Обработчики, компоненты и колбэки выполняются в главном потоке, а ответ может быть отправлен позже из любого потока. Контроллер представления может перенаправлять `preferredStatusBarStyle`, `prefersStatusBarHidden` и `prefersHomeIndicatorAutoHidden` представления, чтобы следовать его настройкам системных панелей.
Хост также может возвращать `preferredScreenEdgesDeferringSystemGestures` представления из своего контроллера для поддержки `mobile.DeferSystemGestures`.

## Android

Для arm64-приложения, импортирующего `base/terminal`, AAR включает `jni/arm64-v8a/libghostty-vt.so` рядом с `libnsgo.so`; в хосте должно быть включено извлечение нативных библиотек. Для пребилда Android и загрузчика терминала требуется [#1452](https://github.com/nsgo-dev/nsgo/issues/1452); библиотеки терминала для x86_64 пока недоступны.

Добавьте AAR в модуль приложения и настройте упаковку с извлечением нативных библиотек, чтобы среда выполнения могла их найти:

```kotlin
implementation(files("libs/Embedded.aar"))

android { packaging { jniLibs { useLegacyPackaging = true } } }
```

Создавайте экземпляры `dev.oneo.nsgo.NsgoView` в коде или в разметке. Каждый из них запускает приложение в собственном процессе при первом получении размера, а `stop()` завершает его. Передавайте результаты активности в каждое представление и обрабатывайте изменения конфигурации в активности, чтобы приложения не перезапускались.

```java
NsgoView app = new NsgoView(context);
app.setParams(Map.of("user", "Ada"));
app.setStorageDirectory(dir);
app.setLocale(Locale.forLanguageTag("de-CH"));
app.setFontScale(1.3f);
app.setAppearance(NsgoView.APPEARANCE_DARK);
app.setTokens(Map.of("accent", "#cf222e"), null);
app.handle("time", (data, reply) -> reply.resolve(now()));
app.handle("tapped", (data, reply) -> status.setText("tapped " + ((Map<?, ?>) data).get("count")));
app.setOpenUrlHandler(url -> router.open(url));
app.setErrorListener(message -> log(message));
app.registerComponent("host.badge", Badge::new);
```

```java
@Override
protected void onActivityResult(int request, int result, Intent data) {
	super.onActivityResult(request, result, data);
	for (NsgoView app : apps) app.activityResult(request, result, data);
}
```

Чтобы отсоединить нативное дерево представлений, сохранив Go-приложение работающим, удерживайте `NsgoEngine` в хосте и явно прикрепляйте к нему представление:

```java
NsgoEngine engine = new NsgoEngine(context.getApplicationContext());
app.attachToEngine(engine);
// Before taking the view off screen:
app.detachFromEngine();
// Reuse the same view later:
app.attachToEngine(engine);
// When the host ends the app:
engine.destroy();
```

Отсоединение освобождает нативные дочерние представления; повторное прикрепление восстанавливает текущие сигналы, ввод, фокус, позицию прокрутки и рисунки без повторного выполнения компонентов. Веб-страницы и плееры создаются заново из конфигурации приложения. Вызов `stop()` лишь отсоединяет явно переданный движок. Каждый движок по-прежнему привязан к своему первому представлению и своей Activity и отклоняет другое представление, поэтому обрабатывайте изменения конфигурации в этой Activity; её замена или показ независимых представлений в одном процессе пока не поддерживаются.

Для проверки обновлений через Google Play добавьте `com.google.android.play:app-update:2.1.0` в зависимости Gradle хоста Android. Сохраните классы AppUpdateManager, AppUpdateInfo, AppUpdateOptions и Tasks API, которые NSGo загружает по имени, с помощью [правил сжатия среды выполнения (shrink rules)](https://github.com/nsgo-dev/nsgo/blob/main/internal/platform/android/runtime/play-review.pro). Показанная выше пересылка `onActivityResult` также доставляет отмену обновления.

Для сеансов аутентификации Android во встраиваемом хосте объявите приватную схему обратного вызова на принимающей Activity и передавайте принимаемые ею ссылки в `view.openLink(uri.toString())` в `onNewIntent` (и в новом `onCreate`). Совпадающий активный сеанс потребляет свой обратный вызов; остальные ссылки попадают в `OnOpenLinks`. Отдельная `NsgoActivity` пересылает их самостоятельно.

## Десктоп

Go-хост запускает цикл событий платформы и помещает `Handle` каждого представления в своё окно. `nsgo.NewView` запускает приложение, когда дескриптор оказывается в окне, поэтому настройки, заданные до этого момента, действуют уже в первом кадре.

```go
channel := core.NewHost()
channel.SetParams(map[string]any{"user": "Ada"})
channel.Handle("time", func(c *core.HostCall) { c.Resolve(time.Now().Format("15:04:05")) })
channel.HandleOpenURL(func(url string, answer func(bool)) {
	answer(strings.HasPrefix(url, "https://example.com/orders/"))
})

view := nsgo.NewView(app.Root, nsgo.HostChannel(channel), nsgo.DataDir(dataDir))
defer view.Close()

view.SetTokens(core.Tokens{Values: map[string]string{"accent": "#cf222e"}})
view.SetLocale("de-CH")
view.SetFontScale(1.3)
view.SetAppearance(core.AppearanceDark)
handle := view.Handle() // an NSView, GtkWidget or child window
```

`view.Host()` отправляет события приложению и принимает его вызовы, а `nsgo.Component` регистрирует нативный компонент для платформы хоста.

## Веб

Веб-сборка представляет собой отдельную страницу, но другая страница может смонтировать её в свой элемент и выступить в роли хоста. `mount(element, url, options)` возвращает плеер, а `nsgo.json` указывает пути к загрузчику, wasm и ресурсам относительно самого себя.

```html
<div id="app"></div>
<script type="module">
const url = new URL("./Embedded-0.1.0-web/nsgo.json", location.href);
const config = await (await fetch(url)).json();
const { mount } = await import(new URL(config.loader, url));

const player = await mount(document.getElementById("app"), url.href, {
	config,
	params: { user: "Ada" },
	host: {
		call(name) {
			if (name === "time") return new Date().toLocaleTimeString();
			throw new Error(`no host function ${name}`);
		},
		post(name, data) { console.log(name, data); },
		openURL(url) { return url.startsWith("https://example.com/orders/"); },
	},
});
player.hostEvent("message", { text: "Hello from the page" });
</script>
```

Без опции `host` загрузчик импортирует модуль, указанный в `web.host` файла `nsgo.yaml`, и ожидает вызова его функции `createHost(player)`, если модуль её экспортирует. Приложение, встроенное в элемент или фрейм, оставляет управление историей и заголовком браузера самой странице.

Параметры `appearance` (`"light"`, `"dark"` или `"system"`), `locale` и `fontScale` подменяют настройки браузера аналогично настройкам нативного хоста, а методы плеера `setAppearance`, `setLocale` и `setFontScale` позволяют менять их позже. Параметр `module` — скомпилированный `WebAssembly.Module` или промис на него — позволяет странице, монтирующей приложение многократно, скачать и скомпилировать его один раз.

До запуска приложения загрузчик отображает элемент `progress.nsgo-load-progress`. Вы можете стилизовать его через CSS, передать `onProgress` в параметрах монтирования или экспортировать `onProgress` из `web.host` для собственной страницы приложения. Колбэк заменяет стандартную полосу загрузки и получает `{ loaded, total }` в декодированных байтах, где `total` равен `null`, если размер неизвестен. По завершении запуска загрузчик вызывает его с `null`, чтобы очистить индикатор. При передаче скомпилированного `module` прогресс загрузки пропускается.

```yaml
web:
  host: host.js
```

## Канал хоста

`core.Host` — это единый контракт, реализуемый каждым хостом: параметры запуска, события, отправляемые хостом, и функции хоста, вызываемые приложением. Значения передаются как в JSON, поэтому отображения превращаются в `map[string]any`, числа — в `float64`, и один и тот же код работает на Swift, Java, Go и JavaScript.

- `HostParams` однократно считывает параметры запуска при старте.
- `cx.OnHostEvent` слушает события из `Host.Send` и метода страницы `hostEvent`.
- `cx.CallHost` вызывает именованный обработчик и разрешается его ответом; `cx.PostHost` вызывает обработчик без ответа.
- `Host.HandleOpenURL` перехватывает `cx.OpenURL` до платформы, и его ответ определяет, откроет ли ссылку сама платформа.
- `Host.SetParams`, `Send`, `Handle` и `HandleDefault` безопасны для вызова из любой горутины.

На iOS хост задаёт `params`, регистрирует обработчики через `handle`, отправляет события через `send` и отвечает в `onOpenURL`. На Android он делает то же самое через `setParams`, `handle`, `send` и `setOpenUrlHandler`. На десктопе используются методы `core.Host`, а в вебе отвечает объект `host` на странице.

## Нативные компоненты

`core.Native` отображает представление, созданное хостом: его `Props` передаются в метод `SetProp` компонента хоста, генерируемые компонентом события попадают в `OnEvent`, а свойство, значением которого является анимированный узел, перемещается по тактовому генератору кадров без нагрузки на движок.

```go
core.Native{
	Component: "host.badge",
	PropsFunc: func() map[string]any { return map[string]any{"count": count.Get()} },
	OnEvent:   func(name string, data any) { if name == "tap" { count.Set(0) } },
	Style:     core.Style{MinWidth: core.Pt(120), MinHeight: core.Pt(28)},
}
```

Зарегистрируйте компонент в хосте: `app.register(component:)` и `NSGoComponent` на iOS, `registerComponent` и `NsgoComponent` на Android, `nsgo.Component` на десктопе. В вебе нативных компонентов нет, поэтому неизвестное имя там отображает пустой блок.

## Настройки

`tokens` переопределяет тему приложения по имени роли (`accent`, `inset`, `raised`) — для обоих оформлений или для одного из них; `cx.Palette()` считывает эти значения. Хост также задаёт локаль, размер шрифта, оформление и каталог данных, а также получает сообщения об ошибках приложения. Сборка `nsgo build -embed` не создаёт окон, меню и иконок в трее: они принадлежат хосту.

Внутренняя паника на горутине движка представления останавливает этот движок, уничтожает связанные с ним поверхности и передаёт панику и стек вызовов в обработчик ошибок хоста (`nsgo.OnError` в Go). Остальные движки продолжают работу; создайте новое представление для перезапуска упавшего приложения. Обычные паники в колбэках приложения по-прежнему сначала попадают в ближайший `core.Catch`.

`examples/embed-ios`, `examples/embed-android`, `examples/embed-desktop` и `examples/embed-web` — это одно и то же приложение с каждым типом хоста.

## Далее

- [Платформенные сервисы](services.md): что приложение запрашивает у платформы.
- [Сборка и публикация](building.md): пакеты, создаваемые `-embed`, и их подпись.
- [Пример: список задач](tutorials/todo.md): приложение, которое встраивает хост.
