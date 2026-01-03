package serializer

import (
	// Have to disable for the time being, because it causes crashes
	// in some architectures (windows WSL, ubuntu 24.04, go 1.25.5)
	// "encoding/json/v2"
	"encoding/json"
)

// Serialize returns a deterministic JSON payload
func Serialize(value map[string]any) ([]byte, error) {
	//return json.Marshal(value, json.Deterministic)
	return json.Marshal(value)
}
