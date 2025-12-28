package cache

import (
	"encoding/json/v2"
	"hash/fnv"
	"log/slog"
)

// Cache tracks the last serialized payload to detect changes.
type Cache struct {
	lastHash uint64
	hasHash  bool
}

// Changed returns a deterministic JSON payload when the value differs from the last call.
func (c *Cache) Changed(logger *slog.Logger, value map[string]any) ([]byte, error) {
	payload, err := MarshalDeterministicJSON(logger, value)
	if err != nil {
		return nil, err
	}
	hash := hashBytes(payload)
	changed := !c.hasHash || c.lastHash != hash
	if !changed {
		return nil, nil
	}
	c.lastHash = hash
	c.hasHash = true
	return payload, nil
}

func hashBytes(payload []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(payload)
	return h.Sum64()
}

// MarshalDeterministicJSON renders a map into JSON with stable key ordering.
func MarshalDeterministicJSON(_ *slog.Logger, value map[string]any) ([]byte, error) {
	return json.Marshal(value, json.Deterministic(true))
}
