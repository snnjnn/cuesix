package cache

import (
	"fmt"
	"hash/fnv"
	"os"
	"sort"

	"go.yaml.in/yaml/v3"
)

type Cache struct {
	lastHash uint64
	hasHash  bool
}

func (c *Cache) Changed(value map[string]any) (string, error) {
	payload, err := MarshalDeterministicYAML(value)
	if err != nil {
		return "", err
	}
	hash := hashBytes(payload)
	changed := !c.hasHash || c.lastHash != hash
	if !changed {
		return "", nil
	}
	path, err := writeTempYAML(payload)
	if err != nil {
		return "", err
	}
	c.lastHash = hash
	c.hasHash = true
	return path, nil
}

func hashBytes(payload []byte) uint64 {
	h := fnv.New64a()
	_, _ = h.Write(payload)
	return h.Sum64()
}

func writeTempYAML(payload []byte) (string, error) {
	tmp, err := os.CreateTemp("", "cuesix-cache-*.yaml")
	if err != nil {
		return "", err
	}
	path := tmp.Name()
	if _, err := tmp.Write(payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(path)
		return "", err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

// MarshalDeterministicYAML renders a map into YAML with stable key ordering.
func MarshalDeterministicYAML(value map[string]any) ([]byte, error) {
	if err := validateStringKeys(value); err != nil {
		return nil, err
	}
	var node yaml.Node
	if err := node.Encode(value); err != nil {
		return nil, err
	}
	sortYAMLNode(&node)
	payload, err := yaml.Marshal(&node)
	if err != nil {
		return nil, err
	}
	return appendEndComment(payload), nil
}

func appendEndComment(payload []byte) []byte {
	if len(payload) == 0 {
		return []byte("#END\n")
	}
	const marker = "\n#END\n"
	if len(payload) >= len(marker) && string(payload[len(payload)-len(marker):]) == marker {
		return payload
	}
	if payload[len(payload)-1] != '\n' {
		payload = append(payload, '\n')
	}
	payload = append(payload, []byte("#END\n")...)
	return payload
}

func validateStringKeys(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for _, nested := range typed {
			if err := validateStringKeys(nested); err != nil {
				return err
			}
		}
	case map[any]any:
		for key, nested := range typed {
			if _, ok := key.(string); !ok {
				return fmt.Errorf("non-string map key: %v", key)
			}
			if err := validateStringKeys(nested); err != nil {
				return err
			}
		}
	case []any:
		for _, item := range typed {
			if err := validateStringKeys(item); err != nil {
				return err
			}
		}
	default:
	}
	return nil
}

func sortYAMLNode(node *yaml.Node) {
	if node == nil {
		return
	}
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			sortYAMLNode(child)
		}
	case yaml.MappingNode:
		if len(node.Content)%2 != 0 {
			return
		}
		type pair struct {
			key   *yaml.Node
			value *yaml.Node
		}
		pairs := make([]pair, 0, len(node.Content)/2)
		for i := 0; i < len(node.Content); i += 2 {
			pairs = append(pairs, pair{
				key:   node.Content[i],
				value: node.Content[i+1],
			})
		}
		for _, p := range pairs {
			sortYAMLNode(p.key)
			sortYAMLNode(p.value)
		}
		sort.Slice(pairs, func(i, j int) bool {
			return pairs[i].key.Value < pairs[j].key.Value
		})
		out := make([]*yaml.Node, 0, len(node.Content))
		for _, p := range pairs {
			out = append(out, p.key, p.value)
		}
		node.Content = out
	case yaml.SequenceNode:
		for _, child := range node.Content {
			sortYAMLNode(child)
		}
	default:
	}
}
