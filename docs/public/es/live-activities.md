---
title: Actividades en vivo
description: Sigue algo en tiempo real en la pantalla de bloqueo y en la Dynamic Island, y como Live Updates en Android.
group: Funciones de plataforma
order: 48
---

## Activarlas

Una Actividad en vivo sigue algo mientras sucede, un envío o un temporizador: en la pantalla de bloqueo y en la Dynamic Island de los iPhone desde iOS 16.1, y como una Live Update en Android 16, en la parte superior del panel de notificaciones y en un chip de la barra de estado; versiones anteriores de Android muestran una notificación en curso. Actívalas en `nsgo.yaml`:

```yaml
live_activities:
  enabled: true
  # frequent_updates: true   # iOS: more updates than the system's budget, as the user allows
```

## Iniciar, actualizar y finalizar

`widget.StartActivity` inicia una de un tipo (`kind`), que identifica lo que sigue y se define igual que los tipos de widgets, mostrando un `widget.ActivityContent`. `Activity.Update` reemplaza lo que muestra, `Activity.Alert` hace lo mismo y alerta al usuario, y `Activity.End` la finaliza, con contenido final cuando no es `nil`, y sale de la pantalla de bloqueo según indique `Dismissal`. `widget.Activities` lista las actividades activas de la aplicación, incluidas las de ejecuciones anteriores de la aplicación, y `widget.ActivitiesEnabled` indica si el usuario y la plataforma las permiten.

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

`StartActivity` devuelve `widget.ErrNoActivities` en una aplicación compilada sin ellas, `widget.ErrActivitiesUnsupported` fuera de iOS 16.1 y Android, y `widget.ErrActivitiesDisabled` cuando el usuario las desactivó; `Update` y `End` devuelven `widget.ErrActivityEnded` una vez finalizada. El contenido de una actividad tiene un máximo de 4 KB, según exigen ActivityKit y APNs.

## Qué muestra

Ambas plataformas muestran el título y el cuerpo, el estado o un temporizador que el sistema hace avanzar, una barra de progreso, hasta tres acciones y el color de acento. iOS las dibuja con la extensión de widget y acepta vistas de `widget` en su lugar para la pantalla de bloqueo y cada parte de la Dynamic Island: `LockScreen`, `Leading`, `Trailing`, `Center` y `Bottom` en vista expandida, `CompactLeading` y `CompactTrailing` en la isla compacta, y `Minimal` cuando varias actividades la comparten. Android muestra solo los campos, junto con el icono de la aplicación, ya que las Live Updates no admiten vistas de la aplicación.

```go
widget.ActivityContent{
	Title: "Order 412", Body: "On its way", Status: "8 min", Symbol: "box.truck.fill",
	Tint: widget.RGB(0x30d158), ShowProgress: true, Progress: 0.4,
	Trailing: widget.Text{Content: "8 min", Style: widget.Style{FontSize: 34, FontWeight: core.FontWeightBold}},
}
```

`URL` se abre con la aplicación al tocar la actividad, `Stale` indica cuándo iOS muestra el contenido como desactualizado y `Background` rellena la actividad en la pantalla de bloqueo.

## Pulsaciones

`widget.OnAction` ejecuta el manejador de la pulsación de una acción o de un `widget.Button`, con la actividad y el ID de la acción (`Activity` es `nil` para un [botón de widget](widgets.md#botones)). Configúralo en `main`, antes de `nsgo.Run`: en iOS el sistema inicia la aplicación en segundo plano ante una pulsación, sin sus vistas, y retiene la actividad hasta que el manejador termina, de modo que el manejador la actualiza allí; en Android una pulsación llega al proceso Go en ejecución a través del receptor del entorno de ejecución sin abrir su Activity. Las acciones necesitan que el motor que las publicó siga en ejecución; una acción obsoleta no hace nada después de que ese motor termina. La propia notificación sigue abriendo la aplicación al tocarla. El manejador se ejecuta en una goroutine del sistema, no en la de las vistas de la aplicación, a la cual accede mediante el método `Post` de un `core.Scope`.

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

Una vista de la propia actividad también lo hace, en iOS 17 y posteriores: `widget.Button{Action: "pause"}` ejecuta el mismo manejador sin abrir la aplicación, y muestra solo sus hijos donde una pulsación únicamente abre la aplicación.

## Actualizaciones por push

Una aplicación en segundo plano actualiza sus actividades solo mientras está en ejecución, de modo que una que sigue lo que ocurre en otro lugar las inicia con `widget.ActivityOptions{Push: true}`, envía el token que recibe `widget.OnPushToken` a su servidor y hace que este envíe actualizaciones `liveactivity` de APNs cuyo `content-state` es la respuesta de `widget.ActivityState` para el contenido: no se envían imágenes en una notificación push. Desde iOS 17.2 la aplicación también obtiene un token para iniciar actividades mediante push, con el tipo de atributos `widget.ActivityAttributesType` y los atributos de tipo e id. Las notificaciones push requieren el entitlement de notificaciones push en el perfil de la aplicación.

```go
state, err := widget.ActivityState(content)  // the push's content-state
```

## Límites

El sistema finaliza una actividad después de ocho horas. `widget.DismissDefault` deja una actividad finalizada en la pantalla de bloqueo hasta cuatro horas en iOS y la retira de inmediato en Android, `widget.DismissNow` la retira de inmediato y `widget.DismissAt` en un momento dentro de las cuatro horas. Android solicita `POST_NOTIFICATIONS` y `POST_PROMOTED_NOTIFICATIONS` por su cuenta: pide permiso al usuario primero con `cx.RequestNotificationPermission` ([Notificaciones](notifications.md)). La demostración de Widgets de la galería y `examples/widgets` ejecutan una.

## Siguiente

- [Widgets](widgets.md): las líneas temporales de la pantalla de inicio y la pantalla de bloqueo, dibujadas por la misma extensión.
- [Notificaciones](notifications.md): el permiso que Android solicita primero.
- [Compilación y distribución](building.md): la extensión y los entitlements que necesita.
