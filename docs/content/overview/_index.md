---
title: Overview
weight: 10
bookCollapseSection: true
---

# Overview

MailVoid exists to make inbound mail visible. Point any SMTP client, application,
or test suite at a MailVoid listener and every message becomes inspectable
through a polished web console and a JSON API.

The project is deliberately small: one process, one embedded database, no
external services, and no frontend build step at runtime. The whole system is a
Go binary plus a data directory.

Start with [Introduction](/overview/introduction/) for the design goals, then
[Features](/overview/features/) for the capability list, and
[Architecture](/overview/architecture/) for how the pieces fit together.
