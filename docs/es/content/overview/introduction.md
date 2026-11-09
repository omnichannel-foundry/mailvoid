---
title: Introducción
weight: 10
---

# Introducción

MailVoid es un sumidero SMTP comodín e inspector de correo. Escucha SMTP en uno
o más puertos, acepta correo para cualquier buzón de los dominios que permitas y
almacena los metadatos del sobre, las cabeceras, los cuerpos de texto y HTML
decodificados, y el mensaje en bruto.

A diferencia de un servidor de correo completo, MailVoid no entrega correo a
ningún destino. No hay cola saliente, ni relay, ni IMAP o POP3, ni protocolo de
buzón. Es un dispositivo de captura para desarrollo, pruebas, automatización e
inspección controlada del correo entrante.

## Por qué existe

Probar flujos de correo es incómodo. Los proveedores reales requieren
credenciales y acceso a la red, los servidores de correo locales son pesados y
los servicios de bandeja temporal están fuera de tu control. MailVoid cubre ese
hueco:

- un único binario con una base de datos embebida;
- cualquier cantidad de puertos SMTP y dominios;
- un panel web con eventos en vivo e inspección de mensajes;
- reglas regex que deciden qué conservar, caducar o descartar;
- una API JSON para automatización;
- sin dependencias externas en tiempo de ejecución.

## Principios de diseño

### Primero los eventos

Cada transición de estado significativa es un evento. La apertura de una
conexión, el análisis de `MAIL FROM`, la aceptación o rechazo de `RCPT TO`, la
coincidencia de una política, el almacenamiento de un mensaje, una eliminación
por TTL y un inicio de sesión emiten JSON estructurado. Los eventos se
persisten, se indexan y se difunden en vivo.

Esto convierte el rastro de auditoría en una superficie de producto de primera
clase en lugar de un archivo de log que alguien podría leer más tarde.

### Frontera de repositorio

Toda la persistencia pasa por una pequeña interfaz `storage.Store`. La
implementación de producción usa Genji; las pruebas usan la implementación en
memoria. Cambiar la base de datos no toca las capas SMTP, de políticas, de
eventos ni web.

### Capturar todo, decidir después

El mensaje en bruto y el sobre se conservan siempre cuando se guarda un mensaje.
El panel puede mostrar una vista previa HTML en sandbox mientras la API puede
devolver los bytes originales. Las decisiones de política se registran en el
mensaje para que un operador pueda ver por qué está presente y cuándo caducará.

### Binario único

Las plantillas, CSS, JavaScript y el motor de base de datos se compilan en el
ejecutable. El despliegue consiste en copiar un archivo y elegir un directorio
de datos.

## Para quién es

- Desarrolladores que prueban correo transaccional, flujos de notificación o
  análisis de correo entrante.
- Entornos de QA e integración que necesitan captura determinista.
- Operadores que necesitan un sumidero SMTP controlado con reglas de retención y
  un rastro de auditoría.
- Investigadores de seguridad que inspeccionan cabeceras de mensajes y
  estructura MIME en un visor en sandbox.

## Fuera de alcance

MailVoid no es:

- un MTA ni un relay;
- un servidor IMAP/POP3;
- un proveedor de buzones de producción;
- un servicio con autenticación SMTP ni STARTTLS;
- un sistema multiinquilino con separación de roles (cada cuenta autenticada es
  administradora).

> [!WARNING]
> Con `MAILVOID_DOMAINS=*` (el valor predeterminado), se acepta cualquier
> dominio. Nunca expongas un listener de MailVoid sin restricciones a internet
> público: podría usarse para llenar tu disco o para blanquear tráfico.
> Restringe los dominios y aplica firewall a los puertos SMTP, o coloca el
> servicio en una red privada.
