# AGENTS

This repository defines a small Go service that compiles APISIX standalone config from CUE/YAML fragments. Agents should follow the rules below.

## Layout plan

- `cmd/cuesix/`: program entrypoint and CLI wiring (flags, env, config defaults).
- `internal/listener/`: HTTP server and /compile handler (enqueue only).
- `internal/dispatcher/`: queue, throttling, cooldown timing, compile scheduling.
- `internal/compiler/`: load input directories, unify CUE/YAML, emit temp YAML.
- `internal/cache/`: deterministic hash of compiled YAML and cache check.
- `internal/validator/`: `apisix test` execution against temp file.
- `internal/reloader/`: replace real APISIX config and trigger reload via API.

## Documentation rule

Each module under `internal/<module>` must include:

- `functional.md`: responsibilities and observable behavior.
- `technical.md`: inputs/outputs, dependencies, error handling, and configuration.

When adding a new module, create its folder under `internal/` and add both docs.

## Code principles

- Prefer Go standard library; keep dependencies minimal.
- Favor readability and maintainability over cleverness.
- All flags must be mirrored as environment variables.
- HTTP endpoint responds `204 No Content` immediately after enqueue.
