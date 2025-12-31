# Listener - Technical

- Provide an HTTP server using the Go standard library.
- Accept address/port configuration via flags/env.
- Expose `GET /live` (always 200) and `GET /ready` (200 when ready, 425 otherwise).
- Validate method and path; return 404 for unknown paths and 405 for non-POST on `/compile`.
- On valid `/compile`, push a signal into the dispatcher queue and return 204.
- Ensure the handler is non-blocking and does not perform I/O heavy work.
