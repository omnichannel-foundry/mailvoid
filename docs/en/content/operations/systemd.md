---
title: systemd
weight: 20
---

# systemd

## Unit file

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

If you bind a privileged port, add:

```ini
AmbientCapabilities=CAP_NET_BIND_SERVICE
CapabilityBoundingSet=CAP_NET_BIND_SERVICE
```

## Environment file

`/etc/mailvoid/mailvoid.env`, owned by `root:mailvoid`, mode `0600`:

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

Environment values can be read by any process with access to the unit
environment. Prefer a secret manager that materializes this file at boot, and
never commit the file.

## Enable and start

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now mailvoid
sudo systemctl status mailvoid
sudo journalctl -u mailvoid -f
```

The startup log prints the generated administrator password on first run if no
password was configured. Capture it from the journal immediately, then change it
in the console.

## Reload and restart

MailVoid reads configuration only at startup. There is no reload signal; use:

```bash
sudo systemctl restart mailvoid
```

Sending `SIGTERM` triggers the graceful shutdown path: live streams close, SMTP
listeners stop, and the JSONL log and database close.

## Logs

- Structured events go to the JSONL file and Genji, not to stdout.
- Startup, shutdown, and fatal errors go to stdout/stderr and therefore the
  journal.
- Use `journalctl -u mailvoid` for process-level diagnostics and the console or
  API for the audit trail.

## File ownership

`StateDirectory=mailvoid` creates `/var/lib/mailvoid` with the service user. If
you set `MAILVOID_DATA_DIR` elsewhere, create it yourself and grant write access:

```bash
sudo install -d -o mailvoid -g mailvoid -m 0750 /srv/mailvoid
```

and add the path to `ReadWritePaths`.
