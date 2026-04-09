# Buffering Native Runtime Extension

This directory contains the first implementation slice for route-level response buffering control in APISIX runtime.

## Current API contract

Lua API (`resty.sixpack`):

- `set_proxy_buffering(boolean) -> ok, mode | nil, err`

Semantics:

- `true` on success.
- `true, "immediate"` when applied directly to `r->upstream->buffering`.
- `true, "deferred"` when intent is persisted and applied later when upstream is available.
- `nil, "bad argument: enabled must be boolean"` when argument is invalid.
- `nil, "request context unavailable"` when there is no active request context.
- `nil, "native runtime failure: unable to allocate request context"` on allocation failure.
- `nil, "native runtime unavailable: ngx_http_sixpack_buffering_module not loaded"` when the dynamic module is not loaded.

Native C return codes (`ngx_http_sixpack_set_proxy_buffering`):

- `0`: success
- `1`: deferred success (intent saved for later apply)
- `-1`: bad argument
- `-2`: no request
- `-3`: no memory

## Build integration

`Dockerfile` now compiles and ships:

- `/usr/local/apisix/modules/ngx_http_sixpack_buffering_module.so`
- `/usr/local/apisix/resty/sixpack.lua`

## Runtime loading

The module must be loaded explicitly in APISIX/Nginx main config, and Lua path
must resolve `resty.sixpack` from the image-provided path
(`/usr/local/apisix/resty/sixpack.lua`).

Required `conf/config.yaml` settings:

```yaml
nginx_config:
  main_configuration_snippet: |
    load_module modules/ngx_http_sixpack_buffering_module.so;
```

## Usage contract

- Call from request context only (recommended: APISIX `serverless-pre-function` in `header_filter` for immediate apply).
- Always handle `ok, err` and log `err` when `ok` is `nil`.
- Deferred success (`rc=1`) is treated as Lua success; intent is applied later when upstream exists.

Minimal example:

```lua
local buffering = require("resty.sixpack")
local ok, mode_or_err = buffering.set_proxy_buffering(false)
if not ok then
    ngx.log(ngx.ERR, "set_proxy_buffering failed: ", mode_or_err)
    return 503
end
```

APISIX route example (`serverless-pre-function` in `header_filter` phase):

```yaml
routes:
  - id: nobuf-example
    uri: /nobuf/*
    plugins:
      serverless-pre-function:
        phase: header_filter
        functions:
          - |
            local buffering = require("resty.sixpack")
            return function(conf, ctx)
                local ok, mode_or_err = buffering.set_proxy_buffering(false)
                if not ok then
                    ngx.log(ngx.ERR, "set_proxy_buffering failed: ", mode_or_err)
                    return 503
                end
                if mode_or_err ~= "immediate" then
                    ngx.log(ngx.ERR, "set_proxy_buffering expected immediate mode in header_filter, got: ", mode_or_err)
                end
            end
    upstream:
      type: roundrobin
      nodes:
        "your-upstream:80": 1
```

## Operational verification

Use the E2E runner to validate temp-file behavior and log cleanliness:

```bash
just buffering-e2e
```

The runner verifies:
- baseline route emits response temp-file spool warnings,
- no-buffer route does not add spool warnings,
- no `set_proxy_buffering failed:` error appears in APISIX logs,
- optional concurrent load check (`LOAD_CHECK=1` by default) keeps the same behavior.

## Limitations in this slice

- Deferred apply is best-effort and currently applied by a module header filter when upstream is available.
- Stream subsystem is intentionally unsupported.
- Coverage is integration-focused; there are no standalone unit tests for the C symbol in this repo.
