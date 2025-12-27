package plugin

type Plugin interface {
	Update(value map[string]any) (map[string]any, error)
}

type Chain []Plugin

func (c Chain) Update(value map[string]any) (map[string]any, error) {
	current := value
	for _, p := range c {
		if p == nil {
			continue
		}
		next, err := p.Update(current)
		if err != nil {
			return nil, err
		}
		current = next
	}
	return current, nil
}
