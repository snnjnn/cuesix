# Buffering E2E Tests

This folder contains containerized integration tests for the native buffering extension.

## Test stack

The suite runs three services in one `docker compose` project:

1. `log-volume-init`: one-shot container that prepares shared log volume ownership as UID/GID `636:636`.
2. `upstream`: python upstream fixture server, running as UID/GID `636:636`.
3. `apisix`: APISIX under test, configured in standalone YAML mode (default APISIX user).
4. `tests`: Perl test runner that executes declarative `.t` tests with `prove`, running as UID/GID `636:636`.

APISIX logs are written to `/tmp/apisix-logs/error.log` in a shared Docker volume so
functional tests can assert buffering behavior through log inspection.

## What it validates

1. Baseline proxy route (default buffering) produces response temp-file spool warnings.
2. Route using `resty.sixpack.set_proxy_buffering(false)` avoids additional spool warnings.
3. SSE route with buffering disabled emits first `data:` event quickly.
4. Regression: baseline route still behaves with default buffering after no-buffer traffic.
5. Coexistence: route combining `proxy-control.request_buffering=false` and response buffering disable works.
6. Fast-path requirement: `header_filter` invocation must return mode `immediate`; test fails otherwise.

## Run

```bash
just buffering-e2e
```

or:

```bash
./src/buffering/tests/run-e2e.sh
```

The `tests` service exit code is used as the compose exit code.

Optional env vars:

- `IMAGE_TAG` (default: `apisix-buffering-e2e:local`)
- `PROJECT_NAME` (default: `buf-e2e`)
- `TESTS_TIMEOUT_SECONDS` (default: `300`, timeout for `prove` inside the tests container)
- `WAIT_TIMEOUT_SECONDS` (default: `360`, timeout while runner waits for tests container to exit)
