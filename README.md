# cuesix

A small Go service that compiles APISIX standalone configuration from CUE/YAML fragments, validates it, and triggers APISIX reloads.

## Overview

- POST `/compile` enqueues a compile request and responds with `204 No Content`.
- Requests are throttled and coalesced with a configurable cooldown.
- The compile pipeline is: compile -> hash check -> validate -> reload.

## Build

Docker (multi-stage):

```bash
docker build -t cuesix:local .
```

## Configuration

All flags should be mirrored as environment variables. Planned configuration keys:

- Input directories: `--input-dir` (repeatable), env `CUESIX_INPUT_DIR` (comma-separated)
- Temp directory: `--temp-dir`, env `CUESIX_TEMP_DIR`
- APISIX config path: `--apisix-config`, env `CUESIX_APISIX_CONFIG`
- APISIX URL: `--apisix-url`, env `CUESIX_APISIX_URL`
- Listen address: `--listen`, env `CUESIX_LISTEN`
- Cooldown: `--cooldown`, env `CUESIX_COOLDOWN`

## Runtime notes

- Uses `apisix test` to validate the generated config.
- On success, replaces the live config and triggers APISIX reload via HTTP API.

## Layout

See `AGENTS.md` for module responsibilities and documentation requirements.
