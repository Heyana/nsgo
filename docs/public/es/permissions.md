---
title: Permisos
description: Comprueba y solicita los permisos del sistema, abre sus ajustes y guía al usuario hasta Ajustes del Sistema en macOS.
group: Funciones de plataforma
order: 49
---

## Comprobar y solicitar

El sistema reserva al consentimiento del usuario parte de lo que una aplicación puede hacer: el micrófono, la cámara, los contactos, los calendarios, los recordatorios, las fotos, Bluetooth, la biblioteca multimedia, la ubicación y las notificaciones en teléfonos y Mac, el movimiento y la ubicación en segundo plano en teléfonos, y en macOS también Accesibilidad, Grabación de pantalla, Monitorización de entrada, Acceso total al disco, Gestión de aplicaciones, Herramientas de desarrollador y Automatización de otra aplicación ([ubicación y movimiento](services.md#location-and-motion), [notificaciones](notifications.md)). `core.Permissions()` los enumera como claves `core.Permission`. `cx.PermissionStatus` responde si la aplicación dispone de uno sin preguntar, y `cx.RequestPermission` lo solicita con el aviso del sistema cuando existe y aún no se ha preguntado. Solicítalo cuando el usuario realice algo que necesite el permiso. Face ID y Touch ID no se encuentran entre ellos: `cx.BiometricAvailability` indica su disponibilidad y `cx.AuthenticateBiometric` solicita autenticación con su propio aviso ([biometría](services.md#biometrics)).

```go
cx.RequestPermission(core.PermissionCamera, func(s core.PermissionStatus) {
	if s.Allowed() {
		startScanner()
	}
})
```

En macOS, Automatización se concede por aplicación de destino: pasa `core.PermissionAutomationOf("com.apple.finder")` para comprobar o solicitar permiso para una aplicación de destino en ejecución mediante su identificador de paquete, o `core.PermissionAutomation` para abrir el panel Automatización en Ajustes del Sistema. Una aplicación de destino que no esté en ejecución responde `PermissionUnknown`.

| Estado | `String()` | Significado |
| --- | --- | --- |
| `PermissionNotAsked` | `prompt` | Aún no se ha solicitado: una solicitud muestra el aviso del sistema |
| `PermissionGranted` | `granted` | La aplicación lo tiene |
| `PermissionLimited` | `limited` | Acceso parcial, como fotos o contactos seleccionados |
| `PermissionDenied` | `denied` | Denegado; solo los ajustes del sistema pueden cambiarlo |
| `PermissionUnknown` | `unknown` | Protegido, pero el sistema o la plataforma no lo informan |
| `PermissionNotGated` | `not-gated` | Nada impide el acceso a la aplicación, como en Linux |
| `PermissionUnavailable` | `unavailable` | La plataforma no dispone de ese permiso |

`Allowed` es `true` para concedido (`granted`), limitado (`limited`) y sin restricción (`not-gated`). macOS responde que Accesibilidad, Grabación de pantalla y Monitorización de entrada están denegados hasta que el usuario los active, ya que solicitarlos solo añade la aplicación a su panel, e informa de la concesión de Grabación de pantalla una vez que la aplicación vuelve a iniciarse.

## Descripciones de uso

En macOS e iOS, solicitar un permiso requiere indicar el motivo en el `Info.plist` de la aplicación, que se define en `apple.info` dentro de `nsgo.yaml`; sin él, la solicitud responde como no disponible y registra la clave, ya que de otro modo el sistema finalizaría la aplicación. Un mensaje en los [catálogos](localization.md) de la aplicación con la misma clave, como `NSCameraUsageDescription: Scans the codes you point it at.` en `locales/en.yaml`, traduce el motivo para que el sistema pregunte en el idioma del usuario; un idioma que no lo traduzca mostrará el del idioma original.

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

En Android, cada permiso debe figurar en `android.permissions` dentro de `nsgo.yaml`; uno que quede fuera del manifiesto responde `PermissionUnavailable`. Bajo `nsgo dev`, macOS atribuye lo que hace la aplicación al terminal que la inició, de modo que el terminal es la aplicación a la que se pregunta y la que aparece en la lista; compila la aplicación para probar sus propios permisos.

## Abrir los ajustes

`cx.OpenPermissionSettings` abre la página donde se concede un permiso e indica si se abrió alguna: Privacidad y seguridad en el panel del permiso (incluidos Automatización, Contactos, Fotos y Localización) o Notificaciones en macOS, la página de la aplicación en Ajustes (o su página de Notificaciones a partir de iOS 16) en iOS, la página de la aplicación en Ajustes en Android, y la página del micrófono, la cámara o las notificaciones en Windows.

Para `PermissionLocation` y `PermissionBackgroundLocation`, macOS abre Localización; `PermissionMotion` abre Movimiento y forma física, aunque macOS no proporcione muestras de movimiento. iOS y Android abren la página de la aplicación para los tres. Abrir los ajustes no solicita ni concede ningún permiso.

## Guiar al usuario

`desktop.GuidePermission` abre esa página y espera a que el usuario conceda el permiso. En macOS, un panel sigue a la ventana de Ajustes del Sistema sin robar el foco y, en los paneles cuya lista admite arrastrar una aplicación dentro, ofrece la aplicación para que el usuario la arrastre allí. Termina, y su función recibe el estado, en cuanto la aplicación obtiene el permiso, el usuario cierra el panel o Ajustes del Sistema, se llama a `stop` o el componente desaparece; una nueva guía finaliza la anterior.

```go
stop := desktop.GuidePermission(cx, core.PermissionAccessibility, desktop.GuideOptions{}, func(s core.PermissionStatus) {
	trusted.Set(s == core.PermissionGranted)
})
```

La aplicación ofrecida es su paquete o, en su defecto, su ejecutable; `GuideOptions.App` indica otra y `Name` cómo la denomina el panel. Windows abre la página y espera a que se cierre Configuración. En el resto de plataformas, la función recibe el estado de inmediato.

## Plataformas

| Plataforma | Qué informa y qué solicita |
| --- | --- |
| macOS | Todos los permisos (Automatización por aplicación de destino en ejecución mediante `PermissionAutomationOf`, además de notificaciones para aplicaciones empaquetadas); Automatización sin destino, Gestión de aplicaciones, Herramientas de desarrollador y Contenido multimedia y Apple Music son desconocidos; el movimiento no está disponible |
| iOS | El micrófono, la cámara, los contactos, los calendarios, los recordatorios, las fotos (con `PermissionLimited`), Bluetooth, la biblioteca multimedia, la ubicación, el movimiento y las notificaciones |
| Windows | El micrófono, la cámara y las notificaciones, para las que nunca muestra aviso; el resto no tiene restricción |
| Linux | Sin restricción fuera de entornos aislados; las notificaciones reflejan si responde un servidor de notificaciones; en Flatpak y Snap, la cámara a través del portal de escritorio y el micrófono a través del portal y PipeWire |
| Web | El micrófono y la cámara (desconocido en Firefox), notificaciones, ubicación (no ubicación en segundo plano) y movimiento en navegadores móviles |
| Android | El micrófono, la cámara, los calendarios, Bluetooth y la biblioteca multimedia, cuando `android.permissions` los declara |
| Apple Watch, miniprogramas | Notificaciones, ubicación y movimiento a través de sus servicios; el resto protegido por el sistema es desconocido por ahora |

Los permisos de ubicación y movimiento siguen a los [sensores de cada plataforma](services.md#location-and-motion), y `PermissionNotifications` sigue a las [notificaciones](notifications.md). La actividad física utiliza `PermissionActivityRecognition` (`ACTIVITY_RECOGNITION` en Android, `NSMotionUsageDescription` en Apple), incluida la presión en Apple; el movimiento sin procesar en Android sigue separado. En Safari móvil, solicita `PermissionMotion` desde un controlador de pulsación.

## Pruebas

`nsgotest` responde a partir de su modelo: `app.Permissions` guarda cada estado, inicialmente sin solicitar, y al solicitar uno no pedido adopta el de `app.PermissionAnswers` correspondiente, o concedido en caso contrario. `PermissionNotifications` se mantiene sincronizado con `app.NotificationPermission` y `app.NotificationAnswer`. `app.PermissionSettings` enumera los ajustes abiertos y `app.Guide` la guía que se está mostrando, que finalizan con `app.GrantPermission` y `app.CloseGuide`.

```go
app.PermissionAnswers[core.PermissionMicrophone] = core.PermissionDenied
app.Press("id:record")
app.Find("text:The microphone is off")
```

## Siguiente

- [Notificaciones](notifications.md): su propio permiso, insignias y pulsaciones.
- [Compilación y distribución](building.md): los derechos y capacidades que declara un paquete.
