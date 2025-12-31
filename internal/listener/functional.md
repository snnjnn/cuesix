# Listener - Functional

- Expose `POST /compile`, plus health endpoints `GET /live` and `GET /ready`.
- Enqueue a compile request and return immediately with `204 No Content`.
- Ignore request body and headers (no payload processing).
- Never perform compilation work in the handler.
