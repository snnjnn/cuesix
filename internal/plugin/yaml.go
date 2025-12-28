package plugin

import (
	"encoding/json"

	yaml "go.yaml.in/yaml/v4"
)

type YAMLPlugin struct{}

func (p *YAMLPlugin) Update(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return []byte("#END\n"), nil
	}

	var value any
	if err := json.Unmarshal(payload, &value); err != nil {
		return nil, err
	}

	rendered, err := yaml.Marshal(value)
	if err != nil {
		return nil, err
	}
	// required by APISIX
	return append(rendered, []byte("\n#END\n")...), nil
}
