---
title: Recorrido por el panel
weight: 60
---

# Recorrido por el panel

La consola es una aplicación Fiber renderizada en el servidor. HTMX actualiza la
lista de mensajes y el panel de prueba de políticas sin recargar la página
completa; Cash se encarga del cambio de tema, la copia al portapapeles y el cajón
móvil.

## Navegación

| Página | Propósito |
|------|---------|
| **Panel** | Recuentos, mensajes recientes y eventos recientes |
| **Mensajes** | Buscar y filtrar todos los mensajes almacenados |
| **Flujo de eventos** | Traza de auditoría en vivo con filtros por tipo/severidad |
| **Políticas** | Crear, editar, reordenar, habilitar, probar y eliminar reglas |
| **Ajustes** | Configuración en tiempo de ejecución y cambio de contraseña |

La barra lateral muestra la versión y un indicador de eventos en vivo. En
pantallas estrechas, el botón superior izquierdo abre el cajón.

## Panel

Cuatro tarjetas resumen:

- total de mensajes almacenados;
- mensajes recibidos en las últimas 24 horas;
- número de políticas;
- número total de eventos.

Debajo se encuentran los ocho mensajes más recientes y los ocho eventos más
recientes, cada uno enlazado a su página completa.

## Mensajes

La lista admite filtrado en vivo:

- escribir en **Buscar** desencadena una petición HTMX con debounce (350 ms);
- cambiar un selector se actualiza de inmediato;
- la URL se actualiza para que la vista filtrada sea enlazable;
- la paginación muestra el total de coincidencias y el número de páginas.

Las filas enlazan a la vista de detalle. Los resultados vacíos explican que ningún
mensaje coincidió e invitan a enviar correo.

## Detalle del mensaje

Secciones, en orden:

1. **Cabecera** — asunto, remitente, destinatario, insignia de acción, insignia de
   política y una insignia de caducidad en vivo para mensajes con TTL.
2. **Sobre** — buzón, IP de origen, puerto de recepción, hora de recepción, tamaño,
   ID de mensaje, política y caducidad.
3. **Origen** — un resumen conciso de qué listener y qué par produjeron el mensaje.
4. **Cuerpo HTML** — vista previa en iframe aislado cuando existe una parte HTML.
5. **Cuerpo de texto plano** — texto decodificado copiable.
6. **Cabeceras** — tabla analizada y bloque de cabeceras en bruto, copiables.
7. **Mensaje en bruto** — los bytes exactos, copiables.

El botón **Descargar .eml** devuelve el mensaje RFC 5322 en bruto como adjunto.
El botón **Eliminar** pide confirmación y elimina el mensaje de forma permanente.

## Flujo de eventos

La página carga los eventos recientes de la base de datos y luego se suscribe a
`/ws/events`. Controles:

- **punto en vivo** — verde mientras está conectado, ámbar mientras reconecta;
- **Pausar / Reanudar** — almacena en búfer los eventos entrantes en el navegador;
- **Limpiar vista** — elimina las filas del lado del cliente sin borrar el historial.

Cada fila muestra la hora, la insignia de severidad, el tipo y la carga JSON
expandible.

## Políticas

La página tiene tres paneles:

1. **Formulario de regla** — crea una regla o edita una existente en el mismo
   lugar. El formulario se reemplaza por un formulario de edición cuando pulsas
   **Editar**; **Cancelar** recarga el formulario de creación.
2. **Probador de regex** — envía un campo, un patrón y un valor de ejemplo mediante
   HTMX y muestra si coinciden o no, o el error de la regex.
3. **Tabla de reglas** — ordenada por prioridad con acciones de editar, subir/bajar,
   habilitar/deshabilitar y eliminar.

La tarjeta de orden de evaluación reafirma la regla de que gana la primera
coincidencia, para que los operadores no coloquen por accidente una regla comodín
por encima de reglas de descarte específicas.

## Ajustes

Ajustes muestra:

- un interruptor en vivo para activar o desactivar la captura de correo;
- host SMTP, puertos, dominios aceptados y tamaño máximo de mensaje;
- dirección HTTP;
- ruta de la base de datos Genji y ruta del registro de eventos JSONL;
- intervalo del sweeper, ajuste de cookie segura, estado de configuración del token
  de API, estado de WebDAV habilitado y la ruta del índice de búsqueda;
- total de mensajes almacenados;
- el formulario de cambio de contraseña del administrador.

La configuración se lee al arrancar. Cambiar variables de entorno requiere un
reinicio; el interruptor de captura se aplica de inmediato y se persiste en la
base de datos. El resto de la página de ajustes es informativa, salvo el
formulario de contraseña.

## Tema

El tema inicial es oscuro. El botón de luna/sol alterna entre oscuro y claro y
guarda la elección en `localStorage` bajo `mailvoid-theme`. Un script en línea en
la cabecera aplica el tema guardado antes del pintado para evitar un destello.
