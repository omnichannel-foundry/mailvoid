---
title: Búsqueda de texto completo
weight: 24
---

# Búsqueda de texto completo

MailVoid usa un índice **Bleve** embebido como proyección reconstruible del
almacén de mensajes Genji. Proporciona búsqueda tokenizada y con ranking (BM25)
sobre el contenido de los mensajes, a la vez que mantiene los bytes en bruto
autoritativos en Genji.

## Configuración

| Variable | Predeterminado | Significado |
|----------|----------------|-------------|
| `MAILVOID_SEARCH_ENABLED` | `true` | Habilitar el índice Bleve |
| `MAILVOID_SEARCH_INDEX` | `<data-dir>/search.bleve` | Directorio del índice |

Flags: `-search-enabled`, `-search-index`.

El índice se abre al iniciar. Si falta o su recuento de documentos difiere del
almacén de mensajes, se reconstruye desde Genji. El índice no es autoritativo:
eliminarlo y reiniciar es seguro.

## Campos indexados

- asunto;
- cuerpo de texto plano decodificado;
- cuerpo HTML con las etiquetas eliminadas;
- remitente y destinatario;
- buzón, dominio e IP de origen;
- cabeceras en bruto;
- acción, puerto de recepción, tamaño y marca de tiempo de recepción.

El mensaje completo en bruto **no** se indexa; recupéralo con `get_message` o el
endpoint `.eml` después de encontrar una coincidencia. Los adjuntos no se extraen
ni se indexan por separado.

Los campos de palabra clave reflejan buzón, dominio, destinatario, remitente,
acción e IP de origen para que los filtros puedan combinarse con la consulta de
texto. Las superficies REST y MCP usan esos filtros estructurados en lugar de un
lenguaje de consulta en bruto.

## REST

`GET /api/v1/search`:

```bash
curl -s -H "Authorization: Bearer $MAILVOID_API_TOKEN" \
  'http://localhost:8025/api/v1/search?q=invoice&mailbox=inbox&domain=example.test&limit=20'
```

| Parámetro | Significado |
|-----------|-------------|
| `q` | Consulta de texto completo |
| `mailbox`, `domain`, `recipient`, `sender` | Filtros de metadatos exactos |
| `action` | `keep`, `ttl` o `drop` |
| `port` | Puerto de recepción |
| `since`, `until` | Límites RFC 3339 |
| `limit`, `offset`, `page` | Paginación |
| `sort` | `relevance` (predeterminado) o `received_at` |

Respuesta:

```json
{
  "messages": [
    {
      "id": "msg_...",
      "received_at": "2026-11-08T05:00:00Z",
      "mailbox": "inbox",
      "domain": "example.test",
      "sender": "billing@example.org",
      "subject": "Invoice 42",
      "action": "keep"
    }
  ],
  "count": 1,
  "limit": 20,
  "offset": 0,
  "backend": "bleve",
  "highlights": {
    "msg_...": {
      "subject": ["<mark>Invoice</mark> 42"],
      "body": ["Your <mark>invoice</mark> is attached"]
    }
  }
}
```

`backend` es `bleve` cuando el índice está activo y `fallback` cuando el índice
está deshabilitado o no disponible. El modo alternativo conserva la búsqueda
heredada por subcadena de metadatos y no busca en los cuerpos.

> [!NOTE]
> `GET /api/v1/messages?q=...` mantiene su comportamiento original de subcadena de
> metadatos. Usa `/api/v1/search` para la búsqueda en cuerpos y el ranking.

## MCP

La herramienta `search_messages` usa el mismo índice y filtros:

```json
{
  "q": "invoice",
  "mailbox": "inbox",
  "domain": "example.test",
  "limit": 20,
  "sort": "relevance"
}
```

El resultado incluye `messages`, `count`, `backend` y `highlights` opcional.

## Mantenimiento del índice

- Los mensajes se indexan de forma síncrona tras una escritura correcta en el
  almacén mediante un decorador `IndexedStore`. La captura SMTP, las eliminaciones
  por API/consola y la caducidad TTL actualizan el índice.
- Si la indexación falla, el mensaje se almacena igualmente y se emite un evento
  `search.error`. La búsqueda puede quedar desactualizada hasta una
  reconstrucción.
- El índice se reconstruye al iniciar cuando su recuento de documentos difiere del
  almacén.
- El directorio del índice Bleve contiene términos y campos buscables almacenados.
  Trátalo como datos sensibles e inclúyelo en los mismos controles de acceso que la
  base de datos. Es reconstruible, por lo que las copias de seguridad pueden
  omitirlo.

## Compromisos

Bleve v2.5.7 es la versión más reciente compatible con Go 1.24; v2.6+ requiere Go
1.25. Es Go puro y se compila con `CGO_ENABLED=0`, a costa de un binario más grande
y una segunda estructura en disco junto a Genji.
