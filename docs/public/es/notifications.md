---
title: Notificaciones
description: Solicita permiso para mostrar notificaciones, muéstralas, prográmalas y ciérralas, cuéntalas, muestra el progreso de una tarea en el icono de la aplicación y escucha las pulsaciones.
group: Funciones de plataforma
order: 45
---

## Permiso

`cx.RequestNotificationPermission` solicita al usuario permitir que la aplicación muestre notificaciones, mostrando el aviso del sistema la primera vez, y responde con un `core.NotificationPermission`: `NotificationsGranted`, `NotificationsDenied` o `NotificationsUnavailable` donde la plataforma o la compilación no muestran notificaciones. Solicítalo cuando el usuario realice una acción que necesite notificaciones: iOS y Android preguntan solo una vez, y los navegadores rechazan un aviso que no haya sido provocado por el usuario. `cx.NotificationPermission` lee la respuesta sin preguntar.

```go
cx.RequestNotificationPermission(func(p core.NotificationPermission) {
	if p == core.NotificationsGranted {
		cx.ShowNotification(core.Notification{ID: "chat:42", Title: "Ada", Body: "Lunch?"}, nil)
	}
})
```

## Mostrar una notificación

`cx.ShowNotification` muestra una notificación, también mientras la aplicación está en primer plano, y reemplaza a una con el mismo `ID`; `Silent` omite el sonido y la vibración del sistema. Responde a `done` indicando si el sistema aceptó la notificación, `false` si no tiene permiso, y devuelve `core.ErrNotification`, sin llamar a `done`, cuando falta el ID o el título. `cx.CloseNotification` elimina la que indica el ID.

```go
err := cx.ShowNotification(core.Notification{ID: "sync", Title: "Synced", Body: "12 notes"}, func(shown bool) {})
cx.CloseNotification("sync")
```

`cx.CloseNotifications` elimina todas las notificaciones que mostró la aplicación y cancela las que programó. En macOS, iOS, watchOS, Windows y Android, esto incluye las que ejecuciones anteriores dejaron en el Centro de notificaciones, el Centro de actividades o el panel de notificaciones; en Linux y la web solo alcanza a las que mostró o programó la ejecución actual. Llámala cuando la aplicación pase a primer plano si su ventana ya muestra lo que comunicaban:

```go
cx.Effect(func() {
	if cx.Env().Lifecycle() == core.LifecycleActive {
		cx.CloseNotifications()
		cx.SetBadge(0)
	}
})
```

## Programar una notificación

Una notificación cuyo `At` sea posterior al momento actual queda programada; volver a llamar a `cx.ShowNotification` con su `ID` la reprograma en otra fecha, mientras que `cx.CloseNotification` la cancela antes de que se muestre. `Repeat` la muestra cada día a la hora del día de `At`, o también cada semana en su día de la semana, a partir de la siguiente ocasión en la zona horaria del dispositivo. `cx.Env().CanScheduleNotifications()` indica si la plataforma las programa: las aplicaciones de macOS, iOS y watchOS en un paquete, Android y Windows las programan de forma nativa esté o no en ejecución la aplicación (Windows encadena las repeticiones diarias y semanales mientras la aplicación está en ejecución y las vuelve a armar al iniciarse), mientras que Linux y la web (donde `Notification` esté disponible) las conservan mientras la aplicación o la página siguen en ejecución. En el resto (tvOS, cuyo centro solo muestra insignias, y los miniprogramas), `done` responde `false`.

```go
cx.ShowNotification(core.Notification{ID: "review", Title: "Cards to review", At: due}, nil)
cx.ShowNotification(core.Notification{ID: "habit", Title: "Practice",
	At: time.Date(2026, 1, 1, 9, 0, 0, 0, time.Local), Repeat: core.NotificationDaily}, nil)
```

