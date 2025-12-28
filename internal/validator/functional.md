# Validator - Functional

- Validate the dynamic APISIX config by running `apisix test -c` against a mirrored APISIX home directory created at startup.
- The validator replaces `apisix-${APISIX_PROFILE}.{ext}` inside the mirror and runs `apisix test -c <mirror>/conf/config.yaml` from the mirror root.
- `APISIX_PROFILE` se lee del entorno; si está vacío, se usa `apisix.{ext}` como nombre.
- `{ext}` is `.yaml` when the YAML post-render plugin is enabled, otherwise `.json`.
- If validation fails, stop the pipeline and report the error with the command output.
