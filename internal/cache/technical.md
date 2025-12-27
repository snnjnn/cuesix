# Cache - Technical

- Use a non-cryptographic hash (like FNV) over canonicalized YAML bytes.
- Store the last hash in memory (optionally persisted later).
- Provide a simple API: `Changed(value map[string]any) -> (string, error)`.
- If there is a change, return the path to a temporary YAML file with deterministic ordering.
- If there is no change, return an empty string and nil error.
- If there is an error, return an empty string and remove any temporary file created.
- Ensure canonicalization is deterministic and independent of map ordering.
- Append a trailing `#END` line to the rendered YAML before hashing and writing.
- Cache is fed with the plugin-processed map, not the raw compiler output.
