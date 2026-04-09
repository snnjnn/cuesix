package plugin

import (
	"encoding/json"
	"log/slog"

	yaml "go.yaml.in/yaml/v4"
)

// YAMLPlugin converts JSON payloads to YAML and appends the #END marker.
type YAMLPlugin struct{}

// Update converts JSON payload bytes to YAML and appends the APISIX #END marker.
func (p *YAMLPlugin) Update(logger *slog.Logger, payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		logger.Info("yaml plugin empty payload")
		return []byte("#END\n"), nil
	}

	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		logger.Error("yaml plugin decode failed", "error", err)
		return nil, err
	}

	rendered, err := yaml.Marshal(value)
	if err != nil {
		logger.Error("yaml plugin encode failed", "error", err)
		return nil, err
	}
	// required by APISIX
	logger.Info("yaml plugin complete", "bytes", len(rendered))
	return append(rendered, []byte("\n#END\n")...), nil
}
