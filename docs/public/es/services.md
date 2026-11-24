---
title: Servicios de plataforma
description: Alertas, el portapapeles, archivos, fotos, compartir, almacenamiento, secretos, reconocimiento de voz, lectura de texto en voz alta, redes, inicio de sesión, compras dentro de la aplicación y el ciclo de vida de la aplicación.
group: Funciones de plataforma
order: 44
---

## Alertas y el portapapeles

`cx.ShowAlert` muestra la alerta o la hoja de acciones de la plataforma. Sus `Actions` se devuelven por índice, y un rol dibuja los botones de cancelación y destructivos al modo de la plataforma. En macOS, las alertas y los selectores de archivos usan una hoja mientras su ventana es visible. Una ventana oculta, minimizada o fuera de pantalla, o una aplicación de la bandeja sin ventana, abre en cambio el cuadro de diálogo por separado. Cerrar la ventana que lo solicitó cancela sus cuadros de diálogo.

```go
cx.ShowAlert(core.AlertConfig{
	Title:   "Delete the file?",
	Message: "This cannot be undone.",
	Actions: []core.AlertAction{
		{Label: "Cancel", Role: core.AlertCancel},
		{Label: "Delete", Role: core.AlertDestructive},
	},
}, func(index int) {
	if index == 1 {
		remove()
	}
})
```

`SetClipboard` escribe texto y `SetClipboardImage` una imagen. `ReadClipboard`, `ReadClipboardImage` y `ReadClipboardFiles` responden de forma asíncrona, y `HasClipboardText` y `HasClipboardImage` leen lo que contiene el portapapeles sin aviso de pegado. `core.FileFromURL` abre como `core.File` las URL que dan `ReadClipboardFiles` y `View.OnFileDrop`: URL `file:` y, en la web, URL `blob:`.

tvOS no tiene portapapeles público. Las escrituras en el portapapeles se ignoran, las lecturas no devuelven contenido y las comprobaciones de presencia devuelven false.

```go
cx.ReadClipboard(func(text string) { pasted.Set(text) })
cx.SetClipboard("Hello")
```

En macOS y Windows, `ReadClipboardItems` da una instantánea de todas las representaciones en bytes disponibles, hasta 48 MiB. Cada elemento lleva valores `ClipboardRepresentation` ordenados con un `Type` nativo y sus `Data` sin procesar, incluidos los formatos que solo indican presencia, con datos vacíos. `ClipboardFormats` enumera los nombres de formato nativos sin copiar su contenido. Para restaurar una instantánea, pásala a `WriteClipboardItems` junto con una función de retorno opcional que recibe el error de escritura. Un slice vacío vacía el portapapeles. Compara la igualdad de los valores de `ClipboardChangeCount` para detectar una nueva copia, incluso cuando el texto copiado es idéntico.

macOS conserva los límites entre elementos y los nombres de tipo del portapapeles. Windows usa un único elemento y conserva los nombres Win32 respaldados por bytes, como `CF_HDROP`, y los formatos registrados, como `Rich Text Format`. Al escribir combina los elementos con formatos distintos y usa un byte cero para un contenido vacío. Se omiten los identificadores GDI, los formatos de visualización por el propietario y los ID de formato numéricos privados. En otras plataformas, las funciones de retorno de lectura y escritura reciben `CodeUnavailable` y la del recuento recibe `-1`.

## Abrir enlaces y archivos

`cx.OpenURL` abre una URL en el navegador o la aplicación del sistema, y `core.CheckURL` indica qué rechaza: sin esquema, más de 16 KiB, espacios, una dirección web sin host, y `javascript:`, `data:`, `blob:`, `file:`, `about:` y `asset:`.

```go
if err := core.CheckURL(url); err == nil {
	cx.OpenURL(url, func(opened bool) {
		if !opened {
			status.Set("No app opens that link.")
		}
	})
}
```

`cx.OpenFile` abre un archivo que tiene la aplicación en la aplicación asociada a su tipo, y `cx.Share` muestra la hoja para compartir del sistema para archivos, texto o ambos en macOS, iOS, Android, Windows, Linux (a través del portal de escritorio o del gestor de archivos) y la web (`navigator.share`). Un `core.SharedFile` es una ruta en disco —o, para `Share` en la web y en cualquier lugar donde un archivo se mantenga en memoria, sus bytes `Data`— con el nombre y el tipo que ve la otra aplicación. En los escritorios, `desktop.RevealFile` muestra un archivo o una carpeta seleccionados en Finder, el Explorador o el gestor de archivos, y `desktop.TrashFile` lo mueve a la Papelera; nunca elimina de forma permanente: responde `core.ErrNoTrash` y deja el elemento en su sitio cuando el volumen no tiene papelera o la papelera no lo conservaría, como un archivo mayor que la Papelera de reciclaje.

```go
cx.Share(core.ShareOptions{
	Files: []core.SharedFile{{Path: path, Name: "Notes.txt"}},
	Text:  "A note from NSGo",
}, func(shown bool, err error) {})
```

