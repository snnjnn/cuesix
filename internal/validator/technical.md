# Validator - Technical

- Receive the path to the mirrored APISIX home directory created at startup.
- Replace `apisix-${APISIX_PROFILE}.{ext}` under `<mirror>/conf/` with the candidate config.
- `APISIX_PROFILE` se lee del entorno; si está vacío, se usa `apisix.{ext}` como nombre.
- Run `apisix test -c <mirror>/conf/config.yaml` while setting the working directory to the mirror root.
- `{ext}` is `.yaml` when the YAML post-render plugin is enabled, otherwise `.json`.
- Capture stdout/stderr for logs and error reporting.
- Return a typed error that includes the command output and underlying cause.