Android puede mostrar una notificación programada algunos minutos después de su hora mientras el teléfono está en reposo; una aplicación que la necesite al minuto exacto, como un despertador, incluye `android.permission.SCHEDULE_EXACT_ALARM` en `android.permissions`, que el usuario concede en los ajustes del sistema a partir de Android 12, o `USE_EXACT_ALARM`, que Google Play permite únicamente a aplicaciones de alarma y calendario. Las notificaciones de una aplicación de Android detenida a la fuerza esperan hasta su próximo inicio. Para probar la entrega tras la finalización del proceso, espera a que `ShowNotification` responda `true` y luego mata sus procesos sin forzar la detención del paquete.

## Pulsaciones

`cx.OnNotificationPress` ejecuta su función con el `ID` de una notificación que el usuario ha pulsado, mientras siga vivo el componente que la registró, y trae la aplicación a primer plano. Una pulsación que inició la aplicación llega al primer controlador registrado. Guarda en `ID` lo que abre la pulsación (`"chat:42"`), ya que una pulsación puede volver a iniciar la aplicación desde cero. En Windows lo hace para una aplicación que su instalador haya colocado en el menú Inicio; de lo contrario, una ejecución solo escucha pulsaciones mientras está en marcha.

```go
cx.OnNotificationPress(func(id string) { openChat(strings.TrimPrefix(id, "chat:")) })
```

Una aplicación de Android que solicite este permiso a partir de Android 13 incluye `android.permission.POST_NOTIFICATIONS` en `android.permissions` dentro de `nsgo.yaml`.

## Insignia

`cx.SetBadge` coloca un recuento en el icono de la aplicación: en el Dock en macOS, en la pantalla de inicio en iOS (que necesita permiso de notificaciones), en el botón de la barra de tareas en Windows, en los lanzadores de Linux compatibles con Unity y en una aplicación web instalada. Un recuento de 0 o menor lo borra. Una aplicación de macOS no necesita permiso para su insignia del Dock hasta que comprueba o solicita el permiso de notificaciones o muestra una notificación; a partir de ese momento, en esa ejecución y en las posteriores, el Dock muestra la insignia solo mientras el usuario permita las insignias de la aplicación en Ajustes del Sistema, permiso que `cx.RequestNotificationPermission` solicita junto con las alertas. Los lanzadores sin interfaz de insignia en Linux y Android no tienen un recuento que establecer; los lanzadores de Android marcan una aplicación cuando muestra notificaciones.

```go
cx.SetBadge(len(unread.Get()))
```

## Progreso de tareas

`cx.SetTaskProgress` muestra cuánto ha avanzado una tarea larga en el icono de la aplicación: una barra sobre el mosaico del Dock en macOS, el botón de la barra de tareas en Windows y los lanzadores de Linux compatibles con Unity. No necesita permiso y se muestra junto a la insignia. `TaskProgressNormal` muestra `Value`, una fracción de 0 a 1; `TaskProgressPaused` y `TaskProgressError` la colorean en Windows y macOS; `TaskProgressIndeterminate` se anima en Windows y es una barra fija llena hasta la mitad en macOS y Linux; el valor cero `TaskProgress` la borra. En Windows se muestra en el botón de la ventana del ámbito, y el progreso establecido donde ninguna ventana tiene botón propio, como el propio ámbito de la aplicación o un panel de bandeja, se muestra en cada ventana de la aplicación que no establezca ninguno. iOS, Android y la web no muestran nada.

```go
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressIndeterminate}) // no fraction yet
cx.SetTaskProgress(core.TaskProgress{State: core.TaskProgressNormal, Value: .43})
cx.SetTaskProgress(core.TaskProgress{}) // done
```

## Siguiente

- [Permisos](permissions.md): los demás permisos del sistema, como el micrófono y Accesibilidad.
- [Respuesta háptica](haptics.md): la respuesta que el dispositivo reproduce bajo el dedo del usuario.
- [Servicios de plataforma](services.md): alertas, portapapeles, archivos, almacenamiento y redes.
- [Actividades en vivo](live-activities.md): lo que sigue la aplicación, en la pantalla de bloqueo y en la Dynamic Island.
- [Compilación y distribución](building.md): los permisos que necesita un paquete.