`mobile.OnShared(cx, func(items []mobile.SharedItem) { ... })` recibe texto y archivos para [`android.share_types`](building.md#incoming-shares-on-android) o [`apple.share_types`](building.md#incoming-shares-on-ios). Los contenidos compartidos en Android pueden iniciar la aplicación; iOS los pone en cola en un App Group hasta que la aplicación está activa o el usuario la abre de nuevo. Los contenidos compartidos pendientes esperan al primer manejador. iOS conserva el lote de la bandeja de entrada hasta que la función de retorno termina; la entrega es al menos una vez aunque el proceso finalice. El texto llega en `Text`, y los archivos llegan como copias legibles en caché con `Path`, `Name` y `MIME`. Mueve o copia un archivo a un almacenamiento persistente si debe sobrevivir a la caché.

`desktop.LookUp` muestra el panel de diccionario del sistema para un texto, como hace un toque con tres dedos en macOS, dibujado sobre un recuadro de una vista: el texto y el recuadro de una selección con la `Ref` de su vista `Selectable`. `desktop.CanLookUp` indica si puede hacerlo, en una ventana en macOS; Windows, Linux y la web no tienen ese panel. En las pruebas, `nsgotest.App.LookUps` enumera los paneles mostrados donde `nsgotest.Environment` establece `CanLookUp`.

```go
var area core.Ref
var selected core.TextSelection
core.View{Ref: &area, Selectable: true, OnTextSelection: func(s core.TextSelection) { selected = s }, Children: subtitles}
// From a menu item or a shortcut:
if desktop.CanLookUp(cx) {
	desktop.LookUp(cx, selected.Text, &area, selected.Bounds)
}
```

`desktop.QuickLook` solicita vistas previas de archivos, empezando por un índice, y actualiza los archivos cuando se vuelve a llamar mientras el panel está abierto. `desktop.CanQuickLook` informa de la disponibilidad en una ventana propia de la aplicación en macOS y Windows, y en Linux cuando la sesión expone el servicio D-Bus de GNOME Sushi en ejecución o activable. Windows usa el controlador de vista previa registrado para el tipo de archivo; los formatos admitidos dependen de los controladores o el visor instalados. Un resultado true significa que se hizo la solicitud a la plataforma, no que esta mostrara el archivo.

Espacio o Escape cierran el panel. Las teclas de flecha seleccionan archivos de la solicitud en Windows y en las versiones de Sushi que informan de la selección y la propiedad en X11. En Wayland, Linux abre actualmente una vista previa sin ventana padre; el cierre automático con el propietario y la selección con flechas no están disponibles. Los visores específicos de KDE y la web no son compatibles.

```go
if desktop.CanQuickLook(cx) {
	desktop.QuickLook(cx, []string{reportPath, sourcePath}, 0)
}
```

## Iconos de menús nativos

`desktop.MenuItem.Icon` muestra un icono junto a un elemento en los menús de la aplicación, del Dock, de la bandeja y emergentes en macOS, Windows y Linux. Un `desktop.MenuIcon` acepta un icono vectorial analizado (`SVG`, como `lucide.Folder()`), bytes de imagen codificados (`Data`, PNG, JPEG, GIF, WebP o SVG) o un `core.ImageSource` local, de recurso o de píxeles (`Source`). Establecer `Template` tiñe una imagen monocroma con el color del texto del menú, siguiendo la apariencia clara y oscura (y el estado de resaltado en macOS); los iconos `SVG` monocromos se tratan como plantillas de forma predeterminada.

```go
desktop.SetApplicationMenu(cx, []desktop.MenuItem{{
	Label: "File",
	Items: []desktop.MenuItem{
		{Label: "Open Folder…", Icon: desktop.MenuIcon{SVG: lucide.Folder()}, OnSelect: openFolder},
		{Label: "Export", Icon: desktop.MenuIcon{Data: exportPNG, Template: true}, OnSelect: exportFile},
	},
}})
```

Los iconos se cargan de forma síncrona desde fuentes locales; las URL remotas y las imágenes no válidas dejan el elemento sin icono. Las etiquetas de menú, los atajos y los indicadores de casilla y de opción conservan su comportamiento nativo.

En las pruebas, `nsgotest.MenuItem.Icon` registra el icono de cada elemento en `app.Menu`, `app.DockMenu` y `app.PopupMenu`.

## Fotos y archivos

`cx.PickImages` muestra el selector de fotos del sistema y responde con `core.PickedFile`, cuyos bytes, nombre, tipo y tamaño en píxeles lee la aplicación. `cx.TakePhoto` muestra la cámara donde la plataforma tiene una; `cx.Env().CanTakePhoto` indica si la tiene. En macOS, Windows y Linux muestra la vista previa de la cámara del escáner con un botón Hacer foto. `cx.ScanCode` abre una vista previa de la cámara y responde con el primer código que lee, un código QR salvo que `ScanOptions.Kinds` pida otros, o nil cuando el usuario la cierra; `cx.Env().CanScanCode` indica si la plataforma tiene un escáner: en dispositivos macOS, iOS y Android con cámara, equipos Windows que tengan una, escritorios Linux con el portal Camera y `pipewiresrc` de GStreamer, y navegadores con `getUserMedia` y `BarcodeDetector`. macOS presenta una hoja cuando la ventana que lo solicita es visible, Windows y Linux un modal sobre la ventana, y la web una superposición sobre la página. Pon `NSCameraUsageDescription` en `apple.info` para macOS e iOS, y `CAMERA` en `android.permissions` para Android. Android, Windows y Linux leen todos los tipos salvo PDF417. Trata el texto del código como entrada no fiable.

En tvOS, `PickImages` responde sin imágenes y `TakePhoto` devuelve `core.ErrNoCamera`; NSGo no proporciona el selector de Cámara de Continuidad.

En las pruebas, `nsgotest.App.PickImages`, `nsgotest.App.TakePhoto` y `nsgotest.App.ScanCode` reciben registros `nsgotest.ImagePick` para las selecciones y capturas y registros `nsgotest.CodeScan` para los escaneos, y responden en lugar de la interfaz del sistema. Los ganchos funcionan sin interfaz gráfica y en los anfitriones `nsgo test -target` compatibles. `nsgotest.Environment(core.Environment{Camera: true, ScanCode: true})` da una cámara a un destino que no la tiene.

```go
cx.PickImages(core.PickOptions{Limit: 1, MaxDimension: 2048}, func(files []core.PickedFile) {
	if len(files) == 1 {
		avatar.Set(files[0].Data)
	}
})
```

`cx.PickFiles` selecciona archivos de cualquier tipo, `cx.PickDirectories` selecciona carpetas como un `fs.FS`, y `cx.SaveFile` guarda un archivo o bytes mediante el cuadro de diálogo de guardado del sistema. Ambos selectores aceptan un `Title`, una etiqueta `Button` y una carpeta `Start`, y `PickFilesOptions.TypesName` da nombre al menú de tipos, de modo que una aplicación puede mostrarlos en su propio idioma; son indicaciones, que macOS, Windows y Linux siguen, iOS solo para `Start`, y Android y la web en absoluto. `cx.PickScreenColor` selecciona un color en cualquier lugar de la pantalla donde `cx.Env().CanPickScreenColor()` es true: macOS, Windows, Linux con el portal Screenshot del escritorio y navegadores con `EyeDropper`.

Establece `core.SaveOptions.Start` en una carpeta absoluta existente para abrir allí el cuadro de diálogo de guardado en macOS, Windows, Linux e iOS. Con una indicación vacía o inutilizable, o en otros destinos, el sistema elige la carpeta.

Establece `core.SaveOptions.Accept` para validar el destino elegido antes de escribir. Se ejecuta en la goroutine del motor. Devolver un error deja el destino intacto. En macOS, Windows y en el selector GTK dentro del proceso en Linux, el cuadro de diálogo muestra el motivo y permite al usuario elegir otra ruta. Cancelar tras un rechazo informa de `saved == false`, la ruta rechazada y el error. En Android y en el portal de escritorio de Linux, un rechazo completa el guardado con ese resultado. Android permite al usuario elegir una carpeta y proporciona un URI de árbol con el nombre de archivo solicitado como su parámetro de consulta `name`. Crea el documento tras la aprobación e informa del URI del nuevo documento si tiene éxito. La plataforma abandona la escritura si la comprobación no responde en cinco segundos. iOS y HarmonyOS informan de un error en los guardados que requieren una comprobación. La web no tiene ruta de destino y usa su flujo de guardado habitual.

En las pruebas sin interfaz gráfica, `App.PickFiles`, `App.PickDirectories` y `App.SaveFile` reciben registros `nsgotest.FilePick`, `nsgotest.DirectoryPick` y `nsgotest.FileSave`. Los mismos registros están en `app.FilePicks`, `app.DirectoryPicks` y `app.Saves`.

### Recursos incluidos

Pon archivos en el directorio `resources/` de la aplicación y usa `cx.ReadAsset("asset:maps/level.bin", done)` para leerlos desde Go. La función de retorno recibe `([]byte, error)` en la goroutine del motor tras la lectura y se descarta si su propietario desaparece o se vuelve a ejecutar. Usa `errors.Is(err, core.ErrAssetNotFound)` para identificar los archivos que faltan. La lectura carga el archivo completo en memoria.

## Extraer audio PCM

Usa `audio.NewOutput(cx, audio.OutputConfig{SampleRate: 48000, Channels: 2, Render: render})` para reproducir PCM Float32 entrelazado que tu aplicación decodifica o sintetiza. `Start` empieza a extraer muestras, `Pause` detiene las extracciones y el ámbito propietario cierra la salida cuando desaparece. macOS, iOS, Android, Windows, Linux y la web admiten este flujo. Los demás destinos devuelven `audio.ErrUnsupported`.

`Render` devuelve el número de fotogramas que ha llenado. Se ejecuta en el hilo de audio y debe evitar bloquearse, asignar memoria y acceder al grafo. Un llenado incompleto reproduce silencio en el resto e incrementa `Underruns`. `Position` devuelve el recuento monótono de fotogramas y la hora del anfitrión en ese fotograma. Usa `Flush` para descartar los fotogramas pendientes y `Latency` para leer la latencia del búfer. Los cambios de ruta y las interrupciones pausan la salida y llaman a `OnInterrupt` en el motor. La aplicación puede reanudarla con `Start`.

## Almacenamiento

`cx.Storage()` es el almacenamiento clave-valor de la aplicación, guardado en el dispositivo y compartido por todos los grafos de la aplicación. Las lecturas son síncronas y, leídas dentro de un cálculo, lo suscriben a los cambios; `SetItem` devuelve un `core.StorageFullError` al superar la cuota (`core.StorageQuota`, 5 MiB de forma predeterminada). Es un archivo sin cifrar que cualquier programa del usuario puede leer, así que guarda los tokens y las contraseñas en [secretos](#secrets).

`cx.Store()` devuelve el almacén subyacente, para un anfitrión que crea otra raíz con `core.RootOptions.Storage`, como uno que ejecuta un complemento en una goroutine propia: ambas raíces comparten entonces las entradas de la aplicación y lo que se guarda en el dispositivo. Es el almacén completo, no la partición que `cx.ProvideStorage` da al código que la aplicación no escribió.

Un anfitrión que proporciona su propia persistencia llama a `core.NewStore(backend)` con un `core.StorageBackend`: `Load() (map[string]string, error)` carga una vez, y `Write(core.StorageChange)` recibe los cambios en orden. Haz un switch sobre `change.Op` (`core.StorageSet`, `core.StorageRemove`, `core.StorageClear`) y usa `change.Key` y `change.Value`. Pasa `nil` para un almacenamiento que solo se mantiene en memoria.

```go
store := cx.Storage()
if tab, ok := store.GetItem("tab"); ok {
	selected.Set(tab)
}
store.SetItem("tab", "library")
```

`store.Keys()` enumera las claves, `RemoveItem` elimina una y `Clear` vacía el almacenamiento. `cx.DataDir()` y `cx.CacheDir()` dan las carpetas de la aplicación para archivos para los que el almacenamiento clave-valor es demasiado pequeño; `cx.LogsDir()` da el directorio de la plataforma para los archivos de registro, y un `Scope.ProvideStorage` da a un subárbol una partición propia.

Un App Clip de iOS y su aplicación completa comparten estas ubicaciones de datos y almacenamiento cuando se configuran `app_clip` y un `apple.app_group` explícito; sus cachés siguen separadas. Activar el grupo no migra los datos existentes del entorno aislado. [Almacenamiento de App Clip](building.md#app-clips) trata la configuración y la firma.

En HarmonyOS, las operaciones de archivo de Go como `os.ReadFile`, `os.WriteFile` y `os.Rename` funcionan en estos directorios y en `os.TempDir()`. Las rutas relativas empiezan en el directorio de archivos. Los bloqueos de archivo consultivos y los enlaces simbólicos no están disponibles.

### Las carpetas del usuario

`desktop.UserDir(cx, desktop.DownloadsDir)` devuelve la ruta de una de las carpetas del usuario: `DocumentsDir`, `DownloadsDir`, `DesktopDir`, `PicturesDir`, `MusicDir` o `VideosDir`. Consulta al sistema cada vez, así que sigue a una carpeta que el usuario haya movido: las carpetas del directorio personal en macOS (`Movies` para los vídeos), las carpetas conocidas en Windows, incluidas las de OneDrive, y los directorios de usuario XDG en Linux. Puede que la carpeta aún no exista. Los teléfonos, la web y HarmonyOS no tienen carpetas así a las que una aplicación llegue por ruta, ni tampoco Linux para una carpeta que `user-dirs.dirs` omite: `UserDir` devuelve `core.ErrUserDirUnavailable`, y la aplicación se lo pide al usuario con `cx.PickDirectories` o `cx.SaveFile` en su lugar.

```go
dir, err := desktop.UserDir(cx, desktop.DownloadsDir)
if err != nil {
	dir = cx.DataDir()
}
```

Las pruebas nunca obtienen las carpetas del usuario: cada una es un directorio vacío dentro del directorio temporal de la prueba, y `nsgotest.UserDirs(map[core.StandardDir]string{…})` las fija, y una carpeta omitida queda no disponible.

### Informes de fallos

Las compilaciones nativas de publicación guardan las trazas de fallos de Go en `cx.LogsDir()/crashes`. Apple registra las excepciones de Objective-C no capturadas, Android registra las excepciones de Java no capturadas y Windows registra las excepciones SEH que llegan a su filtro de nivel superior. Enumera los informes pendientes en el siguiente inicio con `cx.CrashReports()`. Cada informe identifica la versión, la compilación y la plataforma que lo produjo. Llama a `Open()` para leer el texto de diagnóstico, cierra el lector y luego llama a `Acknowledge()` para eliminar el informe cuando la aplicación lo haya procesado. Ejecuta estas operaciones de archivo en una goroutine.

## Espacio en disco

`cx.DiskSpace(func(space core.DiskSpace, err error) { … })` consulta de forma asíncrona el volumen que contiene `cx.DataDir()` y llama a la función de retorno en la goroutine del motor. `Total` es la capacidad que informa el sistema, `Available` es lo que la aplicación puede usar para una operación solicitada por el usuario, y `Opportunistic` es lo que puede usar para cachés y precarga. Los tres están en bytes. `Available` incluye el espacio que el sistema recupera bajo demanda donde informa de esa cifra. Las plataformas con una única cifra de espacio libre la devuelven en ambos campos. En la web, `Total` es la cuota de almacenamiento estimada por el navegador para el origen y `Available` es la parte sin usar. Cuando el servicio no está disponible, devuelve `core.ErrDiskSpaceUnavailable`.

Las compilaciones de Apple requieren que `apple.privacy.accessed_apis` en `nsgo.yaml` declare `NSPrivacyAccessedAPICategoryDiskSpace`. Declara `E174.1` para comprobar el espacio antes de escribir o para liberar archivos cuando queda poco espacio, con un cambio que el usuario pueda observar. Declara `85F4.1` para mostrar al usuario el espacio disponible. La aplicación debe seguir las restricciones de Apple sobre el uso y la compartición de las lecturas. Una compilación sin la categoría devuelve `core.ErrNotDeclared`. [Motivos aprobados por Apple](https://developer.apple.com/documentation/bundleresources/app-privacy-configuration/nsprivacyaccessedapitypes/nsprivacyaccessedapitype)

Establece la respuesta de una prueba con `nsgotest.DiskSpace(space, err)`.

## Almacenamiento en la nube

El paquete `cloud` sincroniza entre dispositivos pequeñas entradas clave-valor (`cloud.Get`, `cloud.Set`, `cloud.Delete`, `cloud.Keys`) y archivos más grandes o instantáneas de partidas guardadas (`cloud.ReadFile`, `cloud.WriteFile`, `cloud.DeleteFile`, `cloud.ListFiles`): el almacén clave-valor de iCloud y los contenedores de documentos ubicuos en macOS e iOS, y Saved Games de Google Play Games en Android.

Cada operación entrega su función de retorno en la goroutine del motor y la omite si `cx` se ha desechado. `cx.Env().CanUseCloudStorage()` informa de si el almacenamiento en la nube está disponible; cuando el usuario no ha iniciado sesión, faltan los entitlements o la plataforma no tiene un backend nativo en la nube (Windows, Linux y la web), cada llamada devuelve `cloud.ErrUnavailable` y nunca descarta escrituras en silencio.

```go
if cx.Env().CanUseCloudStorage() {
	cloud.WriteFile(cx, "saves/slot1.sav", saveBytes, func(err error) {
		if err != nil {
			status.Set(err.Error())
		}
	})
}

cloud.WatchChanges(cx, func(ev cloud.ChangeEvent) {
	reload(ev.Keys, ev.Files)
})

cloud.OnConflict(cx, func(c cloud.Conflict) {
	_ = c.Resolve(mergeSaves(c.Local, c.Remote))
})
```

`cloud.WatchChanges` y `cloud.OnConflict` entregan en la goroutine del motor los cambios externos desde varios dispositivos y los conflictos de versión, y se limpian automáticamente cuando `cx` se desecha. Leer un archivo en conflicto devuelve un `*cloud.ConflictError` (que coincide con `errors.Is(err, cloud.ErrConflict)`).

Para activar los entitlements de iCloud de Apple en `nsgo build`, añade una sección `cloud:` (o `icloud:`) a `nsgo.yaml`:

```yaml
cloud:
  kv_store: true
  containers:
    - iCloud.com.example.myapp
```

El perfil de firma debe conceder las capacidades de iCloud que solicitas. En macOS solo las compilaciones para la Mac App Store (`sign.macos_profile`) llevan iCloud, ya que macOS lo concede solo con un perfil de aprovisionamiento; una compilación Developer ID lo omite con una advertencia. En Android, el almacenamiento en la nube necesita el SDK de Play Games Services v2 en la aplicación y un jugador que haya iniciado sesión.

En tiendas de escritorio como Steam, Steam Auto-Cloud sincroniza `cx.DataDir()` directamente sin código adicional: sigue escribiendo allí los archivos de partida locales y deja que Steam sincronice el directorio.

En las pruebas, `nsgotest` da a cada aplicación un almacén en la nube aislado en memoria: cambia la disponibilidad con `nsgotest.CloudAvailable` o `app.SetCloudAvailable`, simula actualizaciones externas desde varios dispositivos con `app.SimulateCloudChange` y prueba la resolución de conflictos con `app.SimulateCloudConflict`.

## Secretos

`cx.Secrets()` es el almacén seguro de la aplicación, para tokens de actualización, claves de API y contraseñas: `Get`, `Set` y `Delete` de una cadena por cuenta, que la plataforma protege. Una cuenta inexistente se lee como `("", false, nil)`, y donde la plataforma no tiene un almacén seguro, como en la web, cada llamada devuelve `core.ErrSecretsUnavailable`; NSGo nunca recurre a un almacenamiento sin protección. El sistema puede hacer esperar una llamada, como cuando un llavero pregunta al usuario, así que hazlas fuera de la goroutine del motor y devuelve el resultado con `cx.Post`.

```go
secrets := cx.Secrets()
go func() {
	token, ok, err := secrets.Get("refresh-token")
	cx.Post(func() {
		if err == nil && ok {
			login(token)
		}
	})
}()
```

| Plataforma | Dónde se guarda un secreto |
| --- | --- |
| macOS | Una contraseña genérica del llavero de inicio de sesión: `kSecAttrService` es el servicio, `kSecAttrAccount` la cuenta |
| iOS, tvOS, watchOS | Una contraseña genérica del llavero, de la misma manera |
| Windows | Una credencial genérica del Administrador de credenciales cuyo destino es `service/account`, de 2560 bytes como máximo |
| Linux | Un elemento del Secret Service (GNOME Keyring, KWallet) con los atributos `service` y `account`; no disponible donde la sesión no ejecuta ninguno |
| Android | Cifrado con una clave AES del Android Keystore, en los archivos privados de la aplicación, fuera de las copias de seguridad; las lecturas también recurren a los valores que `expo-secure-store` cifró en sus `SharedPreferences` de `SecureStore` |
| Miniprogramas de WeChat | Almacenamiento cifrado por la plataforma (`wx.setStorage` / `wx.getStorage` con `encrypt: true`, biblioteca base 2.21.3+); no disponible en bibliotecas base anteriores |
| Web, otros miniprogramas | No disponible (`core.ErrSecretsUnavailable`): los navegadores no tienen un almacén de secretos de cliente protegido por el sistema operativo (guarda las sesiones web en cookies `HttpOnly` establecidas por el servidor), y los miniprogramas de QQ, Alipay y Douyin carecen de almacenamiento cifrado |

El servicio es el ID de la aplicación (`core.App().ID`) de forma predeterminada. Una aplicación que procede de otro framework mantiene la sesión iniciada de sus usuarios nombrando los elementos que escribió ese framework: `Service` y, en las plataformas de Apple, `AccessGroup` y `Accessible`. En Android, establece `Service` en el `keychainService` de `expo-secure-store` (`"key_v1"` de forma predeterminada) para leer los valores que cifró en `SharedPreferences`; escribir o eliminar borra la entrada migrada. En iOS, `core.SecretAfterFirstUnlock` permite que el trabajo en segundo plano lea un elemento mientras el teléfono está bloqueado, y una accesibilidad `ThisDeviceOnly` lo mantiene fuera de las copias de seguridad. En macOS, un `AccessGroup` mueve los elementos al llavero de protección de datos, lo que requiere una compilación firmada por un equipo cuyo entitlement `keychain-access-groups` incluya el grupo, configurado en `macos.entitlements` en `nsgo.yaml` tanto para compilaciones de la Mac App Store como Developer ID.

```go
s := cx.Secrets()
s.Service = "app:no-auth" // the service an earlier build used
s.Accessible = core.SecretAfterFirstUnlockThisDeviceOnly
err := s.Set("refresh-token", token)
```

`nsgotest` guarda los secretos de una aplicación en memoria, en `App.Secrets`, y `nsgotest.Secrets` inicia una prueba con algunos, de modo que una prueba nunca toca el llavero de la máquina. Un paquete TSX accede al almacén con la capacidad `secrets`, bajo un servicio propio.

## Redes

`cx.Env().Network()` informa de si el sistema tiene una ruta utilizable (`Online`) y de si considera esa ruta de uso medido o costosa (`Expensive`). Los enlaces la siguen como cualquier otro valor del entorno. `cx.OnNetworkChange(func(core.Network))` llama al manejador en la goroutine del motor por cada cambio mientras su ámbito vive. La ruta del sistema no dice nada sobre si un servidor concreto responde. Un destino que no puede informar de la conectividad se considera en línea de forma predeterminada, y un coste desconocido se considera false.

Establece `core.Environment{Network: &core.Network{Online: false}}` mediante `nsgotest.Environment` o `App.SetEnvironment` para probar el comportamiento sin conexión.

El propio `net/http` de Go hace peticiones en todos los destinos, con la configuración de TLS y de proxy del sistema. Las señales pertenecen a la goroutine del motor, así que una respuesta desde otra goroutine les llega a través de `cx.Post`.

```go
go func() {
	resp, err := http.Get("https://api.example.com/items")
	if err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	defer resp.Body.Close()
	var items []Item
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		cx.Post(func() { errText.Set(err.Error()) })
		return
	}
	cx.Post(func() { list.Set(items) })
}()
```

`cx.Post` ejecuta su función en la goroutine del grafo, donde se pueden establecer señales. En Android el manifiesto solicita el permiso de internet, y las descargas de imágenes se guardan en caché en disco.

`core.NewResource` hace lo mismo para un valor cargado a partir de una fuente. Su función de obtención se ejecuta en una goroutine propia y vuelve a cargar cuando cambia la fuente:

```go
items := core.NewResource(cx, func() (string, bool) { return query.Get(), query.Get() != "" },
	func(ctx context.Context, q string, _ core.ResourceInfo) ([]Item, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.example.com/items?q="+url.QueryEscape(q), nil)
		if err != nil {
			return nil, err
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		var list []Item
		if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
			return nil, err
		}
		return list, nil
	})
```

`items.Get()`, `items.Err()` y `items.Loading()` son señales que siguen los enlaces. Una nueva consulta cancela mediante `ctx` la petición que aún está en curso, de modo que una respuesta tardía nunca sustituye a una más reciente. `items.Refetch()` lo vuelve a intentar, conservando la última lista mientras carga, y `items.Mutate` establece un valor de inmediato para una actualización optimista.

`core.Suspense` muestra un contenido alternativo mientras los recursos que lee su contenido cargan su primer valor, de modo que una pantalla no necesita un indicador de carga propio:

```go
core.Suspense{
	Content:  func(cx *core.Scope) core.Node { return itemList(cx, items) },
	Fallback: func(*core.Scope) core.Node { return ui.Spinner{Label: "Loading"} },
}
```

El contenido permanece montado pero oculto mientras espera, de modo que se muestra de inmediato cuando llegan los valores. Una nueva obtención lo mantiene visible con el último valor.

### Descubrir servicios locales

En macOS, iOS, Android, Windows y Linux, `cx.AdvertiseService` publica un puerto de escucha y atributos TXT. `cx.BrowseServices` informa de los servicios resueltos con su host, su puerto y sus atributos TXT. `Host` puede ser un nombre de host DNS o una dirección IP. Cuando un servicio desaparece, la función de retorno lo recibe de nuevo con `Removed` establecido. Comprueba `cx.Env().CanDiscoverServices()` para saber si la plataforma lo admite. Ambas llamadas devuelven una función de detención y terminan cuando su ámbito se desecha. Un error de la plataforma se notifica de forma asíncrona en la goroutine del motor. Linux usa Avahi e informa de un error si el daemon no está disponible.

Para Bonjour en las plataformas de Apple, declara los tipos de servicio y explica el acceso a la red local en `nsgo.yaml`. El sistema gestiona el aviso de permiso y recuerda la elección del usuario.

```yaml
apple:
  info:
    NSLocalNetworkUsageDescription: Finds nearby devices to sync with.
    NSBonjourServices:
      - _example._tcp
```

Android usa `NsdManager`. Android 12 y anteriores, y Android 13 antes de T Extensions 7, requieren `CHANGE_WIFI_MULTICAST_STATE` para el descubrimiento por Wi-Fi. Las compilaciones independientes declaran ese permiso. Si integras NSGo en un anfitrión Android, añádelo al manifiesto del anfitrión para esos dispositivos. Los valores TXT anunciados deben ser UTF-8 válido en los dispositivos sin la API de TXT sin procesar de Android (API 37 o T Extensions 22). La exploración conserva los valores TXT sin procesar. Si un anfitrión Android tiene como destino la API 37 o superior, debe declarar y solicitar `ACCESS_LOCAL_NETWORK` en Android 17 o posterior.

Usa `nsgotest.LocalServices` para proporcionar resultados de exploración en una prueba sin interfaz gráfica.

## Reproducción en curso

Usa `cx.SetNowPlaying(core.NowPlaying{...})` para mostrar el contenido multimedia actual de la aplicación en el Centro de control y la barra de menús de macOS. Proporciona `Title`, `Artist`, `Album` y, opcionalmente, `Artwork` como un `*core.ImageSource`. Una `Duration` cero significa una emisión en directo. Indica la `Position` actual y la `Rate` de reproducción (cero mientras está en pausa, uno a velocidad normal). Publica de nuevo cuando cambien la pista, la posición o la velocidad. El sistema avanza la posición entre llamadas.

Enumera los controles de transporte admitidos en `Commands`, como `core.MediaPlay`, `core.MediaPause`, `core.MediaTogglePlayPause` y `core.MediaSeek`. Gestiona las solicitudes con `cx.OnMediaCommand`. La función de retorno recibe el `Kind` del comando y, para un salto, la `Position` solicitada. El manejador activo más reciente recibe los comandos. Llama a `cx.ClearNowPlaying()` cuando la aplicación termine de reproducir. Mientras la aplicación publica manualmente, los vídeos controlados en macOS suspenden su propia publicación. Comprueba `cx.Env().CanShowNowPlaying()` para saber si la plataforma lo admite.

## Descargas en segundo plano

Usa `cx.Download` y `cx.OnDownloadUpdate` en aplicaciones independientes de iOS y macOS, y en Android. Las vistas integradas de Apple no tienen un anfitrión de transferencias. Las aplicaciones Android activan `android.background_downloads: true` en `nsgo.yaml`. Inicia una descarga a partir de una acción del usuario mientras la aplicación es visible. Usa un destino absoluto con un directorio padre existente en el que la aplicación pueda escribir.

```go
cx.OnDownloadUpdate(func(p core.DownloadProgress) {
    progress.Set(p)
})
task, err := cx.Download(core.DownloadRequest{URL: url}, path, core.DownloadOptions{
    CertificatePEM: serverCertificate,
})
```

La transferencia usa GET y acepta cabeceras de petición opcionales. Usa el almacén de confianza HTTPS normal salvo que `CertificatePEM` fije el certificado hoja del servidor. La fijación también requiere un nombre de host coincidente y un certificado válido. Usa la URL final, ya que el servicio requiere una respuesta correcta de esa URL.

En Apple, un certificado autofirmado en un nombre de host público necesita una excepción de App Transport Security para ese host en `apple.info`. La transferencia sigue comprobando la fijación exacta de su certificado, el nombre de host y la validez. Una conexión a un NAS local también necesita un `NSLocalNetworkUsageDescription` que explique el acceso, y los hosts sin calificar o `.local` usan `NSAllowsLocalNetworking`. [Ajustes de ATS de Apple](https://developer.apple.com/documentation/security/preventing-insecure-network-connections)

```yaml
apple:
  info:
    NSAppTransportSecurity:
      NSExceptionDomains:
        files.example.com:
          NSExceptionAllowsInsecureHTTPLoads: true
```

El servicio en primer plano de Android continúa después de que la actividad sale de la pantalla. Publica una notificación de progreso, sujeta al permiso de notificaciones de Android. El progreso usa recuentos de bytes y establece `Total` en -1 hasta que se conoce. Si tiene éxito, sustituye el destino de forma atómica. `task.Pause()` conserva el archivo parcial, `task.Resume()` usa un rango HTTP cuando el servidor proporciona un validador, y `task.Cancel()` elimina el archivo parcial.

iOS usa una URLSession del sistema en segundo plano que continúa las transferencias mientras la aplicación está suspendida. La aplicación vuelve a conectar su sesión al iniciarse y publica los archivos antes de confirmar la finalización en segundo plano. En macOS las transferencias se ejecutan dentro del proceso. Las plataformas de Apple usan los datos de reanudación nativos para las descargas en pausa y guardan los metadatos de la petición en el almacenamiento privado de la aplicación hasta que se guarda un resultado final. Un servidor que no puede reanudar empieza la transferencia de nuevo.

Registra `OnDownloadUpdate` de nuevo cuando se inicia la aplicación para ver las tareas activas y los resultados guardados. Usa `p.Task.Forget()` después de gestionar un resultado final. Android puede detener el servicio, por ejemplo cuando expira su tiempo de transferencia en primer plano. La aplicación recibe entonces `DownloadInterrupted`. En Windows, Linux, tvOS, watchOS y la web, `Download` devuelve `ErrDownloadUnavailable`.

## Reconocimiento de voz y el ciclo de vida

`cx.StartSpeech` reconoce la voz con el reconocedor del sistema donde `cx.Env().CanRecognizeSpeech()` indica que existe (macOS, iOS, Android, Windows y navegadores que exponen `SpeechRecognition` o `webkitSpeechRecognition`), y responde con la transcripción a medida que cambia. En las plataformas de Apple, declara `NSMicrophoneUsageDescription` y `NSSpeechRecognitionUsageDescription` en `apple.info`; en Android, añade `RECORD_AUDIO` a `android.permissions`. En una prueba sin interfaz gráfica, `app.Speech` contiene una `nsgotest.SpeechSession` mientras el reconocimiento está activo. Usa `app.HearSpeech` y `app.EndSpeech` para entregar resultados.

```go
session, err := cx.StartSpeech(core.SpeechOptions{
	Lang:    "en-US",
	Interim: true,
	OnResult: func(text string, final bool) {
		transcript.Set(text)
	},
})
if err == nil {
	defer session.Stop()
}
```

`cx.Env().Lifecycle()` indica si la aplicación está activa, inactiva o en segundo plano: en los escritorios, la ventana en la que está `cx`, de modo que el contenido de una ventana minimizada puede pausarse mientras otra ventana se muestra, y en el ámbito propio de la aplicación, todas sus ventanas en conjunto. `desktop.Minimized(cx)` indica si esa ventana está minimizada, y `desktop.FrontmostApp(cx)` qué aplicación está en primer plano en el escritorio, incluida la propia aplicación, y si su ventana está a pantalla completa, tal como la sigue una mascota de escritorio o un registrador de tiempo; `desktop.CanReadFrontmostApp(cx)` indica si se sigue, en macOS, Windows, X11 y compositores Wayland compatibles. `cx.OnQuit` limpia al salir, sea cual sea la forma de salir, con 200 ms para su trabajo. `desktop.OnQuitRequest` puede cancelar o aplazar un elemento de menú Salir del escritorio o `desktop.RequestQuit(cx)`. En Windows y Linux también se ejecuta después de `OnClosing` cuando cerrar una ventana terminaría la aplicación. `desktop.RequestRestart(cx)` consulta a los mismos manejadores antes de reiniciar; `desktop.Restart(cx)` sigue siendo inmediato. Los reinicios del actualizador en Linux y Windows también consultan, de modo que guardar puede cancelarlos o aplazarlos. Cuando el sistema informa de un cierre de sesión, un reinicio o un apagado, `q.Forced()` es true y la aplicación continúa a través de `OnQuit`. Comprueba `Forced()` antes de pedir una decisión al usuario. `cx.SetAppearance` fuerza el modo claro u oscuro, `cx.SetLocale` elige el idioma, `cx.OnKey` vincula un atajo de teclado, que reposa mientras lo que muestra su propietario está oculto o bajo un cuadro de diálogo salvo que `core.KeyOptions{OverModals: true}` lo mantenga, que toma su tecla antes que el menú de la aplicación salvo que `YieldsToMenu` la deje a un elemento de menú habilitado que tome el mismo atajo donde está el foco (mientras un campo de texto o una vista web tiene el foco, un elemento `MenuNotEditing` deja su tecla al vínculo y la retiene de los elementos posteriores con el mismo atajo, y el menú de Linux no toma ninguna tecla pulsada sin Control, Alt o una tecla de logotipo, así que allí el vínculo conserva esa tecla), que funciona en la ventana en la que está `cx` salvo que `AllSurfaces` lo vincule en todas las ventanas y paneles de la aplicación, también en los que se abran después, para un atajo de toda la aplicación, que reposa en todos ellos mientras lo que muestra su propietario está oculto en la ventana en la que está vinculado, y que deja a un control enfocado las teclas que usa, a un botón su Retorno y Espacio y a un control deslizante, una barra de pestañas o un grupo de opciones sus flechas, salvo que se vincule con `core.KeyAlways` o `core.KeyNotEditing`; `cx.OnOpenFiles` recibe los documentos abiertos en macOS, Windows, Linux e iOS. iOS importa los archivos normales a un almacenamiento temporal propiedad de la aplicación; muévelos a `cx.DataDir()` para conservarlos. En macOS, cerrar la ventana que proporcionó el menú de la aplicación restaura el menú de la ventana superviviente más reciente, o el predeterminado cuando no queda ninguna.

En Wayland, el seguimiento de la aplicación en primer plano requiere la versión 2 o posterior del protocolo wlr foreign-toplevel del compositor; otros compositores informan de `CanReadFrontmostApp` como false. Su ID es el ID de aplicación de Wayland, su nombre procede de la entrada de escritorio cuando está disponible y su PID es 0. Las aplicaciones empaquetadas de Windows usan su AUMID en lugar de una ruta de frame-host. macOS sigue el estado de presentación a pantalla completa de AppKit en lugar de deducirlo del tamaño de la ventana.

Un rol de edición en el menú de la aplicación toma el editor enfocado antes que su alternativa `OnSelect`. En macOS, Deshacer y Rehacer siguen el historial de ese editor, y Cortar sigue su selección, incluso mientras la aplicación está inactiva; sin editor, la alternativa permanece habilitada.

En iOS, da a `cx.OnKeyWith` una función `core.KeyOptions.Title` para ofrecer el atajo en el menú de teclado del sistema mientras el vínculo puede activarse. `Group` da nombre a su menú en iPadOS 26 y posterior. Las funciones pueden leer el catálogo de la aplicación para que los títulos de los atajos sigan el idioma. Usa `app.KeyCommands()` en las pruebas para inspeccionar la lista actual.

```go
cx.OnKey("mod+k", func() { palette.Set(true) })
cx.SetLocale("de")
cx.OnQuit(func(ctx context.Context) { saveDraft(ctx) })
```

`cx.Env().DeviceName()` es el nombre que el usuario dio al dispositivo, y `cx.Env().DeviceModel()` su modelo de hardware, como `Mac15,7`, `iPhone15,2` o `Google Pixel 8`, de modo que una aplicación puede distinguir dos dispositivos emparejados. macOS, Android, Windows y Linux dan ambos; iOS da el modelo, y el nombre solo a una aplicación con el entitlement de Apple para el nombre del dispositivo asignado por el usuario. watchOS da su identificador de hardware, HarmonyOS su nombre comercial o modelo de producto, y los miniprogramas la marca y el modelo de la plataforma; esos destinos dejan vacío el nombre asignado por el usuario. La web no da ninguno de los dos. Las pruebas los establecen con `nsgotest.Environment`.

`cx.Env().ThermalState()` informa de `core.ThermalNominal`, `ThermalFair`, `ThermalSerious` o `ThermalCritical` en las plataformas de Apple y en Android API 29 o posterior. `cx.Env().LowPowerMode()` informa de `core.LowPowerOn` o `LowPowerOff` en las plataformas de Apple, Android, Windows y sesiones de Linux con power-profiles-daemon. Donde el sistema no da ningún valor, las lecturas devuelven `ThermalUnknown` y `LowPowerUnknown`. Cada lectura sigue los cambios de forma independiente, de modo que una aplicación puede ajustar su carga de trabajo mientras se ejecuta. En las pruebas, establece `ThermalState` y `LowPowerMode` en `core.Environment` mediante `nsgotest.Environment` o `app.SetEnvironment`.

`desktop.CursorPoint(cx, func(p core.Point, available bool) { ... })` lee el puntero incluso fuera de las ventanas de la aplicación en macOS, Windows y X11. Toma una muestra por llamada. El punto está en las mismas coordenadas de pantalla que `desktop.Monitors(cx)` y `Window.ShowAt`. En Wayland y otros destinos no compatibles, o si el ámbito carece de `core.AccessCursorPoint`, `available` es false.

`desktop.SetWindowConstraints` limita el tamaño del contenido y controla el cambio de tamaño, la minimización y la maximización; establece `WindowOptions.Constraints` al abrir una ventana o usa `nsgo.WindowConstraints` para la ventana principal. Linux aplica máximos no fijos en X11 y elimina los botones de minimizar y maximizar prohibidos de la barra de título nativa de GTK en X11 y Wayland. Wayland actualmente solo admite tamaños fijos como límite superior.

`desktop.SetWindowLevel` hace flotar una ventana por encima de las demás, y `desktop.SetWindowWorkspaces(cx, desktop.WindowWorkspacesAll)` o `WindowOptions.Workspaces` la mantiene en todos los espacios de trabajo, como una mascota de escritorio, un HUD o un temporizador: en macOS en todos los Spaces y sobre los espacios a pantalla completa de otras aplicaciones, fuera de Cmd-`, y en Linux en todos los escritorios virtuales de un gestor de ventanas X11. Windows y Wayland la dejan en el espacio en el que se abrió, y `desktop.CanSetWindowWorkspaces` indica si tiene efecto: true en macOS y con un gestor de ventanas X11 que admita `_NET_WM_DESKTOP`. Una ventana principal pasa a todos los espacios de trabajo llamando a `desktop.SetWindowWorkspaces` desde su contenido. En las pruebas, `nsgotest.App.Window` enumera las solicitudes, y `nsgotest.Environment` establece `CanSetWindowWorkspaces`.

`desktop.RegisterHotKey` vincula un atajo global, que funciona mientras cualquier aplicación está en primer plano, en macOS, en Linux y en una aplicación de Windows ejecutada con `nsgo.Run(App)`. El `State` de su manejador indica si el sistema lo aceptó: `HotKeyPending` hasta que responde, luego `HotKeyRegistered`, `HotKeyTaken` cuando otro registro ocupa la combinación, o `HotKeyUnsupported` para una tecla o una plataforma sin atajos globales, de modo que un grabador de atajos puede pedir otra combinación. En macOS una combinación cuenta como ocupada solo cuando la aplicación ya la tiene o cuando otra aplicación la registró en exclusiva. En Wayland el compositor pide al usuario que vincule el atajo: queda registrado una vez que lo ha hecho, y ocupado cuando él o el compositor lo omiten. En las pruebas, `nsgotest.HotKeyTaken` y `nsgotest.HotKeyUnsupported` hacen que las combinaciones respondan así.

Con `desktop.TitleBarHidden`, Windows y Linux ofrecen el menú de la aplicación como menú emergente al pulsar y soltar Alt sola o con F10. Alt con la primera letra o dígito de la etiqueta de un nivel superior abre ese submenú o elige ese comando, en la escritura de la etiqueta. Los vínculos de la aplicación conservan sus teclas; Mayús+F10 sigue siendo la tecla del menú contextual, y las demás combinaciones con Alt quedan para el sistema. Escape cierra el menú emergente y restaura el foco del teclado.

```go
shortcut := desktop.RegisterHotKey(cx, "alt+space", launcher.Toggle)
warning := core.Text{ContentFunc: func() string {
	if shortcut.State() == desktop.HotKeyTaken {
		return "Another app uses Alt+Space"
	}
	return ""
}}
```

Los cambios de tamaño en el escritorio en macOS, Windows y Linux esperan hasta 16 ms al nuevo diseño antes de dibujar. Mantén rápidos los cálculos reactivos y los manejadores de eventos para que el diseño pueda cumplir ese plazo; realiza la E/S lenta en una goroutine y publica los resultados mediante `cx.Post`. Una aplicación ocupada sigue cambiando de tamaño y muestra su diseño cuando se pone al día.

`desktop.SetWindowFrame` mueve una ventana cuando `WindowFrame.Move` es true, usando `X` e `Y` en puntos desde la esquina superior izquierda de la pantalla principal. Las dimensiones sin establecer conservan su tamaño actual. Comprueba `desktop.CanPlaceWindows`: macOS, Windows y Linux en X11 admiten el posicionamiento; una ventana de nivel superior ordinaria de Wayland conserva la posición que le da el compositor. `desktop.Frame` sigue la posición de la ventana nativa en estos escritorios.

En macOS y Windows, `WindowFrame{Animated: true}` lleva suavemente un panel visible a su nuevo marco, incluido el tamaño de un panel acoplado. Los paneles ocultos y el movimiento reducido aplican el nuevo marco de inmediato. Las ventanas ordinarias de macOS usan la animación de AppKit.

Establece `desktop.WindowOptions.Parent` en una ventana abierta de la aplicación para cerrar ambas cuando se cierre la ventana padre. En los escritorios, la ventana hija permanece por encima de su ventana padre y la sigue cuando se oculta o se minimiza; una ventana hija ocultada explícitamente permanece oculta cuando la ventana padre vuelve. En macOS también se centra sobre su ventana padre cuando `Frame` no está establecido y la sigue a pantalla completa.

`WindowOptions.NoShadow` y `desktop.SetWindowShadow(cx, false)` también quitan la sombra a una ventana de Windows con la barra de título oculta, que dibuja su propio marco; una ventana con la barra de título del sistema conserva su marco y su sombra, igual que una ventana de Linux que decora su gestor de ventanas, donde `NoShadow` solo elimina la sombra de las decoraciones propias de GTK.

`desktop.SetWindowPointerEvents` deja pasar el puntero a través de una ventana hacia las ventanas que hay debajo, de cualquier aplicación: `WindowPointerNone` en todas partes, como hace una pantalla de visualización frontal, y `WindowPointerPainted` allí donde ninguna vista dibuja algo ni escucha al puntero, como alrededor de una mascota de escritorio que sigue recibiendo pulsaciones, arrastres y clics derechos sobre sí misma; la barra de título que dibuja el sistema cuenta como nada dibujado. `WindowOptions.PointerEvents` y `PanelOptions.PointerEvents` abren así una ventana. Una región `Desktop.WindowDrag` sigue arrastrando dentro de un ancestro pulsable; un control pulsable o de edición más cercano conserva su propia entrada en lugar de arrastrar la ventana. Una ventana que no recibe el puntero no oye pulsaciones, así que la aplicación lo devuelve desde un atajo, un icono de la bandeja u otra ventana. El código que tiene un `Access` necesita `core.AccessOverlay`.

```go
desktop.OpenPanel(cx, desktop.PanelOptions{Width: 600, Height: 400, PointerEvents: desktop.WindowPointerPainted}, Pet)
```

En macOS y Windows, una imagen en una ventana pintada puede establecer `Image.HitTestAlpha` (TSX `hitTestAlpha`) en un valor mayor que 0 y como máximo 1. Por ejemplo, 0.5 recibe el puntero en los píxeles de la imagen que son al menos medio opacos. El umbral también se aplica al `OnPress` de la imagen. Un fondo o un borde de vista sigue contando como pintado. Cero usa toda la vista de imagen, como hacen las imágenes en Linux.

En una ventana pintada de macOS, `Video.HitTestAlpha` (TSX `hitTestAlpha`) aplica el mismo umbral a los fotogramas de vídeo decodificados. Funciona con `Controls` desactivado y una pista de vídeo sin transformar, como HEVC con alfa para una mascota de escritorio. Cero, un fotograma no disponible y los demás anfitriones de escritorio usan los límites de la vista de vídeo.

`desktop.NewTray` puede dibujar un icono dinámico con `TrayOptions.View` en macOS y Windows. Windows ajusta la representación transparente de la vista al tamaño de icono del área de notificación y la actualiza cuando la vista cambia o una imagen termina de cargarse; usa `TrayOptions.OnClick`, `Panel` y `Menu` para sus acciones. Linux muestra actualmente iconos PNG; dibujar su vista de bandeja sigue siendo un objetivo.

`desktop.OpenPanel` abre un panel como los que muestra un atajo así o un icono de la bandeja: una ventana sin bordes por encima de las demás. Al igual que `desktop.OpenWindow`, `desktop.NewTray`, `mobile.OpenOverlay`, `audio.NewPlayer`, `audio.LoadSound` y `core.NewHolder`, pertenece directamente al `cx` que pasas y no al propietario del manejador en ejecución, de modo que pasar el ámbito de la aplicación desde el `OnPress` de un botón mantiene viva la ventana, el panel o el reproductor después de que desaparezca el ámbito de ese botón; las primitivas reactivas (`Effect`, `Cleanup`, `AfterFunc`, `OnKey`) pertenecen en cambio al propietario en ejecución, que `cx.Run` cambia. Una tarjeta emergente se dibuja a sí misma en un panel con `Transparent`, que no tiene fondo, marco ni sombra propios, de modo que solo se ven las esquinas redondeadas y la `BoxShadow` de la tarjeta, con espacio para la sombra dentro del panel; `FitContent` ajusta el tamaño del panel a su contenido, hasta `Width` y `Height`, antes de mostrarse por primera vez y a medida que cambia el contenido. `desktop.Transparent` indica si el panel no dibuja ahora nada propio, como ocurre con una ventana transparente, mientras que `desktop.CanMakeWindowsTransparent` solo indica si una ventana podría serlo: donde es false, como en Linux sin compositor, da un fondo al contenido.

En macOS, un panel sin `Transparent` usa esquinas rectas cuando alguna de sus dimensiones es inferior a 24 puntos y recupera sus esquinas redondeadas en tamaños normales.

```go
lookup := desktop.OpenPanel(cx, desktop.PanelOptions{Width: 380, Height: 520, Transparent: true, FitContent: true, HideOnBlur: true}, Card)
desktop.RegisterHotKey(cx, "alt+space", lookup.Toggle)
```

`desktop.NewTray` muestra `TrayOptions.Title` junto a su imagen o su vista dibujada en macOS. Para un menú emergente personalizado, lee `tray.ClickBounds()` dentro de `OnClick` o `Menu` y pasa el rectángulo disponible a `panel.ShowAt(bounds, core.PlaceBottom)`. Usa los mismos puntos de pantalla que `Env.Displays` e incluye el título. Los límites no están disponibles antes del primer clic, después de la eliminación, en Linux y cuando el shell de Windows no puede proporcionarlos. Cada clic sustituye los límites anteriores, de modo que un resultado no disponible nunca reutiliza un rectángulo antiguo. Establecer `TrayOptions.Panel` mantiene en cambio el comportamiento integrado de alternar al hacer clic.

`PanelOptions.Dock` mantiene un panel centrado a lo largo del borde utilizable de una pantalla a través de `Show`, `Focus`, `Toggle` y los cambios de tamaño del contenido. Establece `Edge` en `core.PlaceTop`, `PlaceBottom`, `PlaceLeft` o `PlaceRight`, `Display` en un ID de `Env.Displays` (vacío para la pantalla principal) y `Margin` en la distancia a ese borde en puntos. Un panel acoplado no puede usar `ShowAt` ni `MoveTo`; `SetWindowFrame` solo cambia su tamaño. Un icono de la bandeja lo muestra en su posición acoplada en lugar de bajo el icono.

```go
panel := desktop.OpenPanel(cx, desktop.PanelOptions{
	Width: 320, Height: 80, NoActivate: true,
	Dock: desktop.PanelDock{Edge: core.PlaceBottom, Margin: 12},
}, Candidates)
panel.OnPlacementError(cx, func(err error) { /* offer another presentation */ })
panel.Show()
```

`desktop.CanDockPanels(cx)` informa de la compatibilidad con el acoplamiento por separado del posicionamiento arbitrario de ventanas. macOS, Windows y X11 lo admiten; Wayland necesita la biblioteca opcional en tiempo de ejecución `libgtk4-layer-shell.so.0` y un compositor que implemente `zwlr_layer_shell_v1`. NSGo carga esa biblioteca antes que GTK; un anfitrión de integración que ya haya cargado Wayland debe precargar gtk4-layer-shell antes de iniciar el anfitrión. Si falta la biblioteca, el protocolo o la pantalla seleccionada, el panel queda oculto y se llama a `OnPlacementError`. Un panel acoplado en Wayland usa la capa superior sin reservar espacio de trabajo: `NoActivate` deja el teclado a la ventana actual, mientras que un panel sin él toma el foco del teclado en exclusiva hasta que se oculta. Las ventanas ordinarias de Wayland conservan su posicionamiento habitual por parte del compositor.

Un grabador de atajos permite al usuario elegir una tecla así. El `OnKeyDown` de una vista `Focusable` oye cada tecla que se pulsa mientras tiene el foco, antes que los vínculos de la aplicación, con los modificadores mantenidos, incluido un modificador pulsado solo; devolver `true` retiene la tecla de los vínculos, de modo que grabar Comando+S no guarda. `KeyEvent.Shortcut` escribe la pulsación tal como la aceptan `OnKey` y `RegisterHotKey`, cada modificador por su nombre ("ctrl+shift+k", "alt+space", "shift+meta+k" para Comando+Mayús+K), y "" para un modificador solo. Su `OnKeyUp` oye cómo se suelta cada una de esas teclas, incluso después de que `OnKeyDown` la consumiera o el foco se moviera, con los modificadores aún mantenidos. Ambos funcionan en macOS, Windows, Linux, la web, donde el navegador se reserva los atajos que no cede a ninguna página, Apple TV y en `nsgotest`; Android entrega los eventos del teclado físico a las vistas enfocadas de forma asíncrona, así que el valor devuelto por su `OnKeyDown` no puede consumir la tecla. iPhone y iPad admiten la consulta de teclas físicas. Un atajo que la aplicación registró globalmente toma su tecla antes que la vista, así que un grabador lo anula mientras graba.

```go
core.View{
	Focusable: func() bool { return true },
	OnKeyDown: func(e core.KeyEvent) bool {
		if s := e.Shortcut(); s != "" {
			shortcut.Set(s)
		}
		return true
	},
	OnKeyUp: func(core.KeyEvent) { recording.Set(false) },
	Children: []core.Node{core.Text{ContentFunc: shortcut.Get}},
}
```

Un juego o un lienzo consulta las teclas en su lugar. `core.KeysDown(cx)` enumera las teclas mantenidas en la ventana y `core.KeyHeld(cx, "left")` indica si una lo está: lecturas con seguimiento, que también lee un bucle de `RequestAnimationFrame`. La primera lectura hace que la plataforma informe de cada tecla a partir de ese momento, de modo que una tecla ya mantenida sigue siendo desconocida hasta que se pulsa de nuevo; las teclas escritas en un campo o en una vista web, o mientras compone un método de entrada, nunca están mantenidas, y una tecla que sigue mantenida cuando la ventana pierde el foco se suelta en ese momento. Funcionan donde funciona `OnKeyUp` y en iPhone y iPad con un teclado físico.

Para los botones del ratón, `View.OnMouseDown` y `View.OnMouseUp` entregan un `core.MouseButtonEvent` para cada botón (`core.MouseButtonLeft`, `core.MouseButtonRight`, `core.MouseButtonMiddle`, `core.MouseButtonBack`, `core.MouseButtonForward`) en macOS, Windows, Linux, la web y `nsgotest`, sin convertir la vista en una parada de foco del teclado. `core.MouseButtonsDown(cx)` y `core.MouseButtonHeld(cx, core.MouseButtonRight)` son lecturas con seguimiento de los botones mantenidos en la ventana, que se borran automáticamente cuando la ventana pierde el foco.

Para un control ligado a una posición física, lee `KeyEvent.Code`, `core.KeyCodesDown(cx)` o `core.KeyCodeHeld(cx, "KeyW")`. El código usa los nombres de `KeyboardEvent.code`: en AZERTY, la posición de la W de QWERTY tiene `Code: "KeyW"` y `Key: "z"`. `core.KeyCodeLabel(cx, "KeyW")` da la etiqueta visible de la distribución activa para un mensaje. En iOS, Android, HarmonyOS y navegadores sin mapa de distribución, usa el último nombre de tecla notificado en esa posición y devuelve una cadena vacía hasta que se ve una tecla. HarmonyOS nombra tanto la posición como la tecla a partir del `KeyCode` de InputKit, ya que ArkUI no informa de ningún código de exploración, así que allí el nombre de la tecla aún no sigue una distribución distinta de QWERTY.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if core.KeyHeld(cx, "left") {
		x.Update(func(x float32) float32 { return x - 4 })
	}
	if core.KeyHeld(cx, "right") {
		x.Update(func(x float32) float32 { return x + 4 })
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

## Mantener la pantalla encendida

`cx.KeepAwake("Playing video", core.KeepAwakeOptions{Display: true})` mantiene la pantalla encendida hasta que se ejecuta la función de liberación devuelta o se desecha el ámbito actual. En Android la retención se aplica mientras la aplicación es visible. En la web funciona en un contexto seguro con compatibilidad con Screen Wake Lock, y la página vuelve a solicitar el bloqueo cuando se hace visible. Safari puede requerir una pulsación o una tecla antes de conceder la primera solicitud. Las retenciones solo del equipo no tienen efecto en estas dos plataformas.

## Bloqueo del puntero

En una cámara de juego o un arrastre que debe continuar en el borde de la pantalla, llama a `ref.LockPointer()` desde una pulsación en la vista que tiene la ref. El puntero permanece en su sitio y oculto mientras la vista recibe el movimiento relativo en `OnPointerDelta(dx, dy)`. Los valores positivos se mueven hacia la derecha y hacia abajo.

Llama a la función devuelta para liberar el bloqueo. Escape, una ventana que pierde el foco, u ocultar o eliminar la vista también lo liberan. Comprueba `cx.Env().CanLockPointer()` para ver si la superficie lo admite. La web requiere un gesto del usuario, como una pulsación. Linux en Wayland requiere un compositor que admita restricciones del puntero y movimiento relativo.

## Límites de sonido

Usa `audio.SetSoundLimits(cx, audio.SoundLimits{MaxSounds: 200, MaxDuration: 20 * time.Second})` antes de cargar sonidos para cambiar los límites del motor. También se pueden establecer `MaxEncodedBytes`, `MaxDecodedBytes` y `MaxPlaybacks`. Los campos a cero usan los valores predeterminados de 8 MiB codificados y 5 segundos por sonido, 64 fuentes, 16 MiB decodificados entre todos los sonidos y 32 reproducciones superpuestas por salida. Los sonidos existentes permanecen cargados y cuentan para los nuevos límites. Los ámbitos restringidos mantienen los límites predeterminados dentro del presupuesto del motor: los sonidos `nsgo/audio` de un paquete TSX firmado los alcanzan sea lo que sea que haya establecido la aplicación Go, mientras que el TSX propio de la aplicación toma los límites de la aplicación Go. El SoundPool de Android reproduce como máximo 32 sonidos a la vez y decodifica como máximo 1 MiB de cada uno, sean cuales sean los límites.

## Pantalla completa

`desktop.SetFullScreen(cx, true, displayID)` ocupa la pantalla nombrada por `cx.Env().Displays()` en macOS, Windows y Linux. Omite el ID para usar la pantalla actual de la ventana. Un ID desconocido deja la ventana donde está. Repetir la misma solicitud no tiene efecto. `desktop.SetFullScreen(cx, false)` sale de la pantalla completa y restaura el marco anterior de la ventana. En la web, entra desde un manejador de pulsación. El navegador elige la pantalla, y `cx.Env().FullScreen()` sigue la entrada y la salida, incluida Esc.

## Pantallas

`cx.Env().Display()` devuelve el `core.DisplayInfo` actual de la ventana y actualiza los enlaces cuando la ventana se mueve a otra pantalla. `cx.Env().Displays()` enumera las pantallas que informa la plataforma. `RefreshRate` y `MaxRefreshRate` están en Hz, con cero para una frecuencia no disponible. iOS informa del máximo del panel. HarmonyOS informa de la pantalla de la ventana, su frecuencia actual y su frecuencia máxima admitida. La web informa de la pantalla actual y marca la medición de su función de retorno de fotograma con `RefreshRateEstimated`. En las pruebas, establece `Displays` y `DisplayID` mediante `nsgotest.Environment`.

## Leer texto en voz alta

`cx.Speak` lee texto en voz alta con la voz del sistema para un idioma donde `cx.Env().CanSpeak()` indica que la plataforma habla: macOS, iOS, tvOS, Android, Windows, la web y Linux con speech-dispatcher. Se pronuncia un enunciado a la vez: otro `Speak` corta el que se está pronunciando, igual que `cx.StopSpeaking` y el fin del ámbito que lo pronunció. `done` oye una vez cómo terminó: finalizado, detenido o fallido con un `core.SpeakError`, como `core.SpeakLanguageUnavailable` cuando ninguna voz instalada habla el idioma.

```go
cx.Speak("Guten Morgen", core.Voice{Lang: "de-DE", Rate: 0.8}, func(stopped bool, err error) {
	speaking.Set(false)
})
```

`Rate` y `Pitch` van de 0.5 a 2, siendo 1 el valor propio de la voz. `cx.Voices("ja", ...)` enumera las voces instaladas para un idioma, de modo que la aplicación puede pedir al usuario que instale una que falte o dejarle elegir una, cuyo `ID` va en `core.Voice`. Los navegadores solo hablan después de que el usuario haya pulsado algo en la página, y antes fallan con `core.SpeakNotAllowed`. Los componentes Go de un paquete TSX solo hablan con la capacidad `text_to_speech`.

## Audio en bucle

Establece `audio.Voice.Loop` para repetir la reproducción de un sonido hasta `Stop` o `Close`. Para música en streaming, establece `audio.PlayerOptions.LoopStart` y llama a `SetLoop(true)`. La primera pasada incluye la introducción. Las pasadas posteriores empiezan en el desplazamiento. Un desplazamiento cero repite la pista completa.

En macOS 14 e iOS 17 o posterior, una pista finita usa la cola nativa para preparar cada repetición antes de que termine la actual. Las versiones anteriores de Apple saltan al inicio del bucle cuando termina la pasada actual, con una breve pausa. En Linux y la web, un reproductor en bucle decodifica en memoria una pista de hasta 32 MiB y 5 minutos, unos 23 MB por minuto, y la repite sin pausa. En Linux, una pista más grande o más larga se transmite a través de su propio decodificador y también se repite sin pausa; en la web salta hacia atrás al final, con una breve pausa. En Windows y Android, un bucle con inicio salta hacia atrás hasta él al final, con una breve pausa. En la web, un reproductor en bucle a una velocidad distinta de 1 también salta hacia atrás al final, ya que la reproducción sin pausas del navegador desplazaría el tono con la velocidad. En la web, un reproductor en bucle, como un sonido, solo reproduce después de que el usuario haya pulsado algo en la página, y antes falla con `NotAllowed`.

## Biometría

`cx.BiometricAvailability` informa de si hay registrados Face ID, Touch ID o una huella dactilar del dispositivo; Android no indica qué tipo está registrado, así que allí informa de una huella dactilar cuando hay una registrada y, en caso contrario, de rostro cuando el dispositivo tiene hardware facial. `cx.AuthenticateBiometric` pide al sistema que confirme al usuario con un motivo, que no debe estar vacío, e informa de éxito, fallo, cancelación o falta de disponibilidad. No elige un código de acceso alternativo para la aplicación.

```go
cx.BiometricAvailability(func(kind core.BiometricKind) {
	if kind != core.BiometricNone {
		cx.AuthenticateBiometric("Confirm deleting this account", func(result core.BiometricResult) {
			if result == core.BiometricSuccess {
				deleteAccount()
			}
		})
	}
})
```

Las aplicaciones con Face ID declaran `NSFaceIDUsageDescription` en `apple.info`; las aplicaciones Android incluyen `USE_BIOMETRIC` en `android.permissions`.

## Inicio de sesión

`cx.AuthSession` inicia la sesión del usuario con un proveedor OAuth en la sesión de autenticación del sistema en lugar de en una vista web: `ASWebAuthenticationSession` en macOS, iOS y watchOS, una Custom Tab o el navegador en Android, el navegador predeterminado en Windows, Linux y un Mac sin esquema, y una ventana emergente en la web. En la web, la página de redirección también envía su retorno por un `BroadcastChannel` del mismo origen, de modo que el inicio de sesión se completa cuando la `Cross-Origin-Opener-Policy` del proveedor separa la ventana emergente de la página. Esa ventana emergente aparece como cerrada para la página, así que cerrar una ventana emergente después de que mostrara la página del proveedor termina la sesión como cancelada solo al cabo de 5 minutos; da a la sesión un botón de cancelar. La aplicación se encarga del protocolo: genera el verificador PKCE y el estado, construye la URL de autorización para la redirección que se le da y comprueba el retorno antes de canjear su código. Cancelar no es fallar: `errors.Is(err, core.AuthCancelled)` indica que el usuario cerró la sesión, o que la función `cancel` devuelta la terminó.

```go
verifier, challenge, state := newPKCE() // the app's own
cx.AuthSession(func(redirect string) string {
	q := url.Values{"client_id": {clientID(redirect)}, "redirect_uri": {redirect}, "response_type": {"code"}, "scope": {"openid email"},
		"state": {state}, "code_challenge": {challenge}, "code_challenge_method": {"S256"}}
	return "https://accounts.example.com/authorize?" + q.Encode()
}, core.AuthSessionOptions{Scheme: "com.example.app", Path: "/oauth2redirect", Timeout: 5 * time.Minute}, func(callback string, err error) {
	if errors.Is(err, core.AuthCancelled) {
		return
	}
	if err != nil {
		showError(err)
		return
	}
	u, _ := url.Parse(callback)
	if u.Query().Get("state") == state {
		exchangeCode(u.Query().Get("code"), verifier)
	}
})
```

La redirección es `com.example.app:/oauth2redirect` cuando hay un esquema establecido (Windows, Linux y Android lo necesitan en los `url_schemes` de `nsgo.yaml` para que el sistema operativo entregue el retorno a la aplicación en ejecución; la sesión del sistema de macOS, iOS y watchOS lo captura sin esa declaración), y `http://127.0.0.1:<port>/oauth2redirect` en el loopback de los escritorios, así que registra ambas con el proveedor, a menudo como clientes independientes. Deja `Scheme` vacío para usar solo loopback, o establécelo en `https` con `Host` para el dominio asociado de la aplicación, que capturan iOS 17.4, macOS 14.4 y watchOS 10.4. watchOS requiere un esquema o un dominio HTTPS asociado; no tiene alternativa de loopback, y sigue aplicándose el mínimo de watchOS 26 existente en NSGo. Una pestaña del navegador cerrada no indica nada a una sesión de loopback, así que dale un `Timeout` o un botón de cancelar. `Ephemeral` pide a macOS, iOS y watchOS que no compartan las cookies del navegador, y solicita una Custom Tab privada en Android cuando el navegador la admite; no des por hecho el aislamiento de cookies en Android. tvOS y los miniprogramas responden `core.AuthUnavailable` por ahora. En TSX, `AuthSession.start(url, options)` necesita la capacidad `auth_session` y da una sesión cuyo `result` se resuelve con el retorno, o con null una vez cancelada.

Android consume un retorno coincidente antes que los `OnOpenLinks` ordinarios, y volver sin un retorno termina la sesión como cancelada. Su retorno debe usar un esquema privado propiedad de la aplicación; un esquema vacío, un retorno HTTPS o un esquema no declarado devuelve `core.AuthUnavailable`, igual que un entorno de ejecución de Android compilado antes de las sesiones de inicio de sesión.

Una sesión de navegador consume solo su enlace de retorno coincidente; los enlaces no relacionados y tardíos siguen llegando a `OnOpenLinks`. Iniciar otra sesión de navegador para el mismo URI de retorno cancela la anterior; URI distintos pueden esperar en ventanas diferentes.

Usa la redirección exacta que se pasa a la función de URL de autorización. Las redirecciones de la web y de loopback del escritorio codifican las rutas Unicode y resuelven los segmentos de punto antes de que la función las reciba; un `Path` de loopback es texto decodificado, así que también se escapa un `%` literal.

Los miniprogramas usan en su lugar `cx.SignInMiniProgram(core.MiniProgramSignInOptions{}, done)`. La función de retorno recibe una `core.MiniProgramCredential` con su `Platform` (`MiniProgramWeChat`, `MiniProgramQQ`, `MiniProgramDouyin` o `MiniProgramAlipay`) y un `Code` no vacío de corta duración. Envíalo a tu servidor para el intercambio de código del anfitrión; guarda allí el secreto de la aplicación. Esto es `wx.login`/`qq.login`/`tt.login`, o la autorización básica `auth_base` de Alipay, sin solicitar información del perfil. No abre una URL ni ejecuta un constructor de URL de OAuth.

`cx.Env().CanSignInMiniProgram()` informa de la disponibilidad de la API del anfitrión. Los demás destinos responden `core.MiniProgramSignInUnavailable`; el código sin `core.AccessMiniProgramSignIn` obtiene `AccessDenied`. Douyin pregunta de forma silenciosa de forma predeterminada: un anfitrión sin sesión iniciada devuelve `MiniProgramSignInNotSignedIn`, y `MiniProgramSignInOptions{Prompt: true}` permite su ventana de inicio de sesión de la cuenta del anfitrión. Cerrar esa ventana devuelve `MiniProgramSignInCancelled`. Un código anónimo de dispositivo nunca cuenta como un usuario con sesión iniciada. Las pruebas establecen `nsgotest.Environment(core.Environment{MiniProgramSignIn: true})` y `app.MiniProgramSignIn`; `app.MiniProgramSignIns` registra las solicitudes. TSX no se ejecuta en los miniprogramas.

Una aplicación cuyas vistas web deban tener iniciada la sesión del usuario tal como ya la tiene en otro lugar, como en su propio navegador, escribe las cookies de ese inicio de sesión con `cx.SetWebCookies`, incluidas las HttpOnly, que los scripts de la página no pueden establecer, y luego carga las páginas: done se ejecuta una vez que el almacén las contiene. El almacén es el persistente que comparten todas las vistas web de la aplicación, así que las cookies con `Expires` sobreviven a la aplicación; las vistas de incógnito no las ven, salvo en Android, que mantiene un almacén por aplicación. HarmonyOS, la web, tvOS y watchOS responden `Unavailable`.

```go
cx.SetWebCookies([]core.WebCookie{
	{Name: "SID", Value: sid, Domain: ".example.com", Expires: expires, Secure: true, HTTPOnly: true},
}, func(err error) {
	if err == nil {
		page.Set(core.WebSrc("https://www.example.com/"))
	}
})
```

Para eliminar una cookie, escríbela de nuevo con el mismo nombre, dominio y ruta y un `Expires` en el pasado; una caducidad cero crea una cookie de sesión. Espera a la función de retorno antes de cargar una página que deba observar la eliminación.

A la inversa, `cx.WebCookies` enumera las cookies del almacén, incluidas las HttpOnly y su dominio, ruta, caducidad y marcas, que `document.cookie` no puede leer, se esté mostrando o no una página del sitio: un `core.WebCookieFilter` elige las que enviaría una petición a una `URL`, o las de un `Domain` y de todos los hosts que dependen de él, y el filtro cero todas. Una aplicación que llama al sitio desde Go con el inicio de sesión del usuario, o que muestra dónde tiene iniciada la sesión el usuario, las lee de esta forma. `cx.ClearWebsiteData` cierra la sesión del usuario en todos los sitios: elimina las cookies del almacén, el almacenamiento local y de sesión, IndexedDB, los service workers y las cachés, y done se ejecuta una vez que han desaparecido. El `CookieManager` de Android solo responde con los nombres y valores que enviaría una URL, así que allí solo enumera un filtro `URL`, que rellena `Domain` con su host y deja vacíos los demás campos, y un filtro `Domain` o el filtro cero responden `Unavailable`; borrar allí deja la caché HTTP. HarmonyOS, la web, tvOS y watchOS responden `Unavailable` a ambos.

```go
cx.WebCookies(core.WebCookieFilter{Domain: "example.com"}, func(cookies []core.WebCookie, err error) {
	signedIn.Set(err == nil && slices.ContainsFunc(cookies, func(c core.WebCookie) bool { return c.Name == "SID" }))
})
```

## Iniciar sesión con Apple

`cx.SignInWithApple` muestra la hoja Iniciar sesión con Apple del sistema desde el propio botón de la aplicación en macOS, iOS y tvOS, donde `cx.Env().CanSignInWithApple()` es true. Responde una vez con la credencial, o con un error: `core.AppleSignInCancelled` cuando el usuario cierra la hoja, que no es un fallo, y en otro caso `AppleSignInFailed` o, en otras plataformas, `AppleSignInUnavailable`. Apple da el correo electrónico y el nombre solo en el primer inicio de sesión del usuario en la aplicación, así que guárdalos en ese momento. `Nonce` entra en la solicitud como su SHA-256 en hexadecimal en minúsculas, que contiene la declaración `nonce` del token de identidad: el servidor calcula el hash del nonce que dio a la aplicación y lo compara. `cx.AppleCredentialState` indica si el usuario la ha revocado desde entonces. En las pruebas sin interfaz gráfica, `app.AppleSignIn` recibe una `nsgotest.AppleSignInRequest` y proporciona la credencial o el error.

```go
nonce := rand.Text() // also sent to the server, which checks the token's claim against its SHA-256
cx.SignInWithApple(core.AppleSignInOptions{FullName: true, Email: true, Nonce: nonce}, func(c core.AppleCredential, err error) {
	switch {
	case errors.Is(err, core.AppleSignInCancelled):
	case err != nil:
		showError(err)
	default:
		signInOnServer(c.IdentityToken, c.AuthorizationCode, nonce, c.GivenName, c.FamilyName, c.Email)
	}
})
cx.AppleCredentialState(savedUser, func(s core.AppleCredentialState, err error) {
	if err == nil && s == core.AppleCredentialRevoked {
		signOut()
	}
})
```

La aplicación debe estar firmada con el entitlement de Iniciar sesión con Apple, que `build` añade cuando `nsgo.yaml` lo solicita; sin él la hoja falla:

```yaml
apple:
  sign_in_with_apple: true  # com.apple.developer.applesignin, in iOS and tvOS packages and Mac App Store builds
```

Xcode registra la capacidad para las compilaciones de dispositivo de `nsgo dev`; un perfil de aprovisionamiento en `sign.profile` o `sign.macos_profile` debe proceder de un App ID con Iniciar sesión con Apple activado. macOS solo la concede con un perfil, así que una imagen de disco o una ejecución de `nsgo dev` en un Mac no pueden iniciar sesión, ni tampoco el simulador, cuyas compilaciones no reclaman ningún entitlement de desarrollador.

## Compras dentro de la aplicación

`cx.Products`, `cx.Purchase` y las llamadas que las acompañan venden a través de la App Store en macOS, iOS y tvOS, donde `cx.Env().CanPurchase()` es true, en una compilación cuyo `nsgo.yaml` solicita StoreKit:

```yaml
apple:
  purchases: true  # StoreKit's library, in macOS, iOS and tvOS packages; no entitlement is needed
```

Muestra los precios que da la tienda, `DisplayPrice`, nunca precios propios de la aplicación: la tienda fija los precios de cada escaparate por sí misma. Una compra responde una vez, con la transacción o con un error: `core.PurchaseCancelled` cuando el usuario cierra la hoja, `core.PurchasePending` mientras espera aprobación, como ocurre con Preguntar para comprar, y en otro caso `core.PurchaseFailed`, con las palabras de la tienda, o `PurchasesUnavailable` en otras plataformas. Envía el `Signed` de la transacción, el JWS de la tienda, al servidor de la aplicación para verificarlo, entrega lo que se compró y luego finalízala; la tienda vuelve a entregar a la aplicación una transacción sin finalizar en cada inicio.

`cx.ManageSubscriptions` abre la interfaz de gestión de suscripciones de Apple (la hoja del sistema en iOS, la página de suscripciones de la App Store en macOS y tvOS). `cx.RequestRefund` inicia una solicitud de reembolso para una transacción y responde `core.RefundSuccess` o `core.RefundCancelled`.

```go
cx.OnPurchaseUpdate(func(t core.Transaction) { // renewals, approved purchases, other devices', refunds, and unfinished ones at launch
	verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
})
cx.Products([]string{"com.example.pro.monthly"}, func(products []core.Product, err error) {
	if err == nil && len(products) > 0 {
		price.Set(products[0].DisplayPrice)
	}
})
cx.Purchase("com.example.pro.monthly", core.PurchaseOptions{AccountToken: userUUID}, func(t core.Transaction, err error) {
	switch {
	case errors.Is(err, core.PurchaseCancelled):
	case errors.Is(err, core.PurchasePending):
		showWaiting() // its transaction comes through OnPurchaseUpdate
	case err != nil:
		showError(err)
	default:
		verifyAndDeliver(t.Signed, func() { cx.FinishTransaction(t.ID, nil) })
	}
})
```

`FinishTransaction` devuelve `PurchaseNotFound` para una transacción ya finalizada, incluida una devuelta por `CurrentEntitlements`; un artículo no consumible finalizado sigue siendo propiedad del usuario.

Registra `OnPurchaseUpdate` al iniciarse la aplicación: las actualizaciones que llegan antes de cualquier manejador esperan al primero. `cx.CurrentEntitlements` responde con lo que posee ahora el usuario, para desbloquearlo al iniciar, y un botón Restaurar compras llama a `cx.SyncPurchases`, que puede pedir al usuario que inicie sesión. `AccountToken` es un UUID propio de la aplicación que llevan la transacción y las notificaciones del servidor de la App Store. macOS solo vende en una compilación para la Mac App Store, y un App Clip no vende nada; Android, watchOS, Windows, Linux, la web y los miniprogramas responden `PurchasesUnavailable` por ahora. Prueba las compras con `nsgotest`, cuyos `Products`, `PurchaseAnswer`, `UpdatePurchase` y `UnfinishedTransactions` sustituyen a la tienda. `app.PurchaseRequests` registra las operaciones de la tienda como valores `nsgotest.PurchaseRequest`. En TSX, el módulo `Purchases` necesita la capacidad `purchases`: `purchase` se resuelve con `{ status: "purchased", transaction }`, `"pending"` o `"cancelled"`, y `on("update", listener)` oye las actualizaciones.

## Orientación de la interfaz

Llama a `cx.RequestOrientation(core.OrientationLandscape)` en el ámbito de un reproductor a pantalla completa para pedir a iOS, Android, HarmonyOS o al navegador la orientación horizontal. Tiene efecto la solicitud más reciente cuyo ámbito sigue vivo. Cuando ese ámbito termina, se reanuda la solicitud anterior o la aplicación vuelve a su política de orientación habitual.

En iOS 16 y posterior, la solicitud puede girar la interfaz de inmediato. En versiones anteriores de iOS restringe la siguiente rotación del sistema. Las `IOS.SupportedOrientations` de un modal presentado tienen prioridad. En Android y HarmonyOS, una combinación de direcciones verticales y horizontales que el sistema no puede representar se reduce a una única dirección permitida, dando preferencia a la que ya se muestra. Android puede ignorar las solicitudes de orientación en los modos de pantalla grande o de varias ventanas.

En una aplicación web, llama a `desktop.ToggleFullScreen(cx)` desde un manejador de pulsación que abra el ámbito del reproductor. El navegador entra en pantalla completa a partir de la pulsación y luego NSGo aplica la solicitud de orientación del reproductor. Salir de la pantalla completa libera el bloqueo. La consola del navegador informa de las operaciones no admitidas y de los rechazos.

`nsgotest.App.Orientation` registra la última máscara solicitada, con cero para la política de orientación habitual.

## Reseñas en la tienda

`cx.RequestReview()` solicita un aviso de valoración dentro de la aplicación a la App Store en macOS e iOS, o a Google Play en Android. `cx.Env().CanRequestReview()` informa de si el servicio está disponible. La tienda decide si muestra el aviso y cuándo, y no informa de ningún resultado. Usa una compilación de desarrollo para probar el aviso de Apple. Las compilaciones de TestFlight lo suprimen.

Los paquetes independientes de Android incluyen el SDK de Play Review, su actividad de diálogo y la consulta de visibilidad de Play Store; compilarlos no necesita el SDK de Android ni el JDK. Un dispositivo sin una Play Store habilitada informa de que el servicio no está disponible. Prueba un paquete publicado mediante [Internal App Sharing](https://developer.android.com/guide/playcore/in-app-review/test).

Al integrar NSGo, añade `com.google.android.play:review:2.0.2` al anfitrión Gradle. En el manifiesto del anfitrión, añade `<package android:name="com.android.vending" />` dentro de `<queries>` para que la aplicación pueda ver la Play Store. Si el anfitrión reduce su código, conserva las API de Play Review y Tasks con las reglas siguientes, ya que NSGo las carga por nombre.

```proguard
-keep class com.google.android.play.core.review.ReviewManagerFactory { *; }
-keep interface com.google.android.play.core.review.ReviewManager { *; }
-keep class com.google.android.play.core.review.ReviewInfo { *; }
-keep class com.google.android.gms.tasks.Task { *; }
-keep interface com.google.android.gms.tasks.OnCompleteListener { *; }
```

En las pruebas, establece `CanRequestReview` en `nsgotest.Environment` y comprueba `nsgotest.App.ReviewRequests`.

## Gestos de borde del sistema

Un juego con controles táctiles en el borde de la pantalla puede llamar a `mobile.DeferSystemGestures` mientras los controles están activos. Usa la función devuelta para liberar la solicitud antes de tiempo. También termina cuando se limpia su ámbito. Las solicitudes de varios ámbitos en la misma ventana se combinan.

```go
release := mobile.DeferSystemGestures(cx, mobile.SystemGestureOptions{
	IOSEdges: mobile.GestureEdgeBottom,
	AndroidRects: []core.Bounds{{
		Origin: core.Point{Y: 120},
		Size:   core.Size{Width: 24, Height: 96},
	}},
})
```

iOS usa los bordes físicos de la pantalla. Android 10 y posterior usan rectángulos en puntos de superficie medidos desde la esquina superior izquierda. Mantén los rectángulos alrededor de los controles que los necesitan. Android se reserva su gesto de inicio y limita el área que una aplicación puede excluir del gesto Atrás. `nsgotest.App.SystemGestures` da a las pruebas la solicitud combinada actual en su superficie principal. Usa `Surface.SystemGestures()` para otra ventana.

## Superposiciones en Android

`mobile.OpenOverlay` muestra una vista de la aplicación sobre otras aplicaciones en Android, un árbol propio que comparte las señales de la aplicación, y un servicio en primer plano mantiene la aplicación en ejecución mientras se muestra. La aplicación incluye `SYSTEM_ALERT_WINDOW` en `android.permissions`, y el usuario lo concede en los ajustes del sistema, que abre `mobile.RequestOverlayPermission`; sin él, la superposición termina de inmediato y `OnEnd` oye el motivo. Recibe toques, pero deja las teclas y los toques a su alrededor a la aplicación en primer plano, salvo que sea `Focusable` o `Passive`, y se mueve con `desktop.SetWindowFrame` desde su contenido. Las demás plataformas no abren ninguna.

```go
mobile.RequestOverlayPermission(cx, func(p core.OverlayPermission) {
	if p == mobile.OverlayGranted {
		mobile.OpenOverlay(cx, mobile.OverlayOptions{Width: 200, Height: 80}, Score)
	}
})
```

## Ubicación y movimiento

`cx.Locate` obtiene una posición, `cx.WatchLocation` transmite posiciones de forma continua y `cx.WatchMotion` transmite muestras del acelerómetro, el giroscopio, el magnetómetro y del movimiento del dispositivo fusionado. Cada uno devuelve un `*core.SensorStream` cuyo `Stop` lo cancela, igual que el fin del ámbito que lo inició; las funciones de retorno se ejecutan en la goroutine del motor. Los permisos son `core.PermissionLocation`, `PermissionBackgroundLocation` y `PermissionMotion` ([permisos](permissions.md)): iniciar un flujo nunca pregunta, y uno sin su permiso falla con `core.SensorNotAllowed`.

```go
cx.RequestPermission(core.PermissionLocation, func(s core.PermissionStatus) {
	if !s.Allowed() {
		return
	}
	cx.Locate(core.LocationOptions{}, func(f core.LocationFix, err error) {
		if err == nil {
			showPosition(f.Latitude, f.Longitude, f.Approximate)
		}
	})
})
```

`Locate` espera una posición nueva, 30 segundos de forma predeterminada (`Timeout`); solo `MaxAge` le permite tomar una en caché, lo que indica con `Cached` y `Age`. `Approximate` señala una posición que el usuario solo permitió de forma aproximada; Android actualiza esa posición como mucho cada diez minutos, así que allí da a `Locate` un `MaxAge` para ella. Las coordenadas son WGS 84; una medida de la que carece el proveedor tiene `Valid` en false, y `Altitude` es elipsoidal mientras que `MSLAltitude` es sobre el nivel medio del mar. El movimiento usa los ejes naturales del dispositivo, la aceleración en m/s² (+9.81 en z boca arriba), la rotación en rad/s y el campo magnético en µT, cada 20 ms de forma predeterminada, limitado a entre 10 ms y 1 s. Con carga, un flujo conserva la muestra más reciente por sensor y cuenta las que sustituyó en `Dropped`.

Los flujos continúan mientras la aplicación está inactiva y se pausan en segundo plano, marcando la primera muestra posterior con `Gap`; un `Locate` pendiente conserva el resto de su tiempo de espera. `LocationOptions.Background` mantiene un flujo de ubicación activo en segundo plano con `PermissionBackgroundLocation`: las aplicaciones de Apple añaden el modo en segundo plano `location`, y en Android un servicio en primer plano muestra una notificación con `NotificationTitle` y `NotificationBody` y mantiene la aplicación en ejecución después de que termine su actividad. Nada inicia un proceso que el sistema haya terminado.

```yaml
apple:
  info:
    NSLocationWhenInUseUsageDescription: Shows where you are on the map.
    NSLocationUsageDescription: Shows where you are on the map.  # macOS
    NSMotionUsageDescription: Counts your reps.
    UIBackgroundModes: [location]                                # background streams
android:
  permissions: [ACCESS_COARSE_LOCATION, ACCESS_FINE_LOCATION, FOREGROUND_SERVICE, FOREGROUND_SERVICE_LOCATION]
```

iOS y watchOS dan ubicación y movimiento, macOS ubicación, tvOS una posición cada vez y Android ambos a través de los proveedores propios del framework. Linux obtiene posiciones y flujos de GeoClue 2 en el bus del sistema, cuyo agente pregunta al usuario cuando se inicia la primera solicitud de la aplicación; como no se puede consultar a GeoClue sin mostrar un aviso, el permiso se lee como no solicitado hasta que la aplicación lo ha solicitado en esa ejecución, y la ubicación en segundo plano no está disponible. Windows los obtiene del `Geolocator` de WinRT; el permiso se lee como denegado mientras la ubicación está desactivada para la aplicación, como concedido una vez que la aplicación lo ha solicitado en esa ejecución y se le ha permitido, y en otro caso como no solicitado, y la ubicación en segundo plano tampoco está disponible allí. Un navegador seguro proporciona posiciones de geolocalización y, en móviles, lecturas del acelerómetro, el giroscopio y del movimiento fusionado. Llama a `cx.RequestPermission(core.PermissionMotion, done)` desde un manejador de pulsación en Safari. El navegador informa de que la ubicación no está disponible sin `navigator.geolocation`, y de que el movimiento no está disponible en escritorios ni para los flujos del magnetómetro. En TSX, `Sensors` tiene `permission`, `requestPermission`, `locate`, `watchLocation` y `watchMotion`, con las capacidades de paquete `location`, `backgroundLocation` y `motion`, en milisegundos.

## Audio

El paquete `audio` reproduce sonidos cortos decodificados una vez en memoria (`audio.LoadSound`) y audio largo en streaming (`audio.NewPlayer`) donde `cx.Env().CanPlayAudio()` es true; en otros lugares, las llamadas fallan con `audio.Unavailable`.

```go
audio.LoadSound(cx, audio.Source{URI: "asset:sounds/click.wav"}, func(click *audio.Sound, err error) {
	if err != nil {
		return
	}
	click.Play(audio.Voice{})                                  // center, full volume, normal rate
	click.Play(audio.Voice{Volume: 0.5, Rate: 1.2, Pan: -0.5}) // overlaps, panned left
})
```

`audio.Voice` establece por reproducción `Volume` (de 0 a 1, 0 como 1), `Rate` (de 0.5 a 2, 0 como 1) y `Pan` (de -1, totalmente a la izquierda, a 1, totalmente a la derecha, 0 centrado); los valores fuera de esos rangos se limitan y NaN toma el valor predeterminado. `Sound.Stop` detiene todas las reproducciones del sonido y `Close` lo libera, como hace su ámbito propietario cuando se desecha. `Player` admite `Play`, `Preload`, `Pause`, `Seek`, `SetVolume`, `SetRate`, `SetLoop` y `Close`. macOS, iOS, Windows, Linux, Android y la web reproducen audio hoy. Los backends de Apple, Windows y del navegador admiten WAV, AAC y MP3. Linux usa los códecs de GStreamer, y su reproductor en streaming reproduce a 1×. Android pausa un reproductor cuando su vista sale de la pantalla y limita cada sonido decodificado a 1 MiB. tvOS, watchOS, HarmonyOS y los miniprogramas siguen informando de `CanPlayAudio` como false.

Los navegadores requieren un gesto del usuario para la reproducción audible. Carga los sonidos cortos con antelación y luego llama a `Sound.Play` tras un clic, un toque o una pulsación de tecla. Un sonido reproducido antes de un gesto queda en silencio. La función de retorno de `Play` de un reproductor recibe `audio.NotAllowed` si el navegador bloquea la reproducción. Vuelve a intentarlo desde un gesto del usuario.

`Sound.Play` devuelve un `audio.Playback`, que en macOS, iOS, Windows, Linux y la web puede establecer su propio volumen y velocidad o detenerse de forma independiente. `Ended()` es reactivo y pasa a true cuando esa reproducción termina, se detiene, es desplazada por el límite de voces o no puede iniciarse. Los backends sin controles individuales devuelven un manejador ya terminado.

En iOS, un motor de audio propio de una biblioteca que reproduce fuera de `audio` y `Video`, como una `AudioQueue`, mantiene la única sesión de audio de la aplicación con `audio.HoldSession(core.AudioSessionPlayback)` mientras reproduce y llama a la función de liberación devuelta cuando se detiene. Acepta los niveles que acepta la `AudioSession` de un reproductor, junto a los vídeos, reproductores y grabaciones de la aplicación, de modo que `AudioSessionPlayback` y `AudioSessionMixWithOthers` siguen reproduciendo con el interruptor de timbre/silencio en silencio. En otros lugares no mantiene nada.

Usa `audio.Voice{Muted: true}` para iniciar un efecto en silencio y `playback.SetVolume(0)` para silenciar un efecto activo. Para el audio largo, `audio.NewPlayer` transmite una fuente y `player.SetMuted(true)` silencia la salida mientras la reproducción continúa. Reactivar el sonido restaura el volumen configurado del reproductor. `Voice.Volume` y `Player.SetVolume` usan cero para el volumen máximo predeterminado.

En TSX, `nsgo/audio` expone `supported()` y `loadSound(uri)`, condicionados por la capacidad de paquete `audio`. `loadSound` se resuelve con un sonido cuyo `play({ volume, rate, pan, loop, muted })` devuelve una reproducción con `setVolume`, `setRate`, `stop` y un `ended` reactivo; allí, un volumen de 0 es silencio. Una carga rechazada se rechaza con un `Error` cuyo `reason` es el nombre del error de audio, como `quota-exceeded`, y cuyo `code` es `ERR_ACCESS_DENIED` sin la capacidad, `ERR_NSH_QUOTA_EXCEEDED` al superar un límite y `ERR_FEATURE_UNAVAILABLE_ON_PLATFORM` donde nada reproduce audio. Un sonido solo se libera mediante `close()` o cuando su vista desaparece; en un entorno aislado, cada carga cuenta para `Limits.Sounds` (256 de forma predeterminada) hasta que falla o su sonido se cierra.

## Mandos de juego

El paquete `gamepad` lee los mandos de juego conectados con la distribución Standard Gamepad del W3C (de `ButtonSouth`/`ButtonA` a `ButtonHome`, `AxisLeftX`/`AxisLeftY`/`AxisRightX`/`AxisRightY`). `cx.Env().SupportsGamepads()` informa de si el destino admite mandos (macOS, iOS, Windows, Linux, Android y navegadores con la Gamepad API).

`gamepad.Pads(cx)` es una lectura reactiva con seguimiento que vuelve a renderizar los ámbitos cuando los mandos se conectan o se desconectan, sin volver a renderizar con los cambios de botones o palancas. Cada `gamepad.Pad` expone su `Family` (`FamilyXbox`, `FamilyPlayStation`, `FamilyNintendo` o `FamilyGeneric`, también clasificada mediante `gamepad.DetectFamily`), los `VendorID` y `ProductID` USB donde la plataforma los informa, y `CanRumble` para la compatibilidad con motores hápticos. Dentro de un bucle de `RequestAnimationFrame`, `gamepad.State(cx, id)` devuelve los botones del mando (`[0, 1]`, con `Pad.Pressed` en true a partir de `>= 0.5`) y los ejes (`[-1, 1]`, negativos a la izquierda/arriba y positivos a la derecha/abajo), capturados una vez por fotograma de pantalla.

`gamepad.Rumble(cx, id, lowFreq, highFreq, duration)` acciona los motores de vibración de baja frecuencia (pesado/izquierdo) y de alta frecuencia (ligero/derecho) con intensidades limitadas a `[0, 1]`, sustituyendo cualquier vibración en curso en ese mando y deteniéndose automáticamente después de `duration`, cuando la aplicación pasa a segundo plano o pierde el foco (`cx.Env().Lifecycle() != core.LifecycleActive`), cuando `cx` se desecha o cuando el mando se desconecta. `gamepad.StopRumble(cx, id)` detiene la vibración antes de tiempo; los mandos con `CanRumble == false` ignoran limpiamente las llamadas de vibración.

```go
var loop func(time.Time)
loop = func(time.Time) {
	if pad, ok := gamepad.State(cx, 0); ok {
		move(pad.Axes[gamepad.AxisLeftX], pad.Axes[gamepad.AxisLeftY], pad.Pressed(gamepad.ButtonA))
		if hit {
			gamepad.Rumble(cx, pad.ID, 0.8, 0.3, 180*time.Millisecond)
		}
	}
	cx.RequestAnimationFrame(loop)
}
cx.RequestAnimationFrame(loop)
```

Llama a `gamepad.EnableFocusNavigation(cx)` para activar la navegación por las paradas de foco nativas con un mando (la cruceta y la palanca izquierda mueven el foco, `ButtonSouth` activa el control enfocado y `ButtonEast` activa la acción de volver). Las vistas de juego activas llaman a `gamepad.CaptureInput(cx)` para pausar la navegación por el foco mientras están montadas. En TSX, `nsgo/gamepad` expone `supported()`, `pads()`, `state(id)`, `Button` y `Axis`, condicionados por la capacidad de paquete `gamepad`.

## Temporización del audio

Usa `audio.Now(cx)` como reloj para la reproducción temporizada. Establece `audio.Voice.StartAt` en un momento futuro cuando `audio.CanSchedule(cx)` sea true. Llama a `audio.OutputLatency(cx, func(latency time.Duration, known bool) { ... })` para la ruta de salida activa. macOS, iOS, Linux y la web admiten los inicios temporizados; en Windows y Android `CanSchedule` es false y un `StartAt` reproduce de inmediato. El valor `known` de la función de retorno indica si la ruta informó de una latencia, cosa que Windows y Android no hacen.

## Servicios de juego

El paquete `gameservices` proporciona una API independiente del proveedor para el inicio de sesión del jugador, los logros, las clasificaciones y las superposiciones nativas de Game Center / Play Games donde `cx.Env().CanUseGameServices()` es true. Cada función de retorno se ejecuta en la goroutine del motor, de modo que puede establecer señales directamente sin bloquear fotogramas.

```go
if cx.Env().CanUseGameServices() {
	gameservices.Authenticate(cx, func(p gameservices.Player, err error) {
		if err == nil && p.Authenticated {
			playerName.Set(p.DisplayName)
			gameservices.ReportAchievement(cx, "first-run", 100, nil)
			gameservices.SubmitScore(cx, "high-scores", 1250, nil)
		}
	})
}
```

`gameservices.ReportAchievement` limita el progreso a `0..100` (`100` desbloquea el logro). `gameservices.LoadScores` lee filas `gameservices.ScoreEntry` clasificadas para `Global` o `Friends` en `AllTime`, `Week` o `Today`, y `gameservices.ShowAchievements` y `gameservices.ShowLeaderboard` abren las superposiciones nativas de la plataforma. macOS, iOS y tvOS acceden a GameKit mediante purego; Android accederá a Play Games Services a través de un gancho del entorno de ejecución Java que aún no está en el entorno de ejecución, así que por ahora informa de `CanUseGameServices() == false`; Windows, Linux, la web, watchOS y los miniprogramas informan de `CanUseGameServices() == false` y responden `gameservices.ErrUnsupported`.

En las pruebas, activa los servicios de juego con `nsgotest.Environment(core.Environment{CanUseGameServices: true})` y controla la simulación aislada de cada aplicación con `App.SetGameServicesPlayer`, `App.SetGameServicesFriends`, `App.SeedLeaderboardScores`, `App.SetGameServicesError`, `App.GameServicesAchievements`, `App.GameServicesScores`, `App.GameServicesShownAchievements` y `App.GameServicesShownLeaderboards`.

## Siguiente

- [Notificaciones](notifications.md): permiso, insignias y pulsaciones.
- [Formularios](forms.md): campos de texto y el teclado.
- [Integración](embedding.md): servicios que una aplicación anfitriona da a su aplicación integrada.
- [Compilar y distribuir](building.md): los permisos y archivos que necesita un paquete.

### Actividad física, regiones y movimiento acotado

`cx.WatchSteps`, `cx.WatchActivity` y `cx.WatchPressure` devuelven flujos ligados a un ámbito. Solicita `core.PermissionActivityRecognition` antes de los pasos o la actividad, y antes de la presión en Apple. Apple proporciona los tres mediante Core Motion; Android proporciona contadores de pasos y presión, e informa de que la clasificación de actividad no está disponible. Los recuentos pertenecen a la ventana `Since` de la muestra; las ventanas reanudadas llevan `Gap`. La presión está en pascales, con altitud relativa opcional en metros. Las distancias, los pisos y la altitud que faltan tienen `Valid` en false.

`cx.WatchGeofences` acepta `core.GeofenceOptions` con entre uno y veinte círculos `GeofenceRegion` distintos (coordenadas WGS 84 y radio en metros). Informa de `GeofenceInside`, `GeofenceOutside` o `GeofenceUnknown` por región. `Background` requiere acceso y declaraciones de ubicación en segundo plano y la autorización Siempre de Apple; Android también necesita un título de notificación. La detención o el desecho elimina sus registros; ninguna suscripción sobrevive al fin del proceso.

`cx.RequestPreciseLocation("route", done)` solicita explícitamente la autorización precisa y responde con un booleano, nunca con una posición. En `apple.info`, declara `NSLocationTemporaryUsageDescriptionDictionary: {route: Explains why this feature needs precision.}`. Android necesita los permisos aproximado y preciso. Una respuesta false puede dejar autorizada la ubicación aproximada.

`cx.WatchBackgroundMotion(core.BackgroundMotionOptions{Motion: core.MotionOptions{}, Duration: 20 * time.Second, NotificationTitle: "Recording motion"}, sample, failed)` solicita un flujo en directo finito. Necesita `AccessMotion` y `AccessBackgroundMotion`; los paquetes TSX declaran `motion` y `backgroundMotion`. iOS puede hacer expirar antes su concesión de ejecución. Las aplicaciones Android establecen `android.background_motion: true` y declaran `FOREGROUND_SERVICE`; el servicio no persistente se detiene en un máximo de tres minutos aunque la duración solicitada sea mayor. La expiración por duración o por el sistema operativo informa de `SensorTimedOut`. La detención o el desecho terminan el proveedor y la concesión; no se registra nada tras la finalización. Los demás destinos informan de que no está disponible.

El `Sensors` de TSX replica estos seis métodos y usa milisegundos. Las pruebas sin interfaz gráfica suministran `FeedSteps`, `FeedActivity`, `FeedPressure` y `FeedGeofence`, y establecen `PreciseLocationAnswer` para las solicitudes de precisión. La actividad física real, la reactivación por región y el comportamiento en segundo plano con la aplicación suspendida siguen necesitando una validación en dispositivo.
