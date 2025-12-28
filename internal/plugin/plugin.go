package plugin

// PreRender modifies the merged config map before serialization.
type PreRender interface {
	Update(value map[string]any) (map[string]any, error)
}

// PostRender modifies the serialized payload.
type PostRender interface {
	Update(value []byte) ([]byte, error)
}

// PreRenderChain applies multiple PreRender plugins in order.
type PreRenderChain []PreRender

func (c PreRenderChain) Update(value map[string]any) (map[string]any, error) {
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

// PostRenderChain applies multiple PostRender plugins in order.
type PostRenderChain []PostRender

func (c PostRenderChain) Update(value []byte) ([]byte, error) {
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

// PostRenderFunc adapts a function to a PostRender plugin.
type PostRenderFunc func(value []byte) ([]byte, error)

func (p PostRenderFunc) Update(value []byte) ([]byte, error) {
	return p(value)
}

// PreRenderFunc adapts a function to a PreRender plugin.
type PreRenderFunc func(value map[string]any) (map[string]any, error)

func (p PreRenderFunc) Update(value map[string]any) (map[string]any, error) {
	return p(value)
}
