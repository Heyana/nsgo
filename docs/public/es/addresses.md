---
title: Direcciones de la aplicación
description: Lee y actualiza la dirección de una aplicación web a través del historial del navegador.
group: Conceptos básicos
order: 59
---

`cx.AppURL()` lee la dirección de la aplicación como un valor rastreado. En la web, la URL inicial de la página está disponible antes de que se renderice la raíz, de modo que una recarga o un enlace compartido puedan elegir la misma vista. La aplicación decide cómo se asigna la URL a sus vistas.

```go
func Root(cx *core.Scope) core.Node {
    return core.View{Children: []core.Node{
        core.Text{ContentFunc: cx.AppURL},
        base.Button{Label: "Open film", OnPress: func(core.PressEvent) {
            if err := cx.SetAppURL("/films/42", core.AppURLOptions{
                History: core.HistoryPush,
            }); err != nil {
                // Show the error in the app's usual way.
            }
        }},
    }}
}
```

`SetAppURL` actualiza de inmediato la dirección rastreada sin solicitar una página ni navegar por una pila. De forma predeterminada reemplaza la entrada actual del navegador; `HistoryPush` solicita una nueva entrada. Una navegación o apertura modal dentro de la misma confirmación aporta esa entrada en su lugar, de manera que una acción no genere dos pasos atrás. Varias escrituras en una misma confirmación conservan la última dirección y mantienen cualquier solicitud de inserción en el historial.

Las direcciones relativas se resuelven respecto a la dirección actual o respecto a una etiqueta base HTML explícita. Una dirección del navegador debe permanecer dentro del origen de la página. Las direcciones no válidas, los espacios en blanco, los caracteres de control, las barras invertidas y las direcciones de más de 16 KiB devuelven un error sin modificarla.

`cx.OnAppURLChange(func(change core.AppURLChange) { ... })` observa los desplazamientos por el historial del navegador y las ediciones del hash. `change.Reason` es `"back"`, `"forward"` o `"address"`; la función devuelta cancela la suscripción y la destrucción del ámbito elimina el oyente. Lee `AppURL` para obtener la dirección inicial. Las escrituras de la propia aplicación actualizan los enlaces reactivos, pero no envían notificación de cambio. Un desplazamiento que conserva la misma dirección no envía ningún cambio.

El botón de retroceso del navegador sigue cerrando un modal o sacando una pantalla de navegación. Si la aplicación rechaza un retroceso, se restauran la entrada existente y su dirección. Un manejador de cierre puede establecer su propia dirección de reemplazo. Un desapilado programático actualiza la dirección después del recorrido asíncrono por el historial, y las escrituras que lleguen durante ese recorrido esperan a que finalice. Retroceder más allá de las entradas de este reproductor sale de la aplicación, incluidas las entradas conservadas de una carga anterior.

Para rutas como `/films/42`, configura el directorio de despliegue de la aplicación y haz que el servidor sirva su `index.html` para las rutas de la aplicación:

```yaml
web:
  base: /        # /my-app/ when deployed below that directory
```

`web.base` escribe una etiqueta base HTML para que los scripts, estilos, Wasm y recursos sigan cargándose cuando se abre directamente una dirección profunda. Sin ella, se mantienen las rutas de recursos relativas existentes; las direcciones con parámetros de consulta o hash pueden usarlas sin cambios. Las reglas de respaldo del servidor deben dejar intactas las solicitudes de recursos reales. Una Política de Seguridad de Contenidos estricta debe permitir esta base del mismo origen (`base-uri 'self'`).

Una aplicación incrustada en otra página conserva su dirección internamente y no toca el historial de la página contenedora. Un iframe del mismo origen puede optar explícitamente por participar mediante `data-nsgo-history`. Los subárboles confinados no pueden leer ni cambiar la dirección de la aplicación.

En otros destinos, `AppURL` conserva el último enlace abierto con la aplicación o el valor almacenado por `SetAppURL`; establecerlo no modifica ninguna ventana nativa ni navegador del sistema. Los miniprogramas también conservan este valor, sin cambiar las rutas de su anfitrión.

TSX utiliza el mismo estado:

```tsx
import { UrlScheme } from "nsgo";

UrlScheme.url();
UrlScheme.setUrl("/films/42", { history: "push" });
const stop = UrlScheme.on("change", ({ url, reason }) => {
  // Update the app's route from the browser's choice.
});
```
