# Reloader - Technical

- Perform an atomic replace of the APISIX config file when possible.
- Call APISIX admin API endpoint to trigger reload.
- Configurable target config path and APISIX URL via flags/env.
- Support optional API key header and configurable HTTP method (defaults to POST).
- Retry reload requests with exponential backoff (configurable attempts and timings).
- Log success/failure with enough detail for operators.
