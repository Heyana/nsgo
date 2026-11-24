---
title: Live Activities
description: Отслеживайте происходящее в реальном времени на экране блокировки и в Dynamic Island, а также в виде Live Updates на Android.
group: Возможности платформ
order: 48
---

## Включение

Live Activity отслеживает что-то по мере того, как оно происходит, — доставку или таймер: на экране блокировки и в Dynamic Island на iPhone начиная с iOS 16.1, а также как Live Update на Android 16 — в верхней части шторки и в виде значка в строке состояния; более ранние версии Android показывают постоянное уведомление. Включите их в `nsgo.yaml`:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Запуск, обновление и завершение

`widget.StartActivity` запускает активность определённого вида, который называет то, что она отслеживает, и записывается так же, как виды виджетов, и показывает `widget.ActivityContent`. `Activity.Update` заменяет отображаемое содержимое, `Activity.Alert` делает то же и при этом оповещает пользователя, а `Activity.End` завершает активность — с итоговым содержимым, если оно не nil, — и убирает её с экрана блокировки так, как указывает `Dismissal`. `widget.Activities` перечисляет запущенные активности приложения, включая активности предыдущих запусков приложения, а `widget.ActivitiesEnabled` сообщает, разрешают ли их пользователь и платформа.

```go
a, err := widget.StartActivity("delivery", widget.ActivityContent{
	Title: "Your order", Body: "On its way", Symbol: "box.truck.fill",
	Timer: arrival, ShowProgress: true, Progress: 0.4,
	Actions: []widget.ActivityAction{{ID: "pause", Title: "Pause", Symbol: "pause.fill"}},
})
if err != nil {
	return
}
a.Update(widget.ActivityContent{Title: "Your order", Body: "Arrived", Status: "Now"})
a.End(nil, widget.DismissAt(time.Now().Add(2*time.Hour)))
```

`StartActivity` возвращает `widget.ErrNoActivities` в приложении, собранном без них, `widget.ErrActivitiesUnsupported` на платформах, отличных от iOS 16.1 и Android, и `widget.ErrActivitiesDisabled`, когда пользователь их отключил; `Update` и `End` возвращают `widget.ErrActivityEnded` после завершения активности. Содержимое активности не превышает 4 КБ — столько принимают ActivityKit и APNs.

## Что она показывает

Обе платформы показывают заголовок и текст, статус или таймер, отсчитываемый системой, индикатор выполнения, до трёх действий и акцентный цвет. iOS рисует их с помощью расширения виджетов и принимает вместо них представления из `widget` для экрана блокировки и каждой части Dynamic Island: `LockScreen`, `Leading`, `Trailing`, `Center` и `Bottom` в развёрнутом виде, `CompactLeading` и `CompactTrailing` в компактном острове и `Minimal`, когда его делят несколько активностей. Android показывает только поля вместе со значком приложения, так как Live Updates не принимают представлений приложения.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` открывается вместе с приложением при нажатии на активность, `Stale` — момент, когда iOS показывает содержимое как устаревшее, а `Background` заполняет активность на экране блокировки.

## Нажатия

`widget.OnAction` запускает обработчик нажатия на действие или на `widget.Button`, передавая активность и ID действия (`Activity` равно nil для [кнопки виджета](widgets.md#buttons)). Задайте его в `main` до `nsgo.Run`: на iOS система запускает приложение в фоне для обработки нажатия, без его представлений, и удерживает активность, пока обработчик не вернёт управление, поэтому обработчик обновляет её именно там; на Android нажатие доходит до запущенного процесса Go через приёмник среды выполнения, не открывая его Activity. Для действий требуется, чтобы движок, который их опубликовал, продолжал работать; устаревшее действие ничего не делает после завершения этого движка. Само уведомление по-прежнему открывает приложение при нажатии. Обработчик выполняется в горутине системы, а не представлений приложения, к которым он обращается через `Post` объекта `core.Scope`.

```go
func main() {
	widget.OnAction(func(p widget.ActivityPress) {
		if p.Action == "pause" {
			p.Activity.Update(paused)
		}
	})
	nsgo.Run(Root)
}
```

Представление активности тоже может это делать на iOS 17 и новее: `widget.Button{Action: "pause"}` запускает тот же обработчик, не открывая приложение, и показывает лишь свои дочерние элементы там, где нажатие только открывает приложение.

## Push-обновления

Приложение в фоне обновляет свои активности только пока оно работает, поэтому приложение, отслеживающее происходящее в другом месте, запускает их с `widget.ActivityOptions{Push: true}`, отправляет на свой сервер токен, полученный через `widget.OnPushToken`, и сервер отправляет APNs-обновления `liveactivity`, у которых `content-state` — это результат `widget.ActivityState` для содержимого: изображения в push-уведомление не входят. Начиная с iOS 17.2 приложение также получает токен для запуска активностей через push с типом атрибутов `widget.ActivityAttributesType` и атрибутами вида и id. Для push-уведомлений нужно право push notifications в профиле приложения.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Ограничения

Система завершает активность через восемь часов. `widget.DismissDefault` оставляет завершённую активность на экране блокировки до четырёх часов на iOS и сразу убирает её на Android, `widget.DismissNow` убирает её сразу, а `widget.DismissAt` — в указанное время в пределах четырёх часов. Android сам запрашивает `POST_NOTIFICATIONS` и `POST_PROMOTED_NOTIFICATIONS`: сначала запросите разрешение у пользователя через `cx.RequestNotificationPermission` ([Уведомления](notifications.md)). Демонстрация Widgets в галерее и `examples/widgets` запускают такую активность.

## Далее

- [Виджеты](widgets.md): временные шкалы экрана «Домой» и экрана блокировки, отрисовываемые тем же расширением.
- [Уведомления](notifications.md): разрешение, которое Android запрашивает первым.
- [Сборка и распространение](building.md): расширение и необходимые ему права.
