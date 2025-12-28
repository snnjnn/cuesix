# Cache - Technical

- Use a non-cryptographic hash (like FNV) over canonicalized JSON bytes.
- Store the last hash in memory (optionally persisted later).
- Provide a simple API: `Changed(logger *slog.Logger, value map[string]any) -> ([]byte, error)`.
- If there is a change, return the []byte formatted version of the JSON with deterministic ordering.
- If there is no change, return nil []byte and nil error.
- If there is an error, return nil []byte and the error.
- Ensure canonicalization is deterministic and independent of map ordering.
- JSON serialization/deserialization must use `encoding/json/v2` with `Deterministic` set to true.
