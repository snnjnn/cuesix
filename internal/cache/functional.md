# Cache - Functional

- Compute a hash of the compiled JSON based on deterministic ordering.
- Compare with the previously accepted hash.
- If unchanged, short-circuit the pipeline (skip validation/reload).
- Return the byte representation of the deterministic JSON, if changed.
