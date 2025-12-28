package plugin

type PreRender interface {
	Update(value map[string]any) (map[string]any, error)
}

type PostRender interface {
	Update(value []byte) ([]byte, error)
}

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

type PostRenderFunc func(value []byte) ([]byte, error)

func (p PostRenderFunc) Update(value []byte) ([]byte, error) {
	return p(value)
}

type PreRenderFunc func(value map[string]any) (map[string]any, error)

func (p PreRenderFunc) Update(value map[string]any) (map[string]any, error) {
	return p(value)
}
