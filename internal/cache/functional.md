# Cache - Functional

- Compute a hash of the compiled YAML based on deterministic ordering.
- Compare with the previously accepted hash.
- If unchanged, short-circuit the pipeline (skip validation/reload).
- If changed, emit the deterministic YAML to a temporary file for downstream steps.
- Ensure the generated YAML ends with a trailing `#END` comment line.
- Operates on the post-processed configuration provided by plugins.
