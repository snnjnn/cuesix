# Buffering Control Extension Spec

## Goal

Implement native/runtime support for per-request response buffering control so higher layers can disable buffering per route.

Primary target behavior:

- Route can set response buffering `on|off` at request runtime.
- Behavior should be equivalent to NGINX upstream runtime buffering switch (`u->buffering`) for proxied HTTP traffic.

## Background

- APISIX already supports dynamic request buffering control via native runtime API (`set_proxy_request_buffering`).
- Stock APISIX configuration snippets are not route-granular for `proxy_buffering`.
- Header-based control (`X-Accel-Buffering: no`) depends on upstream response and cannot be reliably injected late in APISIX plugin phases.

## Scope

In scope:

- New native runtime API: `set_proxy_buffering(boolean)`.
- Build integration into this repo's APISIX Docker image pipeline.
- Automated tests for correctness and regressions.
- C-side code only under `src/buffering`.

Out of scope:

- Stream subsystem.
- Global rewrite of APISIX routing model.
- APISIX plugin schema/runtime implementation.
- Route policy/config distribution in control plane.

## Functional Requirements

1. Runtime API can disable response buffering for selected proxied requests.
2. No behavior change for routes without this config.
3. Fail-safe behavior:
   - If native runtime extension is unavailable, return controlled error and log clearly.
4. Compatibility with current APISIX base image version used by this repo.

## Proposed Design

### 1) Native runtime extension

- Add `set_proxy_buffering(bool)` in native runtime API surface consumed by Lua.
- Lua module namespace must be `resty.sixpack` (avoid `resty.apisix.*` to prevent collisions with APISIX-owned modules).
- Ensure phase-safe application:
  - If upstream object already exists, set `u->buffering` immediately.
  - If not, persist intent in request context and apply at earliest safe upstream hook.
- Return Lua-friendly `(ok, err)` semantics only (never invalid Lua C return values).

### 2) Docker/build integration

- Build and ship native extension in Docker image (similar operational model as existing GeoIP module pipeline).
- Keep module loading explicit and deterministic.
- Document exact image tag and module version mapping.

## Non-Functional Requirements

- Maintainability: small API surface and tight plugin scope.
- Observability: log explicit runtime errors and phase misuse.
- Safety: do not panic/crash on unsupported phases; return deterministic errors.
- Upgradeability: include version guard notes for APISIX/OpenResty runtime updates.

## Test Strategy

### Unit/contract tests

- Lua API returns:
  - success path
  - unavailable runtime path
  - bad argument path

### Integration tests (required)

1. Baseline proxied request path (default buffering).
2. Proxied request path invoking `set_proxy_buffering(false)`.
3. Large upstream response, validate no temp-file spooling when disabled.
4. Streaming/SSE upstream response with buffering disabled.
5. Regression check: requests not invoking the API remain unchanged.
6. Coexistence check with `set_proxy_request_buffering` toggle.

### Operational verification

- Confirm no unexpected error log noise.
- Validate latency/memory impact for representative large responses.

## Risks and Mitigations

- Risk: phase timing mismatch for setting `u->buffering`.
  - Mitigation: deferred apply hook and explicit phase checks.
- Risk: APISIX/OpenResty version drift.
  - Mitigation: pin image tag and add compatibility notes/tests.
- Risk: false confidence from shallow tests.
  - Mitigation: include real proxy_pass large body and streaming test cases.

## Deliverables

- Native runtime extension code with API docs.
- Dockerfile/build updates for image generation.
- Test suite additions.
- Runtime API usage notes for downstream consumers (plugin to be implemented elsewhere).

## Implementation Plan

- [x] Confirm runtime repo/module location used by this Docker build path.
- [x] Define exact API contract for `set_proxy_buffering(bool)` and error model.
- [x] Implement native runtime function with phase-safe apply logic.
- [x] Add native tests (or closest available runtime validation tests).
- [x] Integrate extension build/load into this repo `Dockerfile`.
- [x] Add end-to-end test fixture with large upstream responses.
- [x] Validate temp-file behavior and logs under load.
- [x] Document runtime API contract and known limitations in repo docs.
