package cache

import (
	"encoding/json/v2"
	"hash/fnv"
)

type Cache struct {
	lastHash uint64
	hasHash  bool
}

func (c *Cache) Changed(value map[string]any) ([]byte, error) {
	payload, err := MarshalDeterministicJSON(value)
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
func MarshalDeterministicJSON(value map[string]any) ([]byte, error) {
	return json.Marshal(value, json.Deterministic(true))
}
