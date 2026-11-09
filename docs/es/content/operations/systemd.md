---
title: systemd
weight: 20
---

# systemd

## Archivo de unidad

`/etc/systemd/system/mailvoid.service`:

```ini
[Unit]
Description=MailVoid SMTP capture server
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=mailvoid
Group=mailvoid
WorkingDirectory=/var/lib/mailvoid
EnvironmentFile=/etc/mailvoid/mailvoid.env
ExecStart=/usr/local/bin/mailvoid
Restart=on-failure
RestartSec=2s
TimeoutStopSec=15s

# Create /var/lib/mailvoid owned by User/Group.
StateDirectory=mailvoid
StateDirectoryMode=0750

# Hardening
NoNewPrivileges=true
PrivateTmp=true
PrivateDevices=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictAddressFamilies=AF_INET AF_INET6 AF_UNIX
RestrictNamespaces=true
LockPersonality=true
ReadWritePaths=/var/lib/mailvoid

[Install]
WantedBy=multi-user.target
```

Si vinculas un puerto privilegiado, añade:

```ini
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

## Archivo de entorno

`/etc/mailvoid/mailvoid.env`, propiedad de `root:mailvoid`, modo `0600`:

```bash
MAILVOID_SMTP_HOST=0.0.0.0
MAILVOID_SMTP_PORTS=1025,2525
MAILVOID_HTTP_ADDR=127.0.0.1:8025
MAILVOID_DOMAINS=app.test,*.mail.test
MAILVOID_DATA_DIR=/var/lib/mailvoid
MAILVOID_ADMIN_USER=admin
MAILVOID_ADMIN_PASSWORD=replace-with-a-secret
MAILVOID_COOKIE_SECURE=true
```

Los valores de entorno pueden ser leídos por cualquier proceso con acceso al
entorno de la unidad. Prefiere un gestor de secretos que materialice este archivo
al arrancar y nunca lo confirmes en el repositorio.

## Habilitar e iniciar

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mailvoid
sudo systemctl status mailvoid
sudo journalctl -u mailvoid -f
```

El registro de arranque imprime la contraseña de administrador generada en la
primera ejecución si no se configuró ninguna. Cópiala del journal de inmediato y
cámbiala en la consola.

## Recargar y reiniciar

MailVoid lee la configuración solo al arrancar. No hay señal de recarga; usa:

```bash
sudo systemctl restart mailvoid
```

El envío de `SIGTERM` activa la ruta de apagado ordenado: los flujos en vivo se
cierran, los listeners SMTP se detienen y el registro JSONL y la base de datos se
cierran.

## Registros

- Los eventos estructurados van al archivo JSONL y a Genji, no a stdout.
- El arranque, el apagado y los errores fatales van a stdout/stderr y, por tanto,
  al journal.
- Usa `journalctl -u mailvoid` para diagnósticos a nivel de proceso y la consola
  o la API para el registro de auditoría.

## Propiedad de archivos

`StateDirectory=mailvoid` crea `/var/lib/mailvoid` con el usuario del servicio. Si
estableces `MAILVOID_DATA_DIR` en otra ubicación, créala tú mismo y concede acceso
de escritura:

```bash
sudo install -d -o mailvoid -g mailvoid -m 0750 /srv/mailvoid
```

y añade la ruta a `ReadWritePaths`.
